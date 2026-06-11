-- Allow conversations and runs to exist without a single owning agent.
-- Group runs span multiple agents so agent_id is meaningless in that context
-- and was causing FK failures on group invocations.

ALTER TABLE conversations
    ALTER COLUMN agent_id DROP NOT NULL,
    DROP CONSTRAINT IF EXISTS conversations_agent_id_fkey;
ALTER TABLE conversations
    ADD CONSTRAINT conversations_agent_id_fkey
        FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE SET NULL;

ALTER TABLE runs
    ALTER COLUMN agent_id DROP NOT NULL,
    DROP CONSTRAINT IF EXISTS runs_agent_id_fkey;
ALTER TABLE runs
    ADD CONSTRAINT runs_agent_id_fkey
        FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE SET NULL;
