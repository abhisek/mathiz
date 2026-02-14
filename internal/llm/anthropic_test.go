package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func newTestAnthropicProvider(t *testing.T, handler http.HandlerFunc) *AnthropicProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)
	return &AnthropicProvider{
		client: &client,
		model:  "claude-sonnet-4-20250514",
	}
}

func TestAnthropicProvider_HappyPath(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":   "msg_test",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": `{"question":"What is 2+3?","answer":"5"}`},
			},
			"model":       "claude-sonnet-4-20250514",
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  50,
				"output_tokens": 30,
			},
		})
	}

	p := newTestAnthropicProvider(t, handler)
	resp, err := p.Generate(context.Background(), Request{
		System:    "You are a math tutor.",
		Messages:  []Message{{Role: RoleUser, Content: "Generate a question."}},
		MaxTokens: 256,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Usage.InputTokens != 50 {
		t.Fatalf("expected 50 input tokens, got %d", resp.Usage.InputTokens)
	}
	if resp.StopReason != "end" {
		t.Fatalf("expected stop reason 'end', got %q", resp.StopReason)
	}
}

func TestAnthropicProvider_RateLimit(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    "rate_limit_error",
				"message": "Rate limit exceeded",
			},
		})
	}

	p := newTestAnthropicProvider(t, handler)
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

func TestAnthropicProvider_ServerError(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    "api_error",
				"message": "Internal server error",
			},
		})
	}

	p := newTestAnthropicProvider(t, handler)
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

func TestAnthropicProvider_ModelID(t *testing.T) {
	p := &AnthropicProvider{model: "claude-sonnet-4-20250514"}
	if p.ModelID() != "claude-sonnet-4-20250514" {
		t.Fatalf("expected 'claude-sonnet-4-20250514', got %q", p.ModelID())
	}
}

func TestAnthropicModelMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"claude-sonnet", "claude-sonnet-4-20250514"},
		{"claude-haiku", "claude-haiku-4-5-20251001"},
		{"claude-sonnet-4-20250514", "claude-sonnet-4-20250514"}, // Pass-through
	}
	for _, tt := range tests {
		got := resolveModel(tt.input, anthropicModels)
		if got != tt.expected {
			t.Errorf("resolveModel(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
