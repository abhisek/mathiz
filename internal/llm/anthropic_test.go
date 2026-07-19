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
		model:  "claude-sonnet-4-5-20250929",
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
			"model":       "claude-sonnet-4-5-20250929",
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

// anthropicSuccessBody returns a minimal successful Messages API response.
func anthropicSuccessBody(model string) map[string]any {
	return map[string]any{
		"id":   "msg_test",
		"type": "message",
		"role": "assistant",
		"content": []map[string]any{
			{"type": "text", "text": `{"ok":true}`},
		},
		"model":       model,
		"stop_reason": "end_turn",
		"usage": map[string]any{
			"input_tokens":  10,
			"output_tokens": 5,
		},
	}
}

func decodeRequestBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode request body: %v", err)
	}
	return body
}

func TestAnthropicProvider_Claude5FamilyOmitsTemperature(t *testing.T) {
	models := []string{"claude-sonnet-5", "claude-fable-5", "claude-opus-4-8", "claude-opus-4-7"}
	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			var requests int
			handler := func(w http.ResponseWriter, r *http.Request) {
				requests++
				body := decodeRequestBody(t, r)
				if _, ok := body["temperature"]; ok {
					t.Errorf("request for %s must not carry a temperature field, got %v", model, body["temperature"])
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(anthropicSuccessBody(model))
			}

			p := newTestAnthropicProvider(t, handler)
			p.model = model
			_, err := p.Generate(context.Background(), Request{
				Messages:    []Message{{Role: RoleUser, Content: "test"}},
				MaxTokens:   100,
				Temperature: 0.7,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if requests != 1 {
				t.Fatalf("expected exactly 1 request, got %d", requests)
			}
		})
	}
}

func TestAnthropicProvider_OlderModelKeepsTemperature(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		body := decodeRequestBody(t, r)
		temp, ok := body["temperature"].(float64)
		if !ok || temp != 0.7 {
			t.Errorf("expected temperature 0.7 in request, got %v", body["temperature"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicSuccessBody("claude-sonnet-4-5-20250929"))
	}

	p := newTestAnthropicProvider(t, handler)
	_, err := p.Generate(context.Background(), Request{
		Messages:    []Message{{Role: RoleUser, Content: "test"}},
		MaxTokens:   100,
		Temperature: 0.7,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAnthropicModelRejectsTemperature(t *testing.T) {
	tests := []struct {
		model   string
		rejects bool
	}{
		{"claude-sonnet-5", true},
		{"claude-fable-5", true},
		{"claude-mythos-5", true},
		{"claude-opus-4-7", true},
		{"claude-opus-4-8", true},
		{"claude-sonnet-4-5-20250929", false},
		{"claude-haiku-4-5-20251001", false},
		{"claude-sonnet-4-6", false},
		{"claude-opus-4-6", false},
	}
	for _, tt := range tests {
		if got := anthropicModelRejectsTemperature(tt.model); got != tt.rejects {
			t.Errorf("anthropicModelRejectsTemperature(%q) = %v, want %v", tt.model, got, tt.rejects)
		}
	}
}

func TestAnthropicProvider_ModelID(t *testing.T) {
	p := &AnthropicProvider{model: "claude-sonnet-4-5-20250929"}
	if p.ModelID() != "claude-sonnet-4-5-20250929" {
		t.Fatalf("expected 'claude-sonnet-4-5-20250929', got %q", p.ModelID())
	}
}

func TestAnthropicModelMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"claude-sonnet", "claude-sonnet-4-5-20250929"},
		{"claude-haiku", "claude-haiku-4-5-20251001"},
		{"claude-sonnet-4-5-20250929", "claude-sonnet-4-5-20250929"}, // Pass-through
	}
	for _, tt := range tests {
		got := resolveModel(tt.input, anthropicModels)
		if got != tt.expected {
			t.Errorf("resolveModel(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
