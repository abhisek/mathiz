#!/usr/bin/env bash
# Bootstrap a local PostgreSQL for Mathiz E2E in sandboxes without Docker.
# Idempotent: safe to re-run. Listens on 127.0.0.1:5433.
set -euo pipefail

PGBIN=/usr/lib/postgresql/16/bin
PGDATA=/var/lib/postgresql/mathiz-test

if [ ! -x "$PGBIN/initdb" ]; then
  apt-get install -y postgresql >/dev/null
fi

if [ ! -d "$PGDATA" ]; then
  su postgres -s /bin/bash -c "$PGBIN/initdb -D $PGDATA -A trust -U postgres" >/dev/null
fi

if ! pg_isready -h 127.0.0.1 -p 5433 >/dev/null 2>&1; then
  su postgres -s /bin/bash -c \
    "$PGBIN/pg_ctl -D $PGDATA -o '-p 5433 -k /tmp -c listen_addresses=127.0.0.1' -l $PGDATA.log start"
  sleep 1
fi

for db in mathiz_e2e mathiz_test; do
  psql -h 127.0.0.1 -p 5433 -U postgres -tc \
    "SELECT 1 FROM pg_database WHERE datname='$db'" | grep -q 1 ||
    psql -h 127.0.0.1 -p 5433 -U postgres -c "CREATE DATABASE $db"
done

echo "PostgreSQL ready on 127.0.0.1:5433 (databases: mathiz_e2e, mathiz_test)"
