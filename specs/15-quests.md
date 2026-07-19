# Parent Quests — one-off practice, on the map

## 1. Why

Parents need to align Mathiz with what school is doing *this week*
(e.g. HCF/LCM revision) without waiting for curriculum changes. Quests are
parent-authored question sets that appear to the kid as a special spot on
the existing treasure map. **No login fork, no second mode** — a quest is
played through the exact same expedition overlay as any dig spot.

## 2. Model

- A **quest** belongs to a Family Space (control plane, like invites):
  name, optional emoji, optional `skill_id` tag (skillgraph), target
  `child_uid` (`""` = all children), status `draft | active | archived`.
- **Questions** are an ordered flat list under a quest: text, answer,
  answer_type, format (`numeric | multiple_choice`), optional choices,
  hint, explanation. NO prerequisites/branching — a quest is a set, not a
  graph (v1 hard rule; resist LMS creep).
- **Play**: active quests render as a special map element ("a message in a
  bottle"). Tap → expedition of up to 5 not-yet-correctly-answered
  questions, chunked until the quest is exhausted → completion
  celebration. Same gems/streaks/hints/session events, same 1-credit
  charge at expedition start, same playslot slot.
- **Mastery feed**: if `skill_id` is tagged, answers flow through the
  normal session engine services (mastery + spaced-rep advance — quest
  practice pushes the main map forward). Untagged quests run the engine
  with side-effect-free mastery/spaced-rep services (nil event repos, the
  Map-read trick) so graph state is untouched; answer/session events still
  persist to the child's stream.
- **Progress** is control-plane, not event-sourced: `quest_progress` rows
  (quest, child, question, correct) — avoids widening EventRepo (5-mock
  ripple).

## 3. Authoring

- Manual: parent adds/edits questions in the dashboard. At save the
  answer is checked with the pure-Go math recompute where computable;
  a mismatch returns a warning the parent must acknowledge (typo guard —
  a wrong answer key poisons kid trust).
- **AI generation**: parent writes a brief ("10 HCF word problems, grade
  5"); server generates via the configured LLM through problemgen-style
  validation, saves as draft questions FOR REVIEW — nothing reaches a kid
  unapproved. Publish flips draft → active.
- **Credits**: generation debits ceil(count/5) credits with source
  `questgen:<questID>:<clientKey>` (client-supplied idempotency key —
  retries of the same click never double-debit). Debit happens only after
  successful generation+validation. Manual authoring is free. Playing
  costs the normal 1 credit/expedition — one mental model.
- Kid surfaces never show any of this (Money invariants apply).

## 4. API

Parent (authz: owns the family; cross-tenant → 404):
| POST/GET `/api/v1/family/{id}/quests` | create / list |
| GET/PATCH/DELETE `/api/v1/quests/{id}` | detail (with questions) / rename-retarget-status / delete |
| POST `/api/v1/quests/{id}/questions` · PATCH/DELETE `/api/v1/quests/{id}/questions/{qid}` | manual authoring |
| POST `/api/v1/quests/{id}/generate` `{brief,count,clientKey}` | AI draft (402 on empty wallet) |
| POST `/api/v1/quests/{id}/publish` | draft → active (requires ≥1 question) |

Kid (device token): `GET /api/v1/game/map` gains `quests[]` (active,
targeted at this child, with progress); `POST /api/v1/game/quests/{id}/expeditions`
starts a quest expedition; all existing expedition endpoints then apply
unchanged.

## 5. Kid UX

A floating bottle/scroll card above the islands when quests exist:
"⭐ The Captain left you a quest: <name>" with a progress ring. Tap →
standard expedition overlay. Finishing every question → "Quest complete!"
celebration; the card shows a trophy afterwards. Zero new modes.

## 6. Testing

Service: CRUD + authz (cross-family 404), publish gating, generation
debit idempotency (same clientKey → one debit), validation warnings.
Game: quest expedition serves authored questions in order, skips
already-correct ones, tagged quest advances mastery, untagged leaves the
graph untouched, completion state. Owner scoping: progress rows are
family-scoped control plane (authz-guarded), events stay owner-scoped.
