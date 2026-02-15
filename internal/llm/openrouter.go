package llm

import "fmt"

const defaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

// OpenRouterProvider wraps OpenAIProvider with OpenRouter-specific defaults.
// OpenRouter exposes an OpenAI-compatible API, so the underlying SDK is reused.
type OpenRouterProvider struct {
	*OpenAIProvider
}

// NewOpenRouterProvider creates a provider targeting the OpenRouter API.
func NewOpenRouterProvider(cfg OpenRouterConfig) (*OpenRouterProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openrouter API key is required")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultOpenRouterBaseURL
	}

	oaiCfg := OpenAIConfig{
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
		BaseURL: baseURL,
	}

	inner, err := newOpenAIProviderRaw(oaiCfg)
	if err != nil {
		return nil, err
	}

	return &OpenRouterProvider{OpenAIProvider: inner}, nil
}
