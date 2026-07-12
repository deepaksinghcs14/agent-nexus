-- Repo-scoped lessons: distilled findings from past review sessions, injected
-- into future coding sessions for the same repo so mistakes are not repeated.
ALTER TABLE repo_catalog ADD COLUMN IF NOT EXISTS lessons TEXT NOT NULL DEFAULT '';
