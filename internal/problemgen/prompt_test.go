package problemgen

import (
	"strings"
	"testing"

	"github.com/abhisek/mathiz/internal/skillgraph"
)

func TestBuildUserMessage_MinimalContext(t *testing.T) {
	input := GenerateInput{
		Skill: skillgraph.Skill{
			Name:        "Add 3-Digit Numbers",
			Description: "Addition of three-digit numbers",
			GradeLevel:  3,
			Keywords:    []string{"addition", "carry"},
		},
		Tier: skillgraph.TierLearn,
	}
	cfg := DefaultConfig()
	msg := buildUserMessage(input, cfg)

	if !strings.Contains(msg, "Skill: Add 3-Digit Numbers") {
		t.Error("missing skill name")
	}
	if !strings.Contains(msg, "Grade: 3") {
		t.Error("missing grade")
	}
	if !strings.Contains(msg, "Tier: learn") {
		t.Error("missing tier")
	}
	if !strings.Contains(msg, "Hints allowed: true") {
		t.Error("missing hints allowed")
	}
	if !strings.Contains(msg, "Already asked in this session:\nNone") {
		t.Error("expected 'None' for prior questions")
	}
	if !strings.Contains(msg, "Recent errors by this student:\nNone") {
		t.Error("expected 'None' for errors")
	}
}

func TestBuildUserMessage_ProveTier(t *testing.T) {
	input := GenerateInput{
		Skill: skillgraph.Skill{
			Name:       "Test",
			GradeLevel: 4,
		},
		Tier: skillgraph.TierProve,
	}
	msg := buildUserMessage(input, DefaultConfig())

	if !strings.Contains(msg, "Tier: prove") {
		t.Error("expected prove tier")
	}
	if !strings.Contains(msg, "Hints allowed: false") {
		t.Error("expected hints not allowed for prove")
	}
}

func TestBuildUserMessage_WithHistory(t *testing.T) {
	input := GenerateInput{
		Skill: skillgraph.Skill{
			Name:       "Test",
			GradeLevel: 3,
			Keywords:   []string{"test"},
		},
		Tier:           skillgraph.TierLearn,
		PriorQuestions: []string{"Q1?", "Q2?", "Q3?"},
		RecentErrors:   []string{"Error 1", "Error 2"},
	}
	msg := buildUserMessage(input, DefaultConfig())

	for _, q := range input.PriorQuestions {
		if !strings.Contains(msg, q) {
			t.Errorf("expected message to contain %q", q)
		}
	}
	for _, e := range input.RecentErrors {
		if !strings.Contains(msg, e) {
			t.Errorf("expected message to contain %q", e)
		}
	}
}

func TestBuildUserMessage_TruncatesPriorQuestions(t *testing.T) {
	questions := make([]string, 12)
	for i := range questions {
		questions[i] = "Question " + string(rune('A'+i))
	}

	input := GenerateInput{
		Skill:          skillgraph.Skill{Name: "Test", GradeLevel: 3},
		PriorQuestions: questions,
	}
	cfg := DefaultConfig() // MaxPriorQuestions = 8
	msg := buildUserMessage(input, cfg)

	// First 4 should be dropped (12 - 8 = 4).
	for _, q := range questions[:4] {
		if strings.Contains(msg, q) {
			t.Errorf("expected old question %q to be truncated", q)
		}
	}
	// Last 8 should be present.
	for _, q := range questions[4:] {
		if !strings.Contains(msg, q) {
			t.Errorf("expected recent question %q to be present", q)
		}
	}
}

func TestBuildUserMessage_TruncatesErrors(t *testing.T) {
	errs := make([]string, 8)
	for i := range errs {
		errs[i] = "Error " + string(rune('A'+i))
	}

	input := GenerateInput{
		Skill:        skillgraph.Skill{Name: "Test", GradeLevel: 3},
		RecentErrors: errs,
	}
	cfg := DefaultConfig() // MaxRecentErrors = 5
	msg := buildUserMessage(input, cfg)

	// First 3 should be dropped (8 - 5 = 3).
	for _, e := range errs[:3] {
		if strings.Contains(msg, e) {
			t.Errorf("expected old error %q to be truncated", e)
		}
	}
	// Last 5 should be present.
	for _, e := range errs[3:] {
		if !strings.Contains(msg, e) {
			t.Errorf("expected recent error %q to be present", e)
		}
	}
}

func TestBuildUserMessage_CustomLimits(t *testing.T) {
	questions := []string{"Q1", "Q2", "Q3", "Q4", "Q5"}
	input := GenerateInput{
		Skill:          skillgraph.Skill{Name: "Test", GradeLevel: 3},
		PriorQuestions: questions,
	}
	cfg := DefaultConfig()
	cfg.MaxPriorQuestions = 3
	msg := buildUserMessage(input, cfg)

	// First 2 should be dropped.
	if strings.Contains(msg, "Q1") || strings.Contains(msg, "Q2") {
		t.Error("expected old questions to be truncated with MaxPriorQuestions=3")
	}
	// Last 3 should be present.
	for _, q := range []string{"Q3", "Q4", "Q5"} {
		if !strings.Contains(msg, q) {
			t.Errorf("expected %q to be present", q)
		}
	}
}
