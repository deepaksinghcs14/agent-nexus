package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	gatewayservice "github.com/deepaksingh/agent-nexus/services/api/internal/gateway"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools/native"
)

// Executors for the non-agent workflow nodes (webhook / gateway) and the end
// node's delivery config. These run inline in the workflow walk — they are
// side-effectful integration steps, not LLM calls, so they have no sub-run of
// their own.

// tplPathRe matches dotted-path placeholders like {{input.branch}} or
// {{original_input.ticket.key}} — resolved against the value parsed as JSON.
var tplPathRe = regexp.MustCompile(`\{\{(input|original_input)((?:\.[A-Za-z0-9_-]+)+)\}\}`)

// renderWorkflowTemplate substitutes the placeholders workflow node templates
// support: {{input}} (the previous node's output), {{original_input}} (the
// input the workflow run started with), and dotted-path forms like
// {{input.branch}} that extract a field from the value parsed as JSON —
// needed when a tool node consumes a specific field of a JSON output (e.g.
// the branch a coding session returned). Non-JSON input or a missing path
// renders as the empty string, which downstream arg validation surfaces.
func renderWorkflowTemplate(tpl, input, originalInput string) string {
	out := tplPathRe.ReplaceAllStringFunc(tpl, func(m string) string {
		g := tplPathRe.FindStringSubmatch(m)
		src := input
		if g[1] == "original_input" {
			src = originalInput
		}
		return jsonPathValue(src, strings.Split(strings.TrimPrefix(g[2], "."), "."))
	})
	out = strings.ReplaceAll(out, "{{input}}", input)
	return strings.ReplaceAll(out, "{{original_input}}", originalInput)
}

// jsonPathValue resolves a dotted key path in a JSON document. String leaves
// are returned bare; other values re-marshal as JSON. Missing paths → "".
func jsonPathValue(doc string, path []string) string {
	var cur any
	if json.Unmarshal([]byte(doc), &cur) != nil {
		return ""
	}
	for _, key := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		if cur, ok = m[key]; !ok {
			return ""
		}
	}
	switch v := cur.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// jsonEscapeForTemplate renders a value safe to substitute inside a JSON
// string template: the surrounding quotes from json.Marshal are stripped.
func jsonEscapeForTemplate(s string) string {
	b, _ := json.Marshal(s)
	return strings.Trim(string(b), `"`)
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

	client := native.SafeHTTPClient(h.cfg.HTTPToolAllowHosts, 30*time.Second)
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
