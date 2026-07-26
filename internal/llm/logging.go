package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/abhisek/mathiz/internal/store"
)

// logWriteTimeout bounds the audit write itself. It is independent of the
// request's own deadline — see the note in Generate.
const logWriteTimeout = 5 * time.Second

// LoggingProvider is a decorator that records every LLM request as an event.
type LoggingProvider struct {
	inner        Provider
	eventRepo    store.EventRepo
	providerName string
}

// WithLogging wraps a Provider with event logging.
func WithLogging(p Provider, repo store.EventRepo, providerName string) Provider {
	return &LoggingProvider{inner: p, eventRepo: repo, providerName: providerName}
}

func (l *LoggingProvider) Generate(ctx context.Context, req Request) (*Response, error) {
	start := time.Now()
	purpose := PurposeFrom(ctx)

	resp, err := l.inner.Generate(ctx, req)

	latencyMs := time.Since(start).Milliseconds()

	data := store.LLMRequestEventData{
		Provider:    l.providerName,
		Model:       l.inner.ModelID(),
		Purpose:     purpose,
		LatencyMs:   latencyMs,
		Success:     err == nil,
		RequestBody: serializeRequest(req),
	}

	if resp != nil {
		data.InputTokens = resp.Usage.InputTokens
		data.OutputTokens = resp.Usage.OutputTokens
		data.Model = resp.Model
		data.ResponseBody = string(resp.Content)
	}

	if err != nil {
		data.ErrorMessage = err.Error()
	}

	// Log the event but don't fail the request if logging fails.
	//
	// Deliberately NOT on ctx: this decorator sits inside the per-attempt
	// deadline, so a timed-out or cancelled call hands us a context that is
	// already dead — and the write would fail precisely for the calls the
	// audit trail exists to explain. WithoutCancel keeps the values the
	// store's owner guard reads while dropping the cancellation, and the
	// write gets a short deadline of its own so it can't hang either.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), logWriteTimeout)
	defer cancel()
	if logErr := l.eventRepo.AppendLLMRequest(writeCtx, data); logErr != nil {
		slog.Error("llm: log request event", "purpose", purpose, "err", logErr)
	}

	return resp, err
}

func (l *LoggingProvider) ModelID() string {
	return l.inner.ModelID()
}

// serializeRequest builds a readable representation of the LLM request.
func serializeRequest(req Request) string {
	var b strings.Builder

	if req.System != "" {
		b.WriteString("[system]\n")
		b.WriteString(req.System)
		b.WriteString("\n\n")
	}

	for _, m := range req.Messages {
		b.WriteString(fmt.Sprintf("[%s]\n", m.Role))
		b.WriteString(m.Content)
		b.WriteString("\n\n")
	}

	if req.Schema != nil {
		schemaDef, err := json.Marshal(req.Schema.Definition)
		if err == nil {
			b.WriteString(fmt.Sprintf("[schema: %s]\n", req.Schema.Name))
			b.WriteString(string(schemaDef))
			b.WriteString("\n")
		}
	}

	return b.String()
}
