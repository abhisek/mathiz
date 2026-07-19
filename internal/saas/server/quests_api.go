package server

import (
	"errors"
	"net/http"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/internal/saas/authz"
	"github.com/abhisek/mathiz/internal/saas/credits"
	"github.com/abhisek/mathiz/internal/saas/quests"
)

// Quests API (specs/15-quests.md §4). Authoring is parent-only; the single
// kid-facing endpoint (starting a quest expedition) lives at the bottom and
// is as money-blind as the rest of the child surface.

// ---- Wire types ----

type questJSON struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Emoji         string `json:"emoji,omitempty"`
	SkillID       string `json:"skillId"`
	ChildID       string `json:"childId"` // "" = all children
	Status        string `json:"status"`
	QuestionCount int    `json:"questionCount"`
	CreatedAt     string `json:"createdAt"`
}

type questQuestionJSON struct {
	ID          string   `json:"id"`
	Position    int      `json:"position"`
	Text        string   `json:"text"`
	Answer      string   `json:"answer"`
	AnswerType  string   `json:"answerType"`
	Format      string   `json:"format"`
	Choices     []string `json:"choices,omitempty"`
	Hint        string   `json:"hint,omitempty"`
	Explanation string   `json:"explanation,omitempty"`
	Generated   bool     `json:"generated"` // saved by AI generation, review me
}

type questionInputJSON struct {
	Text        string   `json:"text"`
	Answer      string   `json:"answer"`
	AnswerType  string   `json:"answerType"`
	Format      string   `json:"format"`
	Choices     []string `json:"choices"`
	Hint        string   `json:"hint"`
	Explanation string   `json:"explanation"`
}

func (in questionInputJSON) toInput() quests.QuestionInput {
	return quests.QuestionInput{
		Text:        in.Text,
		Answer:      in.Answer,
		AnswerType:  in.AnswerType,
		Format:      in.Format,
		Choices:     in.Choices,
		Hint:        in.Hint,
		Explanation: in.Explanation,
	}
}

func toQuestJSON(q *ent.Quest, questionCount int) questJSON {
	return questJSON{
		ID:            q.UID,
		Name:          q.Name,
		Emoji:         q.Emoji,
		SkillID:       q.SkillID,
		ChildID:       q.ChildUID,
		Status:        q.Status,
		QuestionCount: questionCount,
		CreatedAt:     rfc3339(q.CreatedAt),
	}
}

func toQuestQuestionJSON(qq *ent.QuestQuestion) questQuestionJSON {
	return questQuestionJSON{
		ID:          qq.UID,
		Position:    qq.Position,
		Text:        qq.Text,
		Answer:      qq.Answer,
		AnswerType:  qq.AnswerType,
		Format:      qq.Format,
		Choices:     qq.Choices,
		Hint:        qq.Hint,
		Explanation: qq.Explanation,
		Generated:   qq.ClientKey != "",
	}
}

// ---- Parent: quest CRUD ----

func (s *Server) handleCreateQuest(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	spaceID := r.PathValue("id")
	if err := s.checker.CanManageSpace(r.Context(), p, spaceID); err != nil {
		writeServiceError(w, err)
		return
	}
	var req struct {
		Name    string `json:"name"`
		Emoji   string `json:"emoji"`
		SkillID string `json:"skillId"`
		ChildID string `json:"childId"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	q, err := s.quests.Create(r.Context(), spaceID, acct.UID, quests.QuestInput{
		Name: req.Name, Emoji: req.Emoji, SkillID: req.SkillID, ChildUID: req.ChildID,
	})
	if err != nil {
		writeQuestError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toQuestJSON(q, 0))
}

func (s *Server) handleListQuests(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	spaceID := r.PathValue("id")
	if err := s.checker.CanManageSpace(r.Context(), p, spaceID); err != nil {
		writeServiceError(w, err)
		return
	}
	list, err := s.quests.BySpace(r.Context(), spaceID)
	if err != nil {
		writeQuestError(w, err)
		return
	}
	out := make([]questJSON, len(list))
	for i, q := range list {
		n, err := s.quests.CountQuestions(r.Context(), q.UID)
		if err != nil {
			writeQuestError(w, err)
			return
		}
		out[i] = toQuestJSON(q, n)
	}
	writeJSON(w, http.StatusOK, map[string]any{"quests": out})
}

func (s *Server) handleGetQuest(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	questID := r.PathValue("id")
	if err := s.checker.CanManageQuest(r.Context(), p, questID); err != nil {
		writeServiceError(w, err)
		return
	}
	q, err := s.quests.Quest(r.Context(), questID)
	if err != nil {
		writeQuestError(w, err)
		return
	}
	questions, err := s.quests.Questions(r.Context(), questID)
	if err != nil {
		writeQuestError(w, err)
		return
	}
	out := make([]questQuestionJSON, len(questions))
	for i, qq := range questions {
		out[i] = toQuestQuestionJSON(qq)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"quest":     toQuestJSON(q, len(questions)),
		"questions": out,
	})
}

func (s *Server) handleUpdateQuest(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	questID := r.PathValue("id")
	if err := s.checker.CanManageQuest(r.Context(), p, questID); err != nil {
		writeServiceError(w, err)
		return
	}
	var req struct {
		Name    *string `json:"name"`
		Emoji   *string `json:"emoji"`
		SkillID *string `json:"skillId"`
		ChildID *string `json:"childId"`
		Status  *string `json:"status"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	q, err := s.quests.Update(r.Context(), questID, quests.UpdateOpts{
		Name: req.Name, Emoji: req.Emoji, SkillID: req.SkillID, ChildUID: req.ChildID, Status: req.Status,
	})
	if err != nil {
		writeQuestError(w, err)
		return
	}
	n, err := s.quests.CountQuestions(r.Context(), q.UID)
	if err != nil {
		writeQuestError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toQuestJSON(q, n))
}

func (s *Server) handleDeleteQuest(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	questID := r.PathValue("id")
	if err := s.checker.CanManageQuest(r.Context(), p, questID); err != nil {
		writeServiceError(w, err)
		return
	}
	if err := s.quests.Delete(r.Context(), questID); err != nil {
		writeQuestError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePublishQuest(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	questID := r.PathValue("id")
	if err := s.checker.CanManageQuest(r.Context(), p, questID); err != nil {
		writeServiceError(w, err)
		return
	}
	q, err := s.quests.Publish(r.Context(), questID)
	if err != nil {
		writeQuestError(w, err)
		return
	}
	n, err := s.quests.CountQuestions(r.Context(), q.UID)
	if err != nil {
		writeQuestError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toQuestJSON(q, n))
}

// ---- Parent: question authoring ----

func (s *Server) handleAddQuestQuestion(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	questID := r.PathValue("id")
	if err := s.checker.CanManageQuest(r.Context(), p, questID); err != nil {
		writeServiceError(w, err)
		return
	}
	var req questionInputJSON
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := s.quests.AddQuestion(r.Context(), questID, req.toInput())
	if err != nil {
		writeQuestError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"question": toQuestQuestionJSON(res.Question),
		"warning":  res.Warning,
	})
}

func (s *Server) handleUpdateQuestQuestion(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	questID := r.PathValue("id")
	if err := s.checker.CanManageQuest(r.Context(), p, questID); err != nil {
		writeServiceError(w, err)
		return
	}
	var req questionInputJSON
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := s.quests.UpdateQuestion(r.Context(), questID, r.PathValue("qid"), req.toInput())
	if err != nil {
		writeQuestError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"question": toQuestQuestionJSON(res.Question),
		"warning":  res.Warning,
	})
}

func (s *Server) handleDeleteQuestQuestion(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	questID := r.PathValue("id")
	if err := s.checker.CanManageQuest(r.Context(), p, questID); err != nil {
		writeServiceError(w, err)
		return
	}
	if err := s.quests.DeleteQuestion(r.Context(), questID, r.PathValue("qid")); err != nil {
		writeQuestError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Parent: AI generation ----

func (s *Server) handleGenerateQuestQuestions(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	questID := r.PathValue("id")
	if err := s.checker.CanManageQuest(r.Context(), p, questID); err != nil {
		writeServiceError(w, err)
		return
	}
	var req struct {
		Brief     string `json:"brief"`
		Count     int    `json:"count"`
		ClientKey string `json:"clientKey"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := s.quests.Generate(r.Context(), questID, req.Brief, req.Count, req.ClientKey)
	if err != nil {
		writeQuestError(w, err)
		return
	}
	out := make([]questQuestionJSON, len(res.Questions))
	for i, qq := range res.Questions {
		out[i] = toQuestQuestionJSON(qq)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"questions": out,
		"replayed":  res.Replayed,
	})
}

// ---- Child: start a quest expedition ----

func (s *Server) handleQuestExpeditionStart(w http.ResponseWriter, r *http.Request, p authz.Principal, child *ent.ChildProfile) {
	questID := r.PathValue("id")
	if err := s.checker.CanPlayQuest(r.Context(), p, questID); err != nil {
		// Cross-tenant, inactive, or mis-targeted: 404, don't confirm.
		writeServiceError(w, err)
		return
	}
	view, err := s.game.StartQuest(r.Context(), child.UID, questID)
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

// writeQuestError maps quests service errors onto HTTP statuses.
func writeQuestError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, quests.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, quests.ErrBadName),
		errors.Is(err, quests.ErrBadSkill),
		errors.Is(err, quests.ErrBadChild),
		errors.Is(err, quests.ErrBadStatus),
		errors.Is(err, quests.ErrBadQuestion),
		errors.Is(err, quests.ErrBadBrief),
		errors.Is(err, quests.ErrBadCount),
		errors.Is(err, quests.ErrBadKey):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, quests.ErrNoQuestions):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, credits.ErrInsufficient):
		writeError(w, http.StatusPaymentRequired, "out_of_credits")
	case errors.Is(err, quests.ErrNoProvider),
		errors.Is(err, quests.ErrGeneration):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeServiceError(w, err)
	}
}
