package problemgen

import "context"

// Generator produces math questions using an LLM provider.
type Generator interface {
	// Generate produces a single question for the given input context.
	// Returns a validated Question or an error.
	// All configured validators are run before returning.
	Generate(ctx context.Context, input GenerateInput) (*Question, error)
}
