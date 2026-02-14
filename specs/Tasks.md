# Mathiz — Implementation Tasks

Components are listed in dependency order. Check off tasks as they are completed.

---

## 1. Project Skeleton & TUI Framework (`01-skeleton.md`)

### Go Module & Cobra CLI
- [ ] Initialize Go module (`github.com/mathiz-ai/mathiz`)
- [ ] Add dependencies: Cobra, Bubble Tea v2, Lip Gloss v2, Bubbles v2, Huh
- [ ] Create `main.go` with Cobra root command (launches TUI when no subcommand)
- [ ] Create `cmd/play.go` — launches TUI (same as bare `mathiz`)
- [ ] Create `cmd/reset.go` — stub that prints "Not yet implemented"
- [ ] Create `cmd/stats.go` — stub that prints "Not yet implemented"

### Theme & Styling
- [ ] Define color palette constants (Primary, Secondary, Accent, Success, Error, etc.)
- [ ] Define Lip Gloss style variables (Title, Subtitle, Body, Hint, Header, Footer, Card, etc.)
- [ ] Define component styles (Selected, Unselected, Correct, Incorrect, ProgressFilled, etc.)

### Screen Interface & Router
- [ ] Define `Screen` interface (`Init`, `Update`, `View`, `Title`)
- [ ] Implement stack-based `Router` (`Push`, `Pop`, `Active`, `Depth`, `Update`, `View`)
- [ ] Define navigation message types (`PushScreenMsg`, `PopScreenMsg`)

### Layout System
- [ ] Implement `RenderFrame` (header + content + footer composition)
- [ ] Implement `RenderHeader` (logo, screen title, gem count, streak)
- [ ] Implement `RenderFooter` (contextual key hints)
- [ ] Implement responsive breakpoints (compact vs full layout)
- [ ] Implement "Terminal too small" screen (< 80x24)

### Shared UI Components
- [ ] Implement `TextInput` component (styled, numeric-only mode, validation indicator)
- [ ] Implement `Menu` component (vertical nav, arrow keys, enter to select)
- [ ] Implement `ProgressBar` component (label, percentage, responsive width)
- [ ] Implement `Button` component (active/inactive states)
- [ ] Implement `MultiChoice` component (A/B/C/D selection, post-answer feedback)

### Screens
- [ ] Implement `HomeScreen` (mascot, greeting, menu with 5 items)
- [ ] Implement robot mascot ASCII art (`internal/screens/home/mascot.go`)
- [ ] Implement compact Home layout (mascot hidden when < 30 rows)
- [ ] Implement `PlaceholderScreen` ("Coming Soon" for unbuilt features)

### AppModel & Wiring
- [ ] Implement `AppModel` (root Bubble Tea model, owns router)
- [ ] Handle global key bindings (Ctrl+C quit, Esc pop, window resize)
- [ ] Wire Home screen as initial screen on stack
- [ ] Wire menu items to push placeholder screens

### Verification
- [ ] `go build ./...` compiles cleanly
- [ ] `go vet ./...` passes
- [ ] TUI launches, shows Home screen with mascot and menu
- [ ] Arrow keys navigate menu, Enter pushes placeholder, Esc pops back
- [ ] Resize below 80x24 shows min-size message

---

## 2. Persistence Layer (`02-persistence.md`)

### Database Setup
- [ ] Add `modernc.org/sqlite` and `entgo.io/ent` dependencies
- [ ] Implement `Store` struct with `Open`/`Close` (holds `*ent.Client`)
- [ ] Configure SQLite pragmas on connection (WAL, busy_timeout, foreign_keys, synchronous)
- [ ] Implement DB file location resolution (CLI flag > env var > XDG default)
- [ ] Auto-create parent directory if it doesn't exist

### ent Schemas
- [ ] Create `EventMixin` with base fields (`id`, `sequence`, `timestamp`)
- [ ] Create `Snapshot` ent schema (`sequence`, `timestamp`, `data` JSON field)
- [ ] Run `go generate ./ent` and verify generated code

### Repository Interfaces
- [ ] Define `EventRepo` interface (`Append`, `QueryByType`, `QueryByTimeRange`, `QueryAfterSequence`, `NextSequence`)
- [ ] Define `SnapshotRepo` interface (`Save`, `Latest`, `Prune`)
- [ ] Define `QueryOpts` struct (Limit, After, Before, From, To)

### Repository Implementations
- [ ] Implement `EventRepo` with append (assigns global sequence) and query methods
- [ ] Implement `SnapshotRepo` with save, latest, and prune (keep N most recent)
- [ ] Define `SnapshotData` struct (Skills, Metrics, Schedules, Gems, Version)

### Snapshot Lifecycle
- [ ] Implement periodic snapshot creation (every N events, configurable)
- [ ] Implement snapshot on graceful shutdown
- [ ] Implement restore from snapshot + event replay on startup

### Auto-Migration
- [ ] Call `client.Schema.Create(ctx)` at startup to auto-migrate

### Testing
- [ ] Write tests using in-memory SQLite (`file::memory:?cache=shared`)
- [ ] Test event append assigns sequential sequence numbers
- [ ] Test event queries filter by type, time range, and sequence
- [ ] Test snapshot save/load round-trips correctly
- [ ] Test snapshot prune keeps only N most recent
- [ ] Test restore from snapshot + replay produces correct state
- [ ] Test pragmas are applied on connection open

---

## 3. Skill Graph (`03-skill-graph.md`)

### Data Model
- [ ] Define `Skill` struct (ID, Name, Description, Strand, GradeLevel, etc.)
- [ ] Define `Strand` type and constants (NumberPlace, AddSub, MultDiv, Fractions, Measurement)
- [ ] Define `Tier` type, constants (`TierLearn`, `TierProve`), and `TierConfig` struct
- [ ] Set default tier configurations (Learn: 8 problems/0.75 acc; Prove: 6 problems/0.85 acc/30s)

### Seed Graph
- [ ] Define all 52 skill nodes as Go literals in `internal/skillgraph/seed.go`
  - [ ] Number & Place Value (8 nodes)
  - [ ] Addition & Subtraction (10 nodes)
  - [ ] Multiplication & Division (14 nodes)
  - [ ] Fractions (12 nodes)
  - [ ] Measurement (8 nodes)

### Graph Traversal API
- [ ] Implement `GetSkill(id)` — lookup by ID
- [ ] Implement `AllSkills()` — return all skills
- [ ] Implement `ByStrand(strand)` — filter by strand, ordered by grade + topo
- [ ] Implement `ByGrade(grade)` — filter by grade
- [ ] Implement `RootSkills()` — skills with no prerequisites
- [ ] Implement `Prerequisites(id)` — direct prerequisites
- [ ] Implement `Dependents(id)` — skills depending on given skill
- [ ] Implement `IsUnlocked(id, mastered)` — check all prereqs mastered
- [ ] Implement `AvailableSkills(mastered)` — unlocked but not mastered
- [ ] Implement `FrontierSkills(mastered)` — next-up skills for learner
- [ ] Implement `BlockedSkills(mastered)` — skills with unmet prereqs
- [ ] Implement `TopologicalOrder()` — valid topological sort
- [ ] Implement `Validate()` — cycle detection, dangling refs, duplicate IDs, etc.

### Diagnostic Placement
- [ ] Implement top-down probing algorithm (2-3 questions per skill, highest grade first)
- [ ] Implement transitive prerequisite marking on correct probe
- [ ] Implement skip option (start from root skills)
- [ ] Target 10-15 total diagnostic questions

### Skill Map Screen
- [ ] Implement Skill Map screen (grouped list by strand)
- [ ] Render state icons per skill (locked, available, learning, proving, mastered, rusty)
- [ ] Implement Enter on available skill → start practice session
- [ ] Implement Enter on locked skill → show needed prerequisites
- [ ] Implement Tab to cycle between strand headers
- [ ] Implement q/Esc to return to Home

### Validation
- [ ] Call `Validate()` at init, panic on failure
- [ ] Test: seed graph passes validation
- [ ] Test: cycle detection catches injected cycle
- [ ] Test: dangling prerequisite reference caught
- [ ] Test: `RootSkills` returns correct set
- [ ] Test: `IsUnlocked`, `AvailableSkills`, `FrontierSkills`, `BlockedSkills` correct
- [ ] Test: `TopologicalOrder` valid ordering
- [ ] Test: `ByStrand` / `ByGrade` correct grouping

---

## 4. LLM Integration (`04-llm.md`)

- [ ] Define provider abstraction interface (multi-provider support)
- [ ] Implement prompt template system
- [ ] Implement strict JSON schema enforcement on LLM outputs
- [ ] Implement JSON validation pipeline
- [ ] Implement token limit management
- [ ] Implement error handling and retries
- [ ] Wire up at least one LLM provider

---

## 5. Problem Generation (`05-problem-gen.md`)

- [ ] Implement LLM-powered question generation from skill + learner context
- [ ] Implement programmatic answer validation
- [ ] Implement difficulty tier integration (Learn vs Prove)
- [ ] Implement within-session deduplication
- [ ] Generate questions using skill keywords, description, and strand for context

---

## 6. Session Engine (`06-session.md`)

### Session Planner
- [ ] Implement session planning: select target skill
- [ ] Implement session mix: 60% frontier, 30% review, 10% boosters
- [ ] Integrate with skill graph (frontier, available, mastered skills)

### Session Lifecycle
- [ ] Implement session start (create session, plan questions)
- [ ] Implement question serving (serve next question based on plan)
- [ ] Implement answer recording (capture response, latency, correctness)
- [ ] Implement session completion (compute summary stats)

### Session Screen (TUI)
- [ ] Implement Session screen (question display, answer input, timer)
- [ ] Implement progress indicator (question N of M)
- [ ] Implement correct/incorrect feedback after each answer
- [ ] Implement timer display for Prove-tier questions

### Session Summary Screen
- [ ] Implement Session Summary screen (accuracy, speed, skills practiced)
- [ ] Show per-skill breakdown
- [ ] Show gems earned (wired in component 11)
- [ ] Implement "Practice Again" / "Back to Home" options

---

## 7. Mastery & Scoring (`07-mastery.md`)

- [ ] Implement per-skill metrics tracking (accuracy, speed, consistency, assist rate)
- [ ] Implement fluency score computation (0-1 composite score)
- [ ] Implement mastery state machine (new → learning → mastered → rusty)
- [ ] Define configurable mastery criteria (per-tier thresholds)
- [ ] Implement state transitions triggered by session results
- [ ] Persist mastery state changes as events

---

## 8. Spaced Repetition (`08-spaced-rep.md`)

- [ ] Implement per-skill review scheduling
- [ ] Implement next-review-date computation (based on mastery strength + recency)
- [ ] Implement decay detection (identify skills becoming rusty)
- [ ] Implement rusty labeling (transition mastered → rusty)
- [ ] Integrate with session planner (feed review skills into 30% review slot)

---

## 9. Error Diagnosis (`09-diagnosis.md`)

- [ ] Implement rule-based error classifier: careless errors
- [ ] Implement rule-based error classifier: speed-rush mistakes
- [ ] Implement rule-based error classifier: misconceptions
- [ ] Implement misconception tagging (link errors to specific misconceptions)
- [ ] Implement AI-assisted diagnosis fallback (when rules are insufficient)
- [ ] Implement intervention recommendations based on diagnosis

---

## 10. AI Lessons, Hints & Compression (`10-ai-lessons.md`)

- [ ] Implement hint generation (progressive hints per question)
- [ ] Implement micro-lesson generation (explanation + worked example + mini practice)
- [ ] Implement context compression snapshots (summarize learner history for LLM context)
- [ ] Enforce strict JSON output + programmatic validation on all AI outputs
- [ ] Integrate hints into Session screen (available in Learn tier)

---

## 11. Rewards — Gems (`11-rewards.md`)

### Gem System
- [ ] Define gem types: mastery gems, retention gems, recovery gems
- [ ] Define rarity levels for gems
- [ ] Implement award triggers (tied to real learning milestones)
- [ ] Persist gem awards as events

### Gem Vault Screen
- [ ] Implement Gem Vault screen (display collected gems)
- [ ] Show gem counts by type and rarity
- [ ] Show recent gem awards

### History Screen
- [ ] Implement History screen (past sessions, gems, milestones)
- [ ] Show session history with date, skill, accuracy
- [ ] Show gem award history

### Integration
- [ ] Wire gem count into header display
- [ ] Wire gem awards into Session Summary screen
