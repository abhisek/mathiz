package problemgen

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var fractionPattern = regexp.MustCompile(`^-?\d+/\d+$`)

// AnswerFormatValidator checks that the answer string matches the declared
// answer_type (integer, decimal, fraction) and that multiple choice
// constraints are satisfied.
type AnswerFormatValidator struct{}

func (v *AnswerFormatValidator) Name() string { return "answer-format" }

func (v *AnswerFormatValidator) Validate(q *Question, _ GenerateInput) *ValidationError {
	// Validate answer matches declared type.
	switch q.AnswerType {
	case AnswerTypeInteger:
		if err := validateInteger(q.Answer); err != nil {
			return &ValidationError{
				Validator: v.Name(),
				Message:   fmt.Sprintf("invalid integer answer %q: %s", q.Answer, err),
				Retryable: true,
			}
		}
	case AnswerTypeDecimal:
		if err := validateDecimal(q.Answer); err != nil {
			return &ValidationError{
				Validator: v.Name(),
				Message:   fmt.Sprintf("invalid decimal answer %q: %s", q.Answer, err),
				Retryable: true,
			}
		}
	case AnswerTypeFraction:
		if err := validateFraction(q.Answer); err != nil {
			return &ValidationError{
				Validator: v.Name(),
				Message:   fmt.Sprintf("invalid fraction answer %q: %s", q.Answer, err),
				Retryable: true,
			}
		}
	}

	// Validate multiple choice constraints.
	if q.Format == FormatMultipleChoice {
		if len(q.Choices) != 4 {
			return &ValidationError{
				Validator: v.Name(),
				Message:   fmt.Sprintf("multiple choice must have exactly 4 choices, got %d", len(q.Choices)),
				Retryable: true,
			}
		}
		// All choices must be non-empty and distinct.
		seen := make(map[string]bool, 4)
		for i, c := range q.Choices {
			c = strings.TrimSpace(c)
			if c == "" {
				return &ValidationError{
					Validator: v.Name(),
					Message:   fmt.Sprintf("choice %d is empty", i+1),
					Retryable: true,
				}
			}
			key := strings.ToLower(c)
			if seen[key] {
				return &ValidationError{
					Validator: v.Name(),
					Message:   fmt.Sprintf("duplicate choice %q", c),
					Retryable: true,
				}
			}
			seen[key] = true
		}
		// Exactly one choice must match the answer.
		answerLower := strings.ToLower(strings.TrimSpace(q.Answer))
		found := false
		for _, c := range q.Choices {
			if strings.ToLower(strings.TrimSpace(c)) == answerLower {
				found = true
				break
			}
		}
		if !found {
			return &ValidationError{
				Validator: v.Name(),
				Message:   fmt.Sprintf("answer %q not found in choices", q.Answer),
				Retryable: true,
			}
		}
	}

	// Numeric format must have no choices.
	if q.Format == FormatNumeric && len(q.Choices) > 0 {
		return &ValidationError{
			Validator: v.Name(),
			Message:   "numeric format must have empty choices",
			Retryable: true,
		}
	}

	return nil
}

// validateInteger checks that s is a valid integer string with no leading zeros.
func validateInteger(s string) error {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("not a valid integer")
	}
	// Check for leading zeros: formatted back should match.
	if strconv.FormatInt(n, 10) != s {
		return fmt.Errorf("has leading zeros")
	}
	return nil
}

// validateDecimal checks that s is a valid decimal string with no trailing zeros.
func validateDecimal(s string) error {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("not a valid decimal")
	}
	// Check for trailing zeros after decimal point.
	normalized := strconv.FormatFloat(f, 'f', -1, 64)
	if normalized != s {
		return fmt.Errorf("has trailing zeros or is not normalized (expected %q)", normalized)
	}
	return nil
}

// validateFraction checks that s matches a/b pattern, denominator > 0, and is in lowest terms.
func validateFraction(s string) error {
	if !fractionPattern.MatchString(s) {
		return fmt.Errorf("does not match fraction pattern a/b")
	}
	num, den, err := parseFraction(s)
	if err != nil {
		return err
	}
	if den <= 0 {
		return fmt.Errorf("denominator must be positive")
	}
	if gcd(abs(num), den) != 1 {
		return fmt.Errorf("fraction is not in lowest terms")
	}
	return nil
}
