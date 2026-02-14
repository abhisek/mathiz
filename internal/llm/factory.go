package llm

import (
	"context"
	"fmt"

	"github.com/abhisek/mathiz/internal/store"
)

// NewProvider creates a Provider from configuration.
// It returns the provider wrapped with retry and logging middleware.
func NewProvider(ctx context.Context, cfg Config, eventRepo store.EventRepo) (Provider, error) {
	var base Provider
	var err error

	switch cfg.Provider {
	case "anthropic":
		base, err = NewAnthropicProvider(cfg.Anthropic)
	case "openai":
		base, err = NewOpenAIProvider(cfg.OpenAI)
	case "gemini":
		base, err = NewGeminiProvider(ctx, cfg.Gemini)
	case "mock":
		return NewMockProvider(), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider: %q", cfg.Provider)
	}
	if err != nil {
		return nil, fmt.Errorf("initializing %s provider: %w", cfg.Provider, err)
	}

	// Wrap with middleware: caller → retry → logging → base
	logged := WithLogging(base, eventRepo)
	retried := WithRetry(logged, cfg.Retry)

	return retried, nil
}

// NewProviderFromEnv auto-discovers LLM credentials from standard env vars
// (GEMINI_API_KEY, OPENAI_API_KEY, ANTHROPIC_API_KEY) and creates a fully
// decorated provider. Returns an error if no credentials are found.
func NewProviderFromEnv(ctx context.Context, eventRepo store.EventRepo) (Provider, error) {
	cfg, ok := DiscoverConfig()
	if !ok {
		return nil, fmt.Errorf("no LLM API key found; set one of GEMINI_API_KEY, OPENAI_API_KEY, or ANTHROPIC_API_KEY")
	}
	return NewProvider(ctx, cfg, eventRepo)
}
