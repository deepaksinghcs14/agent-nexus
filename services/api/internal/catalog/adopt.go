package catalog

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AdoptFromConnector populates the workspace's session allowlist
// (repo_catalog) from a GitHub connector's already-indexed documents: every
// distinct owner/repo the connector has synced becomes an onboarded repo, and
// the connector is linked to the pipeline orchestrator. Idempotent; returns
// how many repos were newly adopted.
//
// This is what makes a user-created GitHub connector the single source of
// intent — syncing it onboards its repos, no separate per-repo step. GitHub
// connectors use SourceDocumentID "{owner}/{repo}/{path}", which is what the
// extraction below relies on; other providers are skipped.
func AdoptFromConnector(ctx context.Context, pool *pgxpool.Pool, connectorID string) (int, error) {
	var provider, workspaceID string
	if err := pool.QueryRow(ctx,
		`SELECT provider, workspace_id::text FROM connectors WHERE id=$1::uuid`, connectorID).
		Scan(&provider, &workspaceID); err != nil {
		return 0, fmt.Errorf("connector not found: %w", err)
	}
	if provider != "github" {
		return 0, nil
	}

	tag, err := pool.Exec(ctx, `
		INSERT INTO repo_catalog(repo, workspace_id, connector_id, default_branch)
		SELECT DISTINCT split_part(cd.source_document_id,'/',1) || '/' || split_part(cd.source_document_id,'/',2),
		       $2::uuid, $1::uuid, 'main'
		FROM connector_documents cd
		WHERE cd.connector_id = $1::uuid
		  AND cd.source_document_id LIKE '%/%/%'
		ON CONFLICT (workspace_id, repo) DO NOTHING`,
		connectorID, workspaceID)
	if err != nil {
		return 0, err
	}

	_, _ = pool.Exec(ctx, `
		INSERT INTO agent_connectors(agent_id, connector_id, enabled, max_chunks, min_score)
		SELECT a.id, $1::uuid, true, 10, 0.3 FROM agents a
		WHERE a.workspace_id=$2::uuid AND a.name='Jira Pipeline Orchestrator'
		ON CONFLICT (agent_id, connector_id) DO NOTHING`,
		connectorID, workspaceID)

	return int(tag.RowsAffected()), nil
}

// AdoptAllGithubConnectors runs adoption for every GitHub connector — used at
// startup so repos synced before this feature (or while the API was down) get
// onboarded without waiting for their next sync.
func AdoptAllGithubConnectors(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	rows, err := pool.Query(ctx, `SELECT id::text FROM connectors WHERE provider='github'`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	total := 0
	for _, id := range ids {
		n, err := AdoptFromConnector(ctx, pool, id)
		if err != nil {
			continue
		}
		total += n
	}
	return total, nil
}
