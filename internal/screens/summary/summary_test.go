package summary

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/abhisek/mathiz/internal/session"
	"github.com/abhisek/mathiz/internal/skillgraph"
)

func testSummary() *session.SessionSummary {
	return &session.SessionSummary{
		Duration:       15 * time.Minute,
		TotalQuestions: 14,
		TotalCorrect:   11,
		Accuracy:       float64(11) / float64(14),
		SkillResults: []session.SkillResult{
			{
				SkillID:    "test-skill-1",
				SkillName:  "Add 3-Digit Numbers",
				Category:   session.CategoryFrontier,
				Attempted:  6,
				Correct:    5,
				TierBefore: skillgraph.TierLearn,
				TierAfter:  skillgraph.TierProve,
			},
			{
				SkillID:    "test-skill-2",
				SkillName:  "Subtract 3-Digit Numbers",
				Category:   session.CategoryFrontier,
				Attempted:  3,
				Correct:    3,
				TierBefore: skillgraph.TierLearn,
				TierAfter:  skillgraph.TierLearn,
			},
		},
	}
}

func TestSummaryScreen_Title(t *testing.T) {
	s := New(testSummary())
	if s.Title() != "Session Summary" {
		t.Errorf("Title = %q, want %q", s.Title(), "Session Summary")
	}
}

func TestSummaryScreen_Display(t *testing.T) {
	s := New(testSummary())
	view := s.View(80, 24)
	if view == "" {
		t.Error("expected non-empty summary view")
	}
}

func TestSummaryScreen_Navigation_Enter(t *testing.T) {
	s := New(testSummary())
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Error("expected a command on Enter (pop)")
	}
}

func TestSummaryScreen_Navigation_Esc(t *testing.T) {
	s := New(testSummary())
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Error("expected a command on Esc (pop)")
	}
}

func TestSummaryScreen_KeyHints(t *testing.T) {
	s := New(testSummary())
	hints := s.KeyHints()
	if len(hints) != 2 {
		t.Errorf("KeyHints length = %d, want 2", len(hints))
	}
}
