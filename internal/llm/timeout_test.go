package llm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/abhisek/mathiz/internal/store"
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

// countingProvider blocks until its context is done and counts attempts.
type countingProvider struct {
	mu    sync.Mutex
	calls int
}

func (c *countingProvider) Generate(ctx context.Context, _ Request) (*Response, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	<-ctx.Done()
	return nil, ctx.Err()
}

func (c *countingProvider) ModelID() string { return "counting" }

func (c *countingProvider) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// TestTimeoutAttemptsAreRetried: a per-attempt deadline that expires is a
// transient hang, not the caller giving up — it must not consume the whole
// retry budget in one shot.
func TestTimeoutAttemptsAreRetried(t *testing.T) {
	inner := &countingProvider{}
	p := WithRetry(WithTimeout(inner, 20*time.Millisecond), RetryConfig{
		MaxAttempts: 3, InitialWait: time.Millisecond, MaxWait: 2 * time.Millisecond, Multiplier: 2,
	})

	if _, err := p.Generate(context.Background(), Request{}); err == nil {
		t.Fatal("want an error from a provider that never responds")
	}
	if got := inner.count(); got != 3 {
		t.Errorf("attempts = %d, want 3 — an internal timeout is ending the retry chain", got)
	}
}

// TestCallerCancellationIsNotRetried: the other half of the distinction. When
// the caller goes away (child closes the tab), retrying is wasted work.
func TestCallerCancellationIsNotRetried(t *testing.T) {
	inner := &countingProvider{}
	p := WithRetry(WithTimeout(inner, time.Minute), RetryConfig{
		MaxAttempts: 3, InitialWait: time.Millisecond, MaxWait: 2 * time.Millisecond, Multiplier: 2,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := p.Generate(ctx, Request{}); err == nil {
		t.Fatal("want an error")
	}
	if got := inner.count(); got != 1 {
		t.Errorf("attempts = %d, want 1 — caller cancellation must not be retried", got)
	}
}

// TestTimedOutAttemptIsStillLogged: the audit trail exists so slow and failed
// calls can be investigated, which makes a timeout the single most important
// thing to record. LoggingProvider sits inside the timeout, so it must not
// persist the event on the very context that just expired.
func TestTimedOutAttemptIsStillLogged(t *testing.T) {
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	repo := st.EventRepo()

	// No purpose on the context, so the short fallback budget applies.
	p := WithTimeout(WithLogging(&blockingProvider{}, repo, "test"), 20*time.Millisecond)
	if _, err := p.Generate(context.Background(), Request{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}

	events, err := repo.QueryLLMEvents(context.Background(), store.QueryOpts{})
	if err != nil {
		t.Fatalf("query llm events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("llm events = %d, want 1 — the timed-out attempt was not recorded", len(events))
	}
	if events[0].Success {
		t.Error("timed-out attempt recorded as successful")
	}
}
