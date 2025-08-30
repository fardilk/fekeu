#!/usr/bin/env bash
set -euo pipefail

# Usage: DB_DSN can be set in environment or in .env file at project root.
# This script will:
# 1) Deduplicate catatan_keuangans by keeping the lowest id per (user_id, file_name)
# 2) Create a UNIQUE INDEX CONCURRENTLY on (user_id, file_name)
# It runs the delete inside a transaction, then creates the index outside the transaction

if [ -z "${DB_DSN:-}" ]; then
  if [ -f .env ]; then
    # handle cases where .env line may contain other tokens before DB_DSN
    DB_DSN=$(sed -n 's/.*DB_DSN=\(.*\)/\1/p' .env | head -n1 | sed 's/^\s*"\?\|"\?\s*$//g')
  fi
fi

if [ -z "${DB_DSN:-}" ]; then
  echo "DB_DSN not set. Export DB_DSN or add it to .env"
  exit 1
fi

echo "Using DB_DSN=$DB_DSN"
# Convert postgres:// URI to libpq key=value form to avoid URI parsing issues in older psql
convert_dsn() {
  uri="$1"
  if [[ "$uri" =~ ^postgres:// ]]; then
    # remove prefix
    body=${uri#postgres://}
    # split into creds@host:port/db?params
    creds_host=${body%%/*}
    rest=${body#*/}
    db_and_params="$rest"
    userpass=${creds_host%@*}
    hostport=${creds_host#*@}
    user=${userpass%%:*}
    pass=${userpass#*:}
    host=${hostport%%:*}
    port=${hostport#*:}
    dbname=${db_and_params%%\?*}
    params=""
    if [[ "$db_and_params" == *\?* ]]; then
      params=${db_and_params#*\?}
    fi
    out="host=$host port=$port user=$user password=$pass dbname=$dbname"
    # parse params like sslmode=disable&foo=bar
    IFS='&' read -r -a kvs <<< "$params"
    for kv in "${kvs[@]}"; do
      if [[ "$kv" == "" ]]; then
        continue
      fi
      out+=" $kv"
    done
    echo "$out"
  else
    echo "$uri"
  fi
}

PSQL_DSN=$(convert_dsn "$DB_DSN")

echo "Using libpq DSN: $PSQL_DSN"

echo "Step 1/2: Deduplicating (keep lowest id per user_id,file_name)"
psql "$PSQL_DSN" -v ON_ERROR_STOP=1 <<SQL
BEGIN;
WITH duplicates AS (
  SELECT user_id, file_name, array_agg(id ORDER BY id) AS ids
  FROM catatan_keuangans
  GROUP BY user_id, file_name
  HAVING count(*) > 1
), to_delete AS (
  SELECT unnest(ids[2:array_length(ids,1)]) AS id FROM duplicates
)
DELETE FROM catatan_keuangans WHERE id IN (SELECT id FROM to_delete);
COMMIT;
SQL

echo "Step 2/2: Creating unique index concurrently (may take time on large tables)"
psql "$DB_DSN" -v ON_ERROR_STOP=1 -c "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_user_file ON catatan_keuangans (user_id, file_name);"

echo "Migration completed successfully. You can remove this script after verification."
