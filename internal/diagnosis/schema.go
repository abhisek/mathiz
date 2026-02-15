package diagnosis

import "github.com/abhisek/mathiz/internal/llm"

// DiagnosisSchema defines the JSON schema for LLM error diagnosis responses.
var DiagnosisSchema = &llm.Schema{
	Name:        "error-diagnosis",
	Description: "Classification of a wrong answer against a known misconception taxonomy",
	Definition: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"misconception_id": map[string]any{
				"type":        []any{"string", "null"},
				"description": "The ID of the matching misconception from the candidate list, or null if no match",
			},
			"confidence": map[string]any{
				"type":        "number",
				"minimum":     0.0,
				"maximum":     1.0,
				"description": "Confidence score (0.0–1.0) reflecting how well the error matches the misconception",
			},
			"reasoning": map[string]any{
				"type":        "string",
				"description": "Brief one-sentence explanation of why this misconception was identified (or why no match)",
			},
		},
		"required":             []any{"misconception_id", "confidence", "reasoning"},
		"additionalProperties": false,
	},
}
