// Package authz centralizes every permission decision in the SaaS layer.
// Handlers build a Principal from verified credentials and ask this package
// before touching control-plane objects or learner data. Deny by default.
package authz

import (
	"context"
	"errors"

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

// Checker answers authorization questions using control-plane data.
type Checker struct {
	family *family.Service
}

func NewChecker(svc *family.Service) *Checker {
	return &Checker{family: svc}
}

// CanManageSpace reports whether p may read and mutate a family space
// (children, invites, devices, stats). V1 policy: only the owning parent.
func (c *Checker) CanManageSpace(ctx context.Context, p Principal, spaceUID string) error {
	if p.Kind != KindParent || p.AccountID == "" {
		return ErrDenied
	}
	sp, err := c.family.Space(ctx, spaceUID)
	if err != nil {
		return ErrDenied
	}
	if sp.OwnerAccountID != p.AccountID {
		return ErrDenied
	}
	return nil
}

// CanManageChild reports whether p may read and mutate a child profile.
// Parents manage children of spaces they own.
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
