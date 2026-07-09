package handler

// Integration tests for the durable-wait resume paths: re-entering the loop
// from a persisted snapshot with an approval decision, and injecting an
// externally produced session result for the pending call. These are the
// paths the Jira pipeline's park/resume machinery depends on.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
)

func approvalSnapshot(fx *loopFixture, calls []provider.ToolCall, waitType string) *runWaitState {
	return &runWaitState{
		Version:        1,
		WaitType:       waitType,
		WorkspaceID:    fx.ws,
		UserID:         fx.uid,
		AgentID:        fx.agent.ID,
		ConversationID: fx.convID,
		Input:          "test input",
		Messages: []provider.Message{
			{Role: "user", Content: "test input"},
			{Role: "assistant", Content: "", ToolCalls: calls},
		},
		PendingCalls: calls,
		CallIndex:    0,
		StepCount:    1,
	}
}

// Resume with an approval decision: the pending gated call must execute and
// the loop continue to a final reply.
func TestLoopResumeWithApprovalDecision(t *testing.T) {
	calls := []provider.ToolCall{{ID: "c1", Name: "risky", Input: json.RawMessage(`{}`)}}
	fx := newLoopFixture(t, []fakeTurn{{deltas: []string{"resumed and done"}}}, "")
	fx.attachCodeTool(t, "risky", "function run(){ return { granted: true } }", true)

	decision := ApprovalDecision{Decision: "approved"}
	fx.run(invokeOpts{resume: approvalSnapshot(fx, calls, "approval"), resumeDecision: &decision})

	status, output := fx.runRow(t)
	if status != "success" || output != "resumed and done" {
		t.Fatalf("run row = %q/%q, want success/resumed and done", status, output)
	}
	evt := fx.firstEvent("tool_call")
	if evt == nil || !strings.Contains(fmt.Sprintf("%v", evt["output"]), "granted") {
		t.Fatalf("gated tool did not execute on resume: %v", evt)
	}
	// The single provider request is the post-tool continuation — the model
	// call that produced the pending calls happened before the park.
	if n := len(fx.fake.recorded()); n != 1 {
		t.Fatalf("provider calls = %d, want 1", n)
	}
}

// Resume with a rejected decision: the pending call must NOT execute; the
// model sees the rejection message instead.
func TestLoopResumeWithRejection(t *testing.T) {
	calls := []provider.ToolCall{{ID: "c1", Name: "risky", Input: json.RawMessage(`{}`)}}
	fx := newLoopFixture(t, []fakeTurn{{deltas: []string{"understood, not doing it"}}}, "")
	fx.attachCodeTool(t, "risky", "function run(){ return { granted: true } }", true)

	decision := ApprovalDecision{Decision: "rejected"}
	fx.run(invokeOpts{resume: approvalSnapshot(fx, calls, "approval"), resumeDecision: &decision})

	status, _ := fx.runRow(t)
	if status != "success" {
		t.Fatalf("run status = %q, want success", status)
	}
	reqs := fx.fake.recorded()
	if len(reqs) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(reqs))
	}
	var sawRejection bool
	for _, m := range reqs[0].Messages {
		if m.Role == "tool" && strings.Contains(m.Content, "rejected") {
			sawRejection = true
		}
	}
	if !sawRejection {
		t.Fatalf("rejection message missing from resumed context: %+v", reqs[0].Messages)
	}
}

// Resume with an externally produced result (session callback): the pending
// call is NOT re-executed — its result is injected verbatim.
func TestLoopResumeWithSessionResult(t *testing.T) {
	calls := []provider.ToolCall{{ID: "c1", Name: "native_launch_repo_session", Input: json.RawMessage(`{"repo":"x"}`)}}
	fx := newLoopFixture(t, []fakeTurn{{deltas: []string{"session handled"}}}, "")

	sessionResult := `{"status":"success","branch":"nexus/test-1"}`
	fx.run(invokeOpts{resume: approvalSnapshot(fx, calls, "session"), resumeToolResult: &sessionResult})

	status, output := fx.runRow(t)
	if status != "success" || output != "session handled" {
		t.Fatalf("run row = %q/%q, want success/session handled", status, output)
	}
	reqs := fx.fake.recorded()
	if len(reqs) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(reqs))
	}
	var sawInjected bool
	for _, m := range reqs[0].Messages {
		if m.Role == "tool" && strings.Contains(m.Content, "nexus/test-1") {
			sawInjected = true
		}
	}
	if !sawInjected {
		t.Fatalf("injected session result missing from resumed context: %+v", reqs[0].Messages)
	}
}

// Wait-state snapshots must survive a save/load round trip intact — this is
// what RecoverInterruptedWorkflowRuns and the Approve endpoint rely on.
func TestWaitStateRoundTrip(t *testing.T) {
	fx := newLoopFixture(t, nil, "")
	st := approvalSnapshot(fx, []provider.ToolCall{{ID: "c9", Name: "x", Input: json.RawMessage(`{"k":1}`)}}, "approval")
	st.RequestedTools = []string{"t1"}
	st.ActiveSkills = []string{"s1"}

	ctx := t.Context()
	if err := saveWaitState(ctx, fx.pool, fx.runID, st); err != nil {
		t.Fatalf("saveWaitState: %v", err)
	}
	got, err := claimWaitState(ctx, fx.pool, fx.runID, "approval")
	if err != nil || got == nil {
		t.Fatalf("claimWaitState: state=%v err=%v", got, err)
	}
	if got.WaitType != "approval" || len(got.PendingCalls) != 1 || got.PendingCalls[0].ID != "c9" ||
		len(got.RequestedTools) != 1 || len(got.ActiveSkills) != 1 {
		t.Fatalf("snapshot mutated in round trip: %+v", got)
	}
	// claimWaitState consumes the row — a second claim must find nothing.
	if again, err := claimWaitState(ctx, fx.pool, fx.runID, "approval"); err != nil || again != nil {
		t.Fatalf("snapshot not consumed by claim: state=%v err=%v", again, err)
	}
}
