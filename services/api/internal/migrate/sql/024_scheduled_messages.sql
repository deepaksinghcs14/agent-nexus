CREATE TABLE IF NOT EXISTS gateway_scheduled_messages (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id     UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    channel_id       UUID NOT NULL REFERENCES gateway_channels(id) ON DELETE CASCADE,
    contact_id       UUID REFERENCES gateway_contacts(id) ON DELETE SET NULL,
    account_id       TEXT NOT NULL DEFAULT 'default',
    peer_kind        TEXT NOT NULL DEFAULT 'direct',
    peer_id          TEXT NOT NULL,
    message          TEXT NOT NULL,
    send_at          TIMESTAMPTZ NOT NULL,
    status           TEXT NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending','sent','failed','cancelled')),
    recurrence_rule  JSONB,
    occurrence_count INT NOT NULL DEFAULT 0,
    last_error       TEXT NOT NULL DEFAULT '',
    created_by       TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS gsm_workspace_idx ON gateway_scheduled_messages(workspace_id);
CREATE INDEX IF NOT EXISTS gsm_channel_idx   ON gateway_scheduled_messages(channel_id);
CREATE INDEX IF NOT EXISTS gsm_status_idx    ON gateway_scheduled_messages(status);
CREATE INDEX IF NOT EXISTS gsm_send_at_idx   ON gateway_scheduled_messages(send_at) WHERE status = 'pending';
