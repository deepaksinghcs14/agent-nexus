-- Resumable state for in-flight workflow (agent group) runs. Persisted after
-- every node completes so a crashed/restarted API process can resume a
-- workflow from where it left off instead of losing all progress. Mirrors
-- the run_wait_states pattern used for approval/session resume, applied to
-- the graph walk's progress instead of a single agent's tool-loop state.
CREATE TABLE IF NOT EXISTS workflow_checkpoints (
    run_id          UUID PRIMARY KEY REFERENCES runs(id) ON DELETE CASCADE,
    workflow_id     UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    node_outputs    JSONB NOT NULL DEFAULT '{}',
    loop_iterations JSONB NOT NULL DEFAULT '{}',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
