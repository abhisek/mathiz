# Treasure Map — The Kid Game Experience

## 1. Overview

Kids don't want a terminal; they want a treasure hunt. This spec replaces the
browser-terminal kid surface (spec 12 §6) with a **treasure map game**: the
skill graph rendered as islands to explore, where solving AI-generated math
questions digs up treasure, collects gems, and lifts the fog on new territory.

Nothing about the learning engine changes. The planner, mastery state machine,
spaced repetition, LLM question generation, diagnosis, hints, learner profiles,
and gems all run exactly as in the terminal app — the game is a new
presentation driven over HTTP instead of a TUI event loop.

**The map is the pedagogy, visualized.** The 56-skill Common Core DAG *is* the
treasure map: prerequisite edges are paths, the five strands are five islands,
`skillgraph.IsUnlocked` is the fog of war, mastery opens treasure chests, and
spaced-repetition due-dates make conquered spots sparkle again ("treasure is
sinking — go back!"). Interleaving, which the terminal session got from the
5-slot plan, the game gets from the kid freely choosing among unlocked dig
spots, with review sparkles pulling them back.

## 2. Game model → engine mapping

| Game concept | Engine concept |
|---|---|
| Island | `skillgraph.Strand` (5 islands) |
| Dig spot | Skill node |
| Fog / locked spot | `!IsUnlocked(skill, mastered)` |
| Glowing X | Unlocked, state `new` — dig here! |
| Digging progress ring | Mastery tier progress (learn → prove) |
| Open treasure chest | `mastery.StateMastered` |
| Sparkling chest ⚡ | Due for review (`spacedrep.DueSkills`) or `rusty` |
| Expedition | A short question run on one chosen skill |
| Gems | Existing `gems.Service` awards (streak/mastery/recovery/retention/session) |
| Map reveal | Mastery transition → newly unlocked dependents |

### Expeditions

An **expedition** is 5 questions on one skill the kid taps (the terminal's
15-minute mixed plan doesn't fit tap-to-dig):

- The plan is a single slot for the chosen skill; category `review` when the
  skill is rusty or due for review, else `frontier`/`booster` semantics as in
  the planner. Tier always comes live from the mastery service.
- `session.HandleAnswer` runs unchanged per answer: checking
  (`problemgen.CheckAnswer`), mastery `RecordAnswer`, streak gems, diagnosis
  (sync rules + async LLM), error-context accumulation, hint availability,
  spaced-rep review recording, misconception penalties.
- Mastery can transition mid-expedition: learn→prove ("you found the vault —
  now prove it!"), prove→mastered (chest opens, dependents unlock, fog lifts).
- Expedition end (5 questions, mastery, or early exit): session end event,
  session gem when fully completed, snapshot save + async learner-profile
  compression — identical to the terminal flow.
- One active expedition per child (in-memory, replaced on new start, expired
  after 30 minutes idle).

## 3. API (child device-token auth, `/api/v1/game`)

| Method & path | Purpose |
|---|---|
| `GET  /game/map` | Full map state: islands, per-skill `{state, unlocked, dueReview, tierProgress}`, gem counts, child info |
| `GET  /game/notebook` | The guide's notebook: every past tip with full content, grouped by island client-side |
| `POST /game/expeditions {skillId}` | Start (replaces any active one) → expedition descriptor |
| `POST /game/expeditions/{id}/question` | Generate/fetch the current question |
| `POST /game/expeditions/{id}/answer {answer, timeMs}` | Grade → `{correct, correctAnswer, explanation, gem, mastery, unlockedSkillIds, streak, done}` |
| `POST /game/expeditions/{id}/hint` | Reveal the hint (records hint event) |
| `POST /game/expeditions/{id}/lesson` | Poll for the guide's micro-lesson (pending after 2 wrong answers on a skill) |
| `POST /game/expeditions/{id}/lesson/answer` | Grade the lesson's practice question (or record a skip) |
| `POST /game/expeditions/{id}/end` | Early exit → summary |

Map reads are side-effect-free (services built with nil event repos so the
decay check cannot write); decay transitions persist when an expedition starts,
exactly like terminal session start.

Authorization: same device-token middleware and `authz` checks as spec 12;
expedition ownership is validated on every sub-resource call.

## 4. Frontend (`/play`)

The kid route becomes the map (the terminal remains available at `/terminal`
for the curious):

- **Map screen**: SVG parchment sea with five islands. Dig spots laid out per
  island in grade columns; prerequisite paths drawn as dotted trails. States:
  fog (locked), glowing X (ready), progress ring (learning/proving), open
  chest (mastered), sparkle overlay (review due / rusty). Gem counter in the
  header with the child's name.
- **Expedition panel**: tapping a spot slides up a question card — big
  friendly text, numeric input or multiple-choice buttons, hint button when
  the engine offers one, gem burst on correct, gentle "try again" energy on
  wrong with the explanation after grading. Progress dots (1–5). Chest-opening
  celebration + "new paths revealed!" when mastery unlocks dependents.
- **The guide** 🧭: after a second wrong answer on a skill, the engine's AI
  micro-lesson surfaces as "the guide has a tip for you" — title, friendly
  explanation, worked example, and a try-it-yourself practice question
  (gradeable or skippable, persisted as lesson events). Lessons are
  best-effort: if generation is slow the hunt just continues.
- **Prove-tier countdown**: timed-in-spirit questions show a shrinking bar
  (advisory — answers are always accepted; speed already feeds the fluency
  score via server-side timing).
- **Gem vault**: the header gem counter opens the collection — counts by gem
  type (mastery 🏆, streak 🔥, expedition ⛵, comeback 💪, keeper 🛡️).
- **The guide's notebook**: every tip ever given is revisitable from the map
  header, grouped by island. Lesson events persist the full lesson content
  (explanation, worked example, practice with answer) to make this possible —
  the terminal app records the same fields, so local tips carry over when a
  learner moves to hosted mode.
- Written with hand-rolled SVG + CSS animation. No game engine dependency.

## 5. Testing

- Engine: expedition lifecycle against a fake `problemgen.Generator`
  (deterministic questions) — start → 5 answers → mastery transitions, gems,
  events, snapshot; wrong-answer path (hints, diagnosis, error context);
  review-category recording for rusty skills; expedition replacement and
  expiry; cross-child ownership denial.
- API: httptest flow with device token.
- Browser: Playwright drive of join → map → dig → answer → gem.
