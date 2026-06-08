package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentNexus/agent-nexus/services/api/internal/api/middleware"
	"github.com/agentNexus/agent-nexus/services/api/internal/config"
	"github.com/agentNexus/agent-nexus/services/api/internal/domain"
	"github.com/agentNexus/agent-nexus/services/api/internal/repository"
	"github.com/agentNexus/agent-nexus/services/api/pkg/errs"
)

type AgentsHandler struct {
	pool   *pgxpool.Pool
	cfg    *config.Config
	agents *repository.AgentRepository
}

func NewAgentsHandler(pool *pgxpool.Pool, cfg *config.Config) *AgentsHandler {
	return &AgentsHandler{pool: pool, cfg: cfg, agents: repository.NewAgentRepository(pool)}
}

func (h *AgentsHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	list, err := h.agents.List(r.Context(), wsID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list agents"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *AgentsHandler) Create(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	userID := middleware.UserIDFromCtx(r.Context())

	var req struct {
		Name                    string  `json:"name"`
		Description             string  `json:"description"`
		Instructions            string  `json:"instructions"`
		Provider                string  `json:"provider"`
		Model                   string  `json:"model"`
		Temperature             float64 `json:"temperature"`
		MaxTokens               int     `json:"max_tokens"`
		MemoryEnabled           bool    `json:"memory_enabled"`
		MemoryScope             string  `json:"memory_scope"`
		ContextRetrievalEnabled bool    `json:"context_retrieval_enabled"`
		MaxSteps                int     `json:"max_steps"`
		MaxToolCalls            int     `json:"max_tool_calls"`
		MaxDurationSecs         int     `json:"max_duration_secs"`
		Status                  string  `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if req.Name == "" {
		errs.Write(w, errs.BadRequest("name is required"))
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if req.MemoryScope == "" {
		req.MemoryScope = "conversation"
	}
	if req.MaxSteps == 0 {
		req.MaxSteps = 10
	}
	if req.MaxToolCalls == 0 {
		req.MaxToolCalls = 20
	}
	if req.MaxDurationSecs == 0 {
		req.MaxDurationSecs = 300
	}

	a := &domain.Agent{
		ID:                      uuid.New().String(),
		WorkspaceID:             wsID,
		Name:                    req.Name,
		Description:             req.Description,
		Instructions:            req.Instructions,
		Provider:                req.Provider,
		Model:                   req.Model,
		Temperature:             req.Temperature,
		MaxTokens:               req.MaxTokens,
		MemoryEnabled:           req.MemoryEnabled,
		MemoryScope:             req.MemoryScope,
		ContextRetrievalEnabled: req.ContextRetrievalEnabled,
		MaxSteps:                req.MaxSteps,
		MaxToolCalls:            req.MaxToolCalls,
		MaxDurationSecs:         req.MaxDurationSecs,
		Status:                  req.Status,
		CreatedBy:               userID,
	}

	if err := h.agents.Create(r.Context(), a); err != nil {
		errs.Write(w, errs.Internal("failed to create agent"))
		return
	}
	writeAudit(r, h.pool, "agent.created", "agent", a.ID)
	errs.WriteJSON(w, http.StatusCreated, a)
}

func (h *AgentsHandler) Get(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	a, err := h.agents.Get(r.Context(), chi.URLParam(r, "id"), wsID)
	if err != nil {
		errs.Write(w, errs.NotFound("agent not found"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, a)
}

func (h *AgentsHandler) Update(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	id := chi.URLParam(r, "id")

	existing, err := h.agents.Get(r.Context(), id, wsID)
	if err != nil {
		errs.Write(w, errs.NotFound("agent not found"))
		return
	}

	if err := json.NewDecoder(r.Body).Decode(existing); err != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	existing.ID = id
	existing.WorkspaceID = wsID

	if err := h.agents.Update(r.Context(), existing); err != nil {
		errs.Write(w, errs.Internal("failed to update agent"))
		return
	}
	writeAudit(r, h.pool, "agent.updated", "agent", existing.ID)
	errs.WriteJSON(w, http.StatusOK, existing)
}

func (h *AgentsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	agentID := chi.URLParam(r, "id")
	if err := h.agents.Delete(r.Context(), agentID, wsID); err != nil {
		errs.Write(w, errs.Internal("failed to delete agent"))
		return
	}
	writeAudit(r, h.pool, "agent.deleted", "agent", agentID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AgentsHandler) ListTools(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	agentID := chi.URLParam(r, "id")
	var exists bool
	if err := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM agents WHERE id=$1::uuid AND workspace_id=$2::uuid)`, agentID, wsID).Scan(&exists); err != nil || !exists {
		errs.Write(w, errs.NotFound("agent not found"))
		return
	}
	rows, err := h.pool.Query(r.Context(),
		`SELECT t.id::text, COALESCE(t.workspace_id::text,''), t.name, t.description, t.type,
		        t.input_schema, t.output_schema, t.risk_level, t.requires_approval,
		        t.timeout_ms, COALESCE(at.enabled, t.enabled), t.created_at, t.updated_at
		 FROM agent_tools at
		 JOIN tools t ON t.id = at.tool_id
		 WHERE at.agent_id=$1::uuid
		 ORDER BY t.name`, agentID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list agent tools"))
		return
	}
	defer rows.Close()
	tools := []domain.Tool{}
	for rows.Next() {
		var t domain.Tool
		if err := rows.Scan(&t.ID, &t.WorkspaceID, &t.Name, &t.Description, &t.Type, &t.InputSchema, &t.OutputSchema, &t.RiskLevel, &t.RequiresApproval, &t.TimeoutMs, &t.Enabled, &t.CreatedAt, &t.UpdatedAt); err != nil {
			errs.Write(w, errs.Internal("failed to read agent tools"))
			return
		}
		tools = append(tools, t)
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": tools})
}

func (h *AgentsHandler) SetTools(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	agentID := chi.URLParam(r, "id")
	var req struct {
		ToolIDs   []string `json:"tool_ids"`
		ToolNames []string `json:"tool_names"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	var exists bool
	if err := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM agents WHERE id=$1::uuid AND workspace_id=$2::uuid)`, agentID, wsID).Scan(&exists); err != nil || !exists {
		errs.Write(w, errs.NotFound("agent not found"))
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		errs.Write(w, errs.Internal("failed to update tools"))
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `DELETE FROM agent_tools WHERE agent_id=$1::uuid`, agentID); err != nil {
		errs.Write(w, errs.Internal("failed to update tools"))
		return
	}
	for _, id := range req.ToolIDs {
		if _, err = tx.Exec(r.Context(), `INSERT INTO agent_tools(agent_id, tool_id, enabled) SELECT $1::uuid, id, true FROM tools WHERE id=$2::uuid AND (workspace_id IS NULL OR workspace_id=$3::uuid)`, agentID, id, wsID); err != nil {
			errs.Write(w, errs.Internal("failed to update tools"))
			return
		}
	}
	for _, name := range req.ToolNames {
		if _, err = tx.Exec(r.Context(), `INSERT INTO agent_tools(agent_id, tool_id, enabled) SELECT $1::uuid, id, true FROM tools WHERE name=$2 AND (workspace_id IS NULL OR workspace_id=$3::uuid) ON CONFLICT DO NOTHING`, agentID, name, wsID); err != nil {
			errs.Write(w, errs.Internal("failed to update tools"))
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		errs.Write(w, errs.Internal("failed to update tools"))
		return
	}
	h.ListTools(w, r)
}
