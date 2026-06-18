package native

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
)

// RequestToolTool activates a specific tool for the current run (lazy loading mode).
// After calling this, the named tool's full schema is injected on the next LLM turn.
type RequestToolTool struct{}

func NewRequestToolTool() *RequestToolTool { return &RequestToolTool{} }

func (t *RequestToolTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The exact tool name to activate (as returned by native_list_tools).",
			},
		},
		"required": []string{"name"},
	})
	return domain.Tool{
		Name:             "native_request_tool",
		Description:      "Activate a tool by name so its full schema is available on the next turn. Call native_list_tools first to see available tool names.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        500,
		Enabled:          true,
	}
}

func (t *RequestToolTool) Execute(input map[string]any) (any, error) {
	return nil, fmt.Errorf("native_request_tool requires run context")
}

func (t *RequestToolTool) ExecuteWithContext(_ context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	name, _ := input["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("native_request_tool: name is required")
	}
	if _, ok := execCtx.ToolSummaries[name]; !ok {
		return map[string]any{
			"activated": false,
			"error":     fmt.Sprintf("tool %q not found — call native_list_tools to see available names", name),
		}, nil
	}
	if execCtx.RequestTool != nil {
		execCtx.RequestTool(name)
	}
	return map[string]any{
		"activated": true,
		"message":   fmt.Sprintf("Tool %q is now active. You can call it on the next turn.", name),
	}, nil
}
