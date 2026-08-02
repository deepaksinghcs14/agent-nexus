package handler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/sse"

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

// awaitSessionResult blocks until a result arrives on ch, ctx is cancelled,
// or timeout elapses. Mirrors awaitApprovalDecision: returns (content, true)
// on delivery — including one that raced the cancel/timeout — or ("", false)
// otherwise, with the registry entry reclaimed, meaning the caller may
// safely park.
func awaitSessionResult(ctx context.Context, runID string, ch chan string, timeout time.Duration) (string, bool) {
	select {
	case c := <-ch:
		return c, true
	case <-ctx.Done():
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
	Mode      string  `json:"mode"`   // "" (code) | review
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

	// A successful review verdict is repo memory: persist its findings so
	// future coding sessions in this repo are warned about them. (A runner
	// redelivery after an ambiguous failure may write the same block twice —
	// harmless, the cap trims it.)
	if p.Mode == "review" && p.Status == "success" && p.Repo != "" {
		recordRepoLessons(r.Context(), h.pool, p.RunID, p.Repo, p.TicketKey, p.Summary)
	}

	// A coding session asked (in the launch prompt) to emit a repo map when
	// the cache was missing or stale is repo memory too: cache it so the next
	// session in this repo skips re-exploring the same structure.
	if p.Mode == "" && p.Status == "success" && p.Repo != "" {
		recordRepoMap(r.Context(), h.pool, p.RunID, p.Repo, p.Summary)
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

// sessionProgressPayload is what the runner POSTs periodically (debounced,
// at most once every few seconds) while a session is in flight — a coarse
// "still alive and roughly where it is" signal, not a transcript.
type sessionProgressPayload struct {
	RunID   string `json:"run_id"`
	Summary string `json:"summary"`
}

// SessionProgress handles POST /internal/sessions/progress from the runner
// service. Best-effort and independent of the run's wait/resume state — a
// progress update that arrives for a run that already finished, parked, or
// was cancelled is simply a no-op write, not an error, since the runner has
// no way to know the run's current status without asking.
func (h *InvokeHandler) SessionProgress(w http.ResponseWriter, r *http.Request) {
	if h.cfg.RunnerCallbackSecret == "" {
		errs.Write(w, errs.NotFound("session callbacks are not configured"))
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Runner-Secret")), []byte(h.cfg.RunnerCallbackSecret)) != 1 {
		errs.Write(w, errs.Unauthorized("invalid runner secret"))
		return
	}

	var p sessionProgressPayload
	if json.NewDecoder(r.Body).Decode(&p) != nil || p.RunID == "" || p.Summary == "" {
		errs.Write(w, errs.BadRequest("run_id and summary are required"))
		return
	}

	meta, _ := json.Marshal(map[string]any{"session_progress": p.Summary, "session_progress_at": time.Now().UTC()})
	if _, err := h.pool.Exec(r.Context(),
		`UPDATE runs SET metadata = metadata || $2::jsonb WHERE id=$1::uuid AND status<>'cancelled'`,
		p.RunID, meta); err != nil {
		slog.Warn("failed to persist session progress", "run_id", p.RunID, "error", err)
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// blockingSessionWait returns a WaitForSession implementation that blocks
// in-process until the runner callback arrives (no durable park). Used for
// child runs — workflow nodes, supervisor delegates, and sub-agents — whose
// parent's control flow lives on this process's stack: parking would return
// control to the parent as if the child had finished. The block is bounded by
// SESSION_WAIT_TIMEOUT_MIN, the same ceiling the watchdog uses.
func (h *InvokeHandler) blockingSessionWait(runID string, emitFn func(string)) func(context.Context, string) (string, error) {
	return func(waitCtx context.Context, sessionKey string) (string, error) {
		timeout := time.Duration(h.cfg.SessionWaitTimeoutMin) * time.Minute
		if timeout <= 0 {
			timeout = 240 * time.Minute
		}
		ch := RegisterSessionWait(runID)
		h.pool.Exec(waitCtx, `UPDATE runs SET status='session_wait' WHERE id=$1::uuid`, runID) //nolint:errcheck
		if emitFn != nil {
			emitFn(sse.SessionWait(runID, sessionKey))
		}
		content, got := awaitSessionResult(waitCtx, runID, ch, timeout)
		// Cancel is terminal: don't flip a cancelled run back to running.
		h.pool.Exec(context.Background(), `UPDATE runs SET status='running' WHERE id=$1::uuid AND status<>'cancelled'`, runID) //nolint:errcheck
		if !got {
			return "", fmt.Errorf("session %s did not complete within %s", sessionKey, timeout)
		}
		return content, nil
	}
}

// ── stale session watchdog ────────────────────────────────────────────────────

// StartSessionWaitWatchdog periodically resumes runs stuck in session_wait
// whose completion callback never arrived — the backstop for a runner that
// crashed AND lost its journal (volume wiped), or callbacks that could never
// be delivered. The timeout must comfortably exceed the runner's own session
// cap so healthy long sessions are never killed; the runner's journal
// recovery normally reports crashes long before this fires.
func (h *InvokeHandler) StartSessionWaitWatchdog(ctx context.Context) {
	timeout := time.Duration(h.cfg.SessionWaitTimeoutMin) * time.Minute
	if timeout <= 0 {
		timeout = 240 * time.Minute
	}
	// Sweep immediately: runs that went stale while the API was down should
	// not wait another tick.
	h.sweepStaleSessionWaits(ctx, timeout)
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.sweepStaleSessionWaits(ctx, timeout)
		}
	}
}

func (h *InvokeHandler) sweepStaleSessionWaits(ctx context.Context, timeout time.Duration) {
	rows, err := h.pool.Query(ctx, `
		SELECT w.run_id::text, w.created_at
		FROM run_wait_states w JOIN runs r ON r.id = w.run_id
		WHERE w.wait_type='session' AND r.status='session_wait' AND w.created_at < NOW() - $1::interval`,
		timeout.String())
	if err != nil {
		slog.Warn("session watchdog query failed", "error", err)
		return
	}
	type stale struct {
		runID     string
		createdAt time.Time
	}
	var stales []stale
	for rows.Next() {
		var s stale
		if rows.Scan(&s.runID, &s.createdAt) == nil {
			stales = append(stales, s)
		}
	}
	rows.Close()

	for _, s := range stales {
		content, _ := json.Marshal(map[string]any{
			"status": "crashed",
			"summary": fmt.Sprintf(
				"no completion callback received within %s (session started waiting %s) — the runner likely died mid-session and lost its state",
				timeout, s.createdAt.UTC().Format(time.RFC3339)),
		})
		resumed, err := h.ResumeSessionRun(s.runID, string(content))
		if err != nil {
			slog.Warn("session watchdog resume failed", "run_id", s.runID, "error", err)
			continue
		}
		if resumed {
			slog.Warn("session watchdog resumed stale run as crashed",
				"run_id", s.runID, "waiting_since", s.createdAt)
		}
	}
}
