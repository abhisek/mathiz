package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	openai "github.com/sashabaranov/go-openai"
)

// openaiModels maps friendly names to OpenAI model IDs.
var openaiModels = map[string]string{
	"gpt-4o":      "gpt-4o",
	"gpt-4o-mini": "gpt-4o-mini",
}

// OpenAIProvider implements Provider using the OpenAI SDK.
// It also supports OpenRouter and other OpenAI-compatible APIs via BaseURL.
type OpenAIProvider struct {
	client *openai.Client
	model  string
}

// NewOpenAIProvider creates a new OpenAI provider.
func NewOpenAIProvider(cfg OpenAIConfig) (*OpenAIProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openai API key is required")
	}

	config := openai.DefaultConfig(cfg.APIKey)
	if cfg.BaseURL != "" {
		config.BaseURL = cfg.BaseURL
	}

	client := openai.NewClientWithConfig(config)
	model := resolveModel(cfg.Model, openaiModels)

	return &OpenAIProvider{
		client: client,
		model:  model,
	}, nil
}

func (p *OpenAIProvider) Generate(ctx context.Context, req Request) (*Response, error) {
	messages := buildOpenAIMessages(req)

	chatReq := openai.ChatCompletionRequest{
		Model:               p.model,
		Messages:            messages,
		MaxCompletionTokens: req.MaxTokens,
		Temperature:         float32(req.Temperature),
	}

	// Use JSON schema response format when schema is provided.
	if req.Schema != nil {
		schemaBytes, err := json.Marshal(req.Schema.Definition)
		if err != nil {
			return nil, fmt.Errorf("marshal schema: %w", err)
		}

		chatReq.ResponseFormat = &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   req.Schema.Name,
				Schema: json.RawMessage(schemaBytes),
				Strict: true,
			},
		}
	}

	resp, err := p.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, mapOpenAIError(err)
	}

	if len(resp.Choices) == 0 {
		return nil, &ErrInvalidResponse{
			Err: fmt.Errorf("no choices in OpenAI response"),
		}
	}

	content := json.RawMessage(resp.Choices[0].Message.Content)

	// Validate against schema if provided.
	if req.Schema != nil {
		if err := validateResponse(req.Schema, content); err != nil {
			return nil, err
		}
	}

	return &Response{
		Content: content,
		Usage: Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		},
		Model:      resp.Model,
		StopReason: mapOpenAIStopReason(resp.Choices[0].FinishReason),
	}, nil
}

func (p *OpenAIProvider) ModelID() string {
	return p.model
}

func buildOpenAIMessages(req Request) []openai.ChatCompletionMessage {
	var messages []openai.ChatCompletionMessage

	if req.System != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: req.System,
		})
	}

	for _, m := range req.Messages {
		role := openai.ChatMessageRoleUser
		if m.Role == RoleAssistant {
			role = openai.ChatMessageRoleAssistant
		}
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    role,
			Content: m.Content,
		})
	}

	return messages
}

func mapOpenAIStopReason(reason openai.FinishReason) string {
	switch reason {
	case openai.FinishReasonStop:
		return "end"
	case openai.FinishReasonLength:
		return "max_tokens"
	default:
		return "end"
	}
}

func mapOpenAIError(err error) error {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.HTTPStatusCode == http.StatusTooManyRequests:
			return &ErrRateLimit{Err: err}
		case apiErr.HTTPStatusCode >= 500:
			return &ErrProviderUnavailable{Err: err}
		}
	}
	return &ErrProviderUnavailable{Err: err}
}
