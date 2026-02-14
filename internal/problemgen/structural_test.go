package problemgen

import (
	"strings"
	"testing"
)

func validQuestion() *Question {
	return &Question{
		Text:        "What is 345 + 278?",
		Format:      FormatNumeric,
		Answer:      "623",
		AnswerType:  AnswerTypeInteger,
		Choices:     nil,
		Hint:        "Try adding column by column.",
		Difficulty:  3,
		Explanation: "345 + 278 = 623",
		SkillID:     "add-3digit",
	}
}

func TestStructural_ValidQuestion(t *testing.T) {
	v := &StructuralValidator{}
	err := v.Validate(validQuestion(), GenerateInput{})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestStructural_EmptyQuestionText(t *testing.T) {
	v := &StructuralValidator{}
	q := validQuestion()
	q.Text = ""
	err := v.Validate(q, GenerateInput{})
	if err == nil {
		t.Fatal("expected error for empty question_text")
	}
	if err.Validator != "structural" {
		t.Errorf("expected validator %q, got %q", "structural", err.Validator)
	}
	if !err.Retryable {
		t.Error("expected retryable")
	}
}

func TestStructural_QuestionTextTooLong(t *testing.T) {
	v := &StructuralValidator{}
	q := validQuestion()
	q.Text = strings.Repeat("a", 501)
	err := v.Validate(q, GenerateInput{})
	if err == nil {
		t.Fatal("expected error for long question_text")
	}
}

func TestStructural_EmptyExplanation(t *testing.T) {
	v := &StructuralValidator{}
	q := validQuestion()
	q.Explanation = ""
	err := v.Validate(q, GenerateInput{})
	if err == nil {
		t.Fatal("expected error for empty explanation")
	}
}

func TestStructural_ExplanationTooLong(t *testing.T) {
	v := &StructuralValidator{}
	q := validQuestion()
	q.Explanation = strings.Repeat("a", 1001)
	err := v.Validate(q, GenerateInput{})
	if err == nil {
		t.Fatal("expected error for long explanation")
	}
}

func TestStructural_DifficultyOutOfRange(t *testing.T) {
	v := &StructuralValidator{}

	for _, d := range []int{0, -1, 6, 100} {
		q := validQuestion()
		q.Difficulty = d
		err := v.Validate(q, GenerateInput{})
		if err == nil {
			t.Errorf("expected error for difficulty %d", d)
		}
	}
}

func TestStructural_ValidDifficulty(t *testing.T) {
	v := &StructuralValidator{}
	for _, d := range []int{1, 2, 3, 4, 5} {
		q := validQuestion()
		q.Difficulty = d
		err := v.Validate(q, GenerateInput{})
		if err != nil {
			t.Errorf("unexpected error for difficulty %d: %v", d, err)
		}
	}
}

func TestStructural_UnknownFormat(t *testing.T) {
	v := &StructuralValidator{}
	q := validQuestion()
	q.Format = "essay"
	err := v.Validate(q, GenerateInput{})
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
}

func TestStructural_UnknownAnswerType(t *testing.T) {
	v := &StructuralValidator{}
	q := validQuestion()
	q.AnswerType = "boolean"
	err := v.Validate(q, GenerateInput{})
	if err == nil {
		t.Fatal("expected error for unknown answer_type")
	}
}
