# Runner service: executes headless Claude Code repo sessions.
# Deployed as its own Railway service. Required env:
#   GITHUB_TOKEN        — repo clone/push over HTTPS
#   ANTHROPIC_API_KEY   — consumed by the claude CLI
#   RUNNER_EXECUTOR     — claude (default) | stub
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -o /runner .

FROM node:22-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends git ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && npm install -g @anthropic-ai/claude-code
COPY --from=build /runner /usr/local/bin/runner
# Run as the non-root `node` user: the claude CLI refuses
# --dangerously-skip-permissions under root. Pre-create the session work dir
# so the named volume inherits node's ownership.
RUN mkdir -p /data/sessions && chown -R node:node /data
USER node
ENV PORT=8092 WORK_DIR=/data/sessions HOME=/home/node
EXPOSE 8092
CMD ["runner"]
