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

// ── native_list_skills ────────────────────────────────────────────────────────

type ListSkillsTool struct{ pool *pgxpool.Pool }

func NewListSkillsTool(pool *pgxpool.Pool) *ListSkillsTool { return &ListSkillsTool{pool} }

func (t *ListSkillsTool) Definition() domain.Tool {
	return domain.Tool{
		Name:             "native_list_skills",
		Description:      "List all enabled skills in the current workspace. Returns id, name, and description for each skill.",
		Type:             "native",
		InputSchema:      json.RawMessage(`{"type":"object","properties":{}}`),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}
func (t *ListSkillsTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_list_skills requires run context")
}
func (t *ListSkillsTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, _ map[string]any) (any, error) {
	rows, err := t.pool.Query(ctx,
		`SELECT id::text, name, COALESCE(description,'') FROM skills
		 WHERE (workspace_id=$1::uuid OR workspace_id IS NULL) AND enabled=true ORDER BY name`,
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
	return map[string]any{"skills": out, "count": len(out)}, nil
}

// ── native_create_skill ───────────────────────────────────────────────────────

type CreateSkillTool struct{ pool *pgxpool.Pool }

func NewCreateSkillTool(pool *pgxpool.Pool) *CreateSkillTool { return &CreateSkillTool{pool} }

func (t *CreateSkillTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":           map[string]any{"type": "string", "description": "Skill name."},
			"description":    map[string]any{"type": "string", "description": "One-line description of what this skill does."},
			"content":        map[string]any{"type": "string", "description": "The skill instructions or knowledge to inject into the system prompt."},
			"attach_to_self": map[string]any{"type": "boolean", "description": "If true, attach this skill to the calling agent immediately (active next turn). Default false."},
			"ephemeral":      map[string]any{"type": "boolean", "description": "If true, skill is deleted when this run ends. Default false."},
		},
		"required": []string{"name", "content"},
	})
	return domain.Tool{
		Name:             "native_create_skill",
		Description:      "Create a new skill (reusable instruction block). Set attach_to_self=true to inject it into the calling agent's context immediately. Set ephemeral=true to auto-delete when this run ends.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}
func (t *CreateSkillTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_create_skill requires run context")
}
func (t *CreateSkillTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	name, _ := input["name"].(string)
	description, _ := input["description"].(string)
	content, _ := input["content"].(string)
	attachToSelf, _ := input["attach_to_self"].(bool)
	ephemeral, _ := input["ephemeral"].(bool)
	if name == "" || content == "" {
		return nil, fmt.Errorf("name and content are required")
	}

	skillID := uuid.NewString()
	_, err := t.pool.Exec(ctx, `
		INSERT INTO skills(id, workspace_id, name, description, content, source, enabled, created_by, source_run_id, ephemeral)
		VALUES($1::uuid,$2::uuid,$3,$4,$5,'manual',true,$6::uuid,
		  CASE WHEN $7='' THEN NULL ELSE $7::uuid END,$8)`,
		skillID, execCtx.WorkspaceID, name, description, content,
		execCtx.UserID, execCtx.RunID, ephemeral)
	if err != nil {
		return nil, fmt.Errorf("create skill: %w", err)
	}

	if attachToSelf && execCtx.AgentID != "" {
		t.pool.Exec(ctx, //nolint:errcheck
			`INSERT INTO agent_skills(agent_id,skill_id,enabled,order_index)
			 VALUES($1::uuid,$2::uuid,true,
			   COALESCE((SELECT MAX(order_index)+1 FROM agent_skills WHERE agent_id=$1::uuid),0))
			 ON CONFLICT DO NOTHING`,
			execCtx.AgentID, skillID)
	}

	return map[string]any{
		"skill_id":       skillID,
		"name":           name,
		"attach_to_self": attachToSelf,
		"ephemeral":      ephemeral,
		"note":           "Skill created. If attach_to_self=true, it will be active on the next run turn.",
	}, nil
}

// ── native_delete_skill ───────────────────────────────────────────────────────

type DeleteSkillTool struct{ pool *pgxpool.Pool }

func NewDeleteSkillTool(pool *pgxpool.Pool) *DeleteSkillTool { return &DeleteSkillTool{pool} }

func (t *DeleteSkillTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type":       "object",
		"properties": map[string]any{"skill_id": map[string]any{"type": "string", "description": "UUID of the skill to delete."}},
		"required":   []string{"skill_id"},
	})
	return domain.Tool{
		Name:             "native_delete_skill",
		Description:      "Delete a skill. Only skills created by the current run (via native_create_skill) can be deleted this way.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "high",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}
func (t *DeleteSkillTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_delete_skill requires run context")
}
func (t *DeleteSkillTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	skillID, _ := input["skill_id"].(string)
	if skillID == "" {
		return nil, fmt.Errorf("skill_id is required")
	}
	var srcRunID string
	err := t.pool.QueryRow(ctx,
		`SELECT COALESCE(source_run_id::text,'') FROM skills WHERE id=$1::uuid AND (workspace_id=$2::uuid OR workspace_id IS NULL)`,
		skillID, execCtx.WorkspaceID).Scan(&srcRunID)
	if err != nil {
		return nil, fmt.Errorf("skill not found")
	}
	if srcRunID != execCtx.RunID {
		return nil, fmt.Errorf("permission denied: can only delete skills created by the current run")
	}
	if _, err := t.pool.Exec(ctx, `DELETE FROM skills WHERE id=$1::uuid`, skillID); err != nil {
		return nil, err
	}
	return map[string]any{"deleted": true, "skill_id": skillID}, nil
}
