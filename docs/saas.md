# Mathiz SaaS — Family Spaces in the Browser

`mathiz serve` turns Mathiz into a hosted product: parents sign up, create a
Family Space, add their children, and children learn in the browser through
the treasure-map game. The local CLI is unchanged.

See [specs/12-saas.md](../specs/12-saas.md) for the full architecture.

## How it fits together

- **Parents** sign in through the deployment's hosted auth provider
  (`MATHIZ_AUTH_PROVIDER`; Supabase today). The SPA loads that provider's
  adapter, and the server verifies its JWTs locally. An account is
  provisioned automatically on the first authenticated request, keyed on
  the provider and the provider's user ID — the provider *is* signup. See
  [specs/17-auth-providers.md](../specs/17-auth-providers.md).
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

### Parent auth

One auth provider serves a deployment. Mathiz stores no passwords and sends
no sign-in email; the provider does both. The server refuses to start when
existing accounts belong to another provider (`mathiz auth remap` moves
them, see below), and on a database from before pluggable auth.

| `MATHIZ_AUTH_PROVIDER` | Use | Settings |
|---|---|---|
| `supabase` | production | `MATHIZ_AUTH_URL` (project URL, https), `MATHIZ_AUTH_PUBLIC_KEY` (anon key), optional `MATHIZ_AUTH_HS256_SECRET` |
| `test` | local dev, E2E | `MATHIZ_AUTH_ALLOW_TEST=1`, `MATHIZ_AUTH_HS256_SECRET` (32+ characters). `/login` shows a paste-a-token form; mint tokens with `.claude/skills/saas-e2e/assets/mint-jwt.mjs`. Refused when `MATHIZ_PUBLIC_BASE_URL` is https, and blanked by the compose `prod` profile. |

The provider's public URL and client key reach the SPA through
`GET /api/v1/config` (`auth.provider`, `auth.url`, `auth.publicKey`). A
parent whose token does not carry a verified email gets a 401: co-parent
invites match on email.

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
5. Authentication → Providers → Email: turn **Confirm email** ON. This
   step is required. Mathiz treats every Supabase session as
   email-verified, and co-parent invites match on email: with confirmation
   off, anyone could sign up with an invited address and accept the
   invite.
6. Copy into your `.env`:
   - `MATHIZ_AUTH_PROVIDER=supabase`
   - Project URL → `MATHIZ_AUTH_URL`
   - Publishable/anon key → `MATHIZ_AUTH_PUBLIC_KEY`
   - JWT secret (projects that still sign with a shared secret) →
     `MATHIZ_AUTH_HS256_SECRET`. Projects on asymmetric signing keys skip
     this — the server fetches the JWKS from
     `{url}/auth/v1/.well-known/jwks.json` automatically.

Mathiz uses Supabase **only for authentication**. All application data —
families, children, learning events — lives in your own PostgreSQL via Ent,
and authorization is enforced entirely by Mathiz.

### Switching provider

A provider switch is explicit and offline. Import the parents into the new
provider, export a CSV of `old_subject,new_subject` (each parent's user ID
at the old and the new provider), stop the server, then:

```sh
MATHIZ_DATABASE_URL=... ./bin/mathiz auth remap --from supabase --to <new> --map map.csv --dry-run
MATHIZ_DATABASE_URL=... ./bin/mathiz auth remap --from supabase --to <new> --map map.csv
```

Every account of `--from` must be in the map, or nothing is written. Set
`MATHIZ_AUTH_PROVIDER` to the new provider and start the server.

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
