package store

// Event repo infrastructure.
//
// The EventRepo interface and implementation will be built out as domain
// specs add their event schemas (AnswerAttemptEvent, HintRequestEvent, etc.).
// Each event type gets its own ent schema using the EventMixin, and the
// EventRepo provides unified append and query access across all types.
//
// For now, this file establishes the sequence counter that will be shared
// across all event types for global ordering.

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

// sequenceCounter manages the global monotonic sequence number shared across
// all event types. Each event type lives in its own ent-managed table, so
// per-table auto-increment IDs can't establish cross-type ordering. This
// shared counter assigns a single increasing sequence to every event
// regardless of type, enabling:
//
//   - Cross-type ordering (e.g. did the hint come before or after the answer?)
//   - Snapshot consistency (query all tables for sequence > snapshot.sequence)
//   - Append-only guarantees (events are never reordered)
//
// Uses raw SQL outside ent because ent doesn't support database-level atomic
// counters. The mutex serializes within the process; the RETURNING clause
// makes the increment atomic at the database level.
type sequenceCounter struct {
	mu sync.Mutex
	db *sql.DB
}

// newSequenceCounter creates a counter and ensures the tracking table exists.
func newSequenceCounter(db *sql.DB) (*sequenceCounter, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS global_sequence (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		next_val INTEGER NOT NULL DEFAULT 1
	)`)
	if err != nil {
		return nil, fmt.Errorf("create sequence table: %w", err)
	}

	_, err = db.Exec(`INSERT OR IGNORE INTO global_sequence (id, next_val) VALUES (1, 1)`)
	if err != nil {
		return nil, fmt.Errorf("seed sequence: %w", err)
	}

	return &sequenceCounter{db: db}, nil
}

// Next atomically returns the next sequence number and increments the counter.
func (sc *sequenceCounter) Next(ctx context.Context) (int64, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	var seq int64
	err := sc.db.QueryRowContext(ctx,
		`UPDATE global_sequence SET next_val = next_val + 1 WHERE id = 1 RETURNING next_val - 1`,
	).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("next sequence: %w", err)
	}
	return seq, nil
}
