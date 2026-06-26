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
PORT="${WHATSAPP_ADAPTER_PORT:-18901}" AUTH_ROOT="${WHATSAPP_AUTH_ROOT:-/data/whatsapp-auth}" NODE_OPTIONS=--experimental-global-webcrypto node /app/whatsapp-adapter/server.js &
ADAPTER_PID="$!"

# Start Ollama and pull embedding model in background (non-blocking).
# The API falls back to keyword-only retrieval until the model is ready.
{
  ollama serve &
  OLLAMA_PID="$!"

  for i in $(seq 1 60); do
    if curl -sf http://localhost:11434/ > /dev/null 2>&1; then
      break
    fi
    sleep 1
  done

  EMBED_MODEL="${EMBED_MODEL:-nomic-embed-text}"
  if ! ollama list 2>/dev/null | grep -q "${EMBED_MODEL}"; then
    echo "Pulling embedding model: ${EMBED_MODEL}"
    ollama pull "${EMBED_MODEL}" || echo "Warning: failed to pull ${EMBED_MODEL}, embeddings will be disabled"
  fi
} &

shutdown() {
  kill "$ADAPTER_PID" "$API_PID" 2>/dev/null || true
  ollama stop 2>/dev/null || true
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
