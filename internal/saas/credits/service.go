// Package credits is the credit ledger — the source of truth for what a
// family space can spend. Grants (starter/plan/topup) carry a remaining
// counter and optional expiry; debits consume grants FIFO by soonest
// expiry. Every entry has a unique source, so replayed webhooks and
// retried charges are structurally idempotent.
package credits

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/creditentry"
)

var (
	// ErrInsufficient means the family's balance can't cover the debit.
	ErrInsufficient = errors.New("not enough credits")
)

// Grant kinds.
const (
	KindStarter = "starter"
	KindPlan    = "plan"
	KindTopup   = "topup"
	kindDebit   = "debit"
)

// StarterCredits is the free grant on family space creation.
const StarterCredits = 30

// StarterExpiry is how long starter credits live.
const StarterExpiry = 30 * 24 * time.Hour

// Service manages the ledger. The mutex serializes debits within this
// process (single-replica assumption, consistent with session locks);
// the transaction keeps each debit atomic at the database level.
type Service struct {
	client *ent.Client
	mu     sync.Mutex
}

func New(client *ent.Client) *Service {
	return &Service{client: client}
}

// Balance is the sum of remaining credits on unexpired grants.
func (s *Service) Balance(ctx context.Context, spaceUID string) (int, error) {
	grants, err := s.client.CreditEntry.Query().
		Where(
			creditentry.FamilySpaceID(spaceUID),
			creditentry.RemainingGT(0),
		).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query grants: %w", err)
	}
	now := time.Now()
	total := 0
	for _, g := range grants {
		if g.ExpiresAt != nil && now.After(*g.ExpiresAt) {
			continue // lazily expired
		}
		total += g.Remaining
	}
	return total, nil
}

// Grant adds credits. Idempotent: a duplicate source is a silent no-op,
// so replayed billing webhooks can't double-grant.
func (s *Service) Grant(ctx context.Context, spaceUID, kind string, amount int, expiresAt *time.Time, source string) error {
	if amount <= 0 {
		return fmt.Errorf("grant amount must be positive")
	}
	create := s.client.CreditEntry.Create().
		SetUID(uuid.NewString()).
		SetFamilySpaceID(spaceUID).
		SetKind(kind).
		SetAmount(amount).
		SetRemaining(amount).
		SetSource(source)
	if expiresAt != nil {
		create.SetExpiresAt(*expiresAt)
	}
	if err := create.Exec(ctx); err != nil {
		if ent.IsConstraintError(err) {
			return nil // already granted (webhook replay / retry)
		}
		return fmt.Errorf("grant credits: %w", err)
	}
	return nil
}

// EnsureStarterGrant gives a family its one-time free credits. Safe and
// cheap to call on every request path that could be "first contact" — it
// runs at the charge chokepoint so families that predate billing being
// enabled still receive their credits without a parent having to visit the
// billing page first.
func (s *Service) EnsureStarterGrant(ctx context.Context, spaceUID string) error {
	source := "starter:" + spaceUID
	exists, err := s.client.CreditEntry.Query().
		Where(creditentry.Source(source)).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("check starter grant: %w", err)
	}
	if exists {
		return nil
	}
	expiry := time.Now().Add(StarterExpiry)
	return s.Grant(ctx, spaceUID, KindStarter, StarterCredits, &expiry, source)
}

// Debit consumes credits FIFO from unexpired grants, soonest expiry first
// (use-it-or-lose-it burns first). Idempotent by source: retrying a charge
// for the same session cannot double-debit. Returns ErrInsufficient
// (without writing anything) when the balance can't cover the amount.
func (s *Service) Debit(ctx context.Context, spaceUID string, amount int, source string) error {
	if amount <= 0 {
		return fmt.Errorf("debit amount must be positive")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin debit tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Idempotency: has this source already been charged?
	exists, err := tx.CreditEntry.Query().
		Where(creditentry.Source(source)).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("check debit source: %w", err)
	}
	if exists {
		return tx.Commit()
	}

	now := time.Now()
	grants, err := tx.CreditEntry.Query().
		Where(
			creditentry.FamilySpaceID(spaceUID),
			creditentry.RemainingGT(0),
		).
		Order(ent.Asc(creditentry.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("query grants: %w", err)
	}

	// Live grants, soonest expiry first, then oldest first.
	live := grants[:0]
	for _, g := range grants {
		if g.ExpiresAt != nil && now.After(*g.ExpiresAt) {
			continue
		}
		live = append(live, g)
	}
	sortByExpiry(live)

	total := 0
	for _, g := range live {
		total += g.Remaining
	}
	if total < amount {
		return ErrInsufficient
	}

	left := amount
	for _, g := range live {
		if left == 0 {
			break
		}
		take := min(left, g.Remaining)
		if err := tx.CreditEntry.UpdateOne(g).SetRemaining(g.Remaining - take).Exec(ctx); err != nil {
			return fmt.Errorf("consume grant: %w", err)
		}
		left -= take
	}

	if err := tx.CreditEntry.Create().
		SetUID(uuid.NewString()).
		SetFamilySpaceID(spaceUID).
		SetKind(kindDebit).
		SetAmount(-amount).
		SetRemaining(0).
		SetSource(source).
		Exec(ctx); err != nil {
		return fmt.Errorf("record debit: %w", err)
	}

	return tx.Commit()
}

// RenewPlanCredits retires the previous period's plan credits and grants the
// new period's batch as one idempotent unit. The source-existence check gates
// BOTH steps: a replayed renewal webhook must not re-expire the grant its own
// first delivery created. Everything runs in one transaction, so a concurrent
// replay that loses the source-uniqueness race rolls its expiry back too.
func (s *Service) RenewPlanCredits(ctx context.Context, spaceUID string, amount int, expiresAt *time.Time, source string) error {
	if amount <= 0 {
		return fmt.Errorf("grant amount must be positive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin renew tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	exists, err := tx.CreditEntry.Query().
		Where(creditentry.Source(source)).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("check renewal source: %w", err)
	}
	if exists {
		return tx.Commit() // replay — the whole renewal is a no-op
	}

	if _, err := tx.CreditEntry.Update().
		Where(
			creditentry.FamilySpaceID(spaceUID),
			creditentry.Kind(KindPlan),
			creditentry.RemainingGT(0),
		).
		SetRemaining(0).
		Save(ctx); err != nil {
		return fmt.Errorf("expire plan credits: %w", err)
	}

	create := tx.CreditEntry.Create().
		SetUID(uuid.NewString()).
		SetFamilySpaceID(spaceUID).
		SetKind(KindPlan).
		SetAmount(amount).
		SetRemaining(amount).
		SetSource(source)
	if expiresAt != nil {
		create.SetExpiresAt(*expiresAt)
	}
	if err := create.Exec(ctx); err != nil {
		if ent.IsConstraintError(err) {
			return nil // concurrent replay won; our expiry rolls back with the tx
		}
		return fmt.Errorf("grant plan credits: %w", err)
	}
	return tx.Commit()
}

// ExpirePlanCredits zeroes any remaining credits from a previous plan grant
// when a new period's grant arrives (plan credits roll over at most one
// period; the previous period's leftover is retired by the next renewal's
// call). Purchased top-ups are never touched.
func (s *Service) ExpirePlanCredits(ctx context.Context, spaceUID string, before time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.client.CreditEntry.Update().
		Where(
			creditentry.FamilySpaceID(spaceUID),
			creditentry.Kind(KindPlan),
			creditentry.RemainingGT(0),
			creditentry.CreatedAtLT(before),
		).
		SetRemaining(0).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("expire plan credits: %w", err)
	}
	return nil
}

func sortByExpiry(grants []*ent.CreditEntry) {
	slices.SortStableFunc(grants, func(a, b *ent.CreditEntry) int {
		switch {
		case expiresBefore(a, b):
			return -1
		case expiresBefore(b, a):
			return 1
		default:
			return 0
		}
	})
}

func expiresBefore(a, b *ent.CreditEntry) bool {
	switch {
	case a.ExpiresAt == nil && b.ExpiresAt == nil:
		return a.CreatedAt.Before(b.CreatedAt)
	case a.ExpiresAt == nil:
		return false // never-expiring burns last
	case b.ExpiresAt == nil:
		return true
	default:
		return a.ExpiresAt.Before(*b.ExpiresAt)
	}
}
