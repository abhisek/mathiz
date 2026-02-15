package lessons

import "github.com/abhisek/mathiz/internal/llm"

// LessonSchema defines the JSON schema for micro-lesson generation.
var LessonSchema = &llm.Schema{
	Name:        "micro-lesson",
	Description: "A micro-lesson with explanation, worked example, and practice question",
	Definition: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "Short title for the lesson (3-8 words)",
			},
			"explanation": map[string]any{
				"type":        "string",
				"description": "Clear, age-appropriate explanation of the concept (3-5 sentences)",
			},
			"worked_example": map[string]any{
				"type":        "string",
				"description": "Step-by-step solution to a similar problem, with numbered steps",
			},
			"practice_question": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "A simpler practice question for the student to try",
					},
					"answer": map[string]any{
						"type":        "string",
						"description": "The correct answer",
					},
					"answer_type": map[string]any{
						"type": "string",
						"enum": []any{"integer", "decimal", "fraction"},
					},
					"explanation": map[string]any{
						"type":        "string",
						"description": "Brief explanation of the practice answer",
					},
				},
				"required":             []any{"text", "answer", "answer_type", "explanation"},
				"additionalProperties": false,
			},
		},
		"required":             []any{"title", "explanation", "worked_example", "practice_question"},
		"additionalProperties": false,
	},
}

// SessionCompressionSchema defines the JSON schema for error summary compression.
var SessionCompressionSchema = &llm.Schema{
	Name:        "error-summary",
	Description: "Compressed summary of a student's error patterns on a skill",
	Definition: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": "2-3 sentence summary of error patterns",
			},
		},
		"required":             []any{"summary"},
		"additionalProperties": false,
	},
}

// ProfileSchema defines the JSON schema for learner profile generation.
var ProfileSchema = &llm.Schema{
	Name:        "learner-profile",
	Description: "Holistic learner profile summarizing strengths, weaknesses, and patterns",
	Definition: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": "3-5 sentence overview of the learner's abilities",
			},
			"strengths": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "2-4 specific strengths (5-10 words each)",
			},
			"weaknesses": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "2-4 specific weaknesses (5-10 words each)",
			},
			"patterns": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "1-3 observed error patterns (5-10 words each)",
			},
		},
		"required":             []any{"summary", "strengths", "weaknesses", "patterns"},
		"additionalProperties": false,
	},
}
