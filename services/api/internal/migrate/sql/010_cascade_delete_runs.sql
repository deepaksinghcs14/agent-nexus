-- Cascade delete runs (and their steps) when a conversation is deleted.
ALTER TABLE runs
    DROP CONSTRAINT IF EXISTS runs_conversation_id_fkey;
ALTER TABLE runs
    ADD CONSTRAINT runs_conversation_id_fkey
        FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE;

-- Run steps already cascade from runs, but ensure it explicitly.
ALTER TABLE run_steps
    DROP CONSTRAINT IF EXISTS run_steps_run_id_fkey;
ALTER TABLE run_steps
    ADD CONSTRAINT run_steps_run_id_fkey
        FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE;
