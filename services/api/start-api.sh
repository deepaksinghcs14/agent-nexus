#!/bin/sh
set -e

: "${LOG_STREAM_INGEST_TOKEN:=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || date +%s)}"
export LOG_STREAM_INGEST_TOKEN

API_PORT="${PORT:-8080}"
API_INTERNAL_URL="${API_INTERNAL_URL:-http://127.0.0.1:${API_PORT}}"
export API_INTERNAL_URL

/app/agent-nexus-api &
API_PID="$!"

PORT="${WHATSAPP_ADAPTER_PORT:-18901}" AUTH_ROOT="${WHATSAPP_AUTH_ROOT:-/data/whatsapp-auth}" node /app/whatsapp-adapter/server.js &
ADAPTER_PID="$!"

shutdown() {
  kill "$ADAPTER_PID" "$API_PID" 2>/dev/null || true
  wait "$ADAPTER_PID" "$API_PID" 2>/dev/null || true
}

trap shutdown INT TERM

while true; do
  if ! kill -0 "$API_PID" 2>/dev/null; then
    shutdown
    wait "$API_PID"
    exit $?
  fi
  if ! kill -0 "$ADAPTER_PID" 2>/dev/null; then
    shutdown
    wait "$ADAPTER_PID"
    exit $?
  fi
  sleep 1
done
