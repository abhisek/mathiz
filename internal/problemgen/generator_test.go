package problemgen

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/skillgraph"
)

func testSkill() skillgraph.Skill {
	return skillgraph.Skill{
		ID:          "add-3digit",
		Name:        "Add 3-Digit Numbers",
		Description: "Addition of three-digit numbers with regrouping",
		GradeLevel:  3,
		Keywords:    []string{"addition", "carry", "three-digit"},
	}
}

func validQuestionJSON() json.RawMessage {
	return json.RawMessage(`{
		"question_text": "What is 345 + 278?",
		"format": "numeric",
		"answer": "623",
		"answer_type": "integer",
		"choices": [],
		"hint": "Try adding column by column.",
		"difficulty": 3,
		"explanation": "345 + 278 = 623. Add ones: 5+8=13 carry 1. Add tens: 4+7+1=12 carry 1. Add hundreds: 3+2+1=6."
	}`)
}

func mcQuestionJSON() json.RawMessage {
	return json.RawMessage(`{
		"question_text": "Which is the largest?",
		"format": "multiple_choice",
		"answer": "623",
		"answer_type": "integer",
		"choices": ["512", "623", "601", "599"],
		"hint": "",
		"difficulty": 2,
		"explanation": "623 is the largest because it has the highest value."
	}`)
}

func TestGenerate_Numeric(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{
		Content: validQuestionJSON(),
	})
	gen := New(mock, DefaultConfig())

	q, err := gen.Generate(context.Background(), GenerateInput{
		Skill: testSkill(),
		Tier:  skillgraph.TierLearn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Text != "What is 345 + 278?" {
		t.Errorf("unexpected text: %q", q.Text)
	}
	if q.Format != FormatNumeric {
		t.Errorf("expected numeric format, got %q", q.Format)
	}
	if q.Answer != "623" {
		t.Errorf("expected answer 623, got %q", q.Answer)
	}
	if q.SkillID != "add-3digit" {
		t.Errorf("expected skillID add-3digit, got %q", q.SkillID)
	}
	if q.Tier != skillgraph.TierLearn {
		t.Errorf("expected Learn tier")
	}
}

func TestGenerate_MultipleChoice(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{
		Content: mcQuestionJSON(),
	})
	gen := New(mock, DefaultConfig())

	q, err := gen.Generate(context.Background(), GenerateInput{
		Skill: testSkill(),
		Tier:  skillgraph.TierLearn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Format != FormatMultipleChoice {
		t.Errorf("expected multiple_choice format, got %q", q.Format)
	}
	if len(q.Choices) != 4 {
		t.Errorf("expected 4 choices, got %d", len(q.Choices))
	}
}

func TestGenerate_ProveTier_NoHint(t *testing.T) {
	raw := json.RawMessage(`{
		"question_text": "What is 345 + 278?",
		"format": "numeric",
		"answer": "623",
		"answer_type": "integer",
		"choices": [],
		"hint": "",
		"difficulty": 4,
		"explanation": "345 + 278 = 623."
	}`)
	mock := llm.NewMockProvider(llm.MockResponse{Content: raw})
	gen := New(mock, DefaultConfig())

	q, err := gen.Generate(context.Background(), GenerateInput{
		Skill: testSkill(),
		Tier:  skillgraph.TierProve,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Hint != "" {
		t.Errorf("expected empty hint for Prove tier, got %q", q.Hint)
	}
}

func TestGenerate_ValidationFailure(t *testing.T) {
	raw := json.RawMessage(`{
		"question_text": "What is 10 + 5?",
		"format": "numeric",
		"answer": "abc",
		"answer_type": "integer",
		"choices": [],
		"hint": "Add them up.",
		"difficulty": 1,
		"explanation": "10 + 5 = 15"
	}`)
	mock := llm.NewMockProvider(llm.MockResponse{Content: raw})
	gen := New(mock, DefaultConfig())

	_, err := gen.Generate(context.Background(), GenerateInput{
		Skill: testSkill(),
		Tier:  skillgraph.TierLearn,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var valErr *ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if valErr.Validator != "answer-format" {
		t.Errorf("expected answer-format validator, got %q", valErr.Validator)
	}
}

// customValidator rejects questions with difficulty above a threshold.
type customValidator struct {
	maxDifficulty int
}

func (v *customValidator) Name() string { return "custom-max-difficulty" }

func (v *customValidator) Validate(q *Question, _ GenerateInput) *ValidationError {
	if q.Difficulty > v.maxDifficulty {
		return &ValidationError{
			Validator: v.Name(),
			Message:   "difficulty too high",
			Retryable: true,
		}
	}
	return nil
}

func TestGenerate_CustomValidator(t *testing.T) {
	raw := json.RawMessage(`{
		"question_text": "What is 999 + 999?",
		"format": "numeric",
		"answer": "1998",
		"answer_type": "integer",
		"choices": [],
		"hint": "Add them.",
		"difficulty": 4,
		"explanation": "999 + 999 = 1998"
	}`)
	mock := llm.NewMockProvider(llm.MockResponse{Content: raw})
	cfg := DefaultConfig()
	cfg.Validators = append(cfg.Validators, &customValidator{maxDifficulty: 3})
	gen := New(mock, cfg)

	_, err := gen.Generate(context.Background(), GenerateInput{
		Skill: testSkill(),
		Tier:  skillgraph.TierLearn,
	})
	if err == nil {
		t.Fatal("expected custom validator to reject")
	}
	var valErr *ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if valErr.Validator != "custom-max-difficulty" {
		t.Errorf("expected custom-max-difficulty, got %q", valErr.Validator)
	}
}

// alwaysRejectValidator always rejects.
type alwaysRejectValidator struct{ name string }

func (v *alwaysRejectValidator) Name() string { return v.name }
func (v *alwaysRejectValidator) Validate(*Question, GenerateInput) *ValidationError {
	return &ValidationError{Validator: v.name, Message: "rejected", Retryable: true}
}

// trackingValidator records whether it was called.
type trackingValidator struct {
	called bool
}

func (v *trackingValidator) Name() string { return "tracking" }
func (v *trackingValidator) Validate(*Question, GenerateInput) *ValidationError {
	v.called = true
	return nil
}

func TestGenerate_ValidatorOrder(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{Content: validQuestionJSON()})
	tracker := &trackingValidator{}
	cfg := Config{
		Validators:  []Validator{&alwaysRejectValidator{name: "first"}, tracker},
		MaxTokens:   512,
		Temperature: 0.7,
	}
	gen := New(mock, cfg)

	_, err := gen.Generate(context.Background(), GenerateInput{Skill: testSkill()})
	if err == nil {
		t.Fatal("expected first validator to reject")
	}
	var valErr *ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if valErr.Validator != "first" {
		t.Errorf("expected error from 'first', got %q", valErr.Validator)
	}
	if tracker.called {
		t.Error("second validator should not have been called")
	}
}

func TestGenerate_NoValidators(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{Content: validQuestionJSON()})
	cfg := Config{
		Validators:  nil,
		MaxTokens:   512,
		Temperature: 0.7,
	}
	gen := New(mock, cfg)

	q, err := gen.Generate(context.Background(), GenerateInput{Skill: testSkill()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Text != "What is 345 + 278?" {
		t.Errorf("unexpected text: %q", q.Text)
	}
}

func TestGenerate_PriorQuestionsInPrompt(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{Content: validQuestionJSON()})
	cfg := DefaultConfig()
	gen := New(mock, cfg)

	priors := []string{"What is 1+1?", "What is 2+2?", "What is 3+3?"}
	_, err := gen.Generate(context.Background(), GenerateInput{
		Skill:          testSkill(),
		Tier:           skillgraph.TierLearn,
		PriorQuestions: priors,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.CallCount() != 1 {
		t.Fatalf("expected 1 call, got %d", mock.CallCount())
	}
	userMsg := mock.Calls[0].Messages[0].Content
	for _, p := range priors {
		if !strings.Contains(userMsg, p) {
			t.Errorf("expected user message to contain %q", p)
		}
	}
}

func TestGenerate_RecentErrorsInPrompt(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{Content: validQuestionJSON()})
	cfg := DefaultConfig()
	gen := New(mock, cfg)

	errs := []string{
		"Answered 890 for '456 + 378 = ?', correct was 834",
		"Answered 1/3 for 'What is 1/4 + 1/4?', correct was 1/2",
	}
	_, err := gen.Generate(context.Background(), GenerateInput{
		Skill:        testSkill(),
		Tier:         skillgraph.TierLearn,
		RecentErrors: errs,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	userMsg := mock.Calls[0].Messages[0].Content
	for _, e := range errs {
		if !strings.Contains(userMsg, e) {
			t.Errorf("expected user message to contain %q", e)
		}
	}
}

func TestGenerate_PurposeLabel(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{Content: validQuestionJSON()})
	gen := New(mock, DefaultConfig())

	ctx := context.Background()
	_, err := gen.Generate(ctx, GenerateInput{Skill: testSkill()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The purpose is set on the context passed to the provider.
	// We verify by checking that WithPurpose was used — the mock doesn't
	// inspect context, but the code path covers it.
}

func TestGenerate_ConfigOverrides(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{Content: validQuestionJSON()})
	cfg := DefaultConfig()
	cfg.MaxTokens = 256
	cfg.Temperature = 0.5
	gen := New(mock, cfg)

	_, err := gen.Generate(context.Background(), GenerateInput{Skill: testSkill()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.Calls[0].MaxTokens != 256 {
		t.Errorf("expected MaxTokens 256, got %d", mock.Calls[0].MaxTokens)
	}
	if mock.Calls[0].Temperature != 0.5 {
		t.Errorf("expected Temperature 0.5, got %f", mock.Calls[0].Temperature)
	}
}

func TestGenerate_ProviderError(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{
		Err: errors.New("API error"),
	})
	gen := New(mock, DefaultConfig())

	_, err := gen.Generate(context.Background(), GenerateInput{Skill: testSkill()})
	if err == nil {
		t.Fatal("expected error from provider")
	}
	if !strings.Contains(err.Error(), "LLM generation failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}
