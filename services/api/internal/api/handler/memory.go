package handler

import (
	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
)

type MemoryHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
	repo *repository.MemoryRepository
}

func NewMemoryHandler(p *pgxpool.Pool, c *config.Config) *MemoryHandler {
	return &MemoryHandler{pool: p, cfg: c, repo: repository.NewMemoryRepository(p)}
}
func (h *MemoryHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	a, e := h.repo.List(r.Context(), middleware.WorkspaceIDFromCtx(r.Context()), q.Get("agent_id"), q.Get("scope"), q.Get("q"), q.Get("status"), q.Get("source"))
	if e != nil {
		errs.Write(w, errs.Internal("failed to list memories"))
		return
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
	_, e := h.pool.Exec(r.Context(), `DELETE FROM memories WHERE workspace_id=$1::uuid AND ($2='' OR agent_id=NULLIF($2,'')::uuid) AND ($3='' OR scope=$3) AND ($4='' OR content ILIKE '%'||$4||'%') AND ($5='' OR status=$5) AND ($6='' OR save_source=$6)`, middleware.WorkspaceIDFromCtx(r.Context()), q.Get("agent_id"), q.Get("scope"), q.Get("q"), q.Get("status"), q.Get("source"))
	if e != nil {
		errs.Write(w, errs.Internal("failed to delete memories"))
		return
	}
	w.WriteHeader(204)
}

func (h *MemoryHandler) Approve(w http.ResponseWriter, r *http.Request) {
	if e := h.repo.SetStatus(r.Context(), chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context()), "active"); e != nil {
		errs.Write(w, errs.Internal("failed to approve memory"))
		return
	}
	w.WriteHeader(204)
}

func (h *MemoryHandler) Reject(w http.ResponseWriter, r *http.Request) {
	if e := h.repo.SetStatus(r.Context(), chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context()), "rejected"); e != nil {
		errs.Write(w, errs.Internal("failed to reject memory"))
		return
	}
	w.WriteHeader(204)
}
