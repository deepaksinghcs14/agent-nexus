package tools

import (
	"fmt"
	"sync"

	"github.com/agentNexus/agent-nexus/services/api/internal/domain"
)

// NativeTool is the interface all native tools implement.
type NativeTool interface {
	Definition() domain.Tool
	Execute(input map[string]any) (any, error)
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]NativeTool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]NativeTool)}
}

func (r *Registry) Register(t NativeTool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Definition().Name] = t
}

func (r *Registry) Get(name string) (NativeTool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not found in registry", name)
	}
	return t, nil
}

func (r *Registry) All() []domain.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Definition())
	}
	return out
}
