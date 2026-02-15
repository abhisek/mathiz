package lessons

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/abhisek/mathiz/internal/llm"
)

// Compressor handles context compression for session and snapshot levels.
type Compressor struct {
	provider llm.Provider
	cfg      CompressorConfig
}

// NewCompressor creates a context compressor.
func NewCompressor(provider llm.Provider, cfg CompressorConfig) *Compressor {
	return &Compressor{provider: provider, cfg: cfg}
}

// CompressErrors compresses a skill's error history into a summary.
// Runs asynchronously. The callback receives the compressed summary.
func (c *Compressor) CompressErrors(
	ctx context.Context,
	skillID string,
	errors []string,
	cb func(skillID string, summary string),
) {
	go func() {
		summary, err := c.compressSession(ctx, errors)
		if err != nil || cb == nil {
			return
		}
		cb(skillID, summary)
	}()
}

type compressionOutput struct {
	Summary string `json:"summary"`
}

func (c *Compressor) compressSession(ctx context.Context, errors []string) (string, error) {
	ctx = llm.WithPurpose(ctx, "session-compress")

	userMsg := buildCompressionUserMessage(errors)

	req := llm.Request{
		System: compressionSystemPrompt,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: userMsg},
		},
		Schema:      SessionCompressionSchema,
		MaxTokens:   c.cfg.SessionMaxTokens,
		Temperature: c.cfg.Temperature,
	}

	resp, err := c.provider.Generate(ctx, req)
	if err != nil {
		return "", fmt.Errorf("session compression: %w", err)
	}

	var out compressionOutput
	if err := json.Unmarshal(resp.Content, &out); err != nil {
		return "", fmt.Errorf("parse compression response: %w", err)
	}

	return out.Summary, nil
}

type profileOutput struct {
	Summary    string   `json:"summary"`
	Strengths  []string `json:"strengths"`
	Weaknesses []string `json:"weaknesses"`
	Patterns   []string `json:"patterns"`
}

// GenerateProfile creates a learner profile from session and mastery data.
func (c *Compressor) GenerateProfile(ctx context.Context, input ProfileInput) (*LearnerProfile, error) {
	ctx = llm.WithPurpose(ctx, "profile")

	userMsg := buildProfileUserMessage(input)

	req := llm.Request{
		System: profileSystemPrompt,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: userMsg},
		},
		Schema:      ProfileSchema,
		MaxTokens:   c.cfg.ProfileMaxTokens,
		Temperature: c.cfg.Temperature,
	}

	resp, err := c.provider.Generate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("profile generation: %w", err)
	}

	var out profileOutput
	if err := json.Unmarshal(resp.Content, &out); err != nil {
		return nil, fmt.Errorf("parse profile response: %w", err)
	}

	return &LearnerProfile{
		Summary:     out.Summary,
		Strengths:   out.Strengths,
		Weaknesses:  out.Weaknesses,
		Patterns:    out.Patterns,
		GeneratedAt: time.Now(),
	}, nil
}
