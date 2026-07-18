# Development Guide

Zero to running locally — CLI mode, hosted (SaaS) mode, web development, and
tests. Companion docs: [personas & flows](./personas.md) ·
[SaaS architecture](./saas.md) · specs in [`specs/`](../specs/).

## Prerequisites

| Tool | Version | Needed for |
|---|---|---|
| Go | 1.25+ | everything |
| Node.js + npm | 20+ | the web SPA (hosted mode UI) |
| Docker | any recent | easiest local PostgreSQL (`make dev-db`) — optional, see below |
| An LLM API key | — | question generation (Gemini/OpenAI/Anthropic/OpenRouter); optional for a first look |

No gcc needed: everything builds with `CGO_ENABLED=0` (pure-Go SQLite and
Postgres drivers).

```bash
git clone https://github.com/abhisek/mathiz.git && cd mathiz
CGO_ENABLED=0 go build ./...        # sanity: compiles everything
```

## 0. One command: the self-contained Docker stack

If you have Docker and just want the whole hosted product running:

```bash
make dev-up        # builds the image (SPA + Go, multi-stage) and starts
                   # PostgreSQL + a stub LLM + mathiz serve on :8080
```

Open `http://localhost:8080/join` — kids need only a join code, so once a
family exists you can play immediately. For the parent API without a real
Supabase project, mint a dev JWT (the compose file's default HS256 secret
matches the minting script):

```bash
JWT=$(node .claude/skills/saas-e2e/assets/mint-jwt.mjs)
AUTH="Authorization: Bearer $JWT"
curl -s -H "$AUTH" -H "Content-Type: application/json" \
  -X POST -d '{"name":"My Family"}' http://localhost:8080/api/v1/family
FAMILY=...   # the "id" from that response (or: curl -s -H "$AUTH" http://localhost:8080/api/v1/me)

# Add a child (grade 2–5; "pin" optional), then mint a 7-day join code:
curl -s -H "$AUTH" -H "Content-Type: application/json" -X POST \
  -d '{"name":"Asha","grade":3}' http://localhost:8080/api/v1/family/$FAMILY/children
curl -s -H "$AUTH" -H "Content-Type: application/json" -X POST \
  -d '{"ttlHours":168}' http://localhost:8080/api/v1/family/$FAMILY/invites
# → {"code":"TIGER-4207",...} — enter it at /join and play.
```

Note: in this Supabase-free mode the **browser parent login cannot work**
(there's no real Supabase behind it) — the minted JWT *is* your parent
session, and the parent role is curl-driven. The kid experience is fully
browser-based. For the real parent dashboard, configure Supabase
(§2b / docs/saas.md) — compose picks the `MATHIZ_SUPABASE_*` values up
from a `.env` file automatically.

Questions come from the bundled stub (always "What is 12 + 7?"). Point the
stack at real services by overriding env vars, e.g.
`MATHIZ_LLM_PROVIDER=gemini MATHIZ_GEMINI_API_KEY=... make dev-up`, or a real
`MATHIZ_SUPABASE_URL`/`_ANON_KEY`/`_JWT_SECRET`. `make dev-down` stops it.

Billing runs too: the stack defaults to the **fake payment provider**, so
every new family gets 30 starter expeditions and the dashboard's wallet card
is fully clickable — subscribe, top up, run out of credits — with no real
money. Details in [§2e](#2e-billing--payments-the-fake-provider).

Everything below is the host-native path — better for fast iteration.

## 1. Run the local CLI (fastest first win)

```bash
export GEMINI_API_KEY=...           # or OPENAI_API_KEY / ANTHROPIC_API_KEY / OPENROUTER_API_KEY
make mathiz
./bin/mathiz                        # welcome → home → play
./bin/mathiz stats
```

Without any key the TUI still runs (AI features disabled — enough to check
screens and navigation). Progress lands in
`~/.local/share/mathiz/mathiz.db`; use `--db /tmp/dev.db` to keep dev
throwaway state separate.

## 2. Run hosted mode (`mathiz serve`)

### 2a. Database

Either Docker:

```bash
make dev-db        # postgres:16 on localhost:5432 (user/pass/db: mathiz)
```

…or, in environments without a Docker daemon, a direct local cluster:

```bash
bash .claude/skills/saas-e2e/assets/pg-local.sh    # port 5433, dbs mathiz_e2e + mathiz_test
```

Schema auto-migrates on server start — no migration step.

### 2b. Configuration

```bash
cp .env.example .env       # then edit
set -a; source .env; set +a
```

Two ways to satisfy parent auth:

- **Real Supabase (the production path)** — create a free project at
  supabase.com, enable the Email provider, and set `MATHIZ_SUPABASE_URL`,
  `MATHIZ_SUPABASE_ANON_KEY`, and (for HS256 projects) 
  `MATHIZ_SUPABASE_JWT_SECRET`. Sign up through the web UI like a real parent.
- **No Supabase (pure-local hacking)** — set any dummy
  `MATHIZ_SUPABASE_URL`/`_ANON_KEY` plus a `MATHIZ_SUPABASE_JWT_SECRET` of
  your choosing, then mint parent JWTs yourself:
  `node .claude/skills/saas-e2e/assets/mint-jwt.mjs` and drive the parent
  API with `curl -H "Authorization: Bearer $JWT"`. The **kid experience
  needs no Supabase at all** — a join code is the only credential.

No LLM key handy? Point the OpenAI provider at the bundled stub:

```bash
node .claude/skills/saas-e2e/assets/llmstub.mjs &   # OpenAI-compatible on :9993
export MATHIZ_LLM_PROVIDER=openai MATHIZ_OPENAI_API_KEY=stub \
       MATHIZ_OPENAI_BASE_URL=http://127.0.0.1:9993/v1
```

(Every question becomes "What is 12 + 7?" — perfect for flow testing.)

### 2c. Build & run

```bash
make web && make mathiz    # SPA → internal/saas/webui/dist → embedded in the binary
./bin/mathiz serve         # listens on MATHIZ_SERVER_ADDR (default :8080)
```

Open `http://localhost:8080` — landing page at `/` (parent + kid doors),
parent sign-in at `/login` (email code first; password fallback), kid join
at `/join`, treasure map at `/play`, browser terminal at `/terminal`.
Typical loop: sign in (or curl with a minted JWT) → create Family Space →
add a child → mint a join code → open `/join` in a private window and play
as the kid.

### 2d. Web development with hot reload

```bash
./bin/mathiz serve                  # backend on :8080
cd web && npm install && npm run dev  # Vite on :5173, proxies /api (incl. WebSockets)
```

Edit `web/src/**` with instant reload; `npm run build` also runs `tsc` and
is the SPA's type check.

### 2e. Billing & payments (the fake provider)

Design: [specs/14-monetisation.md](../specs/14-monetisation.md). The short
version: 1 credit = 1 expedition (5 AI questions, hints and micro-lessons
included); every new Family Space gets **30 free starter credits** that
expire after 30 days; parents subscribe (plan credits, expire at period end)
or buy top-up packs (never expire). Kids never see any of this — at zero
balance they get "The ship needs to rest! ⛵", not a paywall.

**Enable / disable.** One env var controls everything:

```bash
MATHIZ_BILLING_PROVIDER=fake   # dev provider: full money loop, no real money
MATHIZ_BILLING_PROVIDER=       # empty = billing OFF: everything free,
                               # no wallet card, no credit checks (self-hoster default)
```

`make dev-up` sets `fake` for you (see `docker-compose.yml`). Host-native,
export it before `./bin/mathiz serve`. `stripe` enables real payments (see
the Stripe section below); anything else is rejected at startup.

**The clickable loop (recommended).** Sign in on the dashboard → the wallet
card shows "⛵ 30 expeditions left" → pick a plan (Explorer $5/150 ·
Voyager $10/400 · Armada $20/1000, or the $5/100 top-up) → **Subscribe** /
**Buy pack**. With the fake provider the "checkout" URL is a local endpoint
that completes instantly — you land back on `/dashboard?billing=success`
with the credits granted and the plan active. "Manage billing" works too
(the fake portal is just the dashboard).

**The curl loop (scriptable).** Same flow the SPA drives:

```bash
JWT=$(node .claude/skills/saas-e2e/assets/mint-jwt.mjs)
AUTH="Authorization: Bearer $JWT"
FAMILY=... # uid from POST /api/v1/family (§0)

# Wallet: balance, current plan, and the catalog
curl -s -H "$AUTH" http://localhost:8080/api/v1/family/$FAMILY/billing

# Start a checkout (planId: explorer | voyager | armada | topup-100)
URL=$(curl -s -H "$AUTH" -H 'Content-Type: application/json' \
  -X POST -d '{"planId":"explorer"}' \
  http://localhost:8080/api/v1/family/$FAMILY/billing/checkout | jq -r .url)

# "Pay": follow the fake checkout URL (single-use token; replay → 400)
curl -s -o /dev/null -w '%{http_code}\n' "$URL"    # 303 → credits granted

curl -s -H "$AUTH" http://localhost:8080/api/v1/family/$FAMILY/billing
# → {"balance":180,"plan":"explorer","status":"active",...}
```

**Testing the out-of-credits kid flow.** Each expedition (or terminal
session) debits 1 credit. Either play through the starter balance, or
temporarily lower `StarterCredits` in `internal/saas/credits/service.go`
to 1–2 and recreate the family. When the wallet hits zero the next
expedition returns HTTP 402 and the kid sees the ship-resting screen;
subscribing or topping up from the dashboard unblocks them immediately.

**Stripe (real payments).** In the Stripe dashboard create three recurring
prices (Explorer/Voyager/Armada) and one one-time price (top-up pack), then
set: `MATHIZ_BILLING_PROVIDER=stripe`, `MATHIZ_STRIPE_SECRET_KEY`,
`MATHIZ_STRIPE_WEBHOOK_SECRET` (from the webhook endpoint you create for
`POST <public-base-url>/api/v1/billing/webhook` with events
`checkout.session.completed`, `invoice.paid`,
`customer.subscription.deleted`), `MATHIZ_PUBLIC_BASE_URL`, and the four
`MATHIZ_BILLING_PRICE_*` price IDs. Local testing:
`stripe listen --forward-to localhost:8080/api/v1/billing/webhook` (use the
CLI's printed `whsec_...` as the webhook secret). Event application is
idempotent — resending a webhook from the Stripe dashboard must not (and
does not) double-grant. Promotion codes work out of the box: create one in
Stripe and the checkout page shows a code field (that's the early-adopter
comp path).

## 3. Code generation

Ent generates the ORM from `ent/schema/`. After any schema change:

```bash
make generate     # CGO_ENABLED=0 go generate ./ent
```

Never hand-edit generated files under `ent/` (everything except
`ent/schema/` and `ent/generate.go`) — a Claude Code hook blocks it, and
codegen would overwrite it anyway.

## 4. Testing

```bash
go test ./...                              # unit suite (fast, SQLite in-memory)
go test -race ./internal/saas/...          # required for saas/termbridge/game changes
go test -run TestName ./internal/mastery   # one test

# Store suite against real PostgreSQL (otherwise it runs on SQLite):
MATHIZ_TEST_DATABASE_URL="postgres://postgres@127.0.0.1:5433/mathiz_test?sslmode=disable" \
  go test ./internal/store/
```

Full-stack browser E2E (Playwright + the stub LLM, no external services):
follow `.claude/skills/saas-e2e/SKILL.md` — it documents the whole flow
including every UI selector.

Per-area guidance on *what* to run lives in `.claude/skills/verify/SKILL.md`.

## 5. Repo layout, one screen

```
cmd/                  CLI commands; run.go (local wiring), serve.go (SaaS server)
internal/
  session|mastery|spacedrep|problemgen|diagnosis|lessons|gems|skillgraph
                      the learning engine (UI-independent)
  app|router|screen|screens|ui
                      the Bubble Tea TUI
  store/              event sourcing + snapshots (SQLite & PostgreSQL, owner-scoped)
  llm/                provider adapters + retry/logging decorators
  saas/               hosted mode: family, authz, auth, server, game, termbridge, webui
web/                  Vite + React SPA (parent dashboard + kid game)
ent/schema/           hand-written schemas → make generate
specs/                design records (spec-driven repo — add one for significant features)
docs/                 this guide, personas.md, saas.md
.claude/              agent config: skills, hooks, permissions (see CLAUDE.md)
```

## 6. Troubleshooting

| Symptom | Fix |
|---|---|
| `gcc: command not found` during build | You dropped `CGO_ENABLED=0`. All Makefile targets set it; use them. |
| Compile errors in `ent/...` after editing a schema | Run `make generate`. |
| `internal/selfupdate` tests fail (HTTP 404 to 127.0.0.1) | Pre-existing sandbox/proxy artifact — ignore unless you touched selfupdate. |
| `mathiz serve` exits: `MATHIZ_DATABASE_URL is required` | Source your `.env` (`set -a; source .env; set +a`). |
| Parent login fails locally | JWT secret/issuer mismatch: `MATHIZ_SUPABASE_JWT_SECRET` and `MATHIZ_SUPABASE_URL` must match what minted the token. |
| Terminal/expedition won't start, API 500s | Check the serve process's stderr/log — usually the LLM provider (missing key or unreachable base URL). |
| Kid's terminal 403s on WebSocket upgrade behind a proxy | Origin/Host mismatch: add your SPA origin to `MATHIZ_CORS_ORIGINS`. |
| Changed the SPA but `:8080` serves the old one | Rebuild both: `make web && make mathiz` (the binary embeds the SPA at compile time). |
| No wallet/billing card on the dashboard | Billing is off (`MATHIZ_BILLING_PROVIDER` empty). Set it to `fake` and restart — the card hides on 404 by design. |
| Kid sees "The ship needs to rest! ⛵" | Family's credits are at 0. Subscribe/top up on the dashboard (fake checkout is instant), or disable billing. See §2e. |
| Fake checkout URL returns 400 | The token is single-use and in-memory: already redeemed, or the server restarted between checkout and completion. Start a new checkout. |
