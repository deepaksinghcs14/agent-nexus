package native

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	contextretrieval "github.com/deepaksingh/agent-nexus/services/api/internal/runtime/context"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RetrieveContextTool struct{ pool *pgxpool.Pool }

func NewRetrieveContextTool(pool *pgxpool.Pool) *RetrieveContextTool {
	return &RetrieveContextTool{pool: pool}
}

func (t *RetrieveContextTool) Definition() domain.Tool {
	return domain.Tool{
		Name:        "native_retrieve_context",
		Description: "Search connected knowledge sources (connectors) and return the most relevant text chunks. Use this when you need information from indexed documents, codebases, or wikis. Prefer a specific, targeted query over a vague one.",
		Type:        "native",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query":      {"type": "string",  "description": "Search query — be specific for best results"},
				"max_chunks": {"type": "integer", "description": "Max chunks to return (default 8, max 50)"},
				"min_score":  {"type": "number",  "description": "Minimum relevance score 0–1 (default 0.5)"}
			},
			"required": ["query"]
		}`),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        10000,
		Enabled:          true,
	}
}

func (t *RetrieveContextTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_retrieve_context requires run context")
}

func (t *RetrieveContextTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	query, _ := input["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	maxChunks := 8
	if v, ok := input["max_chunks"]; ok {
		switch n := v.(type) {
		case float64:
			maxChunks = int(n)
		case int:
			maxChunks = n
		}
	}
	if maxChunks <= 0 {
		maxChunks = 8
	}
	if maxChunks > 50 {
		maxChunks = 50
	}

	minScore := 0.5
	if v, ok := input["min_score"]; ok {
		if f, ok := v.(float64); ok {
			minScore = f
		}
	}

	if len(execCtx.ConnectorIDs) == 0 {
		return map[string]any{"results": []any{}, "note": "no connectors attached to this agent"}, nil
	}

	var embedding []float32
	if execCtx.Embed != nil {
		embedding, _ = execCtx.Embed(ctx, query)
	}
	chunks, err := contextretrieval.NewRetriever(t.pool).Retrieve(ctx, execCtx.WorkspaceID, execCtx.ConnectorIDs, embedding, maxChunks, minScore, query)
	if err != nil {
		return nil, fmt.Errorf("retrieval failed: %w", err)
	}

	// Format results for the model.
	var sb strings.Builder
	if len(chunks) == 0 {
		return map[string]any{"results": []any{}, "note": "no matching chunks found — try a broader query or lower min_score"}, nil
	}
	for i, c := range chunks {
		sb.WriteString(fmt.Sprintf("[%d] %s", i+1, c.Title))
		if c.URL != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", c.URL))
		}
		sb.WriteString(fmt.Sprintf(" — score %.2f\n%s\n\n", c.Score, c.Content))
	}
	return map[string]any{
		"query":   query,
		"count":   len(chunks),
		"results": sb.String(),
	}, nil
}
