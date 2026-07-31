-- Repo-scoped architecture map: a cached, terse orientation summary (dirs,
-- entrypoints, conventions) so coding sessions stop re-exploring the same
-- repo structure from a cold clone on every ticket.
ALTER TABLE repo_catalog ADD COLUMN IF NOT EXISTS repo_map TEXT NOT NULL DEFAULT '';
ALTER TABLE repo_catalog ADD COLUMN IF NOT EXISTS repo_map_updated_at TIMESTAMPTZ;
