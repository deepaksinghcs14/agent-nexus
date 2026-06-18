package handler

import (
	"encoding/json"
	"fmt"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/internal/runtime/logstream"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strings"
	"time"
)

type AdminHandler struct {
	pool   *pgxpool.Pool
	cfg    *config.Config
	logHub *logstream.Hub
}

func NewAdminHandler(p *pgxpool.Pool, c *config.Config, logHub *logstream.Hub) *AdminHandler {
	return &AdminHandler{pool: p, cfg: c, logHub: logHub}
}
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	a, e := repository.NewUserRepository(h.pool).List(r.Context())
	if e != nil {
		errs.Write(w, errs.Internal("failed to list users"))
		return
	}
	if a == nil {
		a = []domain.User{}
	}
	errs.WriteJSON(w, 200, map[string]any{"data": a})
}
func (h *AdminHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	u, e := repository.NewUserRepository(h.pool).GetByID(r.Context(), chi.URLParam(r, "id"))
	if e != nil {
		errs.Write(w, errs.NotFound("user not found"))
		return
	}
	errs.WriteJSON(w, 200, u)
}
func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	repo := repository.NewUserRepository(h.pool)
	u, e := repo.GetByID(r.Context(), chi.URLParam(r, "id"))
	if e != nil {
		errs.Write(w, errs.NotFound("user not found"))
		return
	}
	var q struct {
		FullName *string `json:"full_name"`
		IsActive *bool   `json:"is_active"`
		IsAdmin  *bool   `json:"is_admin"`
	}
	if json.NewDecoder(r.Body).Decode(&q) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if q.FullName != nil {
		u.FullName = *q.FullName
	}
	if q.IsActive != nil {
		u.IsActive = *q.IsActive
	}
	if q.IsAdmin != nil {
		u.IsAdmin = *q.IsAdmin
	}
	if repo.Update(r.Context(), u) != nil {
		errs.Write(w, errs.Internal("failed to update user"))
		return
	}
	errs.WriteJSON(w, 200, u)
}
func (h *AdminHandler) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	rows, e := h.pool.Query(r.Context(), `SELECT id::text,name,display_name,owner_id::text,settings,workspace_type,created_at,updated_at FROM workspaces ORDER BY created_at DESC`)
	if e != nil {
		errs.Write(w, errs.Internal("failed to list workspaces"))
		return
	}
	defer rows.Close()
	a := []domain.Workspace{}
	for rows.Next() {
		var x domain.Workspace
		if rows.Scan(&x.ID, &x.Name, &x.DisplayName, &x.OwnerID, &x.Settings, &x.WorkspaceType, &x.CreatedAt, &x.UpdatedAt) != nil {
			errs.Write(w, errs.Internal("failed to read workspaces"))
			return
		}
		a = append(a, x)
	}
	errs.WriteJSON(w, 200, map[string]any{"data": a})
}
func (h *AdminHandler) UpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	var q struct {
		DisplayName string          `json:"display_name"`
		Settings    json.RawMessage `json:"settings"`
	}
	if json.NewDecoder(r.Body).Decode(&q) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if len(q.Settings) == 0 {
		q.Settings = json.RawMessage(`{}`)
	}
	var x domain.Workspace
	e := h.pool.QueryRow(r.Context(), `UPDATE workspaces SET display_name=COALESCE(NULLIF($2,''),display_name),settings=$3,updated_at=NOW() WHERE id=$1::uuid RETURNING id::text,name,display_name,owner_id::text,settings,workspace_type,created_at,updated_at`, chi.URLParam(r, "id"), q.DisplayName, q.Settings).Scan(&x.ID, &x.Name, &x.DisplayName, &x.OwnerID, &x.Settings, &x.WorkspaceType, &x.CreatedAt, &x.UpdatedAt)
	if e != nil {
		errs.Write(w, errs.NotFound("workspace not found"))
		return
	}
	errs.WriteJSON(w, 200, x)
}
func (h *AdminHandler) AuditLogs(w http.ResponseWriter, r *http.Request) {
	rows, e := h.pool.Query(r.Context(), `SELECT id::text,COALESCE(workspace_id::text,''),COALESCE(actor_id::text,''),actor_email,action,resource_type,resource_id,metadata,ip_address,created_at FROM admin_audit_logs WHERE ($1='' OR resource_type=$1) ORDER BY created_at DESC LIMIT 200`, r.URL.Query().Get("resource_type"))
	if e != nil {
		errs.Write(w, errs.Internal("failed to list audit logs"))
		return
	}
	defer rows.Close()
	a := []domain.AuditLog{}
	for rows.Next() {
		var x domain.AuditLog
		if rows.Scan(&x.ID, &x.WorkspaceID, &x.ActorID, &x.ActorEmail, &x.Action, &x.ResourceType, &x.ResourceID, &x.Metadata, &x.IPAddress, &x.CreatedAt) != nil {
			errs.Write(w, errs.Internal("failed to read audit logs"))
			return
		}
		a = append(a, x)
	}
	errs.WriteJSON(w, 200, map[string]any{"data": a})
}

func (h *AdminHandler) ServiceLogStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		errs.Write(w, errs.Internal("streaming is not supported"))
		return
	}
	if h.logHub == nil {
		errs.Write(w, errs.Internal("log stream is not configured"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := h.logHub.Subscribe()
	defer h.logHub.Unsubscribe(ch)

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case entry, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (h *AdminHandler) IngestServiceLog(w http.ResponseWriter, r *http.Request) {
	if h.cfg.LogStreamIngestToken == "" {
		errs.Write(w, errs.Forbidden("log ingest is not configured"))
		return
	}
	token := r.Header.Get("X-Log-Stream-Token")
	if token == "" {
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if token != h.cfg.LogStreamIngestToken {
		errs.Write(w, errs.Forbidden("invalid log ingest token"))
		return
	}
	if h.logHub == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var entry logstream.Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		errs.Write(w, errs.BadRequest("invalid log entry"))
		return
	}
	if entry.Source == "" {
		entry.Source = "process"
	}
	entry.Level = strings.ToLower(entry.Level)
	if entry.Level == "" {
		entry.Level = "info"
	}
	h.logHub.Publish(entry)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) Usage(w http.ResponseWriter, r *http.Request) {
	var runs, tokens int
	var cost float64
	var webhookTriggers int
	_ = h.pool.QueryRow(r.Context(), `SELECT COUNT(*),COALESCE(SUM(total_input_tokens+total_output_tokens),0),COALESCE(SUM(cost_estimate),0) FROM runs`).Scan(&runs, &tokens, &cost)
	_ = h.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM webhook_triggers`).Scan(&webhookTriggers)
	errs.WriteJSON(w, 200, map[string]any{"runs": runs, "tokens": tokens, "cost": cost, "webhook_triggers": webhookTriggers})
}
func (h *AdminHandler) GetPolicies(w http.ResponseWriter, r *http.Request) {
	rows, e := h.pool.Query(r.Context(), `SELECT id::text,COALESCE(workspace_id::text,''),key,value,created_at,updated_at FROM policies ORDER BY key`)
	if e != nil {
		errs.Write(w, errs.Internal("failed to list policies"))
		return
	}
	defer rows.Close()
	a := []domain.Policy{}
	for rows.Next() {
		var x domain.Policy
		if rows.Scan(&x.ID, &x.WorkspaceID, &x.Key, &x.Value, &x.CreatedAt, &x.UpdatedAt) != nil {
			errs.Write(w, errs.Internal("failed to read policies"))
			return
		}
		a = append(a, x)
	}
	errs.WriteJSON(w, 200, map[string]any{"data": a})
}
func (h *AdminHandler) SetPolicies(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Policies []struct {
			Key   string          `json:"key"`
			Value json.RawMessage `json:"value"`
		} `json:"policies"`
	}
	if json.NewDecoder(r.Body).Decode(&q) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	tx, e := h.pool.Begin(r.Context())
	if e != nil {
		errs.Write(w, errs.Internal("failed to save policies"))
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	for _, p := range q.Policies {
		_, e = tx.Exec(r.Context(), `INSERT INTO policies(workspace_id,key,value)VALUES(NULL,$1,$2)ON CONFLICT(workspace_id,key)DO UPDATE SET value=$2,updated_at=NOW()`, p.Key, p.Value)
		if e != nil {
			errs.Write(w, errs.Internal("failed to save policies"))
			return
		}
	}
	if tx.Commit(r.Context()) != nil {
		errs.Write(w, errs.Internal("failed to save policies"))
		return
	}
	errs.WriteJSON(w, 200, map[string]any{"status": "saved"})
}
