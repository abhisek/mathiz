# 04 — LLM Integration

## 1. Overview

The LLM Integration module is the **provider abstraction layer** for Mathiz. It gives every other module a single, provider-agnostic interface for generating structured text from large language models. Consumer modules (Problem Generation, Error Diagnosis, AI Lessons) call this layer — they never import provider SDKs directly.

**Design goals:**

- **Provider-agnostic**: Consumers program against a Go interface. Swapping Anthropic for OpenAI or Gemini requires zero consumer changes.
- **Structured output**: Every LLM call specifies a JSON schema. The response is validated against that schema before returning.
- **Observable**: Every request/response is logged as an event for debugging and cost tracking.
- **Resilient**: Retries with exponential backoff, timeout enforcement, and graceful error propagation.
- **Testable**: A deterministic mock provider ships with the module for use in all tests.

### Consumers

| Module | How it uses LLM |
|--------|----------------|
| **Problem Generation (05)** | Generates math questions with structured answer metadata |
| **Error Diagnosis (09)** | Fallback misconception analysis when rule-based detection is uncertain |
| **AI Lessons & Hints (10)** | Generates hints, micro-lessons, worked examples, and context compression snapshots |
| **Diagnostic Placement (03)** | Generates probe questions during initial placement quiz |

---

## 2. Library Selection

### Approach: Individual Official SDKs Behind a Custom Interface

Rather than adopting a multi-provider framework (langchaingo, Eino), Mathiz uses the **official SDK for each provider** wrapped behind a thin adapter. This gives us:

- Full access to provider-specific features (Anthropic structured outputs, Gemini's `CountTokens`, OpenAI's JSON mode)
- No transitive dependency bloat from unused framework features
- Each adapter is ~100-150 lines — easy to audit, debug, and replace
- Type-safe, idiomatic Go throughout

### SDK Dependencies

| Provider | Package | Purpose |
|----------|---------|---------|
| **Anthropic** | `github.com/anthropics/anthropic-sdk-go` | Claude models. Primary provider for MVP. Official SDK with structured output support, token counting API, and streaming. |
| **OpenAI** | `github.com/sashabaranov/go-openai` | GPT models. Also covers **OpenRouter** and any OpenAI-compatible endpoint via base URL override. Mature (10k+ stars), structured output via JSON schema. |
| **Google Gemini** | `google.golang.org/genai` | Gemini models. Official Google SDK (replaces deprecated `github.com/google/generative-ai-go`). Supports structured output via `response_mime_type: application/json` with schema constraints. |

### Why Not langchaingo?

langchaingo (8.6k stars, v0.1.x) was evaluated and rejected:

- **Abstraction mismatch**: LangChain's chains/agents/memory paradigm adds complexity without benefit for Mathiz's focused use case (structured JSON generation).
- **Stringly-typed config**: Heavy use of `map[string]any` is un-idiomatic Go and loses type safety.
- **Structured output gaps**: JSON schema support varies across its provider sub-implementations — not uniformly first-class.
- **Dependency weight**: Pulls in dozens of transitive dependencies for features Mathiz doesn't use (vector stores, embeddings, RAG).

Individual SDKs are simpler, lighter, and give full access to each provider's structured output capabilities.

---

## 3. Core Interface

### Provider Interface

```go
// internal/llm/provider.go

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
    // e.g. "claude-sonnet-4-20250514", "gpt-4o", "gemini-2.0-flash"
    ModelID() string
}
```

> **No streaming in MVP.** All Mathiz LLM calls produce structured JSON objects (questions, hints, lessons). These are short responses (typically <500 tokens) that don't benefit from streaming. The interface is designed so streaming can be added later (a `GenerateStream` method) without breaking existing consumers.

### Request

```go
// Request describes what to send to the LLM.
type Request struct {
    // System is the system prompt. Sets the LLM's role and constraints.
    System string

    // Messages is the conversation history. For single-turn generation
    // (the common case in Mathiz), this contains one user message.
    Messages []Message

    // Schema is the JSON Schema the response must conform to.
    // When set, the provider uses its native structured output mechanism
    // (Anthropic tool_use, OpenAI response_format, Gemini response_mime_type).
    // When nil, the response Content is raw text as json.RawMessage.
    Schema *Schema

    // MaxTokens is the maximum number of tokens in the response.
    // Required. Each consumer sets this based on expected output size.
    MaxTokens int

    // Temperature controls randomness. Range: 0.0 - 1.0.
    // Default: 0.0 (deterministic) when not set.
    Temperature float64
}

// Message represents a single message in the conversation.
type Message struct {
    Role    Role   // "user" or "assistant"
    Content string
}

// Role is the message sender role.
type Role string

const (
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
)
```

### Schema

```go
// Schema defines the JSON structure expected from the LLM.
// It is a simplified representation that each provider adapter translates
// to its native format (Anthropic tool input_schema, OpenAI JSON schema,
// Gemini response schema).
type Schema struct {
    // Name identifies this schema (used as tool name for Anthropic,
    // schema name for OpenAI). Kebab-case, e.g. "math-question".
    Name string

    // Description is a human-readable description of what this schema
    // represents. Sent to the LLM to guide generation.
    Description string

    // Definition is the JSON Schema definition as a map.
    // Example: {"type": "object", "properties": {...}, "required": [...]}
    // This is passed directly to the provider's schema mechanism.
    Definition map[string]any
}
```

### Response

```go
// Response holds the LLM's output.
type Response struct {
    // Content is the generated output. When a Schema was provided in the
    // request, this is the validated JSON object. When no Schema was
    // provided, this is the raw text response wrapped as a JSON string.
    Content json.RawMessage

    // Usage reports token consumption for this request.
    Usage Usage

    // Model is the actual model that served the request (may differ from
    // the configured model if the provider performed fallback).
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
```

### Errors

```go
// Error types for consumer error handling.

// ErrRateLimit indicates the provider returned a rate limit error (429).
// Callers can retry after the suggested delay.
type ErrRateLimit struct {
    RetryAfter time.Duration
    Err        error
}

// ErrInvalidResponse indicates the LLM returned content that does not
// conform to the requested schema.
type ErrInvalidResponse struct {
    Content json.RawMessage // The raw content that failed validation
    Err     error           // The validation error
}

// ErrProviderUnavailable indicates the provider is down or unreachable.
type ErrProviderUnavailable struct {
    Err error
}

// ErrMaxTokensExceeded indicates the response was truncated because it
// hit the MaxTokens limit. The content may be incomplete/invalid JSON.
type ErrMaxTokensExceeded struct {
    Content json.RawMessage
}
```

All error types implement the `error` interface and support `errors.Is` / `errors.As` unwrapping.

---

## 4. Provider Adapters

Each adapter implements the `Provider` interface by translating between Mathiz's types and the provider SDK's types.

### 4.1 Anthropic Adapter

```go
// internal/llm/anthropic.go

type AnthropicProvider struct {
    client *anthropic.Client
    model  string
}

func NewAnthropicProvider(cfg AnthropicConfig) (*AnthropicProvider, error)
```

**Structured output strategy**: Uses Anthropic's native structured output via `response_format` with JSON schema. The `Schema.Definition` is passed as the JSON schema definition. The response is extracted from the structured content block.

**Token counting**: Uses the Anthropic SDK's `CountTokens` API endpoint for accurate pre-flight token estimation (useful for cost tracking).

**Model mapping**:

| Mathiz config | Anthropic model ID |
|--------------|-------------------|
| `claude-sonnet` | `claude-sonnet-4-20250514` |
| `claude-haiku` | `claude-haiku-4-5-20251001` |

### 4.2 OpenAI Adapter

```go
// internal/llm/openai.go

type OpenAIProvider struct {
    client *openai.Client
    model  string
}

func NewOpenAIProvider(cfg OpenAIConfig) (*OpenAIProvider, error)
```

**Structured output strategy**: Uses OpenAI's `response_format` with `type: "json_schema"` and the `Schema.Definition` as the schema.

**OpenRouter support**: Setting `BaseURL` in config to `https://openrouter.ai/api/v1` makes this adapter work with any OpenRouter model. The same `OpenAIProvider` struct is reused — OpenRouter is API-compatible.

**Token counting**: Uses `github.com/pkoukk/tiktoken-go` for client-side token estimation. This is an approximation (tokenizer may not exactly match the server's) but is sufficient for budgeting.

**Model mapping**:

| Mathiz config | OpenAI model ID |
|--------------|----------------|
| `gpt-4o` | `gpt-4o` |
| `gpt-4o-mini` | `gpt-4o-mini` |

### 4.3 Gemini Adapter

```go
// internal/llm/gemini.go

type GeminiProvider struct {
    client *genai.Client
    model  string
}

func NewGeminiProvider(cfg GeminiConfig) (*GeminiProvider, error)
```

**Structured output strategy**: Uses Gemini's `ResponseMIMEType: "application/json"` with `ResponseSchema` set to the JSON schema from `Schema.Definition`.

**Token counting**: Uses Gemini's native `CountTokens` API method.

**Model mapping**:

| Mathiz config | Gemini model ID |
|--------------|----------------|
| `gemini-flash` | `gemini-2.0-flash` |
| `gemini-pro` | `gemini-2.0-pro` |

### 4.4 Mock Provider (for testing)

```go
// internal/llm/mock.go

type MockProvider struct {
    // Responses is a queue of canned responses. Generate() pops from the
    // front. If the queue is empty, it returns ErrProviderUnavailable.
    Responses []MockResponse

    // Calls records every request for assertion in tests.
    Calls []Request
}

type MockResponse struct {
    Content json.RawMessage
    Usage   Usage
    Err     error
}

func NewMockProvider(responses ...MockResponse) *MockProvider
```

The mock provider:
- Returns canned responses in FIFO order
- Records all requests for test assertions (verify prompts, schemas, parameters)
- Can simulate errors (rate limits, invalid responses, timeouts) by setting `Err` on a `MockResponse`
- Is deterministic — no randomness, no network calls

---

## 5. Configuration

### Config Structure

```go
// internal/llm/config.go

// Config holds all LLM provider configuration.
type Config struct {
    // Provider selects which LLM provider to use.
    // Values: "anthropic", "openai", "gemini"
    Provider string

    // Anthropic holds Anthropic-specific configuration.
    Anthropic AnthropicConfig

    // OpenAI holds OpenAI-specific configuration.
    OpenAI OpenAIConfig

    // Gemini holds Gemini-specific configuration.
    Gemini GeminiConfig

    // Retry configures retry behavior for transient failures.
    Retry RetryConfig

    // Timeout is the maximum duration for a single LLM request
    // (including retries). Default: 30s.
    Timeout time.Duration
}

type AnthropicConfig struct {
    APIKey string // Required. Env: MATHIZ_ANTHROPIC_API_KEY
    Model  string // Default: "claude-sonnet"
}

type OpenAIConfig struct {
    APIKey  string // Required. Env: MATHIZ_OPENAI_API_KEY
    Model   string // Default: "gpt-4o-mini"
    BaseURL string // Optional. Override for OpenRouter or compatible APIs.
}

type GeminiConfig struct {
    APIKey string // Required. Env: MATHIZ_GEMINI_API_KEY
    Model  string // Default: "gemini-flash"
}

type RetryConfig struct {
    MaxAttempts int           // Default: 3
    InitialWait time.Duration // Default: 1s
    MaxWait     time.Duration // Default: 10s
    Multiplier  float64       // Default: 2.0 (exponential backoff)
}
```

### Resolution Order

Configuration values are resolved in this order (highest priority first):

1. **CLI flags**: `--llm-provider`, `--llm-model` (for quick override)
2. **Environment variables**: `MATHIZ_LLM_PROVIDER`, `MATHIZ_ANTHROPIC_API_KEY`, etc.
3. **Defaults**: Anthropic provider, claude-sonnet model

### API Key Management

API keys are **never** stored in the database or config files. They are read exclusively from environment variables:

| Variable | Provider |
|----------|----------|
| `MATHIZ_ANTHROPIC_API_KEY` | Anthropic |
| `MATHIZ_OPENAI_API_KEY` | OpenAI (also used for OpenRouter) |
| `MATHIZ_GEMINI_API_KEY` | Gemini |

At startup, the selected provider's API key is validated (non-empty). If missing, the app exits with a clear error message naming the required environment variable.

---

## 6. Schema Validation Pipeline

Every LLM response goes through a validation pipeline before reaching the consumer.

### Pipeline Steps

```
LLM raw response
    │
    ▼
1. Parse as JSON (reject if invalid JSON)
    │
    ▼
2. Validate against Schema.Definition (reject if schema mismatch)
    │
    ▼
3. Return validated json.RawMessage to consumer
```

### Step 1: JSON Parse

The raw response string is parsed with `json.Unmarshal`. If parsing fails (malformed JSON, truncated output), the pipeline returns `ErrInvalidResponse` with the raw content attached for debugging.

### Step 2: Schema Validation

The parsed JSON is validated against the `Schema.Definition` using a lightweight JSON Schema validator. The schema is compiled once per schema definition and cached.

**Library**: `github.com/santhosh-tekuri/jsonschema/v6` — a well-maintained, spec-compliant JSON Schema validator for Go. Supports draft 2020-12.

If validation fails, the pipeline returns `ErrInvalidResponse` with both the raw content and the validation error message.

### Step 3: Return

The validated `json.RawMessage` is returned in the `Response`. The consumer unmarshals it into their domain-specific struct (e.g., `QuestionOutput`, `HintOutput`).

### Provider-Level vs. Application-Level Validation

Modern LLM APIs (Anthropic structured outputs, OpenAI JSON mode) enforce schema conformance **at the model level** — the LLM is constrained to only produce valid JSON matching the schema. This means:

- **Most responses will pass validation** because the provider already enforced the schema.
- **Application-level validation is a safety net** for edge cases: provider bugs, model hallucinations in field values, schema version mismatches, or fallback providers that don't support native structured output.
- The validation step is cheap (microseconds) and provides defense-in-depth.

---

## 7. Retry & Resilience

### Retry Decorator

Retry logic is implemented as a **decorator** that wraps any `Provider`:

```go
// internal/llm/retry.go

type RetryProvider struct {
    inner  Provider
    config RetryConfig
}

func WithRetry(p Provider, cfg RetryConfig) Provider {
    return &RetryProvider{inner: p, config: cfg}
}
```

`RetryProvider.Generate()` calls `inner.Generate()` and retries on transient errors:

| Error Type | Retry? | Notes |
|-----------|--------|-------|
| `ErrRateLimit` | Yes | Wait for `RetryAfter` duration, then retry |
| Network timeout | Yes | Exponential backoff |
| HTTP 5xx | Yes | Exponential backoff |
| `ErrInvalidResponse` | Yes (once) | Re-request; LLM may produce valid output on retry |
| `ErrMaxTokensExceeded` | No | Indicates the request needs a higher `MaxTokens`, not a retry |
| HTTP 4xx (except 429) | No | Client error, retrying won't help |
| Context cancelled | No | Caller intentionally cancelled |

### Backoff Strategy

Exponential backoff with jitter:

```
wait = min(InitialWait * Multiplier^attempt + jitter, MaxWait)
```

Jitter is ±20% of the computed wait time, preventing thundering herd if multiple requests fail simultaneously.

### Timeout

Each `Generate` call respects the `context.Context` deadline. The `Config.Timeout` is applied as a default if the caller doesn't set one:

```go
if _, ok := ctx.Deadline(); !ok {
    var cancel context.CancelFunc
    ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
    defer cancel()
}
```

---

## 8. Event Logging

Every LLM interaction is logged as an event in the persistence layer's event log. This enables:

- **Cost tracking**: Sum `InputTokens` + `OutputTokens` over time
- **Debugging**: Inspect exact prompts and responses for failed generations
- **Analytics**: Track which models/providers are used, success rates, latencies

### Event Schema

```go
// LLMRequestEvent — appended to the event log for every Generate call.
// Defined as an ent schema using the EventMixin from spec 02.

type LLMRequestEvent struct {
    // Base fields from EventMixin: id, sequence, timestamp

    Provider     string          // "anthropic", "openai", "gemini"
    Model        string          // Actual model ID used
    Purpose      string          // Consumer-provided label: "question-gen", "hint", "lesson", "diagnosis"
    InputTokens  int             // Tokens in the request
    OutputTokens int             // Tokens in the response
    LatencyMs    int64           // Wall-clock time for the request
    Success      bool            // Whether the request succeeded
    ErrorMessage string          // Error message if failed (empty on success)
}
```

**What is NOT logged**: The full prompt and response content are not stored in the event log by default (they can be large and contain generated questions that shouldn't be cached as-is). A debug mode flag (`MATHIZ_LLM_DEBUG=1`) enables full prompt/response logging to a separate debug log file for troubleshooting.

### Purpose Labels

Consumers pass a `Purpose` string via context to identify why they're calling the LLM:

```go
// internal/llm/context.go

type contextKey string

const purposeKey contextKey = "llm_purpose"

func WithPurpose(ctx context.Context, purpose string) context.Context {
    return context.WithValue(ctx, purposeKey, purpose)
}

func PurposeFrom(ctx context.Context) string {
    if v, ok := ctx.Value(purposeKey).(string); ok {
        return v
    }
    return "unknown"
}
```

Standard purpose labels:

| Label | Consumer |
|-------|----------|
| `"question-gen"` | Problem Generation |
| `"hint"` | AI Lessons |
| `"lesson"` | AI Lessons |
| `"diagnosis"` | Error Diagnosis |
| `"compression"` | Context Compression |
| `"diagnostic-placement"` | Diagnostic Placement |

---

## 9. Factory & Initialization

### Provider Factory

```go
// internal/llm/factory.go

// NewProvider creates a Provider from configuration.
// It returns the provider wrapped with retry and logging middleware.
func NewProvider(cfg Config, eventRepo store.EventRepo) (Provider, error) {
    var base Provider
    var err error

    switch cfg.Provider {
    case "anthropic":
        base, err = NewAnthropicProvider(cfg.Anthropic)
    case "openai":
        base, err = NewOpenAIProvider(cfg.OpenAI)
    case "gemini":
        base, err = NewGeminiProvider(cfg.Gemini)
    case "mock":
        return NewMockProvider(), nil // No middleware for mock
    default:
        return nil, fmt.Errorf("unknown LLM provider: %q", cfg.Provider)
    }
    if err != nil {
        return nil, fmt.Errorf("initializing %s provider: %w", cfg.Provider, err)
    }

    // Wrap with middleware: retry → logging → base
    logged := WithLogging(base, eventRepo)
    retried := WithRetry(logged, cfg.Retry)

    return retried, nil
}
```

### Middleware Stack

The provider is wrapped in decorators, outermost first:

```
Caller → RetryProvider → LoggingProvider → AnthropicProvider (base)
```

1. **RetryProvider**: Catches transient errors, retries with backoff
2. **LoggingProvider**: Records every request as an `LLMRequestEvent`
3. **Base provider**: Translates to SDK calls

### App Integration

The provider is created once at startup and injected into consumer modules:

```go
// In cmd/play.go or internal/app/app.go setup:

llmProvider, err := llm.NewProvider(llmConfig, eventRepo)
if err != nil {
    // Exit with clear error (e.g., "missing MATHIZ_ANTHROPIC_API_KEY")
}

// Inject into consumers:
questionGen := problemgen.New(llmProvider, skillGraph)
hintGen := lessons.New(llmProvider)
diagnoser := diagnosis.New(llmProvider)
```

---

## 10. Consumer Usage Pattern

This section shows how downstream modules interact with the LLM layer. These are illustrative examples — the full prompt templates and output schemas are defined in their respective specs (05, 09, 10).

### Example: Problem Generation

```go
// internal/problemgen/generate.go (spec 05 defines this fully)

func (g *Generator) GenerateQuestion(ctx context.Context, skill skillgraph.Skill, tier skillgraph.Tier) (*Question, error) {
    ctx = llm.WithPurpose(ctx, "question-gen")

    resp, err := g.provider.Generate(ctx, llm.Request{
        System: "You are a math tutor creating practice problems for children in grades 3-5. " +
                "Generate a single math problem appropriate for the given skill and difficulty.",
        Messages: []llm.Message{
            {Role: llm.RoleUser, Content: fmt.Sprintf(
                "Skill: %s\nDescription: %s\nKeywords: %s\nGrade: %d\nDifficulty: %s",
                skill.Name, skill.Description,
                strings.Join(skill.Keywords, ", "),
                skill.GradeLevel, tier,
            )},
        },
        Schema: &llm.Schema{
            Name:        "math-question",
            Description: "A single math practice question with answer",
            Definition: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "question":     map[string]any{"type": "string"},
                    "answer":       map[string]any{"type": "string"},
                    "answer_type":  map[string]any{"type": "string", "enum": []any{"integer", "decimal", "fraction"}},
                    "difficulty":   map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
                    "hint":         map[string]any{"type": "string"},
                },
                "required": []any{"question", "answer", "answer_type", "difficulty"},
            },
        },
        MaxTokens:   256,
        Temperature: 0.7,
    })
    if err != nil {
        return nil, fmt.Errorf("generating question: %w", err)
    }

    var q Question
    if err := json.Unmarshal(resp.Content, &q); err != nil {
        return nil, fmt.Errorf("parsing question response: %w", err)
    }
    return &q, nil
}
```

### Example: Hint Generation

```go
// internal/lessons/hint.go (spec 10 defines this fully)

func (h *HintGenerator) GenerateHint(ctx context.Context, question, wrongAnswer string, skill skillgraph.Skill) (*Hint, error) {
    ctx = llm.WithPurpose(ctx, "hint")

    resp, err := h.provider.Generate(ctx, llm.Request{
        System: "You are a patient math tutor helping a child who got a problem wrong. " +
                "Give a short, encouraging hint without revealing the answer.",
        Messages: []llm.Message{
            {Role: llm.RoleUser, Content: fmt.Sprintf(
                "The student was working on: %s\nQuestion: %s\nTheir answer: %s\nCorrect answer is not given to you.",
                skill.Name, question, wrongAnswer,
            )},
        },
        Schema: &llm.Schema{
            Name:        "math-hint",
            Description: "A scaffolded hint for a math problem",
            Definition: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "hint_text":  map[string]any{"type": "string"},
                    "strategy":   map[string]any{"type": "string"},
                },
                "required": []any{"hint_text", "strategy"},
            },
        },
        MaxTokens:   128,
        Temperature: 0.3,
    })
    if err != nil {
        return nil, err
    }

    var hint Hint
    if err := json.Unmarshal(resp.Content, &hint); err != nil {
        return nil, err
    }
    return &hint, nil
}
```

---

## 11. Package Structure

```
internal/
  llm/
    provider.go         # Provider interface, Request, Response, Message, Schema, Usage types
    errors.go           # Error types (ErrRateLimit, ErrInvalidResponse, etc.)
    config.go           # Config structs, defaults, env resolution
    factory.go          # NewProvider factory function
    context.go          # WithPurpose, PurposeFrom context helpers
    validate.go         # JSON schema validation pipeline
    retry.go            # RetryProvider decorator
    logging.go          # LoggingProvider decorator (writes LLMRequestEvent)
    anthropic.go        # Anthropic adapter
    openai.go           # OpenAI adapter (also covers OpenRouter)
    gemini.go           # Gemini adapter
    mock.go             # Mock provider for tests
    provider_test.go    # Interface contract tests (run against mock)
    anthropic_test.go   # Anthropic adapter unit tests (mock HTTP)
    openai_test.go      # OpenAI adapter unit tests (mock HTTP)
    gemini_test.go      # Gemini adapter unit tests (mock HTTP)
    validate_test.go    # Schema validation tests
    retry_test.go       # Retry logic tests
```

---

## 12. Testing Strategy

### 12.1 Interface Contract Tests

Tests that verify any `Provider` implementation satisfies the contract. Run against `MockProvider` in CI; can be run against real providers manually.

```go
func TestProviderContract(t *testing.T, p llm.Provider) {
    // 1. Generate with schema returns valid JSON matching the schema
    // 2. Generate without schema returns raw text as json.RawMessage
    // 3. ModelID returns a non-empty string
    // 4. Context cancellation returns context.Canceled
    // 5. Schema validation rejects non-conforming responses
}
```

### 12.2 Adapter Unit Tests

Each adapter gets unit tests using **HTTP mocks** (intercepting the SDK's HTTP client):

- **Happy path**: Provider returns valid structured output → adapter returns correct `Response`
- **Rate limit**: Provider returns 429 → adapter returns `ErrRateLimit` with `RetryAfter`
- **Server error**: Provider returns 500 → adapter returns `ErrProviderUnavailable`
- **Invalid JSON**: Provider returns malformed JSON → adapter returns `ErrInvalidResponse`
- **Max tokens**: Provider indicates truncation → adapter returns `ErrMaxTokensExceeded`
- **Token counting**: Usage fields are correctly extracted from provider response

### 12.3 Retry Tests

- Transient error → retried up to `MaxAttempts`, succeeds on Nth attempt
- Permanent error → not retried, returned immediately
- Rate limit with `RetryAfter` → waits the specified duration
- Context timeout → stops retrying, returns context error
- Backoff timing → each retry waits longer (exponential)

### 12.4 Validation Tests

- Valid JSON matching schema → passes
- Valid JSON not matching schema → `ErrInvalidResponse` with details
- Invalid JSON (malformed) → `ErrInvalidResponse`
- Null/empty response → `ErrInvalidResponse`
- Schema with nested objects, arrays, enums → all validated correctly

### 12.5 Integration Tests (Manual / CI-Optional)

Tagged with `//go:build integration` so they don't run in normal CI:

```go
//go:build integration

func TestAnthropicLive(t *testing.T) {
    // Requires MATHIZ_ANTHROPIC_API_KEY
    // Makes a real API call with a simple schema
    // Verifies response parses and validates correctly
}
```

---

## 13. Cost & Token Budgets

### Per-Request Budgets

Each consumer type has a recommended `MaxTokens` budget:

| Purpose | MaxTokens | Typical Output | Notes |
|---------|-----------|----------------|-------|
| Question generation | 256 | ~100-150 tokens | Single question + answer + metadata |
| Hint | 128 | ~50-80 tokens | Short, focused hint |
| Micro-lesson | 512 | ~200-400 tokens | Explanation + worked example |
| Diagnosis | 256 | ~100-150 tokens | Misconception analysis |
| Context compression | 512 | ~200-400 tokens | Session summary |

### Cost Tracking

The `LoggingProvider` records `InputTokens` and `OutputTokens` for every request. Consumers can query the event log to compute:

- Total tokens used per session
- Total tokens used per day/week
- Cost estimate (tokens × per-token price for the active model)

No hard spending limits are enforced in MVP. The event log provides visibility; alerts can be added later.

---

## 14. Future Considerations

These items are explicitly **out of scope** for the initial implementation but the architecture supports them:

- **Streaming**: Add `GenerateStream(ctx, req) (<-chan StreamEvent, error)` to the `Provider` interface. Useful if Mathiz adds longer-form content (explanations, stories). Each adapter already uses an SDK that supports streaming.
- **Fallback chains**: A `FallbackProvider` decorator that tries a primary provider and falls back to a secondary on failure. Same decorator pattern as `RetryProvider`.
- **Prompt caching**: Anthropic and other providers support prompt caching (beta). The adapter can set cache headers when the system prompt is reused across calls. This reduces cost and latency for repeated calls with the same system prompt.
- **Batching**: Generate multiple questions in a single LLM call by requesting an array in the schema. Reduces latency for session pre-loading. The current interface supports this (schema can define an array type).
- **Model routing**: Choose different models for different purposes (cheaper model for hints, more capable model for diagnosis). The `Config` could support per-purpose model overrides.
- **Spending limits**: Hard cap on daily/weekly token usage. Check the event log before each request; reject if budget exceeded.

---

## 15. Verification

The LLM integration module is verified when:

- [ ] `internal/llm/provider.go` defines the `Provider` interface with `Generate` and `ModelID`
- [ ] `internal/llm/config.go` resolves config from env vars with correct defaults
- [ ] `internal/llm/anthropic.go` implements `Provider` using `anthropic-sdk-go`
- [ ] `internal/llm/openai.go` implements `Provider` using `go-openai` (works with OpenRouter via BaseURL)
- [ ] `internal/llm/gemini.go` implements `Provider` using `google.golang.org/genai`
- [ ] `internal/llm/mock.go` provides a deterministic mock for testing
- [ ] `internal/llm/validate.go` validates responses against JSON schemas
- [ ] `internal/llm/retry.go` retries transient errors with exponential backoff
- [ ] `internal/llm/logging.go` records `LLMRequestEvent` for every request
- [ ] `internal/llm/factory.go` creates the correct provider with middleware stack
- [ ] All tests pass: `go test ./internal/llm/...`
- [ ] Schema validation rejects invalid JSON and non-conforming responses
- [ ] Retry logic handles rate limits, server errors, and context cancellation correctly
- [ ] Mock provider records calls and returns canned responses for consumer tests
- [ ] `CGO_ENABLED=0 go build ./...` succeeds with all new dependencies
