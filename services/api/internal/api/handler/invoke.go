package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	agentprompt "github.com/deepaksingh/agent-nexus/services/api/internal/runtime/agent"
	contextretrieval "github.com/deepaksingh/agent-nexus/services/api/internal/runtime/context"
	"github.com/deepaksingh/agent-nexus/services/api/internal/runtime/memory"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

type InvokeHandler struct {
	pool     *pgxpool.Pool
	cfg      *config.Config
	runs     *RunsHandler
	registry *tools.Registry
	executor *tools.Executor
}

func NewInvokeHandler(pool *pgxpool.Pool, cfg *config.Config, runs *RunsHandler, reg *tools.Registry, exec *tools.Executor) *InvokeHandler {
	return &InvokeHandler{pool: pool, cfg: cfg, runs: runs, registry: reg, executor: exec}
}

// Agent handles POST /api/v1/invoke/agents/:agentId
// Supports streaming (SSE) and non-streaming (polling) modes.
func (h *InvokeHandler) Agent(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())
	agentID := chi.URLParam(r, "agentId")

	var req struct {
		Input          string `json:"input"`
		ConversationID string `json:"conversation_id"`
		Stream         bool   `json:"stream"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Input == "" {
		errs.Write(w, errs.BadRequest("input is required"))
		return
	}

	agents := repository.NewAgentRepository(h.pool)
	a, err := agents.Get(r.Context(), agentID, ws)
	if err != nil {
		errs.Write(w, errs.NotFound("agent not found"))
		return
	}

	convID, err := h.ensureConversation(r.Context(), req.ConversationID, ws, uid, a.ID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to prepare conversation"))
		return
	}

	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO messages(id,conversation_id,role,content) VALUES($1::uuid,$2::uuid,'user',$3)`,
		uuid.NewString(), convID, req.Input); err != nil {
		errs.Write(w, errs.Internal("failed to save message"))
		return
	}

	runID := uuid.NewString()
	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,'running')`,
		runID, ws, a.ID, convID, uid, req.Input); err != nil {
		errs.Write(w, errs.Internal("failed to create run"))
		return
	}

	if req.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		f, ok := w.(http.Flusher)
		if !ok {
			return
		}
		emit := func(s string) { fmt.Fprintf(w, "data: %s\n\n", s); f.Flush() }
		h.executeRun(r.Context(), a, ws, uid, runID, convID, req.Input, nil, emit)
		return
	}

	// Non-streaming: return run_id immediately and execute in background
	errs.WriteJSON(w, http.StatusAccepted, map[string]any{
		"run_id":          runID,
		"conversation_id": convID,
		"status":          "running",
	})

	go h.executeRun(context.Background(), a, ws, uid, runID, convID, req.Input, nil, nil)
}

// Group handles POST /api/v1/invoke/workflows/:workflowId
func (h *InvokeHandler) Group(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())
	groupID := chi.URLParam(r, "workflowId")

	var req struct {
		Input  string `json:"input"`
		Stream bool   `json:"stream"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Input == "" {
		errs.Write(w, errs.BadRequest("input is required"))
		return
	}

	// Verify group exists in this workspace
	var gName, gMode string
	err := h.pool.QueryRow(r.Context(),
		`SELECT name, mode FROM workflows WHERE id=$1::uuid AND workspace_id=$2::uuid AND status='active'`,
		groupID, ws).Scan(&gName, &gMode)
	if err != nil {
		errs.Write(w, errs.NotFound("workflow not found"))
		return
	}

	// Create a conversation and top-level run record scoped to the workflow
	runID := uuid.NewString()
	convID := uuid.NewString()
	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO conversations(id,workspace_id,user_id,title) VALUES($1::uuid,$2::uuid,$3::uuid,$4)`,
		convID, ws, uid, "Workflow: "+gName); err != nil {
		errs.Write(w, errs.Internal("failed to create conversation"))
		return
	}
	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO messages(id,conversation_id,role,content) VALUES($1::uuid,$2::uuid,'user',$3)`,
		uuid.NewString(), convID, req.Input); err != nil {
		errs.Write(w, errs.Internal("failed to save message"))
		return
	}
	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO runs(id,workspace_id,conversation_id,user_id,input,status) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,'running')`,
		runID, ws, convID, uid, req.Input); err != nil {
		errs.Write(w, errs.Internal("failed to create run"))
		return
	}

	if req.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		f, ok := w.(http.Flusher)
		if !ok {
			return
		}
		emit := func(s string) { fmt.Fprintf(w, "data: %s\n\n", s); f.Flush() }
		h.executeGroupRun(r.Context(), groupID, ws, uid, runID, convID, req.Input, emit)
		return
	}

	errs.WriteJSON(w, http.StatusAccepted, map[string]any{
		"run_id":          runID,
		"conversation_id": convID,
		"group_id":        groupID,
		"group_name":      gName,
		"mode":            gMode,
		"status":          "running",
	})

	go h.executeGroupRun(context.Background(), groupID, ws, uid, runID, convID, req.Input, nil)
}

// ensureConversation returns an existing conversation by ID or creates a new one.
func (h *InvokeHandler) ensureConversation(ctx context.Context, convID, ws, uid, agentID string) (string, error) {
	if convID != "" {
		var exists bool
		_ = h.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM conversations WHERE id=$1::uuid AND workspace_id=$2::uuid)`,
			convID, ws).Scan(&exists)
		if !exists {
			return "", fmt.Errorf("conversation not found")
		}
		return convID, nil
	}
	id := uuid.NewString()
	_, err := h.pool.Exec(ctx,
		`INSERT INTO conversations(id,workspace_id,agent_id,user_id,title) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'API Invocation')`,
		id, ws, agentID, uid)
	return id, err
}

// executeRun runs the full agent loop with tool calling. When emit is nil (background mode),
// progress is only written to the database. When emit is non-nil, SSE events are also sent.
// delegateHandlers, if non-nil, maps tool names to functions that execute a delegate agent and
// return its output — used by supervisor nodes to call team agents as tools.
func (h *InvokeHandler) executeRun(ctx context.Context, a *domain.Agent, ws, uid, runID, convID, input string, delegateHandlers map[string]func(context.Context, json.RawMessage) string, emit func(string)) {
	// dbCtx is used for all DB status writes (failRun, final UPDATE).
	// It must NOT be the request context because the request context can be
	// cancelled on client disconnect, which would silently leave the run in
	// 'running' state. A short-lived background context ensures the write
	// always completes even after the HTTP connection closes.
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer dbCancel()

	runCompleted := false
	defer func() {
		if !runCompleted {
			h.runs.failRun(dbCtx, runID, "run terminated unexpectedly") //nolint:errcheck
		}
	}()

	sseEmitOrNil := func(s string) {
		if emit != nil {
			emit(s)
		}
	}
	sseErr := func(msg string) { sseEmitOrNil(fmt.Sprintf(`{"type":"error","error":%q}`, msg)) }

	memories, contextChunks := []string{}, []string{}
	if a.MemoryEnabled {
		start := time.Now()
		found, err := memory.NewEngine(h.pool).Retrieve(ctx, a.ID, ws, input)
		if err == nil {
			for _, m := range found {
				memories = append(memories, m.Content)
			}
		}
		h.runs.createStep(ctx, runID, domain.StepMemoryRetrieval, //nolint:errcheck
			map[string]any{"query": input},
			map[string]any{"count": len(memories)},
			start, 0, "", errString(err))
	}

	llm, err := h.runs.providerFor(ctx, ws, a.Provider)
	if err != nil {
		sseErr(err.Error())
		h.runs.failRun(dbCtx, runID, err.Error()) //nolint:errcheck
		return
	}

	if a.ContextRetrievalEnabled {
		start := time.Now()
		connIDs, err := h.runs.agentConnectorIDs(ctx, a.ID)
		var queryEmbedding []float32
		if err == nil {
			queryEmbedding, _ = llm.Embed(ctx, input)
		}
		chunks := []contextretrieval.Chunk{}
		if err == nil {
			chunks, err = contextretrieval.NewRetriever(h.pool).Retrieve(ctx, ws, connIDs, queryEmbedding, 8)
		}
		if err == nil {
			for _, c := range chunks {
				contextChunks = append(contextChunks, formatChunk(c))
			}
		}
		h.runs.createStep(ctx, runID, domain.StepContextRetrieval, //nolint:errcheck
			map[string]any{"connector_ids": connIDs},
			map[string]any{"count": len(contextChunks)},
			start, 0, "", errString(err))
	}

	historyRows, err := h.pool.Query(ctx,
		`SELECT role, content, COALESCE(tool_call_id,''), COALESCE(tool_name,'') FROM messages WHERE conversation_id=$1::uuid ORDER BY created_at`,
		convID)
	if err != nil {
		sseErr("failed to load conversation history")
		h.runs.failRun(dbCtx, runID, err.Error()) //nolint:errcheck
		return
	}
	defer historyRows.Close()
	history := []provider.Message{}
	for historyRows.Next() {
		var role, content, toolCallID, toolName string
		if historyRows.Scan(&role, &content, &toolCallID, &toolName) == nil && (role == "user" || role == "assistant" || role == "tool") {
			history = append(history, provider.Message{Role: role, Content: content, ToolCallID: toolCallID, ToolName: toolName})
		}
	}

	prompt := agentprompt.NewBuilder().Build(agentprompt.BuildRequest{
		SystemInstructions: a.Instructions,
		MemorySummaries:    memories,
		ContextChunks:      contextChunks,
		History:            history,
	})

	toolDefs, dbTools, _ := loadAgentToolDefs(ctx, h.pool, a.ID)

	sseEmitOrNil(fmt.Sprintf(`{"type":"run_started","run_id":%q}`, runID))

	// ── Tool calling loop ──────────────────────────────────────────────────────
	messages := prompt
	stepCount := 0
	totalInput, totalOutput := 0, 0

	for {
		stream, err := llm.Complete(ctx, provider.CompletionRequest{
			Model:       a.Model,
			Messages:    messages,
			Tools:       toolDefs,
			Temperature: a.Temperature,
			MaxTokens:   a.MaxTokens,
			Stream:      true,
		})
		if err != nil {
			h.runs.failRun(dbCtx, runID, err.Error()) //nolint:errcheck
			sseErr(err.Error())
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
				sseEmitOrNil(fmt.Sprintf(`{"type":"delta","content":%q}`, event.Delta))
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
				h.runs.createStep(ctx, runID, domain.StepError, //nolint:errcheck
					map[string]any{"provider": a.Provider, "model": a.Model},
					map[string]any{}, modelStart, 0, "", msg)
				h.runs.failRun(dbCtx, runID, msg) //nolint:errcheck
				sseErr(msg)
				return
			}
		}

		totalInput += usage.InputTokens
		totalOutput += usage.OutputTokens
		h.runs.createStep(ctx, runID, domain.StepModelCall, //nolint:errcheck
			map[string]any{"provider": a.Provider, "model": a.Model, "messages": len(messages)},
			map[string]any{"content": reply, "tool_calls": len(pendingCalls)},
			modelStart, usage.InputTokens+usage.OutputTokens, "", "")

		if len(pendingCalls) == 0 {
			if strings.TrimSpace(reply) == "" {
				msg := "model returned an empty response"
				h.runs.failRun(dbCtx, runID, msg) //nolint:errcheck
				sseErr(msg)
				return
			}
			h.pool.Exec(ctx, //nolint:errcheck
				`INSERT INTO messages(id,conversation_id,role,content,tokens) VALUES($1::uuid,$2::uuid,'assistant',$3,$4)`,
				uuid.NewString(), convID, reply, usage.OutputTokens)
			if a.MemoryEnabled {
				mem := &domain.Memory{
					WorkspaceID: ws, UserID: uid, AgentID: a.ID,
					Scope:       domain.MemoryScope(a.MemoryScope),
					Content:     "User: " + input + "\nAssistant: " + reply,
					SourceRunID: runID,
				}
				if a.MemoryScope == string(domain.MemoryScopeWorkspace) {
					mem.AgentID = ""
				}
				memory.NewEngine(h.pool).Store(ctx, mem) //nolint:errcheck
			}
			h.runs.createStep(ctx, runID, domain.StepFinalResponse, //nolint:errcheck
				map[string]any{},
				map[string]any{"content": reply},
				time.Now(), usage.OutputTokens, "", "")
			runCompleted = true
			h.pool.Exec(dbCtx, //nolint:errcheck
				`UPDATE runs SET output=$2,status='success',completed_at=NOW(),total_input_tokens=$3,total_output_tokens=$4 WHERE id=$1::uuid`,
				runID, reply, totalInput, totalOutput)
			sseEmitOrNil(fmt.Sprintf(`{"type":"run_completed","run_id":%q,"usage":{"input":%d,"output":%d},"cost":0}`,
				runID, totalInput, totalOutput))
			return
		}

		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   reply,
			ToolCalls: pendingCalls,
		})

		for _, call := range pendingCalls {
			// Delegate tool — hand off to a team agent and return its output.
			if handler, isDelegate := delegateHandlers[call.Name]; isDelegate {
				delegateStart := time.Now()
				delegateOutput := handler(ctx, call.Input)
				h.runs.createStep(ctx, runID, domain.StepToolCall, //nolint:errcheck
					map[string]any{"tool": call.Name, "input": call.Input},
					map[string]any{"output": delegateOutput},
					delegateStart, 0, call.Name, "")
				sseEmitOrNil(fmt.Sprintf(`{"type":"tool_call","tool":%q,"input":%s,"output":%s,"latency_ms":%d}`,
					call.Name, jsonOrStr(call.Input), jsonOrStr([]byte(delegateOutput)),
					int(time.Since(delegateStart).Milliseconds())))
				messages = append(messages, provider.Message{
					Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: delegateOutput,
				})
				stepCount++
				if stepCount > a.MaxSteps {
					h.runs.failRun(dbCtx, runID, "max steps exceeded") //nolint:errcheck
					sseErr("max steps exceeded")
					return
				}
				continue
			}

			dbTool, toolExists := dbTools[call.Name]

			if toolExists && dbTool.RequiresApproval {
				arID := uuid.NewString()
				h.pool.Exec(ctx, //nolint:errcheck
					`INSERT INTO approval_requests(id,run_id,tool_name,tool_input,status)VALUES($1::uuid,$2::uuid,$3,$4::jsonb,'pending')`,
					arID, runID, call.Name, call.Input)
				h.pool.Exec(ctx, `UPDATE runs SET status='approval_wait' WHERE id=$1::uuid`, runID) //nolint:errcheck

				ch := RegisterApprovalWait(runID)
				sseEmitOrNil(fmt.Sprintf(`{"type":"approval_required","tool":%q,"input":%s,"approval_id":%q}`,
					call.Name, string(call.Input), arID))

				var decision ApprovalDecision
				select {
				case decision = <-ch:
				case <-time.After(10 * time.Minute):
					UnregisterApprovalWait(runID)
					h.runs.failRun(dbCtx, runID, "approval timed out") //nolint:errcheck
					sseErr("approval timed out after 10 minutes")
					return
				}
				h.pool.Exec(ctx, `UPDATE runs SET status='running' WHERE id=$1::uuid`, runID) //nolint:errcheck

				if decision.Decision == "rejected" {
					messages = append(messages, provider.Message{Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: "Tool call rejected by user."})
					continue
				}
			}

			var result *tools.ExecutionResult
			var execErr error
			if toolExists && dbTool.Type == "http" {
				var cfg tools.HTTPToolConfig
				_ = json.Unmarshal(dbTool.Config, &cfg)
				result = tools.ExecuteHTTP(ctx, cfg, call.Input, dbTool.TimeoutMs)
			} else {
				result, execErr = h.executor.Execute(ctx, call.Name, call.Input)
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

			h.runs.createStep(ctx, runID, domain.StepToolCall, //nolint:errcheck
				map[string]any{"tool": call.Name, "input": call.Input},
				map[string]any{"output": resultContent},
				time.Now(), 0, call.Name, errMsg)

			sseEmitOrNil(fmt.Sprintf(`{"type":"tool_call","tool":%q,"input":%s,"output":%s,"latency_ms":%d}`,
				call.Name, jsonOrStr(call.Input), jsonOrStr([]byte(resultContent)), latencyMs))

			messages = append(messages, provider.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Content:    resultContent,
			})

			stepCount++
			if stepCount > a.MaxSteps {
				h.runs.failRun(dbCtx, runID, "max steps exceeded") //nolint:errcheck
				sseErr("max steps exceeded")
				return
			}
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Workflow graph execution
// ─────────────────────────────────────────────────────────────────────────────

// wfNode holds the data loaded from the DB for one workflow node.
type wfNode struct {
	ID      string
	Type    string // start | agent | condition | parallel | join | loop
	AgentID string
	Config  map[string]any
	rawCfg  json.RawMessage // kept for config parsing
}

// wfEdge holds the data loaded from the DB for one workflow edge.
type wfEdge struct {
	ID     string
	Source string
	Target string
	Label  string
}

// executeGroupRun walks the workflow graph starting from the start node and
// runs each agent node in sequence, branching on condition / parallel nodes.
// SSE events are emitted via emit (may be nil for background runs).
func (h *InvokeHandler) executeGroupRun(
	ctx context.Context,
	workflowID, ws, uid, parentRunID, convID, input string,
	emit func(string),
) {
	var emitMu sync.Mutex
	sseEmit := func(s string) {
		if emit != nil {
			emitMu.Lock()
			emit(s)
			emitMu.Unlock()
		}
	}

	// Ensure the run is always marked terminal, even on unexpected exit / panic.
	runMarked := false
	defer func() {
		if r := recover(); r != nil {
			sseEmit(fmt.Sprintf(`{"type":"error","error":"workflow panic: %v"}`, r))
		}
		if !runMarked {
			h.runs.failRun(context.Background(), parentRunID, "workflow terminated unexpectedly") //nolint:errcheck
		}
	}()

	// Keepalive: send a ping every 15 s so proxies and browsers don't close the
	// SSE connection during long-running or parallel agent executions.
	if emit != nil {
		keepaliveDone := make(chan struct{})
		defer close(keepaliveDone)
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					sseEmit(`{"type":"ping"}`)
				case <-keepaliveDone:
					return
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// ── 1. Load graph from DB ────────────────────────────────────────────────

	nodeRows, err := h.pool.Query(ctx,
		`SELECT id::text, node_type, COALESCE(agent_id::text,''), config::text
		 FROM workflow_nodes WHERE workflow_id=$1::uuid ORDER BY created_at`,
		workflowID)
	if err != nil {
		sseEmit(fmt.Sprintf(`{"type":"error","error":%q}`, "failed to load workflow nodes"))
		h.runs.failRun(ctx, parentRunID, "failed to load workflow nodes") //nolint:errcheck
		return
	}
	defer nodeRows.Close()

	nodeMap := map[string]*wfNode{}
	for nodeRows.Next() {
		var n wfNode
		var cfgStr string
		if err := nodeRows.Scan(&n.ID, &n.Type, &n.AgentID, &cfgStr); err != nil {
			continue
		}
		n.rawCfg = json.RawMessage(cfgStr)
		_ = json.Unmarshal([]byte(cfgStr), &n.Config)
		if n.Config == nil {
			n.Config = map[string]any{}
		}
		nodeMap[n.ID] = &n
	}

	edgeRows, err := h.pool.Query(ctx,
		`SELECT id::text, source_node_id::text, target_node_id::text, COALESCE(label,'')
		 FROM workflow_edges WHERE workflow_id=$1::uuid ORDER BY created_at`,
		workflowID)
	if err != nil {
		sseEmit(fmt.Sprintf(`{"type":"error","error":%q}`, "failed to load workflow edges"))
		h.runs.failRun(ctx, parentRunID, "failed to load workflow edges") //nolint:errcheck
		return
	}
	defer edgeRows.Close()

	// adjacency: sourceNodeID → []wfEdge
	adj := map[string][]wfEdge{}
	// inDegree: nodeID → count of incoming edges
	inDegree := map[string]int{}
	for _, n := range nodeMap {
		inDegree[n.ID] = 0
	}
	for edgeRows.Next() {
		var e wfEdge
		if err := edgeRows.Scan(&e.ID, &e.Source, &e.Target, &e.Label); err != nil {
			continue
		}
		adj[e.Source] = append(adj[e.Source], e)
		inDegree[e.Target]++
	}

	// ── 2. Find start node ───────────────────────────────────────────────────

	var startNode *wfNode
	for _, n := range nodeMap {
		if n.Type == "start" {
			startNode = n
			break
		}
	}
	if startNode == nil {
		// Fall back: first node with no incoming edges
		for id, deg := range inDegree {
			if deg == 0 {
				startNode = nodeMap[id]
				break
			}
		}
	}
	if startNode == nil {
		sseEmit(`{"type":"error","error":"workflow has no start node"}`)
		h.runs.failRun(ctx, parentRunID, "workflow has no start node") //nolint:errcheck
		return
	}

	// ── 3. Walk the graph ────────────────────────────────────────────────────

	nodeOutputs := map[string]string{} // nodeID → last output
	loopIterations := map[string]int{} // nodeID → iteration count
	originalInput := input
	var outputMu sync.Mutex // guards nodeOutputs for concurrent parallel branches

	// walkBranch performs a BFS walk from the given start node.
	// stopAt is a set of node IDs where this branch must stop without processing
	// (used by parallel branches to hand off join nodes to the parent).
	var walkBranch func(start *wfNode, branchInput string, stopAt map[string]bool)

	totalSteps := 0
	const maxTotalSteps = 100 // global circuit breaker across all nodes and re-entries
	var stepsMu sync.Mutex    // guards totalSteps from parallel branch races

	walkBranch = func(start *wfNode, branchInput string, stopAt map[string]bool) {
		queue := []*wfNode{start}
		lastOutput := branchInput
		prevNodeName := ""

		for len(queue) > 0 {
			// Stop if the request was cancelled (client disconnect / timeout).
			select {
			case <-ctx.Done():
				return
			default:
			}

			stepsMu.Lock()
			totalSteps++
			exceeded := totalSteps > maxTotalSteps
			stepsMu.Unlock()
			if exceeded {
				sseEmit(fmt.Sprintf(`{"type":"error","error":"workflow exceeded maximum steps (%d) — possible infinite loop"}`, maxTotalSteps))
				h.runs.failRun(ctx, parentRunID, fmt.Sprintf("workflow exceeded maximum steps (%d)", maxTotalSteps)) //nolint:errcheck
				return
			}

			node := queue[0]
			queue = queue[1:]

			// Parallel branches stop here — the join node is enqueued by the parent.
			if stopAt[node.ID] {
				return
			}

			nodeName := node.Type
			if node.AgentID != "" {
				// Try to fetch the agent name for richer events
				var aName string
				if err := h.pool.QueryRow(ctx,
					`SELECT name FROM agents WHERE id=$1::uuid AND workspace_id=$2::uuid`,
					node.AgentID, ws).Scan(&aName); err == nil {
					nodeName = aName
				}
			}

			sseEmit(fmt.Sprintf(`{"type":"node_started","node_id":%q,"node_type":%q,"node_name":%q}`,
				node.ID, node.Type, nodeName))

			switch node.Type {

			case "start":
				nodeOutputs[node.ID] = branchInput
				lastOutput = branchInput
				for _, e := range adj[node.ID] {
					if next, ok := nodeMap[e.Target]; ok {
						queue = append(queue, next)
					}
				}

			case "end":
				// Terminal node — capture last output and stop this branch.
				nodeOutputs[node.ID] = lastOutput

			case "agent":
				if node.AgentID == "" {
					sseEmit(fmt.Sprintf(`{"type":"error","error":"agent node %s has no agent_id"}`, node.ID))
					continue
				}

				agents := repository.NewAgentRepository(h.pool)
				a, err := agents.Get(ctx, node.AgentID, ws)
				if err != nil {
					sseEmit(fmt.Sprintf(`{"type":"error","error":%q}`,
						fmt.Sprintf("agent node %s: agent not found", node.ID)))
					continue
				}

				// Build input: for non-first nodes, prepend context from previous output
				agentInput := lastOutput
				if prevNodeName != "" {
					agentInput = fmt.Sprintf("[Task]\n%s\n\n[Previous output from %s]\n%s",
						originalInput, prevNodeName, lastOutput)
				}

				// Create a sub-conversation and sub-run for this agent node
				subConvID := uuid.NewString()
				subRunID := uuid.NewString()
				h.pool.Exec(ctx, //nolint:errcheck
					`INSERT INTO conversations(id,workspace_id,agent_id,user_id,title)
					 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5)`,
					subConvID, ws, a.ID, uid, "Node: "+nodeName)
				h.pool.Exec(ctx, //nolint:errcheck
					`INSERT INTO messages(id,conversation_id,role,content)
					 VALUES($1::uuid,$2::uuid,'user',$3)`,
					uuid.NewString(), subConvID, agentInput)
				h.pool.Exec(ctx, //nolint:errcheck
					`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status,parent_run_id,workflow_node_id)
					 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,'running',$7::uuid,$8)`,
					subRunID, ws, a.ID, subConvID, uid, agentInput, parentRunID, node.ID)

				// Wrap emit so every SSE event from this node carries node_id/node_name.
				// Route through sseEmit (which holds emitMu) — parallel branch
				// goroutines call agentEmit concurrently; calling the raw emit
				// func without the mutex causes a data race on ResponseWriter.
				capturedNodeID := node.ID
				capturedNodeName := nodeName
				agentEmit := func(line string) {
					var m map[string]any
					if json.Unmarshal([]byte(line), &m) == nil {
						m["node_id"] = capturedNodeID
						m["node_name"] = capturedNodeName
						if b, err := json.Marshal(m); err == nil {
							sseEmit(string(b))
							return
						}
					}
					sseEmit(line)
				}

				h.executeRun(ctx, a, ws, uid, subRunID, subConvID, agentInput, nil, agentEmit)

				// Read back the sub-run output using a background context — the
				// request ctx may be cancelled if the SSE client disconnected, but
				// we still need the output for downstream nodes.
				var subOutput string
				_ = h.pool.QueryRow(context.Background(),
					`SELECT COALESCE(output,'') FROM runs WHERE id=$1::uuid`, subRunID).Scan(&subOutput)

				outputMu.Lock()
				nodeOutputs[node.ID] = subOutput
				outputMu.Unlock()
				lastOutput = subOutput
				prevNodeName = nodeName

				for _, e := range adj[node.ID] {
					if next, ok := nodeMap[e.Target]; ok {
						queue = append(queue, next)
					}
				}

			case "condition":
				expression := ""
				if v, ok := node.Config["expression"].(string); ok {
					expression = v
				}
				matched := ""
				nextID := ""

				for _, e := range adj[node.ID] {
					if e.Label == "*" || e.Label == "" {
						// default / fallthrough edge
						if matched == "" {
							matched = e.Label
							nextID = e.Target
						}
						continue
					}
					if evaluateExpression(expression, lastOutput) == (e.Label == "yes" || e.Label == "true") {
						matched = e.Label
						nextID = e.Target
						break
					}
				}
				// If no edge was matched by the expression, fall through to the
				// "no" edge (or the wildcard).
				if nextID == "" {
					for _, e := range adj[node.ID] {
						if e.Label == "no" || e.Label == "false" || e.Label == "" || e.Label == "*" {
							nextID = e.Target
							matched = e.Label
							break
						}
					}
				}

				sseEmit(fmt.Sprintf(`{"type":"node_routed","node_id":%q,"result":%q,"next_node_id":%q}`,
					node.ID, matched, nextID))

				if next, ok := nodeMap[nextID]; ok {
					queue = append(queue, next)
				}

			case "parallel":
				// Fan out: each outgoing edge becomes an independent branch.
				//
				// Find join nodes that terminate this parallel section via BFS
				// from each branch start. Branches stop when they hit a join
				// so the join is processed exactly once by the parent branch.
				edges := adj[node.ID]
				stopNodeIDs := map[string]bool{}
				for _, e := range edges {
					bfsVisited := map[string]bool{e.Target: true}
					bfsQ := []string{e.Target}
					for len(bfsQ) > 0 {
						cur := bfsQ[0]
						bfsQ = bfsQ[1:]
						if n, ok := nodeMap[cur]; ok && n.Type == "join" {
							stopNodeIDs[cur] = true
							continue // don't walk past the join node
						}
						for _, ne := range adj[cur] {
							if !bfsVisited[ne.Target] {
								bfsVisited[ne.Target] = true
								bfsQ = append(bfsQ, ne.Target)
							}
						}
					}
				}

				var wg sync.WaitGroup
				for _, e := range edges {
					next, ok := nodeMap[e.Target]
					if !ok {
						continue
					}
					wg.Add(1)
					go func(branchStart *wfNode, bi string) {
						defer wg.Done()
						defer func() {
							if r := recover(); r != nil {
								sseEmit(fmt.Sprintf(`{"type":"error","error":"parallel branch panic: %v"}`, r))
							}
						}()
						walkBranch(branchStart, bi, stopNodeIDs)
					}(next, lastOutput)
				}
				wg.Wait()

				// Enqueue the join node(s) to be processed in order by this branch.
				for joinID := range stopNodeIDs {
					if joinNode, ok := nodeMap[joinID]; ok {
						queue = append(queue, joinNode)
					}
				}

			case "join":
				// Collect all incoming branch outputs and concatenate.
				// Safe to read nodeOutputs here because parallel branches have
				// already stopped (wg.Wait() in the parallel case) before this
				// node is enqueued. Lock anyway for correctness.
				outputMu.Lock()
				combined := []string{}
				for _, e := range edgesTargeting(node.ID, adj) {
					if out, ok := nodeOutputs[e.Source]; ok {
						combined = append(combined, out)
					}
				}
				if len(combined) > 0 {
					lastOutput = strings.Join(combined, "\n---\n")
				}
				nodeOutputs[node.ID] = lastOutput
				outputMu.Unlock()
				for _, e := range adj[node.ID] {
					if next, ok := nodeMap[e.Target]; ok {
						queue = append(queue, next)
					}
				}

			case "loop":
				exitCond := ""
				if v, ok := node.Config["exit_condition"].(string); ok {
					exitCond = v
				}
				maxIter := 5
				if v, ok := node.Config["max_iterations"].(float64); ok {
					maxIter = int(v)
				}
				loopIterations[node.ID]++

				condMet := evaluateExpression(exitCond, lastOutput)
				done := loopIterations[node.ID] >= maxIter || condMet
				sseEmit(fmt.Sprintf(`{"type":"node_routed","node_id":%q,"result":%q,"iteration":%d,"max":%d}`,
					node.ID, map[bool]string{true: "exit", false: "continue"}[done], loopIterations[node.ID], maxIter))
				if done {
					// Reset the counter so this loop can be re-entered later (e.g.
					// when a condition node's "no" branch circles back through it).
					loopIterations[node.ID] = 0
					// Forward: enqueue successors labeled "exit" or any non-loop edge.
					for _, e := range adj[node.ID] {
						if e.Label == "loop" {
							continue // skip the loop-back edge
						}
						if next, ok := nodeMap[e.Target]; ok {
							queue = append(queue, next)
						}
					}
				} else {
					// Re-enqueue the loop-back target (edge labeled "loop").
					for _, e := range adj[node.ID] {
						if e.Label == "loop" {
							if next, ok := nodeMap[e.Target]; ok {
								queue = append(queue, next)
							}
							break
						}
					}
				}

			case "supervisor":
				if node.AgentID == "" {
					sseEmit(fmt.Sprintf(`{"type":"error","error":"supervisor node %s has no agent_id"}`, node.ID))
					continue
				}
				agentRepo := repository.NewAgentRepository(h.pool)
				supAgent, err := agentRepo.Get(ctx, node.AgentID, ws)
				if err != nil {
					sseEmit(fmt.Sprintf(`{"type":"error","error":%q}`,
						fmt.Sprintf("supervisor node %s: agent not found", node.ID)))
					continue
				}

				// Build delegate tool defs + handlers from "delegate"-labelled edges.
				delegateToolDefs := []provider.ToolDefinition{}
				delegateHandlers := map[string]func(context.Context, json.RawMessage) string{}

				for _, e := range adj[node.ID] {
					if e.Label != "delegate" {
						continue
					}
					dn, ok := nodeMap[e.Target]
					if !ok || dn.AgentID == "" {
						continue
					}
					da, err := agentRepo.Get(ctx, dn.AgentID, ws)
					if err != nil {
						continue
					}
					toolName := "delegate_" + sanitizeToolName(da.Name)
					desc := da.Instructions
					if idx := strings.Index(desc, "\n"); idx > 0 {
						desc = desc[:idx]
					}
					if strings.TrimSpace(desc) == "" {
						desc = "Delegate a task to the " + da.Name + " agent and get its response."
					}
					capturedDA := da
					capturedDN := dn
					capturedSuperNodeID := node.ID
					capturedSuperNodeName := nodeName
					toolNameCopy := toolName
					delegateToolDefs = append(delegateToolDefs, provider.ToolDefinition{
						Name:        toolNameCopy,
						Description: desc,
						InputSchema: json.RawMessage(`{"type":"object","properties":{"task":{"type":"string","description":"The task or question to send to ` + da.Name + `"}},"required":["task"]}`),
					})
					delegateHandlers[toolNameCopy] = func(callCtx context.Context, rawInput json.RawMessage) string {
						var args struct {
							Task string `json:"task"`
						}
						task := lastOutput
						if json.Unmarshal(rawInput, &args) == nil && strings.TrimSpace(args.Task) != "" {
							task = args.Task
						}
						dSubConvID := uuid.NewString()
						dSubRunID := uuid.NewString()
						h.pool.Exec(callCtx, //nolint:errcheck
							`INSERT INTO conversations(id,workspace_id,agent_id,user_id,title) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5)`,
							dSubConvID, ws, capturedDA.ID, uid, "Delegate: "+capturedDA.Name)
						h.pool.Exec(callCtx, //nolint:errcheck
							`INSERT INTO messages(id,conversation_id,role,content) VALUES($1::uuid,$2::uuid,'user',$3)`,
							uuid.NewString(), dSubConvID, task)
						h.pool.Exec(callCtx, //nolint:errcheck
							`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status,parent_run_id,workflow_node_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,'running',$7::uuid,$8)`,
							dSubRunID, ws, capturedDA.ID, dSubConvID, uid, task, parentRunID, capturedDN.ID)
						delEmit := func(line string) {
							var m map[string]any
							if json.Unmarshal([]byte(line), &m) == nil {
								m["node_id"] = capturedSuperNodeID
								m["node_name"] = capturedSuperNodeName
								if b, e2 := json.Marshal(m); e2 == nil {
									sseEmit(string(b))
									return
								}
							}
							sseEmit(line)
						}
						h.executeRun(callCtx, capturedDA, ws, uid, dSubRunID, dSubConvID, task, nil, delEmit)
						var out string
						_ = h.pool.QueryRow(context.Background(),
							`SELECT COALESCE(output,'') FROM runs WHERE id=$1::uuid`, dSubRunID).Scan(&out)
						return out
					}
				}

				// Sub-conversation + sub-run for the supervisor itself.
				subConvID := uuid.NewString()
				subRunID := uuid.NewString()
				agentInput := lastOutput
				if prevNodeName != "" {
					agentInput = fmt.Sprintf("[Task]\n%s\n\n[Previous output from %s]\n%s",
						originalInput, prevNodeName, lastOutput)
				}
				h.pool.Exec(ctx, //nolint:errcheck
					`INSERT INTO conversations(id,workspace_id,agent_id,user_id,title) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5)`,
					subConvID, ws, supAgent.ID, uid, "Supervisor: "+nodeName)
				h.pool.Exec(ctx, //nolint:errcheck
					`INSERT INTO messages(id,conversation_id,role,content) VALUES($1::uuid,$2::uuid,'user',$3)`,
					uuid.NewString(), subConvID, agentInput)
				h.pool.Exec(ctx, //nolint:errcheck
					`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status,parent_run_id,workflow_node_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,'running',$7::uuid,$8)`,
					subRunID, ws, supAgent.ID, subConvID, uid, agentInput, parentRunID, node.ID)

				capturedNodeID := node.ID
				capturedNodeName := nodeName
				supEmit := func(line string) {
					var m map[string]any
					if json.Unmarshal([]byte(line), &m) == nil {
						m["node_id"] = capturedNodeID
						m["node_name"] = capturedNodeName
						if b, e2 := json.Marshal(m); e2 == nil {
							sseEmit(string(b))
							return
						}
					}
					sseEmit(line)
				}

				// executeSupervisorRun merges delegate tool defs into the agent's tool
				// list so the LLM can call team agents by name, and intercepts those
				// calls via delegateHandlers to execute the real agent sub-runs.
				h.executeSupervisorRun(ctx, supAgent, ws, uid, subRunID, subConvID, agentInput, delegateToolDefs, delegateHandlers, supEmit)

				var subOutput string
				_ = h.pool.QueryRow(context.Background(),
					`SELECT COALESCE(output,'') FROM runs WHERE id=$1::uuid`, subRunID).Scan(&subOutput)

				outputMu.Lock()
				nodeOutputs[node.ID] = subOutput
				outputMu.Unlock()
				lastOutput = subOutput
				prevNodeName = nodeName

				// Only non-delegate edges feed downstream nodes.
				for _, e := range adj[node.ID] {
					if e.Label != "delegate" {
						if next, ok := nodeMap[e.Target]; ok {
							queue = append(queue, next)
						}
					}
				}

			} // end switch

			sseEmit(fmt.Sprintf(`{"type":"node_completed","node_id":%q,"node_name":%q}`,
				node.ID, nodeName))
		}
	}

	walkBranch(startNode, input, nil)

	// Mark the parent run as successful
	finalOutput := input
	for _, n := range nodeMap {
		if len(adj[n.ID]) == 0 && n.Type != "start" {
			if out, ok := nodeOutputs[n.ID]; ok && out != "" {
				finalOutput = out
				break
			}
		}
	}
	runMarked = true
	h.pool.Exec(ctx, //nolint:errcheck
		`UPDATE runs SET output=$2,status='success',completed_at=NOW() WHERE id=$1::uuid`,
		parentRunID, finalOutput)

	sseEmit(fmt.Sprintf(`{"type":"run_completed","run_id":%q}`, parentRunID))
}

// edgesTargeting returns all edges in adj whose target is targetNodeID.
// Used by the join node to collect outputs from all incoming branches.
func edgesTargeting(targetNodeID string, adj map[string][]wfEdge) []wfEdge {
	var result []wfEdge
	for _, edges := range adj {
		for _, e := range edges {
			if e.Target == targetNodeID {
				result = append(result, e)
			}
		}
	}
	return result
}

// executeSupervisorRun is like executeRun but also presents delegate tool definitions
// to the LLM so the supervisor agent can call team agents by name.
// delegateToolDefs are added to the agent's normal tool list before the first LLM call.
// delegateHandlers intercept those tool calls and execute the corresponding agent.
func (h *InvokeHandler) executeSupervisorRun(
	ctx context.Context,
	a *domain.Agent,
	ws, uid, runID, convID, input string,
	delegateToolDefs []provider.ToolDefinition,
	delegateHandlers map[string]func(context.Context, json.RawMessage) string,
	emit func(string),
) {
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer dbCancel()

	runCompleted := false
	defer func() {
		if !runCompleted {
			h.runs.failRun(dbCtx, runID, "supervisor run terminated unexpectedly") //nolint:errcheck
		}
	}()

	sseEmitOrNil := func(s string) {
		if emit != nil {
			emit(s)
		}
	}
	sseErr := func(msg string) { sseEmitOrNil(fmt.Sprintf(`{"type":"error","error":%q}`, msg)) }

	memories, contextChunks := []string{}, []string{}
	if a.MemoryEnabled {
		start := time.Now()
		found, err := memory.NewEngine(h.pool).Retrieve(ctx, a.ID, ws, input)
		if err == nil {
			for _, m := range found {
				memories = append(memories, m.Content)
			}
		}
		h.runs.createStep(ctx, runID, domain.StepMemoryRetrieval, //nolint:errcheck
			map[string]any{"query": input},
			map[string]any{"count": len(memories)},
			start, 0, "", errString(err))
	}

	llm, err := h.runs.providerFor(ctx, ws, a.Provider)
	if err != nil {
		sseErr(err.Error())
		h.runs.failRun(dbCtx, runID, err.Error()) //nolint:errcheck
		return
	}

	if a.ContextRetrievalEnabled {
		start := time.Now()
		connIDs, err := h.runs.agentConnectorIDs(ctx, a.ID)
		var queryEmbedding []float32
		if err == nil {
			queryEmbedding, _ = llm.Embed(ctx, input)
		}
		chunks := []contextretrieval.Chunk{}
		if err == nil {
			chunks, err = contextretrieval.NewRetriever(h.pool).Retrieve(ctx, ws, connIDs, queryEmbedding, 8)
		}
		if err == nil {
			for _, c := range chunks {
				contextChunks = append(contextChunks, formatChunk(c))
			}
		}
		h.runs.createStep(ctx, runID, domain.StepContextRetrieval, //nolint:errcheck
			map[string]any{"connector_ids": connIDs},
			map[string]any{"count": len(contextChunks)},
			start, 0, "", errString(err))
	}

	historyRows, err := h.pool.Query(ctx,
		`SELECT role, content, COALESCE(tool_call_id,''), COALESCE(tool_name,'') FROM messages WHERE conversation_id=$1::uuid ORDER BY created_at`,
		convID)
	if err != nil {
		sseErr("failed to load conversation history")
		h.runs.failRun(dbCtx, runID, err.Error()) //nolint:errcheck
		return
	}
	defer historyRows.Close()
	history := []provider.Message{}
	for historyRows.Next() {
		var role, content, toolCallID, toolName string
		if historyRows.Scan(&role, &content, &toolCallID, &toolName) == nil && (role == "user" || role == "assistant" || role == "tool") {
			history = append(history, provider.Message{Role: role, Content: content, ToolCallID: toolCallID, ToolName: toolName})
		}
	}

	prompt := agentprompt.NewBuilder().Build(agentprompt.BuildRequest{
		SystemInstructions: a.Instructions,
		MemorySummaries:    memories,
		ContextChunks:      contextChunks,
		History:            history,
	})

	// Regular agent tools + delegate tools merged into one list.
	regularToolDefs, dbTools, _ := loadAgentToolDefs(ctx, h.pool, a.ID)
	toolDefs := append(regularToolDefs, delegateToolDefs...)

	sseEmitOrNil(fmt.Sprintf(`{"type":"run_started","run_id":%q}`, runID))

	messages := prompt
	stepCount := 0
	totalInput, totalOutput := 0, 0

	for {
		stream, err := llm.Complete(ctx, provider.CompletionRequest{
			Model:       a.Model,
			Messages:    messages,
			Tools:       toolDefs,
			Temperature: a.Temperature,
			MaxTokens:   a.MaxTokens,
			Stream:      true,
		})
		if err != nil {
			h.runs.failRun(dbCtx, runID, err.Error()) //nolint:errcheck
			sseErr(err.Error())
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
				sseEmitOrNil(fmt.Sprintf(`{"type":"delta","content":%q}`, event.Delta))
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
				h.runs.createStep(ctx, runID, domain.StepError, //nolint:errcheck
					map[string]any{"provider": a.Provider, "model": a.Model},
					map[string]any{}, modelStart, 0, "", msg)
				h.runs.failRun(dbCtx, runID, msg) //nolint:errcheck
				sseErr(msg)
				return
			}
		}

		totalInput += usage.InputTokens
		totalOutput += usage.OutputTokens
		h.runs.createStep(ctx, runID, domain.StepModelCall, //nolint:errcheck
			map[string]any{"provider": a.Provider, "model": a.Model, "messages": len(messages)},
			map[string]any{"content": reply, "tool_calls": len(pendingCalls)},
			modelStart, usage.InputTokens+usage.OutputTokens, "", "")

		if len(pendingCalls) == 0 {
			if strings.TrimSpace(reply) == "" {
				msg := "model returned an empty response"
				h.runs.failRun(dbCtx, runID, msg) //nolint:errcheck
				sseErr(msg)
				return
			}
			h.pool.Exec(ctx, //nolint:errcheck
				`INSERT INTO messages(id,conversation_id,role,content,tokens) VALUES($1::uuid,$2::uuid,'assistant',$3,$4)`,
				uuid.NewString(), convID, reply, usage.OutputTokens)
			if a.MemoryEnabled {
				mem := &domain.Memory{
					WorkspaceID: ws, UserID: uid, AgentID: a.ID,
					Scope:       domain.MemoryScope(a.MemoryScope),
					Content:     "User: " + input + "\nAssistant: " + reply,
					SourceRunID: runID,
				}
				if a.MemoryScope == string(domain.MemoryScopeWorkspace) {
					mem.AgentID = ""
				}
				memory.NewEngine(h.pool).Store(ctx, mem) //nolint:errcheck
			}
			h.runs.createStep(ctx, runID, domain.StepFinalResponse, //nolint:errcheck
				map[string]any{},
				map[string]any{"content": reply},
				time.Now(), usage.OutputTokens, "", "")
			runCompleted = true
			h.pool.Exec(dbCtx, //nolint:errcheck
				`UPDATE runs SET output=$2,status='success',completed_at=NOW(),total_input_tokens=$3,total_output_tokens=$4 WHERE id=$1::uuid`,
				runID, reply, totalInput, totalOutput)
			sseEmitOrNil(fmt.Sprintf(`{"type":"run_completed","run_id":%q,"usage":{"input":%d,"output":%d},"cost":0}`,
				runID, totalInput, totalOutput))
			return
		}

		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   reply,
			ToolCalls: pendingCalls,
		})

		for _, call := range pendingCalls {
			// Delegate tool — execute the team agent and return its output.
			if handler, isDelegate := delegateHandlers[call.Name]; isDelegate {
				delegateStart := time.Now()
				delegateOutput := handler(ctx, call.Input)
				h.runs.createStep(ctx, runID, domain.StepToolCall, //nolint:errcheck
					map[string]any{"tool": call.Name, "input": call.Input},
					map[string]any{"output": delegateOutput},
					delegateStart, 0, call.Name, "")
				sseEmitOrNil(fmt.Sprintf(`{"type":"tool_call","tool":%q,"input":%s,"output":%s,"latency_ms":%d}`,
					call.Name, jsonOrStr(call.Input), jsonOrStr([]byte(delegateOutput)),
					int(time.Since(delegateStart).Milliseconds())))
				messages = append(messages, provider.Message{
					Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: delegateOutput,
				})
				stepCount++
				if stepCount > a.MaxSteps {
					h.runs.failRun(dbCtx, runID, "max steps exceeded") //nolint:errcheck
					sseErr("max steps exceeded")
					return
				}
				continue
			}

			dbTool, toolExists := dbTools[call.Name]

			if toolExists && dbTool.RequiresApproval {
				arID := uuid.NewString()
				h.pool.Exec(ctx, //nolint:errcheck
					`INSERT INTO approval_requests(id,run_id,tool_name,tool_input,status)VALUES($1::uuid,$2::uuid,$3,$4::jsonb,'pending')`,
					arID, runID, call.Name, call.Input)
				h.pool.Exec(ctx, `UPDATE runs SET status='approval_wait' WHERE id=$1::uuid`, runID) //nolint:errcheck

				ch := RegisterApprovalWait(runID)
				sseEmitOrNil(fmt.Sprintf(`{"type":"approval_required","tool":%q,"input":%s,"approval_id":%q}`,
					call.Name, string(call.Input), arID))

				var decision ApprovalDecision
				select {
				case decision = <-ch:
				case <-time.After(10 * time.Minute):
					UnregisterApprovalWait(runID)
					h.runs.failRun(dbCtx, runID, "approval timed out") //nolint:errcheck
					sseErr("approval timed out after 10 minutes")
					return
				}
				h.pool.Exec(ctx, `UPDATE runs SET status='running' WHERE id=$1::uuid`, runID) //nolint:errcheck

				if decision.Decision == "rejected" {
					messages = append(messages, provider.Message{Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: "Tool call rejected by user."})
					continue
				}
			}

			var result *tools.ExecutionResult
			var execErr error
			if toolExists && dbTool.Type == "http" {
				var cfg tools.HTTPToolConfig
				_ = json.Unmarshal(dbTool.Config, &cfg)
				result = tools.ExecuteHTTP(ctx, cfg, call.Input, dbTool.TimeoutMs)
			} else {
				result, execErr = h.executor.Execute(ctx, call.Name, call.Input)
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

			h.runs.createStep(ctx, runID, domain.StepToolCall, //nolint:errcheck
				map[string]any{"tool": call.Name, "input": call.Input},
				map[string]any{"output": resultContent},
				time.Now(), 0, call.Name, errMsg)

			sseEmitOrNil(fmt.Sprintf(`{"type":"tool_call","tool":%q,"input":%s,"output":%s,"latency_ms":%d}`,
				call.Name, jsonOrStr(call.Input), jsonOrStr([]byte(resultContent)), latencyMs))

			messages = append(messages, provider.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Content:    resultContent,
			})

			stepCount++
			if stepCount > a.MaxSteps {
				h.runs.failRun(dbCtx, runID, "max steps exceeded") //nolint:errcheck
				sseErr("max steps exceeded")
				return
			}
		}
	}
}

// sanitizeToolName converts an agent name into a valid LLM tool name (lowercase, underscores).
func sanitizeToolName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		case r == ' ', r == '-', r == '.':
			b.WriteRune('_')
		}
	}
	s := b.String()
	if len(s) > 50 {
		s = s[:50]
	}
	if s == "" {
		s = "agent"
	}
	return s
}

// evaluateExpression evaluates a simple string expression against the given
// output text. Returns true if the output satisfies the expression.
//
// Supported prefixes:
//
//	"*" or ""         → always true
//	"contains:<s>"    → output contains s
//	"not_contains:<s>"→ output does not contain s
//	"startswith:<s>"  → output starts with s
//	"endswith:<s>"    → output ends with s
func evaluateExpression(expression, output string) bool {
	output = strings.ToLower(strings.TrimSpace(output))
	switch {
	case expression == "*" || expression == "":
		return true
	case strings.HasPrefix(expression, "contains:"):
		return strings.Contains(output, strings.ToLower(strings.TrimSpace(expression[9:])))
	case strings.HasPrefix(expression, "not_contains:"):
		return !strings.Contains(output, strings.ToLower(strings.TrimSpace(expression[13:])))
	case strings.HasPrefix(expression, "startswith:"):
		return strings.HasPrefix(output, strings.ToLower(strings.TrimSpace(expression[11:])))
	case strings.HasPrefix(expression, "endswith:"):
		return strings.HasSuffix(output, strings.ToLower(strings.TrimSpace(expression[9:])))
	}
	return true
}

