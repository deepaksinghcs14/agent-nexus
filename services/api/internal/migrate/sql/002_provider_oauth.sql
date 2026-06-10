-- 002_provider_oauth.sql
-- Adds OAuth support columns to provider_credentials and a short-lived oauth_states table.

ALTER TABLE provider_credentials
  ADD COLUMN IF NOT EXISTS auth_type           TEXT NOT NULL DEFAULT 'api_key',
  ADD COLUMN IF NOT EXISTS oauth_access_token  TEXT,
  ADD COLUMN IF NOT EXISTS oauth_refresh_token TEXT,
  ADD COLUMN IF NOT EXISTS oauth_token_expiry  TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS oauth_scopes        TEXT[];

-- encrypted_key may be empty for oauth rows
ALTER TABLE provider_credentials ALTER COLUMN encrypted_key SET DEFAULT '';

CREATE TABLE IF NOT EXISTS oauth_states (
  state        TEXT PRIMARY KEY,
  workspace_id UUID NOT NULL,
  user_id      UUID NOT NULL,
  expires_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS oauth_states_expires_at ON oauth_states(expires_at);
