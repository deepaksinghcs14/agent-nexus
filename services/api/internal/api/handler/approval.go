package handler

import (
	"encoding/json"
	"sync"
)

// ApprovalDecision is sent from the Approve endpoint to a waiting run goroutine.
type ApprovalDecision struct {
	Decision string          // "approved" | "rejected"
	Input    json.RawMessage // optional override tool input
}

var approvalRegistry sync.Map // map[runID string] → chan ApprovalDecision

// ── User input registry ────────────────────────────────────────────────────────

var userInputRegistry sync.Map // map[runID string] → chan string

// RegisterUserInputWait creates and stores a channel for the given run ID.
// The run goroutine calls this before entering user_input_wait state and then blocks on the channel.
func RegisterUserInputWait(runID string) chan string {
	ch := make(chan string, 1)
	userInputRegistry.Store(runID, ch)
	return ch
}

// SendUserInput delivers the user's answer to the waiting run goroutine.
// Returns true if a goroutine was waiting and the answer was sent.
func SendUserInput(runID, answer string) bool {
	v, ok := userInputRegistry.LoadAndDelete(runID)
	if !ok {
		return false
	}
	select {
	case v.(chan string) <- answer:
		return true
	default:
		return false
	}
}

// UnregisterUserInputWait removes a run from the user input registry (called on timeout or cancellation).
func UnregisterUserInputWait(runID string) {
	userInputRegistry.Delete(runID)
}

// RegisterApprovalWait creates and stores a channel for the given run ID.
// The run goroutine calls this before entering approval_wait state and then blocks on the channel.
func RegisterApprovalWait(runID string) chan ApprovalDecision {
	ch := make(chan ApprovalDecision, 1)
	approvalRegistry.Store(runID, ch)
	return ch
}

// SendApprovalDecision delivers a decision to the waiting run goroutine.
// Returns true if a goroutine was waiting and the decision was sent.
func SendApprovalDecision(runID string, d ApprovalDecision) bool {
	v, ok := approvalRegistry.Load(runID)
	if !ok {
		return false
	}
	ch := v.(chan ApprovalDecision)
	select {
	case ch <- d:
		approvalRegistry.Delete(runID)
		return true
	default:
		return false
	}
}

// UnregisterApprovalWait removes a run from the registry (called on timeout or cancellation).
func UnregisterApprovalWait(runID string) {
	approvalRegistry.Delete(runID)
}
