package handler

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

// ── Session wait registry ─────────────────────────────────────────────────────
//
// Same shape as the approval registry: a run goroutine blocks on a channel
// keyed by run ID while an external repo session (runner service) executes.
// The channel carries the session result as a JSON string — exactly what gets
// appended as the tool result message.

var sessionRegistry sync.Map // map[runID string] → chan string

// RegisterSessionWait creates and stores a channel for the given run ID.
func RegisterSessionWait(runID string) chan string {
	ch := make(chan string, 1)
	sessionRegistry.Store(runID, ch)
	return ch
}

// SendSessionResult delivers a session result to the waiting run goroutine.
// The LoadAndDelete makes the claim exclusive, so the buffered send never blocks.
func SendSessionResult(runID, content string) bool {
	v, ok := sessionRegistry.LoadAndDelete(runID)
	if !ok {
		return false
	}
	v.(chan string) <- content
	return true
}

// awaitSessionResult blocks until a result arrives on ch or timeout elapses.
// Mirrors awaitApprovalDecision: returns (content, true) on delivery —
// including one that raced the timeout — or ("", false) on a clean timeout
// with the registry entry reclaimed, meaning the caller may safely park.
func awaitSessionResult(runID string, ch chan string, timeout time.Duration) (string, bool) {
	select {
	case c := <-ch:
		return c, true
	case <-time.After(timeout):
	}
	if _, stillOurs := sessionRegistry.LoadAndDelete(runID); !stillOurs {
		select {
		case c := <-ch:
			return c, true
		case <-time.After(5 * time.Second):
			return "", false
		}
	}
	select {
	case c := <-ch:
		return c, true
	default:
		return "", false
	}
}

// ── Session callback endpoint ─────────────────────────────────────────────────

// sessionCallbackPayload is what the runner service POSTs when a repo session
// finishes (in any terminal state).
type sessionCallbackPayload struct {
	RunID     string  `json:"run_id"`
	SessionID string  `json:"session_id"`
	Status    string  `json:"status"` // success | budget-exceeded | crashed
	Repo      string  `json:"repo"`
	TicketKey string  `json:"ticket_key"`
	Branch    string  `json:"branch"`
	Summary   string  `json:"summary"`
	CostUSD   float64 `json:"cost_usd"`
}

// SessionCallback handles POST /internal/sessions/callback from the runner
// service. It resolves the in-process wait channel when the run goroutine is
// alive, otherwise claims the persisted session wait state and resumes the
// run headlessly with the session result injected as the pending tool result.
func (h *InvokeHandler) SessionCallback(w http.ResponseWriter, r *http.Request) {
	if h.cfg.RunnerCallbackSecret == "" {
		errs.Write(w, errs.NotFound("session callbacks are not configured"))
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Runner-Secret")), []byte(h.cfg.RunnerCallbackSecret)) != 1 {
		errs.Write(w, errs.Unauthorized("invalid runner secret"))
		return
	}

	var p sessionCallbackPayload
	if json.NewDecoder(r.Body).Decode(&p) != nil || p.RunID == "" || p.Status == "" {
		errs.Write(w, errs.BadRequest("run_id and status are required"))
		return
	}
	switch p.Status {
	case "success", "budget-exceeded", "crashed":
	default:
		errs.Write(w, errs.BadRequest("status must be success, budget-exceeded, or crashed"))
		return
	}

	content, _ := json.Marshal(map[string]any{
		"status":     p.Status,
		"repo":       p.Repo,
		"ticket_key": p.TicketKey,
		"branch":     p.Branch,
		"summary":    p.Summary,
		"cost_usd":   p.CostUSD,
	})

	if SendSessionResult(p.RunID, string(content)) {
		errs.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "delivered": "channel"})
		return
	}

	resumed, err := h.ResumeSessionRun(p.RunID, string(content))
	if err != nil {
		errs.Write(w, errs.Internal("failed to resume run: "+err.Error()))
		return
	}
	if resumed {
		errs.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "delivered": "resume"})
		return
	}

	// No goroutine and no wait state: the run already got this result (runner
	// retry), or it was cancelled/failed. Report the current status and accept
	// the callback so the runner stops retrying.
	var status string
	_ = h.pool.QueryRow(r.Context(), `SELECT status FROM runs WHERE id=$1::uuid`, p.RunID).Scan(&status)
	slog.Info("session callback had no waiting run", "run_id", p.RunID, "run_status", status, "session_status", p.Status)
	errs.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "delivered": "ignored", "run_status": status})
}
