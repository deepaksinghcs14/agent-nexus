ALTER TABLE runs
    ADD COLUMN IF NOT EXISTS parent_run_id    UUID REFERENCES runs(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS workflow_node_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS runs_parent_run_id_idx ON runs(parent_run_id) WHERE parent_run_id IS NOT NULL;
