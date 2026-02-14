package problemgen

import "testing"

func TestMathCheck_Addition(t *testing.T) {
	v := &MathCheckValidator{}

	q := validQuestion()
	q.Text = "What is 345 + 278?"
	q.Answer = "623"
	q.AnswerType = AnswerTypeInteger
	if err := v.Validate(q, GenerateInput{}); err != nil {
		t.Fatalf("correct addition should pass: %v", err)
	}

	q.Answer = "612"
	if err := v.Validate(q, GenerateInput{}); err == nil {
		t.Fatal("wrong addition should fail")
	}
}

func TestMathCheck_Subtraction(t *testing.T) {
	v := &MathCheckValidator{}

	q := validQuestion()
	q.Text = "567 - 289 = ?"
	q.Answer = "278"
	q.AnswerType = AnswerTypeInteger
	if err := v.Validate(q, GenerateInput{}); err != nil {
		t.Fatalf("correct subtraction should pass: %v", err)
	}

	q.Answer = "288"
	if err := v.Validate(q, GenerateInput{}); err == nil {
		t.Fatal("wrong subtraction should fail")
	}
}

func TestMathCheck_Multiplication(t *testing.T) {
	v := &MathCheckValidator{}

	q := validQuestion()
	q.Text = "What is 23 * 45?"
	q.Answer = "1035"
	q.AnswerType = AnswerTypeInteger
	if err := v.Validate(q, GenerateInput{}); err != nil {
		t.Fatalf("correct multiplication should pass: %v", err)
	}

	q.Answer = "1025"
	if err := v.Validate(q, GenerateInput{}); err == nil {
		t.Fatal("wrong multiplication should fail")
	}
}

func TestMathCheck_Division(t *testing.T) {
	v := &MathCheckValidator{}

	q := validQuestion()
	q.Text = "What is 144 / 12?"
	q.Answer = "12"
	q.AnswerType = AnswerTypeInteger
	if err := v.Validate(q, GenerateInput{}); err != nil {
		t.Fatalf("correct division should pass: %v", err)
	}

	q.Answer = "11"
	if err := v.Validate(q, GenerateInput{}); err == nil {
		t.Fatal("wrong division should fail")
	}
}

func TestMathCheck_FractionArithmetic(t *testing.T) {
	v := &MathCheckValidator{}

	tests := []struct {
		text       string
		answer     string
		answerType AnswerType
	}{
		{"What is 1/4 + 1/2?", "3/4", AnswerTypeFraction},
		{"What is 3/4 - 1/3?", "5/12", AnswerTypeFraction},
		{"What is 2/3 * 3/4?", "1/2", AnswerTypeFraction},
		{"What is 1/2 ÷ 1/4?", "2", AnswerTypeInteger},
	}

	for _, tc := range tests {
		q := validQuestion()
		q.Text = tc.text
		q.Answer = tc.answer
		q.AnswerType = tc.answerType
		if err := v.Validate(q, GenerateInput{}); err != nil {
			t.Errorf("expected %q with answer %q to pass: %v", tc.text, tc.answer, err)
		}
	}
}

func TestMathCheck_FractionWrongAnswer(t *testing.T) {
	v := &MathCheckValidator{}

	q := validQuestion()
	q.Text = "What is 1/4 + 1/2?"
	q.Answer = "2/6"
	q.AnswerType = AnswerTypeFraction
	if err := v.Validate(q, GenerateInput{}); err == nil {
		t.Fatal("wrong fraction answer should fail")
	}
}

func TestMathCheck_NonComputable(t *testing.T) {
	v := &MathCheckValidator{}

	texts := []string{
		"Which fraction is larger: 3/4 or 2/3?",
		"A farmer has 345 apples and gives away 123. How many are left?",
		"What place value does 5 have in 5,432?",
	}

	for _, text := range texts {
		q := validQuestion()
		q.Text = text
		if err := v.Validate(q, GenerateInput{}); err != nil {
			t.Errorf("non-computable %q should pass silently: %v", text, err)
		}
	}
}

func TestMathCheck_LargeNumbers(t *testing.T) {
	v := &MathCheckValidator{}

	tests := []struct {
		text   string
		answer string
	}{
		{"What is 12345 + 67890?", "80235"},
		{"What is 456 * 789?", "359784"},
	}

	for _, tc := range tests {
		q := validQuestion()
		q.Text = tc.text
		q.Answer = tc.answer
		q.AnswerType = AnswerTypeInteger
		if err := v.Validate(q, GenerateInput{}); err != nil {
			t.Errorf("expected %q with answer %q to pass: %v", tc.text, tc.answer, err)
		}
	}
}
