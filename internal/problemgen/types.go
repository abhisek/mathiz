package problemgen

import "github.com/abhisek/mathiz/internal/skillgraph"

// Question represents a generated math question ready for display.
type Question struct {
	// Text is the question prompt displayed to the learner.
	// Plain ASCII text, e.g. "What is 345 + 278?" or "Which fraction is larger: 3/4 or 2/3?"
	Text string

	// Format indicates how the learner answers this question.
	Format AnswerFormat

	// Answer is the canonical correct answer as a string.
	// For numeric: "623", "0.75", "3/4"
	// For multiple choice: the text of the correct option (e.g. "3/4")
	Answer string

	// AnswerType describes the numeric type of the answer for validation.
	AnswerType AnswerType

	// Choices is populated only when Format is FormatMultipleChoice.
	// Contains exactly 4 options, one of which matches Answer.
	Choices []string

	// Hint is an optional short hint the learner can request (Learn tier only).
	// Empty string if no hint was generated.
	Hint string

	// Difficulty is the LLM's self-assessed difficulty (1-5).
	// Used for analytics, not for gating.
	Difficulty int

	// Explanation is a brief worked solution shown after the learner answers.
	// Always present.
	Explanation string

	// SkillID is the skill this question was generated for.
	SkillID string

	// Tier is the tier this question was generated for.
	Tier skillgraph.Tier
}

// AnswerType describes the numeric representation of the correct answer.
type AnswerType string

const (
	AnswerTypeInteger  AnswerType = "integer"  // e.g. "623", "-15"
	AnswerTypeDecimal  AnswerType = "decimal"  // e.g. "3.75", "0.5"
	AnswerTypeFraction AnswerType = "fraction" // e.g. "3/4", "7/2"
)

// AnswerFormat describes how the learner provides their answer.
type AnswerFormat string

const (
	// FormatNumeric means the learner types a numeric answer.
	FormatNumeric AnswerFormat = "numeric"

	// FormatMultipleChoice means the learner picks from 4 choices.
	FormatMultipleChoice AnswerFormat = "multiple_choice"
)

// GenerateInput holds all context needed to generate a question.
type GenerateInput struct {
	// Skill is the target skill for the question.
	Skill skillgraph.Skill

	// Tier is the difficulty tier (Learn or Prove).
	Tier skillgraph.Tier

	// PriorQuestions contains the Text of questions already asked in this
	// session for this skill. Used for deduplication in the prompt.
	PriorQuestions []string

	// RecentErrors contains descriptions of the learner's recent mistakes
	// on this skill (e.g. "answered 623 for 345 + 289, correct was 634").
	// Up to 5 most recent errors. Empty slice if no history.
	RecentErrors []string

	// LearnerProfile is an optional AI-generated summary of the learner.
	// Included in the prompt when available for better personalization.
	LearnerProfile string
}
