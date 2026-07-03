-- Split "indexed for retrieval" from "sessions may modify": repos adopted from
-- connector syncs arrive with sessions_enabled=false and need an explicit
-- enable (Settings → Claude Code → Repositories) before the session gate
-- passes. Explicit onboarding (UI add / catalog-ingest CLI) enables directly.
-- Existing rows default to disabled — provenance is unknown, so require the
-- one-click opt-in rather than guessing.
ALTER TABLE repo_catalog ADD COLUMN IF NOT EXISTS sessions_enabled BOOLEAN NOT NULL DEFAULT false;
