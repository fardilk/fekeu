#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

stop_if_pidfile() {
  f="$1"
  if [ -f "$f" ]; then
    pid=$(cat "$f")
    if kill -0 "$pid" 2>/dev/null; then
      echo "Stopping $pid from $f"
      kill "$pid" || true
      sleep 1
    fi
    rm -f "$f"
  fi
}

stop_if_pidfile logs/api.pid
stop_if_pidfile logs/watcher.pid

# fallback: kill by binary names
pkill -f './bin/be03_app' || true
pkill -f './bin/be03_watcher' || true

echo "Stopped (if running)."
