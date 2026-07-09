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
type RequestToolTool struct{ tools.RequiresRunContext }

func NewRequestToolTool() *RequestToolTool { return &RequestToolTool{} }

func (t *RequestToolTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Exact tool name.",
			},
		},
		"required": []string{"name"},
	})
	return domain.Tool{
		Name:             "native_request_tool",
		Description:      "Request a tool so its full schema is available on the next turn.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        500,
		Enabled:          true,
	}
}

func (t *RequestToolTool) ExecuteWithContext(_ context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	name, _ := input["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("native_request_tool: name is required")
	}
	if _, ok := execCtx.ToolSummaries[name]; !ok {
		// Check if an on-demand skill exposes this tool and auto-activate it.
		if skillName, found := execCtx.SkillToolMap[name]; found && execCtx.RequestSkill != nil {
			activated := execCtx.RequestSkill(skillName)
			if activated {
				return map[string]any{
					"activated": true,
					"via_skill": skillName,
					"message":   fmt.Sprintf("Tool %q is provided by skill %q — skill activated. The tool (and any other tools from that skill) will be available on the next turn.", name, skillName),
				}, nil
			}
		}
		hint := fmt.Sprintf("tool %q not found", name)
		if len(execCtx.SkillSummaries) > 0 {
			hint += " — it may be exposed by an on-demand skill. Call native_list_agent_skills to see available skills, then native_request_skill(skill_name) to activate one."
		} else {
			hint += " — call native_list_tools to see available names."
		}
		return map[string]any{
			"activated": false,
			"error":     hint,
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
