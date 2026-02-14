package problemgen

// Config controls the behavior of the LLMGenerator.
type Config struct {
	// Validators is the ordered list of validators to run on every
	// generated question. They execute in order; the first failure
	// stops the pipeline.
	Validators []Validator

	// MaxTokens is the token budget for the LLM response.
	MaxTokens int

	// Temperature controls LLM output randomness (0.0-1.0).
	Temperature float64

	// MaxPriorQuestions is the maximum number of prior questions
	// to include in the prompt for deduplication.
	MaxPriorQuestions int

	// MaxRecentErrors is the maximum number of recent errors
	// to include in the prompt for context.
	MaxRecentErrors int
}

// DefaultConfig returns a Config with the standard validator chain
// and recommended defaults.
func DefaultConfig() Config {
	return Config{
		Validators: []Validator{
			&StructuralValidator{},
			&AnswerFormatValidator{},
			&MathCheckValidator{},
		},
		MaxTokens:         512,
		Temperature:       0.7,
		MaxPriorQuestions: 8,
		MaxRecentErrors:   5,
	}
}
