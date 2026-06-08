package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agentNexus/agent-nexus/services/api/internal/domain"
)

// Client connects to an MCP server and proxies tool calls.
type Client struct {
	serverID string
	url      string
	config   json.RawMessage
}

func NewClient(serverID, url string, config json.RawMessage) *Client {
	return &Client{serverID: serverID, url: url, config: config}
}

func (c *Client) ListTools(ctx context.Context) ([]domain.MCPTool, error) {
	return nil, fmt.Errorf("mcp: ListTools not implemented")
}

func (c *Client) CallTool(ctx context.Context, toolName string, input json.RawMessage) (any, error) {
	return nil, fmt.Errorf("mcp: CallTool not implemented")
}

func (c *Client) Ping(ctx context.Context) error {
	return fmt.Errorf("mcp: Ping not implemented")
}
