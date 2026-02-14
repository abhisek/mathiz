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
	Version      int                         `json:"version"`
	TierProgress map[string]*TierProgressData `json:"tier_progress,omitempty"`
	MasteredSet  []string                     `json:"mastered_set,omitempty"`
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

// EventRepo provides append access to domain events.
type EventRepo interface {
	// AppendLLMRequest records an LLM API call event.
	AppendLLMRequest(ctx context.Context, data LLMRequestEventData) error

	// AppendSessionEvent records a session lifecycle event (start/end).
	AppendSessionEvent(ctx context.Context, data SessionEventData) error

	// AppendAnswerEvent records a single answer event.
	AppendAnswerEvent(ctx context.Context, data AnswerEventData) error

	// LatestAnswerTime returns the most recent answer timestamp for a skill,
	// or zero time if no answers exist.
	LatestAnswerTime(ctx context.Context, skillID string) (time.Time, error)

	// SkillAccuracy returns the historical accuracy for a skill (correct/total),
	// or 0 if no answers exist.
	SkillAccuracy(ctx context.Context, skillID string) (float64, error)
}
