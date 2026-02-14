package problemgen

import (
	"fmt"
	"strconv"
	"strings"
)

// CheckAnswer compares the learner's input against the correct answer.
// Returns true if the answer is correct.
//
// Normalization rules:
// - Whitespace is trimmed
// - Comparison is case-insensitive
// - For fractions: equivalent fractions are accepted (e.g., "2/4" matches "1/2")
// - For decimals: trailing zeros are ignored (e.g., "3.50" matches "3.5")
// - For integers: leading zeros are ignored (e.g., "007" matches "7")
// - For multiple choice: matches against the choice text or index (1-4)
func CheckAnswer(learnerAnswer string, question *Question) bool {
	learnerAnswer = strings.TrimSpace(learnerAnswer)
	if learnerAnswer == "" {
		return false
	}

	if question.Format == FormatMultipleChoice {
		return checkMultipleChoice(learnerAnswer, question)
	}

	normalizedLearner, err := normalizeAnswer(learnerAnswer, question.AnswerType)
	if err != nil {
		return false
	}
	normalizedCorrect, err := normalizeAnswer(question.Answer, question.AnswerType)
	if err != nil {
		return false
	}
	return normalizedLearner == normalizedCorrect
}

// checkMultipleChoice checks the learner's answer against MC choices.
func checkMultipleChoice(learnerAnswer string, question *Question) bool {
	// Try matching by index (1-4).
	if idx, err := strconv.Atoi(learnerAnswer); err == nil && idx >= 1 && idx <= len(question.Choices) {
		return strings.EqualFold(
			strings.TrimSpace(question.Choices[idx-1]),
			strings.TrimSpace(question.Answer),
		)
	}

	// Match by text (case-insensitive).
	return strings.EqualFold(
		strings.TrimSpace(learnerAnswer),
		strings.TrimSpace(question.Answer),
	)
}

// normalizeAnswer normalizes an answer string for comparison.
func normalizeAnswer(answer string, answerType AnswerType) (string, error) {
	answer = strings.TrimSpace(answer)

	switch answerType {
	case AnswerTypeInteger:
		n, err := strconv.ParseInt(answer, 10, 64)
		if err != nil {
			return "", fmt.Errorf("invalid integer: %w", err)
		}
		return strconv.FormatInt(n, 10), nil

	case AnswerTypeDecimal:
		f, err := strconv.ParseFloat(answer, 64)
		if err != nil {
			return "", fmt.Errorf("invalid decimal: %w", err)
		}
		return strconv.FormatFloat(f, 'f', -1, 64), nil

	case AnswerTypeFraction:
		num, den, err := parseFraction(answer)
		if err != nil {
			return "", err
		}
		if den == 0 {
			return "", fmt.Errorf("zero denominator")
		}
		// Normalize sign: negative sign on numerator only.
		if den < 0 {
			num = -num
			den = -den
		}
		// Reduce to lowest terms.
		g := gcd(abs(num), den)
		num /= g
		den /= g
		return fmt.Sprintf("%d/%d", num, den), nil

	default:
		return answer, nil
	}
}

// parseFraction parses "a/b" into numerator and denominator.
func parseFraction(s string) (int64, int64, error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid fraction format: %q", s)
	}
	num, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid numerator: %w", err)
	}
	den, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid denominator: %w", err)
	}
	return num, den, nil
}

// gcd returns the greatest common divisor of a and b.
// Both a and b must be non-negative.
func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// abs returns the absolute value of n.
func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
