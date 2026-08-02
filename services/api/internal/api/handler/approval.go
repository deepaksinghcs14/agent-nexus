package handler

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// ApprovalDecision is sent from the Approve endpoint to a waiting run goroutine.
type ApprovalDecision struct {
	Decision string          // "approved" | "rejected"
	Input    json.RawMessage // optional override tool input
}

var approvalRegistry sync.Map // map[runID string] → chan ApprovalDecision

// ── Run cancel registry ────────────────────────────────────────────────────
//
// Maps a run ID to the cancel func of the goroutine executing it, so Cancel
// stops in-flight model calls, tool execution and waits instead of only
// flipping the DB status. Same shape as ConnectorsHandler.syncCancels. The
// stored value is a pointer wrapper, not the func itself: sync.Map's
// CompareAndDelete panics on an uncomparable type (func values are
// uncomparable), and without CompareAndDelete a resumed run's fresh entry
// could be deleted by the previous goroutine still unwinding.
type runCancel struct{ cancel context.CancelFunc }

var runCancels sync.Map // map[runID string] → *runCancel

// registerRunCancel derives a cancellable context from ctx and registers its
// cancel func under runID. The returned release func must be deferred by the
// caller — it unregisters (so a finished run's entry doesn't leak) and always
// cancels the derived context to free its resources.
func registerRunCancel(ctx context.Context, runID string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	rc := &runCancel{cancel: cancel}
	runCancels.Store(runID, rc)
	return ctx, func() {
		runCancels.CompareAndDelete(runID, rc)
		cancel()
	}
}

// cancelRun stops the goroutine executing runID, if one is currently
// registered. No-op if the run isn't live in this process (already finished,
// parked, or on another replica).
func cancelRun(runID string) {
	if v, ok := runCancels.LoadAndDelete(runID); ok {
		v.(*runCancel).cancel()
	}
}

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
// The LoadAndDelete makes the claim exclusive: at most one sender ever gets
// the channel, so the buffered send below can never block.
func SendApprovalDecision(runID string, d ApprovalDecision) bool {
	v, ok := approvalRegistry.LoadAndDelete(runID)
	if !ok {
		return false
	}
	v.(chan ApprovalDecision) <- d
	return true
}

// awaitApprovalDecision blocks until a decision arrives on ch, ctx is
// cancelled, or timeout elapses. Returns (decision, true) when a decision was
// received — including one that raced the cancel/timeout — or (zero, false)
// otherwise, with the registry entry reclaimed, meaning the caller may safely
// park or fail the run.
func awaitApprovalDecision(ctx context.Context, runID string, ch chan ApprovalDecision, timeout time.Duration) (ApprovalDecision, bool) {
	select {
	case d := <-ch:
		return d, true
	case <-ctx.Done():
	case <-time.After(timeout):
	}
	if _, stillOurs := approvalRegistry.LoadAndDelete(runID); !stillOurs {
		// A sender claimed the channel concurrently with the timeout; its
		// buffered send is in flight — give it a moment to land.
		select {
		case d := <-ch:
			return d, true
		case <-time.After(5 * time.Second):
			return ApprovalDecision{}, false
		}
	}
	// We reclaimed the entry, so no future sender can appear. Drain in case a
	// send landed between the timeout firing and the reclaim.
	select {
	case d := <-ch:
		return d, true
	default:
		return ApprovalDecision{}, false
	}
}
