package family

import (
	"context"
	"errors"
	"testing"

	"github.com/abhisek/mathiz/ent/familymember"
)

// TestCreateSpaceCreatesOwnerMembership: new spaces get their owner member
// row eagerly.
func TestCreateSpaceCreatesOwnerMembership(t *testing.T) {
	svc := newTestService(t)
	accountUID, spaceUID, _ := bootstrap(t, svc, "")
	ctx := context.Background()

	role, err := svc.MemberRole(ctx, spaceUID, accountUID)
	if err != nil {
		t.Fatalf("member role: %v", err)
	}
	if role != RoleOwner {
		t.Errorf("role = %q, want %q", role, RoleOwner)
	}
}

// TestSpaceForAccountLazyBackfill: an account owning a space without a member
// row (pre-membership data) gets its owner row created on the /me read path.
func TestSpaceForAccountLazyBackfill(t *testing.T) {
	svc := newTestService(t)
	accountUID, spaceUID, _ := bootstrap(t, svc, "")
	ctx := context.Background()

	// Simulate a pre-membership space: drop the owner's member row.
	if _, err := svc.client.FamilyMember.Delete().
		Where(familymember.AccountID(accountUID)).
		Exec(ctx); err != nil {
		t.Fatalf("delete member row: %v", err)
	}

	// MemberRole still resolves owners read-only (authz path), no row written.
	if role, err := svc.MemberRole(ctx, spaceUID, accountUID); err != nil || role != RoleOwner {
		t.Fatalf("owner fallback role = %q, %v", role, err)
	}
	if n, _ := svc.client.FamilyMember.Query().Count(ctx); n != 0 {
		t.Fatalf("MemberRole wrote %d member rows, want 0", n)
	}

	sp, role, err := svc.SpaceForAccount(ctx, accountUID)
	if err != nil {
		t.Fatalf("space for account: %v", err)
	}
	if sp == nil || sp.UID != spaceUID || role != RoleOwner {
		t.Fatalf("space = %v, role = %q", sp, role)
	}
	// The backfilled row now exists.
	m, err := svc.client.FamilyMember.Query().
		Where(familymember.AccountID(accountUID)).
		Only(ctx)
	if err != nil {
		t.Fatalf("backfilled row: %v", err)
	}
	if m.FamilySpaceID != spaceUID || m.Role != RoleOwner {
		t.Errorf("backfilled row = %+v", m)
	}

	// Accounts with no family at all resolve to (nil, "").
	stray, _ := svc.EnsureAccount(ctx, "sb-stray", "stray@example.com", "S")
	sp2, role2, err := svc.SpaceForAccount(ctx, stray.UID)
	if err != nil || sp2 != nil || role2 != "" {
		t.Errorf("no-family resolution = %v, %q, %v", sp2, role2, err)
	}
}

func TestInviteParentValidation(t *testing.T) {
	svc := newTestService(t)
	accountUID, spaceUID, _ := bootstrap(t, svc, "")
	ctx := context.Background()

	for _, bad := range []string{"", "   ", "not-an-email"} {
		if _, err := svc.InviteParent(ctx, spaceUID, bad, accountUID); !errors.Is(err, ErrBadEmail) {
			t.Errorf("invite %q: got %v, want ErrBadEmail", bad, err)
		}
	}
	// The owner's own email already belongs to this family.
	if _, err := svc.InviteParent(ctx, spaceUID, "PARENT@example.com", accountUID); !errors.Is(err, ErrAlreadyMember) {
		t.Errorf("invite owner email: got %v, want ErrAlreadyMember", err)
	}

	inv, err := svc.InviteParent(ctx, spaceUID, "  Co.Parent@Example.COM ", accountUID)
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if inv.Email != "co.parent@example.com" {
		t.Errorf("email not lowercased: %q", inv.Email)
	}
	if inv.Status != ParentInvitePending {
		t.Errorf("status = %q", inv.Status)
	}
	// Duplicate pending invite for the same space is rejected (any case).
	if _, err := svc.InviteParent(ctx, spaceUID, "CO.PARENT@example.com", accountUID); !errors.Is(err, ErrAlreadyInvited) {
		t.Errorf("dup invite: got %v, want ErrAlreadyInvited", err)
	}
}

func TestAcceptInviteLifecycle(t *testing.T) {
	svc := newTestService(t)
	ownerUID, spaceUID, _ := bootstrap(t, svc, "")
	ctx := context.Background()

	inv, err := svc.InviteParent(ctx, spaceUID, "co@example.com", ownerUID)
	if err != nil {
		t.Fatalf("invite: %v", err)
	}

	// Lookup is case-insensitive against the account email.
	if got, err := svc.PendingInviteForEmail(ctx, "CO@Example.com"); err != nil || got == nil || got.UID != inv.UID {
		t.Fatalf("pending lookup = %v, %v", got, err)
	}
	if got, _ := svc.PendingInviteForEmail(ctx, "other@example.com"); got != nil {
		t.Errorf("typo'd email matched an invite: %v", got)
	}

	// An account whose email doesn't match cannot accept (404 semantics).
	wrong, _ := svc.EnsureAccount(ctx, "sb-wrong", "wrong@example.com", "W")
	if _, err := svc.AcceptInvite(ctx, inv.UID, wrong.UID); !errors.Is(err, ErrNotFound) {
		t.Errorf("email-mismatch accept: got %v, want ErrNotFound", err)
	}

	co, _ := svc.EnsureAccount(ctx, "sb-co", "Co@Example.com", "Casey")
	m, err := svc.AcceptInvite(ctx, inv.UID, co.UID)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if m.FamilySpaceID != spaceUID || m.Role != RoleParent {
		t.Errorf("membership = %+v", m)
	}
	// Invite is now consumed.
	if _, err := svc.AcceptInvite(ctx, inv.UID, co.UID); !errors.Is(err, ErrParentInviteInvalid) {
		t.Errorf("double accept: got %v, want ErrParentInviteInvalid", err)
	}
	if got, _ := svc.PendingInviteForEmail(ctx, "co@example.com"); got != nil {
		t.Errorf("accepted invite still pending: %v", got)
	}

	// The co-parent resolves to the family with role parent.
	sp, role, err := svc.SpaceForAccount(ctx, co.UID)
	if err != nil || sp == nil || sp.UID != spaceUID || role != RoleParent {
		t.Fatalf("co-parent resolution = %v, %q, %v", sp, role, err)
	}

	// One family per account: the co-parent can't start their own space, and
	// no other family can invite their email.
	if _, err := svc.CreateSpace(ctx, co.UID, "Second Family"); !errors.Is(err, ErrSpaceExists) {
		t.Errorf("co-parent create space: got %v, want ErrSpaceExists", err)
	}
	other, _ := svc.EnsureAccount(ctx, "sb-other", "other-owner@example.com", "O")
	otherSpace, _ := svc.CreateSpace(ctx, other.UID, "The Others")
	if _, err := svc.InviteParent(ctx, otherSpace.UID, "co@example.com", other.UID); !errors.Is(err, ErrAlreadyMember) {
		t.Errorf("cross-family invite of a member: got %v, want ErrAlreadyMember", err)
	}

	// Members lists both, joined with account data, owner first.
	members, err := svc.Members(ctx, spaceUID)
	if err != nil {
		t.Fatalf("members: %v", err)
	}
	if len(members) != 2 || members[0].Role != RoleOwner || members[1].Role != RoleParent {
		t.Fatalf("members = %+v", members)
	}
	if members[1].Email != "Co@Example.com" || members[1].DisplayName != "Casey" {
		t.Errorf("join fields = %+v", members[1])
	}

	// The owner cannot be removed; the co-parent can, and loses access.
	if err := svc.RemoveParent(ctx, spaceUID, ownerUID); !errors.Is(err, ErrOwnerRemoval) {
		t.Errorf("remove owner: got %v, want ErrOwnerRemoval", err)
	}
	if err := svc.RemoveParent(ctx, spaceUID, co.UID); err != nil {
		t.Fatalf("remove co-parent: %v", err)
	}
	if _, err := svc.MemberRole(ctx, spaceUID, co.UID); !errors.Is(err, ErrNotFound) {
		t.Errorf("removed member role: got %v, want ErrNotFound", err)
	}
	if err := svc.RemoveParent(ctx, spaceUID, co.UID); !errors.Is(err, ErrNotFound) {
		t.Errorf("remove twice: got %v, want ErrNotFound", err)
	}
	// Their email can be invited again (old invite is accepted, not pending).
	if _, err := svc.InviteParent(ctx, spaceUID, "co@example.com", ownerUID); err != nil {
		t.Errorf("re-invite after removal: %v", err)
	}
}

func TestRevokeParentInvite(t *testing.T) {
	svc := newTestService(t)
	ownerUID, spaceUID, _ := bootstrap(t, svc, "")
	ctx := context.Background()

	inv, err := svc.InviteParent(ctx, spaceUID, "co@example.com", ownerUID)
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if err := svc.RevokeParentInvite(ctx, inv.UID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if got, _ := svc.PendingInviteForEmail(ctx, "co@example.com"); got != nil {
		t.Errorf("revoked invite still pending: %v", got)
	}
	co, _ := svc.EnsureAccount(ctx, "sb-co", "co@example.com", "Casey")
	if _, err := svc.AcceptInvite(ctx, inv.UID, co.UID); !errors.Is(err, ErrParentInviteInvalid) {
		t.Errorf("accept revoked: got %v, want ErrParentInviteInvalid", err)
	}
	if err := svc.RevokeParentInvite(ctx, inv.UID); !errors.Is(err, ErrParentInviteInvalid) {
		t.Errorf("revoke twice: got %v, want ErrParentInviteInvalid", err)
	}
	if err := svc.RevokeParentInvite(ctx, "no-such-invite"); !errors.Is(err, ErrNotFound) {
		t.Errorf("revoke missing: got %v, want ErrNotFound", err)
	}
	// After revocation the email is invitable again.
	if _, err := svc.InviteParent(ctx, spaceUID, "co@example.com", ownerUID); err != nil {
		t.Errorf("re-invite after revoke: %v", err)
	}
}

func TestMembersBackfillsOwner(t *testing.T) {
	svc := newTestService(t)
	accountUID, spaceUID, _ := bootstrap(t, svc, "")
	ctx := context.Background()

	// Pre-membership space: no member rows at all.
	if _, err := svc.client.FamilyMember.Delete().
		Where(familymember.AccountID(accountUID)).
		Exec(ctx); err != nil {
		t.Fatalf("delete member row: %v", err)
	}
	members, err := svc.Members(ctx, spaceUID)
	if err != nil {
		t.Fatalf("members: %v", err)
	}
	if len(members) != 1 || members[0].AccountID != accountUID || members[0].Role != RoleOwner {
		t.Fatalf("members = %+v", members)
	}
}
