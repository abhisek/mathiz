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

Open `http://localhost:8080` — parent dashboard at `/`, kid join at `/join`,
treasure map at `/play`, browser terminal at `/terminal`. Typical loop:
sign in (or curl with a minted JWT) → create Family Space → add a child →
mint a join code → open `/join` in a private window and play as the kid.

### 2d. Web development with hot reload

```bash
./bin/mathiz serve                  # backend on :8080
cd web && npm install && npm run dev  # Vite on :5173, proxies /api (incl. WebSockets)
```

Edit `web/src/**` with instant reload; `npm run build` also runs `tsc` and
is the SPA's type check.

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
