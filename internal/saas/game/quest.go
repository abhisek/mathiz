package game

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/abhisek/mathiz/internal/gems"
	"github.com/abhisek/mathiz/internal/mastery"
	"github.com/abhisek/mathiz/internal/problemgen"
	sess "github.com/abhisek/mathiz/internal/session"
	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/spacedrep"
	"github.com/abhisek/mathiz/internal/store"
)

// Parent quests on the map (specs/15-quests.md): a quest expedition is the
// SAME expedition machinery as a dig spot — same charge, same play slot,
// same session engine — except questions come from the parent-authored list
// instead of the LLM.

var (
	// ErrQuestUnavailable means the quest doesn't exist, isn't active, or
	// isn't targeted at this child. One error for all cases: don't confirm
	// the existence of quests a child cannot see.
	ErrQuestUnavailable = errors.New("that quest isn't on the map")
	// ErrQuestDone means every question is already answered correctly.
	ErrQuestDone = errors.New("quest complete — nothing left to answer")
)

// QuestPlayQuestion is one authored question ready to serve.
type QuestPlayQuestion struct {
	UID         string
	Text        string
	Answer      string
	AnswerType  string
	Format      string
	Choices     []string
	Hint        string
	Explanation string
}

// QuestPlay is a playable quest for one child: its identity plus the
// not-yet-correctly-answered questions, in authored order.
type QuestPlay struct {
	QuestUID  string
	Name      string
	Emoji     string
	SkillID   string // "" = untagged (no mastery feed)
	Questions []QuestPlayQuestion
}

// QuestMapItem is one quest card on the child's map. Kid-facing: it never
// carries prices, balances, or any monetisation data.
type QuestMapItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Emoji   string `json:"emoji,omitempty"`
	Total   int    `json:"total"`
	Correct int    `json:"correct"`
	Done    bool   `json:"done"`
}

// QuestSource is the game's window into the quests control plane
// (implemented by internal/saas/quests.Service). All methods are scoped to
// the child: PlayableQuest must return ErrQuestUnavailable for quests the
// child may not play, and ErrQuestDone when nothing remains.
type QuestSource interface {
	PlayableQuest(ctx context.Context, childUID, questUID string) (*QuestPlay, error)
	RecordAnswer(ctx context.Context, questUID, childUID, questionUID string, correct bool) (remaining int, err error)
	// ActiveQuests must be side-effect-free: it backs the map read.
	ActiveQuests(ctx context.Context, childUID string) ([]QuestMapItem, error)
}

// questRun is the quest-specific state carried by a quest expedition.
type questRun struct {
	uid       string
	name      string
	tagged    bool // skill_id set and resolvable → answers feed mastery
	total     int  // questions this expedition serves: min(5, remaining)
	questions []QuestPlayQuestion
	complete  bool // every question in the quest answered correctly
}

// questGenerator implements problemgen.Generator over the authored question
// list, serving them in order. It runs under the expedition mutex, so no
// locking of its own.
type questGenerator struct {
	questions []QuestPlayQuestion
	next      int
}

func (g *questGenerator) Generate(_ context.Context, input problemgen.GenerateInput) (*problemgen.Question, error) {
	if g.next >= len(g.questions) {
		return nil, ErrExpeditionOver
	}
	qq := g.questions[g.next]
	g.next++
	return &problemgen.Question{
		Text:        qq.Text,
		Format:      problemgen.AnswerFormat(qq.Format),
		Answer:      qq.Answer,
		AnswerType:  problemgen.AnswerType(qq.AnswerType),
		Choices:     qq.Choices,
		Hint:        qq.Hint,
		Difficulty:  3,
		Explanation: qq.Explanation,
		// For tagged quests this is the real skill (mastery advances); for
		// untagged it is the synthetic "quest:<uid>" ID, which is not in the
		// skill graph, so the session engine skips mastery/spaced-rep for it.
		SkillID: input.Skill.ID,
		Tier:    input.Tier,
	}, nil
}

// StartQuest begins a quest expedition, mirroring Start: one start at a time
// per child, double-click reuse, cross-surface play slot, and the same
// 1-credit charge keyed by the session ID.
func (m *Manager) StartQuest(ctx context.Context, childUID, questUID string) (*ExpeditionView, error) {
	if m.cfg.Quests == nil {
		return nil, ErrQuestUnavailable
	}

	start := m.childStartLock(childUID)
	start.Lock()
	defer start.Unlock()

	m.reapIdle(ctx)

	// An untouched expedition on the same quest is returned as-is: a
	// double-click must not debit a second credit or fork the snapshot.
	m.mu.Lock()
	prev := m.byChild[childUID]
	m.mu.Unlock()
	if prev != nil {
		prev.mu.Lock()
		reuse := !prev.finished && prev.quest != nil && prev.quest.uid == questUID && prev.questionsAsked == 0
		var view *ExpeditionView
		if reuse {
			prev.touch()
			view = prev.expeditionView()
		}
		prev.mu.Unlock()
		if reuse {
			return view, nil
		}
		m.remove(prev)
		prev.finish(ctx, false)
	}

	play, err := m.cfg.Quests.PlayableQuest(ctx, childUID, questUID)
	if err != nil {
		return nil, err
	}
	if len(play.Questions) == 0 {
		return nil, ErrQuestDone
	}

	releaseSlot, err := m.cfg.Slots.Acquire(childUID, "the treasure map")
	if err != nil {
		return nil, ErrElsewhere
	}
	registered := false
	defer func() {
		if !registered {
			releaseSlot()
		}
	}()

	eventRepo := m.cfg.Store.EventRepoFor(childUID)
	snapRepo := m.cfg.Store.SnapshotRepoFor(childUID)

	snap, err := snapRepo.Latest(ctx)
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}
	var snapData *store.SnapshotData
	if snap != nil {
		snapData = &snap.Data
	}

	// Tagged quests feed the normal mastery/spaced-rep services (with real
	// event repos — quest practice pushes the main map forward, and decay
	// persists at expedition start, like Start). Untagged quests run the
	// engine over side-effect-free services: nil event repos and no decay
	// check (the Map-read trick), so graph state is untouched while answer /
	// session / gem events still persist to the child's stream.
	tagged := play.SkillID != ""
	var skill skillgraph.Skill
	if tagged {
		skill, err = skillgraph.GetSkill(play.SkillID)
		if err != nil {
			tagged = false // stale tag: degrade to untagged rather than block play
		}
	}
	if !tagged {
		skill = skillgraph.Skill{ID: "quest:" + play.QuestUID, Name: play.Name}
	}

	var masterySvc *mastery.Service
	var scheduler *spacedrep.Scheduler
	if tagged {
		masterySvc = mastery.NewService(snapData, eventRepo)
		scheduler = spacedrep.NewScheduler(snapData, masterySvc, eventRepo)
		scheduler.RunDecayCheck(ctx, time.Now())
	} else {
		masterySvc = mastery.NewService(snapData, nil)
		scheduler = spacedrep.NewScheduler(snapData, masterySvc, nil)
	}

	mastered := masterySvc.MasteredSkills()
	due := make(map[string]bool)
	for _, id := range scheduler.DueSkills(time.Now()) {
		due[id] = true
	}

	// Category and tier. Quests are never fog-locked — the parent chose the
	// content, so a tagged skill the child hasn't unlocked still plays.
	tier := skillgraph.TierLearn
	category := sess.CategoryFrontier
	if tagged {
		sm := masterySvc.GetMastery(skill.ID)
		tier = sm.CurrentTier
		switch sm.State {
		case mastery.StateRusty:
			category = sess.CategoryReview
		case mastery.StateMastered:
			if due[skill.ID] {
				category = sess.CategoryReview
			} else {
				category = sess.CategoryBooster
			}
		}
	}

	tierProgress := make(map[string]*sess.TierProgress)
	for id, skm := range masterySvc.AllSkillMasteries() {
		if skm.State == mastery.StateNew {
			continue
		}
		tierProgress[id] = &sess.TierProgress{
			SkillID:       id,
			CurrentTier:   skm.CurrentTier,
			TotalAttempts: skm.TotalAttempts,
			CorrectCount:  skm.CorrectCount,
			Accuracy:      skm.Accuracy(),
		}
	}

	// Build the toolset BEFORE charging (diagnosis/lessons still run for
	// quests); the question generator is the authored list, not the LLM.
	tools, err := m.cfg.Toolset(ctx, eventRepo)
	if err != nil {
		return nil, err
	}

	sessionID := uuid.NewString()
	if m.cfg.Charge != nil {
		if err := m.cfg.Charge(ctx, childUID, sessionID); err != nil {
			if tools.Diagnosis != nil {
				tools.Diagnosis.Close()
			}
			return nil, err
		}
	}

	total := len(play.Questions)
	if total > QuestionsPerExpedition {
		total = QuestionsPerExpedition
	}
	quest := &questRun{
		uid:       play.QuestUID,
		name:      play.Name,
		tagged:    tagged,
		total:     total,
		questions: play.Questions[:total],
	}
	tools.Generator = &questGenerator{questions: quest.questions}

	plan := &sess.Plan{
		Slots: []sess.PlanSlot{{
			Skill:    skill,
			Tier:     tier,
			Category: category,
		}},
		Duration: sess.DefaultSessionDuration,
	}

	state := sess.NewSessionState(plan, sessionID, mastered, tierProgress)
	gemSvc := gems.NewService(eventRepo)

	state.MasteryService = masterySvc
	state.SpacedRepSched = scheduler
	state.DiagnosisService = tools.Diagnosis
	state.EventRepo = eventRepo
	state.LessonService = tools.Lessons
	state.Compressor = tools.Compressor
	state.GemService = gemSvc
	gemSvc.ResetSession()

	_ = eventRepo.AppendSessionEvent(ctx, store.SessionEventData{
		SessionID: sessionID,
		Action:    "start",
		PlanSummary: []store.PlanSlotSummaryData{{
			SkillID:  skill.ID,
			Tier:     sess.TierString(tier),
			Category: string(category),
		}},
	})

	learnerProfile := ""
	if snapData != nil && snapData.LearnerProfile != nil {
		learnerProfile = snapData.LearnerProfile.Summary
	}

	exp := &expedition{
		id:             uuid.NewString(),
		childUID:       childUID,
		skill:          skill,
		category:       category,
		quest:          quest,
		origSnap:       snapData,
		state:          state,
		masterySvc:     masterySvc,
		scheduler:      scheduler,
		gemSvc:         gemSvc,
		tools:          tools,
		eventRepo:      eventRepo,
		snapRepo:       snapRepo,
		learnerProfile: learnerProfile,
		releaseSlot:    releaseSlot,
	}
	exp.touch()

	// Build the view before publishing the expedition in the maps —
	// expeditionView requires e.mu once the expedition is reachable.
	view := exp.expeditionView()

	m.mu.Lock()
	m.byID[exp.id] = exp
	m.byChild[childUID] = exp
	m.mu.Unlock()
	registered = true

	return view, nil
}

// expeditionView builds the start/reuse view. Caller holds e.mu (or the
// expedition is not yet registered).
func (e *expedition) expeditionView() *ExpeditionView {
	v := &ExpeditionView{
		ID:             e.id,
		SkillID:        e.skill.ID,
		SkillName:      e.skill.Name,
		TotalQuestions: e.totalQuestions(),
		Tier:           sess.TierString(e.state.Plan.Slots[0].Tier),
		Category:       string(e.category),
	}
	if e.quest != nil {
		v.QuestID = e.quest.uid
		v.SkillName = e.quest.name
		if !e.quest.tagged {
			v.SkillID = ""
		}
	}
	return v
}

// recordQuestProgress upserts the quest_progress row for the just-graded
// question and flags quest completion. Caller holds e.mu.
func (m *Manager) recordQuestProgress(ctx context.Context, exp *expedition, correct bool) {
	q := exp.quest
	if q == nil || m.cfg.Quests == nil {
		return
	}
	idx := exp.questionsAsked - 1
	if idx < 0 || idx >= len(q.questions) {
		return
	}
	remaining, err := m.cfg.Quests.RecordAnswer(ctx, q.uid, exp.childUID, q.questions[idx].UID, correct)
	if err != nil {
		log.Printf("game: record quest progress for %s: %v", exp.childUID, err)
		return
	}
	if remaining == 0 {
		q.complete = true
	}
}
