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
	"github.com/deepaksingh/agent-nexus/services/api/internal/runtime/cost"
	"github.com/deepaksingh/agent-nexus/services/api/internal/runtime/memory"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools/native"
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
		h.executeRun(r.Context(), a, ws, uid, runID, convID, req.Input, nil, emit, invokeOpts{})
		return
	}

	// Non-streaming: return run_id immediately and execute in background
	errs.WriteJSON(w, http.StatusAccepted, map[string]any{
		"run_id":          runID,
		"conversation_id": convID,
		"status":          "running",
	})

	go h.executeRun(context.Background(), a, ws, uid, runID, convID, req.Input, nil, nil, invokeOpts{})
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
		`INSERT INTO conversations(id,workspace_id,user_id,title,workflow_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5::uuid)`,
		convID, ws, uid, "Workflow: "+gName, groupID); err != nil {
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

const maxInvokeDepth = 3

// invokeOpts carries optional parameters for executeRun that are set when an agent
// is called as a sub-task by another agent (via native_call_agent).
type invokeOpts struct {
	invokeDepth int    // nesting depth; 0 = root run
	rootTraceID string // trace_id of the root run; empty means this IS the root
	// SendMessage, if non-nil, delivers mid-run progress text back to the caller (e.g. WhatsApp).
	SendMessage func(ctx context.Context, msg string) error
}

// progressLabel returns a short human-readable status for a tool that is about to execute.
// Returns "" for tools that send their own messages (e.g. native_send_message).
func progressLabel(toolName string) string {
	switch toolName {
	// Tools that send their own message — suppress automated prefix.
	case "native_send_message", "native_ask_user":
		return ""

	// Agent & workflow delegation
	case "native_call_agent", "call_agent":
		return "Calling a sub-agent…"
	case "native_run_workflow", "run_workflow":
		return "Running a workflow…"
	case "native_create_agent":
		return "Spinning up agents…"
	case "native_delete_agent":
		return "Cleaning up…"
	case "native_update_agent":
		return "Updating agent…"
	case "native_list_agents":
		return "Looking up agents…"

	// Web & HTTP
	case "native_web_search", "web_search":
		return "Searching the web…"
	case "native_http_request", "http_request":
		return "Fetching data…"

	// Files
	case "native_read_file", "read_file":
		return "Reading file…"
	case "native_write_file", "write_file":
		return "Writing file…"

	// Memory
	case "native_save_memory":
		return "Saving to memory…"

	// Skills & tools management
	case "native_create_skill", "native_update_skill", "native_delete_skill",
		"native_attach_skill", "native_detach_skill", "native_list_skills":
		return "Managing skills…"
	case "native_create_http_tool", "native_create_code_tool", "native_delete_tool",
		"native_attach_tool", "native_detach_tool", "native_list_tools",
		"native_list_http_tools", "native_list_workspace_tools", "native_request_tool":
		return "Managing tools…"

	// Workflow management
	case "native_create_workflow", "native_delete_workflow",
		"native_save_workflow_graph", "native_list_workflows":
		return "Managing workflows…"

	// WhatsApp
	case "whatsapp_send_message":
		return "Sending message…"
	case "whatsapp_create_reminder":
		return "Setting a reminder…"
	case "whatsapp_complete_reminder":
		return "Completing reminder…"
	case "whatsapp_list_reminders":
		return "Checking reminders…"
	case "whatsapp_schedule_message":
		return "Scheduling a message…"
	case "whatsapp_search_contacts":
		return "Looking up contacts…"
	case "whatsapp_list_recent_messages":
		return "Reading recent messages…"
	case "whatsapp_get_current_context":
		return "Checking context…"
	case "whatsapp_request_owner_approval":
		return "Requesting approval…"
	case "whatsapp_send_media_status":
		return "Updating status…"
	case "whatsapp_summarize_link":
		return "Summarizing link…"

	// MCP tools — strip the mcp_ prefix and humanise
	default:
		if strings.HasPrefix(toolName, "mcp_") {
			label := strings.ReplaceAll(strings.TrimPrefix(toolName, "mcp_"), "_", " ")
			return "Working: " + label + "…"
		}
		// Code tools and anything else
		return "Working on it…"
	}
}

// runAgentInline executes an agent synchronously as a sub-call from native_call_agent.
// It creates a new conversation and run, executes the agent, and returns the output string.
func (h *InvokeHandler) runAgentInline(ctx context.Context, ws, uid, agentID, task, parentRunID, rootTraceID string, depth int) (string, error) {
	agents := repository.NewAgentRepository(h.pool)
	a, err := agents.Get(ctx, agentID, ws)
	if err != nil {
		return "", fmt.Errorf("agent %s not found", agentID)
	}
	if a.Status != "active" {
		return "", fmt.Errorf("agent %s is not active", a.Name)
	}

	convID := uuid.NewString()
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO conversations(id,workspace_id,agent_id,user_id,title) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'Sub-agent call')`,
		convID, ws, agentID, uid); err != nil {
		return "", fmt.Errorf("create conversation: %w", err)
	}
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO messages(id,conversation_id,role,content) VALUES($1::uuid,$2::uuid,'user',$3)`,
		uuid.NewString(), convID, task); err != nil {
		return "", err
	}

	traceID := rootTraceID
	if traceID == "" {
		traceID = parentRunID
	}
	runID := uuid.NewString()
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status,parent_run_id,trace_id)
		 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,'running',$7::uuid,$8::uuid)`,
		runID, ws, agentID, convID, uid, task, parentRunID, traceID); err != nil {
		return "", fmt.Errorf("create run: %w", err)
	}

	// Execute synchronously; emit=nil means no SSE output from sub-runs.
	h.executeRun(ctx, a, ws, uid, runID, convID, task, nil, nil, invokeOpts{
		invokeDepth: depth,
		rootTraceID: traceID,
	})

	var output, status, errMsg string
	_ = h.pool.QueryRow(context.Background(),
		`SELECT COALESCE(output,''), status, COALESCE(error_message,'') FROM runs WHERE id=$1::uuid`,
		runID).Scan(&output, &status, &errMsg)

	if status != "success" {
		if errMsg == "" {
			errMsg = "run did not complete successfully (status: " + status + ")"
		}
		return "", fmt.Errorf("sub-agent run failed: %s", errMsg)
	}
	return output, nil
}

// runWorkflowInline creates a run record for workflowID and executes it in a background goroutine.
// Returns the run_id immediately.
func (h *InvokeHandler) runWorkflowInline(ctx context.Context, ws, uid, workflowID, input, parentRunID string) (string, error) {
	// Verify workflow exists
	var wName string
	if err := h.pool.QueryRow(ctx,
		`SELECT name FROM workflows WHERE id=$1::uuid AND workspace_id=$2::uuid AND status='active'`,
		workflowID, ws).Scan(&wName); err != nil {
		return "", fmt.Errorf("workflow %s not found", workflowID)
	}
	convID := uuid.NewString()
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO conversations(id,workspace_id,user_id,title,workflow_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5::uuid)`,
		convID, ws, uid, "Workflow: "+wName, workflowID); err != nil {
		return "", fmt.Errorf("create conversation: %w", err)
	}
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO messages(id,conversation_id,role,content) VALUES($1::uuid,$2::uuid,'user',$3)`,
		uuid.NewString(), convID, input); err != nil {
		return "", err
	}
	runID := uuid.NewString()
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO runs(id,workspace_id,conversation_id,user_id,input,status,parent_run_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,'running',$6::uuid)`,
		runID, ws, convID, uid, input, parentRunID); err != nil {
		return "", fmt.Errorf("create run: %w", err)
	}
	go h.executeGroupRun(context.Background(), workflowID, ws, uid, runID, convID, input, nil)
	return runID, nil
}

// cleanupEphemeralResources deletes run-scoped resources created with
// ephemeral=true during the given run or any child run.
func (h *InvokeHandler) cleanupEphemeralResources(ctx context.Context, runID string) {
	runTree := `
		WITH RECURSIVE run_tree AS (
			SELECT id FROM runs WHERE id=$1::uuid
			UNION ALL
			SELECT r.id FROM runs r JOIN run_tree rt ON r.parent_run_id=rt.id
		)`
	h.pool.Exec(ctx, //nolint:errcheck
		runTree+`
		DELETE FROM workflows
		WHERE source_run_id IN (SELECT id FROM run_tree) AND ephemeral=true`, runID)
	h.pool.Exec(ctx, //nolint:errcheck
		runTree+`
		DELETE FROM agent_skills
		WHERE agent_id IN (
			SELECT id FROM agents WHERE source_run_id IN (SELECT id FROM run_tree) AND ephemeral=true
		)`, runID)
	h.pool.Exec(ctx, //nolint:errcheck
		runTree+`
		DELETE FROM agents
		WHERE source_run_id IN (SELECT id FROM run_tree) AND ephemeral=true`, runID)
	h.pool.Exec(ctx, //nolint:errcheck
		runTree+`
		DELETE FROM skills
		WHERE source_run_id IN (SELECT id FROM run_tree) AND ephemeral=true`, runID)
	h.pool.Exec(ctx, //nolint:errcheck
		runTree+`
		DELETE FROM tools
		WHERE source_run_id IN (SELECT id FROM run_tree) AND ephemeral=true`, runID)
}

// executeRun runs the full agent loop with tool calling. When emit is nil (background mode),
// progress is only written to the database. When emit is non-nil, SSE events are also sent.
// delegateHandlers, if non-nil, maps tool names to functions that execute a delegate agent and
// return its output — used by supervisor nodes to call team agents as tools.
func (h *InvokeHandler) executeRun(ctx context.Context, a *domain.Agent, ws, uid, runID, convID, input string, delegateHandlers map[string]func(context.Context, json.RawMessage) string, emit func(string), opts invokeOpts) {
	// dbCtx is used for all DB status writes (failRun, final UPDATE).
	// It must NOT be the request context because the request context can be
	// cancelled on client disconnect, which would silently leave the run in
	// 'running' state. Use a generous timeout that outlasts any realistic run
	// while still bounding resource lifetime if something hangs.
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer dbCancel()

	runCompleted := false
	runErrMsg := "run terminated unexpectedly"
	defer func() {
		if !runCompleted {
			h.runs.failRun(dbCtx, runID, runErrMsg) //nolint:errcheck
		}
		if opts.invokeDepth == 0 {
			h.cleanupEphemeralResources(context.Background(), runID)
		}
	}()

	sseEmitOrNil := func(s string) {
		if emit != nil {
			emit(s)
		}
	}
	sseErr := func(msg string) { sseEmitOrNil(fmt.Sprintf(`{"type":"error","error":%q}`, msg)) }

	llm, err := h.runs.providerFor(ctx, ws, a.Provider)
	if err != nil {
		runErrMsg = err.Error()
		sseErr(err.Error())
		return
	}

	contextChunks := []agentprompt.ContextChunk{}

	if a.ContextRetrievalEnabled {
		start := time.Now()
		connIDs, err := h.runs.agentConnectorIDs(ctx, a.ID)
		var queryEmbedding []float32
		if err == nil {
			queryEmbedding = tryEmbed(ctx, h.cfg, llm, input)
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

	var convCompaction string
	_ = h.pool.QueryRow(ctx,
		`SELECT COALESCE(compaction, '') FROM conversations WHERE id=$1::uuid`,
		convID).Scan(&convCompaction)

	histLimit := 4
	if convCompaction == "" {
		histLimit = a.MaxHistoryMessages
		if histLimit <= 0 {
			histLimit = 20
		}
	}
	historyRows, err := h.pool.Query(ctx,
		`SELECT role, content, COALESCE(tool_call_id,''), COALESCE(tool_name,''), COALESCE(tool_calls::text,'')
		 FROM messages WHERE conversation_id=$1::uuid
		 ORDER BY created_at DESC LIMIT $2`,
		convID, histLimit)
	if err != nil {
		runErrMsg = err.Error()
		sseErr("failed to load conversation history")
		return
	}
	defer historyRows.Close()
	history := []provider.Message{}
	for historyRows.Next() {
		var role, content, toolCallID, toolName, toolCallsRaw string
		if historyRows.Scan(&role, &content, &toolCallID, &toolName, &toolCallsRaw) == nil && (role == "user" || role == "assistant" || role == "tool") {
			content = injectActionLog(role, content, toolCallsRaw)
			if len(content) > 800 {
				content = content[:800] + "…[truncated]"
			}
			history = append(history, provider.Message{Role: role, Content: content, ToolCallID: toolCallID, ToolName: toolName})
		}
	}
	// Reverse: query returned newest-first, LLM needs oldest-first.
	for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
		history[i], history[j] = history[j], history[i]
	}

	allToolDefs, dbTools, _ := loadAgentToolDefs(ctx, h.pool, a.ID)
	allToolDefs, dbTools = ensureMemoryToolDefs(allToolDefs, dbTools)

	// Build tool summary map for lazy loading and native meta-tools.
	toolSummaries := make(map[string]string, len(allToolDefs))
	for _, td := range allToolDefs {
		toolSummaries[td.Name] = td.Description
	}

	hasCallAgent := false
	hasCreateAgent := false
	for _, td := range allToolDefs {
		switch td.Name {
		case "native_call_agent":
			hasCallAgent = true
		case "native_create_agent":
			hasCreateAgent = true
		}
	}

	skills, _ := loadAgentSkills(ctx, h.pool, a.ID)
	instructions := a.Instructions + onDemandSkillsInstructions(skills.OnDemand)
	initialMessages, stableSystem := agentprompt.NewBuilder().Build(agentprompt.BuildRequest{
		SystemInstructions: instructions,
		Skills:             skills.Always,
		ContextChunks:      contextChunks,
		History:            history,
		MemoryEnabled:      a.MemoryEnabled,
		MemorySaveMode:     a.MemorySaveMode,
		HasCallAgent:       hasCallAgent,
		HasCreateAgent:     hasCreateAgent,
		LazyToolLoading:    a.LazyToolLoading,
		ConvCompaction:     convCompaction,
	})

	// requestedTools grows when native_request_tool is called (lazy loading mode).
	requestedTools := map[string]bool{}
	skillSummaries := map[string]string{}
	skillToolMap := map[string]string{}
	for name, skill := range skills.OnDemand {
		skillSummaries[name] = skill.Description
		for _, toolName := range skill.RequiredToolNames {
			skillToolMap[toolName] = name
		}
	}
	activeSkills := map[string]bool{}
	activeMemoryIDs := map[string]bool{}
	var messages []provider.Message

	// Determine trace root and depth for sub-agent calls.
	rootTraceID := opts.rootTraceID
	if rootTraceID == "" {
		rootTraceID = runID
	}

	// CallAgent closure — nil when at max depth.
	var callAgentFn func(ctx context.Context, agentID, task string) (string, error)
	if opts.invokeDepth < maxInvokeDepth {
		capturedDepth := opts.invokeDepth
		capturedRoot := rootTraceID
		capturedRunID := runID
		callAgentFn = func(ctx context.Context, agentID, task string) (string, error) {
			return h.runAgentInline(ctx, ws, uid, agentID, task, capturedRunID, capturedRoot, capturedDepth+1)
		}
	}

	execCtx := tools.ExecutionContext{
		WorkspaceID:       ws,
		AgentID:           a.ID,
		AgentProvider:     a.Provider,
		AgentModel:        a.Model,
		UserID:            uid,
		RunID:             runID,
		ConversationID:    convID,
		ToolSummaries:     toolSummaries,
		AlwaysActiveTools: metaToolNameSet(),
		SkillSummaries:    skillSummaries,
		SkillToolMap:      skillToolMap,
		InvokeDepth:       opts.invokeDepth,
		RootRunID:         rootTraceID,
		CallAgent:         callAgentFn,
		RunWorkflow: func(ctx context.Context, workflowID, input string) (string, error) {
			return h.runWorkflowInline(ctx, ws, uid, workflowID, input, runID)
		},
		CompressText: func(ctx context.Context, text string) (string, error) {
			ch, cerr := llm.Complete(ctx, provider.CompletionRequest{
				Model: a.Model,
				Messages: []provider.Message{
					{Role: "system", Content: "You are a memory compressor. Return ONLY the compressed memory — no preamble, no explanation."},
					{Role: "user", Content: "Compress this to ≤100 words, preserving all key facts:\n" + text},
				},
				Temperature: 0,
				MaxTokens:   200,
				Stream:      true,
			})
			if cerr != nil {
				return "", cerr
			}
			var result strings.Builder
			for event := range ch {
				if event.Type == provider.EventDelta {
					result.WriteString(event.Delta)
				}
				if event.Type == provider.EventError {
					return "", event.Error
				}
			}
			return strings.TrimSpace(result.String()), nil
		},
		SearchMemory: func(ctx context.Context, query string, limit int) ([]domain.Memory, error) {
			embedding := memoryEmbedding(ctx, llm, query)
			return memory.NewEngine(h.pool).Retrieve(ctx, a.ID, ws, convID, embedding, limit, a.MinRelevanceScore)
		},
		RequestMemory: func(memories []domain.Memory) bool {
			return appendMemoryContext(messages, memories, activeMemoryIDs) > 0
		},
		RequestTool: func(name string) {
			requestedTools[name] = true
		},
		RequestSkill: func(name string) bool {
			skill, ok := skills.OnDemand[name]
			if !ok || activeSkills[name] {
				return false
			}
			activeSkills[name] = true
			for _, toolName := range skill.RequiredToolNames {
				requestedTools[toolName] = true
				// Ensure the tool appears in toolSummaries so native_list_tools and
				// native_request_tool can find it even if it isn't in agent_tools.
				if _, known := toolSummaries[toolName]; !known {
					if tool, err := h.registry.Get(toolName); err == nil {
						def := tool.Definition()
						toolSummaries[def.Name] = def.Description
					}
				}
			}
			if len(messages) > 0 {
				messages[0].Content += "\n\n[Skill: " + skill.Name + "]\n" + skill.Content
			}
			h.runs.createStep(ctx, runID, domain.StepToolCall, map[string]any{"skill": skill.Name}, map[string]any{"activated": true}, time.Now(), 0, "native_request_skill", "") //nolint:errcheck
			return true
		},
	}
	_ = h.pool.QueryRow(ctx, `SELECT COALESCE(channel_session_id::text,'') FROM runs WHERE id=$1::uuid`, runID).Scan(&execCtx.ChannelSessionID)
	execCtx.SendMessage = opts.SendMessage
	capturedRunID := runID
	execCtx.WaitForUserInput = func(ctx context.Context, question string) (string, error) {
		sseEmitOrNil(fmt.Sprintf(`{"type":"user_input_required","run_id":%q,"question":%s}`,
			capturedRunID, jsonOrStr([]byte(`"`+question+`"`))))
		if opts.SendMessage != nil {
			opts.SendMessage(ctx, question) //nolint:errcheck
		}
		ch := RegisterUserInputWait(capturedRunID)
		h.pool.Exec(ctx, `UPDATE runs SET status='user_input_wait' WHERE id=$1::uuid`, capturedRunID) //nolint:errcheck
		select {
		case answer := <-ch:
			h.pool.Exec(ctx, `UPDATE runs SET status='running' WHERE id=$1::uuid`, capturedRunID) //nolint:errcheck
			return answer, nil
		case <-time.After(30 * time.Minute):
			UnregisterUserInputWait(capturedRunID)
			h.pool.Exec(ctx, `UPDATE runs SET status='running' WHERE id=$1::uuid`, capturedRunID) //nolint:errcheck
			return "", fmt.Errorf("user input timed out after 30 minutes")
		}
	}

	sseEmitOrNil(fmt.Sprintf(`{"type":"run_started","run_id":%q}`, runID))

	// ── Tool calling loop ──────────────────────────────────────────────────────
	messages = initialMessages
	stepCount := 0
	totalInput, totalOutput := 0, 0
	memorySaveCalled := false
	futureWorkRetried := false
	actionLog := []string{}

	for {
		// Build the tool list for this iteration.
		var toolDefs []provider.ToolDefinition
		if a.LazyToolLoading {
			// Start with just the meta-tools; add any tools the agent has requested.
			toolDefs = lazyMetaToolDefs(h.registry)
			coveredByDB := map[string]bool{}
			for _, td := range allToolDefs {
				coveredByDB[td.Name] = true
				if requestedTools[td.Name] {
					toolDefs = append(toolDefs, td)
				}
			}
			// Fallback: requested tools not in agent_tools (e.g. skill required tools not yet
			// auto-attached) are loaded directly from the native registry.
			for name := range requestedTools {
				if !coveredByDB[name] {
					if tool, err := h.registry.Get(name); err == nil {
						def := tool.Definition()
						toolDefs = append(toolDefs, provider.ToolDefinition{
							Name:        def.Name,
							Description: def.Description,
							InputSchema: def.InputSchema,
						})
					}
				}
			}
		} else {
			toolDefs = append(lazyMetaToolDefs(h.registry), allToolDefs...)
		}
		toolDefs = dedupeToolDefs(toolDefs)
		allowedToolNames := map[string]bool{}
		for _, td := range toolDefs {
			allowedToolNames[td.Name] = true
		}

		if trimmed, n := provider.TruncateMessages(messages, a.Model, a.MaxTokens); n > 0 {
			messages = trimmed
			sseEmitOrNil(fmt.Sprintf(`{"type":"delta","content":%q}`,
				fmt.Sprintf("[Context trimmed: dropped %d older messages to fit within model context window]\n\n", n)))
		}

		modelStart := time.Now()
		completion, err := completeWithEmptyRetry(ctx, llm, provider.CompletionRequest{
			Model:               a.Model,
			Messages:            messages,
			Tools:               toolDefs,
			Temperature:         a.Temperature,
			MaxTokens:           a.MaxTokens,
			Stream:              true,
			StableSystemContent: stableSystem,
		}, func(delta string) {
			sseEmitOrNil(fmt.Sprintf(`{"type":"delta","content":%q}`, delta))
		}, "Return a direct, non-empty reply. Do not explain limitations. Reply as Deepak would naturally reply, and make sure the message is not blank.")
		if err != nil {
			runErrMsg = err.Error()
			sseErr(err.Error())
			return
		}

		reply := completion.Reply
		usage := completion.Usage
		pendingCalls := completion.ToolCalls

		totalInput += usage.InputTokens
		totalOutput += usage.OutputTokens
		h.runs.createStep(ctx, runID, domain.StepModelCall, //nolint:errcheck
			map[string]any{"provider": a.Provider, "model": a.Model, "messages": len(messages)},
			map[string]any{"content": reply, "tool_calls": len(pendingCalls)},
			modelStart, usage.InputTokens+usage.OutputTokens, "", "")

		if len(pendingCalls) == 0 {
			if stepCount == 0 && promisesUnconfirmedFutureWork(reply) {
				if !futureWorkRetried {
					futureWorkRetried = true
					messages[0].Content += futureWorkCorrection
					continue
				}
				reply = "I don't have enough information to answer that."
			}
			if strings.TrimSpace(reply) == "" {
				msg := "model returned an empty response"
				runErrMsg = msg
				sseErr(msg)
				return
			}
			var actionLogJSON any
			if len(actionLog) > 0 {
				b, _ := json.Marshal(actionLog)
				actionLogJSON = b
			}
			h.pool.Exec(ctx, //nolint:errcheck
				`INSERT INTO messages(id,conversation_id,role,content,tool_calls,tokens) VALUES($1::uuid,$2::uuid,'assistant',$3,$4::jsonb,$5)`,
				uuid.NewString(), convID, reply, actionLogJSON, usage.OutputTokens)
			h.runs.createStep(ctx, runID, domain.StepFinalResponse, //nolint:errcheck
				map[string]any{},
				map[string]any{"content": reply},
				time.Now(), usage.OutputTokens, "", "")
			runCompleted = true
			costUSD := cost.Estimate(a.Provider, a.Model, totalInput, totalOutput)
			h.pool.Exec(dbCtx, //nolint:errcheck
				`UPDATE runs SET output=$2,status='success',completed_at=NOW(),total_input_tokens=$3,total_output_tokens=$4,cost_estimate=$5 WHERE id=$1::uuid`,
				runID, reply, totalInput, totalOutput, costUSD)
			sseEmitOrNil(fmt.Sprintf(`{"type":"run_completed","run_id":%q,"usage":{"input":%d,"output":%d},"cost":%g}`,
				runID, totalInput, totalOutput, costUSD))
			// Memory extraction runs after marking run complete so gateway delivery
			// is not blocked by the extra LLM call.
			if shouldRunMemoryExtractor(a, memorySaveCalled) {
				aCopy, llmCopy, replySnap, inputSnap := a, llm, reply, input
				go func() {
					start := time.Now()
					count, err := runMemoryExtractor(context.Background(), h.pool, llmCopy, aCopy, ws, uid, convID, runID, inputSnap, replySnap)
					h.runs.createStep(context.Background(), runID, domain.StepToolCall, //nolint:errcheck
						map[string]any{"tool": "memory_extractor"},
						map[string]any{"saved": count},
						start, 0, "memory_extractor", errString(err))
				}()
			}
			{
				threshold := a.CompactionThreshold
				if threshold <= 0 {
					threshold = 6
				}
				tokenThreshold := a.CompactionTokenThreshold
				if tokenThreshold <= 0 {
					tokenThreshold = 3000
				}
				if convCompaction != "" || len(history) >= threshold || totalInput > tokenThreshold {
					llmCopy, modelSnap, convSnap, compSnap := llm, a.Model, convID, convCompaction
					go func() {
						sseEmitOrNil(`{"type":"compacting","status":"start"}`)
						newCompaction, err := compactConversation(context.Background(), h.pool, llmCopy, modelSnap, convSnap, compSnap)
						if err != nil || newCompaction == "" {
							sseEmitOrNil(`{"type":"compacting","status":"done"}`)
							return
						}
						h.pool.Exec(context.Background(), //nolint:errcheck
							`UPDATE conversations SET compaction=$2, updated_at=NOW() WHERE id=$1::uuid`,
							convSnap, newCompaction)
						sseEmitOrNil(`{"type":"compacting","status":"done"}`)
					}()
				}
			}
			return
		}

		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   reply,
			ToolCalls: pendingCalls,
		})

		// Pre-compute: if there are multiple native_call_agent calls in this batch,
		// run them all concurrently in goroutines. Results are keyed by tool call ID.
		parallelAgentResults := map[string]string{}
		{
			var agentCallsInBatch []provider.ToolCall
			for _, call := range pendingCalls {
				if call.Name == "native_call_agent" {
					agentCallsInBatch = append(agentCallsInBatch, call)
				}
			}
			if len(agentCallsInBatch) > 1 {
				type agentCallOut struct {
					id      string
					content string
				}
				resultsCh := make(chan agentCallOut, len(agentCallsInBatch))
				for _, call := range agentCallsInBatch {
					capturedCall := call
					go func() {
						result, execErr := h.executor.ExecuteWithContext(ctx, execCtx, capturedCall.Name, capturedCall.Input)
						var content string
						if execErr != nil {
							content = fmt.Sprintf(`{"error":%q}`, execErr.Error())
						} else if result != nil {
							if result.Error != "" {
								content = fmt.Sprintf(`{"error":%q}`, result.Error)
							} else {
								b, _ := json.Marshal(result.Output)
								content = string(b)
							}
						}
						resultsCh <- agentCallOut{id: capturedCall.ID, content: content}
					}()
				}
				for range agentCallsInBatch {
					out := <-resultsCh
					parallelAgentResults[out.id] = out.content
				}
			}
		}

		for _, call := range pendingCalls {
			if call.Name == "native_save_memory" {
				memorySaveCalled = true
			}
			// Delegate tool — hand off to a team agent and return its output.
			if handler, isDelegate := delegateHandlers[call.Name]; isDelegate {
				sseEmitOrNil(fmt.Sprintf(`{"type":"tool_started","call_id":%q,"tool":%q,"input":%s}`,
					call.ID, call.Name, jsonOrStr(call.Input)))
				delegateStart := time.Now()
				delegateOutput := handler(ctx, call.Input)
				h.runs.createStep(ctx, runID, domain.StepToolCall, //nolint:errcheck
					map[string]any{"tool": call.Name, "input": call.Input},
					map[string]any{"output": delegateOutput},
					delegateStart, 0, call.Name, "")
				sseEmitOrNil(fmt.Sprintf(`{"type":"tool_call","call_id":%q,"tool":%q,"input":%s,"output":%s,"latency_ms":%d}`,
					call.ID, call.Name, jsonOrStr(call.Input), jsonOrStr([]byte(delegateOutput)),
					int(time.Since(delegateStart).Milliseconds())))
				messages = append(messages, provider.Message{
					Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: delegateOutput,
				})
				stepCount++
				if stepCount > a.MaxSteps {
					runErrMsg = "max steps exceeded"
					sseErr("max steps exceeded")
					return
				}
				continue
			}

			// Lazy loading gate: reject calls to tools that weren't actually offered to the
			// model this turn (e.g. it guessed a name from native_list_tools output) so it's
			// forced through native_request_tool/native_request_skill — keeping traces honest
			// about what was activated.
			if a.LazyToolLoading && !allowedToolNames[call.Name] {
				gateErr := lazyToolNotActiveError(call.Name, execCtx.SkillToolMap)
				h.runs.createStep(ctx, runID, domain.StepToolCall, //nolint:errcheck
					map[string]any{"tool": call.Name, "input": call.Input},
					map[string]any{"error": gateErr},
					time.Now(), 0, call.Name, gateErr)
				sseEmitOrNil(fmt.Sprintf(`{"type":"tool_call","call_id":%q,"tool":%q,"input":%s,"output":%s,"latency_ms":0}`,
					call.ID, call.Name, jsonOrStr(call.Input), jsonOrStr([]byte(fmt.Sprintf(`{"error":%q}`, gateErr)))))
				messages = append(messages, provider.Message{
					Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: fmt.Sprintf(`{"error":%q}`, gateErr),
				})
				stepCount++
				if stepCount > a.MaxSteps {
					runErrMsg = "max steps exceeded"
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
					runErrMsg = "approval timed out"
					sseErr("approval timed out after 10 minutes")
					return
				}
				h.pool.Exec(ctx, `UPDATE runs SET status='running' WHERE id=$1::uuid`, runID) //nolint:errcheck

				if decision.Decision == "rejected" {
					messages = append(messages, provider.Message{Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: "Tool call rejected by user."})
					continue
				}
			}

			sseEmitOrNil(fmt.Sprintf(`{"type":"tool_started","call_id":%q,"tool":%q,"input":%s}`,
				call.ID, call.Name, jsonOrStr(call.Input)))
			if execCtx.SendMessage != nil {
				if label := progressLabel(call.Name); label != "" {
					execCtx.SendMessage(ctx, label) //nolint:errcheck
				}
			}

			var resultContent, errMsg string
			latencyMs := 0
			if precomputed, ok := parallelAgentResults[call.ID]; ok {
				// Already executed concurrently in the parallel batch above; content is ready.
				resultContent = precomputed
			} else {
				var result *tools.ExecutionResult
				var execErr error
				if toolExists && dbTool.Type == "http" {
					var cfg tools.HTTPToolConfig
					_ = json.Unmarshal(dbTool.Config, &cfg)
					result = tools.ExecuteHTTP(ctx, cfg, call.Input, dbTool.TimeoutMs)
				} else if toolExists && dbTool.Type == "code" {
					var codeCfg struct {
						Code string `json:"code"`
					}
					_ = json.Unmarshal(dbTool.Config, &codeCfg)
					start := time.Now()
					out, codeErr := native.ExecuteCodeTool(ctx, codeCfg.Code, call.Input)
					result = &tools.ExecutionResult{LatencyMs: int(time.Since(start).Milliseconds())}
					if codeErr != nil {
						result.Error = codeErr.Error()
					} else {
						result.Output = out
					}
				} else {
					result, execErr = h.executor.ExecuteWithContext(ctx, execCtx, call.Name, call.Input)
				}
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
			}
			if !execCtx.AlwaysActiveTools[call.Name] {
				actionLog = append(actionLog, summarizeToolCall(call.Name, call.Input, errMsg))
			}

			h.runs.createStep(ctx, runID, domain.StepToolCall, //nolint:errcheck
				map[string]any{"tool": call.Name, "input": call.Input},
				map[string]any{"output": resultContent},
				time.Now(), 0, call.Name, errMsg)

			sseEmitOrNil(fmt.Sprintf(`{"type":"tool_call","call_id":%q,"tool":%q,"input":%s,"output":%s,"latency_ms":%d}`,
				call.ID, call.Name, jsonOrStr(call.Input), jsonOrStr([]byte(resultContent)), latencyMs))

			messages = append(messages, provider.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Content:    resultContent,
				IsError:    errMsg != "",
			})

			stepCount++
			if stepCount > a.MaxSteps {
				runErrMsg = "max steps exceeded"
				sseErr("max steps exceeded")
				return
			}
		}
		// Reload tool definitions to pick up tools created or attached mid-run.
		if freshAll, freshDB, err := loadAgentToolDefs(ctx, h.pool, a.ID); err == nil {
			freshAll, freshDB = ensureMemoryToolDefs(freshAll, freshDB)
			allToolDefs, dbTools = freshAll, freshDB
			for _, td := range freshAll {
				toolSummaries[td.Name] = td.Description
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
		h.cleanupEphemeralResources(context.Background(), parentRunID)
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
					`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status,parent_run_id,workflow_node_id,trace_id)
					 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,'running',$7::uuid,$8,$9::uuid)`,
					subRunID, ws, a.ID, subConvID, uid, agentInput, parentRunID, node.ID, parentRunID)

				// Wrap emit so every SSE event from this node carries node_id/node_name.
				// Route through sseEmit (which holds emitMu) — parallel branch
				// goroutines call agentEmit concurrently; calling the raw emit
				// func without the mutex causes a data race on ResponseWriter.
				capturedNodeID := node.ID
				capturedNodeName := nodeName
				agentEmit := func(line string) {
					var m map[string]any
					if json.Unmarshal([]byte(line), &m) == nil {
						// Sub-run lifecycle events must not leak to the workflow stream.
						if t, _ := m["type"].(string); t == "run_completed" || t == "run_started" {
							return
						}
						m["node_id"] = capturedNodeID
						m["node_name"] = capturedNodeName
						if b, err := json.Marshal(m); err == nil {
							sseEmit(string(b))
							return
						}
					}
					sseEmit(line)
				}

				h.executeRun(ctx, a, ws, uid, subRunID, subConvID, agentInput, nil, agentEmit, invokeOpts{})

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

				// Pre-declare the supervisor sub-run ID so delegate handlers can use it
				// as their parent_run_id, establishing the correct trace hierarchy.
				supervisorSubRunID := uuid.NewString()

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
					// Prefer the one-sentence Description field; fall back to first line
					// of Instructions; then fall back to a generic sentinel.
					desc := strings.TrimSpace(da.Description)
					if desc == "" {
						desc = da.Instructions
						if idx := strings.Index(desc, "\n"); idx > 0 {
							desc = desc[:idx]
						}
						desc = strings.TrimSpace(desc)
					}
					if desc == "" {
						desc = "Delegate a task to the " + da.Name + " agent and get its response."
					}
					capturedDA := da
					capturedDN := dn
					capturedSuperNodeID := node.ID
					capturedSuperNodeName := nodeName
					capturedSuperSubRunID := supervisorSubRunID
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
							`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status,parent_run_id,workflow_node_id,trace_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,'running',$7::uuid,$8,$9::uuid)`,
							dSubRunID, ws, capturedDA.ID, dSubConvID, uid, task, capturedSuperSubRunID, capturedDN.ID, parentRunID)
						// Light up the delegate node in the canvas.
						sseEmit(fmt.Sprintf(`{"type":"node_started","node_id":%q,"node_type":"agent","node_name":%q}`,
							capturedDN.ID, capturedDA.Name))
						delEmit := func(line string) {
							var m map[string]any
							if json.Unmarshal([]byte(line), &m) == nil {
								// Sub-run lifecycle events must not leak to the workflow stream.
								if t, _ := m["type"].(string); t == "run_completed" || t == "run_started" {
									return
								}
								m["node_id"] = capturedSuperNodeID
								m["node_name"] = capturedSuperNodeName
								if b, e2 := json.Marshal(m); e2 == nil {
									sseEmit(string(b))
									return
								}
							}
							sseEmit(line)
						}
						h.executeRun(callCtx, capturedDA, ws, uid, dSubRunID, dSubConvID, task, nil, delEmit, invokeOpts{})
						sseEmit(fmt.Sprintf(`{"type":"node_completed","node_id":%q,"node_name":%q}`,
							capturedDN.ID, capturedDA.Name))
						var out string
						_ = h.pool.QueryRow(context.Background(),
							`SELECT COALESCE(output,'') FROM runs WHERE id=$1::uuid`, dSubRunID).Scan(&out)
						return out
					}
				}

				// Sub-conversation + sub-run for the supervisor itself.
				subConvID := uuid.NewString()
				subRunID := supervisorSubRunID
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
					`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status,parent_run_id,workflow_node_id,trace_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,'running',$7::uuid,$8,$9::uuid)`,
					subRunID, ws, supAgent.ID, subConvID, uid, agentInput, parentRunID, node.ID, parentRunID)

				capturedNodeID := node.ID
				capturedNodeName := nodeName
				supEmit := func(line string) {
					var m map[string]any
					if json.Unmarshal([]byte(line), &m) == nil {
						// Sub-run lifecycle events must not leak to the workflow stream.
						if t, _ := m["type"].(string); t == "run_completed" || t == "run_started" {
							return
						}
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
	// Aggregate token counts and cost from all sub-runs belonging to this trace.
	var totalInputTokens, totalOutputTokens int
	var totalCostUSD float64
	_ = h.pool.QueryRow(context.Background(),
		`SELECT COALESCE(SUM(total_input_tokens),0), COALESCE(SUM(total_output_tokens),0), COALESCE(SUM(cost_estimate),0)
		 FROM runs WHERE trace_id=$1::uuid AND id!=$1::uuid`,
		parentRunID).Scan(&totalInputTokens, &totalOutputTokens, &totalCostUSD)

	runMarked = true
	h.pool.Exec(context.Background(), //nolint:errcheck
		`UPDATE runs SET output=$2,status='success',completed_at=NOW(),total_input_tokens=$3,total_output_tokens=$4,cost_estimate=$5 WHERE id=$1::uuid`,
		parentRunID, finalOutput, totalInputTokens, totalOutputTokens, totalCostUSD)

	sseEmit(fmt.Sprintf(`{"type":"run_completed","run_id":%q,"usage":{"input":%d,"output":%d},"cost":%g}`,
		parentRunID, totalInputTokens, totalOutputTokens, totalCostUSD))
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
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer dbCancel()

	runCompleted := false
	runErrMsg := "supervisor run terminated unexpectedly"
	defer func() {
		if !runCompleted {
			h.runs.failRun(dbCtx, runID, runErrMsg) //nolint:errcheck
		}
		h.cleanupEphemeralResources(context.Background(), runID)
	}()

	sseEmitOrNil := func(s string) {
		if emit != nil {
			emit(s)
		}
	}
	sseErr := func(msg string) { sseEmitOrNil(fmt.Sprintf(`{"type":"error","error":%q}`, msg)) }

	llm, err := h.runs.providerFor(ctx, ws, a.Provider)
	if err != nil {
		runErrMsg = err.Error()
		sseErr(err.Error())
		return
	}

	contextChunks := []agentprompt.ContextChunk{}

	if a.ContextRetrievalEnabled {
		start := time.Now()
		connIDs, err := h.runs.agentConnectorIDs(ctx, a.ID)
		var queryEmbedding []float32
		if err == nil {
			queryEmbedding = tryEmbed(ctx, h.cfg, llm, input)
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

	var supConvCompaction string
	_ = h.pool.QueryRow(ctx,
		`SELECT COALESCE(compaction, '') FROM conversations WHERE id=$1::uuid`,
		convID).Scan(&supConvCompaction)

	supHistLimit := 4
	if supConvCompaction == "" {
		supHistLimit = a.MaxHistoryMessages
		if supHistLimit <= 0 {
			supHistLimit = 20
		}
	}
	historyRows, err := h.pool.Query(ctx,
		`SELECT role, content, COALESCE(tool_call_id,''), COALESCE(tool_name,''), COALESCE(tool_calls::text,'')
		 FROM messages WHERE conversation_id=$1::uuid
		 ORDER BY created_at DESC LIMIT $2`,
		convID, supHistLimit)
	if err != nil {
		runErrMsg = err.Error()
		sseErr("failed to load conversation history")
		return
	}
	defer historyRows.Close()
	history := []provider.Message{}
	for historyRows.Next() {
		var role, content, toolCallID, toolName, toolCallsRaw string
		if historyRows.Scan(&role, &content, &toolCallID, &toolName, &toolCallsRaw) == nil && (role == "user" || role == "assistant" || role == "tool") {
			content = injectActionLog(role, content, toolCallsRaw)
			if len(content) > 800 {
				content = content[:800] + "…[truncated]"
			}
			history = append(history, provider.Message{Role: role, Content: content, ToolCallID: toolCallID, ToolName: toolName})
		}
	}
	// Reverse: query returned newest-first, LLM needs oldest-first.
	for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
		history[i], history[j] = history[j], history[i]
	}

	// Regular agent tools + delegate tools merged into one list.
	regularToolDefs, dbTools, _ := loadAgentToolDefs(ctx, h.pool, a.ID)

	// Inject the live delegate tool listing so the LLM knows exactly which team
	// agents are available and their exact tool names — without requiring the
	// supervisor's Instructions to hardcode them. The "(injected at runtime)"
	// label signals to the model that these names are dynamic.
	supervisorInstructions := a.Instructions
	if len(delegateToolDefs) > 0 {
		var delegateList strings.Builder
		for _, td := range delegateToolDefs {
			delegateList.WriteString("- **" + td.Name + "**: " + td.Description + "\n")
		}
		supervisorInstructions += "\n\n## Available Team Agents (injected at runtime)\n" +
			"You MUST delegate work to your team agents using the tools below — do NOT produce a final answer without calling at least one team agent first.\n\n" +
			delegateList.String() +
			"\nCall each relevant team agent with a specific task string. After receiving all outputs, synthesize them into a comprehensive final response."
	}

	skills, _ := loadAgentSkills(ctx, h.pool, a.ID)
	supervisorInstructions += onDemandSkillsInstructions(skills.OnDemand)
	supSkillSummaries := map[string]string{}
	supSkillToolMap := map[string]string{}
	for name, skill := range skills.OnDemand {
		supSkillSummaries[name] = skill.Description
		for _, toolName := range skill.RequiredToolNames {
			supSkillToolMap[toolName] = name
		}
	}
	supActiveSkills := map[string]bool{}
	supHasCallAgent := false
	supHasCreateAgent := false
	for _, td := range regularToolDefs {
		switch td.Name {
		case "native_call_agent":
			supHasCallAgent = true
		case "native_create_agent":
			supHasCreateAgent = true
		}
	}
	supMessages, supStableSystem := agentprompt.NewBuilder().Build(agentprompt.BuildRequest{
		SystemInstructions: supervisorInstructions,
		Skills:             skills.Always,
		ContextChunks:      contextChunks,
		History:            history,
		MemoryEnabled:      a.MemoryEnabled,
		MemorySaveMode:     a.MemorySaveMode,
		HasCallAgent:       supHasCallAgent || len(delegateToolDefs) > 0,
		HasCreateAgent:     supHasCreateAgent,
		ConvCompaction:     supConvCompaction,
	})
	regularToolDefs, dbTools = ensureMemoryToolDefs(regularToolDefs, dbTools)
	toolDefs := append(regularToolDefs, delegateToolDefs...)

	supRootTraceID := runID
	var supCallAgentFn func(ctx context.Context, agentID, task string) (string, error)
	if 0 < maxInvokeDepth {
		supCallAgentFn = func(ctx context.Context, agentID, task string) (string, error) {
			return h.runAgentInline(ctx, ws, uid, agentID, task, runID, supRootTraceID, 1)
		}
	}

	var messages []provider.Message
	activeMemoryIDs := map[string]bool{}
	execCtx := tools.ExecutionContext{
		WorkspaceID:       ws,
		AgentID:           a.ID,
		AgentProvider:     a.Provider,
		AgentModel:        a.Model,
		UserID:            uid,
		RunID:             runID,
		ConversationID:    convID,
		AlwaysActiveTools: metaToolNameSet(),
		InvokeDepth:       0,
		RootRunID:         supRootTraceID,
		CallAgent:         supCallAgentFn,
		RunWorkflow: func(ctx context.Context, workflowID, input string) (string, error) {
			return h.runWorkflowInline(ctx, ws, uid, workflowID, input, runID)
		},
		CompressText: func(ctx context.Context, text string) (string, error) {
			ch, cerr := llm.Complete(ctx, provider.CompletionRequest{
				Model: a.Model,
				Messages: []provider.Message{
					{Role: "system", Content: "You are a memory compressor. Return ONLY the compressed memory — no preamble, no explanation."},
					{Role: "user", Content: "Compress this to ≤100 words, preserving all key facts:\n" + text},
				},
				Temperature: 0,
				MaxTokens:   200,
				Stream:      true,
			})
			if cerr != nil {
				return "", cerr
			}
			var result strings.Builder
			for event := range ch {
				if event.Type == provider.EventDelta {
					result.WriteString(event.Delta)
				}
				if event.Type == provider.EventError {
					return "", event.Error
				}
			}
			return strings.TrimSpace(result.String()), nil
		},
		SearchMemory: func(ctx context.Context, query string, limit int) ([]domain.Memory, error) {
			embedding := memoryEmbedding(ctx, llm, query)
			return memory.NewEngine(h.pool).Retrieve(ctx, a.ID, ws, convID, embedding, limit, a.MinRelevanceScore)
		},
		RequestMemory: func(memories []domain.Memory) bool {
			return appendMemoryContext(messages, memories, activeMemoryIDs) > 0
		},
		SkillSummaries: supSkillSummaries,
		SkillToolMap:   supSkillToolMap,
		RequestTool:    func(name string) {}, // no-op: all tools are always visible in supervisor runs
		RequestSkill: func(name string) bool {
			skill, ok := skills.OnDemand[name]
			if !ok || supActiveSkills[name] {
				return false
			}
			supActiveSkills[name] = true
			if len(messages) > 0 {
				messages[0].Content += "\n\n[Skill: " + skill.Name + "]\n" + skill.Content
			}
			h.runs.createStep(ctx, runID, domain.StepToolCall, map[string]any{"skill": skill.Name}, map[string]any{"activated": true}, time.Now(), 0, "native_request_skill", "") //nolint:errcheck
			return true
		},
	}
	_ = h.pool.QueryRow(ctx, `SELECT COALESCE(channel_session_id::text,'') FROM runs WHERE id=$1::uuid`, runID).Scan(&execCtx.ChannelSessionID)
	execCtx.SendMessage = nil      // supervisor runs are always embedded inside a workflow, never directly gateway-dispatched
	execCtx.WaitForUserInput = nil // supervisor runs cannot pause for user input

	sseEmitOrNil(fmt.Sprintf(`{"type":"run_started","run_id":%q}`, runID))

	messages = supMessages
	stepCount := 0
	totalInput, totalOutput := 0, 0
	memorySaveCalled := false
	actionLog := []string{}

	for {
		if trimmed, n := provider.TruncateMessages(messages, a.Model, a.MaxTokens); n > 0 {
			messages = trimmed
			sseEmitOrNil(fmt.Sprintf(`{"type":"delta","content":%q}`,
				fmt.Sprintf("[Context trimmed: dropped %d older messages to fit within model context window]\n\n", n)))
		}

		modelStart := time.Now()
		completion, err := completeWithEmptyRetry(ctx, llm, provider.CompletionRequest{
			Model:               a.Model,
			Messages:            messages,
			Tools:               toolDefs,
			Temperature:         a.Temperature,
			MaxTokens:           a.MaxTokens,
			Stream:              true,
			StableSystemContent: supStableSystem,
		}, func(delta string) {
			sseEmitOrNil(fmt.Sprintf(`{"type":"delta","content":%q}`, delta))
		}, "Return a direct, non-empty reply. Do not explain limitations. Reply as Deepak would naturally reply, and make sure the message is not blank.")
		if err != nil {
			runErrMsg = err.Error()
			sseErr(err.Error())
			return
		}

		reply := completion.Reply
		usage := completion.Usage
		pendingCalls := completion.ToolCalls

		totalInput += usage.InputTokens
		totalOutput += usage.OutputTokens
		h.runs.createStep(ctx, runID, domain.StepModelCall, //nolint:errcheck
			map[string]any{"provider": a.Provider, "model": a.Model, "messages": len(messages)},
			map[string]any{"content": reply, "tool_calls": len(pendingCalls)},
			modelStart, usage.InputTokens+usage.OutputTokens, "", "")

		if len(pendingCalls) == 0 {
			if strings.TrimSpace(reply) == "" {
				msg := "model returned an empty response"
				runErrMsg = msg
				sseErr(msg)
				return
			}
			var actionLogJSON any
			if len(actionLog) > 0 {
				b, _ := json.Marshal(actionLog)
				actionLogJSON = b
			}
			h.pool.Exec(ctx, //nolint:errcheck
				`INSERT INTO messages(id,conversation_id,role,content,tool_calls,tokens) VALUES($1::uuid,$2::uuid,'assistant',$3,$4::jsonb,$5)`,
				uuid.NewString(), convID, reply, actionLogJSON, usage.OutputTokens)
			h.runs.createStep(ctx, runID, domain.StepFinalResponse, //nolint:errcheck
				map[string]any{},
				map[string]any{"content": reply},
				time.Now(), usage.OutputTokens, "", "")
			runCompleted = true
			costUSD := cost.Estimate(a.Provider, a.Model, totalInput, totalOutput)
			h.pool.Exec(dbCtx, //nolint:errcheck
				`UPDATE runs SET output=$2,status='success',completed_at=NOW(),total_input_tokens=$3,total_output_tokens=$4,cost_estimate=$5 WHERE id=$1::uuid`,
				runID, reply, totalInput, totalOutput, costUSD)
			sseEmitOrNil(fmt.Sprintf(`{"type":"run_completed","run_id":%q,"usage":{"input":%d,"output":%d},"cost":%g}`,
				runID, totalInput, totalOutput, costUSD))
			if shouldRunMemoryExtractor(a, memorySaveCalled) {
				aCopy, llmCopy, replySnap, inputSnap := a, llm, reply, input
				go func() {
					start := time.Now()
					count, err := runMemoryExtractor(context.Background(), h.pool, llmCopy, aCopy, ws, uid, convID, runID, inputSnap, replySnap)
					h.runs.createStep(context.Background(), runID, domain.StepToolCall, //nolint:errcheck
						map[string]any{"tool": "memory_extractor"},
						map[string]any{"saved": count},
						start, 0, "memory_extractor", errString(err))
				}()
			}
			{
				threshold := a.CompactionThreshold
				if threshold <= 0 {
					threshold = 6
				}
				tokenThreshold := a.CompactionTokenThreshold
				if tokenThreshold <= 0 {
					tokenThreshold = 3000
				}
				if supConvCompaction != "" || len(history) >= threshold || totalInput > tokenThreshold {
					llmCopy, modelSnap, convSnap, compSnap := llm, a.Model, convID, supConvCompaction
					go func() {
						sseEmitOrNil(`{"type":"compacting","status":"start"}`)
						newCompaction, err := compactConversation(context.Background(), h.pool, llmCopy, modelSnap, convSnap, compSnap)
						if err != nil || newCompaction == "" {
							sseEmitOrNil(`{"type":"compacting","status":"done"}`)
							return
						}
						h.pool.Exec(context.Background(), //nolint:errcheck
							`UPDATE conversations SET compaction=$2, updated_at=NOW() WHERE id=$1::uuid`,
							convSnap, newCompaction)
						sseEmitOrNil(`{"type":"compacting","status":"done"}`)
					}()
				}
			}
			return
		}

		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   reply,
			ToolCalls: pendingCalls,
		})

		// Pre-compute: if there are multiple delegate calls in this batch, run them concurrently.
		parallelDelegateResults := map[string]string{}
		{
			var delegateCallsInBatch []provider.ToolCall
			for _, call := range pendingCalls {
				if _, isDelegate := delegateHandlers[call.Name]; isDelegate {
					delegateCallsInBatch = append(delegateCallsInBatch, call)
				}
			}
			if len(delegateCallsInBatch) > 1 {
				type delegateOut struct {
					id      string
					content string
					latency int
				}
				resultsCh := make(chan delegateOut, len(delegateCallsInBatch))
				for _, call := range delegateCallsInBatch {
					capturedCall := call
					capturedHandler := delegateHandlers[capturedCall.Name]
					go func() {
						start := time.Now()
						output := capturedHandler(ctx, capturedCall.Input)
						resultsCh <- delegateOut{id: capturedCall.ID, content: output, latency: int(time.Since(start).Milliseconds())}
					}()
				}
				for range delegateCallsInBatch {
					out := <-resultsCh
					parallelDelegateResults[out.id] = out.content
				}
			}
		}

		for _, call := range pendingCalls {
			if call.Name == "native_save_memory" {
				memorySaveCalled = true
			}
			// Delegate tool — execute the team agent and return its output.
			if handler, isDelegate := delegateHandlers[call.Name]; isDelegate {
				sseEmitOrNil(fmt.Sprintf(`{"type":"tool_started","call_id":%q,"tool":%q,"input":%s}`,
					call.ID, call.Name, jsonOrStr(call.Input)))
				delegateStart := time.Now()
				var delegateOutput string
				if precomputed, ok := parallelDelegateResults[call.ID]; ok {
					delegateOutput = precomputed
				} else {
					delegateOutput = handler(ctx, call.Input)
				}
				actionLog = append(actionLog, summarizeToolCall(call.Name, call.Input, ""))
				h.runs.createStep(ctx, runID, domain.StepToolCall, //nolint:errcheck
					map[string]any{"tool": call.Name, "input": call.Input},
					map[string]any{"output": delegateOutput},
					delegateStart, 0, call.Name, "")
				sseEmitOrNil(fmt.Sprintf(`{"type":"tool_call","call_id":%q,"tool":%q,"input":%s,"output":%s,"latency_ms":%d}`,
					call.ID, call.Name, jsonOrStr(call.Input), jsonOrStr([]byte(delegateOutput)),
					int(time.Since(delegateStart).Milliseconds())))
				messages = append(messages, provider.Message{
					Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: delegateOutput,
				})
				stepCount++
				if stepCount > a.MaxSteps {
					runErrMsg = "max steps exceeded"
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
					runErrMsg = "approval timed out"
					sseErr("approval timed out after 10 minutes")
					return
				}
				h.pool.Exec(ctx, `UPDATE runs SET status='running' WHERE id=$1::uuid`, runID) //nolint:errcheck

				if decision.Decision == "rejected" {
					messages = append(messages, provider.Message{Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: "Tool call rejected by user."})
					continue
				}
			}

			sseEmitOrNil(fmt.Sprintf(`{"type":"tool_started","call_id":%q,"tool":%q,"input":%s}`,
				call.ID, call.Name, jsonOrStr(call.Input)))
			if execCtx.SendMessage != nil {
				if label := progressLabel(call.Name); label != "" {
					execCtx.SendMessage(ctx, label) //nolint:errcheck
				}
			}

			var result *tools.ExecutionResult
			var execErr error
			if toolExists && dbTool.Type == "http" {
				var cfg tools.HTTPToolConfig
				_ = json.Unmarshal(dbTool.Config, &cfg)
				result = tools.ExecuteHTTP(ctx, cfg, call.Input, dbTool.TimeoutMs)
			} else if toolExists && dbTool.Type == "code" {
				var codeCfg struct {
					Code string `json:"code"`
				}
				_ = json.Unmarshal(dbTool.Config, &codeCfg)
				start := time.Now()
				out, codeErr := native.ExecuteCodeTool(ctx, codeCfg.Code, call.Input)
				result = &tools.ExecutionResult{LatencyMs: int(time.Since(start).Milliseconds())}
				if codeErr != nil {
					result.Error = codeErr.Error()
				} else {
					result.Output = out
				}
			} else {
				result, execErr = h.executor.ExecuteWithContext(ctx, execCtx, call.Name, call.Input)
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
			if !execCtx.AlwaysActiveTools[call.Name] {
				actionLog = append(actionLog, summarizeToolCall(call.Name, call.Input, errMsg))
			}

			h.runs.createStep(ctx, runID, domain.StepToolCall, //nolint:errcheck
				map[string]any{"tool": call.Name, "input": call.Input},
				map[string]any{"output": resultContent},
				time.Now(), 0, call.Name, errMsg)

			sseEmitOrNil(fmt.Sprintf(`{"type":"tool_call","call_id":%q,"tool":%q,"input":%s,"output":%s,"latency_ms":%d}`,
				call.ID, call.Name, jsonOrStr(call.Input), jsonOrStr([]byte(resultContent)), latencyMs))

			messages = append(messages, provider.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Content:    resultContent,
				IsError:    errMsg != "",
			})

			stepCount++
			if stepCount > a.MaxSteps {
				runErrMsg = "max steps exceeded"
				sseErr("max steps exceeded")
				return
			}
		}
		// Reload tool definitions to pick up tools created or attached mid-run.
		if freshAll, freshDB, err := loadAgentToolDefs(ctx, h.pool, a.ID); err == nil {
			freshAll, freshDB = ensureMemoryToolDefs(freshAll, freshDB)
			regularToolDefs, dbTools = freshAll, freshDB
			toolDefs = append(regularToolDefs, delegateToolDefs...)
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
