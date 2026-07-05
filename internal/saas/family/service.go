// Package family implements the SaaS control plane: parent accounts,
// family spaces, child profiles, join codes, and child device tokens.
//
// The service owns data invariants (uniqueness, liveness, PIN checks).
// Permission decisions live in internal/saas/authz — callers are expected
// to authorize before mutating.
package family

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/account"
	"github.com/abhisek/mathiz/ent/childprofile"
	"github.com/abhisek/mathiz/ent/devicetoken"
	"github.com/abhisek/mathiz/ent/familyspace"
	"github.com/abhisek/mathiz/ent/invite"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrSpaceExists   = errors.New("account already owns a family space")
	ErrInviteInvalid = errors.New("join code is invalid, expired, or revoked")
	ErrPINRequired   = errors.New("this profile requires a PIN")
	ErrPINMismatch   = errors.New("incorrect PIN")
	ErrBadPIN        = errors.New("PIN must be 4-6 digits")
	ErrBadGrade      = errors.New("grade must be between 2 and 5")
	ErrBadName       = errors.New("name must not be empty")
	ErrArchived      = errors.New("child profile is archived")
	ErrTokenInvalid  = errors.New("device token is invalid or revoked")
)

// TokenPrefix marks Mathiz child device tokens.
const TokenPrefix = "mzd_"

// DefaultInviteTTL is how long a join code stays redeemable.
const DefaultInviteTTL = 7 * 24 * time.Hour

// MaxInviteTTL caps parent-chosen invite expiry. A join code is a
// family-scoped credential; the cap bounds the exposure window of a code
// that leaks (profile PINs are the per-child guard).
const MaxInviteTTL = 90 * 24 * time.Hour

var pinPattern = regexp.MustCompile(`^\d{4,6}$`)

// Service implements control-plane operations on top of the ent client.
type Service struct {
	client *ent.Client
}

func New(client *ent.Client) *Service {
	return &Service{client: client}
}

// ---- Accounts ----

// EnsureAccount returns the account for a verified Supabase user, creating it
// on first sight. Email and display name are refreshed from the latest token.
func (s *Service) EnsureAccount(ctx context.Context, supabaseUserID, email, displayName string) (*ent.Account, error) {
	if supabaseUserID == "" {
		return nil, fmt.Errorf("empty supabase user id")
	}

	existing, err := s.client.Account.Query().
		Where(account.SupabaseUserID(supabaseUserID)).
		Only(ctx)
	switch {
	case err == nil:
		if (email != "" && existing.Email != email) || (displayName != "" && existing.DisplayName != displayName) {
			upd := existing.Update()
			if email != "" {
				upd.SetEmail(email)
			}
			if displayName != "" {
				upd.SetDisplayName(displayName)
			}
			return upd.Save(ctx)
		}
		return existing, nil
	case !ent.IsNotFound(err):
		return nil, fmt.Errorf("query account: %w", err)
	}

	created, err := s.client.Account.Create().
		SetUID(uuid.NewString()).
		SetSupabaseUserID(supabaseUserID).
		SetEmail(email).
		SetDisplayName(displayName).
		Save(ctx)
	if err != nil {
		// Lost a race with a concurrent first request for the same user.
		if ent.IsConstraintError(err) {
			return s.client.Account.Query().
				Where(account.SupabaseUserID(supabaseUserID)).
				Only(ctx)
		}
		return nil, fmt.Errorf("create account: %w", err)
	}
	return created, nil
}

// ---- Family spaces ----

// CreateSpace creates the account's family space. V1 allows one per account,
// enforced by a unique constraint on owner_account_id so concurrent creates
// can't produce duplicates.
func (s *Service) CreateSpace(ctx context.Context, ownerAccountUID, name string) (*ent.FamilySpace, error) {
	if name == "" {
		return nil, ErrBadName
	}
	sp, err := s.client.FamilySpace.Create().
		SetUID(uuid.NewString()).
		SetOwnerAccountID(ownerAccountUID).
		SetName(name).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return nil, ErrSpaceExists
	}
	return sp, err
}

// SpaceByOwner returns the space owned by the account, or nil if none exists.
func (s *Service) SpaceByOwner(ctx context.Context, ownerAccountUID string) (*ent.FamilySpace, error) {
	sp, err := s.client.FamilySpace.Query().
		Where(familyspace.OwnerAccountID(ownerAccountUID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	return sp, err
}

// Space returns a family space by UID.
func (s *Service) Space(ctx context.Context, spaceUID string) (*ent.FamilySpace, error) {
	sp, err := s.client.FamilySpace.Query().
		Where(familyspace.UID(spaceUID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return sp, err
}

// RenameSpace updates the space name.
func (s *Service) RenameSpace(ctx context.Context, spaceUID, name string) (*ent.FamilySpace, error) {
	if name == "" {
		return nil, ErrBadName
	}
	sp, err := s.Space(ctx, spaceUID)
	if err != nil {
		return nil, err
	}
	return sp.Update().SetName(name).Save(ctx)
}

// ---- Child profiles ----

// AddChild creates a child profile in a space. pin may be empty (no PIN).
func (s *Service) AddChild(ctx context.Context, spaceUID, name string, grade int, pin string) (*ent.ChildProfile, error) {
	if name == "" {
		return nil, ErrBadName
	}
	if grade < 2 || grade > 5 {
		return nil, ErrBadGrade
	}
	pinHash, err := hashPIN(pin)
	if err != nil {
		return nil, err
	}
	if _, err := s.Space(ctx, spaceUID); err != nil {
		return nil, err
	}
	return s.client.ChildProfile.Create().
		SetUID(uuid.NewString()).
		SetFamilySpaceID(spaceUID).
		SetName(name).
		SetGrade(grade).
		SetPinHash(pinHash).
		Save(ctx)
}

// Child returns a child profile by UID.
func (s *Service) Child(ctx context.Context, childUID string) (*ent.ChildProfile, error) {
	c, err := s.client.ChildProfile.Query().
		Where(childprofile.UID(childUID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return c, err
}

// Children lists all non-archived child profiles in a space.
func (s *Service) Children(ctx context.Context, spaceUID string) ([]*ent.ChildProfile, error) {
	return s.client.ChildProfile.Query().
		Where(childprofile.FamilySpaceID(spaceUID), childprofile.Archived(false)).
		Order(ent.Asc(childprofile.FieldCreatedAt)).
		All(ctx)
}

// UpdateChildOpts carries optional child profile updates. Nil fields are
// left unchanged. Setting PIN to the empty string clears it.
type UpdateChildOpts struct {
	Name     *string
	Grade    *int
	PIN      *string
	Archived *bool
}

// UpdateChild applies the provided updates to a child profile.
func (s *Service) UpdateChild(ctx context.Context, childUID string, opts UpdateChildOpts) (*ent.ChildProfile, error) {
	c, err := s.Child(ctx, childUID)
	if err != nil {
		return nil, err
	}
	upd := c.Update()
	if opts.Name != nil {
		if *opts.Name == "" {
			return nil, ErrBadName
		}
		upd.SetName(*opts.Name)
	}
	if opts.Grade != nil {
		if *opts.Grade < 2 || *opts.Grade > 5 {
			return nil, ErrBadGrade
		}
		upd.SetGrade(*opts.Grade)
	}
	if opts.PIN != nil {
		pinHash, err := hashPIN(*opts.PIN)
		if err != nil {
			return nil, err
		}
		upd.SetPinHash(pinHash)
	}
	if opts.Archived != nil {
		upd.SetArchived(*opts.Archived)
		if *opts.Archived {
			// Archiving a child cuts off all of their devices.
			if err := s.revokeChildDevices(ctx, childUID); err != nil {
				return nil, err
			}
		}
	}
	return upd.Save(ctx)
}

// ---- Invites (join codes) ----

// CreateInvite mints a new join code for a space.
func (s *Service) CreateInvite(ctx context.Context, spaceUID string, ttl time.Duration) (*ent.Invite, error) {
	if _, err := s.Space(ctx, spaceUID); err != nil {
		return nil, err
	}
	if ttl <= 0 {
		ttl = DefaultInviteTTL
	}
	if ttl > MaxInviteTTL {
		ttl = MaxInviteTTL
	}
	// Retry on the (unlikely) collision of the human-friendly code.
	for attempt := 0; attempt < 5; attempt++ {
		code, err := generateJoinCode()
		if err != nil {
			return nil, err
		}
		inv, err := s.client.Invite.Create().
			SetUID(uuid.NewString()).
			SetFamilySpaceID(spaceUID).
			SetCode(code).
			SetExpiresAt(time.Now().Add(ttl)).
			Save(ctx)
		if err == nil {
			return inv, nil
		}
		if !ent.IsConstraintError(err) {
			return nil, fmt.Errorf("create invite: %w", err)
		}
	}
	return nil, fmt.Errorf("could not generate a unique join code")
}

// ActiveInvites lists live (unexpired, unrevoked) codes for a space.
func (s *Service) ActiveInvites(ctx context.Context, spaceUID string) ([]*ent.Invite, error) {
	return s.client.Invite.Query().
		Where(
			invite.FamilySpaceID(spaceUID),
			invite.Revoked(false),
			invite.ExpiresAtGT(time.Now()),
		).
		Order(ent.Desc(invite.FieldCreatedAt)).
		All(ctx)
}

// Invite returns an invite by UID.
func (s *Service) Invite(ctx context.Context, inviteUID string) (*ent.Invite, error) {
	inv, err := s.client.Invite.Query().
		Where(invite.UID(inviteUID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return inv, err
}

// RevokeInvite kills a join code.
func (s *Service) RevokeInvite(ctx context.Context, inviteUID string) error {
	inv, err := s.Invite(ctx, inviteUID)
	if err != nil {
		return err
	}
	return inv.Update().SetRevoked(true).Exec(ctx)
}

// liveInviteByCode resolves a code to a live invite.
func (s *Service) liveInviteByCode(ctx context.Context, code string) (*ent.Invite, error) {
	inv, err := s.client.Invite.Query().
		Where(invite.Code(NormalizeJoinCode(code))).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrInviteInvalid
	}
	if err != nil {
		return nil, err
	}
	if inv.Revoked || time.Now().After(inv.ExpiresAt) {
		return nil, ErrInviteInvalid
	}
	return inv, nil
}

// JoinPreview describes what a child sees after entering a join code.
type JoinPreview struct {
	SpaceUID  string
	SpaceName string
	Children  []*ent.ChildProfile
}

// PreviewJoin resolves a join code to the family space and its profiles.
func (s *Service) PreviewJoin(ctx context.Context, code string) (*JoinPreview, error) {
	inv, err := s.liveInviteByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	sp, err := s.Space(ctx, inv.FamilySpaceID)
	if err != nil {
		return nil, err
	}
	children, err := s.Children(ctx, sp.UID)
	if err != nil {
		return nil, err
	}
	return &JoinPreview{SpaceUID: sp.UID, SpaceName: sp.Name, Children: children}, nil
}

// ---- Device tokens ----

// RedeemInvite exchanges a live join code + profile selection (+ PIN when the
// profile has one) for a new device token. Returns the plaintext token; only
// its hash is stored.
func (s *Service) RedeemInvite(ctx context.Context, code, childUID, pin, deviceLabel string) (string, *ent.DeviceToken, error) {
	inv, err := s.liveInviteByCode(ctx, code)
	if err != nil {
		return "", nil, err
	}
	child, err := s.Child(ctx, childUID)
	if err != nil {
		return "", nil, err
	}
	if child.FamilySpaceID != inv.FamilySpaceID {
		// The profile is not part of the invite's family: treat as invalid
		// rather than leaking that the profile exists elsewhere.
		return "", nil, ErrInviteInvalid
	}
	if child.Archived {
		return "", nil, ErrArchived
	}
	if child.PinHash != "" {
		if pin == "" {
			return "", nil, ErrPINRequired
		}
		if bcrypt.CompareHashAndPassword([]byte(child.PinHash), []byte(pin)) != nil {
			return "", nil, ErrPINMismatch
		}
	}

	plaintext, hash, err := newDeviceToken()
	if err != nil {
		return "", nil, err
	}
	dt, err := s.client.DeviceToken.Create().
		SetUID(uuid.NewString()).
		SetChildProfileID(child.UID).
		SetFamilySpaceID(child.FamilySpaceID).
		SetTokenHash(hash).
		SetDeviceLabel(deviceLabel).
		Save(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("create device token: %w", err)
	}
	return plaintext, dt, nil
}

// ResolveDeviceToken authenticates a plaintext device token and returns the
// token record plus the (non-archived) child profile it belongs to.
// last_used_at is updated best-effort.
func (s *Service) ResolveDeviceToken(ctx context.Context, plaintext string) (*ent.DeviceToken, *ent.ChildProfile, error) {
	hash, ok := hashDeviceToken(plaintext)
	if !ok {
		return nil, nil, ErrTokenInvalid
	}
	dt, err := s.client.DeviceToken.Query().
		Where(devicetoken.TokenHash(hash)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil, ErrTokenInvalid
	}
	if err != nil {
		return nil, nil, err
	}
	if dt.Revoked {
		return nil, nil, ErrTokenInvalid
	}
	child, err := s.Child(ctx, dt.ChildProfileID)
	if err != nil {
		return nil, nil, ErrTokenInvalid
	}
	if child.Archived {
		return nil, nil, ErrTokenInvalid
	}
	_ = dt.Update().SetLastUsedAt(time.Now()).Exec(ctx)
	return dt, child, nil
}

// Devices lists all non-revoked device tokens for a child.
func (s *Service) Devices(ctx context.Context, childUID string) ([]*ent.DeviceToken, error) {
	return s.client.DeviceToken.Query().
		Where(devicetoken.ChildProfileID(childUID), devicetoken.Revoked(false)).
		Order(ent.Desc(devicetoken.FieldCreatedAt)).
		All(ctx)
}

// Device returns a device token record by UID.
func (s *Service) Device(ctx context.Context, deviceUID string) (*ent.DeviceToken, error) {
	dt, err := s.client.DeviceToken.Query().
		Where(devicetoken.UID(deviceUID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return dt, err
}

// RevokeDevice invalidates a device token.
func (s *Service) RevokeDevice(ctx context.Context, deviceUID string) error {
	dt, err := s.Device(ctx, deviceUID)
	if err != nil {
		return err
	}
	return dt.Update().SetRevoked(true).Exec(ctx)
}

func (s *Service) revokeChildDevices(ctx context.Context, childUID string) error {
	_, err := s.client.DeviceToken.Update().
		Where(devicetoken.ChildProfileID(childUID), devicetoken.Revoked(false)).
		SetRevoked(true).
		Save(ctx)
	return err
}

// ---- Helpers ----

func hashPIN(pin string) (string, error) {
	if pin == "" {
		return "", nil
	}
	if !pinPattern.MatchString(pin) {
		return "", ErrBadPIN
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash pin: %w", err)
	}
	return string(h), nil
}

// newDeviceToken returns (plaintext, hexSHA256(plaintext)).
func newDeviceToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	plaintext := TokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plaintext))
	return plaintext, hex.EncodeToString(sum[:]), nil
}

// hashDeviceToken hashes a presented token, rejecting malformed input early.
// The constant-time prefix check avoids leaking timing on the cheap path.
func hashDeviceToken(plaintext string) (string, bool) {
	if len(plaintext) < len(TokenPrefix)+16 {
		return "", false
	}
	if subtle.ConstantTimeCompare([]byte(plaintext[:len(TokenPrefix)]), []byte(TokenPrefix)) != 1 {
		return "", false
	}
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:]), true
}

// joinCodeWords are kid-friendly, unambiguous words for join codes.
var joinCodeWords = []string{
	"TIGER", "PANDA", "EAGLE", "ZEBRA", "KOALA", "OTTER", "ROBIN", "SHARK",
	"WHALE", "CAMEL", "GECKO", "MOOSE", "BISON", "HERON", "LEMUR", "DINGO",
	"COBRA", "FINCH", "HIPPO", "LLAMA", "MANGO", "PLUTO", "COMET", "NOVA",
}

// generateJoinCode returns codes like "TIGER-4207".
func generateJoinCode() (string, error) {
	wi, err := rand.Int(rand.Reader, big.NewInt(int64(len(joinCodeWords))))
	if err != nil {
		return "", err
	}
	n, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%04d", joinCodeWords[wi.Int64()], n.Int64()), nil
}

// NormalizeJoinCode uppercases and trims a user-typed join code.
func NormalizeJoinCode(code string) string {
	out := make([]rune, 0, len(code))
	for _, r := range code {
		switch {
		case r >= 'a' && r <= 'z':
			out = append(out, r-'a'+'A')
		case r == ' ' || r == '\t':
			// drop stray whitespace
		default:
			out = append(out, r)
		}
	}
	return string(out)
}
