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

// ListToolsTool returns a filtered summary of requestable tools for the current agent.
// Used by agents with LazyToolLoading enabled so they can discover tools before requesting them.
type ListToolsTool struct{}

func NewListToolsTool() *ListToolsTool { return &ListToolsTool{} }

func (t *ListToolsTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Keywords to filter tools by name or description. Each word matched independently (OR). Leave empty to list all.",
			},
		},
	})
	return domain.Tool{
		Name:             "native_list_tools",
		Description:      "List available tools. Pass keyword(s) to filter — each word matched independently against name and description. Leave empty to list all.",
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

func (t *ListToolsTool) ExecuteWithContext(_ context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	if len(execCtx.ToolSummaries) == 0 {
		return map[string]any{"tools": []any{}, "hint": "No tools are currently available."}, nil
	}

	query, _ := input["query"].(string)
	words := strings.Fields(strings.ToLower(strings.TrimSpace(query)))

	var matched []string
	total := 0
	for name := range execCtx.ToolSummaries {
		if execCtx.AlwaysActiveTools[name] {
			continue
		}
		total++
		if len(words) == 0 {
			matched = append(matched, name)
		} else {
			haystack := strings.ToLower(name + " " + execCtx.ToolSummaries[name])
			for _, word := range words {
				if strings.Contains(haystack, word) {
					matched = append(matched, name)
					break
				}
			}
		}
	}

	if len(matched) == 0 {
		if len(words) > 0 {
			return map[string]any{
				"summary": fmt.Sprintf("No tools matched %q (checked %d tools). Call native_list_tools() with no query to see all.", query, total),
			}, nil
		}
		return map[string]any{"tools": []any{}, "hint": "No requestable tools are currently available."}, nil
	}

	sort.Strings(matched)

	var sb strings.Builder
	if len(words) > 0 {
		sb.WriteString(fmt.Sprintf("tools[%d of %d matched %q]{name,description}:\n", len(matched), total, query))
	} else {
		sb.WriteString(fmt.Sprintf("tools[%d]{name,description}:\n", len(matched)))
	}
	for _, name := range matched {
		desc := execCtx.ToolSummaries[name]
		if len(desc) > 120 {
			desc = desc[:120] + "…"
		}
		sb.WriteString(fmt.Sprintf("  %s,%s\n", name, desc))
	}
	if len(words) > 0 {
		sb.WriteString(fmt.Sprintf("\nNot finding it? Call native_list_tools() with no query to see all %d tools.", total))
	} else {
		sb.WriteString("\nCall native_request_tool(name) to activate a tool, then use it on the next turn.")
	}
	if len(execCtx.SkillSummaries) > 0 {
		sb.WriteString("\nAdditional tools are available through on-demand skills — call native_list_agent_skills to see them.")
	}

	return map[string]any{"summary": sb.String()}, nil
}
