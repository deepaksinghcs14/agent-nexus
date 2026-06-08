# Agent Nexus

> Self-hosted, model-agnostic AI agent orchestration platform.

Create AI agents backed by any LLM (Anthropic, OpenAI, Gemini, Ollama), attach tools, connect memory, and observe every run with full trace logging — all from your own infrastructure.

---

## Quick Start

### Option A — One command (Docker Compose, everything containerised)

```bash
cd infra
docker compose up -d
```

Open http://localhost:3000, register an account, and you're in.

### Option B — Local dev (hot-reload for API and web)

```bash
make dev
```

This starts Postgres in Docker if it isn't running, then runs the Go API and Next.js dev server in parallel. Requires Go 1.22+ and Node 20+.

Individual targets:

```bash
make postgres   # start Postgres only (Docker)
make api        # start Go API only  (reads services/api/.env)
make web        # start Next.js only (reads apps/web/.env.local)
make stop       # stop everything
make logs       # tail logs from all services
```

---

## Prerequisites

| Tool | Version | Install |
|------|---------|---------|
| Docker | 24+ | https://docs.docker.com/get-docker/ |
| Docker Compose | v2 | bundled with Docker Desktop |
| Go | 1.22+ | https://go.dev/dl/ |
| Node.js | 20+ | https://nodejs.org/ |

---

## Project Structure

```
agent-nexus/
  Makefile                 ← dev workflow commands
  CLAUDE.md                ← full engineering spec (read this)
  apps/
    web/                   ← Next.js 14 frontend (port 3000)
  services/
    api/                   ← Go API + agent runtime (port 8080)
      .env                 ← local dev env vars (create from .env.example)
  infra/
    docker-compose.yml     ← Postgres + API + Web (all containerised)
    migrations/            ← SQL applied automatically on first Postgres start
  docs/
    architecture.md
    api.md
```

---

## Environment Variables

### `services/api/.env` (local dev)

Copy from the example and fill in your secrets:

```bash
cp services/api/.env.example services/api/.env
```

| Variable | Required | Description |
|----------|----------|-------------|
| `DATABASE_URL` | yes | Postgres connection string |
| `JWT_SECRET` | yes | 32+ character secret for JWT signing |
| `ENCRYPTION_KEY` | yes | Exactly 32 characters — used for AES-256-GCM key encryption |
| `PORT` | no | API port, default `8080` |
| `CORS_ORIGINS` | no | Comma-separated allowed origins, default `http://localhost:3000` |
| `LOG_LEVEL` | no | `debug` / `info` / `warn` / `error` |
| `STORAGE_PATH` | no | Local file storage path, default `./data/files` |

### `apps/web/.env.local` (local dev)

```bash
NEXT_PUBLIC_API_URL=http://localhost:8080
NEXT_PUBLIC_APP_NAME=Agent Nexus
```

---

## Tech Stack

| Layer | Choice |
|-------|--------|
| Backend | Go — `net/http` + chi router, no ORM |
| Database | PostgreSQL 16 + pgvector extension |
| Frontend | Next.js 14, TypeScript, Tailwind CSS, shadcn/ui |
| Auth | JWT access token (24h) + refresh token (httpOnly cookie, 30d) |
| Encryption | AES-256-GCM for API keys and connector credentials |
| Deployment | Docker Compose |

---

## Features (v0.1)

- **Multi-provider** — Anthropic, OpenAI, Gemini, Ollama; bring your own API keys per workspace
- **Agent builder** — name, instructions, model, temperature, memory scope, tool list, guardrails
- **Playground** — chat interface with live SSE trace panel
- **Tool execution** — native tools (`read_file`, `write_file`, `web_search`, `http_request`) with risk-based approval gates
- **Memory** — conversation / agent / workspace scopes with pgvector similarity retrieval
- **Connectors** — filesystem connector (index local files → chunked embeddings)
- **Run traces** — every step logged: memory retrieval, context retrieval, model call, tool call, latency, tokens, cost estimate
- **Admin dashboard** — users, workspaces, audit logs, policies

Coming in v0.2: agent groups, Slack/Jira/GitHub/Confluence connectors, Redis queue.

---

## API

The REST API runs on port 8080. Full reference: [`docs/api.md`](docs/api.md).

```
POST   /api/v1/auth/register
POST   /api/v1/auth/login
GET    /api/v1/agents
POST   /api/v1/agents
POST   /api/v1/conversations
POST   /api/v1/conversations/:id/runs    ← SSE stream
GET    /api/v1/runs/:id
...
```

---

## License

MIT
