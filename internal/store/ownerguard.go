package store

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"
	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/intercept"
)

// Owner guard: central, fail-closed tenant isolation for owner-scoped tables.
//
// Every repo method stamps its owner into the context (withOwner). A query
// interceptor and a mutation hook registered on the ent client in store.Open
// then enforce isolation structurally:
//
//   - Queries on owner-scoped types get an owner_id predicate added from the
//     context. A context without an owner errors instead of returning
//     another family's rows — a forgotten wrapper fails closed.
//   - Creates get owner_id stamped from the context; a value conflicting
//     with the context owner errors.
//   - Updates/deletes are constrained to the context owner's rows.
//
// The explicit SetOwnerID/Where(OwnerID(...)) calls in the repos remain as
// belt and suspenders; the guard's added predicate is duplicative but
// harmless.

// ownerIDColumn is the column stamped on every owner-scoped row.
const ownerIDColumn = "owner_id"

// ownerScopedTypes lists every ent type whose rows belong to a single
// learner: all event schemas embedding EventMixin, plus Snapshot.
// Family-scoped control-plane types (Account, FamilySpace, ChildProfile,
// Invite, DeviceToken, CreditEntry, BillingState) are intentionally NOT
// here — they have no owner_id column and are scoped by the authz layer.
var ownerScopedTypes = map[string]bool{
	ent.TypeAnswerEvent:         true,
	ent.TypeDiagnosisEvent:      true,
	ent.TypeGemEvent:            true,
	ent.TypeHintEvent:           true,
	ent.TypeLearnerProfileEvent: true,
	ent.TypeLessonEvent:         true,
	ent.TypeLLMRequestEvent:     true,
	ent.TypeMasteryEvent:        true,
	ent.TypeSessionEvent:        true,
	ent.TypeSnapshot:            true,
}

// registerOwnerGuard installs the query interceptor and mutation hook on the
// client. Called once, from store.Open — the single choke point every
// deployment mode shares.
func registerOwnerGuard(client *ent.Client) {
	client.Intercept(ownerQueryInterceptor())
	client.Use(ownerMutationHook())
}

// ownerQueryInterceptor adds the owner_id predicate to every query on an
// owner-scoped type, reading the owner from the context. No owner in the
// context is an error: unscoped reads fail closed instead of leaking.
func ownerQueryInterceptor() ent.Interceptor {
	return intercept.TraverseFunc(func(ctx context.Context, q intercept.Query) error {
		if !ownerScopedTypes[q.Type()] {
			return nil
		}
		owner, ok := ownerFromCtx(ctx)
		if !ok {
			return fmt.Errorf("store: unscoped query on owner-scoped table %s: context carries no owner (use Store.EventRepoFor/SnapshotRepoFor)", q.Type())
		}
		q.WhereP(sql.FieldEQ(ownerIDColumn, owner))
		return nil
	})
}

// ownerMutationHook stamps owner_id on creates and constrains
// updates/deletes to the context owner's rows. No owner in the context is an
// error; so is a create whose owner_id conflicts with the context owner.
func ownerMutationHook() ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if !ownerScopedTypes[m.Type()] {
				return next.Mutate(ctx, m)
			}
			owner, ok := ownerFromCtx(ctx)
			if !ok {
				return nil, fmt.Errorf("store: unscoped mutation on owner-scoped table %s: context carries no owner (use Store.EventRepoFor/SnapshotRepoFor)", m.Type())
			}
			if m.Op().Is(ent.OpCreate) {
				if err := stampCreateOwner(m, owner); err != nil {
					return nil, err
				}
			} else {
				// Updates and deletes: fence the statement to the context
				// owner's rows regardless of what predicates the caller set.
				wp, ok := m.(interface{ WhereP(...func(*sql.Selector)) })
				if !ok {
					return nil, fmt.Errorf("store: mutation on owner-scoped table %s does not support owner fencing", m.Type())
				}
				wp.WhereP(sql.FieldEQ(ownerIDColumn, owner))
			}
			return next.Mutate(ctx, m)
		})
	}
}

// stampCreateOwner ensures a create mutation carries the context owner.
// The schema default ("") is applied before hooks run, so an empty owner_id
// with a non-empty context owner means "not explicitly set": stamp the
// context owner. Any other mismatch is a conflict and errors.
func stampCreateOwner(m ent.Mutation, owner string) error {
	cur, exists := m.Field(ownerIDColumn)
	if exists {
		got, _ := cur.(string)
		if got == owner {
			return nil
		}
		if got != LocalOwner {
			return fmt.Errorf("store: owner conflict on %s: mutation sets owner %q but context owner is %q", m.Type(), got, owner)
		}
	}
	if err := m.SetField(ownerIDColumn, owner); err != nil {
		return fmt.Errorf("store: stamp owner on %s: %w", m.Type(), err)
	}
	return nil
}
