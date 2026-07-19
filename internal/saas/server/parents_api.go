package server

import (
	"net/http"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/internal/saas/authz"
	"github.com/abhisek/mathiz/internal/saas/family"
)

// Co-parent management API (specs/12-saas.md, "Co-parents"). Reading the
// parent roster is open to any member; inviting, revoking, and removing are
// owner-only (authz.CanManageParents). Accepting an invite needs no space
// permission — the service matches the accepting account's email.

type parentMemberJSON struct {
	AccountID   string `json:"accountId"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	CreatedAt   string `json:"createdAt"`
}

type parentInviteJSON struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

func toParentMemberJSON(m family.Member) parentMemberJSON {
	return parentMemberJSON{
		AccountID:   m.AccountID,
		Email:       m.Email,
		DisplayName: m.DisplayName,
		Role:        m.Role,
		CreatedAt:   rfc3339(m.CreatedAt),
	}
}

func toParentInviteJSON(inv *ent.ParentInvite) parentInviteJSON {
	return parentInviteJSON{
		ID:        inv.UID,
		Email:     inv.Email,
		Status:    inv.Status,
		CreatedAt: rfc3339(inv.CreatedAt),
	}
}

// handleListParents returns the space's members plus pending invites.
// Any member may look.
func (s *Server) handleListParents(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	spaceID := r.PathValue("id")
	if err := s.checker.CanManageSpace(r.Context(), p, spaceID); err != nil {
		writeServiceError(w, err)
		return
	}
	members, err := s.family.Members(r.Context(), spaceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	pending, err := s.family.PendingInvites(r.Context(), spaceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	outMembers := make([]parentMemberJSON, len(members))
	for i, m := range members {
		outMembers[i] = toParentMemberJSON(m)
	}
	outInvites := make([]parentInviteJSON, len(pending))
	for i, inv := range pending {
		outInvites[i] = toParentInviteJSON(inv)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"parents": outMembers,
		"invites": outInvites,
	})
}

// handleInviteParent records a co-parent invitation by email. Owner-only.
func (s *Server) handleInviteParent(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	spaceID := r.PathValue("id")
	if err := s.checker.CanManageParents(r.Context(), p, spaceID); err != nil {
		writeServiceError(w, err)
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	inv, err := s.family.InviteParent(r.Context(), spaceID, req.Email, acct.UID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toParentInviteJSON(inv))
}

// handleRemoveParent deletes a co-parent's membership. Owner-only; the owner
// itself can never be removed.
func (s *Server) handleRemoveParent(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	spaceID := r.PathValue("id")
	if err := s.checker.CanManageParents(r.Context(), p, spaceID); err != nil {
		writeServiceError(w, err)
		return
	}
	if err := s.family.RemoveParent(r.Context(), spaceID, r.PathValue("accountId")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRevokeParentInvite withdraws a pending co-parent invite. Owner of
// the invite's space only.
func (s *Server) handleRevokeParentInvite(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	inviteID := r.PathValue("id")
	if err := s.checker.CanManageParentInvite(r.Context(), p, inviteID); err != nil {
		writeServiceError(w, err)
		return
	}
	if err := s.family.RevokeParentInvite(r.Context(), inviteID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAcceptParentInvite joins the calling account to the inviting family
// as a co-parent. The service enforces the email match and one-family-per-
// account; mismatches surface as 404.
func (s *Server) handleAcceptParentInvite(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	inviteID := r.PathValue("id")
	m, err := s.family.AcceptInvite(r.Context(), inviteID, acct.UID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	sp, err := s.family.Space(r.Context(), m.FamilySpaceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"family": toSpaceJSON(sp),
		"role":   m.Role,
	})
}
