package native

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── native_list_agents ────────────────────────────────────────────────────────

type ListAgentsTool struct{ pool *pgxpool.Pool }

func NewListAgentsTool(pool *pgxpool.Pool) *ListAgentsTool { return &ListAgentsTool{pool} }

func (t *ListAgentsTool) Definition() domain.Tool {
	return domain.Tool{
		Name:             "native_list_agents",
		Description:      "List all active agents in the current workspace. Returns id, name, description, provider, and model for each agent.",
		Type:             "native",
		InputSchema:      json.RawMessage(`{"type":"object","properties":{}}`),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}
func (t *ListAgentsTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_list_agents requires run context")
}
func (t *ListAgentsTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, _ map[string]any) (any, error) {
	rows, err := t.pool.Query(ctx,
		`SELECT id::text, name, COALESCE(description,''), provider, model FROM agents
		 WHERE workspace_id=$1::uuid AND status='active' ORDER BY name`,
		execCtx.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type entry struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Provider    string `json:"provider"`
		Model       string `json:"model"`
	}
	var out []entry
	for rows.Next() {
		var e entry
		if rows.Scan(&e.ID, &e.Name, &e.Description, &e.Provider, &e.Model) == nil {
			out = append(out, e)
		}
	}
	if out == nil {
		out = []entry{}
	}
	return map[string]any{"agents": out, "count": len(out)}, nil
}

// ── native_call_agent ─────────────────────────────────────────────────────────

type CallAgentTool struct{ pool *pgxpool.Pool }

func NewCallAgentTool(pool *pgxpool.Pool) *CallAgentTool { return &CallAgentTool{pool} }

func (t *CallAgentTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id": map[string]any{"type": "string", "description": "UUID of the agent to invoke."},
			"task":     map[string]any{"type": "string", "description": "The task or question to send to the agent."},
		},
		"required": []string{"agent_id", "task"},
	})
	return domain.Tool{
		Name:             "native_call_agent",
		Description:      "Invoke another workspace agent as a sub-task. Multiple calls in one response run in parallel. Returns the agent's output string.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        300000,
		Enabled:          true,
	}
}
func (t *CallAgentTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_call_agent requires run context")
}
func (t *CallAgentTool) Parallelizable() bool { return true }
func (t *CallAgentTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	agentID, _ := input["agent_id"].(string)
	task, _ := input["task"].(string)
	if agentID == "" || task == "" {
		return nil, fmt.Errorf("native_call_agent: agent_id and task are required")
	}
	if agentID == execCtx.AgentID {
		return nil, fmt.Errorf("native_call_agent: an agent cannot call itself")
	}
	if execCtx.CallAgent == nil {
		if execCtx.InvokeDepth >= 3 {
			return nil, fmt.Errorf("native_call_agent: max recursion depth (3) reached")
		}
		return nil, fmt.Errorf("native_call_agent: sub-agent calls are not available in this execution context (supervisor mode). Run this agent directly from the playground, not as part of a workflow")
	}
	output, err := execCtx.CallAgent(ctx, agentID, task)
	if err != nil {
		return nil, fmt.Errorf("native_call_agent: sub-agent failed: %w", err)
	}
	return map[string]any{"output": output, "agent_id": agentID}, nil
}

// ── native_create_agent ───────────────────────────────────────────────────────

type CreateAgentTool struct{ pool *pgxpool.Pool }

func NewCreateAgentTool(pool *pgxpool.Pool) *CreateAgentTool { return &CreateAgentTool{pool} }

func (t *CreateAgentTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":         map[string]any{"type": "string", "description": "Agent name."},
			"instructions": map[string]any{"type": "string", "description": "System instructions for the agent."},
			"description":  map[string]any{"type": "string", "description": "Short description of what this agent does."},
			"tool_names":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tool names to attach."},
			"skill_names":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Skill names to attach to the agent at creation."},
			"ephemeral":    map[string]any{"type": "boolean", "description": "If true, agent is deleted when this run ends. Default true; set false only when the user explicitly asks to keep it."},
		},
		"required": []string{"name", "instructions"},
	})
	return domain.Tool{
		Name:             "native_create_agent",
		Description:      "Create a new agent in the workspace. Agents are temporary by default and auto-delete when this run ends unless ephemeral=false is explicitly provided. Use native_call_agent to invoke it.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        10000,
		Enabled:          true,
	}
}
func (t *CreateAgentTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_create_agent requires run context")
}
func (t *CreateAgentTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	name, _ := input["name"].(string)
	instructions, _ := input["instructions"].(string)
	description, _ := input["description"].(string)
	ephemeral := ephemeralFromInput(input)

	var toolNames []string
	if tn, ok := input["tool_names"].([]any); ok {
		for _, n := range tn {
			if s, ok := n.(string); ok {
				toolNames = append(toolNames, s)
			}
		}
	}

	var skillNames []string
	if sn, ok := input["skill_names"].([]any); ok {
		for _, n := range sn {
			if s, ok := n.(string); ok {
				skillNames = append(skillNames, s)
			}
		}
	}

	agentID, err := createAgentRecordFromTool(ctx, t.pool, execCtx.WorkspaceID, execCtx.UserID, createToolAgentParams{
		Name:         name,
		Description:  description,
		Instructions: instructions,
		Provider:     execCtx.AgentProvider,
		Model:        execCtx.AgentModel,
		ToolNames:    toolNames,
		SourceRunID:  execCtx.RunID,
		Ephemeral:    ephemeral,
	})
	if err != nil {
		return nil, err
	}

	// Attach skills if provided.
	for _, skillName := range skillNames {
		var skillID string
		if err := t.pool.QueryRow(ctx,
			`SELECT id::text FROM skills
			 WHERE name=$1 AND (workspace_id IS NULL OR workspace_id=$2::uuid) AND enabled=true
			 ORDER BY workspace_id NULLS LAST LIMIT 1`,
			skillName, execCtx.WorkspaceID).Scan(&skillID); err != nil {
			continue
		}
		t.pool.Exec(ctx, //nolint:errcheck
			`INSERT INTO agent_skills(agent_id, skill_id, enabled, order_index)
			 SELECT $1::uuid, $2::uuid, true,
			   COALESCE((SELECT MAX(order_index)+1 FROM agent_skills WHERE agent_id=$1::uuid), 0)
			 ON CONFLICT DO NOTHING`,
			agentID, skillID)
		attachRequiredToolsForSkill(ctx, t.pool, agentID, skillID) //nolint:errcheck
	}

	return map[string]any{"agent_id": agentID, "name": name, "ephemeral": ephemeral}, nil
}

// ── native_delete_agent ───────────────────────────────────────────────────────

type DeleteAgentTool struct{ pool *pgxpool.Pool }

func NewDeleteAgentTool(pool *pgxpool.Pool) *DeleteAgentTool { return &DeleteAgentTool{pool} }

func (t *DeleteAgentTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type":       "object",
		"properties": map[string]any{"agent_id": map[string]any{"type": "string", "description": "UUID of the agent to delete."}},
		"required":   []string{"agent_id"},
	})
	return domain.Tool{
		Name:             "native_delete_agent",
		Description:      "Delete an agent. Only agents created by the current run (via native_create_agent) can be deleted this way.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "high",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}
func (t *DeleteAgentTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_delete_agent requires run context")
}
func (t *DeleteAgentTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	agentID, _ := input["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	var runID string
	err := t.pool.QueryRow(ctx,
		`SELECT COALESCE(source_run_id::text,'') FROM agents WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		agentID, execCtx.WorkspaceID).Scan(&runID)
	if err != nil {
		return nil, fmt.Errorf("agent not found")
	}
	if runID != execCtx.RunID {
		return nil, fmt.Errorf("permission denied: can only delete agents created by the current run")
	}
	if _, err := t.pool.Exec(ctx, `DELETE FROM agents WHERE id=$1::uuid`, agentID); err != nil {
		return nil, err
	}
	return map[string]any{"deleted": true, "agent_id": agentID}, nil
}

// ── native_update_agent ───────────────────────────────────────────────────────

type UpdateAgentTool struct{ pool *pgxpool.Pool }

func NewUpdateAgentTool(pool *pgxpool.Pool) *UpdateAgentTool { return &UpdateAgentTool{pool} }

func (t *UpdateAgentTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id":       map[string]any{"type": "string", "description": "UUID of the agent to update."},
			"name":           map[string]any{"type": "string", "description": "New name for the agent."},
			"description":    map[string]any{"type": "string", "description": "New description."},
			"instructions":   map[string]any{"type": "string", "description": "New system instructions."},
			"provider":       map[string]any{"type": "string", "description": "LLM provider (anthropic, openai, gemini, ollama)."},
			"model":          map[string]any{"type": "string", "description": "Model ID."},
			"temperature":    map[string]any{"type": "number", "description": "Sampling temperature."},
			"max_tokens":     map[string]any{"type": "integer", "description": "Maximum tokens per response."},
			"max_steps":      map[string]any{"type": "integer", "description": "Maximum tool-call steps per run."},
			"memory_enabled": map[string]any{"type": "boolean", "description": "Enable or disable memory for this agent."},
			"memory_scope":   map[string]any{"type": "string", "enum": []string{"conversation", "agent", "workspace"}, "description": "Memory scope: conversation, agent, or workspace."},
			"status":         map[string]any{"type": "string", "enum": []string{"active", "paused"}, "description": "Agent status: active or paused."},
		},
		"required": []string{"agent_id"},
	})
	return domain.Tool{
		Name:             "native_update_agent",
		Description:      "Update any agent in the workspace. Only the fields you provide are changed. Status can be set to active or paused (not archived).",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}
func (t *UpdateAgentTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_update_agent requires run context")
}
func (t *UpdateAgentTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	agentID, _ := input["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	// Verify agent exists in workspace.
	var exists bool
	err := t.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM agents WHERE id=$1::uuid AND workspace_id=$2::uuid)`,
		agentID, execCtx.WorkspaceID).Scan(&exists)
	if err != nil || !exists {
		return nil, fmt.Errorf("agent not found in workspace")
	}

	// Validate status if provided.
	if s, ok := input["status"].(string); ok && s != "" {
		if s != "active" && s != "paused" {
			return nil, fmt.Errorf("status must be 'active' or 'paused' (archived is not allowed)")
		}
	}

	// Build dynamic SET clause.
	setClauses := []string{}
	args := []any{}
	argIdx := 1

	addField := func(col string, val any) {
		setClauses = append(setClauses, fmt.Sprintf("%s=$%d", col, argIdx))
		args = append(args, val)
		argIdx++
	}

	if v, ok := input["name"].(string); ok && v != "" {
		addField("name", v)
	}
	if v, ok := input["description"].(string); ok && v != "" {
		addField("description", v)
	}
	if v, ok := input["instructions"].(string); ok && v != "" {
		addField("instructions", v)
	}
	if v, ok := input["provider"].(string); ok && v != "" {
		addField("provider", v)
	}
	if v, ok := input["model"].(string); ok && v != "" {
		addField("model", v)
	}
	if v, ok := input["temperature"].(float64); ok {
		addField("temperature", v)
	}
	if v, ok := input["max_tokens"].(float64); ok {
		addField("max_tokens", int(v))
	}
	if v, ok := input["max_steps"].(float64); ok {
		addField("max_steps", int(v))
	}
	if v, ok := input["memory_enabled"].(bool); ok {
		addField("memory_enabled", v)
	}
	if v, ok := input["memory_scope"].(string); ok && v != "" {
		addField("memory_scope", v)
	}
	if v, ok := input["status"].(string); ok && v != "" {
		addField("status", v)
	}

	if len(setClauses) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	// Append updated_at and the WHERE args.
	setClauses = append(setClauses, "updated_at=NOW()")
	query := fmt.Sprintf(
		`UPDATE agents SET %s WHERE id=$%d::uuid AND workspace_id=$%d::uuid`,
		strings.Join(setClauses, ", "),
		argIdx, argIdx+1,
	)
	args = append(args, agentID, execCtx.WorkspaceID)

	tag, err := t.pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update agent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("agent not found")
	}
	return map[string]any{"updated": true, "agent_id": agentID}, nil
}

// ── internal helper ───────────────────────────────────────────────────────────

type createToolAgentParams struct {
	Name         string
	Description  string
	Instructions string
	Provider     string
	Model        string
	ToolNames    []string
	SourceRunID  string
	Ephemeral    bool
}

func createAgentRecordFromTool(ctx context.Context, pool *pgxpool.Pool, ws, uid string, p createToolAgentParams) (string, error) {
	if p.Name == "" || p.Instructions == "" || p.Provider == "" || p.Model == "" {
		return "", fmt.Errorf("name, instructions, provider, and model are required")
	}
	agentID := uuid.NewString()
	_, err := pool.Exec(ctx, `
		INSERT INTO agents(id, workspace_id, name, description, instructions, provider, model,
		  temperature, max_tokens, max_steps, max_tool_calls, max_duration_secs,
		  memory_scope, status, created_by, source_run_id, ephemeral)
		VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,
		  0.7,4096,10,20,300,
		  'conversation','active',$8::uuid,
		  CASE WHEN $9='' THEN NULL ELSE $9::uuid END, $10)`,
		agentID, ws, p.Name, p.Description, p.Instructions, p.Provider, p.Model,
		uid, p.SourceRunID, p.Ephemeral)
	if err != nil {
		return "", fmt.Errorf("create agent: %w", err)
	}
	for _, name := range p.ToolNames {
		pool.Exec(ctx, //nolint:errcheck
			`INSERT INTO agent_tools(agent_id,tool_id,enabled)
			 SELECT $1::uuid,id,true FROM tools WHERE name=$2 AND (workspace_id IS NULL OR workspace_id=$3::uuid)
			 ON CONFLICT DO NOTHING`,
			agentID, name, ws)
	}
	return agentID, nil
}
