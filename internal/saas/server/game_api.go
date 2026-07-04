package server

import (
	"errors"
	"net/http"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/internal/saas/authz"
	"github.com/abhisek/mathiz/internal/saas/game"
)

// Game API — the treasure-map play experience. All endpoints are child
// (device token) authenticated; expedition ownership is enforced by the
// game manager on every call.

func (s *Server) handleGameMap(w http.ResponseWriter, r *http.Request, p authz.Principal, child *ent.ChildProfile) {
	view, err := s.game.Map(r.Context(), child.UID)
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleExpeditionStart(w http.ResponseWriter, r *http.Request, p authz.Principal, child *ent.ChildProfile) {
	var req struct {
		SkillID string `json:"skillId"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	view, err := s.game.Start(r.Context(), child.UID, req.SkillID)
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (s *Server) handleExpeditionQuestion(w http.ResponseWriter, r *http.Request, p authz.Principal, child *ent.ChildProfile) {
	view, err := s.game.Question(r.Context(), child.UID, r.PathValue("id"))
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleExpeditionAnswer(w http.ResponseWriter, r *http.Request, p authz.Principal, child *ent.ChildProfile) {
	var req struct {
		Answer string `json:"answer"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	view, err := s.game.Answer(r.Context(), child.UID, r.PathValue("id"), req.Answer)
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleExpeditionHint(w http.ResponseWriter, r *http.Request, p authz.Principal, child *ent.ChildProfile) {
	view, err := s.game.Hint(r.Context(), child.UID, r.PathValue("id"))
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleExpeditionEnd(w http.ResponseWriter, r *http.Request, p authz.Principal, child *ent.ChildProfile) {
	view, err := s.game.End(r.Context(), child.UID, r.PathValue("id"))
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// writeGameError maps game errors onto kid-safe HTTP responses.
func writeGameError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, game.ErrNoExpedition):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, game.ErrLocked),
		errors.Is(err, game.ErrNoQuestion),
		errors.Is(err, game.ErrExpeditionOver),
		errors.Is(err, game.ErrNoHint):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, game.ErrGeneration):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeServiceError(w, err)
	}
}
