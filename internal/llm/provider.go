package llm

import (
	"context"
	"encoding/json"
)

// Provider is the core abstraction for LLM interaction.
// Consumers call Generate with a Request and receive structured JSON.
type Provider interface {
	// Generate sends a prompt to the LLM and returns a structured response.
	// The request's Schema field, when set, instructs the provider to return
	// JSON conforming to that schema. The response Content will be the
	// validated JSON.
	Generate(ctx context.Context, req Request) (*Response, error)

	// ModelID returns the model identifier this provider is configured to use.
	ModelID() string
}

// Request describes what to send to the LLM.
type Request struct {
	// System is the system prompt. Sets the LLM's role and constraints.
	System string

	// Messages is the conversation history. For single-turn generation
	// (the common case in Mathiz), this contains one user message.
	Messages []Message

	// Schema is the JSON Schema the response must conform to.
	// When set, the provider uses its native structured output mechanism.
	// When nil, the response Content is raw text as json.RawMessage.
	Schema *Schema

	// MaxTokens is the maximum number of tokens in the response.
	MaxTokens int

	// Temperature controls randomness. Range: 0.0 - 1.0.
	// Default: 0.0 (deterministic) when not set.
	Temperature float64
}

// Message represents a single message in the conversation.
type Message struct {
	Role    Role
	Content string
}

// Role is the message sender role.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Schema defines the JSON structure expected from the LLM.
type Schema struct {
	// Name identifies this schema (used as tool name for Anthropic,
	// schema name for OpenAI). Kebab-case, e.g. "math-question".
	Name string

	// Description is a human-readable description of what this schema
	// represents. Sent to the LLM to guide generation.
	Description string

	// Definition is the JSON Schema definition as a map.
	Definition map[string]any
}

// Response holds the LLM's output.
type Response struct {
	// Content is the generated output. When a Schema was provided in the
	// request, this is the validated JSON object. When no Schema was
	// provided, this is the raw text response wrapped as a JSON string.
	Content json.RawMessage

	// Usage reports token consumption for this request.
	Usage Usage

	// Model is the actual model that served the request.
	Model string

	// StopReason indicates why generation stopped.
	// Normalized to: "end", "max_tokens", "error"
	StopReason string
}

// Usage tracks token consumption for a single request.
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}
