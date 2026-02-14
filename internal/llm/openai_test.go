package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func newTestOpenAIProvider(t *testing.T, handler http.HandlerFunc) *OpenAIProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	config := openai.DefaultConfig("test-key")
	config.BaseURL = server.URL + "/v1"
	client := openai.NewClientWithConfig(config)

	return &OpenAIProvider{
		client: client,
		model:  "gpt-4o-mini",
	}
}

func TestOpenAIProvider_HappyPath(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   "gpt-4o-mini",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": `{"question":"What is 2+3?","answer":"5"}`,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     40,
				"completion_tokens": 25,
				"total_tokens":      65,
			},
		})
	}

	p := newTestOpenAIProvider(t, handler)
	resp, err := p.Generate(context.Background(), Request{
		System:    "You are a math tutor.",
		Messages:  []Message{{Role: RoleUser, Content: "Generate a question."}},
		MaxTokens: 256,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Usage.InputTokens != 40 {
		t.Fatalf("expected 40 input tokens, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 25 {
		t.Fatalf("expected 25 output tokens, got %d", resp.Usage.OutputTokens)
	}
	if resp.StopReason != "end" {
		t.Fatalf("expected stop reason 'end', got %q", resp.StopReason)
	}
}

func TestOpenAIProvider_RateLimit(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"type":    "tokens",
				"message": "Rate limit exceeded",
				"code":    "rate_limit_exceeded",
			},
		})
	}

	p := newTestOpenAIProvider(t, handler)
	_, err := p.Generate(context.Background(), Request{
		Messages:  []Message{{Role: RoleUser, Content: "test"}},
		MaxTokens: 100,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var rl *ErrRateLimit
	if !errors.As(err, &rl) {
		t.Fatalf("expected ErrRateLimit, got: %T (%v)", err, err)
	}
}

func TestOpenAIProvider_ServerError(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"type":    "server_error",
				"message": "Internal server error",
			},
		})
	}

	p := newTestOpenAIProvider(t, handler)
	_, err := p.Generate(context.Background(), Request{
		Messages:  []Message{{Role: RoleUser, Content: "test"}},
		MaxTokens: 100,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var unavail *ErrProviderUnavailable
	if !errors.As(err, &unavail) {
		t.Fatalf("expected ErrProviderUnavailable, got: %T (%v)", err, err)
	}
}

func TestOpenAIProvider_ModelID(t *testing.T) {
	p := &OpenAIProvider{model: "gpt-4o-mini"}
	if p.ModelID() != "gpt-4o-mini" {
		t.Fatalf("expected 'gpt-4o-mini', got %q", p.ModelID())
	}
}

func TestOpenAIProvider_BaseURLOverride(t *testing.T) {
	// Verify that the provider can be created with a custom BaseURL.
	cfg := OpenAIConfig{
		APIKey:  "test-key",
		Model:   "gpt-4o",
		BaseURL: "https://openrouter.ai/api/v1",
	}
	p, err := NewOpenAIProvider(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ModelID() != "gpt-4o" {
		t.Fatalf("expected 'gpt-4o', got %q", p.ModelID())
	}
}
