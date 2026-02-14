package problemgen

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/abhisek/mathiz/internal/llm"
)

// LLMGenerator implements Generator using the LLM provider.
type LLMGenerator struct {
	provider llm.Provider
	config   Config
}

// New creates a new LLMGenerator with the given provider and config.
func New(provider llm.Provider, cfg Config) *LLMGenerator {
	return &LLMGenerator{provider: provider, config: cfg}
}

// questionOutput is the raw LLM response before validation.
type questionOutput struct {
	QuestionText string   `json:"question_text"`
	Format       string   `json:"format"`
	Answer       string   `json:"answer"`
	AnswerType   string   `json:"answer_type"`
	Choices      []string `json:"choices"`
	Hint         string   `json:"hint"`
	Difficulty   int      `json:"difficulty"`
	Explanation  string   `json:"explanation"`
}

// Generate produces a single question for the given input context.
func (g *LLMGenerator) Generate(ctx context.Context, input GenerateInput) (*Question, error) {
	ctx = llm.WithPurpose(ctx, "question-gen")

	userMsg := buildUserMessage(input, g.config)

	req := llm.Request{
		System: systemPrompt,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: userMsg},
		},
		Schema:      QuestionSchema,
		MaxTokens:   g.config.MaxTokens,
		Temperature: g.config.Temperature,
	}

	resp, err := g.provider.Generate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	var raw questionOutput
	if err := json.Unmarshal(resp.Content, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	q := &Question{
		Text:        raw.QuestionText,
		Format:      AnswerFormat(raw.Format),
		Answer:      raw.Answer,
		AnswerType:  AnswerType(raw.AnswerType),
		Choices:     raw.Choices,
		Hint:        raw.Hint,
		Difficulty:  raw.Difficulty,
		Explanation: raw.Explanation,
		SkillID:     input.Skill.ID,
		Tier:        input.Tier,
	}

	// Run validators in order.
	for _, v := range g.config.Validators {
		if verr := v.Validate(q, input); verr != nil {
			return nil, verr
		}
	}

	return q, nil
}
