package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools/native"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

type ToolsHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
	repo *repository.ToolRepository
}

func NewToolsHandler(pool *pgxpool.Pool, cfg *config.Config) *ToolsHandler {
	return &ToolsHandler{pool: pool, cfg: cfg, repo: repository.NewToolRepository(pool)}
}

func (h *ToolsHandler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.repo.List(r.Context(), middleware.WorkspaceIDFromCtx(r.Context()))
	if err != nil {
		errs.Write(w, errs.Internal("failed to list tools"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *ToolsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var t domain.Tool
	if json.NewDecoder(r.Body).Decode(&t) != nil || t.Name == "" {
		errs.Write(w, errs.BadRequest("name and valid request body are required"))
		return
	}
	t.ID = uuid.NewString()
	t.WorkspaceID = middleware.WorkspaceIDFromCtx(r.Context())
	if t.Type == "" {
		t.Type = "http"
	}
	if t.RiskLevel == "" {
		t.RiskLevel = "low"
	}
	if t.TimeoutMs == 0 {
		t.TimeoutMs = 30000
	}
	if len(t.InputSchema) == 0 {
		t.InputSchema = json.RawMessage(`{}`)
	}
	if len(t.OutputSchema) == 0 {
		t.OutputSchema = json.RawMessage(`{}`)
	}
	if len(t.Config) == 0 {
		t.Config = json.RawMessage(`{}`)
	}
	if err := h.repo.Create(r.Context(), &t); err != nil {
		errs.Write(w, errs.Internal("failed to create tool"))
		return
	}
	writeAudit(r, h.pool, "tool.created", "tool", t.ID)
	errs.WriteJSON(w, http.StatusCreated, t)
}

func (h *ToolsHandler) Get(w http.ResponseWriter, r *http.Request) {
	t, err := h.repo.Get(r.Context(), chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context()))
	if err != nil {
		errs.Write(w, errs.NotFound("tool not found"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, t)
}

func (h *ToolsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var t domain.Tool
	if json.NewDecoder(r.Body).Decode(&t) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	t.ID = chi.URLParam(r, "id")
	t.WorkspaceID = middleware.WorkspaceIDFromCtx(r.Context())
	if len(t.InputSchema) == 0 {
		t.InputSchema = json.RawMessage(`{}`)
	}
	if len(t.OutputSchema) == 0 {
		t.OutputSchema = json.RawMessage(`{}`)
	}
	if len(t.Config) == 0 {
		t.Config = json.RawMessage(`{}`)
	}
	found, err := h.repo.Update(r.Context(), &t)
	if err != nil {
		errs.Write(w, errs.Internal("failed to update tool"))
		return
	}
	if !found {
		errs.Write(w, errs.NotFound("tool not found"))
		return
	}
	writeAudit(r, h.pool, "tool.updated", "tool", t.ID)
	errs.WriteJSON(w, http.StatusOK, t)
}

// TestCode handles POST /tools/test-code — executes JS code against a given input and returns the result.
func (h *ToolsHandler) TestCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code  string          `json:"code"`
		Input json.RawMessage `json:"input"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Code == "" {
		errs.Write(w, errs.BadRequest("code is required"))
		return
	}
	if len(req.Input) == 0 {
		req.Input = json.RawMessage(`{}`)
	}
	result, err := native.ExecuteCodeTool(r.Context(), req.Code, req.Input)
	if err != nil {
		errs.WriteJSON(w, http.StatusOK, map[string]any{"error": err.Error()})
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (h *ToolsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	toolID := chi.URLParam(r, "id")
	found, err := h.repo.Delete(r.Context(), toolID, middleware.WorkspaceIDFromCtx(r.Context()))
	if err != nil {
		errs.Write(w, errs.Internal("failed to delete tool"))
		return
	}
	if !found {
		errs.Write(w, errs.NotFound("tool not found"))
		return
	}
	writeAudit(r, h.pool, "tool.deleted", "tool", toolID)
	w.WriteHeader(http.StatusNoContent)
}
