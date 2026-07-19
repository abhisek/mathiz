# CLAUDE.md

Guidance for AI coding agents working in this repository. Read the
**Invariants** section before changing persistence, SaaS, or game code —
those rules are load-bearing and violating them ships tenant-data leaks.

## Build, Test, Generate

```bash
CGO_ENABLED=0 go build ./...    # Build (CGO must stay disabled — no gcc in containers)
go test ./...                   # All tests
go test -run TestName ./internal/mastery   # Single test
go test -race ./internal/saas/...          # Race pass (required for saas/game changes)
make generate                   # Ent codegen — REQUIRED after any ent/schema change
make mathiz                     # Binary to bin/mathiz (embeds web SPA if built)
make web                        # Build the React SPA into internal/saas/webui/dist
make dev-db                     # Local PostgreSQL via docker compose
gofmt -w <files you touched>    # Format ONLY files you touch (see Quirks)
```

Project skills in `.claude/skills/` cover the recurring multi-step jobs:
`saas-e2e` (full-stack browser E2E in a sandbox, no real LLM or Supabase
needed), `frontend` (web/ patterns — read it BEFORE touching web/),
`analytics` (PostHog pattern + child-privacy rules — read it BEFORE any
tracking/telemetry change), `add-event-type`, `add-math-skill`,
`add-billing-provider`, `verify`,
`db-backup` (pg_dump backups via `scripts/db-backup.sh`).
If a PR changes a frontend pattern, update `.claude/skills/frontend/SKILL.md`
in the same PR (same rule as `docs/personas.md` for user-facing flows).

## Architecture

**Mathiz** teaches kids math (US grades ~2–5) by doing. Core differentiator:
questions are never static — the LLM generates each one from the child's
learner profile, recent errors, and mastery tier.

Two deployment modes, one engine:
- **Local CLI**: single learner, SQLite file, Bubble Tea TUI.
- **`mathiz serve` (SaaS)**: multi-tenant PostgreSQL, Supabase-authenticated
  parents, join-code-authenticated children, browser treasure-map game.

### Data Flow
- TUI: `cmd/` → `app.AppModel` → `router` (screen stack) → `screen.Screen` impls → domain packages
- Game: SPA (`web/`) → `/api/v1/game/*` → `internal/saas/game.Manager` → same domain packages

### Key Packages
- **`internal/session/`** — the pure session engine. `HandleAnswer` is the
  single entry point for grading: checking, mastery, streaks, gems,
  diagnosis, error context. Both the TUI screen and the game manager drive it.
- **`internal/skillgraph/`** — 56-skill Common Core DAG (5 strands, single
  root), package-level singleton initialized in `init()`, panics on
  validation failure.
- **`internal/mastery/`** — state machine (new → learning → mastered → rusty),
  learn/prove tiers, fluency scoring.
- **`internal/spacedrep/`** — review scheduling (1/3/7/14/30/60-day intervals), decay to rusty.
- **`internal/problemgen/`** — LLM question generation + validator chain
  (structural, answer format, pure-Go math recompute). `CheckAnswer` is the
  only answer comparator — never write another.
- **`internal/diagnosis/`**, **`internal/lessons/`**, **`internal/gems/`** —
  error classification (sync rules + async LLM), micro-lessons/learner
  profiles, rewards.
- **`internal/store/`** — event sourcing + snapshots. `EventRepo` /
  `SnapshotRepo` interfaces; SQLite or PostgreSQL behind one `store.Open(dsn)`.
- **`internal/saas/`** — `family` (accounts, family spaces, child profiles,
  join codes, device tokens), `authz` (ALL permission decisions), `auth`
  (Supabase JWT verify), `server` (REST API), `game` (treasure-map
  expeditions), `webui` (embedded SPA),
  `credits` (prepaid credit ledger — THE entitlement source of truth),
  `billing` (thin payment-provider abstraction; `fake` + `stripe` shipped),
  `quests` (parent-authored one-off question sets played on the map;
  control-plane, specs/15-quests.md).
- **`web/`** — Vite + React SPA. `npm run build` emits into
  `internal/saas/webui/dist` (gitignored except `.gitkeep`).
- **`cmd/run.go` / `cmd/serve.go`** — wiring. The CLI's dependency graph
  lives in `app.BuildOptions`; the game manager wires the same engine in
  `internal/saas/game`. A new session dependency must reach both.

Specs in `specs/` are the design record (spec-driven repo): `12-saas.md`
(SaaS layer), `13-treasure-map.md` (game), `14-monetisation.md` (credits,
plans, billing providers). Write/update a spec for any
significant feature. `docs/personas.md` is the source of truth for
user-facing flows — **update it when you add or change one**.
`docs/development.md` is the human dev-setup guide.

## Invariants (do not break)

**Tenant isolation**
- Every event/snapshot row is scoped by `owner_id` (child profile UID; `""`
  = local CLI). Every repo append must `SetOwnerID(r.owner)`; every repo
  query must filter `OwnerID(r.owner)`. Enforcement is central and
  fail-closed: a query interceptor + mutation hooks registered in
  `store.Open` (`internal/store/ownerguard.go`) stamp/filter `owner_id` from
  the context (repos wrap ctx via `withOwner` at method entry); an ent query
  or mutation on an owner-scoped table without an owner in ctx errors instead
  of leaking. Raw SQL bypasses the guard — keep parameterizing the owner
  there — and still add an owner-isolation test for any new query
  (see `internal/store/owner_scope_test.go`).
- Server-side code must only use `Store.EventRepoFor(childUID)` /
  `SnapshotRepoFor(childUID)` — never the unscoped `EventRepo()` /
  `SnapshotRepo()` (those are the local CLI's owner-`""` view).
- Cross-tenant API denials return **404, not 403** (don't confirm object
  existence). All permission decisions live in `internal/saas/authz` —
  handlers never inline policy.

**Store**
- Changing the `EventRepo` interface ripples to five hand-written test mocks:
  `internal/{gems,mastery,spacedrep,session,screens/session}/*_test.go`.
  Add a stub to each or nothing compiles.
- SQLite runs with `SetMaxOpenConns(1)` because pragmas are per-connection —
  don't "fix" the pool size. The `global_sequence` table provides cross-table
  event ordering; both its SQL and any raw SQL must stay portable across
  SQLite and PostgreSQL (`?` vs `$1` placeholders — see
  `eventRepo.ownerPlaceholder`).
- Ent schemas use plain string UID fields + indexes, **no ent edges** (house
  style). New event schemas embed `EventMixin`.

**Game**
- Map reads (`game.Manager.Map`) must stay side-effect-free: services built
  with **nil** event repos so the spaced-rep decay check can't persist from a
  render. Decay events persist only at expedition/session start.
- One live session per child is enforced ACROSS surfaces by the shared
  `internal/saas/playslot.Registry` (wired in cmd/serve.go into the game
  manager — today's only play surface) because concurrent sessions clobber
  each other's snapshot on save. Any future play surface MUST acquire a
  slot from the same registry, and release it only after the final
  snapshot save.

**Money (credits / billing)**
- Entitlements live in OUR ledger (`internal/saas/credits`), never in the
  payment provider. The provider is a money pipe only: 3 methods
  (`CreateCheckout`/`PortalURL`/`ParseWebhook`) emitting 4 normalized events.
  Do not widen that interface, and do not adopt provider-side entitlement
  features — decided: Stripe = subscriptions + one-time payments, **no
  metered billing, no Stripe credit grants** (specs/14-monetisation.md §4).
- Idempotency is structural: every grant/debit row carries a unique `source`
  (`starter:<spaceID>`, `sub:<providerEventID>`, `session:<sessionID>`).
  Webhook replays and request retries MUST be no-ops — never write a ledger
  entry without a deterministic source, and never invent a fresh UUID as one.
- Debits consume grants FIFO by soonest expiry inside one transaction;
  insufficient balance writes nothing and surfaces as HTTP 402
  `out_of_credits`.
- **The kid never sees the meter.** Zero balance → "The ship needs to
  rest! ⛵ Ask your grown-up" — no prices, balances, plans, or upgrade
  prompts on any child surface, ever. Purchase/balance UI is parent-dashboard
  only.
- `Charge` hooks are nil-able and nil means free: local CLI, self-hosters
  with billing off, and tests must keep working with no billing wired.
  Enforcement lives ONLY at the session-start chokepoint
  (`game.Manager.Start`) — a running expedition is never interrupted by
  the meter.

**Skill graph**
- Exactly **one root skill**. `skillgraph.Validate()` panics at init on
  cycles/dangling prereqs, so a bad seed kills every binary and test — run
  `go test ./internal/skillgraph/` first after editing the seed.
- In tests use `AllSkills()`, not `RootSkills()[:N]`.

**Generated code**
- Never hand-edit files under `ent/` outside `ent/schema/` — they're
  generated (`make generate`). A PreToolUse hook blocks this; if you hit it,
  edit the schema instead.

## Charm Libraries v2 API

These use v2 APIs that differ significantly from v1:
- `charm.land/bubbletea/v2`: `View()` returns `tea.View` (not string). Use
  `tea.NewView()` and `v.SetContent()`. AltScreen is a View field, not a
  program option. Programs run headless fine with `tea.WithInput`/`WithOutput`.
- `charm.land/bubbles/v2/textinput`: No `CursorStyle`/`TextStyle`/
  `PlaceholderStyle` fields. Use `Focus()`.
- `charm.land/lipgloss/v2`: pinned to a specific beta commit.

## Responsive Screen Layout (TUI)

Screens receive `(width, height int)` in `View()`. Use **measure-then-render**:
pre-render elements, measure with `lipgloss.Height()`, greedily include by
priority, downgrade variants when space is tight. `width < N` only for
horizontal/text concerns. Reference: `internal/screens/home/home.go`.

## Testing Patterns

- Table-driven tests with mock interface implementations;
  `tea.KeyPressMsg{Code: rune}` simulates keys (bubbletea v2).
- In-memory store for tests: `store.Open("file::memory:?cache=shared")` —
  the DB drops when the last connection closes, so `t.Cleanup(st.Close)`
  gives per-test isolation.
- Against persistent databases (Postgres), derive **unique owner IDs per
  test invocation** (see `testOwner` in `owner_scope_test.go`) — data
  survives across runs and `-count=2` reruns.
- `MATHIZ_TEST_DATABASE_URL=postgres://... go test ./internal/store/` runs
  the owner-isolation suite against real PostgreSQL (SQLite otherwise).
- LLM-dependent services are testable without a network:
  `llm.NewMockProvider(llm.MockResponse{Content: json})` for real services,
  or inject a fake `problemgen.Generator` (see `internal/saas/game/manager_test.go`).

## Environment Variables

Local CLI: `MATHIZ_LLM_PROVIDER` (`anthropic|openai|gemini|openrouter|mock`),
`MATHIZ_*_API_KEY` / `_MODEL`, `MATHIZ_OPENAI_BASE_URL` (any OpenAI-compatible
endpoint — this is how E2E tests stub the LLM), bare `GEMINI_API_KEY` /
`OPENAI_API_KEY` / `ANTHROPIC_API_KEY` / `OPENROUTER_API_KEY` also discovered.

`mathiz serve`: see `.env.example` — `MATHIZ_DATABASE_URL` (postgres),
`MATHIZ_SUPABASE_URL` / `_ANON_KEY` / `_JWT_SECRET`, `MATHIZ_SERVER_ADDR`,
`MATHIZ_SESSION_IDLE_MINUTES`, `MATHIZ_CORS_ORIGINS`,
`MATHIZ_TRUST_PROXY`.

Billing: `MATHIZ_BILLING_PROVIDER` (empty = billing off/everything free;
`fake` = dev provider; `stripe` = real payments — also needs
`MATHIZ_STRIPE_SECRET_KEY` + `MATHIZ_STRIPE_WEBHOOK_SECRET`),
`MATHIZ_PUBLIC_BASE_URL`
(checkout/webhook redirects), `MATHIZ_BILLING_PRICE_{EXPLORER,VOYAGER,ARMADA,TOPUP_100}`
(provider price-object IDs; unused by `fake`).

## Environment Quirks (sandboxes / CI)

- `internal/selfupdate` tests pass everywhere since the platform-asset fix
  (they used to 404 off-macOS because the test server hardcoded the Darwin
  asset name). If they fail for you, that's a real regression — investigate.
- The repo is historically not 100% gofmt-clean. Run `gofmt -w` on files you
  touched only; do not reformat untouched files (diff noise).
- No Docker daemon in some sandboxes: bootstrap Postgres directly with the
  `postgresql` package (`initdb`/`pg_ctl` as the `postgres` user) — the
  `saas-e2e` skill scripts this.
- Playwright + Chromium are preinstalled in Claude Code web sandboxes
  (`executablePath: '/opt/pw-browsers/chromium'`); Supabase is NOT needed for
  E2E — HS256 `MATHIZ_SUPABASE_JWT_SECRET` plus a self-minted JWT covers the
  parent API, and children only need a join code.
