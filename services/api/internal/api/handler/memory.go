package handler

import (
	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
)

type MemoryHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewMemoryHandler(p *pgxpool.Pool, c *config.Config) *MemoryHandler { return &MemoryHandler{p, c} }
func (h *MemoryHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rows, e := h.pool.Query(r.Context(), `SELECT id::text,workspace_id::text,COALESCE(agent_id::text,''),COALESCE(user_id::text,''),scope,content,relevance_score,COALESCE(source_run_id::text,''),created_at,updated_at FROM memories WHERE workspace_id=$1::uuid AND ($2='' OR agent_id=$2::uuid) AND ($3='' OR scope=$3) AND ($4='' OR content ILIKE '%'||$4||'%') ORDER BY created_at DESC`, middleware.WorkspaceIDFromCtx(r.Context()), q.Get("agent_id"), q.Get("scope"), q.Get("q"))
	if e != nil {
		errs.Write(w, errs.Internal("failed to list memories"))
		return
	}
	defer rows.Close()
	a := []domain.Memory{}
	for rows.Next() {
		var m domain.Memory
		if rows.Scan(&m.ID, &m.WorkspaceID, &m.AgentID, &m.UserID, &m.Scope, &m.Content, &m.RelevanceScore, &m.SourceRunID, &m.CreatedAt, &m.UpdatedAt) != nil {
			errs.Write(w, errs.Internal("failed to read memories"))
			return
		}
		a = append(a, m)
	}
	errs.WriteJSON(w, 200, map[string]any{"data": a})
}
func (h *MemoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	t, e := h.pool.Exec(r.Context(), `DELETE FROM memories WHERE id=$1::uuid AND workspace_id=$2::uuid`, chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context()))
	if e != nil {
		errs.Write(w, errs.Internal("failed to delete memory"))
		return
	}
	if t.RowsAffected() == 0 {
		errs.Write(w, errs.NotFound("memory not found"))
		return
	}
	w.WriteHeader(204)
}
func (h *MemoryHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	_, e := h.pool.Exec(r.Context(), `DELETE FROM memories WHERE workspace_id=$1::uuid AND ($2='' OR agent_id=$2::uuid) AND ($3='' OR scope=$3) AND ($4='' OR content ILIKE '%'||$4||'%')`, middleware.WorkspaceIDFromCtx(r.Context()), q.Get("agent_id"), q.Get("scope"), q.Get("q"))
	if e != nil {
		errs.Write(w, errs.Internal("failed to delete memories"))
		return
	}
	w.WriteHeader(204)
}
