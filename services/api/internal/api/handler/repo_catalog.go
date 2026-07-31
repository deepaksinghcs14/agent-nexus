package handler

import (
	"context"
	"encoding/json"
	"log/slog"
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
	Repo             string     `json:"repo"`
	DefaultBranch    string     `json:"default_branch"`
	SessionsEnabled  bool       `json:"sessions_enabled"`
	Lessons          string     `json:"lessons"`
	RepoMap          string     `json:"repo_map"`
	RepoMapUpdatedAt *time.Time `json:"repo_map_updated_at"`
	MapGenerating    bool       `json:"map_generating"`
	Documents        int        `json:"documents"`
	Chunks           int        `json:"chunks"`
	UpdatedAt        time.Time  `json:"updated_at"`
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
		SELECT rc.repo, rc.default_branch, rc.sessions_enabled, rc.lessons, rc.repo_map, rc.repo_map_updated_at, rc.updated_at,
		       COUNT(DISTINCT cd.id) AS documents,
		       COUNT(cc.id) AS chunks,
		       EXISTS (
		         SELECT 1 FROM runs r2 JOIN conversations c2 ON c2.id = r2.conversation_id
		         WHERE c2.workspace_id = rc.workspace_id AND c2.title = 'Repo map: ' || rc.repo
		           AND r2.status IN ('running', 'session_wait')
		       ) AS map_generating
		FROM repo_catalog rc
		LEFT JOIN connector_documents cd
		       ON cd.workspace_id = rc.workspace_id AND cd.source_document_id LIKE rc.repo || '/%'
		LEFT JOIN connector_chunks cc ON cc.document_id = cd.id
		WHERE rc.workspace_id=$1::uuid
		GROUP BY rc.repo, rc.default_branch, rc.sessions_enabled, rc.lessons, rc.repo_map, rc.repo_map_updated_at, rc.updated_at
		ORDER BY rc.sessions_enabled DESC, rc.repo`, ws)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list repo catalog"))
		return
	}
	defer rows.Close()
	list := []repoCatalogEntry{}
	for rows.Next() {
		var e repoCatalogEntry
		if rows.Scan(&e.Repo, &e.DefaultBranch, &e.SessionsEnabled, &e.Lessons, &e.RepoMap, &e.RepoMapUpdatedAt, &e.UpdatedAt, &e.Documents, &e.Chunks, &e.MapGenerating) == nil {
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
		Lessons         *string `json:"lessons"`  // set → edit lessons only, leave the toggle alone
		RepoMap         *string `json:"repo_map"` // set → edit the cached repo map only, leave the toggle alone
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

	// Repo map edit: same own-branch treatment as lessons. Editing manually
	// stamps repo_map_updated_at too, so the hand-written map is trusted as
	// fresh instead of immediately flagged stale on the next session launch.
	if req.RepoMap != nil {
		if strings.TrimSpace(req.Repo) == "" {
			errs.Write(w, errs.BadRequest("repo is required"))
			return
		}
		tag, err := h.pool.Exec(r.Context(),
			`UPDATE repo_catalog SET repo_map=$3, repo_map_updated_at=NOW(), updated_at=NOW() WHERE workspace_id=$1::uuid AND repo=$2`,
			ws, strings.TrimSpace(req.Repo), *req.RepoMap)
		if err != nil || tag.RowsAffected() == 0 {
			errs.Write(w, errs.NotFound("repo not found in catalog"))
			return
		}
		writeAudit(r, h.pool, "repo_catalog.repo_map_updated", "workspace", ws)
		errs.WriteJSON(w, http.StatusOK, map[string]any{"repo": req.Repo, "repo_map": *req.RepoMap})
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

// GenerateRepoMap handles POST /api/v1/repo-catalog/generate-map — queues an
// on-demand Claude Code survey session that (re)generates the cached
// architecture map for one repo ({"repo":"owner/name"}) or every
// session-enabled repo in the workspace ({"all":true}). Sessions run in the
// background; poll the repo list (or the Sessions tab) to see the result land.
func (h *WorkspaceHandler) GenerateRepoMap(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	if h.invokeH == nil {
		errs.Write(w, errs.Internal("repo sessions are not available"))
		return
	}
	var req struct {
		Repo string `json:"repo"`
		All  bool   `json:"all"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}

	var repos []string
	if req.All {
		rows, err := h.pool.Query(r.Context(),
			`SELECT repo FROM repo_catalog WHERE workspace_id=$1::uuid AND sessions_enabled ORDER BY repo`, ws)
		if err != nil {
			errs.Write(w, errs.Internal("failed to list repo catalog"))
			return
		}
		for rows.Next() {
			var repo string
			if rows.Scan(&repo) == nil {
				repos = append(repos, repo)
			}
		}
		rows.Close()
	} else if repo := strings.TrimSpace(req.Repo); repo != "" {
		repos = []string{repo}
	} else {
		errs.Write(w, errs.BadRequest("repo or all is required"))
		return
	}
	if len(repos) == 0 {
		errs.Write(w, errs.BadRequest("no session-enabled repositories to generate maps for"))
		return
	}

	// Anchored to the orchestrator (not e.g. Docs Map Maintainer) specifically
	// so these runs land in the Claude Code page's Sessions tab, which filters
	// by that agent's ID.
	var agentID string
	if err := h.pool.QueryRow(r.Context(),
		`SELECT id FROM agents WHERE workspace_id=$1::uuid AND name='Jira Pipeline Orchestrator' LIMIT 1`, ws).
		Scan(&agentID); err != nil {
		errs.Write(w, errs.BadRequest("Jira Pipeline Orchestrator agent not found — run infra/scripts/setup_pipeline.sh"))
		return
	}
	uid := middleware.UserIDFromCtx(r.Context())

	// Sessions-enabled is enforced inside native_launch_repo_session itself
	// (checkSessionsEnabled) — a disabled single repo still gets queued here
	// but the generation run fails fast with a clear error, visible in the
	// Sessions tab.
	queued := make([]string, 0, len(repos))
	for _, repo := range repos {
		if _, err := h.invokeH.launchMapGenerationRun(context.Background(), ws, uid, agentID, repo); err != nil {
			slog.Warn("repo map: failed to queue generation run", "repo", repo, "error", err)
			continue
		}
		queued = append(queued, repo)
	}
	writeAudit(r, h.pool, "repo_catalog.map_generation_queued", "workspace", ws)
	errs.WriteJSON(w, http.StatusAccepted, map[string]any{"queued": queued})
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
