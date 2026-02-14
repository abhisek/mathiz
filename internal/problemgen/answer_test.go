package problemgen

import "testing"

func TestCheckAnswer_Integer(t *testing.T) {
	q := &Question{
		Format:     FormatNumeric,
		Answer:     "42",
		AnswerType: AnswerTypeInteger,
	}

	tests := []struct {
		input string
		want  bool
	}{
		{"42", true},
		{" 42 ", true},
		{"042", true},
		{"43", false},
		{"", false},
		{"abc", false},
	}

	for _, tc := range tests {
		got := CheckAnswer(tc.input, q)
		if got != tc.want {
			t.Errorf("CheckAnswer(%q, 42/integer) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestCheckAnswer_Decimal(t *testing.T) {
	q := &Question{
		Format:     FormatNumeric,
		Answer:     "3.5",
		AnswerType: AnswerTypeDecimal,
	}

	tests := []struct {
		input string
		want  bool
	}{
		{"3.5", true},
		{"3.50", true},
		{"3.500", true},
		{" 3.5 ", true},
		{"3.6", false},
	}

	for _, tc := range tests {
		got := CheckAnswer(tc.input, q)
		if got != tc.want {
			t.Errorf("CheckAnswer(%q, 3.5/decimal) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestCheckAnswer_Fraction(t *testing.T) {
	q := &Question{
		Format:     FormatNumeric,
		Answer:     "1/2",
		AnswerType: AnswerTypeFraction,
	}

	tests := []struct {
		input string
		want  bool
	}{
		{"1/2", true},
		{"2/4", true},
		{"3/6", true},
		{" 1/2 ", true},
		{"1/3", false},
	}

	for _, tc := range tests {
		got := CheckAnswer(tc.input, q)
		if got != tc.want {
			t.Errorf("CheckAnswer(%q, 1/2/fraction) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestCheckAnswer_MultipleChoice_ByIndex(t *testing.T) {
	q := &Question{
		Format:     FormatMultipleChoice,
		Answer:     "3/4",
		AnswerType: AnswerTypeFraction,
		Choices:    []string{"1/2", "3/4", "2/3", "1/4"},
	}

	// Answer is "3/4" which is choices[1], so index "2" should match.
	if !CheckAnswer("2", q) {
		t.Error("expected index 2 to match choice '3/4'")
	}
	// Index 1 is "1/2", not the correct answer.
	if CheckAnswer("1", q) {
		t.Error("expected index 1 not to match")
	}
}

func TestCheckAnswer_MultipleChoice_ByText(t *testing.T) {
	q := &Question{
		Format:     FormatMultipleChoice,
		Answer:     "3/4",
		AnswerType: AnswerTypeFraction,
		Choices:    []string{"1/2", "3/4", "2/3", "1/4"},
	}

	if !CheckAnswer("3/4", q) {
		t.Error("expected text '3/4' to match")
	}
	if CheckAnswer("2/3", q) {
		t.Error("expected text '2/3' not to match")
	}
}

func TestCheckAnswer_MultipleChoice_CaseInsensitive(t *testing.T) {
	q := &Question{
		Format:     FormatMultipleChoice,
		Answer:     "three quarters",
		AnswerType: AnswerTypeFraction,
		Choices:    []string{"one half", "three quarters", "two thirds", "one quarter"},
	}

	if !CheckAnswer("Three Quarters", q) {
		t.Error("expected case-insensitive match")
	}
}
