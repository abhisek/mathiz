package llm

import "context"

type contextKey string

const purposeKey contextKey = "llm_purpose"

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
