# Spec 11 — Rewards (Gems)

## 1. Overview

The Rewards system awards **gems** to the learner as visual, motivational markers of real learning milestones. Gems are purely cosmetic — they do not unlock content or features — but they provide tangible acknowledgment of mastery, retention, recovery, streaks, and session completion. Every gem has a **type** and a **rarity** that together make progress feel concrete and rewarding.

The system adds two new screens: **Gem Vault** (a rich collection view of all earned gems) and **History** (past sessions and gem awards). It also integrates inline gem-award notifications into the session screen and a recap section in the session summary.

### Design goals

- **Real milestones only** — every gem maps to a genuine learning event (mastery, retention, recovery, streak, session completion); no inflated or participation-only awards
- **Achievement-difficulty rarity** — rarity is determined per gem type by the difficulty of the achievement, not by grade level, so it scales naturally to K-12
- **Immediate gratification** — inline notifications during session for gem awards, with full recap on the summary screen
- **No shame mechanics** — gems celebrate progress; there are no penalties for failing to earn gems
- **Purely motivational** — gems are visual markers only; no unlock gates, no spending economy
- **Scalable** — rarity formulas and quartile splits auto-adapt as the skill graph grows

### Consumers

| Module | Relationship | What it uses |
|--------|-------------|-------------|
| Session screen (06) | → awards gems | Calls `GemService.Award*` on mastery/recovery/streak/session events |
| Session summary (06) | → displays gems | Reads `GemsEarned` field from `SessionSummary` |
| Gem Vault screen (11) | → displays collection | Reads from `EventRepo.QueryGemEvents` |
| History screen (11) | → displays timeline | Reads from `EventRepo.QuerySessionEvents` + `QueryGemEvents` |
| Home screen (01) | → shows gem count | Reads total count from snapshot `Gems` field |
| Snapshot (02) | → persists aggregate | `GemsSnapshotData` in `SnapshotData` |

---

## 2. Gem Types

Five gem types, each tied to a distinct learning achievement:

| Type | ID | Trigger | Description |
|------|----|---------|-------------|
| **Mastery** | `mastery` | Skill transitions Learning → Mastered (trigger: `prove-complete`) | Awarded when a skill is mastered for the first time |
| **Recovery** | `recovery` | Skill transitions Rusty → Mastered (trigger: `recovery-complete`) | Awarded when a rusty skill is recovered |
| **Retention** | `retention` | Spaced-rep graduation (`ConsecutiveHits >= 6`) | Awarded when a skill graduates from the review schedule |
| **Streak** | `streak` | 5 consecutive correct answers within a session | Awarded each time a 5-answer streak is achieved |
| **Session** | `session` | Session timer expires naturally (no early quit) | Awarded for completing a full session |

```go
// internal/gems/types.go

package gems

// GemType identifies the category of achievement.
type GemType string

const (
    GemMastery   GemType = "mastery"
    GemRecovery  GemType = "recovery"
    GemRetention GemType = "retention"
    GemStreak    GemType = "streak"
    GemSession   GemType = "session"
)

// AllGemTypes returns all gem types in display order.
func AllGemTypes() []GemType {
    return []GemType{GemMastery, GemRecovery, GemRetention, GemStreak, GemSession}
}

// DisplayName returns a human-readable label for the gem type.
func (t GemType) DisplayName() string {
    switch t {
    case GemMastery:
        return "Mastery"
    case GemRecovery:
        return "Recovery"
    case GemRetention:
        return "Retention"
    case GemStreak:
        return "Streak"
    case GemSession:
        return "Session"
    default:
        return string(t)
    }
}

// Icon returns the display icon for the gem type.
func (t GemType) Icon() string {
    switch t {
    case GemMastery:
        return "💎"
    case GemRecovery:
        return "🔥"
    case GemRetention:
        return "🛡️"
    case GemStreak:
        return "⚡"
    case GemSession:
        return "🏆"
    default:
        return "✦"
    }
}
```

---

## 3. Rarity System

Four rarity tiers, determined **per gem type** by the difficulty of the specific achievement. This is completely independent of grade level and scales automatically as the skill graph grows.

| Rarity | ID | Color | Description |
|--------|----|-------|-------------|
| **Common** | `common` | `theme.Text` (white/default) | Baseline achievements |
| **Rare** | `rare` | `theme.Info` (blue) | Above-average difficulty |
| **Epic** | `epic` | `theme.Primary` (purple) | High difficulty |
| **Legendary** | `legendary` | `theme.Warning` (gold/amber) | Exceptional achievements |

```go
// internal/gems/rarity.go

package gems

// Rarity represents the difficulty tier of a gem.
type Rarity string

const (
    RarityCommon    Rarity = "common"
    RarityRare      Rarity = "rare"
    RarityEpic      Rarity = "epic"
    RarityLegendary Rarity = "legendary"
)

// AllRarities returns all rarities in order from lowest to highest.
func AllRarities() []Rarity {
    return []Rarity{RarityCommon, RarityRare, RarityEpic, RarityLegendary}
}

// DisplayName returns a human-readable label for the rarity.
func (r Rarity) DisplayName() string {
    switch r {
    case RarityCommon:
        return "Common"
    case RarityRare:
        return "Rare"
    case RarityEpic:
        return "Epic"
    case RarityLegendary:
        return "Legendary"
    default:
        return string(r)
    }
}
```

### 3.1 Rarity Rules by Gem Type

Each gem type has its own rarity formula. The key principle: **harder achievements → rarer gems**.

#### Mastery Gems — DAG Depth Quartiles

Rarity is based on the **depth** of the skill in the prerequisite DAG (longest path from any root to this skill). Skills deeper in the graph require more prior mastery, making them harder achievements.

- Compute `depth(skill)` = longest path from any root to the skill (roots have depth 0)
- Divide all skills into quartiles by depth:
  - Q1 (bottom 25%): **Common**
  - Q2 (25-50%): **Rare**
  - Q3 (50-75%): **Epic**
  - Q4 (top 25%): **Legendary**

The quartile boundaries are computed once at startup from the skill graph and cached. As the graph grows (new grades, new strands), the quartiles shift automatically.

```go
// internal/gems/depth.go

package gems

import "github.com/abhisek/mathiz/internal/skillgraph"

// DepthMap holds the DAG depth for each skill and the quartile boundaries.
type DepthMap struct {
    Depths     map[string]int // skillID → depth
    Boundaries [3]int         // Q1/Q2, Q2/Q3, Q3/Q4 boundaries
}

// ComputeDepthMap computes DAG depths for all skills and quartile boundaries.
// Depth = longest path from any root to this skill.
func ComputeDepthMap() *DepthMap {
    skills := skillgraph.TopologicalOrder()
    depths := make(map[string]int, len(skills))

    // In topological order, compute longest path from root.
    for _, s := range skills {
        depth := 0
        for _, prereqID := range s.Prerequisites {
            if d, ok := depths[prereqID]; ok && d+1 > depth {
                depth = d + 1
            }
        }
        depths[s.ID] = depth
    }

    // Collect all depths and sort for quartile computation.
    vals := make([]int, 0, len(depths))
    for _, d := range depths {
        vals = append(vals, d)
    }
    sort.Ints(vals)

    n := len(vals)
    boundaries := [3]int{
        vals[n/4],     // Q1/Q2 boundary
        vals[n/2],     // Q2/Q3 boundary
        vals[3*n/4],   // Q3/Q4 boundary
    }

    return &DepthMap{Depths: depths, Boundaries: boundaries}
}

// RarityForSkill returns the rarity based on a skill's DAG depth.
func (dm *DepthMap) RarityForSkill(skillID string) Rarity {
    depth := dm.Depths[skillID]
    switch {
    case depth > dm.Boundaries[2]:
        return RarityLegendary
    case depth > dm.Boundaries[1]:
        return RarityEpic
    case depth > dm.Boundaries[0]:
        return RarityRare
    default:
        return RarityCommon
    }
}
```

#### Recovery Gems — DAG Depth Quartiles (same as Mastery)

Recovery gems use the same DAG depth quartile system. Recovering a deep skill is harder than recovering a shallow one because the learner must demonstrate competence on more advanced material.

#### Retention Gems — DAG Depth Quartiles (same as Mastery)

Awarded when a skill graduates from spaced-rep review (6 consecutive successful reviews). Rarity is based on the skill's DAG depth, since retaining mastery of a deeper skill over time is a greater achievement.

#### Streak Gems — Streak Length

Streak gems are awarded each time the learner achieves a consecutive correct-answer streak within a single session. The streak counter resets on any wrong answer.

| Streak Length | Rarity |
|---------------|--------|
| 5 | Common |
| 10 | Rare |
| 15 | Epic |
| 20+ | Legendary |

Gems are awarded **at each threshold**, not retroactively. Reaching 10 earns two gems: a Common at 5 and a Rare at 10.

```go
// StreakThresholds defines the streak lengths that award gems.
var StreakThresholds = []struct {
    Length int
    Rarity Rarity
}{
    {5, RarityCommon},
    {10, RarityRare},
    {15, RarityEpic},
    {20, RarityLegendary},
}
```

#### Session Gems — Session Accuracy

Session gems are awarded when the timer expires naturally (no early quit). Rarity is based on session accuracy:

| Accuracy | Rarity |
|----------|--------|
| < 50% | Common |
| 50–74% | Rare |
| 75–89% | Epic |
| 90%+ | Legendary |

Completing a session is always rewarded; higher accuracy earns a rarer gem.

---

## 4. Gem Data Model

### 4.1 GemAward struct

A `GemAward` represents a single gem earned during a session. It is used both in-memory (during session) and for event persistence.

```go
// internal/gems/gem.go

package gems

import "time"

// GemAward represents a single gem earned.
type GemAward struct {
    Type      GemType
    Rarity    Rarity
    SkillID   string // empty for session/streak gems
    SkillName string // empty for session/streak gems
    SessionID string
    Reason    string    // human-readable reason, e.g. "Mastered 3-Digit Addition"
    AwardedAt time.Time
}
```

### 4.2 Ent Schema — GemEvent

```go
// ent/schema/gem_event.go

package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/schema/field"
    "entgo.io/ent/schema/index"
    "entgo.io/ent/schema/mixin"
)

// GemEvent records a gem award.
type GemEvent struct {
    ent.Schema
}

func (GemEvent) Mixin() []ent.Mixin {
    return []ent.Mixin{EventMixin{}}
}

func (GemEvent) Fields() []ent.Field {
    return []ent.Field{
        field.String("gem_type").NotEmpty(),
        field.String("rarity").NotEmpty(),
        field.String("skill_id").Optional().Nillable(),
        field.String("skill_name").Optional().Nillable(),
        field.String("session_id").NotEmpty(),
        field.String("reason").NotEmpty(),
    }
}

func (GemEvent) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("gem_type"),
        index.Fields("session_id"),
        index.Fields("rarity"),
    }
}
```

### 4.3 Store Types

```go
// Addition to store/repo.go

// GemEventData captures the data for a gem award event.
type GemEventData struct {
    GemType   string
    Rarity    string
    SkillID   *string // nil for session/streak gems
    SkillName *string
    SessionID string
    Reason    string
}

// GemEventRecord is a hydrated gem event for display (includes timestamp).
type GemEventRecord struct {
    GemEventData
    Sequence  int64
    Timestamp time.Time
}
```

### 4.4 Snapshot Extension

```go
// Addition to store/repo.go

// GemsSnapshotData holds aggregate gem counts for quick loading.
type GemsSnapshotData struct {
    TotalCount  int            `json:"total_count"`
    CountByType map[string]int `json:"count_by_type"` // gem_type → count
}
```

The `SnapshotData` struct gets a new field:

```go
// Modified in store/repo.go
type SnapshotData struct {
    Version        int                          `json:"version"`
    Mastery        *MasterySnapshotData         `json:"mastery,omitempty"`
    SpacedRep      *SpacedRepSnapshotData       `json:"spaced_rep,omitempty"`
    LearnerProfile *LearnerProfileData          `json:"learner_profile,omitempty"`
    Gems           *GemsSnapshotData            `json:"gems,omitempty"` // NEW
    // ...deprecated fields...
}
```

Snapshot version increments to **4**.

---

## 5. GemService

The `GemService` is the core orchestrator. It computes rarity, creates `GemAward` records, persists them via `EventRepo`, and maintains in-session state for summary and inline notifications.

```go
// internal/gems/service.go

package gems

import (
    "context"
    "fmt"
    "time"

    "github.com/abhisek/mathiz/internal/skillgraph"
    "github.com/abhisek/mathiz/internal/store"
)

// Service manages gem computation and award tracking.
type Service struct {
    depthMap  *DepthMap
    eventRepo store.EventRepo

    // SessionGems accumulates gems awarded during the current session.
    SessionGems []GemAward
}

// NewService creates a GemService with precomputed depth map.
func NewService(eventRepo store.EventRepo) *Service {
    return &Service{
        depthMap:  ComputeDepthMap(),
        eventRepo: eventRepo,
    }
}

// AwardMastery awards a mastery gem for a newly mastered skill.
func (s *Service) AwardMastery(ctx context.Context, skillID, skillName, sessionID string) *GemAward {
    rarity := s.depthMap.RarityForSkill(skillID)
    award := &GemAward{
        Type:      GemMastery,
        Rarity:    rarity,
        SkillID:   skillID,
        SkillName: skillName,
        SessionID: sessionID,
        Reason:    fmt.Sprintf("Mastered %s", skillName),
        AwardedAt: time.Now(),
    }
    s.persist(ctx, award)
    s.SessionGems = append(s.SessionGems, *award)
    return award
}

// AwardRecovery awards a recovery gem for a recovered rusty skill.
func (s *Service) AwardRecovery(ctx context.Context, skillID, skillName, sessionID string) *GemAward {
    rarity := s.depthMap.RarityForSkill(skillID)
    award := &GemAward{
        Type:      GemRecovery,
        Rarity:    rarity,
        SkillID:   skillID,
        SkillName: skillName,
        SessionID: sessionID,
        Reason:    fmt.Sprintf("Recovered %s", skillName),
        AwardedAt: time.Now(),
    }
    s.persist(ctx, award)
    s.SessionGems = append(s.SessionGems, *award)
    return award
}

// AwardRetention awards a retention gem for a graduated skill.
func (s *Service) AwardRetention(ctx context.Context, skillID, skillName, sessionID string) *GemAward {
    rarity := s.depthMap.RarityForSkill(skillID)
    award := &GemAward{
        Type:      GemRetention,
        Rarity:    rarity,
        SkillID:   skillID,
        SkillName: skillName,
        SessionID: sessionID,
        Reason:    fmt.Sprintf("Retained %s", skillName),
        AwardedAt: time.Now(),
    }
    s.persist(ctx, award)
    s.SessionGems = append(s.SessionGems, *award)
    return award
}

// AwardStreak awards a streak gem for consecutive correct answers.
func (s *Service) AwardStreak(ctx context.Context, streakLength int, sessionID string) *GemAward {
    rarity := StreakRarity(streakLength)
    award := &GemAward{
        Type:      GemStreak,
        Rarity:    rarity,
        SessionID: sessionID,
        Reason:    fmt.Sprintf("%d correct in a row!", streakLength),
        AwardedAt: time.Now(),
    }
    s.persist(ctx, award)
    s.SessionGems = append(s.SessionGems, *award)
    return award
}

// AwardSession awards a session-completion gem.
func (s *Service) AwardSession(ctx context.Context, accuracy float64, sessionID string) *GemAward {
    rarity := SessionRarity(accuracy)
    award := &GemAward{
        Type:      GemSession,
        Rarity:    rarity,
        SessionID: sessionID,
        Reason:    fmt.Sprintf("Session complete (%.0f%% accuracy)", accuracy*100),
        AwardedAt: time.Now(),
    }
    s.persist(ctx, award)
    s.SessionGems = append(s.SessionGems, *award)
    return award
}

// ResetSession clears the session gem accumulator. Called at session start.
func (s *Service) ResetSession() {
    s.SessionGems = nil
}

// SnapshotData builds the gem counts for snapshot persistence.
func (s *Service) SnapshotData(ctx context.Context) *store.GemsSnapshotData {
    counts, total, _ := s.eventRepo.GemCounts(ctx)
    return &store.GemsSnapshotData{
        TotalCount:  total,
        CountByType: counts,
    }
}

func (s *Service) persist(ctx context.Context, award *GemAward) {
    data := store.GemEventData{
        GemType:   string(award.Type),
        Rarity:    string(award.Rarity),
        SessionID: award.SessionID,
        Reason:    award.Reason,
    }
    if award.SkillID != "" {
        data.SkillID = &award.SkillID
        data.SkillName = &award.SkillName
    }
    _ = s.eventRepo.AppendGemEvent(ctx, data)
}

// StreakRarity returns the rarity for a given streak length.
func StreakRarity(length int) Rarity {
    switch {
    case length >= 20:
        return RarityLegendary
    case length >= 15:
        return RarityEpic
    case length >= 10:
        return RarityRare
    default:
        return RarityCommon
    }
}

// SessionRarity returns the rarity for a given session accuracy.
func SessionRarity(accuracy float64) Rarity {
    switch {
    case accuracy >= 0.90:
        return RarityLegendary
    case accuracy >= 0.75:
        return RarityEpic
    case accuracy >= 0.50:
        return RarityRare
    default:
        return RarityCommon
    }
}
```

---

## 6. Streak Tracking

The session needs a consecutive-correct counter. This is tracked on `SessionState` and reset on any wrong answer.

```go
// Addition to internal/session/state.go

type SessionState struct {
    // ... existing fields ...

    // ConsecutiveCorrect tracks the current correct-answer streak within the session.
    ConsecutiveCorrect int

    // NextStreakThreshold is the next streak milestone that will award a gem.
    // Starts at 5 and advances through 10, 15, 20 as each is reached.
    NextStreakThreshold int

    // GemService is the gem service for awarding gems (nil if rewards disabled).
    GemService *gems.Service

    // PendingGemAward is set when a gem is earned, for inline display on the feedback screen.
    PendingGemAward *gems.GemAward
}
```

`NewSessionState` initializes `NextStreakThreshold` to **5**.

### Streak logic in HandleAnswer

After the existing answer processing in `HandleAnswer`:

```go
// Pseudocode addition to HandleAnswer in internal/session/session.go

if correct {
    state.ConsecutiveCorrect++

    // Check streak gem thresholds.
    if state.GemService != nil && state.ConsecutiveCorrect >= state.NextStreakThreshold {
        award := state.GemService.AwardStreak(ctx, state.ConsecutiveCorrect, state.SessionID)
        state.PendingGemAward = award
        // Advance to next threshold.
        state.NextStreakThreshold = nextStreakThreshold(state.ConsecutiveCorrect)
    }
} else {
    state.ConsecutiveCorrect = 0
    // Reset threshold back to the base.
    state.NextStreakThreshold = 5
}
```

The `nextStreakThreshold` helper returns the next milestone above the current streak length:

```go
func nextStreakThreshold(current int) int {
    thresholds := []int{5, 10, 15, 20}
    for _, t := range thresholds {
        if t > current {
            return t
        }
    }
    // Beyond 20, award every 5.
    return ((current / 5) + 1) * 5
}
```

---

## 7. Award Integration Points

### 7.1 Mastery and Recovery Gems — in `submitAnswer()`

After the existing mastery transition persistence in `submitAnswer()` (session screen), check for mastery/recovery:

```go
// Modified in internal/screens/session/session.go → submitAnswer()

if s.state.MasteryTransition != nil && s.state.GemService != nil {
    t := s.state.MasteryTransition
    switch {
    case t.From == mastery.StateLearning && t.To == mastery.StateMastered:
        award := s.state.GemService.AwardMastery(ctx, t.SkillID, t.SkillName, s.state.SessionID)
        s.state.PendingGemAward = award
    case t.From == mastery.StateRusty && t.To == mastery.StateMastered:
        award := s.state.GemService.AwardRecovery(ctx, t.SkillID, t.SkillName, s.state.SessionID)
        s.state.PendingGemAward = award
    }
}
```

If both a streak gem and a mastery gem trigger on the same answer, **the mastery/recovery gem takes display priority** (it is the rarer event). The streak gem is still awarded and persisted; it just doesn't get the inline notification. Both appear in the summary.

### 7.2 Retention Gems — in `RecordReview()`

The spaced-rep `RecordReview` already tracks `ConsecutiveHits`. When graduation occurs, the session screen awards a retention gem.

```go
// Modified in internal/session/session.go → handleAnswerWithMastery()
// After the existing RecordReview call:

if state.SpacedRepSched != nil && slot != nil && slot.Category == CategoryReview {
    state.SpacedRepSched.RecordReview(q.SkillID, correct, time.Now())

    // Check for graduation (retention gem).
    if correct && state.GemService != nil {
        rs := state.SpacedRepSched.GetReviewState(q.SkillID)
        if rs != nil && rs.Graduated && rs.ConsecutiveHits == spacedrep.GraduationStage {
            skill, _ := skillgraph.GetSkill(q.SkillID)
            award := state.GemService.AwardRetention(ctx, q.SkillID, skill.Name, state.SessionID)
            state.PendingGemAward = award
        }
    }
}
```

The `GetReviewState` method needs to be exposed through the `SpacedRepScheduler` interface:

```go
// Addition to internal/session/state.go
type SpacedRepScheduler interface {
    RecordReview(skillID string, correct bool, now time.Time)
    InitSkill(skillID string, masteredAt time.Time)
    ReInitSkill(skillID string, now time.Time)
    GetReviewState(skillID string) *spacedrep.ReviewState // NEW
}
```

### 7.3 Session Gems — in `handleSessionEnd()`

A session gem is awarded only when the session timer expires naturally (not on early quit via Esc+Y).

```go
// Modified in internal/screens/session/session.go → handleSessionEnd()

// Award session gem if timer expired (not early quit).
if s.state.TimeExpired && s.state.GemService != nil {
    accuracy := float64(0)
    if s.state.TotalQuestions > 0 {
        accuracy = float64(s.state.TotalCorrect) / float64(s.state.TotalQuestions)
    }
    s.state.GemService.AwardSession(ctx, accuracy, s.state.SessionID)
}
```

### 7.4 Summary Integration

The `SessionSummary` struct gains a gems field:

```go
// Modified in internal/session/summary.go

type SessionSummary struct {
    Duration       time.Duration
    TotalQuestions int
    TotalCorrect   int
    Accuracy       float64
    SkillResults   []SkillResult
    GemsEarned     []gems.GemAward // NEW — populated from GemService.SessionGems
}
```

`BuildSummary` copies the gems:

```go
// Modified in BuildSummary

func BuildSummary(state *SessionState) *SessionSummary {
    // ... existing code ...

    summary := &SessionSummary{
        Duration:       state.Elapsed,
        TotalQuestions: state.TotalQuestions,
        TotalCorrect:  state.TotalCorrect,
        Accuracy:       accuracy,
        SkillResults:   results,
    }

    if state.GemService != nil {
        summary.GemsEarned = state.GemService.SessionGems
    }

    return summary
}
```

---

## 8. Inline Gem Notification

When `state.PendingGemAward` is non-nil, the feedback overlay shows a celebratory gem notification **below** the correct/incorrect feedback.

### Feedback overlay addition

```
  ✓ Correct!

  💎 Legendary Mastery Gem
  "Mastered Long Division"
```

The notification renders the gem icon, rarity, type, and reason. It is displayed for the duration of the feedback overlay (until any key dismisses it).

After the feedback is dismissed (`handleFeedbackDone`), `PendingGemAward` is cleared along with the other feedback state:

```go
// Modified in handleFeedbackDone()
s.state.PendingGemAward = nil
```

### Rendering

```go
// Addition to internal/screens/session/render.go (in renderFeedback)

if s.state.PendingGemAward != nil {
    award := s.state.PendingGemAward
    gemLine := fmt.Sprintf("%s %s %s Gem",
        award.Type.Icon(),
        award.Rarity.DisplayName(),
        award.Type.DisplayName())
    reasonLine := fmt.Sprintf("\"%s\"", award.Reason)

    // Render with rarity-appropriate color
    gemStyle := lipgloss.NewStyle().
        Bold(true).
        Foreground(rarityColor(award.Rarity))

    b.WriteString("\n\n")
    b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center,
        gemStyle.Render(gemLine)))
    b.WriteString("\n")
    b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center,
        lipgloss.NewStyle().Foreground(theme.TextDim).Italic(true).
            Render(reasonLine)))
}
```

Rarity-to-color mapping:

```go
func rarityColor(r gems.Rarity) lipgloss.Color {
    switch r {
    case gems.RarityCommon:
        return theme.Text
    case gems.RarityRare:
        return theme.Info
    case gems.RarityEpic:
        return theme.Primary
    case gems.RarityLegendary:
        return theme.Warning
    default:
        return theme.Text
    }
}
```

---

## 9. Summary Screen — Gems Section

Replace the existing `(Gems display — see spec 11)` placeholder with a real gems section.

```
  Session complete!

  Duration: 5:00

  Questions: 12        Correct: 10        Accuracy: 83%

                          Skills
  ──────────────────────────────────────────────────────
    3-Digit Addition (frontier)    8/8 correct    Learn > Prove
    Long Division (review)         2/4 correct    Prove

                           Gems
  ──────────────────────────────────────────────────────
    💎 Epic Mastery Gem — Mastered 3-Digit Addition
    ⚡ Common Streak Gem — 5 correct in a row!
    🏆 Epic Session Gem — Session complete (83% accuracy)
```

Each gem line shows: icon, rarity (colored), type, and reason. If no gems were earned, this section is omitted.

```go
// Modified in internal/screens/summary/summary.go → View()

if len(sum.GemsEarned) > 0 {
    b.WriteString("\n")
    b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center,
        lipgloss.NewStyle().Foreground(theme.TextDim).Render("Gems")))
    b.WriteString("\n")
    b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, divider))
    b.WriteString("\n\n")

    for _, gem := range sum.GemsEarned {
        line := fmt.Sprintf("  %s %s %s Gem — %s",
            gem.Type.Icon(),
            gem.Rarity.DisplayName(),
            gem.Type.DisplayName(),
            gem.Reason)
        style := lipgloss.NewStyle().Foreground(rarityColor(gem.Rarity))
        b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center,
            style.Render(line)))
        b.WriteString("\n")
    }
}
```

---

## 10. EventRepo Extensions

### New methods on EventRepo

```go
// Addition to store/repo.go EventRepo interface

// AppendGemEvent records a gem award event.
AppendGemEvent(ctx context.Context, data GemEventData) error

// QueryGemEvents returns gem events matching the query options.
QueryGemEvents(ctx context.Context, opts QueryOpts) ([]GemEventRecord, error)

// GemCounts returns gem counts grouped by type and the total count.
GemCounts(ctx context.Context) (byType map[string]int, total int, err error)

// QuerySessionSummaries returns session end events for the history screen.
QuerySessionSummaries(ctx context.Context, opts QueryOpts) ([]SessionSummaryRecord, error)
```

### SessionSummaryRecord

```go
// Addition to store/repo.go

// SessionSummaryRecord is a hydrated session event for the history screen.
type SessionSummaryRecord struct {
    SessionID       string
    Timestamp       time.Time
    QuestionsServed int
    CorrectAnswers  int
    DurationSecs    int
    GemCount        int // gems awarded in this session
}
```

---

## 11. Gem Vault Screen

The Gem Vault is a rich collection view of all earned gems, grouped by gem type with a visual grid layout.

### Layout

```
                          Gem Vault
  ──────────────────────────────────────────────────────

  Total: 42 gems

  💎 Mastery (12)     🔥 Recovery (3)     🛡️ Retention (5)
  ⚡ Streak (15)      🏆 Session (7)

  ──────────────────────────────────────────────────────
  [Tab] to switch type         [↑↓] to scroll

                     💎 Mastery Gems (12)
  ──────────────────────────────────────────────────────

  Epic      3-Digit Addition          Feb 14, 2026
  Legendary Long Division             Feb 13, 2026
  Common    Place Value Basics        Feb 12, 2026
  Rare      2-Digit Multiplication    Feb 11, 2026
  ...
```

### Screen Structure

```go
// internal/screens/gemvault/gemvault.go

package gemvault

// GemVaultScreen displays the learner's gem collection.
type GemVaultScreen struct {
    eventRepo    store.EventRepo
    gems         []store.GemEventRecord
    selectedType int    // index into AllGemTypes
    scrollOffset int
    loaded       bool
}

var _ screen.Screen = (*GemVaultScreen)(nil)
var _ screen.KeyHintProvider = (*GemVaultScreen)(nil)

func New(eventRepo store.EventRepo) *GemVaultScreen {
    return &GemVaultScreen{
        eventRepo: eventRepo,
    }
}
```

### Behavior

- **Init**: Loads all `GemEvent` records from `EventRepo.QueryGemEvents` asynchronously
- **Tab / Shift-Tab**: Cycles through gem types (mastery, recovery, retention, streak, session)
- **Up/Down / j/k**: Scrolls through the gem list for the selected type
- **Esc**: Returns to home screen (PopScreenMsg)
- **Display**: Each gem shows rarity (colored), skill name (if applicable), and award date
- Gems are sorted by most recent first

### Key Hints

```go
func (s *GemVaultScreen) KeyHints() []layout.KeyHint {
    return []layout.KeyHint{
        {Key: "Tab", Description: "Switch type"},
        {Key: "↑↓", Description: "Scroll"},
        {Key: "Esc", Description: "Back"},
    }
}
```

---

## 12. History Screen

The History screen shows a combined view of past sessions and gem awards.

### Layout

```
                           History
  ──────────────────────────────────────────────────────

  Feb 15, 2026  5:00  12 questions  83% accuracy  3 gems
  Feb 14, 2026  5:00   9 questions  78% accuracy  1 gem
  Feb 13, 2026  5:00  15 questions  93% accuracy  5 gems
  ...

  ──────────────────────────────────────────────────────
  [Enter] Session details     [↑↓] Scroll     [Esc] Back
```

Selecting a session with Enter expands it to show gem details inline:

```
  Feb 15, 2026  5:00  12 questions  83% accuracy  3 gems
    💎 Epic Mastery Gem — Mastered 3-Digit Addition
    ⚡ Common Streak Gem — 5 correct in a row!
    🏆 Epic Session Gem — Session complete (83%)
```

### Screen Structure

```go
// internal/screens/history/history.go

package history

// HistoryScreen displays past sessions and gem awards.
type HistoryScreen struct {
    eventRepo store.EventRepo
    sessions  []store.SessionSummaryRecord
    gems      map[string][]store.GemEventRecord // sessionID → gems
    selected  int
    expanded  map[int]bool
    loaded    bool
}

var _ screen.Screen = (*HistoryScreen)(nil)
var _ screen.KeyHintProvider = (*HistoryScreen)(nil)

func New(eventRepo store.EventRepo) *HistoryScreen {
    return &HistoryScreen{
        eventRepo: eventRepo,
        expanded:  make(map[int]bool),
    }
}
```

### Behavior

- **Init**: Loads session summaries and gem events asynchronously
- **Up/Down / j/k**: Navigate sessions
- **Enter**: Toggle expand/collapse to show gems for that session
- **Esc**: Back to home
- Sessions sorted most recent first
- Shows last 50 sessions (paginated if needed in future)

### Key Hints

```go
func (s *HistoryScreen) KeyHints() []layout.KeyHint {
    return []layout.KeyHint{
        {Key: "Enter", Description: "Details"},
        {Key: "↑↓", Description: "Navigate"},
        {Key: "Esc", Description: "Back"},
    }
}
```

---

## 13. Home Screen Integration

### Gem Count Display

The home screen shows a total gem count below the greeting, loaded from the snapshot's `GemsSnapshotData`:

```
  Hey there, math explorer!
  Ready to level up today? ✦

  ✦ 42 gems

  > Start Practice
    Skill Map
    Gem Vault
    History
    Settings
```

### Implementation

The `HomeScreen` constructor receives the `SnapshotRepo` (already passed) and reads the gem count from the latest snapshot:

```go
// Modified in internal/screens/home/home.go

type HomeScreen struct {
    menu     components.Menu
    gemCount int // total gems from snapshot
}

func New(generator problemgen.Generator, eventRepo store.EventRepo, snapRepo store.SnapshotRepo, diagService *diagnosis.Service) *HomeScreen {
    // Load gem count from snapshot.
    var gemCount int
    if snapRepo != nil {
        snap, err := snapRepo.Latest(context.Background())
        if err == nil && snap != nil && snap.Data.Gems != nil {
            gemCount = snap.Data.Gems.TotalCount
        }
    }

    // ... existing menu setup, replacing Gem Vault placeholder ...
    items := []components.MenuItem{
        // ... existing items ...
        {Label: "Gem Vault", Action: func() tea.Cmd {
            return func() tea.Msg {
                return router.PushScreenMsg{Screen: gemvault.New(eventRepo)}
            }
        }},
        {Label: "History", Action: func() tea.Cmd {
            return func() tea.Msg {
                return router.PushScreenMsg{Screen: history.New(eventRepo)}
            }
        }},
        // ...
    }

    return &HomeScreen{menu: components.NewMenu(items), gemCount: gemCount}
}
```

In `View`, render the gem count if > 0:

```go
if h.gemCount > 0 {
    gemLine := fmt.Sprintf("✦ %d gems", h.gemCount)
    gemDisplay := lipgloss.NewStyle().
        Width(width).
        Foreground(theme.Warning).
        Align(lipgloss.Center).
        Render(gemLine)
    sections = append(sections, gemDisplay)
}
```

---

## 14. Dependency Injection Wiring

### app.Options

```go
// Modified in internal/app/app.go

type Options struct {
    LLMProvider      llm.Provider
    EventRepo        store.EventRepo
    SnapshotRepo     store.SnapshotRepo
    Generator        problemgen.Generator
    DiagnosisService *diagnosis.Service
    GemService       *gems.Service // NEW
}
```

### Session Screen

The `GemService` is passed through to the session state:

```go
// Modified in internal/screens/session/session.go → New()

type SessionScreen struct {
    // ... existing fields ...
    gemService *gems.Service
}

func New(generator problemgen.Generator, eventRepo store.EventRepo, snapRepo store.SnapshotRepo, diagService *diagnosis.Service, gemService *gems.Service) *SessionScreen {
    return &SessionScreen{
        // ... existing fields ...
        gemService: gemService,
    }
}
```

In `initSession()`, wire the gem service into the session state:

```go
state.GemService = s.gemService
if s.gemService != nil {
    s.gemService.ResetSession()
}
```

### Home Screen

```go
// Modified in internal/screens/home/home.go → New()

func New(generator problemgen.Generator, eventRepo store.EventRepo, snapRepo store.SnapshotRepo, diagService *diagnosis.Service, gemService *gems.Service) *HomeScreen
```

The `gemService` is needed by the session screen constructor. The home screen passes it through when creating session screens.

### cmd/play.go

```go
// Modified in cmd/play.go

gemService := gems.NewService(eventRepo)
opts := app.Options{
    // ... existing fields ...
    GemService: gemService,
}
```

---

## 15. Snapshot Persistence

### Save — in `saveSnapshot()`

```go
// Modified in internal/screens/session/session.go → saveSnapshot()

if s.gemService != nil {
    snapData.Gems = s.gemService.SnapshotData(ctx)
}
```

Snapshot version increments from 3 to **4**.

### Load — backward compatibility

If `snap.Data.Gems` is `nil`, the home screen shows 0 gems. The `GemService.SnapshotData` will re-aggregate from `GemEvent` records on the next session save. No migration needed.

---

## 16. Package Structure

```
internal/gems/
├── types.go       # GemType, AllGemTypes, DisplayName, Icon
├── rarity.go      # Rarity, AllRarities, StreakRarity, SessionRarity
├── depth.go       # DepthMap, ComputeDepthMap, RarityForSkill
├── gem.go         # GemAward struct
└── service.go     # Service: Award*, ResetSession, SnapshotData, persist

internal/screens/gemvault/
└── gemvault.go    # GemVaultScreen

internal/screens/history/
└── history.go     # HistoryScreen

ent/schema/
└── gem_event.go   # GemEvent ent schema
```

---

## 17. Example Flows

### Flow 1: Mastering a deep skill

1. Learner completes the Prove tier for "Long Division" (DAG depth 5, in top quartile)
2. `MasteryService.RecordAnswer` returns `StateTransition{From: Learning, To: Mastered, Trigger: "prove-complete"}`
3. `submitAnswer()` detects the mastery transition and calls `GemService.AwardMastery(ctx, "div-long", "Long Division", sessionID)`
4. `GemService` computes `depthMap.RarityForSkill("div-long")` → **Legendary** (top quartile)
5. Creates `GemAward{Type: GemMastery, Rarity: Legendary, SkillName: "Long Division", Reason: "Mastered Long Division"}`
6. Persists `GemEvent` to database
7. Sets `state.PendingGemAward` for inline notification
8. Feedback overlay shows: `💎 Legendary Mastery Gem — "Mastered Long Division"`
9. Summary screen includes the gem in the Gems section
10. Snapshot saves `Gems.TotalCount` incremented by 1

### Flow 2: Streak within a session

1. Learner answers 5 questions correctly in a row
2. `HandleAnswer` increments `state.ConsecutiveCorrect` to 5
3. `ConsecutiveCorrect >= NextStreakThreshold(5)` → true
4. `GemService.AwardStreak(ctx, 5, sessionID)` → `GemAward{Type: Streak, Rarity: Common}`
5. `NextStreakThreshold` advances to 10
6. Inline notification: `⚡ Common Streak Gem — "5 correct in a row!"`
7. Learner continues, gets answer #6 wrong
8. `ConsecutiveCorrect` resets to 0, `NextStreakThreshold` resets to 5
9. Later, learner builds another streak to 10 → awards **Rare** streak gem

### Flow 3: Session completion

1. Session timer reaches 5:00, `TimeExpired` = true
2. `handleSessionEnd()` fires
3. `state.TimeExpired` is true → awards session gem
4. Accuracy is 10/12 = 83.3% → `SessionRarity(0.833)` → **Epic**
5. `GemAward{Type: Session, Rarity: Epic, Reason: "Session complete (83% accuracy)"}`
6. Gem appears in summary (inline notification not shown since session is ending)

### Flow 4: Spaced-rep graduation

1. Learner reviews "Place Value Basics" (DAG depth 0, bottom quartile) for the 6th consecutive time
2. `RecordReview` sets `ConsecutiveHits` = 6, `Graduated` = true
3. Session screen detects `ConsecutiveHits == GraduationStage` → calls `AwardRetention`
4. `depthMap.RarityForSkill("npv-place-value")` → **Common** (shallow skill)
5. Inline notification: `🛡️ Common Retention Gem — "Retained Place Value Basics"`

### Flow 5: Recovery from rusty

1. Learner has a rusty "2-Digit Multiplication" (DAG depth 3, middle quartile)
2. Completes 4 recovery questions with 75%+ accuracy
3. `CheckReviewPerformance` returns `StateTransition{From: Rusty, To: Mastered, Trigger: "recovery-complete"}`
4. `AwardRecovery(ctx, "mul-2digit", "2-Digit Multiplication", sessionID)`
5. `depthMap.RarityForSkill("mul-2digit")` → **Rare**
6. Inline notification: `🔥 Rare Recovery Gem — "Recovered 2-Digit Multiplication"`

---

## 18. Testing Strategy

### Unit Tests — `internal/gems/`

| Test | Description |
|------|-------------|
| `TestComputeDepthMap` | Verify depth computation for known graph; root has depth 0, leaf has correct depth |
| `TestComputeDepthMap_Quartiles` | Verify quartile boundaries split skills into expected groups |
| `TestRarityForSkill_Quartiles` | Verify correct rarity assignment for skills at different depths |
| `TestStreakRarity` | Test all streak length thresholds: 5→Common, 10→Rare, 15→Epic, 20→Legendary |
| `TestSessionRarity` | Test all accuracy thresholds: <50%→Common, 50-74%→Rare, 75-89%→Epic, 90%+→Legendary |
| `TestAwardMastery` | Awards gem with correct type, rarity, skill info; persists to EventRepo |
| `TestAwardRecovery` | Same as mastery but with recovery type |
| `TestAwardRetention` | Same as mastery but with retention type |
| `TestAwardStreak` | Awards streak gem with correct rarity for streak length |
| `TestAwardSession` | Awards session gem with accuracy-based rarity |
| `TestSessionGems_Accumulation` | Verify `SessionGems` accumulates across multiple awards in same session |
| `TestResetSession` | Verify `ResetSession` clears `SessionGems` |
| `TestSnapshotData` | Verify `SnapshotData` aggregates counts correctly |

### Unit Tests — `internal/session/`

| Test | Description |
|------|-------------|
| `TestHandleAnswer_StreakTracking` | 5 correct → streak gem, wrong → reset, 5 more → another gem |
| `TestHandleAnswer_StreakThresholdAdvance` | 10 correct → 2 gems (at 5 and 10) |
| `TestHandleAnswer_MasteryGemOnTransition` | Mastery transition triggers mastery gem award |
| `TestHandleAnswer_RecoveryGemOnTransition` | Recovery transition triggers recovery gem award |
| `TestHandleAnswer_NoGemWithoutService` | No panic when GemService is nil |
| `TestBuildSummary_IncludesGems` | Summary includes all session gems |

### Integration Tests — `internal/screens/session/`

| Test | Description |
|------|-------------|
| `TestSessionEnd_FullTimer_AwardsSessionGem` | Timer expires → session gem awarded |
| `TestSessionEnd_EarlyQuit_NoSessionGem` | Quit via Esc+Y → no session gem |
| `TestFeedback_ShowsGemNotification` | PendingGemAward renders in feedback overlay |
| `TestFeedback_ClearsGemAfterDismiss` | PendingGemAward nil after feedback dismissed |
| `TestRetentionGem_OnGraduation` | Spaced-rep graduation during review → retention gem |

### Screen Tests — `internal/screens/gemvault/`

| Test | Description |
|------|-------------|
| `TestGemVault_LoadsGems` | Init loads gem events from EventRepo |
| `TestGemVault_TabSwitchesType` | Tab key cycles through gem types |
| `TestGemVault_ScrollsGemList` | Up/Down navigates the gem list |
| `TestGemVault_EscReturnsHome` | Esc sends PopScreenMsg |
| `TestGemVault_EmptyState` | Shows appropriate message when no gems |

### Screen Tests — `internal/screens/history/`

| Test | Description |
|------|-------------|
| `TestHistory_LoadsSessions` | Init loads session summaries from EventRepo |
| `TestHistory_ExpandShowsGems` | Enter toggles gem detail expansion |
| `TestHistory_NavigateSessions` | Up/Down navigates session list |
| `TestHistory_EscReturnsHome` | Esc sends PopScreenMsg |
| `TestHistory_EmptyState` | Shows appropriate message when no sessions |

### Store Tests

| Test | Description |
|------|-------------|
| `TestAppendGemEvent` | Persists gem event with correct fields |
| `TestQueryGemEvents` | Returns gem events filtered by QueryOpts |
| `TestGemCounts` | Returns correct counts by type and total |
| `TestQuerySessionSummaries` | Returns session summaries with gem counts |
| `TestSnapshot_GemsField` | Snapshot round-trips GemsSnapshotData |

---

## 19. Verification Checklist

- [ ] `GemType` and `Rarity` types defined with `DisplayName()`, `Icon()` methods
- [ ] `DepthMap` computed from skill graph with correct quartile boundaries
- [ ] `RarityForSkill` returns correct rarity based on depth quartiles
- [ ] `StreakRarity` and `SessionRarity` return correct rarity for thresholds
- [ ] `GemService` awards mastery gems on `prove-complete` transition
- [ ] `GemService` awards recovery gems on `recovery-complete` transition
- [ ] `GemService` awards retention gems on spaced-rep graduation
- [ ] Streak counter increments on correct, resets on wrong
- [ ] Streak gems awarded at 5, 10, 15, 20 thresholds
- [ ] Session gems awarded only when timer expires (not early quit)
- [ ] Session gem rarity based on session accuracy
- [ ] `GemEvent` ent schema created with EventMixin
- [ ] `EventRepo.AppendGemEvent` persists gem events
- [ ] `EventRepo.QueryGemEvents` retrieves gem events with filtering
- [ ] `EventRepo.GemCounts` returns aggregate counts
- [ ] `SnapshotData.Gems` field added, version bumped to 4
- [ ] Inline gem notification shown in session feedback overlay
- [ ] `PendingGemAward` cleared after feedback dismissed
- [ ] Summary screen shows gems earned in session
- [ ] Gem Vault screen shows rich collection grouped by type
- [ ] History screen shows sessions with expandable gem details
- [ ] Home screen shows total gem count from snapshot
- [ ] Home screen Gem Vault menu pushes real GemVaultScreen
- [ ] Home screen History menu pushes real HistoryScreen
- [ ] `app.Options` includes `GemService`
- [ ] `SessionScreen.New` accepts `GemService` parameter
- [ ] `HomeScreen.New` accepts `GemService` parameter
- [ ] `cmd/play.go` creates and wires `GemService`
- [ ] Backward compatibility: nil `Gems` in snapshot → 0 count, no crash
- [ ] All tests pass: `CGO_ENABLED=0 go test ./internal/gems/... ./internal/screens/gemvault/... ./internal/screens/history/...`
- [ ] Build passes: `CGO_ENABLED=0 go build ./...`

---

## 20. Dependencies

| Module | Direction | What's used |
|--------|-----------|-------------|
| `internal/skillgraph` | → imports | `TopologicalOrder`, `GetSkill`, skill graph structure for depth computation |
| `internal/mastery` | → imports | `StateTransition`, `StateLearning`, `StateMastered`, `StateRusty` for gem triggers |
| `internal/spacedrep` | → imports | `ReviewState`, `GraduationStage` for retention gem detection |
| `internal/session` | → imports | `SessionState`, `HandleAnswer` for streak tracking and gem wiring |
| `internal/store` | → imports | `EventRepo`, `SnapshotData`, `GemEventData`, `GemsSnapshotData` |
| `internal/screen` | → imports | `Screen`, `KeyHintProvider` interfaces for Gem Vault and History screens |
| `internal/ui/theme` | → imports | Rarity colors |
| `internal/ui/layout` | → imports | `KeyHint` for footer hints |
| `internal/ui/components` | → imports | Reusable components for screen rendering |
| Session screen (06) | ← consumed by | Awards gems on mastery/recovery/streak/session events |
| Summary screen (06) | ← consumed by | Displays `GemsEarned` from summary |
| Home screen (01) | ← consumed by | Shows total gem count, pushes Gem Vault and History screens |
