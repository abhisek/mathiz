package session

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/abhisek/mathiz/internal/diagnosis"
	"github.com/abhisek/mathiz/internal/lessons"
	"github.com/abhisek/mathiz/internal/mastery"
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/router"
	"github.com/abhisek/mathiz/internal/screen"
	sess "github.com/abhisek/mathiz/internal/session"
	"github.com/abhisek/mathiz/internal/spacedrep"
	"github.com/abhisek/mathiz/internal/store"
	"github.com/abhisek/mathiz/internal/ui/components"
	"github.com/abhisek/mathiz/internal/ui/layout"

	"github.com/google/uuid"
)

// practiceState tracks the mini-practice question state within a lesson.
type practiceState int

const (
	practiceIdle          practiceState = iota
	practiceAnswering
	practiceShowingResult
)

// SessionScreen implements screen.Screen for the active session.
type SessionScreen struct {
	state         *sess.SessionState
	generator     problemgen.Generator
	eventRepo     store.EventRepo
	snapRepo      store.SnapshotRepo
	diagService   *diagnosis.Service
	lessonService *lessons.Service
	compressor    *lessons.Compressor
	planner       sess.Planner
	scheduler     *spacedrep.Scheduler
	input         components.TextInput
	mcActive      bool // true when showing multiple choice
	mcSelected    int
	errMsg        string

	// Hint overlay.
	showingHint bool

	// Lesson view.
	showingLesson   bool
	currentLesson   *lessons.Lesson
	practiceInput   components.TextInput
	practicePhase   practiceState
	practiceCorrect bool
}

var _ screen.Screen = (*SessionScreen)(nil)
var _ screen.KeyHintProvider = (*SessionScreen)(nil)

// New creates a new SessionScreen with injected dependencies.
func New(generator problemgen.Generator, eventRepo store.EventRepo, snapRepo store.SnapshotRepo, diagService *diagnosis.Service, lessonService *lessons.Service, compressor *lessons.Compressor) *SessionScreen {
	return &SessionScreen{
		generator:     generator,
		eventRepo:     eventRepo,
		snapRepo:      snapRepo,
		diagService:   diagService,
		lessonService: lessonService,
		compressor:    compressor,
		planner:       sess.NewPlanner(context.Background(), eventRepo),
		input:         components.NewTextInput("Type your answer...", false, 20),
		practiceInput: components.NewTextInput("Type your answer...", false, 20),
	}
}

func (s *SessionScreen) Init() tea.Cmd {
	return tea.Batch(
		s.initSession(),
		s.input.Init(),
	)
}

func (s *SessionScreen) Title() string {
	return "Session"
}

func (s *SessionScreen) KeyHints() []layout.KeyHint {
	if s.state == nil {
		return nil
	}
	if s.state.ShowingQuitConfirm {
		return []layout.KeyHint{
			{Key: "Y", Description: "End session"},
			{Key: "N", Description: "Keep going"},
		}
	}
	if s.showingHint {
		return []layout.KeyHint{
			{Key: "any key", Description: "Close hint"},
		}
	}
	if s.showingLesson {
		if s.practicePhase == practiceShowingResult {
			return []layout.KeyHint{
				{Key: "any key", Description: "Continue"},
			}
		}
		return []layout.KeyHint{
			{Key: "Enter", Description: "Submit"},
			{Key: "q", Description: "Skip"},
		}
	}
	if s.state.ShowingFeedback {
		return []layout.KeyHint{
			{Key: "any key", Description: "Continue"},
		}
	}
	hints := []layout.KeyHint{
		{Key: "Enter", Description: "Submit"},
	}
	if s.state.HintAvailable && !s.state.HintShown {
		hints = append(hints, layout.KeyHint{Key: "h", Description: "Hint"})
	}
	hints = append(hints, layout.KeyHint{Key: "Esc", Description: "Quit"})
	return hints
}

func (s *SessionScreen) View(width, height int) string {
	if s.errMsg != "" {
		return renderError(width, height, s.errMsg)
	}
	if s.state == nil {
		return renderLoading(width, height)
	}
	if s.showingHint {
		return s.renderHintOverlay(width, height)
	}
	if s.showingLesson {
		return s.renderLessonView(width, height)
	}
	if s.state.ShowingQuitConfirm {
		return renderQuitConfirm(width, height)
	}
	if s.state.ShowingFeedback {
		return s.renderFeedback(width, height)
	}
	return s.renderQuestionView(width, height)
}

func (s *SessionScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionInitMsg:
		return s.handleInit(msg)

	case questionReadyMsg:
		return s.handleQuestionReady(msg)

	case questionGenFailedMsg:
		return s.handleQuestionFailed(msg)

	case timerTickMsg:
		return s.handleTimerTick(msg)

	case feedbackDoneMsg:
		return s.handleFeedbackDone()

	case sessionEndMsg:
		return s.handleSessionEnd()

	case tea.KeyMsg:
		return s.handleKey(msg)
	}

	// Forward to practice input if in lesson mode.
	if s.showingLesson && s.practicePhase == practiceAnswering {
		var cmd tea.Cmd
		s.practiceInput, cmd = s.practiceInput.Update(msg)
		return s, cmd
	}

	// Forward to input if active.
	if s.state != nil && s.state.Phase == sess.PhaseActive && !s.state.ShowingFeedback && !s.state.ShowingQuitConfirm && !s.mcActive {
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return s, cmd
	}

	return s, nil
}

// initSession loads state from snapshot and builds the plan.
func (s *SessionScreen) initSession() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Load learner state from latest snapshot.
		var snapData *store.SnapshotData
		snap, err := s.snapRepo.Latest(ctx)
		if err != nil {
			return sessionInitMsg{Err: err}
		}
		if snap != nil {
			snapData = &snap.Data
		}

		// Create mastery service from snapshot.
		masterySvc := mastery.NewService(snapData, s.eventRepo)

		// Create spaced rep scheduler and run decay check.
		scheduler := spacedrep.NewScheduler(snapData, masterySvc, s.eventRepo)
		scheduler.RunDecayCheck(ctx, time.Now())

		// Wire scheduler into planner if it supports it.
		if dp, ok := s.planner.(*sess.DefaultPlanner); ok {
			dp.SetScheduler(scheduler)
		}

		// Derive mastered set and tier progress from mastery service.
		mastered := masterySvc.MasteredSkills()
		tierProgress := make(map[string]*sess.TierProgress)
		for id, sm := range masterySvc.AllSkillMasteries() {
			if sm.State == mastery.StateNew {
				continue
			}
			tierProgress[id] = &sess.TierProgress{
				SkillID:       id,
				CurrentTier:   sm.CurrentTier,
				TotalAttempts: sm.TotalAttempts,
				CorrectCount:  sm.CorrectCount,
				Accuracy:      sm.Accuracy(),
			}
		}

		// Build plan.
		plan, err := s.planner.BuildPlan(mastered, tierProgress)
		if err != nil {
			return sessionInitMsg{Err: err}
		}

		if len(plan.Slots) == 0 {
			return sessionInitMsg{Err: errors.New("no skills available for practice")}
		}

		sessionID := uuid.New().String()
		state := sess.NewSessionState(plan, sessionID, mastered, tierProgress)
		state.MasteryService = masterySvc
		state.SpacedRepSched = scheduler
		state.DiagnosisService = s.diagService
		state.EventRepo = s.eventRepo
		state.LessonService = s.lessonService
		state.Compressor = s.compressor

		// Persist session start event.
		var planSummary []store.PlanSlotSummaryData
		for _, slot := range plan.Slots {
			planSummary = append(planSummary, store.PlanSlotSummaryData{
				SkillID:  slot.Skill.ID,
				Tier:     sess.TierString(slot.Tier),
				Category: string(slot.Category),
			})
		}
		_ = s.eventRepo.AppendSessionEvent(ctx, store.SessionEventData{
			SessionID:   sessionID,
			Action:      "start",
			PlanSummary: planSummary,
		})

		// Keep scheduler reference for snapshot saving.
		s.scheduler = scheduler

		return sessionInitMsg{State: state}
	}
}

func (s *SessionScreen) handleInit(msg sessionInitMsg) (screen.Screen, tea.Cmd) {
	if msg.Err != nil {
		s.errMsg = msg.Err.Error()
		return s, nil
	}
	s.state = msg.State
	return s, tea.Batch(
		s.generateNextQuestion(),
		tickCmd(),
	)
}

func (s *SessionScreen) handleQuestionReady(msg questionReadyMsg) (screen.Screen, tea.Cmd) {
	if msg.Err != nil {
		// Skip to next slot on error.
		if s.state != nil {
			if !sess.AdvanceSlot(s.state) {
				return s, func() tea.Msg { return sessionEndMsg{} }
			}
			return s, s.generateNextQuestion()
		}
		s.errMsg = msg.Err.Error()
		return s, nil
	}

	s.state.CurrentQuestion = msg.Question
	s.state.QuestionsInSlot++
	s.state.QuestionStartTime = time.Now()
	s.state.HintShown = false
	s.state.HintAvailable = false

	// Setup input based on question format.
	if msg.Question.Format == problemgen.FormatMultipleChoice {
		s.mcActive = true
		s.mcSelected = 0
	} else {
		s.mcActive = false
		s.input = components.NewTextInput("Type your answer...", false, 20)
	}

	return s, s.input.Init()
}

func (s *SessionScreen) handleQuestionFailed(msg questionGenFailedMsg) (screen.Screen, tea.Cmd) {
	if s.state != nil {
		if !sess.AdvanceSlot(s.state) {
			return s, func() tea.Msg { return sessionEndMsg{} }
		}
		return s, s.generateNextQuestion()
	}
	s.errMsg = msg.Err.Error()
	return s, nil
}

func (s *SessionScreen) handleTimerTick(msg timerTickMsg) (screen.Screen, tea.Cmd) {
	if s.state == nil || s.state.Phase == sess.PhaseEnding || s.state.Phase == sess.PhaseSummary {
		return s, nil
	}

	s.state.Elapsed = time.Since(s.state.StartTime)

	if s.state.Elapsed >= s.state.Plan.Duration {
		s.state.TimeExpired = true
		// If not currently answering a question, end now.
		if s.state.ShowingFeedback || s.state.CurrentQuestion == nil {
			return s, func() tea.Msg { return sessionEndMsg{} }
		}
		// Otherwise let the learner finish their current question.
	}

	return s, tickCmd()
}

func (s *SessionScreen) handleFeedbackDone() (screen.Screen, tea.Cmd) {
	if s.state == nil {
		return s, nil
	}

	s.state.ShowingFeedback = false
	s.state.TierAdvanced = nil
	s.state.MasteryTransition = nil

	// If time expired, end the session.
	if s.state.TimeExpired {
		return s, func() tea.Msg { return sessionEndMsg{} }
	}

	// Check if tier was completed and slot should be removed.
	if s.state.CurrentQuestion != nil {
		slot := sess.CurrentSlot(s.state)
		if slot != nil {
			tp := s.state.TierProgress[slot.Skill.ID]
			if tp != nil {
				// Check if this skill was just mastered or tier advanced.
				if s.state.Mastered[slot.Skill.ID] {
					s.state.CompletedSlots[s.state.CurrentSlotIndex] = true
				}
			}
		}
	}

	// Check if a lesson is ready.
	if s.state.PendingLesson && s.lessonService != nil {
		if lesson, ok := s.lessonService.ConsumeLesson(); ok {
			s.currentLesson = lesson
			s.showingLesson = true
			s.practicePhase = practiceAnswering
			s.practiceInput = components.NewTextInput("Type your answer...", false, 20)
			s.state.PendingLesson = false
			return s, s.practiceInput.Init()
		}
		// Lesson not ready yet — don't block, proceed to next question.
		s.state.PendingLesson = false
	}

	// Advance to next question or slot.
	if sess.ShouldAdvanceSlot(s.state) {
		if !sess.AdvanceSlot(s.state) {
			return s, func() tea.Msg { return sessionEndMsg{} }
		}
	}

	return s, s.generateNextQuestion()
}

func (s *SessionScreen) handleSessionEnd() (screen.Screen, tea.Cmd) {
	if s.state == nil {
		return s, func() tea.Msg { return router.PopScreenMsg{} }
	}

	s.state.Phase = sess.PhaseEnding

	// Persist session end event.
	ctx := context.Background()
	durationSecs := int(s.state.Elapsed.Seconds())
	_ = s.eventRepo.AppendSessionEvent(ctx, store.SessionEventData{
		SessionID:       s.state.SessionID,
		Action:          "end",
		QuestionsServed: s.state.TotalQuestions,
		CorrectAnswers:  s.state.TotalCorrect,
		DurationSecs:    durationSecs,
	})

	// Generate learner profile asynchronously and save snapshot.
	s.saveSnapshotWithProfile(ctx)

	// Build summary and navigate.
	summary := sess.BuildSummary(s.state)

	return s, func() tea.Msg {
		return router.PushScreenMsg{
			Screen: newSummaryScreenAdapter(summary),
		}
	}
}

func (s *SessionScreen) handleKey(msg tea.KeyMsg) (screen.Screen, tea.Cmd) {
	key := msg.String()

	// Error state — any key goes back.
	if s.errMsg != "" {
		return s, func() tea.Msg { return router.PopScreenMsg{} }
	}

	if s.state == nil {
		return s, nil
	}

	// Hint overlay — any key dismisses.
	if s.showingHint {
		s.showingHint = false
		return s, nil
	}

	// Lesson view.
	if s.showingLesson {
		return s.handleLessonKey(key, msg)
	}

	// Quit confirmation dialog.
	if s.state.ShowingQuitConfirm {
		switch key {
		case "y", "Y":
			s.state.ShowingQuitConfirm = false
			return s, func() tea.Msg { return sessionEndMsg{} }
		case "n", "N", "esc":
			s.state.ShowingQuitConfirm = false
			return s, nil
		}
		return s, nil
	}

	// Feedback overlay — any key dismisses.
	if s.state.ShowingFeedback {
		return s, func() tea.Msg { return feedbackDoneMsg{} }
	}

	// Active question phase.
	if s.state.Phase == sess.PhaseActive {
		switch key {
		case "esc":
			s.state.ShowingQuitConfirm = true
			return s, nil
		case "enter":
			return s.submitAnswer()
		case "h":
			if s.state.HintAvailable && !s.state.HintShown {
				s.state.HintShown = true
				s.state.HintAvailable = false
				s.showingHint = true
				// Persist hint event.
				if s.state.CurrentQuestion != nil {
					_ = s.eventRepo.AppendHintEvent(context.Background(), store.HintEventData{
						SessionID:    s.state.SessionID,
						SkillID:      s.state.CurrentQuestion.SkillID,
						QuestionText: s.state.CurrentQuestion.Text,
						HintText:     s.state.CurrentQuestion.Hint,
					})
				}
				return s, nil
			}
		}

		// Multiple choice: number keys and arrows.
		if s.mcActive {
			switch key {
			case "1":
				s.mcSelected = 0
				return s.submitAnswer()
			case "2":
				if len(s.state.CurrentQuestion.Choices) > 1 {
					s.mcSelected = 1
					return s.submitAnswer()
				}
			case "3":
				if len(s.state.CurrentQuestion.Choices) > 2 {
					s.mcSelected = 2
					return s.submitAnswer()
				}
			case "4":
				if len(s.state.CurrentQuestion.Choices) > 3 {
					s.mcSelected = 3
					return s.submitAnswer()
				}
			case "up", "k":
				if s.mcSelected > 0 {
					s.mcSelected--
				}
				return s, nil
			case "down", "j":
				if s.mcSelected < len(s.state.CurrentQuestion.Choices)-1 {
					s.mcSelected++
				}
				return s, nil
			}
		}

		// Forward to text input.
		if !s.mcActive {
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			return s, cmd
		}
	}

	return s, nil
}

// handleLessonKey processes key presses during lesson view.
func (s *SessionScreen) handleLessonKey(key string, msg tea.KeyMsg) (screen.Screen, tea.Cmd) {
	if s.practicePhase == practiceShowingResult {
		// Any key exits lesson and resumes session.
		s.finishLesson()
		return s.advanceAfterLesson()
	}

	// In practice answering mode.
	switch key {
	case "q":
		// Skip practice.
		s.practiceCorrect = false
		s.persistLessonEvent(false, false, true)
		s.finishLesson()
		return s.advanceAfterLesson()
	case "enter":
		answer := s.practiceInput.Value()
		if answer == "" {
			return s, nil
		}
		// Check the practice answer.
		pq := s.currentLesson.PracticeQuestion
		tmpQ := &problemgen.Question{
			Answer:     pq.Answer,
			AnswerType: problemgen.AnswerType(pq.AnswerType),
			Format:     problemgen.FormatNumeric,
		}
		s.practiceCorrect = problemgen.CheckAnswer(answer, tmpQ)
		s.practicePhase = practiceShowingResult
		s.persistLessonEvent(true, s.practiceCorrect, false)
		return s, nil
	default:
		var cmd tea.Cmd
		s.practiceInput, cmd = s.practiceInput.Update(msg)
		return s, cmd
	}
}

func (s *SessionScreen) persistLessonEvent(attempted, correct, skipped bool) {
	if s.currentLesson == nil {
		return
	}
	_ = s.eventRepo.AppendLessonEvent(context.Background(), store.LessonEventData{
		SessionID:         s.state.SessionID,
		SkillID:           s.currentLesson.SkillID,
		LessonTitle:       s.currentLesson.Title,
		PracticeAttempted: attempted,
		PracticeCorrect:   correct,
		PracticeSkipped:   skipped,
	})
}

func (s *SessionScreen) finishLesson() {
	s.showingLesson = false
	s.currentLesson = nil
	s.practicePhase = practiceIdle
}

func (s *SessionScreen) advanceAfterLesson() (screen.Screen, tea.Cmd) {
	// If time expired, end the session.
	if s.state.TimeExpired {
		return s, func() tea.Msg { return sessionEndMsg{} }
	}

	// Check if slot should advance.
	if sess.ShouldAdvanceSlot(s.state) {
		if !sess.AdvanceSlot(s.state) {
			return s, func() tea.Msg { return sessionEndMsg{} }
		}
	}

	return s, s.generateNextQuestion()
}

// submitAnswer processes the current answer.
func (s *SessionScreen) submitAnswer() (screen.Screen, tea.Cmd) {
	if s.state == nil || s.state.CurrentQuestion == nil {
		return s, nil
	}

	var learnerAnswer string
	if s.mcActive {
		if s.mcSelected >= 0 && s.mcSelected < len(s.state.CurrentQuestion.Choices) {
			learnerAnswer = s.state.CurrentQuestion.Choices[s.mcSelected]
		}
	} else {
		learnerAnswer = s.input.Value()
		if learnerAnswer == "" {
			return s, nil
		}
	}

	// Record answer time.
	timeMs := int(time.Since(s.state.QuestionStartTime).Milliseconds())

	// Handle the answer (updates state, checks tier advancement).
	adv := sess.HandleAnswer(s.state, learnerAnswer)
	s.state.TierAdvanced = adv

	ctx := context.Background()

	// Persist mastery transition event if applicable.
	if s.state.MasteryTransition != nil && s.state.MasteryService != nil {
		t := s.state.MasteryTransition
		sm := s.state.MasteryService.GetMastery(s.state.CurrentQuestion.SkillID)
		_ = s.eventRepo.AppendMasteryEvent(ctx, store.MasteryEventData{
			SkillID:      t.SkillID,
			FromState:    string(t.From),
			ToState:      string(t.To),
			Trigger:      t.Trigger,
			FluencyScore: sm.FluencyScore(),
			SessionID:    s.state.SessionID,
		})
	}

	// Persist answer event.
	slot := sess.CurrentSlot(s.state)
	var category, tier string
	if slot != nil {
		category = string(slot.Category)
		tier = sess.TierString(slot.Tier)
	}
	_ = s.eventRepo.AppendAnswerEvent(ctx, store.AnswerEventData{
		SessionID:     s.state.SessionID,
		SkillID:       s.state.CurrentQuestion.SkillID,
		Tier:          tier,
		Category:      category,
		QuestionText:  s.state.CurrentQuestion.Text,
		CorrectAnswer: s.state.CurrentQuestion.Answer,
		LearnerAnswer: learnerAnswer,
		Correct:       s.state.LastAnswerCorrect,
		TimeMs:        timeMs,
		AnswerFormat:  string(s.state.CurrentQuestion.Format),
	})

	// If tier advanced mid-block, mark slot as completed.
	if adv != nil {
		s.state.CompletedSlots[s.state.CurrentSlotIndex] = true
	}

	// Show feedback.
	s.state.ShowingFeedback = true
	s.state.Phase = sess.PhaseFeedback

	return s, nil
}

// generateNextQuestion generates the next question asynchronously.
func (s *SessionScreen) generateNextQuestion() tea.Cmd {
	state := s.state
	return func() tea.Msg {
		if state == nil || len(state.Plan.Slots) == 0 {
			return questionGenFailedMsg{Err: errors.New("no slots available")}
		}

		slot := &state.Plan.Slots[state.CurrentSlotIndex]
		input := problemgen.GenerateInput{
			Skill:          slot.Skill,
			Tier:           slot.Tier,
			PriorQuestions: state.PriorQuestions[slot.Skill.ID],
			RecentErrors:   state.RecentErrors[slot.Skill.ID],
		}

		// Include learner profile if available from snapshot.
		if s.snapRepo != nil {
			snap, err := s.snapRepo.Latest(context.Background())
			if err == nil && snap != nil && snap.Data.LearnerProfile != nil {
				input.LearnerProfile = snap.Data.LearnerProfile.Summary
			}
		}

		var q *problemgen.Question
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			q, err = s.generator.Generate(context.Background(), input)
			if err == nil {
				break
			}
			var valErr *problemgen.ValidationError
			if errors.As(err, &valErr) && !valErr.Retryable {
				break
			}
		}
		if err != nil {
			return questionReadyMsg{Err: err}
		}
		return questionReadyMsg{Question: q}
	}
}

// saveSnapshot persists the current mastery state.
func (s *SessionScreen) saveSnapshot(ctx context.Context) *store.SnapshotData {
	snapData := store.SnapshotData{
		Version: 3,
	}

	if s.state.MasteryService != nil {
		snapData.Mastery = s.state.MasteryService.SnapshotData()
	}

	if s.scheduler != nil {
		snapData.SpacedRep = s.scheduler.SnapshotData()
	}

	// Preserve existing learner profile from previous snapshot.
	prevSnap, err := s.snapRepo.Latest(ctx)
	if err == nil && prevSnap != nil && prevSnap.Data.LearnerProfile != nil {
		snapData.LearnerProfile = prevSnap.Data.LearnerProfile
	}

	if s.state.MasteryService == nil {
		// Legacy fallback.
		tierProgressData := make(map[string]*store.TierProgressData)
		for id, tp := range s.state.TierProgress {
			tierProgressData[id] = &store.TierProgressData{
				SkillID:       tp.SkillID,
				CurrentTier:   sess.TierString(tp.CurrentTier),
				TotalAttempts: tp.TotalAttempts,
				CorrectCount:  tp.CorrectCount,
			}
		}
		var masteredSet []string
		for id := range s.state.Mastered {
			masteredSet = append(masteredSet, id)
		}
		snapData.TierProgress = tierProgressData
		snapData.MasteredSet = masteredSet
	}

	snap := &store.Snapshot{
		Timestamp: time.Now(),
		Data:      snapData,
	}
	_ = s.snapRepo.Save(ctx, snap)
	return &snapData
}

// saveSnapshotWithProfile saves the snapshot and triggers async profile generation.
func (s *SessionScreen) saveSnapshotWithProfile(ctx context.Context) {
	snapData := s.saveSnapshot(ctx)

	// Generate learner profile asynchronously if compressor is available.
	if s.compressor == nil {
		return
	}

	// Build profile input from session state.
	perSkillResults := make(map[string]lessons.SkillResultSummary)
	for id, sr := range s.state.PerSkillResults {
		perSkillResults[id] = lessons.SkillResultSummary{
			Attempted: sr.Attempted,
			Correct:   sr.Correct,
		}
	}

	masteryData := make(map[string]lessons.MasteryDataSummary)
	if snapData.Mastery != nil {
		for id, sm := range snapData.Mastery.Skills {
			masteryData[id] = lessons.MasteryDataSummary{
				State:        sm.State,
				FluencyScore: 0, // Simplified — fluency is derived, not stored directly.
			}
		}
	}

	var prevProfile *lessons.LearnerProfile
	if snapData.LearnerProfile != nil {
		prevProfile = &lessons.LearnerProfile{
			Summary:    snapData.LearnerProfile.Summary,
			Strengths:  snapData.LearnerProfile.Strengths,
			Weaknesses: snapData.LearnerProfile.Weaknesses,
			Patterns:   snapData.LearnerProfile.Patterns,
		}
	}

	input := lessons.ProfileInput{
		PerSkillResults: perSkillResults,
		MasteryData:     masteryData,
		ErrorHistory:    s.state.RecentErrors,
		PreviousProfile: prevProfile,
	}

	go func() {
		profile, err := s.compressor.GenerateProfile(ctx, input)
		if err != nil || profile == nil {
			return
		}
		profileData := &store.LearnerProfileData{
			Summary:     profile.Summary,
			Strengths:   profile.Strengths,
			Weaknesses:  profile.Weaknesses,
			Patterns:    profile.Patterns,
			GeneratedAt: profile.GeneratedAt.Format(time.RFC3339),
		}
		// Re-load and update snapshot with profile.
		latestSnap, err := s.snapRepo.Latest(ctx)
		if err != nil || latestSnap == nil {
			return
		}
		latestSnap.Data.LearnerProfile = profileData
		_ = s.snapRepo.Save(ctx, latestSnap)
	}()
}

// tickCmd returns a 1-second tick command.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return timerTickMsg(t)
	})
}
