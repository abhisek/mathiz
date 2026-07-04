package family

import (
	"context"
	"errors"
	"strings"
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

// bootstrap creates an account, family space and one child, returning their IDs.
func bootstrap(t *testing.T, svc *Service, pin string) (accountUID, spaceUID, childUID string) {
	t.Helper()
	ctx := context.Background()
	acct, err := svc.EnsureAccount(ctx, "sb-user-1", "parent@example.com", "Pat Parent")
	if err != nil {
		t.Fatalf("ensure account: %v", err)
	}
	sp, err := svc.CreateSpace(ctx, acct.UID, "The Parkers")
	if err != nil {
		t.Fatalf("create space: %v", err)
	}
	child, err := svc.AddChild(ctx, sp.UID, "Alice", 3, pin)
	if err != nil {
		t.Fatalf("add child: %v", err)
	}
	return acct.UID, sp.UID, child.UID
}

func TestEnsureAccountIdempotent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	a1, err := svc.EnsureAccount(ctx, "sb-1", "a@example.com", "A")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	a2, err := svc.EnsureAccount(ctx, "sb-1", "new@example.com", "A2")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if a1.UID != a2.UID {
		t.Errorf("account UID changed on re-ensure: %s vs %s", a1.UID, a2.UID)
	}
	if a2.Email != "new@example.com" || a2.DisplayName != "A2" {
		t.Errorf("claims not refreshed: %+v", a2)
	}
}

func TestCreateSpaceOnePerAccount(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	acct, _ := svc.EnsureAccount(ctx, "sb-1", "a@example.com", "A")

	if _, err := svc.CreateSpace(ctx, acct.UID, "First"); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err := svc.CreateSpace(ctx, acct.UID, "Second")
	if !errors.Is(err, ErrSpaceExists) {
		t.Errorf("expected ErrSpaceExists, got %v", err)
	}
}

func TestAddChildValidation(t *testing.T) {
	svc := newTestService(t)
	_, spaceUID, _ := bootstrap(t, svc, "")
	ctx := context.Background()

	if _, err := svc.AddChild(ctx, spaceUID, "", 3, ""); !errors.Is(err, ErrBadName) {
		t.Errorf("empty name: got %v", err)
	}
	if _, err := svc.AddChild(ctx, spaceUID, "Bob", 9, ""); !errors.Is(err, ErrBadGrade) {
		t.Errorf("bad grade: got %v", err)
	}
	if _, err := svc.AddChild(ctx, spaceUID, "Bob", 4, "12ab"); !errors.Is(err, ErrBadPIN) {
		t.Errorf("bad pin: got %v", err)
	}
	if _, err := svc.AddChild(ctx, spaceUID, "Bob", 4, "1234"); err != nil {
		t.Errorf("valid child: %v", err)
	}
}

func TestJoinFlowWithPIN(t *testing.T) {
	svc := newTestService(t)
	_, spaceUID, childUID := bootstrap(t, svc, "4321")
	ctx := context.Background()

	inv, err := svc.CreateInvite(ctx, spaceUID, 0)
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	if !strings.Contains(inv.Code, "-") {
		t.Errorf("unexpected code shape: %q", inv.Code)
	}

	// Preview shows the family and its children.
	prev, err := svc.PreviewJoin(ctx, strings.ToLower(inv.Code)) // case-insensitive
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if prev.SpaceName != "The Parkers" || len(prev.Children) != 1 {
		t.Errorf("preview = %+v", prev)
	}

	// Redeem without PIN → required.
	if _, _, err := svc.RedeemInvite(ctx, inv.Code, childUID, "", "iPad"); !errors.Is(err, ErrPINRequired) {
		t.Errorf("no pin: got %v", err)
	}
	// Wrong PIN → mismatch.
	if _, _, err := svc.RedeemInvite(ctx, inv.Code, childUID, "0000", "iPad"); !errors.Is(err, ErrPINMismatch) {
		t.Errorf("wrong pin: got %v", err)
	}
	// Correct PIN → token.
	plaintext, dt, err := svc.RedeemInvite(ctx, inv.Code, childUID, "4321", "iPad")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if !strings.HasPrefix(plaintext, TokenPrefix) {
		t.Errorf("token prefix: %q", plaintext)
	}
	if dt.ChildProfileID != childUID {
		t.Errorf("token child = %s, want %s", dt.ChildProfileID, childUID)
	}

	// Token resolves to the child.
	rdt, child, err := svc.ResolveDeviceToken(ctx, plaintext)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if rdt.UID != dt.UID || child.UID != childUID {
		t.Errorf("resolve mismatch: %s / %s", rdt.UID, child.UID)
	}

	// Garbage tokens fail.
	if _, _, err := svc.ResolveDeviceToken(ctx, "mzd_bogus-token-value-here"); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("bogus token: got %v", err)
	}
	if _, _, err := svc.ResolveDeviceToken(ctx, ""); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("empty token: got %v", err)
	}
}

func TestInviteLifecycle(t *testing.T) {
	svc := newTestService(t)
	_, spaceUID, childUID := bootstrap(t, svc, "")
	ctx := context.Background()

	// Revoked invite is dead.
	inv, _ := svc.CreateInvite(ctx, spaceUID, 0)
	if err := svc.RevokeInvite(ctx, inv.UID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.PreviewJoin(ctx, inv.Code); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("revoked preview: got %v", err)
	}
	if _, _, err := svc.RedeemInvite(ctx, inv.Code, childUID, "", "x"); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("revoked redeem: got %v", err)
	}

	// Expired invite is dead.
	expired, _ := svc.CreateInvite(ctx, spaceUID, time.Nanosecond)
	time.Sleep(2 * time.Millisecond)
	if _, err := svc.PreviewJoin(ctx, expired.Code); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("expired preview: got %v", err)
	}

	// Active list only shows live codes.
	live, _ := svc.CreateInvite(ctx, spaceUID, 0)
	actives, err := svc.ActiveInvites(ctx, spaceUID)
	if err != nil {
		t.Fatalf("active invites: %v", err)
	}
	if len(actives) != 1 || actives[0].UID != live.UID {
		t.Errorf("actives = %d, want just the live one", len(actives))
	}
}

func TestRedeemCrossFamilyDenied(t *testing.T) {
	svc := newTestService(t)
	_, spaceUID, _ := bootstrap(t, svc, "")
	ctx := context.Background()

	// Second family with its own child.
	acct2, _ := svc.EnsureAccount(ctx, "sb-user-2", "other@example.com", "Other")
	sp2, _ := svc.CreateSpace(ctx, acct2.UID, "The Others")
	otherChild, _ := svc.AddChild(ctx, sp2.UID, "Zed", 4, "")

	inv, _ := svc.CreateInvite(ctx, spaceUID, 0)
	// Family-1 code must not redeem for a family-2 profile.
	if _, _, err := svc.RedeemInvite(ctx, inv.Code, otherChild.UID, "", "x"); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("cross-family redeem: got %v", err)
	}
}

func TestArchiveChildRevokesDevices(t *testing.T) {
	svc := newTestService(t)
	_, spaceUID, childUID := bootstrap(t, svc, "")
	ctx := context.Background()

	inv, _ := svc.CreateInvite(ctx, spaceUID, 0)
	plaintext, _, err := svc.RedeemInvite(ctx, inv.Code, childUID, "", "laptop")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	archived := true
	if _, err := svc.UpdateChild(ctx, childUID, UpdateChildOpts{Archived: &archived}); err != nil {
		t.Fatalf("archive: %v", err)
	}

	if _, _, err := svc.ResolveDeviceToken(ctx, plaintext); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("archived child token still resolves: %v", err)
	}
	// Archived children disappear from the roster.
	kids, _ := svc.Children(ctx, spaceUID)
	if len(kids) != 0 {
		t.Errorf("children = %d, want 0 after archive", len(kids))
	}
}

func TestRevokeDevice(t *testing.T) {
	svc := newTestService(t)
	_, spaceUID, childUID := bootstrap(t, svc, "")
	ctx := context.Background()

	inv, _ := svc.CreateInvite(ctx, spaceUID, 0)
	plaintext, dt, _ := svc.RedeemInvite(ctx, inv.Code, childUID, "", "tablet")

	if err := svc.RevokeDevice(ctx, dt.UID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, _, err := svc.ResolveDeviceToken(ctx, plaintext); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("revoked token resolves: %v", err)
	}
	devices, _ := svc.Devices(ctx, childUID)
	if len(devices) != 0 {
		t.Errorf("devices = %d, want 0", len(devices))
	}
}

func TestNormalizeJoinCode(t *testing.T) {
	cases := map[string]string{
		"tiger-4207":   "TIGER-4207",
		" TIGER-4207 ": "TIGER-4207",
		"tiger - 4207": "TIGER-4207",
	}
	for in, want := range cases {
		if got := NormalizeJoinCode(in); got != want {
			t.Errorf("NormalizeJoinCode(%q) = %q, want %q", in, got, want)
		}
	}
}
