package problemgen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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

// maxValidationRetries is how many more times Generate asks after a
// validator rejects a question as retryable (specs/19 §3.1). Provider
// errors are retried by llm.RetryProvider, not here.
const maxValidationRetries = 2

// minRetryBudget is the least time a validation retry needs to be worth
// starting: a retry that the deadline would cut short only burns a call.
const minRetryBudget = 5 * time.Second

// Generate produces a single question for the given input context. A
// question a validator rejects as retryable is asked for again, with the
// rejected text added to the prior questions so it isn't repeated.
func (g *LLMGenerator) Generate(ctx context.Context, input GenerateInput) (*Question, error) {
	ctx = llm.WithPurpose(ctx, llm.PurposeQuestionGen)

	for retry := 0; ; retry++ {
		q, err := g.generateOnce(ctx, input)
		var verr *ValidationError
		if err == nil || !errors.As(err, &verr) || !verr.Retryable ||
			retry == maxValidationRetries || !hasBudget(ctx, minRetryBudget) {
			return q, err
		}
		// Copy: the caller's slice belongs to its session state.
		prior := make([]string, 0, len(input.PriorQuestions)+1)
		input.PriorQuestions = append(append(prior, input.PriorQuestions...), verr.QuestionText)
	}
}

// hasBudget reports whether ctx has at least d left (no deadline = yes).
func hasBudget(ctx context.Context, d time.Duration) bool {
	deadline, ok := ctx.Deadline()
	return !ok || time.Until(deadline) >= d
}

// generateOnce is one LLM call plus the validator chain.
func (g *LLMGenerator) generateOnce(ctx context.Context, input GenerateInput) (*Question, error) {
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
			verr.QuestionText = q.Text
			return nil, verr
		}
	}

	// After validation so the checker sees exactly what the LLM claimed;
	// see ShuffleChoices for why this is mandatory.
	q.ShuffleChoices()

	return q, nil
}
