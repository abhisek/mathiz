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
	Version int `json:"version"`
	// Domain-specific state will be added by later specs:
	// Skills, Metrics, Schedules, Gems, etc.
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

// EventRepo provides append access to domain events.
type EventRepo interface {
	// AppendLLMRequest records an LLM API call event.
	AppendLLMRequest(ctx context.Context, data LLMRequestEventData) error
}
