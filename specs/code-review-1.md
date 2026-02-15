# Mathiz Code Review

Comprehensive analysis of all implemented components against their specifications. Each component was reviewed for spec compliance, wiring correctness, bugs, gaps, and areas of improvement.

---

## Executive Summary

| Component | Spec | Compliance | Critical Issues |
|-----------|------|------------|-----------------|
| Skeleton & TUI | 01 | 85-90% | Missing header emoji, wrong gem/streak symbols |
| Persistence | 02 | ~70% | No `--db` flag, missing generic EventRepo queries, no snapshot triggers |
| Skill Graph | 03 | 90% | SkillMapScreen mastered data always empty |
| LLM Integration | 04 | 92% | **BUG**: Logging records model ID as provider name |
| Problem Generation | 05 | ~99% | LearnerProfile not tested in prompts |
| Session Engine | 06 | 99% | No breaking deviations |
| Mastery & Scoring | 07 | ~90% | **BUG**: `CheckReviewPerformance()` never called |
| Spaced Repetition | 08 | ~85% | Missing UI integration (Skill Map badges, stats, notifications) |
| Error Diagnosis | 09 | 89% | **BUG**: `AppendDiagnosisEvent` never called from production code |
| AI Lessons | 10 | 95% | Minor: goroutines without context cancellation |
| Rewards (Gems) | 11 | 99% | Minor color naming deviation |
| Cross-cutting Wiring | -- | Excellent | No circular deps, all nil checks in place |

**3 Critical Bugs**, **5 Major Gaps**, **~10 Minor Issues** identified.

---

## Critical Bugs

### 1. `CheckReviewPerformance()` never called in HandleAnswer

**Location**: `internal/session/session.go` lines 177-197

**Spec 07 (Section 5.3)** states: "After recording an answer on a review question, the session engine calls `CheckReviewPerformance`."

The code detects review answers and calls `SpacedRepSched.RecordReview()`, but never calls `MasteryService.CheckReviewPerformance()`. This means poor review performance (< 50% accuracy on last 4 review answers) does **not** automatically transition skills to rusty.

**Impact**: Mastery decay via poor review performance is completely non-functional. Only time-decay (from spaced rep) works.

**Fix**: Add after the spaced rep handling block:
```go
if transition := state.MasteryService.CheckReviewPerformance(ctx, q.SkillID); transition != nil {
    state.MasteryTransition = transition
}
```

### 2. `AppendDiagnosisEvent` never called from production code

**Location**: `internal/session/session.go` (async callback ~line 71 and sync result ~line 87)

The DiagnosisEvent ent schema is defined, the `AppendDiagnosisEvent` interface method is defined and implemented, but **no production code ever calls it**. Diagnosis events are never persisted to the event log.

**Impact**: Diagnosis data is not auditable, cannot be analyzed across sessions, and breaks the spec requirement that "diagnosis results feed into question generation."

**Fix**: Add `AppendDiagnosisEvent` call in the async callback and after synchronous diagnosis results in `HandleAnswer`.

### 3. LLM Logging Provider field records ModelID instead of provider name

**Location**: `internal/llm/logging.go` lines 32-33

```go
Provider:  l.inner.ModelID(),  // BUG: should be "anthropic"/"openai"/"gemini"
Model:     l.inner.ModelID(),  // correct
```

Both `Provider` and `Model` are set to the same value from `ModelID()`. The `Provider` field should contain the provider name (e.g., "anthropic"), not the model ID.

**Impact**: Event logs cannot distinguish which provider was used. Cost tracking and analytics by provider are broken.

**Fix**: Add a `ProviderName()` method to the Provider interface, or store the provider name in the logging decorator when created by the factory.

---

## Major Gaps

### 4. Generic EventRepo query methods missing (Spec 02)

**Location**: `internal/store/repo.go`

Spec requires: `QueryByType`, `QueryByTimeRange`, `QueryAfterSequence`, `NextSequence`. None are implemented. The EventRepo only has domain-specific append and query methods.

**Impact**: Sync-readiness (spec section 7) impossible; generic event export not available; snapshot restore via event replay not supported.

### 5. No snapshot creation triggers or restore logic (Spec 02)

Spec requires periodic snapshot creation (every 100 events), on-exit snapshots, and restore logic (load latest snapshot + replay events). None of this orchestration exists.

**Impact**: Snapshots are only saved at session end. No fast startup via snapshot restore. Full event replay required every startup (but not even implemented).

### 6. SkillMapScreen mastered data always empty (Spec 03)

**Location**: `internal/screens/skillmap/skillmap.go` line 43

The `mastered` map is created empty in `New()` and never populated from the persistence/mastery layer. All skills display as locked or available regardless of actual progress.

**Impact**: Skill Map screen doesn't reflect learner progress. Learning/Proving/Rusty states cannot be differentiated.

**Fix**: Pass mastery data from snapshot to `SkillMapScreen.New()`.

### 7. Spaced repetition UI integration missing (Spec 08)

Core engine is fully implemented (53 tests passing), but three UI features are not:
- **Skill Map review badges**: No review schedule info (Review in N days, Due, Overdue, Graduated) shown
- **Stats command sections**: No graduated/upcoming/overdue skill listings
- **Session start notification**: `RunDecayCheck` is called but no visual notification when skills decay to rusty

### 8. CLI `--db` flag not implemented (Spec 02)

**Location**: `cmd/root.go`, `cmd/run.go`

Spec requires database path override via `--db` CLI flag (highest priority), then env var, then default. Only env var and default path are implemented.

---

## Minor Issues

### 9. Missing header emoji and wrong gem/streak symbols (Spec 01)

**Location**: `internal/ui/layout/layout.go`

- Line 72: Header shows `"  Mathiz"` instead of `"  Mathiz"` (missing robot emoji)
- Line 80: Uses `◆` instead of diamond emoji for gems
- Line 86: Uses `★` instead of fire emoji for streak

### 10. RetryAfter header not extracted from rate limit responses (Spec 04)

**Location**: `internal/llm/anthropic.go`, `openai.go`, `gemini.go`

All three adapters create `ErrRateLimit{Err: err}` without extracting the `Retry-After` header from 429 responses. The retry decorator's RetryAfter support exists but is never populated.

### 11. "Exit Game" instead of "Settings" in home menu (Spec 01)

**Location**: `internal/screens/home/home.go` line 85

Spec requires "Settings" as the last menu item. Implementation has "Exit Game" instead. Reasonable UX choice but deviates from spec.

### 12. Placeholder screen icon deviation (Spec 01)

Uses `╌╌ Coming Soon ╌╌` instead of spec's `Coming Soon!` construction emoji.

### 13. Rarity color naming deviation (Spec 11)

**Location**: 4 screen files with `rarityColor()` function

Spec references `theme.Info` (Rare) and `theme.Warning` (Legendary), but these constants don't exist in the theme. Implementation uses `theme.Secondary` and `theme.Accent` respectively. Visually appropriate but deviates from spec naming.

### 14. ProfileInput.SessionCount not used in prompt (Spec 10)

**Location**: `internal/lessons/prompt.go`

`ProfileInput.SessionCount` is defined in types but never included in `buildProfileUserMessage`. Could provide useful longitudinal context.

### 15. Goroutines without context cancellation (Spec 10)

Profile generation and compression goroutines spawned at session end don't respect context cancellation. Could leak if app shuts down during generation.

### 16. Planner doesn't prioritize rusty skills (Spec 07)

**Location**: `internal/session/planner.go`

Spec Section 6.3 says "Rusty skills take priority over regular frontier skills." The planner doesn't distinguish rusty skills from regular frontier skills.

### 17. Sequence counter uses raw SQL outside ent (Spec 02)

**Location**: `internal/store/event.go`

`global_sequence` table created via raw SQL, not through ent schemas. Creates dual schema management concern. Not auto-migrated by ent.

### 18. LearnerProfile not tested in prompt_test.go (Spec 05)

The `LearnerProfile` field exists in `GenerateInput` and is included in `buildUserMessage()`, but no test case verifies its inclusion in the prompt.

---

## Component Details

### Spec 01 - Skeleton & TUI Framework (85-90%)

**Strengths**:
- All 5 shared UI components (TextInput, Menu, Progress, Button, MultiChoice) match spec exactly
- Theme colors match spec hex values perfectly (10 colors, 14+ styles)
- Router with Push/Pop/Replace works correctly (tests passing)
- Screen interface and KeyHintProvider properly implemented
- Responsive layout system with min-size detection
- Mascot ASCII art matches spec exactly

**Beyond Spec**: Welcome/splash screen with animated transition, full dependency injection in `cmd/run.go` (later components already integrated), `stats` command fully implemented instead of stub.

### Spec 02 - Persistence Layer (~70%)

**Strengths**:
- SQLite via modernc.org/sqlite (pure Go, no CGO)
- All pragmas correctly applied (WAL, busy_timeout, foreign_keys, synchronous)
- EventMixin with sequence + timestamp used by all 8 event schemas
- SnapshotRepo with Save/Latest/Prune working
- Global sequence counter is mutex-protected and atomic
- Append-only design enforced (no Update/Delete operations)

**Missing**: `--db` CLI flag, generic EventRepo queries, snapshot triggers/restore, base Event interface, comprehensive event append tests.

### Spec 03 - Skill Graph (90%)

**Strengths**:
- All 52 skills properly seeded across 5 strands with correct metadata
- All 13 required graph traversal APIs implemented (GetSkill, AllSkills, ByStrand, etc.)
- Topological sort via Kahn's algorithm with cycle detection
- Comprehensive validation (6 checks, panics on failure at init)
- 31 tests, all passing
- Defensive copies via `slices.Clone()` prevent mutation

**Gaps**: SkillMapScreen mastered data always empty, FrontierSkills uses approximation (acceptable per spec), DiagnosticResult skeleton only (intentionally deferred).

### Spec 04 - LLM Integration (92%)

**Strengths**:
- Clean Provider interface with 3 adapters (Anthropic, OpenAI, Gemini)
- Structured output via each provider's native JSON schema support
- Proper error types with unwrapping (ErrRateLimit, ErrInvalidResponse, etc.)
- Retry decorator with exponential backoff + jitter
- JSON schema validation with thread-safe caching (sync.Map)
- Mock provider with FIFO responses and call recording
- Factory creates correct middleware stack: Caller -> Retry -> Logging -> Base
- Graceful degradation when no API key configured

**Bugs**: Provider field in logging records model ID (see Critical Bug #3). RetryAfter not extracted (see Minor Issue #10).

### Spec 05 - Problem Generation (~99%)

**Strengths**:
- Complete validator pipeline: Structural -> AnswerFormat -> MathCheck
- MathCheck with regex-based extractors for integer/decimal/fraction arithmetic
- Answer normalization with equivalence checking (equivalent fractions, trailing zeros)
- Multiple choice support (by index 1-4 or by text, case-insensitive)
- Deduplication via prior questions in prompt context
- Tier-specific behavior (Learn: hints allowed, Prove: no hints)
- 44+ tests, all passing
- LearnerProfile enhancement beyond spec (good)

**Gap**: LearnerProfile not tested in prompt_test.go.

### Spec 06 - Session Engine (99%)

**Strengths**:
- Planner with 60/30/10 frontier/review/booster allocation
- Complete session lifecycle: Start -> Serve -> End, with Early Quit confirmation
- Async question generation with 3 retry attempts
- Mini-block rotation (3 questions per slot, round-robin, skip completed)
- 15-minute countdown timer with graceful expiry
- SessionEvent and AnswerEvent persistence via EventMixin
- Comprehensive test coverage (planner, progress, session, summary)
- Clean integration with specs 07-11 (all optional, nil-checked)

### Spec 07 - Mastery & Scoring (~90%)

**Strengths**:
- State machine: new -> learning -> mastered <-> rusty fully implemented
- Fluency scoring: 0.6*accuracy + 0.2*speed + 0.2*consistency, clamped [0,1]
- Speed uses rolling average (window=10), 0.5 neutral for Learn tier
- Recovery mechanics: 4 questions, 75% accuracy, Learn tier difficulty
- Snapshot migration from old TierProgress format
- MasteryEvent ent schema with proper persistence
- 58 tests, all passing

**Critical Bug**: `CheckReviewPerformance()` never called (see Critical Bug #1). Planner doesn't prioritize rusty skills.

### Spec 08 - Spaced Repetition (~85%)

**Strengths**:
- Interval schedule: [1, 3, 7, 14, 30, 60] days with graduation at stage 6
- ReviewState with IsDue, OverdueDays, IsRustyThreshold, Status methods
- RunDecayCheck at session start marks overdue skills rusty
- DueSkills sorted most-overdue-first for planner
- RecordReview: correct advances stage/hits, incorrect resets hits
- InitSkill/ReInitSkill for mastery transitions
- Bootstrap migration from mastery data when no spaced rep snapshot
- 53 tests, all passing

**Missing**: Skill Map review badges, Stats command sections, session start decay notifications, `app.Options.Scheduler` field.

### Spec 09 - Error Diagnosis (89%)

**Strengths**:
- Rule-based classifiers: SpeedRush (<2s) and Careless (>80% accuracy) with correct priority
- 19 misconceptions across 5 strands (4-4-4-4-3 distribution)
- LLM async diagnosis via buffered channel (size 32), non-blocking
- Validation ensures LLM can't return arbitrary misconception IDs
- MisconceptionPenalty on SkillMastery with IsTierComplete adjustment
- Penalty resets on all 3 tier advancement paths
- BuildErrorContext enriches errors with `[category: label]` suffix
- ~35 tests across all files

**Critical Bug**: `AppendDiagnosisEvent` never called (see Critical Bug #2).

### Spec 10 - AI Lessons & Compression (95%)

**Strengths**:
- Hints: available after first wrong answer, Learn tier only, no LLM call needed
- Micro-lessons: triggered after 2+ wrong on same skill, async generation
- Session compression: 800 char threshold, async, replaces with `[compressed]` prefix
- Learner profile: generated end of session, stored in snapshot, includes previous profile
- Purpose labels correctly set: "lesson", "session-compress", "profile"
- HintEvent and LessonEvent ent schemas with persistence
- Proper mutex protection on concurrent access (ErrorMu)

**Gaps**: SessionCount not used in profile prompt, goroutines without context cancellation.

### Spec 11 - Rewards / Gems (99%)

**Strengths**:
- All 5 gem types with correct icons and display names
- DAG depth quartiles for mastery/recovery/retention rarity
- Streak tracking with threshold advancement (5, 10, 15, 20+)
- Session gems only on natural timer expiration (not early quit)
- Gem Vault and History screens fully functional
- GemEvent ent schema with QueryGemEvents, GemCounts, QuerySessionSummaries
- Inline gem notifications in feedback overlay
- Full snapshot persistence with backward compatibility
- 20+ tests, all passing

**Gap**: Color naming uses theme.Secondary/Accent instead of spec's theme.Info/Warning (constants don't exist in theme).

### Cross-Cutting Wiring

**Strengths**:
- Clean linear dependency flow: Store -> Repos -> Services -> App -> Screens
- No circular dependencies
- All 9 optional services (LLM, Generator, Diagnosis, Lessons, Compressor, GemService, MasteryService, SpacedRepScheduler) nil-checked at every use point
- Factory pattern for HomeScreen (lazy creation via WelcomeScreen)
- LLM middleware stack correctly ordered: Caller -> Retry -> Logging -> Base
- Proper resource cleanup (defer Store.Close, diagService.Close)
- Thread-safe async operations throughout

---

## Recommendations

### Priority 1 - Fix Critical Bugs
1. Add `CheckReviewPerformance()` call after review answer recording in HandleAnswer
2. Add `AppendDiagnosisEvent` calls in session async callback and sync result
3. Fix logging Provider field to record actual provider name

### Priority 2 - Address Major Gaps
4. Wire mastery data into SkillMapScreen
5. Add `--db` CLI flag support
6. Implement spaced rep UI features (Skill Map badges, Stats sections)

### Priority 3 - Minor Improvements
7. Fix header emoji and gem/streak symbols
8. Extract RetryAfter from rate limit headers
9. Add generic EventRepo query methods for future sync support
10. Add context cancellation to async goroutines
11. Test LearnerProfile prompt inclusion

---

## Build & Test Status

- `CGO_ENABLED=0 go build ./...` - **PASSES**
- `go test ./internal/skillgraph` - **31 tests PASS**
- `go test ./internal/llm` - **All tests PASS**
- `go test ./internal/problemgen` - **44+ tests PASS**
- `go test ./internal/session` - **13+ tests PASS**
- `go test ./internal/mastery` - **58 tests PASS**
- `go test ./internal/spacedrep` - **53 tests PASS**
- `go test ./internal/diagnosis` - **35+ tests PASS**
- `go test ./internal/gems` - **20+ tests PASS**
- Router, store, summary tests - **All PASS**
