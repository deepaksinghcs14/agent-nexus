package handler

import (
	"encoding/json"
	"github.com/agentNexus/agent-nexus/services/api/internal/api/middleware"
	"github.com/agentNexus/agent-nexus/services/api/internal/config"
	"github.com/agentNexus/agent-nexus/services/api/pkg/errs"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
)

type GroupsHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewGroupsHandler(p *pgxpool.Pool, c *config.Config) *GroupsHandler { return &GroupsHandler{p, c} }

type groupRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Mode        string   `json:"mode"`
	Status      string   `json:"status"`
	AgentIDs    []string `json:"agent_ids"`
}

func (h *GroupsHandler) group(r *http.Request, id string) (map[string]any, error) {
	var gID, ws, name, desc, mode, status, createdBy string
	var created, updated any
	e := h.pool.QueryRow(r.Context(), `SELECT id::text,workspace_id::text,name,description,mode,status,created_by::text,created_at,updated_at FROM agent_groups WHERE id=$1::uuid AND workspace_id=$2::uuid`, id, middleware.WorkspaceIDFromCtx(r.Context())).Scan(&gID, &ws, &name, &desc, &mode, &status, &createdBy, &created, &updated)
	if e != nil {
		return nil, e
	}
	rows, e := h.pool.Query(r.Context(), `SELECT agent_id::text FROM agent_group_members WHERE group_id=$1::uuid ORDER BY position`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var x string
		_ = rows.Scan(&x)
		ids = append(ids, x)
	}
	return map[string]any{"id": gID, "workspace_id": ws, "name": name, "description": desc, "mode": mode, "status": status, "agent_ids": ids, "created_by": createdBy, "created_at": created, "updated_at": updated}, nil
}
func (h *GroupsHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, e := h.pool.Query(r.Context(), `SELECT id::text FROM agent_groups WHERE workspace_id=$1::uuid ORDER BY created_at DESC`, middleware.WorkspaceIDFromCtx(r.Context()))
	if e != nil {
		errs.Write(w, errs.Internal("failed to list groups"))
		return
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	a := []map[string]any{}
	for _, id := range ids {
		g, e := h.group(r, id)
		if e == nil {
			a = append(a, g)
		}
	}
	errs.WriteJSON(w, 200, map[string]any{"data": a})
}
func (h *GroupsHandler) save(w http.ResponseWriter, r *http.Request, id string, create bool) {
	var q groupRequest
	if json.NewDecoder(r.Body).Decode(&q) != nil || q.Name == "" {
		errs.Write(w, errs.BadRequest("name is required"))
		return
	}
	if q.Mode == "" {
		q.Mode = "pipeline"
	}
	if q.Status == "" {
		q.Status = "active"
	}
	tx, e := h.pool.Begin(r.Context())
	if e != nil {
		errs.Write(w, errs.Internal("failed to save group"))
		return
	}
	defer tx.Rollback(r.Context())
	if create {
		_, e = tx.Exec(r.Context(), `INSERT INTO agent_groups(id,workspace_id,name,description,mode,status,created_by)VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7::uuid)`, id, middleware.WorkspaceIDFromCtx(r.Context()), q.Name, q.Description, q.Mode, q.Status, middleware.UserIDFromCtx(r.Context()))
	} else {
		_, e = tx.Exec(r.Context(), `UPDATE agent_groups SET name=$3,description=$4,mode=$5,status=$6,updated_at=NOW() WHERE id=$1::uuid AND workspace_id=$2::uuid`, id, middleware.WorkspaceIDFromCtx(r.Context()), q.Name, q.Description, q.Mode, q.Status)
		if e == nil {
			_, e = tx.Exec(r.Context(), `DELETE FROM agent_group_members WHERE group_id=$1::uuid`, id)
		}
	}
	for i, a := range q.AgentIDs {
		if e != nil {
			break
		}
		_, e = tx.Exec(r.Context(), `INSERT INTO agent_group_members(group_id,agent_id,position,role)VALUES($1::uuid,$2::uuid,$3,$4)`, id, a, i, map[bool]string{true: "supervisor", false: "member"}[q.Mode == "supervisor" && i == 0])
	}
	if e != nil || tx.Commit(r.Context()) != nil {
		errs.Write(w, errs.Internal("failed to save group"))
		return
	}
	g, _ := h.group(r, id)
	errs.WriteJSON(w, map[bool]int{true: 201, false: 200}[create], g)
}
func (h *GroupsHandler) Create(w http.ResponseWriter, r *http.Request) {
	h.save(w, r, uuid.NewString(), true)
}
func (h *GroupsHandler) Get(w http.ResponseWriter, r *http.Request) {
	g, e := h.group(r, chi.URLParam(r, "id"))
	if e != nil {
		errs.Write(w, errs.NotFound("group not found"))
		return
	}
	errs.WriteJSON(w, 200, g)
}
func (h *GroupsHandler) Update(w http.ResponseWriter, r *http.Request) {
	h.save(w, r, chi.URLParam(r, "id"), false)
}
func (h *GroupsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	t, e := h.pool.Exec(r.Context(), `DELETE FROM agent_groups WHERE id=$1::uuid AND workspace_id=$2::uuid`, chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context()))
	if e != nil {
		errs.Write(w, errs.Internal("failed to delete group"))
		return
	}
	if t.RowsAffected() == 0 {
		errs.Write(w, errs.NotFound("group not found"))
		return
	}
	w.WriteHeader(204)
}
func (h *GroupsHandler) Run(w http.ResponseWriter, r *http.Request) {
	g, e := h.group(r, chi.URLParam(r, "id"))
	if e != nil {
		errs.Write(w, errs.NotFound("group not found"))
		return
	}
	errs.WriteJSON(w, 202, map[string]any{"group": g, "status": "accepted", "message": "group run queued"})
}
