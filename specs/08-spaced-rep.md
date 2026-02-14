# 08 — Spaced Repetition

## 1. Overview

The Spaced Repetition module is Mathiz's scheduling engine for long-term retention. It determines **when** each mastered skill should be reviewed and **which** skills are most urgently due, replacing the session planner's placeholder "least-recently-practiced" heuristic with a proper expanding-interval algorithm.

**Design goals:**

- **Simple exponential spacing**: A 6-stage interval schedule (1→3→7→14→30→60 days) that expands with each successful review. Transparent, predictable, and easy to tune — no opaque ease factors or complex memory models.
- **Session-start decay check**: Overdue skills are detected and marked rusty at session start, so the learner immediately sees what needs attention.
- **Most-overdue-first priority**: When multiple skills are due for review but only 1–2 session slots are available, the most overdue skill gets priority.
- **Graduation**: After 6 successful consecutive reviews, a skill "graduates" to a 90-day max interval, reducing review load for well-established knowledge.
- **UI visibility**: Review schedule info (next review date, overdue status, graduated badge) is exposed to the skill map and stats screens.
- **Uniform intervals**: All skills use the same interval schedule. Fluency score already captures per-skill difficulty indirectly. No skill-weighting for MVP.

### Consumers

| Module | How it uses Spaced Repetition |
|--------|-------------------------------|
| **Mastery Service (07)** | Spaced rep calls `MarkRusty()` for overdue skills |
| **Session Planner (06)** | Spaced rep replaces `selectReviewSkills()` with due-date-based selection |
| **Session Engine (06)** | Calls `RecordReview()` after review answers to advance intervals |
| **Skill Map (03)** | Displays review schedule info (due, overdue, graduated) |
| **Stats Command** | Shows upcoming reviews and graduated skills |

---

## 2. Interval Schedule

### 2.1 Base Intervals

The interval schedule is a fixed sequence of day counts. After mastery, the first review is due after 1 day. Each successful review advances to the next interval in the sequence.

| Stage | Interval (days) | Cumulative (days since mastery) |
|-------|----------------:|-------------------------------:|
| 0 | 1 | 1 |
| 1 | 3 | 4 |
| 2 | 7 | 11 |
| 3 | 14 | 25 |
| 4 | 30 | 55 |
| 5 | 60 | 115 |

```go
// internal/spacedrep/schedule.go

package spacedrep

// BaseIntervals defines the expanding interval schedule in days.
// Stage 0 = first review after mastery.
var BaseIntervals = []int{1, 3, 7, 14, 30, 60}

// MaxStage is the highest stage index in BaseIntervals.
const MaxStage = 5

// GraduationStage is the stage at which a skill graduates.
// A skill graduates after completing all 6 stages (0–5) successfully.
const GraduationStage = 6

// GraduatedIntervalDays is the review interval for graduated skills.
const GraduatedIntervalDays = 90
```

### 2.2 Interval Progression

- **On mastery**: Skill enters stage 0. Next review due in `BaseIntervals[0]` = 1 day.
- **On successful review**: Stage increments by 1. Next review due in `BaseIntervals[stage]` days from now.
- **After stage 5**: Skill graduates. All subsequent reviews use `GraduatedIntervalDays` (90 days).
- **On failed review** (triggered by `CheckReviewPerformance`): The existing mechanism handles this — `MarkRusty` is called, which resets the skill to rusty state. When the skill recovers (Rusty → Mastered via recovery check), it re-enters the schedule at **stage 0** (interval resets to 1 day).

### 2.3 Overdue Threshold

A skill becomes **overdue** when the current time exceeds the next review date. A skill is marked **rusty** when it is overdue by more than a grace period:

```
RustyThreshold = NextReviewDate + (CurrentInterval × 0.5)
```

For example, if a skill is at stage 2 (7-day interval) and its next review date was 7 days ago:
- NextReviewDate = Jan 1
- Overdue since Jan 1
- Grace period = 7 × 0.5 = 3.5 days
- RustyThreshold = Jan 1 + 3.5 days = Jan 4.5
- If current date > Jan 4.5, skill is marked rusty

This means a skill at stage 0 (1-day interval) has only 0.5 days of grace, while a graduated skill (90-day interval) has 45 days of grace. This scales naturally — skills reviewed more frequently are expected to be practiced more regularly.

---

## 3. Review State

### 3.1 Per-Skill Review Data

Each mastered skill tracks its position in the review schedule:

```go
// internal/spacedrep/review.go

// ReviewState holds the spaced repetition state for a single skill.
type ReviewState struct {
    SkillID         string    `json:"skill_id"`
    Stage           int       `json:"stage"`             // Current stage (0–5, or 6+ for graduated)
    NextReviewDate  time.Time `json:"next_review_date"`  // When the next review is due
    ConsecutiveHits int       `json:"consecutive_hits"`  // Successful reviews in a row (for graduation)
    Graduated       bool      `json:"graduated"`         // Whether the skill has graduated
    LastReviewDate  time.Time `json:"last_review_date"`  // When the last review occurred
}

// IsDue returns true if the skill is due for review (at or past the review date).
func (rs *ReviewState) IsDue(now time.Time) bool {
    return !now.Before(rs.NextReviewDate)
}

// IsOverdue returns true if the skill is past its review date.
// OverdueDays returns how many days past due the skill is.
func (rs *ReviewState) OverdueDays(now time.Time) float64 {
    if now.Before(rs.NextReviewDate) {
        return 0
    }
    return now.Sub(rs.NextReviewDate).Hours() / 24.0
}

// IsRustyThreshold returns true if the skill has exceeded its grace period
// and should be marked rusty.
func (rs *ReviewState) IsRustyThreshold(now time.Time) bool {
    if !rs.IsDue(now) {
        return false
    }
    interval := rs.CurrentIntervalDays()
    graceHours := float64(interval) * 0.5 * 24.0
    threshold := rs.NextReviewDate.Add(time.Duration(graceHours * float64(time.Hour)))
    return now.After(threshold)
}

// CurrentIntervalDays returns the current interval in days.
func (rs *ReviewState) CurrentIntervalDays() int {
    if rs.Graduated {
        return GraduatedIntervalDays
    }
    if rs.Stage >= len(BaseIntervals) {
        return BaseIntervals[len(BaseIntervals)-1]
    }
    return BaseIntervals[rs.Stage]
}
```

### 3.2 Review Status (for UI)

```go
// ReviewStatus describes a skill's review status for display.
type ReviewStatus string

const (
    ReviewNotDue   ReviewStatus = "not_due"   // Next review is in the future
    ReviewDue      ReviewStatus = "due"       // At or past the review date
    ReviewOverdue  ReviewStatus = "overdue"   // Past the review date + grace period
    ReviewGraduated ReviewStatus = "graduated" // Completed all stages
)

// Status returns the review status for UI display.
func (rs *ReviewState) Status(now time.Time) ReviewStatus {
    if rs.Graduated && !rs.IsDue(now) {
        return ReviewGraduated
    }
    if rs.IsRustyThreshold(now) {
        return ReviewOverdue
    }
    if rs.IsDue(now) {
        return ReviewDue
    }
    if rs.Graduated {
        return ReviewGraduated
    }
    return ReviewNotDue
}

// DaysUntilReview returns the number of days until the next review.
// Returns 0 if already due.
func (rs *ReviewState) DaysUntilReview(now time.Time) int {
    if rs.IsDue(now) {
        return 0
    }
    return int(rs.NextReviewDate.Sub(now).Hours()/24.0) + 1
}
```

---

## 4. Scheduler Service

The scheduler is the central API for managing review schedules. It operates on the mastery service's state and the persisted review state.

```go
// internal/spacedrep/scheduler.go

// Scheduler manages spaced repetition review scheduling.
type Scheduler struct {
    reviews   map[string]*ReviewState  // skillID → review state
    mastery   *mastery.Service         // reads mastery state
    eventRepo store.EventRepo          // for logging
}

// NewScheduler creates a scheduler, loading review state from the snapshot.
func NewScheduler(snap *store.SnapshotData, mastery *mastery.Service, eventRepo store.EventRepo) *Scheduler

// RunDecayCheck scans all mastered skills and marks overdue ones as rusty.
// Called at session start. Returns the list of skills that transitioned to rusty.
func (s *Scheduler) RunDecayCheck(ctx context.Context, now time.Time) []*mastery.StateTransition

// DueSkills returns mastered skills that are due for review, sorted by
// most overdue first. Used by the session planner for review slot selection.
func (s *Scheduler) DueSkills(now time.Time) []string

// RecordReview updates the review schedule after a review answer.
// Called by the session engine after a review-category answer.
func (s *Scheduler) RecordReview(skillID string, correct bool, now time.Time)

// InitSkill initializes review state for a newly mastered skill.
// Called when a skill transitions Learning → Mastered.
func (s *Scheduler) InitSkill(skillID string, masteredAt time.Time)

// ReInitSkill re-initializes review state after recovery (Rusty → Mastered).
// Resets to stage 0 with a fresh interval.
func (s *Scheduler) ReInitSkill(skillID string, now time.Time)

// GetReviewState returns the review state for a skill, or nil if not tracked.
func (s *Scheduler) GetReviewState(skillID string) *ReviewState

// AllReviewStates returns all review states (for stats/UI).
func (s *Scheduler) AllReviewStates() map[string]*ReviewState

// SnapshotData exports the current review state for persistence.
func (s *Scheduler) SnapshotData() *store.SpacedRepSnapshotData
```

### 4.1 RunDecayCheck Flow

Called once at session start:

1. Iterate all review states where the skill is currently `Mastered` (not already Rusty).
2. For each, check `IsRustyThreshold(now)`.
3. If threshold exceeded, call `mastery.MarkRusty(skillID)` with trigger `"time-decay"`.
4. Log a `MasteryEvent` with trigger `"time-decay"` and no `session_id` (pre-session).
5. Return all transitions for UI notification.

```go
func (s *Scheduler) RunDecayCheck(ctx context.Context, now time.Time) []*mastery.StateTransition {
    var transitions []*mastery.StateTransition

    for skillID, rs := range s.reviews {
        sm := s.mastery.GetMastery(skillID)
        if sm.State != mastery.StateMastered {
            continue
        }
        if rs.IsRustyThreshold(now) {
            transition := s.mastery.MarkRusty(skillID)
            if transition != nil {
                transitions = append(transitions, transition)
                // Log mastery event
                s.eventRepo.AppendMasteryEvent(ctx, store.MasteryEventData{
                    SkillID:      skillID,
                    FromState:    string(transition.From),
                    ToState:      string(transition.To),
                    Trigger:      "time-decay",
                    FluencyScore: sm.FluencyScore(),
                })
            }
        }
    }
    return transitions
}
```

### 4.2 DueSkills Flow

Returns review candidates sorted by most-overdue-first:

1. Iterate all review states.
2. Filter to skills that are `IsDue(now)` AND currently `Mastered` (not Rusty — rusty skills are handled as recovery slots, not review slots).
3. Sort by `OverdueDays(now)` descending (most overdue first).
4. Return sorted skill IDs.

```go
func (s *Scheduler) DueSkills(now time.Time) []string {
    type dueSkill struct {
        id      string
        overdue float64
    }
    var due []dueSkill

    for skillID, rs := range s.reviews {
        sm := s.mastery.GetMastery(skillID)
        if sm.State != mastery.StateMastered {
            continue
        }
        if rs.IsDue(now) {
            due = append(due, dueSkill{id: skillID, overdue: rs.OverdueDays(now)})
        }
    }

    sort.Slice(due, func(i, j int) bool {
        return due[i].overdue > due[j].overdue
    })

    ids := make([]string, len(due))
    for i, d := range due {
        ids[i] = d.id
    }
    return ids
}
```

### 4.3 RecordReview Flow

Called after a review-category answer in the session:

1. Look up the `ReviewState` for the skill.
2. If `correct`:
   - Increment `ConsecutiveHits`.
   - Advance `Stage` by 1 (if not already graduated).
   - If `ConsecutiveHits >= GraduationStage` (6), mark `Graduated = true`.
   - Compute next interval: `BaseIntervals[stage]` or `GraduatedIntervalDays`.
   - Set `NextReviewDate = now + interval`.
   - Set `LastReviewDate = now`.
3. If `!correct`:
   - Reset `ConsecutiveHits` to 0.
   - Do **not** change stage or interval — the existing `CheckReviewPerformance` mechanism (spec 07) handles sustained poor performance by calling `MarkRusty` after 4 bad reviews. A single wrong answer just resets the consecutive hit counter.
   - Set `LastReviewDate = now`.

```go
func (s *Scheduler) RecordReview(skillID string, correct bool, now time.Time) {
    rs := s.reviews[skillID]
    if rs == nil {
        return
    }

    rs.LastReviewDate = now

    if correct {
        rs.ConsecutiveHits++

        // Advance stage
        if !rs.Graduated {
            rs.Stage++
            if rs.ConsecutiveHits >= GraduationStage {
                rs.Graduated = true
            }
        }

        // Compute next interval
        intervalDays := rs.CurrentIntervalDays()
        rs.NextReviewDate = now.AddDate(0, 0, intervalDays)
    } else {
        rs.ConsecutiveHits = 0
        // Don't change stage or next review date.
        // CheckReviewPerformance handles sustained failure (spec 07).
    }
}
```

### 4.4 InitSkill Flow

Called when a skill transitions to Mastered for the first time:

```go
func (s *Scheduler) InitSkill(skillID string, masteredAt time.Time) {
    s.reviews[skillID] = &ReviewState{
        SkillID:         skillID,
        Stage:           0,
        NextReviewDate:  masteredAt.AddDate(0, 0, BaseIntervals[0]),
        ConsecutiveHits: 0,
        Graduated:       false,
        LastReviewDate:  masteredAt,
    }
}
```

### 4.5 ReInitSkill Flow

Called when a skill recovers from Rusty → Mastered. Resets to stage 0:

```go
func (s *Scheduler) ReInitSkill(skillID string, now time.Time) {
    s.reviews[skillID] = &ReviewState{
        SkillID:         skillID,
        Stage:           0,
        NextReviewDate:  now.AddDate(0, 0, BaseIntervals[0]),
        ConsecutiveHits: 0,
        Graduated:       false,
        LastReviewDate:  now,
    }
}
```

---

## 5. Session Planner Integration

### 5.1 Replacing selectReviewSkills

The session planner's `selectReviewSkills` placeholder (spec 06) is replaced with a scheduler-aware implementation:

```go
// Updated in internal/session/planner.go

func (p *Planner) selectReviewSkills(masteredIDs []string, count int) []string {
    if p.scheduler == nil {
        // Fallback: least-recently-practiced (unchanged from spec 06)
        return p.selectReviewSkillsFallback(masteredIDs, count)
    }

    // Use scheduler's due list (most-overdue-first).
    due := p.scheduler.DueSkills(time.Now())

    // Cap to requested count.
    if len(due) > count {
        due = due[:count]
    }

    // If not enough due skills, don't pad — fewer review slots is fine.
    return due
}
```

### 5.2 Planner State Updates

The `Planner` struct gains a scheduler reference:

```go
// Updated Planner struct (addition only)
type Planner struct {
    // ... existing fields ...
    scheduler *spacedrep.Scheduler // nil if spaced rep not enabled
}
```

### 5.3 Session Start Integration

When a session starts:

1. Load snapshot.
2. Create mastery service from snapshot.
3. Create scheduler from snapshot + mastery service.
4. **Run decay check**: `scheduler.RunDecayCheck(ctx, time.Now())`.
5. If any skills became rusty, display notification on session screen.
6. Build session plan (scheduler's `DueSkills` feeds review slot selection).

### 5.4 Answer Recording Integration

When a review-category answer is recorded in `HandleAnswer`:

1. Existing flow: `MasteryService.RecordAnswer(...)` (spec 07).
2. **New**: `Scheduler.RecordReview(skillID, correct, time.Now())`.
3. Existing flow: `CheckReviewPerformance(...)` if review category (spec 07).

When a mastery transition occurs:

- **Learning → Mastered**: Call `Scheduler.InitSkill(skillID, time.Now())`.
- **Rusty → Mastered** (recovery): Call `Scheduler.ReInitSkill(skillID, time.Now())`.

---

## 6. Persistence

### 6.1 Snapshot Data

Review state is stored in the snapshot alongside mastery data:

```go
// Added to internal/store/repo.go

// SpacedRepSnapshotData holds all spaced repetition state for persistence.
type SpacedRepSnapshotData struct {
    Reviews map[string]*ReviewStateData `json:"reviews,omitempty"`
}

// ReviewStateData is the serialized form of ReviewState.
type ReviewStateData struct {
    SkillID         string `json:"skill_id"`
    Stage           int    `json:"stage"`
    NextReviewDate  string `json:"next_review_date"`  // RFC3339
    ConsecutiveHits int    `json:"consecutive_hits"`
    Graduated       bool   `json:"graduated"`
    LastReviewDate  string `json:"last_review_date"`  // RFC3339
}
```

### 6.2 SnapshotData Update

```go
// Updated SnapshotData
type SnapshotData struct {
    Version   int                      `json:"version"`
    Mastery   *MasterySnapshotData     `json:"mastery,omitempty"`
    SpacedRep *SpacedRepSnapshotData   `json:"spaced_rep,omitempty"` // NEW

    // Deprecated
    TierProgress map[string]*TierProgressData `json:"tier_progress,omitempty"`
    MasteredSet  []string                     `json:"mastered_set,omitempty"`
}
```

Version is incremented to indicate the new format.

### 6.3 Snapshot Loading

When loading a snapshot:

- If `SpacedRep` field is present, deserialize review states.
- If `SpacedRep` is nil but `Mastery` has mastered skills with `MasteredAt` timestamps, **bootstrap review states** from mastery data:
  - For each mastered skill, create a `ReviewState` at stage 0 with `NextReviewDate = MasteredAt + 1 day`.
  - This handles the migration from spec 07 (no spaced rep) to spec 08.

```go
// internal/spacedrep/snapshot.go

// BootstrapFromMastery creates initial review states for mastered skills
// that don't have existing spaced rep data. Used during migration.
func BootstrapFromMastery(masterySnap *store.MasterySnapshotData) *store.SpacedRepSnapshotData {
    data := &store.SpacedRepSnapshotData{
        Reviews: make(map[string]*store.ReviewStateData),
    }
    for skillID, skill := range masterySnap.Skills {
        if skill.State != "mastered" || skill.MasteredAt == nil {
            continue
        }
        masteredAt, err := time.Parse(time.RFC3339, *skill.MasteredAt)
        if err != nil {
            continue
        }
        nextReview := masteredAt.AddDate(0, 0, BaseIntervals[0])
        data.Reviews[skillID] = &store.ReviewStateData{
            SkillID:         skillID,
            Stage:           0,
            NextReviewDate:  nextReview.Format(time.RFC3339),
            ConsecutiveHits: 0,
            Graduated:       false,
            LastReviewDate:  masteredAt.Format(time.RFC3339),
        }
    }
    return data
}
```

---

## 7. UI Integration

### 7.1 Skill Map Enhancements

The skill map (spec 03) gains review schedule indicators for mastered skills:

```
  ✅ Place Value to 1000         Review in 12 days
  ✅ Add 2-Digit Numbers         Due for review
  ✅ Subtract 2-Digit Numbers    Overdue!
  🎓 Basic Multiplication        Graduated
  🔄 Division Facts              Rusty (recovering)
```

**Display rules:**

| Review Status | Icon | Label |
|---------------|------|-------|
| Not due | ✅ | "Review in N days" |
| Due | ✅ | "Due for review" (highlighted) |
| Overdue | ✅ | "Overdue!" (warning color) |
| Graduated | 🎓 | "Graduated" |
| Rusty | 🔄 | "Rusty (recovering)" |

The graduated icon (🎓) replaces the standard mastered icon (✅) for graduated skills.

### 7.2 Stats Command Enhancements

The stats command (spec 07, section 8) gains review schedule info:

```
Mathiz Stats
────────────────────────────────────

Skills: 5 mastered, 3 learning, 1 rusty, 43 new

Graduated Skills (3):
  🎓 Place Value to 1000          Next review: Mar 15
  🎓 Add 2-Digit Numbers          Next review: Apr 2
  🎓 Count by 5s and 10s          Next review: Mar 28

Upcoming Reviews:
  ✅ Subtract 3-Digit Numbers     Due in 2 days (stage 3)
  ✅ Multiply by 10               Due in 5 days (stage 2)
  ✅ Basic Division               Due in 11 days (stage 4)

Overdue:
  ⚠️  Compare Fractions            3 days overdue (stage 1)

Rusty Skills:
  🔄 Basic Multiplication         Needs recovery
```

### 7.3 Session Start Notification

When `RunDecayCheck` marks skills as rusty, the session screen shows a brief notification before the first question:

```
┌──────────────────────────────────────────────────┐
│                                                  │
│  ⏰  Some skills need refreshing!                │
│                                                  │
│  "Compare Fractions" hasn't been practiced       │
│  in a while and needs review.                    │
│                                                  │
│  Press any key to continue...                    │
│                                                  │
└──────────────────────────────────────────────────┘
```

If multiple skills became rusty:

```
│  ⏰  2 skills need refreshing!                   │
│                                                  │
│  • Compare Fractions                             │
│  • Long Division                                 │
```

---

## 8. Wiring & Dependency Injection

### 8.1 app.Options Update

```go
// Updated app.Options (addition only)
type Options struct {
    // ... existing fields from specs 06/07 ...
    Scheduler *spacedrep.Scheduler // Spaced repetition scheduler
}
```

### 8.2 cmd/play.go Wiring

The `play` command wires the scheduler:

1. Load snapshot via `SnapshotRepo.Latest()`.
2. Create mastery service: `mastery.NewService(snap, eventRepo)`.
3. Create scheduler: `spacedrep.NewScheduler(snap, masteryService, eventRepo)`.
4. Run decay check: `scheduler.RunDecayCheck(ctx, time.Now())`.
5. Pass scheduler to planner and session state via `app.Options`.

### 8.3 Session State Update

```go
// Updated SessionState (addition only)
type SessionState struct {
    // ... existing fields ...
    Scheduler *spacedrep.Scheduler
}
```

### 8.4 HandleAnswer Integration

The `HandleAnswer` function in `internal/session/session.go` is updated:

```go
func HandleAnswer(state *SessionState, learnerAnswer string) *mastery.StateTransition {
    // ... existing flow (spec 07) ...

    transition := state.MasteryService.RecordAnswer(q.SkillID, correct, responseTimeMs, tierCfg)

    // Update spaced rep schedule for review answers.
    if state.Scheduler != nil && state.CurrentSlot().Category == "review" {
        state.Scheduler.RecordReview(q.SkillID, correct, time.Now())
    }

    // Initialize spaced rep for newly mastered skills.
    if state.Scheduler != nil && transition != nil {
        switch {
        case transition.From == mastery.StateLearning && transition.To == mastery.StateMastered:
            state.Scheduler.InitSkill(q.SkillID, time.Now())
        case transition.From == mastery.StateRusty && transition.To == mastery.StateMastered:
            state.Scheduler.ReInitSkill(q.SkillID, time.Now())
        }
    }

    // ... rest of existing flow ...
    return transition
}
```

---

## 9. Package Structure

```
internal/
  spacedrep/
    schedule.go          # BaseIntervals, constants, interval computation
    review.go            # ReviewState type, IsDue, IsRustyThreshold, Status
    scheduler.go         # Scheduler service (RunDecayCheck, DueSkills, RecordReview, Init/ReInit)
    snapshot.go          # Snapshot serialization, BootstrapFromMastery
    schedule_test.go     # Interval computation tests
    review_test.go       # ReviewState method tests
    scheduler_test.go    # Scheduler integration tests
    snapshot_test.go     # Snapshot round-trip and migration tests
  store/
    repo.go              # Updated SnapshotData (+ SpacedRepSnapshotData)
```

---

## 10. Dependencies

| Dependency | Direction | What's Used |
|-----------|-----------|------------|
| `internal/mastery` | → imports | `Service.MarkRusty()`, `Service.GetMastery()`, `MasteryState`, `StateTransition` |
| `internal/store` | → imports | `EventRepo`, `SnapshotData`, `SpacedRepSnapshotData` |
| `internal/session` | ← consumed by | Planner calls `DueSkills()` for review selection; `HandleAnswer` calls `RecordReview` |
| `internal/screens/session` | ← consumed by | Displays decay check notification at session start |
| `internal/screens/skillmap` | ← consumed by | Displays review status badges and graduated icon |
| `cmd/play.go` | ← consumed by | Creates and wires the scheduler |

---

## 11. Testing Strategy

### 11.1 Interval Schedule Tests

```go
func TestBaseIntervals_Length(t *testing.T) {
    // Assert: 6 stages (indices 0–5)
}

func TestCurrentIntervalDays_EachStage(t *testing.T) {
    // Stage 0 → 1, Stage 1 → 3, ..., Stage 5 → 60
}

func TestCurrentIntervalDays_Graduated(t *testing.T) {
    // Graduated skill → 90 days
}
```

### 11.2 ReviewState Tests

```go
func TestIsDue_BeforeDate(t *testing.T) {
    // Now is before NextReviewDate → false
}

func TestIsDue_OnDate(t *testing.T) {
    // Now equals NextReviewDate → true
}

func TestIsDue_AfterDate(t *testing.T) {
    // Now is after NextReviewDate → true
}

func TestOverdueDays_NotDue(t *testing.T) {
    // Not yet due → 0
}

func TestOverdueDays_ThreeDaysOverdue(t *testing.T) {
    // 3 days past NextReviewDate → 3.0
}

func TestIsRustyThreshold_WithinGrace(t *testing.T) {
    // Stage 2 (7-day interval), 2 days overdue → grace is 3.5 days → not rusty
}

func TestIsRustyThreshold_PastGrace(t *testing.T) {
    // Stage 2 (7-day interval), 4 days overdue → grace is 3.5 days → rusty
}

func TestIsRustyThreshold_Stage0(t *testing.T) {
    // Stage 0 (1-day interval), 1 day overdue → grace is 0.5 days → rusty
}

func TestIsRustyThreshold_Graduated(t *testing.T) {
    // Graduated (90-day interval), 30 days overdue → grace is 45 days → not rusty
    // 50 days overdue → rusty
}

func TestStatus_NotDue(t *testing.T) {
    // Future review date → ReviewNotDue
}

func TestStatus_Due(t *testing.T) {
    // Past review date, within grace → ReviewDue
}

func TestStatus_Overdue(t *testing.T) {
    // Past grace period → ReviewOverdue
}

func TestStatus_Graduated(t *testing.T) {
    // Graduated, not due → ReviewGraduated
}

func TestDaysUntilReview(t *testing.T) {
    // 5 days in the future → 5
    // Already due → 0
}
```

### 11.3 Scheduler Tests

```go
func TestInitSkill_SetsStageZero(t *testing.T) {
    // After InitSkill: Stage=0, NextReview=mastered+1day, ConsecutiveHits=0
}

func TestRecordReview_Correct_AdvancesStage(t *testing.T) {
    // Stage 0, correct → Stage 1, NextReview=now+3days
}

func TestRecordReview_Correct_MultipleTimes(t *testing.T) {
    // 6 correct reviews → graduated
}

func TestRecordReview_Correct_Graduated_StaysGraduated(t *testing.T) {
    // Graduated skill, correct → still graduated, NextReview=now+90days
}

func TestRecordReview_Incorrect_ResetsConsecutiveHits(t *testing.T) {
    // 3 correct then 1 incorrect → ConsecutiveHits=0, Stage unchanged
}

func TestRecordReview_Incorrect_DoesNotChangeStage(t *testing.T) {
    // Stage 3, incorrect → Stage still 3
}

func TestReInitSkill_ResetsToStageZero(t *testing.T) {
    // Previously at stage 4, then rusty, then recovered
    // After ReInit: Stage=0, Graduated=false, ConsecutiveHits=0
}

func TestRunDecayCheck_MarksOverdueRusty(t *testing.T) {
    // Skill past rusty threshold → MarkRusty called → returns transition
}

func TestRunDecayCheck_SkipsWithinGrace(t *testing.T) {
    // Skill overdue but within grace → no transition
}

func TestRunDecayCheck_SkipsAlreadyRusty(t *testing.T) {
    // Skill already rusty → not processed
}

func TestRunDecayCheck_SkipsLearning(t *testing.T) {
    // Skill in Learning state → not processed
}

func TestDueSkills_SortedMostOverdueFirst(t *testing.T) {
    // 3 due skills with different overdue amounts → sorted correctly
}

func TestDueSkills_ExcludesNotDue(t *testing.T) {
    // Skills not yet due → excluded from list
}

func TestDueSkills_ExcludesRusty(t *testing.T) {
    // Rusty skills → excluded (handled as recovery, not review)
}
```

### 11.4 Graduation Tests

```go
func TestGraduation_After6Reviews(t *testing.T) {
    // Init skill, record 6 correct reviews
    // Assert: Graduated=true, ConsecutiveHits=6
}

func TestGraduation_ResetOnWrongAnswer(t *testing.T) {
    // 4 correct, 1 wrong, then 6 correct
    // Assert: Graduated after 6 consecutive correct (not counting pre-reset)
}

func TestGraduation_IntervalIs90Days(t *testing.T) {
    // Graduated skill → CurrentIntervalDays() = 90
}

func TestGraduation_SurvivesRecovery(t *testing.T) {
    // Graduated skill goes rusty → recovers → NOT graduated (reset)
    // Must re-earn graduation
}
```

### 11.5 Snapshot Tests

```go
func TestSnapshotRoundTrip(t *testing.T) {
    // Scheduler with various states → SnapshotData → NewScheduler → states match
}

func TestBootstrapFromMastery_CreateReviewStates(t *testing.T) {
    // Mastery data with 3 mastered skills
    // Bootstrap → 3 ReviewStates at stage 0
}

func TestBootstrapFromMastery_SkipsNonMastered(t *testing.T) {
    // Mastery data with learning/rusty skills → not bootstrapped
}

func TestBootstrapFromMastery_UsesCorrectDates(t *testing.T) {
    // MasteredAt = Jan 1 → NextReviewDate = Jan 2 (1 day later)
}

func TestNewScheduler_MigrationPath(t *testing.T) {
    // Snapshot with Mastery but no SpacedRep → bootstrap runs automatically
}
```

### 11.6 Planner Integration Tests

```go
func TestPlanner_UsesSchedulerForReviewSelection(t *testing.T) {
    // Scheduler has 3 due skills → planner picks most overdue for review slot
}

func TestPlanner_FallsBackWithoutScheduler(t *testing.T) {
    // No scheduler → uses least-recently-practiced heuristic
}

func TestPlanner_NoDueSkills_EmptyReviewSlots(t *testing.T) {
    // No skills due → review slots empty, more frontier slots available
}
```

---

## 12. Example Flow

### 12.1 Full Lifecycle: Mastery → Reviews → Graduation

**Day 0**: Learner masters "Add 3-Digit Numbers".
- `InitSkill("add-3digit", Day0)`
- ReviewState: Stage 0, NextReview = Day 1

**Day 1**: Session starts. Skill is due for review (stage 0).
- Planner includes it in review slot. Learner answers correctly.
- `RecordReview("add-3digit", true, Day1)`
- ReviewState: Stage 1, NextReview = Day 4, ConsecutiveHits = 1

**Day 4**: Due for review (stage 1, 3-day interval).
- Learner answers correctly.
- ReviewState: Stage 2, NextReview = Day 11, ConsecutiveHits = 2

**Day 11**: Due for review (stage 2, 7-day interval).
- Learner answers correctly.
- ReviewState: Stage 3, NextReview = Day 25, ConsecutiveHits = 3

**Day 25**: Due for review (stage 3, 14-day interval).
- Learner answers correctly.
- ReviewState: Stage 4, NextReview = Day 55, ConsecutiveHits = 4

**Day 55**: Due for review (stage 4, 30-day interval).
- Learner answers correctly.
- ReviewState: Stage 5, NextReview = Day 115, ConsecutiveHits = 5

**Day 115**: Due for review (stage 5, 60-day interval).
- Learner answers correctly.
- ReviewState: Stage 6, **Graduated = true**, NextReview = Day 205, ConsecutiveHits = 6
- Skill Map shows 🎓 icon.

**Day 205+**: Graduated reviews every 90 days.

### 12.2 Decay → Rusty → Recovery → Re-enter Schedule

**Day 0**: Skill "Compare Fractions" mastered. Stage 0, NextReview = Day 1.

**Day 4**: Skill was at stage 1 (3-day interval). Next review was Day 4. Learner plays on Day 4.
- Learner answers correctly.
- Stage 2, NextReview = Day 11.

**Day 18**: Learner hasn't played in a week. Session starts on Day 18.
- NextReview was Day 11. Overdue by 7 days. Grace = 7 × 0.5 = 3.5 days.
- Day 11 + 3.5 = Day 14.5. Current day 18 > 14.5. **Rusty threshold exceeded**.
- `RunDecayCheck` → `MarkRusty("compare-fractions")`.
- Notification: "Compare Fractions needs refreshing!"

**Day 18 session**: Planner includes "Compare Fractions" as recovery (frontier slot).
- 4 recovery questions at Learn difficulty. Learner gets 3/4 correct.
- Recovery passed → Rusty → Mastered.
- `ReInitSkill("compare-fractions", Day18)`.
- ReviewState: Stage 0, NextReview = Day 19, ConsecutiveHits = 0.
- Schedule restarts from the beginning.

### 12.3 Poor Review Performance → Rusty

**Day 30**: "Place Value to 1000" is mastered, stage 3. Due for review.
- Review question 1: Incorrect.
- Review question 2: Incorrect.
- `RecordReview` on each: ConsecutiveHits reset to 0 each time.
- Review question 3: Correct.
- Review question 4: Incorrect.
- `CheckReviewPerformance`: 1/4 = 25% < 50% → `MarkRusty`.
- Skill goes rusty via "review-performance" trigger (spec 07 mechanism).
- When recovered, `ReInitSkill` resets to stage 0.

---

## 13. Open Questions / Future Considerations

- **Spec 09 (Error Diagnosis)**: Misconception-tagged errors during review could trigger rusty earlier or require longer recovery. Not in MVP.
- **Spec 10 (AI Lessons)**: Could generate a micro-review lesson before a recovery check begins, helping the learner refresh before being tested.
- **Spec 11 (Rewards)**: Graduation could award a special "Retention Gem." Streak-based review gems (e.g., 3 reviews in a row without missing a date) are also possible.
- **Adaptive intervals**: A future version could adjust the interval multiplier based on fluency score (higher fluency → longer intervals). For MVP, the fixed schedule is sufficient.
- **Per-skill weighting**: Foundational skills with many dependents could get shorter intervals. Deferred — the uniform schedule is simpler and fluency captures difficulty indirectly.
- **Calendar awareness**: The scheduler could account for weekends, school breaks, or custom schedules. Deferred to post-MVP.
- **Review batching**: If many skills are due on the same day, the scheduler could spread them across multiple sessions. Currently handled by the 1–2 review slots per session cap.

---

## 14. Verification

The Spaced Repetition module is verified when:

- [ ] `internal/spacedrep/schedule.go` defines `BaseIntervals` (1, 3, 7, 14, 30, 60), `MaxStage`, `GraduationStage`, `GraduatedIntervalDays` (90)
- [ ] `internal/spacedrep/review.go` defines `ReviewState` with `Stage`, `NextReviewDate`, `ConsecutiveHits`, `Graduated`, `LastReviewDate`
- [ ] `ReviewState.IsDue(now)` returns true when current time is at or past the review date
- [ ] `ReviewState.IsRustyThreshold(now)` returns true when overdue by more than `interval × 0.5` days
- [ ] `ReviewState.Status(now)` returns correct `ReviewStatus` (not_due, due, overdue, graduated)
- [ ] `ReviewState.CurrentIntervalDays()` returns correct interval for each stage and graduated
- [ ] `internal/spacedrep/scheduler.go` defines `Scheduler` with `RunDecayCheck`, `DueSkills`, `RecordReview`, `InitSkill`, `ReInitSkill`
- [ ] `RunDecayCheck` iterates mastered skills, checks `IsRustyThreshold`, calls `MarkRusty` with trigger `"time-decay"`, logs mastery events
- [ ] `RunDecayCheck` runs at session start (wired in `cmd/play.go`)
- [ ] `DueSkills` returns mastered skills sorted by most-overdue-first, excludes rusty/learning skills
- [ ] `RecordReview` on correct answer: increments `ConsecutiveHits`, advances `Stage`, computes `NextReviewDate`
- [ ] `RecordReview` on incorrect answer: resets `ConsecutiveHits` to 0, does not change stage
- [ ] `InitSkill` creates `ReviewState` at stage 0 with `NextReviewDate = masteredAt + 1 day`
- [ ] `ReInitSkill` resets `ReviewState` to stage 0 (full reset after recovery)
- [ ] Graduation triggers after 6 consecutive successful reviews (`ConsecutiveHits >= 6`)
- [ ] Graduated skills use 90-day interval
- [ ] Graduation is lost on recovery (ReInitSkill resets `Graduated = false`)
- [ ] Session planner's `selectReviewSkills` uses `Scheduler.DueSkills()` when scheduler is available
- [ ] Planner falls back to least-recently-practiced heuristic when scheduler is nil
- [ ] `HandleAnswer` calls `Scheduler.RecordReview` for review-category answers
- [ ] `HandleAnswer` calls `Scheduler.InitSkill` on Learning → Mastered transitions
- [ ] `HandleAnswer` calls `Scheduler.ReInitSkill` on Rusty → Mastered transitions
- [ ] `store.SnapshotData` extended with `SpacedRep *SpacedRepSnapshotData`
- [ ] Snapshot round-trip: Scheduler → SnapshotData → NewScheduler → all review states preserved
- [ ] `BootstrapFromMastery` creates stage-0 review states for mastered skills without existing spaced rep data
- [ ] Skill map displays review schedule info (due date, overdue status, graduated badge)
- [ ] Stats command shows graduated skills, upcoming reviews, and overdue skills
- [ ] Session start shows notification when `RunDecayCheck` marks skills rusty
- [ ] All schedule, review state, scheduler, graduation, snapshot, and planner integration tests pass
- [ ] `CGO_ENABLED=0 go build ./...` succeeds
- [ ] `go test ./internal/spacedrep/... ./internal/session/... ./internal/store/...` passes
