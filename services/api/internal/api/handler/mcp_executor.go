package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/mcp"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

// executeMCPTool runs a tools-table entry of type 'mcp' by proxying the call
// to its MCP server. The tool's config carries {"server_id": ...} (written by
// the sync handler); the server row supplies URL, transport, and auth config.
func executeMCPTool(ctx context.Context, pool *pgxpool.Pool, dbTool domain.Tool, input json.RawMessage) *tools.ExecutionResult {
	start := time.Now()
	fail := func(msg string) *tools.ExecutionResult {
		return &tools.ExecutionResult{Error: msg, LatencyMs: int(time.Since(start).Milliseconds())}
	}

	var cfg struct {
		ServerID string `json:"server_id"`
	}
	if err := json.Unmarshal(dbTool.Config, &cfg); err != nil || cfg.ServerID == "" {
		return fail("mcp tool is missing server_id in config")
	}

	var url, transport string
	var serverCfg []byte
	if err := pool.QueryRow(ctx,
		`SELECT url, transport, COALESCE(config,'{}'::jsonb) FROM mcp_servers WHERE id=$1::uuid`,
		cfg.ServerID).Scan(&url, &transport, &serverCfg); err != nil {
		return fail("mcp server not found for tool " + dbTool.Name)
	}

	timeout := time.Duration(dbTool.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := mcp.NewClient(cfg.ServerID, url, transport, serverCfg)
	out, err := client.CallTool(callCtx, dbTool.Name, input)
	if err != nil {
		return fail(err.Error())
	}
	return &tools.ExecutionResult{Output: out, LatencyMs: int(time.Since(start).Milliseconds())}
}
