package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/anthropic"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/gemini"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/ollama"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/openai"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	agentprompt "github.com/deepaksingh/agent-nexus/services/api/internal/runtime/agent"
	contextretrieval "github.com/deepaksingh/agent-nexus/services/api/internal/runtime/context"
	"github.com/deepaksingh/agent-nexus/services/api/internal/runtime/cost"
	"github.com/deepaksingh/agent-nexus/services/api/internal/runtime/memory"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools/native"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/encrypt"
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
}

func NewRunsHandler(p *pgxpool.Pool, c *config.Config, reg *tools.Registry, exec *tools.Executor) *RunsHandler {
	return &RunsHandler{pool: p, cfg: c, conversations: repository.NewConversationRepository(p), registry: reg, executor: exec}
}

// SetInvokeHandler wires the InvokeHandler so that native_call_agent works in conversation runs.
func (h *RunsHandler) SetInvokeHandler(ih *InvokeHandler) { h.invokeH = ih }

const runSelect = `SELECT id::text,workspace_id::text,COALESCE(agent_id::text,''),conversation_id::text,user_id::text,input,output,status,started_at,completed_at,total_input_tokens,total_output_tokens,cost_estimate,error_message,COALESCE(trigger_id::text,''),COALESCE(parent_run_id::text,''),workflow_node_id,COALESCE(trace_id::text,'') FROM runs`

func scanRun(row interface{ Scan(...any) error }) (domain.Run, error) {
	var x domain.Run
	e := row.Scan(&x.ID, &x.WorkspaceID, &x.AgentID, &x.ConversationID, &x.UserID, &x.Input, &x.Output, &x.Status, &x.StartedAt, &x.CompletedAt, &x.TotalInputTokens, &x.TotalOutputTokens, &x.CostEstimate, &x.ErrorMessage, &x.TriggerID, &x.ParentRunID, &x.WorkflowNodeID, &x.TraceID)
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
	runParked := false
	defer func() {
		// A parked run keeps its ephemeral resources — it resumes later via the
		// Approve endpoint with tool state reloaded from the DB.
		if !runParked && h.invokeH != nil {
			h.invokeH.cleanupEphemeralResources(context.Background(), id)
		}
	}()

	contextChunks := []agentprompt.ContextChunk{}
	var runConnSettings connectorSettings
	if a.ContextRetrievalEnabled {
		runConnSettings, _ = h.agentConnectorSettings(r.Context(), a.ID)
	}

	if a.ContextRetrievalEnabled && !a.AgenticRAG {
		start := time.Now()
		var queryEmbedding []float32
		if len(runConnSettings.IDs) > 0 {
			queryEmbedding = tryEmbed(r.Context(), h.cfg, llm, q.Input)
		}
		chunks, err := contextretrieval.NewRetriever(h.pool).Retrieve(r.Context(), ws, runConnSettings.IDs, queryEmbedding, runConnSettings.MaxChunks, runConnSettings.MinScore, q.Input)
		if err == nil {
			for _, chunk := range chunks {
				contextChunks = append(contextChunks, formatChunk(chunk))
			}
		}
		_ = h.createStep(r.Context(), id, domain.StepContextRetrieval, map[string]any{"connector_ids": runConnSettings.IDs}, map[string]any{"count": len(contextChunks)}, start, 0, "", errString(err))
	}

	var convCompaction string
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COALESCE(compaction, '') FROM conversations WHERE id=$1::uuid`,
		c.ID).Scan(&convCompaction)

	histLimit := 4
	if convCompaction == "" {
		histLimit = a.MaxHistoryMessages
		if histLimit <= 0 {
			histLimit = 20
		}
	}
	historyRows, e := h.pool.Query(r.Context(),
		`SELECT role, content, COALESCE(tool_call_id,''), COALESCE(tool_name,''), COALESCE(tool_calls::text,'')
		 FROM messages WHERE conversation_id=$1::uuid
		 ORDER BY created_at DESC LIMIT $2`,
		c.ID, histLimit)
	if e != nil {
		sseErr("failed to load conversation history")
		_ = h.failRun(r.Context(), id, e.Error())
		return
	}
	defer historyRows.Close()
	history := make([]provider.Message, 0)
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

	// Load tool definitions for this agent (before building prompt so we can inject hints)
	allToolDefs, dbTools, _ := loadAgentToolDefs(r.Context(), h.pool, a.ID)
	allToolDefs, dbTools = ensureMemoryToolDefs(allToolDefs, dbTools)
	if a.ContextRetrievalEnabled && a.AgenticRAG {
		if rt, err := h.registry.Get("native_retrieve_context"); err == nil {
			def := rt.Definition()
			allToolDefs = append(allToolDefs, provider.ToolDefinition{Name: def.Name, Description: def.Description, InputSchema: def.InputSchema})
			dbTools[def.Name] = def
		}
	}

	hasCallAgent := false
	for _, td := range allToolDefs {
		if td.Name == "native_call_agent" {
			hasCallAgent = true
			break
		}
	}

	skills, _ := loadAgentSkills(r.Context(), h.pool, a.ID)
	skillCatalog, _ := loadWorkspaceSkillCatalog(r.Context(), h.pool, ws)
	instructions := a.Instructions + onDemandSkillsInstructions(skills.OnDemand)
	if a.ContextRetrievalEnabled && a.AgenticRAG {
		instructions += "\n\nTo look up information from your connected knowledge sources, use the `native_retrieve_context` tool. Always call it before answering questions that require specific knowledge from your documents, codebase, or indexed files. Do NOT call any tool named \"search\" — use `native_retrieve_context` instead."
	}
	initialMessages, stableSystem := agentprompt.NewBuilder().Build(agentprompt.BuildRequest{
		SystemInstructions: instructions,
		Skills:             skills.Always,
		ContextChunks:      contextChunks,
		History:            history,
		MemoryEnabled:      a.MemoryEnabled,
		MemorySaveMode:     a.MemorySaveMode,
		HasCallAgent:       hasCallAgent,
		LazyToolLoading:    a.LazyToolLoading,
		ConvCompaction:     convCompaction,
	})

	capturedWs, capturedUID, capturedRunID := ws, uid, id
	var callAgentFn func(ctx context.Context, agentID, task string) (string, error)
	if h.invokeH != nil {
		callAgentFn = func(ctx context.Context, agentID, task string) (string, error) {
			return h.invokeH.runAgentInline(ctx, capturedWs, capturedUID, agentID, task, capturedRunID, capturedRunID, 1)
		}
	}

	// Discovery is workspace-wide (not just tools/skills already attached to this agent) so
	// native_list_tools/native_request_tool and native_list_agent_skills/native_request_skill
	// let the agent self-serve the full catalog.
	toolCatalogDefs, toolCatalog, _ := loadWorkspaceToolCatalog(r.Context(), h.pool, ws)
	toolSummaries := make(map[string]string, len(toolCatalogDefs)+len(allToolDefs))
	for _, td := range toolCatalogDefs {
		toolSummaries[td.Name] = td.Description
	}
	for _, td := range allToolDefs {
		if _, ok := toolSummaries[td.Name]; !ok {
			toolSummaries[td.Name] = td.Description
		}
	}
	skillSummaries := map[string]string{}
	skillToolMap := map[string]string{}
	for name, skill := range skillCatalog {
		skillSummaries[name] = skill.Description
		for _, toolName := range skill.RequiredToolNames {
			skillToolMap[toolName] = name
		}
	}
	hiddenToolNames := map[string]bool{}
	for _, skill := range skills.OnDemand {
		for _, name := range skill.RequiredToolNames {
			if !skills.AlwaysRequired[name] {
				hiddenToolNames[name] = true
			}
		}
	}
	requestedTools := map[string]bool{}
	if a.ContextRetrievalEnabled && a.AgenticRAG {
		requestedTools["native_retrieve_context"] = true
	}
	activeSkills := map[string]bool{}
	activeSkillTools := map[string]bool{}
	visibleToolDefs := func() []provider.ToolDefinition {
		out := make([]provider.ToolDefinition, 0, len(allToolDefs))
		for _, td := range allToolDefs {
			if !hiddenToolNames[td.Name] || activeSkillTools[td.Name] {
				out = append(out, td)
			}
		}
		return out
	}
	activeMemoryIDs := map[string]bool{}
	var messages []provider.Message
	execCtx := tools.ExecutionContext{
		WorkspaceID:       ws,
		AgentID:           a.ID,
		AgentProvider:     a.Provider,
		AgentModel:        a.Model,
		UserID:            uid,
		RunID:             id,
		ConversationID:    c.ID,
		ConnectorIDs:      runConnSettings.IDs,
		ToolSummaries:     toolSummaries,
		AlwaysActiveTools: metaToolNameSet(),
		SkillSummaries:    skillSummaries,
		SkillToolMap:      skillToolMap,
		InvokeDepth:       0,
		RootRunID:         id,
		CallAgent:         callAgentFn,
		Embed: func(ctx context.Context, text string) ([]float32, error) {
			v := tryEmbed(ctx, h.cfg, llm, text)
			return v, nil
		},
		RunWorkflow: func(ctx context.Context, workflowID, input string) (string, error) {
			if h.invokeH == nil {
				return "", fmt.Errorf("workflow execution not available")
			}
			return h.invokeH.runWorkflowInline(ctx, capturedWs, capturedUID, workflowID, input, capturedRunID)
		},
		SendMessage: nil, // playground runs don't have a gateway channel
		SearchMemory: func(ctx context.Context, query string, limit int) ([]domain.Memory, error) {
			embedding := memoryEmbedding(ctx, llm, query)
			return memory.NewEngine(h.pool).Retrieve(ctx, a.ID, ws, c.ID, embedding, limit, a.MinRelevanceScore)
		},
		RequestMemory: func(memories []domain.Memory) bool {
			return appendMemoryContext(messages, memories, activeMemoryIDs) > 0
		},
		RequestTool: func(name string) { requestedTools[name] = true },
		RequestSkill: func(name string) bool {
			skill, ok := skills.OnDemand[name]
			if !ok {
				skill, ok = skillCatalog[name]
			}
			if !ok || activeSkills[name] {
				return false
			}
			activeSkills[name] = true
			for _, toolName := range skill.RequiredToolNames {
				activeSkillTools[toolName] = true
				requestedTools[toolName] = true
				// Ensure the tool appears in toolSummaries so native_list_tools and
				// native_request_tool can find it even if it isn't in agent_tools.
				if _, known := toolSummaries[toolName]; !known {
					if tool, err := h.registry.Get(toolName); err == nil {
						def := tool.Definition()
						toolSummaries[def.Name] = def.Description
					}
				} else {
					for _, td := range allToolDefs {
						if td.Name == toolName {
							toolSummaries[td.Name] = td.Description
						}
					}
				}
			}
			if len(messages) > 0 {
				messages[0].Content += "\n\n[Skill: " + skill.Name + "]\n" + skill.Content
			}
			_ = h.createStep(r.Context(), id, domain.StepToolCall, map[string]any{"skill": skill.Name}, map[string]any{"activated": true}, time.Now(), 0, "native_request_skill", "")
			return true
		},
		WaitForUserInput: func(ctx context.Context, question string) (string, error) {
			emit(fmt.Sprintf(`{"type":"user_input_required","run_id":%q,"question":%s}`,
				capturedRunID, jsonOrStr([]byte(`"`+question+`"`))))
			ch := RegisterUserInputWait(capturedRunID)
			h.pool.Exec(ctx, `UPDATE runs SET status='user_input_wait' WHERE id=$1::uuid`, capturedRunID) //nolint:errcheck
			select {
			case answer := <-ch:
				h.pool.Exec(ctx, `UPDATE runs SET status='running' WHERE id=$1::uuid`, capturedRunID) //nolint:errcheck
				return answer, nil
			case <-time.After(30 * time.Minute):
				UnregisterUserInputWait(capturedRunID)
				return "", fmt.Errorf("user input timed out after 30 minutes")
			}
		},
	}

	emit(fmt.Sprintf(`{"type":"run_started","run_id":%q}`, id))

	// ── Tool calling loop ──────────────────────────────────────────────────────
	messages = initialMessages
	stepCount := 0
	totalInput, totalOutput := 0, 0
	memorySaveCalled := false
	futureWorkRetried := false
	fabricationRetried := false
	actionLog := []string{}

	// Session-wait plumbing for native_launch_repo_session — mirrors the
	// executeRun loop: snapshot at the current call, block briefly on the
	// runner callback, then park by returning tools.ErrRunParked. A parked
	// playground run resumes headlessly via ResumeSessionRun (executeRun),
	// exactly like a parked approval does.
	var curBatch []provider.ToolCall
	var curCallIdx int
	execCtx.WaitForSession = func(waitCtx context.Context, sessionKey string) (string, error) {
		st := &runWaitState{
			Version:          1,
			WaitType:         "session",
			WorkspaceID:      ws,
			UserID:           uid,
			AgentID:          a.ID,
			ConversationID:   c.ID,
			Input:            q.Input,
			Messages:         messages,
			StableSystem:     stableSystem,
			PendingCalls:     curBatch,
			CallIndex:        curCallIdx,
			StepCount:        stepCount,
			TotalInput:       totalInput,
			TotalOutput:      totalOutput,
			ActionLog:        actionLog,
			MemorySaveCalled: memorySaveCalled,
			RequestedTools:   keysOf(requestedTools),
			ActiveSkills:     keysOf(activeSkills),
			ActiveMemoryIDs:  keysOf(activeMemoryIDs),
		}
		if err := saveWaitState(context.Background(), h.pool, id, st); err != nil {
			return "", fmt.Errorf("failed to persist session wait state: %w", err)
		}
		ch := RegisterSessionWait(id)
		h.pool.Exec(waitCtx, `UPDATE runs SET status='session_wait' WHERE id=$1::uuid`, id) //nolint:errcheck
		emit(fmt.Sprintf(`{"type":"session_wait","run_id":%q,"session":%q}`, id, sessionKey))

		content, got := awaitSessionResult(id, ch, 60*time.Second)
		if !got {
			return "", tools.ErrRunParked
		}
		deleteWaitState(context.Background(), h.pool, id)
		h.pool.Exec(waitCtx, `UPDATE runs SET status='running' WHERE id=$1::uuid`, id) //nolint:errcheck
		return content, nil
	}

	for {
		var toolDefs []provider.ToolDefinition
		if a.LazyToolLoading {
			toolDefs = lazyMetaToolDefs(h.registry)
			coveredByDB := map[string]bool{}
			for _, td := range allToolDefs {
				coveredByDB[td.Name] = true
				if requestedTools[td.Name] {
					toolDefs = append(toolDefs, td)
				}
			}
			// Fallback: requested tools not in agent_tools (e.g. discovered via the workspace
			// catalog, or skill-required tools not yet auto-attached) are loaded from the
			// workspace catalog first (so http/mcp/code tools carry their config for dispatch),
			// then the native registry as a last resort.
			for name := range requestedTools {
				if coveredByDB[name] {
					continue
				}
				if fullTool, ok := toolCatalog[name]; ok {
					toolDefs = append(toolDefs, provider.ToolDefinition{
						Name:        fullTool.Name,
						Description: fullTool.Description,
						InputSchema: fullTool.InputSchema,
					})
					dbTools[name] = fullTool
				} else if tool, err := h.registry.Get(name); err == nil {
					def := tool.Definition()
					toolDefs = append(toolDefs, provider.ToolDefinition{
						Name:        def.Name,
						Description: def.Description,
						InputSchema: def.InputSchema,
					})
				}
			}
		} else {
			toolDefs = append(lazyMetaToolDefs(h.registry), visibleToolDefs()...)
		}
		toolDefs = dedupeToolDefs(toolDefs)
		allowedToolNames := map[string]bool{}
		for _, td := range toolDefs {
			allowedToolNames[td.Name] = true
		}
		if trimmed, n := provider.TruncateMessages(messages, a.Model, a.MaxTokens); n > 0 {
			messages = trimmed
			emit(fmt.Sprintf(`{"type":"delta","content":%q}`,
				fmt.Sprintf("[Context trimmed: dropped %d older messages to fit within model context window]\n\n", n)))
		}

		modelStart := time.Now()
		completion, e := completeWithEmptyRetry(r.Context(), llm, provider.CompletionRequest{
			Model:               a.Model,
			Messages:            messages,
			Tools:               toolDefs,
			Temperature:         a.Temperature,
			MaxTokens:           a.MaxTokens,
			Stream:              true,
			StableSystemContent: stableSystem,
		}, func(delta string) {
			emit(fmt.Sprintf(`{"type":"delta","content":%q}`, delta))
		}, "Return a direct, non-empty reply. Do not explain limitations or say you cannot help. If you started a multi-step task, continue to the next step rather than asking for confirmation.")
		if e != nil {
			_ = h.failRun(r.Context(), id, e.Error())
			sseErr(e.Error())
			return
		}

		reply := completion.Reply
		usage := completion.Usage
		pendingCalls := completion.ToolCalls

		totalInput += usage.InputTokens
		totalOutput += usage.OutputTokens
		_ = h.createStep(r.Context(), id, domain.StepModelCall,
			map[string]any{"provider": a.Provider, "model": a.Model, "messages": len(messages)},
			map[string]any{"content": reply, "tool_calls": len(pendingCalls)},
			modelStart, usage.InputTokens+usage.OutputTokens, "", "")

		if len(pendingCalls) == 0 {
			// Fabrication guard: a reply written in the internal action-log
			// replay format means the model narrated tool activity it never
			// performed. Retry once with a correction; never deliver it.
			if fabricatesActionLog(reply) {
				if !fabricationRetried {
					fabricationRetried = true
					messages[0].Content += fabricatedActionCorrection
					continue
				}
				reply = fabricatedActionReply
			}
			if stepCount == 0 && promisesUnconfirmedFutureWork(reply) {
				if !futureWorkRetried {
					futureWorkRetried = true
					messages[0].Content += futureWorkCorrection
					continue
				}
				reply = "I don't have enough information to answer that."
			}
			// No tool calls — this is the final response
			if strings.TrimSpace(reply) == "" {
				msg := "model returned an empty response"
				_ = h.failRun(r.Context(), id, msg)
				sseErr(msg)
				return
			}
			var actionLogJSON json.RawMessage
			if len(actionLog) > 0 {
				actionLogJSON, _ = json.Marshal(actionLog)
			}
			if e := h.conversations.AddMessage(r.Context(), &domain.Message{ID: uuid.NewString(), ConversationID: c.ID, Role: "assistant", Content: reply, ToolCalls: actionLogJSON, Tokens: usage.OutputTokens}); e != nil {
				_ = h.failRun(r.Context(), id, e.Error())
				sseErr("failed to save assistant message")
				return
			}
			_ = h.createStep(r.Context(), id, domain.StepFinalResponse, map[string]any{}, map[string]any{"content": reply}, time.Now(), usage.OutputTokens, "", "")
			costUSD := cost.Estimate(a.Provider, a.Model, totalInput, totalOutput)
			_, _ = h.pool.Exec(r.Context(), `UPDATE runs SET output=$2,status='success',completed_at=NOW(),total_input_tokens=$3,total_output_tokens=$4,cost_estimate=$5 WHERE id=$1::uuid`, id, reply, totalInput, totalOutput, costUSD)
			emit(fmt.Sprintf(`{"type":"run_completed","run_id":%q,"usage":{"input":%d,"output":%d},"cost":%g}`, id, totalInput, totalOutput, costUSD))
			if shouldRunMemoryExtractor(a, memorySaveCalled) {
				aCopy, llmCopy, inputSnap, replySnap := a, llm, q.Input, reply
				go func() {
					start := time.Now()
					count, err := runMemoryExtractor(context.Background(), h.pool, llmCopy, aCopy, ws, uid, c.ID, id, inputSnap, replySnap)
					_ = h.createStep(context.Background(), id, domain.StepToolCall, map[string]any{"tool": "memory_extractor"}, map[string]any{"saved": count}, start, 0, "memory_extractor", errString(err))
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
					llmCopy, modelSnap, convSnap, compSnap := llm, a.Model, c.ID, convCompaction
					go func() {
						newCompaction, err := compactConversation(context.Background(), h.pool, llmCopy, modelSnap, convSnap, compSnap)
						if err != nil || newCompaction == "" {
							return
						}
						h.pool.Exec(context.Background(), //nolint:errcheck
							`UPDATE conversations SET compaction=$2, updated_at=NOW() WHERE id=$1::uuid`,
							convSnap, newCompaction)
					}()
				}
			}
			return
		}

		// Append assistant turn (with tool calls) to the message history
		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   reply,
			ToolCalls: pendingCalls,
		})

		curBatch = pendingCalls
		for callIdx := 0; callIdx < len(pendingCalls); callIdx++ {
			call := pendingCalls[callIdx]
			curCallIdx = callIdx
			if call.Name == "native_save_memory" {
				memorySaveCalled = true
			}

			// Lazy loading gate: reject calls to tools that weren't actually offered to the
			// model this turn (e.g. it guessed a name from native_list_tools output) so it's
			// forced through native_request_tool/native_request_skill — keeping traces honest
			// about what was activated.
			if a.LazyToolLoading && !allowedToolNames[call.Name] {
				gateErr := lazyToolNotActiveError(call.Name, execCtx.SkillToolMap)
				_ = h.createStep(r.Context(), id, domain.StepToolCall,
					map[string]any{"tool": call.Name, "input": call.Input},
					map[string]any{"error": gateErr},
					time.Now(), 0, call.Name, gateErr)
				emit(fmt.Sprintf(`{"type":"tool_call","call_id":%q,"tool":%q,"input":%s,"output":%s,"latency_ms":0}`,
					call.ID, call.Name, jsonOrStr(call.Input), jsonOrStr([]byte(fmt.Sprintf(`{"error":%q}`, gateErr)))))
				messages = append(messages, provider.Message{
					Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: fmt.Sprintf(`{"error":%q}`, gateErr),
				})
				stepCount++
				if stepCount > a.MaxSteps {
					_ = h.failRun(r.Context(), id, "max steps exceeded")
					sseErr("max steps exceeded")
					return
				}
				continue
			}

			dbTool, toolExists := dbTools[call.Name]

			if !toolExists {
				if gateErr, matched := unattachedCatalogToolError(call.Name, toolCatalog, a.LazyToolLoading); matched {
					_ = h.createStep(r.Context(), id, domain.StepToolCall,
						map[string]any{"tool": call.Name, "input": call.Input},
						map[string]any{"error": gateErr},
						time.Now(), 0, call.Name, gateErr)
					emit(fmt.Sprintf(`{"type":"tool_call","call_id":%q,"tool":%q,"input":%s,"output":%s,"latency_ms":0}`,
						call.ID, call.Name, jsonOrStr(call.Input), jsonOrStr([]byte(fmt.Sprintf(`{"error":%q}`, gateErr)))))
					messages = append(messages, provider.Message{
						Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: fmt.Sprintf(`{"error":%q}`, gateErr),
					})
					stepCount++
					if stepCount > a.MaxSteps {
						_ = h.failRun(r.Context(), id, "max steps exceeded")
						sseErr("max steps exceeded")
						return
					}
					continue
				}
			}

			// Approval gate for high-risk tools
			if toolExists && dbTool.RequiresApproval {
				arID := uuid.NewString()
				h.pool.Exec(r.Context(), //nolint:errcheck
					`INSERT INTO approval_requests(id,run_id,tool_name,tool_input,status)VALUES($1::uuid,$2::uuid,$3,$4::jsonb,'pending')`,
					arID, id, call.Name, call.Input)
				h.pool.Exec(r.Context(), `UPDATE runs SET status='approval_wait' WHERE id=$1::uuid`, id) //nolint:errcheck

				// Snapshot the loop state so the run survives a process restart and
				// can park if nobody approves before the in-process timeout. Uses a
				// background context: the SSE request context dies with the client.
				st := &runWaitState{
					Version:          1,
					WorkspaceID:      ws,
					UserID:           uid,
					AgentID:          a.ID,
					ConversationID:   c.ID,
					Input:            q.Input,
					Messages:         messages,
					StableSystem:     stableSystem,
					PendingCalls:     pendingCalls,
					CallIndex:        callIdx,
					StepCount:        stepCount,
					TotalInput:       totalInput,
					TotalOutput:      totalOutput,
					ActionLog:        actionLog,
					MemorySaveCalled: memorySaveCalled,
					RequestedTools:   keysOf(requestedTools),
					ActiveSkills:     keysOf(activeSkills),
					ActiveMemoryIDs:  keysOf(activeMemoryIDs),
				}
				if err := saveWaitState(context.Background(), h.pool, id, st); err != nil {
					slog.Warn("failed to persist approval wait state; run will not survive restart",
						"run_id", id, "error", err)
				}

				ch := RegisterApprovalWait(id)
				emit(fmt.Sprintf(`{"type":"approval_required","tool":%q,"input":%s,"approval_id":%q}`,
					call.Name, string(call.Input), arID))

				decision, got := awaitApprovalDecision(id, ch, 10*time.Minute)
				if !got {
					// Park: end the SSE stream and free this goroutine, leaving the
					// run in approval_wait with its state persisted. The Approve
					// endpoint resumes it headlessly (see ResumeApprovedRun).
					emit(fmt.Sprintf(`{"type":"approval_parked","run_id":%q,"approval_id":%q}`, id, arID))
					runParked = true
					return
				}
				deleteWaitState(context.Background(), h.pool, id)
				h.pool.Exec(r.Context(), `UPDATE runs SET status='running' WHERE id=$1::uuid`, id) //nolint:errcheck

				if decision.Decision == "rejected" {
					messages = append(messages, provider.Message{Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: "Tool call rejected by user."})
					continue
				}
			}

			emit(fmt.Sprintf(`{"type":"tool_started","call_id":%q,"tool":%q,"input":%s}`,
				call.ID, call.Name, jsonOrStr(call.Input)))

			// Execute the tool — HTTP, code, and MCP tools bypass the native registry
			var result *tools.ExecutionResult
			var execErr error
			if toolExists && dbTool.Type == "http" {
				var cfg tools.HTTPToolConfig
				_ = json.Unmarshal(dbTool.Config, &cfg)
				result = tools.ExecuteHTTP(r.Context(), cfg, call.Input, dbTool.TimeoutMs)
			} else if toolExists && dbTool.Type == "mcp" {
				result = executeMCPTool(r.Context(), h.pool, h.cfg, dbTool, call.Input)
			} else if toolExists && dbTool.Type == "code" {
				var codeCfg struct {
					Code string `json:"code"`
				}
				_ = json.Unmarshal(dbTool.Config, &codeCfg)
				start := time.Now()
				out, codeErr := native.ExecuteCodeTool(r.Context(), codeCfg.Code, call.Input)
				result = &tools.ExecutionResult{LatencyMs: int(time.Since(start).Milliseconds())}
				if codeErr != nil {
					result.Error = codeErr.Error()
				} else {
					result.Output = out
				}
			} else {
				result, execErr = h.executor.ExecuteWithContext(r.Context(), execCtx, call.Name, call.Input)
			}
			if errors.Is(execErr, tools.ErrRunParked) {
				// The session tool persisted its wait state and the in-process
				// wait expired. End the SSE stream without failing the run —
				// the runner's callback resumes it via ResumeSessionRun.
				emit(fmt.Sprintf(`{"type":"session_parked","run_id":%q}`, id))
				runParked = true
				return
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

			_ = h.createStep(r.Context(), id, domain.StepToolCall,
				map[string]any{"tool": call.Name, "input": call.Input},
				map[string]any{"output": resultContent},
				time.Now(), 0, call.Name, errMsg)

			emit(fmt.Sprintf(`{"type":"tool_call","call_id":%q,"tool":%q,"input":%s,"output":%s,"latency_ms":%d}`,
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
				_ = h.failRun(r.Context(), id, "max steps exceeded")
				sseErr("max steps exceeded")
				return
			}
		}
		// Reload tool definitions to pick up tools created or attached mid-run.
		if freshDefs, freshDB, err := loadAgentToolDefs(r.Context(), h.pool, a.ID); err == nil {
			freshDefs, freshDB = ensureMemoryToolDefs(freshDefs, freshDB)
			if a.ContextRetrievalEnabled && a.AgenticRAG {
				if rt, err := h.registry.Get("native_retrieve_context"); err == nil {
					def := rt.Definition()
					freshDefs = append(freshDefs, provider.ToolDefinition{Name: def.Name, Description: def.Description, InputSchema: def.InputSchema})
					freshDB[def.Name] = def
				}
			}
			allToolDefs, dbTools = freshDefs, freshDB
			for name := range toolSummaries {
				delete(toolSummaries, name)
			}
			for _, td := range allToolDefs {
				toolSummaries[td.Name] = td.Description
			}
		}
		// Refresh the workspace catalogs too, so tools/skills created mid-run become
		// discoverable and requestable even before/without being attached to this agent.
		if freshCatalogDefs, freshCatalog, err := loadWorkspaceToolCatalog(r.Context(), h.pool, ws); err == nil {
			toolCatalog = freshCatalog
			for _, td := range freshCatalogDefs {
				toolSummaries[td.Name] = td.Description
			}
		}
		if freshSkillCatalog, err := loadWorkspaceSkillCatalog(r.Context(), h.pool, ws); err == nil {
			skillCatalog = freshSkillCatalog
			for name, skill := range skillCatalog {
				skillSummaries[name] = skill.Description
				for _, toolName := range skill.RequiredToolNames {
					skillToolMap[toolName] = name
				}
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
	if status != "user_input_wait" {
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
	t, e := h.pool.Exec(r.Context(), `UPDATE runs SET status='cancelled',completed_at=NOW() WHERE id=$1::uuid AND workspace_id=$2::uuid AND status IN('pending','running','approval_wait','session_wait')`, runID, middleware.WorkspaceIDFromCtx(r.Context()))
	if e != nil {
		errs.Write(w, errs.Internal("failed to cancel run"))
		return
	}
	if t.RowsAffected() == 0 {
		errs.Write(w, errs.BadRequest("run cannot be cancelled"))
		return
	}
	// A cancelled approval_wait run must not be resumable later.
	deleteWaitState(r.Context(), h.pool, runID)
	errs.WriteJSON(w, 200, map[string]any{"status": "cancelled"})
}
