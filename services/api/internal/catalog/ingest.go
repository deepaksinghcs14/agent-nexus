// Package catalog onboards GitHub repositories into the Jira→PR pipeline's
// repo catalog: it clones the repo, extracts the documentation surface
// (README, docs/, architecture notes, module manifests, and a generated
// file-tree summary), writes it as searchable chunks into the workspace's
// "repo-catalog" connector, records the repo in the repo_catalog allowlist,
// and links the connector to the seeded pipeline orchestrator.
//
// Embeddings are optional: chunks are stored without vectors and served by
// the retriever's full-text fallback, so no embedding provider is required.
// Used by both the repo-catalog API (server-side, workspace GitHub token) and
// the catalog-ingest CLI (env token, headless installs).
package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ConnectorName = "repo-catalog"
	chunkSize     = 2048
	chunkOverlap  = 256
	maxDocBytes   = 512 * 1024
)

// Request describes one repo onboarding.
type Request struct {
	WorkspaceID string
	Repo        string // owner/name
	Branch      string // optional; default = repo default branch
	Token       string // GitHub token; may be empty for public repos
	CloneURL    string // optional override (local path, GHE) — used verbatim when set
}

// Result reports what the ingestion did.
type Result struct {
	Repo        string `json:"repo"`
	Branch      string `json:"branch"`
	Documents   int    `json:"documents"`
	Chunks      int    `json:"chunks"`
	ConnectorID string `json:"connector_id"`
}

// Ingest onboards (or refreshes) one repository. Idempotent: unchanged
// documents are skipped, changed ones re-chunked, and the repo_catalog row
// upserted.
func Ingest(ctx context.Context, pool *pgxpool.Pool, req Request) (*Result, error) {
	if strings.Count(req.Repo, "/") != 1 || strings.Contains(req.Repo, "..") || strings.ContainsAny(req.Repo, " \t\n") {
		return nil, fmt.Errorf("repo must be owner/name")
	}

	dir, err := os.MkdirTemp("", "catalog-ingest-*")
	if err != nil {
		return nil, fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	src := req.CloneURL
	if src == "" {
		if req.Token != "" {
			src = fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", req.Token, req.Repo)
		} else {
			src = fmt.Sprintf("https://github.com/%s.git", req.Repo)
		}
	}
	args := []string{"clone", "--depth", "1"}
	if req.Branch != "" {
		args = append(args, "--branch", req.Branch)
	}
	args = append(args, src, dir)
	if out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput(); err != nil {
		detail := strings.ReplaceAll(string(out), req.Token, "***")
		if len(detail) > 300 {
			detail = detail[:300] + "…"
		}
		return nil, fmt.Errorf("git clone failed: %s", detail)
	}

	headBranch := req.Branch
	if headBranch == "" {
		if b, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
			headBranch = strings.TrimSpace(string(b))
		} else {
			headBranch = "main"
		}
	}

	docs := collectDocs(dir, req.Repo)
	if len(docs) == 0 {
		return nil, fmt.Errorf("no documentation content found in repo")
	}

	connectorID, err := ensureConnector(ctx, pool, req.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("ensure connector: %w", err)
	}

	totalChunks := 0
	for _, d := range docs {
		n, err := upsertDocument(ctx, pool, connectorID, req.WorkspaceID, req.Repo, d)
		if err != nil {
			return nil, fmt.Errorf("upsert %s: %w", d.path, err)
		}
		totalChunks += n
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO repo_catalog(repo, workspace_id, connector_id, default_branch, updated_at)
		VALUES ($1, $2::uuid, $3::uuid, $4, NOW())
		ON CONFLICT (workspace_id, repo) DO UPDATE SET connector_id=$3::uuid, default_branch=$4, updated_at=NOW()`,
		req.Repo, req.WorkspaceID, connectorID, headBranch); err != nil {
		return nil, fmt.Errorf("upsert repo_catalog: %w", err)
	}

	// Link the catalog to the workspace's seeded pipeline orchestrator so
	// onboarding is the only step needed for repo selection to work.
	_, _ = pool.Exec(ctx, `
		INSERT INTO agent_connectors(agent_id, connector_id, enabled, max_chunks, min_score)
		SELECT a.id, $1::uuid, true, 10, 0.3 FROM agents a
		WHERE a.workspace_id=$2::uuid AND a.name='Jira Pipeline Orchestrator'
		ON CONFLICT (agent_id, connector_id) DO NOTHING`,
		connectorID, req.WorkspaceID)

	return &Result{
		Repo: req.Repo, Branch: headBranch,
		Documents: len(docs), Chunks: totalChunks, ConnectorID: connectorID,
	}, nil
}

// Remove deletes a repo from the catalog: the allowlist row and its indexed
// documents (chunks cascade).
func Remove(ctx context.Context, pool *pgxpool.Pool, workspaceID, repo string) error {
	var connectorID string
	err := pool.QueryRow(ctx,
		`DELETE FROM repo_catalog WHERE workspace_id=$1::uuid AND repo=$2 RETURNING COALESCE(connector_id::text,'')`,
		workspaceID, repo).Scan(&connectorID)
	if err != nil {
		return fmt.Errorf("repo not found in catalog")
	}
	if connectorID != "" {
		pool.Exec(ctx, //nolint:errcheck
			`DELETE FROM connector_documents WHERE connector_id=$1::uuid AND source_document_id LIKE $2`,
			connectorID, repo+"/%")
	}
	return nil
}

type doc struct {
	path    string // repo-relative path, or a synthetic name like "_file_tree"
	title   string
	content string
}

// collectDocs gathers the repo's documentation surface: README/architecture
// files anywhere in the tree (depth-limited), everything under docs/, module
// manifests, and a synthesized file-tree overview.
func collectDocs(dir, repo string) []doc {
	var docs []doc
	var treeLines []string

	isDocFile := func(name string) bool {
		lower := strings.ToLower(name)
		switch {
		case strings.HasPrefix(lower, "readme"),
			strings.HasPrefix(lower, "architecture"),
			strings.HasPrefix(lower, "contributing"),
			lower == "claude.md", lower == "llms.txt", lower == "agents.md":
			return true
		}
		return false
	}
	isManifest := func(name string) bool {
		switch name {
		case "go.mod", "package.json", "pyproject.toml", "Cargo.toml", "pom.xml", "build.gradle":
			return true
		}
		return false
	}

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		if rel == "." {
			return nil
		}
		depth := strings.Count(rel, string(os.PathSeparator))
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			if depth <= 2 {
				treeLines = append(treeLines, rel+"/")
			}
			return nil
		}
		if depth <= 2 {
			treeLines = append(treeLines, rel)
		}
		inDocsDir := strings.HasPrefix(rel, "docs"+string(os.PathSeparator)) && strings.HasSuffix(strings.ToLower(rel), ".md")
		if depth <= 3 && (isDocFile(d.Name()) || isManifest(d.Name())) || inDocsDir {
			if info, err := d.Info(); err == nil && info.Size() <= maxDocBytes {
				if b, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(b))) > 0 {
					docs = append(docs, doc{path: rel, title: repo + ": " + rel, content: string(b)})
				}
			}
		}
		return nil
	})

	sort.Strings(treeLines)
	docs = append(docs, doc{
		path:  "_file_tree",
		title: repo + ": repository structure",
		content: "Repository " + repo + " — directory and file layout (top levels):\n" +
			strings.Join(treeLines, "\n"),
	})
	sort.Slice(docs, func(i, j int) bool { return docs[i].path < docs[j].path })
	return docs
}

// ensureConnector finds or creates the workspace's repo-catalog connector.
func ensureConnector(ctx context.Context, pool *pgxpool.Pool, workspace string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`SELECT id::text FROM connectors WHERE workspace_id=$1::uuid AND name=$2`,
		workspace, ConnectorName).Scan(&id)
	if err == nil {
		return id, nil
	}
	var owner string
	if err := pool.QueryRow(ctx,
		`SELECT owner_id::text FROM workspaces WHERE id=$1::uuid`, workspace).Scan(&owner); err != nil {
		return "", fmt.Errorf("workspace not found: %w", err)
	}
	id = uuid.NewString()
	_, err = pool.Exec(ctx, `
		INSERT INTO connectors(id, workspace_id, name, provider, type, auth_type, status, config, created_by)
		VALUES ($1::uuid, $2::uuid, $3, 'github', 'native', 'pat', 'connected', '{"purpose":"repo-catalog"}', $4::uuid)`,
		id, workspace, ConnectorName, owner)
	return id, err
}

// upsertDocument replaces the document's chunks when its content changed.
// Chunks are stored without embeddings; the retriever's full-text fallback
// serves them.
func upsertDocument(ctx context.Context, pool *pgxpool.Pool, connectorID, workspace, repo string, d doc) (int, error) {
	sum := sha256.Sum256([]byte(d.content))
	hash := hex.EncodeToString(sum[:])
	sourceDocID := repo + "/" + d.path
	url := "https://github.com/" + repo + "/blob/HEAD/" + d.path
	if strings.HasPrefix(d.path, "_") {
		url = "https://github.com/" + repo
	}

	var docID, oldHash string
	err := pool.QueryRow(ctx,
		`SELECT id::text, content_hash FROM connector_documents WHERE connector_id=$1::uuid AND source_document_id=$2`,
		connectorID, sourceDocID).Scan(&docID, &oldHash)
	if err == nil && oldHash == hash {
		return 0, nil // unchanged
	}
	if err != nil {
		docID = uuid.NewString()
		if _, err := pool.Exec(ctx, `
			INSERT INTO connector_documents(id, connector_id, workspace_id, source, source_document_id, title, url, content_hash, indexed_at)
			VALUES ($1::uuid, $2::uuid, $3::uuid, 'repo-catalog', $4, $5, $6, $7, NOW())`,
			docID, connectorID, workspace, sourceDocID, d.title, url, hash); err != nil {
			return 0, err
		}
	} else {
		if _, err := pool.Exec(ctx,
			`UPDATE connector_documents SET title=$2, url=$3, content_hash=$4, indexed_at=NOW() WHERE id=$1::uuid`,
			docID, d.title, url, hash); err != nil {
			return 0, err
		}
		if _, err := pool.Exec(ctx, `DELETE FROM connector_chunks WHERE document_id=$1::uuid`, docID); err != nil {
			return 0, err
		}
	}

	chunks := chunkText(d.content, chunkSize, chunkOverlap)
	for i, c := range chunks {
		if _, err := pool.Exec(ctx, `
			INSERT INTO connector_chunks(id, document_id, chunk_index, content, metadata)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5)`,
			uuid.NewString(), docID, i, c, fmt.Sprintf(`{"repo":%q,"path":%q}`, repo, d.path)); err != nil {
			return i, err
		}
	}
	return len(chunks), nil
}

// chunkText splits text into ~size-rune chunks with overlap (mirrors the
// connector pipeline's chunking).
func chunkText(text string, size, overlap int) []string {
	runes := []rune(text)
	total := len(runes)
	if total <= size {
		return []string{text}
	}
	var chunks []string
	start := 0
	for start < total {
		end := start + size
		if end > total {
			end = total
		}
		chunks = append(chunks, string(runes[start:end]))
		if end == total {
			break
		}
		next := end - overlap
		if next <= start {
			next = start + 1
		}
		start = next
	}
	return chunks
}
