package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

// blockingProvider blocks until the context is done, then reports the error.
// It stands in for a provider that has stopped responding.
type blockingProvider struct{ started chan struct{} }

func (b *blockingProvider) Generate(ctx context.Context, _ Request) (*Response, error) {
	if b.started != nil {
		close(b.started)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *blockingProvider) ModelID() string { return "blocking" }

// deadlineProvider records the deadline it was handed.
type deadlineProvider struct {
	deadline time.Time
	ok       bool
}

func (d *deadlineProvider) Generate(ctx context.Context, _ Request) (*Response, error) {
	d.deadline, d.ok = ctx.Deadline()
	return &Response{Content: []byte(`{}`)}, nil
}

func (d *deadlineProvider) ModelID() string { return "deadline" }

// TestWithTimeoutBoundsAHungCall is the core guarantee: before this decorator
// existed, Config.Timeout was declared and never wired, so a provider that
// stopped responding hung the caller indefinitely — on the child's play
// surface, that is a spinner that never resolves.
func TestWithTimeoutBoundsAHungCall(t *testing.T) {
	p := WithTimeout(&blockingProvider{}, 20*time.Millisecond)

	done := make(chan error, 1)
	go func() {
		_, err := p.Generate(context.Background(), Request{})
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call was not bounded by the timeout")
	}
}

func TestWithTimeoutPerPurposeBudgets(t *testing.T) {
	fallback := 90 * time.Second
	tests := []struct {
		purpose string
		want    time.Duration
	}{
		{PurposeQuestionGen, 20 * time.Second},
		{PurposeQuestGen, 120 * time.Second},
		{PurposeProfile, 25 * time.Second},
		{PurposeSessionCompress, 25 * time.Second},
		{PurposeDiagnosis, 30 * time.Second},
		{PurposeLesson, 30 * time.Second},
		{"something-new", fallback},
	}
	for _, tc := range tests {
		t.Run(tc.purpose, func(t *testing.T) {
			inner := &deadlineProvider{}
			p := WithTimeout(inner, fallback)
			if _, err := p.Generate(WithPurpose(context.Background(), tc.purpose), Request{}); err != nil {
				t.Fatalf("generate: %v", err)
			}
			if !inner.ok {
				t.Fatal("inner provider received no deadline")
			}
			// Allow slack for the time between setting and reading the deadline.
			if got := time.Until(inner.deadline); got > tc.want || got < tc.want-time.Second {
				t.Errorf("budget for %q ≈ %v, want %v", tc.purpose, got.Round(time.Second), tc.want)
			}
		})
	}
}

// TestWithTimeoutNeverExtendsCallerDeadline: a request context that is already
// closer to expiry must win, so a child closing the tab still cancels
// immediately rather than waiting out the purpose budget.
func TestWithTimeoutNeverExtendsCallerDeadline(t *testing.T) {
	inner := &deadlineProvider{}
	p := WithTimeout(inner, time.Minute)

	ctx, cancel := context.WithTimeout(WithPurpose(context.Background(), PurposeQuestGen), 50*time.Millisecond)
	defer cancel()
	if _, err := p.Generate(ctx, Request{}); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if got := time.Until(inner.deadline); got > time.Second {
		t.Errorf("deadline %v extended past the caller's; want the caller's to win", got)
	}
}

// TestWithTimeoutZeroFallback guards the footgun: a zero Config.Timeout must
// not mean "already expired".
func TestWithTimeoutZeroFallback(t *testing.T) {
	inner := &deadlineProvider{}
	p := WithTimeout(inner, 0)
	if _, err := p.Generate(context.Background(), Request{}); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if got := time.Until(inner.deadline); got < defaultTimeout-time.Second {
		t.Errorf("zero fallback gave %v, want ~%v", got, defaultTimeout)
	}
}
