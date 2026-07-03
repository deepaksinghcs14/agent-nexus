#!/bin/sh
set -e

# Corporate TLS interception support: any PEM/CRT mounted at /corp-certs is
# added to the system trust store (Go API: provider/MCP/GitHub calls) and
# exported to node subprocesses (WhatsApp adapter, npx MCP servers), which use
# their own bundled roots. No-op when the directory is empty or absent.
extra_ca=""
for f in /corp-certs/*.pem /corp-certs/*.crt; do
  # Unmatched globs stay literal — skip anything that isn't a real file.
  [ -f "$f" ] || continue
  mkdir -p /usr/local/share/ca-certificates
  cp "$f" "/usr/local/share/ca-certificates/corp-$(basename "$f" | tr . -).crt"
  extra_ca="$f"
done
if [ -n "$extra_ca" ]; then
  update-ca-certificates >/dev/null 2>&1 || true
  export NODE_EXTRA_CA_CERTS="$extra_ca"
  echo "trusted extra CA(s) from /corp-certs"
fi

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
