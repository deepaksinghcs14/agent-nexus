-- Workspace-level GitHub token for the Jira→PR pipeline. Stored encrypted
-- alongside the Claude token; either credential can be set independently, so
-- claude_token loses its NOT NULL. A workspace token takes precedence over the
-- instance-level GITHUB_TOKEN env (which becomes a single-tenant fallback).
ALTER TABLE runner_credentials ALTER COLUMN claude_token DROP NOT NULL;
ALTER TABLE runner_credentials ADD COLUMN IF NOT EXISTS github_token TEXT;
