// catalog-ingest onboards a GitHub repository into the pipeline's repo
// catalog from the command line — a thin wrapper over internal/catalog,
// for headless installs and CI. Interactive users should prefer the
// Repositories card in Settings → Claude Code, which onboards server-side
// using the workspace's stored GitHub token.
//
// Usage:
//
//	DATABASE_URL=... GITHUB_TOKEN=... go run ./cmd/catalog-ingest \
//	    -repo owner/name -workspace <workspace-uuid> [-branch main] [-clone-url url]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/deepaksingh/agent-nexus/services/api/internal/catalog"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	repo := flag.String("repo", "", "GitHub repository as owner/name")
	workspace := flag.String("workspace", "", "workspace UUID to onboard the repo into")
	branch := flag.String("branch", "", "branch to index (default: repo default branch)")
	cloneURL := flag.String("clone-url", "", "override the clone URL (default: token-authenticated github.com URL; useful for GHE or local testing)")
	flag.Parse()

	if *repo == "" || *workspace == "" {
		fmt.Fprintln(os.Stderr, "usage: catalog-ingest -repo owner/name -workspace <uuid> [-branch main] [-clone-url url]")
		os.Exit(2)
	}
	dbURL := os.Getenv("DATABASE_URL")
	token := os.Getenv("GITHUB_TOKEN")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(2)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "catalog-ingest: database connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	res, err := catalog.Ingest(ctx, pool, catalog.Request{
		WorkspaceID: *workspace,
		Repo:        *repo,
		Branch:      *branch,
		Token:       token,
		CloneURL:    *cloneURL,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "catalog-ingest: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("onboarded %s: %d documents, %d chunks, connector %s, branch %s\n",
		res.Repo, res.Documents, res.Chunks, res.ConnectorID, res.Branch)
}
