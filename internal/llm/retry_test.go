package llm

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func retryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		InitialWait: 1 * time.Millisecond,
		MaxWait:     10 * time.Millisecond,
		Multiplier:  2.0,
	}
}

func TestRetry_SucceedsOnFirstAttempt(t *testing.T) {
	mock := NewMockProvider(
		MockResponse{Content: json.RawMessage(`{"ok":true}`)},
	)
	p := WithRetry(mock, retryConfig())

	resp, err := p.Generate(context.Background(), Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(resp.Content) != `{"ok":true}` {
		t.Fatalf("unexpected content: %s", resp.Content)
	}
	if mock.CallCount() != 1 {
		t.Fatalf("expected 1 call, got %d", mock.CallCount())
	}
}

func TestRetry_TransientThenSuccess(t *testing.T) {
	mock := NewMockProvider(
		MockResponse{Err: &ErrProviderUnavailable{Err: errors.New("down")}},
		MockResponse{Content: json.RawMessage(`{"ok":true}`)},
	)
	p := WithRetry(mock, retryConfig())

	resp, err := p.Generate(context.Background(), Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(resp.Content) != `{"ok":true}` {
		t.Fatalf("unexpected content: %s", resp.Content)
	}
	if mock.CallCount() != 2 {
		t.Fatalf("expected 2 calls, got %d", mock.CallCount())
	}
}

func TestRetry_AllAttemptsFail(t *testing.T) {
	mock := NewMockProvider(
		MockResponse{Err: &ErrProviderUnavailable{Err: errors.New("down")}},
		MockResponse{Err: &ErrProviderUnavailable{Err: errors.New("down")}},
		MockResponse{Err: &ErrProviderUnavailable{Err: errors.New("down")}},
	)
	p := WithRetry(mock, retryConfig())

	_, err := p.Generate(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error")
	}
	if mock.CallCount() != 3 {
		t.Fatalf("expected 3 calls, got %d", mock.CallCount())
	}
}

func TestRetry_MaxTokensNotRetried(t *testing.T) {
	mock := NewMockProvider(
		MockResponse{Err: &ErrMaxTokensExceeded{Content: json.RawMessage(`{}`)}},
	)
	p := WithRetry(mock, retryConfig())

	_, err := p.Generate(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error")
	}
	var maxTok *ErrMaxTokensExceeded
	if !errors.As(err, &maxTok) {
		t.Fatalf("expected ErrMaxTokensExceeded, got: %T", err)
	}
	if mock.CallCount() != 1 {
		t.Fatalf("expected 1 call (no retry), got %d", mock.CallCount())
	}
}

func TestRetry_InvalidResponseRetriedOnce(t *testing.T) {
	mock := NewMockProvider(
		MockResponse{Err: &ErrInvalidResponse{Content: json.RawMessage(`bad`), Err: errors.New("bad")}},
		MockResponse{Err: &ErrInvalidResponse{Content: json.RawMessage(`bad`), Err: errors.New("bad")}},
		MockResponse{Content: json.RawMessage(`{"ok":true}`)}, // Won't be reached.
	)
	p := WithRetry(mock, retryConfig())

	_, err := p.Generate(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error")
	}
	// Should have retried once (2 calls total), then stopped.
	if mock.CallCount() != 2 {
		t.Fatalf("expected 2 calls, got %d", mock.CallCount())
	}
}

func TestRetry_ContextCancellation(t *testing.T) {
	mock := NewMockProvider(
		MockResponse{Err: &ErrProviderUnavailable{Err: errors.New("down")}},
		MockResponse{Err: &ErrProviderUnavailable{Err: errors.New("down")}},
		MockResponse{Content: json.RawMessage(`{"ok":true}`)},
	)
	p := WithRetry(mock, retryConfig())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := p.Generate(ctx, Request{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRetry_RateLimitRespectsRetryAfter(t *testing.T) {
	mock := NewMockProvider(
		MockResponse{Err: &ErrRateLimit{RetryAfter: 1 * time.Millisecond, Err: errors.New("429")}},
		MockResponse{Content: json.RawMessage(`{"ok":true}`)},
	)
	p := WithRetry(mock, retryConfig())

	resp, err := p.Generate(context.Background(), Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(resp.Content) != `{"ok":true}` {
		t.Fatalf("unexpected content: %s", resp.Content)
	}
	if mock.CallCount() != 2 {
		t.Fatalf("expected 2 calls, got %d", mock.CallCount())
	}
}

func TestRetry_ModelIDDelegates(t *testing.T) {
	mock := NewMockProvider()
	p := WithRetry(mock, retryConfig())
	if p.ModelID() != "mock" {
		t.Fatalf("expected 'mock', got %q", p.ModelID())
	}
}
