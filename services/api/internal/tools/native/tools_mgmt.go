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

type ListWorkspaceToolsTool struct {
	tools.RequiresRunContext
	pool *pgxpool.Pool
}

func NewListWorkspaceToolsTool(pool *pgxpool.Pool) *ListWorkspaceToolsTool {
	return &ListWorkspaceToolsTool{pool: pool}
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

// resolveTargetAgent resolves the agent an attach/detach call operates on:
// explicit agent_id, then agent_name, then the calling agent itself. The
// resolved agent must belong to the caller's workspace — these tools must not
// reach across workspaces even with a guessed UUID.
func resolveTargetAgent(ctx context.Context, pool *pgxpool.Pool, execCtx tools.ExecutionContext, input map[string]any) (string, error) {
	agentID, _ := input["agent_id"].(string)
	if agentID == "" {
		if agentName, _ := input["agent_name"].(string); agentName != "" {
			pool.QueryRow(ctx, `SELECT id::text FROM agents WHERE name=$1 AND workspace_id=$2::uuid LIMIT 1`, agentName, execCtx.WorkspaceID).Scan(&agentID) //nolint:errcheck
			if agentID == "" {
				return "", fmt.Errorf("agent %q not found in this workspace", agentName)
			}
		}
	}
	if agentID == "" {
		agentID = execCtx.AgentID
	}
	if agentID == "" {
		return "", fmt.Errorf("agent_id is required")
	}
	var ok bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM agents WHERE id=$1::uuid AND workspace_id=$2::uuid)`,
		agentID, execCtx.WorkspaceID).Scan(&ok); err != nil || !ok {
		return "", fmt.Errorf("agent %s not found in this workspace", agentID)
	}
	return agentID, nil
}

// ── native_attach_tool ────────────────────────────────────────────────────────

type AttachToolTool struct {
	tools.RequiresRunContext
	pool *pgxpool.Pool
}

func NewAttachToolTool(pool *pgxpool.Pool) *AttachToolTool { return &AttachToolTool{pool: pool} }

func (t *AttachToolTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id":  map[string]any{"type": "string", "description": "UUID of the agent to attach the tool to. Defaults to the calling agent — omit to give yourself the tool."},
			"tool_name": map[string]any{"type": "string", "description": "Name of the tool to attach."},
		},
		"required": []string{"tool_name"},
	})
	return domain.Tool{
		Name:             "native_attach_tool",
		Description:      "Attach (enable) a workspace tool for an agent by name. Defaults to the calling agent, so you can give yourself any tool you discover — it becomes callable on your next turn.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}

func (t *AttachToolTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	toolName, _ := input["tool_name"].(string)
	if toolName == "" {
		return nil, fmt.Errorf("tool_name is required")
	}
	agentID, err := resolveTargetAgent(ctx, t.pool, execCtx, input)
	if err != nil {
		return nil, err
	}
	// DO UPDATE (not DO NOTHING) so attaching is idempotent and re-enables a
	// previously disabled attachment; RowsAffected then cleanly distinguishes
	// "tool doesn't exist" from "already attached".
	tag, err := t.pool.Exec(ctx,
		`INSERT INTO agent_tools(agent_id, tool_id, enabled)
		 SELECT $1::uuid, id, true FROM tools
		 WHERE name=$2 AND (workspace_id IS NULL OR workspace_id=$3::uuid) AND enabled=true
		 ON CONFLICT (agent_id, tool_id) DO UPDATE SET enabled=true`,
		agentID, toolName, execCtx.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("attach tool: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("tool not found: %s — call native_list_tools to see available names", toolName)
	}
	out := map[string]any{"attached": true, "agent_id": agentID, "tool_name": toolName}
	if agentID == execCtx.AgentID {
		// Self-attach: also activate for the current run so lazy-loading agents
		// don't need a separate native_request_tool round-trip.
		if execCtx.RequestTool != nil {
			execCtx.RequestTool(toolName)
		}
		out["message"] = fmt.Sprintf("Tool %q attached to you — call it on your next turn.", toolName)
	}
	return out, nil
}

// ── native_detach_tool ────────────────────────────────────────────────────────

type DetachToolTool struct {
	tools.RequiresRunContext
	pool *pgxpool.Pool
}

func NewDetachToolTool(pool *pgxpool.Pool) *DetachToolTool { return &DetachToolTool{pool: pool} }

func (t *DetachToolTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id":  map[string]any{"type": "string", "description": "UUID of the agent to detach the tool from. Defaults to the calling agent — omit to drop a tool you no longer need."},
			"tool_name": map[string]any{"type": "string", "description": "Name of the tool to detach."},
		},
		"required": []string{"tool_name"},
	})
	return domain.Tool{
		Name:             "native_detach_tool",
		Description:      "Detach (disable) a tool from an agent by tool name. Defaults to the calling agent.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}

func (t *DetachToolTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	toolName, _ := input["tool_name"].(string)
	if toolName == "" {
		return nil, fmt.Errorf("tool_name is required")
	}
	agentID, err := resolveTargetAgent(ctx, t.pool, execCtx, input)
	if err != nil {
		return nil, err
	}
	_, err = t.pool.Exec(ctx,
		`DELETE FROM agent_tools
		 WHERE agent_id=$1::uuid
		   AND tool_id=(SELECT id FROM tools WHERE name=$2 AND (workspace_id IS NULL OR workspace_id=$3::uuid) LIMIT 1)`,
		agentID, toolName, execCtx.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("detach tool: %w", err)
	}
	return map[string]any{"detached": true, "agent_id": agentID, "tool_name": toolName}, nil
}
