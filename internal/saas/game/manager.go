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
	"time"

	"github.com/google/uuid"

	"github.com/abhisek/mathiz/internal/diagnosis"
	"github.com/abhisek/mathiz/internal/gems"
	"github.com/abhisek/mathiz/internal/lessons"
	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/mastery"
	"github.com/abhisek/mathiz/internal/problemgen"
	sess "github.com/abhisek/mathiz/internal/session"
	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/spacedrep"
	"github.com/abhisek/mathiz/internal/store"
)

var (
	ErrLocked         = errors.New("this spot is still in the fog — master its paths first")
	ErrNoExpedition   = errors.New("no active expedition")
	ErrNoQuestion     = errors.New("no question awaiting an answer")
	ErrExpeditionOver = errors.New("expedition is finished")
	ErrNoHint         = errors.New("no hint available")
	ErrGeneration     = errors.New("could not conjure a question, try again")
)

// QuestionsPerExpedition is how many digs one expedition holds.
const QuestionsPerExpedition = 5

// maxGenFailures aborts an expedition after this many consecutive LLM
// generation failures (mirrors the TUI's circuit breaker).
const maxGenFailures = 3

// Toolset is the per-child AI tooling for an expedition.
type Toolset struct {
	Generator  problemgen.Generator
	Diagnosis  *diagnosis.Service  // optional
	Compressor *lessons.Compressor // optional
}

// ToolsetFactory builds AI tooling wired to a child's event stream.
// Production uses NewLLMToolset; tests inject deterministic fakes.
type ToolsetFactory func(ctx context.Context, eventRepo store.EventRepo) (*Toolset, error)

// NewLLMToolset is the production factory: provider from env with usage
// logging into the child's event stream, exactly like the terminal bridge.
func NewLLMToolset(ctx context.Context, eventRepo store.EventRepo) (*Toolset, error) {
	provider, err := llm.NewProviderFromEnv(ctx, eventRepo)
	if err != nil {
		return nil, fmt.Errorf("LLM provider: %w", err)
	}
	return &Toolset{
		Generator:  problemgen.New(provider, problemgen.DefaultConfig()),
		Diagnosis:  diagnosis.NewService(provider),
		Compressor: lessons.NewCompressor(provider, lessons.DefaultCompressorConfig()),
	}, nil
}

// Config configures a Manager.
type Config struct {
	Store       *store.Store
	Toolset     ToolsetFactory // defaults to NewLLMToolset
	IdleTimeout time.Duration  // defaults to 30 minutes
}

// Manager owns all live expeditions (one per child).
type Manager struct {
	cfg Config

	mu      sync.Mutex
	byID    map[string]*expedition
	byChild map[string]*expedition
}

func NewManager(cfg Config) *Manager {
	if cfg.Toolset == nil {
		cfg.Toolset = NewLLMToolset
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 30 * time.Minute
	}
	return &Manager{
		cfg:     cfg,
		byID:    make(map[string]*expedition),
		byChild: make(map[string]*expedition),
	}
}

// expedition is one live question run for one child on one skill.
type expedition struct {
	mu sync.Mutex

	id       string
	childUID string
	skill    skillgraph.Skill
	category sess.PlanCategory

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
	lastActivity   time.Time
	finished       bool
}

// ---- Expedition lifecycle ----

// Start begins an expedition on a skill, replacing any active one for the
// child (the replaced expedition's progress is saved first).
func (m *Manager) Start(ctx context.Context, childUID, skillID string) (*ExpeditionView, error) {
	skill, err := skillgraph.GetSkill(skillID)
	if err != nil {
		return nil, ErrLocked
	}

	m.reapIdle(ctx)

	// Retire an existing expedition for this child before starting fresh.
	m.mu.Lock()
	if prev := m.byChild[childUID]; prev != nil {
		m.removeLocked(prev)
		m.mu.Unlock()
		prev.finish(ctx, false)
		m.mu.Lock()
	}
	m.mu.Unlock()

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

	tools, err := m.cfg.Toolset(ctx, eventRepo)
	if err != nil {
		return nil, err
	}

	plan := &sess.Plan{
		Slots: []sess.PlanSlot{{
			Skill:    skill,
			Tier:     sm.CurrentTier,
			Category: category,
		}},
		Duration: sess.DefaultSessionDuration,
	}

	sessionID := uuid.NewString()
	state := sess.NewSessionState(plan, sessionID, mastered, tierProgress)
	gemSvc := gems.NewService(eventRepo)

	state.MasteryService = masterySvc
	state.SpacedRepSched = scheduler
	state.DiagnosisService = tools.Diagnosis
	state.EventRepo = eventRepo
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
		lastActivity:   time.Now(),
	}

	m.mu.Lock()
	m.byID[exp.id] = exp
	m.byChild[childUID] = exp
	m.mu.Unlock()

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
	exp.lastActivity = time.Now()

	if exp.finished {
		return nil, ErrExpeditionOver
	}

	// The unanswered current question is idempotent (page reloads).
	if exp.state.CurrentQuestion != nil && !exp.answered {
		return exp.questionView(), nil
	}
	if exp.questionsAsked >= QuestionsPerExpedition {
		return nil, ErrExpeditionOver
	}

	// Live tier, never the frozen plan tier.
	tier := exp.masterySvc.GetMastery(exp.skill.ID).CurrentTier

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
	return &QuestionView{
		Index:      e.questionsAsked,
		Total:      QuestionsPerExpedition,
		Text:       q.Text,
		Format:     string(q.Format),
		Choices:    q.Choices,
		AnswerType: string(q.AnswerType),
		Tier:       sess.TierString(q.Tier),
	}
}

// Answer grades the current question through the shared session engine.
func (m *Manager) Answer(ctx context.Context, childUID, expID, answer string) (*AnswerResultView, error) {
	exp, err := m.lookup(childUID, expID)
	if err != nil {
		return nil, err
	}
	exp.mu.Lock()
	defer exp.mu.Unlock()
	exp.lastActivity = time.Now()

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

	result := &AnswerResultView{
		Correct:           state.LastAnswerCorrect,
		CorrectAnswer:     q.Answer,
		Explanation:       q.Explanation,
		HintAvailable:     state.HintAvailable && !state.HintShown,
		Streak:            state.ConsecutiveCorrect,
		QuestionsAnswered: exp.questionsAsked,
		TotalQuestions:    QuestionsPerExpedition,
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
	if exp.questionsAsked >= QuestionsPerExpedition || mastered {
		completed := exp.questionsAsked >= QuestionsPerExpedition || mastered
		m.remove(exp)
		summary := exp.finishLocked(ctx, completed)
		summary.Mastered = mastered
		result.Done = true
		result.Summary = summary
	}
	return result, nil
}

// Hint reveals the hint for the just-answered question.
func (m *Manager) Hint(ctx context.Context, childUID, expID string) (*HintView, error) {
	exp, err := m.lookup(childUID, expID)
	if err != nil {
		return nil, err
	}
	exp.mu.Lock()
	defer exp.mu.Unlock()
	exp.lastActivity = time.Now()

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
func (m *Manager) reapIdle(ctx context.Context) {
	m.mu.Lock()
	var idle []*expedition
	for _, exp := range m.byID {
		exp.mu.Lock()
		if time.Since(exp.lastActivity) > m.cfg.IdleTimeout {
			idle = append(idle, exp)
		}
		exp.mu.Unlock()
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
		if award := e.gemSvc.AwardSession(ctx, accuracy, state.SessionID); award != nil {
			// surfaced through the summary's gem list
			_ = award
		}
	}

	e.saveSnapshot(ctx)

	if e.tools != nil && e.tools.Diagnosis != nil {
		e.tools.Diagnosis.Close()
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
	for _, g := range e.gemSvc.SessionGems {
		s.Gems = append(s.Gems, GemAwardView{Type: string(g.Type), Rarity: string(g.Rarity), Reason: g.Reason})
	}
	return s
}

// saveSnapshot mirrors the terminal driver's snapshot save + async profile
// compression (session.go saveSnapshotWithProfile).
func (e *expedition) saveSnapshot(ctx context.Context) {
	snapData := store.SnapshotData{Version: 4}
	snapData.Mastery = e.masterySvc.SnapshotData()
	snapData.SpacedRep = e.scheduler.SnapshotData()
	snapData.Gems = e.gemSvc.SnapshotData(ctx)

	// Preserve the existing learner profile until the compressor refreshes it.
	if prev, err := e.snapRepo.Latest(ctx); err == nil && prev != nil && prev.Data.LearnerProfile != nil {
		snapData.LearnerProfile = prev.Data.LearnerProfile
	}

	if err := e.snapRepo.Save(ctx, &store.Snapshot{Timestamp: time.Now(), Data: snapData}); err != nil {
		log.Printf("game: save snapshot for %s: %v", e.childUID, err)
		return
	}
	_ = e.snapRepo.Prune(ctx, 10)

	if e.tools == nil || e.tools.Compressor == nil || e.state.TotalQuestions == 0 {
		return
	}

	// Async learner-profile refresh from this expedition's performance.
	input := lessons.ProfileInput{
		PerSkillResults: make(map[string]lessons.SkillResultSummary),
		MasteryData:     make(map[string]lessons.MasteryDataSummary),
		ErrorHistory:    make(map[string][]string),
	}
	for id, r := range e.state.PerSkillResults {
		input.PerSkillResults[id] = lessons.SkillResultSummary{Attempted: r.Attempted, Correct: r.Correct}
	}
	for id, skm := range snapData.Mastery.Skills {
		input.MasteryData[id] = lessons.MasteryDataSummary{State: skm.State}
	}
	e.state.ErrorMu.Lock()
	for id, errs := range e.state.RecentErrors {
		input.ErrorHistory[id] = append([]string(nil), errs...)
	}
	e.state.ErrorMu.Unlock()
	if prev, err := e.snapRepo.Latest(ctx); err == nil && prev != nil && prev.Data.LearnerProfile != nil {
		lp := prev.Data.LearnerProfile
		input.PreviousProfile = &lessons.LearnerProfile{
			Summary: lp.Summary, Strengths: lp.Strengths,
			Weaknesses: lp.Weaknesses, Patterns: lp.Patterns,
		}
	}

	compressor := e.tools.Compressor
	snapRepo := e.snapRepo
	childUID := e.childUID
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		profile, err := compressor.GenerateProfile(bg, input)
		if err != nil || profile == nil {
			return
		}
		latest, err := snapRepo.Latest(bg)
		if err != nil || latest == nil {
			return
		}
		latest.Data.LearnerProfile = &store.LearnerProfileData{
			Summary:     profile.Summary,
			Strengths:   profile.Strengths,
			Weaknesses:  profile.Weaknesses,
			Patterns:    profile.Patterns,
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if err := snapRepo.Save(bg, &store.Snapshot{Timestamp: time.Now(), Data: latest.Data}); err != nil {
			log.Printf("game: save learner profile for %s: %v", childUID, err)
		}
	}()
}
