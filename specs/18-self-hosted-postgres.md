# Self-hosted PostgreSQL — core storage off Supabase

**Status:** implemented (code, compose, docs). The decisions in §2 are the
owner's (2026-10-17) and override the first draft of this spec.

## 1. Why

Production data (every learner event, snapshot, family, ledger entry and
quest) lived in the `mathiz` database on the Supabase Postgres cluster. We
want all core storage on a PostgreSQL we run and control. Supabase stays,
at most, an auth provider (specs/17). After this spec, no Mathiz data
lives on Supabase.

Non-goals: changing the schema, the event-sourcing model, or the SQLite
local mode.

## 2. Decisions

- **Hosting:** one VM running the `prod` profile of `docker-compose.yml`.
  No Kubernetes in the near future.
- **Version:** PostgreSQL 18, the latest stable major (19 is a release
  candidate; GA is planned for 2026-11-19). The store refuses servers older
  than 16, the oldest major CI and dev have run.
- **Recovery targets:** RPO and RTO are 12 hours, met with a `pg_dump`
  every 12 hours copied off the VM. No point-in-time recovery, no WAL
  archiving, no standby. The targets are documented, not enforced by code.
- **No data migration.** The Supabase deployment was a test deployment.
  Production starts on a fresh database, and nothing is imported (specs/17
  §2). The first draft's dump/restore runbook, verification script and
  rollback plan are dropped. The owner drops the Supabase `mathiz`
  database as an ops step.

## 3. Current state (verified in code)

- **Storage is vendor-neutral.** `store.Open` (`internal/store/store.go`)
  opens any `postgres://` DSN with pgx and runs ent auto-migrate. No
  extensions, no server-side UID/JSON functions, no RLS, no Supabase roles.
- **Raw SQL is limited and portable:** the `global_sequence` table and
  owner-parameterized event queries.

## 4. Code changes (done)

1. **Pool limits for PostgreSQL** in `store.open`: 20 open, 10 idle (never
   more than open), 30 min lifetime, 5 min idle time. Overrides:
   `MATHIZ_DB_MAX_OPEN_CONNS`, `MATHIZ_DB_MAX_IDLE_CONNS`; a malformed
   value fails at open. SQLite keeps `SetMaxOpenConns(1)`.
2. **Startup check:** `store.Open` logs the server version and the
   effective `sslmode` (the DSN's, else `PGSSLMODE`, else `prefer`; never
   the DSN itself), and fails on a server older than PostgreSQL 16.
3. **`store.BeforeMigrate`:** checks that must see the schema as an older
   release left it run before auto-migrate (specs/17 §3.3 uses it).

## 5. Deployment (docs/saas.md "Deploying on a VM")

- `prod-db`: `postgres:18-alpine`, no published port, a named volume,
  healthcheck, `restart: unless-stopped`. An empty `MATHIZ_DB_PASSWORD`
  stops it (the image refuses to initialise without a password).
- `prod-app`: built from the repo, `env_file: .env`, DSN to `prod-db` on the
  private compose network with `sslmode=disable`, port bound to
  `127.0.0.1:8080` for a TLS reverse proxy on the host. It blanks
  `MATHIZ_AUTH_ALLOW_TEST`, so a `.env` copied from dev cannot enable test
  tokens.
- PostgreSQL 18 images keep data in `/var/lib/postgresql/18/docker`, so
  volumes mount at `/var/lib/postgresql`. The dev volume is renamed
  (`mathiz-pg18data`) so an old 16 volume is never mounted into 18.
- The dev database and CI also run PostgreSQL 18.

## 6. Operations

| Capability | How |
|---|---|
| Backups | host cron every 12 h: `pg_dump -Fc` inside `prod-db`, copied off the VM, 14+ days kept |
| Restore drill | monthly: restore the latest dump into a scratch database, compare row counts of the main tables |
| TLS | reverse proxy on the host; `sslmode=verify-full` whenever the database is reached across a network |
| Upgrades | minor: image pull + restart; major: dump and restore into a new volume |
| Monitoring | disk, connections, backup job success (alert on a missing dump) |

## 7. Open questions — resolved

Hosting target, PostgreSQL version and recovery targets: §2. Data
residency of the VM is a privacy-policy statement and follows the hosting
choice.

## 8. Testing

- `store`: pool settings applied, malformed overrides fail, SQLite keeps
  one connection, sslmode reporting, `ColumnExists` on both dialects.
- The store and owner-isolation suites pass on PostgreSQL 18 and 16.
- The `prod` profile was booted once: PostgreSQL 18 data directory, a
  dump, a restore into a scratch database, and the empty-password refusal.
