#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Load .env if present
if [ -f .env ]; then
  set -a; source .env; set +a
fi

mkdir -p bin logs

echo "Building API..."
go build -o bin/be03_app .

echo "Building watcher (if present)..."
if [ -d cmd/watcher ] || [ -f cmd/watcher/main.go ]; then
  if go build -o bin/be03_watcher ./cmd/watcher; then
    echo "watcher built"
  else
    echo "watcher build failed or not present; continuing without watcher binary" >&2 || true
  fi
fi

# stop existing pids if they exist
stop_if_pidfile() {
  f="$1"
  if [ -f "$f" ]; then
    pid=$(cat "$f")
    if kill -0 "$pid" 2>/dev/null; then
      echo "stopping pid $pid from $f"
      kill "$pid" || true
      sleep 1
    fi
    rm -f "$f"
  fi
}

stop_if_pidfile logs/api.pid
stop_if_pidfile logs/watcher.pid

PORT="${SERVER_PORT:-8081}"
echo "Starting API on :$PORT..."
nohup ./bin/be03_app > logs/server.log 2>&1 &
echo $! > logs/api.pid
sleep 1

if [ -x bin/be03_watcher ]; then
  echo "Starting watcher..."
  nohup ./bin/be03_watcher > logs/watcher.log 2>&1 &
  echo $! > logs/watcher.pid
else
  echo "No watcher binary found; skipping watcher start"
fi

echo "Started. API pid=$(cat logs/api.pid) watcher_pid=$( [ -f logs/watcher.pid ] && cat logs/watcher.pid || echo - )"
echo "Tail logs: tail -f logs/server.log logs/watcher.log"
