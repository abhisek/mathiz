# Mathiz SaaS — Family Spaces in the Browser

`mathiz serve` turns Mathiz into a hosted product: parents sign up, create a
Family Space, add their children, and children learn in the browser through
the treasure-map game. The local CLI is unchanged.

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
  see [specs/13-treasure-map.md](../specs/13-treasure-map.md).
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
`cd web && npm run dev` — Vite proxies `/api`.

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
   `{{ .Token }}` in the body so the email shows the sign-in code, e.g.
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

- Point `MATHIZ_DATABASE_URL` at a PostgreSQL 16 or newer that you run.
  Production runs PostgreSQL 18. No Mathiz data lives on Supabase. The
  schema migrates automatically on startup.
- The server caps its connection pool at 20 open and 10 idle connections.
  `MATHIZ_DB_MAX_OPEN_CONNS` and `MATHIZ_DB_MAX_IDLE_CONNS` override the
  caps. At startup the server logs the PostgreSQL version and the effective
  `sslmode`, and refuses a server older than 16.
- Use `sslmode=verify-full` when the database is not on the same host or
  private network.
- The binary is self-contained (SPA embedded): deploy it anywhere that can
  run a Go binary and reach Postgres. Put TLS in front.
- LLM API keys stay server-side; browsers never see them.
- `MATHIZ_SESSION_IDLE_MINUTES` bounds idle learning-session lifetime.
- Logging: one structured line per request on stdout (`MATHIZ_LOG_FORMAT`
  `text`/`json`, `MATHIZ_LOG_LEVEL`); `MATHIZ_LOG_FILE` tees to a file.
  No built-in rotation — use logrotate or your container's log driver.

## Deploying on a VM

Production is one VM that runs the `prod` profile of `docker-compose.yml`:
PostgreSQL 18 (`prod-db`) and `mathiz serve` (`prod-app`). Kubernetes is
not in scope.

1. Install Docker Engine with the compose plugin on the VM.
2. Clone the repository on the VM. Create `.env` from `.env.example`. Set
   `MATHIZ_DB_PASSWORD`, the auth settings, the LLM key and, if you bill,
   the billing settings. The prod profile has no defaults: a missing
   setting stops the server at boot.
3. Start the stack:

   ```bash
   docker compose --profile prod up -d --build
   ```

4. Put a TLS reverse proxy (Caddy, nginx) on the host in front of
   `127.0.0.1:8080`. The app port is bound to loopback only. Set
   `MATHIZ_TRUST_PROXY=true` and `MATHIZ_PUBLIC_BASE_URL=https://…`.
5. The database has no published port. The app reaches it on the private
   compose network, so its DSN uses `sslmode=disable`.

Upgrades: a minor PostgreSQL upgrade is an image pull and a restart
(`docker compose --profile prod pull prod-db && docker compose --profile
prod up -d`). A major upgrade is a dump and a restore into a new volume.
Use the backup and restore steps below.

### Recovery targets

| Target | Value | How |
|---|---|---|
| RPO (data loss) | 12 hours | a `pg_dump` every 12 hours, copied off the VM |
| RTO (downtime) | 12 hours | a new VM, the compose stack, and a restore of the latest dump |

There is no point-in-time recovery, no WAL archiving and no standby. These
targets do not need them.

### Backups

Add this host cron entry on the VM. It writes a custom-format dump every
12 hours. Copy the directory off the VM (object storage, another host)
with the tool you use for off-site copies. Keep at least 14 days of dumps.

```cron
0 */12 * * * cd /srv/mathiz && docker compose --profile prod exec -T prod-db pg_dump -Fc -U mathiz mathiz > /var/backups/mathiz/mathiz-$(date +\%Y\%m\%d-\%H\%M).dump
```

`scripts/db-backup.sh` does the same from any machine that can reach the
database. Alert when a dump fails or is missing.

### Restore and the monthly drill

```bash
docker compose --profile prod exec -T prod-db createdb -U mathiz mathiz_restore
docker compose --profile prod exec -T prod-db pg_restore --no-owner --no-privileges -U mathiz -d mathiz_restore < mathiz-YYYYMMDD-HHMM.dump
```

Once a month, restore the latest dump into a scratch database as above.
Compare the row counts of the main tables (`accounts`, `family_spaces`,
`child_profiles`, `credit_entries`, `snapshots`) with production. Then
drop the scratch database. A real restore uses the same steps into the
`mathiz` database of a new `prod-db`.

## API surface

Everything is under `/api/v1` — see `internal/saas/server/server.go` for the
route table and `specs/12-saas.md` §7 for the endpoint reference.
