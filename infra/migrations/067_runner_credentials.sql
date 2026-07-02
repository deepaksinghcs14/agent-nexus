-- Per-workspace Claude account credential for repo sessions: the long-lived
-- OAuth token produced by `claude setup-token` (subscription billing), stored
-- AES-256-GCM encrypted. Injected per session-launch into the runner so no
-- static Anthropic secret needs to live on the runner service.
CREATE TABLE IF NOT EXISTS runner_credentials (
    workspace_id UUID PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    claude_token TEXT NOT NULL,
    updated_by   UUID REFERENCES users(id),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
