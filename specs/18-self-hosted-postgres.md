# Self-hosted PostgreSQL — core storage off Supabase

## 1. Why

Production data (every learner event, snapshot, family, ledger entry and
quest) lives in the `mathiz` database on the Supabase Postgres cluster
(`.claude/skills/db-backup/SKILL.md`). We want all core storage on a
PostgreSQL we run and control, with Supabase reduced to, at most, an auth
provider (specs/17). After this spec, no Mathiz data lives on Supabase.

Non-goals: changing the schema, the event-sourcing model, or the SQLite
local mode; moving Supabase's own `auth` schema (that belongs to the auth
provider and goes away with specs/17 §6, if ever).

## 2. Current state (verified in code)

- **Storage is already vendor-neutral.** `store.Open` (`internal/store/store.go`)
  opens any `postgres://` DSN with pgx's `database/sql` driver and runs
  ent auto-migrate (`Schema.Create`). There are no extensions, no
  `jsonb`/`uuid` server functions, no RLS policies and no Supabase roles in
  our schema. Timestamps and UIDs are generated in Go.
- **Raw SQL is limited and portable:** the `global_sequence` table
  (`internal/store/event.go`) and owner-parameterized event queries. Both
  already run on SQLite and stock PostgreSQL (CI uses `postgres` service
  containers; docker-compose uses `postgres:16-alpine`).
- **Isolation from Supabase's API surface:** our tables live in a separate
  `mathiz` database, so PostgREST can't see them. A plain server has no
  PostgREST, so this concern disappears.
- **Gap: the PostgreSQL connection pool is unbounded.** Only SQLite sets
  `SetMaxOpenConns`. Supabase's pooler absorbed this; a self-run server
  with `max_connections = 100` will not.

So the migration is an **ops move plus a small hardening change**, not a
storage rewrite.

## 3. What Supabase gives us today that we must replace

| Capability | Supabase today | Self-hosted replacement (required) |
|---|---|---|
| Backups | Managed daily backups | Nightly `scripts/db-backup.sh` to off-host storage, plus WAL-based point-in-time recovery (pgBackRest or WAL-G). Retention ≥ 14 days. |
| Restore confidence | Implicit | A **restore drill** before cutover and monthly: restore the latest backup into a scratch database, then run §5.3 verification against it. |
| TLS | Enforced | Server TLS cert; DSN uses `sslmode=verify-full` whenever the database isn't on the same host or private network. |
| Connection pooling | Supavisor/PgBouncer | App-side pool limits (§4). PgBouncer is optional and must use **session** mode: pgx's prepared statements break in transaction mode. |
| Monitoring | Dashboard | Disk, connections, replication lag (if any), slow queries (`log_min_duration_statement`), backup job success alerts. |
| Upgrades | Managed | Minor upgrades monthly; major upgrades through dump/restore or `pg_upgrade` following the same runbook as §5. |
| HA | Managed | Not required for v1: a single primary plus PITR. Revisit if the recovery-time target demands a standby. |

Hosting location (VM, Kubernetes, or a managed-but-not-Supabase service)
is an operator choice. Everything in this spec assumes only "a PostgreSQL
≥ the source major version that we hold superuser on".

## 4. Code changes

Small, and shipped before cutover:

1. **Pool limits for PostgreSQL** in `store.open`: `SetMaxOpenConns`,
   `SetMaxIdleConns`, `SetConnMaxLifetime`, `SetConnMaxIdleTime`. Defaults
   are 20 / 10 / 30m / 5m, overridable with `MATHIZ_DB_MAX_OPEN_CONNS` and
   `MATHIZ_DB_MAX_IDLE_CONNS`. SQLite keeps `SetMaxOpenConns(1)` unchanged
   (see CLAUDE.md).
2. **Startup check:** `store.Open` logs the server version and the active
   `sslmode` at INFO (no credentials), and fails fast if PostgreSQL is
   older than the minimum supported major (§7).
3. **Docs and config:** remove "Prod: Supabase's connection string" from
   `.env.example`. Update the db-backup skill's "production database is
   on the Supabase cluster" section, `docs/saas.md`,
   `docs/development.md`, `docs/personas.md` (operator row) and the
   README's hosted-mode section.

No schema change and no new ent types. Owner-scoped tables, the owner
guard and the `global_sequence` semantics stay exactly as they are.

## 5. Migration runbook

### 5.1 Prepare (no downtime)

1. Provision the target PostgreSQL (§3), with the same or a newer major
   version than Supabase's (`SELECT version();` on the source). Create a role
   `mathiz` (no superuser) and a database `mathiz` owned by it.
2. Use `pg_dump` / `pg_restore` binaries at least as new as the **newer** of
   source and target.
3. Connect to the source **directly** (port 5432 host), not through the
   transaction-mode pooler (port 6543). `pg_dump` needs a session.
4. Rehearse: dump production, restore into a scratch target database, run
   §5.3, and time the whole thing. The measured time sets the maintenance
   window.

### 5.2 Cutover (short maintenance window)

Expected to be minutes at current data size; confirm with the rehearsal.

1. Stop `mathiz serve` (or scale to zero). No writes may happen after the
   dump starts. Live expeditions end; their in-memory progress since the
   last snapshot save is lost, the same as a deploy restart.
2. `scripts/db-backup.sh --env-file .env <dir>` against Supabase (custom
   format). Keep this file as the rollback artifact.
3. `pg_restore --no-owner --no-privileges --role=mathiz -d "$TARGET_DSN" <file>.dump`
4. Run §5.3 verification. **Any failure aborts the cutover** (start the server
   again on the old DSN).
5. Set `MATHIZ_DATABASE_URL` to the target DSN and start `mathiz serve`.
   Auto-migrate runs as a no-op.
6. Smoke test: a parent loads the dashboard, a child joins and finishes
   an expedition, and the credit debit and the events appear in the target.

### 5.3 Verification

- Exact row count for every table, source vs target (script it: list
  tables from `information_schema.tables` in schema `public`).
- `global_sequence.next_val` > `MAX(sequence)` over every event table and
  snapshots on the target. Otherwise new events would reuse sequence numbers
  and break cross-table ordering.
- `credit_entries`: the sum of `remaining` per family space is equal on both
  sides, so balances are unchanged.
- Uniqueness intact: `pg_restore` reported no constraint or index
  errors. Any error output counts as a failure.
- Start the server against the target with the startup check (§4.2) green.

### 5.4 Rollback

- Before any new writes on the target: repoint `MATHIZ_DATABASE_URL` to
  Supabase and restart. Nothing is lost.
- After writes on the target: rolling back loses those writes unless they
  are copied back. Prefer fixing forward; the Supabase database is kept
  as-is for 14 days and then deleted (§6).

### 5.5 If the window is too long

If the rehearsal shows the dump/restore takes too long, use logical
replication instead: a publication on the Supabase `mathiz` database and a
subscription on the target. Sync, stop writes, wait for lag to reach 0,
rerun §5.3 (logical replication doesn't copy sequence state, so
`global_sequence` must be checked), and switch over. This only
becomes worth the extra complexity if downtime exceeds a few minutes.

## 6. Decommission

1. 14 days after a clean cutover: take one final archival dump of the
   Supabase `mathiz` database to off-host storage, then **drop the
   `mathiz` database on Supabase**. No core data then remains there.
2. Remove Supabase database credentials from every secret store and
   `.env`.
3. If the deployment still uses the `supabase` auth preset (specs/17), the
   Supabase project stays alive **for auth only**. Keep the project from
   being paused for inactivity on the free tier (a paused project means no
   parent can sign in). Clerk's Supabase migration tool reads `auth.users`
   from Supabase Postgres, so keep that access until specs/17 §6 is done or
   ruled out.

## 7. Open questions

- **Hosting target** (VM + systemd, Kubernetes operator such as
  CloudNativePG, or a managed PostgreSQL elsewhere). This decides how §3
  backups and PITR are implemented, but not the rest of this spec.
- **Supabase's current major version**, which sets the target minimum and
  the `store.Open` minimum check. docker-compose and CI use 16, so the
  floor is 16 unless production is newer.
- **Recovery targets:** acceptable data loss (RPO) and downtime (RTO). PITR
  gives an RPO of minutes; a single primary gives an RTO of
  "restore time".
- **Data residency** of the new host. This is a privacy-policy statement
  for parents; update the processor list in `web/src/pages/Legal.tsx`.

## 8. Testing

- `store`: a unit test that the PostgreSQL pool options are applied, and
  that SQLite still forces one connection.
- Existing owner-isolation suite against the target's PostgreSQL version
  (`MATHIZ_TEST_DATABASE_URL`, scratch database only, never production).
- Set CI's PostgreSQL service image to the target major version.
- The rehearsal (§5.1.4) is the migration test. Its verification output is
  attached to the cutover PR or ticket.

## 9. Rollout

1. §4 code changes (pool limits, startup check, docs).
2. Provision the target with backups, PITR and monitoring; run a restore drill.
3. Rehearse §5 against a scratch database.
4. Cutover (§5.2), then decommission (§6) after 14 days.

This spec is independent of specs/17. It can ship before, after, or
alongside the auth work.
