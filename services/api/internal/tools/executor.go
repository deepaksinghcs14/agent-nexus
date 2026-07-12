package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrRunParked signals that the run persisted its wait state and parked while
// blocking on an external callback (e.g. a repo session). The run loop must
// exit without failing the run; an external callback resumes it later.
var ErrRunParked = errors.New("run parked awaiting external callback")

type ExecutionResult struct {
	Output    any
	LatencyMs int
	Error     string
}

// Executor runs a tool after risk-checking and optionally waiting for approval.
type Executor struct {
	registry *Registry
}

func NewExecutor(registry *Registry) *Executor {
	return &Executor{registry: registry}
}

func (e *Executor) ExecuteWithContext(ctx context.Context, execCtx ExecutionContext, toolName string, rawInput json.RawMessage) (*ExecutionResult, error) {
	tool, err := e.registry.Get(toolName)
	if err != nil {
		return nil, fmt.Errorf("executor: %w", err)
	}

	var input map[string]any
	if err := json.Unmarshal(rawInput, &input); err != nil {
		return nil, fmt.Errorf("executor: parse input: %w", err)
	}

	start := time.Now()
	var output any
	var execErr error
	if t, ok := tool.(ContextAwareTool); ok {
		output, execErr = t.ExecuteWithContext(ctx, execCtx, input)
	} else {
		output, execErr = tool.Execute(input)
	}
	// A park is control flow, not a tool failure — propagate it as an error so
	// the run loop can exit without recording a failed step.
	if errors.Is(execErr, ErrRunParked) {
		return nil, execErr
	}
	latency := int(time.Since(start).Milliseconds())

	result := &ExecutionResult{Output: output, LatencyMs: latency}
	if execErr != nil {
		result.Error = execErr.Error()
	}
	return result, nil
}
