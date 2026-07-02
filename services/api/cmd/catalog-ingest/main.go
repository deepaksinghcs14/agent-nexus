// catalog-ingest onboards a GitHub repository into the pipeline's repo
// catalog: it clones the repo (token-based over HTTPS), extracts the
// documentation surface (README, docs/, architecture notes, module manifests,
// and a generated file-tree summary), and writes it as searchable chunks into
// the workspace's "repo-catalog" connector. The repo-selection agent retrieves
// against this catalog to decide which repos a Jira ticket touches.
//
// Embeddings are optional: chunks are stored without vectors and served by the
// retriever's full-text fallback, so no embedding provider is required.
//
// Usage:
//
//	DATABASE_URL=... GITHUB_TOKEN=... go run ./cmd/catalog-ingest \
//	    -repo owner/name -workspace <workspace-uuid> [-branch main]
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
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
	connectorName = "repo-catalog"
	chunkSize     = 2048
	chunkOverlap  = 256
	maxDocBytes   = 512 * 1024
)

func main() {
	repo := flag.String("repo", "", "GitHub repository as owner/name")
	workspace := flag.String("workspace", "", "workspace UUID to onboard the repo into")
	branch := flag.String("branch", "", "branch to index (default: repo default branch)")
	cloneURL := flag.String("clone-url", "", "override the clone URL (default: token-authenticated github.com URL; useful for GHE or local testing)")
	flag.Parse()

	if *repo == "" || *workspace == "" || strings.Count(*repo, "/") != 1 {
		fmt.Fprintln(os.Stderr, "usage: catalog-ingest -repo owner/name -workspace <uuid> [-branch main] [-clone-url url]")
		os.Exit(2)
	}
	dbURL := os.Getenv("DATABASE_URL")
	token := os.Getenv("GITHUB_TOKEN")
	if dbURL == "" || (token == "" && *cloneURL == "") {
		fmt.Fprintln(os.Stderr, "DATABASE_URL and GITHUB_TOKEN (or -clone-url) are required")
		os.Exit(2)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	die(err, "database connect")
	defer pool.Close()

	dir, err := os.MkdirTemp("", "catalog-ingest-*")
	die(err, "temp dir")
	defer os.RemoveAll(dir)

	src := *cloneURL
	if src == "" {
		src = fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", token, *repo)
	}
	args := []string{"clone", "--depth", "1"}
	if *branch != "" {
		args = append(args, "--branch", *branch)
	}
	args = append(args, src, dir)
	out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	die(err, "git clone: "+strings.ReplaceAll(string(out), token, "***"))

	headBranch := *branch
	if headBranch == "" {
		if b, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
			headBranch = strings.TrimSpace(string(b))
		} else {
			headBranch = "main"
		}
	}

	docs := collectDocs(dir, *repo)
	if len(docs) == 0 {
		fmt.Fprintln(os.Stderr, "no documentation content found in repo")
		os.Exit(1)
	}

	connectorID, err := ensureConnector(ctx, pool, *workspace)
	die(err, "ensure connector")

	totalChunks := 0
	for _, d := range docs {
		n, err := upsertDocument(ctx, pool, connectorID, *workspace, *repo, d)
		die(err, "upsert "+d.path)
		totalChunks += n
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO repo_catalog(repo, workspace_id, connector_id, default_branch, updated_at)
		VALUES ($1, $2::uuid, $3::uuid, $4, NOW())
		ON CONFLICT (workspace_id, repo) DO UPDATE SET connector_id=$3::uuid, default_branch=$4, updated_at=NOW()`,
		*repo, *workspace, connectorID, headBranch)
	die(err, "upsert repo_catalog")

	// Link the catalog to the workspace's seeded pipeline orchestrator so
	// onboarding a repo is the only step needed for repo selection to work.
	_, err = pool.Exec(ctx, `
		INSERT INTO agent_connectors(agent_id, connector_id, enabled, max_chunks, min_score)
		SELECT a.id, $1::uuid, true, 10, 0.3 FROM agents a
		WHERE a.workspace_id=$2::uuid AND a.name='Jira Pipeline Orchestrator'
		ON CONFLICT (agent_id, connector_id) DO NOTHING`,
		connectorID, *workspace)
	die(err, "link connector to orchestrator")

	fmt.Printf("onboarded %s: %d documents, %d chunks, connector %s, branch %s\n",
		*repo, len(docs), totalChunks, connectorID, headBranch)
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
		workspace, connectorName).Scan(&id)
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
		id, workspace, connectorName, owner)
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

func die(err error, stage string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "catalog-ingest: %s: %v\n", stage, err)
		os.Exit(1)
	}
}
