package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/encrypt"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

// Runner credentials: the workspace's Claude account token for repo sessions
// (the output of `claude setup-token`, subscription billing). Stored encrypted;
// never returned by the API; injected per session launch.

// GetRunnerCredentials reports connection status only — the token itself
// never leaves the server.
func (h *WorkspaceHandler) GetRunnerCredentials(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	var updatedAt time.Time
	err := h.pool.QueryRow(r.Context(),
		`SELECT updated_at FROM runner_credentials WHERE workspace_id=$1::uuid`, ws).Scan(&updatedAt)
	if err != nil {
		errs.WriteJSON(w, http.StatusOK, map[string]any{"connected": false})
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"connected": true, "updated_at": updatedAt})
}

// PutRunnerCredentials stores/replaces the workspace's Claude token.
func (h *WorkspaceHandler) PutRunnerCredentials(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())

	var req struct {
		ClaudeToken string `json:"claude_token"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	token := strings.TrimSpace(req.ClaudeToken)
	if token == "" {
		errs.Write(w, errs.BadRequest("claude_token is required"))
		return
	}
	if !strings.HasPrefix(token, "sk-ant-") {
		errs.Write(w, errs.BadRequest("that does not look like a Claude token — run `claude setup-token` and paste its output (starts with sk-ant-)"))
		return
	}

	enc, err := encrypt.Encrypt([]byte(h.cfg.EncryptionKey), token)
	if err != nil {
		errs.Write(w, errs.Internal("failed to encrypt token"))
		return
	}
	if _, err := h.pool.Exec(r.Context(), `
		INSERT INTO runner_credentials(workspace_id, claude_token, updated_by, updated_at)
		VALUES ($1::uuid, $2, $3::uuid, NOW())
		ON CONFLICT (workspace_id) DO UPDATE SET claude_token=$2, updated_by=$3::uuid, updated_at=NOW()`,
		ws, enc, uid); err != nil {
		errs.Write(w, errs.Internal("failed to store token"))
		return
	}
	writeAudit(r, h.pool, "runner_credentials.updated", "workspace", ws)
	errs.WriteJSON(w, http.StatusOK, map[string]any{"connected": true})
}

// DeleteRunnerCredentials disconnects the Claude account.
func (h *WorkspaceHandler) DeleteRunnerCredentials(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	h.pool.Exec(r.Context(), `DELETE FROM runner_credentials WHERE workspace_id=$1::uuid`, ws) //nolint:errcheck
	writeAudit(r, h.pool, "runner_credentials.deleted", "workspace", ws)
	errs.WriteJSON(w, http.StatusOK, map[string]any{"connected": false})
}
