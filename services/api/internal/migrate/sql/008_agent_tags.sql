-- Add tags array to agents for grouping/categorization
ALTER TABLE agents ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';

-- Index for tag filtering
CREATE INDEX IF NOT EXISTS idx_agents_tags ON agents USING GIN(tags);
