package native

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/internal/runtime/memory"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SaveMemoryTool struct {
	tools.RequiresRunContext
	pool *pgxpool.Pool
}

func NewSaveMemoryTool(pool *pgxpool.Pool) *SaveMemoryTool {
	return &SaveMemoryTool{pool: pool}
}

func (t *SaveMemoryTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "Compact fact, preference, goal, or decision.",
			},
			"importance_score": map[string]any{
				"type":        "number",
				"minimum":     0,
				"maximum":     1,
				"description": "0.0–1.0 usefulness score.",
			},
			"reason": map[string]any{
				"type":        "string",
				"description": "Short reason.",
			},
		},
		"required": []string{"content", "importance_score", "reason"},
	})
	return domain.Tool{
		Name:             "native_save_memory",
		Description:      "Save a durable memory for future runs.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}

const compressThreshold = 200

func (t *SaveMemoryTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	if execCtx.AgentID == "" || execCtx.WorkspaceID == "" {
		return nil, fmt.Errorf("native_save_memory: missing agent context")
	}
	agent, err := repository.NewAgentRepository(t.pool).Get(ctx, execCtx.AgentID, execCtx.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("native_save_memory: load agent: %w", err)
	}
	content, _ := input["content"].(string)
	reason, _ := input["reason"].(string)
	importance, _ := input["importance_score"].(float64)

	// Compress verbose content before storing so all future retrievals are compact.
	if execCtx.CompressText != nil && len(content) > compressThreshold {
		if compressed, cerr := execCtx.CompressText(ctx, content); cerr == nil && compressed != "" {
			content = compressed
		}
	}
	result, err := memory.NewEngine(t.pool).SaveCandidate(ctx, memory.SaveRequest{
		Agent:          agent,
		WorkspaceID:    execCtx.WorkspaceID,
		UserID:         execCtx.UserID,
		ConversationID: execCtx.ConversationID,
		RunID:          execCtx.RunID,
		Source:         "tool",
		Candidate: memory.Candidate{
			Content:         content,
			ImportanceScore: importance,
			Reason:          reason,
		},
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
