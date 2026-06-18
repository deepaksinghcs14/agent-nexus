CREATE TABLE IF NOT EXISTS gateway_contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES gateway_channels(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL DEFAULT 'default',
    display_name TEXT NOT NULL,
    alias TEXT NOT NULL DEFAULT '',
    phone_number TEXT NOT NULL DEFAULT '',
    whatsapp_jid TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL CHECK (role IN ('owner', 'trusted', 'blocked')),
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    auto_reply_enabled BOOLEAN NOT NULL DEFAULT true,
    last_matched_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS gateway_contacts_workspace_idx ON gateway_contacts(workspace_id);
CREATE INDEX IF NOT EXISTS gateway_contacts_channel_idx ON gateway_contacts(channel_id);
CREATE INDEX IF NOT EXISTS gateway_contacts_alias_idx ON gateway_contacts(channel_id, alias);
CREATE UNIQUE INDEX IF NOT EXISTS gateway_contacts_jid_unique
    ON gateway_contacts(channel_id, account_id, whatsapp_jid)
    WHERE whatsapp_jid <> '';
CREATE UNIQUE INDEX IF NOT EXISTS gateway_contacts_phone_unique
    ON gateway_contacts(channel_id, account_id, phone_number)
    WHERE phone_number <> '';
