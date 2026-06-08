package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentNexus/agent-nexus/services/api/internal/api/middleware"
	"github.com/agentNexus/agent-nexus/services/api/internal/config"
	"github.com/agentNexus/agent-nexus/services/api/internal/domain"
	"github.com/agentNexus/agent-nexus/services/api/internal/provider"
	"github.com/agentNexus/agent-nexus/services/api/internal/repository"
	agentprompt "github.com/agentNexus/agent-nexus/services/api/internal/runtime/agent"
	contextretrieval "github.com/agentNexus/agent-nexus/services/api/internal/runtime/context"
	"github.com/agentNexus/agent-nexus/services/api/internal/runtime/memory"
	"github.com/agentNexus/agent-nexus/services/api/pkg/errs"
)

type InvokeHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
	runs *RunsHandler
}

func NewInvokeHandler(pool *pgxpool.Pool, cfg *config.Config, runs *RunsHandler) *InvokeHandler {
	return &InvokeHandler{pool: pool, cfg: cfg, runs: runs}
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
		h.executeRun(r.Context(), a, ws, uid, runID, convID, req.Input, emit)
		return
	}

	// Non-streaming: return run_id immediately and execute in background
	errs.WriteJSON(w, http.StatusAccepted, map[string]any{
		"run_id":          runID,
		"conversation_id": convID,
		"status":          "running",
	})

	go h.executeRun(context.Background(), a, ws, uid, runID, convID, req.Input, nil)
}

// Group handles POST /api/v1/invoke/groups/:groupId
func (h *InvokeHandler) Group(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())
	groupID := chi.URLParam(r, "groupId")

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
		`SELECT name, mode FROM agent_groups WHERE id=$1::uuid AND workspace_id=$2::uuid AND status='active'`,
		groupID, ws).Scan(&gName, &gMode)
	if err != nil {
		errs.Write(w, errs.NotFound("agent group not found"))
		return
	}

	// Create a run record scoped to the group
	runID := uuid.NewString()
	convID := uuid.NewString()
	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO conversations(id,workspace_id,agent_id,user_id,title) VALUES($1::uuid,$2::uuid,gen_random_uuid(),$3::uuid,$4)`,
		convID, ws, uid, "Group: "+gName); err != nil {
		errs.Write(w, errs.Internal("failed to create conversation"))
		return
	}
	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO messages(id,conversation_id,role,content) VALUES($1::uuid,$2::uuid,'user',$3)`,
		uuid.NewString(), convID, req.Input); err != nil {
		errs.Write(w, errs.Internal("failed to save message"))
		return
	}

	// Note: group pipeline execution is queued for the background worker (v0.2).
	// For now we insert a run record and return immediately.
	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status) VALUES($1::uuid,$2::uuid,gen_random_uuid(),$3::uuid,$4::uuid,$5,'pending')`,
		runID, ws, convID, uid, req.Input); err != nil {
		errs.Write(w, errs.Internal("failed to create run"))
		return
	}

	errs.WriteJSON(w, http.StatusAccepted, map[string]any{
		"run_id":          runID,
		"conversation_id": convID,
		"group_id":        groupID,
		"group_name":      gName,
		"mode":            gMode,
		"status":          "pending",
		"message":         "group run queued",
	})
}

// ensureConversation returns an existing conversation by ID or creates a new one.
func (h *InvokeHandler) ensureConversation(ctx context.Context, convID, ws, uid, agentID string) (string, error) {
	if convID != "" {
		var exists bool
		h.pool.QueryRow(ctx,
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

// executeRun runs the full agent loop. When emit is nil (background mode), progress is only
// written to the database. When emit is non-nil, SSE events are also sent to the client.
func (h *InvokeHandler) executeRun(ctx context.Context, a *domain.Agent, ws, uid, runID, convID, input string, emit func(string)) {
	sseErr := func(msg string) {
		if emit != nil {
			emit(fmt.Sprintf(`{"type":"error","error":%q}`, msg))
		}
	}

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
	if a.ContextRetrievalEnabled {
		start := time.Now()
		connIDs, err := h.runs.agentConnectorIDs(ctx, a.ID)
		chunks := []contextretrieval.Chunk{}
		if err == nil {
			chunks, err = contextretrieval.NewRetriever(h.pool).Retrieve(ctx, ws, connIDs, nil, 8)
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
		h.runs.failRun(ctx, runID, err.Error()) //nolint:errcheck
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

	llm, err := h.runs.providerFor(ctx, ws, a.Provider)
	if err != nil {
		sseErr(err.Error())
		h.runs.failRun(ctx, runID, err.Error()) //nolint:errcheck
		return
	}

	if emit != nil {
		emit(fmt.Sprintf(`{"type":"run_started","run_id":%q}`, runID))
	}

	stream, err := llm.Complete(ctx, provider.CompletionRequest{
		Model:       a.Model,
		Messages:    prompt,
		Temperature: a.Temperature,
		MaxTokens:   a.MaxTokens,
		Stream:      true,
	})
	if err != nil {
		sseErr(err.Error())
		h.runs.failRun(ctx, runID, err.Error()) //nolint:errcheck
		return
	}

	modelStart := time.Now()
	reply := ""
	usage := provider.Usage{}
	for event := range stream {
		switch event.Type {
		case provider.EventDelta:
			reply += event.Delta
			if emit != nil {
				emit(fmt.Sprintf(`{"type":"delta","content":%q}`, event.Delta))
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
			h.runs.failRun(ctx, runID, msg) //nolint:errcheck
			sseErr(msg)
			return
		}
	}

	if strings.TrimSpace(reply) == "" {
		msg := "model returned an empty response"
		h.runs.failRun(ctx, runID, msg) //nolint:errcheck
		sseErr(msg)
		return
	}

	h.runs.createStep(ctx, runID, domain.StepModelCall, //nolint:errcheck
		map[string]any{"provider": a.Provider, "model": a.Model, "messages": len(prompt)},
		map[string]any{"content": reply},
		modelStart, usage.InputTokens+usage.OutputTokens, "", "")

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

	h.pool.Exec(ctx, //nolint:errcheck
		`UPDATE runs SET output=$2,status='success',completed_at=NOW(),total_input_tokens=$3,total_output_tokens=$4 WHERE id=$1::uuid`,
		runID, reply, usage.InputTokens, usage.OutputTokens)

	if emit != nil {
		emit(fmt.Sprintf(`{"type":"run_completed","run_id":%q,"usage":{"input":%d,"output":%d},"cost":0}`,
			runID, usage.InputTokens, usage.OutputTokens))
	}
}

// scanMessages is a helper used in executeRun.
func scanMessages(rows interface {
	Next() bool
	Scan(...any) error
	Close()
}) []provider.Message {
	defer rows.Close()
	var msgs []provider.Message
	for rows.Next() {
		var role, content, toolCallID, toolName string
		if rows.Scan(&role, &content, &toolCallID, &toolName) == nil {
			msgs = append(msgs, provider.Message{Role: role, Content: content, ToolCallID: toolCallID, ToolName: toolName})
		}
	}
	return msgs
}
