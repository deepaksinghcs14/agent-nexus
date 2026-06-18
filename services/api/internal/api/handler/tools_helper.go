package handler

import (
	"context"
	"encoding/json"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
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

func ensureMemoryToolDef(defs []provider.ToolDefinition, nameMap map[string]domain.Tool, enabled bool) ([]provider.ToolDefinition, map[string]domain.Tool) {
	if !enabled {
		return defs, nameMap
	}
	if nameMap == nil {
		nameMap = map[string]domain.Tool{}
	}
	if _, ok := nameMap["native_save_memory"]; ok {
		return defs, nameMap
	}
	schema := json.RawMessage(`{"type":"object","properties":{"content":{"type":"string","description":"A compact durable fact, preference, goal, or decision to remember. Do not include secrets or transient chat."},"importance_score":{"type":"number","description":"0 to 1 score for long-term usefulness."},"reason":{"type":"string","description":"Short reason this should be remembered."}},"required":["content","importance_score","reason"]}`)
	defs = append(defs, provider.ToolDefinition{
		Name:        "native_save_memory",
		Description: "Save a durable memory for future runs when the user reveals a stable preference, fact, goal, or reusable decision.",
		InputSchema: schema,
	})
	nameMap["native_save_memory"] = domain.Tool{
		Name:             "native_save_memory",
		Type:             "native",
		InputSchema:      schema,
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
	return defs, nameMap
}
