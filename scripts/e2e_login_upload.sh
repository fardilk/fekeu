#!/usr/bin/env bash
set -euo pipefail

if [ $# -lt 3 ]; then
  echo "Usage: $0 <username> <password> <file_path>"
  exit 2
fi

ROOT="/home/tw-fardil/Documents/Fardil/Project/be03"
cd "$ROOT"

USERN="$1"; shift
PASSW="$1"; shift
FILE="$1"; shift || true

if [ ! -f "$FILE" ]; then
  echo "File not found: $FILE" >&2
  exit 3
fi

if [ -f .env ]; then
  set -a; source ./.env; set +a
fi
PORT="${SERVER_PORT:-${PORT:-8081}}"

echo "[e2e] logging in as $USERN ..."
RESP=$(curl -s -H 'Content-Type: application/json' -H 'Origin: http://localhost:5173' \
  -X POST "http://127.0.0.1:${PORT}/login" \
  --data "{\"username\":\"${USERN}\",\"password\":\"${PASSW}\"}")

ACCESS_TOKEN=""
if command -v jq >/dev/null 2>&1; then
  ACCESS_TOKEN=$(printf '%s' "$RESP" | jq -r '.access_token // empty')
else
  # crude extractor for access_token
  ACCESS_TOKEN=$(printf '%s' "$RESP" | sed -n 's/.*"access_token"\s*:\s*"\([^"]*\)".*/\1/p')
fi

if [ -z "$ACCESS_TOKEN" ] || [ "$ACCESS_TOKEN" = "null" ]; then
  echo "[e2e] login failed: $RESP" >&2
  exit 4
fi
echo "[e2e] login ok"

BN=$(basename -- "$FILE")
echo "[e2e] uploading $BN ... (forced to public/keu)"
URESP=$(curl -s -H "Authorization: Bearer $ACCESS_TOKEN" \
  -F "file=@${FILE}" \
  "http://127.0.0.1:${PORT}/uploads")
echo "[e2e] upload response: $URESP"

echo "[e2e] polling DB for OCR result on file_name=$BN ..."
go run ./scripts/query_amount --username "$USERN" --file "$BN" --wait 20
