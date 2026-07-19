// Package authz centralizes every permission decision in the SaaS layer.
// Handlers build a Principal from verified credentials and ask this package
// before touching control-plane objects or learner data. Deny by default.
package authz

import (
	"context"
	"errors"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/internal/saas/family"
)

// ErrDenied is returned for every failed check. The API layer maps it to
// 403 — or 404 when revealing the object's existence would itself leak.
var ErrDenied = errors.New("permission denied")

// PrincipalKind discriminates who is calling.
type PrincipalKind string

const (
	KindParent PrincipalKind = "parent"
	KindChild  PrincipalKind = "child"
)

// Principal is a verified caller identity.
type Principal struct {
	Kind PrincipalKind

	// AccountID is set for parents (internal account UID).
	AccountID string

	// ChildProfileID and FamilySpaceID are set for children, resolved from
	// their device token.
	ChildProfileID string
	FamilySpaceID  string
}

// ChildPrincipal builds the principal for an authenticated child device.
// Every child auth surface (today the HTTP middleware) uses this so
// hardening added to the principal reaches all of them.
func ChildPrincipal(child *ent.ChildProfile) Principal {
	return Principal{
		Kind:           KindChild,
		ChildProfileID: child.UID,
		FamilySpaceID:  child.FamilySpaceID,
	}
}

// QuestDirectory resolves quests for permission checks (implemented by
// internal/saas/quests.Service; an interface here keeps authz free of the
// quests package's LLM/game dependencies).
type QuestDirectory interface {
	Quest(ctx context.Context, questUID string) (*ent.Quest, error)
}

// questStatusActive mirrors quests.StatusActive (see QuestDirectory: authz
// deliberately does not import the quests package).
const questStatusActive = "active"

// Checker answers authorization questions using control-plane data.
type Checker struct {
	family *family.Service
	quests QuestDirectory // nil until SetQuests; quest checks then deny
}

func NewChecker(svc *family.Service) *Checker {
	return &Checker{family: svc}
}

// SetQuests wires the quest directory. Without it every quest check denies
// (fail closed).
func (c *Checker) SetQuests(q QuestDirectory) {
	c.quests = q
}

// spaceRole resolves the parent principal's membership role in a space.
// Non-members (including cross-family parents) get ErrDenied — the API maps
// that to 404 so object existence never leaks.
func (c *Checker) spaceRole(ctx context.Context, p Principal, spaceUID string) (string, error) {
	if p.Kind != KindParent || p.AccountID == "" {
		return "", ErrDenied
	}
	role, err := c.family.MemberRole(ctx, spaceUID, p.AccountID)
	if err != nil {
		return "", ErrDenied
	}
	return role, nil
}

// CanManageSpace reports whether p may read and mutate a family space
// (children, invites, devices, stats, quests). Membership policy: any
// member — owner or co-parent.
func (c *Checker) CanManageSpace(ctx context.Context, p Principal, spaceUID string) error {
	_, err := c.spaceRole(ctx, p, spaceUID)
	return err
}

// CanManageBilling reports whether p may see and operate the space's wallet
// (balance, checkout, portal). Owner-only: the payment provider's customer
// is the payer's identity.
func (c *Checker) CanManageBilling(ctx context.Context, p Principal, spaceUID string) error {
	return c.requireOwner(ctx, p, spaceUID)
}

// CanManageParents reports whether p may invite and remove co-parents.
// Owner-only.
func (c *Checker) CanManageParents(ctx context.Context, p Principal, spaceUID string) error {
	return c.requireOwner(ctx, p, spaceUID)
}

// CanManageParentInvite reports whether p may revoke a co-parent invite
// (owner of the invite's space only).
func (c *Checker) CanManageParentInvite(ctx context.Context, p Principal, inviteUID string) error {
	if p.Kind != KindParent || p.AccountID == "" {
		return ErrDenied
	}
	inv, err := c.family.ParentInvite(ctx, inviteUID)
	if err != nil {
		return ErrDenied
	}
	return c.CanManageParents(ctx, p, inv.FamilySpaceID)
}

func (c *Checker) requireOwner(ctx context.Context, p Principal, spaceUID string) error {
	role, err := c.spaceRole(ctx, p, spaceUID)
	if err != nil {
		return err
	}
	if role != family.RoleOwner {
		return ErrDenied
	}
	return nil
}

// CanManageChild reports whether p may read and mutate a child profile.
// Parents manage children of the space they belong to.
func (c *Checker) CanManageChild(ctx context.Context, p Principal, childUID string) error {
	child, err := c.family.Child(ctx, childUID)
	if err != nil {
		return ErrDenied
	}
	return c.CanManageSpace(ctx, p, child.FamilySpaceID)
}

// CanManageInvite reports whether p may revoke an invite.
func (c *Checker) CanManageInvite(ctx context.Context, p Principal, inviteUID string) error {
	inv, err := c.family.Invite(ctx, inviteUID)
	if err != nil {
		return ErrDenied
	}
	return c.CanManageSpace(ctx, p, inv.FamilySpaceID)
}

// CanManageDevice reports whether p may revoke a device token.
func (c *Checker) CanManageDevice(ctx context.Context, p Principal, deviceUID string) error {
	dt, err := c.family.Device(ctx, deviceUID)
	if err != nil {
		return ErrDenied
	}
	return c.CanManageSpace(ctx, p, dt.FamilySpaceID)
}

// CanManageQuest reports whether p may read and mutate a quest (rename,
// retarget, author questions, generate, publish, delete). Parents manage
// quests of the space they belong to.
func (c *Checker) CanManageQuest(ctx context.Context, p Principal, questUID string) error {
	if c.quests == nil {
		return ErrDenied
	}
	q, err := c.quests.Quest(ctx, questUID)
	if err != nil {
		return ErrDenied
	}
	return c.CanManageSpace(ctx, p, q.FamilySpaceID)
}

// CanPlayQuest reports whether p (a child) may start an expedition on a
// quest: the quest must be active, live in the child's own family space,
// and target this child (or all children).
func (c *Checker) CanPlayQuest(ctx context.Context, p Principal, questUID string) error {
	if p.Kind != KindChild || p.ChildProfileID == "" {
		return ErrDenied
	}
	if c.quests == nil {
		return ErrDenied
	}
	q, err := c.quests.Quest(ctx, questUID)
	if err != nil {
		return ErrDenied
	}
	if q.FamilySpaceID != p.FamilySpaceID {
		return ErrDenied
	}
	if q.Status != questStatusActive {
		return ErrDenied
	}
	if q.ChildUID != "" && q.ChildUID != p.ChildProfileID {
		return ErrDenied
	}
	return nil
}

// CanLearnAs reports whether p may run a learning session (and read own
// profile data) as the given child. Children may act only as themselves;
// parents may not impersonate children (sessions belong to the learner).
func (c *Checker) CanLearnAs(ctx context.Context, p Principal, childUID string) error {
	if p.Kind != KindChild {
		return ErrDenied
	}
	if p.ChildProfileID == "" || p.ChildProfileID != childUID {
		return ErrDenied
	}
	return nil
}
