#!/usr/bin/env bash
set -euo pipefail

ROOT="/home/tw-fardil/Documents/Fardil/Project/be03"
cd "$ROOT"

stop_pid_file() {
  local file="$1"
  if [ -f "$file" ]; then
    local pid
    pid=$(cat "$file" || true)
    if [ -n "${pid:-}" ] && kill -0 "$pid" 2>/dev/null; then
      echo "[be03] Stopping PID $pid from $file"
      kill "$pid" || true
      sleep 1
    fi
    rm -f "$file"
  fi
}

stop_pid_file logs/supervisor.pid

# Fallback: kill stray processes
pkill -f '/bin/be03_app' || true
pkill -f 'be03_app' || true

echo "[be03] Stopped."
