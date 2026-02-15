package session

import (
	"context"
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

	s := New(gen, eventRepo, snapRepo, nil, nil, nil)
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
