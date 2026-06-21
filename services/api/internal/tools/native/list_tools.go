package native

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
)

// ListToolsTool returns a compact summary of tools available to the current agent.
// Used by agents with LazyToolLoading enabled so they can discover tools before requesting them.
type ListToolsTool struct{}

func NewListToolsTool() *ListToolsTool { return &ListToolsTool{} }

func (t *ListToolsTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	})
	return domain.Tool{
		Name:             "native_list_tools",
		Description:      "List available tools when you need the exact name of a tool.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        2000,
		Enabled:          true,
	}
}

func (t *ListToolsTool) Execute(input map[string]any) (any, error) {
	return nil, fmt.Errorf("native_list_tools requires run context")
}

func (t *ListToolsTool) ExecuteWithContext(_ context.Context, execCtx tools.ExecutionContext, _ map[string]any) (any, error) {
	if len(execCtx.ToolSummaries) == 0 {
		return map[string]any{"tools": []any{}, "hint": "No tools are currently available."}, nil
	}

	names := make([]string, 0, len(execCtx.ToolSummaries))
	for name := range execCtx.ToolSummaries {
		names = append(names, name)
	}
	sort.Strings(names)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("tools[%d]{name,description}:\n", len(names)))
	for _, name := range names {
		desc := execCtx.ToolSummaries[name]
		if len(desc) > 120 {
			desc = desc[:120] + "…"
		}
		sb.WriteString(fmt.Sprintf("  %s,%s\n", name, desc))
	}
	sb.WriteString("\nCall native_request_tool(name) to activate a tool before using it.")

	return map[string]any{"summary": sb.String()}, nil
}
