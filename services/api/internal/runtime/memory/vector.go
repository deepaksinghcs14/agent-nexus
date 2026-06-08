package memory

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// VectorSearch performs pgvector similarity search against the memories table.
type VectorSearch struct {
	pool *pgxpool.Pool
}

func NewVectorSearch(pool *pgxpool.Pool) *VectorSearch {
	return &VectorSearch{pool: pool}
}

func (v *VectorSearch) Search(ctx context.Context, workspaceID, agentID string, embedding []float32, limit int) ([]string, error) {
	return nil, fmt.Errorf("vector search: not implemented")
}
