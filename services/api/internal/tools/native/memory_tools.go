package native

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
)

type ListMemoriesTool struct{ tools.RequiresRunContext }

func NewListMemoriesTool() *ListMemoriesTool { return &ListMemoriesTool{} }

func (t *ListMemoriesTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Optional search text.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"maximum":     10,
				"description": "Max results (default 5).",
			},
		},
	})
	return domain.Tool{
		Name:             "native_list_memories",
		Description:      "List saved memories when stable prior context may matter.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        2000,
		Enabled:          true,
	}
}

func (t *ListMemoriesTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	return searchMemories(ctx, execCtx, input, false)
}

type RequestMemoryTool struct{ tools.RequiresRunContext }

func NewRequestMemoryTool() *RequestMemoryTool { return &RequestMemoryTool{} }

func (t *RequestMemoryTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search text for matching memories.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"maximum":     10,
				"description": "Max results to inject (default 5).",
			},
		},
		"required": []string{"query"},
	})
	return domain.Tool{
		Name:             "native_request_memory",
		Description:      "Load selected memories into the current run.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        2000,
		Enabled:          true,
	}
}

func (t *RequestMemoryTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	out, err := searchMemories(ctx, execCtx, input, true)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func searchMemories(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any, inject bool) (any, error) {
	if execCtx.SearchMemory == nil {
		return nil, fmt.Errorf("memory search is not available in this execution context")
	}
	query, _ := input["query"].(string)
	limit := limitFromInput(input, 5, 10)

	memories, err := execCtx.SearchMemory(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	results := make([]map[string]any, 0, len(memories))
	for _, m := range memories {
		results = append(results, map[string]any{
			"id":               m.ID,
			"content":          trimMemoryContent(m.Content),
			"relevance_score":  m.RelevanceScore,
			"importance_score": m.ImportanceScore,
			"scope":            m.Scope,
			"created_at":       m.CreatedAt,
			"source_run_id":    m.SourceRunID,
			"save_source":      m.SaveSource,
			"conversation_id":  m.ConversationID,
			"status":           m.Status,
		})
	}

	if inject && len(memories) > 0 && execCtx.RequestMemory != nil {
		execCtx.RequestMemory(memories)
	}

	out := map[string]any{
		"query":    strings.TrimSpace(query),
		"count":    len(results),
		"memories": results,
	}
	if inject {
		out["activated"] = len(results) > 0
	}
	return out, nil
}

func limitFromInput(input map[string]any, def, max int) int {
	limit := def
	switch v := input["limit"].(type) {
	case float64:
		limit = int(v)
	case int:
		limit = v
	case int32:
		limit = int(v)
	case int64:
		limit = int(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			limit = int(n)
		}
	case string:
		if n := strings.TrimSpace(v); n != "" {
			if parsed, err := strconv.Atoi(n); err == nil {
				limit = parsed
			}
		}
	}
	if limit <= 0 {
		limit = def
	}
	if max > 0 && limit > max {
		limit = max
	}
	return limit
}

func trimMemoryContent(content string) string {
	content = strings.TrimSpace(content)
	if len(content) > 300 {
		return content[:300] + "…"
	}
	return content
}
