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

// ── native_list_workflows ─────────────────────────────────────────────────────

type ListWorkflowsTool struct{ pool *pgxpool.Pool }

func NewListWorkflowsTool(pool *pgxpool.Pool) *ListWorkflowsTool { return &ListWorkflowsTool{pool} }

func (t *ListWorkflowsTool) Definition() domain.Tool {
	return domain.Tool{
		Name:             "native_list_workflows",
		Description:      "List all active workflows in the workspace.",
		Type:             "native",
		InputSchema:      json.RawMessage(`{"type":"object","properties":{}}`),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}
func (t *ListWorkflowsTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_list_workflows requires run context")
}
func (t *ListWorkflowsTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, _ map[string]any) (any, error) {
	rows, err := t.pool.Query(ctx,
		`SELECT id::text, name, COALESCE(description,''), mode FROM workflows
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
		Mode        string `json:"mode"`
	}
	var out []entry
	for rows.Next() {
		var e entry
		if rows.Scan(&e.ID, &e.Name, &e.Description, &e.Mode) == nil {
			out = append(out, e)
		}
	}
	if out == nil {
		out = []entry{}
	}
	return map[string]any{"workflows": out, "count": len(out)}, nil
}

// ── native_create_workflow ────────────────────────────────────────────────────

type CreateWorkflowTool struct{ pool *pgxpool.Pool }

func NewCreateWorkflowTool(pool *pgxpool.Pool) *CreateWorkflowTool {
	return &CreateWorkflowTool{pool}
}

func (t *CreateWorkflowTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":        map[string]any{"type": "string", "description": "Workflow name."},
			"description": map[string]any{"type": "string", "description": "Short description of what this workflow does."},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"pipeline", "supervisor"},
				"description": "Execution mode: 'pipeline' runs agents sequentially, 'supervisor' uses a supervisor agent to coordinate.",
			},
			"agent_ids": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional list of agent UUIDs to add as workflow members.",
			},
		},
		"required": []string{"name", "mode"},
	})
	return domain.Tool{
		Name:             "native_create_workflow",
		Description:      "Create a new workflow. mode must be 'pipeline' or 'supervisor'. Optionally provide agent_ids to add as members.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        10000,
		Enabled:          true,
	}
}
func (t *CreateWorkflowTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_create_workflow requires run context")
}
func (t *CreateWorkflowTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	name, _ := input["name"].(string)
	description, _ := input["description"].(string)
	mode, _ := input["mode"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if mode != "pipeline" && mode != "supervisor" {
		return nil, fmt.Errorf("mode must be 'pipeline' or 'supervisor'")
	}

	var agentIDs []string
	if ids, ok := input["agent_ids"].([]any); ok {
		for _, v := range ids {
			if s, ok := v.(string); ok && s != "" {
				agentIDs = append(agentIDs, s)
			}
		}
	}

	workflowID := uuid.NewString()
	_, err := t.pool.Exec(ctx, `
		INSERT INTO workflows(id, workspace_id, name, description, mode, status, created_by, source_run_id)
		VALUES($1::uuid, $2::uuid, $3, $4, $5, 'active', $6::uuid,
		  CASE WHEN $7='' THEN NULL ELSE $7::uuid END)`,
		workflowID, execCtx.WorkspaceID, name, description, mode,
		execCtx.UserID, execCtx.RunID)
	if err != nil {
		return nil, fmt.Errorf("create workflow: %w", err)
	}

	for i, agentID := range agentIDs {
		t.pool.Exec(ctx, //nolint:errcheck
			`INSERT INTO workflow_members(id, group_id, agent_id, position, role)
			 VALUES($1::uuid, $2::uuid, $3::uuid, $4, 'member')
			 ON CONFLICT DO NOTHING`,
			uuid.NewString(), workflowID, agentID, i)
	}

	return map[string]any{
		"workflow_id":  workflowID,
		"name":         name,
		"mode":         mode,
		"member_count": len(agentIDs),
	}, nil
}

// ── native_run_workflow ───────────────────────────────────────────────────────

type RunWorkflowTool struct{ pool *pgxpool.Pool }

func NewRunWorkflowTool(pool *pgxpool.Pool) *RunWorkflowTool { return &RunWorkflowTool{pool} }

func (t *RunWorkflowTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workflow_id": map[string]any{"type": "string", "description": "UUID of the workflow to run."},
			"input":       map[string]any{"type": "string", "description": "Input text to pass to the workflow."},
		},
		"required": []string{"workflow_id", "input"},
	})
	return domain.Tool{
		Name:             "native_run_workflow",
		Description:      "Trigger a workflow run in the background. Returns the run_id immediately.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        30000,
		Enabled:          true,
	}
}
func (t *RunWorkflowTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_run_workflow requires run context")
}
func (t *RunWorkflowTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	workflowID, _ := input["workflow_id"].(string)
	userInput, _ := input["input"].(string)
	if workflowID == "" || userInput == "" {
		return nil, fmt.Errorf("workflow_id and input are required")
	}
	if execCtx.RunWorkflow == nil {
		return nil, fmt.Errorf("workflow execution not available in this context")
	}
	runID, err := execCtx.RunWorkflow(ctx, workflowID, userInput)
	if err != nil {
		return nil, fmt.Errorf("native_run_workflow: %w", err)
	}
	return map[string]any{
		"run_id":      runID,
		"workflow_id": workflowID,
		"status":      "running",
		"note":        "Workflow run started in background.",
	}, nil
}

// ── native_delete_workflow ────────────────────────────────────────────────────

type DeleteWorkflowTool struct{ pool *pgxpool.Pool }

func NewDeleteWorkflowTool(pool *pgxpool.Pool) *DeleteWorkflowTool {
	return &DeleteWorkflowTool{pool}
}

func (t *DeleteWorkflowTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workflow_id": map[string]any{"type": "string", "description": "UUID of the workflow to delete."},
		},
		"required": []string{"workflow_id"},
	})
	return domain.Tool{
		Name:             "native_delete_workflow",
		Description:      "Delete a workflow. Only workflows created by the current run can be deleted.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "high",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}
func (t *DeleteWorkflowTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_delete_workflow requires run context")
}
func (t *DeleteWorkflowTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	workflowID, _ := input["workflow_id"].(string)
	if workflowID == "" {
		return nil, fmt.Errorf("workflow_id is required")
	}
	var srcRunID string
	err := t.pool.QueryRow(ctx,
		`SELECT COALESCE(source_run_id::text,'') FROM workflows WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		workflowID, execCtx.WorkspaceID).Scan(&srcRunID)
	if err != nil {
		return nil, fmt.Errorf("workflow not found")
	}
	if srcRunID != execCtx.RunID {
		return nil, fmt.Errorf("permission denied: can only delete workflows created by the current run")
	}
	if _, err := t.pool.Exec(ctx, `DELETE FROM workflows WHERE id=$1::uuid`, workflowID); err != nil {
		return nil, err
	}
	return map[string]any{"deleted": true, "workflow_id": workflowID}, nil
}
