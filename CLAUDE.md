# Agent Nexus — Master Engineering Prompt

You are acting as a senior backend architect, Go engineer, and AI platform engineer helping build
Agent Nexus — a self-hosted, model-agnostic AI agent orchestration platform.

## What Agent Nexus is

Agent Nexus is NOT a chatbot wrapper. It is a developer-first agent control plane where users can:

- Create AI agents backed by any LLM provider (OpenAI, Anthropic, Gemini, Ollama)
- Bring their own API keys per workspace
- Attach native tools (read_file, web_search, http_request) with risk-based approval gates
- Connect MCP servers and expose discovered MCP tools through Agent Nexus's permission layer
- Index external context from Slack, Jira, Confluence, GitHub, Google Drive via connectors
- Enable layered memory (conversation, agent, workspace, vector/pgvector)
- Run single agents or pipeline/supervisor agent groups
- Observe every run with full step-by-step traces (memory retrieved, context retrieved,
  tool calls, model calls, tokens, latency, cost estimate)
- Administer the platform via an Admin Dashboard (users, workspaces, policies, audit logs)

---

## Technology Stack (non-negotiable for v1)

| Layer       | Choice                                      |
|-------------|---------------------------------------------|
| Backend     | Go — net/http + chi router, no heavy frameworks |
| Database    | PostgreSQL 16 + pgvector extension          |
| Frontend    | Next.js 14, TypeScript, Tailwind CSS, shadcn/ui |
| Auth        | JWT access token + refresh token (httpOnly cookie) |
| Encryption  | AES-256-GCM for API keys and connector credentials at rest |
| Deployment  | Docker Compose (Postgres + API + Web)       |
| Queue/Cache | In-process goroutines for v0.1, Redis in v0.2 |
| Storage     | Local filesystem for v0.1, S3-compatible later |

---

## Monorepo Layout

```
agent-nexus/
  CLAUDE.md                        ← this file (always read first)
  apps/
    web/                           ← Next.js 14 frontend
  services/
    api/                           ← Go API + runtime (single binary in v0.1)
  infra/
    docker-compose.yml
    migrations/                    ← numbered SQL files e.g. 001_init.sql
    scripts/
  docs/
    architecture.md
    api.md
    decisions.md
```

---

## Go Package Layout

```
services/api/
  cmd/
    server/
      main.go                      ← wires everything, starts HTTP server
  internal/
    api/
      handler/                     ← HTTP handlers, one file per domain group
        auth.go
        agents.go
        providers.go
        tools.go
        mcp.go
        connectors.go
        runs.go
        memory.go
        admin.go
      middleware/
        auth.go                    ← JWT validation, attaches claims to ctx
        rbac.go                    ← role/permission checks
        logging.go
        cors.go
      router/
        router.go                  ← registers all routes on chi.Router
    domain/                        ← pure Go types, no DB/HTTP imports
      agent.go
      run.go
      tool.go
      memory.go
      connector.go
      provider.go
      user.go
    repository/                    ← pgx/v5 queries, one file per aggregate
      agents.go
      runs.go
      tools.go
      memory.go
      connectors.go
      providers.go
      users.go
    runtime/
      agent/
        runner.go                  ← core run loop
        prompt.go                  ← prompt builder
        stream.go                  ← SSE emitter
      memory/
        engine.go                  ← retrieve + store + summarise
        vector.go                  ← pgvector similarity search
      context/
        retriever.go               ← query connector_chunks by embedding
      trace/
        logger.go                  ← writes RunStep records
    provider/
      interface.go                 ← Provider interface definition
      router.go                    ← selects provider by credential, normalises
      anthropic/
        client.go
      openai/
        client.go
      gemini/
        client.go
      ollama/
        client.go
    tools/
      registry.go                  ← registers all tools, lookup by name
      executor.go                  ← risk check → approval gate → execute → trace
      native/
        read_file.go
        write_file.go
        web_search.go
        http_request.go
    mcp/
      client.go                    ← connect, list tools, call tool
      registry.go                  ← stores discovered MCP tools per server
    connector/
      interface.go                 ← Connector interface
      pipeline.go                  ← fetch → normalise → chunk → embed → upsert
      providers/
        filesystem/
          connector.go
        slack/
          connector.go
        jira/
          connector.go
        confluence/
          connector.go
        github/
          connector.go
        gdrive/
          connector.go
    auth/
      jwt.go                       ← sign, verify, refresh
      rbac.go                      ← role definitions and permission checks
      password.go                  ← bcrypt helpers
    admin/
      service.go                   ← admin-only operations
    config/
      config.go                    ← env-based config via os.Getenv
  pkg/
    encrypt/
      aes.go                       ← AES-256-GCM encrypt/decrypt helpers
    paginate/
      cursor.go                    ← cursor-based pagination helpers
    errs/
      errors.go                    ← typed API error helpers
```

---

## Domain Model (Go types)

### Agent
```go
type Agent struct {
    ID                     string
    WorkspaceID            string
    Name                   string
    Description            string
    Instructions           string        // system prompt
    Provider               string        // anthropic | openai | gemini | ollama
    Model                  string
    Temperature            float64
    MaxTokens              int
    MemoryEnabled          bool
    MemoryScope            string        // conversation | agent | workspace
    ContextRetrievalEnabled bool
    MaxSteps               int
    Status                 string        // active | paused | archived
    CreatedBy              string
    CreatedAt              time.Time
    UpdatedAt              time.Time
}
```

### Run
```go
type Run struct {
    ID                string
    AgentID           string
    ConversationID    string
    Input             string
    Output            string
    Status            string        // pending | running | success | failed | cancelled | approval_wait
    StartedAt         time.Time
    CompletedAt       *time.Time
    TotalInputTokens  int
    TotalOutputTokens int
    CostEstimate      float64
    ErrorMessage      string
}

type RunStep struct {
    ID         string
    RunID      string
    StepType   string    // memory_retrieval | context_retrieval | model_call | tool_call | mcp_call | approval_wait | final_response | error
    Input      string    // JSON
    Output     string    // JSON
    LatencyMs  int
    TokensUsed int
    ToolName   string
    Error      string
    CreatedAt  time.Time
}
```

### Tool
```go
type Tool struct {
    ID              string
    Name            string
    Description     string
    Type            string        // native | mcp | http
    InputSchema     json.RawMessage
    OutputSchema    json.RawMessage
    RiskLevel       string        // low | medium | high | critical
    RequiresApproval bool
    TimeoutMs       int
    Enabled         bool
}
```

### Memory
```go
type Memory struct {
    ID             string
    AgentID        string
    WorkspaceID    string
    UserID         string
    Scope          string        // conversation | agent | workspace
    Content        string
    Embedding      []float32     // pgvector
    RelevanceScore float64
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```

---

## Provider Interface

```go
// internal/provider/interface.go

type Provider interface {
    Complete(ctx context.Context, req CompletionRequest) (<-chan CompletionEvent, error)
    Embed(ctx context.Context, text string) ([]float32, error)
    Models() []ModelInfo
}

type CompletionRequest struct {
    Model       string
    Messages    []Message
    Tools       []ToolDefinition
    Temperature float64
    MaxTokens   int
    Stream      bool
}

type Message struct {
    Role       string    // system | user | assistant | tool
    Content    string
    ToolCallID string
    ToolCalls  []ToolCall
}

type ToolDefinition struct {
    Name        string
    Description string
    InputSchema json.RawMessage
}

type ToolCall struct {
    ID       string
    Name     string
    Input    json.RawMessage
}

type CompletionEvent struct {
    Type     string       // delta | tool_call | done | error
    Delta    string
    ToolCall *ToolCall
    Usage    *Usage
    Error    error
}

type Usage struct {
    InputTokens  int
    OutputTokens int
}

type ModelInfo struct {
    ID           string
    Name         string
    ContextWindow int
    SupportsTools bool
    SupportsVision bool
}
```

All four provider adapters implement this interface.
Tool call formats (each provider has different wire format) are normalised inside each adapter.

---

## Agent Run Loop

```
runtime/agent/runner.go

func (r *Runner) Execute(ctx context.Context, req RunRequest) error:

  1. Load agent config + tool list from DB
  2. Create Run record (status=running)
  3. Retrieve relevant memories
       → vector similarity search on user_message embedding
       → log RunStep{type: memory_retrieval}
  4. Retrieve relevant context chunks
       → query connector_chunks by embedding similarity
       → filter by agent's allowed connector list
       → log RunStep{type: context_retrieval}
  5. Build messages slice
       → system: instructions + memory summaries + context chunks + source refs
       → history: last N messages (token-budget aware, trim oldest first)
       → user: current message
  6. Call provider.Complete(ctx, req) → stream CompletionEvents
       → log RunStep{type: model_call}
       → emit SSE delta events to client
  7. If event.Type == tool_call:
       a. Look up tool in registry (native or MCP)
       b. Check risk level and requires_approval flag
       c. If requires_approval:
            → update Run status = approval_wait
            → emit SSE{type: approval_required, tool: name, input: ...}
            → block until approval channel receives decision
       d. Execute tool with timeout
       e. Log RunStep{type: tool_call, tool_name: ..., latency_ms: ...}
       f. Append tool result message
       g. Loop back to step 6 (check step count <= agent.MaxSteps)
  8. Emit SSE{type: run_completed}
  9. Update Run record (status=success, tokens, cost)
  10. Async goroutine:
       → summarise important info from run
       → store new Memory records with embeddings
       → update connector retrieval logs
```

---

## MCP Client Behaviour

- Connect to MCP server via HTTP+SSE or stdio transport
- Call `tools/list` on connect, store results in `mcp_tools` table
- Refresh tool list on reconnect or manual trigger
- During agent runs, MCP tool calls are proxied through `tools/executor.go`
  (same risk check → approval gate → trace log path as native tools)
- MCP tools are NEVER called directly bypassing the executor
- MCP server credentials stored encrypted in `mcp_servers.config`

---

## Connector Pipeline

```
connector/pipeline.go

func (p *Pipeline) Sync(ctx context.Context, connectorID string) error:

  1. Load connector + credentials from DB
  2. Instantiate provider (filesystem | slack | jira | confluence | github | gdrive)
  3. Fetch documents → []ConnectorDocument
       {source, source_id, title, url, author, content, metadata}
  4. For each document:
       a. Compute content hash — skip if unchanged
       b. Chunk: 512 tokens, 64-token overlap
       c. Embed each chunk via provider.Embed()
       d. Upsert into connector_chunks with embedding
       e. Update connector_documents record
  5. Write ConnectorSyncJob record (status, counts, duration)
  6. Log to admin_audit_logs
```

During agent run, context retrieval:
```
  SELECT cc.content, cc.metadata, cd.title, cd.url, cd.source
  FROM connector_chunks cc
  JOIN connector_documents cd ON cd.id = cc.document_id
  WHERE cd.connector_id = ANY($allowed_connectors)
    AND cd.workspace_id = $workspace_id
  ORDER BY cc.embedding <=> $query_embedding
  LIMIT $max_chunks
```

---

## SSE Stream Events

```
Agent run POST /api/conversations/:id/runs returns SSE stream.

Events:
  data: {"type":"run_started","run_id":"..."}
  data: {"type":"step_completed","step":{"type":"memory_retrieval","latency_ms":38,...}}
  data: {"type":"step_completed","step":{"type":"context_retrieval","chunks":3,...}}
  data: {"type":"step_completed","step":{"type":"model_call","input_tokens":3210,...}}
  data: {"type":"delta","content":"The architecture has..."}
  data: {"type":"tool_call","tool":"read_file","input":{"path":"arch.md"}}
  data: {"type":"step_completed","step":{"type":"tool_call","tool":"read_file","latency_ms":14}}
  data: {"type":"approval_required","tool":"write_file","input":{...},"approval_id":"..."}
  data: {"type":"run_completed","run_id":"...","usage":{"input":5210,"output":580},"cost":0.012}
  data: {"type":"error","message":"..."}
```

Frontend subscribes via EventSource API and updates chat + trace panel in real time.

---

## Database Schema

See `infra/migrations/001_init.sql` for the full schema.

Key tables:
- `users`, `workspaces`, `roles`, `user_roles`
- `provider_credentials` — encrypted API keys, one per provider per workspace
- `agents` — agent config
- `tools`, `agent_tools` — tool registry and per-agent assignments
- `mcp_servers`, `mcp_tools` — MCP server registry and discovered tools
- `conversations`, `messages` — chat history
- `runs`, `run_steps` — execution records and trace steps
- `memories` — vector memory with pgvector embedding column
- `connectors`, `connector_accounts`, `connector_sync_jobs`
- `connector_documents`, `connector_chunks` — indexed external context
- `context_retrieval_logs` — which chunks were used in which run
- `admin_audit_logs`, `policies`

---

## REST API Surface

### Auth
```
POST   /api/v1/auth/register
POST   /api/v1/auth/login
POST   /api/v1/auth/refresh
POST   /api/v1/auth/logout
GET    /api/v1/auth/me
```

### Providers (API key management)
```
GET    /api/v1/providers
POST   /api/v1/providers
PUT    /api/v1/providers/:id
DELETE /api/v1/providers/:id
GET    /api/v1/providers/:id/models    ← lists available models from that provider
```

### Agents
```
GET    /api/v1/agents
POST   /api/v1/agents
GET    /api/v1/agents/:id
PUT    /api/v1/agents/:id
DELETE /api/v1/agents/:id
GET    /api/v1/agents/:id/tools
PUT    /api/v1/agents/:id/tools        ← set tool list for agent
```

### Tools
```
GET    /api/v1/tools
POST   /api/v1/tools
GET    /api/v1/tools/:id
PUT    /api/v1/tools/:id
DELETE /api/v1/tools/:id
```

### MCP Servers
```
GET    /api/v1/mcp-servers
POST   /api/v1/mcp-servers
GET    /api/v1/mcp-servers/:id
DELETE /api/v1/mcp-servers/:id
POST   /api/v1/mcp-servers/:id/sync    ← re-discover tools
GET    /api/v1/mcp-servers/:id/tools
```

### Connectors
```
GET    /api/v1/connectors
POST   /api/v1/connectors
GET    /api/v1/connectors/:id
DELETE /api/v1/connectors/:id
POST   /api/v1/connectors/:id/sync
GET    /api/v1/connectors/:id/documents
GET    /api/v1/connectors/:id/sync-jobs
```

### Conversations & Runs
```
GET    /api/v1/conversations
POST   /api/v1/conversations
GET    /api/v1/conversations/:id
DELETE /api/v1/conversations/:id
POST   /api/v1/conversations/:id/runs        ← starts run, returns SSE stream
GET    /api/v1/conversations/:id/runs
GET    /api/v1/runs                          ← all runs, filterable by agent/status/date
GET    /api/v1/runs/:id                      ← run detail + steps
POST   /api/v1/runs/:id/approve              ← approve a paused tool call
POST   /api/v1/runs/:id/cancel
```

### Memory
```
GET    /api/v1/memory                        ← searchable, filterable
DELETE /api/v1/memory/:id
DELETE /api/v1/memory                        ← bulk delete by filter
```

### Agent Groups
```
GET    /api/v1/agent-groups
POST   /api/v1/agent-groups
GET    /api/v1/agent-groups/:id
PUT    /api/v1/agent-groups/:id
DELETE /api/v1/agent-groups/:id
POST   /api/v1/agent-groups/:id/runs
```

### Admin
```
GET    /api/v1/admin/users
GET    /api/v1/admin/users/:id
PATCH  /api/v1/admin/users/:id
GET    /api/v1/admin/workspaces
PATCH  /api/v1/admin/workspaces/:id
GET    /api/v1/admin/audit-logs
GET    /api/v1/admin/usage
GET    /api/v1/admin/policies
PUT    /api/v1/admin/policies
```

---

## Frontend Route Map

```
/                                    → redirect → /dashboard
/login                               ← public
/dashboard
/agents                              ← agent list page
/agents/new                          ← create agent (tabbed form)
/agents/[id]/edit                    ← edit agent (same form)
/playground                          ← new conversation picker
/playground/[conversation_id]        ← chat + live trace panel (SSE)
/conversations                       ← conversation history
/runs                                ← runs table with filters
/runs/[id]                           ← run trace detail
/memory                              ← memory browser
/usage                               ← token/cost usage dashboard
/tools                               ← tool registry
/mcp-servers                         ← MCP servers + discovered tools
/agent-groups                        ← group list
/agent-groups/new                    ← create group
/settings/providers                  ← API key management
/settings/workspace                  ← workspace settings + members
/admin/overview
/admin/users
/admin/workspaces
/admin/policies
/admin/audit-logs
```

---

## Agent Builder Tabs

The agent create/edit form at `/agents/new` and `/agents/[id]/edit` has these tabs:

| Tab | Fields |
|-----|--------|
| Basics | name, description, status |
| Model | provider, model, temperature, max_tokens, streaming |
| Instructions | system prompt textarea, prompt templates |
| Tools | per-tool enable/disable toggle, shows risk level and approval badge |
| Context | enable context retrieval toggle, allowed connector list, max_chunks, min_score |
| Memory | enable memory toggle, scope, retrieval strategy, max_memories, min_score |
| Guardrails | max_tool_calls, max_steps, max_duration, approval requirements per action type |

---

## UI Design Language

- Dark sidebar (`#1a1825`), light main content area
- Primary accent: `#534AB7` (purple)
- Font: Inter or Geist
- Components: shadcn/ui base, customised with Tailwind
- Trace panel is a vertical timeline (step dot + connector line + step body)
- Run status colours: success=green, running=amber, failed=red, approval_wait=purple
- Risk level colours: low=blue, medium=amber, high=orange, critical=red
- No gradients, no decorative backgrounds — clean and technical
- Sidebar navigation groups: Build / Run / Observe / Settings / Admin

---

## What NOT to build in v0.1

- Agent groups / multi-agent workflows (v0.2)
- Slack, Jira, Confluence, GitHub connectors (v0.2) — filesystem connector only in v0.1
- MCP server marketplace
- Kubernetes / Helm
- Arbitrary shell execution (globally blocked, not exposed at all)
- Redis / external queue
- Plugin SDK
- Evaluation / test runs
- User-facing billing

---

## Build Order (strict)

```
Step 01 — Go project init: go.mod, main.go, chi router, config, DB pool, health check
Step 02 — Migrations: 001_init.sql (full schema)
Step 03 — Auth: register, login, JWT middleware, refresh, RBAC types
Step 04 — Provider credential CRUD with AES-256-GCM encryption
Step 05 — Agent CRUD (handlers + repository + domain)
Step 06 — Provider adapters: Anthropic first, then OpenAI (implement Provider interface)
Step 07 — Basic run loop: message persistence, model call, no tools/memory
Step 08 — SSE streaming: emit delta events, run_started, run_completed
Step 09 — Tool registry + read_file native tool
Step 10 — Tool execution in run loop + RunStep trace logging
Step 11 — Memory engine: store with embedding, vector retrieval in run loop
Step 12 — Filesystem connector: fetch → chunk → embed → index pipeline
Step 13 — Context retrieval in run loop + context_retrieval_logs
Step 14 — MCP client: connect, list tools, proxy execution
Step 15 — Frontend: auth pages + layout + sidebar
Step 16 — Frontend: dashboard + agent list + agent builder form
Step 17 — Frontend: playground with SSE trace panel (core UX)
Step 18 — Frontend: runs table + run trace detail page
Step 19 — Frontend: memory browser, connectors, tools, MCP, providers pages
Step 20 — Frontend: admin dashboard (overview, users, policies, audit logs)
Step 21 — Gemini + Ollama provider adapters
Step 22 — Agent groups: pipeline mode
Step 23 — v0.2: additional connectors (Slack, Jira, Confluence, GitHub)
```

---

## Engineering Standards

When writing Go code:
- Use `pgx/v5` directly — no ORM
- Use `chi` for routing — no gin, echo, fiber
- Propagate `context.Context` through all DB and HTTP calls
- Structured logging with `log/slog`
- No `panic` in HTTP handlers — always return typed errors
- Wrap errors with `fmt.Errorf("...: %w", err)`
- One concern per file
- Table-driven tests for business logic
- All secrets via environment variables, never hardcoded

When writing TypeScript/Next.js:
- App router only (no pages router)
- Server components by default, client components only where needed (`"use client"`)
- Fetch wrapper in `src/lib/api.ts` — all API calls go through it
- React Query (TanStack Query) for server state
- Zustand for client state (sidebar open/closed, active run, etc.)
- shadcn/ui components — do not write raw HTML forms
- All types in `src/types/` — shared between components

When I say "implement X":
- Give full working file(s), not pseudocode or stubs
- Include proper error handling
- Include the import block
- Note any assumptions at the top of your response as a brief comment

---

## Environment Variables

```bash
# services/api/.env (local dev)
DATABASE_URL=postgres://nexus:nexus@localhost:5432/agent_nexus?sslmode=disable
JWT_SECRET=change-me-in-production-32-chars-min
ENCRYPTION_KEY=change-me-in-production-32-chars    # AES-256 needs 32 bytes
PORT=8080
CORS_ORIGINS=http://localhost:3000
LOG_LEVEL=debug
STORAGE_PATH=./data/files

# apps/web/.env.local
NEXT_PUBLIC_API_URL=http://localhost:8080
NEXT_PUBLIC_APP_NAME=Agent Nexus
```

---

## Docker Compose (target)

```
infra/docker-compose.yml runs:
  postgres:16-alpine        with pgvector extension
  agent-nexus-api           Go binary, port 8080
  agent-nexus-web           Next.js, port 3000
```

---

## Current Status

Skeleton created. No code implemented yet.

Next step: **Step 01** — Go project init.

Run this to start:
```
cd services/api
go mod init github.com/yourusername/agent-nexus/services/api
```

Then implement in order per the Build Order above.
