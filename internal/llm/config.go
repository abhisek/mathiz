package llm

import (
	"fmt"
	"os"
	"time"
)

// Config holds all LLM provider configuration.
type Config struct {
	// Provider selects which LLM provider to use.
	// Values: "anthropic", "openai", "gemini", "openrouter", "mock"
	Provider string

	Anthropic  AnthropicConfig
	OpenAI     OpenAIConfig
	Gemini     GeminiConfig
	OpenRouter OpenRouterConfig
	Retry      RetryConfig

	// Timeout is the maximum duration for a single LLM request
	// (including retries). Default: 30s.
	Timeout time.Duration
}

// AnthropicConfig holds Anthropic-specific configuration.
type AnthropicConfig struct {
	APIKey string
	Model  string // Default: "claude-haiku"
}

// OpenAIConfig holds OpenAI-specific configuration.
type OpenAIConfig struct {
	APIKey  string
	Model   string // Default: "gpt-4o-mini"
	BaseURL string // Optional. Override for OpenRouter or compatible APIs.
}

// GeminiConfig holds Gemini-specific configuration.
type GeminiConfig struct {
	APIKey string
	Model  string // Default: "gemini-flash"
}

// OpenRouterConfig holds OpenRouter-specific configuration.
type OpenRouterConfig struct {
	APIKey  string
	Model   string // Default: "google/gemini-2.0-flash-exp"
	BaseURL string // Default: "https://openrouter.ai/api/v1"
}

// RetryConfig configures retry behavior for transient failures.
type RetryConfig struct {
	MaxAttempts int
	InitialWait time.Duration
	MaxWait     time.Duration
	Multiplier  float64
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Provider: "anthropic",
		Anthropic: AnthropicConfig{
			Model: "claude-haiku",
		},
		OpenAI: OpenAIConfig{
			Model: "gpt-4o-mini",
		},
		Gemini: GeminiConfig{
			Model: "gemini-flash",
		},
		OpenRouter: OpenRouterConfig{
			Model: "google/gemini-2.0-flash-exp",
		},
		Retry: RetryConfig{
			MaxAttempts: 3,
			InitialWait: 1 * time.Second,
			MaxWait:     10 * time.Second,
			Multiplier:  2.0,
		},
		Timeout: 30 * time.Second,
	}
}

// ConfigFromEnv builds a Config from environment variables, falling back
// to defaults for unset values.
func ConfigFromEnv() Config {
	cfg := DefaultConfig()

	if p := os.Getenv("MATHIZ_LLM_PROVIDER"); p != "" {
		cfg.Provider = p
	}

	if k := os.Getenv("MATHIZ_ANTHROPIC_API_KEY"); k != "" {
		cfg.Anthropic.APIKey = k
	}
	if m := os.Getenv("MATHIZ_ANTHROPIC_MODEL"); m != "" {
		cfg.Anthropic.Model = m
	}

	if k := os.Getenv("MATHIZ_OPENAI_API_KEY"); k != "" {
		cfg.OpenAI.APIKey = k
	}
	if m := os.Getenv("MATHIZ_OPENAI_MODEL"); m != "" {
		cfg.OpenAI.Model = m
	}
	if u := os.Getenv("MATHIZ_OPENAI_BASE_URL"); u != "" {
		cfg.OpenAI.BaseURL = u
	}

	if k := os.Getenv("MATHIZ_GEMINI_API_KEY"); k != "" {
		cfg.Gemini.APIKey = k
	}
	if m := os.Getenv("MATHIZ_GEMINI_MODEL"); m != "" {
		cfg.Gemini.Model = m
	}

	if k := os.Getenv("MATHIZ_OPENROUTER_API_KEY"); k != "" {
		cfg.OpenRouter.APIKey = k
	}
	if m := os.Getenv("MATHIZ_OPENROUTER_MODEL"); m != "" {
		cfg.OpenRouter.Model = m
	}

	return cfg
}

// DiscoverConfig probes standard API key env vars in priority order
// (Gemini → OpenAI → Anthropic) and returns a Config for the first
// provider whose key is found. Returns (Config{}, false) if none found.
func DiscoverConfig() (Config, bool) {
	cfg := DefaultConfig()

	if k := os.Getenv("GEMINI_API_KEY"); k != "" {
		cfg.Provider = "gemini"
		cfg.Gemini.APIKey = k
		return cfg, true
	}
	if k := os.Getenv("OPENAI_API_KEY"); k != "" {
		cfg.Provider = "openai"
		cfg.OpenAI.APIKey = k
		return cfg, true
	}
	if k := os.Getenv("ANTHROPIC_API_KEY"); k != "" {
		cfg.Provider = "anthropic"
		cfg.Anthropic.APIKey = k
		return cfg, true
	}
	if k := os.Getenv("OPENROUTER_API_KEY"); k != "" {
		cfg.Provider = "openrouter"
		cfg.OpenRouter.APIKey = k
		return cfg, true
	}

	return Config{}, false
}

// Validate checks that the selected provider has its required API key set.
func (c Config) Validate() error {
	switch c.Provider {
	case "anthropic":
		if c.Anthropic.APIKey == "" {
			return fmt.Errorf("MATHIZ_ANTHROPIC_API_KEY is required for the anthropic provider")
		}
	case "openai":
		if c.OpenAI.APIKey == "" {
			return fmt.Errorf("MATHIZ_OPENAI_API_KEY is required for the openai provider")
		}
	case "gemini":
		if c.Gemini.APIKey == "" {
			return fmt.Errorf("MATHIZ_GEMINI_API_KEY is required for the gemini provider")
		}
	case "openrouter":
		if c.OpenRouter.APIKey == "" {
			return fmt.Errorf("MATHIZ_OPENROUTER_API_KEY is required for the openrouter provider")
		}
	case "mock":
		// No API key needed.
	default:
		return fmt.Errorf("unknown LLM provider: %q", c.Provider)
	}
	return nil
}
