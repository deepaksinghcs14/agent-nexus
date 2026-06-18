-- Agent self-management: track which run created a resource (ownership + ephemeral cleanup).
-- Agents, skills, and HTTP tools created by a run carry source_run_id and an ephemeral flag.
-- After the root run completes, ephemeral resources are auto-deleted.

ALTER TABLE agents ADD COLUMN IF NOT EXISTS source_run_id UUID REFERENCES runs(id) ON DELETE SET NULL;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS ephemeral BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE skills ADD COLUMN IF NOT EXISTS source_run_id UUID REFERENCES runs(id) ON DELETE SET NULL;
ALTER TABLE skills ADD COLUMN IF NOT EXISTS ephemeral BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE tools  ADD COLUMN IF NOT EXISTS source_run_id UUID REFERENCES runs(id) ON DELETE SET NULL;
ALTER TABLE tools  ADD COLUMN IF NOT EXISTS ephemeral BOOLEAN NOT NULL DEFAULT false;
