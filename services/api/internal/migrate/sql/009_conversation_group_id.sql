-- Track which agent group originated a group-run conversation.
-- Enables per-group run counts without relying on title string matching.
ALTER TABLE conversations
    ADD COLUMN IF NOT EXISTS agent_group_id UUID REFERENCES agent_groups(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_conversations_agent_group_id ON conversations(agent_group_id);
