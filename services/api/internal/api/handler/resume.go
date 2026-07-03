package handler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
)

// ResumeApprovedRun continues a run that is parked in approval_wait with its
// loop state persisted in run_wait_states — either because the API process
// restarted while the run was waiting, or because the in-process wait timed
// out and the goroutine parked. It reclaims the snapshot (an atomic
// delete-returning, so two callers can't resume the same run twice) and
// re-enters the tool-calling loop headlessly at the pending call, applying
// the given decision.
//
// Returns false when no snapshot exists for the run — the caller should fall
// back to its no-active-goroutine handling.
func (h *InvokeHandler) ResumeApprovedRun(runID string, decision ApprovalDecision) (bool, error) {
	ctx := context.Background()
	st, err := claimWaitState(ctx, h.pool, runID, "approval")
	if err != nil {
		return false, err
	}
	if st == nil {
		return false, nil
	}
	if len(st.PendingCalls) == 0 || st.CallIndex < 0 || st.CallIndex >= len(st.PendingCalls) {
		h.runs.failRun(ctx, runID, "resume failed: corrupt wait state") //nolint:errcheck
		return true, fmt.Errorf("corrupt wait state for run %s", runID)
	}

	agents := repository.NewAgentRepository(h.pool)
	a, err := agents.Get(ctx, st.AgentID, st.WorkspaceID)
	if err != nil {
		h.runs.failRun(ctx, runID, "resume failed: agent no longer exists") //nolint:errcheck
		return true, fmt.Errorf("resume run %s: agent %s not found", runID, st.AgentID)
	}

	h.pool.Exec(ctx, `UPDATE runs SET status='running' WHERE id=$1::uuid`, runID) //nolint:errcheck
	slog.Info("resuming approval-parked run", "run_id", runID, "decision", decision.Decision,
		"tool", st.PendingCalls[st.CallIndex].Name)

	// Headless resume: no SSE client and no gateway channel survive a park, so
	// emit is nil and progress lands only in run_steps. Parked runs are always
	// top-level (snapshots are only written at invokeDepth 0).
	go h.executeRun(context.Background(), a, st.WorkspaceID, st.UserID, runID, st.ConversationID, st.Input,
		nil, nil, invokeOpts{resume: st, resumeDecision: &decision})
	return true, nil
}

// ResumeSessionRun continues a run parked in session_wait: the runner's
// completion callback arrived after the waiting goroutine was gone (process
// restart or in-process wait timeout). It reclaims the session snapshot and
// re-enters the tool-calling loop headlessly, injecting resultContent as the
// pending session tool's result and continuing after it.
//
// Returns false when no session snapshot exists for the run.
func (h *InvokeHandler) ResumeSessionRun(runID, resultContent string) (bool, error) {
	ctx := context.Background()
	st, err := claimWaitState(ctx, h.pool, runID, "session")
	if err != nil {
		return false, err
	}
	if st == nil {
		return false, nil
	}
	if len(st.PendingCalls) == 0 || st.CallIndex < 0 || st.CallIndex >= len(st.PendingCalls) {
		h.runs.failRun(ctx, runID, "resume failed: corrupt session wait state") //nolint:errcheck
		return true, fmt.Errorf("corrupt session wait state for run %s", runID)
	}

	agents := repository.NewAgentRepository(h.pool)
	a, err := agents.Get(ctx, st.AgentID, st.WorkspaceID)
	if err != nil {
		h.runs.failRun(ctx, runID, "resume failed: agent no longer exists") //nolint:errcheck
		return true, fmt.Errorf("resume run %s: agent %s not found", runID, st.AgentID)
	}

	h.pool.Exec(ctx, `UPDATE runs SET status='running' WHERE id=$1::uuid`, runID) //nolint:errcheck
	slog.Info("resuming session-parked run", "run_id", runID,
		"tool", st.PendingCalls[st.CallIndex].Name)

	go h.executeRun(context.Background(), a, st.WorkspaceID, st.UserID, runID, st.ConversationID, st.Input,
		nil, nil, invokeOpts{resume: st, resumeToolResult: &resultContent})
	return true, nil
}
