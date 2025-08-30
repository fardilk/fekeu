#!/usr/bin/env bash
set -euo pipefail

# migrate_roles_to_ids.sh
# Safely migrate string-based `users.role` values into numeric `roles` table and
# populate `users.role_id`.
#
# Usage:
#  ./scripts/migrate_roles_to_ids.sh --dry-run   # show planned actions
#  ./scripts/migrate_roles_to_ids.sh --apply     # perform migration
#  ./scripts/migrate_roles_to_ids.sh --apply --drop-old  # also drop `users.role` column
#
# It will:
#  - read DB_DSN from env or .env
#  - create missing `roles` table (AutoMigrate) if necessary
#  - insert distinct role names found in `users.role` into `roles` (ignore existing)
#  - add nullable `role_id` column to `users` if missing
#  - back up affected user rows to CSV (in apply mode)
#  - populate `users.role_id` by joining on `roles.name`
#  - optionally set NOT NULL and add FK constraint and/or drop old column

MODE="dry-run"
DROP_OLD=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply) MODE=apply; shift ;;
    --drop-old) DROP_OLD=1; shift ;;
    --help) echo "Usage: $0 [--dry-run|--apply] [--drop-old]"; exit 0 ;;
    *) echo "Unknown arg: $1"; exit 1 ;;
  esac
done

if [ -z "${DB_DSN:-}" ]; then
  if [ -f .env ]; then
    DB_DSN=$(sed -n 's/.*DB_DSN=\(.*\)/\1/p' .env | head -n1 | sed 's/^\s*"\?\|"\?\s*$//g')
  fi
fi

if [ -z "${DB_DSN:-}" ]; then
  echo "DB_DSN not set. Export DB_DSN or add it to .env" >&2
  exit 1
fi

echo "DB_DSN=$DB_DSN"

run_or_echo() {
  if [ "$MODE" = "dry-run" ]; then
    echo "+ $*"
  else
    echo "-> $*";
    eval "$*";
  fi
}

# Create roles table if not exists using a minimal SQL (safe) - we rely on GORM normally
run_or_echo "psql \"$DB_DSN\" -c \"CREATE TABLE IF NOT EXISTS roles (id SERIAL PRIMARY KEY, name VARCHAR(32) UNIQUE NOT NULL, description VARCHAR(255));\""

echo "Collecting distinct role names from users.role..."
ROLE_NAMES=$(psql "$DB_DSN" -Atc "SELECT DISTINCT role FROM users WHERE role IS NOT NULL AND role <> '';")
if [ -z "$ROLE_NAMES" ]; then
  echo "No role strings found in users table. Nothing to insert.";
else
  echo "Found role names:"; echo "$ROLE_NAMES"
fi

# Insert role names into roles table (idempotent)
while IFS= read -r rn; do
  [ -z "$rn" ] && continue
  esc=$(printf "%s" "$rn" | sed "s/'/''/g")
  run_or_echo "psql \"$DB_DSN\" -v ON_ERROR_STOP=1 -c \"INSERT INTO roles (name, description) VALUES ('$esc', '') ON CONFLICT (name) DO NOTHING;\""
done <<< "$ROLE_NAMES"

# Add users.role_id column if missing
has_col=$(psql "$DB_DSN" -Atc "SELECT column_name FROM information_schema.columns WHERE table_name='users' AND column_name='role_id';")
if [ -z "$has_col" ]; then
  run_or_echo "psql \"$DB_DSN\" -v ON_ERROR_STOP=1 -c \"ALTER TABLE users ADD COLUMN role_id bigint;\""
else
  echo "users.role_id already exists"
fi

# Backup affected users (those with role not null and role_id null) before changing
BACKUP_FILE="./tmp_users_role_backup_$(date +%s).csv"
mkdir -p tmp
affected_count=$(psql "$DB_DSN" -Atc "SELECT count(*) FROM users WHERE role IS NOT NULL AND role <> '' AND (role_id IS NULL OR role_id = 0);")
if [ "$affected_count" -gt 0 ]; then
  echo "Users to update: $affected_count"
  if [ "$MODE" = "apply" ]; then
    echo "Backing up affected users to $BACKUP_FILE"
    psql "$DB_DSN" -c "COPY (SELECT * FROM users WHERE role IS NOT NULL AND role <> '' AND (role_id IS NULL OR role_id = 0)) TO STDOUT WITH CSV HEADER" > "$BACKUP_FILE"
  else
    echo "(dry-run) would backup affected users to $BACKUP_FILE"
  fi
fi

# Populate users.role_id by joining roles.name = users.role
run_or_echo "psql \"$DB_DSN\" -v ON_ERROR_STOP=1 -c \"UPDATE users SET role_id = r.id FROM roles r WHERE r.name = users.role AND (users.role_id IS NULL OR users.role_id = 0);\""

if [ "$MODE" = "apply" ]; then
  echo "Verifying population..."
  psql "$DB_DSN" -c "SELECT r.name, count(u.*) FROM roles r LEFT JOIN users u ON u.role_id = r.id GROUP BY r.name ORDER BY r.name;"
fi

# Optionally set NOT NULL and add FK constraint
echo "To enforce NOT NULL and FK, run the following commands after verifying data:"
echo "psql \"$DB_DSN\" -v ON_ERROR_STOP=1 -c \"ALTER TABLE users ALTER COLUMN role_id SET NOT NULL;\""
echo "psql \"$DB_DSN\" -v ON_ERROR_STOP=1 -c \"ALTER TABLE users ADD CONSTRAINT fk_users_roles FOREIGN KEY (role_id) REFERENCES roles(id) ON UPDATE CASCADE ON DELETE RESTRICT;\""

if [ "$DROP_OLD" -eq 1 ]; then
  echo "Note: --drop-old will remove users.role column. This is destructive and irreversible."
  echo "If you really want to drop users.role, run:"
  echo "psql \"$DB_DSN\" -v ON_ERROR_STOP=1 -c \"ALTER TABLE users DROP COLUMN role;\""
fi

echo "Done. Script ran in mode: $MODE"
