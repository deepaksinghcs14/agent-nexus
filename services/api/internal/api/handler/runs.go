package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/api/sse"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	agentprompt "github.com/deepaksingh/agent-nexus/services/api/internal/runtime/agent"
	contextretrieval "github.com/deepaksingh/agent-nexus/services/api/internal/runtime/context"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
)

type RunsHandler struct {
	pool          *pgxpool.Pool
	cfg           *config.Config
	conversations *repository.ConversationRepository
	registry      *tools.Registry
	executor      *tools.Executor
	invokeH       *InvokeHandler // set post-construction via SetInvokeHandler to avoid circular init
	// providerOverride, when non-nil, replaces credential-based provider
	// resolution. Set only by tests.
	providerOverride provider.Provider
}

func NewRunsHandler(p *pgxpool.Pool, c *config.Config, reg *tools.Registry, exec *tools.Executor) *RunsHandler {
	return &RunsHandler{pool: p, cfg: c, conversations: repository.NewConversationRepository(p), registry: reg, executor: exec}
}

// SetInvokeHandler wires the InvokeHandler so that native_call_agent works in conversation runs.
func (h *RunsHandler) SetInvokeHandler(ih *InvokeHandler) { h.invokeH = ih }

const runSelect = `SELECT id::text,workspace_id::text,COALESCE(agent_id::text,''),conversation_id::text,user_id::text,input,output,status,started_at,completed_at,total_input_tokens,total_output_tokens,cost_estimate,error_message,COALESCE(trigger_id::text,''),COALESCE(parent_run_id::text,''),workflow_node_id,COALESCE(trace_id::text,''),metadata FROM runs`

func scanRun(row interface{ Scan(...any) error }) (domain.Run, error) {
	var x domain.Run
	e := row.Scan(&x.ID, &x.WorkspaceID, &x.AgentID, &x.ConversationID, &x.UserID, &x.Input, &x.Output, &x.Status, &x.StartedAt, &x.CompletedAt, &x.TotalInputTokens, &x.TotalOutputTokens, &x.CostEstimate, &x.ErrorMessage, &x.TriggerID, &x.ParentRunID, &x.WorkflowNodeID, &x.TraceID, &x.Metadata)
	return x, e
}

func (h *RunsHandler) Start(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())

	c, e := h.conversations.Get(r.Context(), chi.URLParam(r, "id"), ws)
	if e != nil {
		errs.Write(w, errs.NotFound("conversation not found"))
		return
	}
	var q struct {
		Input string `json:"input"`
	}
	if json.NewDecoder(r.Body).Decode(&q) != nil || q.Input == "" {
		errs.Write(w, errs.BadRequest("input is required"))
		return
	}
	agents := repository.NewAgentRepository(h.pool)
	a, e := agents.Get(r.Context(), c.AgentID, ws)
	if e != nil {
		errs.Write(w, errs.NotFound("agent not found"))
		return
	}
	// Preflight the provider credential so a misconfigured workspace gets a
	// clean HTTP error before the SSE stream starts; executeRun re-resolves it.
	if _, e := h.providerFor(r.Context(), ws, a.Provider); e != nil {
		errs.Write(w, errs.Internal(e.Error()))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	em, ok := newSafeEmitter(w)
	if !ok {
		return
	}
	defer em.close()
	emit := em.emit
	sseErr := func(msg string) { emit(sse.Error(msg)) }

	if e := h.conversations.AddMessage(r.Context(), &domain.Message{ID: uuid.NewString(), ConversationID: c.ID, Role: "user", Content: q.Input}); e != nil {
		sseErr("failed to save user message")
		return
	}
	id := uuid.NewString()
	if _, e = h.pool.Exec(r.Context(), `INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status)VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,'running')`, id, ws, c.AgentID, c.ID, uid, q.Input); e != nil {
		sseErr("failed to create run")
		return
	}

	// Everything past run creation is the shared agent loop — executeRun owns
	// provider lookup, retrieval, history, prompt assembly, tool dispatch,
	// approval/session waits, status flips, cost accounting, and ephemeral
	// cleanup for depth-0 runs.
	if h.invokeH == nil {
		sseErr("agent execution is not available")
		return
	}
	h.invokeH.executeRun(r.Context(), a, ws, uid, id, c.ID, q.Input, nil, emit, invokeOpts{})
}

func (h *RunsHandler) providerFor(ctx context.Context, workspaceID, providerName string) (provider.Provider, error) {
	// Test seam: the loop tests inject a scripted provider here so executeRun
	// runs against real Postgres without a real LLM credential.
	if h.providerOverride != nil {
		return h.providerOverride, nil
	}
	providers := repository.NewProviderRepository(h.pool)
	cred, encKey, err := providers.GetActiveByProvider(ctx, workspaceID, providerName)
	if err != nil {
		return nil, fmt.Errorf("no active %s provider credential configured", providerName)
	}
	return providerFromCredential(ctx, h.cfg, providers, cred, encKey)
}

func (h *RunsHandler) createStep(ctx context.Context, runID string, stepType domain.StepType, input, output any, started time.Time, tokens int, toolName, errMsg string) error {
	in, _ := json.Marshal(input)
	out, _ := json.Marshal(output)
	_, err := h.pool.Exec(ctx,
		`INSERT INTO run_steps(id,run_id,step_type,input,output,latency_ms,tokens_used,tool_name,error)
		 VALUES($1::uuid,$2::uuid,$3,$4::jsonb,$5::jsonb,$6,$7,$8,$9)`,
		uuid.NewString(), runID, stepType, json.RawMessage(in), json.RawMessage(out), int(time.Since(started).Milliseconds()), tokens, toolName, errMsg)
	return err
}

func (h *RunsHandler) failRun(ctx context.Context, runID, message string) error {
	// Cancel is terminal: never resurrect a cancelled run with a later status write.
	_, err := h.pool.Exec(ctx, `UPDATE runs SET status='failed',completed_at=NOW(),error_message=$2 WHERE id=$1::uuid AND status<>'cancelled'`, runID, message)
	return err
}

type connectorSettings struct {
	IDs       []string
	MaxChunks int
	MinScore  float64
}

func (h *RunsHandler) agentConnectorSettings(ctx context.Context, agentID string) (connectorSettings, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT connector_id::text, max_chunks, min_score FROM agent_connectors WHERE agent_id=$1::uuid AND enabled=true ORDER BY created_at DESC`,
		agentID)
	if err != nil {
		return connectorSettings{}, err
	}
	defer rows.Close()
	s := connectorSettings{MaxChunks: 8, MinScore: 0.75}
	for rows.Next() {
		var id string
		var maxChunks int
		var minScore float64
		if err := rows.Scan(&id, &maxChunks, &minScore); err != nil {
			return connectorSettings{}, err
		}
		s.IDs = append(s.IDs, id)
		if maxChunks > s.MaxChunks {
			s.MaxChunks = maxChunks
		}
		if minScore < s.MinScore {
			s.MinScore = minScore
		}
	}
	return s, rows.Err()
}

func formatChunk(chunk contextretrieval.Chunk) agentprompt.ContextChunk {
	title := strings.TrimSpace(chunk.Title)
	var ref string
	if title != "" && chunk.URL != "" {
		ref = "[" + title + "](" + chunk.URL + ")"
	} else if chunk.URL != "" {
		ref = chunk.URL
	} else {
		ref = title
	}
	return agentprompt.ContextChunk{Ref: ref, Content: chunk.Content}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (h *RunsHandler) ListByConversation(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	conv := chi.URLParam(r, "id")
	rows, e := h.pool.Query(r.Context(), runSelect+` WHERE workspace_id=$1::uuid AND ($2='' OR conversation_id=$2::uuid) AND ($3='' OR agent_id=$3::uuid) AND ($4='' OR status=$4) AND ($5='' OR trigger_id=$5::uuid) ORDER BY started_at DESC`, middleware.WorkspaceIDFromCtx(r.Context()), conv, q.Get("agent_id"), q.Get("status"), q.Get("trigger_id"))
	if e != nil {
		errs.Write(w, errs.Internal("failed to list runs"))
		return
	}
	defer rows.Close()
	a := []domain.Run{}
	for rows.Next() {
		x, e := scanRun(rows)
		if e != nil {
			errs.Write(w, errs.Internal("failed to read runs"))
			return
		}
		a = append(a, x)
	}
	errs.WriteJSON(w, 200, map[string]any{"data": a})
}

// Stats returns per-agent token/cost/run aggregations for the workspace (root runs only).
func (h *RunsHandler) Stats(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	type agentStat struct {
		AgentID   string  `json:"agent_id"`
		AgentName string  `json:"agent_name"`
		Tokens    int64   `json:"tokens"`
		Cost      float64 `json:"cost"`
		Runs      int64   `json:"runs"`
	}
	rows, e := h.pool.Query(r.Context(), `
		SELECT COALESCE(r.agent_id::text,'') AS agent_id,
		       COALESCE(a.name,'Group runs') AS agent_name,
		       SUM(r.total_input_tokens+r.total_output_tokens) AS tokens,
		       SUM(r.cost_estimate) AS cost,
		       COUNT(*) AS runs
		FROM runs r
		LEFT JOIN agents a ON a.id=r.agent_id
		WHERE r.workspace_id=$1::uuid AND r.parent_run_id IS NULL
		GROUP BY r.agent_id, a.name
		ORDER BY tokens DESC`, ws)
	if e != nil {
		errs.Write(w, errs.Internal("failed to aggregate usage"))
		return
	}
	defer rows.Close()
	byAgent := []agentStat{}
	var totalTokens int64
	var totalCost float64
	var totalRuns int64
	for rows.Next() {
		var s agentStat
		if rows.Scan(&s.AgentID, &s.AgentName, &s.Tokens, &s.Cost, &s.Runs) != nil {
			continue
		}
		byAgent = append(byAgent, s)
		totalTokens += s.Tokens
		totalCost += s.Cost
		totalRuns += s.Runs
	}
	errs.WriteJSON(w, 200, map[string]any{
		"total_tokens": totalTokens,
		"total_cost":   totalCost,
		"total_runs":   totalRuns,
		"by_agent":     byAgent,
	})
}

// List returns root runs only (parent_run_id IS NULL), paginated by cursor.
// Pass ?before=<RFC3339 timestamp> to load the next page; returns has_more + next_cursor.
func (h *RunsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	before := q.Get("before")
	rows, e := h.pool.Query(r.Context(),
		runSelect+` WHERE workspace_id=$1::uuid AND parent_run_id IS NULL`+
			` AND ($2='' OR agent_id=$2::uuid)`+
			` AND ($3='' OR status=$3)`+
			` AND ($4='' OR trigger_id=$4::uuid)`+
			` AND ($5='' OR started_at < $5::timestamptz)`+
			` ORDER BY started_at DESC LIMIT 11`,
		ws, q.Get("agent_id"), q.Get("status"), q.Get("trigger_id"), before)
	if e != nil {
		errs.Write(w, errs.Internal("failed to list runs"))
		return
	}
	defer rows.Close()
	a := []domain.Run{}
	for rows.Next() {
		x, e := scanRun(rows)
		if e != nil {
			errs.Write(w, errs.Internal("failed to read runs"))
			return
		}
		a = append(a, x)
	}
	hasMore := len(a) == 11
	if hasMore {
		a = a[:10]
	}
	nextCursor := ""
	if hasMore && len(a) > 0 {
		nextCursor = a[len(a)-1].StartedAt.Format(time.RFC3339Nano)
	}
	errs.WriteJSON(w, 200, map[string]any{"data": a, "has_more": hasMore, "next_cursor": nextCursor})
}

// ListChildren returns all runs in a trace tree.
// For root runs (trace_id IS NULL) it queries by trace_id to get all descendants.
// For sub-runs it returns direct children.
func (h *RunsHandler) ListChildren(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	ws := middleware.WorkspaceIDFromCtx(r.Context())

	var traceIDVal string
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COALESCE(trace_id::text,'') FROM runs WHERE id=$1::uuid`, runID).Scan(&traceIDVal)

	var query string
	var args []any
	if traceIDVal == "" {
		query = runSelect + ` WHERE trace_id=$1::uuid AND workspace_id=$2::uuid ORDER BY started_at ASC`
		args = []any{runID, ws}
	} else {
		query = runSelect + ` WHERE parent_run_id=$1::uuid AND workspace_id=$2::uuid ORDER BY started_at ASC`
		args = []any{runID, ws}
	}
	rows, e := h.pool.Query(r.Context(), query, args...)
	if e != nil {
		errs.Write(w, errs.Internal("failed to list child runs"))
		return
	}
	defer rows.Close()
	a := []domain.Run{}
	for rows.Next() {
		x, e := scanRun(rows)
		if e != nil {
			errs.Write(w, errs.Internal("failed to read child runs"))
			return
		}
		a = append(a, x)
	}
	errs.WriteJSON(w, 200, map[string]any{"data": a})
}
func (h *RunsHandler) Get(w http.ResponseWriter, r *http.Request) {
	x, e := scanRun(h.pool.QueryRow(r.Context(), runSelect+` WHERE id=$1::uuid AND workspace_id=$2::uuid`, chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context())))
	if e != nil {
		errs.Write(w, errs.NotFound("run not found"))
		return
	}
	rows, _ := h.pool.Query(r.Context(), `SELECT id::text,run_id::text,step_type,input,output,latency_ms,tokens_used,tool_name,error,created_at FROM run_steps WHERE run_id=$1::uuid ORDER BY created_at`, x.ID)
	steps := []domain.RunStep{}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var s domain.RunStep
			if rows.Scan(&s.ID, &s.RunID, &s.StepType, &s.Input, &s.Output, &s.LatencyMs, &s.TokensUsed, &s.ToolName, &s.Error, &s.CreatedAt) == nil {
				steps = append(steps, s)
			}
		}
	}
	resp := map[string]any{"run": x, "steps": steps}
	// For workflow root runs (no agent, not a sub-run), include the workflow_id so the
	// frontend can fetch the workflow graph for the trace visualisation.
	if x.AgentID == "" && x.ParentRunID == "" && x.ConversationID != "" {
		var wfID string
		_ = h.pool.QueryRow(r.Context(),
			`SELECT COALESCE(workflow_id::text,'') FROM conversations WHERE id=$1::uuid`,
			x.ConversationID).Scan(&wfID)
		if wfID != "" {
			resp["workflow_id"] = wfID
		}
	}
	if domain.RunStatus(x.Status) == domain.RunStatusApprovalWait {
		var arID, toolName, arStatus string
		var toolInput json.RawMessage
		var arCreatedAt time.Time
		err := h.pool.QueryRow(r.Context(),
			`SELECT id::text, tool_name, tool_input, status, created_at FROM approval_requests WHERE run_id=$1::uuid AND status='pending' ORDER BY created_at LIMIT 1`,
			x.ID).Scan(&arID, &toolName, &toolInput, &arStatus, &arCreatedAt)
		if err == nil {
			resp["approval_request"] = map[string]any{
				"id":         arID,
				"tool_name":  toolName,
				"tool_input": toolInput,
				"status":     arStatus,
				"created_at": arCreatedAt,
			}
		}
	}
	errs.WriteJSON(w, 200, resp)
}
func (h *RunsHandler) Approve(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())

	var req struct {
		Decision string          `json:"decision"`
		Input    json.RawMessage `json:"input"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || (req.Decision != "approved" && req.Decision != "rejected") {
		errs.Write(w, errs.BadRequest("decision must be 'approved' or 'rejected'"))
		return
	}

	var status string
	if err := h.pool.QueryRow(r.Context(),
		`SELECT status FROM runs WHERE id=$1::uuid AND workspace_id=$2::uuid`, runID, ws).Scan(&status); err != nil {
		errs.Write(w, errs.NotFound("run not found"))
		return
	}
	if domain.RunStatus(status) != domain.RunStatusApprovalWait {
		errs.Write(w, errs.BadRequest("run is not waiting for approval"))
		return
	}

	var arID string
	if err := h.pool.QueryRow(r.Context(),
		`SELECT id::text FROM approval_requests WHERE run_id=$1::uuid AND status='pending' ORDER BY created_at LIMIT 1`,
		runID).Scan(&arID); err != nil {
		errs.Write(w, errs.NotFound("no pending approval request found"))
		return
	}

	inp := req.Input
	if len(inp) == 0 {
		inp = json.RawMessage(`{}`)
	}
	h.pool.Exec(r.Context(), //nolint:errcheck
		`UPDATE approval_requests SET status=$2, decided_by=$3::uuid, decided_at=NOW() WHERE id=$1::uuid`,
		arID, req.Decision, uid)

	decision := ApprovalDecision{Decision: req.Decision, Input: inp}
	if !SendApprovalDecision(runID, decision) {
		// No goroutine is waiting — the in-process wait parked, or the process
		// restarted since the run entered approval_wait. Resume the run from its
		// persisted wait state instead of failing it.
		resumed := false
		if h.invokeH != nil {
			var err error
			resumed, err = h.invokeH.ResumeApprovedRun(runID, decision)
			if err != nil {
				errs.Write(w, errs.Internal("failed to resume run: "+err.Error()))
				return
			}
		}
		if !resumed {
			h.pool.Exec(r.Context(), //nolint:errcheck
				`UPDATE runs SET status='failed', completed_at=NOW(), error_message='Approval received but run goroutine no longer active' WHERE id=$1::uuid`,
				runID)
		}
	}

	errs.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "decision": req.Decision})
}

func (h *RunsHandler) SubmitUserInput(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	ws := middleware.WorkspaceIDFromCtx(r.Context())

	var req struct {
		Answer string `json:"answer"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Answer == "" {
		errs.Write(w, errs.BadRequest("answer is required"))
		return
	}

	var status string
	if err := h.pool.QueryRow(r.Context(),
		`SELECT status FROM runs WHERE id=$1::uuid AND workspace_id=$2::uuid`, runID, ws).Scan(&status); err != nil {
		errs.Write(w, errs.NotFound("run not found"))
		return
	}
	if domain.RunStatus(status) != domain.RunStatusUserInputWait {
		errs.Write(w, errs.BadRequest("run is not waiting for user input"))
		return
	}
	if !SendUserInput(runID, req.Answer) {
		errs.Write(w, errs.BadRequest("run no longer active"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *RunsHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	t, e := h.pool.Exec(r.Context(), `UPDATE runs SET status='cancelled',completed_at=NOW() WHERE id=$1::uuid AND workspace_id=$2::uuid AND status IN('pending','running','approval_wait','session_wait','user_input_wait')`, runID, middleware.WorkspaceIDFromCtx(r.Context()))
	if e != nil {
		errs.Write(w, errs.Internal("failed to cancel run"))
		return
	}
	if t.RowsAffected() == 0 {
		errs.Write(w, errs.BadRequest("run cannot be cancelled"))
		return
	}
	// Status first: any terminal write racing this now sees 'cancelled' and
	// its status<>'cancelled' guard drops it. Then stop the in-process
	// goroutine (if any) and make a durable park unresumable. The
	// workspace_id check above is the only tenant guard on this run — nothing
	// here may run before RowsAffected confirms it passed.
	cancelRun(runID)
	h.cancelRunnerSession(r.Context(), runID)
	deleteWaitState(r.Context(), h.pool, runID)
	errs.WriteJSON(w, 200, map[string]any{"status": "cancelled"})
}

// cancelRunnerSession tells the runner service to stop the claude subprocess
// backing this run, if it launched one. Best-effort: the run is already
// committed to 'cancelled' by the time this is called, so a runner that's
// unreachable or slow must not block or fail the API-side cancel — the
// stale-session watchdog is the existing backstop for a runner that never
// hears from this call at all.
func (h *RunsHandler) cancelRunnerSession(ctx context.Context, runID string) {
	if h.cfg.RunnerURL == "" {
		return
	}
	var meta struct {
		RunnerSessionKey string `json:"runner_session_key"`
	}
	var raw []byte
	if err := h.pool.QueryRow(ctx, `SELECT metadata FROM runs WHERE id=$1::uuid`, runID).Scan(&raw); err != nil {
		return
	}
	if json.Unmarshal(raw, &meta) != nil || meta.RunnerSessionKey == "" {
		return
	}
	payload, _ := json.Marshal(map[string]string{"session_key": meta.RunnerSessionKey})
	cancelCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cancelCtx, http.MethodPost, strings.TrimRight(h.cfg.RunnerURL, "/")+"/sessions/cancel", bytes.NewReader(payload))
	if err != nil {
		slog.Warn("failed to build runner cancel request", "run_id", runID, "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Runner-Secret", h.cfg.RunnerCallbackSecret)
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		slog.Warn("runner cancel request failed; session may keep running until it times out", "run_id", runID, "session_key", meta.RunnerSessionKey, "error", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slog.Warn("runner rejected cancel request", "run_id", runID, "session_key", meta.RunnerSessionKey, "status", resp.StatusCode)
	}
}
