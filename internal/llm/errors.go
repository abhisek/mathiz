package llm

import (
	"encoding/json"
	"fmt"
	"time"
)

// ErrRateLimit indicates the provider returned a rate limit error (429).
type ErrRateLimit struct {
	RetryAfter time.Duration
	Err        error
}

func (e *ErrRateLimit) Error() string {
	return fmt.Sprintf("rate limited (retry after %s): %v", e.RetryAfter, e.Err)
}

func (e *ErrRateLimit) Unwrap() error { return e.Err }

// ErrInvalidResponse indicates the LLM returned content that does not
// conform to the requested schema.
type ErrInvalidResponse struct {
	Content json.RawMessage
	Err     error
}

func (e *ErrInvalidResponse) Error() string {
	return fmt.Sprintf("invalid LLM response: %v", e.Err)
}

func (e *ErrInvalidResponse) Unwrap() error { return e.Err }

// ErrProviderUnavailable indicates the provider is down or unreachable.
type ErrProviderUnavailable struct {
	Err error
}

func (e *ErrProviderUnavailable) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("LLM provider unavailable: %v", e.Err)
	}
	return "LLM provider unavailable"
}

func (e *ErrProviderUnavailable) Unwrap() error { return e.Err }

// ErrMaxTokensExceeded indicates the response was truncated because it
// hit the MaxTokens limit.
type ErrMaxTokensExceeded struct {
	Content json.RawMessage
}

func (e *ErrMaxTokensExceeded) Error() string {
	return "LLM response truncated: max tokens exceeded"
}
