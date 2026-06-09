package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/agentNexus/agent-nexus/services/api/internal/domain"
)

// ─── JSON-RPC 2.0 types ───────────────────────────────────────────────────────

type rpcReq struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"` // nil → notification (no response expected)
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResp struct {
	ID     *int64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcErr         `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// UnmarshalJSON handles both standard JSON-RPC errors {"code":N,"message":"..."}
// and non-standard string errors {"error": "some message"}.
func (e *rpcErr) UnmarshalJSON(data []byte) error {
	// Try standard object form first.
	var obj struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &obj) == nil && (obj.Code != 0 || obj.Message != "") {
		e.Code = obj.Code
		e.Message = obj.Message
		return nil
	}
	// Fall back to plain string.
	var s string
	if json.Unmarshal(data, &s) == nil {
		e.Message = s
		return nil
	}
	e.Message = string(data)
	return nil
}

var reqCounter int64

func nextID() int64 { return atomic.AddInt64(&reqCounter, 1) }

func newReq(method string, params any) rpcReq {
	id := nextID()
	return rpcReq{JSONRPC: "2.0", ID: &id, Method: method, Params: params}
}

func notification(method string, params any) rpcReq {
	return rpcReq{JSONRPC: "2.0", Method: method, Params: params}
}

// MCP protocol version we advertise during initialize.
var initParams = map[string]any{
	"protocolVersion": "2024-11-05",
	"capabilities":    map[string]any{},
	"clientInfo":      map[string]any{"name": "agent-nexus", "version": "0.1.0"},
}

// ─── server response shapes ───────────────────────────────────────────────────

type toolsListResult struct {
	Tools []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"inputSchema"`
	} `json:"tools"`
}

type callToolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// ─── docker host rewriting ────────────────────────────────────────────────────

// inDocker reports whether we are running inside a Docker container.
var inDocker = func() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}()

// resolveURL rewrites 127.0.0.1 / localhost to host.docker.internal when
// running inside Docker so the API container can reach services on the host.
func resolveURL(raw string) string {
	if !inDocker {
		return raw
	}
	s := strings.Replace(raw, "127.0.0.1", "host.docker.internal", 1)
	s = strings.Replace(s, "localhost", "host.docker.internal", 1)
	return s
}

// ─── client ───────────────────────────────────────────────────────────────────

// Client connects to an MCP server and proxies tool calls.
// For HTTP transport, url is the server's base URL (e.g. "https://mcp.example.com").
// For stdio transport, url holds the command string (e.g. "npx @modelcontextprotocol/server-filesystem /data").
type Client struct {
	serverID  string
	url       string
	transport string // "http" | "stdio"
	config    json.RawMessage

	// parsed from config
	authHeader string // header name, default "Authorization"
	authValue  string // full header value, e.g. "Bearer sk-abc123" or raw token
}

func NewClient(serverID, url, transport string, config json.RawMessage) *Client {
	c := &Client{serverID: serverID, url: url, transport: transport, config: config}
	// Parse optional auth fields from config:
	//   {"token": "sk-abc123"}              → Authorization: Bearer sk-abc123
	//   {"header": "X-API-Key", "token": "abc123"} → X-API-Key: abc123
	if len(config) > 0 {
		var cfg struct {
			Token  string `json:"token"`
			Header string `json:"header"`
		}
		if json.Unmarshal(config, &cfg) == nil && cfg.Token != "" {
			c.authHeader = cfg.Header
			if c.authHeader == "" {
				c.authHeader = "Authorization"
				c.authValue = "Bearer " + cfg.Token
			} else {
				c.authValue = cfg.Token
			}
		}
	}
	return c
}

// ListTools connects to the server, runs the initialize handshake, and calls
// tools/list. Returns the full list of tools advertised by the server.
func (c *Client) ListTools(ctx context.Context) ([]domain.MCPTool, error) {
	if c.transport == "stdio" {
		return c.listToolsStdio(ctx)
	}
	return c.listToolsHTTP(ctx)
}

// CallTool connects, handshakes, and calls tools/call with the given name and
// JSON-encoded arguments.
func (c *Client) CallTool(ctx context.Context, toolName string, input json.RawMessage) (any, error) {
	if c.transport == "stdio" {
		return c.callToolStdio(ctx, toolName, input)
	}
	return c.callToolHTTP(ctx, toolName, input)
}

// Ping verifies the server is reachable by running a minimal tools/list call.
func (c *Client) Ping(ctx context.Context) error {
	if c.transport == "stdio" {
		_, err := c.listToolsStdio(ctx)
		return err
	}
	_, err := c.listToolsHTTP(ctx)
	return err
}

// ─── HTTP transport ───────────────────────────────────────────────────────────

var httpClient = &http.Client{Timeout: 30 * time.Second}

// doHTTP sends a single JSON-RPC request via POST and returns the parsed response
// plus any Mcp-Session-Id the server set (empty string if none).
// Pass a non-empty sessionID to include it in the request headers.
func (c *Client) doHTTP(ctx context.Context, req rpcReq, sessionID string) (rpcResp, string, error) {
	body, _ := json.Marshal(req)
	resolvedURL := resolveURL(c.url)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, resolvedURL, bytes.NewReader(body))
	if err != nil {
		return rpcResp{}, "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	// MCP Streamable HTTP spec: Origin must match the server host (CSRF protection).
	if u, err := url.Parse(resolvedURL); err == nil {
		httpReq.Header.Set("Origin", u.Scheme+"://"+u.Host)
	}
	// Session continuity: echo back the session ID the server gave us on initialize.
	if sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", sessionID)
	}
	if c.authHeader != "" {
		httpReq.Header.Set(c.authHeader, c.authValue)
	}

	slog.Debug("mcp http request",
		"method", req.Method,
		"url", resolvedURL,
		"session_id", sessionID,
	)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return rpcResp{}, "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	newSessionID := resp.Header.Get("Mcp-Session-Id")

	// SSE responses start with "data: " — parse the first data line.
	ct := resp.Header.Get("Content-Type")
	var rawBody []byte
	if strings.Contains(ct, "text/event-stream") {
		rawBody, err = extractSSEData(io.LimitReader(resp.Body, 1<<20))
	} else {
		rawBody, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	}
	if err != nil {
		return rpcResp{}, newSessionID, fmt.Errorf("read body: %w", err)
	}

	var r rpcResp
	if err := json.Unmarshal(rawBody, &r); err != nil {
		return rpcResp{}, newSessionID, fmt.Errorf("decode: %w (body: %.200s)", err, rawBody)
	}
	if r.Error != nil {
		return rpcResp{}, newSessionID, fmt.Errorf("rpc error %d: %s", r.Error.Code, r.Error.Message)
	}
	return r, newSessionID, nil
}

// extractSSEData reads the first "data: {...}" line from an SSE stream and
// returns the JSON payload.
func extractSSEData(r io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			return []byte(strings.TrimPrefix(line, "data: ")), nil
		}
	}
	return nil, fmt.Errorf("no data line in SSE response")
}

func (c *Client) listToolsHTTP(ctx context.Context) ([]domain.MCPTool, error) {
	// initialize — capture the session ID the server assigns.
	_, sessionID, _ := c.doHTTP(ctx, newReq("initialize", initParams), "")

	r, _, err := c.doHTTP(ctx, newReq("tools/list", map[string]any{}), sessionID)
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}
	return parseToolsList(c.serverID, r.Result)
}

func (c *Client) callToolHTTP(ctx context.Context, toolName string, input json.RawMessage) (any, error) {
	_, sessionID, _ := c.doHTTP(ctx, newReq("initialize", initParams), "")

	var args any
	json.Unmarshal(input, &args) //nolint:errcheck
	r, _, err := c.doHTTP(ctx, newReq("tools/call", map[string]any{
		"name":      toolName,
		"arguments": args,
	}), sessionID)
	if err != nil {
		return nil, err
	}
	return parseCallResult(r.Result)
}

// ─── stdio transport ──────────────────────────────────────────────────────────

type stdioConn struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	enc    *json.Encoder
}

// dialStdio spawns the MCP process and returns an open connection.
// The command string in c.url is split by whitespace (simple tokenisation).
func (c *Client) dialStdio(ctx context.Context) (*stdioConn, error) {
	parts := strings.Fields(c.url)
	if len(parts) == 0 {
		return nil, fmt.Errorf("mcp stdio: empty command")
	}
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Stderr = io.Discard // prevent debug output from contaminating stdout

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("spawn %q: %w", parts[0], err)
	}
	conn := &stdioConn{
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReader(stdout),
		enc:    json.NewEncoder(stdin),
	}
	return conn, nil
}

// send writes a JSON-RPC request to stdin (newline-delimited JSON).
func (conn *stdioConn) send(req rpcReq) error {
	return conn.enc.Encode(req)
}

// recv reads lines from stdout until it finds a JSON-RPC response whose id
// matches wantID. Non-JSON lines and server-initiated notifications are skipped.
// The calling context's deadline governs the overall timeout via process kill.
func (conn *stdioConn) recv(wantID int64) (rpcResp, error) {
	for {
		line, err := conn.reader.ReadBytes('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF {
				return rpcResp{}, fmt.Errorf("stdio: process closed stdout")
			}
			return rpcResp{}, fmt.Errorf("stdio read: %w", err)
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] != '{' {
			continue // skip blank / non-JSON startup output
		}
		var r rpcResp
		if json.Unmarshal(line, &r) != nil {
			continue // malformed line — skip
		}
		if r.ID == nil || *r.ID != wantID {
			continue // notification or a response to a different request
		}
		if r.Error != nil {
			return rpcResp{}, fmt.Errorf("rpc error %d: %s", r.Error.Code, r.Error.Message)
		}
		return r, nil
	}
}

// close kills the child process and closes stdin.
func (conn *stdioConn) close() {
	conn.stdin.Close()
	if conn.cmd.Process != nil {
		conn.cmd.Process.Kill() //nolint:errcheck
	}
	conn.cmd.Wait() //nolint:errcheck
}

// stdioHandshake runs the MCP initialize / initialized exchange.
func (c *Client) stdioHandshake(conn *stdioConn) error {
	req := newReq("initialize", initParams)
	if err := conn.send(req); err != nil {
		return err
	}
	if _, err := conn.recv(*req.ID); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	// Send initialized notification — no response expected.
	return conn.send(notification("notifications/initialized", nil))
}

func (c *Client) listToolsStdio(ctx context.Context) ([]domain.MCPTool, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := c.dialStdio(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.close()

	if err := c.stdioHandshake(conn); err != nil {
		return nil, err
	}
	req := newReq("tools/list", map[string]any{})
	if err := conn.send(req); err != nil {
		return nil, err
	}
	r, err := conn.recv(*req.ID)
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}
	return parseToolsList(c.serverID, r.Result)
}

func (c *Client) callToolStdio(ctx context.Context, toolName string, input json.RawMessage) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := c.dialStdio(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.close()

	if err := c.stdioHandshake(conn); err != nil {
		return nil, err
	}
	var args any
	json.Unmarshal(input, &args) //nolint:errcheck
	req := newReq("tools/call", map[string]any{
		"name":      toolName,
		"arguments": args,
	})
	if err := conn.send(req); err != nil {
		return nil, err
	}
	r, err := conn.recv(*req.ID)
	if err != nil {
		return nil, fmt.Errorf("tools/call: %w", err)
	}
	return parseCallResult(r.Result)
}

// ─── result parsers ───────────────────────────────────────────────────────────

func parseToolsList(serverID string, raw json.RawMessage) ([]domain.MCPTool, error) {
	var result toolsListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list: %w", err)
	}
	tools := make([]domain.MCPTool, 0, len(result.Tools))
	for _, t := range result.Tools {
		schema := t.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{}`)
		}
		tools = append(tools, domain.MCPTool{
			ServerID:    serverID,
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
			RiskLevel:   "medium",
			Enabled:     true,
		})
	}
	return tools, nil
}

func parseCallResult(raw json.RawMessage) (any, error) {
	var result callToolResult
	if json.Unmarshal(raw, &result) == nil && len(result.Content) > 0 {
		if result.IsError {
			return nil, fmt.Errorf("tool error: %s", result.Content[0].Text)
		}
		if len(result.Content) == 1 {
			return result.Content[0].Text, nil
		}
		texts := make([]string, len(result.Content))
		for i, ct := range result.Content {
			texts[i] = ct.Text
		}
		return texts, nil
	}
	// Fall back: return whatever the server gave us.
	var v any
	if json.Unmarshal(raw, &v) == nil {
		return v, nil
	}
	return string(raw), nil
}
