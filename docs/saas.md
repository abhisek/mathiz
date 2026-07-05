# Mathiz SaaS — Family Spaces in the Browser

`mathiz serve` turns Mathiz into a hosted product: parents sign up, create a
Family Space, add their children, and children learn in the browser — the same
terminal experience, streamed over a WebSocket. The local CLI is unchanged.

See [specs/12-saas.md](../specs/12-saas.md) for the full architecture.

## How it fits together

- **Parents** authenticate with Supabase (the SPA uses supabase-js; the server
  verifies JWTs locally). An account is provisioned automatically on first
  authenticated request — Supabase *is* signup.
- **Children** never need email. A parent mints a **join code** (e.g.
  `TIGER-4207`), the child enters it at `/join`, picks their profile, confirms
  their PIN, and the browser stores a revocable **device token**.
- **Children play a treasure-map game** at `/play`: the 56-skill Common Core
  DAG rendered as five islands. Solving AI-generated questions digs treasure,
  collects gems, and lifts the fog on new territory — mastery opens chests,
  spaced-repetition due-dates make conquered spots "sink" until rescued. The
  full learning engine (planner, mastery, spaced rep, diagnosis, hints,
  learner profiles, gems) drives the game over the `/api/v1/game` endpoints;
  see [specs/13-treasure-map.md](../specs/13-treasure-map.md). The classic
  terminal experience is still streamed at `/terminal`.
- **Data** lives in PostgreSQL. Every learning event and snapshot is scoped to
  the child profile that owns it; the authorization layer
  (`internal/saas/authz`) enforces that parents only see their own family and
  children only see themselves.

## Local development

```sh
# 1. Start PostgreSQL
make dev-db          # docker compose up -d postgres

# 2. Configure (copy and edit)
cp .env.example .env
set -a; source .env; set +a

# 3. Build the SPA + binary and run
make web && make mathiz
./bin/mathiz serve
```

Open http://localhost:8080 — landing page at `/` (parent and kid doors),
parent sign-in at `/login`, kid flow at `/join`.

For SPA development with hot reload, run `mathiz serve` on :8080 and
`cd web && npm run dev` — Vite proxies `/api` (including the WebSocket).

### Supabase setup

Parent sign-in is **email-code (OTP) first**: the parent enters their email,
receives a code + magic link, and either one signs them in. Email+password
remains available behind a "Prefer a password?" link on `/login`.

1. Create a project at [supabase.com](https://supabase.com) (free tier works).
2. Enable the **Email** provider (Authentication → Providers).
3. Authentication → URL Configuration: set **Site URL** to your app origin
   (dev: `http://localhost:8080`) and add `http://localhost:8080/login` to
   the **Redirect URLs** — magic links land on `/login`, where the Supabase
   client boots and picks up the session.
4. Authentication → Email Templates → **Magic Link**: include
   `{{ .Token }}` in the body so the email shows the 6-digit code, e.g.
   "Your sign-in code: `{{ .Token }}` — or just click the link below."
   (The default template only contains the link; without `{{ .Token }}`
   parents can still sign in by clicking, but the code box has nothing to
   type.)
5. Copy into your `.env`:
   - Project URL → `MATHIZ_SUPABASE_URL`
   - Publishable/anon key → `MATHIZ_SUPABASE_ANON_KEY`
   - JWT secret (legacy HS256 projects) → `MATHIZ_SUPABASE_JWT_SECRET`.
     Projects on asymmetric signing keys can skip this — the server fetches
     the JWKS from `{url}/auth/v1/.well-known/jwks.json` automatically.

Mathiz uses Supabase **only for authentication**. All application data —
families, children, learning events — lives in your own PostgreSQL via Ent,
and authorization is enforced entirely by Mathiz.

## Monetisation (optional)

Credit-based: 1 credit = 1 expedition, 30 free starter credits per family,
subscriptions + top-up packs beyond that. Off by default — set
`MATHIZ_BILLING_PROVIDER` to enable (`fake` for dev; Stripe/Paddle planned).
Kids never see prices or balances. Design:
[specs/14-monetisation.md](../specs/14-monetisation.md) · dev setup:
[docs/development.md §2e](./development.md#2e-billing--payments-the-fake-provider).

## Production notes

- Point `MATHIZ_DATABASE_URL` at Supabase's Postgres connection string or any
  hosted PostgreSQL. Schema migrates automatically on startup.
- The binary is self-contained (SPA embedded): deploy it anywhere that can
  run a Go binary and reach Postgres. Put TLS in front (the WebSocket works
  over `wss://` automatically when the page is served over HTTPS).
- LLM API keys stay server-side; browsers never see them.
- `MATHIZ_MAX_SESSIONS` and `MATHIZ_SESSION_IDLE_MINUTES` bound terminal
  session resources.

## API surface

Everything is under `/api/v1` — see `internal/saas/server/server.go` for the
route table and `specs/12-saas.md` §7 for the endpoint reference.
