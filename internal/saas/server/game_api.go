package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/internal/saas/authz"
	"github.com/abhisek/mathiz/internal/saas/game"
	"github.com/abhisek/mathiz/internal/saas/logctx"
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

func (s *Server) handleGameNotebook(w http.ResponseWriter, r *http.Request, p authz.Principal, child *ent.ChildProfile) {
	view, err := s.game.Notebook(r.Context(), child.UID)
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
	// How long a kid waits for a question is the latency goal of specs/19;
	// the canonical line carries it so p50/p95 come from the logs.
	start := time.Now()
	view, err := s.game.Question(r.Context(), child.UID, r.PathValue("id"))
	logctx.Add(r.Context(), "q_wait_ms", time.Since(start).Milliseconds())
	if view != nil {
		logctx.Add(r.Context(), "q_source", string(view.Source))
		logctx.Add(r.Context(), "q_attempts", view.Attempts)
	}
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

func (s *Server) handleExpeditionLesson(w http.ResponseWriter, r *http.Request, p authz.Principal, child *ent.ChildProfile) {
	view, err := s.game.Lesson(r.Context(), child.UID, r.PathValue("id"))
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleExpeditionLessonAnswer(w http.ResponseWriter, r *http.Request, p authz.Principal, child *ent.ChildProfile) {
	var req struct {
		Answer string `json:"answer"`
		Skip   bool   `json:"skip"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	view, err := s.game.AnswerLesson(r.Context(), child.UID, r.PathValue("id"), req.Answer, req.Skip)
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

// gameErrorCodes are the stable codes the kid client branches on. The
// message text is kid copy and may change; a code never does.
var gameErrorCodes = []struct {
	err    error
	status int
	code   string
}{
	{game.ErrNoExpedition, http.StatusNotFound, "no_expedition"},
	{game.ErrLocked, http.StatusConflict, "locked"},
	{game.ErrNoQuestion, http.StatusConflict, "no_question"},
	{game.ErrExpeditionOver, http.StatusConflict, "expedition_over"},
	{game.ErrNoHint, http.StatusConflict, "no_hint"},
	{game.ErrNoLesson, http.StatusConflict, "no_lesson"},
	{game.ErrQuestDone, http.StatusConflict, "quest_done"},
	{game.ErrElsewhere, http.StatusConflict, "elsewhere"},
	{game.ErrGeneration, http.StatusServiceUnavailable, "generation_failed"},
}

// writeGameError maps game errors onto kid-safe HTTP responses.
func writeGameError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, game.ErrQuestUnavailable):
		// Cross-tenant/inactive quest probes: 404, don't confirm existence.
		writeError(w, http.StatusNotFound, "not found")
		return
	case errors.Is(err, game.ErrNoCredits):
		// The kid client renders this as "the ship needs to rest" — no
		// prices on the child surface, ever.
		writeErrorCode(w, http.StatusPaymentRequired, "out_of_credits", "out_of_credits")
		return
	}
	for _, m := range gameErrorCodes {
		if errors.Is(err, m.err) {
			writeErrorCode(w, m.status, m.code, err.Error())
			return
		}
	}
	writeServiceError(w, err)
}
