package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agentNexus/agent-nexus/services/api/internal/api/middleware"
	"github.com/agentNexus/agent-nexus/services/api/internal/config"
	"github.com/agentNexus/agent-nexus/services/api/internal/domain"
	"github.com/agentNexus/agent-nexus/services/api/internal/provider"
	"github.com/agentNexus/agent-nexus/services/api/internal/provider/anthropic"
	"github.com/agentNexus/agent-nexus/services/api/internal/provider/gemini"
	"github.com/agentNexus/agent-nexus/services/api/internal/provider/ollama"
	"github.com/agentNexus/agent-nexus/services/api/internal/provider/openai"
	"github.com/agentNexus/agent-nexus/services/api/internal/repository"
	agentprompt "github.com/agentNexus/agent-nexus/services/api/internal/runtime/agent"
	contextretrieval "github.com/agentNexus/agent-nexus/services/api/internal/runtime/context"
	"github.com/agentNexus/agent-nexus/services/api/internal/runtime/memory"
	"github.com/agentNexus/agent-nexus/services/api/internal/tools"
	"github.com/agentNexus/agent-nexus/services/api/pkg/encrypt"
	"github.com/agentNexus/agent-nexus/services/api/pkg/errs"
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
}

func NewRunsHandler(p *pgxpool.Pool, c *config.Config, reg *tools.Registry, exec *tools.Executor) *RunsHandler {
	return &RunsHandler{p, c, repository.NewConversationRepository(p), reg, exec}
}

const runSelect = `SELECT id::text,workspace_id::text,COALESCE(agent_id::text,''),conversation_id::text,user_id::text,input,output,status,started_at,completed_at,total_input_tokens,total_output_tokens,cost_estimate,error_message FROM runs`

func scanRun(row interface{ Scan(...any) error }) (domain.Run, error) {
	var x domain.Run
	e := row.Scan(&x.ID, &x.WorkspaceID, &x.AgentID, &x.ConversationID, &x.UserID, &x.Input, &x.Output, &x.Status, &x.StartedAt, &x.CompletedAt, &x.TotalInputTokens, &x.TotalOutputTokens, &x.CostEstimate, &x.ErrorMessage)
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
	llm, e := h.providerFor(r.Context(), ws, a.Provider)
	if e != nil {
		errs.Write(w, errs.Internal(e.Error()))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	f, ok := w.(http.Flusher)
	if !ok {
		return
	}
	emit := func(s string) { fmt.Fprintf(w, "data: %s\n\n", s); f.Flush() }
	sseErr := func(msg string) { emit(fmt.Sprintf(`{"type":"error","error":%q}`, msg)) }

	if e := h.conversations.AddMessage(r.Context(), &domain.Message{ID: uuid.NewString(), ConversationID: c.ID, Role: "user", Content: q.Input}); e != nil {
		sseErr("failed to save user message")
		return
	}
	id := uuid.NewString()
	if _, e = h.pool.Exec(r.Context(), `INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status)VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,'running')`, id, ws, c.AgentID, c.ID, uid, q.Input); e != nil {
		sseErr("failed to create run")
		return
	}

	memories, contextChunks := []string{}, []string{}
	if a.MemoryEnabled {
		start := time.Now()
		found, err := memory.NewEngine(h.pool).Retrieve(r.Context(), a.ID, ws, q.Input)
		if err == nil {
			for _, m := range found {
				memories = append(memories, m.Content)
			}
		}
		_ = h.createStep(r.Context(), id, domain.StepMemoryRetrieval, map[string]any{"query": q.Input}, map[string]any{"count": len(memories), "memories": memories}, start, 0, "", errString(err))
	}
	if a.ContextRetrievalEnabled {
		start := time.Now()
		connectorIDs, err := h.agentConnectorIDs(r.Context(), a.ID)
		var queryEmbedding []float32
		if err == nil {
			queryEmbedding, _ = llm.Embed(r.Context(), q.Input)
		}
		chunks := []contextretrieval.Chunk{}
		if err == nil {
			chunks, err = contextretrieval.NewRetriever(h.pool).Retrieve(r.Context(), ws, connectorIDs, queryEmbedding, 8)
		}
		if err == nil {
			for _, chunk := range chunks {
				contextChunks = append(contextChunks, formatChunk(chunk))
			}
		}
		_ = h.createStep(r.Context(), id, domain.StepContextRetrieval, map[string]any{"connector_ids": connectorIDs}, map[string]any{"count": len(contextChunks), "chunks": contextChunks}, start, 0, "", errString(err))
	}

	historyRows, e := h.conversations.ListMessages(r.Context(), c.ID)
	if e != nil {
		sseErr("failed to load conversation history")
		_ = h.failRun(r.Context(), id, e.Error())
		return
	}
	history := make([]provider.Message, 0, len(historyRows))
	for _, msg := range historyRows {
		if msg.Role == "user" || msg.Role == "assistant" || msg.Role == "tool" {
			history = append(history, provider.Message{Role: msg.Role, Content: msg.Content, ToolCallID: msg.ToolCallID, ToolName: msg.ToolName})
		}
	}
	prompt := agentprompt.NewBuilder().Build(agentprompt.BuildRequest{
		SystemInstructions: a.Instructions,
		MemorySummaries:    memories,
		ContextChunks:      contextChunks,
		History:            history,
	})

	// Load tool definitions for this agent
	toolDefs, dbTools, _ := loadAgentToolDefs(r.Context(), h.pool, a.ID)

	emit(fmt.Sprintf(`{"type":"run_started","run_id":%q}`, id))

	// ── Tool calling loop ──────────────────────────────────────────────────────
	messages := prompt
	stepCount := 0
	totalInput, totalOutput := 0, 0

	for {
		stream, e := llm.Complete(r.Context(), provider.CompletionRequest{
			Model:       a.Model,
			Messages:    messages,
			Tools:       toolDefs,
			Temperature: a.Temperature,
			MaxTokens:   a.MaxTokens,
			Stream:      true,
		})
		if e != nil {
			_ = h.failRun(r.Context(), id, e.Error())
			sseErr(e.Error())
			return
		}

		modelStart := time.Now()
		reply := ""
		usage := provider.Usage{}
		var pendingCalls []provider.ToolCall

		for event := range stream {
			switch event.Type {
			case provider.EventDelta:
				reply += event.Delta
				emit(fmt.Sprintf(`{"type":"delta","content":%q}`, event.Delta))
			case provider.EventToolCall:
				if event.ToolCall != nil {
					pendingCalls = append(pendingCalls, *event.ToolCall)
				}
			case provider.EventDone:
				if event.Usage != nil {
					usage = *event.Usage
				}
			case provider.EventError:
				msg := "model call failed"
				if event.Error != nil {
					msg = event.Error.Error()
				}
				_ = h.createStep(r.Context(), id, domain.StepError, map[string]any{"provider": a.Provider, "model": a.Model}, map[string]any{}, modelStart, 0, "", msg)
				_ = h.failRun(r.Context(), id, msg)
				sseErr(msg)
				return
			}
		}

		totalInput += usage.InputTokens
		totalOutput += usage.OutputTokens
		_ = h.createStep(r.Context(), id, domain.StepModelCall,
			map[string]any{"provider": a.Provider, "model": a.Model, "messages": len(messages)},
			map[string]any{"content": reply, "tool_calls": len(pendingCalls)},
			modelStart, usage.InputTokens+usage.OutputTokens, "", "")

		if len(pendingCalls) == 0 {
			// No tool calls — this is the final response
			if strings.TrimSpace(reply) == "" {
				msg := "model returned an empty response"
				_ = h.failRun(r.Context(), id, msg)
				sseErr(msg)
				return
			}
			if e := h.conversations.AddMessage(r.Context(), &domain.Message{ID: uuid.NewString(), ConversationID: c.ID, Role: "assistant", Content: reply, Tokens: usage.OutputTokens}); e != nil {
				_ = h.failRun(r.Context(), id, e.Error())
				sseErr("failed to save assistant message")
				return
			}
			if a.MemoryEnabled {
				mem := &domain.Memory{WorkspaceID: ws, UserID: uid, AgentID: a.ID, Scope: domain.MemoryScope(a.MemoryScope), Content: "User: " + q.Input + "\nAssistant: " + reply, SourceRunID: id}
				if a.MemoryScope == string(domain.MemoryScopeWorkspace) {
					mem.AgentID = ""
				}
				_ = memory.NewEngine(h.pool).Store(r.Context(), mem)
			}
			_ = h.createStep(r.Context(), id, domain.StepFinalResponse, map[string]any{}, map[string]any{"content": reply}, time.Now(), usage.OutputTokens, "", "")
			_, _ = h.pool.Exec(r.Context(), `UPDATE runs SET output=$2,status='success',completed_at=NOW(),total_input_tokens=$3,total_output_tokens=$4 WHERE id=$1::uuid`, id, reply, totalInput, totalOutput)
			emit(fmt.Sprintf(`{"type":"run_completed","run_id":%q,"usage":{"input":%d,"output":%d},"cost":0}`, id, totalInput, totalOutput))
			return
		}

		// Append assistant turn (with tool calls) to the message history
		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   reply,
			ToolCalls: pendingCalls,
		})

		for _, call := range pendingCalls {
			dbTool, toolExists := dbTools[call.Name]

			// Approval gate for high-risk tools
			if toolExists && dbTool.RequiresApproval {
				arID := uuid.NewString()
				h.pool.Exec(r.Context(), //nolint:errcheck
					`INSERT INTO approval_requests(id,run_id,tool_name,tool_input,status)VALUES($1::uuid,$2::uuid,$3,$4::jsonb,'pending')`,
					arID, id, call.Name, call.Input)
				h.pool.Exec(r.Context(), `UPDATE runs SET status='approval_wait' WHERE id=$1::uuid`, id) //nolint:errcheck

				ch := RegisterApprovalWait(id)
				emit(fmt.Sprintf(`{"type":"approval_required","tool":%q,"input":%s,"approval_id":%q}`,
					call.Name, string(call.Input), arID))

				var decision ApprovalDecision
				select {
				case decision = <-ch:
				case <-time.After(10 * time.Minute):
					UnregisterApprovalWait(id)
					_ = h.failRun(r.Context(), id, "approval timed out")
					sseErr("approval timed out after 10 minutes")
					return
				}
				h.pool.Exec(r.Context(), `UPDATE runs SET status='running' WHERE id=$1::uuid`, id) //nolint:errcheck

				if decision.Decision == "rejected" {
					messages = append(messages, provider.Message{Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: "Tool call rejected by user."})
					continue
				}
			}

			// Execute the tool — HTTP tools bypass the native registry
			var result *tools.ExecutionResult
			var execErr error
			if toolExists && dbTool.Type == "http" {
				var cfg tools.HTTPToolConfig
				_ = json.Unmarshal(dbTool.Config, &cfg)
				result = tools.ExecuteHTTP(r.Context(), cfg, call.Input, dbTool.TimeoutMs)
			} else {
				result, execErr = h.executor.Execute(r.Context(), call.Name, call.Input)
			}
			var resultContent, errMsg string
			latencyMs := 0
			if result != nil {
				latencyMs = result.LatencyMs
				if result.Error != "" {
					errMsg = result.Error
					resultContent = fmt.Sprintf(`{"error":%q}`, result.Error)
				} else {
					b, _ := json.Marshal(result.Output)
					resultContent = string(b)
				}
			} else if execErr != nil {
				errMsg = execErr.Error()
				resultContent = fmt.Sprintf(`{"error":%q}`, execErr.Error())
			}

			_ = h.createStep(r.Context(), id, domain.StepToolCall,
				map[string]any{"tool": call.Name, "input": call.Input},
				map[string]any{"output": resultContent},
				time.Now(), 0, call.Name, errMsg)

			emit(fmt.Sprintf(`{"type":"tool_call","tool":%q,"input":%s,"output":%s,"latency_ms":%d}`,
				call.Name, jsonOrStr(call.Input), jsonOrStr([]byte(resultContent)), latencyMs))

			messages = append(messages, provider.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Content:    resultContent,
			})

			stepCount++
			if stepCount > a.MaxSteps {
				_ = h.failRun(r.Context(), id, "max steps exceeded")
				sseErr("max steps exceeded")
				return
			}
		}
		// Loop: call model again with tool results in messages
	}
}

func jsonOrStr(b []byte) string {
	if json.Valid(b) {
		return string(b)
	}
	s, _ := json.Marshal(string(b))
	return string(s)
}

func (h *RunsHandler) providerFor(ctx context.Context, workspaceID, providerName string) (provider.Provider, error) {
	providers := repository.NewProviderRepository(h.pool)
	cred, encKey, err := providers.GetActiveByProvider(ctx, workspaceID, providerName)
	if err != nil {
		return nil, fmt.Errorf("no active %s provider credential configured", providerName)
	}
	if cred.AuthType == "oauth" && cred.Provider == "gemini" {
		accessToken, err := providers.GetDecryptedAccessToken(ctx, cred.ID, h.cfg.EncryptionKey, h.cfg.GoogleOAuthClientID, h.cfg.GoogleOAuthClientSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to get gemini oauth token: %w", err)
		}
		return gemini.New(accessToken, "oauth"), nil
	}
	apiKey, err := encrypt.Decrypt([]byte(h.cfg.EncryptionKey), encKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt provider credential")
	}
	switch cred.Provider {
	case "openai":
		return openai.New(apiKey, cred.BaseURL), nil
	case "ollama":
		return ollama.New(cred.BaseURL), nil
	case "anthropic":
		return anthropic.New(apiKey, cred.BaseURL), nil
	case "gemini":
		return gemini.New(apiKey, "api_key"), nil
	default:
		return nil, fmt.Errorf("provider %q is not supported", cred.Provider)
	}
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
	_, err := h.pool.Exec(ctx, `UPDATE runs SET status='failed',completed_at=NOW(),error_message=$2 WHERE id=$1::uuid`, runID, message)
	return err
}

func (h *RunsHandler) agentConnectorIDs(ctx context.Context, agentID string) ([]string, error) {
	rows, err := h.pool.Query(ctx, `SELECT connector_id::text FROM agent_connectors WHERE agent_id=$1::uuid AND enabled=true ORDER BY created_at DESC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func formatChunk(chunk contextretrieval.Chunk) string {
	prefix := strings.TrimSpace(chunk.Title)
	if chunk.URL != "" {
		prefix = strings.TrimSpace(prefix + " " + chunk.URL)
	}
	if prefix == "" {
		return chunk.Content
	}
	return prefix + ": " + chunk.Content
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (h *RunsHandler) list(w http.ResponseWriter, r *http.Request, conv string) {
	q := r.URL.Query()
	rows, e := h.pool.Query(r.Context(), runSelect+` WHERE workspace_id=$1::uuid AND ($2='' OR conversation_id=$2::uuid) AND ($3='' OR agent_id=$3::uuid) AND ($4='' OR status=$4) ORDER BY started_at DESC`, middleware.WorkspaceIDFromCtx(r.Context()), conv, q.Get("agent_id"), q.Get("status"))
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
func (h *RunsHandler) ListByConversation(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, chi.URLParam(r, "id"))
}
func (h *RunsHandler) List(w http.ResponseWriter, r *http.Request) { h.list(w, r, "") }
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

	if !SendApprovalDecision(runID, ApprovalDecision{Decision: req.Decision, Input: inp}) {
		h.pool.Exec(r.Context(), //nolint:errcheck
			`UPDATE runs SET status='failed', completed_at=NOW(), error_message='Approval received but run goroutine no longer active' WHERE id=$1::uuid`,
			runID)
	}

	errs.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "decision": req.Decision})
}
func (h *RunsHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	t, e := h.pool.Exec(r.Context(), `UPDATE runs SET status='cancelled',completed_at=NOW() WHERE id=$1::uuid AND workspace_id=$2::uuid AND status IN('pending','running','approval_wait')`, chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context()))
	if e != nil {
		errs.Write(w, errs.Internal("failed to cancel run"))
		return
	}
	if t.RowsAffected() == 0 {
		errs.Write(w, errs.BadRequest("run cannot be cancelled"))
		return
	}
	errs.WriteJSON(w, 200, map[string]any{"status": "cancelled"})
}
