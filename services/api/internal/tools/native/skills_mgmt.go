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
			"ephemeral":      map[string]any{"type": "boolean", "description": "Default true (auto-deletes at run end). Set false at creation time, or call native_promote_resource after creation once you know it's worth keeping."},
		},
		"required": []string{"name", "content"},
	})
	return domain.Tool{
		Name:             "native_create_skill",
		Description:      "Create a new skill instruction block. Skills are temporary by default and auto-delete when this run ends unless ephemeral=false is explicitly provided. Set attach_to_self=true to inject it into the calling agent's context immediately.",
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
	if content == "" {
		content, _ = input["instructions"].(string) // accept instructions as alias for content
	}
	attachToSelf, _ := input["attach_to_self"].(bool)
	ephemeral := ephemeralFromInput(input)
	if name == "" || content == "" {
		return nil, fmt.Errorf("name and content are required (use field 'content', not 'instructions')")
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

// ── native_attach_skill ───────────────────────────────────────────────────────

type AttachSkillTool struct{ pool *pgxpool.Pool }

func NewAttachSkillTool(pool *pgxpool.Pool) *AttachSkillTool { return &AttachSkillTool{pool} }

func (t *AttachSkillTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id":   map[string]any{"type": "string", "description": "UUID of the agent to attach the skill to."},
			"skill_name": map[string]any{"type": "string", "description": "Name of the skill to attach."},
		},
		"required": []string{"agent_id", "skill_name"},
	})
	return domain.Tool{
		Name:             "native_attach_skill",
		Description:      "Attach a skill to any agent by skill name.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}
func (t *AttachSkillTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_attach_skill requires run context")
}
func (t *AttachSkillTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	agentID, _ := input["agent_id"].(string)
	skillName, _ := input["skill_name"].(string)
	if agentID == "" {
		if agentName, _ := input["agent_name"].(string); agentName != "" {
			t.pool.QueryRow(ctx, `SELECT id::text FROM agents WHERE name=$1 AND workspace_id=$2::uuid LIMIT 1`, agentName, execCtx.WorkspaceID).Scan(&agentID) //nolint:errcheck
		}
	}
	if agentID == "" || skillName == "" {
		return nil, fmt.Errorf("agent_id is required — pass the UUID returned by native_create_agent")
	}

	var skillID string
	err := t.pool.QueryRow(ctx,
		`SELECT id::text FROM skills
		 WHERE name=$1 AND (workspace_id=$2::uuid OR workspace_id IS NULL) AND enabled=true
		 ORDER BY workspace_id NULLS LAST LIMIT 1`,
		skillName, execCtx.WorkspaceID).Scan(&skillID)
	if err != nil {
		return nil, fmt.Errorf("skill not found: %s", skillName)
	}

	_, err = t.pool.Exec(ctx,
		`INSERT INTO agent_skills(agent_id, skill_id, enabled, order_index)
		 VALUES($1::uuid, $2::uuid, true,
		   COALESCE((SELECT MAX(order_index)+1 FROM agent_skills WHERE agent_id=$1::uuid), 0))
		 ON CONFLICT DO NOTHING`,
		agentID, skillID)
	if err != nil {
		return nil, fmt.Errorf("attach skill: %w", err)
	}
	attachRequiredToolsForSkill(ctx, t.pool, agentID, skillID) //nolint:errcheck

	return map[string]any{
		"attached":   true,
		"agent_id":   agentID,
		"skill_id":   skillID,
		"skill_name": skillName,
	}, nil
}

func attachRequiredToolsForSkill(ctx context.Context, pool *pgxpool.Pool, agentID, skillID string) error {
	rows, err := pool.Query(ctx,
		`SELECT unnest(required_tool_names) FROM skills WHERE id=$1::uuid AND required_tool_names <> '{}'`,
		skillID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var toolName string
		if rows.Scan(&toolName) == nil && toolName != "" {
			pool.Exec(ctx, //nolint:errcheck
				`INSERT INTO agent_tools(agent_id, tool_id, enabled)
				 SELECT $1::uuid, id, true FROM tools WHERE name=$2
				 ON CONFLICT DO NOTHING`,
				agentID, toolName)
		}
	}
	return rows.Err()
}

// ── native_detach_skill ───────────────────────────────────────────────────────

type DetachSkillTool struct{ pool *pgxpool.Pool }

func NewDetachSkillTool(pool *pgxpool.Pool) *DetachSkillTool { return &DetachSkillTool{pool} }

func (t *DetachSkillTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id": map[string]any{"type": "string", "description": "UUID of the agent to detach the skill from."},
			"skill_id": map[string]any{"type": "string", "description": "UUID of the skill to detach."},
		},
		"required": []string{"agent_id", "skill_id"},
	})
	return domain.Tool{
		Name:             "native_detach_skill",
		Description:      "Detach a skill from any agent.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}
func (t *DetachSkillTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_detach_skill requires run context")
}
func (t *DetachSkillTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	agentID, _ := input["agent_id"].(string)
	skillID, _ := input["skill_id"].(string)
	if agentID == "" || skillID == "" {
		return nil, fmt.Errorf("agent_id and skill_id are required")
	}

	_, err := t.pool.Exec(ctx,
		`DELETE FROM agent_skills WHERE agent_id=$1::uuid AND skill_id=$2::uuid`,
		agentID, skillID)
	if err != nil {
		return nil, fmt.Errorf("detach skill: %w", err)
	}

	return map[string]any{
		"detached": true,
		"agent_id": agentID,
		"skill_id": skillID,
	}, nil
}

// ── native_update_skill ───────────────────────────────────────────────────────

type UpdateSkillTool struct{ pool *pgxpool.Pool }

func NewUpdateSkillTool(pool *pgxpool.Pool) *UpdateSkillTool { return &UpdateSkillTool{pool} }

func (t *UpdateSkillTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"skill_id":    map[string]any{"type": "string", "description": "UUID of the skill to update."},
			"name":        map[string]any{"type": "string", "description": "New skill name."},
			"description": map[string]any{"type": "string", "description": "New skill description."},
			"content":     map[string]any{"type": "string", "description": "New skill content/instructions."},
		},
		"required": []string{"skill_id"},
	})
	return domain.Tool{
		Name:             "native_update_skill",
		Description:      "Update a skill's name, description, or content. Cannot update managed (built-in) skills.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}
func (t *UpdateSkillTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_update_skill requires run context")
}
func (t *UpdateSkillTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	skillID, _ := input["skill_id"].(string)
	if skillID == "" {
		return nil, fmt.Errorf("skill_id is required")
	}

	var source string
	err := t.pool.QueryRow(ctx,
		`SELECT COALESCE(source,'') FROM skills
		 WHERE id=$1::uuid AND (workspace_id=$2::uuid OR workspace_id IS NULL)`,
		skillID, execCtx.WorkspaceID).Scan(&source)
	if err != nil {
		return nil, fmt.Errorf("skill not found")
	}
	if source == "managed" {
		return nil, fmt.Errorf("cannot update managed skills")
	}

	setClauses := []string{"updated_at=NOW()"}
	args := []any{}
	argIdx := 1

	if name, ok := input["name"].(string); ok && name != "" {
		argIdx++
		setClauses = append(setClauses, fmt.Sprintf("name=$%d", argIdx))
		args = append(args, name)
	}
	if description, ok := input["description"].(string); ok && description != "" {
		argIdx++
		setClauses = append(setClauses, fmt.Sprintf("description=$%d", argIdx))
		args = append(args, description)
	}
	if content, ok := input["content"].(string); ok && content != "" {
		argIdx++
		setClauses = append(setClauses, fmt.Sprintf("content=$%d", argIdx))
		args = append(args, content)
	}

	argIdx++
	args = append(args, skillID)

	query := fmt.Sprintf("UPDATE skills SET %s WHERE id=$%d::uuid",
		joinStrings(setClauses, ", "), argIdx)

	if _, err := t.pool.Exec(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("update skill: %w", err)
	}

	return map[string]any{
		"updated":  true,
		"skill_id": skillID,
	}, nil
}

// joinStrings joins a slice of strings with a separator.
func joinStrings(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}
