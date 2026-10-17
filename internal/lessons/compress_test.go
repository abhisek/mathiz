package lessons

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/abhisek/mathiz/internal/llm"
)

func TestCompressor_SessionCompression(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{
		Content: json.RawMessage(`{"summary": "Student consistently forgets to carry in addition"}`),
	})
	comp := NewCompressor(mock, DefaultCompressorConfig())

	errors := []string{
		"Answered 715 for '47 + 38', correct was 85",
		"Answered 842 for '567 + 285', correct was 852",
		"Answered 611 for '345 + 278', correct was 623",
	}

	done := make(chan string, 1)
	comp.CompressErrors(t.Context(), "add-3digit", errors, func(skillID, summary string) {
		done <- summary
	})

	select {
	case summary := <-done:
		if summary == "" {
			t.Error("expected non-empty summary")
		}
		if summary != "Student consistently forgets to carry in addition" {
			t.Errorf("unexpected summary: %q", summary)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("compression did not complete in time")
	}

	if mock.CallCount() != 1 {
		t.Fatalf("expected 1 LLM call, got %d", mock.CallCount())
	}

	req := mock.Calls[0]
	if req.Schema == nil || req.Schema.Name != "error-summary" {
		t.Error("expected schema name 'error-summary'")
	}
}

func TestCompressor_ProfileGeneration(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{
		Content: json.RawMessage(`{
			"summary": "Student has strong multiplication facts but struggles with fractions.",
			"strengths": ["solid multiplication facts", "good addition accuracy"],
			"weaknesses": ["fraction addition", "carrying in complex addition"],
			"patterns": ["adds fraction numerators and denominators separately"]
		}`),
	})
	comp := NewCompressor(mock, DefaultCompressorConfig())

	input := ProfileInput{
		PerSkillResults: map[string]SkillResultSummary{
			"mul-facts": {Attempted: 5, Correct: 5},
			"frac-add":  {Attempted: 6, Correct: 2},
		},
		MasteryData: map[string]MasteryDataSummary{
			"mul-facts": {State: "mastered", FluencyScore: 0.9},
			"frac-add":  {State: "learning", FluencyScore: 0.3},
		},
	}

	profile, err := comp.GenerateProfile(t.Context(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if profile.Summary == "" {
		t.Error("expected non-empty summary")
	}
	if len(profile.Strengths) != 2 {
		t.Errorf("expected 2 strengths, got %d", len(profile.Strengths))
	}
	if len(profile.Weaknesses) != 2 {
		t.Errorf("expected 2 weaknesses, got %d", len(profile.Weaknesses))
	}
	if len(profile.Patterns) != 1 {
		t.Errorf("expected 1 pattern, got %d", len(profile.Patterns))
	}
	if profile.GeneratedAt.IsZero() {
		t.Error("expected non-zero GeneratedAt")
	}
}

func TestCompressor_ProfileWithPrevious(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{
		Content: json.RawMessage(`{
			"summary": "Updated profile.",
			"strengths": ["s1"],
			"weaknesses": ["w1"],
			"patterns": ["p1"]
		}`),
	})
	comp := NewCompressor(mock, DefaultCompressorConfig())

	input := ProfileInput{
		PerSkillResults: map[string]SkillResultSummary{
			"add-3digit": {Attempted: 10, Correct: 8},
		},
		PreviousProfile: &LearnerProfile{
			Summary:    "Previous summary",
			Strengths:  []string{"old strength"},
			Weaknesses: []string{"old weakness"},
		},
	}

	profile, err := comp.GenerateProfile(t.Context(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile == nil {
		t.Fatal("expected non-nil profile")
	}

	// Verify the prompt included previous profile.
	if mock.CallCount() != 1 {
		t.Fatalf("expected 1 call, got %d", mock.CallCount())
	}
	req := mock.Calls[0]
	if len(req.Messages) == 0 {
		t.Fatal("expected messages")
	}
	userMsg := req.Messages[0].Content
	if !contains(userMsg, "Previous Profile") {
		t.Error("expected prompt to include 'Previous Profile'")
	}
	if !contains(userMsg, "Previous summary") {
		t.Error("expected prompt to include previous summary")
	}
}

func TestCompressor_ProfileFirstSession(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{
		Content: json.RawMessage(`{
			"summary": "First session profile.",
			"strengths": ["s1"],
			"weaknesses": ["w1"],
			"patterns": ["p1"]
		}`),
	})
	comp := NewCompressor(mock, DefaultCompressorConfig())

	input := ProfileInput{
		PerSkillResults: map[string]SkillResultSummary{
			"add-3digit": {Attempted: 5, Correct: 4},
		},
		PreviousProfile: nil,
	}

	profile, err := comp.GenerateProfile(t.Context(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile == nil {
		t.Fatal("expected non-nil profile")
	}

	// Verify prompt does NOT include previous profile.
	req := mock.Calls[0]
	userMsg := req.Messages[0].Content
	if contains(userMsg, "Previous Profile") {
		t.Error("did not expect 'Previous Profile' in first session prompt")
	}
}

func TestCompressor_LLMError(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{
		Err: &llm.ErrProviderUnavailable{},
	})
	comp := NewCompressor(mock, DefaultCompressorConfig())

	_, err := comp.GenerateProfile(t.Context(), ProfileInput{})
	if err == nil {
		t.Error("expected error on LLM failure")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestProfilePromptRendersHistory(t *testing.T) {
	input := ProfileInput{
		SessionCount: 12,
		MasteryData: map[string]MasteryDataSummary{
			"mul-facts": {Name: "Multiplication facts", Grade: 3, State: "mastered", Tier: "prove", FluencyScore: 0.9, Attempts: 40, Correct: 38},
			"frac-add":  {Name: "Adding fractions", Grade: 4, State: "learning", Tier: "learn", Attempts: 10, Correct: 4},
			"div-long":  {Name: "Long division", Grade: 5, State: "new"},
		},
		RecentSessions: []SessionSummary{
			{EndedAt: time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC), Skills: []string{"Adding fractions"}, Questions: 5, Correct: 2, Quest: true},
		},
		ErrorPatterns: []ErrorPatternSummary{
			{SkillID: "frac-add", SkillName: "Adding fractions", Category: "misconception", Misconception: "Adds denominators", Count: 4},
			{SkillID: "frac-add", SkillName: "Adding fractions", Category: "careless", Count: 1},
		},
		PerSkillResults: map[string]SkillResultSummary{
			"frac-add": {Name: "Adding fractions", Attempted: 5, Correct: 2},
		},
		ErrorHistory: map[string][]string{"frac-add": {"e1", "e2", "e3", "e4", "e5"}},
		PreviousProfile: &LearnerProfile{
			Summary: "Old summary", GeneratedAt: time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC),
		},
	}
	prompt := buildProfileUserMessage(input)

	for _, want := range []string{
		"Sessions completed: 12",
		"Skills mastered: 1, learning: 1, rusty: 0",
		"- Multiplication facts (mul-facts): grade 3, state=mastered, tier=prove, 38/40 correct (95%), fluency=0.90",
		"- 2026-10-03 (quest): Adding fractions, 2/5 correct",
		"- Adding fractions (frac-add): misconception: Adds denominators (4 times)",
		"- Adding fractions (frac-add): careless (1 times)",
		"- Adding fractions (frac-add): 5 attempted, 2 correct (40%)",
		"Previous Profile (from 2026-09-20)",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "Long division") {
		t.Error("prompt lists a skill the student never attempted")
	}
	// Only the newest session errors are kept.
	if strings.Contains(prompt, "error: e2") || !strings.Contains(prompt, "error: e3") || !strings.Contains(prompt, "error: e5") {
		t.Errorf("session errors not capped to the newest %d:\n%s", maxSessionErrorsPerSkill, prompt)
	}
	// Map order is random, so the skill lists are sorted.
	if prompt != buildProfileUserMessage(input) {
		t.Error("the same input rendered two different prompts")
	}
}
