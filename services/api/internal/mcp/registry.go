package mcp

import (
	"sync"
)

// ServerRegistry holds active MCP client connections keyed by server ID.
type ServerRegistry struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

func NewServerRegistry() *ServerRegistry {
	return &ServerRegistry{clients: make(map[string]*Client)}
}

func (r *ServerRegistry) Register(serverID string, c *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[serverID] = c
}

func (r *ServerRegistry) Get(serverID string) (*Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.clients[serverID]
	return c, ok
}

func (r *ServerRegistry) Remove(serverID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, serverID)
}
