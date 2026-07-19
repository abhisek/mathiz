// Package game is the treasure-map play experience: the skill graph rendered
// as islands, where solving AI-generated questions digs treasure, collects
// gems, and lifts the fog on new territory.
//
// It is a presentation layer only. Every learning mechanic — question
// generation, answer checking, mastery, spaced repetition, diagnosis, gems,
// learner profiles — runs through the same engine the terminal app uses
// (internal/session and friends), driven over HTTP instead of a TUI loop.
package game

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/abhisek/mathiz/internal/gems"
	"github.com/abhisek/mathiz/internal/lessons"
	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/mastery"
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/saas/playslot"
	sess "github.com/abhisek/mathiz/internal/session"
	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/spacedrep"
	"github.com/abhisek/mathiz/internal/store"
	"github.com/abhisek/mathiz/internal/tutor"
)

var (
	ErrLocked         = errors.New("this spot is still in the fog — master its paths first")
	ErrNoExpedition   = errors.New("no active expedition")
	ErrNoQuestion     = errors.New("no question awaiting an answer")
	ErrExpeditionOver = errors.New("expedition is finished")
	ErrNoHint         = errors.New("no hint available")
	ErrNoLesson       = errors.New("the guide has nothing to show right now")
	ErrGeneration     = errors.New("could not conjure a question, try again")
	// ErrElsewhere means the child's play slot is held by another surface
	// (e.g. a live expedition in another tab).
	ErrElsewhere = errors.New("you're already playing on another screen — close it first!")
	// ErrNoCredits means the family's credit balance can't cover the
	// expedition. The kid-facing surface must never show prices — the API
	// maps this to out_of_credits and the client shows "the ship rests".
	ErrNoCredits = errors.New("out of credits")
)

// QuestionsPerExpedition is how many digs one expedition holds.
const QuestionsPerExpedition = 5

// maxGenFailures aborts an expedition after this many consecutive LLM
// generation failures (mirrors the TUI's circuit breaker).
const maxGenFailures = 3

// Toolset is the per-child AI tooling for an expedition. It is the shared
// tutor.Toolset — the same bundle the terminal app wires in app.BuildOptions.
type Toolset = tutor.Toolset

// ToolsetFactory builds AI tooling wired to a child's event stream.
// Production uses NewLLMToolset; tests inject deterministic fakes.
type ToolsetFactory func(ctx context.Context, eventRepo store.EventRepo) (*Toolset, error)

// NewLLMToolset is the production factory: provider from env with usage
// logging into the child's event stream, exactly like the local CLI.
func NewLLMToolset(ctx context.Context, eventRepo store.EventRepo) (*Toolset, error) {
	provider, err := llm.NewProviderFromEnv(ctx, eventRepo)
	if err != nil {
		return nil, fmt.Errorf("LLM provider: %w", err)
	}
	return tutor.New(provider), nil
}

// Config configures a Manager.
type Config struct {
	Store       *store.Store
	Toolset     ToolsetFactory // defaults to NewLLMToolset
	IdleTimeout time.Duration  // defaults to 30 minutes

	// Charge debits the cost of one expedition before it starts (sessionID
	// is the idempotency key). Return ErrNoCredits to refuse. Nil = free
	// (local mode, self-hosters without billing, tests).
	Charge func(ctx context.Context, childUID, sessionID string) error

	// Slots is the cross-surface one-session-per-child registry: any play
	// surface must acquire from the same registry so two live sessions
	// can't run concurrently over the same snapshot. Nil = a private
	// registry (standalone/test use).
	Slots *playslot.Registry

	// Quests serves parent-authored quests (specs/15-quests.md). Nil =
	// quests disabled: no quest cards on the map, StartQuest refuses.
	Quests QuestSource
}

// Manager owns all live expeditions (one per child).
type Manager struct {
	cfg Config

	mu      sync.Mutex
	byID    map[string]*expedition
	byChild map[string]*expedition

	// startLocks serializes Start per child: the check-charge-register
	// sequence spans several critical sections, and two concurrent Starts
	// for one child must not both pass the byChild check (double debit,
	// two live expeditions clobbering each other's snapshot).
	startLocks map[string]*sync.Mutex
}

func NewManager(cfg Config) *Manager {
	if cfg.Toolset == nil {
		cfg.Toolset = NewLLMToolset
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 30 * time.Minute
	}
	if cfg.Slots == nil {
		cfg.Slots = playslot.NewRegistry()
	}
	return &Manager{
		cfg:        cfg,
		byID:       make(map[string]*expedition),
		byChild:    make(map[string]*expedition),
		startLocks: make(map[string]*sync.Mutex),
	}
}

func (m *Manager) childStartLock(childUID string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := m.startLocks[childUID]
	if l == nil {
		l = &sync.Mutex{}
		m.startLocks[childUID] = l
	}
	return l
}

// expedition is one live question run for one child on one skill.
type expedition struct {
	mu sync.Mutex

	id       string
	childUID string
	skill    skillgraph.Skill
	category sess.PlanCategory

	// quest is set for quest expeditions (see quest.go); nil for normal
	// dig-spot expeditions. origSnap is the snapshot the expedition loaded
	// at start — untagged quest expeditions save it back verbatim so quest
	// play can never move graph state.
	quest    *questRun
	origSnap *store.SnapshotData

	state      *sess.SessionState
	masterySvc *mastery.Service
	scheduler  *spacedrep.Scheduler
	gemSvc     *gems.Service
	tools      *Toolset
	eventRepo  store.EventRepo
	snapRepo   store.SnapshotRepo

	learnerProfile string // cached summary for question generation

	questionsAsked int  // questions generated so far
	answered       bool // current question already graded
	genFailures    int
	lesson         *lessons.Lesson // delivered lesson awaiting practice
	finished       bool
	releaseSlot    func() // frees the child's cross-surface play slot

	// lastActivity is atomic (unix nanos) so reapIdle can read it WITHOUT
	// exp.mu. Taking exp.mu under m.mu deadlocks against handlers that
	// call m.remove while holding exp.mu, and exp.mu is held across LLM
	// calls — never acquire it from anything that holds m.mu.
	lastActivity atomic.Int64
}

func (e *expedition) touch() { e.lastActivity.Store(time.Now().UnixNano()) }

// totalQuestions is how many questions this expedition serves: the standard
// 5 for dig spots, min(5, remaining) for quest expeditions.
func (e *expedition) totalQuestions() int {
	if e.quest != nil {
		return e.quest.total
	}
	return QuestionsPerExpedition
}

func (e *expedition) idle(timeout time.Duration) bool {
	return time.Since(time.Unix(0, e.lastActivity.Load())) > timeout
}

// ---- Expedition lifecycle ----

// Start begins an expedition on a skill, replacing any active one for the
// child (the replaced expedition's progress is saved first).
func (m *Manager) Start(ctx context.Context, childUID, skillID string) (*ExpeditionView, error) {
	skill, err := skillgraph.GetSkill(skillID)
	if err != nil {
		return nil, ErrLocked
	}

	// One Start at a time per child (see startLocks).
	start := m.childStartLock(childUID)
	start.Lock()
	defer start.Unlock()

	m.reapIdle(ctx)

	// An untouched expedition on the same skill is returned as-is instead
	// of being replaced: a double-click or double-tab must not debit a
	// second credit or fork the snapshot lineage.
	m.mu.Lock()
	prev := m.byChild[childUID]
	m.mu.Unlock()
	if prev != nil {
		prev.mu.Lock()
		reuse := !prev.finished && prev.quest == nil && prev.skill.ID == skillID && prev.questionsAsked == 0
		var view *ExpeditionView
		if reuse {
			prev.touch()
			view = &ExpeditionView{
				ID:             prev.id,
				SkillID:        prev.skill.ID,
				SkillName:      prev.skill.Name,
				TotalQuestions: QuestionsPerExpedition,
				Tier:           sess.TierString(prev.state.Plan.Slots[0].Tier),
				Category:       string(prev.category),
			}
		}
		prev.mu.Unlock()
		if reuse {
			return view, nil
		}
		// Retire the existing expedition before starting fresh (finish
		// saves its progress and frees its play slot).
		m.remove(prev)
		prev.finish(ctx, false)
	}

	// Claim the child's cross-surface play slot: two live sessions must
	// never run concurrently — they'd drive the same snapshot.
	// finishLocked frees the slot after the final save.
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

	masterySvc := mastery.NewService(snapData, eventRepo)
	scheduler := spacedrep.NewScheduler(snapData, masterySvc, eventRepo)
	scheduler.RunDecayCheck(ctx, time.Now())

	mastered := masterySvc.MasteredSkills()
	sm := masterySvc.GetMastery(skillID)

	due := make(map[string]bool)
	for _, id := range scheduler.DueSkills(time.Now()) {
		due[id] = true
	}

	// Diggability + category. Locked spots stay in the fog.
	var category sess.PlanCategory
	switch sm.State {
	case mastery.StateRusty:
		category = sess.CategoryReview
	case mastery.StateMastered:
		if due[skillID] {
			category = sess.CategoryReview
		} else {
			category = sess.CategoryBooster
		}
	case mastery.StateLearning:
		category = sess.CategoryFrontier
	default: // StateNew
		if !skillgraph.IsUnlocked(skillID, mastered) {
			return nil, ErrLocked
		}
		category = sess.CategoryFrontier
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

	// Build the toolset BEFORE charging: a misconfigured or down LLM must
	// not cost the family a credit for an expedition that can never start.
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

	plan := &sess.Plan{
		Slots: []sess.PlanSlot{{
			Skill:    skill,
			Tier:     sm.CurrentTier,
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
			Tier:     sess.TierString(sm.CurrentTier),
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

	m.mu.Lock()
	m.byID[exp.id] = exp
	m.byChild[childUID] = exp
	m.mu.Unlock()
	registered = true

	return &ExpeditionView{
		ID:             exp.id,
		SkillID:        skill.ID,
		SkillName:      skill.Name,
		TotalQuestions: QuestionsPerExpedition,
		Tier:           sess.TierString(sm.CurrentTier),
		Category:       string(category),
	}, nil
}

// lookup fetches a live expedition, enforcing ownership.
func (m *Manager) lookup(childUID, expID string) (*expedition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	exp := m.byID[expID]
	if exp == nil || exp.childUID != childUID {
		return nil, ErrNoExpedition
	}
	return exp, nil
}

// Question returns the current question, generating one if needed.
func (m *Manager) Question(ctx context.Context, childUID, expID string) (*QuestionView, error) {
	exp, err := m.lookup(childUID, expID)
	if err != nil {
		return nil, err
	}
	exp.mu.Lock()
	defer exp.mu.Unlock()
	exp.touch()

	if exp.finished {
		return nil, ErrExpeditionOver
	}

	// The unanswered current question is idempotent (page reloads).
	if exp.state.CurrentQuestion != nil && !exp.answered {
		return exp.questionView(), nil
	}
	if exp.questionsAsked >= exp.totalQuestions() {
		return nil, ErrExpeditionOver
	}

	// Live tier, never the frozen plan tier. Untagged quest expeditions
	// skip the lookup: their synthetic skill ID must not seed a phantom
	// mastery entry.
	tier := skillgraph.TierLearn
	if exp.quest == nil || exp.quest.tagged {
		tier = exp.masterySvc.GetMastery(exp.skill.ID).CurrentTier
	}

	exp.state.ErrorMu.Lock()
	recentErrors := append([]string(nil), exp.state.RecentErrors[exp.skill.ID]...)
	exp.state.ErrorMu.Unlock()

	q, err := exp.tools.Generator.Generate(ctx, problemgen.GenerateInput{
		Skill:          exp.skill,
		Tier:           tier,
		PriorQuestions: exp.state.PriorQuestions[exp.skill.ID],
		RecentErrors:   recentErrors,
		LearnerProfile: exp.learnerProfile,
	})
	if err != nil {
		exp.genFailures++
		if exp.genFailures >= maxGenFailures {
			m.remove(exp)
			exp.finishLocked(ctx, false)
		}
		return nil, ErrGeneration
	}
	exp.genFailures = 0

	exp.state.CurrentQuestion = q
	exp.state.QuestionStartTime = time.Now()
	exp.state.HintShown = false
	exp.state.HintAvailable = false
	exp.answered = false
	exp.questionsAsked++

	return exp.questionView(), nil
}

func (e *expedition) questionView() *QuestionView {
	q := e.state.CurrentQuestion
	v := &QuestionView{
		Index:      e.questionsAsked,
		Total:      e.totalQuestions(),
		Text:       q.Text,
		Format:     string(q.Format),
		Choices:    q.Choices,
		AnswerType: string(q.AnswerType),
		Tier:       sess.TierString(q.Tier),
	}
	// Prove-tier questions are timed in spirit: the client shows a countdown
	// (speed feeds the fluency score via server-side timing; nothing is
	// force-submitted — this is a nudge, not a guillotine).
	if q.Tier == skillgraph.TierProve {
		v.TimeLimitSecs = e.skill.Tiers[skillgraph.TierProve].TimeLimitSecs
	}
	return v
}

// Answer grades the current question through the shared session engine.
func (m *Manager) Answer(ctx context.Context, childUID, expID, answer string) (*AnswerResultView, error) {
	exp, err := m.lookup(childUID, expID)
	if err != nil {
		return nil, err
	}
	exp.mu.Lock()
	defer exp.mu.Unlock()
	exp.touch()

	if exp.finished {
		return nil, ErrExpeditionOver
	}
	if exp.state.CurrentQuestion == nil || exp.answered {
		return nil, ErrNoQuestion
	}

	state := exp.state
	q := state.CurrentQuestion
	masteredBefore := exp.masterySvc.MasteredSkills()

	adv := sess.HandleAnswer(state, answer)
	exp.answered = true

	// Persist exactly what the terminal driver persists.
	if state.MasteryTransition != nil {
		sm := exp.masterySvc.GetMastery(q.SkillID)
		_ = exp.eventRepo.AppendMasteryEvent(ctx, store.MasteryEventData{
			SkillID:      q.SkillID,
			FromState:    string(state.MasteryTransition.From),
			ToState:      string(state.MasteryTransition.To),
			Trigger:      state.MasteryTransition.Trigger,
			FluencyScore: sm.FluencyScore(),
			SessionID:    state.SessionID,
		})
	}
	timeMs := int(time.Since(state.QuestionStartTime).Milliseconds())
	_ = exp.eventRepo.AppendAnswerEvent(ctx, store.AnswerEventData{
		SessionID:     state.SessionID,
		SkillID:       q.SkillID,
		Tier:          sess.TierString(q.Tier),
		Category:      string(exp.category),
		QuestionText:  q.Text,
		CorrectAnswer: q.Answer,
		LearnerAnswer: answer,
		Correct:       state.LastAnswerCorrect,
		TimeMs:        timeMs,
		AnswerFormat:  string(q.Format),
	})

	// Quest progress is control-plane (specs/15-quests.md): one upsert per
	// graded answer, and completion detection when the last remaining
	// question is answered correctly.
	m.recordQuestProgress(ctx, exp, state.LastAnswerCorrect)

	result := &AnswerResultView{
		Correct:           state.LastAnswerCorrect,
		CorrectAnswer:     q.Answer,
		Explanation:       q.Explanation,
		HintAvailable:     state.HintAvailable && !state.HintShown,
		Streak:            state.ConsecutiveCorrect,
		QuestionsAnswered: exp.questionsAsked,
		TotalQuestions:    exp.totalQuestions(),
		LessonPending:     state.PendingLesson,
	}
	if award := state.PendingGemAward; award != nil {
		result.Gem = &GemAwardView{Type: string(award.Type), Rarity: string(award.Rarity), Reason: award.Reason}
	}
	if tr := state.MasteryTransition; tr != nil {
		result.Mastery = &MasteryChangeView{From: string(tr.From), To: string(tr.To)}
	}

	// Fog lifts: skills newly unlocked by this answer's mastery change.
	masteredAfter := exp.masterySvc.MasteredSkills()
	if len(masteredAfter) != len(masteredBefore) {
		for _, s := range skillgraph.AllSkills() {
			if masteredAfter[s.ID] {
				continue
			}
			if skillgraph.IsUnlocked(s.ID, masteredAfter) && !skillgraph.IsUnlocked(s.ID, masteredBefore) {
				result.UnlockedSkillIDs = append(result.UnlockedSkillIDs, s.ID)
			}
		}
	}
	_ = adv // tier advancement is visible through result.Mastery

	// The expedition ends after the last question, or triumphantly early
	// when the chest opens (skill mastered).
	mastered := masteredAfter[exp.skill.ID] && !masteredBefore[exp.skill.ID]
	if exp.questionsAsked >= exp.totalQuestions() || mastered {
		m.remove(exp)
		summary := exp.finishLocked(ctx, true)
		summary.Mastered = mastered
		result.Done = true
		result.Summary = summary
		result.LessonPending = false // no lesson after the ship sails home
	}
	return result, nil
}

// Lesson fetches the pending micro-lesson if the guide has finished writing
// it. Lessons generate asynchronously after a kid's second wrong answer on a
// skill, so the client polls until Ready.
func (m *Manager) Lesson(ctx context.Context, childUID, expID string) (*LessonView, error) {
	exp, err := m.lookup(childUID, expID)
	if err != nil {
		return nil, err
	}
	exp.mu.Lock()
	defer exp.mu.Unlock()
	exp.touch()

	if exp.lesson != nil {
		return lessonView(exp.lesson, true), nil
	}
	if !exp.state.PendingLesson || exp.tools == nil || exp.tools.Lessons == nil {
		return nil, ErrNoLesson
	}
	lesson, ready := exp.tools.Lessons.ConsumeLesson()
	if !ready || lesson == nil {
		return &LessonView{Ready: false}, nil
	}
	exp.lesson = lesson
	exp.state.PendingLesson = false
	return lessonView(lesson, true), nil
}

func lessonView(l *lessons.Lesson, ready bool) *LessonView {
	return &LessonView{
		Ready:         ready,
		Title:         l.Title,
		Explanation:   l.Explanation,
		WorkedExample: l.WorkedExample,
		Practice: &LessonPracticeView{
			Text:       l.PracticeQuestion.Text,
			AnswerType: l.PracticeQuestion.AnswerType,
		},
	}
}

// AnswerLesson grades the lesson's practice question (or records a skip),
// exactly as the terminal driver does.
func (m *Manager) AnswerLesson(ctx context.Context, childUID, expID, answer string, skip bool) (*LessonAnswerView, error) {
	exp, err := m.lookup(childUID, expID)
	if err != nil {
		return nil, err
	}
	exp.mu.Lock()
	defer exp.mu.Unlock()
	exp.touch()

	lesson := exp.lesson
	if lesson == nil {
		return nil, ErrNoLesson
	}
	exp.lesson = nil

	correct := false
	if !skip {
		correct = lesson.GradePractice(answer)
	}
	_ = exp.eventRepo.AppendLessonEvent(ctx, lesson.EventData(exp.state.SessionID, lesson.SkillID, !skip, correct, skip))
	return &LessonAnswerView{
		Correct:       correct,
		CorrectAnswer: lesson.PracticeQuestion.Answer,
		Explanation:   lesson.PracticeQuestion.Explanation,
	}, nil
}

// Hint reveals the hint for the just-answered question.
func (m *Manager) Hint(ctx context.Context, childUID, expID string) (*HintView, error) {
	exp, err := m.lookup(childUID, expID)
	if err != nil {
		return nil, err
	}
	exp.mu.Lock()
	defer exp.mu.Unlock()
	exp.touch()

	state := exp.state
	q := state.CurrentQuestion
	if q == nil || !state.HintAvailable || state.HintShown || q.Hint == "" {
		return nil, ErrNoHint
	}
	state.HintShown = true
	state.HintAvailable = false
	_ = exp.eventRepo.AppendHintEvent(ctx, store.HintEventData{
		SessionID:    state.SessionID,
		SkillID:      q.SkillID,
		QuestionText: q.Text,
		HintText:     q.Hint,
	})
	return &HintView{Hint: q.Hint}, nil
}

// End finishes an expedition early (kid sails home).
func (m *Manager) End(ctx context.Context, childUID, expID string) (*SummaryView, error) {
	exp, err := m.lookup(childUID, expID)
	if err != nil {
		return nil, err
	}
	m.remove(exp)
	return exp.finish(ctx, false), nil
}

// ---- internal plumbing ----

func (m *Manager) remove(exp *expedition) {
	m.mu.Lock()
	m.removeLocked(exp)
	m.mu.Unlock()
}

func (m *Manager) removeLocked(exp *expedition) {
	delete(m.byID, exp.id)
	if m.byChild[exp.childUID] == exp {
		delete(m.byChild, exp.childUID)
	}
}

// reapIdle finishes expeditions that went quiet, saving their progress.
// lastActivity is read atomically — never take exp.mu here (m.mu is held,
// and handlers acquire the locks in the opposite order).
func (m *Manager) reapIdle(ctx context.Context) {
	m.mu.Lock()
	var idle []*expedition
	for _, exp := range m.byID {
		if exp.idle(m.cfg.IdleTimeout) {
			idle = append(idle, exp)
		}
	}
	for _, exp := range idle {
		m.removeLocked(exp)
	}
	m.mu.Unlock()
	for _, exp := range idle {
		exp.finish(ctx, false)
	}
}

func (e *expedition) finish(ctx context.Context, completed bool) *SummaryView {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.finishLocked(ctx, completed)
}

// finishLocked ends the expedition: end event, session gem on natural
// completion, snapshot save, async learner-profile refresh. Caller holds e.mu.
func (e *expedition) finishLocked(ctx context.Context, completed bool) *SummaryView {
	if e.finished {
		return e.summaryLocked()
	}
	e.finished = true
	state := e.state

	_ = e.eventRepo.AppendSessionEvent(ctx, store.SessionEventData{
		SessionID:       state.SessionID,
		Action:          "end",
		QuestionsServed: state.TotalQuestions,
		CorrectAnswers:  state.TotalCorrect,
		DurationSecs:    int(time.Since(state.StartTime).Seconds()),
	})

	if completed && state.TotalQuestions > 0 {
		accuracy := float64(state.TotalCorrect) / float64(state.TotalQuestions)
		// Awarded for its side effect; summaryLocked reads SessionGems.
		e.gemSvc.AwardSession(ctx, accuracy, state.SessionID)
	}

	e.saveSnapshot(ctx)

	if e.tools != nil && e.tools.Diagnosis != nil {
		e.tools.Diagnosis.Close()
	}
	// Free the cross-surface play slot only now, after the final snapshot
	// save — earlier would reopen the concurrent-write window.
	if e.releaseSlot != nil {
		e.releaseSlot()
	}
	return e.summaryLocked()
}

func (e *expedition) summaryLocked() *SummaryView {
	state := e.state
	accuracy := 0.0
	if state.TotalQuestions > 0 {
		accuracy = float64(state.TotalCorrect) / float64(state.TotalQuestions)
	}
	s := &SummaryView{
		Questions: state.TotalQuestions,
		Correct:   state.TotalCorrect,
		Accuracy:  accuracy,
	}
	if e.quest != nil {
		s.QuestID = e.quest.uid
		s.QuestComplete = e.quest.complete
	}
	for _, g := range e.gemSvc.SessionGems {
		s.Gems = append(s.Gems, GemAwardView{Type: string(g.Type), Rarity: string(g.Rarity), Reason: g.Reason})
	}
	return s
}

// saveSnapshot persists the expedition's progress through the shared
// end-of-session path (sess.SaveSnapshotWithProfile) — the same code the
// terminal session screen runs.
func (e *expedition) saveSnapshot(ctx context.Context) {
	snapData := store.SnapshotData{Version: 4}
	snapData.Mastery = e.masterySvc.SnapshotData()
	snapData.SpacedRep = e.scheduler.SnapshotData()
	snapData.Gems = e.gemSvc.SnapshotData(ctx)

	// Untagged quest expeditions must leave graph state untouched
	// (specs/15-quests.md §2): save back exactly the mastery/spaced-rep
	// data the expedition loaded, not the services' in-memory state.
	if e.quest != nil && !e.quest.tagged {
		if e.origSnap != nil {
			snapData.Mastery = e.origSnap.Mastery
			snapData.SpacedRep = e.origSnap.SpacedRep
			// Legacy (pre-Mastery) snapshots keep their migration fields —
			// dropping them here would wipe the graph state this branch
			// exists to protect.
			snapData.TierProgress = e.origSnap.TierProgress
			snapData.MasteredSet = e.origSnap.MasteredSet
		} else {
			snapData.Mastery = nil
			snapData.SpacedRep = nil
		}
	}

	// No profile refresh for an expedition that never asked a question.
	var compressor *lessons.Compressor
	if e.tools != nil && e.state.TotalQuestions > 0 {
		compressor = e.tools.Compressor
	}
	if err := sess.SaveSnapshotWithProfile(ctx, e.snapRepo, compressor, e.state, snapData); err != nil {
		log.Printf("game: save snapshot for %s: %v", e.childUID, err)
	}
}
