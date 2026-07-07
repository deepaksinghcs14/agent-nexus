-- Add a functional category to tools and skills so the UI can group and filter
-- them (Communication, Web & Search, Dev & Code, Data & HTTP, Memory & Context,
-- Orchestration, Knowledge, AI, General). Native tool categories are (re)seeded
-- from Go on every startup (Registry.SeedDB); this column just holds the value.
ALTER TABLE tools  ADD COLUMN IF NOT EXISTS category TEXT;
ALTER TABLE skills ADD COLUMN IF NOT EXISTS category TEXT;

-- Best-effort backfill for custom tools created before categories existed.
UPDATE tools SET category = 'Communication' WHERE category IS NULL AND name LIKE 'whatsapp_%';
UPDATE tools SET category = 'Dev & Code'    WHERE category IS NULL AND (name LIKE 'github_%' OR name LIKE 'jira_%' OR name LIKE 'code_%' OR type = 'code');
UPDATE tools SET category = 'Data & HTTP'   WHERE category IS NULL AND type = 'http';
