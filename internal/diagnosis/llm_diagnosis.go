package diagnosis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/abhisek/mathiz/internal/llm"
)

// DiagnoserConfig holds configuration for the LLM diagnoser.
type DiagnoserConfig struct {
	MaxTokens   int
	Temperature float64
}

// DefaultDiagnoserConfig returns sensible defaults.
func DefaultDiagnoserConfig() DiagnoserConfig {
	return DiagnoserConfig{
		MaxTokens:   256,
		Temperature: 0.3,
	}
}

// Diagnoser performs LLM-based misconception identification.
type Diagnoser struct {
	provider llm.Provider
	cfg      DiagnoserConfig
}

// NewDiagnoser creates an LLM-based diagnoser.
func NewDiagnoser(provider llm.Provider, cfg DiagnoserConfig) *Diagnoser {
	return &Diagnoser{provider: provider, cfg: cfg}
}

// DiagnosisRequest is the input for LLM misconception identification.
type DiagnosisRequest struct {
	SkillID       string
	SkillName     string
	QuestionText  string
	CorrectAnswer string
	LearnerAnswer string
	AnswerType    string
	Candidates    []*Misconception
}

// diagnosisOutput is the raw LLM response.
type diagnosisOutput struct {
	MisconceptionID *string `json:"misconception_id"`
	Confidence      float64 `json:"confidence"`
	Reasoning       string  `json:"reasoning"`
}

// Diagnose sends a wrong answer to the LLM for misconception identification.
func (d *Diagnoser) Diagnose(ctx context.Context, req *DiagnosisRequest) (*DiagnosisResult, error) {
	ctx = llm.WithPurpose(ctx, "error-diagnosis")

	userMsg, err := buildDiagnosisMessage(req)
	if err != nil {
		return nil, fmt.Errorf("build diagnosis prompt: %w", err)
	}

	llmReq := llm.Request{
		System: diagnosisSystemPrompt,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: userMsg},
		},
		Schema:      DiagnosisSchema,
		MaxTokens:   d.cfg.MaxTokens,
		Temperature: d.cfg.Temperature,
	}

	resp, err := d.provider.Generate(ctx, llmReq)
	if err != nil {
		return nil, fmt.Errorf("LLM diagnosis failed: %w", err)
	}

	var raw diagnosisOutput
	if err := json.Unmarshal(resp.Content, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse diagnosis response: %w", err)
	}

	// Validate misconception_id if present.
	if raw.MisconceptionID != nil {
		valid := false
		for _, c := range req.Candidates {
			if c.ID == *raw.MisconceptionID {
				valid = true
				break
			}
		}
		if !valid {
			// LLM returned an ID not in the candidate list — treat as no match.
			return &DiagnosisResult{
				Category:       CategoryUnclassified,
				Confidence:     raw.Confidence,
				ClassifierName: "llm",
				Reasoning:      raw.Reasoning,
			}, nil
		}

		return &DiagnosisResult{
			Category:        CategoryMisconception,
			MisconceptionID: *raw.MisconceptionID,
			Confidence:      raw.Confidence,
			ClassifierName:  "llm",
			Reasoning:       raw.Reasoning,
		}, nil
	}

	return &DiagnosisResult{
		Category:       CategoryUnclassified,
		Confidence:     raw.Confidence,
		ClassifierName: "llm",
		Reasoning:      raw.Reasoning,
	}, nil
}

const diagnosisSystemPrompt = `You are an expert math education diagnostician. A learner answered a math question incorrectly. Your job is to determine if their error matches a known misconception pattern.

Instructions:
- If the learner's error clearly matches one of the listed misconceptions, return its ID.
- If the error does not match any listed misconception, return null for misconception_id.
- Do NOT invent new misconception IDs. Only use IDs from the list provided.
- Provide a confidence score (0.0–1.0) reflecting how well the error matches.
- Keep reasoning to one sentence.`

var diagnosisUserTemplate = template.Must(template.New("diagnosis").Parse(`Skill: {{.SkillName}}
Question: {{.QuestionText}}
Correct answer: {{.CorrectAnswer}}
Learner's answer: {{.LearnerAnswer}}
Answer type: {{.AnswerType}}

Known misconceptions for this strand:
{{range .Candidates}}- {{.ID}}: {{.Description}}
{{end}}`))

func buildDiagnosisMessage(req *DiagnosisRequest) (string, error) {
	var buf bytes.Buffer
	if err := diagnosisUserTemplate.Execute(&buf, req); err != nil {
		return "", err
	}
	return buf.String(), nil
}
