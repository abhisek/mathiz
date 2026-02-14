# 02 — Persistence Layer

## 1. Overview

The Persistence Layer is the storage foundation for Mathiz. It provides:

- A **SQLite database** (pure Go, no CGO) for all local state
- An **append-only event log** recording every learner action at fine granularity
- **Snapshots** for fast state restoration without full event replay
- **Repository interfaces** that domain modules implement for their own entities

All other modules depend on this layer for durable storage. Domain-specific entities (skills, sessions, etc.) are defined in their respective specs — this spec covers infrastructure: database setup, event log, snapshots, and the repository pattern.

## 2. Database Setup

### Engine

SQLite via [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — pure Go, no CGO dependency.

### File Location

Resolved in order:

1. `--db <path>` CLI flag (highest priority)
2. `MATHIZ_DB` environment variable
3. Default: `~/.local/share/mathiz/mathiz.db` (XDG `$XDG_DATA_HOME/mathiz/mathiz.db`)

The parent directory is created automatically if it does not exist.

### Connection Configuration

Applied on every connection open:

```sql
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA synchronous = NORMAL;
```

- **WAL mode** — concurrent reads during writes, better performance for a single-user app.
- **Busy timeout** — avoids `SQLITE_BUSY` errors if a snapshot write overlaps with event appends.
- **Foreign keys** — enforced at the database level.
- **Synchronous NORMAL** — safe with WAL; balances durability and speed.

### Connection Management

A single `*ent.Client` is created at startup and shared across the application. It is closed on graceful shutdown.

## 3. ent Integration

### Schema Directory

```
ent/
  schema/           # ent schema definitions
    event.go        # base event mixin + per-type event schemas
    snapshot.go     # snapshot schema
  ...               # ent-generated code (do not edit)
```

Domain modules add their own schemas in `ent/schema/` as specified by their respective component specs (e.g., `skill.go`, `session.go`).

### Code Generation

```sh
go generate ./ent
```

Run after any schema change. Generated code is committed to the repo.

### Migration Strategy

**Auto-migration** via `client.Schema.Create(ctx)` at startup:

- Creates tables and columns that don't exist.
- Adds new indexes and foreign keys.
- Does **not** drop columns or tables (safe by default).

This is appropriate for a local single-user app. If destructive migrations are ever needed (e.g., column rename), a manual migration step will be documented in release notes.

## 4. Event Log

The event log is the authoritative record of everything that happens in a learning session. It is **append-only** — events are never updated or deleted.

### Base Fields (Mixin)

Every event entity includes these fields via an ent mixin:

| Field | Type | Description |
|-------|------|-------------|
| `id` | `int` | Auto-increment primary key |
| `sequence` | `int64` | Monotonically increasing sequence number (global, unique) |
| `timestamp` | `time.Time` | UTC wall-clock time of the event |

The `sequence` field is a global counter across all event types, providing a total ordering for sync and replay.

### Event Types

Each event type is a **separate ent schema** with its own table. This gives each type its own typed fields and indexes while keeping the base fields consistent via the mixin.

Initial event types (fields beyond the base are illustrative — domain specs finalize them):

| Event Type | Key Fields | Defined By |
|------------|-----------|------------|
| `AnswerAttemptEvent` | skill_id, question_hash, given_answer, correct, latency_ms | Session spec |
| `HintRequestEvent` | skill_id, question_hash, hint_level | AI Lessons spec |
| `LessonViewEvent` | skill_id, lesson_id | AI Lessons spec |
| `SkillStateChangeEvent` | skill_id, from_state, to_state | Mastery spec |
| `SessionStartEvent` | session_id, target_skill_id | Session spec |
| `SessionEndEvent` | session_id, questions_answered, accuracy | Session spec |
| `GemAwardEvent` | gem_type, rarity, reason_skill_id | Rewards spec |

### Extensibility

Domain specs add new event types by:

1. Creating a new ent schema in `ent/schema/` that uses the `EventMixin`.
2. Adding type-specific fields and indexes.
3. Running `go generate ./ent`.
4. Registering the type with the `EventRepo` for unified queries.

This spec does **not** implement the event schemas above — it provides the mixin and the pattern. Each domain spec owns its event schemas.

### Indexes

Every event table has:

- Index on `sequence` (for ordered replay)
- Index on `timestamp` (for time-range queries)

Type-specific indexes (e.g., `skill_id` on `AnswerAttemptEvent`) are defined by the owning spec.

## 5. Snapshots

A snapshot captures the **full learner state** at a point in time, enabling fast restore without replaying the entire event log.

### Schema

```go
// ent/schema/snapshot.go
field.Int("id"),
field.Int64("sequence"),       // event sequence at time of snapshot
field.Time("timestamp"),       // when the snapshot was taken
field.JSON("data", &SnapshotData{}), // full learner state
```

### SnapshotData Structure

```go
// internal/store/snapshot.go
type SnapshotData struct {
    Skills    []SkillState    `json:"skills"`
    Metrics   []SkillMetrics  `json:"metrics"`
    Schedules []ReviewSchedule `json:"schedules"`
    Gems      []GemRecord     `json:"gems"`
    Version   int             `json:"version"` // schema version for forward compat
}
```

The concrete types (`SkillState`, `SkillMetrics`, etc.) are defined by their respective domain specs. The persistence layer provides the snapshot infrastructure; domain modules register their state for inclusion.

### Creation Triggers

- **Periodic:** Every N events (default: 100), configurable.
- **On exit:** Taken during graceful shutdown.
- **Manual:** `mathiz snapshot` CLI command.

### Restore

On startup:

1. Load the most recent snapshot.
2. Replay all events with `sequence > snapshot.sequence`.
3. If no snapshot exists, replay all events from the beginning.

### Retention

Keep the most recent 5 snapshots. Older snapshots are deleted automatically.

## 6. Repository Interfaces

### Pattern

Each domain entity gets its own repository interface. The persistence layer defines the infrastructure repos; domain specs define their own following the same pattern.

```go
// internal/store/repo.go

// EventRepo provides append and query access to the event log.
type EventRepo interface {
    // Append writes an event and assigns the next global sequence number.
    Append(ctx context.Context, event Event) error

    // QueryByType returns events of a specific type in sequence order.
    QueryByType(ctx context.Context, eventType string, opts QueryOpts) ([]Event, error)

    // QueryByTimeRange returns all events within a time window.
    QueryByTimeRange(ctx context.Context, from, to time.Time) ([]Event, error)

    // QueryAfterSequence returns all events after a given sequence number.
    QueryAfterSequence(ctx context.Context, seq int64) ([]Event, error)

    // NextSequence returns the next global sequence number.
    NextSequence(ctx context.Context) (int64, error)
}

// SnapshotRepo manages learner state snapshots.
type SnapshotRepo interface {
    // Save stores a new snapshot.
    Save(ctx context.Context, snap *Snapshot) error

    // Latest returns the most recent snapshot, or nil if none.
    Latest(ctx context.Context) (*Snapshot, error)

    // Prune deletes all but the N most recent snapshots.
    Prune(ctx context.Context, keep int) error
}
```

### QueryOpts

```go
type QueryOpts struct {
    Limit  int       // max results (0 = unlimited)
    After  int64     // sequence > After
    Before int64     // sequence < Before
    From   time.Time // timestamp >= From
    To     time.Time // timestamp <= To
}
```

### Domain Repos

Domain specs define their own interfaces following this pattern. Example shape (defined in spec 03):

```go
type SkillRepo interface {
    Get(ctx context.Context, id string) (*Skill, error)
    List(ctx context.Context) ([]*Skill, error)
    Save(ctx context.Context, skill *Skill) error
    // ...
}
```

All repo implementations live in `internal/store/` and depend on the `*ent.Client`.

## 7. Sync-Readiness

The design supports future sync without implementing it now:

- **Global sequence numbers** on all events provide a total ordering across event types.
- **Snapshots** can serve as sync checkpoints — send the snapshot + events since.
- **Event export** is trivial: query all events after a sequence number.

### Future Considerations (Not Implemented Now)

- Export format: JSON Lines (one event per line), gzipped.
- Import: validate sequence numbers, detect conflicts, merge.
- Conflict resolution: last-writer-wins on snapshots, append-only on events.

These are design notes, not commitments. The sync spec (if written) will finalize the approach.

## 8. Directory Structure

```
internal/
  store/
    store.go          # Store struct (holds *ent.Client), Open/Close, pragma setup
    event.go          # EventRepo implementation
    snapshot.go       # SnapshotRepo implementation, SnapshotData types
    repo.go           # Repository interfaces
    store_test.go     # Integration tests

ent/
  schema/
    event_mixin.go    # EventMixin (base fields for all event types)
    snapshot.go       # Snapshot ent schema
    # domain schemas added by other specs
  ...                 # generated code
```

## 9. Testing

### In-Memory SQLite

Tests use `file::memory:?cache=shared` as the SQLite DSN. This gives each test a fresh database with no filesystem overhead.

```go
func TestStore(t *testing.T) {
    client := enttest.Open(t, "sqlite3", "file::memory:?cache=shared")
    defer client.Close()
    // ...
}
```

### Repository Mocking

Since repos are interfaces, domain tests can mock them without touching SQLite:

```go
type mockEventRepo struct {
    events []Event
}

func (m *mockEventRepo) Append(ctx context.Context, e Event) error {
    m.events = append(m.events, e)
    return nil
}
// ...
```

### What to Test

- Event append assigns sequential sequence numbers.
- Event queries filter correctly by type, time range, and sequence.
- Snapshot save/load round-trips correctly.
- Snapshot prune keeps only the N most recent.
- Restore from snapshot + event replay produces correct state.
- Pragmas are applied on connection open.

## 10. Verification

The persistence layer is verified when:

- [ ] `ent/schema/` contains the event mixin and snapshot schema.
- [ ] `go generate ./ent` completes without errors.
- [ ] `internal/store/store.go` opens SQLite with correct pragmas.
- [ ] `internal/store/repo.go` defines `EventRepo` and `SnapshotRepo` interfaces.
- [ ] `internal/store/event.go` implements `EventRepo` with append and query methods.
- [ ] `internal/store/snapshot.go` implements `SnapshotRepo` with save, latest, and prune.
- [ ] All tests in `internal/store/store_test.go` pass using in-memory SQLite.
- [ ] Database file is created at the configured location on first run.
- [ ] Events are append-only — no update or delete operations exist on event tables.
- [ ] Sequence numbers are globally unique and monotonically increasing.
