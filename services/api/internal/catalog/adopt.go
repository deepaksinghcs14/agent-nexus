package catalog

import (
	"context"
	"fmt"
	"strings"

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

	// Adopted repos arrive with sessions DISABLED: syncing makes a repo
	// known/searchable, but write access requires an explicit enable in
	// Settings → Claude Code (or explicit onboarding).
	tag, err := pool.Exec(ctx, `
		INSERT INTO repo_catalog(repo, workspace_id, connector_id, default_branch, sessions_enabled)
		SELECT DISTINCT split_part(cd.source_document_id,'/',1) || '/' || split_part(cd.source_document_id,'/',2),
		       $2::uuid, $1::uuid, 'main', false
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

	// Every allowlisted repo needs at least a name card in the catalog, or
	// repo selection can't find adopted-but-never-ingested repos at all.
	if err := WriteCatalogCards(ctx, pool, workspaceID); err != nil {
		return int(tag.RowsAffected()), fmt.Errorf("write catalog cards: %w", err)
	}

	return int(tag.RowsAffected()), nil
}

// WriteCatalogCards upserts one small searchable "card" document per
// repo_catalog row into the workspace's repo-catalog connector: the repo's
// full name plus its name split into words, so keyword queries like "aadhaar
// scanner" match the repo Bureau-Inc/aadhaar-qr-scanner-sdk even before the
// repo is explicitly ingested. Idempotent (content-hash skip in
// upsertDocument); explicit ingestion adds the richer docs on top.
func WriteCatalogCards(ctx context.Context, pool *pgxpool.Pool, workspaceID string) error {
	connectorID, err := ensureConnector(ctx, pool, workspaceID)
	if err != nil {
		return err
	}
	rows, err := pool.Query(ctx, `
		SELECT repo, default_branch, sessions_enabled FROM repo_catalog
		WHERE workspace_id=$1::uuid ORDER BY repo`, workspaceID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type entry struct {
		repo, branch string
		enabled      bool
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if rows.Scan(&e.repo, &e.branch, &e.enabled) == nil {
			entries = append(entries, e)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, e := range entries {
		sessions := "sessions disabled (indexed only — enable in Settings → Claude Code to allow coding sessions)"
		if e.enabled {
			sessions = "sessions enabled"
		}
		card := doc{
			path:  "_catalog_card",
			title: e.repo + ": repository",
			content: fmt.Sprintf(
				"Repository %s (default branch %s, %s).\nName terms: %s",
				e.repo, e.branch, sessions, strings.Join(nameTerms(e.repo), " ")),
		}
		if _, err := upsertDocument(ctx, pool, connectorID, workspaceID, e.repo, card); err != nil {
			return fmt.Errorf("card for %s: %w", e.repo, err)
		}
	}
	return nil
}

// nameTerms splits an owner/name repo slug into searchable words:
// "Bureau-Inc/aadhaar-qr-scanner-sdk" → [bureau inc aadhaar qr scanner sdk].
func nameTerms(repo string) []string {
	var terms []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			terms = append(terms, b.String())
			b.Reset()
		}
	}
	for _, r := range strings.ToLower(repo) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return terms
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
