package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

// mcpToolsID returns a deterministic UUID for the mirrored tools-table entry.
// Same (workspaceID + serverID + toolName) always produces the same UUID so that
// agent_tools FK references survive MCP server re-syncs.
func mcpToolsID(workspaceID, serverID, toolName string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(workspaceID+":"+serverID+":"+toolName)).String()
}

type MCPHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewMCPHandler(pool *pgxpool.Pool, cfg *config.Config) *MCPHandler {
	return &MCPHandler{pool: pool, cfg: cfg}
}

const mcpSelect = `SELECT id::text,workspace_id::text,name,url,transport,status,auth_type,config,tools_synced_at,created_by::text,created_at,updated_at FROM mcp_servers`

func scanMCP(row interface{ Scan(...any) error }) (domain.MCPServer, error) {
	var s domain.MCPServer
	err := row.Scan(&s.ID, &s.WorkspaceID, &s.Name, &s.URL, &s.Transport, &s.Status, &s.AuthType, &s.Config, &s.ToolsSyncedAt, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
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
		s.Config = redactMCPConfig(s.Config)
		list = append(list, s)
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}
func (h *MCPHandler) Create(w http.ResponseWriter, r *http.Request) {
	if h.cfg.DemoMode {
		errs.Write(w, errs.Forbidden("MCP server creation is not available in demo mode"))
		return
	}
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
	s.Config = encryptMCPConfig([]byte(h.cfg.EncryptionKey), s.Config)
	err := h.pool.QueryRow(r.Context(), `INSERT INTO mcp_servers(id,workspace_id,name,url,transport,status,config,created_by) VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8::uuid) RETURNING created_at,updated_at`, s.ID, s.WorkspaceID, s.Name, s.URL, s.Transport, s.Status, s.Config, s.CreatedBy).Scan(&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		errs.Write(w, errs.Internal("failed to create MCP server"))
		return
	}
	writeAudit(r, h.pool, "mcp_server.created", "mcp_server", s.ID)
	s.Config = redactMCPConfig(s.Config)
	errs.WriteJSON(w, http.StatusCreated, s)
}
func (h *MCPHandler) Get(w http.ResponseWriter, r *http.Request) {
	s, err := scanMCP(h.pool.QueryRow(r.Context(), mcpSelect+` WHERE id=$1::uuid AND workspace_id=$2::uuid`, chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context())))
	if err != nil {
		errs.Write(w, errs.NotFound("MCP server not found"))
		return
	}
	s.Config = redactMCPConfig(s.Config)
	errs.WriteJSON(w, http.StatusOK, s)
}
func (h *MCPHandler) Delete(w http.ResponseWriter, r *http.Request) {
	mcpID := chi.URLParam(r, "id")
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	// Clean up mirrored tools entries before deleting the server.
	h.pool.Exec(r.Context(), //nolint:errcheck
		`DELETE FROM tools WHERE type='mcp' AND workspace_id=$1::uuid AND (config->>'server_id')=$2`,
		wsID, mcpID,
	)
	tag, err := h.pool.Exec(r.Context(), `DELETE FROM mcp_servers WHERE id=$1::uuid AND workspace_id=$2::uuid`, mcpID, wsID)
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

	count, err := h.syncServerTools(r.Context(), s)
	if err != nil {
		errs.Write(w, errs.BadRequest("failed to connect to MCP server: "+err.Error()))
		return
	}

	s.Status = "connected"
	writeAudit(r, h.pool, "mcp_server.synced", "mcp_server", s.ID)
	s.Config = redactMCPConfig(s.Config)
	errs.WriteJSON(w, http.StatusOK, map[string]any{"server": s, "tools_discovered": count})
}

// syncServerTools connects to the MCP server, lists its tools, and replaces
// the mcp_tools rows plus the mirrored tools-table entries. Shared by the
// Sync endpoint and the post-OAuth auto-sync.
func (h *MCPHandler) syncServerTools(ctx context.Context, s domain.MCPServer) (int, error) {
	client, err := mcpClientForServer(ctx, h.pool, h.cfg, s.ID, s.URL, s.Transport, s.AuthType, s.Config)
	if err != nil {
		return 0, err
	}
	tools, listErr := client.ListTools(ctx)
	if listErr != nil {
		slog.Warn("mcp sync failed", "server_id", s.ID, "transport", s.Transport, "error", listErr)
		h.pool.Exec(ctx, `UPDATE mcp_servers SET status='error',updated_at=NOW() WHERE id=$1::uuid`, s.ID) //nolint:errcheck
		return 0, listErr
	}

	// Replace all tools for this server atomically.
	err = repository.WithTx(ctx, h.pool, func(tx pgx.Tx) error {
		tx.Exec(ctx, `DELETE FROM mcp_tools WHERE server_id=$1::uuid`, s.ID) //nolint:errcheck
		for _, t := range tools {
			mcpID := uuid.NewString()
			tx.Exec(ctx, //nolint:errcheck
				`INSERT INTO mcp_tools(id,server_id,name,description,input_schema,risk_level,enabled)
			 VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7)
			 ON CONFLICT(server_id,name) DO UPDATE SET description=$4,input_schema=$5,risk_level=$6,updated_at=NOW()`,
				mcpID, s.ID, t.Name, t.Description, t.InputSchema, t.RiskLevel, t.Enabled,
			)
			// Mirror into tools table so agent assignment (agent_tools FK) works.
			// Use a deterministic UUID so re-syncs don't orphan existing agent_tools rows.
			// Do NOT overwrite risk_level on conflict — preserve user edits.
			toolsID := mcpToolsID(s.WorkspaceID, s.ID, t.Name)
			cfg, _ := json.Marshal(map[string]string{"server_id": s.ID, "server_name": s.Name})
			tx.Exec(ctx, //nolint:errcheck
				`INSERT INTO tools(id,workspace_id,name,description,type,input_schema,output_schema,config,risk_level,requires_approval,timeout_ms,enabled)
			 VALUES($1::uuid,$2::uuid,$3,$4,'mcp',$5,'{}',$6,$7,false,30000,true)
			 ON CONFLICT(workspace_id,name) DO UPDATE SET
			   description=EXCLUDED.description,
			   input_schema=EXCLUDED.input_schema,
			   config=EXCLUDED.config,
			   updated_at=NOW()`,
				toolsID, s.WorkspaceID, t.Name, t.Description, t.InputSchema, cfg, t.RiskLevel,
			)
		}
		tx.Exec(ctx, `UPDATE mcp_servers SET status='connected',tools_synced_at=NOW(),updated_at=NOW() WHERE id=$1::uuid`, s.ID) //nolint:errcheck
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("failed to sync tools: %w", err)
	}
	return len(tools), nil
}
func (h *MCPHandler) UpdateToolRisk(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	serverID := chi.URLParam(r, "id")
	toolID := chi.URLParam(r, "toolId")
	var req struct {
		RiskLevel string `json:"risk_level"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.RiskLevel == "" {
		errs.Write(w, errs.BadRequest("risk_level is required"))
		return
	}
	valid := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
	if !valid[req.RiskLevel] {
		errs.Write(w, errs.BadRequest("risk_level must be low, medium, high, or critical"))
		return
	}
	// Update mcp_tools.
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE mcp_tools SET risk_level=$1,updated_at=NOW()
		 WHERE id=$2::uuid AND server_id IN (SELECT id FROM mcp_servers WHERE workspace_id=$3::uuid)`,
		req.RiskLevel, toolID, wsID)
	if err != nil || tag.RowsAffected() == 0 {
		errs.Write(w, errs.NotFound("tool not found"))
		return
	}
	// Mirror the change to tools table (by server_id + server tool name lookup).
	h.pool.Exec(r.Context(), //nolint:errcheck
		`UPDATE tools SET risk_level=$1,updated_at=NOW()
		 WHERE type='mcp' AND workspace_id=$2::uuid AND (config->>'server_id')=$3
		   AND name=(SELECT name FROM mcp_tools WHERE id=$4::uuid)`,
		req.RiskLevel, wsID, serverID, toolID)
	w.WriteHeader(http.StatusNoContent)
}

// TestTool handles POST /mcp-servers/{id}/tools/test — calls a real tool on
// the MCP server with user-supplied input and returns its raw result. Lets a
// user confirm a tool works right after authenticating a server, instead of
// only finding out via a failed agent run.
func (h *MCPHandler) TestTool(w http.ResponseWriter, r *http.Request) {
	if h.cfg.DemoMode {
		errs.Write(w, errs.Forbidden("MCP tool testing is not available in demo mode"))
		return
	}
	serverID := chi.URLParam(r, "id")
	wsID := middleware.WorkspaceIDFromCtx(r.Context())

	var req struct {
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Name == "" {
		errs.Write(w, errs.BadRequest("name is required"))
		return
	}
	if len(req.Input) == 0 {
		req.Input = json.RawMessage(`{}`)
	}

	// Tenant guard: executeMCPTool's own server lookup has no workspace_id
	// filter, so this is the only check standing between a caller and a
	// foreign workspace's authenticated MCP server.
	var name string
	if err := h.pool.QueryRow(r.Context(),
		`SELECT mt.name FROM mcp_tools mt JOIN mcp_servers ms ON ms.id = mt.server_id
		 WHERE mt.server_id=$1::uuid AND ms.workspace_id=$2::uuid AND mt.name=$3`,
		serverID, wsID, req.Name).Scan(&name); err != nil {
		errs.Write(w, errs.NotFound("tool not found"))
		return
	}

	dbTool := domain.Tool{
		Name:      name,
		TimeoutMs: 30000,
		Config:    json.RawMessage(`{"server_id":"` + serverID + `"}`),
	}
	res := executeMCPTool(r.Context(), h.pool, h.cfg, dbTool, req.Input)
	errs.WriteJSON(w, http.StatusOK, map[string]any{"output": res.Output, "error": res.Error, "latency_ms": res.LatencyMs})
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
