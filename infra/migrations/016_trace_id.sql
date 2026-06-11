-- Add trace_id to runs: every sub-run in a workflow execution shares the root run's ID
-- as trace_id, enabling "SELECT WHERE trace_id = $root" to retrieve the full tree.
-- Root runs keep trace_id NULL (they ARE the trace root).
ALTER TABLE runs ADD COLUMN IF NOT EXISTS trace_id UUID REFERENCES runs(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS runs_trace_id_idx ON runs(trace_id) WHERE trace_id IS NOT NULL;
