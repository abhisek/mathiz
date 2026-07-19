#!/usr/bin/env bash
# db-backup.sh — back up the Mathiz PostgreSQL database with pg_dump.
#
# Usage:
#   scripts/db-backup.sh [options] <output-path>
#
#   <output-path>          Backup destination. If it is an existing directory,
#                          a timestamped file is created inside it
#                          (mathiz-YYYYMMDD-HHMMSS.dump / .sql).
#
# Options:
#   -e, --env-file FILE    Source FILE (e.g. .env) before reading
#                          MATHIZ_DATABASE_URL.
#   -F, --format FORMAT    custom (default) or plain.
#                          custom = compressed, restore with pg_restore;
#                          plain  = readable SQL, restore with psql.
#   --no-verify            Skip the post-dump integrity check.
#   -h, --help             Show this help.
#
# Environment:
#   MATHIZ_DATABASE_URL    PostgreSQL DSN to back up (required).
#   MATHIZ_PG_BIN          Directory containing psql/pg_dump to use.
#                          Otherwise PATH is tried, then Homebrew libpq.
set -euo pipefail

usage() { sed -n '2,21p' "$0" | sed 's/^# \{0,1\}//'; }

die() {
	echo "error: $*" >&2
	exit 1
}

FORMAT=custom
ENV_FILE=""
VERIFY=1
OUTPUT=""

while [ $# -gt 0 ]; do
	case "$1" in
	-e | --env-file)
		[ $# -ge 2 ] || die "$1 requires an argument"
		ENV_FILE=$2
		shift 2
		;;
	-F | --format)
		[ $# -ge 2 ] || die "$1 requires an argument"
		FORMAT=$2
		shift 2
		;;
	--no-verify)
		VERIFY=0
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	-*)
		die "unknown option: $1 (see --help)"
		;;
	*)
		[ -z "$OUTPUT" ] || die "unexpected extra argument: $1"
		OUTPUT=$1
		shift
		;;
	esac
done

[ -n "$OUTPUT" ] || die "output path required (see --help)"
case "$FORMAT" in custom | plain) ;; *) die "--format must be 'custom' or 'plain'" ;; esac

# --- 1. MATHIZ_DATABASE_URL ---------------------------------------------------
if [ -n "$ENV_FILE" ]; then
	[ -f "$ENV_FILE" ] || die "env file not found: $ENV_FILE"
	set -a
	# shellcheck disable=SC1090
	. "$ENV_FILE"
	set +a
fi

[ -n "${MATHIZ_DATABASE_URL:-}" ] || die "MATHIZ_DATABASE_URL is not set (export it or pass --env-file .env)"
case "$MATHIZ_DATABASE_URL" in
postgres://* | postgresql://*) ;;
*) die "MATHIZ_DATABASE_URL is not a PostgreSQL DSN (SQLite files can be copied directly)" ;;
esac

# --- 2. Working psql / pg_dump ------------------------------------------------
# A binary can exist but still be broken (e.g. an asdf shim with no version
# pinned), so candidates must actually run --version successfully.
find_pg_bin() {
	local dir
	for dir in "${MATHIZ_PG_BIN:-}" "" /opt/homebrew/opt/libpq/bin /usr/local/opt/libpq/bin /usr/lib/postgresql/*/bin; do
		local psql=${dir:+$dir/}psql
		local pg_dump=${dir:+$dir/}pg_dump
		if command -v "$psql" >/dev/null 2>&1 && command -v "$pg_dump" >/dev/null 2>&1 &&
			"$psql" --version >/dev/null 2>&1 && "$pg_dump" --version >/dev/null 2>&1; then
			echo "${dir:-PATH}"
			return 0
		fi
	done
	return 1
}

PG_BIN=$(find_pg_bin) || die "no working psql/pg_dump found (install libpq: brew install libpq, or set MATHIZ_PG_BIN)"
if [ "$PG_BIN" = "PATH" ]; then
	PSQL=psql PG_DUMP=pg_dump PG_RESTORE=pg_restore
else
	PSQL=$PG_BIN/psql PG_DUMP=$PG_BIN/pg_dump PG_RESTORE=$PG_BIN/pg_restore
fi
echo "using $($PG_DUMP --version) [$PG_BIN]"

# --- 3. Connectivity ----------------------------------------------------------
"$PSQL" "$MATHIZ_DATABASE_URL" -Atc 'select 1' >/dev/null ||
	die "cannot connect to the database (check MATHIZ_DATABASE_URL / network)"

# --- 4. Dump ------------------------------------------------------------------
if [ "$FORMAT" = custom ]; then EXT=dump PG_FORMAT=custom; else EXT=sql PG_FORMAT=plain; fi

if [ -d "$OUTPUT" ]; then
	OUTPUT=${OUTPUT%/}/mathiz-$(date +%Y%m%d-%H%M%S).$EXT
fi
[ -d "$(dirname "$OUTPUT")" ] || die "directory does not exist: $(dirname "$OUTPUT")"
[ -e "$OUTPUT" ] && die "refusing to overwrite existing file: $OUTPUT"

# --no-owner/--no-privileges: hosted providers (Supabase) use roles that won't
# exist on a restore target.
"$PG_DUMP" --format="$PG_FORMAT" --no-owner --no-privileges \
	--file="$OUTPUT" "$MATHIZ_DATABASE_URL"

# --- 5. Verify ----------------------------------------------------------------
if [ "$VERIFY" = 1 ]; then
	if [ "$FORMAT" = custom ]; then
		TABLES=$("$PG_RESTORE" --list "$OUTPUT" | grep -c 'TABLE DATA' || true)
	else
		TABLES=$(grep -c '^COPY ' "$OUTPUT" || true)
	fi
	[ "$TABLES" -gt 0 ] || die "dump verification failed: no table data found in $OUTPUT"
	echo "verified: $TABLES tables with data sections"
fi

echo "backup written: $OUTPUT ($(du -h "$OUTPUT" | cut -f1 | tr -d ' '))"
