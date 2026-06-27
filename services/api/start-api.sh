#!/bin/sh
set -e

: "${LOG_STREAM_INGEST_TOKEN:=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || date +%s)}"
export LOG_STREAM_INGEST_TOKEN

API_PORT="${PORT:-8080}"
API_INTERNAL_URL="${API_INTERNAL_URL:-http://127.0.0.1:${API_PORT}}"
export API_INTERNAL_URL

# Per-process CPU limits as % of one core (100 = 1 vCPU, 200 = 2 vCPU, …).
# Override via env vars to tune without rebuilding the image.
API_CPU_LIMIT="${API_CPU_LIMIT:-300}"         # 3 vCPU
ADAPTER_CPU_LIMIT="${ADAPTER_CPU_LIMIT:-100}" # 1 vCPU
OLLAMA_CPU_LIMIT="${OLLAMA_CPU_LIMIT:-200}"   # 2 vCPU

apply_cpu_limit() {
  pid="$1"; limit="$2"
  cpulimit --pid "$pid" --limit "$limit" --background 2>/dev/null || true
}

# --- API ---
/app/agent-nexus-api &
API_PID="$!"
apply_cpu_limit "$API_PID" "$API_CPU_LIMIT"

# --- WhatsApp adapter ---
PORT="${WHATSAPP_ADAPTER_PORT:-18901}" \
  AUTH_ROOT="${WHATSAPP_AUTH_ROOT:-/data/whatsapp-auth}" \
  NODE_OPTIONS=--experimental-global-webcrypto \
  node /app/whatsapp-adapter/server.js &
ADAPTER_PID="$!"
apply_cpu_limit "$ADAPTER_PID" "$ADAPTER_CPU_LIMIT"

# --- Ollama ---
ollama serve &
OLLAMA_PID="$!"
apply_cpu_limit "$OLLAMA_PID" "$OLLAMA_CPU_LIMIT"

# Pull embedding model once Ollama is ready (non-blocking, best-effort).
{
  for i in $(seq 1 60); do
    curl -sf http://localhost:11434/ > /dev/null 2>&1 && break
    sleep 1
  done
  EMBED_MODEL="${EMBED_MODEL:-nomic-embed-text}"
  if ! ollama list 2>/dev/null | grep -q "${EMBED_MODEL}"; then
    echo "Pulling embedding model: ${EMBED_MODEL}"
    ollama pull "${EMBED_MODEL}" || echo "Warning: failed to pull ${EMBED_MODEL}, embeddings will be disabled"
  fi
} &

shutdown() {
  kill "$API_PID" "$ADAPTER_PID" "$OLLAMA_PID" 2>/dev/null || true
  wait "$API_PID" "$ADAPTER_PID" "$OLLAMA_PID" 2>/dev/null || true
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
