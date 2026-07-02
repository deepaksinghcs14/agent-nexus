package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
)

func TestRunWaitStateRoundTrip(t *testing.T) {
	st := &runWaitState{
		Version:        1,
		WorkspaceID:    "ws-1",
		UserID:         "user-1",
		AgentID:        "agent-1",
		ConversationID: "conv-1",
		Input:          "do the thing",
		Messages: []provider.Message{
			{Role: "system", Content: "instructions"},
			{Role: "user", Content: "do the thing"},
			{Role: "assistant", Content: "on it", ToolCalls: []provider.ToolCall{
				{ID: "call-1", Name: "danger_tool", Input: json.RawMessage(`{"x":1}`)},
			}},
		},
		StableSystem:     "stable",
		PendingCalls:     []provider.ToolCall{{ID: "call-1", Name: "danger_tool", Input: json.RawMessage(`{"x":1}`)}},
		CallIndex:        0,
		StepCount:        3,
		TotalInput:       120,
		TotalOutput:      45,
		ActionLog:        []string{"did a thing"},
		MemorySaveCalled: true,
		RequestedTools:   []string{"native_web_search"},
		ActiveSkills:     []string{"research"},
	}
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got runWaitState
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.CallIndex != 0 || got.StepCount != 3 || got.TotalInput != 120 || !got.MemorySaveCalled {
		t.Errorf("counters not preserved: %+v", got)
	}
	if len(got.Messages) != 3 || len(got.Messages[2].ToolCalls) != 1 {
		t.Errorf("messages not preserved: %+v", got.Messages)
	}
	if got.PendingCalls[0].Name != "danger_tool" || string(got.PendingCalls[0].Input) != `{"x":1}` {
		t.Errorf("pending calls not preserved: %+v", got.PendingCalls)
	}
	if !setOf(got.RequestedTools)["native_web_search"] || !setOf(got.ActiveSkills)["research"] {
		t.Errorf("tool/skill sets not preserved")
	}
}

func TestKeysOfSetOfRoundTrip(t *testing.T) {
	in := map[string]bool{"a": true, "b": true, "c": false}
	out := setOf(keysOf(in))
	if !out["a"] || !out["b"] {
		t.Errorf("true keys lost: %v", out)
	}
	if out["c"] {
		t.Errorf("false key should not survive: %v", out)
	}
}

func TestAwaitApprovalDecisionReceived(t *testing.T) {
	runID := "test-run-received"
	ch := RegisterApprovalWait(runID)
	go func() {
		time.Sleep(10 * time.Millisecond)
		if !SendApprovalDecision(runID, ApprovalDecision{Decision: "approved"}) {
			t.Error("send should find the registered channel")
		}
	}()
	d, got := awaitApprovalDecision(runID, ch, 5*time.Second)
	if !got || d.Decision != "approved" {
		t.Fatalf("expected approved decision, got=%v d=%+v", got, d)
	}
	// The sender's LoadAndDelete removed the entry; a second send finds nothing.
	if SendApprovalDecision(runID, ApprovalDecision{Decision: "approved"}) {
		t.Error("second send should not find a channel")
	}
}

func TestAwaitApprovalDecisionTimeout(t *testing.T) {
	runID := "test-run-timeout"
	ch := RegisterApprovalWait(runID)
	_, got := awaitApprovalDecision(runID, ch, 20*time.Millisecond)
	if got {
		t.Fatal("expected clean timeout")
	}
	// The timeout reclaimed the registry entry — a late Approve must fall back
	// to the resume path, not deliver into a dead channel.
	if SendApprovalDecision(runID, ApprovalDecision{Decision: "approved"}) {
		t.Error("send after timeout should not find a channel")
	}
}
