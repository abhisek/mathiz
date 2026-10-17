# Mathiz Glossary — first-class concepts

This is the registry of every named concept in Mathiz: what it is, where it
lives in code, and which spec designed it. It exists so that new work reuses
an existing concept (and its name) instead of inventing a near-duplicate.
Code is the truth: where a spec and the code disagree, the entry says so and
describes the code. Entries marked **(proposed, specs/NN)** describe a
design that is not yet in the code.

**How to use.** Before naming a new type, table, event, API resource, UI
term or ledger `source`, search this file (and `grep` the **Code** paths)
for the idea. If it exists, reuse its name verbatim — including the
kid-facing word if the surface is a child surface. If you add, rename,
split or remove a concept, update its entry in the same PR. If two sections
seem to describe your idea, read **Homonyms & look-alikes** at the bottom
first: several common words (plan, session, invite, tier, provider, token,
profile, slot, owner, captain, explorer, fog) already mean different things in
different packages.

Sections: Learning engine · Content generation (LLM) · Persistence ·
Family & identity · Game (treasure map) · Quests · Money · Platform/ops ·
Homonyms & look-alikes. Entries are alphabetical within a section.

---

## Learning engine

The pure Go domain: `internal/{skillgraph,mastery,spacedrep,session,gems}`.
Driven identically by the TUI session screen and the game manager.

### Accuracy
Correct ÷ attempted for one skill; 60 % of the fluency score and the gate for tier completion (`SkillMastery.Accuracy`). Historical accuracy over the whole event stream (`EventRepo.SkillAccuracy`) feeds the careless classifier.
- **Code:** `mastery.SkillMastery.Accuracy` · `store.EventRepo.SkillAccuracy`
- **Spec:** [07-mastery.md §2.3](07-mastery.md)

### Booster
See **Plan category**. A mastered skill scheduled for a confidence-building slot (10 % of a TUI plan; a non-due mastered dig spot on the map). Dashboard copy: "⭐ Confidence builder".
- **Code:** `session.CategoryBooster`
- **Spec:** [06-session.md §2.4](06-session.md)

### Consistency score
Fluency component (20 %): current correct-answer streak ÷ streak cap (default 8), clamped to 1.
- **Code:** `mastery.ConsistencyScore` · `mastery.DefaultStreakCap`
- **Spec:** [07-mastery.md §2.5](07-mastery.md)

### Decay check
Session-start scan that marks overdue mastered skills **rusty** (past the review date plus a 50 % grace of the interval). Persists a mastery event only when the scheduler has a non-nil event repo — map reads run it with nil repos so they stay side-effect-free.
- **Code:** `spacedrep.Scheduler.RunDecayCheck` · `spacedrep.ReviewState.IsRustyThreshold`
- **Spec:** [08-spaced-rep.md §4.1](08-spaced-rep.md)
- **Aka / don't call it:** "time-decay" (the mastery trigger string). Not "expiry".

### Dependents
Skills that list a given skill as a prerequisite; the set that may unlock when it is mastered ("map reveal").
- **Code:** `skillgraph.Dependents`
- **Spec:** [03-skill-graph.md "Graph Traversal API"](03-skill-graph.md)

### Diagnostic placement
Designed top-down probing quiz to place a learner on the graph. **Not implemented** — only a stub result type exists.
- **Code:** `skillgraph.DiagnosticResult` (stub)
- **Spec:** [03-skill-graph.md "Diagnostic Placement"](03-skill-graph.md)

### Error context (recent errors)
Per-skill list of natural-language strings describing recent wrong answers ("Answered 623 for '345 + 289', correct was 634 [careless]"), capped at 5, fed to the LLM on the next generation. Enriched with the diagnosis category/misconception when available; compressed by the Compressor once it exceeds 800 chars.
- **Code:** `session.BuildErrorContext` · `session.MaxRecentErrors` · `session.SessionState.RecentErrors`
- **Spec:** [05-problem-gen.md §13](05-problem-gen.md) · [09-diagnosis.md §8.2](09-diagnosis.md)

### Fluency score
0–1 per-skill score = 0.6·accuracy + 0.2·speed + 0.2·consistency. Recorded on every mastery event; shown in the TUI summary and parent stats.
- **Code:** `mastery.FluencyScore` · `mastery.FluencyMetrics`
- **Spec:** [07-mastery.md §2](07-mastery.md)

### Frontier
Skills that are unlocked but not yet mastered — where learning happens. 60 % of a TUI plan; the default category of a map dig on a `new`/`learning` spot. Dashboard copy: "🌱 New skill".
- **Code:** `skillgraph.FrontierSkills` · `session.CategoryFrontier`
- **Spec:** [06-session.md §2.2](06-session.md)

### Gem
A reward earned for a real learning milestone — never for login streaks or time spent. Five types × four rarities; persisted as gem events, counted in the snapshot, shown in the Gem Vault (TUI) and the kid's 💎 vault (web).
- **Code:** `gems.GemAward` · `gems.Service` · `ent/schema/gem_event.go`
- **Spec:** [11-rewards.md](11-rewards.md)

### Gem rarity
`common | rare | epic | legendary`. Mastery/recovery/retention gems take rarity from the skill's DAG depth quartile; streak gems from streak length (5/10/15/20); session gems from session accuracy (50/75/90 %).
- **Code:** `gems.Rarity` · `gems.ComputeDepthMap` · `gems.StreakRarity` · `gems.SessionRarity`
- **Spec:** [11-rewards.md §3](11-rewards.md)

### Gem type
`mastery` (first time mastered), `recovery` (rusty → mastered), `retention` (spaced-rep graduation), `streak` (in-session correct streak at 5/10/15/20, then every 5), `session` (a fully completed session/expedition). The kid web surface renames them — see **Gem (kid labels)** in Game.
- **Code:** `gems.GemType` · `gems.AllGemTypes`
- **Spec:** [11-rewards.md §2](11-rewards.md)
- **Aka / don't call it:** Go icons (💎🔥🛡️⚡🏆) differ from the web's (🏆🔥⛵💪🛡️); the IDs are canonical, the icons are per-surface.

### Grade level
US school grade 2–5 on a skill (`GradeLevel`) and on a child profile (`grade`). Skills are laid out in grade columns on the map and grouped by grade in the curriculum.
- **Code:** `skillgraph.Skill.GradeLevel` · `ent/schema/child_profile.go` (`grade`)
- **Spec:** [03-skill-graph.md "Skill Node Schema"](03-skill-graph.md)

### Graduation (spaced rep)
A mastered skill that has passed all six review stages (6 consecutive hits) graduates: 90-day interval, retention gem awarded. Lost again on recovery from rusty.
- **Code:** `spacedrep.GraduationStage` · `spacedrep.GraduatedIntervalDays` · `spacedrep.ReviewState.Graduated`
- **Spec:** [08-spaced-rep.md §2.2](08-spaced-rep.md)

### HandleAnswer
The single grading entry point. Checks the answer, records mastery and fluency, updates tier progress, awards streak/mastery/recovery gems, runs diagnosis, accumulates error context, computes hint availability, records spaced-rep reviews, triggers the lesson request, and appends answer/diagnosis events. Both surfaces call it; nothing else grades.
- **Code:** `session.HandleAnswer`
- **Spec:** [06-session.md §4.2](06-session.md) · [10-ai-lessons.md §8.1](10-ai-lessons.md)

### Interval schedule (review stages)
Expanding review intervals in days: stage 0..5 = 1, 3, 7, 14, 30, 60; graduated = 90. A correct review advances the stage; a wrong one resets consecutive hits but keeps the stage.
- **Code:** `spacedrep.BaseIntervals` · `spacedrep.MaxStage`
- **Spec:** [08-spaced-rep.md §2.1](08-spaced-rep.md)

### Learner
The person whose events/snapshot a repo is scoped to: the local CLI user, or one child profile in serve mode. Domain packages say "learner"; SaaS code says "child". The same person.
- **Code:** `store.EventRepoFor(owner)` (owner = learner)
- **Spec:** [12-saas.md §3.1](12-saas.md)

### Mastery state
Per-skill lifecycle: `new → learning → mastered ⇄ rusty`. Mastered means the Prove tier was passed; rusty means decayed or poor recent review performance (last 4 reviews < 50 %). Stored in the snapshot; every change is a mastery event.
- **Code:** `mastery.MasteryState` · `mastery.Service.RecordAnswer` · `mastery.Service.MarkRusty`
- **Spec:** [07-mastery.md §3](07-mastery.md)
- **Aka / don't call it:** distinct from **Skill state (display)** — don't add a state here for UI reasons.

### Mastery transition
A recorded state change with a `Trigger` string: `first-attempt`, `tier-complete`, `prove-complete`, `time-decay`, `review-performance`, `recovery-complete`. Drives gems, map reveals and the dashboard "Milestones" kind (only to-`mastered`/`rusty` are shown to parents).
- **Code:** `mastery.StateTransition` · `store.MasteryEventData`
- **Spec:** [07-mastery.md §3.3](07-mastery.md)

### Misconception penalty
Each wrong answer diagnosed as a misconception adds +1 to the correct answers required to complete the current tier of that skill. Reset on tier advancement.
- **Code:** `mastery.SkillMastery.MisconceptionPenalty` · `mastery.SkillMastery.IsTierComplete`
- **Spec:** [09-diagnosis.md §7](09-diagnosis.md)

### Plan (session plan)
The ordered list of plan slots a session will cycle through. TUI: five slots (3 frontier / 1 review / 1 booster), 15 minutes. Game: exactly one slot for the tapped skill. Serialized onto the session "start" event as `plan_summary`.
- **Code:** `session.Plan` · `session.DefaultTotalSlots` · `store.PlanSlotSummaryData`
- **Spec:** [06-session.md §2.6](06-session.md)
- **Aka / don't call it:** NOT a billing plan — see Homonyms.

### Plan category
Why a skill is in the plan: `frontier` (learning it), `review` (spaced-rep due or rusty), `booster` (mastered, confidence). Recorded on every answer event and surfaced to parents as the expedition "why" chip (🌱 New skill / 🔄 Review / ⭐ Confidence builder).
- **Code:** `session.PlanCategory` · `game.Manager.Start` (category derivation)
- **Spec:** [06-session.md §2.1](06-session.md) · [12-saas.md "Activity timeline"](12-saas.md)

### Plan slot
One (skill, tier, category) triple in a plan. The TUI serves 3 questions per slot ("mini-block") then cycles; a completed tier retires its slot.
- **Code:** `session.PlanSlot` · `session.QuestionsPerSlot` · `session.AdvanceSlot`
- **Spec:** [06-session.md §10](06-session.md)
- **Aka / don't call it:** NOT a play slot — see Homonyms.

### Planner
Builds the TUI's 60/30/10 plan from mastered set, tier progress and the scheduler's due skills. The game does not use it (single-slot plan built inline).
- **Code:** `session.Planner` · `session.DefaultPlanner.BuildPlan`
- **Spec:** [06-session.md §2](06-session.md)

### Prerequisite
Edge in the skill DAG. Every prerequisite must be `mastered` for a skill to unlock. Drawn as paths/trails on the map.
- **Code:** `skillgraph.Skill.Prerequisites` · `skillgraph.Prerequisites`
- **Spec:** [03-skill-graph.md "Prerequisite Model"](03-skill-graph.md)

### Recovery
Returning a rusty skill to mastered: 4 Learn-tier-difficulty questions at ≥ 75 % (hints allowed). Awards a recovery gem and re-initializes the review schedule at stage 0.
- **Code:** `mastery.RecoveryTierConfig` · `spacedrep.Scheduler.ReInitSkill`
- **Spec:** [07-mastery.md §3.3](07-mastery.md) · [08-spaced-rep.md §4.5](08-spaced-rep.md)

### Review (spaced-rep)
A practice answer on a mastered/rusty skill that feeds the review schedule (`RecordReview`) and the review-performance check. Also the plan category for such a slot. Kid-facing: a "sinking" spot.
- **Code:** `spacedrep.Scheduler.RecordReview` · `session.CategoryReview` · `store.EventRepo.RecentReviewAccuracy`
- **Spec:** [08-spaced-rep.md §4.3](08-spaced-rep.md)

### Review state
Per-skill spaced-rep record: stage, next/last review date, consecutive hits, graduated. Lives in the snapshot's `spaced_rep` block; bootstrapped from mastery data when absent.
- **Code:** `spacedrep.ReviewState` · `store.ReviewStateData` · `spacedrep.BootstrapFromMastery`
- **Spec:** [08-spaced-rep.md §3.1](08-spaced-rep.md)

### Review status
UI-only classification of a review state at a moment: `not_due | due | overdue | graduated`. Due skills sort most-overdue first for the planner.
- **Code:** `spacedrep.ReviewStatus` · `spacedrep.Scheduler.DueSkills`
- **Spec:** [08-spaced-rep.md §3.2](08-spaced-rep.md)

### Root skill
A skill with no prerequisites. The seed has exactly one (`pv-hundreds`); validation panics otherwise. Tests must use `AllSkills()`, not `RootSkills()[:N]`.
- **Code:** `skillgraph.RootSkills` · `skillgraph.Validate`
- **Spec:** [03-skill-graph.md "Graph Validation"](03-skill-graph.md)

### Rusty
Mastery state for a previously mastered skill that decayed (overdue past grace) or whose recent reviews were poor. Excluded from `DueSkills`; recovered via **Recovery**. Kid-facing: "sinking" spot; dashboard pill "🌧️ Rusty".
- **Code:** `mastery.StateRusty` · `spacedrep.Scheduler.RunDecayCheck`
- **Spec:** [07-mastery.md §3.3](07-mastery.md)

### Session
One run of the engine for one learner, identified by a UUID: a "start" event, N answers, an "end" event, then a snapshot save with async profile refresh. TUI = 15-minute mixed plan; game = one expedition (5 questions, one skill). The ledger charges per session ID.
- **Code:** `session.SessionState.SessionID` · `store.SessionEventData` · `session.SaveSnapshotWithProfile`
- **Spec:** [06-session.md §4](06-session.md) · [13-treasure-map.md §2](13-treasure-map.md)
- **Aka / don't call it:** "expedition" is the kid-facing name for a game session; "practice session" in TUI copy. Not a login session.

### Session phase
TUI session lifecycle: `loading → active → feedback → ending → summary`. Not used by the game manager (HTTP calls are the phases).
- **Code:** `session.SessionPhase`
- **Spec:** [06-session.md §5](06-session.md)

### Session state
The in-memory bag the engine mutates: plan, current question, per-skill results, tier progress, mastered set, recent errors, streak, pending lesson/gem, and the injected services (mastery, scheduler, diagnosis, lessons, compressor, gems). Built per session from the latest snapshot.
- **Code:** `session.SessionState` · `session.NewSessionState`
- **Spec:** [06-session.md §5](06-session.md)

### Session summary
End-of-session roll-up: duration, totals, per-skill results with tier before/after and fluency, gems earned. TUI "Session Summary" screen; game `SummaryView`.
- **Code:** `session.SessionSummary` · `session.BuildSummary` · `game.SummaryView`
- **Spec:** [06-session.md §8](06-session.md)

### Skill
A node of the curriculum DAG: ID (kebab-case, e.g. `add-3digit`), name, description, strand, grade, Common Core ID, keywords, prerequisites, two tier configs. 56 skills in the seed (the comment in `seed.go` still says 54 — stale). Kid-facing: a **dig spot**.
- **Code:** `skillgraph.Skill` · `internal/skillgraph/seed.go` · `skillgraph.GetSkill`
- **Spec:** [03-skill-graph.md "Skill Node Schema"](03-skill-graph.md)

### Skill graph
The package-level singleton DAG of 56 skills across 5 strands, one root, validated in `init()` (cycles, dangling prereqs, duplicate IDs, bad tier configs panic). Also the data behind the TUI Skill Map, the kid's treasure map and the public curriculum API.
- **Code:** `internal/skillgraph/graph.go` · `skillgraph.TopologicalOrder`
- **Spec:** [03-skill-graph.md](03-skill-graph.md)
- **Aka / don't call it:** "curriculum" (public API / dashboard), "map" (kid).

### Skill state (display)
UI projection combining mastery state, prerequisites and tier: `locked 🔒 | available 🔓 | learning 📖 | proving 📝 | mastered ✅ | rusty 🔄`. The game translates these again into spot states.
- **Code:** `skillgraph.SkillState` · `mastery.ResolveDisplayState`
- **Spec:** [03-skill-graph.md "Skill States"](03-skill-graph.md)

### Speed score
Fluency component (20 %): response time vs the tier's time limit (1.0 at ≤ 50 % of limit, 0.5 at the limit, decaying after). Learn tier is untimed → neutral 0.5. Rolling window of the last 10.
- **Code:** `mastery.SpeedScore` · `mastery.RecordSpeed` · `mastery.DefaultSpeedWindow`
- **Spec:** [07-mastery.md §2.4](07-mastery.md)

### Strand
One of five content areas: `number-and-place-value`, `addition-and-subtraction`, `multiplication-and-division`, `fractions`, `measurement`. Kid-facing: an **island**. Misconceptions are indexed by strand.
- **Code:** `skillgraph.Strand` · `skillgraph.AllStrands` · `skillgraph.StrandDisplayName`
- **Spec:** [03-skill-graph.md "Strands"](03-skill-graph.md)

### Streak (in-session)
Consecutive correct answers within one session; streak gems at 5, 10, 15, 20 then every 5. Distinct from the per-skill fluency streak (consistency) which persists across sessions.
- **Code:** `session.SessionState.ConsecutiveCorrect` · `gems.NextStreakThreshold` · `gems.BaseStreakThreshold`
- **Spec:** [11-rewards.md §6](11-rewards.md)

### Tier (learn / prove)
Each skill has two difficulty tiers. **Learn**: 8 problems, 75 %, untimed, hints allowed. **Prove**: 6 problems, 85 %, 30 s/problem, no hints. Passing Learn unlocks Prove; passing Prove = mastered. Serialized as `"learn"`/`"prove"`.
- **Code:** `skillgraph.Tier` · `skillgraph.TierConfig` · `skillgraph.DefaultTiers` · `session.TierString`
- **Spec:** [03-skill-graph.md "Difficulty Tiers"](03-skill-graph.md)
- **Aka / don't call it:** NOT a billing tier/plan, NOT gem rarity — see Homonyms.

### Tier progress / tier advancement
Cumulative attempts and correct count toward the current tier. The authoritative copy is `SkillMastery` (mastery service); `session.TierProgress` is the pre-mastery legacy shape still used to seed the session and for the TUI fallback path. `TierAdvancement` is the display record of Learn→Prove or →Mastered ("You found the vault — now prove it!").
- **Code:** `mastery.SkillMastery.IsTierComplete` · `session.TierProgress` · `session.TierAdvancement`
- **Spec:** [06-session.md §3](06-session.md) · [07-mastery.md §5.2](07-mastery.md)

### Unlocked
A skill whose prerequisites are all mastered (`IsUnlocked`). `AvailableSkills` = unlocked and not mastered; `BlockedSkills` = the rest. Kid-facing: fog lifts.
- **Code:** `skillgraph.IsUnlocked` · `skillgraph.AvailableSkills` · `skillgraph.BlockedSkills`
- **Spec:** [03-skill-graph.md "Graph Traversal API"](03-skill-graph.md)

---

## Content generation (LLM)

`internal/{llm,problemgen,diagnosis,lessons,tutor}`. The LLM generates;
deterministic Go validates, grades and classifies.

### Answer format
How the learner answers: `numeric` (types a value) or `multiple_choice` (picks one of exactly 4 choices). Stored on answer events as `answer_format`.
- **Code:** `problemgen.AnswerFormat` · `problemgen.FormatNumeric` / `FormatMultipleChoice`
- **Spec:** [05-problem-gen.md §2 "Answer Format"](05-problem-gen.md)

### Answer type
Numeric representation of the canonical answer: `integer | decimal | fraction | text` (`text` only with multiple choice, for conceptual questions). Drives normalization in `CheckAnswer` and the math-check recompute.
- **Code:** `problemgen.AnswerType`
- **Spec:** [05-problem-gen.md §2 "Answer Types"](05-problem-gen.md)

### CheckAnswer
The **only** answer comparator in the codebase. Trims, case-folds, reduces fractions, drops trailing/leading zeros, accepts MC by text or 1–4 index. Lesson practice (`Lesson.GradePractice`), quest questions and every session go through it. Never write another.
- **Code:** `problemgen.CheckAnswer`
- **Spec:** [05-problem-gen.md §10](05-problem-gen.md)

### Choice shuffling
Every generated MC question has its choices shuffled after validation (LLMs put the right answer first). Safe because grading is by text. Parent-authored quest questions are deliberately **not** shuffled.
- **Code:** `problemgen.Question.ShuffleChoices`
- **Spec:** [05-problem-gen.md §7](05-problem-gen.md) (shuffle rule is code-only)

### Classifier (rule-based)
Synchronous, deterministic wrong-answer rules run in priority order: speed-rush (< 2000 ms) then careless (historical accuracy > 80 %). First match wins; no match → async LLM diagnosis.
- **Code:** `diagnosis.Classifier` · `diagnosis.DefaultClassifiers` · `diagnosis.SpeedRushThresholdMs` · `diagnosis.CarelessAccuracyThreshold`
- **Spec:** [09-diagnosis.md §3](09-diagnosis.md)

### Compressor (context compression)
LLM service with two jobs: **session compression** — collapse a skill's error context into one summary once it passes 800 chars (purpose `session-compress`); **profile generation** — produce the learner profile after a session (purpose `profile`).
- **Code:** `lessons.Compressor` · `lessons.Compressor.CompressErrors` · `lessons.Compressor.GenerateProfile` · `lessons.SessionCompressionThreshold`
- **Spec:** [10-ai-lessons.md §4–5](10-ai-lessons.md)

### Diagnosis
Classification of a wrong answer into an error category (plus misconception ID, confidence, classifier name, reasoning). Rules run synchronously inside `HandleAnswer`; the LLM diagnoser runs async and its result is appended as a diagnosis event. Feeds error context, the misconception penalty and lesson input.
- **Code:** `diagnosis.Service.Diagnose` · `diagnosis.DiagnosisResult` · `ent/schema/diagnosis_event.go`
- **Spec:** [09-diagnosis.md §6](09-diagnosis.md)

### Diagnoser (LLM)
The async LLM fallback: given the question, answer and the strand's known misconceptions, returns a misconception ID (validated against the taxonomy) or null. Purpose label `error-diagnosis`.
- **Code:** `diagnosis.Diagnoser` · `diagnosis.DiagnosisRequest` · `llm.PurposeDiagnosis`
- **Spec:** [09-diagnosis.md §5](09-diagnosis.md)

### Difficulty (self-assessed)
The LLM's 1–5 difficulty rating on a question. Analytics only — never gates anything. Not to be confused with tier.
- **Code:** `problemgen.Question.Difficulty`
- **Spec:** [05-problem-gen.md §2](05-problem-gen.md)

### Error category
`careless | speed-rush | misconception | unclassified`. Persisted on diagnosis events and prefixed into error context.
- **Code:** `diagnosis.ErrorCategory`
- **Spec:** [09-diagnosis.md §2](09-diagnosis.md)

### Explanation
Short worked solution attached to every question, shown after grading (kid: after a wrong map dig; after a correct quest answer — see **Sealed answer**). Also on lesson practice questions.
- **Code:** `problemgen.Question.Explanation` · `game.AnswerResultView.Explanation`
- **Spec:** [05-problem-gen.md §2](05-problem-gen.md)

### Generator
Interface that produces one validated `Question` from a `GenerateInput` (skill, tier, prior questions, recent errors, learner profile). `LLMGenerator` is production; tests inject fakes. Purpose label `question-gen`.
- **Code:** `problemgen.Generator` · `problemgen.LLMGenerator` · `problemgen.GenerateInput`
- **Spec:** [05-problem-gen.md §5](05-problem-gen.md)

### Hint
Optional short nudge generated with Learn-tier questions only. Becomes available after one wrong answer on the current question, once; viewing it appends a hint event. Kid-facing: "🗺️ Show me a clue".
- **Code:** `problemgen.Question.Hint` · `session.SessionState.HintAvailable` · `ent/schema/hint_event.go`
- **Spec:** [10-ai-lessons.md §2](10-ai-lessons.md)
- **Aka / don't call it:** there is no `hint` LLM purpose — hints ride along with question generation.

### Learner profile
LLM-written holistic summary of the learner (summary, strengths, weaknesses, patterns, generated-at), made from the child's history across sessions (lifetime answer totals, recent sessions, diagnosed error patterns) with the last session as the newest evidence. Refreshed asynchronously after a session only past the evidence gate (15 lifetime answers), and then only for a first profile, a change of mastery state or tier, or 3 sessions since the last one. Stored on the latest snapshot and — when changed — appended as a learner-profile event. Fed into question generation and shown to parents as "what the tutor has learned".
- **Code:** `lessons.LearnerProfile` · `store.LearnerProfileData` · `session.SaveSnapshotWithProfile`
- **Spec:** [10-ai-lessons.md §5](10-ai-lessons.md) · [12-saas.md §3.1](12-saas.md)
- **Aka / don't call it:** NOT a child profile — see Homonyms.

### Lesson (micro-lesson)
LLM-generated remediation for one skill: title, explanation, worked example, practice question. Requested asynchronously after **2 wrong answers on the same skill in one session**; best-effort (if slow, play continues). Full content is persisted as a lesson event so it can be revisited. Kid-facing: "🧭 The guide has a tip for you".
- **Code:** `lessons.Lesson` · `lessons.Service.RequestLesson` / `ConsumeLesson` · `ent/schema/lesson_event.go`
- **Spec:** [10-ai-lessons.md §3](10-ai-lessons.md)

### LLM middleware
Decorator stack built by the factory: caller → retry (backoff, `Retry-After`) → timeout (per-purpose budgets, e.g. 20 s question-gen, 120 s quest-gen) → logging (LLM request events) → base provider. A timed-out attempt is still logged.
- **Code:** `llm.NewProvider` · `llm.WithRetry` · `llm.WithTimeout` · `llm.WithLogging`
- **Spec:** [04-llm-integration.md §7–9](04-llm-integration.md)

### LLM provider
The vendor abstraction: `Generate(ctx, Request) → Response` with a JSON schema for structured output, plus `ModelID()`. Adapters: `anthropic`, `openai` (any OpenAI-compatible base URL), `gemini`, `openrouter`, `mock`. Selected by env.
- **Code:** `llm.Provider` · `llm.Request` / `llm.Response` / `llm.Schema` · `internal/llm/factory.go`
- **Spec:** [04-llm-integration.md §3–4](04-llm-integration.md)
- **Aka / don't call it:** NOT a billing provider or auth provider — see Homonyms.

### LLM purpose
Context label identifying why the LLM is called; keyed to per-purpose timeouts and recorded on every LLM request event for auditing. Values: `question-gen`, `quest-gen`, `lesson`, `error-diagnosis`, `session-compress`, `profile`.
- **Code:** `llm.WithPurpose` / `llm.PurposeFrom` · `internal/llm/timeout.go` (constants)
- **Spec:** [04-llm-integration.md §8 "Purpose Labels"](04-llm-integration.md)

### Math-check validator
Pure-Go recompute of the answer from the question text (arithmetic and fraction expressions, ~70 % of skills); non-computable questions pass silently. Also the quest editor's "typo warning".
- **Code:** `problemgen.MathCheckValidator`
- **Spec:** [05-problem-gen.md §8.3](05-problem-gen.md)

### Misconception (taxonomy)
A named, strand-scoped wrong-thinking pattern (e.g. `frac-add-straight`, `add-no-carry`); 19 seeded. The LLM diagnoser may only return IDs from this registry.
- **Code:** `diagnosis.Misconception` · `internal/diagnosis/taxonomy_seed.go` · `diagnosis.MisconceptionsByStrand`
- **Spec:** [09-diagnosis.md §4](09-diagnosis.md)

### Mock provider
Canned-response `llm.Provider` for tests (`MockResponse{Content: json}`); records calls. E2E stubs the LLM differently — via an OpenAI-compatible HTTP stub and `MATHIZ_OPENAI_BASE_URL`.
- **Code:** `llm.NewMockProvider` · `llm.MockResponse`
- **Spec:** [04-llm-integration.md §4.4](04-llm-integration.md)

### Model cost
Embedded USD-per-million-token table used by `mathiz llm stats` to estimate spend. Unknown models cost nil.
- **Code:** `llm.ModelCost` · `llm.LookupCost`
- **Spec:** [04-llm-integration.md §13](04-llm-integration.md)

### Practice question (lesson)
The try-it-yourself question inside a micro-lesson (text, answer, answer type, explanation). Graded through `CheckAnswer`; attempt/correct/skip recorded on the lesson event. Its reveal is always safe (different numbers from the gating question).
- **Code:** `lessons.PracticeQuestion` · `lessons.Lesson.GradePractice`
- **Spec:** [10-ai-lessons.md §3.2](10-ai-lessons.md)

### Prefetcher
Generates the next question while the learner reads feedback: one held question, used only for an input whose fingerprint (every `GenerateInput` field) is identical, so staleness needs no reasoning. Generations run on its own context, so a caller that leaves can't cancel a shared one. One per expedition (none for quests) and one per terminal session.
- **Code:** `problemgen.Prefetcher` · `problemgen.PrefetchConfig`
- **Spec:** [19-question-latency.md §5](19-question-latency.md)

### Prior questions (dedup)
Texts of questions already asked for a skill this session, passed to the prompt (last 8) so the LLM varies.
- **Code:** `problemgen.GenerateInput.PriorQuestions` · `problemgen.Config.MaxPriorQuestions`
- **Spec:** [05-problem-gen.md §11](05-problem-gen.md)

### Question
The generated unit of practice: text, format, canonical answer, answer type, choices, hint, difficulty, explanation, skill ID, tier. Never static — generated per learner at ask time. Kid-facing: a "clue" ("Next clue →").
- **Code:** `problemgen.Question` · `problemgen.QuestionSchema`
- **Spec:** [05-problem-gen.md §2](05-problem-gen.md)
- **Aka / don't call it:** a **quest question** is a different, parent-authored type (see Quests).

### Question source
Where a served question came from: `prefetch_hit` (warmed, ready on arrival), `prefetch_join` (warm in flight, awaited), `fresh`, `quest` (authored, no LLM), `current` (the unanswered question again). Telemetry only: the `q_source` field of the `/question` canonical line, never sent to the kid.
- **Code:** `problemgen.Source` · `game.QuestionSource` · `internal/saas/server/game_api.go`
- **Spec:** [19-question-latency.md §4](19-question-latency.md)

### Skill preview
`mathiz preview --skill X [--tier prove]` generates questions for a skill with no database — for evaluating prompts and new skills.
- **Code:** `cmd/preview.go` · `cmd/skill.go`
- **Spec:** [docs/skill-preview.md](../docs/skill-preview.md)

### Structural / answer-format validators
First two links of the default validator chain: field presence, lengths, difficulty range, known format/type; then answer-vs-type consistency and 4 choices containing the answer.
- **Code:** `problemgen.StructuralValidator` · `problemgen.AnswerFormatValidator`
- **Spec:** [05-problem-gen.md §8.1–8.2](05-problem-gen.md)

### Toolset (tutor)
The per-learner bundle of AI services — Generator, Diagnosis, Lessons, Compressor — built from one provider at one construction point so the TUI and the game can never drift. Everything but the Generator is nil-able (= feature off).
- **Code:** `tutor.Toolset` · `tutor.New` · `game.ToolsetFactory`
- **Spec:** [13-treasure-map.md §1](13-treasure-map.md) (code-level concept; no dedicated spec)

### Validator (chain)
Ordered, stateless checks every generated question must pass; first failure stops the pipeline and, if `Retryable`, triggers regeneration (up to 3 failures in the game). Default chain: structural → answer-format → math-check.
- **Code:** `problemgen.Validator` · `problemgen.ValidationError` · `problemgen.DefaultConfig`
- **Spec:** [05-problem-gen.md §3, §9](05-problem-gen.md)

---

## Persistence

`internal/store` + `ent/schema`. Event sourcing with periodic snapshots,
one `store.Open(dsn)` for SQLite (local) and PostgreSQL (serve).

### Answer event
One row per graded answer: session, skill, tier, category, question text, correct/learner answer, correctness, time, format. Backs accuracy queries, the activity session detail and the quest-attribution reader.
- **Code:** `ent/schema/answer_event.go` · `store.AnswerEventData` · `store.EventRepo.AnswersForSession`
- **Spec:** [06-session.md §6.2](06-session.md)

### Control plane vs data plane
**Data plane** = owner-scoped learning events + snapshots (guarded by `owner_id`). **Control plane** = family-scoped ent tables with no `owner_id` (Account, FamilySpace, FamilyMember, ChildProfile, Invite, ParentInvite, DeviceToken, CreditEntry, BillingState, Quest, QuestQuestion, QuestProgress) protected by authz. `ownerguard.go`'s comment lists only the first seven — stale.
- **Code:** `internal/store/ownerguard.go` (`ownerScopedTypes`) · `ent/schema/`
- **Spec:** [12-saas.md §3](12-saas.md)

### Diagnosis event
Persisted diagnosis result for a wrong answer (category, misconception ID, confidence, classifier, reasoning). The learner profile reads it back as counts per skill, category and misconception.
- **Code:** `ent/schema/diagnosis_event.go` · `store.DiagnosisEventData` · `store.EventRepo.DiagnosisCounts`
- **Spec:** [09-diagnosis.md §9](09-diagnosis.md)

### Event (event sourcing)
An immutable, append-only fact in the learner's stream. Each event type is its own ent table embedding `EventMixin` (sequence, timestamp, owner_id). Nine types: answer, session, mastery, diagnosis, hint, lesson, learner-profile, gem, LLM-request. Spec 02's names (`AnswerAttemptEvent`, `SkillStateChangeEvent`, …) were superseded by these.
- **Code:** `ent/schema/event_mixin.go` · `internal/store/*_event.go`
- **Spec:** [02-persistence.md §4](02-persistence.md)

### Event repo
The append/query interface over all event types, scoped to one owner. Changing it ripples to five hand-written mocks in `internal/{gems,mastery,spacedrep,session,screens/session}/*_test.go`. Server code must only use `Store.EventRepoFor(childUID)`.
- **Code:** `store.EventRepo` · `store.Store.EventRepoFor`
- **Spec:** [02-persistence.md §6](02-persistence.md)

### Family LLM stream
Owner ID `space:<familySpaceID>` under which parent-initiated quest-generation LLM calls are logged (they belong to no child). Isolated by the owner guard like any other owner.
- **Code:** `quests.LLMOwnerID`
- **Spec:** [15-quests.md §3](15-quests.md) (code-only detail)

### Gem event
One row per gem award (type, rarity, skill, session, reason). `GemCounts` aggregates for the vault and snapshot.
- **Code:** `ent/schema/gem_event.go` · `store.EventRepo.GemCounts`
- **Spec:** [11-rewards.md §4.2](11-rewards.md)

### Global sequence
Single monotonic counter (`global_sequence` table, raw portable SQL) stamped on every event regardless of table, so cross-type ordering and snapshot consistency work. Global, not per owner. Activity paging cursors are sequences.
- **Code:** `internal/store/event.go` (`sequenceCounter`)
- **Spec:** [02-persistence.md §4 "Base Fields"](02-persistence.md) · [12-saas.md §3.1](12-saas.md)

### Hint event
Records that a hint was shown (session, skill, question, hint text). Counted per session for the activity detail.
- **Code:** `ent/schema/hint_event.go` · `store.EventRepo.HintCountForSession`
- **Spec:** [10-ai-lessons.md §6.1](10-ai-lessons.md)

### Learner profile event
Append-only history of learner-profile versions; written only when the regenerated profile differs. No consumer reads it yet except the recovery path when the snapshot read fails.
- **Code:** `ent/schema/learner_profile_event.go` · `store.EventRepo.AppendLearnerProfileEvent`
- **Spec:** [12-saas.md §3.1](12-saas.md)

### Lesson event
Full micro-lesson content plus practice outcome, so past tips can be re-read (guide's notebook, local TUI alike).
- **Code:** `ent/schema/lesson_event.go` · `store.EventRepo.QueryLessonEvents`
- **Spec:** [10-ai-lessons.md §6.2](10-ai-lessons.md) · [13-treasure-map.md §4](13-treasure-map.md)

### LLM request event
Audit row for every LLM call: provider, model, purpose, tokens, latency, success, full request/response bodies. Per learner in serve mode; browsed by `mathiz llm` locally. The schema comment's purpose list is stale (see **LLM purpose**).
- **Code:** `ent/schema/llm_request_event.go` · `store.LLMRequestEventData` · `llm.LoggingProvider`
- **Spec:** [04-llm-integration.md §8](04-llm-integration.md) · [docs/llm-usage-auditing.md](../docs/llm-usage-auditing.md)

### Local owner
Owner ID `""` — the single-learner CLI's view, predating multi-tenancy. `Store.EventRepo()`/`SnapshotRepo()` are shorthands for it and must not be used server-side.
- **Code:** `store.LocalOwner` · `store.Store.EventRepo`
- **Spec:** [12-saas.md §3.1](12-saas.md)

### Mastery event
Persisted mastery transition (skill, from, to, trigger, fluency, session). Parents see to-`mastered`/`rusty` ones as "Milestones".
- **Code:** `ent/schema/mastery_event.go` · `store.EventRepo.QueryMasteryEvents`
- **Spec:** [07-mastery.md §7.4](07-mastery.md)

### Owner / owner_id
Tenant key on every data-plane row: the child profile UID in serve mode, `""` locally, `space:<id>` for family LLM streams. Repos stamp it on append and filter on query.
- **Code:** `ent/schema/event_mixin.go` (`owner_id`) · `ent/schema/snapshot.go`
- **Spec:** [12-saas.md §3.1](12-saas.md)
- **Aka / don't call it:** NOT the family **owner** role — see Homonyms.

### Owner guard
Fail-closed enforcement registered in `store.Open`: an ent query interceptor adds the `owner_id` predicate from the context; a mutation hook stamps/validates it. A query on an owner-scoped type with no owner in context **errors** instead of leaking. Raw SQL bypasses it — keep parameterizing the owner there.
- **Code:** `internal/store/ownerguard.go` · `internal/store/ownerctx.go` · `internal/store/owner_scope_test.go`
- **Spec:** [12-saas.md §11](12-saas.md)

### Query opts
Common filter/paging struct for event queries: limit, after/before sequence, from/to time.
- **Code:** `store.QueryOpts`
- **Spec:** [02-persistence.md §6 "QueryOpts"](02-persistence.md)

### Session event
`action = start | end` rows sharing a session ID. Start carries the serialized plan and (for quest expeditions) `quest_uid` + as-of-play `quest_name`; end carries totals and duration. `QuerySessionSummaries` joins the pair for history/activity.
- **Code:** `ent/schema/session_event.go` · `store.SessionEventData` · `store.SessionSummaryRecord`
- **Spec:** [06-session.md §6.1](06-session.md) · [15-quests.md "Completion visibility"](15-quests.md)

### Snapshot
Point-in-time JSON of full learner state (mastery, spaced-rep, learner profile, gem counts; legacy `tier_progress`/`mastered_set` kept for migration) saved at session end; the last 10 are kept. Sessions load from the latest snapshot — which is why two concurrent sessions clobber each other.
- **Code:** `store.Snapshot` · `store.SnapshotData` · `ent/schema/snapshot.go` · `mastery.MigrateSnapshot`
- **Spec:** [02-persistence.md §5](02-persistence.md) · [07-mastery.md §7](07-mastery.md)

### Snapshot repo
Save / Latest / UpdateLearnerProfile (in-place profile refresh, no new row) / Prune, scoped to one owner.
- **Code:** `store.SnapshotRepo` · `store.Store.SnapshotRepoFor`
- **Spec:** [02-persistence.md §6](02-persistence.md)

### Store
The one database handle: `store.Open(dsn)` picks SQLite (file path; `SetMaxOpenConns(1)`, WAL pragmas) or PostgreSQL (`postgres://`), runs ent auto-migrate, registers the owner guard and the sequence counter. Tests use `file::memory:?cache=shared`.
- **Code:** `store.Store` · `store.Open` · `store.IsPostgresDSN`
- **Spec:** [02-persistence.md §2–3](02-persistence.md)

---

## Family & identity

`internal/saas/{family,auth,authz,activity,server}`. Parents are accounts;
children are profiles with device tokens; every permission decision is in
`authz`.

### Account
A parent identity, auto-provisioned on the first authenticated request (no signup endpoint). Keyed on (`auth_provider`, `auth_subject` = the JWT `sub`), never on email; email and display name refresh from each token.
- **Code:** `ent/schema/account.go` · `family.Service.EnsureAccount`
- **Spec:** [12-saas.md §4.1](12-saas.md) · [17-auth-providers.md §3.3](17-auth-providers.md)

### Activity timeline
Parent-facing read model over a child's event streams: expedition items (with plan category "why", quest attribution), mastery milestones, lessons; cursor-paged by global sequence; filterable by kind, date, quest. Writes nothing.
- **Code:** `activity.Reader.Timeline` · `activity.TimelineItem` · `GET /api/v1/children/{id}/activity`
- **Spec:** [12-saas.md "Activity timeline"](12-saas.md)

### Archive (child)
Soft-delete of a child profile; revokes all its device tokens and hides it from quest targeting. Data stays owner-scoped under the UID.
- **Code:** `family.Service.UpdateChild` (`UpdateChildOpts.Archived`) · `family.ErrArchived`
- **Spec:** [docs/personas.md §1](../docs/personas.md)

### Auth adapter
The SPA's door to the deployment's auth provider: `getToken / user / onChange / signOut / SignIn`, one lazily-loaded adapter per provider (`supabase`, `test`), chosen from the boot config. Parent API calls take its `TokenGetter`. Kid and public pages never load one.
- **Code:** `web/src/auth/index.ts` (`getAuth`, `AuthAdapter`, `signOut`) · `web/src/auth/supabase.ts` · `web/src/auth/test.ts`
- **Spec:** [17-auth-providers.md §5](17-auth-providers.md)

### Auth preset / provider
The deployment's one hosted auth provider (`MATHIZ_AUTH_PROVIDER`: `supabase`, or `test` for dev/E2E) and the preset that holds all vendor knowledge (issuer, JWKS URL or HS256 secret, audience or authorized parties, claim mapping) for the one generic verifier. A key outage is `ErrUnavailable` (503), never a 401. Switching provider is an explicit offline `mathiz auth remap`; serve refuses to start while accounts belong to another provider. Replaces the Supabase-only verifier.
- **Code:** `auth.JWTVerifier` · `auth.Preset` · `auth.NewVerifier` · `cmd/auth.go`
- **Spec:** [17-auth-providers.md §3, §6](17-auth-providers.md)

### Authz (Checker)
All permission decisions. Parent checks resolve the membership role in the space (`CanManageSpace/Child/Invite/Device/Quest`; owner-only `CanManageBilling/Parents/ParentInvite`); child checks (`CanLearnAs`, `CanPlayQuest`) pin a device to its own profile. Every denial is `ErrDenied`, which the API maps to **404**, never 403.
- **Code:** `authz.Checker` · `authz.ErrDenied` · `internal/saas/server/middleware.go` (`writeServiceError`)
- **Spec:** [12-saas.md §5, "Co-parents"](12-saas.md)

### Boot config
Public `GET /api/v1/config` the SPA fetches at startup: `auth{provider, url, publicKey}` (the provider's public settings), and PostHog key + `/relay` host only when analytics is configured.
- **Code:** `web/src/api.ts` (`BootConfig`) · `internal/saas/server/api.go`
- **Spec:** [12-saas.md §7](12-saas.md) · [16-analytics.md](16-analytics.md) · [17-auth-providers.md §3.6](17-auth-providers.md)

### Child profile
A learner inside a family space: name, grade 2–5, optional bcrypt PIN hash, archived flag. Its UID **is** the learning-data `owner_id`. No email, no auth-provider account, ever.
- **Code:** `ent/schema/child_profile.go` · `family.Service.AddChild`
- **Spec:** [12-saas.md §3.2](12-saas.md)
- **Aka / don't call it:** "child", "kid", "learner", "explorer" (dashboard copy). NOT a learner profile.

### Child stats
Parent-facing per-child progress: mastery tally (mastered/learning/rusty from the latest snapshot), per-skill states, strand bars, recent sessions, gems, learner profile. Compact `summary` on child cards; full `stats` endpoint.
- **Code:** `internal/saas/server/stats.go` · `GET /api/v1/children/{id}/stats`
- **Spec:** [12-saas.md §7](12-saas.md)

### Co-parent
A second parent account in the family with role `parent`: everything except billing and parent management. Joins by accepting a parent invite. Kid surfaces are role-agnostic.
- **Code:** `family.RoleParent` · `family.Service.AcceptInvite`
- **Spec:** [12-saas.md "Co-parents"](12-saas.md)

### Cross-tenant denial
Any request touching an object outside the caller's family returns **404 "not found"** — never 403 — so existence isn't confirmed. Applies to quests on the map too (`ErrQuestUnavailable`).
- **Code:** `internal/saas/server/middleware.go` (`writeServiceError`) · `internal/saas/server/game_api.go` (`writeGameError`)
- **Spec:** [12-saas.md §5](12-saas.md)

### Curriculum (public)
Unauthenticated, cached skill catalog — islands (strands) → skills (id, name, grade, prereqs, keywords) — for the marketing pages, the dashboard curriculum browser and quest skill tagging.
- **Code:** `internal/saas/server/curriculum_api.go` · `GET /api/v1/curriculum` · `web/src/api.ts` (`CurriculumInfo`)
- **Spec:** [12-saas.md §7](12-saas.md)

### Device token
The child's bearer credential: `mzd_` + 32 random bytes, shown once, stored client-side (`localStorage` key `mathiz.deviceToken`), only its SHA-256 kept server-side; revocable, `last_used_at` tracked. Minted by redeeming a join code.
- **Code:** `ent/schema/device_token.go` · `family.TokenPrefix` · `family.Service.ResolveDeviceToken` · `web/src/api.ts` (`deviceToken`)
- **Spec:** [12-saas.md §3.2, §4.2](12-saas.md)

### Family member / role
Membership row linking an account to one family space with role `owner | parent`. One family per account (v1). Owners get a member row lazily on `/me`.
- **Code:** `ent/schema/family_member.go` · `family.RoleOwner` / `RoleParent` · `family.Service.MemberRole`
- **Spec:** [12-saas.md "Co-parents"](12-saas.md)

### Family space
The tenant: one per owning account, holds child profiles, join codes, quests, the credit ledger and billing state. Created free with 30 starter credits.
- **Code:** `ent/schema/family_space.go` · `family.Service.CreateSpace`
- **Spec:** [12-saas.md §3.2](12-saas.md) · [14-monetisation.md §1](14-monetisation.md)
- **Aka / don't call it:** "family", "space", `familyId` in JSON.

### Identity (verified)
What the JWT verifier returns for a parent: provider, subject (`sub`), email, email-verified flag, display name. `withParent` refuses an unverified email (invites match on email); `EnsureAccount` turns the rest into an Account.
- **Code:** `auth.Identity` · `auth.JWTVerifier.Verify` · `family.Identity`
- **Spec:** [12-saas.md §4.1](12-saas.md) · [17-auth-providers.md §3.1](17-auth-providers.md)

### Join code (Invite)
Family-scoped, human-friendly code (`TIGER-4207`) a child enters to get a device token. Stored plaintext (low privilege), expires (default 7 days, parent-chosen up to 90), revocable. The ent type is `Invite`; the UI says "join code".
- **Code:** `ent/schema/invite.go` · `family.Service.CreateInvite` · `family.DefaultInviteTTL` / `MaxInviteTTL`
- **Spec:** [12-saas.md §3.2](12-saas.md)
- **Aka / don't call it:** NOT a parent invite — see Homonyms.

### Join flow
Two unauthenticated calls: **preview** (code → space name + profiles with `has_pin`) then **redeem** (code + profile + PIN + device label → device token). Rate-limited per IP.
- **Code:** `family.Service.PreviewJoin` / `RedeemInvite` · `POST /api/v1/join/preview` · `POST /api/v1/join/redeem`
- **Spec:** [12-saas.md §4.2](12-saas.md)

### Owner (family role)
The account that created the space (`owner_account_id`, role `owner`): the only member who can buy credits, manage billing and invite/remove parents; can never be removed.
- **Code:** `family.RoleOwner` · `authz.Checker.CanManageBilling` / `CanManageParents`
- **Spec:** [12-saas.md "Co-parents"](12-saas.md)

### Parent (persona)
Hosted-mode adult user: signs in through the deployment's auth provider, runs the family space, reads progress, authors quests, buys credits (owner). The buyer, not the user, of credits.
- **Code:** `authz.KindParent` · `internal/saas/server/middleware.go` (`withParent`)
- **Spec:** [docs/personas.md §1](../docs/personas.md)

### Parent invite
Owner-recorded email invitation for a co-parent (`pending | accepted | revoked`). No email is sent; the invitee signs in normally and `/me` surfaces a matching pending invite to accept.
- **Code:** `ent/schema/parent_invite.go` · `family.Service.InviteParent` · `family.ParentInvitePending`
- **Spec:** [12-saas.md "Co-parents"](12-saas.md)

### PIN
Optional 4–6 digit code on a child profile, bcrypt-hashed, checked only at join-code redemption so siblings can't pick each other's profile. Not meant to resist adults; nudged (never forced) when a family has 2+ kids.
- **Code:** `ent/schema/child_profile.go` (`pin_hash`) · `family.ErrBadPIN` / `ErrPINMismatch`
- **Spec:** [12-saas.md §3.2](12-saas.md)

### Principal
The verified caller handed to authz: `Kind` parent (with `AccountID`) or child (with `ChildProfileID` + `FamilySpaceID` from the device token). Built only by the auth middlewares.
- **Code:** `authz.Principal` · `authz.ChildPrincipal`
- **Spec:** [12-saas.md §5](12-saas.md)

### Session detail
Per-expedition drill-down for parents: every answer in question order plus hint count. Not a timeline item — behind an expand.
- **Code:** `activity.Reader.SessionDetail` · `GET /api/v1/children/{id}/activity/sessions/{sessionId}`
- **Spec:** [12-saas.md "Activity timeline"](12-saas.md)

---

## Game (treasure map)

`internal/saas/game` + `web/src/{game.ts,pages/Play.tsx}`. The skill graph
rendered as islands; the engine's vocabulary is translated to the kid's in
`game/views.go` and nowhere else.

### Captain
Kid-facing word with **two** meanings in `Play.tsx`: the child ("Captain {name}" in the header) **and** the quest-authoring parent ("⭐ The Captain left you a quest", "the Captain keeps the answer sealed"). Pick one deliberately when writing copy.
- **Code:** `web/src/pages/Play.tsx`
- **Spec:** [15-quests.md §5](15-quests.md)

### Chest / treasure
A mastered skill: open chest, spot state `treasure`, label "Treasure secured!". Mastery mid-expedition = "Treasure chest opened!" plus fog lifting on dependents.
- **Code:** `game.SpotView.State` (`"treasure"`) · `web/src/pages/Play.tsx` (`SPOT_LABEL`)
- **Spec:** [13-treasure-map.md §2](13-treasure-map.md)

### Clue
Kid-facing word for a **question** ("Next clue →") and, as "🗺️ Show me a clue", for a **hint**. Internally these are `Question` and `Hint`.
- **Code:** `web/src/pages/Play.tsx`
- **Spec:** [13-treasure-map.md §4](13-treasure-map.md)

### Dig spot
A skill on the map. Tapping an unlocked spot starts an expedition. Carries name, description, grade, prerequisites, state, tier progress, review-due flag.
- **Code:** `game.SpotView` · `web/src/game.ts` (`Spot`)
- **Spec:** [13-treasure-map.md §2](13-treasure-map.md)

### Expedition
The game's session: 5 AI-generated questions on one tapped skill (or up to 5 unsolved quest questions), one live per child, idle-expired after 30 min, replaced on a new start (an untouched same-skill start is reused, not re-charged). Costs 1 credit at start. Ends on 5 answers, mastery, or early exit.
- **Code:** `game.Manager.Start` · `game.QuestionsPerExpedition` · `game.ExpeditionView`
- **Spec:** [13-treasure-map.md §2 "Expeditions"](13-treasure-map.md) · [14-monetisation.md §3](14-monetisation.md)

### Fog (locked)
A spot whose prerequisites aren't mastered: `locked`, label "Hidden in the fog". Starting one returns `ErrLocked` (409).
- **Code:** `game.ErrLocked` · `skillgraph.IsUnlocked`
- **Spec:** [13-treasure-map.md §2](13-treasure-map.md)
- **Aka / don't call it:** NOT the foggy map — see Homonyms.

### Foggy map (generation retry)
Kid-facing screen for a question that couldn't be generated: "The map is foggy… 🌫️" with **Try again**, on the same expedition at no cost. The third failure in a row ends the expedition (circuit breaker) with the normal summary.
- **Code:** `game.ErrGeneration` (code `generation_failed`) · `game.ErrExpeditionOver` · `web/src/pages/Play.tsx` (`'fog'` phase)
- **Spec:** [19-question-latency.md §3.3](19-question-latency.md)

### Game manager
Server-side owner of all live expeditions; wires the engine per child (toolset, mastery, scheduler, gems), enforces the play slot and the charge hook at `Start`, and translates results into kid-safe views and errors.
- **Code:** `game.Manager` · `game.Config` · `internal/saas/server/game_api.go`
- **Spec:** [13-treasure-map.md §3](13-treasure-map.md)

### Gem (kid labels)
The web renames gem types: `mastery` → "Mastery gems 🏆", `streak` → "Streak gems 🔥", `session` → "Expedition gems ⛵", `recovery` → "Comeback gems 💪", `retention` → "Keeper gems 🛡️". IDs unchanged; use these labels on child surfaces.
- **Code:** `web/src/pages/Play.tsx` (`GEM_META`) · `game.GemsView`
- **Spec:** [13-treasure-map.md §4](13-treasure-map.md)

### Guide 🧭
The kid-facing persona of the micro-lesson ("The guide has a tip for you", "The guide is drawing a picture for you…"). Internally `lessons.Service`.
- **Code:** `web/src/pages/Play.tsx` · `game.Manager.Lesson`
- **Spec:** [13-treasure-map.md §4](13-treasure-map.md)

### Guide's notebook
Every past micro-lesson, read back from lesson events (last 50), grouped by island client-side.
- **Code:** `game.Manager.Notebook` · `game.NotebookTipView` · `GET /api/v1/game/notebook`
- **Spec:** [13-treasure-map.md §3–4](13-treasure-map.md)

### Island
A strand rendered on the map; ID = strand key, name = `StrandDisplayName`. Also the grouping word in the curriculum API and dashboard.
- **Code:** `game.IslandView` · `web/src/game.ts` (`Island`)
- **Spec:** [13-treasure-map.md §2](13-treasure-map.md)

### Kid surface
`/join` and `/play` (and the game API): role-agnostic, device-token authed, **never** shows prices/balances/plans, never identifies the child in analytics (memory persistence, family group key only), never loads an auth adapter.
- **Code:** `internal/saas/server/middleware.go` (`withChild`) · `web/src/analytics.ts` (`AnalyticsSurface = 'child'`)
- **Spec:** [14-monetisation.md §1](14-monetisation.md) · [16-analytics.md](16-analytics.md)

### Map (MapView)
Full map state for a child: islands → spots, gem counts, active quests. **Side-effect-free**: services are built with nil event repos so the decay check cannot write from a render.
- **Code:** `game.Manager.Map` · `game.MapView` · `GET /api/v1/game/map`
- **Spec:** [13-treasure-map.md §3](13-treasure-map.md)

### Map reveal
After a mastery transition, the newly unlocked dependents returned on the answer (`unlockedSkillIds`) so the client can lift fog ("new paths revealed!").
- **Code:** `game.AnswerResultView` (`UnlockedSkillIDs`) · `skillgraph.Dependents`
- **Spec:** [13-treasure-map.md §2](13-treasure-map.md)

### Play slot
Cross-surface guarantee of ONE live session per child, enforced by a shared registry keyed by child UID; a second start returns `ErrElsewhere` ("you're already playing on another screen"). Released only after the final snapshot save. Any future play surface must acquire from the same registry.
- **Code:** `playslot.Registry.Acquire` · `playslot.ErrBusy` · `game.ErrElsewhere`
- **Spec:** [13-treasure-map.md §2](13-treasure-map.md) (CLAUDE.md invariant)
- **Aka / don't call it:** NOT a plan slot — see Homonyms.

### Prove-tier countdown
Advisory shrinking bar on Prove-tier questions (`timeLimitSecs`); answers are always accepted — speed already feeds fluency server-side.
- **Code:** `game.QuestionView.TimeLimitSecs` · `web/src/pages/Play.tsx`
- **Spec:** [13-treasure-map.md §4](13-treasure-map.md)

### Ship needs to rest
The only thing a kid sees at zero balance: "The ship needs to rest! ⛵ Ask your grown-up". Triggered by HTTP 402 `out_of_credits` from expedition start. No prices, plans or upgrade prompts on any child surface, ever.
- **Code:** `game.ErrNoCredits` · `internal/saas/server/game_api.go` (`writeGameError`) · `web/src/pages/Play.tsx`
- **Spec:** [14-monetisation.md §1 "Buyer ≠ user"](14-monetisation.md)

### Sinking
Spot state for a mastered skill that is review-due or rusty: "Treasure sinking — rescue it!" (`reviewDue` flag). Pulls the kid back for spaced-rep review.
- **Code:** `game.SpotView` (`"sinking"`, `ReviewDue`) · `spacedrep.Scheduler.DueSkills`
- **Spec:** [13-treasure-map.md §2](13-treasure-map.md)

### Spot state
Kid projection of skill state: `locked | ready | digging | proving | treasure | sinking` (fog / glowing X / learn ring / prove ring / open chest / sparkle). Mapped from mastery state + unlock + due in `map.go`; the SPA type mirrors it.
- **Code:** `game.SpotView.State` · `web/src/game.ts` (`SpotState`)
- **Spec:** [13-treasure-map.md §2](13-treasure-map.md)

### Treasure map
The kid game as a whole (`/play`): hand-rolled SVG + CSS, no game engine. "The map is the pedagogy, visualized" — it replaced the browser terminal (termbridge) on 2026-07-19.
- **Code:** `web/src/pages/Play.tsx` · `internal/saas/game`
- **Spec:** [13-treasure-map.md](13-treasure-map.md)

### Views (game)
JSON view types that are the single translation layer from engine vocabulary (tier, mastery state, category) to the game's (digging, treasure, sinking). Kid-facing views never carry monetisation data.
- **Code:** `internal/saas/game/views.go`
- **Spec:** [13-treasure-map.md §3](13-treasure-map.md)

---

## Quests

`internal/saas/quests` + `ent/schema/quest*.go` + `web/src/pages/dashboard/Quest*.tsx`.
Parent-authored one-off question sets played through the normal expedition.

### Math-recompute warning
On manual save the answer is checked with `MathCheckValidator`; a mismatch returns a warning the parent must acknowledge (a wrong answer key poisons kid trust).
- **Code:** `quests.QuestionResult` · `quests.Service.AddQuestion`
- **Spec:** [15-quests.md §3](15-quests.md)

### Quest
A family-scoped, parent-authored set of questions with name, optional emoji, optional skill tag, target child (`""` = all), status, `created_by`. A set, not a graph — no branching (v1 hard rule). Kid-facing: "a message in a bottle" / "⭐ The Captain left you a quest".
- **Code:** `ent/schema/quest.go` · `quests.Service` · `quests.QuestInput`
- **Spec:** [15-quests.md §2](15-quests.md)

### Quest attribution
Quest expeditions denormalize `quest_uid` + as-of-play `quest_name` onto the session start event so the activity feed attributes them even after quest rename/deletion. Legacy fallback: untagged quests plan the synthetic skill ID `quest:<uid>`.
- **Code:** `store.SessionEventData.QuestUID/QuestName` · `internal/saas/activity/activity.go` (`questSkillPrefix`)
- **Spec:** [15-quests.md "Completion visibility & attribution"](15-quests.md)

### Quest expedition
An expedition over up to 5 not-yet-solved quest questions in authored order, chunked until the quest is exhausted; same gems/streaks/hints/lesson/1-credit charge/play slot. Tagged quests advance mastery; untagged run with nil mastery/spaced-rep repos.
- **Code:** `game.Manager.StartQuest` · `game.QuestPlay` · `POST /api/v1/game/quests/{id}/expeditions`
- **Spec:** [15-quests.md §2 "Play"](15-quests.md)

### Quest generation (AI)
Parent brief + count (≤ 20) → LLM batch through problemgen-style validation → **draft** questions for review. Idempotent via a client key (retry returns the saved batch); debits ⌈count/5⌉ credits with source `questgen:<questUID>:<clientKey>` only after success; 402 on empty wallet. Purpose `quest-gen`, logged to the family LLM stream.
- **Code:** `quests.Service.Generate` · `quests.MaxGenerateCount` · `quests.GenerateCostDivisor` · `llm.PurposeQuestGen`
- **Spec:** [15-quests.md §3](15-quests.md)

### Quest map item
The kid's quest card: id, name, emoji, total, correct, done. Completed quests collapse client-side into one "🏆 N quests completed" trophy row; archiving removes them from the payload.
- **Code:** `game.QuestMapItem` · `quests.Service.ActiveQuests` · `web/src/pages/Play.tsx` (`QuestTrophies`)
- **Spec:** [15-quests.md §5](15-quests.md)

### Quest progress
Control-plane rows (quest, child, question, correct) — deliberately **not** event-sourced to avoid widening `EventRepo`. Backs per-child completion chips (`✓ done / 2-of-5 / not started`) and which questions remain.
- **Code:** `ent/schema/quest_progress.go` · `quests.Service.RecordAnswer` / `ProgressFor`
- **Spec:** [15-quests.md §2 "Progress"](15-quests.md)

### Quest question
A fixed, parent-authored (or AI-drafted) question: position, text, answer, answer type, format, choices, hint, explanation, `client_key`. Not shuffled. Repeats until answered correctly.
- **Code:** `ent/schema/quest_question.go` · `quests.QuestionInput` · `game.QuestPlayQuestion`
- **Spec:** [15-quests.md §2](15-quests.md)

### Quest status
`draft | active | archived`. Publish (needs ≥ 1 question) flips draft → active. Dashboard labels: Draft / **"On the map"** / Archived.
- **Code:** `quests.StatusDraft` / `StatusActive` / `StatusArchived` · `web/src/pages/dashboard/questStatus.ts`
- **Spec:** [15-quests.md §2](15-quests.md)

### Sealed answer
On a **wrong** quest answer the response omits `correctAnswer`/`explanation` (the question will gate again); the client shows "the Captain keeps the answer sealed". Hints, diagnosis and the lesson still fire. Correct answers reveal the explanation. Map digs are unaffected.
- **Code:** `game.Manager.Answer` · `web/src/game.ts` (`AnswerResult` comment)
- **Spec:** [15-quests.md "Sealed answers"](15-quests.md)

### Tagged / untagged quest
`skill_id` set → answers feed mastery and spaced-rep for that skill (pushes the main map). Empty → engine runs with side-effect-free mastery/spaced-rep services; answer/session events still persist. Dashboard: "Untagged" = standalone practice.
- **Code:** `ent/schema/quest.go` (`skill_id`) · `game.QuestPlay.SkillID`
- **Spec:** [15-quests.md §2 "Mastery feed"](15-quests.md)

---

## Money

`internal/saas/{credits,billing}`. Our ledger is the entitlement source of
truth; the payment provider is a money pipe.

### Balance
Σ `remaining` of unexpired grants for a family space. Expiry is lazy (expired remainders are ignored, no background job).
- **Code:** `credits.Service.Balance`
- **Spec:** [14-monetisation.md §2](14-monetisation.md)

### Billing event (normalized)
The entire provider→ledger surface: `subscription_activated`, `subscription_renewed`, `subscription_canceled`, `topup_purchased`, each with a stable `EventID` that becomes the ledger source. Applied by `billing.Service.Apply`.
- **Code:** `billing.EventType` · `billing.Event` · `billing.Service.Apply`
- **Spec:** [14-monetisation.md §4](14-monetisation.md)

### Billing provider
Three-method payment adapter: `CreateCheckout`, `PortalURL`, `ParseWebhook` (signature-verified → normalized events). Shipped: `fake`, `stripe`. Do not widen; no metered billing, no Stripe credit grants.
- **Code:** `billing.Provider` · `billing.FakeProvider` · `billing.StripeProvider`
- **Spec:** [14-monetisation.md §4](14-monetisation.md)
- **Aka / don't call it:** NOT an LLM provider or auth provider — see Homonyms.

### Billing state
One row per family space mirroring the provider: provider, customer ID, subscription ID, plan ID, status (`none | active | past_due | canceled`), current period end. Written only from webhook events; read by the dashboard.
- **Code:** `ent/schema/billing_state.go` · `billing.Service.State`
- **Spec:** [14-monetisation.md §4](14-monetisation.md)

### Charge hook
Nil-able function the game manager calls at `Start` with the session ID; nil = free (local CLI, billing off, tests). `mathiz serve` wires it to `credits.Debit(…, 1, "session:"+sessionID)` only when a billing provider is configured. The **only** enforcement point.
- **Code:** `game.Config.Charge` · `cmd/serve.go`
- **Spec:** [14-monetisation.md §3](14-monetisation.md)

### Checkout
Provider-hosted purchase page for a plan (subscription) or top-up (one-time); success returns to `/dashboard/billing?billing=success`. Owner-only.
- **Code:** `billing.CheckoutParams` · `POST /api/v1/family/{id}/billing/checkout`
- **Spec:** [14-monetisation.md §4–5](14-monetisation.md)

### Credit
The unit parents buy and kids spend: **1 credit = 1 expedition** (5 questions, hints and lesson included). Quest AI generation costs ⌈count/5⌉. Parent UI calls them "expeditions".
- **Code:** `internal/saas/credits` · `billing.Plans` (doc comment)
- **Spec:** [14-monetisation.md §1](14-monetisation.md)

### Credit entry (ledger)
Event-sourced row on the family space: `kind` (`starter | plan | topup | debit`), signed `amount`, `remaining` (grants), unique `source`, optional `expires_at`. The ledger, not the provider, is the entitlement truth.
- **Code:** `ent/schema/credit_entry.go` · `credits.Service.Grant`
- **Spec:** [14-monetisation.md §2](14-monetisation.md)

### Debit
Negative entry that consumes grants FIFO by soonest expiry inside one transaction; insufficient balance writes nothing (`ErrInsufficient` → 402).
- **Code:** `credits.Service.Debit` · `credits.ErrInsufficient`
- **Spec:** [14-monetisation.md §2](14-monetisation.md)

### Expedition wallet
Parent-dashboard name for the balance and plan tiles ("⛵ 42 expeditions left"); shows a beta card when billing is off. Owner-only route.
- **Code:** `web/src/pages/dashboard/Billing.tsx` · `GET /api/v1/family/{id}/billing`
- **Spec:** [14-monetisation.md §5](14-monetisation.md)

### Fake provider
Dev/test billing provider: checkout "succeeds" immediately via `GET /api/v1/billing/fake/complete`, so the purchase→grant→play loop is clickable with no external service.
- **Code:** `billing.FakeProvider`
- **Spec:** [14-monetisation.md §4](14-monetisation.md)

### Grant
Positive ledger entry of kind `starter`, `plan` or `topup`. Plan grants are expired and re-granted on renewal (`RenewPlanCredits`); top-ups never expire.
- **Code:** `credits.KindStarter` / `KindPlan` / `KindTopup` · `credits.Service.RenewPlanCredits`
- **Spec:** [14-monetisation.md §1–2](14-monetisation.md)

### Out of credits
HTTP **402** with body `out_of_credits` from expedition start or quest generation. Kid surface renders "the ship needs to rest"; parent surface renders a wallet prompt.
- **Code:** `game.ErrNoCredits` · `internal/saas/server/game_api.go` · `internal/saas/server/quests_api.go`
- **Spec:** [14-monetisation.md §3](14-monetisation.md)

### Plan (billing)
A catalog item: monthly subscription (`explorer` 150, `voyager` 400, `armada` 1000 credits/mo) or one-time top-up (`topup-100`). Catalog lives in code; provider price IDs come from env.
- **Code:** `billing.Plan` · `billing.Plans` · `billing.PlanByID`
- **Spec:** [14-monetisation.md §1 "Default catalog"](14-monetisation.md)
- **Aka / don't call it:** NOT a session plan — see Homonyms.

### Portal
Provider-hosted self-service page (cancel, change plan, update card). Owner-only.
- **Code:** `billing.Provider.PortalURL` · `POST /api/v1/family/{id}/billing/portal`
- **Spec:** [14-monetisation.md §4](14-monetisation.md)

### Pricing (public)
Unauthenticated catalog endpoint served even with billing off: `billingEnabled`, starter credits, plans. Drives all "public beta — everything is free" messaging in the SPA.
- **Code:** `GET /api/v1/pricing` · `web/src/pages/Pricing.tsx`
- **Spec:** [14-monetisation.md §5 "Public pricing page"](14-monetisation.md)

### Source (idempotency key)
Unique string on every ledger entry making replays structural no-ops: `starter:<spaceID>`, `sub:<providerEventID>`, `topup:<providerEventID>`, `session:<sessionID>`, `questgen:<questUID>:<clientKey>`. Never a fresh UUID. The `ent/schema/credit_entry.go` comment still says `expedition:<sessionID>`; code uses `session:`.
- **Code:** `ent/schema/credit_entry.go` (`source`) · `cmd/serve.go` · `internal/saas/quests/generate.go`
- **Spec:** [14-monetisation.md §2](14-monetisation.md)

### Starter credits
30 free credits, expiring in 30 days, granted exactly once per family space (`starter:<spaceID>`).
- **Code:** `credits.StarterCredits` · `credits.StarterExpiry` · `credits.Service.EnsureStarterGrant`
- **Spec:** [14-monetisation.md §1](14-monetisation.md)

### Stripe provider
Real payments: Checkout Sessions (`subscription` / `payment`), hosted Customer Portal, signature-verified webhooks. Subscription credits flow only from `invoice.paid`; `checkout.session.completed` maps only top-ups.
- **Code:** `billing.StripeProvider` · `internal/saas/billing/stripe.go`
- **Spec:** [14-monetisation.md §4 "Provider decision"](14-monetisation.md)

### Top-up
One-time credit pack (`topup-100`); credits never expire. Realizes the "usage beyond plan" leg as prepaid, never metered.
- **Code:** `billing.Plan.TopupCredits` · `billing.EventTopupPurchased`
- **Spec:** [14-monetisation.md §1, §4](14-monetisation.md)

### Webhook
`POST /api/v1/billing/webhook`: provider → `ParseWebhook` → normalized events → ledger + billing state. Replays are no-ops via the source column.
- **Code:** `internal/saas/server/billing_api.go` · `billing.Service.Apply`
- **Spec:** [14-monetisation.md §5](14-monetisation.md)

---

## Platform/ops

Wiring, surfaces, logging, analytics, commands.

### Analytics (PostHog)
Operator opt-in product analytics. Parents are identified by account UID; **children are never identified** (memory persistence, family group key only). The browser talks only to the same-origin `/relay` reverse proxy. `web/src/analytics.ts` is the only file that may import `posthog-js` or name an event.
- **Code:** `web/src/analytics.ts` · `internal/saas/server/posthog_proxy.go`
- **Spec:** [16-analytics.md](16-analytics.md)

### Analytics surface
`public | parent | child` — picks PostHog options and identity rules per route group. Child = no identify, no pageviews, memory persistence.
- **Code:** `web/src/analytics.ts` (`AnalyticsSurface`)
- **Spec:** [16-analytics.md "Surface configs"](16-analytics.md)

### App options (CLI wiring)
The TUI's dependency graph: provider, repos, generator, diagnosis, lessons, compressor, gems, update channel. A new session dependency must reach both this and `game.Config`.
- **Code:** `app.Options` · `internal/app/app.go` · `cmd/run.go`
- **Spec:** [01-skeleton.md §4](01-skeleton.md)

### Canonical log line
ONE structured slog line per HTTP request (method, route pattern, status, dur_ms, bytes, ip, plus everything added to the request's `logctx` bag: principal/account/child UIDs, `err`, `err_detail`, panic). 5xx→Error, 4xx→Warn, API→Info, assets→Debug. Standalone slog records are for background goroutines only.
- **Code:** `internal/saas/server/reqlog.go` · `logctx.Add` / `logctx.Attrs`
- **Spec:** [12-saas.md §8 "Canonical request logging"](12-saas.md)

### Deployment modes
**Local CLI** (`mathiz`, `mathiz play`): one learner, SQLite, Bubble Tea TUI, owner `""`. **Hosted / serve** (`mathiz serve`): multi-tenant PostgreSQL, auth-provider parents, device-token kids, embedded SPA. Same engine, different presentation.
- **Code:** `cmd/run.go` · `cmd/serve.go`
- **Spec:** [docs/personas.md](../docs/personas.md) · [12-saas.md §1](12-saas.md)

### LLM usage auditing
`mathiz llm list|view|stats` over LLM request events: per-purpose/model token usage and estimated cost. Serve mode logs per child (and per family for quest-gen).
- **Code:** `cmd/llm.go` · `store.EventRepo.LLMUsageByPurpose` / `LLMUsageByModel`
- **Spec:** [docs/llm-usage-auditing.md](../docs/llm-usage-auditing.md)

### Operator (persona)
Whoever runs `mathiz serve`: configures DB, auth, LLM, billing, analytics, logging via `MATHIZ_*` env. Billing off = everything free.
- **Code:** `server.Config` · `server.ConfigFromEnv`
- **Spec:** [docs/personas.md §4](../docs/personas.md)

### Persona
The four documented user types — Parent, Child, Solo learner (CLI), Operator — and their supported flows. `docs/personas.md` is the source of truth for user-facing flows and must be updated with them.
- **Spec:** [docs/personas.md](../docs/personas.md)

### Router / Screen (TUI)
Bubble Tea v2 screen stack: `Screen` (Init/Update/View(width,height)/Title) pushed/popped/replaced via router messages. Screens: Welcome, Home, Session, Session Summary, Skill Map (+ detail), Gem Vault, History. Measure-then-render layout.
- **Code:** `screen.Screen` · `router.Router` · `internal/screens/`
- **Spec:** [01-skeleton.md §5](01-skeleton.md)

### Self-update
`mathiz update` and the async version check surfaced on the Home screen.
- **Code:** `internal/selfupdate` · `cmd/update.go` · `app.Options.UpdateCh`
- **Spec:** [docs/personas.md §3](../docs/personas.md)

### Serve (server)
`mathiz serve` wiring: `server.Deps` (store, family, verifier, game manager, credits, billing, quests, activity, web UI) → `server.Server` → `/api/v1/*` routes with `withParent` / `withChild` middlewares. Fails fast on missing DB/auth config.
- **Code:** `server.Deps` · `internal/saas/server/server.go` · `cmd/serve.go`
- **Spec:** [12-saas.md §2, §7](12-saas.md)

### Skill Map (TUI)
Terminal screen listing skills by strand with skill-state icons and review status; the CLI counterpart of the treasure map.
- **Code:** `internal/screens/skillmap`
- **Spec:** [03-skill-graph.md "Skill Map Screen"](03-skill-graph.md)

### Solo learner (persona)
Local CLI user with an LLM key: `mathiz`, `play`, `stats`, `llm`, `preview`, `skill`, `reset`, `update`; multiple learners via `--db`.
- **Code:** `cmd/`
- **Spec:** [docs/personas.md §3](../docs/personas.md)

### SSH server / termbridge (historical)
Two superseded kid transports: the Wish/SSH design (`ssh-server.md`, never the SaaS path) and the in-process browser terminal (`termbridge`, removed 2026-07-19). Design record only; do not revive.
- **Spec:** [ssh-server.md](ssh-server.md) · [12-saas.md §6](12-saas.md)

### Web UI (embedded SPA)
Vite + React in `web/`, built into `internal/saas/webui/dist` and served by the Go binary via `embed.FS`; fetches boot config at runtime so one build runs anywhere. Routes: `/`, `/pricing`, `/how-it-works`, `/demo`, `/legal`, `/login`, `/dashboard/*`, `/join`, `/play`.
- **Code:** `webui.Handler` · `web/src/App.tsx`
- **Spec:** [12-saas.md §9](12-saas.md)

---

## Homonyms & look-alikes

- **Plan** — *session plan* (`session.Plan`, the slots a session cycles) vs *billing plan* (`billing.Plan`, a purchasable subscription/top-up) vs *plan grant* (`credits.KindPlan`, credits from a subscription). Say "session plan" or "billing plan".
- **Session** — a *learning session* (`session.*`, one UUID, start/end events; the game calls it an expedition) vs a *parent's auth-provider session in the browser* (web only) vs *session gem* (`gems.GemSession`, awarded for completing one).
- **Slot** — *plan slot* (`session.PlanSlot`, a skill+tier in a plan) vs *play slot* (`playslot.Registry`, the one-live-session-per-child lock).
- **Invite** — the ent `Invite` is a kid **join code**; a `ParentInvite` is a co-parent email invitation. UI never says "invite" for join codes.
- **Tier** — *difficulty tier* (`skillgraph.Tier`: learn/prove) vs *gem rarity* (`gems.Rarity`) vs *billing plan* (never called a tier in code).
- **Provider** — *LLM provider* (`llm.Provider`) vs *billing provider* (`billing.Provider`) vs *auth provider* (`auth.Preset`, specs/17) vs `quests.ProviderFactory` (an LLM provider per family).
- **Token** — *device token* (`mzd_…`, kid credential) vs *auth-provider JWT / access token* (parent) vs *LLM tokens* (`llm.Usage`).
- **Profile** — *child profile* (`ent.ChildProfile`, the learner identity) vs *learner profile* (`lessons.LearnerProfile`, the LLM's summary of the learner) vs LLM purpose `profile`.
- **Owner** — *owner_id* (data-plane tenant key = child UID / `""` / `space:<id>`) vs *family owner* (`family.RoleOwner`, the paying parent) vs `FamilySpace.owner_account_id`.
- **Streak** — *in-session streak* (`SessionState.ConsecutiveCorrect`, streak gems) vs *per-skill fluency streak* (`FluencyMetrics.Streak`, consistency score).
- **State** — *mastery state* (`mastery.MasteryState`: new/learning/mastered/rusty) vs *skill display state* (`skillgraph.SkillState`: locked/available/learning/proving/mastered/rusty) vs *spot state* (`"digging"`, `"treasure"`, …) vs *billing state* (`ent.BillingState`) vs *session state* (`session.SessionState`).
- **Category** — *plan category* (`session.PlanCategory`: frontier/review/booster) vs *error category* (`diagnosis.ErrorCategory`: careless/speed-rush/misconception/unclassified).
- **Question** — *generated question* (`problemgen.Question`, never static) vs *quest question* (`ent.QuestQuestion`, fixed, parent-authored, not shuffled) vs *practice question* (`lessons.PracticeQuestion`, inside a lesson). Kid word for all three: "clue".
- **Fog** — a *locked spot* ("Hidden in the fog", `ErrLocked`) vs the *foggy map* retry screen ("The map is foggy… 🌫️", `ErrGeneration`).
- **Review** — *spaced-rep review* (an answer feeding `RecordReview`; plan category `review`) vs *parent review of AI-drafted quest questions* (draft → publish).
- **Explorer** — the `explorer` billing plan vs dashboard copy calling a child an "explorer" ("more than one explorer aboard"). Avoid the second use in new copy.
- **Captain** — the child (header) *and* the quest-authoring parent (quest card, sealed-answer copy). See Game → Captain.
- **Guide** — the micro-lesson persona (🧭) only; not the hint, not the AI tutor generally.
- **Expedition** — a game session (1 credit) vs the parent-UI unit of credits ("42 expeditions left") vs `session` gem's kid label "Expedition gems".
- **Diagnosis vs diagnostic** — *diagnosis* classifies a wrong answer (`internal/diagnosis`); *diagnostic placement* is the unbuilt entry quiz (`skillgraph.DiagnosticResult`).
- **Compression** — *session compression* (error context → summary) vs *profile generation*; both live on `lessons.Compressor` with different purposes (`session-compress`, `profile`).
- **Snapshot** — the learner-state `Snapshot` row vs "snapshot" used loosely for the pure-Go `SnapshotData` struct; there is no other snapshot concept (no DB snapshot).
- **Family** — the `FamilySpace` entity; `familyId` in JSON; the PostHog `family` group. One concept, three spellings.
- **Curriculum / skill graph / map / islands** — the same 56-skill DAG seen from the public API, the Go package, the kid, and the dashboard respectively.
