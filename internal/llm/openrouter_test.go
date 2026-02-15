package llm

import (
	"testing"
)

func TestNewOpenRouterProvider(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		p, err := NewOpenRouterProvider(OpenRouterConfig{
			APIKey: "sk-or-test",
			Model:  "google/gemini-2.0-flash-exp",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.ModelID() != "google/gemini-2.0-flash-exp" {
			t.Errorf("model = %q, want %q", p.ModelID(), "google/gemini-2.0-flash-exp")
		}
	})

	t.Run("empty API key", func(t *testing.T) {
		_, err := NewOpenRouterProvider(OpenRouterConfig{
			Model: "google/gemini-2.0-flash-exp",
		})
		if err == nil {
			t.Fatal("expected error for empty API key")
		}
	})

	t.Run("default base URL", func(t *testing.T) {
		p, err := NewOpenRouterProvider(OpenRouterConfig{
			APIKey: "sk-or-test",
			Model:  "meta-llama/llama-3-8b",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// The provider should be created successfully with default base URL.
		if p == nil {
			t.Fatal("expected non-nil provider")
		}
	})

	t.Run("custom model pass-through", func(t *testing.T) {
		p, err := NewOpenRouterProvider(OpenRouterConfig{
			APIKey: "sk-or-test",
			Model:  "anthropic/claude-3-haiku",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Model ID should be used as-is (no friendly-name mapping).
		if p.ModelID() != "anthropic/claude-3-haiku" {
			t.Errorf("model = %q, want %q", p.ModelID(), "anthropic/claude-3-haiku")
		}
	})

	t.Run("custom base URL", func(t *testing.T) {
		p, err := NewOpenRouterProvider(OpenRouterConfig{
			APIKey:  "sk-or-test",
			Model:   "google/gemini-2.0-flash-exp",
			BaseURL: "https://custom.openrouter.example/v1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil provider")
		}
	})
}
