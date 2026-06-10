-- Add config column to tools for storing HTTP tool runtime settings
-- (URL, method, headers, query params, body mode/template)
ALTER TABLE tools ADD COLUMN IF NOT EXISTS config JSONB NOT NULL DEFAULT '{}';
