package family

import (
	"context"
	"errors"
	"fmt"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/account"
)

// RemapPair moves one account from its old provider subject to its new one.
type RemapPair struct {
	OldSubject string
	NewSubject string
}

// RemapResult counts what a remap changed (or would change, in a dry run).
type RemapResult struct {
	Remapped    int // accounts moved from the old provider to the new one
	AlreadyDone int // map rows a previous run already applied
}

// ErrRemap wraps every reason a remap refuses to write.
var ErrRemap = errors.New("remap refused")

// RemapProvider moves accounts from one auth provider to another in one
// transaction (specs/17 §6). Every account of from must be in the map: an
// account left behind would lose its family, so a partial map writes
// nothing. A row whose new subject already holds a to-account counts as
// done, which makes a rerun with the same map a no-op.
func (s *Service) RemapProvider(ctx context.Context, from, to string, pairs []RemapPair, dryRun bool) (RemapResult, error) {
	var res RemapResult
	if from == "" || to == "" || from == to {
		return res, fmt.Errorf("%w: --from and --to must be two different providers", ErrRemap)
	}
	if err := validatePairs(pairs); err != nil {
		return res, err
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return res, fmt.Errorf("begin remap: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	pending, err := tx.Account.Query().Where(account.AuthProvider(from)).All(ctx)
	if err != nil {
		return res, fmt.Errorf("load %s accounts: %w", from, err)
	}
	bySubject := make(map[string]*ent.Account, len(pending))
	for _, a := range pending {
		bySubject[a.AuthSubject] = a
	}
	mapped := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		mapped[p.OldSubject] = true
	}
	var unmapped int
	for sub := range bySubject {
		if !mapped[sub] {
			unmapped++
		}
	}
	if unmapped > 0 {
		return res, fmt.Errorf("%w: %d %s accounts are missing from the map; their parents would lose their families", ErrRemap, unmapped, from)
	}

	var updates []func() error
	for i, p := range pairs {
		row := i + 1
		taken, err := tx.Account.Query().
			Where(account.AuthProvider(to), account.AuthSubject(p.NewSubject)).
			Exist(ctx)
		if err != nil {
			return res, fmt.Errorf("check row %d: %w", row, err)
		}
		acct, ok := bySubject[p.OldSubject]
		switch {
		case !ok && taken:
			res.AlreadyDone++
		case !ok:
			return res, fmt.Errorf("%w: row %d: no %s account has subject %q", ErrRemap, row, from, p.OldSubject)
		case taken:
			return res, fmt.Errorf("%w: row %d: %s subject %q already belongs to another account", ErrRemap, row, to, p.NewSubject)
		default:
			res.Remapped++
			updates = append(updates, func() error {
				return tx.Account.UpdateOne(acct).SetAuthProvider(to).SetAuthSubject(p.NewSubject).Exec(ctx)
			})
		}
	}
	if dryRun {
		return res, nil
	}
	for _, update := range updates {
		if err := update(); err != nil {
			return RemapResult{}, fmt.Errorf("remap account: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return RemapResult{}, fmt.Errorf("commit remap: %w", err)
	}
	return res, nil
}

// validatePairs rejects a map that can't be applied unambiguously.
func validatePairs(pairs []RemapPair) error {
	oldSeen := make(map[string]int, len(pairs))
	newSeen := make(map[string]int, len(pairs))
	for i, p := range pairs {
		row := i + 1
		if p.OldSubject == "" || p.NewSubject == "" {
			return fmt.Errorf("%w: row %d: empty subject", ErrRemap, row)
		}
		if prev, dup := oldSeen[p.OldSubject]; dup {
			return fmt.Errorf("%w: rows %d and %d map the same old subject", ErrRemap, prev, row)
		}
		if prev, dup := newSeen[p.NewSubject]; dup {
			return fmt.Errorf("%w: rows %d and %d map to the same new subject", ErrRemap, prev, row)
		}
		oldSeen[p.OldSubject], newSeen[p.NewSubject] = row, row
	}
	return nil
}
