package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
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
		Provider:    l.inner.ModelID(),
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
	if logErr := l.eventRepo.AppendLLMRequest(ctx, data); logErr != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to log LLM request event: %v\n", logErr)
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
