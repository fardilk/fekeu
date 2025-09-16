#!/usr/bin/env bash
set -euo pipefail

ROOT="/home/tw-fardil/Documents/Fardil/Project/be03"
cd "$ROOT"

if [ -f .env ]; then
  set -a; source ./.env; set +a
fi
PORT="${SERVER_PORT:-${PORT:-8081}}"

echo "[be03] Supervisor:"
if [ -f logs/supervisor.pid ] && kill -0 "$(cat logs/supervisor.pid)" 2>/dev/null; then
  ps -p "$(cat logs/supervisor.pid)" -o pid,cmd=
else
  echo "  not running"
fi

echo
echo "[be03] Server processes:"
pgrep -af 'be03_app' || echo "  none"

echo
echo "[be03] Listening on :$PORT (if running):"
ss -ltnp "( sport = :$PORT )" 2>/dev/null || true

echo
echo "[be03] Recent logs (tail -n 50 logs/server.log):"
tail -n 50 logs/server.log 2>/dev/null || echo "  logs/server.log is empty"
