# 07 — Mastery & Scoring

## 1. Overview

The Mastery & Scoring module is Mathiz's engine for measuring learning progress. It replaces the simple tier-tracking from spec 06 with a **fluency score** (0–1) that blends accuracy, speed, and consistency, and a **mastery state machine** that models each skill's lifecycle from first encounter to durable mastery — including decay and recovery.

**Design goals:**

- **Multi-signal scoring**: Fluency is not just "did they get it right?" It combines accuracy (60%), speed (20%), and consistency (20%) into a single 0–1 score that captures automaticity.
- **State machine progression**: Every skill moves through `new → learning → mastered → rusty` based on fluency score thresholds and external triggers. Transitions are deterministic and auditable.
- **Light recovery**: A mastered skill that goes rusty doesn't require full re-proving. A short recovery check (4 questions, 75% accuracy) restores mastery with minimal frustration.
- **Per-skill configurability**: Mastery criteria use the existing per-skill `TierConfig` system. No global override needed for MVP.
- **Cumulative**: All metrics accumulate across sessions via the snapshot system. A learner never loses credit for work done in a previous session.

### Consumers

| Module | How it uses Mastery & Scoring |
|--------|------------------------------|
| **Session Engine (06)** | Calls `HandleAnswer` which now updates fluency metrics; reads mastery state for plan building |
| **Session Planner (06)** | Uses mastery state to classify skills as frontier, review, or booster |
| **Spaced Repetition (08)** | Reads fluency scores and mastery state to schedule reviews; triggers `rusty` transitions via time-decay |
| **Skill Map (03)** | Displays per-skill mastery state and fluency score |
| **Rewards (11)** | Awards gems on mastery transitions and recovery |

---

## 2. Fluency Score

The fluency score is a single number (0.0–1.0) that captures how well a learner has internalized a skill. It is computed **per skill** from three components.

### 2.1 Components

| Component | Weight | Range | What it measures |
|-----------|--------|-------|-----------------|
| **Accuracy** | 60% | 0.0–1.0 | Fraction of correct answers |
| **Speed** | 20% | 0.0–1.0 | Response time relative to tier time limit |
| **Consistency** | 20% | 0.0–1.0 | Current correct-answer streak, capped |

### 2.2 Formula

```
FluencyScore = 0.6 × Accuracy + 0.2 × Speed + 0.2 × Consistency
```

Each component is clamped to [0.0, 1.0] before weighting.

### 2.3 Accuracy Component

```go
Accuracy = CorrectCount / TotalAttempts
```

Uses the cumulative `TierProgress.CorrectCount` and `TierProgress.TotalAttempts` for the skill's current tier. Resets when the tier advances (as already implemented in spec 06).

If `TotalAttempts == 0`, Accuracy = 0.0.

### 2.4 Speed Component

Speed measures how quickly the learner answers relative to the tier's time limit.

```go
// SpeedScore computes the speed component from response time and tier config.
func SpeedScore(responseTimeMs int, tierCfg skillgraph.TierConfig) float64
```

**For Prove tier** (has `TimeLimitSecs > 0`):

```
timeLimitMs = tierCfg.TimeLimitSecs * 1000
ratio = responseTimeMs / timeLimitMs

if ratio <= 0.5:  speed = 1.0        // Very fast — answered in under half the time
if ratio <= 1.0:  speed = 1.0 - (ratio - 0.5)  // Linear decay from 1.0 to 0.5
if ratio > 1.0:   speed = max(0.0, 0.5 - 0.5 * (ratio - 1.0))  // Over time, decay to 0
```

This gives a smooth curve: full marks for fast answers, graceful degradation for slower ones, zero for answers taking more than 2× the time limit.

**For Learn tier** (has `TimeLimitSecs == 0`):

The Learn tier has no time pressure. Speed is not measured for Learn tier questions. The speed component defaults to **0.5** (neutral) for Learn tier, so it neither helps nor hurts the fluency score.

**Speed is tracked as a rolling average** of the last N answers (configurable, default 10). Each new answer's speed score is blended into the rolling average:

```go
// RollingSpeed tracks the rolling average of speed scores.
type RollingSpeed struct {
    scores []float64
    window int // default 10
}

func (rs *RollingSpeed) Record(score float64) {
    rs.scores = append(rs.scores, score)
    if len(rs.scores) > rs.window {
        rs.scores = rs.scores[len(rs.scores)-rs.window:]
    }
}

func (rs *RollingSpeed) Average() float64 {
    if len(rs.scores) == 0 {
        return 0.5 // neutral default
    }
    sum := 0.0
    for _, s := range rs.scores {
        sum += s
    }
    return sum / float64(len(rs.scores))
}
```

### 2.5 Consistency Component

Consistency is streak-based: it measures how many consecutive correct answers the learner has given on this skill.

```go
// ConsistencyScore computes the consistency component from the current streak.
func ConsistencyScore(streak int, cap int) float64 {
    if cap <= 0 {
        return 0.0
    }
    if streak >= cap {
        return 1.0
    }
    return float64(streak) / float64(cap)
}
```

**Default streak cap: 8.** This means a learner who answers 8 in a row correctly gets full consistency credit. A wrong answer resets the streak to 0.

The streak persists across sessions via the snapshot system.

### 2.6 Fluency Data Structure

```go
// internal/mastery/fluency.go

package mastery

// FluencyMetrics holds the raw data needed to compute a fluency score.
type FluencyMetrics struct {
    // Accuracy is computed from TierProgress (not stored separately).

    // Speed tracks the rolling average of speed scores.
    SpeedScores []float64 `json:"speed_scores"` // last N speed scores
    SpeedWindow int       `json:"speed_window"` // rolling window size (default 10)

    // Consistency tracks the current correct-answer streak.
    Streak    int `json:"streak"`
    StreakCap int `json:"streak_cap"` // default 8
}

// FluencyScore computes the combined fluency score from metrics and tier progress.
func FluencyScore(metrics *FluencyMetrics, accuracy float64) float64 {
    speed := averageSpeed(metrics)
    consistency := ConsistencyScore(metrics.Streak, metrics.StreakCap)

    score := 0.6*accuracy + 0.2*speed + 0.2*consistency
    return clamp(score, 0.0, 1.0)
}
```

---

## 3. Mastery State Machine

Each skill has a mastery state that progresses through a defined lifecycle. The state machine governs what the session planner does with the skill and how the UI presents it.

### 3.1 States

| State | Description | Planner treatment |
|-------|-------------|-------------------|
| **New** | Skill has never been attempted. Prerequisites may or may not be met. | If prerequisites met: frontier candidate. If not: blocked. |
| **Learning** | Learn tier in progress. Learner is actively working on this skill. | Frontier slot. |
| **Mastered** | Both Learn and Prove tiers completed. Skill is durably learned. | Review or booster slot. |
| **Rusty** | Previously mastered but flagged for review — either by time decay (spec 08) or poor review performance. | Recovery slot (treated like frontier but with recovery tier config). |

### 3.2 State Transitions

```
                    ┌─────────────────────────┐
                    │                         │
                    ▼                         │
    ┌─────┐    ┌──────────┐    ┌──────────┐  │  ┌───────┐
    │ New │───▸│ Learning │───▸│ Mastered │──┼─▸│ Rusty │
    └─────┘    └──────────┘    └──────────┘  │  └───────┘
                    ▲                         │      │
                    │                         │      │
                    └─────────────────────────┘      │
                              ▲                      │
                              │   recovery complete  │
                              └──────────────────────┘
```

| From | To | Trigger |
|------|----|---------|
| **New → Learning** | Learner attempts their first question on this skill. |
| **Learning → Mastered** | Prove tier completed (both Learn and Prove tier criteria met, as per spec 06). |
| **Mastered → Rusty** | Time-decay trigger from spaced repetition (spec 08), **or** accuracy on last 4 review questions drops below 50%. |
| **Rusty → Mastered** | Recovery check passed (4 questions, ≥75% accuracy at Learn-tier difficulty). |

### 3.3 Transition Details

#### New → Learning

Triggered automatically when `HandleAnswer` is called for a skill that has no existing `TierProgress`. The system creates a new `TierProgress` at Learn tier and a new `FluencyMetrics` with defaults.

#### Learning → Mastered

This transition already exists in spec 06. When the Prove tier's `IsTierComplete()` returns true, the skill is added to the mastered set. Spec 07 additionally:

1. Records the mastery timestamp for spaced repetition (spec 08)
2. Snapshots the final fluency score at time of mastery

#### Mastered → Rusty

Two triggers, whichever fires first:

1. **Time-decay** (spec 08): The spaced repetition scheduler determines the skill is overdue for review. This spec defines the `MarkRusty(skillID)` API that spec 08 calls. The mechanism for deciding *when* to mark rusty is deferred to spec 08.

2. **Poor review performance**: During a session, if a mastered skill receives review questions and the learner's accuracy on the **last 4 review answers** for that skill drops below 50% (i.e., 0 or 1 correct out of 4), the skill transitions to rusty immediately.

   The review performance check uses a sliding window over the most recent review answers for that skill (from `AnswerEvent` records where `category = "review"`).

#### Rusty → Mastered (Recovery)

When a rusty skill is included in a session plan, it enters a **recovery check**:

- **Questions required**: 4
- **Accuracy threshold**: 75% (3/4 correct)
- **Difficulty**: Learn tier (hints allowed, no time pressure)
- **Speed**: Not counted toward recovery (speed component uses neutral 0.5)

The recovery is tracked using the existing `TierProgress` mechanism with a special recovery tier config:

```go
// RecoveryTierConfig returns the tier config used for rusty skill recovery.
func RecoveryTierConfig() skillgraph.TierConfig {
    return skillgraph.TierConfig{
        Tier:              skillgraph.TierLearn, // Learn-tier difficulty
        ProblemsRequired:  4,
        AccuracyThreshold: 0.75,
        TimeLimitSecs:     0,    // No time pressure
        HintsAllowed:      true, // Hints available
    }
}
```

When the recovery check passes, the skill returns to `Mastered`. The fluency metrics are preserved (not reset) — the recovery answers update the rolling speed and streak as normal.

If the recovery check is not completed in one session, progress carries over to the next session (cumulative, like regular tier progress).

### 3.4 MasteryState Type

```go
// internal/mastery/state.go

package mastery

// MasteryState represents a skill's position in the mastery lifecycle.
type MasteryState string

const (
    StateNew      MasteryState = "new"
    StateLearning MasteryState = "learning"
    StateMastered MasteryState = "mastered"
    StateRusty    MasteryState = "rusty"
)
```

**Mapping to existing `skillgraph.SkillState`**: The `skillgraph` package already defines `StateLocked`, `StateAvailable`, `StateLearning`, `StateProving`, `StateMastered`, `StateRusty`. The `mastery.MasteryState` is the **stored** state, while `skillgraph.SkillState` is the **display** state that also incorporates graph position (locked/available). The mapping:

| mastery.MasteryState | + Graph position | → skillgraph.SkillState |
|---------------------|-------------------|------------------------|
| `new` | prerequisites not met | `StateLocked` |
| `new` | prerequisites met | `StateAvailable` |
| `learning` | Learn tier in progress | `StateLearning` |
| `learning` | Prove tier in progress | `StateProving` |
| `mastered` | — | `StateMastered` |
| `rusty` | — | `StateRusty` |

```go
// ResolveDisplayState maps a mastery state + graph position + tier progress
// into the display state used by the UI.
func ResolveDisplayState(
    state MasteryState,
    prerequisitesMet bool,
    currentTier skillgraph.Tier,
) skillgraph.SkillState {
    switch state {
    case StateNew:
        if prerequisitesMet {
            return skillgraph.StateAvailable
        }
        return skillgraph.StateLocked
    case StateLearning:
        if currentTier == skillgraph.TierProve {
            return skillgraph.StateProving
        }
        return skillgraph.StateLearning
    case StateMastered:
        return skillgraph.StateMastered
    case StateRusty:
        return skillgraph.StateRusty
    default:
        return skillgraph.StateLocked
    }
}
```

---

## 4. SkillMastery Record

Each skill's mastery data is captured in a single record that combines state, tier progress, and fluency metrics.

```go
// internal/mastery/skill_mastery.go

// SkillMastery holds all mastery-related data for a single skill.
type SkillMastery struct {
    SkillID       string         `json:"skill_id"`
    State         MasteryState   `json:"state"`
    CurrentTier   skillgraph.Tier `json:"current_tier"`
    TotalAttempts int            `json:"total_attempts"`
    CorrectCount  int            `json:"correct_count"`
    Fluency       FluencyMetrics `json:"fluency"`
    MasteredAt    *time.Time     `json:"mastered_at,omitempty"` // When mastery was first achieved
    RustyAt       *time.Time     `json:"rusty_at,omitempty"`   // When skill was last marked rusty
}

// Accuracy returns the current accuracy ratio.
func (sm *SkillMastery) Accuracy() float64 {
    if sm.TotalAttempts == 0 {
        return 0.0
    }
    return float64(sm.CorrectCount) / float64(sm.TotalAttempts)
}

// FluencyScore returns the computed fluency score (0.0-1.0).
func (sm *SkillMastery) FluencyScore() float64 {
    return FluencyScore(&sm.Fluency, sm.Accuracy())
}

// IsTierComplete checks if the current tier's criteria are met.
func (sm *SkillMastery) IsTierComplete(cfg skillgraph.TierConfig) bool {
    return sm.TotalAttempts >= cfg.ProblemsRequired &&
        sm.Accuracy() >= cfg.AccuracyThreshold
}
```

---

## 5. Mastery Service

The mastery service is the central API for reading and updating mastery state. It replaces direct manipulation of `TierProgress` and `Mastered` maps in the session engine.

```go
// internal/mastery/service.go

// Service provides mastery state management for all skills.
type Service struct {
    skills    map[string]*SkillMastery // In-memory state, loaded from snapshot
    eventRepo store.EventRepo          // For querying review performance
}

// NewService creates a mastery service, loading state from the snapshot.
func NewService(snap *store.SnapshotData, eventRepo store.EventRepo) *Service

// GetMastery returns the mastery record for a skill.
// Returns a default (StateNew) record if the skill hasn't been encountered.
func (s *Service) GetMastery(skillID string) *SkillMastery

// MasteredSkills returns the set of mastered skill IDs.
func (s *Service) MasteredSkills() map[string]bool

// RecordAnswer updates mastery state after a learner answers a question.
// Returns a StateTransition if the answer caused a state change, nil otherwise.
func (s *Service) RecordAnswer(skillID string, correct bool, responseTimeMs int, tierCfg skillgraph.TierConfig) *StateTransition

// MarkRusty transitions a mastered skill to rusty state.
// Called by the spaced repetition module (spec 08) or by review performance check.
// Returns a StateTransition, or nil if the skill is not currently mastered.
func (s *Service) MarkRusty(skillID string) *StateTransition

// CheckReviewPerformance checks if a mastered skill should go rusty
// based on recent review performance. Called after recording a review answer.
func (s *Service) CheckReviewPerformance(ctx context.Context, skillID string) *StateTransition

// SnapshotData exports the current mastery state for persistence.
func (s *Service) SnapshotData() *MasterySnapshotData

// AllSkillMasteries returns all skill mastery records (for stats/UI).
func (s *Service) AllSkillMasteries() map[string]*SkillMastery
```

### 5.1 StateTransition

```go
// StateTransition records a mastery state change for display and event logging.
type StateTransition struct {
    SkillID   string
    SkillName string
    From      MasteryState
    To        MasteryState
    Trigger   string // "tier-complete", "prove-complete", "time-decay", "review-performance", "recovery-complete"
}
```

### 5.2 RecordAnswer Flow

When `RecordAnswer` is called:

1. Get or create the `SkillMastery` for the skill
2. If state is `New`, transition to `Learning`
3. Update `TotalAttempts` and `CorrectCount`
4. Update fluency metrics:
   - Compute speed score from `responseTimeMs` and `tierCfg`
   - Record speed score into rolling average
   - Update streak (increment if correct, reset to 0 if incorrect)
5. Check tier completion via `IsTierComplete(tierCfg)`
6. If tier complete:
   - **Learn → Prove**: Reset attempt counters, advance `CurrentTier` to Prove
   - **Prove → Mastered**: Transition state to `Mastered`, record `MasteredAt`
   - **Recovery → Mastered** (if state is `Rusty`): Transition state back to `Mastered`, clear `RustyAt`
7. Return `StateTransition` if state changed, nil otherwise

### 5.3 Review Performance Check

After recording an answer on a review question, the session engine calls `CheckReviewPerformance`. This method:

1. Queries the last 4 `AnswerEvent` records for this skill where `category = "review"`
2. If fewer than 4 review answers exist, returns nil (not enough data)
3. Computes accuracy over those 4 answers
4. If accuracy < 0.50 (fewer than 2 correct out of 4), calls `MarkRusty`
5. Returns the `StateTransition` if the skill went rusty

```go
func (s *Service) CheckReviewPerformance(ctx context.Context, skillID string) *StateTransition {
    sm := s.GetMastery(skillID)
    if sm.State != StateMastered {
        return nil
    }

    accuracy, count, err := s.eventRepo.RecentReviewAccuracy(ctx, skillID, 4)
    if err != nil || count < 4 {
        return nil
    }

    if accuracy < 0.50 {
        return s.MarkRusty(skillID)
    }
    return nil
}
```

---

## 6. Integration with Session Engine

The mastery service integrates with the existing session engine from spec 06. The key changes:

### 6.1 SessionState Updates

The `SessionState` struct gains a reference to the mastery service:

```go
// Updated SessionState (additions only)
type SessionState struct {
    // ... existing fields from spec 06 ...

    // MasteryService manages per-skill mastery state and fluency scoring.
    MasteryService *mastery.Service
}
```

### 6.2 HandleAnswer Updates

The existing `HandleAnswer` function in `internal/session/session.go` is updated to delegate mastery tracking to the service:

```go
func HandleAnswer(state *SessionState, learnerAnswer string) *mastery.StateTransition {
    q := state.CurrentQuestion
    if q == nil {
        return nil
    }

    correct := problemgen.CheckAnswer(learnerAnswer, q)
    state.LastAnswerCorrect = correct
    state.TotalQuestions++

    if correct {
        state.TotalCorrect++
    }

    // Update per-skill results (unchanged from spec 06).
    sr := state.PerSkillResults[q.SkillID]
    if sr != nil {
        sr.Attempted++
        if correct {
            sr.Correct++
        }
    }

    // Track prior questions for dedup (unchanged).
    state.PriorQuestions[q.SkillID] = append(state.PriorQuestions[q.SkillID], q.Text)

    // Track errors for LLM context (unchanged).
    if !correct {
        errCtx := BuildErrorContext(q, learnerAnswer)
        errors := state.RecentErrors[q.SkillID]
        errors = append(errors, errCtx)
        if len(errors) > MaxRecentErrors {
            errors = errors[len(errors)-MaxRecentErrors:]
        }
        state.RecentErrors[q.SkillID] = errors
    }

    // Compute response time.
    responseTimeMs := int(time.Since(state.QuestionStartTime).Milliseconds())

    // Delegate to mastery service.
    skill, err := skillgraph.GetSkill(q.SkillID)
    if err != nil {
        return nil
    }

    tierCfg := skill.Tiers[q.Tier]
    transition := state.MasteryService.RecordAnswer(q.SkillID, correct, responseTimeMs, tierCfg)

    // Update mastered set from service (for planner compatibility).
    state.Mastered = state.MasteryService.MasteredSkills()

    return transition
}
```

### 6.3 Planner Integration

The session planner uses `MasteryService.GetMastery()` to determine skill states for plan building:

- **Frontier skills**: Skills where `State == New || State == Learning` and prerequisites are met
- **Review skills**: Skills where `State == Mastered`
- **Recovery skills**: Skills where `State == Rusty` — treated like frontier but use `RecoveryTierConfig()`
- **Booster skills**: Mastered skills with highest fluency score (replaces raw accuracy)

The planner's slot allocation is updated:

| Category | Share | Description |
|----------|-------|-------------|
| **Frontier** | 60% | New or Learning skills |
| **Review** | 30% | Mastered skills due for review |
| **Recovery** | — | Rusty skills replace frontier slots (highest priority) |
| **Booster** | 10% | Mastered skills with highest fluency |

Rusty skills take priority over regular frontier skills. If there are rusty skills, they fill frontier slots first, and remaining frontier slots go to New/Learning skills.

### 6.4 Session Screen Updates

The session screen's tier advancement notification (spec 06, section 7.3) is extended to handle mastery transitions:

```
│                     🏆  Skill mastered!                                    │
│              "Add 3-Digit Numbers" — mastered! Fluency: 0.85              │
```

For recovery:

```
│                     ✅  Skill recovered!                                   │
│              "Add 3-Digit Numbers" — back to mastered!                    │
```

### 6.5 Summary Screen Updates

The session summary (spec 06, section 8) includes fluency scores:

```
  Add 3-Digit Numbers (frontier)          6/8 correct   Learn ▸ Prove   ⚡ 0.72
  Subtract 3-Digit Numbers (frontier)     3/3 correct   Learn           ⚡ 0.65
  Place Value to 1000 (review)            2/3 correct   Mastered        ⚡ 0.88
  Basic Multiplication (recovery)         3/4 correct   Rusty ▸ Mastered ⚡ 0.81
```

---

## 7. Persistence

### 7.1 Snapshot Data Updates

The `SnapshotData` is extended to store mastery state and fluency metrics:

```go
// MasterySnapshotData replaces the old TierProgressData for richer state.
type MasterySnapshotData struct {
    Skills map[string]*SkillMasteryData `json:"skills,omitempty"`
}

// SkillMasteryData is the serialized form of SkillMastery for snapshot storage.
type SkillMasteryData struct {
    SkillID       string   `json:"skill_id"`
    State         string   `json:"state"`          // "new", "learning", "mastered", "rusty"
    CurrentTier   string   `json:"current_tier"`    // "learn" or "prove"
    TotalAttempts int      `json:"total_attempts"`
    CorrectCount  int      `json:"correct_count"`
    SpeedScores   []float64 `json:"speed_scores,omitempty"`
    SpeedWindow   int      `json:"speed_window"`
    Streak        int      `json:"streak"`
    StreakCap     int      `json:"streak_cap"`
    MasteredAt    *string  `json:"mastered_at,omitempty"` // RFC3339
    RustyAt       *string  `json:"rusty_at,omitempty"`    // RFC3339
}
```

**Backward compatibility**: The snapshot version is incremented. When loading a snapshot with the old format (`TierProgress` + `MasteredSet`), the service migrates it to the new format:

- Each `TierProgressData` becomes a `SkillMasteryData` with `State: "learning"` (or `"mastered"` if in the mastered set)
- Missing fluency fields get defaults (speed window = 10, streak cap = 8, streak = 0)

```go
// MigrateSnapshot converts old-format snapshot data to the new mastery format.
func MigrateSnapshot(old *store.SnapshotData) *MasterySnapshotData
```

### 7.2 Updated SnapshotData

```go
// Updated store.SnapshotData
type SnapshotData struct {
    Version  int                  `json:"version"`
    Mastery  *MasterySnapshotData `json:"mastery,omitempty"`

    // Deprecated: kept for migration only. New snapshots use Mastery field.
    TierProgress map[string]*TierProgressData `json:"tier_progress,omitempty"`
    MasteredSet  []string                     `json:"mastered_set,omitempty"`
}
```

### 7.3 EventRepo Additions

```go
// Added to store.EventRepo interface

// RecentReviewAccuracy returns the accuracy and count of the last N
// review answers for a skill. Used by the mastery service to detect
// poor review performance.
RecentReviewAccuracy(ctx context.Context, skillID string, lastN int) (accuracy float64, count int, err error)
```

### 7.4 Mastery Event (ent schema)

A new event schema records mastery state transitions for audit and analytics:

```go
// ent/schema/mastery_event.go

func (MasteryEvent) Fields() []ent.Field {
    return []ent.Field{
        field.String("skill_id").NotEmpty(),
        field.String("from_state").NotEmpty(),    // "new", "learning", "mastered", "rusty"
        field.String("to_state").NotEmpty(),
        field.String("trigger").NotEmpty(),        // "tier-complete", "prove-complete", "time-decay", "review-performance", "recovery-complete"
        field.Float("fluency_score"),              // Fluency score at time of transition
        field.String("session_id").Optional(),     // Session during which this happened (empty for time-decay)
    }
}
```

Uses the `EventMixin` (sequence + timestamp), same as other event schemas.

```go
// Added to store.EventRepo interface

// AppendMasteryEvent records a mastery state transition.
AppendMasteryEvent(ctx context.Context, data MasteryEventData) error

// MasteryEventData is the input for persisting a mastery event.
type MasteryEventData struct {
    SkillID      string
    FromState    string
    ToState      string
    Trigger      string
    FluencyScore float64
    SessionID    string
}
```

---

## 8. Stats Command Integration

The `stats` command (existing in `cmd/stats.go`) is updated to display mastery information:

```
Mathiz Stats
────────────────────────────────────

Skills: 5 mastered, 3 learning, 1 rusty, 43 new

Top Skills by Fluency:
  ✅ Place Value to 1000          0.92
  ✅ Add 2-Digit Numbers          0.88
  ✅ Subtract 2-Digit Numbers     0.85
  📖 Add 3-Digit Numbers          0.72
  📖 Subtract 3-Digit Numbers     0.65

Rusty Skills:
  🔄 Basic Multiplication         0.45 (last mastered 14 days ago)
```

The stats command reads from the latest snapshot and displays:
- Skill count by mastery state
- Top skills sorted by fluency score
- Rusty skills needing recovery

---

## 9. Package Structure

```
internal/
  mastery/
    state.go            # MasteryState type, state constants
    fluency.go          # FluencyMetrics, FluencyScore, SpeedScore, ConsistencyScore
    skill_mastery.go    # SkillMastery record type
    service.go          # Service (state management, RecordAnswer, MarkRusty, CheckReviewPerformance)
    recovery.go         # RecoveryTierConfig, recovery check logic
    display.go          # ResolveDisplayState (mastery → skillgraph.SkillState mapping)
    snapshot.go         # MasterySnapshotData, SkillMasteryData, MigrateSnapshot, serialization
    fluency_test.go     # Fluency score computation tests
    service_test.go     # Service tests (state transitions, RecordAnswer, MarkRusty)
    recovery_test.go    # Recovery check tests
    display_test.go     # Display state mapping tests
    snapshot_test.go    # Snapshot serialization and migration tests
  store/
    repo.go             # Updated EventRepo interface (+ RecentReviewAccuracy, AppendMasteryEvent)
  ent/schema/
    mastery_event.go    # MasteryEvent ent schema
```

---

## 10. Dependencies

| Dependency | Direction | What's Used |
|-----------|-----------|------------|
| `internal/skillgraph` | → imports | `Skill`, `Tier`, `TierConfig`, `SkillState`, `DefaultTiers` |
| `internal/store` | → imports | `EventRepo`, `SnapshotData`, `MasteryEventData` |
| `internal/session` | ← consumed by | Calls `Service.RecordAnswer()` from `HandleAnswer` |
| `internal/screens/session` | ← consumed by | Displays `StateTransition` notifications |
| `internal/screens/summary` | ← consumed by | Shows fluency scores in summary |
| `internal/screens/skillmap` | ← consumed by | Uses `ResolveDisplayState` for skill state icons |
| Spaced Repetition (08) | ← consumed by | Calls `Service.MarkRusty()` for time-decay transitions |
| Rewards (11) | ← consumed by | Reads `MasteryEvent` records for gem awards |

---

## 11. Testing Strategy

### 11.1 Fluency Score Tests

```go
func TestFluencyScore_AllPerfect(t *testing.T) {
    // Accuracy 1.0, Speed 1.0 (all fast), Streak at cap
    // Assert: score = 1.0
}

func TestFluencyScore_ZeroAttempts(t *testing.T) {
    // No attempts → accuracy 0, speed defaults to 0.5, streak 0
    // Assert: score = 0.0*0.6 + 0.5*0.2 + 0.0*0.2 = 0.1
}

func TestFluencyScore_MixedPerformance(t *testing.T) {
    // Accuracy 0.8, Speed avg 0.7, Streak 4/8 = 0.5
    // Assert: score = 0.8*0.6 + 0.7*0.2 + 0.5*0.2 = 0.48 + 0.14 + 0.10 = 0.72
}

func TestFluencyScore_Clamping(t *testing.T) {
    // Components that would push score above 1.0 → clamped to 1.0
}
```

### 11.2 Speed Score Tests

```go
func TestSpeedScore_ProveTier_VeryFast(t *testing.T) {
    // Response 10s, time limit 30s → ratio 0.33 → speed 1.0
}

func TestSpeedScore_ProveTier_AtLimit(t *testing.T) {
    // Response 30s, time limit 30s → ratio 1.0 → speed 0.5
}

func TestSpeedScore_ProveTier_Slow(t *testing.T) {
    // Response 45s, time limit 30s → ratio 1.5 → speed 0.25
}

func TestSpeedScore_ProveTier_VeryOverTime(t *testing.T) {
    // Response 60s, time limit 30s → ratio 2.0 → speed 0.0
}

func TestSpeedScore_LearnTier_Neutral(t *testing.T) {
    // Learn tier (TimeLimitSecs=0) → speed = 0.5 regardless of response time
}

func TestSpeedScore_RollingAverage(t *testing.T) {
    // Record 10 speed scores, verify average is correct
    // Record 11th score, verify window slides (oldest dropped)
}
```

### 11.3 Consistency Score Tests

```go
func TestConsistencyScore_ZeroStreak(t *testing.T) {
    // Streak 0, cap 8 → 0.0
}

func TestConsistencyScore_PartialStreak(t *testing.T) {
    // Streak 4, cap 8 → 0.5
}

func TestConsistencyScore_FullStreak(t *testing.T) {
    // Streak 8, cap 8 → 1.0
}

func TestConsistencyScore_OverCap(t *testing.T) {
    // Streak 12, cap 8 → 1.0 (clamped)
}

func TestConsistencyScore_StreakResetOnWrong(t *testing.T) {
    // Record correct × 5, then incorrect → streak = 0
}
```

### 11.4 State Machine Tests

```go
func TestStateMachine_NewToLearning(t *testing.T) {
    // First answer on a new skill → state transitions to Learning
    // Assert: StateTransition{From: New, To: Learning, Trigger: "first-attempt"}
}

func TestStateMachine_LearningToMastered(t *testing.T) {
    // Complete Learn tier, then complete Prove tier → Mastered
    // Assert: StateTransition{From: Learning, To: Mastered, Trigger: "prove-complete"}
    // Assert: MasteredAt is set
}

func TestStateMachine_MasteredToRusty_TimeTrigger(t *testing.T) {
    // Call MarkRusty on a mastered skill
    // Assert: StateTransition{From: Mastered, To: Rusty, Trigger: "time-decay"}
    // Assert: RustyAt is set
}

func TestStateMachine_MasteredToRusty_ReviewPerformance(t *testing.T) {
    // Record 4 review answers: 1 correct, 3 incorrect (25% < 50%)
    // Assert: StateTransition{From: Mastered, To: Rusty, Trigger: "review-performance"}
}

func TestStateMachine_NotRusty_GoodReview(t *testing.T) {
    // Record 4 review answers: 3 correct, 1 incorrect (75% ≥ 50%)
    // Assert: no transition, skill stays Mastered
}

func TestStateMachine_NotRusty_TooFewReviews(t *testing.T) {
    // Only 2 review answers exist → not enough data, no transition
}

func TestStateMachine_RustyToMastered_Recovery(t *testing.T) {
    // Skill is rusty, answer 4 recovery questions with 3/4 correct
    // Assert: StateTransition{From: Rusty, To: Mastered, Trigger: "recovery-complete"}
    // Assert: RustyAt is cleared
}

func TestStateMachine_RustyRecoveryFails(t *testing.T) {
    // Skill is rusty, answer 4 recovery questions with 1/4 correct
    // Assert: no transition, skill stays Rusty
}

func TestStateMachine_MarkRusty_NotMastered(t *testing.T) {
    // Call MarkRusty on a Learning skill → returns nil (no-op)
}

func TestStateMachine_RecoveryCumulative(t *testing.T) {
    // Answer 2 recovery questions in session 1, 2 more in session 2
    // Assert: recovery is evaluated on all 4 cumulative answers
}
```

### 11.5 Service Tests

```go
func TestService_NewService_EmptySnapshot(t *testing.T) {
    // Empty snapshot → all skills are New
}

func TestService_NewService_WithExistingData(t *testing.T) {
    // Snapshot with mastery data → skills loaded with correct states
}

func TestService_MasteredSkills(t *testing.T) {
    // Service with 3 mastered skills → MasteredSkills() returns map of 3
}

func TestService_RecordAnswer_UpdatesFluency(t *testing.T) {
    // Record an answer → FluencyMetrics updated (speed, streak)
    // Assert: FluencyScore changes appropriately
}

func TestService_SnapshotRoundTrip(t *testing.T) {
    // Service with various states → SnapshotData() → NewService(data) → states match
}
```

### 11.6 Snapshot Migration Tests

```go
func TestMigrateSnapshot_OldFormat(t *testing.T) {
    // Old SnapshotData with TierProgress + MasteredSet
    // → MasterySnapshotData with correct states and default fluency fields
}

func TestMigrateSnapshot_MasteredSkills(t *testing.T) {
    // Old format with skills in MasteredSet → State is Mastered
}

func TestMigrateSnapshot_LearningSkills(t *testing.T) {
    // Old format with TierProgress but not mastered → State is Learning
}

func TestMigrateSnapshot_EmptySnapshot(t *testing.T) {
    // Empty old snapshot → empty new snapshot
}

func TestMigrateSnapshot_DefaultFluency(t *testing.T) {
    // Migrated skills get SpeedWindow=10, StreakCap=8, Streak=0
}
```

### 11.7 Display State Tests

```go
func TestResolveDisplayState_NewLocked(t *testing.T) {
    // State New, prerequisites not met → StateLocked
}

func TestResolveDisplayState_NewAvailable(t *testing.T) {
    // State New, prerequisites met → StateAvailable
}

func TestResolveDisplayState_LearningLearnTier(t *testing.T) {
    // State Learning, tier Learn → StateLearning
}

func TestResolveDisplayState_LearningProveTier(t *testing.T) {
    // State Learning, tier Prove → StateProving
}

func TestResolveDisplayState_Mastered(t *testing.T) {
    // State Mastered → StateMastered
}

func TestResolveDisplayState_Rusty(t *testing.T) {
    // State Rusty → StateRusty
}
```

### 11.8 Persistence Tests

```go
func TestAppendMasteryEvent(t *testing.T) {
    // MasteryEvent with all fields is persisted correctly
}

func TestRecentReviewAccuracy_NoReviews(t *testing.T) {
    // No review answers → count 0, no error
}

func TestRecentReviewAccuracy_PartialReviews(t *testing.T) {
    // 2 review answers → count 2, accuracy computed correctly
}

func TestRecentReviewAccuracy_FullWindow(t *testing.T) {
    // 6 review answers, request last 4 → only most recent 4 counted
}
```

---

## 12. Example Flow

### 12.1 New Skill → Learning → Mastered

**Learner starts a session. "Add 3-Digit Numbers" is new (never attempted).**

1. Planner includes "Add 3-Digit Numbers" as a frontier skill (State: New).
2. First question generated. Learner answers correctly in 12s.
3. `RecordAnswer` called:
   - State: New → Learning (transition fired)
   - TotalAttempts: 1, CorrectCount: 1, Accuracy: 1.0
   - Speed: Learn tier → 0.5 (neutral)
   - Streak: 1
   - FluencyScore: 0.6×1.0 + 0.2×0.5 + 0.2×(1/8) = 0.725
4. After 8 questions (6 correct, 2 wrong): Accuracy = 0.75, Streak = 2 (reset twice).
5. Learn tier complete (8 attempts, 75% accuracy meets 75% threshold).
6. Tier advances to Prove. Counters reset.
7. Prove tier: 6 questions, 5 correct. Response times: 15s, 20s, 25s, 18s, 22s, 28s.
   - Accuracy: 0.833 (≥ 0.85 threshold? No, 5/6 = 0.833)
   - Need more questions. 7th question answered correctly in 20s.
   - Accuracy: 6/7 = 0.857 ≥ 0.85 ✓, Attempts: 7 ≥ 6 ✓
8. Prove tier complete. State: Learning → Mastered.
9. `MasteredAt` recorded. Mastery event persisted.
10. Summary shows: "Add 3-Digit Numbers — Mastered! ⚡ 0.81"

### 12.2 Mastered → Rusty → Recovery

**Two weeks later. Learner hasn't practiced "Add 3-Digit Numbers".**

1. Spaced repetition (spec 08) detects the skill is overdue.
2. Calls `Service.MarkRusty("add-3digit")`.
3. State: Mastered → Rusty. `RustyAt` recorded.
4. Next session, planner includes "Add 3-Digit Numbers" as a recovery skill (takes a frontier slot).
5. Recovery questions served at Learn-tier difficulty (hints available).
6. Learner answers 4 questions: 3 correct, 1 wrong.
7. Recovery check: 3/4 = 75% ≥ 75% threshold ✓.
8. State: Rusty → Mastered. `RustyAt` cleared.
9. Summary shows: "Add 3-Digit Numbers — Recovered! ⚡ 0.78"

### 12.3 Rusty via Poor Review

**Learner has "Place Value to 1000" mastered. During a session, it appears in a review slot.**

1. Review question 1: Incorrect.
2. Review question 2: Incorrect.
3. Review question 3: Correct.
4. Review question 4: Incorrect.
5. After question 4, `CheckReviewPerformance` runs:
   - Last 4 review answers: 1/4 = 25% < 50%
   - State: Mastered → Rusty. Trigger: "review-performance".
6. Notification shown: "Place Value to 1000 needs review!"
7. Skill will appear as recovery in the next session.

---

## 13. Open Questions / Future Considerations

- **Spec 08 (Spaced Repetition)** will implement the time-decay logic that calls `MarkRusty`. This spec provides the API; spec 08 provides the scheduling algorithm.
- **Spec 09 (Error Diagnosis)** may feed into fluency scoring — misconception-tagged errors could reduce fluency more than careless errors. Not in MVP.
- **Spec 10 (AI Lessons)** may generate recovery micro-lessons for rusty skills before the recovery check begins.
- **Spec 11 (Rewards)** will award mastery gems on Learning → Mastered transitions and recovery gems on Rusty → Mastered transitions.
- **Fluency thresholds for mastery**: Currently mastery is purely tier-based (accuracy + attempts). A future version could require a minimum fluency score (e.g., 0.7) in addition to tier completion.
- **Speed benchmarks per skill**: Some skills naturally take longer (multi-digit multiplication vs. simple addition). Per-skill speed targets could improve the speed component's accuracy. Deferred to post-MVP.
- **Assist rate**: The README mentions "assist rate" as a metric. This would track how often the learner uses hints. Deferred until spec 10 implements hints.

---

## 14. Verification

The Mastery & Scoring module is verified when:

- [ ] `internal/mastery/state.go` defines `MasteryState` type with `New`, `Learning`, `Mastered`, `Rusty` constants
- [ ] `internal/mastery/fluency.go` defines `FluencyMetrics`, `FluencyScore()`, `SpeedScore()`, `ConsistencyScore()`
- [ ] `FluencyScore()` computes weighted sum: 0.6×accuracy + 0.2×speed + 0.2×consistency, clamped to [0, 1]
- [ ] `SpeedScore()` returns 0.5 for Learn tier (no time limit), and computes ratio-based score for Prove tier
- [ ] `ConsistencyScore()` returns streak/cap, clamped to [0, 1]; streak resets on incorrect answer
- [ ] Speed uses a rolling average over the last 10 scores (configurable window)
- [ ] `internal/mastery/skill_mastery.go` defines `SkillMastery` with state, tier progress, fluency metrics, and timestamps
- [ ] `internal/mastery/service.go` defines `Service` with `GetMastery`, `MasteredSkills`, `RecordAnswer`, `MarkRusty`, `CheckReviewPerformance`, `SnapshotData`
- [ ] `RecordAnswer` transitions New → Learning on first attempt
- [ ] `RecordAnswer` transitions Learning → Mastered when Prove tier completes
- [ ] `RecordAnswer` transitions Rusty → Mastered when recovery check passes (4 questions, ≥75%)
- [ ] `MarkRusty` transitions Mastered → Rusty and sets `RustyAt`
- [ ] `CheckReviewPerformance` queries last 4 review answers; triggers rusty if accuracy < 50%
- [ ] `CheckReviewPerformance` is a no-op if fewer than 4 review answers exist or skill is not mastered
- [ ] `internal/mastery/recovery.go` defines `RecoveryTierConfig()` (4 questions, 75%, Learn difficulty)
- [ ] `internal/mastery/display.go` defines `ResolveDisplayState` mapping mastery state → skillgraph.SkillState
- [ ] `internal/mastery/snapshot.go` defines `MasterySnapshotData`, `SkillMasteryData`, and `MigrateSnapshot`
- [ ] `MigrateSnapshot` correctly converts old `TierProgress + MasteredSet` format to new `MasterySnapshotData`
- [ ] Snapshot round-trip: Service → SnapshotData → NewService → all states and metrics preserved
- [ ] `ent/schema/mastery_event.go` defines `MasteryEvent` with `EventMixin`
- [ ] `store.EventRepo` updated with `RecentReviewAccuracy` and `AppendMasteryEvent`
- [ ] `SessionState` updated with `MasteryService` field
- [ ] `HandleAnswer` delegates to `MasteryService.RecordAnswer()` and returns `StateTransition`
- [ ] Session summary displays fluency scores per skill
- [ ] `StateTransition` type records `From`, `To`, `Trigger` for display and event logging
- [ ] All fluency, service, state machine, recovery, display, snapshot, and persistence tests pass
- [ ] `CGO_ENABLED=0 go build ./...` succeeds
- [ ] `go test ./internal/mastery/... ./internal/session/... ./internal/store/...` passes
