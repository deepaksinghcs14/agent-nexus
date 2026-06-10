CREATE TABLE webhook_triggers (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id      UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name              VARCHAR(255) NOT NULL,
  description       TEXT NOT NULL DEFAULT '',
  target_type       VARCHAR(50) NOT NULL CHECK (target_type IN ('agent', 'workflow')),
  target_id         UUID NOT NULL,
  input_template    TEXT NOT NULL DEFAULT '{{.RawBody}}',
  secret            TEXT,
  is_active         BOOLEAN NOT NULL DEFAULT true,
  created_by        UUID REFERENCES users(id),
  created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_triggered_at TIMESTAMPTZ,
  trigger_count     BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX webhook_triggers_workspace_idx ON webhook_triggers(workspace_id);
