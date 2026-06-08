-- API tokens for programmatic access
-- Token format: "anx_" + 40 hex chars (stored as SHA-256 hash)

CREATE TABLE api_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id)      ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,   -- SHA-256(raw_token)
    token_prefix TEXT NOT NULL,          -- first 12 chars of raw token, shown in UI
    scopes       TEXT[] NOT NULL DEFAULT '{}',
    last_used_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ,            -- NULL = never expires
    revoked_at   TIMESTAMPTZ,            -- NULL = active
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX ON api_tokens(token_hash);
CREATE INDEX ON api_tokens(workspace_id, user_id);
