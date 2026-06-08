package handler

import (
	"encoding/json"
	"github.com/agentNexus/agent-nexus/services/api/internal/config"
	"github.com/agentNexus/agent-nexus/services/api/internal/domain"
	"github.com/agentNexus/agent-nexus/services/api/internal/repository"
	"github.com/agentNexus/agent-nexus/services/api/pkg/errs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
)

type AdminHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewAdminHandler(p *pgxpool.Pool, c *config.Config) *AdminHandler { return &AdminHandler{p, c} }
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
func (h *AdminHandler) Usage(w http.ResponseWriter, r *http.Request) {
	var runs, tokens int
	var cost float64
	_ = h.pool.QueryRow(r.Context(), `SELECT COUNT(*),COALESCE(SUM(total_input_tokens+total_output_tokens),0),COALESCE(SUM(cost_estimate),0) FROM runs`).Scan(&runs, &tokens, &cost)
	errs.WriteJSON(w, 200, map[string]any{"runs": runs, "tokens": tokens, "cost": cost})
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
	defer tx.Rollback(r.Context())
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
