package llm

import (
	"context"
	"sync/atomic"
)

type contextKey string

const (
	purposeKey  contextKey = "llm_purpose"
	attemptsKey contextKey = "llm_attempts"
)

// AttemptCounter counts provider attempts (retries included) made under one
// context. Latency telemetry reports it next to the wait it caused.
type AttemptCounter struct{ n atomic.Int32 }

// Count returns the attempts made so far.
func (c *AttemptCounter) Count() int { return int(c.n.Load()) }

// WithAttemptCounter attaches a fresh counter. Every provider attempt made
// with the returned context, or a context derived from it, increments it.
func WithAttemptCounter(ctx context.Context) (context.Context, *AttemptCounter) {
	c := &AttemptCounter{}
	return context.WithValue(ctx, attemptsKey, c), c
}

// countAttempt records one provider attempt, if a counter is attached.
func countAttempt(ctx context.Context) {
	if c, ok := ctx.Value(attemptsKey).(*AttemptCounter); ok {
		c.n.Add(1)
	}
}

// WithPurpose attaches a purpose label to the context for event logging.
func WithPurpose(ctx context.Context, purpose string) context.Context {
	return context.WithValue(ctx, purposeKey, purpose)
}

// PurposeFrom extracts the purpose label from the context.
func PurposeFrom(ctx context.Context) string {
	if v, ok := ctx.Value(purposeKey).(string); ok {
		return v
	}
	return "unknown"
}
