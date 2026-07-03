-- Repo catalog for the Jira→PR pipeline: one row per onboarded repository.
-- The searchable content itself lives in connector_documents/connector_chunks
-- (populated by cmd/catalog-ingest); this table tracks catalog membership and
-- the docs-map maintenance cursor (Phase 5 idempotency).
CREATE TABLE IF NOT EXISTS repo_catalog (
    repo                TEXT NOT NULL,              -- owner/name
    workspace_id        UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    connector_id        UUID REFERENCES connectors(id) ON DELETE SET NULL,
    default_branch      TEXT NOT NULL DEFAULT 'main',
    last_map_commit_sha TEXT NOT NULL DEFAULT '',
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (workspace_id, repo)
);
