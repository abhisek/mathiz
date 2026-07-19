---
name: db-backup
description: Back up the Mathiz PostgreSQL database using scripts/db-backup.sh. Use whenever the user asks to take, create, or restore a DB/database backup, dump the database, or snapshot production data.
---

# Database backup

Use `scripts/db-backup.sh` — do not hand-roll pg_dump invocations. The script
verifies `MATHIZ_DATABASE_URL` is set, finds a *working* psql/pg_dump (asdf
shims on this machine are broken — it falls back to Homebrew libpq
automatically), checks connectivity, dumps, and verifies the dump contains
table data.

```bash
scripts/db-backup.sh --env-file .env <output-path>
```

- `<output-path>`: a file path, or an existing directory (gets a timestamped
  `mathiz-YYYYMMDD-HHMMSS.dump` inside). The script refuses to overwrite
  existing files.
- `--env-file .env` loads `MATHIZ_DATABASE_URL` when it isn't exported —
  the common case in this repo.
- `-F plain` for a readable `.sql` instead of the default compressed custom
  format. Default to custom unless the user wants to read/diff the SQL.
- `MATHIZ_PG_BIN=<dir>` overrides tool discovery if needed.

Unless the user names a destination, ask where they want the backup stored
(or use an obvious one they've used before). Never write backups into the
repo working tree — the dump contains real tenant data and must not be
committed.

## Restore

- Custom format: `pg_restore --no-owner --no-privileges -d "$TARGET_DSN" <file>.dump`
- Plain format: `psql "$TARGET_DSN" -f <file>.sql`

The production database is the `mathiz` database on the Supabase cluster
(NOT the default `postgres` database — the Supabase dashboard cannot see it;
that is intentional, it keeps tables out of PostgREST's reach). Restoring
into a Supabase target: connect to the same `/mathiz` database name.
