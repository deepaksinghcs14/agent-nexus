-- Add trigger_id FK to runs so webhook-triggered runs can be identified
ALTER TABLE runs ADD COLUMN IF NOT EXISTS trigger_id UUID REFERENCES webhook_triggers(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_runs_trigger_id ON runs(trigger_id) WHERE trigger_id IS NOT NULL;
