#!/usr/bin/env bash
set -euo pipefail

ROOT="/home/tw-fardil/Documents/Fardil/Project/be03"
cd "$ROOT"

mkdir -p bin logs

# Load .env if present
if [ -f .env ]; then
  # shellcheck disable=SC1091
  set -a; source ./.env; set +a
fi

PORT="${SERVER_PORT:-${PORT:-8081}}"
export PORT

echo "[be03] Building API..."
go build -o bin/be03_app .

# Stop existing supervisor loop if running
if [ -f logs/supervisor.pid ]; then
  PID=$(cat logs/supervisor.pid || true)
  if [ -n "${PID:-}" ] && kill -0 "$PID" 2>/dev/null; then
    echo "[be03] Stopping existing supervisor (pid=$PID)"
    kill "$PID" || true
    sleep 1
  fi
  rm -f logs/supervisor.pid
fi

echo "[be03] Starting on :$PORT (logs: logs/server.log)"
# Run an auto-restart loop under nohup so the service keeps running
nohup bash -lc 'while true; do \
  ./bin/be03_app >> logs/server.log 2>&1; \
  echo "$(date "+%F %T") be03_app exited with code $?; restarting in 2s" >> logs/server.log; \
  sleep 2; \
done' >/dev/null 2>&1 &
echo $! > logs/supervisor.pid

sleep 1
echo "[be03] Supervisor PID: $(cat logs/supervisor.pid)"
echo "[be03] Try: curl -s http://127.0.0.1:$PORT/health"
