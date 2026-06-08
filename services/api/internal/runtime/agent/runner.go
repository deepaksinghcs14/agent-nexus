package agent

import (
	"context"
	"fmt"
)

// RunRequest carries the inputs for a single agent run.
type RunRequest struct {
	RunID          string
	ConversationID string
	AgentID        string
	WorkspaceID    string
	UserID         string
	Input          string
}

// Runner executes the agent run loop.
type Runner struct{}

func NewRunner() *Runner { return &Runner{} }

func (r *Runner) Execute(ctx context.Context, req RunRequest) error {
	return fmt.Errorf("runner: Execute not implemented")
}
