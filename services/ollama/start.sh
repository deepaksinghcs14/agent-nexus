#!/bin/sh
set -e

OLLAMA_PORT="${PORT:-11434}"
export OLLAMA_HOST="0.0.0.0:${OLLAMA_PORT}"

ollama serve &
OLLAMA_PID=$!

# Wait up to 120s for ollama to be ready before pulling the model.
echo "Waiting for Ollama to start..."
for i in $(seq 1 120); do
  curl -sf "http://localhost:${OLLAMA_PORT}/" > /dev/null 2>&1 && break
  sleep 1
done

EMBED_MODEL="${EMBED_MODEL:-nomic-embed-text}"
if ! ollama list 2>/dev/null | grep -q "${EMBED_MODEL}"; then
  echo "Pulling ${EMBED_MODEL}..."
  ollama pull "${EMBED_MODEL}" || echo "Warning: pull failed — embeddings disabled until next restart"
else
  echo "${EMBED_MODEL} already present"
fi

wait $OLLAMA_PID
