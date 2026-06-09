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
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

type ToolsHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewToolsHandler(pool *pgxpool.Pool, cfg *config.Config) *ToolsHandler {
	return &ToolsHandler{pool: pool, cfg: cfg}
}

func scanTool(rows interface{ Scan(...any) error }) (domain.Tool, error) {
	var t domain.Tool
	err := rows.Scan(&t.ID, &t.WorkspaceID, &t.Name, &t.Description, &t.Type, &t.InputSchema, &t.OutputSchema, &t.Config, &t.RiskLevel, &t.RequiresApproval, &t.TimeoutMs, &t.Enabled, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

const toolSelect = `SELECT id::text, COALESCE(workspace_id::text,''), name, description, type, input_schema, output_schema, config, risk_level, requires_approval, timeout_ms, enabled, created_at, updated_at FROM tools`

func (h *ToolsHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	rows, err := h.pool.Query(r.Context(), toolSelect+` WHERE workspace_id IS NULL OR workspace_id=$1::uuid ORDER BY type,name`, wsID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list tools"))
		return
	}
	defer rows.Close()
	list := []domain.Tool{}
	for rows.Next() {
		t, e := scanTool(rows)
		if e != nil {
			errs.Write(w, errs.Internal("failed to read tools"))
			return
		}
		list = append(list, t)
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
	err := h.pool.QueryRow(r.Context(), `INSERT INTO tools(id,workspace_id,name,description,type,input_schema,output_schema,config,risk_level,requires_approval,timeout_ms,enabled) VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING created_at,updated_at`, t.ID, t.WorkspaceID, t.Name, t.Description, t.Type, t.InputSchema, t.OutputSchema, t.Config, t.RiskLevel, t.RequiresApproval, t.TimeoutMs, t.Enabled).Scan(&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		errs.Write(w, errs.Internal("failed to create tool"))
		return
	}
	writeAudit(r, h.pool, "tool.created", "tool", t.ID)
	errs.WriteJSON(w, http.StatusCreated, t)
}
func (h *ToolsHandler) Get(w http.ResponseWriter, r *http.Request) {
	t, err := scanTool(h.pool.QueryRow(r.Context(), toolSelect+` WHERE id=$1::uuid AND (workspace_id IS NULL OR workspace_id=$2::uuid)`, chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context())))
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
	if len(t.Config) == 0 {
		t.Config = json.RawMessage(`{}`)
	}
	tag, err := h.pool.Exec(r.Context(), `UPDATE tools SET name=$3,description=$4,type=$5,input_schema=$6,output_schema=$7,config=$8,risk_level=$9,requires_approval=$10,timeout_ms=$11,enabled=$12,updated_at=NOW() WHERE id=$1::uuid AND workspace_id=$2::uuid`, t.ID, t.WorkspaceID, t.Name, t.Description, t.Type, t.InputSchema, t.OutputSchema, t.Config, t.RiskLevel, t.RequiresApproval, t.TimeoutMs, t.Enabled)
	if err != nil {
		errs.Write(w, errs.Internal("failed to update tool"))
		return
	}
	if tag.RowsAffected() == 0 {
		errs.Write(w, errs.NotFound("tool not found"))
		return
	}
	writeAudit(r, h.pool, "tool.updated", "tool", t.ID)
	errs.WriteJSON(w, http.StatusOK, t)
}
func (h *ToolsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	toolID := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(r.Context(), `DELETE FROM tools WHERE id=$1::uuid AND workspace_id=$2::uuid`, toolID, middleware.WorkspaceIDFromCtx(r.Context()))
	if err != nil {
		errs.Write(w, errs.Internal("failed to delete tool"))
		return
	}
	if tag.RowsAffected() == 0 {
		errs.Write(w, errs.NotFound("tool not found"))
		return
	}
	writeAudit(r, h.pool, "tool.deleted", "tool", toolID)
	w.WriteHeader(http.StatusNoContent)
}
