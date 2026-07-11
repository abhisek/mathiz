package store

import "context"

// ownerCtxKey is the unexported context key carrying the owner (learner) ID
// for the duration of a repo call. The owner guard (see ownerguard.go) reads
// it to scope every ent query and mutation on owner-scoped tables.
type ownerCtxKey struct{}

// withOwner returns a context carrying the given owner ID. LocalOwner ("")
// is a valid owner — the local CLI's single-learner view — so presence is
// tracked separately from the value ("present and empty" != "absent").
func withOwner(ctx context.Context, owner string) context.Context {
	return context.WithValue(ctx, ownerCtxKey{}, owner)
}

// ownerFromCtx extracts the owner ID from ctx. ok is false when no owner was
// stamped, which the owner guard treats as an unscoped (forbidden) access.
func ownerFromCtx(ctx context.Context) (string, bool) {
	owner, ok := ctx.Value(ownerCtxKey{}).(string)
	return owner, ok
}
