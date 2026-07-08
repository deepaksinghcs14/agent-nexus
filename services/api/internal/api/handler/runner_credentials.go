package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/encrypt"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

// Workspace pipeline credentials for repo sessions: the Claude account token
// (`claude setup-token` output, subscription billing) and the GitHub token
// (clone/push + PR tools). Both stored AES-256-GCM encrypted per workspace,
// never returned by the API, injected per session launch / resolved per tool
// call. A workspace GitHub token takes precedence over the instance-level
// GITHUB_TOKEN env, which remains a single-tenant fallback.

// GetRunnerCredentials reports connection status only — tokens never leave
// the server.
func (h *WorkspaceHandler) GetRunnerCredentials(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	var claude, github *string
	var updatedAt time.Time
	err := h.pool.QueryRow(r.Context(),
		`SELECT claude_token, github_token, updated_at FROM runner_credentials WHERE workspace_id=$1::uuid`, ws).
		Scan(&claude, &github, &updatedAt)
	if err != nil {
		errs.WriteJSON(w, http.StatusOK, map[string]any{
			"claude_connected": false,
			"github_connected": false,
			// Whether the instance provides fallback tokens via env.
			"github_env_fallback": h.cfg.GithubToken != "",
		})
		return
	}
	out := map[string]any{
		"claude_connected":    claude != nil && *claude != "",
		"github_connected":    github != nil && *github != "",
		"github_env_fallback": h.cfg.GithubToken != "",
		"updated_at":          updatedAt,
	}
	errs.WriteJSON(w, http.StatusOK, out)
}

// PutRunnerCredentials stores/replaces whichever tokens the request carries.
func (h *WorkspaceHandler) PutRunnerCredentials(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())

	var req struct {
		ClaudeToken string `json:"claude_token"`
		GithubToken string `json:"github_token"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	claude := strings.TrimSpace(req.ClaudeToken)
	github := strings.TrimSpace(req.GithubToken)
	if claude == "" && github == "" {
		errs.Write(w, errs.BadRequest("provide claude_token and/or github_token"))
		return
	}
	if claude != "" && !strings.HasPrefix(claude, "sk-ant-") {
		errs.Write(w, errs.BadRequest("that does not look like a Claude token — run `claude setup-token` and paste its output (starts with sk-ant-)"))
		return
	}
	if github != "" && !strings.HasPrefix(github, "ghp_") && !strings.HasPrefix(github, "github_pat_") && !strings.HasPrefix(github, "gho_") {
		errs.Write(w, errs.BadRequest("that does not look like a GitHub token (expected ghp_…, github_pat_…, or gho_…)"))
		return
	}

	encMaybe := func(v string) (*string, error) {
		if v == "" {
			return nil, nil
		}
		e, err := encrypt.Encrypt([]byte(h.cfg.EncryptionKey), v)
		if err != nil {
			return nil, err
		}
		return &e, nil
	}
	encClaude, err1 := encMaybe(claude)
	encGithub, err2 := encMaybe(github)
	if err1 != nil || err2 != nil {
		errs.Write(w, errs.Internal("failed to encrypt token"))
		return
	}

	// COALESCE keeps the other credential untouched when only one is sent.
	if _, err := h.pool.Exec(r.Context(), `
		INSERT INTO runner_credentials(workspace_id, claude_token, github_token, updated_by, updated_at)
		VALUES ($1::uuid, $2, $3, $4::uuid, NOW())
		ON CONFLICT (workspace_id) DO UPDATE SET
		  claude_token = COALESCE($2, runner_credentials.claude_token),
		  github_token = COALESCE($3, runner_credentials.github_token),
		  updated_by = $4::uuid, updated_at = NOW()`,
		ws, encClaude, encGithub, uid); err != nil {
		errs.Write(w, errs.Internal("failed to store token"))
		return
	}
	writeAudit(r, h.pool, "runner_credentials.updated", "workspace", ws)
	h.GetRunnerCredentials(w, r)
}

// DeleteRunnerCredentials disconnects one credential
// (?field=claude|github) or all of them (?field=all, the default).
func (h *WorkspaceHandler) DeleteRunnerCredentials(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	switch r.URL.Query().Get("field") {
	case "claude":
		h.pool.Exec(r.Context(), `UPDATE runner_credentials SET claude_token=NULL, updated_at=NOW() WHERE workspace_id=$1::uuid`, ws) //nolint:errcheck
	case "github":
		h.pool.Exec(r.Context(), `UPDATE runner_credentials SET github_token=NULL, updated_at=NOW() WHERE workspace_id=$1::uuid`, ws) //nolint:errcheck
	default:
		h.pool.Exec(r.Context(), `DELETE FROM runner_credentials WHERE workspace_id=$1::uuid`, ws) //nolint:errcheck
	}
	writeAudit(r, h.pool, "runner_credentials.deleted", "workspace", ws)
	h.GetRunnerCredentials(w, r)
}

// WorkspaceGithubToken resolves the effective GitHub token for a workspace:
// the workspace credential when set, else the instance env fallback, else "".
// Used by the GitHub tools and the session-launch tool.
func WorkspaceGithubToken(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, workspaceID string) string {
	var enc *string
	if err := pool.QueryRow(ctx,
		`SELECT github_token FROM runner_credentials WHERE workspace_id=$1::uuid`, workspaceID).Scan(&enc); err == nil && enc != nil && *enc != "" {
		if token, err := encrypt.Decrypt([]byte(cfg.EncryptionKey), *enc); err == nil {
			return token
		}
	}
	return cfg.GithubToken
}
