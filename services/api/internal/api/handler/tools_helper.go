package handler

import (
	"context"
	"encoding/json"

	"github.com/agentNexus/agent-nexus/services/api/internal/domain"
	"github.com/agentNexus/agent-nexus/services/api/internal/provider"
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
