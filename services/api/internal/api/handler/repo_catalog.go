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
	Repo            string    `json:"repo"`
	DefaultBranch   string    `json:"default_branch"`
	SessionsEnabled bool      `json:"sessions_enabled"`
	Lessons         string    `json:"lessons"`
	Documents       int       `json:"documents"`
	Chunks          int       `json:"chunks"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ListRepoCatalog handles GET /api/v1/repo-catalog.
func (h *WorkspaceHandler) ListRepoCatalog(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	// Documents/chunks are counted across every connector in the workspace,
	// not just the one the allowlist row points at: an adopted repo's full
	// content lives in the GitHub connector that synced it, while its catalog
	// card lives in the repo-catalog connector — both make the repo
	// searchable, so both belong in the "how indexed is this repo" number.
	rows, err := h.pool.Query(r.Context(), `
		SELECT rc.repo, rc.default_branch, rc.sessions_enabled, rc.lessons, rc.updated_at,
		       COUNT(DISTINCT cd.id) AS documents,
		       COUNT(cc.id) AS chunks
		FROM repo_catalog rc
		LEFT JOIN connector_documents cd
		       ON cd.workspace_id = rc.workspace_id AND cd.source_document_id LIKE rc.repo || '/%'
		LEFT JOIN connector_chunks cc ON cc.document_id = cd.id
		WHERE rc.workspace_id=$1::uuid
		GROUP BY rc.repo, rc.default_branch, rc.sessions_enabled, rc.lessons, rc.updated_at
		ORDER BY rc.sessions_enabled DESC, rc.repo`, ws)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list repo catalog"))
		return
	}
	defer rows.Close()
	list := []repoCatalogEntry{}
	for rows.Next() {
		var e repoCatalogEntry
		if rows.Scan(&e.Repo, &e.DefaultBranch, &e.SessionsEnabled, &e.Lessons, &e.UpdatedAt, &e.Documents, &e.Chunks) == nil {
			list = append(list, e)
		}
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

// SetRepoSessions handles PATCH /api/v1/repo-catalog — enables or disables
// coding sessions for one repo, or for ALL repos in the workspace when
// {"all": true} is passed. This toggle is the deliberate grant of write
// access; connector syncs only make repos known.
func (h *WorkspaceHandler) SetRepoSessions(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	var req struct {
		Repo            string  `json:"repo"`
		All             bool    `json:"all"`
		SessionsEnabled bool    `json:"sessions_enabled"`
		Lessons         *string `json:"lessons"` // set → edit lessons only, leave the toggle alone
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}

	// Lessons edit: its own branch so the UI can save the textarea without
	// also carrying (and possibly clobbering) the sessions toggle.
	if req.Lessons != nil {
		if strings.TrimSpace(req.Repo) == "" {
			errs.Write(w, errs.BadRequest("repo is required"))
			return
		}
		tag, err := h.pool.Exec(r.Context(),
			`UPDATE repo_catalog SET lessons=$3, updated_at=NOW() WHERE workspace_id=$1::uuid AND repo=$2`,
			ws, strings.TrimSpace(req.Repo), *req.Lessons)
		if err != nil || tag.RowsAffected() == 0 {
			errs.Write(w, errs.NotFound("repo not found in catalog"))
			return
		}
		writeAudit(r, h.pool, "repo_catalog.lessons_updated", "workspace", ws)
		errs.WriteJSON(w, http.StatusOK, map[string]any{"repo": req.Repo, "lessons": *req.Lessons})
		return
	}

	// Bulk mode: a single UPDATE across every repo in the workspace — avoids
	// firing one request per repo from the client.
	if req.All {
		tag, err := h.pool.Exec(r.Context(),
			`UPDATE repo_catalog SET sessions_enabled=$2, updated_at=NOW()
			 WHERE workspace_id=$1::uuid AND sessions_enabled <> $2`,
			ws, req.SessionsEnabled)
		if err != nil {
			errs.Write(w, errs.Internal("failed to update repo catalog"))
			return
		}
		writeAudit(r, h.pool, "repo_catalog.sessions_toggled_all", "workspace", ws)
		errs.WriteJSON(w, http.StatusOK, map[string]any{"updated": tag.RowsAffected(), "sessions_enabled": req.SessionsEnabled})
		return
	}

	if strings.TrimSpace(req.Repo) == "" {
		errs.Write(w, errs.BadRequest("repo is required"))
		return
	}
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE repo_catalog SET sessions_enabled=$3, updated_at=NOW() WHERE workspace_id=$1::uuid AND repo=$2`,
		ws, strings.TrimSpace(req.Repo), req.SessionsEnabled)
	if err != nil || tag.RowsAffected() == 0 {
		errs.Write(w, errs.NotFound("repo not found in catalog"))
		return
	}
	writeAudit(r, h.pool, "repo_catalog.sessions_toggled", "workspace", ws)
	errs.WriteJSON(w, http.StatusOK, map[string]any{"repo": req.Repo, "sessions_enabled": req.SessionsEnabled})
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
