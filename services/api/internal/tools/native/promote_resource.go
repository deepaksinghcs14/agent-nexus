package native

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PromoteResourceTool flips ephemeral=false on an agent, skill, tool, or workflow
// created during this run, so it survives when cleanupEphemeralResources runs at run end.
type PromoteResourceTool struct{ pool *pgxpool.Pool }

func NewPromoteResourceTool(pool *pgxpool.Pool) *PromoteResourceTool {
	return &PromoteResourceTool{pool}
}

func (t *PromoteResourceTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type": map[string]any{
				"type":        "string",
				"enum":        []string{"agent", "skill", "tool", "workflow"},
				"description": "Resource type to promote.",
			},
			"id": map[string]any{
				"type":        "string",
				"description": "UUID of the resource to keep.",
			},
		},
		"required": []string{"type", "id"},
	})
	return domain.Tool{
		Name:             "native_promote_resource",
		Description:      "Mark an ephemeral agent, skill, tool, or workflow as permanent so it survives when this run ends. Call this after creation once you know the resource is worth keeping. Has no effect on resources already set to ephemeral=false.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}

func (t *PromoteResourceTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_promote_resource requires run context")
}

func (t *PromoteResourceTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	resType, _ := input["type"].(string)
	id, _ := input["id"].(string)
	if resType == "" || id == "" {
		return nil, fmt.Errorf("native_promote_resource: type and id are required")
	}

	tableMap := map[string]string{
		"agent":    "agents",
		"skill":    "skills",
		"tool":     "tools",
		"workflow": "workflows",
	}
	table, ok := tableMap[resType]
	if !ok {
		return map[string]any{
			"promoted": false,
			"error":    fmt.Sprintf("unknown type %q — must be one of: agent, skill, tool, workflow", resType),
		}, nil
	}

	tag, err := t.pool.Exec(ctx,
		fmt.Sprintf(`UPDATE %s SET ephemeral=false WHERE id=$1::uuid AND workspace_id=$2::uuid`, table),
		id, execCtx.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("native_promote_resource: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return map[string]any{
			"promoted": false,
			"error":    fmt.Sprintf("%s %q not found in this workspace", resType, id),
		}, nil
	}
	return map[string]any{
		"promoted": true,
		"type":     resType,
		"id":       id,
		"message":  fmt.Sprintf("%s %q will be kept after this run ends", resType, id),
	}, nil
}
