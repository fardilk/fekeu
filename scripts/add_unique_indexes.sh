#!/usr/bin/env bash
set -eu

# scripts/add_unique_indexes.sh
# Usage:
#  DB_DSN="postgres://user:pass@host:5432/dbname?sslmode=disable" ./scripts/add_unique_indexes.sh [--concurrent] [--force]
#
# This script performs a safe, idempotent creation of two UNIQUE indexes:
#  - uploads(profile_id, file_name)
#  - catatan_keuangans(user_id, file_name)
#
# By default it will:
#  1) check for duplicates and exit with a non-zero status if any are found (you should dedupe first)
#  2) run CREATE UNIQUE INDEX IF NOT EXISTS ... to create the indexes
#
# Use --force to skip duplicate checks (only if you're sure) and --concurrent to attempt CONCURRENTLY creation.

DB_DSN=${DB_DSN:-}
if [ -z "$DB_DSN" ]; then
  echo "ERROR: DB_DSN environment variable must be set (Postgres DSN)." >&2
  echo "Example: export DB_DSN=\"postgres://postgres:postgres@localhost:5432/cat_keu?sslmode=disable\"" >&2
  exit 2
fi

CONCURRENT=0
FORCE=0
while [ $# -gt 0 ]; do
  case "$1" in
    --concurrent) CONCURRENT=1; shift ;;
    --force) FORCE=1; shift ;;
    -h|--help) echo "Usage: DB_DSN=... $0 [--concurrent] [--force]"; exit 0 ;;
    *) echo "Unknown arg: $1" >&2; exit 2 ;;
  esac
done

echo "Using DB_DSN from environment. Checking for duplicates..."

set +e
DUP_CAT=$(psql "$DB_DSN" -At -c "SELECT user_id||'|'||file_name||'|'||count FROM (SELECT user_id, file_name, count(*) as count FROM catatan_keuangans GROUP BY user_id, file_name HAVING count(*)>1) t;" 2>/dev/null)
RC_CAT=$?
DUP_UP=$(psql "$DB_DSN" -At -c "SELECT profile_id||'|'||file_name||'|'||count FROM (SELECT profile_id, file_name, count(*) as count FROM uploads GROUP BY profile_id, file_name HAVING count(*)>1) t;" 2>/dev/null)
RC_UP=$?
set -e

if [ $RC_CAT -ne 0 ] || [ $RC_UP -ne 0 ]; then
  echo "Warning: failed to check duplicates (psql returned non-zero). Ensure DB_DSN is correct and you can connect." >&2
  exit 3
fi

if [ -n "$DUP_CAT" ] || [ -n "$DUP_UP" ]; then
  echo "Duplicates detected:" >&2
  if [ -n "$DUP_CAT" ]; then
    echo "catatan_keuangans duplicates (user_id|file_name|count):" >&2
    echo "$DUP_CAT" | sed 's/|/\t/g' >&2
  fi
  if [ -n "$DUP_UP" ]; then
    echo "uploads duplicates (profile_id|file_name|count):" >&2
    echo "$DUP_UP" | sed 's/|/\t/g' >&2
  fi
  if [ $FORCE -eq 0 ]; then
    echo "Resolve duplicates before creating unique indexes, or re-run with --force to skip checks." >&2
    exit 4
  else
    echo "--force provided; proceeding despite duplicates." >&2
  fi
else
  echo "No duplicates found." >&2
fi

# Index names
IDX_UP_NAME=idx_uploads_profile_file_unique
IDX_CAT_NAME=idx_catatan_user_file_unique

if [ $CONCURRENT -eq 1 ]; then
  echo "Creating indexes using CONCURRENTLY (will take longer but avoids long locks)."
  # Check index existence first because CREATE INDEX CONCURRENTLY does not support IF NOT EXISTS on some PG versions
  EXISTS_UP=$(psql "$DB_DSN" -At -c "SELECT 1 FROM pg_indexes WHERE indexname='$IDX_UP_NAME';")
  if [ -z "$EXISTS_UP" ]; then
    echo "Creating $IDX_UP_NAME concurrently..."
    psql "$DB_DSN" -c "CREATE UNIQUE INDEX CONCURRENTLY $IDX_UP_NAME ON uploads(profile_id, file_name);"
  else
    echo "$IDX_UP_NAME already exists. Skipping."
  fi

  EXISTS_CAT=$(psql "$DB_DSN" -At -c "SELECT 1 FROM pg_indexes WHERE indexname='$IDX_CAT_NAME';")
  if [ -z "$EXISTS_CAT" ]; then
    echo "Creating $IDX_CAT_NAME concurrently..."
    psql "$DB_DSN" -c "CREATE UNIQUE INDEX CONCURRENTLY $IDX_CAT_NAME ON catatan_keuangans(user_id, file_name);"
  else
    echo "$IDX_CAT_NAME already exists. Skipping."
  fi
else
  echo "Creating indexes using IF NOT EXISTS (fast, may take locks on large tables)."
  psql "$DB_DSN" -c "CREATE UNIQUE INDEX IF NOT EXISTS $IDX_UP_NAME ON uploads(profile_id, file_name);"
  psql "$DB_DSN" -c "CREATE UNIQUE INDEX IF NOT EXISTS $IDX_CAT_NAME ON catatan_keuangans(user_id, file_name);"
fi

echo "Indexes created (or already existed)."
exit 0
