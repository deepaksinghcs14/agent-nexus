package native

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── native_list_http_tools ────────────────────────────────────────────────────

type ListHttpToolsTool struct {
	tools.RequiresRunContext
	pool *pgxpool.Pool
}

func NewListHttpToolsTool(pool *pgxpool.Pool) *ListHttpToolsTool {
	return &ListHttpToolsTool{pool: pool}
}

func (t *ListHttpToolsTool) Definition() domain.Tool {
	return domain.Tool{
		Name:             "native_list_http_tools",
		Description:      "List all HTTP tools in the current workspace. Returns id, name, and description for each.",
		Type:             "native",
		InputSchema:      json.RawMessage(`{"type":"object","properties":{}}`),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}
func (t *ListHttpToolsTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, _ map[string]any) (any, error) {
	rows, err := t.pool.Query(ctx,
		`SELECT id::text, name, COALESCE(description,'') FROM tools
		 WHERE workspace_id=$1::uuid AND type='http' AND enabled=true ORDER BY name`,
		execCtx.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type entry struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	var out []entry
	for rows.Next() {
		var e entry
		if rows.Scan(&e.ID, &e.Name, &e.Description) == nil {
			out = append(out, e)
		}
	}
	if out == nil {
		out = []entry{}
	}
	return map[string]any{"tools": out, "count": len(out)}, nil
}

// ── native_create_http_tool ───────────────────────────────────────────────────

type CreateHttpToolTool struct {
	tools.RequiresRunContext
	pool *pgxpool.Pool
}

func NewCreateHttpToolTool(pool *pgxpool.Pool) *CreateHttpToolTool {
	return &CreateHttpToolTool{pool: pool}
}

func (t *CreateHttpToolTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":         map[string]any{"type": "string", "description": "Tool name (snake_case recommended)."},
			"description":  map[string]any{"type": "string", "description": "What this tool does."},
			"url":          map[string]any{"type": "string", "description": "The HTTP endpoint URL."},
			"method":       map[string]any{"type": "string", "description": "HTTP method: GET, POST, PUT, DELETE. Default GET."},
			"headers":      map[string]any{"type": "object", "description": "Static headers to include (e.g. Authorization)."},
			"input_schema": map[string]any{"type": "object", "description": "JSON Schema for the tool's input parameters. Defaults to accepting any object."},
			"ephemeral":    map[string]any{"type": "boolean", "description": "Default true (auto-deletes at run end). Set false at creation time, or call native_promote_resource after creation once you know it's worth keeping."},
		},
		"required": []string{"name", "url"},
	})
	return domain.Tool{
		Name:             "native_create_http_tool",
		Description:      "Create a new HTTP tool that calls an external URL. Tools are temporary by default and auto-delete when this run ends unless ephemeral=false is explicitly provided.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        10000,
		Enabled:          true,
	}
}
func (t *CreateHttpToolTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	name, _ := input["name"].(string)
	description, _ := input["description"].(string)
	url, _ := input["url"].(string)
	method, _ := input["method"].(string)
	ephemeral := ephemeralFromInput(input)
	if name == "" || url == "" {
		return nil, fmt.Errorf("name and url are required")
	}
	if method == "" {
		method = "GET"
	}

	headers := map[string]string{}
	if h, ok := input["headers"].(map[string]any); ok {
		for k, v := range h {
			if s, ok := v.(string); ok {
				headers[k] = s
			}
		}
	}

	inputSchema := map[string]any{"type": "object", "properties": map[string]any{}}
	if is, ok := input["input_schema"].(map[string]any); ok {
		inputSchema = is
	}

	cfg := map[string]any{
		"url":     url,
		"method":  method,
		"headers": headers,
	}
	cfgJSON, _ := json.Marshal(cfg)
	schemaJSON, _ := json.Marshal(inputSchema)

	toolID := uuid.NewString()
	_, err := t.pool.Exec(ctx, `
		INSERT INTO tools(id, workspace_id, name, description, type, input_schema, output_schema, config, risk_level, requires_approval, timeout_ms, enabled, source_run_id, ephemeral)
		VALUES($1::uuid,$2::uuid,$3,$4,'http',$5::jsonb,'{}','{}','medium',false,30000,true,
		  CASE WHEN $6='' THEN NULL ELSE $6::uuid END,$7)`,
		toolID, execCtx.WorkspaceID, name, description, json.RawMessage(schemaJSON),
		execCtx.RunID, ephemeral)
	if err != nil {
		return nil, fmt.Errorf("create http tool: %w", err)
	}
	// Store config separately (tools table has config column)
	t.pool.Exec(ctx, `UPDATE tools SET config=$2::jsonb WHERE id=$1::uuid`, toolID, json.RawMessage(cfgJSON)) //nolint:errcheck

	// Auto-attach to calling agent so it's immediately usable
	if execCtx.AgentID != "" {
		t.pool.Exec(ctx, //nolint:errcheck
			`INSERT INTO agent_tools(agent_id,tool_id,enabled)
			 VALUES($1::uuid,$2::uuid,true) ON CONFLICT DO NOTHING`,
			execCtx.AgentID, toolID)
	}

	return map[string]any{
		"tool_id":   toolID,
		"name":      name,
		"ephemeral": ephemeral,
		"note":      fmt.Sprintf("HTTP tool '%s' created and attached to this agent. It will be available on the next turn.", name),
	}, nil
}

// ── native_delete_tool ────────────────────────────────────────────────────────

type DeleteToolTool struct {
	tools.RequiresRunContext
	pool *pgxpool.Pool
}

func NewDeleteToolTool(pool *pgxpool.Pool) *DeleteToolTool { return &DeleteToolTool{pool: pool} }

func (t *DeleteToolTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type":       "object",
		"properties": map[string]any{"tool_id": map[string]any{"type": "string", "description": "UUID of the tool to delete."}},
		"required":   []string{"tool_id"},
	})
	return domain.Tool{
		Name:             "native_delete_tool",
		Description:      "Delete an HTTP tool. Only tools created by the current run (via native_create_http_tool) can be deleted this way.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "high",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}
func (t *DeleteToolTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	toolID, _ := input["tool_id"].(string)
	if toolID == "" {
		return nil, fmt.Errorf("tool_id is required")
	}
	var srcRunID string
	err := t.pool.QueryRow(ctx,
		`SELECT COALESCE(source_run_id::text,'') FROM tools WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		toolID, execCtx.WorkspaceID).Scan(&srcRunID)
	if err != nil {
		return nil, fmt.Errorf("tool not found")
	}
	if srcRunID != execCtx.RunID {
		return nil, fmt.Errorf("permission denied: can only delete tools created by the current run")
	}
	if _, err := t.pool.Exec(ctx, `DELETE FROM tools WHERE id=$1::uuid`, toolID); err != nil {
		return nil, err
	}
	return map[string]any{"deleted": true, "tool_id": toolID}, nil
}
