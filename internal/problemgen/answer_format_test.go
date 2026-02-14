package problemgen

import "testing"

func TestAnswerFormat_Integer(t *testing.T) {
	v := &AnswerFormatValidator{}

	valid := []string{"42", "0", "-5", "1000"}
	for _, a := range valid {
		q := validQuestion()
		q.AnswerType = AnswerTypeInteger
		q.Answer = a
		if err := v.Validate(q, GenerateInput{}); err != nil {
			t.Errorf("expected %q to be valid integer, got: %v", a, err)
		}
	}

	invalid := []string{"3.5", "abc", "3/4", "007", ""}
	for _, a := range invalid {
		q := validQuestion()
		q.AnswerType = AnswerTypeInteger
		q.Answer = a
		if err := v.Validate(q, GenerateInput{}); err == nil {
			t.Errorf("expected %q to be invalid integer", a)
		}
	}
}

func TestAnswerFormat_Decimal(t *testing.T) {
	v := &AnswerFormatValidator{}

	valid := []string{"3.5", "0.75", "-2.1", "0", "100"}
	for _, a := range valid {
		q := validQuestion()
		q.AnswerType = AnswerTypeDecimal
		q.Answer = a
		if err := v.Validate(q, GenerateInput{}); err != nil {
			t.Errorf("expected %q to be valid decimal, got: %v", a, err)
		}
	}

	invalid := []string{"abc", "3.50"}
	for _, a := range invalid {
		q := validQuestion()
		q.AnswerType = AnswerTypeDecimal
		q.Answer = a
		if err := v.Validate(q, GenerateInput{}); err == nil {
			t.Errorf("expected %q to be invalid decimal", a)
		}
	}
}

func TestAnswerFormat_Fraction(t *testing.T) {
	v := &AnswerFormatValidator{}

	valid := []string{"3/4", "1/2", "-7/3", "1/1"}
	for _, a := range valid {
		q := validQuestion()
		q.AnswerType = AnswerTypeFraction
		q.Answer = a
		if err := v.Validate(q, GenerateInput{}); err != nil {
			t.Errorf("expected %q to be valid fraction, got: %v", a, err)
		}
	}

	invalid := []string{"3/0", "2/4", "abc", "3.5", ""}
	for _, a := range invalid {
		q := validQuestion()
		q.AnswerType = AnswerTypeFraction
		q.Answer = a
		if err := v.Validate(q, GenerateInput{}); err == nil {
			t.Errorf("expected %q to be invalid fraction", a)
		}
	}
}

func TestAnswerFormat_MultipleChoice(t *testing.T) {
	v := &AnswerFormatValidator{}

	// Valid MC question.
	q := validQuestion()
	q.Format = FormatMultipleChoice
	q.Answer = "623"
	q.Choices = []string{"612", "623", "633", "652"}
	if err := v.Validate(q, GenerateInput{}); err != nil {
		t.Fatalf("expected valid MC, got: %v", err)
	}

	// Too few choices.
	q2 := validQuestion()
	q2.Format = FormatMultipleChoice
	q2.Answer = "623"
	q2.Choices = []string{"612", "623", "633"}
	if err := v.Validate(q2, GenerateInput{}); err == nil {
		t.Error("expected error for 3 choices")
	}

	// Duplicate choices.
	q3 := validQuestion()
	q3.Format = FormatMultipleChoice
	q3.Answer = "623"
	q3.Choices = []string{"612", "623", "623", "652"}
	if err := v.Validate(q3, GenerateInput{}); err == nil {
		t.Error("expected error for duplicate choices")
	}

	// Answer not in choices.
	q4 := validQuestion()
	q4.Format = FormatMultipleChoice
	q4.Answer = "999"
	q4.Choices = []string{"612", "623", "633", "652"}
	if err := v.Validate(q4, GenerateInput{}); err == nil {
		t.Error("expected error for answer not in choices")
	}
}

func TestAnswerFormat_NumericWithChoices(t *testing.T) {
	v := &AnswerFormatValidator{}
	q := validQuestion()
	q.Format = FormatNumeric
	q.Choices = []string{"1", "2", "3", "4"}
	if err := v.Validate(q, GenerateInput{}); err == nil {
		t.Error("expected error for numeric format with choices")
	}
}
