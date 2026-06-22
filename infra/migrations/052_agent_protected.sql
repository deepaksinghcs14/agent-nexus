ALTER TABLE agents ADD COLUMN IF NOT EXISTS protected boolean NOT NULL DEFAULT false;

UPDATE agents SET protected = true WHERE name = 'Nexus Orchestrator';
