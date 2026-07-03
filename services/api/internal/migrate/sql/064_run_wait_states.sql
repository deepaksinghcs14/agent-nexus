-- Durable approval-wait state. A run goroutine snapshots its loop state here
-- before blocking on an approval decision, so the run can be resumed after a
-- process restart (or after the in-process wait times out and parks).
CREATE TABLE IF NOT EXISTS run_wait_states (
    run_id     UUID PRIMARY KEY REFERENCES runs(id) ON DELETE CASCADE,
    wait_type  TEXT NOT NULL DEFAULT 'approval',
    state      JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
