-- Agent Nexus — Initial Schema
-- Migration: 001_init.sql

CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "vector";

-- ============================================================
-- USERS & AUTH
-- ============================================================

CREATE TABLE users (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email        TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    full_name    TEXT NOT NULL DEFAULT '',
    avatar_url   TEXT NOT NULL DEFAULT '',
    is_active    BOOLEAN NOT NULL DEFAULT true,
    is_admin     BOOLEAN NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE workspaces (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    owner_id     UUID NOT NULL REFERENCES users(id),
    settings     JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE roles (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,  -- owner | admin | member | viewer
    permissions  JSONB NOT NULL DEFAULT '[]',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workspace_id, name)
);

CREATE TABLE user_roles (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    role_id      UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, workspace_id)
);

CREATE TABLE refresh_tokens (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- PROVIDER CREDENTIALS
-- ============================================================

CREATE TABLE provider_credentials (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id   UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    provider       TEXT NOT NULL,  -- anthropic | openai | gemini | ollama
    display_name   TEXT NOT NULL DEFAULT '',
    encrypted_key  TEXT NOT NULL,  -- AES-256-GCM encrypted API key
    base_url       TEXT NOT NULL DEFAULT '',  -- for ollama / custom endpoints
    is_active      BOOLEAN NOT NULL DEFAULT true,
    created_by     UUID NOT NULL REFERENCES users(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workspace_id, provider)
);

-- ============================================================
-- AGENTS
-- ============================================================

CREATE TABLE agents (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id              UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name                      TEXT NOT NULL,
    description               TEXT NOT NULL DEFAULT '',
    instructions              TEXT NOT NULL DEFAULT '',  -- system prompt
    provider                  TEXT NOT NULL DEFAULT 'anthropic',
    model                     TEXT NOT NULL DEFAULT 'claude-sonnet-4-20250514',
    temperature               FLOAT NOT NULL DEFAULT 0.7,
    max_tokens                INT NOT NULL DEFAULT 4096,
    memory_enabled            BOOLEAN NOT NULL DEFAULT false,
    memory_scope              TEXT NOT NULL DEFAULT 'agent',  -- conversation | agent | workspace
    context_retrieval_enabled BOOLEAN NOT NULL DEFAULT false,
    max_steps                 INT NOT NULL DEFAULT 10,
    max_tool_calls            INT NOT NULL DEFAULT 10,
    max_duration_secs         INT NOT NULL DEFAULT 120,
    status                    TEXT NOT NULL DEFAULT 'active',  -- active | paused | archived
    created_by                UUID NOT NULL REFERENCES users(id),
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- TOOLS
-- ============================================================

CREATE TABLE tools (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id     UUID REFERENCES workspaces(id) ON DELETE CASCADE,  -- NULL = built-in global tool
    name             TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    type             TEXT NOT NULL DEFAULT 'native',  -- native | mcp | http
    input_schema     JSONB NOT NULL DEFAULT '{}',
    output_schema    JSONB NOT NULL DEFAULT '{}',
    risk_level       TEXT NOT NULL DEFAULT 'low',    -- low | medium | high | critical
    requires_approval BOOLEAN NOT NULL DEFAULT false,
    timeout_ms       INT NOT NULL DEFAULT 30000,
    enabled          BOOLEAN NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workspace_id, name)
);

CREATE TABLE agent_tools (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id     UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tool_id      UUID NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    enabled      BOOLEAN NOT NULL DEFAULT true,
    overrides    JSONB NOT NULL DEFAULT '{}',  -- per-agent overrides: approval, timeout
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (agent_id, tool_id)
);

-- ============================================================
-- MCP SERVERS
-- ============================================================

CREATE TABLE mcp_servers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    url             TEXT NOT NULL,  -- e.g. http://localhost:3001 or stdio path
    transport       TEXT NOT NULL DEFAULT 'http',  -- http | stdio
    status          TEXT NOT NULL DEFAULT 'disconnected',  -- connected | disconnected | error
    config          JSONB NOT NULL DEFAULT '{}',   -- encrypted creds stored here
    tools_synced_at TIMESTAMPTZ,
    created_by      UUID NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE mcp_tools (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id    UUID NOT NULL REFERENCES mcp_servers(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    input_schema JSONB NOT NULL DEFAULT '{}',
    risk_level   TEXT NOT NULL DEFAULT 'medium',
    enabled      BOOLEAN NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (server_id, name)
);

-- ============================================================
-- CONVERSATIONS & MESSAGES
-- ============================================================

CREATE TABLE conversations (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id     UUID NOT NULL REFERENCES agents(id),
    user_id      UUID NOT NULL REFERENCES users(id),
    title        TEXT NOT NULL DEFAULT 'New conversation',
    message_count INT NOT NULL DEFAULT 0,
    token_count   INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL,  -- user | assistant | tool
    content         TEXT NOT NULL DEFAULT '',
    tool_calls      JSONB,          -- present when role=assistant and model called tools
    tool_call_id    TEXT,           -- present when role=tool (result)
    tool_name       TEXT,
    tokens          INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- RUNS & TRACE
-- ============================================================

CREATE TABLE runs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id            UUID NOT NULL REFERENCES agents(id),
    conversation_id     UUID NOT NULL REFERENCES conversations(id),
    user_id             UUID NOT NULL REFERENCES users(id),
    input               TEXT NOT NULL DEFAULT '',
    output              TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL DEFAULT 'pending',
        -- pending | running | success | failed | cancelled | approval_wait
    started_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ,
    total_input_tokens  INT NOT NULL DEFAULT 0,
    total_output_tokens INT NOT NULL DEFAULT 0,
    cost_estimate       FLOAT NOT NULL DEFAULT 0,
    error_message       TEXT NOT NULL DEFAULT '',
    metadata            JSONB NOT NULL DEFAULT '{}'
);

CREATE TABLE run_steps (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id      UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    step_type   TEXT NOT NULL,
        -- memory_retrieval | context_retrieval | model_call | tool_call
        -- mcp_call | approval_wait | final_response | error
    input       JSONB NOT NULL DEFAULT '{}',
    output      JSONB NOT NULL DEFAULT '{}',
    latency_ms  INT NOT NULL DEFAULT 0,
    tokens_used INT NOT NULL DEFAULT 0,
    tool_name   TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE approval_requests (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id      UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    tool_name   TEXT NOT NULL,
    tool_input  JSONB NOT NULL DEFAULT '{}',
    status      TEXT NOT NULL DEFAULT 'pending',  -- pending | approved | rejected
    decided_by  UUID REFERENCES users(id),
    decided_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- MEMORY
-- ============================================================

CREATE TABLE memories (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id        UUID REFERENCES agents(id) ON DELETE CASCADE,  -- NULL = workspace memory
    user_id         UUID REFERENCES users(id) ON DELETE CASCADE,
    scope           TEXT NOT NULL DEFAULT 'agent',  -- conversation | agent | workspace
    content         TEXT NOT NULL,
    embedding       vector(1536),
    relevance_score FLOAT NOT NULL DEFAULT 0,
    source_run_id   UUID REFERENCES runs(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX memories_embedding_idx ON memories
    USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- ============================================================
-- CONNECTORS
-- ============================================================

CREATE TABLE connectors (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    provider     TEXT NOT NULL,
        -- filesystem | slack | jira | confluence | github | gdrive | notion | gmail | postgres | mongo | s3
    type         TEXT NOT NULL DEFAULT 'native',  -- native | mcp
    auth_type    TEXT NOT NULL DEFAULT 'api_key', -- oauth | api_key | pat | basic | none
    status       TEXT NOT NULL DEFAULT 'disconnected',
        -- connected | disconnected | syncing | error
    config       JSONB NOT NULL DEFAULT '{}',     -- non-sensitive config
    created_by   UUID NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE connector_accounts (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    connector_id          UUID NOT NULL REFERENCES connectors(id) ON DELETE CASCADE,
    account_identifier    TEXT NOT NULL DEFAULT '',
    encrypted_credentials TEXT NOT NULL,  -- AES-256-GCM
    scopes                JSONB NOT NULL DEFAULT '[]',
    expires_at            TIMESTAMPTZ,
    refresh_required      BOOLEAN NOT NULL DEFAULT false,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE connector_sync_jobs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    connector_id      UUID NOT NULL REFERENCES connectors(id) ON DELETE CASCADE,
    status            TEXT NOT NULL DEFAULT 'pending',
        -- pending | running | completed | failed
    started_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ,
    documents_found   INT NOT NULL DEFAULT 0,
    documents_indexed INT NOT NULL DEFAULT 0,
    error_message     TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE connector_documents (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    connector_id       UUID NOT NULL REFERENCES connectors(id) ON DELETE CASCADE,
    workspace_id       UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    source             TEXT NOT NULL,  -- slack | jira | confluence | github | etc
    source_document_id TEXT NOT NULL,
    title              TEXT NOT NULL DEFAULT '',
    url                TEXT NOT NULL DEFAULT '',
    author             TEXT NOT NULL DEFAULT '',
    content_hash       TEXT NOT NULL DEFAULT '',
    last_modified_at   TIMESTAMPTZ,
    indexed_at         TIMESTAMPTZ,
    metadata           JSONB NOT NULL DEFAULT '{}',
    UNIQUE (connector_id, source_document_id)
);

CREATE TABLE connector_chunks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES connector_documents(id) ON DELETE CASCADE,
    chunk_index INT NOT NULL,
    content     TEXT NOT NULL,
    embedding   vector(1536),
    metadata    JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (document_id, chunk_index)
);

CREATE INDEX connector_chunks_embedding_idx ON connector_chunks
    USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

CREATE TABLE context_retrieval_logs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id       UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    connector_id UUID NOT NULL REFERENCES connectors(id),
    document_id  UUID NOT NULL REFERENCES connector_documents(id),
    chunk_id     UUID NOT NULL REFERENCES connector_chunks(id),
    score        FLOAT NOT NULL DEFAULT 0,
    retrieved_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Allowed connectors per agent (set via agent context tab)
CREATE TABLE agent_connectors (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id     UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    connector_id UUID NOT NULL REFERENCES connectors(id) ON DELETE CASCADE,
    enabled      BOOLEAN NOT NULL DEFAULT true,
    max_chunks   INT NOT NULL DEFAULT 8,
    min_score    FLOAT NOT NULL DEFAULT 0.75,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (agent_id, connector_id)
);

-- ============================================================
-- AGENT GROUPS
-- ============================================================

CREATE TABLE agent_groups (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    mode         TEXT NOT NULL DEFAULT 'pipeline',  -- pipeline | supervisor
    status       TEXT NOT NULL DEFAULT 'active',
    created_by   UUID NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE agent_group_members (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id     UUID NOT NULL REFERENCES agent_groups(id) ON DELETE CASCADE,
    agent_id     UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    position     INT NOT NULL DEFAULT 0,  -- order in pipeline
    role         TEXT NOT NULL DEFAULT 'member',  -- member | supervisor (for supervisor mode)
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (group_id, agent_id)
);

-- ============================================================
-- ADMIN & GOVERNANCE
-- ============================================================

CREATE TABLE admin_audit_logs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID REFERENCES workspaces(id),
    actor_id      UUID REFERENCES users(id),
    actor_email   TEXT NOT NULL DEFAULT '',
    action        TEXT NOT NULL,
        -- e.g. agent.created | tool.blocked | credential.rotated | connector.synced
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id   TEXT NOT NULL DEFAULT '',
    metadata      JSONB NOT NULL DEFAULT '{}',
    ip_address    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE policies (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,  -- NULL = global policy
    key          TEXT NOT NULL,
    value        JSONB NOT NULL DEFAULT '{}',
    created_by   UUID REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workspace_id, key)
);

-- Default global policies
INSERT INTO policies (workspace_id, key, value) VALUES
    (NULL, 'allowed_providers',          '["anthropic","openai"]'),
    (NULL, 'blocked_tools',              '["execute_command"]'),
    (NULL, 'approval_required_tools',    '["write_file","http_request","send_email"]'),
    (NULL, 'max_tokens_per_run',         '32000'),
    (NULL, 'max_runs_per_day_per_user',  '200'),
    (NULL, 'memory_retention_days',      '90'),
    (NULL, 'audit_log_retention_days',   '365');

-- ============================================================
-- INDEXES
-- ============================================================

CREATE INDEX agents_workspace_idx ON agents(workspace_id);
CREATE INDEX agents_status_idx ON agents(status);
CREATE INDEX messages_conversation_idx ON messages(conversation_id);
CREATE INDEX runs_agent_idx ON runs(agent_id);
CREATE INDEX runs_workspace_idx ON runs(workspace_id);
CREATE INDEX runs_status_idx ON runs(status);
CREATE INDEX runs_started_at_idx ON runs(started_at DESC);
CREATE INDEX run_steps_run_idx ON run_steps(run_id);
CREATE INDEX memories_agent_idx ON memories(agent_id);
CREATE INDEX memories_workspace_idx ON memories(workspace_id);
CREATE INDEX memories_scope_idx ON memories(scope);
CREATE INDEX connector_documents_connector_idx ON connector_documents(connector_id);
CREATE INDEX connector_chunks_document_idx ON connector_chunks(document_id);
CREATE INDEX context_retrieval_logs_run_idx ON context_retrieval_logs(run_id);
CREATE INDEX audit_logs_workspace_idx ON admin_audit_logs(workspace_id);
CREATE INDEX audit_logs_created_at_idx ON admin_audit_logs(created_at DESC);
