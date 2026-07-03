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
	var claude, github, jiraURL, jiraToken *string
	var updatedAt time.Time
	err := h.pool.QueryRow(r.Context(),
		`SELECT claude_token, github_token, jira_base_url, jira_api_token, updated_at FROM runner_credentials WHERE workspace_id=$1::uuid`, ws).
		Scan(&claude, &github, &jiraURL, &jiraToken, &updatedAt)
	if err != nil {
		errs.WriteJSON(w, http.StatusOK, map[string]any{
			"claude_connected": false,
			"github_connected": false,
			"jira_connected":   false,
			// Whether the instance provides fallback tokens via env.
			"github_env_fallback": h.cfg.GithubToken != "",
			"jira_env_fallback":   h.cfg.JiraBaseURL != "" && h.cfg.JiraAPIToken != "",
		})
		return
	}
	out := map[string]any{
		"claude_connected":    claude != nil && *claude != "",
		"github_connected":    github != nil && *github != "",
		"jira_connected":      jiraURL != nil && *jiraURL != "" && jiraToken != nil && *jiraToken != "",
		"github_env_fallback": h.cfg.GithubToken != "",
		"jira_env_fallback":   h.cfg.JiraBaseURL != "" && h.cfg.JiraAPIToken != "",
		"updated_at":          updatedAt,
	}
	if jiraURL != nil && *jiraURL != "" {
		out["jira_base_url"] = *jiraURL
	}
	errs.WriteJSON(w, http.StatusOK, out)
}

// PutRunnerCredentials stores/replaces whichever tokens the request carries.
func (h *WorkspaceHandler) PutRunnerCredentials(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())

	var req struct {
		ClaudeToken  string `json:"claude_token"`
		GithubToken  string `json:"github_token"`
		JiraBaseURL  string `json:"jira_base_url"`
		JiraEmail    string `json:"jira_email"`
		JiraAPIToken string `json:"jira_api_token"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	claude := strings.TrimSpace(req.ClaudeToken)
	github := strings.TrimSpace(req.GithubToken)
	jiraURL := strings.TrimRight(strings.TrimSpace(req.JiraBaseURL), "/")
	jiraEmail := strings.TrimSpace(req.JiraEmail)
	jiraToken := strings.TrimSpace(req.JiraAPIToken)
	if claude == "" && github == "" && jiraToken == "" {
		errs.Write(w, errs.BadRequest("provide claude_token, github_token, and/or jira credentials"))
		return
	}
	if (jiraToken != "") != (jiraURL != "") {
		errs.Write(w, errs.BadRequest("jira_base_url and jira_api_token must be provided together"))
		return
	}
	if jiraURL != "" && !strings.HasPrefix(jiraURL, "https://") && !strings.HasPrefix(jiraURL, "http://") {
		errs.Write(w, errs.BadRequest("jira_base_url must be a full URL, e.g. https://yourorg.atlassian.net"))
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
	encJira, err3 := encMaybe(jiraToken)
	if err1 != nil || err2 != nil || err3 != nil {
		errs.Write(w, errs.Internal("failed to encrypt token"))
		return
	}
	// Base URL and email travel with the token (all three replaced together);
	// email may legitimately be empty (Data Center PATs use Bearer auth).
	var jiraURLp, jiraEmailp *string
	if jiraToken != "" {
		jiraURLp, jiraEmailp = &jiraURL, &jiraEmail
	}

	// COALESCE keeps the other credentials untouched when only one is sent.
	if _, err := h.pool.Exec(r.Context(), `
		INSERT INTO runner_credentials(workspace_id, claude_token, github_token, jira_base_url, jira_email, jira_api_token, updated_by, updated_at)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::uuid, NOW())
		ON CONFLICT (workspace_id) DO UPDATE SET
		  claude_token = COALESCE($2, runner_credentials.claude_token),
		  github_token = COALESCE($3, runner_credentials.github_token),
		  jira_base_url = COALESCE($4, runner_credentials.jira_base_url),
		  jira_email = COALESCE($5, runner_credentials.jira_email),
		  jira_api_token = COALESCE($6, runner_credentials.jira_api_token),
		  updated_by = $7::uuid, updated_at = NOW()`,
		ws, encClaude, encGithub, jiraURLp, jiraEmailp, encJira, uid); err != nil {
		errs.Write(w, errs.Internal("failed to store token"))
		return
	}
	writeAudit(r, h.pool, "runner_credentials.updated", "workspace", ws)
	h.GetRunnerCredentials(w, r)
}

// DeleteRunnerCredentials disconnects one credential
// (?field=claude|github|jira) or all of them (?field=all, the default).
func (h *WorkspaceHandler) DeleteRunnerCredentials(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	switch r.URL.Query().Get("field") {
	case "claude":
		h.pool.Exec(r.Context(), `UPDATE runner_credentials SET claude_token=NULL, updated_at=NOW() WHERE workspace_id=$1::uuid`, ws) //nolint:errcheck
	case "github":
		h.pool.Exec(r.Context(), `UPDATE runner_credentials SET github_token=NULL, updated_at=NOW() WHERE workspace_id=$1::uuid`, ws) //nolint:errcheck
	case "jira":
		h.pool.Exec(r.Context(), `UPDATE runner_credentials SET jira_base_url=NULL, jira_email=NULL, jira_api_token=NULL, updated_at=NOW() WHERE workspace_id=$1::uuid`, ws) //nolint:errcheck
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
