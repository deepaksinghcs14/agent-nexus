# Agent Nexus

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/license-MIT-green)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white)

> Self-hosted, model-agnostic AI agent orchestration platform.

Create AI agents backed by any LLM (Anthropic, OpenAI, Gemini, Ollama), attach tools, connect memory, and observe every run with full trace logging — all from your own infrastructure.

---

## Quick Start

### Option A — One command (Docker Compose)

```bash
cp infra/.env.example infra/.env
# Edit infra/.env — set JWT_SECRET and ENCRYPTION_KEY
cd infra && docker compose up -d
```

Open http://localhost:3000, register an account, and you're in.

### Option B — Local dev (hot-reload)

```bash
cp services/api/.env.example services/api/.env
# Edit services/api/.env — set JWT_SECRET and ENCRYPTION_KEY
make dev
```

Requires Go 1.22+ and Node 20+. Starts Postgres in Docker, then runs the API and web dev server in parallel.

Individual targets:

```bash
make postgres   # start Postgres only (Docker)
make api        # start Go API only
make web        # start Next.js only
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
  ARCHITECTURE.md          ← architecture, domain model, API reference
  apps/
    web/                   ← Next.js 14 frontend (port 3000)
  services/
    api/                   ← Go API + agent runtime (port 8080)
      .env.example         ← copy to .env and fill in secrets
  infra/
    docker-compose.yml     ← Postgres + API + Web
    .env.example           ← copy to .env for docker compose vars
    migrations/            ← SQL migrations applied automatically on first start
```

---

## Environment Variables

### `services/api/.env` (local dev)

```bash
cp services/api/.env.example services/api/.env
```

| Variable | Required | Description |
|----------|----------|-------------|
| `DATABASE_URL` | yes | Postgres connection string |
| `JWT_SECRET` | yes | 32+ character secret for JWT signing |
| `ENCRYPTION_KEY` | yes | Exactly 32 characters — AES-256-GCM key encryption |
| `PORT` | no | API port, default `8080` |
| `CORS_ORIGINS` | no | Comma-separated allowed origins |
| `LOG_LEVEL` | no | `debug` / `info` / `warn` / `error` |
| `STORAGE_PATH` | no | Local file storage path |
| `GOOGLE_OAUTH_CLIENT_ID` | no | Google OAuth — leave blank to disable |
| `GOOGLE_OAUTH_CLIENT_SECRET` | no | Google OAuth — leave blank to disable |

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

## Features

- **Multi-provider** — Anthropic, OpenAI, Gemini, Ollama; bring your own API keys per workspace
- **Agent builder** — name, instructions, model, temperature, memory scope, tool list, guardrails
- **Playground** — chat interface with live SSE trace panel
- **Tool execution** — native tools (`native_read_file`, `native_write_file`, `native_web_search`, `native_http_request`) and MCP tools, with risk-based approval gates
- **MCP server support** — connect any MCP server, auto-discover tools, proxy calls through the approval pipeline
- **Memory** — conversation / agent / workspace scopes with pgvector similarity retrieval
- **Connectors** — index external files for context retrieval (RAG)
- **Agent groups** — pipeline and supervisor multi-agent workflows with a visual canvas editor
- **Run traces** — every step logged: memory retrieval, context retrieval, model call, tool call, latency, tokens, cost estimate
- **Admin dashboard** — users, workspaces, audit logs, policies

---

## API

The REST API runs on port 8080. See [ARCHITECTURE.md](ARCHITECTURE.md) for the full route reference.

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

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup instructions and code standards.
See [ARCHITECTURE.md](ARCHITECTURE.md) for a deep-dive into the domain model, run loop, and system design.

---

## License

MIT — see [LICENSE](LICENSE).
