package problemgen

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// MathCheckValidator attempts to independently recompute the answer from
// the question text. Covers ~70% of MVP skills (pure arithmetic and
// fraction operations). Non-computable questions pass through silently.
type MathCheckValidator struct{}

func (v *MathCheckValidator) Name() string { return "math-check" }

func (v *MathCheckValidator) Validate(q *Question, _ GenerateInput) *ValidationError {
	computed, err := computeAnswer(q.Text, q.AnswerType)
	if err != nil {
		// Question is not computable (word problem, comparison, etc.)
		// This is fine — pass through silently.
		return nil
	}
	if !answersEqual(computed, q.Answer, q.AnswerType) {
		return &ValidationError{
			Validator: v.Name(),
			Message:   fmt.Sprintf("computed %q but LLM claimed %q", computed, q.Answer),
			Retryable: true,
		}
	}
	return nil
}

// Regex patterns for extracting arithmetic expressions from question text.
var (
	// Fraction arithmetic: "a/b + c/d", "a/b - c/d", "a/b * c/d", "a/b ÷ c/d"
	fractionArithRe = regexp.MustCompile(`(-?\d+)\s*/\s*(\d+)\s*([+\-*×÷])\s*(-?\d+)\s*/\s*(\d+)`)

	// Integer/decimal arithmetic with +, -, *, ×
	intArithRe = regexp.MustCompile(`(?:^|[^\d/])(-?\d+(?:\.\d+)?)\s*([+\-*×])\s*(-?\d+(?:\.\d+)?)(?:[^\d/]|$)`)

	// Division requires spaces around the operator to distinguish from fractions (3/4 vs 144 / 12).
	intDivRe = regexp.MustCompile(`(-?\d+(?:\.\d+)?)\s+[/÷]\s+(-?\d+(?:\.\d+)?)`)
)

// computeAnswer attempts to extract and compute the answer from question text.
// Returns the computed answer as a string, or an error if not computable.
func computeAnswer(text string, answerType AnswerType) (string, error) {
	// Try fraction arithmetic first.
	if answerType == AnswerTypeFraction || answerType == AnswerTypeInteger {
		if result, err := tryFractionArith(text); err == nil {
			return result, nil
		}
	}

	// Try integer/decimal arithmetic.
	if answerType == AnswerTypeInteger || answerType == AnswerTypeDecimal {
		if result, err := tryIntArith(text, answerType); err == nil {
			return result, nil
		}
	}

	return "", fmt.Errorf("not computable")
}

// tryFractionArith tries to extract and compute fraction arithmetic.
func tryFractionArith(text string) (string, error) {
	matches := fractionArithRe.FindStringSubmatch(text)
	if matches == nil {
		return "", fmt.Errorf("no fraction expression found")
	}

	aN, _ := strconv.ParseInt(matches[1], 10, 64)
	aD, _ := strconv.ParseInt(matches[2], 10, 64)
	op := normalizeOp(matches[3])
	bN, _ := strconv.ParseInt(matches[4], 10, 64)
	bD, _ := strconv.ParseInt(matches[5], 10, 64)

	if aD == 0 || bD == 0 {
		return "", fmt.Errorf("zero denominator")
	}

	var rN, rD int64
	switch op {
	case "+":
		rN = aN*bD + bN*aD
		rD = aD * bD
	case "-":
		rN = aN*bD - bN*aD
		rD = aD * bD
	case "*":
		rN = aN * bN
		rD = aD * bD
	case "/":
		if bN == 0 {
			return "", fmt.Errorf("division by zero")
		}
		rN = aN * bD
		rD = aD * bN
	default:
		return "", fmt.Errorf("unsupported operator: %s", op)
	}

	// Normalize sign.
	if rD < 0 {
		rN = -rN
		rD = -rD
	}

	// Reduce.
	g := gcd(abs(rN), rD)
	rN /= g
	rD /= g

	// If denominator is 1, return as integer.
	if rD == 1 {
		return strconv.FormatInt(rN, 10), nil
	}

	return fmt.Sprintf("%d/%d", rN, rD), nil
}

// tryIntArith tries to extract and compute integer/decimal arithmetic.
func tryIntArith(text string, answerType AnswerType) (string, error) {
	// Try +, -, *, × first.
	matches := intArithRe.FindStringSubmatch(text)
	if matches != nil {
		return computeIntOp(matches[1], normalizeOp(matches[2]), matches[3], answerType)
	}

	// Try division (requires spaces around operator to avoid matching fractions).
	divMatches := intDivRe.FindStringSubmatch(text)
	if divMatches != nil {
		return computeIntOp(divMatches[1], "/", divMatches[2], answerType)
	}

	return "", fmt.Errorf("no arithmetic expression found")
}

// computeIntOp evaluates a binary arithmetic operation on two number strings.
func computeIntOp(aStr, op, bStr string, answerType AnswerType) (string, error) {
	a, err := strconv.ParseFloat(aStr, 64)
	if err != nil {
		return "", err
	}
	b, err := strconv.ParseFloat(bStr, 64)
	if err != nil {
		return "", err
	}

	var result float64
	switch op {
	case "+":
		result = a + b
	case "-":
		result = a - b
	case "*":
		result = a * b
	case "/":
		if b == 0 {
			return "", fmt.Errorf("division by zero")
		}
		result = a / b
	default:
		return "", fmt.Errorf("unsupported operator: %s", op)
	}

	if answerType == AnswerTypeInteger {
		return strconv.FormatInt(int64(result), 10), nil
	}
	return strconv.FormatFloat(result, 'f', -1, 64), nil
}

// normalizeOp normalizes multiplication and division symbols.
func normalizeOp(op string) string {
	switch op {
	case "×":
		return "*"
	case "÷":
		return "/"
	default:
		return op
	}
}

// answersEqual compares two answer strings for equality, with normalization.
func answersEqual(a, b string, answerType AnswerType) bool {
	na, err := normalizeAnswer(a, answerType)
	if err != nil {
		return strings.TrimSpace(a) == strings.TrimSpace(b)
	}
	nb, err := normalizeAnswer(b, answerType)
	if err != nil {
		return strings.TrimSpace(a) == strings.TrimSpace(b)
	}
	return na == nb
}
