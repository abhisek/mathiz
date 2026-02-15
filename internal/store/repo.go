package store

import (
	"context"
	"time"
)

// QueryOpts configures event queries with filtering and pagination.
type QueryOpts struct {
	Limit  int       // max results (0 = unlimited)
	After  int64     // sequence > After
	Before int64     // sequence < Before
	From   time.Time // timestamp >= From
	To     time.Time // timestamp <= To
}

// SnapshotData captures the full learner state at a point in time.
// Domain modules register their state types here as they are implemented.
type SnapshotData struct {
	Version   int                    `json:"version"`
	Mastery   *MasterySnapshotData   `json:"mastery,omitempty"`
	SpacedRep *SpacedRepSnapshotData `json:"spaced_rep,omitempty"`

	// Deprecated: kept for migration only. New snapshots use Mastery field.
	TierProgress map[string]*TierProgressData `json:"tier_progress,omitempty"`
	MasteredSet  []string                     `json:"mastered_set,omitempty"`
}

// SpacedRepSnapshotData holds all spaced repetition state for persistence.
type SpacedRepSnapshotData struct {
	Reviews map[string]*ReviewStateData `json:"reviews,omitempty"`
}

// ReviewStateData is the serialized form of ReviewState.
type ReviewStateData struct {
	SkillID         string `json:"skill_id"`
	Stage           int    `json:"stage"`
	NextReviewDate  string `json:"next_review_date"`
	ConsecutiveHits int    `json:"consecutive_hits"`
	Graduated       bool   `json:"graduated"`
	LastReviewDate  string `json:"last_review_date"`
}

// MasterySnapshotData holds mastery state for all skills in a snapshot.
type MasterySnapshotData struct {
	Skills map[string]*SkillMasteryData `json:"skills,omitempty"`
}

// SkillMasteryData is the serialized form of SkillMastery for snapshot storage.
type SkillMasteryData struct {
	SkillID       string    `json:"skill_id"`
	State         string    `json:"state"`
	CurrentTier   string    `json:"current_tier"`
	TotalAttempts int       `json:"total_attempts"`
	CorrectCount  int       `json:"correct_count"`
	SpeedScores   []float64 `json:"speed_scores,omitempty"`
	SpeedWindow   int       `json:"speed_window"`
	Streak        int       `json:"streak"`
	StreakCap     int       `json:"streak_cap"`
	MasteredAt           *string `json:"mastered_at,omitempty"`
	RustyAt              *string `json:"rusty_at,omitempty"`
	MisconceptionPenalty int     `json:"misconception_penalty,omitempty"`
}

// TierProgressData is the serialized form of tier progress for a skill.
type TierProgressData struct {
	SkillID       string `json:"skill_id"`
	CurrentTier   string `json:"current_tier"` // "learn" or "prove"
	TotalAttempts int    `json:"total_attempts"`
	CorrectCount  int    `json:"correct_count"`
}

// Snapshot represents a point-in-time capture of learner state.
type Snapshot struct {
	ID        int
	Sequence  int64
	Timestamp time.Time
	Data      SnapshotData
}

// SnapshotRepo manages learner state snapshots.
type SnapshotRepo interface {
	// Save stores a new snapshot.
	Save(ctx context.Context, snap *Snapshot) error

	// Latest returns the most recent snapshot, or nil if none exist.
	Latest(ctx context.Context) (*Snapshot, error)

	// Prune deletes all but the N most recent snapshots.
	Prune(ctx context.Context, keep int) error
}

// LLMRequestEventData captures the data for a single LLM request event.
type LLMRequestEventData struct {
	Provider     string
	Model        string
	Purpose      string
	InputTokens  int
	OutputTokens int
	LatencyMs    int64
	Success      bool
	ErrorMessage string
}

// SessionEventData captures the data for a session lifecycle event.
type SessionEventData struct {
	SessionID      string
	Action         string // "start" or "end"
	QuestionsServed int
	CorrectAnswers int
	DurationSecs   int
	PlanSummary    []PlanSlotSummaryData
}

// PlanSlotSummaryData is the serialized form of a plan slot for events.
type PlanSlotSummaryData struct {
	SkillID  string `json:"skill_id"`
	Tier     string `json:"tier"`
	Category string `json:"category"`
}

// AnswerEventData captures the data for a single answer event.
type AnswerEventData struct {
	SessionID     string
	SkillID       string
	Tier          string
	Category      string
	QuestionText  string
	CorrectAnswer string
	LearnerAnswer string
	Correct       bool
	TimeMs        int
	AnswerFormat  string
}

// MasteryEventData captures the data for a mastery state transition event.
type MasteryEventData struct {
	SkillID      string
	FromState    string
	ToState      string
	Trigger      string
	FluencyScore float64
	SessionID    string
}

// DiagnosisEventData captures a diagnosis result for persistence.
type DiagnosisEventData struct {
	SessionID       string
	SkillID         string
	QuestionText    string
	CorrectAnswer   string
	LearnerAnswer   string
	Category        string
	MisconceptionID *string
	Confidence      float64
	ClassifierName  string
	Reasoning       string
}

// EventRepo provides append access to domain events.
type EventRepo interface {
	// AppendLLMRequest records an LLM API call event.
	AppendLLMRequest(ctx context.Context, data LLMRequestEventData) error

	// AppendSessionEvent records a session lifecycle event (start/end).
	AppendSessionEvent(ctx context.Context, data SessionEventData) error

	// AppendAnswerEvent records a single answer event.
	AppendAnswerEvent(ctx context.Context, data AnswerEventData) error

	// AppendMasteryEvent records a mastery state transition.
	AppendMasteryEvent(ctx context.Context, data MasteryEventData) error

	// LatestAnswerTime returns the most recent answer timestamp for a skill,
	// or zero time if no answers exist.
	LatestAnswerTime(ctx context.Context, skillID string) (time.Time, error)

	// SkillAccuracy returns the historical accuracy for a skill (correct/total),
	// or 0 if no answers exist.
	SkillAccuracy(ctx context.Context, skillID string) (float64, error)

	// RecentReviewAccuracy returns the accuracy and count of the last N
	// review answers for a skill.
	RecentReviewAccuracy(ctx context.Context, skillID string, lastN int) (accuracy float64, count int, err error)

	// AppendDiagnosisEvent records a diagnosis result for a wrong answer.
	AppendDiagnosisEvent(ctx context.Context, data DiagnosisEventData) error
}
