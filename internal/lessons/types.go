package lessons

import (
	"time"

	"github.com/abhisek/mathiz/internal/diagnosis"
	"github.com/abhisek/mathiz/internal/skillgraph"
)

// Lesson is an LLM-generated micro-lesson for a specific skill and error pattern.
type Lesson struct {
	SkillID       string
	Title         string
	Explanation   string
	WorkedExample string
	PracticeQuestion PracticeQuestion
}

// PracticeQuestion is a mini-practice embedded in a lesson.
type PracticeQuestion struct {
	Text        string
	Answer      string
	AnswerType  string // "integer", "decimal", "fraction"
	Explanation string
}

// LessonInput holds all context needed to generate a micro-lesson.
type LessonInput struct {
	Skill         skillgraph.Skill
	Tier          skillgraph.Tier
	RecentErrors  []string
	LastDiagnosis *diagnosis.DiagnosisResult
	Accuracy      float64
}

// LearnerProfile is a holistic summary of the learner's patterns.
type LearnerProfile struct {
	Summary     string
	Strengths   []string
	Weaknesses  []string
	Patterns    []string
	GeneratedAt time.Time
}

// ProfileInput holds all context for profile generation.
type ProfileInput struct {
	PerSkillResults map[string]SkillResultSummary
	MasteryData     map[string]MasteryDataSummary
	ErrorHistory    map[string][]string
	PreviousProfile *LearnerProfile
	SessionCount    int
}

// SkillResultSummary is a simplified skill result for profile generation.
type SkillResultSummary struct {
	Attempted int
	Correct   int
}

// MasteryDataSummary is a simplified mastery state for profile generation.
type MasteryDataSummary struct {
	State        string
	FluencyScore float64
}
