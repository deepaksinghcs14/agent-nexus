package memory

import (
	"context"

	"github.com/agentNexus/agent-nexus/services/api/internal/domain"
	"github.com/agentNexus/agent-nexus/services/api/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Engine retrieves and stores memories for an agent run.
type Engine struct {
	memories *repository.MemoryRepository
}

func NewEngine(pool *pgxpool.Pool) *Engine {
	return &Engine{memories: repository.NewMemoryRepository(pool)}
}

func (e *Engine) Retrieve(ctx context.Context, agentID, workspaceID, query string) ([]domain.Memory, error) {
	return e.memories.Search(ctx, workspaceID, agentID, nil, 8)
}

func (e *Engine) Store(ctx context.Context, m *domain.Memory) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	return e.memories.Store(ctx, m, nil)
}

func (e *Engine) Summarise(ctx context.Context, runID string) error {
	return nil
}
