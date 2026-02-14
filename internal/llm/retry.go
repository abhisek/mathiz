package llm

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"time"
)

// RetryProvider is a decorator that retries transient errors with
// exponential backoff and jitter.
type RetryProvider struct {
	inner  Provider
	config RetryConfig
}

// WithRetry wraps a Provider with retry logic.
func WithRetry(p Provider, cfg RetryConfig) Provider {
	return &RetryProvider{inner: p, config: cfg}
}

func (r *RetryProvider) Generate(ctx context.Context, req Request) (*Response, error) {
	var lastErr error
	invalidRetried := false

	for attempt := range r.config.MaxAttempts {
		resp, err := r.inner.Generate(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		if !r.shouldRetry(err, &invalidRetried) {
			return nil, err
		}

		// Last attempt — don't sleep, just return the error.
		if attempt == r.config.MaxAttempts-1 {
			break
		}

		wait := r.backoff(attempt, err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}

	return nil, lastErr
}

func (r *RetryProvider) ModelID() string {
	return r.inner.ModelID()
}

// shouldRetry determines if an error is retryable.
func (r *RetryProvider) shouldRetry(err error, invalidRetried *bool) bool {
	// Context errors are never retried.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Max tokens is a configuration issue, not transient.
	var maxTok *ErrMaxTokensExceeded
	if errors.As(err, &maxTok) {
		return false
	}

	// Invalid response gets one retry.
	var invResp *ErrInvalidResponse
	if errors.As(err, &invResp) {
		if *invalidRetried {
			return false
		}
		*invalidRetried = true
		return true
	}

	// Rate limit and provider unavailable are retryable.
	var rl *ErrRateLimit
	if errors.As(err, &rl) {
		return true
	}
	var unavail *ErrProviderUnavailable
	if errors.As(err, &unavail) {
		return true
	}

	// Other errors (network, etc.) are treated as transient.
	return true
}

// backoff computes the wait duration for the given attempt.
func (r *RetryProvider) backoff(attempt int, err error) time.Duration {
	// Respect RetryAfter for rate limits.
	var rl *ErrRateLimit
	if errors.As(err, &rl) && rl.RetryAfter > 0 {
		return rl.RetryAfter
	}

	wait := float64(r.config.InitialWait) * math.Pow(r.config.Multiplier, float64(attempt))
	if wait > float64(r.config.MaxWait) {
		wait = float64(r.config.MaxWait)
	}

	// Add ±20% jitter.
	jitter := wait * 0.2 * (2*rand.Float64() - 1)
	wait += jitter

	if wait < 0 {
		wait = 0
	}
	return time.Duration(wait)
}
