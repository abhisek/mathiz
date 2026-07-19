package family

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/account"
	"github.com/abhisek/mathiz/ent/familymember"
	"github.com/abhisek/mathiz/ent/familyspace"
	"github.com/abhisek/mathiz/ent/parentinvite"
)

// Co-parent memberships (specs/12-saas.md, "Co-parents"). A family space
// holds one owner plus any number of parent members; family_member.account_id
// is unique, so an account belongs to at most one family. The space's
// owner_account_id stays the owner anchor — the owner's member row is
// backfilled lazily (SpaceForAccount / Members).

var (
	ErrBadEmail            = errors.New("a valid email address is required")
	ErrAlreadyMember       = errors.New("this account already belongs to a family")
	ErrAlreadyInvited      = errors.New("this email already has a pending invite")
	ErrOwnerRemoval        = errors.New("the family owner cannot be removed")
	ErrParentInviteInvalid = errors.New("this invite is no longer pending")
)

// Membership roles. Exactly two: the owner additionally manages billing and
// parents (authz.CanManageBilling / CanManageParents).
const (
	RoleOwner  = "owner"
	RoleParent = "parent"
)

// Parent invite statuses.
const (
	ParentInvitePending  = "pending"
	ParentInviteAccepted = "accepted"
	ParentInviteRevoked  = "revoked"
)

// Member is one parent of a family space, joined with account display data.
type Member struct {
	AccountID   string
	Email       string
	DisplayName string
	Role        string
	CreatedAt   time.Time
}

// Account returns an account by internal UID.
func (s *Service) Account(ctx context.Context, accountUID string) (*ent.Account, error) {
	a, err := s.client.Account.Query().
		Where(account.UID(accountUID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return a, err
}

// SpaceForAccount resolves the family an account belongs to via membership,
// with lazy migration: an account that owns a space but has no member row
// yet gets its owner member row created on this read. Returns (nil, "", nil)
// when the account has no family. The second return is the account's role.
func (s *Service) SpaceForAccount(ctx context.Context, accountUID string) (*ent.FamilySpace, string, error) {
	m, err := s.memberByAccount(ctx, accountUID)
	if err != nil {
		return nil, "", err
	}
	if m != nil {
		sp, err := s.Space(ctx, m.FamilySpaceID)
		if err != nil {
			return nil, "", err
		}
		return sp, m.Role, nil
	}
	// Lazy backfill: pre-membership owners get their member row here.
	sp, err := s.client.FamilySpace.Query().
		Where(familyspace.OwnerAccountID(accountUID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	if err := s.ensureOwnerMember(ctx, sp); err != nil {
		return nil, "", err
	}
	return sp, RoleOwner, nil
}

// MemberRole returns the account's role in the space, or ErrNotFound when it
// is not a member. Read-only (authz calls this on every request): an owner
// without a backfilled member row still resolves to RoleOwner via the
// space's owner anchor, but no row is written here.
func (s *Service) MemberRole(ctx context.Context, spaceUID, accountUID string) (string, error) {
	m, err := s.memberByAccount(ctx, accountUID)
	if err != nil {
		return "", err
	}
	if m != nil {
		if m.FamilySpaceID != spaceUID {
			return "", ErrNotFound
		}
		return m.Role, nil
	}
	sp, err := s.Space(ctx, spaceUID)
	if err != nil {
		return "", err
	}
	if sp.OwnerAccountID != accountUID {
		return "", ErrNotFound
	}
	return RoleOwner, nil
}

// Members lists the space's parents (owner first by creation time), joined
// with account emails and display names for the dashboard.
func (s *Service) Members(ctx context.Context, spaceUID string) ([]Member, error) {
	sp, err := s.Space(ctx, spaceUID)
	if err != nil {
		return nil, err
	}
	// Defensive backfill so the owner is always on the list, even if this
	// space has only ever been touched by pre-membership sessions.
	if err := s.ensureOwnerMember(ctx, sp); err != nil {
		return nil, err
	}
	rows, err := s.client.FamilyMember.Query().
		Where(familymember.FamilySpaceID(spaceUID)).
		Order(ent.Asc(familymember.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(rows))
	for i, m := range rows {
		ids[i] = m.AccountID
	}
	accounts, err := s.client.Account.Query().
		Where(account.UIDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	byUID := make(map[string]*ent.Account, len(accounts))
	for _, a := range accounts {
		byUID[a.UID] = a
	}
	out := make([]Member, 0, len(rows))
	for _, m := range rows {
		mem := Member{AccountID: m.AccountID, Role: m.Role, CreatedAt: m.CreatedAt}
		if a := byUID[m.AccountID]; a != nil {
			mem.Email = a.Email
			mem.DisplayName = a.DisplayName
		}
		out = append(out, mem)
	}
	return out, nil
}

// InviteParent records a co-parent invitation by email (no email is sent —
// the invitee sees it on /me after signing in). Rejects emails that already
// belong to any family and duplicate pending invites for this space.
func (s *Service) InviteParent(ctx context.Context, spaceUID, email, createdByAccountUID string) (*ent.ParentInvite, error) {
	email = NormalizeEmail(email)
	if email == "" || !strings.Contains(email, "@") {
		return nil, ErrBadEmail
	}
	if _, err := s.Space(ctx, spaceUID); err != nil {
		return nil, err
	}
	inFamily, err := s.emailHasFamily(ctx, email)
	if err != nil {
		return nil, err
	}
	if inFamily {
		return nil, ErrAlreadyMember
	}
	dup, err := s.client.ParentInvite.Query().
		Where(
			parentinvite.FamilySpaceID(spaceUID),
			parentinvite.Email(email),
			parentinvite.Status(ParentInvitePending),
		).
		Exist(ctx)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, ErrAlreadyInvited
	}
	return s.client.ParentInvite.Create().
		SetUID(uuid.NewString()).
		SetFamilySpaceID(spaceUID).
		SetEmail(email).
		SetStatus(ParentInvitePending).
		SetCreatedBy(createdByAccountUID).
		Save(ctx)
}

// ParentInvite returns a parent invite by UID.
func (s *Service) ParentInvite(ctx context.Context, inviteUID string) (*ent.ParentInvite, error) {
	inv, err := s.client.ParentInvite.Query().
		Where(parentinvite.UID(inviteUID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return inv, err
}

// PendingInviteForEmail returns the newest pending invite matching an email,
// or nil when none exists. Backs the /me accept banner.
func (s *Service) PendingInviteForEmail(ctx context.Context, email string) (*ent.ParentInvite, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return nil, nil
	}
	inv, err := s.client.ParentInvite.Query().
		Where(
			parentinvite.Email(email),
			parentinvite.Status(ParentInvitePending),
		).
		Order(ent.Desc(parentinvite.FieldCreatedAt)).
		First(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	return inv, err
}

// PendingInvites lists a space's pending co-parent invites, newest first.
func (s *Service) PendingInvites(ctx context.Context, spaceUID string) ([]*ent.ParentInvite, error) {
	return s.client.ParentInvite.Query().
		Where(
			parentinvite.FamilySpaceID(spaceUID),
			parentinvite.Status(ParentInvitePending),
		).
		Order(ent.Desc(parentinvite.FieldCreatedAt)).
		All(ctx)
}

// AcceptInvite turns a pending invite into a parent membership for the
// accepting account. The account email must match the invited email —
// a mismatch is ErrNotFound (don't confirm the invite exists to strangers).
func (s *Service) AcceptInvite(ctx context.Context, inviteUID, accountUID string) (*ent.FamilyMember, error) {
	inv, err := s.ParentInvite(ctx, inviteUID)
	if err != nil {
		return nil, err
	}
	acct, err := s.Account(ctx, accountUID)
	if err != nil {
		return nil, err
	}
	if NormalizeEmail(acct.Email) != inv.Email {
		return nil, ErrNotFound
	}
	if inv.Status != ParentInvitePending {
		return nil, ErrParentInviteInvalid
	}
	inFamily, err := s.accountHasFamily(ctx, accountUID)
	if err != nil {
		return nil, err
	}
	if inFamily {
		return nil, ErrAlreadyMember
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin accept tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	m, err := tx.FamilyMember.Create().
		SetUID(uuid.NewString()).
		SetFamilySpaceID(inv.FamilySpaceID).
		SetAccountID(accountUID).
		SetRole(RoleParent).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			// Lost a race with a concurrent accept: one family per account.
			return nil, ErrAlreadyMember
		}
		return nil, fmt.Errorf("create membership: %w", err)
	}
	if err := tx.ParentInvite.Update().
		Where(parentinvite.UID(inv.UID)).
		SetStatus(ParentInviteAccepted).
		Exec(ctx); err != nil {
		return nil, fmt.Errorf("mark invite accepted: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit accept tx: %w", err)
	}
	return m, nil
}

// RevokeParentInvite withdraws a pending invite.
func (s *Service) RevokeParentInvite(ctx context.Context, inviteUID string) error {
	inv, err := s.ParentInvite(ctx, inviteUID)
	if err != nil {
		return err
	}
	if inv.Status != ParentInvitePending {
		return ErrParentInviteInvalid
	}
	return inv.Update().SetStatus(ParentInviteRevoked).Exec(ctx)
}

// RemoveParent deletes a co-parent's membership. The owner can never be
// removed (the Stripe customer is the payer's identity).
func (s *Service) RemoveParent(ctx context.Context, spaceUID, accountUID string) error {
	sp, err := s.Space(ctx, spaceUID)
	if err != nil {
		return err
	}
	if sp.OwnerAccountID == accountUID {
		return ErrOwnerRemoval
	}
	m, err := s.client.FamilyMember.Query().
		Where(
			familymember.FamilySpaceID(spaceUID),
			familymember.AccountID(accountUID),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if m.Role == RoleOwner {
		return ErrOwnerRemoval
	}
	return s.client.FamilyMember.DeleteOne(m).Exec(ctx)
}

// ---- Helpers ----

// memberByAccount returns the account's member row, or nil when none exists.
func (s *Service) memberByAccount(ctx context.Context, accountUID string) (*ent.FamilyMember, error) {
	m, err := s.client.FamilyMember.Query().
		Where(familymember.AccountID(accountUID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	return m, err
}

// accountHasFamily reports whether an account has any membership — via a
// member row or as a not-yet-backfilled space owner.
func (s *Service) accountHasFamily(ctx context.Context, accountUID string) (bool, error) {
	m, err := s.memberByAccount(ctx, accountUID)
	if err != nil {
		return false, err
	}
	if m != nil {
		return true, nil
	}
	return s.client.FamilySpace.Query().
		Where(familyspace.OwnerAccountID(accountUID)).
		Exist(ctx)
}

// emailHasFamily reports whether any account with this email (compared
// case-insensitively) already belongs to a family.
func (s *Service) emailHasFamily(ctx context.Context, email string) (bool, error) {
	accounts, err := s.client.Account.Query().
		Where(account.EmailEqualFold(email)).
		All(ctx)
	if err != nil {
		return false, err
	}
	for _, a := range accounts {
		in, err := s.accountHasFamily(ctx, a.UID)
		if err != nil {
			return false, err
		}
		if in {
			return true, nil
		}
	}
	return false, nil
}

// ensureOwnerMember lazily creates the owner's member row (idempotent).
func (s *Service) ensureOwnerMember(ctx context.Context, sp *ent.FamilySpace) error {
	m, err := s.memberByAccount(ctx, sp.OwnerAccountID)
	if err != nil {
		return err
	}
	if m != nil {
		return nil // already backfilled (any row: account_id is unique)
	}
	err = s.client.FamilyMember.Create().
		SetUID(uuid.NewString()).
		SetFamilySpaceID(sp.UID).
		SetAccountID(sp.OwnerAccountID).
		SetRole(RoleOwner).
		Exec(ctx)
	if err != nil && !ent.IsConstraintError(err) {
		return fmt.Errorf("backfill owner membership: %w", err)
	}
	return nil // constraint error = lost a benign backfill race
}

// NormalizeEmail lowercases and trims an email for storage and comparison.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
