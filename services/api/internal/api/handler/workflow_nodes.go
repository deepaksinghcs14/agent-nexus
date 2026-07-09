package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	gatewayservice "github.com/deepaksingh/agent-nexus/services/api/internal/gateway"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools/native"
)

// Executors for the non-agent workflow nodes (tool / webhook / gateway) and
// the end node's delivery config. These run inline in the workflow walk —
// they are side-effectful integration steps, not LLM calls, so they have no
// sub-run of their own; their result is recorded as a step on the parent run.

// renderWorkflowTemplate substitutes the two placeholders workflow node
// templates support: {{input}} (the previous node's output) and
// {{original_input}} (the input the workflow run started with).
func renderWorkflowTemplate(tpl, input, originalInput string) string {
	out := strings.ReplaceAll(tpl, "{{input}}", input)
	return strings.ReplaceAll(out, "{{original_input}}", originalInput)
}

// jsonEscapeForTemplate renders a value safe to substitute inside a JSON
// string template: the surrounding quotes from json.Marshal are stripped.
func jsonEscapeForTemplate(s string) string {
	b, _ := json.Marshal(s)
	return strings.Trim(string(b), `"`)
}

// executeWorkflowToolNode runs a single workspace tool (http / mcp / code /
// native) as a workflow step. Config:
//
//	tool_name (required) — name of an enabled tool visible in the workspace
//	args (optional)      — JSON object passed as the tool input; string values
//	                       may contain {{input}} / {{original_input}}
//
// Without args the tool receives {"input": "<previous output>"}.
// Approval-gated tools are refused: a workflow run has no approval loop, so
// executing one here would silently bypass the gate an agent run enforces.
func (h *InvokeHandler) executeWorkflowToolNode(ctx context.Context, ws, uid, parentRunID string, node *wfNode, input, originalInput string) (string, error) {
	toolName, _ := node.Config["tool_name"].(string)
	if toolName == "" {
		return "", fmt.Errorf("tool node has no tool_name configured")
	}

	var t domain.Tool
	var inputSchema, cfg []byte
	err := h.pool.QueryRow(ctx,
		`SELECT name, description, type, input_schema, config, risk_level, requires_approval, timeout_ms
		 FROM tools WHERE name=$1 AND (workspace_id IS NULL OR workspace_id=$2::uuid) AND enabled=true
		 ORDER BY workspace_id NULLS LAST LIMIT 1`,
		toolName, ws).Scan(&t.Name, &t.Description, &t.Type, &inputSchema, &cfg, &t.RiskLevel, &t.RequiresApproval, &t.TimeoutMs)
	if err != nil {
		return "", fmt.Errorf("tool %q not found in workspace", toolName)
	}
	t.InputSchema = json.RawMessage(inputSchema)
	if len(cfg) > 0 {
		t.Config = json.RawMessage(cfg)
	}
	if t.RequiresApproval {
		return "", fmt.Errorf("tool %q requires approval and cannot run unattended in a workflow tool node — clear its approval flag or call it from an agent node instead", toolName)
	}

	// Build the tool input: configured args (with template substitution in
	// string values) or the default {"input": ...} envelope.
	toolInput := json.RawMessage(fmt.Sprintf(`{"input":%q}`, input))
	if rawArgs, ok := node.Config["args"].(map[string]any); ok && len(rawArgs) > 0 {
		rendered := make(map[string]any, len(rawArgs))
		for k, v := range rawArgs {
			if s, isStr := v.(string); isStr {
				rendered[k] = renderWorkflowTemplate(s, input, originalInput)
			} else {
				rendered[k] = v
			}
		}
		if b, mErr := json.Marshal(rendered); mErr == nil {
			toolInput = b
		}
	}

	start := time.Now()
	var result *tools.ExecutionResult
	switch t.Type {
	case "http":
		var httpCfg tools.HTTPToolConfig
		_ = json.Unmarshal(t.Config, &httpCfg)
		result = tools.ExecuteHTTP(ctx, httpCfg, toolInput, t.TimeoutMs)
	case "mcp":
		result = executeMCPTool(ctx, h.pool, h.cfg, t, toolInput)
	case "code":
		var codeCfg struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(t.Config, &codeCfg)
		out, codeErr := native.ExecuteCodeTool(ctx, codeCfg.Code, toolInput)
		result = &tools.ExecutionResult{LatencyMs: int(time.Since(start).Milliseconds())}
		if codeErr != nil {
			result.Error = codeErr.Error()
		} else {
			result.Output = out
		}
	default: // native
		execCtx := tools.ExecutionContext{
			WorkspaceID: ws,
			UserID:      uid,
			RunID:       parentRunID,
			RootRunID:   parentRunID,
		}
		var execErr error
		result, execErr = h.executor.ExecuteWithContext(ctx, execCtx, toolName, toolInput)
		if execErr != nil {
			return "", execErr
		}
	}

	latency := int(time.Since(start).Milliseconds())
	if result != nil && result.LatencyMs > 0 {
		latency = result.LatencyMs
	}
	if result != nil && result.Error != "" {
		h.runs.createStep(ctx, parentRunID, domain.StepToolCall, //nolint:errcheck
			map[string]any{"tool": toolName, "input": toolInput, "workflow_node_id": node.ID},
			map[string]any{"error": result.Error}, start, latency, toolName, result.Error)
		return "", fmt.Errorf("%s", result.Error)
	}
	var outStr string
	if result != nil {
		b, _ := json.Marshal(result.Output)
		outStr = string(b)
	}
	h.runs.createStep(ctx, parentRunID, domain.StepToolCall, //nolint:errcheck
		map[string]any{"tool": toolName, "input": toolInput, "workflow_node_id": node.ID},
		map[string]any{"output": outStr}, start, latency, toolName, "")
	return outStr, nil
}

// executeWorkflowWebhook delivers a payload to an external HTTP endpoint.
// Config (node config for webhook nodes, or the end node's delivery keys):
//
//	url (required)              — destination
//	method (optional)           — default POST
//	headers (optional)          — map of extra request headers
//	payload_template (optional) — raw body; {{input}} / {{original_input}}
//	                              substituted. Default: JSON envelope with
//	                              workflow_id, run_id, node_id and input.
//
// Returns the (size-capped) response body so downstream nodes can branch on
// it. Non-2xx responses and transport errors return an error.
func (h *InvokeHandler) executeWorkflowWebhook(ctx context.Context, cfg map[string]any, workflowID, runID, nodeID, input, originalInput string) (string, error) {
	rawURL, _ := cfg["url"].(string)
	if rawURL == "" {
		return "", fmt.Errorf("webhook node has no url configured")
	}
	method, _ := cfg["method"].(string)
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodPost
	}

	body := ""
	if tpl, ok := cfg["payload_template"].(string); ok && strings.TrimSpace(tpl) != "" {
		body = renderWorkflowTemplate(tpl, jsonEscapeForTemplate(input), jsonEscapeForTemplate(originalInput))
	} else {
		b, _ := json.Marshal(map[string]any{
			"workflow_id": workflowID,
			"run_id":      runID,
			"node_id":     nodeID,
			"input":       input,
		})
		body = string(b)
	}

	var reader io.Reader
	if method != http.MethodGet && method != http.MethodHead {
		reader = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return "", fmt.Errorf("webhook: invalid request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if hdrs, ok := cfg["headers"].(map[string]any); ok {
		for k, v := range hdrs {
			if s, isStr := v.(string); isStr {
				req.Header.Set(k, s)
			}
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("webhook: %w", err)
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 256<<10))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("webhook: %s returned %s: %s", rawURL, res.Status, strings.TrimSpace(string(respBody)))
	}
	if len(respBody) == 0 {
		return fmt.Sprintf(`{"delivered":true,"status":%d}`, res.StatusCode), nil
	}
	return string(respBody), nil
}

// deliverWorkflowGateway sends a message through a gateway channel. Config
// (node config for gateway nodes, prefixed gateway_* on end nodes):
//
//	channel_id (required)       — gateway channel in this workspace
//	peer_id (required)          — recipient (phone/JID for WhatsApp)
//	peer_kind (optional)        — default "direct"
//	message_template (optional) — default: the node input as-is
func (h *InvokeHandler) deliverWorkflowGateway(ctx context.Context, ws string, cfg map[string]any, keyPrefix, runID, input, originalInput string) error {
	channelID, _ := cfg[keyPrefix+"channel_id"].(string)
	peerID, _ := cfg[keyPrefix+"peer_id"].(string)
	if channelID == "" || peerID == "" {
		return fmt.Errorf("gateway delivery needs channel_id and peer_id")
	}
	peerKind, _ := cfg[keyPrefix+"peer_kind"].(string)
	if peerKind == "" {
		peerKind = "direct"
	}
	body := input
	if tpl, ok := cfg[keyPrefix+"message_template"].(string); ok && strings.TrimSpace(tpl) != "" {
		body = renderWorkflowTemplate(tpl, input, originalInput)
	}

	repo := repository.NewGatewayRepository(h.pool)
	ch, err := repo.GetChannelInWorkspace(ctx, channelID, ws)
	if err != nil {
		return fmt.Errorf("gateway channel not found")
	}
	if ch.ChannelType != "whatsapp" {
		return fmt.Errorf("gateway delivery supports whatsapp channels only (channel %q is %q)", ch.Name, ch.ChannelType)
	}
	gwCfg := gatewayservice.ParseConfig(ch.Config, h.cfg.WhatsAppAdapterURL)
	if strings.Contains(peerID, "+") || !strings.Contains(peerID, "@") {
		if jid := gatewayservice.PhoneToJID(peerID); jid != "" {
			peerID = jid
		}
	}
	_, err = gatewayservice.NewService(h.pool).SendWhatsApp(ctx, gatewayservice.SendRequest{
		Channel:  ch,
		Config:   gwCfg,
		PeerKind: peerKind,
		PeerID:   peerID,
		Body:     body,
		Source:   "workflow",
		RunID:    runID,
	})
	return err
}
