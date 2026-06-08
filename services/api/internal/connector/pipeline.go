package connector

import (
	"context"
	"fmt"
)

// Pipeline orchestrates fetch → chunk → embed → upsert for a connector sync.
type Pipeline struct{}

func NewPipeline() *Pipeline { return &Pipeline{} }

func (p *Pipeline) Sync(ctx context.Context, connectorID string) error {
	return fmt.Errorf("connector pipeline: Sync not implemented for %s", connectorID)
}
