package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
)

// The code branch runs entirely in-process, so it exercises the dispatch
// without a DB, config, or registry.
func TestInvokeToolByTypeCode(t *testing.T) {
	tool := domain.Tool{
		Name:      "adder",
		Type:      "code",
		Config:    json.RawMessage(`{"code":"function run(input){ return { sum: input.a + input.b } }"}`),
		TimeoutMs: 2000,
	}
	res, err := invokeToolByType(context.Background(), nil, nil, nil, tools.ExecutionContext{}, tool, true, "adder", json.RawMessage(`{"a":2,"b":3}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Error != "" {
		t.Fatalf("expected success result, got %+v", res)
	}
	out, ok := res.Output.(map[string]any)
	if !ok || fmt.Sprintf("%v", out["sum"]) != "5" {
		t.Fatalf("expected sum=5, got %#v", res.Output)
	}
}

// A tool without a DB row (toolExists=false) must fall through to the native
// registry executor rather than any typed transport.
func TestInvokeToolByTypeFallsThroughToRegistry(t *testing.T) {
	reg := tools.NewRegistry()
	exec := tools.NewExecutor(reg)
	_, err := invokeToolByType(context.Background(), nil, nil, exec, tools.ExecutionContext{}, domain.Tool{}, false, "nonexistent_tool", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected registry-miss error for unknown native tool")
	}
}
