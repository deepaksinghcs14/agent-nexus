package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/catalog"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

// Repo catalog endpoints: the workspace's allowlist of repositories that
// autonomous coding sessions may target. Onboarding runs server-side using
// the workspace's stored GitHub token (Settings → Claude Code), so users
// never re-enter credentials on the CLI.

type repoCatalogEntry struct {
	Repo          string    `json:"repo"`
	DefaultBranch string    `json:"default_branch"`
	Documents     int       `json:"documents"`
	Chunks        int       `json:"chunks"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ListRepoCatalog handles GET /api/v1/repo-catalog.
func (h *WorkspaceHandler) ListRepoCatalog(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	rows, err := h.pool.Query(r.Context(), `
		SELECT rc.repo, rc.default_branch, rc.updated_at,
		       COUNT(DISTINCT cd.id) AS documents,
		       COUNT(cc.id) AS chunks
		FROM repo_catalog rc
		LEFT JOIN connector_documents cd
		       ON cd.connector_id = rc.connector_id AND cd.source_document_id LIKE rc.repo || '/%'
		LEFT JOIN connector_chunks cc ON cc.document_id = cd.id
		WHERE rc.workspace_id=$1::uuid
		GROUP BY rc.repo, rc.default_branch, rc.updated_at
		ORDER BY rc.repo`, ws)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list repo catalog"))
		return
	}
	defer rows.Close()
	list := []repoCatalogEntry{}
	for rows.Next() {
		var e repoCatalogEntry
		if rows.Scan(&e.Repo, &e.DefaultBranch, &e.UpdatedAt, &e.Documents, &e.Chunks) == nil {
			list = append(list, e)
		}
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

// OnboardRepo handles POST /api/v1/repo-catalog — clones and indexes the repo
// server-side with the workspace GitHub token (env fallback; empty works for
// public repos).
func (h *WorkspaceHandler) OnboardRepo(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	var req struct {
		Repo   string `json:"repo"`
		Branch string `json:"branch"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || strings.TrimSpace(req.Repo) == "" {
		errs.Write(w, errs.BadRequest("repo (owner/name) is required"))
		return
	}

	// Cloning and chunking a large repo can exceed the request context.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	res, err := catalog.Ingest(ctx, h.pool, catalog.Request{
		WorkspaceID: ws,
		Repo:        strings.TrimSpace(req.Repo),
		Branch:      strings.TrimSpace(req.Branch),
		Token:       WorkspaceGithubToken(ctx, h.pool, h.cfg, ws),
	})
	if err != nil {
		errs.Write(w, errs.BadRequest(err.Error()))
		return
	}
	writeAudit(r, h.pool, "repo_catalog.onboarded", "workspace", ws)
	errs.WriteJSON(w, http.StatusCreated, res)
}

// RemoveRepo handles DELETE /api/v1/repo-catalog?repo=owner/name.
func (h *WorkspaceHandler) RemoveRepo(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	repo := strings.TrimSpace(r.URL.Query().Get("repo"))
	if repo == "" {
		errs.Write(w, errs.BadRequest("repo query parameter is required"))
		return
	}
	if err := catalog.Remove(r.Context(), h.pool, ws, repo); err != nil {
		errs.Write(w, errs.NotFound(err.Error()))
		return
	}
	writeAudit(r, h.pool, "repo_catalog.removed", "workspace", ws)
	errs.WriteJSON(w, http.StatusOK, map[string]any{"removed": repo})
}
