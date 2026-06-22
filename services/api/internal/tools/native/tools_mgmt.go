package native

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── native_list_workspace_tools ───────────────────────────────────────────────

type ListWorkspaceToolsTool struct{ pool *pgxpool.Pool }

func NewListWorkspaceToolsTool(pool *pgxpool.Pool) *ListWorkspaceToolsTool {
	return &ListWorkspaceToolsTool{pool}
}

func (t *ListWorkspaceToolsTool) Definition() domain.Tool {
	return domain.Tool{
		Name:             "native_list_workspace_tools",
		Description:      "List all tools available in the workspace (native, HTTP, MCP). Use this to discover tool names for native_attach_tool.",
		Type:             "native",
		InputSchema:      json.RawMessage(`{"type":"object","properties":{}}`),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}

func (t *ListWorkspaceToolsTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_list_workspace_tools requires run context")
}

func (t *ListWorkspaceToolsTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, _ map[string]any) (any, error) {
	rows, err := t.pool.Query(ctx,
		`SELECT id::text, name, COALESCE(description,''), type FROM tools
		 WHERE (workspace_id=$1::uuid OR workspace_id IS NULL) AND enabled=true
		 ORDER BY type, name`,
		execCtx.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type entry struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Type        string `json:"type"`
	}
	var out []entry
	for rows.Next() {
		var e entry
		if rows.Scan(&e.ID, &e.Name, &e.Description, &e.Type) == nil {
			out = append(out, e)
		}
	}
	if out == nil {
		out = []entry{}
	}
	return map[string]any{"tools": out, "count": len(out)}, nil
}

// ── native_attach_tool ────────────────────────────────────────────────────────

type AttachToolTool struct{ pool *pgxpool.Pool }

func NewAttachToolTool(pool *pgxpool.Pool) *AttachToolTool { return &AttachToolTool{pool} }

func (t *AttachToolTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id":  map[string]any{"type": "string", "description": "UUID of the agent to attach the tool to."},
			"tool_name": map[string]any{"type": "string", "description": "Name of the tool to attach."},
		},
		"required": []string{"agent_id", "tool_name"},
	})
	return domain.Tool{
		Name:             "native_attach_tool",
		Description:      "Attach any tool by name to an agent.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}

func (t *AttachToolTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_attach_tool requires run context")
}

func (t *AttachToolTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	agentID, _ := input["agent_id"].(string)
	toolName, _ := input["tool_name"].(string)
	if agentID == "" {
		if agentName, _ := input["agent_name"].(string); agentName != "" {
			t.pool.QueryRow(ctx, `SELECT id::text FROM agents WHERE name=$1 AND workspace_id=$2::uuid LIMIT 1`, agentName, execCtx.WorkspaceID).Scan(&agentID) //nolint:errcheck
		}
	}
	if agentID == "" || toolName == "" {
		return nil, fmt.Errorf("agent_id is required — pass the UUID returned by native_create_agent")
	}
	tag, err := t.pool.Exec(ctx,
		`INSERT INTO agent_tools(agent_id, tool_id, enabled)
		 SELECT $1::uuid, id, true FROM tools
		 WHERE name=$2 AND (workspace_id IS NULL OR workspace_id=$3::uuid)
		 ON CONFLICT DO NOTHING`,
		agentID, toolName, execCtx.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("attach tool: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}
	return map[string]any{"attached": true, "agent_id": agentID, "tool_name": toolName}, nil
}

// ── native_detach_tool ────────────────────────────────────────────────────────

type DetachToolTool struct{ pool *pgxpool.Pool }

func NewDetachToolTool(pool *pgxpool.Pool) *DetachToolTool { return &DetachToolTool{pool} }

func (t *DetachToolTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id":  map[string]any{"type": "string", "description": "UUID of the agent to detach the tool from."},
			"tool_name": map[string]any{"type": "string", "description": "Name of the tool to detach."},
		},
		"required": []string{"agent_id", "tool_name"},
	})
	return domain.Tool{
		Name:             "native_detach_tool",
		Description:      "Detach a tool from an agent by tool name.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}

func (t *DetachToolTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_detach_tool requires run context")
}

func (t *DetachToolTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	agentID, _ := input["agent_id"].(string)
	toolName, _ := input["tool_name"].(string)
	if agentID == "" || toolName == "" {
		return nil, fmt.Errorf("agent_id and tool_name are required")
	}
	_, err := t.pool.Exec(ctx,
		`DELETE FROM agent_tools
		 WHERE agent_id=$1::uuid
		   AND tool_id=(SELECT id FROM tools WHERE name=$2 AND (workspace_id IS NULL OR workspace_id=$3::uuid) LIMIT 1)`,
		agentID, toolName, execCtx.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("detach tool: %w", err)
	}
	return map[string]any{"detached": true, "agent_id": agentID, "tool_name": toolName}, nil
}
