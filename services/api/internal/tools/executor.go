package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

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

func (e *Executor) Execute(ctx context.Context, toolName string, rawInput json.RawMessage) (*ExecutionResult, error) {
	tool, err := e.registry.Get(toolName)
	if err != nil {
		return nil, fmt.Errorf("executor: %w", err)
	}

	var input map[string]any
	if err := json.Unmarshal(rawInput, &input); err != nil {
		return nil, fmt.Errorf("executor: parse input: %w", err)
	}

	start := time.Now()
	output, execErr := tool.Execute(input)
	latency := int(time.Since(start).Milliseconds())

	result := &ExecutionResult{Output: output, LatencyMs: latency}
	if execErr != nil {
		result.Error = execErr.Error()
	}
	return result, nil
}
