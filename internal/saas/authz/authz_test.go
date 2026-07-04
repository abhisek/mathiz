package authz

import (
	"context"
	"errors"
	"testing"

	"github.com/abhisek/mathiz/internal/saas/family"
	"github.com/abhisek/mathiz/internal/store"
)

type fixture struct {
	checker *Checker
	svc     *family.Service

	ownerAcct  string
	otherAcct  string
	spaceUID   string
	childUID   string
	inviteUID  string
	deviceUID  string
	otherChild string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	svc := family.New(st.Client())
	ctx := context.Background()

	owner, _ := svc.EnsureAccount(ctx, "sb-owner", "o@example.com", "Owner")
	other, _ := svc.EnsureAccount(ctx, "sb-other", "x@example.com", "Other")
	sp, _ := svc.CreateSpace(ctx, owner.UID, "Family A")
	child, _ := svc.AddChild(ctx, sp.UID, "Alice", 3, "")
	inv, _ := svc.CreateInvite(ctx, sp.UID, 0)
	_, dt, _ := svc.RedeemInvite(ctx, inv.Code, child.UID, "", "dev")

	sp2, _ := svc.CreateSpace(ctx, other.UID, "Family B")
	child2, _ := svc.AddChild(ctx, sp2.UID, "Zed", 4, "")

	return &fixture{
		checker:    NewChecker(svc),
		svc:        svc,
		ownerAcct:  owner.UID,
		otherAcct:  other.UID,
		spaceUID:   sp.UID,
		childUID:   child.UID,
		inviteUID:  inv.UID,
		deviceUID:  dt.UID,
		otherChild: child2.UID,
	}
}

func parent(accountUID string) Principal {
	return Principal{Kind: KindParent, AccountID: accountUID}
}

func childP(childUID, spaceUID string) Principal {
	return Principal{Kind: KindChild, ChildProfileID: childUID, FamilySpaceID: spaceUID}
}

func TestSpaceOwnership(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if err := f.checker.CanManageSpace(ctx, parent(f.ownerAcct), f.spaceUID); err != nil {
		t.Errorf("owner denied: %v", err)
	}
	if err := f.checker.CanManageSpace(ctx, parent(f.otherAcct), f.spaceUID); !errors.Is(err, ErrDenied) {
		t.Errorf("non-owner allowed: %v", err)
	}
	if err := f.checker.CanManageSpace(ctx, parent(f.ownerAcct), "nope"); !errors.Is(err, ErrDenied) {
		t.Errorf("missing space allowed: %v", err)
	}
	// Children never manage spaces.
	if err := f.checker.CanManageSpace(ctx, childP(f.childUID, f.spaceUID), f.spaceUID); !errors.Is(err, ErrDenied) {
		t.Errorf("child allowed to manage space: %v", err)
	}
}

func TestChildInviteDeviceOwnership(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if err := f.checker.CanManageChild(ctx, parent(f.ownerAcct), f.childUID); err != nil {
		t.Errorf("owner denied child: %v", err)
	}
	if err := f.checker.CanManageChild(ctx, parent(f.otherAcct), f.childUID); !errors.Is(err, ErrDenied) {
		t.Errorf("non-owner allowed child: %v", err)
	}
	if err := f.checker.CanManageInvite(ctx, parent(f.otherAcct), f.inviteUID); !errors.Is(err, ErrDenied) {
		t.Errorf("non-owner allowed invite: %v", err)
	}
	if err := f.checker.CanManageDevice(ctx, parent(f.otherAcct), f.deviceUID); !errors.Is(err, ErrDenied) {
		t.Errorf("non-owner allowed device: %v", err)
	}
	if err := f.checker.CanManageDevice(ctx, parent(f.ownerAcct), f.deviceUID); err != nil {
		t.Errorf("owner denied device: %v", err)
	}
}

func TestLearnAs(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if err := f.checker.CanLearnAs(ctx, childP(f.childUID, f.spaceUID), f.childUID); err != nil {
		t.Errorf("child denied own session: %v", err)
	}
	// A child cannot learn as a sibling or any other profile.
	if err := f.checker.CanLearnAs(ctx, childP(f.childUID, f.spaceUID), f.otherChild); !errors.Is(err, ErrDenied) {
		t.Errorf("child allowed as other child: %v", err)
	}
	// Parents do not impersonate learners.
	if err := f.checker.CanLearnAs(ctx, parent(f.ownerAcct), f.childUID); !errors.Is(err, ErrDenied) {
		t.Errorf("parent allowed to learn as child: %v", err)
	}
	// Empty principal is denied.
	if err := f.checker.CanLearnAs(ctx, Principal{}, f.childUID); !errors.Is(err, ErrDenied) {
		t.Errorf("empty principal allowed: %v", err)
	}
}
