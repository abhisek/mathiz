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

The account owner: signs up, runs the Family Space, follows their children's
progress. Authenticated by a **Supabase JWT**; an account is
auto-provisioned on first authenticated request.

| Flow | Surface | Backing |
|---|---|---|
| Land on the front door, pick the parent path | `/` (static landing, parent + kid CTAs) | no backend — routes to `/login` or `/join` |
| Sign in — email code (OTP) first; the emailed magic link also works; email+password behind a fallback link. Account auto-created on first sign-in | `/login` (SPA, supabase-js) | Supabase Auth (`signInWithOtp`/`verifyOtp`, password fallback); server verifies JWT locally (HS256 secret or JWKS) |
| Create / rename Family Space (one per account) | `/dashboard` | `POST/PATCH /api/v1/family` |
| Add child (name, grade 2–5, optional 4–6 digit PIN) | `/dashboard` | `POST /api/v1/family/{id}/children` |
| Edit / archive child (archiving revokes their devices) | `/dashboard` child card | `PATCH /api/v1/children/{id}` |
| Set / change a child's PIN any time; with 2+ kids and a PIN missing, the dashboard nudges (never forces) | `/dashboard` child card + tip banner | `PATCH /api/v1/children/{id}` (`pin`) |
| Mint / list / revoke join codes — parent picks expiry (7/30/90 days; default 7, server caps at 90) | `/dashboard` join codes panel | `POST/GET /api/v1/family/{id}/invites` (`ttlHours`), `DELETE /api/v1/invites/{id}` |
| See per-child progress: island bars, mastered/learning counts, gems, recent sessions | `/dashboard` child card | `GET /api/v1/family/{id}/children`, `GET /api/v1/children/{id}/stats` |
| Read the AI tutor's learner profile ("what the tutor has learned about X") | `/dashboard` child card | learner profile from latest snapshot |
| List / sign out child devices | `/dashboard` child card | `GET /api/v1/children/{id}/devices`, `DELETE /api/v1/devices/{id}` |
| Expedition wallet: balance, plans, subscribe / top-up, manage billing | `/dashboard` billing card | `GET/POST /api/v1/family/{id}/billing*` (only when a billing provider is configured; 30 free starter credits on space creation) |
| Author quests: one-off question sets ("HCF revision this week") for one child or all — manual authoring (free, with a math-recompute typo warning) or AI generation from a brief (debits ceil(count/5) credits, 402 on empty wallet); publish flips draft → active | `/dashboard` quests panel | `POST/GET /api/v1/family/{id}/quests`, `GET/PATCH/DELETE /api/v1/quests/{id}`, `POST/PATCH/DELETE .../questions[/{qid}]`, `POST .../generate`, `POST .../publish` — see [specs/15-quests.md](../specs/15-quests.md) |

Authorization: a parent can only ever see and manage the Family Space they
own; cross-tenant requests return 404 (`internal/saas/authz`).

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
| Terminal mode: the classic TUI streamed to the browser | `/terminal` | WebSocket `/api/v1/terminal` |
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
| Resource limits: concurrent sessions, idle timeout | `MATHIZ_MAX_SESSIONS`, `MATHIZ_SESSION_IDLE_MINUTES` |
| Reverse-proxy correctness (rate limiting by real client IP) | `MATHIZ_TRUST_PROXY=true` |
| Split SPA deployments | `MATHIZ_CORS_ORIGINS` |
| Monetisation on/off + provider (off = everything free; fake for dev; Stripe/Paddle planned) | `MATHIZ_BILLING_PROVIDER`, `MATHIZ_BILLING_PRICE_*` — see [specs/14-monetisation.md](../specs/14-monetisation.md) |
| Per-child LLM usage events (auditing/cost attribution) | logged into each child's event stream |

Full setup: [docs/saas.md](./saas.md) · env reference: [.env.example](../.env.example)

---

## Cross-persona guarantees

- **Tenant isolation**: every learning event and snapshot is owner-scoped to
  a child profile; parents see only their family; children see only
  themselves. Cross-tenant probes return 404.
- **Same engine everywhere**: mastery, spaced repetition, question
  generation, diagnosis, lessons, and gems are identical across the CLI,
  the browser terminal, and the treasure map — only the presentation differs.
- **AI-generated questions**: no persona ever sees a static question bank;
  every question is generated for that learner at ask time.
