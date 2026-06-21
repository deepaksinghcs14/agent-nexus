package handler

import (
	"context"
	"encoding/json"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

// loadAgentToolDefs returns the tool definitions to pass to the LLM and a name→Tool map
// for risk/approval checks. Only tools enabled for the agent are returned.
func loadAgentToolDefs(ctx context.Context, pool *pgxpool.Pool, agentID string) ([]provider.ToolDefinition, map[string]domain.Tool, error) {
	rows, err := pool.Query(ctx,
		`SELECT t.name, t.description, t.type, t.input_schema, t.config, t.risk_level, t.requires_approval, t.timeout_ms
		 FROM agent_tools at
		 JOIN tools t ON t.id = at.tool_id
		 WHERE at.agent_id=$1::uuid AND COALESCE(at.enabled, t.enabled) = true
		 ORDER BY t.name`,
		agentID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var defs []provider.ToolDefinition
	nameMap := map[string]domain.Tool{}

	for rows.Next() {
		var t domain.Tool
		var inputSchema, cfg []byte
		if err := rows.Scan(&t.Name, &t.Description, &t.Type, &inputSchema, &cfg, &t.RiskLevel, &t.RequiresApproval, &t.TimeoutMs); err != nil {
			continue
		}
		t.InputSchema = json.RawMessage(inputSchema)
		if len(cfg) > 0 {
			t.Config = json.RawMessage(cfg)
		}
		defs = append(defs, provider.ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
		nameMap[t.Name] = t
	}
	return defs, nameMap, rows.Err()
}

// lazyMetaToolDefs returns the meta-tools always included in lazy-loading mode.
func lazyMetaToolDefs(reg *tools.Registry) []provider.ToolDefinition {
	names := []string{
		"native_list_tools",
		"native_request_tool",
		"native_list_agent_skills",
		"native_request_skill",
		"native_list_memories",
		"native_request_memory",
		"native_save_memory",
	}
	var defs []provider.ToolDefinition
	for _, name := range names {
		t, err := reg.Get(name)
		if err != nil {
			continue
		}
		d := t.Definition()
		defs = append(defs, provider.ToolDefinition{
			Name:        d.Name,
			Description: d.Description,
			InputSchema: d.InputSchema,
		})
	}
	return defs
}

func ensureMemoryToolDefs(defs []provider.ToolDefinition, nameMap map[string]domain.Tool) ([]provider.ToolDefinition, map[string]domain.Tool) {
	if nameMap == nil {
		nameMap = map[string]domain.Tool{}
	}
	add := func(name, description string, schema json.RawMessage, risk string, timeoutMs int) {
		if _, ok := nameMap[name]; ok {
			return
		}
		defs = append(defs, provider.ToolDefinition{
			Name:        name,
			Description: description,
			InputSchema: schema,
		})
		nameMap[name] = domain.Tool{
			Name:             name,
			Type:             "native",
			InputSchema:      schema,
			RiskLevel:        risk,
			RequiresApproval: false,
			TimeoutMs:        timeoutMs,
			Enabled:          true,
		}
	}

	memorySearchSchema := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Optional search text. Leave empty to list the most recent relevant memories."},"limit":{"type":"integer","minimum":1,"maximum":10,"description":"Maximum number of memories to return. Default 5."}}}`)
	memoryRequestSchema := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search text describing the memories you want to load into the current run context."},"limit":{"type":"integer","minimum":1,"maximum":10,"description":"Maximum number of memories to inject. Default 5."}},"required":["query"]}`)
	memorySaveSchema := json.RawMessage(`{"type":"object","properties":{"content":{"type":"string","description":"A compact durable fact, preference, goal, or decision to remember. Do not include secrets or transient chat."},"importance_score":{"type":"number","description":"0 to 1 score for long-term usefulness."},"reason":{"type":"string","description":"Short reason this should be remembered."}},"required":["content","importance_score","reason"]}`)

	add("native_list_memories", "List relevant memories on demand. Use this when earlier preferences, facts, or decisions matter.", memorySearchSchema, "low", 2000)
	add("native_request_memory", "Search memories and inject the matching results into the current run context for subsequent turns.", memoryRequestSchema, "low", 2000)
	add("native_save_memory", "Save a durable memory for future runs when the user reveals a stable preference, fact, goal, or reusable decision.", memorySaveSchema, "low", 5000)
	return defs, nameMap
}

// dedupeToolDefs removes repeated tool names while preserving the first
// declaration order. Gemini rejects duplicate function declarations outright,
// and the same rule keeps the other providers' tool lists clean.
func dedupeToolDefs(defs []provider.ToolDefinition) []provider.ToolDefinition {
	if len(defs) < 2 {
		return defs
	}
	seen := make(map[string]struct{}, len(defs))
	out := make([]provider.ToolDefinition, 0, len(defs))
	for _, def := range defs {
		if def.Name == "" {
			continue
		}
		if _, ok := seen[def.Name]; ok {
			continue
		}
		seen[def.Name] = struct{}{}
		out = append(out, def)
	}
	return out
}
