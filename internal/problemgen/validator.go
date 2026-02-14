package problemgen

import "fmt"

// Validator checks a generated question for correctness.
// Implementations should be stateless and safe for concurrent use.
type Validator interface {
	// Name returns a short identifier for this validator (for error messages
	// and logging), e.g. "structural", "math-check", "answer-format".
	Name() string

	// Validate checks the question and returns nil if it passes.
	// Returns a ValidationError if the question fails the check.
	// The validator receives the full GenerateInput for context (e.g., to
	// know which skill/tier the question was generated for).
	Validate(q *Question, input GenerateInput) *ValidationError
}

// ValidationError describes why a question failed validation.
type ValidationError struct {
	Validator string // Name of the validator that failed
	Message   string // Human-readable description of the failure
	Retryable bool   // Whether regeneration is likely to fix this
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validator %q: %s", e.Validator, e.Message)
}
