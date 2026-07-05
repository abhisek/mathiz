package server

import (
	"log"
	"net/http"
	"time"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/internal/saas/authz"
	"github.com/abhisek/mathiz/internal/saas/family"
)

// ---- Wire types ----

type accountJSON struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

type spaceJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

type childJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Grade     int    `json:"grade"`
	HasPIN    bool   `json:"hasPin"`
	Archived  bool   `json:"archived"`
	CreatedAt string `json:"createdAt"`
}

type inviteJSON struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	ExpiresAt string `json:"expiresAt"`
	CreatedAt string `json:"createdAt"`
}

type deviceJSON struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	CreatedAt  string  `json:"createdAt"`
	LastUsedAt *string `json:"lastUsedAt"`
}

// rfc3339 is the single timestamp format the API speaks.
func rfc3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func rfc3339Ptr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := rfc3339(*t)
	return &s
}

func toAccountJSON(a *ent.Account) accountJSON {
	return accountJSON{ID: a.UID, Email: a.Email, DisplayName: a.DisplayName}
}

func toSpaceJSON(sp *ent.FamilySpace) spaceJSON {
	return spaceJSON{ID: sp.UID, Name: sp.Name, CreatedAt: rfc3339(sp.CreatedAt)}
}

func toChildJSON(c *ent.ChildProfile) childJSON {
	return childJSON{
		ID: c.UID, Name: c.Name, Grade: c.Grade,
		HasPIN: c.PinHash != "", Archived: c.Archived,
		CreatedAt: rfc3339(c.CreatedAt),
	}
}

func toInviteJSON(inv *ent.Invite) inviteJSON {
	return inviteJSON{
		ID: inv.UID, Code: inv.Code,
		ExpiresAt: rfc3339(inv.ExpiresAt),
		CreatedAt: rfc3339(inv.CreatedAt),
	}
}

func toDeviceJSON(dt *ent.DeviceToken) deviceJSON {
	return deviceJSON{
		ID: dt.UID, Label: dt.DeviceLabel,
		CreatedAt:  rfc3339(dt.CreatedAt),
		LastUsedAt: rfc3339Ptr(dt.LastUsedAt),
	}
}

// ---- Public ----

// handleBootConfig hands the SPA what it needs to initialize supabase-js.
// The anon key is public by design (it ships in every Supabase frontend).
func (s *Server) handleBootConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"supabaseUrl":     s.cfg.SupabaseURL,
		"supabaseAnonKey": s.cfg.SupabaseAnonKey,
	})
}

func (s *Server) handleJoinPreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	prev, err := s.family.PreviewJoin(r.Context(), req.Code)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	children := make([]childJSON, len(prev.Children))
	for i, c := range prev.Children {
		children[i] = toChildJSON(c)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"familyName": prev.SpaceName,
		"children":   children,
	})
}

func (s *Server) handleJoinRedeem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code           string `json:"code"`
		ChildProfileID string `json:"childProfileId"`
		PIN            string `json:"pin"`
		DeviceLabel    string `json:"deviceLabel"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	plaintext, dt, err := s.family.RedeemInvite(r.Context(), req.Code, req.ChildProfileID, req.PIN, req.DeviceLabel)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	child, err := s.family.Child(r.Context(), dt.ChildProfileID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": plaintext,
		"child": toChildJSON(child),
	})
}

// ---- Parent ----

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	sp, err := s.family.SpaceByOwner(r.Context(), acct.UID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	resp := map[string]any{"account": toAccountJSON(acct), "family": nil}
	if sp != nil {
		resp["family"] = toSpaceJSON(sp)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCreateFamily(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	sp, err := s.family.CreateSpace(r.Context(), acct.UID, req.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	// Free starter credits: kids play immediately, no card required.
	if s.credits != nil {
		if err := s.credits.EnsureStarterGrant(r.Context(), sp.UID); err != nil {
			log.Printf("starter grant for %s: %v", sp.UID, err)
		}
	}
	writeJSON(w, http.StatusCreated, toSpaceJSON(sp))
}

func (s *Server) handleRenameFamily(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	spaceID := r.PathValue("id")
	if err := s.checker.CanManageSpace(r.Context(), p, spaceID); err != nil {
		writeServiceError(w, err)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	sp, err := s.family.RenameSpace(r.Context(), spaceID, req.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toSpaceJSON(sp))
}

func (s *Server) handleAddChild(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	spaceID := r.PathValue("id")
	if err := s.checker.CanManageSpace(r.Context(), p, spaceID); err != nil {
		writeServiceError(w, err)
		return
	}
	var req struct {
		Name  string `json:"name"`
		Grade int    `json:"grade"`
		PIN   string `json:"pin"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	child, err := s.family.AddChild(r.Context(), spaceID, req.Name, req.Grade, req.PIN)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toChildJSON(child))
}

func (s *Server) handleListChildren(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	spaceID := r.PathValue("id")
	if err := s.checker.CanManageSpace(r.Context(), p, spaceID); err != nil {
		writeServiceError(w, err)
		return
	}
	children, err := s.family.Children(r.Context(), spaceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]map[string]any, len(children))
	for i, c := range children {
		summary, err := s.childSummary(r.Context(), c.UID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		out[i] = map[string]any{
			"profile": toChildJSON(c),
			"summary": summary,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"children": out})
}

func (s *Server) handleUpdateChild(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	childID := r.PathValue("id")
	if err := s.checker.CanManageChild(r.Context(), p, childID); err != nil {
		writeServiceError(w, err)
		return
	}
	var req struct {
		Name     *string `json:"name"`
		Grade    *int    `json:"grade"`
		PIN      *string `json:"pin"`
		Archived *bool   `json:"archived"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	child, err := s.family.UpdateChild(r.Context(), childID, family.UpdateChildOpts{
		Name: req.Name, Grade: req.Grade, PIN: req.PIN, Archived: req.Archived,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toChildJSON(child))
}

func (s *Server) handleChildStats(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	childID := r.PathValue("id")
	if err := s.checker.CanManageChild(r.Context(), p, childID); err != nil {
		writeServiceError(w, err)
		return
	}
	stats, err := s.childStats(r.Context(), childID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	spaceID := r.PathValue("id")
	if err := s.checker.CanManageSpace(r.Context(), p, spaceID); err != nil {
		writeServiceError(w, err)
		return
	}
	var req struct {
		TTLHours int `json:"ttlHours"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	ttl := time.Duration(req.TTLHours) * time.Hour
	inv, err := s.family.CreateInvite(r.Context(), spaceID, ttl)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toInviteJSON(inv))
}

func (s *Server) handleListInvites(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	spaceID := r.PathValue("id")
	if err := s.checker.CanManageSpace(r.Context(), p, spaceID); err != nil {
		writeServiceError(w, err)
		return
	}
	invites, err := s.family.ActiveInvites(r.Context(), spaceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]inviteJSON, len(invites))
	for i, inv := range invites {
		out[i] = toInviteJSON(inv)
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": out})
}

func (s *Server) handleRevokeInvite(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	inviteID := r.PathValue("id")
	if err := s.checker.CanManageInvite(r.Context(), p, inviteID); err != nil {
		writeServiceError(w, err)
		return
	}
	if err := s.family.RevokeInvite(r.Context(), inviteID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	childID := r.PathValue("id")
	if err := s.checker.CanManageChild(r.Context(), p, childID); err != nil {
		writeServiceError(w, err)
		return
	}
	devices, err := s.family.Devices(r.Context(), childID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]deviceJSON, len(devices))
	for i, dt := range devices {
		out[i] = toDeviceJSON(dt)
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": out})
}

func (s *Server) handleRevokeDevice(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	deviceID := r.PathValue("id")
	if err := s.checker.CanManageDevice(r.Context(), p, deviceID); err != nil {
		writeServiceError(w, err)
		return
	}
	if err := s.family.RevokeDevice(r.Context(), deviceID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Child ----

func (s *Server) handleChildMe(w http.ResponseWriter, r *http.Request, p authz.Principal, child *ent.ChildProfile) {
	sp, err := s.family.Space(r.Context(), child.FamilySpaceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"profile":    toChildJSON(child),
		"familyName": sp.Name,
	})
}
