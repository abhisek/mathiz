package lessons

import (
	"encoding/json"
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
