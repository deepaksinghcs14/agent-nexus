# Agent Nexus

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/license-MIT-green)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white)

> Self-hosted, model-agnostic AI agent orchestration platform.

Create AI agents backed by any LLM (Anthropic, OpenAI, Gemini, Ollama), attach tools, connect memory, and observe every run with full trace logging — all from your own infrastructure. No vendor lock-in, no data leaving your servers.

---

## Screenshots

<table>
<tr>
<td width="50%">

**Dashboard**
![Dashboard](docs/screenshots/02_dashboard.png)

</td>
<td width="50%">

**Agent List**
![Agents](docs/screenshots/03_agents_list.png)

</td>
</tr>
<tr>
<td width="50%">

**Agent Builder — Tools (grouped by source)**
![Agent Builder Tools](docs/screenshots/06_agent_builder_tools.png)

</td>
<td width="50%">

**Playground with Live Trace Panel**
![Playground](docs/screenshots/08_playground.png)

</td>
</tr>
<tr>
<td width="50%">

**Visual Workflow Canvas (Supervisor)**
![Supervisor Workflow](docs/screenshots/14_canvas_supervisor_Research_and_Content_Creation_.png)

</td>
<td width="50%">

**Hierarchical Multi-Agent Workflow**
![Hierarchical Workflow](docs/screenshots/16_canvas_supervisor_Hierarchical_Code_Review.png)

</td>
</tr>
<tr>
<td width="50%">

**Run Traces — Full Step Observability**
![Runs](docs/screenshots/09_runs.png)

</td>
<td width="50%">

**MCP Server Integration**
![MCP Servers](docs/screenshots/11_mcp_servers.png)

</td>
</tr>
<tr>
<td width="50%">

**Nexus AI — Meta-Agent Chat Interface**
![Nexus AI](docs/screenshots/20_nexus_ai.png)

</td>
<td width="50%">

**Tools Registry (grouped by source)**
![Tools](docs/screenshots/10_tools.png)

</td>
</tr>
</table>

---

## Features

### Core Agent Platform
- **Model-agnostic** — Anthropic Claude, OpenAI GPT, Google Gemini, and local Ollama models. Bring your own API keys per workspace. Switch providers without changing your agent config.
- **Agent builder** — configure instructions (system prompt), model, temperature, max tokens, memory scope, tool list, and guardrails (max steps, max tool calls, timeout) from a clean tabbed UI
- **Playground** — send messages and watch the agent think in real time via a live SSE trace panel showing every memory retrieval, tool call, model call, latency, and token count

### Multi-Agent Workflows
- **Visual canvas editor** — drag-and-drop workflow builder powered by React Flow; add agent nodes, condition branches, parallel fans, join gates, and loop nodes
- **Pipeline mode** — agents execute in sequence, each receiving the previous agent's output
- **Supervisor mode** — a supervisor LLM routes tasks dynamically to specialist sub-agents; full BFS executor with conditional routing and parallel execution
- **Workflow SSE** — live node status updates streamed to the canvas as a group run executes

### Tools & MCP
- **Native tools** — `read_file`, `write_file`, `web_search`, `http_request` with configurable risk levels
- **MCP server support** — connect any MCP-compatible server (HTTP+SSE or stdio transport); auto-discover and sync tools; proxy all calls through the approval pipeline
- **HTTP tools** — define arbitrary HTTP tools with JSON schemas; treat any external API as an agent tool
- **Risk-based approval gates** — mark any tool `requires_approval`; the run pauses and waits for human approval before executing; approval can be granted from the UI or API

### Memory & Context (RAG)
- **Layered memory** — conversation, agent, and workspace scopes; each run stores a memory summary with pgvector embeddings for similarity retrieval in future runs
- **Connector RAG** — index external files via the filesystem connector; chunks are embedded with pgvector and retrieved at query time to ground agent responses in your documents
- **Vector search** — pgvector cosine similarity with configurable score thresholds and chunk counts per agent

### Observability
- **Full run traces** — every step is logged: memory retrieval (which memories, score), context retrieval (which chunks, source), model call (input/output tokens, latency, cost), tool calls (name, input, output, latency)
- **Cost tracking** — per-run input/output token counts with cost estimates displayed in the runs table and usage dashboard
- **Usage dashboard** — workspace-level token and cost aggregates over time

### Multi-Workspace & Administration
- **Multi-workspace** — isolate agents, tools, memory, and API keys per workspace; support for personal, team, organization, project, and sandbox workspace types
- **Role-based access** — owner, admin, member, and viewer roles per workspace
- **Admin dashboard** — manage all users, workspaces, and policies from a single admin panel
- **Audit logs** — every create, update, and delete action is recorded with actor, resource, timestamp, and IP address

### Nexus AI
- **Meta-agent** — a built-in AI assistant that can list agents/tools/connectors, create new agents, and build workflow graphs using natural language — backed by the same agent runtime as user-created agents

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

---

## More Screenshots

### Admin Portal

<table>
<tr>
<td width="50%">

**Admin Overview**
![Admin Overview](docs/screenshots/admin_01_overview.png)

</td>
<td width="50%">

**User Management**
![Admin Users](docs/screenshots/admin_02_users.png)

</td>
</tr>
<tr>
<td width="50%">

**Workspace Management**
![Admin Workspaces](docs/screenshots/admin_03_workspaces.png)

</td>
<td width="50%">

**Audit Logs**
![Admin Audit Logs](docs/screenshots/admin_04_audit_logs.png)

</td>
</tr>
<tr>
<td width="50%">

**Policies**
![Admin Policies](docs/screenshots/admin_05_policies.png)

</td>
<td width="50%"></td>
</tr>
</table>

### Built-in Documentation

<table>
<tr>
<td width="50%">

**What is an Agent?**
![Docs Agent](docs/screenshots/docs_01_what_is_an_agent.png)

</td>
<td width="50%">

**Agent Configuration Reference**
![Docs Agent Config](docs/screenshots/docs_02_agent_configuration.png)

</td>
</tr>
<tr>
<td width="50%">

**Invoke API**
![Docs Invoke API](docs/screenshots/docs_03_invoke_api.png)

</td>
<td width="50%">

**SSE Stream Events**
![Docs SSE Events](docs/screenshots/docs_06_sse_events.png)

</td>
</tr>
<tr>
<td width="50%">

**MCP Servers Guide**
![Docs MCP](docs/screenshots/docs_05_mcp_servers.png)

</td>
<td width="50%">

**Workflow Patterns**
![Docs Workflows](docs/screenshots/docs_07_workflows.png)

</td>
</tr>
</table>
