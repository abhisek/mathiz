// Package activity is a read model over the child's owner-scoped event
// streams: a merged, newest-first timeline of expeditions, notable mastery
// transitions, and micro-lessons for the parent dashboard, plus a per-session
// answer drill-down. It never writes events and never touches another
// tenant's stream — every read goes through Store.EventRepoFor(childUID).
package activity

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/store"
)

// Timeline item kinds.
const (
	KindExpedition = "expedition"
	KindMastery    = "mastery"
	KindLesson     = "lesson"
)

// Page size defaults.
const (
	DefaultLimit = 20
	MaxLimit     = 50
)

// questSkillPrefix marks the synthetic skill ID an untagged quest session
// plans with (see game.StartQuest). Tagged quest sessions carry the real
// skill ID and are indistinguishable from normal digs — documented limitation.
const questSkillPrefix = "quest:"

// ErrBadKind is returned for a kind filter outside
// expedition/mastery/lesson. The API maps it to 400.
var ErrBadKind = errors.New("unknown activity kind")

// ErrSessionNotFound is returned by SessionDetail when the session has no
// answers in this child's stream (unknown or foreign session). The API maps
// it to 404.
var ErrSessionNotFound = errors.New("session not found")

// Quests resolves quest display attribution. Implemented by
// internal/saas/quests.Service (QuestMeta). Nil-able: without it, quest
// sessions still appear, just without name/emoji/author.
type Quests interface {
	QuestMeta(ctx context.Context, questUID string) (name, emoji, createdByAccountID string, err error)
}

// MemberNameFunc resolves an account UID to a display name for quest
// createdBy attribution. Nil-able; errors degrade to "".
type MemberNameFunc func(ctx context.Context, accountID string) (string, error)

// Reader serves the activity timeline from the store.
type Reader struct {
	st         *store.Store
	quests     Quests
	memberName MemberNameFunc
}

// NewReader builds a Reader. quests and memberName may be nil — attribution
// then degrades to bare IDs / empty names, never to an error.
func NewReader(st *store.Store, quests Quests, memberName MemberNameFunc) *Reader {
	return &Reader{st: st, quests: quests, memberName: memberName}
}

// TimelineQuery selects a timeline page.
type TimelineQuery struct {
	Before int64     // exclusive global-sequence cursor (0 = newest)
	Limit  int       // page size (default DefaultLimit, capped at MaxLimit)
	Kinds  []string  // subset of expedition/mastery/lesson; empty = all
	From   time.Time // optional inclusive lower time bound
	To     time.Time // optional inclusive upper time bound
}

// SkillRef names a skill on a timeline item.
type SkillRef struct {
	ID   string
	Name string
}

// QuestRef attributes an expedition to a parent quest.
type QuestRef struct {
	ID        string
	Name      string
	Emoji     string
	CreatedBy string // authoring parent's display name ("" when unknown)
}

// ExpeditionItem is one finished session (session "end" event).
type ExpeditionItem struct {
	SessionID    string
	Questions    int
	Correct      int
	DurationSecs int
	Gems         int
	// Category is the first plan slot's category ("frontier" | "review" |
	// "booster") — why this expedition happened. "" when the plan is empty.
	Category string
	Skills   []SkillRef
	Quest    *QuestRef // nil for non-quest sessions
}

// MasteryItem is a transition worth a parent's attention.
type MasteryItem struct {
	SkillID   string
	SkillName string
	FromState string
	ToState   string
}

// LessonItem is a micro-lesson shown to the child.
type LessonItem struct {
	SkillID   string
	SkillName string
	Title     string
}

// TimelineItem is one merged timeline entry; exactly one of the kind
// payloads is set, matching Kind.
type TimelineItem struct {
	Kind       string
	Seq        int64
	At         time.Time
	Expedition *ExpeditionItem
	Mastery    *MasteryItem
	Lesson     *LessonItem
}

// TimelinePage is one page of merged items, newest first.
type TimelinePage struct {
	Items []TimelineItem
	// NextBefore is the cursor for the next page (lowest sequence in this
	// page); 0 when the page was short — no more data.
	NextBefore int64
}

// Timeline returns one page of the child's activity, merged newest-first by
// global sequence across the selected kinds.
func (r *Reader) Timeline(ctx context.Context, childUID string, q TimelineQuery) (TimelinePage, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	want := map[string]bool{}
	if len(q.Kinds) == 0 {
		want[KindExpedition], want[KindMastery], want[KindLesson] = true, true, true
	}
	for _, k := range q.Kinds {
		switch k {
		case KindExpedition, KindMastery, KindLesson:
			want[k] = true
		default:
			return TimelinePage{}, fmt.Errorf("%w: %q", ErrBadKind, k)
		}
	}

	// The one owner-scoped view this reader is allowed to touch.
	repo := r.st.EventRepoFor(childUID)
	opts := store.QueryOpts{Limit: limit, Before: q.Before, From: q.From, To: q.To}

	var items []TimelineItem

	if want[KindExpedition] {
		sums, err := repo.QuerySessionSummaries(ctx, opts)
		if err != nil {
			return TimelinePage{}, fmt.Errorf("query sessions: %w", err)
		}
		questCache := map[string]*QuestRef{}
		for _, sum := range sums {
			items = append(items, r.expeditionItem(ctx, sum, questCache))
		}
	}

	if want[KindMastery] {
		notable, err := r.notableMastery(ctx, repo, opts, limit)
		if err != nil {
			return TimelinePage{}, err
		}
		for _, ev := range notable {
			items = append(items, TimelineItem{
				Kind: KindMastery, Seq: ev.Sequence, At: ev.Timestamp,
				Mastery: &MasteryItem{
					SkillID:   ev.SkillID,
					SkillName: skillName(ev.SkillID),
					FromState: ev.FromState,
					ToState:   ev.ToState,
				},
			})
		}
	}

	if want[KindLesson] {
		lessons, err := repo.QueryLessonEvents(ctx, opts)
		if err != nil {
			return TimelinePage{}, fmt.Errorf("query lessons: %w", err)
		}
		for _, le := range lessons {
			items = append(items, TimelineItem{
				Kind: KindLesson, Seq: le.Sequence, At: le.Timestamp,
				Lesson: &LessonItem{
					SkillID:   le.SkillID,
					SkillName: skillName(le.SkillID),
					Title:     le.LessonTitle,
				},
			})
		}
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Seq > items[j].Seq })
	if len(items) > limit {
		items = items[:limit]
	}

	page := TimelinePage{Items: items}
	if len(items) == limit {
		page.NextBefore = items[len(items)-1].Seq
	}
	return page, nil
}

// expeditionItem hydrates one session summary: plan skills resolved through
// the skill graph and quest attribution from the synthetic "quest:" plan ID.
func (r *Reader) expeditionItem(ctx context.Context, sum store.SessionSummaryRecord, questCache map[string]*QuestRef) TimelineItem {
	exp := &ExpeditionItem{
		SessionID:    sum.SessionID,
		Questions:    sum.QuestionsServed,
		Correct:      sum.CorrectAnswers,
		DurationSecs: sum.DurationSecs,
		Gems:         sum.GemCount,
	}
	if len(sum.Plan) > 0 {
		exp.Category = sum.Plan[0].Category
	}
	seen := map[string]bool{}
	for _, slot := range sum.Plan {
		if questUID, ok := strings.CutPrefix(slot.SkillID, questSkillPrefix); ok {
			// Untagged quest session: the synthetic ID is attribution, not a
			// skill. (Tagged quests plan the real skill and are not
			// distinguishable here.)
			if exp.Quest == nil {
				exp.Quest = r.questRef(ctx, questUID, questCache)
			}
			continue
		}
		if seen[slot.SkillID] {
			continue
		}
		seen[slot.SkillID] = true
		exp.Skills = append(exp.Skills, SkillRef{ID: slot.SkillID, Name: skillName(slot.SkillID)})
	}
	return TimelineItem{Kind: KindExpedition, Seq: sum.Sequence, At: sum.Timestamp, Expedition: exp}
}

// questRef resolves quest attribution, degrading to a bare ID when the
// lookup is missing or fails — attribution must never fail the timeline.
func (r *Reader) questRef(ctx context.Context, questUID string, cache map[string]*QuestRef) *QuestRef {
	if ref, ok := cache[questUID]; ok {
		return ref
	}
	ref := &QuestRef{ID: questUID}
	cache[questUID] = ref
	if r.quests == nil {
		return ref
	}
	name, emoji, createdBy, err := r.quests.QuestMeta(ctx, questUID)
	if err != nil {
		return ref // quest deleted since — keep the bare ID
	}
	ref.Name, ref.Emoji = name, emoji
	if createdBy != "" && r.memberName != nil {
		if display, err := r.memberName(ctx, createdBy); err == nil {
			ref.CreatedBy = display
		}
	}
	return ref
}

// notableMastery fetches up to limit mastery transitions a parent cares
// about (to mastered/rusty), paging past uninteresting transitions so a run
// of learning-state noise can't hide older notable ones from the merge.
func (r *Reader) notableMastery(ctx context.Context, repo store.EventRepo, opts store.QueryOpts, limit int) ([]store.MasteryEventRecord, error) {
	var out []store.MasteryEventRecord
	before := opts.Before
	for {
		o := opts
		o.Before = before
		o.Limit = limit
		batch, err := repo.QueryMasteryEvents(ctx, o)
		if err != nil {
			return nil, fmt.Errorf("query mastery events: %w", err)
		}
		for _, ev := range batch {
			if ev.ToState != "mastered" && ev.ToState != "rusty" {
				continue
			}
			out = append(out, ev)
			if len(out) == limit {
				return out, nil
			}
		}
		if len(batch) < o.Limit {
			return out, nil // stream exhausted
		}
		before = batch[len(batch)-1].Sequence
	}
}

// AnswerDetail is one graded answer inside a session.
type AnswerDetail struct {
	Seq           int64
	At            time.Time
	SkillID       string
	SkillName     string
	QuestionText  string
	LearnerAnswer string
	CorrectAnswer string
	Correct       bool
	TimeMs        int
}

// SessionDetail is the expandable per-session drill-down.
type SessionDetail struct {
	Answers   []AnswerDetail
	HintCount int
}

// SessionDetail returns a session's answers in question order plus the hint
// count. A session with no answers in this child's stream — unknown ID or
// another child's session — returns ErrSessionNotFound.
func (r *Reader) SessionDetail(ctx context.Context, childUID, sessionID string) (SessionDetail, error) {
	repo := r.st.EventRepoFor(childUID)
	answers, err := repo.AnswersForSession(ctx, sessionID)
	if err != nil {
		return SessionDetail{}, fmt.Errorf("query answers: %w", err)
	}
	if len(answers) == 0 {
		return SessionDetail{}, ErrSessionNotFound
	}
	hints, err := repo.HintCountForSession(ctx, sessionID)
	if err != nil {
		return SessionDetail{}, fmt.Errorf("count hints: %w", err)
	}
	detail := SessionDetail{HintCount: hints, Answers: make([]AnswerDetail, len(answers))}
	for i, a := range answers {
		detail.Answers[i] = AnswerDetail{
			Seq:           a.Sequence,
			At:            a.Timestamp,
			SkillID:       a.SkillID,
			SkillName:     skillName(a.SkillID),
			QuestionText:  a.QuestionText,
			LearnerAnswer: a.LearnerAnswer,
			CorrectAnswer: a.CorrectAnswer,
			Correct:       a.Correct,
			TimeMs:        a.TimeMs,
		}
	}
	return detail, nil
}

// skillName resolves a skill ID through the skill graph; unknown IDs keep
// the ID as the name (removed skills, synthetic quest IDs on answers).
func skillName(id string) string {
	if sk, err := skillgraph.GetSkill(id); err == nil {
		return sk.Name
	}
	return id
}
