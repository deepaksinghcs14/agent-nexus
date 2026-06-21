package native

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
)

type ListAgentSkillsTool struct{}

func NewListAgentSkillsTool() *ListAgentSkillsTool { return &ListAgentSkillsTool{} }

func (t *ListAgentSkillsTool) Definition() domain.Tool {
	return domain.Tool{Name: "native_list_agent_skills", Description: "List on-demand skills when you need the exact skill name.", Type: "native", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`), OutputSchema: json.RawMessage(`{"type":"object"}`), RiskLevel: "low", TimeoutMs: 500, Enabled: true}
}
func (t *ListAgentSkillsTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_list_agent_skills requires run context")
}
func (t *ListAgentSkillsTool) ExecuteWithContext(_ context.Context, execCtx tools.ExecutionContext, _ map[string]any) (any, error) {
	return map[string]any{"skills": execCtx.SkillSummaries}, nil
}

type RequestSkillTool struct{}

func NewRequestSkillTool() *RequestSkillTool { return &RequestSkillTool{} }

func (t *RequestSkillTool) Definition() domain.Tool {
	schema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Exact skill name returned by native_list_agent_skills."}},"required":["name"]}`)
	return domain.Tool{Name: "native_request_skill", Description: "Request a skill so its instructions are available on the next turn.", Type: "native", InputSchema: schema, OutputSchema: json.RawMessage(`{"type":"object"}`), RiskLevel: "low", TimeoutMs: 500, Enabled: true}
}
func (t *RequestSkillTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_request_skill requires run context")
}
func (t *RequestSkillTool) ExecuteWithContext(_ context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	name, _ := input["name"].(string)
	if name == "" || execCtx.RequestSkill == nil {
		return map[string]any{"activated": false, "error": "skill not found — call native_list_agent_skills"}, nil
	}
	if !execCtx.RequestSkill(name) {
		return map[string]any{"activated": false, "error": "skill not found or already active"}, nil
	}
	return map[string]any{"activated": true, "message": fmt.Sprintf("Skill %q is active for this run.", name)}, nil
}
