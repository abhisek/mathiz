package diagnosis

import "github.com/abhisek/mathiz/internal/problemgen"

// ErrorCategory classifies a wrong answer.
type ErrorCategory string

const (
	CategoryCareless      ErrorCategory = "careless"
	CategorySpeedRush     ErrorCategory = "speed-rush"
	CategoryMisconception ErrorCategory = "misconception"
	CategoryUnclassified  ErrorCategory = "unclassified"
)

// ClassifyInput holds the context for classification.
type ClassifyInput struct {
	Question       *problemgen.Question
	LearnerAnswer  string
	ResponseTimeMs int
	SkillAccuracy  float64 // Historical accuracy for this skill (0.0–1.0)
}

// DiagnosisResult is the output of classifying a wrong answer.
type DiagnosisResult struct {
	Category        ErrorCategory // careless, speed-rush, misconception, unclassified
	MisconceptionID string        // Non-empty only when Category == misconception
	Confidence      float64       // 0.0–1.0
	ClassifierName  string        // Which classifier/LLM produced this result
	Reasoning       string        // LLM reasoning (empty for rule-based)
}
