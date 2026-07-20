# Personas & Supported Flows

The source of truth for who Mathiz serves and what each persona can do.
**Keep this current**: when a PR adds or changes a user-facing flow, update
the matching table here (the specs in `specs/` record the design; this doc
records what exists).

Mathiz has two deployment modes — the **local CLI** (single learner, one
SQLite file) and **hosted mode** (`mathiz serve`: multi-tenant, families,
browser) — and four personas across them.

---

## 1. Parent (hosted mode)

Runs the Family Space and follows the children's progress. Authenticated by
a **Supabase JWT**; an account is auto-provisioned on first authenticated
request. A family holds one **owner** plus any number of **co-parents**
(role `parent`): co-parents can do everything except billing and managing
parents — those stay with the owner. One family per account.

| Flow | Surface | Backing |
|---|---|---|
| Land on the front door, pick the parent path | `/` (static landing, parent + kid CTAs) | no backend — routes to `/login` or `/join` |
| Read indicative pricing before signing up — public, pre-auth; while no billing provider is configured (public beta) the page shows a "everything is free right now" banner instead of live purchase flows | `/pricing` (static SPA page, linked from the landing + legal footers) | `GET /api/v1/pricing` (public: `billingEnabled`, starter credits, plan catalog) |
| Read how the teaching engine works before (or after) signing up — fresh-generated questions, the mastery journey (learning → mastered → re-checks → rusty), what an expedition is, quests, and the full curriculum by island and grade | `/how-it-works` (public, pre-auth; linked from the landing footer, the pricing explainer, and the dashboard sidebar foot) | `GET /api/v1/curriculum` (public skill catalog; static fallback copy if unavailable) |
| Sign in — email code (OTP) first; the emailed magic link also works; email+password behind a fallback link. Account auto-created on first sign-in | `/login` (SPA, supabase-js) | Supabase Auth (`signInWithOtp`/`verifyOtp`, password fallback); server verifies JWT locally (HS256 secret or JWKS) |
| Create / rename Family Space (one per account) | `/dashboard` (Kids) | `POST/PATCH /api/v1/family` |
| Add child (name, grade 2–5, optional 4–6 digit PIN) | `/dashboard` (Kids) | `POST /api/v1/family/{id}/children` |
| Edit / archive child (archiving revokes their devices) | `/dashboard` (Kids) child card | `PATCH /api/v1/children/{id}` |
| Set / change a child's PIN any time; with 2+ kids and a PIN missing, the dashboard nudges (never forces) | `/dashboard` (Kids) child card + tip banner | `PATCH /api/v1/children/{id}` (`pin`) |
| Mint / list / revoke join codes — parent picks expiry (7/30/90 days; default 7, server caps at 90) | `/dashboard/family` join codes panel | `POST/GET /api/v1/family/{id}/invites` (`ttlHours`), `DELETE /api/v1/invites/{id}` |
| See per-child progress: island bars, mastered/learning counts, gems, recent sessions | `/dashboard` (Kids) child card | `GET /api/v1/family/{id}/children`, `GET /api/v1/children/{id}/stats` |
| Activity timeline per child: expeditions (expandable to every question, her answer, hints used; a "why" chip when the engine tagged the run — 🌱 New skill / 🔄 Review / ⭐ Confidence builder), mastery milestones (mastered / rusty), guide's lessons — filterable by kind and date range, "Load more" paging | `/dashboard/activity` | `GET /api/v1/children/{id}/activity` (cursor `before`, `kinds`, `from`/`to`), `GET /api/v1/children/{id}/activity/sessions/{sessionId}` |
| Read the AI tutor's learner profile ("what the tutor has learned about X") | `/dashboard` (Kids) child card | learner profile from latest snapshot |
| Browse the curriculum per child: every skill by island and grade with the child's state (Mastered 🏆 / Learning 🌱 / Rusty 🌧️ / Not started); each row offers "Create quest →", jumping into quest authoring with that skill preselected | `/dashboard/curriculum` (child chips like Activity) | `GET /api/v1/curriculum` + `GET /api/v1/children/{id}/stats` (merged client-side) |
| List / sign out child devices | `/dashboard` (Kids) child card | `GET /api/v1/children/{id}/devices`, `DELETE /api/v1/devices/{id}` |
| Invite a co-parent by email — no email is sent; the invitee sees an accept banner after signing in normally (**owner only**) | `/dashboard/family` parents panel | `POST /api/v1/family/{id}/parents` (`email`) |
| See the parent roster (members + pending invites) — any member | `/dashboard/family` parents panel | `GET /api/v1/family/{id}/parents` |
| Accept a pending co-parent invite matching the account email → join the family with role `parent` | `/dashboard` accept banner (shown on every dashboard route while the account has no family) | `GET /api/v1/me` (`pendingInvite`), `POST /api/v1/invites/parent/{id}/accept` |
| Revoke a pending co-parent invite / remove a co-parent — the owner can never be removed (**owner only**) | `/dashboard/family` parents panel | `DELETE /api/v1/parent-invites/{id}`, `DELETE /api/v1/family/{id}/parents/{accountId}` |
| Expedition wallet: balance, plans, subscribe / top-up, manage billing (**owner only** — the payment provider's customer is the payer's identity; a co-parent deep-linking here is bounced to Kids; checkout success returns to `/dashboard/billing?billing=success`) | `/dashboard/billing` | `GET/POST /api/v1/family/{id}/billing*` (only when a billing provider is configured; 30 free starter credits on space creation) |
| Author quests: one-off question sets ("HCF revision this week") for one child or all — manual authoring (free, with a math-recompute typo warning) or AI generation from a brief (debits ceil(count/5) credits, 402 on empty wallet); the skill tag is picked from a human-named curriculum dropdown (islands as groups, "name (grade N)", "Untagged" for standalone practice; free-text fallback if the catalog is unavailable); publish flips draft → active; the list keeps archived quests behind a toggle | `/dashboard/quests` (list + create; `?create=<skillId>` deep-opens the create modal preselected) + `/dashboard/quests/{id}` (full-page editor) | `POST/GET /api/v1/family/{id}/quests`, `GET/PATCH/DELETE /api/v1/quests/{id}`, `POST/PATCH/DELETE .../questions[/{qid}]`, `POST .../generate`, `POST .../publish` — see [specs/15-quests.md](../specs/15-quests.md) |

Authorization: a parent can only ever see and manage the Family Space they
are a **member** of (owner or co-parent); billing and parent management are
owner-only. Cross-tenant requests return 404 (`internal/saas/authz`). Kid
surfaces are role-agnostic.

## 2. Child (hosted mode)

The learner. **No email, no Supabase account** — a join code from a parent,
redeemed once per device (with the profile PIN if set), yields a revocable
device token stored in the browser.

| Flow | Surface | Backing |
|---|---|---|
| Front door: "I'm a kid → Enter my code" | `/` | static landing, routes to `/join` |
| Join: enter code → pick profile → PIN | `/join` | `POST /api/v1/join/preview`, `POST /api/v1/join/redeem` |
| Treasure map: 5 islands (strands), 56 dig spots (skills); fog on locked spots, glowing X on ready ones, progress rings while digging/proving, open chests when mastered, "sinking" sparkle when review is due | `/play` | `GET /api/v1/game/map` |
| Expedition: 5 AI-generated questions on a tapped spot (numeric or multiple choice), gem bursts, streak fire, prove-tier countdown | `/play` expedition overlay | `POST /api/v1/game/expeditions` (+ `/question`, `/answer`) |
| Hints after a wrong answer | expedition overlay | `POST .../hint` |
| The guide's micro-lesson after two wrong answers on a skill: explanation, worked example, practice question | expedition overlay | `POST .../lesson`, `POST .../lesson/answer` |
| Mastery celebration: chest opens, fog lifts on newly unlocked spots, expedition ends triumphantly | expedition overlay | mastery transition in answer response |
| Quest card above the islands ("⭐ The Captain left you a quest") with a progress ring; trophy when every question is solved | `/play` map | `quests[]` in `GET /api/v1/game/map` |
| Quest expedition: up to 5 not-yet-solved quest questions per run (chunked until done), same gems/streaks/hints/1-credit charge; tagged quests advance the main map, "Quest complete!" celebration at the end | `/play` expedition overlay | `POST /api/v1/game/quests/{id}/expeditions` (+ the standard expedition endpoints) |
| Guide's notebook: revisit every past tip, grouped by island | 🧭 button on `/play` | `GET /api/v1/game/notebook` |
| Gem vault: collection by gem type | 💎 button on `/play` | gem counts from map response |
| Switch player / leave device | header buttons | clears local device token |

Constraints kids can rely on: one live session per child (a second tab is
politely refused); a child can only ever act as themselves; the LLM key
never reaches the browser. **Kids never see the credit meter** — when the
family's expeditions run out they get "The ship needs to rest! ⛵ Ask your
grown-up", never prices or a paywall.

## 3. Solo learner (local CLI)

A kid (or grown-up) with a terminal and an LLM API key. Single-user, offline
from any server, one SQLite file. Unchanged by hosted mode.

| Flow | Command |
|---|---|
| Full TUI: welcome → home → adaptive session (planner-mixed skills) | `mathiz` |
| Jump straight into practice | `mathiz play` |
| Progress stats | `mathiz stats` |
| Skill map, gem vault, session history | in-TUI screens |
| LLM usage auditing (requests, tokens, costs) | `mathiz llm` |
| Skill preview without a database | `mathiz preview` |
| Reset progress | `mathiz reset` |
| Multiple learners on one machine | `--db ~/mathiz-alice.db` per learner |
| Self-update | `mathiz update` |

## 4. Operator (self-hosting hosted mode)

Whoever runs `mathiz serve` for their family or a community.

| Flow | Mechanism |
|---|---|
| Deploy: one self-contained binary (SPA embedded) | `make web && make mathiz`, run behind TLS |
| Database: any PostgreSQL (Supabase's included); schema auto-migrates | `MATHIZ_DATABASE_URL` |
| Auth config: Supabase project URL + anon key (+ legacy JWT secret) | `MATHIZ_SUPABASE_*` |
| LLM provider selection & keys (server-side only) | `MATHIZ_LLM_PROVIDER`, `MATHIZ_*_API_KEY`, `MATHIZ_OPENAI_BASE_URL` for compatible gateways |
| Resource limits: session idle timeout | `MATHIZ_SESSION_IDLE_MINUTES` |
| Reverse-proxy correctness (rate limiting by real client IP) | `MATHIZ_TRUST_PROXY=true` |
| Split SPA deployments | `MATHIZ_CORS_ORIGINS` |
| Monetisation on/off + provider (off = everything free; fake for dev; Stripe/Paddle planned) | `MATHIZ_BILLING_PROVIDER`, `MATHIZ_BILLING_PRICE_*` — see [specs/14-monetisation.md](../specs/14-monetisation.md) |
| Product analytics on/off (off = default; child identity is never sent — family-level only) | `MATHIZ_POSTHOG_API_KEY`, `MATHIZ_POSTHOG_HOST` — see [specs/16-analytics.md](../specs/16-analytics.md) |
| Structured logs: one canonical line per request on stdout, optional file tee (rotate externally) | `MATHIZ_LOG_FILE`, `MATHIZ_LOG_FORMAT` (`text`/`json`), `MATHIZ_LOG_LEVEL` |
| Per-child LLM usage events (auditing/cost attribution) | logged into each child's event stream |

Full setup: [docs/saas.md](./saas.md) · env reference: [.env.example](../.env.example)

---

## Cross-persona guarantees

- **Tenant isolation**: every learning event and snapshot is owner-scoped to
  a child profile; parents see only their family; children see only
  themselves. Cross-tenant probes return 404.
- **Same engine everywhere**: mastery, spaced repetition, question
  generation, diagnosis, lessons, and gems are identical across the CLI
  and the treasure map — only the presentation differs.
- **AI-generated questions**: no persona ever sees a static question bank;
  every question is generated for that learner at ask time.
