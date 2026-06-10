# Architecture — Agent Nexus

Agent Nexus is a developer-first AI agent control plane — not a chatbot wrapper. It gives you a self-hosted platform to create, run, and observe AI agents backed by any LLM.

Users can:
- Create agents backed by any LLM (Anthropic, OpenAI, Gemini, Ollama) using their own API keys
- Attach native tools (`native_read_file`, `native_web_search`, `native_http_request`) with risk-based approval gates
- Connect MCP servers and expose discovered tools through the same approval pipeline
- Index external context (files, Slack, Jira, GitHub, Google Drive) via connectors
- Enable layered memory (conversation, agent, or workspace scope with pgvector similarity retrieval)
- Run single agents or pipeline/supervisor agent groups
- Observe every run with full step-by-step traces — memory retrieved, context retrieved, model calls, tool calls, tokens, latency, cost estimate
- Administer the platform via an Admin Dashboard (users, workspaces, policies, audit logs)

---

## System Diagram

![Agent Nexus Architecture](./ARCHITECTURE.svg)

---

## Technology Stack

| Layer | Choice |
|-------|--------|
| Backend | Go — `net/http` + chi router, no heavy frameworks |
| Database | PostgreSQL 16 + pgvector extension |
| Frontend | Next.js 14, TypeScript, Tailwind CSS, shadcn/ui |
| Auth | JWT access token + refresh token (httpOnly cookie) |
| Encryption | AES-256-GCM for API keys and connector credentials at rest |
| Deployment | Docker Compose (Postgres + API + Web) |

---

## Monorepo Layout

```
agent-nexus/
  apps/
    web/                           ← Next.js 14 frontend (port 3000)
  services/
    api/                           ← Go API + agent runtime (port 8080)
  infra/
    docker-compose.yml
    migrations/                    ← numbered SQL files (001_init.sql …)
  ARCHITECTURE.md                  ← this file
  CONTRIBUTING.md
  LICENSE
```

---

## Go Package Layout

```
services/api/
  cmd/server/main.go               ← wires everything, starts HTTP server
  internal/
    api/
      handler/                     ← HTTP handlers, one file per domain group
      middleware/                  ← JWT auth, RBAC, logging, CORS
      router/router.go             ← registers all routes on chi.Router
    domain/                        ← pure Go types, no DB/HTTP imports
    repository/                    ← pgx/v5 queries, one file per aggregate
    runtime/
      agent/runner.go              ← core run loop
      memory/engine.go             ← retrieve + store + summarise
      context/retriever.go         ← query connector_chunks by embedding
      trace/logger.go              ← writes RunStep records
    provider/
      interface.go                 ← Provider interface
      anthropic/ openai/ gemini/ ollama/
    tools/
      registry.go                  ← tool lookup by name
      executor.go                  ← risk check → approval gate → execute → trace
      native/                      ← read_file, write_file, web_search, http_request
    mcp/client.go                  ← connect, list tools, proxy calls
    connector/pipeline.go          ← fetch → chunk → embed → upsert
    auth/                          ← JWT, RBAC, bcrypt
    config/config.go               ← env-based config
  pkg/
    encrypt/aes.go                 ← AES-256-GCM helpers
    paginate/cursor.go
    errs/errors.go                 ← typed API error helpers
```

---

## Domain Model

### Agent
```go
type Agent struct {
    ID                      string
    WorkspaceID             string
    Name                    string
    Description             string
    Instructions            string    // system prompt
    Provider                string    // anthropic | openai | gemini | ollama
    Model                   string
    Temperature             float64
    MaxTokens               int
    MemoryEnabled           bool
    MemoryScope             string    // conversation | agent | workspace
    ContextRetrievalEnabled bool
    MaxSteps                int
    Status                  string    // active | paused | archived
    CreatedBy               string
    CreatedAt               time.Time
    UpdatedAt               time.Time
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
    Status            string    // pending | running | success | failed | cancelled | approval_wait
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
    ID               string
    Name             string
    Description      string
    Type             string          // native | mcp | http
    InputSchema      json.RawMessage
    OutputSchema     json.RawMessage
    RiskLevel        string          // low | medium | high | critical
    RequiresApproval bool
    TimeoutMs        int
    Enabled          bool
}
```

### Memory
```go
type Memory struct {
    ID             string
    AgentID        string
    WorkspaceID    string
    UserID         string
    Scope          string      // conversation | agent | workspace
    Content        string
    Embedding      []float32   // stored in pgvector
    RelevanceScore float64
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```

---

## Provider Interface

All LLM adapters implement the same interface (`internal/provider/interface.go`), normalising provider-specific wire formats internally:

```go
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

type CompletionEvent struct {
    Type     string    // delta | tool_call | done | error
    Delta    string
    ToolCall *ToolCall
    Usage    *Usage
    Error    error
}
```

---

## Agent Run Loop

`runtime/agent/runner.go` — `Execute(ctx, req)`:

1. Load agent config + tool list from DB
2. Create `Run` record (`status=running`)
3. Retrieve relevant memories — vector similarity search on the user message embedding → log `RunStep{type: memory_retrieval}`
4. Retrieve relevant context chunks — pgvector search across `connector_chunks` filtered by agent's allowed connectors → log `RunStep{type: context_retrieval}`
5. Build the messages slice: system prompt + memory summaries + context chunks + conversation history (token-budget trimmed) + user message
6. Call `provider.Complete()` → stream `CompletionEvent`s → log `RunStep{type: model_call}` → emit SSE deltas to client
7. On `tool_call` event:
   - Look up tool in registry (native or MCP)
   - If `requires_approval`: update run to `approval_wait`, emit `approval_required` SSE, block until decision arrives
   - Execute tool with timeout → log `RunStep{type: tool_call}` → append result message → loop back to step 6
8. Emit `run_completed` SSE
9. Update `Run` record (`status=success`, token counts, cost estimate)
10. Async: summarise run, store new `Memory` records with embeddings

---

## MCP Client

- Connects to MCP servers via HTTP+SSE or stdio transport
- Calls `tools/list` on connect, stores results in `mcp_tools` table (prefixed `mcp_`)
- All MCP tool calls during agent runs go through `tools/executor.go` — same risk-check and approval-gate path as native tools; never called directly
- Server credentials stored AES-256-GCM encrypted in `mcp_servers.config`

---

## Connector Pipeline

`connector/pipeline.go` — `Sync(ctx, connectorID)`:

1. Load connector + credentials from DB
2. Fetch documents from provider (filesystem / Slack / Jira / GitHub / Google Drive)
3. For each document: hash content (skip if unchanged) → chunk (512 tokens, 64-token overlap) → embed via `provider.Embed()` → upsert into `connector_chunks`
4. Write `ConnectorSyncJob` record

During agent runs, context retrieval queries `connector_chunks` ordered by `embedding <=> query_embedding` (pgvector cosine distance), filtered to the agent's allowed connector list.

---

## SSE Stream Events

`POST /api/v1/conversations/:id/runs` returns a Server-Sent Events stream:

```
data: {"type":"run_started","run_id":"..."}
data: {"type":"step_completed","step":{"type":"memory_retrieval","latency_ms":38}}
data: {"type":"step_completed","step":{"type":"context_retrieval","chunks":3}}
data: {"type":"step_completed","step":{"type":"model_call","input_tokens":3210}}
data: {"type":"delta","content":"The answer is..."}
data: {"type":"tool_call","tool":"native_read_file","input":{"path":"readme.md"}}
data: {"type":"step_completed","step":{"type":"tool_call","tool":"native_read_file","latency_ms":14}}
data: {"type":"approval_required","tool":"native_write_file","input":{...},"approval_id":"..."}
data: {"type":"run_completed","run_id":"...","usage":{"input":5210,"output":580},"cost":0.012}
data: {"type":"error","message":"..."}
```

---

## Database Schema

Full schema: `infra/migrations/001_init.sql`

Key tables:

| Table | Purpose |
|-------|---------|
| `users`, `workspaces`, `roles`, `user_roles` | Auth and multi-tenancy |
| `provider_credentials` | Encrypted API keys, one per provider per workspace |
| `agents` | Agent configuration |
| `tools`, `agent_tools` | Tool registry + per-agent assignments |
| `mcp_servers`, `mcp_tools` | MCP server registry + discovered tools |
| `conversations`, `messages` | Chat history |
| `runs`, `run_steps` | Execution records + trace steps |
| `webhook_triggers` | Persistent inbound HTTP endpoints; each tied to an agent or workflow |
| `memories` | Vector memory (pgvector embedding column) |
| `connectors`, `connector_documents`, `connector_chunks` | Indexed external context |
| `context_retrieval_logs` | Which chunks were used in which run |
| `admin_audit_logs`, `policies` | Governance |

---

## REST API Surface

```
# Auth
POST   /api/v1/auth/register
POST   /api/v1/auth/login
POST   /api/v1/auth/refresh
POST   /api/v1/auth/logout
GET    /api/v1/auth/me

# Providers
GET    /api/v1/providers
POST   /api/v1/providers
PUT    /api/v1/providers/:id
DELETE /api/v1/providers/:id
GET    /api/v1/providers/:id/models

# Agents
GET    /api/v1/agents
POST   /api/v1/agents
GET    /api/v1/agents/:id
PUT    /api/v1/agents/:id
DELETE /api/v1/agents/:id
GET    /api/v1/agents/:id/tools
PUT    /api/v1/agents/:id/tools

# Tools
GET    /api/v1/tools
POST   /api/v1/tools
GET    /api/v1/tools/:id
PUT    /api/v1/tools/:id
DELETE /api/v1/tools/:id

# MCP Servers
GET    /api/v1/mcp-servers
POST   /api/v1/mcp-servers
GET    /api/v1/mcp-servers/:id
DELETE /api/v1/mcp-servers/:id
POST   /api/v1/mcp-servers/:id/sync
GET    /api/v1/mcp-servers/:id/tools

# Connectors
GET    /api/v1/connectors
POST   /api/v1/connectors
GET    /api/v1/connectors/:id
DELETE /api/v1/connectors/:id
POST   /api/v1/connectors/:id/sync
GET    /api/v1/connectors/:id/documents
GET    /api/v1/connectors/:id/sync-jobs

# Conversations & Runs
GET    /api/v1/conversations
POST   /api/v1/conversations
GET    /api/v1/conversations/:id
DELETE /api/v1/conversations/:id
POST   /api/v1/conversations/:id/runs    ← starts run, returns SSE stream
GET    /api/v1/conversations/:id/runs
GET    /api/v1/runs
GET    /api/v1/runs/:id
POST   /api/v1/runs/:id/approve
POST   /api/v1/runs/:id/cancel

# Memory
GET    /api/v1/memory
DELETE /api/v1/memory/:id
DELETE /api/v1/memory

# Agent Groups
GET    /api/v1/agent-groups
POST   /api/v1/agent-groups
GET    /api/v1/agent-groups/:id
PUT    /api/v1/agent-groups/:id
DELETE /api/v1/agent-groups/:id
POST   /api/v1/agent-groups/:id/runs

# Webhook Triggers (authenticated CRUD)
GET    /api/v1/webhook-triggers
POST   /api/v1/webhook-triggers
GET    /api/v1/webhook-triggers/:id
PUT    /api/v1/webhook-triggers/:id
DELETE /api/v1/webhook-triggers/:id

# Webhook Inbound (public — no auth)
POST   /webhook/:webhookId               ← fires a run; verifies HMAC-SHA256 if secret is set

# Admin
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
/                         → /dashboard
/login                    public
/dashboard
/agents                   agent list
/agents/new               create agent (tabbed form)
/agents/[id]/edit         edit agent
/playground               new conversation picker
/playground/[id]          chat + live SSE trace panel
/runs                     runs table with filters
/runs/[id]                run trace detail
/memory                   memory browser
/usage                    token / cost dashboard
/tools                    tool registry
/mcp-servers              MCP servers + discovered tools
/agent-groups             group list
/agent-groups/new         create group
/agent-groups/[id]        workflow canvas editor
/settings/providers       API key management
/settings/workspace       workspace settings + members
/admin/overview
/admin/users
/admin/workspaces
/admin/policies
/admin/audit-logs
```
