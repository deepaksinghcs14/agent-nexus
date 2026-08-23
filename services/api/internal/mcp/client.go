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

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
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
	env        map[string]string
}

func NewClient(serverID, url, transport string, config json.RawMessage) *Client {
	c := &Client{serverID: serverID, url: url, transport: transport, config: config}
	// Parse optional auth fields from config:
	//   {"token": "sk-abc123"}              → Authorization: Bearer sk-abc123
	//   {"header": "X-API-Key", "token": "abc123"} → X-API-Key: abc123
	//   {"env": {"JIRA_API_TOKEN": "abc"}}  → stdio process environment
	if len(config) > 0 {
		var cfg struct {
			Token  string            `json:"token"`
			Header string            `json:"header"`
			Env    map[string]string `json:"env"`
		}
		if json.Unmarshal(config, &cfg) == nil {
			c.env = cfg.Env
			if cfg.Token != "" {
				c.authHeader = cfg.Header
				if c.authHeader == "" {
					c.authHeader = "Authorization"
					c.authValue = "Bearer " + cfg.Token
				} else {
					c.authValue = cfg.Token
				}
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
// JSON-encoded arguments. Also returns the raw JSON-RPC request and response
// exchanged for the tools/call step, so a caller debugging a broken
// integration can see exactly what was sent and received.
func (c *Client) CallTool(ctx context.Context, toolName string, input json.RawMessage) (result any, reqJSON, respJSON []byte, err error) {
	if c.transport == "stdio" {
		return c.callToolStdio(ctx, toolName, input)
	}
	return c.callToolHTTP(ctx, toolName, input)
}

// ─── HTTP transport ───────────────────────────────────────────────────────────

var httpClient = &http.Client{Timeout: 30 * time.Second}

// doHTTP sends a single JSON-RPC request via POST and returns the parsed response
// plus any Mcp-Session-Id the server set (empty string if none).
// Pass a non-empty sessionID to include it in the request headers.
// doHTTP also returns the raw request and response bytes exchanged, so a
// caller debugging a broken MCP integration (the tool-test UI) can show
// exactly what was sent and received, not just the parsed result.
func (c *Client) doHTTP(ctx context.Context, req rpcReq, sessionID string) (rpcResp, string, []byte, []byte, error) {
	reqBody, _ := json.Marshal(req)
	resolvedURL := resolveURL(c.url)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, resolvedURL, bytes.NewReader(reqBody))
	if err != nil {
		return rpcResp{}, "", reqBody, nil, err
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
		return rpcResp{}, "", reqBody, nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	newSessionID := resp.Header.Get("Mcp-Session-Id")

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return rpcResp{}, newSessionID, reqBody, nil, fmt.Errorf("read body: %w", err)
	}
	// Check status before attempting to parse the body — an auth failure
	// typically comes back with a short or empty body that isn't valid SSE
	// or JSON-RPC, and would otherwise surface as a confusing parse error
	// instead of the actual rejection.
	if resp.StatusCode >= 400 {
		return rpcResp{}, newSessionID, reqBody, bodyBytes, fmt.Errorf("mcp server returned %d: %.300s", resp.StatusCode, bodyBytes)
	}

	// Content-Type isn't trustworthy here — some servers declare
	// text/event-stream (echoing our Accept header) but actually respond
	// with a single plain JSON object, not real SSE framing. Try the body
	// as-is first; only fall back to SSE data-line extraction if it isn't
	// valid JSON on its own.
	rawBody := bodyBytes
	if !json.Valid(bodyBytes) {
		rawBody, err = extractSSEData(bytes.NewReader(bodyBytes))
		if err != nil {
			return rpcResp{}, newSessionID, reqBody, bodyBytes, fmt.Errorf("read body: %w", err)
		}
	}

	var r rpcResp
	if err := json.Unmarshal(rawBody, &r); err != nil {
		return rpcResp{}, newSessionID, reqBody, bodyBytes, fmt.Errorf("decode: %w (body: %.200s)", err, rawBody)
	}
	if r.Error != nil {
		return rpcResp{}, newSessionID, reqBody, bodyBytes, fmt.Errorf("rpc error %d: %s", r.Error.Code, r.Error.Message)
	}
	return r, newSessionID, reqBody, bodyBytes, nil
}

// extractSSEData reads the first event's data field from an SSE stream and
// returns the JSON payload. Per the SSE spec, a "data:" line's value may omit
// the single space after the colon, and a single event's data may be split
// across multiple "data:" lines (joined with "\n") before the blank line
// that ends the event.
func extractSSEData(r io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(r)
	// bufio.Scanner's default max line length is 64KB — a single "data:"
	// line easily exceeds that once the payload is a real tool result (a
	// task list, search results, etc.), not the toy fixtures this was
	// tested against. Match the 1MB cap doHTTP already reads the body under,
	// so a too-long line can only happen for a body that would've been
	// truncated anyway.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var data []string
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		case line == "" && len(data) > 0:
			return []byte(strings.Join(data, "\n")), nil
		}
	}
	if len(data) > 0 {
		return []byte(strings.Join(data, "\n")), nil
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("no data line in SSE response: %w", err)
	}
	return nil, fmt.Errorf("no data line in SSE response")
}

func (c *Client) listToolsHTTP(ctx context.Context) ([]domain.MCPTool, error) {
	// initialize — capture the session ID the server assigns.
	_, sessionID, _, _, _ := c.doHTTP(ctx, newReq("initialize", initParams), "")

	r, _, _, _, err := c.doHTTP(ctx, newReq("tools/list", map[string]any{}), sessionID)
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}
	return parseToolsList(c.serverID, r.Result)
}

func (c *Client) callToolHTTP(ctx context.Context, toolName string, input json.RawMessage) (any, []byte, []byte, error) {
	_, sessionID, _, _, _ := c.doHTTP(ctx, newReq("initialize", initParams), "")

	var args any
	json.Unmarshal(input, &args) //nolint:errcheck
	r, _, reqBody, respBody, err := c.doHTTP(ctx, newReq("tools/call", map[string]any{
		"name":      toolName,
		"arguments": args,
	}), sessionID)
	if err != nil {
		return nil, reqBody, respBody, err
	}
	result, err := parseCallResult(r.Result)
	return result, reqBody, respBody, err
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
	if len(c.env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range c.env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

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
// Also returns the matched raw line, for callers that need to show the exact
// wire response (the tool-test UI).
func (conn *stdioConn) recv(wantID int64) (rpcResp, []byte, error) {
	for {
		line, err := conn.reader.ReadBytes('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF {
				return rpcResp{}, nil, fmt.Errorf("stdio: process closed stdout")
			}
			return rpcResp{}, nil, fmt.Errorf("stdio read: %w", err)
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
			return rpcResp{}, line, fmt.Errorf("rpc error %d: %s", r.Error.Code, r.Error.Message)
		}
		return r, line, nil
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
	if _, _, err := conn.recv(*req.ID); err != nil {
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
	r, _, err := conn.recv(*req.ID)
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}
	return parseToolsList(c.serverID, r.Result)
}

func (c *Client) callToolStdio(ctx context.Context, toolName string, input json.RawMessage) (any, []byte, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := c.dialStdio(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	defer conn.close()

	if err := c.stdioHandshake(conn); err != nil {
		return nil, nil, nil, err
	}
	var args any
	json.Unmarshal(input, &args) //nolint:errcheck
	req := newReq("tools/call", map[string]any{
		"name":      toolName,
		"arguments": args,
	})
	reqBody, _ := json.Marshal(req)
	if err := conn.send(req); err != nil {
		return nil, reqBody, nil, err
	}
	r, respBody, err := conn.recv(*req.ID)
	if err != nil {
		return nil, reqBody, respBody, fmt.Errorf("tools/call: %w", err)
	}
	result, err := parseCallResult(r.Result)
	return result, reqBody, respBody, err
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
