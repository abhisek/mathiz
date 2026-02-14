package llm

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestMockProvider_ReturnsCanedResponses(t *testing.T) {
	mock := NewMockProvider(
		MockResponse{Content: json.RawMessage(`{"a":1}`), Usage: Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}},
		MockResponse{Content: json.RawMessage(`{"b":2}`)},
	)

	resp1, err := mock.Generate(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "first"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(resp1.Content) != `{"a":1}` {
		t.Fatalf("expected {\"a\":1}, got %s", resp1.Content)
	}
	if resp1.Usage.InputTokens != 10 {
		t.Fatalf("expected 10 input tokens, got %d", resp1.Usage.InputTokens)
	}
	if resp1.StopReason != "end" {
		t.Fatalf("expected stop reason 'end', got %q", resp1.StopReason)
	}

	resp2, err := mock.Generate(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "second"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(resp2.Content) != `{"b":2}` {
		t.Fatalf("expected {\"b\":2}, got %s", resp2.Content)
	}
}

func TestMockProvider_EmptyQueueReturnsError(t *testing.T) {
	mock := NewMockProvider()
	_, err := mock.Generate(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error from empty queue")
	}
	var unavail *ErrProviderUnavailable
	if !errors.As(err, &unavail) {
		t.Fatalf("expected ErrProviderUnavailable, got: %T", err)
	}
}

func TestMockProvider_RecordsCalls(t *testing.T) {
	mock := NewMockProvider(
		MockResponse{Content: json.RawMessage(`{}`)},
	)

	req := Request{
		System:   "sys",
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	}
	_, _ = mock.Generate(context.Background(), req)

	if mock.CallCount() != 1 {
		t.Fatalf("expected 1 call, got %d", mock.CallCount())
	}
	if mock.Calls[0].System != "sys" {
		t.Fatalf("expected system 'sys', got %q", mock.Calls[0].System)
	}
}

func TestMockProvider_ReturnsConfiguredError(t *testing.T) {
	mock := NewMockProvider(
		MockResponse{Err: &ErrRateLimit{RetryAfter: 0}},
	)

	_, err := mock.Generate(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error")
	}
	var rl *ErrRateLimit
	if !errors.As(err, &rl) {
		t.Fatalf("expected ErrRateLimit, got: %T", err)
	}
}

func TestMockProvider_ModelID(t *testing.T) {
	mock := NewMockProvider()
	if mock.ModelID() != "mock" {
		t.Fatalf("expected 'mock', got %q", mock.ModelID())
	}
}

func TestPurposeContext(t *testing.T) {
	ctx := context.Background()
	if p := PurposeFrom(ctx); p != "unknown" {
		t.Fatalf("expected 'unknown', got %q", p)
	}

	ctx = WithPurpose(ctx, "question-gen")
	if p := PurposeFrom(ctx); p != "question-gen" {
		t.Fatalf("expected 'question-gen', got %q", p)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "anthropic without key",
			cfg:     Config{Provider: "anthropic"},
			wantErr: true,
		},
		{
			name:    "anthropic with key",
			cfg:     Config{Provider: "anthropic", Anthropic: AnthropicConfig{APIKey: "sk-test"}},
			wantErr: false,
		},
		{
			name:    "openai without key",
			cfg:     Config{Provider: "openai"},
			wantErr: true,
		},
		{
			name:    "openai with key",
			cfg:     Config{Provider: "openai", OpenAI: OpenAIConfig{APIKey: "sk-test"}},
			wantErr: false,
		},
		{
			name:    "mock needs no key",
			cfg:     Config{Provider: "mock"},
			wantErr: false,
		},
		{
			name:    "unknown provider",
			cfg:     Config{Provider: "unknown"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
