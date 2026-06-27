#!/bin/sh
set -e

: "${LOG_STREAM_INGEST_TOKEN:=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || date +%s)}"
export LOG_STREAM_INGEST_TOKEN

API_PORT="${PORT:-8080}"
API_INTERNAL_URL="${API_INTERNAL_URL:-http://127.0.0.1:${API_PORT}}"
export API_INTERNAL_URL

# Start API immediately so the healthcheck passes.
/app/agent-nexus-api &
API_PID="$!"

# Start WhatsApp adapter immediately.
PORT="${WHATSAPP_ADAPTER_PORT:-18901}" \
  AUTH_ROOT="${WHATSAPP_AUTH_ROOT:-/data/whatsapp-auth}" \
  NODE_OPTIONS=--experimental-global-webcrypto \
  node /app/whatsapp-adapter/server.js &
ADAPTER_PID="$!"

shutdown() {
  kill "$API_PID" "$ADAPTER_PID" 2>/dev/null || true
  wait "$API_PID" "$ADAPTER_PID" 2>/dev/null || true
}
trap shutdown INT TERM

# Watch the two required processes; exit if either dies.
while true; do
  if ! kill -0 "$API_PID" 2>/dev/null; then
    echo "API process exited" >&2
    shutdown; exit 1
  fi
  if ! kill -0 "$ADAPTER_PID" 2>/dev/null; then
    echo "WhatsApp adapter exited" >&2
    shutdown; exit 1
  fi
  sleep 1
done
