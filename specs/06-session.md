# 06 — Session Engine

## 1. Overview

The Session Engine is the core gameplay loop of Mathiz. It orchestrates a **15-minute, auto-planned practice session** where the learner works through a mixed queue of math questions across multiple skills. There is no manual skill selection — the session planner builds the queue automatically from the skill graph, balancing frontier practice, spaced review, and confidence boosters.

**Design goals:**

- **Auto-planned**: The planner builds a skill queue using the 60/30/10 mix (frontier/review/booster). The learner just hits "Play."
- **Time-boxed**: Every session runs for exactly 15 minutes. When time expires, the learner finishes their current question and the session ends.
- **Mini-block rotation**: Questions are served in mini-blocks of ~3 per skill before rotating to the next skill in the plan. This balances focused practice with interleaving benefits.
- **Cumulative tier progress**: Progress toward tier completion (e.g., 5/8 Learn questions correct) persists across sessions. A learner doesn't need to complete a tier in a single sitting.
- **Feedback-rich**: After every answer, the learner sees whether they were correct and a short LLM-generated explanation.
- **Graceful exit**: The learner can quit early with a confirmation dialog. Progress is always saved.

### Consumers

| Module | How it uses Session Engine |
|--------|---------------------------|
| **Home Screen (01)** | "Play" button pushes the Session screen onto the router |
| **Mastery & Scoring (07)** | Reads session answer events to compute fluency scores and state transitions |
| **Spaced Repetition (08)** | Provides "due for review" skills to the planner (placeholder until spec 08) |
| **Rewards (11)** | Reads session events to award gems |

---

## 2. Session Planner

The planner runs once at session start. It builds an ordered list of **skill slots** — each slot is a skill + tier pair that will receive a mini-block of ~3 questions.

### 2.1 Plan Mix

The session budget is divided into three categories:

| Category | Share | Description |
|----------|-------|-------------|
| **Frontier** | 60% | Skills the learner is currently working on (available but not mastered). These advance the learner through the graph. |
| **Review** | 30% | Mastered skills due for review. Prevents decay and maintains automaticity. |
| **Booster** | 10% | Easy questions on well-mastered skills. Quick confidence wins to maintain motivation. |

Shares are applied to the total number of slots. With a 15-minute session and ~1 minute per question (including thinking, answering, and reading explanation), a session produces roughly 12-15 questions, which maps to 4-5 skill slots of 3 questions each.

**Slot allocation (for 5 total slots):**
- 3 frontier slots
- 1 review slot
- 1 booster slot

If fewer slots are available (e.g., no mastered skills for review), the budget is redistributed to frontier.

### 2.2 Frontier Skill Selection

Frontier skills are selected from `skillgraph.AvailableSkills(mastered)` using this priority:

1. **Lowest grade first** — fill foundational gaps before advancing
2. **Most dependents** — within the same grade, prefer skills that unlock the most downstream skills (higher-impact)
3. **Alphabetical by ID** — deterministic tiebreaker

The planner selects up to 3 distinct frontier skills (one per slot). If fewer than 3 are available, duplicates are allowed (the learner gets more practice on the same skill).

### 2.3 Review Skill Selection

**Placeholder implementation** (until spec 08 — Spaced Repetition):

Select mastered skills that were **least recently practiced**. The planner queries the most recent `AnswerEvent` timestamp for each mastered skill and picks the one with the oldest timestamp. If no mastered skills exist, the review slot is reallocated to frontier.

**Future**: Spec 08 will provide a `DueForReview(mastered) []Skill` API. The planner will call this instead.

### 2.4 Booster Selection

Select a well-mastered skill (highest historical accuracy) and serve it at **Learn tier** difficulty — easy wins. If no mastered skills exist, the booster slot is reallocated to frontier.

### 2.5 Tier Assignment

Each skill slot needs a tier (Learn or Prove). Until spec 07 (Mastery) defines the state machine:

- **Default**: All skills start at **Learn** tier.
- **Tier progression**: When a learner accumulates enough correct answers to meet the Learn tier's `AccuracyThreshold` over `ProblemsRequired` attempts (tracked cumulatively across sessions), the skill advances to Prove tier.
- **Booster slots**: Always use Learn tier regardless of skill state.
- **Review slots**: Use the skill's current tier.

### 2.6 Plan Data Structure

```go
// internal/session/planner.go

package session

// PlanCategory represents the reason a skill was included in the plan.
type PlanCategory string

const (
    CategoryFrontier PlanCategory = "frontier"
    CategoryReview   PlanCategory = "review"
    CategoryBooster  PlanCategory = "booster"
)

// PlanSlot is a single slot in the session plan — a skill + tier pair
// that will receive a mini-block of questions.
type PlanSlot struct {
    Skill    skillgraph.Skill
    Tier     skillgraph.Tier
    Category PlanCategory
}

// Plan is the ordered list of skill slots for a session.
type Plan struct {
    Slots    []PlanSlot
    Duration time.Duration // Always 15 minutes
}
```

### 2.7 Planner Interface

```go
// Planner builds a session plan from the current learner state.
type Planner interface {
    // BuildPlan creates a session plan. The mastered set determines which
    // skills are available, frontier, or mastered. The tierProgress map
    // tracks cumulative progress toward tier completion for each skill.
    BuildPlan(mastered map[string]bool, tierProgress map[string]*TierProgress) (*Plan, error)
}
```

---

## 3. Tier Progress Tracking

Tier progress is tracked **cumulatively across sessions**. This is the minimal state needed before spec 07 (Mastery) is implemented.

```go
// internal/session/progress.go

// TierProgress tracks cumulative progress toward completing a tier for a skill.
type TierProgress struct {
    SkillID       string
    CurrentTier   skillgraph.Tier
    TotalAttempts int     // Total questions attempted on this tier
    CorrectCount  int     // Total correct answers on this tier
    Accuracy      float64 // CorrectCount / TotalAttempts (computed)
}

// IsTierComplete returns true if the learner has met the tier's completion criteria.
func (tp *TierProgress) IsTierComplete(cfg skillgraph.TierConfig) bool {
    return tp.TotalAttempts >= cfg.ProblemsRequired &&
        tp.Accuracy >= cfg.AccuracyThreshold
}

// Record adds a new answer result to the progress.
func (tp *TierProgress) Record(correct bool) {
    tp.TotalAttempts++
    if correct {
        tp.CorrectCount++
    }
    if tp.TotalAttempts > 0 {
        tp.Accuracy = float64(tp.CorrectCount) / float64(tp.TotalAttempts)
    }
}
```

### Tier Advancement

When `IsTierComplete()` returns true:

- **Learn → Prove**: The skill advances to Prove tier. A new `TierProgress` is created for Prove with zero counts.
- **Prove → Mastered**: The skill is added to the `mastered` set. It becomes eligible for review/booster slots.

This logic lives in the session engine's answer-handling code, not in the `TierProgress` struct itself.

---

## 4. Session Lifecycle

A session progresses through these phases:

```
[Start] → [Plan] → [Serve Questions] → [End] → [Summary]
```

### 4.1 Start

1. Load learner state: mastered set and tier progress from the latest snapshot
2. Build session plan via `Planner.BuildPlan()`
3. Initialize session timer (15 minutes)
4. Persist a `SessionEvent` with `Action: "start"`
5. Begin serving questions from the first slot

### 4.2 Serve Questions (Main Loop)

The session iterates through plan slots. For each slot:

1. Generate a question via `problemgen.Generator.Generate()` with the slot's skill and tier
2. Display the question to the learner
3. Wait for the learner's answer (or early quit)
4. Check the answer via `problemgen.CheckAnswer()`
5. Show feedback: correct/incorrect indicator + the question's `Explanation`
6. Persist an `AnswerEvent`
7. Update `TierProgress` for the skill
8. Check for tier advancement
9. After 3 questions on the current slot, move to the next slot
10. If all slots are exhausted, cycle back to the first slot
11. If the session timer has expired, finish the current question and end

### 4.3 End

1. Stop the session timer
2. Persist a `SessionEvent` with `Action: "end"`
3. Save updated tier progress and mastered set to snapshot
4. Transition to the Session Summary screen

### 4.4 Early Quit

When the learner presses `Esc` or `q` during a session:

1. Show a confirmation dialog: "End session early? Progress will be saved. [Y/N]"
2. If confirmed: proceed to End phase (section 4.3)
3. If declined: resume the current question

---

## 5. Session State

```go
// internal/session/state.go

// SessionState tracks the runtime state of an active session.
type SessionState struct {
    // Plan is the session plan built at start.
    Plan *Plan

    // CurrentSlotIndex is the index into Plan.Slots for the current skill.
    CurrentSlotIndex int

    // QuestionsInSlot is the number of questions served in the current slot.
    QuestionsInSlot int

    // CurrentQuestion is the active question being displayed (nil between questions).
    CurrentQuestion *problemgen.Question

    // TotalQuestions is the count of questions served so far.
    TotalQuestions int

    // TotalCorrect is the count of correct answers so far.
    TotalCorrect int

    // PerSkillResults tracks per-skill stats for the summary screen.
    PerSkillResults map[string]*SkillResult

    // TierProgress tracks cumulative tier progress (loaded from snapshot, updated live).
    TierProgress map[string]*TierProgress

    // Mastered is the set of mastered skill IDs (loaded from snapshot, updated live).
    Mastered map[string]bool

    // StartTime is when the session began.
    StartTime time.Time

    // Elapsed tracks total elapsed time.
    Elapsed time.Duration

    // Phase is the current session phase.
    Phase SessionPhase

    // PriorQuestions tracks questions asked per skill in this session (for dedup).
    PriorQuestions map[string][]string

    // RecentErrors tracks recent errors per skill (for LLM context).
    RecentErrors map[string][]string

    // ShowingFeedback is true when the feedback overlay is displayed.
    ShowingFeedback bool

    // ShowingQuitConfirm is true when the quit confirmation dialog is displayed.
    ShowingQuitConfirm bool

    // LastAnswerCorrect records whether the most recent answer was correct (for feedback display).
    LastAnswerCorrect bool

    // TierAdvanced is set when a tier advancement happens, for feedback display.
    TierAdvanced *TierAdvancement
}

// SessionPhase represents the current phase of the session.
type SessionPhase int

const (
    PhaseLoading   SessionPhase = iota // Loading state from snapshot
    PhaseActive                        // Serving questions
    PhaseFeedback                      // Showing answer feedback
    PhaseEnding                        // Session time expired or quit confirmed
    PhaseSummary                       // Showing summary screen
)

// SkillResult tracks per-skill performance within a single session.
type SkillResult struct {
    SkillID   string
    SkillName string
    Category  PlanCategory
    Attempted int
    Correct   int
    TierBefore skillgraph.Tier
    TierAfter  skillgraph.Tier
}

// TierAdvancement records a tier transition for display purposes.
type TierAdvancement struct {
    SkillID   string
    SkillName string
    FromTier  skillgraph.Tier
    ToTier    skillgraph.Tier // TierProve or "mastered" sentinel
}
```

---

## 6. Persistence

### 6.1 Session Event (ent schema)

```go
// ent/schema/session_event.go

func (SessionEvent) Fields() []ent.Field {
    return []ent.Field{
        field.String("session_id").NotEmpty(),       // UUID, groups events in a session
        field.String("action").NotEmpty(),            // "start" or "end"
        field.Int("questions_served").Default(0),     // Total questions (on "end" only)
        field.Int("correct_answers").Default(0),      // Total correct (on "end" only)
        field.Int("duration_secs").Default(0),        // Actual duration in seconds (on "end" only)
        field.JSON("plan_summary", []PlanSlotSummary{}).Optional(), // Serialized plan (on "start" only)
    }
}

// PlanSlotSummary is the serialized form of a plan slot for persistence.
type PlanSlotSummary struct {
    SkillID  string       `json:"skill_id"`
    Tier     string       `json:"tier"`
    Category string       `json:"category"`
}
```

### 6.2 Answer Event (ent schema)

```go
// ent/schema/answer_event.go

func (AnswerEvent) Fields() []ent.Field {
    return []ent.Field{
        field.String("session_id").NotEmpty(),          // Links to SessionEvent
        field.String("skill_id").NotEmpty(),            // Skill this question was for
        field.String("tier").NotEmpty(),                // "learn" or "prove"
        field.String("category").NotEmpty(),            // "frontier", "review", "booster"
        field.String("question_text").NotEmpty(),       // The question shown
        field.String("correct_answer").NotEmpty(),      // The canonical correct answer
        field.String("learner_answer").NotEmpty(),      // What the learner entered
        field.Bool("correct"),                          // Whether the answer was correct
        field.Int("time_ms"),                           // Milliseconds to answer
        field.String("answer_format").NotEmpty(),       // "numeric" or "multiple_choice"
    }
}
```

Both schemas use the `EventMixin` (sequence + timestamp).

### 6.3 Snapshot Data

Tier progress and mastered set are stored in `SnapshotData`:

```go
// Additions to store.SnapshotData

type SnapshotData struct {
    Version      int                        `json:"version"`
    TierProgress map[string]*TierProgressData `json:"tier_progress,omitempty"`
    MasteredSet  []string                     `json:"mastered_set,omitempty"`
}

type TierProgressData struct {
    SkillID       string  `json:"skill_id"`
    CurrentTier   string  `json:"current_tier"`   // "learn" or "prove"
    TotalAttempts int     `json:"total_attempts"`
    CorrectCount  int     `json:"correct_count"`
}
```

### 6.4 EventRepo Additions

```go
// Added to store.EventRepo interface

// AppendSessionEvent records a session lifecycle event (start/end).
AppendSessionEvent(ctx context.Context, data SessionEventData) error

// AppendAnswerEvent records a single answer event.
AppendAnswerEvent(ctx context.Context, data AnswerEventData) error

// LatestAnswerTime returns the most recent answer timestamp for a skill,
// or zero time if no answers exist. Used by the planner for review selection.
LatestAnswerTime(ctx context.Context, skillID string) (time.Time, error)

// SkillAccuracy returns the historical accuracy for a skill (correct/total),
// or 0 if no answers exist. Used by the planner for booster selection.
SkillAccuracy(ctx context.Context, skillID string) (float64, error)
```

---

## 7. Session Screen (TUI)

The Session screen implements the `screen.Screen` interface and is the primary interactive surface during a session.

### 7.1 Layout

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ 🎯 Mathiz                                          Session  ⏱ 12:34        │
│ ─────────────────────────────────────────────────────────────────────────── │
│                                                                            │
│   Skill: Add 3-Digit Numbers                          Q 7/~15  ✓ 5        │
│   ───────────────────────────────────────────────────────────────────────   │
│                                                                            │
│                                                                            │
│                     What is 567 + 285?                                     │
│                                                                            │
│                                                                            │
│                     Answer: █                                              │
│                                                                            │
│                                                                            │
│                                                                            │
│                                                                            │
│                                                                            │
│                                                                            │
│                                                                            │
│                                                                            │
│ ─────────────────────────────────────────────────────────────────────────── │
│ enter Submit  │  esc Quit                                                  │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Header area:**
- Session timer counting down (MM:SS format)
- Current skill name
- Question count (e.g., "Q 7/~15" — approximate total since it's time-based)
- Correct count (e.g., "✓ 5")

**Content area:**
- Question text (centered, prominent)
- For numeric: a text input field
- For multiple choice: a numbered list of 4 options with arrow-key selection

**Footer:**
- `enter` Submit answer
- `esc` Quit session (shows confirmation)

### 7.2 Feedback Overlay

After submitting an answer, a feedback overlay appears for 3 seconds (or until the learner presses any key):

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                                                                            │
│                          ✓  Correct!                                       │
│                                                                            │
│   Step 1: Add ones: 7 + 5 = 12. Write 2, carry 1.                        │
│   Step 2: Add tens: 6 + 8 + 1 = 15. Write 5, carry 1.                    │
│   Step 3: Add hundreds: 5 + 2 + 1 = 8.                                   │
│   Answer: 852                                                              │
│                                                                            │
│                     Press any key to continue...                           │
│                                                                            │
└──────────────────────────────────────────────────────────────────────────────┘
```

For incorrect answers:

```
│                          ✗  Not quite                                      │
│                          Correct answer: 852                               │
```

### 7.3 Tier Advancement Notification

When a tier advances, an additional notification is shown after the feedback:

```
│                     ⭐  Level up!                                          │
│              "Add 3-Digit Numbers" — Prove tier unlocked!                  │
```

Or for mastery:

```
│                     🏆  Skill mastered!                                    │
│              "Add 3-Digit Numbers" — fully mastered!                       │
```

### 7.4 Quit Confirmation Dialog

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                                                                            │
│                   End session early?                                        │
│                   Your progress will be saved.                              │
│                                                                            │
│                   [Y] Yes, end session                                      │
│                   [N] No, keep going                                        │
│                                                                            │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 7.5 Multiple Choice Input

For multiple choice questions, the layout changes to show numbered options:

```
│                     Which fraction is equivalent to 2/4?                   │
│                                                                            │
│                     1) 3/4                                                 │
│                     2) 1/2                                                 │
│                     3) 2/3                                                 │
│                     4) 1/4                                                 │
│                                                                            │
│                     Select (1-4): █                                        │
```

The learner can type 1-4 or use arrow keys to highlight and press Enter.

---

## 8. Session Summary Screen

After a session ends (time up or early quit), the Summary screen is pushed onto the router.

### 8.1 Layout

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ 🎯 Mathiz                                       Session Summary            │
│ ─────────────────────────────────────────────────────────────────────────── │
│                                                                            │
│   Session complete!                         Duration: 15:00                │
│                                                                            │
│   Questions: 14        Correct: 11        Accuracy: 78%                   │
│                                                                            │
│   ───── Skills ──────────────────────────────────────────────────────────  │
│                                                                            │
│   Add 3-Digit Numbers (frontier)          6/8 correct   Learn ▸ Prove     │
│   Subtract 3-Digit Numbers (frontier)     3/3 correct   Learn             │
│   Place Value to 1000 (review)            2/3 correct   Mastered          │
│                                                                            │
│   ───── Gems Earned ────────────────────────────────────────────────────  │
│                                                                            │
│   (Gems display placeholder — see spec 11)                                │
│                                                                            │
│                                                                            │
│ ─────────────────────────────────────────────────────────────────────────── │
│ enter Continue  │  esc Home                                                │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 8.2 Summary Data

```go
// internal/session/summary.go

// SessionSummary holds the data displayed on the summary screen.
type SessionSummary struct {
    Duration       time.Duration
    TotalQuestions int
    TotalCorrect   int
    Accuracy       float64
    SkillResults   []SkillResult   // Per-skill breakdown
    // GemsEarned  []Gem           // Placeholder for spec 11
}
```

---

## 9. Session Screen Implementation

### 9.1 Screen Interface

```go
// internal/screens/session/session.go

package session

// SessionScreen implements screen.Screen for the active session.
type SessionScreen struct {
    state     *SessionState
    generator problemgen.Generator
    eventRepo store.EventRepo
    snapRepo  store.SnapshotRepo
    input     textinput.Model   // For numeric answer input
    ticker    tea.Tick           // 1-second timer tick
}

// New creates a new SessionScreen with injected dependencies.
func New(generator problemgen.Generator, eventRepo store.EventRepo, snapRepo store.SnapshotRepo) *SessionScreen
```

### 9.2 Messages

```go
// Internal messages used by the session screen.

// questionReadyMsg is sent when a question has been generated.
type questionReadyMsg struct {
    Question *problemgen.Question
    Err      error
}

// questionGenFailedMsg is sent when question generation fails after retries.
type questionGenFailedMsg struct {
    Err error
}

// timerTickMsg is sent every second to update the countdown.
type timerTickMsg time.Time

// feedbackDoneMsg is sent when the feedback display period ends.
type feedbackDoneMsg struct{}
```

### 9.3 Question Generation

Questions are generated asynchronously using `tea.Cmd` to avoid blocking the UI:

```go
func (s *SessionScreen) generateNextQuestion() tea.Cmd {
    return func() tea.Msg {
        slot := s.state.Plan.Slots[s.state.CurrentSlotIndex]
        input := problemgen.GenerateInput{
            Skill:          slot.Skill,
            Tier:           slot.Tier,
            PriorQuestions: s.state.PriorQuestions[slot.Skill.ID],
            RecentErrors:   s.state.RecentErrors[slot.Skill.ID],
        }

        var q *problemgen.Question
        var err error
        for attempt := 0; attempt < 3; attempt++ {
            q, err = s.generator.Generate(context.Background(), input)
            if err == nil {
                break
            }
            var valErr *problemgen.ValidationError
            if errors.As(err, &valErr) && !valErr.Retryable {
                break
            }
        }
        if err != nil {
            return questionGenFailedMsg{Err: err}
        }
        return questionReadyMsg{Question: q}
    }
}
```

### 9.4 Answer Handling

When the learner submits an answer:

1. Record answer time (milliseconds since question was shown)
2. Call `problemgen.CheckAnswer(learnerAnswer, question)`
3. Update `SessionState` (totals, per-skill results, tier progress)
4. Build error context string if incorrect (for `RecentErrors`)
5. Persist `AnswerEvent`
6. Check for tier advancement
7. Transition to feedback phase
8. After feedback, advance to next question or next slot

### 9.5 Timer

A 1-second ticker drives the countdown display:

```go
func tickCmd() tea.Cmd {
    return tea.Tick(time.Second, func(t time.Time) tea.Msg {
        return timerTickMsg(t)
    })
}
```

On each tick:
- Update `state.Elapsed`
- If `state.Elapsed >= state.Plan.Duration` and not currently showing feedback:
  - Allow current question to be finished (if answering)
  - After the current question's feedback, end the session

### 9.6 Error Recovery

If question generation fails (all 3 retry attempts exhausted):

1. Log the error
2. Skip to the next slot in the plan
3. If all slots fail, show an error message and end the session with whatever progress exists
4. The learner should never be stuck — always either get a question or get moved along

---

## 10. Slot Cycling

The session rotates through plan slots in a round-robin pattern:

```
Slot 0 (frontier: Add 3-Digit) → 3 questions
Slot 1 (frontier: Subtract 3-Digit) → 3 questions
Slot 2 (frontier: Multiply 1-Digit) → 3 questions
Slot 3 (review: Place Value) → 3 questions
Slot 4 (booster: Add 2-Digit) → 3 questions
Slot 0 (frontier: Add 3-Digit) → 3 questions  ← cycle repeats
...until time expires
```

After completing a mini-block of 3 questions on a slot, advance `CurrentSlotIndex`. When `CurrentSlotIndex` reaches the end of the slots, wrap back to index 0.

**Exception**: If a skill's tier is completed during a mini-block (tier advancement happens), the remaining questions in that slot are skipped and the session advances to the next slot. The slot is removed from the rotation for subsequent cycles.

---

## 11. Error Context Construction

When a learner answers incorrectly, the session engine constructs an error description string for `GenerateInput.RecentErrors`:

```go
func buildErrorContext(question *problemgen.Question, learnerAnswer string) string {
    return fmt.Sprintf(
        "Answered %s for '%s', correct answer was %s",
        learnerAnswer,
        question.Text,
        question.Answer,
    )
}
```

These are stored per-skill in `SessionState.RecentErrors` and trimmed to the 5 most recent.

---

## 12. Dependencies

| Dependency | Direction | What's Used |
|-----------|-----------|------------|
| `internal/problemgen` | → imports | `Generator`, `GenerateInput`, `Question`, `CheckAnswer` |
| `internal/skillgraph` | → imports | `Skill`, `Tier`, `TierConfig`, `AvailableSkills`, `FrontierSkills` |
| `internal/screen` | → implements | `Screen` interface, `KeyHintProvider` |
| `internal/router` | → uses | `PushScreenMsg`, `PopScreenMsg` for navigation |
| `internal/store` | → imports | `EventRepo`, `SnapshotRepo`, `SnapshotData` |
| `internal/ui/theme` | → imports | Colors, styles |
| `internal/ui/layout` | → imports | `KeyHint` for footer |
| `internal/ui/components` | → imports | Shared UI components |
| `charm.land/bubbles/v2/textinput` | → imports | Text input for answer entry |
| Home Screen (01) | ← consumed by | Pushes SessionScreen onto router |
| Mastery & Scoring (07) | ← consumed by | Reads `AnswerEvent` records |
| Spaced Repetition (08) | ← consumed by | Will provide review skill selection |
| Rewards (11) | ← consumed by | Reads session events to award gems |

---

## 13. Package Structure

```
internal/
  session/
    planner.go          # Planner interface, DefaultPlanner, plan building logic
    plan.go             # Plan, PlanSlot, PlanCategory types
    progress.go         # TierProgress, TierProgressData, tier advancement logic
    state.go            # SessionState, SessionPhase, SkillResult types
    summary.go          # SessionSummary type
    session.go          # Core session logic (answer handling, slot cycling, error context)
    planner_test.go     # Planner tests (mix allocation, skill selection, edge cases)
    progress_test.go    # TierProgress tests (recording, completion, advancement)
    session_test.go     # Session logic tests (answer handling, slot cycling)
  screens/
    session/
      session.go        # SessionScreen (screen.Screen implementation, TUI rendering)
      messages.go       # Internal messages (questionReadyMsg, timerTickMsg, etc.)
      view.go           # View rendering (question display, feedback overlay, quit dialog)
      session_test.go   # Screen tests (key handling, phase transitions, rendering)
    summary/
      summary.go        # SummaryScreen (screen.Screen implementation)
      view.go           # Summary view rendering
      summary_test.go   # Summary screen tests
  store/
    repo.go             # Updated EventRepo interface (+ AppendSessionEvent, AppendAnswerEvent, etc.)
  ent/schema/
    session_event.go    # SessionEvent ent schema
    answer_event.go     # AnswerEvent ent schema
```

---

## 14. App Integration

### 14.1 Options Update

```go
// Updated internal/app/options.go

type Options struct {
    LLMProvider  llm.Provider
    EventRepo    store.EventRepo
    SnapshotRepo store.SnapshotRepo
    Generator    problemgen.Generator  // Added: problem generator
}
```

### 14.2 Home Screen "Play" Action

The Home screen's "Play" button creates and pushes a new SessionScreen:

```go
// In home screen's Update() handler:
case "enter": // Play selected
    sessionScreen := sessionscreen.New(
        opts.Generator,
        opts.EventRepo,
        opts.SnapshotRepo,
    )
    return s, router.Push(sessionScreen)
```

### 14.3 Session End Navigation

When the session ends, the SessionScreen pushes the SummaryScreen. When the learner dismisses the summary, it pops back to Home:

```go
// In session screen, after session ends:
return s, func() tea.Msg {
    return router.PushScreenMsg{Screen: summary.New(sessionSummary)}
}

// In summary screen, on Enter or Esc:
return s, func() tea.Msg {
    return router.PopScreenMsg{} // Pops summary
}
// The session screen behind it also needs to pop — handled by popping twice
// or by having the session screen detect it's no longer active and self-pop.
```

A cleaner pattern: the session screen replaces itself with the summary screen by returning both a `PopScreenMsg` (to remove itself) and a `PushScreenMsg` (to add the summary). Since Bubble Tea processes commands sequentially, this results in the summary screen sitting on top of the Home screen.

---

## 15. Testing Strategy

### 15.1 Planner Tests

```go
func TestBuildPlan_AllFrontier(t *testing.T) {
    // No mastered skills → all slots are frontier
    // Assert: 5 frontier slots, 0 review, 0 booster
}

func TestBuildPlan_MixedPlan(t *testing.T) {
    // Some mastered skills available
    // Assert: 3 frontier, 1 review, 1 booster (60/30/10 rounded)
}

func TestBuildPlan_FrontierPriority(t *testing.T) {
    // Multiple frontier skills at different grades
    // Assert: lowest grade skills selected first
}

func TestBuildPlan_FrontierTiebreaker(t *testing.T) {
    // Multiple frontier skills at same grade
    // Assert: skills with more dependents preferred
}

func TestBuildPlan_NoFrontierSkills(t *testing.T) {
    // All skills mastered → all slots are review/booster
}

func TestBuildPlan_ReviewSelection(t *testing.T) {
    // Multiple mastered skills with different last-practiced times
    // Assert: least recently practiced skill selected for review
}

func TestBuildPlan_BoosterSelection(t *testing.T) {
    // Multiple mastered skills with different accuracy
    // Assert: highest accuracy skill selected for booster, Learn tier
}

func TestBuildPlan_Duration(t *testing.T) {
    // Assert: plan duration is always 15 minutes
}
```

### 15.2 Tier Progress Tests

```go
func TestTierProgress_Record(t *testing.T) {
    // Record correct → increments both TotalAttempts and CorrectCount
    // Record incorrect → increments only TotalAttempts
    // Accuracy is correctly computed
}

func TestTierProgress_IsTierComplete_Met(t *testing.T) {
    // 8 attempts, 7 correct (87.5%) vs 75% threshold → true
}

func TestTierProgress_IsTierComplete_NotEnoughAttempts(t *testing.T) {
    // 5 attempts, 5 correct (100%) but need 8 → false
}

func TestTierProgress_IsTierComplete_LowAccuracy(t *testing.T) {
    // 8 attempts, 5 correct (62.5%) vs 75% threshold → false
}
```

### 15.3 Session Logic Tests

```go
func TestSlotCycling_AfterThreeQuestions(t *testing.T) {
    // After 3 questions on slot 0, advances to slot 1
}

func TestSlotCycling_Wraparound(t *testing.T) {
    // After exhausting all slots, wraps back to slot 0
}

func TestSlotCycling_SkipCompletedTier(t *testing.T) {
    // When tier completes mid-block, skip remaining questions and advance
}

func TestAnswerHandling_Correct(t *testing.T) {
    // Correct answer → TotalCorrect incremented, SkillResult updated
}

func TestAnswerHandling_Incorrect(t *testing.T) {
    // Incorrect answer → error context added to RecentErrors
}

func TestAnswerHandling_TierAdvancement(t *testing.T) {
    // Correct answer completes Learn tier → TierAdvanced set, new Prove progress created
}

func TestAnswerHandling_Mastery(t *testing.T) {
    // Correct answer completes Prove tier → skill added to Mastered set
}

func TestErrorContext_Construction(t *testing.T) {
    // buildErrorContext produces expected format string
}

func TestErrorContext_Limit(t *testing.T) {
    // RecentErrors per skill capped at 5 most recent
}

func TestSessionTimer_Expiry(t *testing.T) {
    // When timer reaches 15 minutes, session enters ending phase
}

func TestSessionTimer_FinishCurrentQuestion(t *testing.T) {
    // Timer expires while answering → learner can still submit, then session ends
}

func TestEarlyQuit_Confirmed(t *testing.T) {
    // Quit confirmed → session ends, events persisted, summary shown
}

func TestEarlyQuit_Declined(t *testing.T) {
    // Quit declined → resume current question
}
```

### 15.4 Screen Tests

```go
func TestSessionScreen_Init(t *testing.T) {
    // Screen init loads state, builds plan, starts timer, generates first question
}

func TestSessionScreen_AnswerSubmit(t *testing.T) {
    // Enter key submits answer, transitions to feedback phase
}

func TestSessionScreen_FeedbackDismiss(t *testing.T) {
    // Any key during feedback → advance to next question
}

func TestSessionScreen_QuitConfirm(t *testing.T) {
    // Esc → shows dialog, Y → ends session, N → resumes
}

func TestSessionScreen_MultipleChoice(t *testing.T) {
    // Number keys 1-4 select an option
}

func TestSessionScreen_TimerDisplay(t *testing.T) {
    // Timer counts down correctly and displays MM:SS
}

func TestSummaryScreen_Display(t *testing.T) {
    // Summary shows total stats and per-skill breakdown
}

func TestSummaryScreen_Navigation(t *testing.T) {
    // Enter or Esc pops back to home
}
```

### 15.5 Persistence Tests

```go
func TestAppendSessionEvent_Start(t *testing.T) {
    // SessionEvent with action "start" and plan summary is persisted
}

func TestAppendSessionEvent_End(t *testing.T) {
    // SessionEvent with action "end", question count, accuracy is persisted
}

func TestAppendAnswerEvent(t *testing.T) {
    // AnswerEvent with all fields is persisted correctly
}

func TestSnapshotData_TierProgress(t *testing.T) {
    // TierProgress survives save/load cycle via SnapshotData
}

func TestSnapshotData_MasteredSet(t *testing.T) {
    // MasteredSet survives save/load cycle via SnapshotData
}
```

---

## 16. Example Flow

A complete session flow for a learner with 3 mastered skills and 10 available frontier skills:

**1. Learner presses "Play" on Home screen.**

**2. Planner builds a plan:**

```go
plan := &Plan{
    Slots: []PlanSlot{
        {Skill: "add-3digit",  Tier: TierLearn, Category: CategoryFrontier},  // Grade 3, 8 dependents
        {Skill: "sub-3digit",  Tier: TierLearn, Category: CategoryFrontier},  // Grade 3, 6 dependents
        {Skill: "mult-1digit", Tier: TierLearn, Category: CategoryFrontier},  // Grade 3, 5 dependents
        {Skill: "place-value", Tier: TierProve, Category: CategoryReview},    // Mastered, least recent
        {Skill: "add-2digit",  Tier: TierLearn, Category: CategoryBooster},   // Mastered, highest accuracy
    },
    Duration: 15 * time.Minute,
}
```

**3. Session starts. Timer begins counting down from 15:00.**

**4. Slot 0 — "Add 3-Digit Numbers" (frontier, Learn):**

- Q1: "What is 345 + 278?" → Learner answers "623" → Correct! Explanation shown.
- Q2: "What is 567 + 285?" → Learner answers "842" → Incorrect. Correct answer: 852. Explanation shown. Error context saved.
- Q3: "What is 194 + 638?" → Learner answers "832" → Correct! Explanation shown.

**5. Advance to Slot 1 — "Subtract 3-Digit Numbers" (frontier, Learn):**

- Q4-Q6: Three subtraction questions...

**6. Continue through Slots 2-4, then cycle back to Slot 0...**

**7. At 14:42, timer shows 0:18 remaining. Learner is on Q14.**

**8. At 15:00, timer expires. Learner is mid-answer on Q14.**

**9. Learner submits answer for Q14. Feedback shown.**

**10. After feedback, session ends. Events persisted.**

**11. Summary screen pushed:**

```
Session complete!                         Duration: 15:12

Questions: 14        Correct: 11        Accuracy: 78%

Add 3-Digit Numbers (frontier)          5/6 correct   Learn
Subtract 3-Digit Numbers (frontier)     3/3 correct   Learn ▸ Prove
Multiply 1-Digit (frontier)             2/3 correct   Learn
Place Value to 1000 (review)            1/2 correct   Mastered

Gems: (placeholder for spec 11)
```

**12. Learner presses Enter → returns to Home screen.**

---

## 17. Open Questions / Future Considerations

- **Spec 07 (Mastery)** will replace the simple tier tracking with a full state machine (new → learning → mastered → rusty) and fluency scoring.
- **Spec 08 (Spaced Repetition)** will provide a proper `DueForReview()` API to replace the least-recently-practiced heuristic.
- **Spec 10 (AI Lessons)** will add hint generation hooks. The session screen will need a hint button/key for Learn tier questions.
- **Spec 11 (Rewards)** will add gem awarding logic and populate the summary screen's gems section.
- **Adaptive session length**: Future versions may allow the learner to choose session duration (5/10/15/20 min).
- **Skill pinning**: Future versions may let the learner pin a specific skill for focused practice within an auto-planned session.
- **Session pause/resume**: Pausing a session mid-question and resuming later is not supported in MVP. The timer is wall-clock based.

---

## 18. Verification

The Session Engine module is verified when:

- [ ] `internal/session/plan.go` defines `Plan`, `PlanSlot`, `PlanCategory` types
- [ ] `internal/session/planner.go` defines `Planner` interface and `DefaultPlanner` implementation
- [ ] `DefaultPlanner.BuildPlan()` allocates slots with 60/30/10 mix, falling back to frontier when review/booster slots can't be filled
- [ ] Frontier skills are prioritized by lowest grade, then most dependents, then alphabetical ID
- [ ] Review skill selection uses least-recently-practiced heuristic via `LatestAnswerTime`
- [ ] Booster skill selection uses highest historical accuracy via `SkillAccuracy`, assigned Learn tier
- [ ] `internal/session/progress.go` defines `TierProgress` with `Record()` and `IsTierComplete()`
- [ ] Tier progress persists across sessions via `SnapshotData.TierProgress`
- [ ] Tier advancement triggers when Learn tier completes (→ Prove) and when Prove tier completes (→ mastered)
- [ ] `internal/session/state.go` defines `SessionState`, `SessionPhase`, `SkillResult`
- [ ] `ent/schema/session_event.go` defines `SessionEvent` with EventMixin
- [ ] `ent/schema/answer_event.go` defines `AnswerEvent` with EventMixin
- [ ] `store.EventRepo` interface updated with `AppendSessionEvent`, `AppendAnswerEvent`, `LatestAnswerTime`, `SkillAccuracy`
- [ ] `SnapshotData` extended with `TierProgress` and `MasteredSet`
- [ ] Session screen implements `screen.Screen` and `screen.KeyHintProvider`
- [ ] Session screen generates questions asynchronously via `tea.Cmd`
- [ ] Session screen implements the answer flow: submit → check → feedback → next question
- [ ] Feedback overlay shows correct/incorrect + explanation, dismisses on any key
- [ ] Tier advancement shows a notification overlay
- [ ] Mini-block rotation: 3 questions per slot, then advance; wrap around at end
- [ ] Completed tiers are removed from rotation
- [ ] Session timer counts down from 15:00; on expiry, finishes current question then ends
- [ ] Early quit shows confirmation dialog; Y ends session with progress saved, N resumes
- [ ] Multiple choice questions render 4 options and accept 1-4 input
- [ ] Error context strings built for incorrect answers and stored per-skill (max 5)
- [ ] Prior questions tracked per-skill for deduplication
- [ ] Summary screen displays total stats, per-skill breakdown, and gems placeholder
- [ ] Summary screen pops back to Home on Enter or Esc
- [ ] All planner, progress, session logic, screen, and persistence tests pass
- [ ] `CGO_ENABLED=0 go build ./...` succeeds
- [ ] `go test ./internal/session/... ./internal/screens/session/... ./internal/screens/summary/...` passes
