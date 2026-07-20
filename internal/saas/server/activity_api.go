package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/internal/saas/activity"
	"github.com/abhisek/mathiz/internal/saas/authz"
)

// Activity timeline API (read model) — parent-only, same authz as stats:
// any family member may view, strangers get 404.

type skillRefJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type questRefJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Emoji     string `json:"emoji"`
	CreatedBy string `json:"createdBy"`
}

type expeditionItemJSON struct {
	SessionID    string         `json:"sessionId"`
	Questions    int            `json:"questions"`
	Correct      int            `json:"correct"`
	DurationSecs int            `json:"durationSecs"`
	Gems         int            `json:"gems"`
	Category     string         `json:"category,omitempty"` // "frontier" | "review" | "booster"
	Skills       []skillRefJSON `json:"skills"`
	Quest        *questRefJSON  `json:"quest,omitempty"`
}

type masteryItemJSON struct {
	SkillID   string `json:"skillId"`
	SkillName string `json:"skillName"`
	FromState string `json:"fromState"`
	ToState   string `json:"toState"`
}

type lessonItemJSON struct {
	SkillID   string `json:"skillId"`
	SkillName string `json:"skillName"`
	Title     string `json:"title"`
}

type activityItemJSON struct {
	Kind       string              `json:"kind"`
	Seq        int64               `json:"seq"`
	At         string              `json:"at"`
	Expedition *expeditionItemJSON `json:"expedition,omitempty"`
	Mastery    *masteryItemJSON    `json:"mastery,omitempty"`
	Lesson     *lessonItemJSON     `json:"lesson,omitempty"`
}

type answerDetailJSON struct {
	Seq           int64  `json:"seq"`
	At            string `json:"at"`
	SkillID       string `json:"skillId"`
	SkillName     string `json:"skillName"`
	QuestionText  string `json:"questionText"`
	LearnerAnswer string `json:"learnerAnswer"`
	CorrectAnswer string `json:"correctAnswer"`
	Correct       bool   `json:"correct"`
	TimeMs        int    `json:"timeMs"`
}

func toActivityItemJSON(it activity.TimelineItem) activityItemJSON {
	out := activityItemJSON{Kind: it.Kind, Seq: it.Seq, At: rfc3339(it.At)}
	switch {
	case it.Expedition != nil:
		exp := &expeditionItemJSON{
			SessionID:    it.Expedition.SessionID,
			Questions:    it.Expedition.Questions,
			Correct:      it.Expedition.Correct,
			DurationSecs: it.Expedition.DurationSecs,
			Gems:         it.Expedition.Gems,
			Category:     it.Expedition.Category,
			Skills:       make([]skillRefJSON, len(it.Expedition.Skills)),
		}
		for i, sk := range it.Expedition.Skills {
			exp.Skills[i] = skillRefJSON{ID: sk.ID, Name: sk.Name}
		}
		if q := it.Expedition.Quest; q != nil {
			exp.Quest = &questRefJSON{ID: q.ID, Name: q.Name, Emoji: q.Emoji, CreatedBy: q.CreatedBy}
		}
		out.Expedition = exp
	case it.Mastery != nil:
		out.Mastery = &masteryItemJSON{
			SkillID: it.Mastery.SkillID, SkillName: it.Mastery.SkillName,
			FromState: it.Mastery.FromState, ToState: it.Mastery.ToState,
		}
	case it.Lesson != nil:
		out.Lesson = &lessonItemJSON{
			SkillID: it.Lesson.SkillID, SkillName: it.Lesson.SkillName,
			Title: it.Lesson.Title,
		}
	}
	return out
}

// parseTimelineQuery reads before/limit/kinds/from/to. ok=false means an
// error response was already written.
func parseTimelineQuery(w http.ResponseWriter, r *http.Request) (activity.TimelineQuery, bool) {
	var q activity.TimelineQuery
	get := r.URL.Query()
	if v := get.Get("before"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid before cursor")
			return q, false
		}
		q.Before = n
	}
	if v := get.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return q, false
		}
		q.Limit = n
	}
	if v := get.Get("kinds"); v != "" {
		for _, k := range strings.Split(v, ",") {
			if k = strings.TrimSpace(k); k != "" {
				q.Kinds = append(q.Kinds, k)
			}
		}
	}
	if v := get.Get("quest"); v != "" {
		// A quest filter implies expeditions — only expeditions carry quest
		// attribution — so the kinds param is deliberately ignored/overridden
		// when quest is set.
		q.QuestUID = v
		q.Kinds = nil
	}
	for _, tp := range []struct {
		name string
		dst  *time.Time
	}{{"from", &q.From}, {"to", &q.To}} {
		if v := get.Get(tp.name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid "+tp.name+" timestamp (want RFC3339)")
				return q, false
			}
			*tp.dst = t
		}
	}
	return q, true
}

func (s *Server) handleChildActivity(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	childID := r.PathValue("id")
	if err := s.checker.CanManageChild(r.Context(), p, childID); err != nil {
		writeServiceError(w, err)
		return
	}
	q, ok := parseTimelineQuery(w, r)
	if !ok {
		return
	}
	page, err := s.activity.Timeline(r.Context(), childID, q)
	if err != nil {
		if errors.Is(err, activity.ErrBadKind) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeServiceError(w, err)
		return
	}
	items := make([]activityItemJSON, len(page.Items))
	for i, it := range page.Items {
		items[i] = toActivityItemJSON(it)
	}
	resp := map[string]any{"items": items}
	if page.NextBefore > 0 {
		resp["nextBefore"] = page.NextBefore
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleChildActivitySession(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	childID := r.PathValue("id")
	if err := s.checker.CanManageChild(r.Context(), p, childID); err != nil {
		writeServiceError(w, err)
		return
	}
	detail, err := s.activity.SessionDetail(r.Context(), childID, r.PathValue("sessionId"))
	if err != nil {
		if errors.Is(err, activity.ErrSessionNotFound) {
			// Unknown or foreign session: 404 without confirming existence.
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeServiceError(w, err)
		return
	}
	answers := make([]answerDetailJSON, len(detail.Answers))
	for i, a := range detail.Answers {
		answers[i] = answerDetailJSON{
			Seq: a.Seq, At: rfc3339(a.At),
			SkillID: a.SkillID, SkillName: a.SkillName,
			QuestionText:  a.QuestionText,
			LearnerAnswer: a.LearnerAnswer, CorrectAnswer: a.CorrectAnswer,
			Correct: a.Correct, TimeMs: a.TimeMs,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"answers":   answers,
		"hintCount": detail.HintCount,
	})
}
