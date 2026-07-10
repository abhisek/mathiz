package credits

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/abhisek/mathiz/internal/store"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st.Client())
}

func TestGrantAndBalance(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	if err := s.Grant(ctx, "fam-1", KindTopup, 100, nil, "topup:1"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	bal, err := s.Balance(ctx, "fam-1")
	if err != nil || bal != 100 {
		t.Fatalf("balance = %d (%v), want 100", bal, err)
	}

	// Duplicate source is a silent no-op (webhook replay).
	if err := s.Grant(ctx, "fam-1", KindTopup, 100, nil, "topup:1"); err != nil {
		t.Fatalf("replayed grant: %v", err)
	}
	bal, _ = s.Balance(ctx, "fam-1")
	if bal != 100 {
		t.Errorf("balance after replay = %d, want 100", bal)
	}

	// Another family's balance is separate.
	bal, _ = s.Balance(ctx, "fam-2")
	if bal != 0 {
		t.Errorf("fam-2 balance = %d, want 0", bal)
	}
}

func TestDebitFIFOAndIdempotency(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	// An expiring grant and a permanent one: the expiring credits burn first.
	soon := time.Now().Add(time.Hour)
	if err := s.Grant(ctx, "fam-1", KindStarter, 5, &soon, "starter:fam-1"); err != nil {
		t.Fatalf("grant starter: %v", err)
	}
	if err := s.Grant(ctx, "fam-1", KindTopup, 10, nil, "topup:1"); err != nil {
		t.Fatalf("grant topup: %v", err)
	}

	if err := s.Debit(ctx, "fam-1", 7, "session:a"); err != nil {
		t.Fatalf("debit: %v", err)
	}
	bal, _ := s.Balance(ctx, "fam-1")
	if bal != 8 {
		t.Fatalf("balance = %d, want 8", bal)
	}

	// Retrying the same charge (same source) doesn't double-debit.
	if err := s.Debit(ctx, "fam-1", 7, "session:a"); err != nil {
		t.Fatalf("retried debit: %v", err)
	}
	bal, _ = s.Balance(ctx, "fam-1")
	if bal != 8 {
		t.Errorf("balance after retry = %d, want 8", bal)
	}

	// The expiring grant burned first: all 5 starter + 2 topup consumed.
	// Expire check: even if the starter grant now expires, balance holds.
	if err := s.Debit(ctx, "fam-1", 8, "session:b"); err != nil {
		t.Fatalf("debit rest: %v", err)
	}
	if err := s.Debit(ctx, "fam-1", 1, "session:c"); !errors.Is(err, ErrInsufficient) {
		t.Errorf("over-debit: got %v, want ErrInsufficient", err)
	}
}

func TestExpiredGrantsDontCount(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	past := time.Now().Add(-time.Minute)
	if err := s.Grant(ctx, "fam-1", KindStarter, 30, &past, "starter:fam-1"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	bal, _ := s.Balance(ctx, "fam-1")
	if bal != 0 {
		t.Errorf("expired balance = %d, want 0", bal)
	}
	if err := s.Debit(ctx, "fam-1", 1, "session:a"); !errors.Is(err, ErrInsufficient) {
		t.Errorf("debit against expired: got %v", err)
	}
}

func TestInsufficientWritesNothing(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	if err := s.Grant(ctx, "fam-1", KindTopup, 3, nil, "topup:1"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := s.Debit(ctx, "fam-1", 5, "session:big"); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("expected ErrInsufficient, got %v", err)
	}
	// Balance untouched, and the failed source is reusable.
	bal, _ := s.Balance(ctx, "fam-1")
	if bal != 3 {
		t.Errorf("balance = %d, want 3", bal)
	}
	if err := s.Debit(ctx, "fam-1", 3, "session:big"); err != nil {
		t.Errorf("retry with affordable amount: %v", err)
	}
}

func TestExpirePlanCredits(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	if err := s.Grant(ctx, "fam-1", KindPlan, 150, nil, "sub:period1"); err != nil {
		t.Fatalf("grant plan: %v", err)
	}
	if err := s.Grant(ctx, "fam-1", KindTopup, 40, nil, "topup:1"); err != nil {
		t.Fatalf("grant topup: %v", err)
	}
	if err := s.Debit(ctx, "fam-1", 50, "session:a"); err != nil {
		t.Fatalf("debit: %v", err)
	}

	// Renewal: previous plan leftover retires; purchased top-ups survive.
	if err := s.ExpirePlanCredits(ctx, "fam-1", time.Now()); err != nil {
		t.Fatalf("expire plan: %v", err)
	}
	if err := s.Grant(ctx, "fam-1", KindPlan, 150, nil, "sub:period2"); err != nil {
		t.Fatalf("grant period2: %v", err)
	}
	bal, _ := s.Balance(ctx, "fam-1")
	// 40 topup - 50 debited... FIFO burned plan first (no expiry: oldest
	// first → plan grant was older), so debit consumed 50 plan credits.
	// Remaining: plan 100 (expired to 0) + topup 40 + new plan 150 = 190.
	if bal != 190 {
		t.Errorf("balance after renewal = %d, want 190", bal)
	}

	if err := s.EnsureStarterGrant(ctx, "fam-1"); err != nil {
		t.Fatalf("starter: %v", err)
	}
	if err := s.EnsureStarterGrant(ctx, "fam-1"); err != nil {
		t.Fatalf("starter again: %v", err)
	}
	bal2, _ := s.Balance(ctx, "fam-1")
	if bal2 != bal+StarterCredits {
		t.Errorf("starter granted %d, want exactly once (+%d)", bal2-bal, StarterCredits)
	}
}

func TestRenewPlanCreditsReplaySafe(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	if err := s.RenewPlanCredits(ctx, "fam-1", 150, nil, "sub:ev1"); err != nil {
		t.Fatalf("period 1: %v", err)
	}
	if err := s.Debit(ctx, "fam-1", 20, "session:a"); err != nil {
		t.Fatalf("debit: %v", err)
	}
	if err := s.Grant(ctx, "fam-1", KindTopup, 40, nil, "topup:1"); err != nil {
		t.Fatalf("topup: %v", err)
	}

	// Renewal retires the old plan leftover (130), keeps the top-up.
	if err := s.RenewPlanCredits(ctx, "fam-1", 150, nil, "sub:ev2"); err != nil {
		t.Fatalf("renewal: %v", err)
	}
	bal, _ := s.Balance(ctx, "fam-1")
	if bal != 190 {
		t.Fatalf("balance after renewal = %d, want 190 (150 plan + 40 topup)", bal)
	}

	// A replayed renewal webhook must be a complete no-op: it must NOT
	// re-expire the grant its own first delivery created.
	if err := s.RenewPlanCredits(ctx, "fam-1", 150, nil, "sub:ev2"); err != nil {
		t.Fatalf("replay: %v", err)
	}
	bal, _ = s.Balance(ctx, "fam-1")
	if bal != 190 {
		t.Errorf("balance after replay = %d, want 190 — replay wiped plan credits", bal)
	}
}
