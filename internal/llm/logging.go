package llm

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/abhisek/mathiz/internal/store"
)

// LoggingProvider is a decorator that records every LLM request as an event.
type LoggingProvider struct {
	inner     Provider
	eventRepo store.EventRepo
}

// WithLogging wraps a Provider with event logging.
func WithLogging(p Provider, repo store.EventRepo) Provider {
	return &LoggingProvider{inner: p, eventRepo: repo}
}

func (l *LoggingProvider) Generate(ctx context.Context, req Request) (*Response, error) {
	start := time.Now()
	purpose := PurposeFrom(ctx)

	resp, err := l.inner.Generate(ctx, req)

	latencyMs := time.Since(start).Milliseconds()

	data := store.LLMRequestEventData{
		Provider:  l.inner.ModelID(),
		Model:     l.inner.ModelID(),
		Purpose:   purpose,
		LatencyMs: latencyMs,
		Success:   err == nil,
	}

	if resp != nil {
		data.InputTokens = resp.Usage.InputTokens
		data.OutputTokens = resp.Usage.OutputTokens
		data.Model = resp.Model
	}

	if err != nil {
		data.ErrorMessage = err.Error()
	}

	// Log the event but don't fail the request if logging fails.
	if logErr := l.eventRepo.AppendLLMRequest(ctx, data); logErr != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to log LLM request event: %v\n", logErr)
	}

	return resp, err
}

func (l *LoggingProvider) ModelID() string {
	return l.inner.ModelID()
}
