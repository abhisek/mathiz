package problemgen

import "github.com/abhisek/mathiz/internal/llm"

// QuestionSchema defines the JSON schema for LLM question generation responses.
var QuestionSchema = &llm.Schema{
	Name:        "math-question",
	Description: "A single math practice question with answer and explanation",
	Definition: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question_text": map[string]any{
				"type":        "string",
				"description": "The question prompt shown to the learner, in plain ASCII text",
			},
			"format": map[string]any{
				"type":        "string",
				"enum":        []any{"numeric", "multiple_choice"},
				"description": "How the learner answers: type a number or pick from choices",
			},
			"answer": map[string]any{
				"type":        "string",
				"description": "The correct answer. For numeric: the number as a string. For MC: the text of the correct option.",
			},
			"answer_type": map[string]any{
				"type":        "string",
				"enum":        []any{"integer", "decimal", "fraction"},
				"description": "The numeric type of the answer",
			},
			"choices": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Exactly 4 options for multiple_choice format. Empty array for numeric format.",
			},
			"hint": map[string]any{
				"type":        "string",
				"description": "A short scaffolding hint for the learner. Non-empty for learn tier, empty for prove tier.",
			},
			"difficulty": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"maximum":     5,
				"description": "Self-assessed difficulty from 1 (easy) to 5 (hard)",
			},
			"explanation": map[string]any{
				"type":        "string",
				"description": "Step-by-step worked solution, age-appropriate for a child",
			},
		},
		"required":             []any{"question_text", "format", "answer", "answer_type", "choices", "hint", "difficulty", "explanation"},
		"additionalProperties": false,
	},
}
