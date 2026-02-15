package diagnosis

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/skillgraph"
)

func TestDiagnoser_MatchesMisconception(t *testing.T) {
	resp := json.RawMessage(`{"misconception_id":"add-no-carry","confidence":0.92,"reasoning":"Added columns without carrying"}`)
	mock := llm.NewMockProvider(llm.MockResponse{Content: resp})
	d := NewDiagnoser(mock, DefaultDiagnoserConfig())

	candidates := MisconceptionsByStrand(skillgraph.StrandAddSub)
	req := &DiagnosisRequest{
		SkillID:       "test-skill",
		SkillName:     "3-Digit Addition",
		QuestionText:  "What is 47 + 38?",
		CorrectAnswer: "85",
		LearnerAnswer: "715",
		AnswerType:    "integer",
		Candidates:    candidates,
	}

	result, err := d.Diagnose(context.Background(), req)
	if err != nil {
		t.Fatalf("Diagnose failed: %v", err)
	}
	if result.Category != CategoryMisconception {
		t.Errorf("category = %q, want %q", result.Category, CategoryMisconception)
	}
	if result.MisconceptionID != "add-no-carry" {
		t.Errorf("misconception_id = %q, want add-no-carry", result.MisconceptionID)
	}
	if result.Confidence != 0.92 {
		t.Errorf("confidence = %f, want 0.92", result.Confidence)
	}
}

func TestDiagnoser_NullMisconception(t *testing.T) {
	resp := json.RawMessage(`{"misconception_id":null,"confidence":0.3,"reasoning":"No clear pattern"}`)
	mock := llm.NewMockProvider(llm.MockResponse{Content: resp})
	d := NewDiagnoser(mock, DefaultDiagnoserConfig())

	candidates := MisconceptionsByStrand(skillgraph.StrandAddSub)
	req := &DiagnosisRequest{
		SkillID:       "test-skill",
		SkillName:     "3-Digit Addition",
		QuestionText:  "What is 1 + 1?",
		CorrectAnswer: "2",
		LearnerAnswer: "5",
		AnswerType:    "integer",
		Candidates:    candidates,
	}

	result, err := d.Diagnose(context.Background(), req)
	if err != nil {
		t.Fatalf("Diagnose failed: %v", err)
	}
	if result.Category != CategoryUnclassified {
		t.Errorf("category = %q, want %q", result.Category, CategoryUnclassified)
	}
	if result.MisconceptionID != "" {
		t.Errorf("misconception_id = %q, want empty", result.MisconceptionID)
	}
}

func TestDiagnoser_InvalidIDRejected(t *testing.T) {
	resp := json.RawMessage(`{"misconception_id":"fake-id","confidence":0.9,"reasoning":"test"}`)
	mock := llm.NewMockProvider(llm.MockResponse{Content: resp})
	d := NewDiagnoser(mock, DefaultDiagnoserConfig())

	candidates := MisconceptionsByStrand(skillgraph.StrandAddSub)
	req := &DiagnosisRequest{
		SkillID:       "test-skill",
		SkillName:     "Addition",
		QuestionText:  "1 + 1?",
		CorrectAnswer: "2",
		LearnerAnswer: "3",
		AnswerType:    "integer",
		Candidates:    candidates,
	}

	result, err := d.Diagnose(context.Background(), req)
	if err != nil {
		t.Fatalf("Diagnose failed: %v", err)
	}
	if result.Category != CategoryUnclassified {
		t.Errorf("category = %q, want %q (invalid ID should be rejected)", result.Category, CategoryUnclassified)
	}
}

func TestDiagnoser_LLMError(t *testing.T) {
	mock := llm.NewMockProvider() // Empty queue → ErrProviderUnavailable
	d := NewDiagnoser(mock, DefaultDiagnoserConfig())

	candidates := MisconceptionsByStrand(skillgraph.StrandAddSub)
	req := &DiagnosisRequest{
		SkillID:       "test-skill",
		SkillName:     "Addition",
		QuestionText:  "1 + 1?",
		CorrectAnswer: "2",
		LearnerAnswer: "3",
		AnswerType:    "integer",
		Candidates:    candidates,
	}

	_, err := d.Diagnose(context.Background(), req)
	if err == nil {
		t.Error("expected error from empty mock provider")
	}
}

func TestBuildDiagnosisMessage(t *testing.T) {
	candidates := MisconceptionsByStrand(skillgraph.StrandAddSub)
	req := &DiagnosisRequest{
		SkillID:       "test",
		SkillName:     "Addition",
		QuestionText:  "1 + 1?",
		CorrectAnswer: "2",
		LearnerAnswer: "3",
		AnswerType:    "integer",
		Candidates:    candidates,
	}

	msg, err := buildDiagnosisMessage(req)
	if err != nil {
		t.Fatalf("buildDiagnosisMessage failed: %v", err)
	}
	if msg == "" {
		t.Error("message is empty")
	}
	// Should contain skill name and question.
	if !contains(msg, "Addition") {
		t.Error("message should contain skill name")
	}
	if !contains(msg, "1 + 1?") {
		t.Error("message should contain question text")
	}
	// Should contain candidate misconception IDs.
	if !contains(msg, "add-no-carry") {
		t.Error("message should contain candidate misconception IDs")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
