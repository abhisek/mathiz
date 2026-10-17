package lessons

import (
	"time"

	"github.com/abhisek/mathiz/internal/diagnosis"
	"github.com/abhisek/mathiz/internal/skillgraph"
)

// Lesson is an LLM-generated micro-lesson for a specific skill and error pattern.
type Lesson struct {
	SkillID          string
	Title            string
	Explanation      string
	WorkedExample    string
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

// ProfileInput holds all context for profile generation. The profile
// describes the whole child, so lifetime history leads and this session is
// only the newest evidence: one game expedition is five questions on one
// skill (FOU-241).
type ProfileInput struct {
	// MasteryData is every curriculum skill in the snapshot, keyed by skill
	// ID, with lifetime answer totals merged in.
	MasteryData map[string]MasteryDataSummary
	// SessionCount is how many sessions the child has completed.
	SessionCount int
	// RecentSessions are the newest completed sessions, newest first.
	RecentSessions []SessionSummary
	// ErrorPatterns are diagnosed wrong answers over a recent window, most
	// frequent first.
	ErrorPatterns []ErrorPatternSummary

	// PerSkillResults and ErrorHistory are this session's evidence.
	PerSkillResults map[string]SkillResultSummary
	ErrorHistory    map[string][]string

	PreviousProfile *LearnerProfile
}

// SkillResultSummary is a simplified skill result for profile generation.
// Name is the human-readable skill name; the map key is the skill ID, which
// means nothing to the model on its own.
type SkillResultSummary struct {
	Name      string
	Attempted int
	Correct   int
}

// MasteryDataSummary is one skill's standing for profile generation.
// Attempts and Correct are lifetime totals from the answer history: the
// snapshot's own counters restart when the skill changes tier or decays.
type MasteryDataSummary struct {
	Name         string
	Grade        int
	State        string
	Tier         string
	FluencyScore float64
	Attempts     int
	Correct      int
}

// SessionSummary is one completed session for profile generation.
type SessionSummary struct {
	EndedAt   time.Time
	Skills    []string // skill names
	Questions int
	Correct   int
	Quest     bool
}

// ErrorPatternSummary counts one diagnosed error type on one skill.
type ErrorPatternSummary struct {
	SkillID       string
	SkillName     string
	Category      string
	Misconception string // label from the taxonomy; "" when none was named
	Count         int
}
