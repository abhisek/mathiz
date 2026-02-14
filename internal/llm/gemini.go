package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/genai"
)

// geminiModels maps friendly names to Gemini model IDs.
var geminiModels = map[string]string{
	"gemini-flash": "gemini-2.0-flash",
	"gemini-pro":   "gemini-2.0-pro",
}

// GeminiProvider implements Provider using the Google Gemini SDK.
type GeminiProvider struct {
	client *genai.Client
	model  string
}

// NewGeminiProvider creates a new Gemini provider.
func NewGeminiProvider(ctx context.Context, cfg GeminiConfig) (*GeminiProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("gemini API key is required")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create Gemini client: %w", err)
	}

	model := resolveModel(cfg.Model, geminiModels)

	return &GeminiProvider{
		client: client,
		model:  model,
	}, nil
}

func (p *GeminiProvider) Generate(ctx context.Context, req Request) (*Response, error) {
	config := &genai.GenerateContentConfig{
		MaxOutputTokens: int32(req.MaxTokens),
	}

	if req.Temperature > 0 {
		temp := float32(req.Temperature)
		config.Temperature = &temp
	}

	if req.System != "" {
		config.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: req.System}},
		}
	}

	// Configure structured output.
	if req.Schema != nil {
		config.ResponseMIMEType = "application/json"
		config.ResponseSchema = buildGeminiSchema(req.Schema.Definition)
	}

	contents := buildGeminiContents(req.Messages)

	result, err := p.client.Models.GenerateContent(ctx, p.model, contents, config)
	if err != nil {
		return nil, mapGeminiError(err)
	}

	content := json.RawMessage(result.Text())

	// Validate against schema if provided.
	if req.Schema != nil {
		if err := validateResponse(req.Schema, content); err != nil {
			return nil, err
		}
	}

	resp := &Response{
		Content:    content,
		Model:      p.model,
		StopReason: mapGeminiStopReason(result),
	}

	if result.UsageMetadata != nil {
		resp.Usage = Usage{
			InputTokens:  int(result.UsageMetadata.PromptTokenCount),
			OutputTokens: int(result.UsageMetadata.CandidatesTokenCount),
			TotalTokens:  int(result.UsageMetadata.TotalTokenCount),
		}
	}

	return resp, nil
}

func (p *GeminiProvider) ModelID() string {
	return p.model
}

func buildGeminiContents(msgs []Message) []*genai.Content {
	out := make([]*genai.Content, len(msgs))
	for i, m := range msgs {
		role := "user"
		if m.Role == RoleAssistant {
			role = "model"
		}
		out[i] = &genai.Content{
			Role:  role,
			Parts: []*genai.Part{{Text: m.Content}},
		}
	}
	return out
}

// buildGeminiSchema converts a JSON Schema definition map to a genai.Schema.
func buildGeminiSchema(def map[string]any) *genai.Schema {
	schema := &genai.Schema{}

	if t, ok := def["type"].(string); ok {
		schema.Type = mapGeminiType(t)
	}
	if desc, ok := def["description"].(string); ok {
		schema.Description = desc
	}

	if props, ok := def["properties"].(map[string]any); ok {
		schema.Properties = make(map[string]*genai.Schema)
		for k, v := range props {
			if propDef, ok := v.(map[string]any); ok {
				schema.Properties[k] = buildGeminiSchema(propDef)
			}
		}
	}

	if req, ok := def["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				schema.Required = append(schema.Required, s)
			}
		}
	}

	if enums, ok := def["enum"].([]any); ok {
		for _, e := range enums {
			if s, ok := e.(string); ok {
				schema.Enum = append(schema.Enum, s)
			}
		}
	}

	if items, ok := def["items"].(map[string]any); ok {
		schema.Items = buildGeminiSchema(items)
	}

	return schema
}

func mapGeminiType(t string) genai.Type {
	switch t {
	case "string":
		return genai.TypeString
	case "number":
		return genai.TypeNumber
	case "integer":
		return genai.TypeInteger
	case "boolean":
		return genai.TypeBoolean
	case "array":
		return genai.TypeArray
	case "object":
		return genai.TypeObject
	default:
		return genai.TypeString
	}
}

func mapGeminiStopReason(result *genai.GenerateContentResponse) string {
	if len(result.Candidates) > 0 {
		switch result.Candidates[0].FinishReason {
		case "STOP":
			return "end"
		case "MAX_TOKENS":
			return "max_tokens"
		}
	}
	return "end"
}

func mapGeminiError(err error) error {
	var apiErr *genai.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.Code == http.StatusTooManyRequests:
			return &ErrRateLimit{Err: err}
		case apiErr.Code >= 500:
			return &ErrProviderUnavailable{Err: err}
		}
	}
	return &ErrProviderUnavailable{Err: err}
}
