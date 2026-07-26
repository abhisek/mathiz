package llm

import (
	"context"
	"errors"
	"time"
)

// Purpose labels. These double as the keys of the per-purpose deadline table
// below, so a caller that tags its context gets the right budget for free.
const (
	PurposeQuestionGen     = "question-gen"
	PurposeQuestGen        = "quest-gen"
	PurposeLesson          = "lesson"
	PurposeDiagnosis       = "error-diagnosis"
	PurposeSessionCompress = "session-compress"
	PurposeProfile         = "profile"
)

// purposeTimeouts bounds a single LLM attempt, by what the call is for. One
// flat number is wrong here: the calls have very different shapes.
//
//   - question-gen blocks a child staring at a loading spinner, so it fails
//     fast. It is also the only entry a human waits on directly.
//   - quest-gen generates up to MaxGenerateCount questions in one batch
//     (512 tokens each), so it legitimately runs long. A 30s cap would break
//     large batches that work today.
//   - profile and session-compress must finish inside the caller's own 60s
//     budget (session.profileTimeout) — a per-attempt cap at or above that
//     would let the outer deadline fire mid-retry instead.
//   - diagnosis and lesson are background work nobody waits on, but were
//     previously unbounded: they run on context.Background().
//
// A purpose with no entry gets the configured fallback (Config.Timeout).
var purposeTimeouts = map[string]time.Duration{
	PurposeQuestionGen:     20 * time.Second,
	PurposeQuestGen:        120 * time.Second,
	PurposeLesson:          30 * time.Second,
	PurposeDiagnosis:       30 * time.Second,
	PurposeSessionCompress: 25 * time.Second,
	PurposeProfile:         25 * time.Second,
}

// defaultTimeout applies when the configured fallback is unset. A zero
// duration would otherwise mean "already expired" and fail every call.
const defaultTimeout = 30 * time.Second

// TimeoutProvider bounds every call with a deadline. It sits inside the retry
// decorator so each attempt is bounded independently — which only works
// because an expired attempt is reported as ErrTimeout rather than a bare
// context error, so the retry decorator can tell it apart from the caller
// giving up (see Generate).
//
// It sits outside logging so a timed-out attempt is still recorded; note the
// logging decorator must not persist on the context it is handed here, since
// by then that context is dead.
//
// Note this can only ever tighten the caller's deadline, never extend it: a
// request context that is already closer to expiry still wins.
type TimeoutProvider struct {
	inner    Provider
	fallback time.Duration
}

// WithTimeout wraps a Provider so no call can hang indefinitely. fallback is
// used for purposes without a specific budget.
func WithTimeout(p Provider, fallback time.Duration) Provider {
	if fallback <= 0 {
		fallback = defaultTimeout
	}
	return &TimeoutProvider{inner: p, fallback: fallback}
}

func (t *TimeoutProvider) Generate(ctx context.Context, req Request) (*Response, error) {
	purpose := PurposeFrom(ctx)
	attemptCtx, cancel := context.WithTimeout(ctx, t.timeoutFor(purpose))
	defer cancel()

	resp, err := t.inner.Generate(attemptCtx, req)
	// Distinguish "this attempt hung" from "the caller went away". Both surface
	// as context.DeadlineExceeded, but only the first is transient and worth
	// retrying — reporting them identically would make every timeout consume
	// the whole retry budget in a single attempt.
	if err != nil && ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
		return nil, &ErrTimeout{Purpose: purpose, Err: err}
	}
	return resp, err
}

func (t *TimeoutProvider) ModelID() string { return t.inner.ModelID() }

// timeoutFor returns the deadline for a purpose, falling back to the
// configured default for unlabelled or unknown calls.
func (t *TimeoutProvider) timeoutFor(purpose string) time.Duration {
	if d, ok := purposeTimeouts[purpose]; ok {
		return d
	}
	return t.fallback
}
