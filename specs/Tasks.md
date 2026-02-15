# Mathiz — Implementation Tasks

Components are listed in dependency order. Check off tasks as they are completed.

---

## 1. Project Skeleton & TUI Framework (`01-skeleton.md`)

### Go Module & Cobra CLI
- [x] Initialize Go module (`github.com/mathiz-ai/mathiz`)
- [x] Add dependencies: Cobra, Bubble Tea v2, Lip Gloss v2, Bubbles v2, Huh
- [x] Create `main.go` with Cobra root command (launches TUI when no subcommand)
- [x] Create `cmd/play.go` — launches TUI (same as bare `mathiz`)
- [x] Create `cmd/reset.go` — stub that prints "Not yet implemented"
- [x] Create `cmd/stats.go` — stub that prints "Not yet implemented"

### Theme & Styling
- [x] Define color palette constants (Primary, Secondary, Accent, Success, Error, etc.)
- [x] Define Lip Gloss style variables (Title, Subtitle, Body, Hint, Header, Footer, Card, etc.)
- [x] Define component styles (Selected, Unselected, Correct, Incorrect, ProgressFilled, etc.)

### Screen Interface & Router
- [x] Define `Screen` interface (`Init`, `Update`, `View`, `Title`)
- [x] Implement stack-based `Router` (`Push`, `Pop`, `Active`, `Depth`, `Update`, `View`)
- [x] Define navigation message types (`PushScreenMsg`, `PopScreenMsg`)

### Layout System
- [x] Implement `RenderFrame` (header + content + footer composition)
- [x] Implement `RenderHeader` (logo, screen title, gem count, streak)
- [x] Implement `RenderFooter` (contextual key hints)
- [x] Implement responsive breakpoints (compact vs full layout)
- [x] Implement "Terminal too small" screen (< 80x24)

### Shared UI Components
- [x] Implement `TextInput` component (styled, numeric-only mode, validation indicator)
- [x] Implement `Menu` component (vertical nav, arrow keys, enter to select)
- [x] Implement `ProgressBar` component (label, percentage, responsive width)
- [x] Implement `Button` component (active/inactive states)
- [x] Implement `MultiChoice` component (A/B/C/D selection, post-answer feedback)

### Screens
- [x] Implement `HomeScreen` (mascot, greeting, menu with 5 items)
- [x] Implement robot mascot ASCII art (`internal/screens/home/mascot.go`)
- [x] Implement compact Home layout (mascot hidden when < 30 rows)
- [x] Implement `PlaceholderScreen` ("Coming Soon" for unbuilt features)

### AppModel & Wiring
- [x] Implement `AppModel` (root Bubble Tea model, owns router)
- [x] Handle global key bindings (Ctrl+C quit, Esc pop, window resize)
- [x] Wire Home screen as initial screen on stack
- [x] Wire menu items to push placeholder screens

### Verification
- [x] `go build ./...` compiles cleanly
- [ ] `go vet ./...` passes
- [x] TUI launches, shows Home screen with mascot and menu
- [x] Arrow keys navigate menu, Enter pushes placeholder, Esc pops back
- [x] Resize below 80x24 shows min-size message

### Known Issues
- Header missing robot emoji (shows "Mathiz" instead of "🤖 Mathiz")
- Gem/streak symbols use `◆`/`★` instead of spec's `💎`/`🔥`
- Menu has "Exit Game" instead of spec's "Settings" as last item
- Placeholder screen uses `╌╌ Coming Soon ╌╌` instead of spec's `🚧 Coming Soon!`
- Module name is `github.com/abhisek/mathiz` instead of `github.com/mathiz-ai/mathiz`
- `stats` command is fully implemented (not a stub) — beyond spec scope

---

## 2. Persistence Layer (`02-persistence.md`)

### Database Setup
- [x] Add `modernc.org/sqlite` and `entgo.io/ent` dependencies
- [x] Implement `Store` struct with `Open`/`Close` (holds `*ent.Client`)
- [x] Configure SQLite pragmas on connection (WAL, busy_timeout, foreign_keys, synchronous)
- [ ] Implement DB file location resolution (CLI flag > env var > XDG default) — **`--db` CLI flag not implemented; only env var + XDG default**
- [x] Auto-create parent directory if it doesn't exist

### ent Schemas
- [x] Create `EventMixin` with base fields (`id`, `sequence`, `timestamp`)
- [x] Create `Snapshot` ent schema (`sequence`, `timestamp`, `data` JSON field)
- [x] Run `go generate ./ent` and verify generated code

### Repository Interfaces
- [ ] Define `EventRepo` interface (`Append`, `QueryByType`, `QueryByTimeRange`, `QueryAfterSequence`, `NextSequence`) — **Only domain-specific append/query methods implemented; generic query methods missing**
- [x] Define `SnapshotRepo` interface (`Save`, `Latest`, `Prune`)
- [x] Define `QueryOpts` struct (Limit, After, Before, From, To)

### Repository Implementations
- [x] Implement `EventRepo` with append (assigns global sequence) and query methods — **8 domain-specific append methods + 6 query methods; generic queries missing**
- [x] Implement `SnapshotRepo` with save, latest, and prune (keep N most recent)
- [x] Define `SnapshotData` struct (Skills, Metrics, Schedules, Gems, Version)

### Snapshot Lifecycle
- [ ] Implement periodic snapshot creation (every N events, configurable)
- [ ] Implement snapshot on graceful shutdown — **Snapshots saved at session end only, not on graceful shutdown**
- [ ] Implement restore from snapshot + event replay on startup

### Auto-Migration
- [x] Call `client.Schema.Create(ctx)` at startup to auto-migrate

### Testing
- [x] Write tests using in-memory SQLite (`file::memory:?cache=shared`)
- [x] Test event append assigns sequential sequence numbers
- [ ] Test event queries filter by type, time range, and sequence — **No generic event query tests**
- [x] Test snapshot save/load round-trips correctly
- [x] Test snapshot prune keeps only N most recent
- [ ] Test restore from snapshot + replay produces correct state
- [x] Test pragmas are applied on connection open

### Known Issues
- `--db` CLI flag not implemented (spec requires CLI flag > env var > default priority)
- Generic EventRepo query methods missing (QueryByType, QueryByTimeRange, QueryAfterSequence, NextSequence)
- No snapshot creation triggers or restore logic
- Sequence counter uses raw SQL outside ent framework (dual schema management)

---

## 3. Skill Graph (`03-skill-graph.md`)

### Data Model
- [x] Define `Skill` struct (ID, Name, Description, Strand, GradeLevel, etc.)
- [x] Define `Strand` type and constants (NumberPlace, AddSub, MultDiv, Fractions, Measurement)
- [x] Define `Tier` type, constants (`TierLearn`, `TierProve`), and `TierConfig` struct
- [x] Set default tier configurations (Learn: 8 problems/0.75 acc; Prove: 6 problems/0.85 acc/30s)

### Seed Graph
- [x] Define all 52 skill nodes as Go literals in `internal/skillgraph/seed.go`
  - [x] Number & Place Value (8 nodes)
  - [x] Addition & Subtraction (10 nodes)
  - [x] Multiplication & Division (14 nodes)
  - [x] Fractions (12 nodes)
  - [x] Measurement (8 nodes)

### Graph Traversal API
- [x] Implement `GetSkill(id)` — lookup by ID
- [x] Implement `AllSkills()` — return all skills
- [x] Implement `ByStrand(strand)` — filter by strand, ordered by grade + topo
- [x] Implement `ByGrade(grade)` — filter by grade
- [x] Implement `RootSkills()` — skills with no prerequisites
- [x] Implement `Prerequisites(id)` — direct prerequisites
- [x] Implement `Dependents(id)` — skills depending on given skill
- [x] Implement `IsUnlocked(id, mastered)` — check all prereqs mastered
- [x] Implement `AvailableSkills(mastered)` — unlocked but not mastered
- [x] Implement `FrontierSkills(mastered)` — next-up skills for learner
- [x] Implement `BlockedSkills(mastered)` — skills with unmet prereqs
- [x] Implement `TopologicalOrder()` — valid topological sort
- [x] Implement `Validate()` — cycle detection, dangling refs, duplicate IDs, etc.

### Diagnostic Placement
- [ ] Implement top-down probing algorithm (2-3 questions per skill, highest grade first) — **Skeleton only (DiagnosticResult type defined, no algorithm)**
- [ ] Implement transitive prerequisite marking on correct probe
- [ ] Implement skip option (start from root skills)
- [ ] Target 10-15 total diagnostic questions

### Skill Map Screen
- [x] Implement Skill Map screen (grouped list by strand)
- [x] Render state icons per skill (locked, available, learning, proving, mastered, rusty) — **Icons rendered but mastered data always empty (not wired to persistence)**
- [x] Implement Enter on available skill → start practice session — **Uses placeholder screen currently**
- [x] Implement Enter on locked skill → show needed prerequisites
- [x] Implement Tab to cycle between strand headers
- [x] Implement q/Esc to return to Home

### Validation
- [x] Call `Validate()` at init, panic on failure
- [x] Test: seed graph passes validation
- [x] Test: cycle detection catches injected cycle
- [x] Test: dangling prerequisite reference caught
- [x] Test: `RootSkills` returns correct set
- [x] Test: `IsUnlocked`, `AvailableSkills`, `FrontierSkills`, `BlockedSkills` correct
- [x] Test: `TopologicalOrder` valid ordering
- [x] Test: `ByStrand` / `ByGrade` correct grouping

### Known Issues
- SkillMapScreen `mastered` map always empty — not populated from persistence/mastery layer
- Diagnostic placement algorithm not implemented (intentionally deferred)

---

## 4. LLM Integration (`04-llm.md`)

- [x] Define provider abstraction interface (multi-provider support)
- [x] Implement prompt template system
- [x] Implement strict JSON schema enforcement on LLM outputs
- [x] Implement JSON validation pipeline
- [x] Implement token limit management
- [x] Implement error handling and retries
- [x] Wire up at least one LLM provider — **All three: Anthropic, OpenAI, Gemini**

### Known Issues
- **BUG**: Logging decorator records ModelID as Provider name (`logging.go:32-33`) — both Provider and Model fields set to `l.inner.ModelID()`
- RetryAfter header not extracted from rate limit (429) responses in any adapter

---

## 5. Problem Generation (`05-problem-gen.md`)

- [x] Implement LLM-powered question generation from skill + learner context
- [x] Implement programmatic answer validation
- [x] Implement difficulty tier integration (Learn vs Prove)
- [x] Implement within-session deduplication
- [x] Generate questions using skill keywords, description, and strand for context

### Known Issues
- LearnerProfile field included in prompts but not tested in `prompt_test.go`

---

## 6. Session Engine (`06-session.md`)

### Session Planner
- [x] Implement session planning: select target skill
- [x] Implement session mix: 60% frontier, 30% review, 10% boosters
- [x] Integrate with skill graph (frontier, available, mastered skills)

### Session Lifecycle
- [x] Implement session start (create session, plan questions)
- [x] Implement question serving (serve next question based on plan)
- [x] Implement answer recording (capture response, latency, correctness)
- [x] Implement session completion (compute summary stats)

### Session Screen (TUI)
- [x] Implement Session screen (question display, answer input, timer)
- [x] Implement progress indicator (question N of M)
- [x] Implement correct/incorrect feedback after each answer
- [x] Implement timer display for Prove-tier questions

### Session Summary Screen
- [x] Implement Session Summary screen (accuracy, speed, skills practiced)
- [x] Show per-skill breakdown
- [x] Show gems earned (wired in component 11)
- [x] Implement "Practice Again" / "Back to Home" options

---

## 7. Mastery & Scoring (`07-mastery.md`)

- [x] Implement per-skill metrics tracking (accuracy, speed, consistency, assist rate)
- [x] Implement fluency score computation (0-1 composite score)
- [x] Implement mastery state machine (new → learning → mastered → rusty)
- [x] Define configurable mastery criteria (per-tier thresholds)
- [x] Implement state transitions triggered by session results
- [x] Persist mastery state changes as events

### Known Issues
- **BUG**: `CheckReviewPerformance()` never called in HandleAnswer — poor review performance does not transition skills to rusty
- Planner does not prioritize rusty skills over regular frontier skills (spec requirement)

---

## 8. Spaced Repetition (`08-spaced-rep.md`)

- [x] Implement per-skill review scheduling
- [x] Implement next-review-date computation (based on mastery strength + recency)
- [x] Implement decay detection (identify skills becoming rusty)
- [x] Implement rusty labeling (transition mastered → rusty)
- [x] Integrate with session planner (feed review skills into 30% review slot)

### Known Issues
- Skill Map screen does not display review schedule badges (Review in N days, Due, Overdue, Graduated)
- Stats command does not show graduated/upcoming/overdue skill sections
- No visual notification when RunDecayCheck marks skills as rusty at session start
- Scheduler not added to `app.Options` (passed via local variable)

---

## 9. Error Diagnosis (`09-diagnosis.md`)

- [x] Implement rule-based error classifier: careless errors
- [x] Implement rule-based error classifier: speed-rush mistakes
- [x] Implement rule-based error classifier: misconceptions — **LLM-based async classification**
- [x] Implement misconception tagging (link errors to specific misconceptions)
- [x] Implement AI-assisted diagnosis fallback (when rules are insufficient)
- [x] Implement intervention recommendations based on diagnosis — **Misconception penalty adjusts tier completion requirements**

### Known Issues
- **BUG**: `AppendDiagnosisEvent` never called from production code — diagnosis events are never persisted to the event log

---

## 10. AI Lessons, Hints & Compression (`10-ai-lessons.md`)

- [x] Implement hint generation (progressive hints per question)
- [x] Implement micro-lesson generation (explanation + worked example + mini practice)
- [x] Implement context compression snapshots (summarize learner history for LLM context)
- [x] Enforce strict JSON output + programmatic validation on all AI outputs
- [x] Integrate hints into Session screen (available in Learn tier)

### Known Issues
- ProfileInput.SessionCount defined but not included in profile generation prompt
- Profile/compression goroutines spawned without context cancellation (potential leak on shutdown)

---

## 11. Rewards — Gems (`11-rewards.md`)

### Gem System
- [x] Define gem types: mastery gems, retention gems, recovery gems
- [x] Define rarity levels for gems
- [x] Implement award triggers (tied to real learning milestones)
- [x] Persist gem awards as events

### Gem Vault Screen
- [x] Implement Gem Vault screen (display collected gems)
- [x] Show gem counts by type and rarity
- [x] Show recent gem awards

### History Screen
- [x] Implement History screen (past sessions, gems, milestones)
- [x] Show session history with date, skill, accuracy
- [x] Show gem award history

### Integration
- [x] Wire gem count into header display
- [x] Wire gem awards into Session Summary screen

### Known Issues
- Rarity colors use `theme.Secondary`/`theme.Accent` instead of spec's `theme.Info`/`theme.Warning` (those constants don't exist in theme)
