# Question Latency — no kid waits on the LLM

**Status:** Phases 1a, 1b and 2 implemented (game and terminal). Phase 3
waits for the Phase 2 numbers. Owner decisions (2026-10-17): quest
expeditions keep generating under the expedition lock; the retry copy in
§3.3 is approved; budgets as in §2.1.

## 1. Why

Every question is one blocking LLM call that starts only when the kid asks
for it:

```
answer graded → feedback screen → kid taps "Next clue →" → LLM call → 🧭 spins → question
                 (nothing running)                          (seconds; up to 30s)
```

- `POST /api/v1/game/expeditions/{id}/question` calls
  `Generator.Generate` synchronously inside `game.Manager.Question`
  (`internal/saas/game/manager.go`), with a 30s budget
  (`questionGenBudget`) and up to 3 provider attempts of 20s each
  (`internal/llm/timeout.go`, `retry.go`). Nothing is prefetched; the TUI
  behaves the same way (`internal/screens/session/session.go`
  `generateNextQuestion`).
- The time the kid spends reading feedback is idle. By the time
  `Answer` returns, every input to the next question is already known.
- Failure handling makes it worse:
  - A question rejected by a validator is never retried, although
    specs/05 §"retry" says callers retry up to 3 times.
  - The SPA drops the expedition on the first `503`
    (`web/src/pages/Play.tsx` `nextQuestion`). Tapping the spot again
    finishes the old expedition and charges a **new credit**
    (`Start` reuses only when `questionsAsked == 0`).
  - `exp.mu` is held across the LLM call, so `End` ("Sail home"),
    `Hint`, `Answer` and a replacing `Start` block for up to 30s.
- There is no measurement of how long kids actually wait.

This spec supersedes specs/05 §1's "on-demand, no pre-caching" decision.
**Every served question stays personalised.** Nothing here introduces a
static bank on the normal path.

## 2. Goals and non-goals

Goals, measured with §4 telemetry:
- p50 wait after "Next clue →" ≤ 300 ms; p95 ≤ 3 s.
- Fewer than 1 in 200 question requests surfaces an error to a kid.
- A generation failure never costs a credit and never drops a kid back to
  the map mid-expedition unless the circuit breaker trips.

Non-goals:
- Changing the prompt, the validators or the question content.
- Pre-generated shared pools (they defeat personalisation).
- Streaming question text: the validators need the whole question before
  anything is shown.

### 2.1 Budgets

One budget per question generation, `questionGenBudget` = 30 s, whether it
runs in the background (a warm) or on request. It covers provider retries
and validator retries. A request that joins an in-flight warm waits at most
its own 30 s; if the request gives up, the warm keeps running and a later
request for the same input can still use it. Validator retries (§3.1) start
only while at least 5 s of the deadline remain.

## 3. Phase 1a — failure fixes

These ship first because they're bugs whatever else we do.

### 3.1 Retry rejected questions

`problemgen.LLMGenerator.Generate` re-asks up to **2 more times** when a
validator returns a `ValidationError` with `Retryable: true`, while at
least 5 s of the caller's deadline remain. The rejected question's text goes in the
"already asked" list for the retry, so it isn't regenerated. Each attempt
is a normal LLM call and is logged as one. The game and TUI callers
already treat any error as a failure; they need no change.

### 3.2 Don't hold `exp.mu` across generation

`Manager.Question` becomes lock → read → unlock → generate → lock →
install:

1. Under `exp.mu`: the existing checks (finished, idempotent current
   question, `questionsAsked < total`); build the `GenerateInput`; note
   `asked := exp.questionsAsked`.
2. Release `exp.mu` and generate through the prefetcher (§5), which merges
   concurrent identical requests into one call.
3. Re-take `exp.mu`. If `exp.finished`, return `ErrExpeditionOver`. If
   `exp.questionsAsked != asked`, a concurrent request (a double tap)
   installed a question meanwhile, and the kid may even have answered it:
   start `Question` over, so the normal checks decide (current question,
   next question, or over). If the request's own context ended (closed
   tab, reload), return `ErrClientGone` (logged as 499 `client_gone`): the
   kid saw no failure, so the breaker does not tick. Otherwise install the
   question. Both taps share one generator call through the prefetcher's
   de-duplication.

Quest expeditions skip all of this: their questions are authored and
instant, and `questGenerator` advances a cursor, so they keep generating
under `exp.mu`.

`Hint`, `Answer`, `End` and a replacing `Start` no longer wait on the
LLM. Every handler refuses a finished expedition (`activeLocked`) and does
not refresh its activity, so the idle reaper still collects it. `Answer` during generation returns `ErrNoQuestion`, as it does today
when no question exists. The idle reaper keeps using `lastActivity` and
never takes `exp.mu`.

### 3.3 Keep the expedition alive on failure (client)

On `ErrGeneration` (503) the SPA keeps the expedition and shows a retry
state on the same screen: "The map is foggy… 🌫️" with a **Try again**
button that calls `/question` again. Kid copy only: no error codes, and
nothing about credits. The server's `maxGenFailures = 3` circuit breaker
still ends the expedition (progress saved, play slot freed) but keeps it
reachable: the failure that trips it answers 409 `expedition_over`, and the
client's `End` returns the normal summary instead of a 404. No new
`Start`, so no second debit.

Game errors carry a stable `code` (`generation_failed`, `expedition_over`,
`no_expedition`, …) next to the kid copy, and the client branches on the
code, never on the text.

Also: after 6 s of loading, the loader copy changes to "Still digging…
the treasure is deep today ⛏️", so a long wait reads as progress, not a
hang.

### 3.4 TUI

- Cache the learner profile on the session screen at start instead of
  reading `snapRepo.Latest` on every question.
- Prefetch as in §5 (same component).

## 4. Phase 1b — telemetry (baseline before and after)

- **Canonical log line** on `/question` (request path, so `logctx` fields
  in `internal/saas/server`, per CLAUDE.md):
  - `q_wait_ms`: handler time spent waiting for the question.
  - `q_source`: one of `prefetch_hit` (ready on arrival), `prefetch_join`
    (in flight, awaited), `fresh` (no prefetch), `quest`, `current` (the
    unanswered question again, e.g. a reload).
  - `q_attempts`: provider attempts, retries included, counted by
    `llm.WithAttemptCounter` in `TimeoutProvider` (every attempt passes it,
    with or without event logging).
- **Background slog** for prefetch outcomes (non-request path):
  `slog.Info("question prefetch", "child_uid", uid, "outcome",
  "used|discarded|failed|cancelled", "reason", …, "ms", …)` (the terminal
  logs the same line without `child_uid`). Reasons: `input_changed`,
  `expedition_over`, or the error text for `failed`.
- **PostHog** (follow the `analytics` skill: anonymous kid usage inside the
  family group only, no child identity): add `track.questionShown({index,
  waitMs})` and `track.questionGenerationFailed({index})` in
  `web/src/analytics.ts`. `waitMs` is measured client-side from tap to
  render, so it includes network time.

Ship 1b alongside or before 1a, and collect at least a few days of
baseline before Phase 2 lands.

## 5. Phase 2 — prefetch (A + B)

### 5.1 `problemgen.Prefetcher`

A per-session component that wraps any `Generator`:

```go
// internal/problemgen/prefetch.go
func NewPrefetcher(gen Generator, cfg PrefetchConfig) *Prefetcher // Budget, DisableWarm, LogAttrs
// Warm starts generating input in the background unless an identical
// request is already in flight, ready, or was just handed out.
func (p *Prefetcher) Warm(input GenerateInput)
// Get returns the held question for an identical input — waiting for an
// in-flight one — and otherwise generates fresh.
func (p *Prefetcher) Get(ctx context.Context, input GenerateInput) (Result, error) // Result{Question, Source, Attempts}
// Close cancels in-flight work and drops results. Idempotent.
func (p *Prefetcher) Close()
```

- **Correctness rests on the fingerprint:** a hash of every
  `GenerateInput` field (skill ID, tier, prior questions, recent errors,
  learner profile). If anything the next question depends on changed
  between `Warm` and `Get`, the warmed question is simply not used. No
  caller needs to reason about staleness.
- **At most one warmed question.** A new `Warm` with a different
  fingerprint cancels the old one.
- A failed warm is dropped silently and `Get` generates fresh once. So does
  an entry cancelled under its waiter (replaced, or closed: then `Get`
  returns `ErrPrefetcherClosed`). Neither counts toward `genFailures`;
  only kid-visible failures do.
- Every generation, warm or fresh, runs on the prefetcher's own context
  plus the budget, never on a request context: it outlives the request
  that started it, a caller that leaves cannot cancel a generation another
  caller waits on, and `Close` cancels it. The prefetcher's mutex is never
  held while waiting, and replacing or closing only cancels, never joins,
  so `Warm` and `Close` are safe under the expedition lock.
- A delivered question stays held until a different input replaces it, so
  a late duplicate request gets the same question, not a second call.
- LLM usage logging is unchanged: the provider's `LoggingProvider` records
  each attempt into the child's event stream as today. The owner is bound
  by the repo, not the request context.

### 5.2 Game wiring

- `expedition` gains `prefetch *problemgen.Prefetcher`, built in `Start`
  around `exp.tools.Generator`.
- **B (Q1 during "Setting sail…"):** at the end of `Start`, call
  `Warm(firstInput)`. The client's following `/question` gets it as a hit or
  a join.
- **A (after grading):** at the end of `Answer`, if `questionsAsked <
  total` and the expedition isn't finished, build the next
  `GenerateInput` from the post-`HandleAnswer` state (live tier, updated
  prior questions and recent errors) and `Warm` it. The lesson flow is
  unaffected; if a lesson changes inputs, the fingerprint discards the
  warmed question.
- `Question` uses `prefetch.Get` (§3.2 step 2).
- `finishLocked` calls `prefetch.Close()` **before** `Diagnosis.Close()`
  and before releasing the play slot, so no background generation outlives
  the expedition's resources.
- **Quests are excluded.** `questGenerator` advances a cursor on every
  `Generate`, so a discarded warm would skip a parent's question. Quest
  expeditions use the generator directly, and their questions are
  instant anyway.
- Async error compression can rewrite `RecentErrors` after `Warm`. That
  causes a fingerprint miss (logged `input_changed`), which is correct
  and expected to be rare.

### 5.3 TUI wiring

The session screen owns one `Prefetcher` per session (closed on session
end and every exit path): `Warm` when feedback is shown and the next
question is predictable (same slot, skill not just mastered, time left),
`Get` in `generateNextQuestion`. `tutor.Toolset` gains nothing; the
prefetcher is per session, not per provider. The off switch
`MATHIZ_QUESTION_PREFETCH=0` applies to both surfaces
(`problemgen.PrefetchDisabledByEnv`).

### 5.4 Cost

Phase 2 adds no LLM calls in the common case. The calls it wastes:
- one per expedition ended early with a warm in flight,
- one per fingerprint miss.

Both show up in the prefetch log. A provider outage costs more calls than
before: a failed warm is retried fresh, so one kid-visible failure can take
2 generations × 3 provider attempts, and the breaker trips after up to 18
provider calls instead of 9. Credits are per expedition, so kids and
parents are unaffected.

## 6. Phase 3 — decided by measurement

Each phase ships only if Phase 2 telemetry shows the goal in §2 isn't met.

- **C. Speculative warm.** When a question is installed, also `Warm` the
  input that a *correct* answer would produce (prior questions + this text,
  same tier, same errors). On a correct answer with no tier change it's a
  hit with near-zero wait; otherwise the fingerprint discards it and A
  starts. Costs one wasted call per wrong answer. Gate it behind
  `MATHIZ_QUESTION_SPECULATE=1` and compare cost against the wait gained.
- **D. Two-stage generation.** Stage 1 returns question, format, choices,
  answer and answer type: the parts needed to show and grade the question.
  Stage 2 (hint and explanation) starts as soon as stage 1 is validated
  and must finish before grading returns (`AnswerResult.Explanation`). If
  it isn't ready, the result goes out without an explanation rather than
  waiting. This needs a schema split and its own validators; spec it
  separately if chosen.
- **E. Procedural fallback.** A `problemgen.ProceduralGenerator` for
  pure-computation skills, using per-skill numeric ranges and templates
  and verified by the existing exact-math code (`mathcheck.go`). It is
  wrapped as `FallbackGenerator{Primary, Fallback}`, used **only** when
  the primary fails or exceeds a shorter deadline (for example 10 s), so a
  provider outage becomes a plain question instead of fog. Needs new
  per-skill metadata in the skill seed and an `AnswerEvent` source marker
  so the dashboard and analytics can tell fallback questions apart. Spec it
  separately if chosen.
- **H. LLM layer.** Per-purpose model routing (a faster model for
  `question-gen`), a fallback provider, and one provider per process
  instead of one per expedition. Small, independent PRs.

Rejected: a batch of all 5 questions per expedition (same latency as A+B
but loses mid-expedition adaptation) and shared question pools (not
personal).

## 7. Invariants to preserve

- **One live session per child:** the play slot is released only after the
  final snapshot save; `Close` runs before it (§5.2).
- **Map reads stay side-effect-free:** prefetch never runs from `Map`.
- **Enforcement only at `Start`:** prefetching and retries never debit
  credits.
- **`CheckAnswer` stays the only answer comparator.** Prefetched and
  retried questions go through the same validators.
- **Kid surfaces show no meter, prices or error codes;** retry copy is
  in-world.
- **No child identity in analytics** (§4).

## 8. Testing

- `problemgen`:
  - prefetcher hit, join, fingerprint miss, failed warm falls back to
    fresh, `Close` cancels in-flight work (fake generator with a
    controllable delay);
  - validator retry, including that the rejected text reaches the "already
    asked" list.
- `game` (`-race`):
  - `End` returns while generation is blocked;
  - a double-tap `/question` makes one generator call;
  - `Answer` warms the next question, and Q1 is warmed by `Start`;
  - quest expeditions never warm;
  - `finish` cancels the warm before releasing the slot;
  - three failures trip the breaker without a second debit.
- `web`: retry state on 503 keeps the expedition; the "still digging" copy
  appears after 6 s. Covered in the `saas-e2e` flow with the LLM stub
  delayed (`llmstub.mjs` gains a configurable delay and failure mode).
- TUI: the prefetch path, and the learner-profile cache read once per
  session.

## 9. Rollout

1. Done: Phase 1b telemetry and 1a fixes.
2. Done: Phase 2 (A + B), with `MATHIZ_QUESTION_PREFETCH=0` as an off switch
   for the first release. Remove the switch in the release after. Phases 1
   and 2 ship together, so the "before" baseline is a release run with the
   off switch set.
3. Review the numbers against §2, then decide on C/D/E/H.

Docs to update alongside: specs/05 §1 (point here), specs/13 (question
flow), `docs/personas.md` (kid retry flow), and `specs/glossary.md`:
"Prefetcher" and "question source", following the `glossary` skill.
