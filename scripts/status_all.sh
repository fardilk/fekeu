#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "API:" 
if [ -f logs/api.pid ] && kill -0 "$(cat logs/api.pid)" 2>/dev/null; then
  ps -p "$(cat logs/api.pid)" -o pid,cmd=
else
  pgrep -af './bin/be03_app' || echo "API not running"
fi

echo
echo "Watcher:"
if [ -f logs/watcher.pid ] && kill -0 "$(cat logs/watcher.pid)" 2>/dev/null; then
  ps -p "$(cat logs/watcher.pid)" -o pid,cmd=
else
  pgrep -af './bin/be03_watcher' || echo "Watcher not running"
fi

echo
echo "Last 200 lines server.log:"
tail -n 200 logs/server.log || true
echo
echo "Last 200 lines watcher.log:"
tail -n 200 logs/watcher.log || true
