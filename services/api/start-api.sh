#!/bin/sh
set -e

PORT="${WHATSAPP_ADAPTER_PORT:-18901}" AUTH_ROOT="${WHATSAPP_AUTH_ROOT:-/data/whatsapp-auth}" node /app/whatsapp-adapter/server.js &
exec /app/agent-nexus-api
