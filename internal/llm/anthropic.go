package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// anthropicModels maps friendly names to Anthropic model IDs.
var anthropicModels = map[string]string{
	"claude-sonnet": "claude-sonnet-4-20250514",
	"claude-haiku":  "claude-haiku-4-5-20251001",
}

// AnthropicProvider implements Provider using the Anthropic SDK.
type AnthropicProvider struct {
	client *anthropic.Client
	model  string
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(cfg AnthropicConfig) (*AnthropicProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("anthropic API key is required")
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}

	client := anthropic.NewClient(opts...)
	model := resolveModel(cfg.Model, anthropicModels)

	return &AnthropicProvider{
		client: &client,
		model:  model,
	}, nil
}

func (p *AnthropicProvider) Generate(ctx context.Context, req Request) (*Response, error) {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: int64(req.MaxTokens),
		Messages:  buildAnthropicMessages(req.Messages),
	}

	if req.System != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.System},
		}
	}

	if req.Temperature > 0 {
		params.Temperature = anthropic.Float(req.Temperature)
	}

	// Use structured output via JSON output format when schema is provided.
	if req.Schema != nil {
		params.OutputConfig = anthropic.OutputConfigParam{
			Format: anthropic.JSONOutputFormatParam{
				Schema: req.Schema.Definition,
			},
		}
	}

	msg, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, mapAnthropicError(err)
	}

	content, err := extractAnthropicContent(msg)
	if err != nil {
		return nil, err
	}

	// Validate against schema if provided.
	if req.Schema != nil {
		if err := validateResponse(req.Schema, content); err != nil {
			return nil, err
		}
	}

	return &Response{
		Content:    content,
		Usage:      mapAnthropicUsage(msg.Usage),
		Model:      string(msg.Model),
		StopReason: mapAnthropicStopReason(msg.StopReason),
	}, nil
}

func (p *AnthropicProvider) ModelID() string {
	return p.model
}

func buildAnthropicMessages(msgs []Message) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, len(msgs))
	for i, m := range msgs {
		role := anthropic.MessageParamRoleUser
		if m.Role == RoleAssistant {
			role = anthropic.MessageParamRoleAssistant
		}
		out[i] = anthropic.MessageParam{
			Role: role,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock(m.Content),
			},
		}
	}
	return out
}

func extractAnthropicContent(msg *anthropic.Message) (json.RawMessage, error) {
	for _, block := range msg.Content {
		if block.Type == "text" {
			return json.RawMessage(block.Text), nil
		}
	}
	return nil, &ErrInvalidResponse{
		Err: fmt.Errorf("no text content in Anthropic response"),
	}
}

func mapAnthropicUsage(u anthropic.Usage) Usage {
	return Usage{
		InputTokens:  int(u.InputTokens),
		OutputTokens: int(u.OutputTokens),
		TotalTokens:  int(u.InputTokens + u.OutputTokens),
	}
}

func mapAnthropicStopReason(reason anthropic.StopReason) string {
	switch reason {
	case "end_turn":
		return "end"
	case "max_tokens":
		return "max_tokens"
	default:
		return "end"
	}
}

func mapAnthropicError(err error) error {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode == http.StatusTooManyRequests:
			return &ErrRateLimit{Err: err}
		case apiErr.StatusCode >= 500:
			return &ErrProviderUnavailable{Err: err}
		}
	}
	return &ErrProviderUnavailable{Err: err}
}

// resolveModel maps a friendly model name to a provider model ID.
func resolveModel(name string, models map[string]string) string {
	if id, ok := models[name]; ok {
		return id
	}
	// If not in the map, use as-is (allows direct model IDs).
	return name
}
