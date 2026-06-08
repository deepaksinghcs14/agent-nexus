package native

import (
	"encoding/json"
	"fmt"

	"github.com/agentNexus/agent-nexus/services/api/internal/domain"
)

type WebSearchTool struct{}

func NewWebSearchTool() *WebSearchTool { return &WebSearchTool{} }

func (t *WebSearchTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Search query"},
			"limit": map[string]any{"type": "integer", "description": "Max results (default 5)"},
		},
		"required": []string{"query"},
	})
	return domain.Tool{
		Name:             "web_search",
		Description:      "Search the web and return a list of results with titles, URLs, and snippets",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        15000,
		Enabled:          true,
	}
}

func (t *WebSearchTool) Execute(input map[string]any) (any, error) {
	return nil, fmt.Errorf("web_search: not implemented")
}
