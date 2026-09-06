package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools/native"
	"github.com/jackc/pgx/v5/pgxpool"
)

// invokeToolByType dispatches a tool call to its transport — http, mcp, code,
// or (default) the native registry executor. This is the single home for the
// tool-type switch that every run loop and workflow node shares; approval
// gating, SSE emission, and step persistence stay with the callers.
//
// dbTool/toolExists come from resolveDBTool: when the tool has no DB row
// (toolExists=false) the call falls through to the native registry, matching
// the historical behaviour of each inlined switch.
func invokeToolByType(
	ctx context.Context,
	pool *pgxpool.Pool,
	cfg *config.Config,
	executor *tools.Executor,
	execCtx tools.ExecutionContext,
	dbTool domain.Tool,
	toolExists bool,
	name string,
	input json.RawMessage,
) (*tools.ExecutionResult, error) {
	switch {
	case toolExists && dbTool.Type == "http":
		var httpCfg tools.HTTPToolConfig
		_ = json.Unmarshal(dbTool.Config, &httpCfg)
		client := native.SafeHTTPClient(cfg.HTTPToolAllowHosts, time.Duration(dbTool.TimeoutMs)*time.Millisecond)
		return tools.ExecuteHTTP(ctx, client, httpCfg, input), nil

	case toolExists && dbTool.Type == "mcp":
		res, _, _ := executeMCPTool(ctx, pool, cfg, dbTool, input)
		return res, nil

	case toolExists && dbTool.Type == "code":
		var codeCfg struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(dbTool.Config, &codeCfg)
		start := time.Now()
		out, codeErr := native.ExecuteCodeTool(ctx, codeCfg.Code, input)
		result := &tools.ExecutionResult{LatencyMs: int(time.Since(start).Milliseconds())}
		if codeErr != nil {
			result.Error = codeErr.Error()
		} else {
			result.Output = out
		}
		return result, nil

	default:
		return executor.ExecuteWithContext(ctx, execCtx, name, input)
	}
}
