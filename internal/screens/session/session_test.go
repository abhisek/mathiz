package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/screen"
	sess "github.com/abhisek/mathiz/internal/session"
	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/store"
)

// mockGenerator implements problemgen.Generator for testing.
type mockGenerator struct {
	question *problemgen.Question
	err      error
}

func (m *mockGenerator) Generate(_ context.Context, input problemgen.GenerateInput) (*problemgen.Question, error) {
	if m.err != nil {
		return nil, m.err
	}
	q := *m.question
	q.SkillID = input.Skill.ID
	q.Tier = input.Tier
	return &q, nil
}

// mockEventRepo implements store.EventRepo for testing.
type mockEventRepo struct {
	sessionEvents []store.SessionEventData
	answerEvents  []store.AnswerEventData
}

func (m *mockEventRepo) AppendLLMRequest(_ context.Context, _ store.LLMRequestEventData) error {
	return nil
}
func (m *mockEventRepo) AppendSessionEvent(_ context.Context, data store.SessionEventData) error {
	m.sessionEvents = append(m.sessionEvents, data)
	return nil
}
func (m *mockEventRepo) AppendAnswerEvent(_ context.Context, data store.AnswerEventData) error {
	m.answerEvents = append(m.answerEvents, data)
	return nil
}
func (m *mockEventRepo) LatestAnswerTime(_ context.Context, _ string) (time.Time, error) {
	return time.Time{}, nil
}
func (m *mockEventRepo) SkillAccuracy(_ context.Context, _ string) (float64, error) {
	return 0, nil
}
func (m *mockEventRepo) AppendMasteryEvent(_ context.Context, _ store.MasteryEventData) error {
	return nil
}
func (m *mockEventRepo) RecentReviewAccuracy(_ context.Context, _ string, _ int) (float64, int, error) {
	return 0, 0, nil
}
func (m *mockEventRepo) AppendDiagnosisEvent(_ context.Context, _ store.DiagnosisEventData) error {
	return nil
}
func (m *mockEventRepo) AppendHintEvent(_ context.Context, _ store.HintEventData) error {
	return nil
}
func (m *mockEventRepo) AppendLessonEvent(_ context.Context, _ store.LessonEventData) error {
	return nil
}

func (m *mockEventRepo) AppendGemEvent(_ context.Context, _ store.GemEventData) error {
	return nil
}

func (m *mockEventRepo) QueryGemEvents(_ context.Context, _ store.QueryOpts) ([]store.GemEventRecord, error) {
	return nil, nil
}

func (m *mockEventRepo) GemCounts(_ context.Context) (map[string]int, int, error) {
	return nil, 0, nil
}

func (m *mockEventRepo) QuerySessionSummaries(_ context.Context, _ store.QueryOpts) ([]store.SessionSummaryRecord, error) {
	return nil, nil
}
func (m *mockEventRepo) QueryLLMEvents(_ context.Context, _ store.QueryOpts) ([]store.LLMRequestEventRecord, error) {
	return nil, nil
}

func (m *mockEventRepo) QueryLessonEvents(_ context.Context, _ store.QueryOpts) ([]store.LessonEventRecord, error) {
	return nil, nil
}
func (m *mockEventRepo) AppendLearnerProfileEvent(_ context.Context, _ store.LearnerProfileEventData) error {
	return nil
}
func (m *mockEventRepo) QueryLearnerProfileEvents(_ context.Context, _ store.QueryOpts) ([]store.LearnerProfileEventRecord, error) {
	return nil, nil
}
func (m *mockEventRepo) GetLLMEvent(_ context.Context, _ int) (*store.LLMRequestEventRecord, error) {
	return nil, nil
}
func (m *mockEventRepo) LLMUsageByPurpose(_ context.Context) ([]store.LLMUsageStats, error) {
	return nil, nil
}
func (m *mockEventRepo) LLMUsageByModel(_ context.Context) ([]store.LLMModelUsage, error) {
	return nil, nil
}
func (m *mockEventRepo) QueryMasteryEvents(_ context.Context, _ store.QueryOpts) ([]store.MasteryEventRecord, error) {
	return nil, nil
}
func (m *mockEventRepo) AnswersForSession(_ context.Context, _ string) ([]store.AnswerEventRecord, error) {
	return nil, nil
}
func (m *mockEventRepo) HintCountForSession(_ context.Context, _ string) (int, error) {
	return 0, nil
}

// mockSnapshotRepo implements store.SnapshotRepo for testing.
type mockSnapshotRepo struct {
	snapshots []*store.Snapshot
}

func (m *mockSnapshotRepo) Save(_ context.Context, snap *store.Snapshot) error {
	m.snapshots = append(m.snapshots, snap)
	return nil
}
func (m *mockSnapshotRepo) Latest(_ context.Context) (*store.Snapshot, error) {
	if len(m.snapshots) == 0 {
		return nil, nil
	}
	return m.snapshots[len(m.snapshots)-1], nil
}
func (m *mockSnapshotRepo) UpdateLearnerProfile(_ context.Context, profile *store.LearnerProfileData) error {
	if len(m.snapshots) == 0 {
		return store.ErrNoSnapshot
	}
	// In place, like the real repo: no new snapshot row.
	m.snapshots[len(m.snapshots)-1].Data.LearnerProfile = profile
	return nil
}
func (m *mockSnapshotRepo) Prune(_ context.Context, _ int) error {
	return nil
}

func keyPress(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func specialKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func testSessionScreen() (*SessionScreen, *mockEventRepo, *mockSnapshotRepo) {
	gen := &mockGenerator{
		question: &problemgen.Question{
			Text:        "What is 1 + 1?",
			Format:      problemgen.FormatNumeric,
			Answer:      "2",
			AnswerType:  problemgen.AnswerTypeInteger,
			Explanation: "1 + 1 = 2",
		},
	}
	eventRepo := &mockEventRepo{}
	snapRepo := &mockSnapshotRepo{}

	s := New(gen, eventRepo, snapRepo, nil, nil, nil, nil)
	return s, eventRepo, snapRepo
}

func setupActiveSession(s *SessionScreen) {
	skills := skillgraph.AllSkills()
	plan := &sess.Plan{
		Slots: []sess.PlanSlot{
			{Skill: skills[0], Tier: skillgraph.TierLearn, Category: sess.CategoryFrontier},
			{Skill: skills[1], Tier: skillgraph.TierLearn, Category: sess.CategoryFrontier},
		},
		Duration: sess.DefaultSessionDuration,
	}
	s.state = sess.NewSessionState(plan, "test-session", nil, nil)
	s.state.CurrentQuestion = &problemgen.Question{
		Text:        "What is 1 + 1?",
		Format:      problemgen.FormatNumeric,
		Answer:      "2",
		AnswerType:  problemgen.AnswerTypeInteger,
		Explanation: "1 + 1 = 2",
		SkillID:     skills[0].ID,
		Tier:        skillgraph.TierLearn,
	}
	s.state.QuestionsInSlot = 1
	s.state.QuestionStartTime = time.Now()
}

func TestSessionScreen_Title(t *testing.T) {
	s, _, _ := testSessionScreen()
	if s.Title() != "Session" {
		t.Errorf("Title = %q, want %q", s.Title(), "Session")
	}
}

func TestSessionScreen_View_Loading(t *testing.T) {
	s, _, _ := testSessionScreen()
	view := s.View(80, 24)
	if view == "" {
		t.Error("expected non-empty view for loading state")
	}
}

func TestSessionScreen_View_Error(t *testing.T) {
	s, _, _ := testSessionScreen()
	s.errMsg = "test error"
	view := s.View(80, 24)
	if view == "" {
		t.Error("expected non-empty view for error state")
	}
}

func TestSessionScreen_QuitConfirm(t *testing.T) {
	s, _, _ := testSessionScreen()
	setupActiveSession(s)

	// Press Esc to show quit dialog.
	var scr screen.Screen = s
	scr, _ = scr.Update(specialKey(tea.KeyEscape))
	ss := scr.(*SessionScreen)
	if !ss.state.ShowingQuitConfirm {
		t.Error("expected quit confirmation dialog")
	}

	// Press N to dismiss.
	scr, _ = ss.Update(keyPress('n'))
	ss = scr.(*SessionScreen)
	if ss.state.ShowingQuitConfirm {
		t.Error("expected quit confirmation to be dismissed")
	}
}

func TestSessionScreen_QuitConfirm_Yes(t *testing.T) {
	s, _, _ := testSessionScreen()
	setupActiveSession(s)

	// Press Esc then Y.
	var scr screen.Screen = s
	scr, _ = scr.Update(specialKey(tea.KeyEscape))
	_, cmd := scr.Update(keyPress('y'))

	if cmd == nil {
		t.Error("expected a command after quit confirmation")
	}
}

func TestSessionScreen_FeedbackDismiss(t *testing.T) {
	s, _, _ := testSessionScreen()
	setupActiveSession(s)
	s.state.ShowingFeedback = true
	s.state.Phase = sess.PhaseFeedback

	// Press any key to dismiss feedback.
	var scr screen.Screen = s
	_, cmd := scr.Update(keyPress(' '))
	if cmd == nil {
		t.Error("expected a command after feedback dismiss")
	}
}

func TestSessionScreen_AnswerSubmit(t *testing.T) {
	s, eventRepo, _ := testSessionScreen()
	setupActiveSession(s)

	// Type answer.
	s.input.Model.SetValue("2")

	// Submit.
	var scr screen.Screen = s
	scr, _ = scr.Update(specialKey(tea.KeyEnter))
	ss := scr.(*SessionScreen)

	if !ss.state.ShowingFeedback {
		t.Error("expected feedback to be shown after submit")
	}
	if !ss.state.LastAnswerCorrect {
		t.Error("expected answer to be correct")
	}

	// Check event was persisted.
	if len(eventRepo.answerEvents) != 1 {
		t.Errorf("answer events = %d, want 1", len(eventRepo.answerEvents))
	}
}

func TestSessionScreen_MultipleChoice(t *testing.T) {
	s, _, _ := testSessionScreen()
	setupActiveSession(s)

	// Switch to multiple choice.
	s.state.CurrentQuestion.Format = problemgen.FormatMultipleChoice
	s.state.CurrentQuestion.Choices = []string{"1", "2", "3", "4"}
	s.state.CurrentQuestion.Answer = "2"
	s.mcActive = true
	s.mcSelected = 0

	// Press 2 to select correct answer.
	var scr screen.Screen = s
	scr, _ = scr.Update(keyPress('2'))
	ss := scr.(*SessionScreen)

	if !ss.state.ShowingFeedback {
		t.Error("expected feedback after MC answer")
	}
	if !ss.state.LastAnswerCorrect {
		t.Error("expected correct answer for choice 2")
	}
}

func TestSessionScreen_KeyHints(t *testing.T) {
	s, _, _ := testSessionScreen()
	setupActiveSession(s)

	hints := s.KeyHints()
	if len(hints) == 0 {
		t.Error("expected non-empty key hints")
	}
}

func TestSessionScreen_TimerDisplay(t *testing.T) {
	s, _, _ := testSessionScreen()
	setupActiveSession(s)

	view := s.View(80, 24)
	if view == "" {
		t.Error("expected non-empty view with timer")
	}
}

// recordingGenerator counts calls and records inputs; prefetching calls it
// from a background goroutine.
type recordingGenerator struct {
	mu     sync.Mutex
	inputs []problemgen.GenerateInput
}

func (g *recordingGenerator) Generate(_ context.Context, input problemgen.GenerateInput) (*problemgen.Question, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inputs = append(g.inputs, input)
	return &problemgen.Question{
		Text:       fmt.Sprintf("What is %d + 1?", len(g.inputs)),
		Format:     problemgen.FormatNumeric,
		Answer:     fmt.Sprint(len(g.inputs) + 1),
		AnswerType: problemgen.AnswerTypeInteger,
		SkillID:    input.Skill.ID,
		Tier:       input.Tier,
	}, nil
}

func (g *recordingGenerator) calls() []problemgen.GenerateInput {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]problemgen.GenerateInput(nil), g.inputs...)
}

func TestSessionScreen_PrefetchesNextQuestionDuringFeedback(t *testing.T) {
	gen := &recordingGenerator{}
	s := New(gen, &mockEventRepo{}, &mockSnapshotRepo{}, nil, nil, nil, nil)
	t.Cleanup(s.closePrefetch)
	setupActiveSession(s)
	s.learnerProfile = "loves pirate stories"

	s.input.Model.SetValue("2")
	var scr screen.Screen = s
	scr, _ = scr.Update(specialKey(tea.KeyEnter))
	ss := scr.(*SessionScreen)
	if !ss.state.ShowingFeedback {
		t.Fatal("expected feedback after submit")
	}

	// The next question starts generating while feedback is on screen.
	deadline := time.Now().Add(2 * time.Second)
	for len(gen.calls()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	calls := gen.calls()
	if len(calls) != 1 {
		t.Fatalf("generator calls during feedback = %d, want 1", len(calls))
	}
	if calls[0].LearnerProfile != "loves pirate stories" {
		t.Errorf("warm input profile = %q, want the cached profile", calls[0].LearnerProfile)
	}
	if len(calls[0].PriorQuestions) != 1 || calls[0].PriorQuestions[0] != "What is 1 + 1?" {
		t.Errorf("warm input prior questions = %v", calls[0].PriorQuestions)
	}

	// Dismissing feedback asks for the same input: the warmed question is
	// served without a second call.
	ss.state.ShowingFeedback = false
	ss.state.Phase = sess.PhaseActive
	ss.state.CurrentQuestion = nil
	msg := ss.generateNextQuestion()()
	ready, ok := msg.(questionReadyMsg)
	if !ok || ready.Err != nil || ready.Question == nil {
		t.Fatalf("generateNextQuestion = %#v", msg)
	}
	if len(gen.calls()) != 1 {
		t.Errorf("generator calls after dismiss = %d, want 1 (prefetch hit)", len(gen.calls()))
	}
}

func TestSessionScreen_SessionEndClosesPrefetch(t *testing.T) {
	gen := &recordingGenerator{}
	s := New(gen, &mockEventRepo{}, &mockSnapshotRepo{}, nil, nil, nil, nil)
	setupActiveSession(s)
	s.handleSessionEnd()
	if _, err := s.prefetch.Get(context.Background(), problemgen.GenerateInput{}); !errors.Is(err, problemgen.ErrPrefetcherClosed) {
		t.Errorf("prefetcher after session end: %v, want closed", err)
	}
}
