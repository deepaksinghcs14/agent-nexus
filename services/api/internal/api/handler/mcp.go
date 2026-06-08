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
	"github.com/agentNexus/agent-nexus/services/api/pkg/errs"
)

type MCPHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewMCPHandler(pool *pgxpool.Pool, cfg *config.Config) *MCPHandler {
	return &MCPHandler{pool: pool, cfg: cfg}
}

const mcpSelect = `SELECT id::text,workspace_id::text,name,url,transport,status,config,tools_synced_at,created_by::text,created_at,updated_at FROM mcp_servers`

func scanMCP(row interface{ Scan(...any) error }) (domain.MCPServer, error) {
	var s domain.MCPServer
	err := row.Scan(&s.ID, &s.WorkspaceID, &s.Name, &s.URL, &s.Transport, &s.Status, &s.Config, &s.ToolsSyncedAt, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}
func (h *MCPHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), mcpSelect+` WHERE workspace_id=$1::uuid ORDER BY created_at DESC`, middleware.WorkspaceIDFromCtx(r.Context()))
	if err != nil {
		errs.Write(w, errs.Internal("failed to list MCP servers"))
		return
	}
	defer rows.Close()
	list := []domain.MCPServer{}
	for rows.Next() {
		s, e := scanMCP(rows)
		if e != nil {
			errs.Write(w, errs.Internal("failed to read MCP servers"))
			return
		}
		list = append(list, s)
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}
func (h *MCPHandler) Create(w http.ResponseWriter, r *http.Request) {
	var s domain.MCPServer
	if json.NewDecoder(r.Body).Decode(&s) != nil || s.Name == "" || s.URL == "" {
		errs.Write(w, errs.BadRequest("name and url are required"))
		return
	}
	s.ID = uuid.NewString()
	s.WorkspaceID = middleware.WorkspaceIDFromCtx(r.Context())
	s.CreatedBy = middleware.UserIDFromCtx(r.Context())
	if s.Transport == "" {
		s.Transport = "http"
	}
	s.Status = "disconnected"
	if len(s.Config) == 0 {
		s.Config = json.RawMessage(`{}`)
	}
	err := h.pool.QueryRow(r.Context(), `INSERT INTO mcp_servers(id,workspace_id,name,url,transport,status,config,created_by) VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8::uuid) RETURNING created_at,updated_at`, s.ID, s.WorkspaceID, s.Name, s.URL, s.Transport, s.Status, s.Config, s.CreatedBy).Scan(&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		errs.Write(w, errs.Internal("failed to create MCP server"))
		return
	}
	writeAudit(r, h.pool, "mcp_server.created", "mcp_server", s.ID)
	errs.WriteJSON(w, http.StatusCreated, s)
}
func (h *MCPHandler) Get(w http.ResponseWriter, r *http.Request) {
	s, err := scanMCP(h.pool.QueryRow(r.Context(), mcpSelect+` WHERE id=$1::uuid AND workspace_id=$2::uuid`, chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context())))
	if err != nil {
		errs.Write(w, errs.NotFound("MCP server not found"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, s)
}
func (h *MCPHandler) Delete(w http.ResponseWriter, r *http.Request) {
	mcpID := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(r.Context(), `DELETE FROM mcp_servers WHERE id=$1::uuid AND workspace_id=$2::uuid`, mcpID, middleware.WorkspaceIDFromCtx(r.Context()))
	if err != nil {
		errs.Write(w, errs.Internal("failed to delete MCP server"))
		return
	}
	if tag.RowsAffected() == 0 {
		errs.Write(w, errs.NotFound("MCP server not found"))
		return
	}
	writeAudit(r, h.pool, "mcp_server.deleted", "mcp_server", mcpID)
	w.WriteHeader(http.StatusNoContent)
}
func (h *MCPHandler) Sync(w http.ResponseWriter, r *http.Request) {
	s, err := scanMCP(h.pool.QueryRow(r.Context(), mcpSelect+` WHERE id=$1::uuid AND workspace_id=$2::uuid`, chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context())))
	if err != nil {
		errs.Write(w, errs.NotFound("MCP server not found"))
		return
	}
	_, err = h.pool.Exec(r.Context(), `UPDATE mcp_servers SET status='connected',tools_synced_at=NOW(),updated_at=NOW() WHERE id=$1::uuid`, s.ID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to sync MCP server"))
		return
	}
	s.Status = "connected"
	errs.WriteJSON(w, http.StatusOK, s)
}
func (h *MCPHandler) ListTools(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `SELECT mt.id::text,mt.server_id::text,mt.name,mt.description,mt.input_schema,mt.risk_level,mt.enabled FROM mcp_tools mt JOIN mcp_servers ms ON ms.id=mt.server_id WHERE mt.server_id=$1::uuid AND ms.workspace_id=$2::uuid ORDER BY mt.name`, chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context()))
	if err != nil {
		errs.Write(w, errs.Internal("failed to list MCP tools"))
		return
	}
	defer rows.Close()
	list := []domain.MCPTool{}
	for rows.Next() {
		var t domain.MCPTool
		if rows.Scan(&t.ID, &t.ServerID, &t.Name, &t.Description, &t.InputSchema, &t.RiskLevel, &t.Enabled) != nil {
			errs.Write(w, errs.Internal("failed to read MCP tools"))
			return
		}
		list = append(list, t)
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}
