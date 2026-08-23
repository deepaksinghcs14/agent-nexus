package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

// executeMCPTool runs a tools-table entry of type 'mcp' by proxying the call
// to its MCP server. The tool's config carries {"server_id": ...} (written by
// the sync handler); the server row supplies URL, transport, and auth — for
// oauth servers a fresh access token is resolved (refreshing if expired).
// Also returns the raw JSON-RPC request/response exchanged (nil unless the
// call reached the CallTool step), for the tool-test UI to show what was
// actually sent and received when debugging a broken integration.
func executeMCPTool(ctx context.Context, pool *pgxpool.Pool, appCfg *config.Config, dbTool domain.Tool, input json.RawMessage) (result *tools.ExecutionResult, reqJSON, respJSON []byte) {
	start := time.Now()
	fail := func(msg string) *tools.ExecutionResult {
		return &tools.ExecutionResult{Error: msg, LatencyMs: int(time.Since(start).Milliseconds())}
	}

	var cfg struct {
		ServerID string `json:"server_id"`
	}
	if err := json.Unmarshal(dbTool.Config, &cfg); err != nil || cfg.ServerID == "" {
		return fail("mcp tool is missing server_id in config"), nil, nil
	}

	var url, transport, authType string
	var serverCfg []byte
	if err := pool.QueryRow(ctx,
		`SELECT url, transport, COALESCE(auth_type,'config'), COALESCE(config,'{}'::jsonb) FROM mcp_servers WHERE id=$1::uuid`,
		cfg.ServerID).Scan(&url, &transport, &authType, &serverCfg); err != nil {
		return fail("mcp server not found for tool " + dbTool.Name), nil, nil
	}

	timeout := time.Duration(dbTool.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := mcpClientForServer(callCtx, pool, appCfg, cfg.ServerID, url, transport, authType, serverCfg)
	if err != nil {
		return fail(err.Error()), nil, nil
	}
	out, reqJSON, respJSON, err := client.CallTool(callCtx, dbTool.Name, input)
	if err != nil {
		return fail(err.Error()), reqJSON, respJSON
	}
	return &tools.ExecutionResult{Output: out, LatencyMs: int(time.Since(start).Milliseconds())}, reqJSON, respJSON
}
