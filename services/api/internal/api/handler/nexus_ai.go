package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

// NexusAIHandler powers the Nexus AI chat endpoint — a meta-agent that creates
// agents and workflow graphs from natural language using the workspace's own
// provider credentials (no separate API key required).
type NexusAIHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
	runs *RunsHandler // reuse providerFor credential lookup + decryption
}

func NewNexusAIHandler(pool *pgxpool.Pool, cfg *config.Config, runs *RunsHandler) *NexusAIHandler {
	return &NexusAIHandler{pool: pool, cfg: cfg, runs: runs}
}

// nexusSystemPrompt builds the system prompt for the Nexus AI assistant,
// injecting the deployment's base URL so the LLM generates correct navigation links.
func nexusSystemPrompt(appURL string) string {
	return `You are Nexus AI, an intelligent assistant built into the Agent Nexus platform.
The base URL of this deployment is: ` + appURL + `
When sharing links to resources you create, always use this base URL. For example:
  - Agent:    ` + appURL + `/agents/<id>
  - Workflow: ` + appURL + `/workflows/<id>
  - Triggers: ` + appURL + `/triggers

` + nexusSystemPromptBody
}

const nexusSystemPromptBody = `Your purpose is to help users create fully configured AI agents and multi-agent workflow graphs using natural language — fast.

You have access to these tools:
- list_available_models: See what LLM providers and models the workspace has configured.
- list_agents: See existing agents (useful when building workflows that reference them).
- list_tools: See all available tools (native + MCP) that can be attached to an agent.
- list_connectors: See all connected data sources whose knowledge can be retrieved by agents.
- create_agent: Create a fully configured AI agent — including tools, memory, and context retrieval.
- create_workflow: Create a named workflow (the container for a multi-agent graph).
- save_workflow_graph: Define the visual workflow canvas — nodes and edges — for a workflow.
- create_trigger: Create a webhook trigger that fires an agent or workflow from an inbound HTTP POST. Returns the public webhook URL.

Guidelines:
- ALWAYS call list_available_models first — before creating any agent, you must know what providers and models are available in this workspace. Never assume a model exists; always verify.
- Pick the best available model using this priority: prefer the newest/most capable model from each provider. Ranking by provider: Anthropic → claude-opus-4-8 > claude-sonnet-4-6 > claude-haiku-4-5; OpenAI → gpt-4o > gpt-4o-mini; Gemini → gemini-2.5-pro > gemini-2.5-flash; Ollama → use whatever is listed. Always use the model ID exactly as returned by list_available_models — never guess or invent a model ID.
- ALWAYS call list_agents before creating an agent — reuse an existing agent if one already fits the user's requirements rather than creating a duplicate. If the user asks for "a research agent" and one already exists, use it.
- ALWAYS call list_workflows before creating a workflow — if one exists that matches the user's intent, ask the user: "A workflow called '<name>' already exists. Do you want to update it, or create a new one?" and wait for their answer before proceeding.
- Use distinct, descriptive names for every resource (agent or workflow) that reflect its specific purpose. Never use generic names like "Research Agent" or "Pipeline" — be specific, e.g. "Market Research Agent", "Daily News Pipeline".
- Always call list_tools before create_agent if the user mentions web search, file reading, HTTP requests, or any tool capability.
- Always call list_connectors before create_agent if the user mentions documents, knowledge base, or context retrieval.
- Use sensible defaults: temperature=0.7, max_tokens=4096, max_steps=10, max_tool_calls=20, status="active".
- Enable memory (memory_enabled=true, memory_scope="agent") for agents that need to remember past interactions.
- Enable context retrieval (context_retrieval_enabled=true) for agents that need to search documents.
- For workflows, always include a start node (node_type="start") and an end node (node_type="end").
- Use condition nodes for branching (yes/no edges), parallel+join nodes for concurrent execution, loop nodes for retry patterns.
- For supervisor workflows: use node_type="supervisor" for the coordinating agent (not "agent"). Connect start→supervisor→end with normal edges. Connect supervisor→each team agent node with edges labelled "delegate" — these agents become callable tools the supervisor uses at runtime. The supervisor's LLM decides when and how to call each team agent. Always assign a real agent to the supervisor node via agent_id.
- For node positions, use a horizontal layout: start at x=50 y=200, then space nodes 250px apart horizontally; branch parallel nodes ±150px vertically from the main axis. For supervisor team agents, place them to the right of the supervisor node at ±140px vertically.
- After creating resources, tell the user what was created and provide navigation links. Keep responses concise.
- If the user has no provider configured, tell them to go to Settings → Providers to add one.`

// nexusToolDefs are the tool definitions sent to the LLM so it can call them.
var nexusToolDefs = []provider.ToolDefinition{
	{
		Name:        "list_available_models",
		Description: "List the LLM providers and models configured in this workspace. Call this when the user hasn't specified a provider/model and you need to know what's available.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	},
	{
		Name:        "list_agents",
		Description: "List all agents in this workspace. Call this when building a workflow that needs to reference existing agents by ID.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	},
	{
		Name:        "list_tools",
		Description: "List all available tools (native and MCP) in this workspace. Call this before create_agent when the user mentions any capability like web search, file reading, HTTP requests, or tool use.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	},
	{
		Name:        "list_connectors",
		Description: "List all data source connectors in this workspace. Call this before create_agent when the user mentions documents, knowledge base, context retrieval, or grounding from external sources.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	},
	{
		Name:        "create_agent",
		Description: "Create a fully configured AI agent with all settings, tools, and context sources. The agent is immediately available in the workspace.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Short, descriptive name for the agent"},
				"description":{"type":"string","description":"One-sentence description of what this agent does"},
				"instructions":{"type":"string","description":"Full system prompt — the agent's core instructions and behaviour"},
				"provider":{"type":"string","enum":["anthropic","openai","gemini","ollama"],"description":"LLM provider to use"},
				"model":{"type":"string","description":"Model ID, e.g. claude-sonnet-4-6, gpt-4o, gemini-2.5-flash. Always use the exact model ID from list_available_models."},
				"temperature":{"type":"number","description":"Sampling temperature 0-2, default 0.7"},
				"max_tokens":{"type":"integer","description":"Max output tokens, default 4096"},
				"max_steps":{"type":"integer","description":"Maximum agentic steps per run, default 10"},
				"max_tool_calls":{"type":"integer","description":"Maximum total tool calls per run, default 20"},
				"max_duration_secs":{"type":"integer","description":"Max run duration in seconds, default 300"},
				"memory_enabled":{"type":"boolean","description":"Enable memory so the agent remembers past interactions"},
				"memory_scope":{"type":"string","enum":["conversation","agent","workspace"],"description":"Memory scope: conversation (per-chat), agent (across all chats with this agent), workspace (shared across all agents). Default: agent"},
				"context_retrieval_enabled":{"type":"boolean","description":"Enable retrieval-augmented context from connected data sources (connectors)"},
				"tool_ids":{"type":"array","items":{"type":"string"},"description":"List of tool UUIDs to attach. Get these from list_tools."},
				"tool_names":{"type":"array","items":{"type":"string"},"description":"List of tool names to attach (alternative to tool_ids, e.g. ['native_web_search','native_read_file']). Resolved by name."},
				"connector_ids":{"type":"array","items":{"type":"string"},"description":"List of connector UUIDs to enable for context retrieval. Requires context_retrieval_enabled=true. Get IDs from list_connectors."},
				"max_chunks":{"type":"integer","description":"Max context chunks to retrieve per run (default 8, only used when context_retrieval_enabled=true)"},
				"min_score":{"type":"number","description":"Minimum similarity score for context retrieval, 0-1 (default 0.5, only used when context_retrieval_enabled=true)"},
				"status":{"type":"string","enum":["active","paused","archived"],"description":"Agent status, default active"}
			},
			"required":["name","instructions","provider","model"]
		}`),
	},
	{
		Name:        "list_workflows",
		Description: "List all existing workflows in this workspace. ALWAYS call this before create_workflow to check if a similar workflow already exists. If one does, ask the user whether to update it or create a new one.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	},
	{
		Name:        "create_workflow",
		Description: "Create a named workflow — the container for a multi-agent graph. After creating it, call save_workflow_graph to define the canvas.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Name of the workflow"},
				"description":{"type":"string","description":"What this workflow does"},
				"mode":{"type":"string","enum":["pipeline","supervisor"],"description":"pipeline = sequential/graph execution (default); supervisor = one agent coordinates others"}
			},
			"required":["name","mode"]
		}`),
	},
	{
		Name:        "save_workflow_graph",
		Description: "Save the visual workflow graph (nodes + edges) for a workflow. Creates the complete workflow canvas visible in the Workflows page. Always include start and end nodes.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"workflow_id":{"type":"string","description":"The workflow ID returned by create_workflow"},
				"nodes":{
					"type":"array",
					"description":"List of workflow nodes",
					"items":{
						"type":"object",
						"properties":{
							"id":{"type":"string","description":"Client-side ID used to reference this node in edges, e.g. 'n1','n2'"},
							"node_type":{"type":"string","enum":["start","end","agent","supervisor","condition","parallel","join","loop"]},
							"agent_id":{"type":"string","description":"Agent UUID — for node_type=agent or node_type=supervisor"},
							"position_x":{"type":"number","description":"Canvas X position"},
							"position_y":{"type":"number","description":"Canvas Y position"},
							"config":{"type":"object","description":"Node config: {label} for agent, {expression} for condition (e.g. contains:APPROVED), {exit_condition, max_iterations} for loop"}
						},
						"required":["id","node_type","position_x","position_y","config"]
					}
				},
				"edges":{
					"type":"array",
					"description":"Directed connections between nodes",
					"items":{
						"type":"object",
						"properties":{
							"source_node_id":{"type":"string","description":"Client ID of the source node"},
							"target_node_id":{"type":"string","description":"Client ID of the target node"},
							"label":{"type":"string","description":"Edge label: 'yes' or 'no' for condition branches, 'loop' for loop-back, 'exit' for loop exit, 'delegate' for supervisor→agent team edges, empty for default"}
						},
						"required":["source_node_id","target_node_id"]
					}
				}
			},
			"required":["workflow_id","nodes","edges"]
		}`),
	},
	{
		Name:        "create_trigger",
		Description: "Create a webhook trigger that invokes an agent or workflow from an external HTTP POST request. Returns the webhook URL that external systems should call.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Name of the trigger, e.g. 'GitHub PR Webhook'"},
				"description":{"type":"string","description":"Optional description of what this trigger does"},
				"target_type":{"type":"string","enum":["agent","workflow"],"description":"Whether to invoke an agent or a workflow"},
				"target_id":{"type":"string","description":"UUID of the target agent or workflow. Get from list_agents or list_workflows."},
				"input_template":{"type":"string","description":"Go text/template for building the run input from the request. Available: {{.RawBody}} (full body), {{.Body.field}} (JSON field), {{.Headers.X-Name}} (header), {{.Query.param}} (query param). Default: {{.RawBody}}"},
				"secret":{"type":"string","description":"Optional HMAC-SHA256 shared secret. When set, callers must include X-Hub-Signature-256: sha256=<hex> header."},
				"is_active":{"type":"boolean","description":"Whether the trigger is active immediately. Default true."}
			},
			"required":["name","target_type","target_id"]
		}`),
	},
}

// providerPriority determines the preferred provider when multiple are configured.
var providerPriority = []string{"anthropic", "openai", "gemini", "ollama"}

// defaultModel returns the best default model for a given provider name.
func defaultModel(providerName string) string {
	switch providerName {
	case "anthropic":
		return "claude-sonnet-4-6"
	case "openai":
		return "gpt-4o"
	case "gemini":
		return "gemini-2.5-flash"
	default:
		return "llama3"
	}
}

// Chat handles POST /api/v1/nexus-ai/chat.
// Accepts the full conversation history and streams SSE events back.
func (h *NexusAIHandler) Chat(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())

	var req struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Provider string `json:"provider"` // optional — client-chosen provider
		Model    string `json:"model"`    // optional — client-chosen model
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) == 0 {
		errs.Write(w, errs.BadRequest("messages array is required"))
		return
	}

	// ── Resolve provider ─────────────────────────────────────────────────────
	providerRepo := repository.NewProviderRepository(h.pool)
	creds, err := providerRepo.List(r.Context(), ws)
	if err != nil || len(creds) == 0 {
		errs.Write(w, errs.BadRequest("no provider configured — add one in Settings → Providers"))
		return
	}

	chosenProvider := req.Provider
	if chosenProvider != "" {
		// Validate that the requested provider is active in this workspace.
		var found bool
		for _, c := range creds {
			if c.IsActive && c.Provider == chosenProvider {
				found = true
				break
			}
		}
		if !found {
			errs.Write(w, errs.BadRequest("provider '"+chosenProvider+"' is not active in this workspace"))
			return
		}
	} else {
		// Auto-pick by priority order.
		for _, prio := range providerPriority {
			for _, c := range creds {
				if c.IsActive && c.Provider == prio {
					chosenProvider = c.Provider
					break
				}
			}
			if chosenProvider != "" {
				break
			}
		}
		if chosenProvider == "" {
			for _, c := range creds {
				if c.IsActive {
					chosenProvider = c.Provider
					break
				}
			}
		}
	}
	if chosenProvider == "" {
		errs.Write(w, errs.BadRequest("no active provider configured — add one in Settings → Providers"))
		return
	}

	llm, err := h.runs.providerFor(r.Context(), ws, chosenProvider)
	if err != nil {
		errs.Write(w, errs.Internal("failed to load provider: "+err.Error()))
		return
	}

	// ── SSE setup ────────────────────────────────────────────────────────────
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

	// Keepalive: prevents proxy/browser timeouts during long LLM responses.
	keepDone := make(chan struct{})
	defer close(keepDone)
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				emit(`{"type":"ping"}`)
			case <-keepDone:
				return
			case <-r.Context().Done():
				return
			}
		}
	}()

	// ── Build messages ────────────────────────────────────────────────────────
	messages := []provider.Message{
		{Role: "system", Content: nexusSystemPrompt(h.cfg.PublicAppURL)},
	}
	for _, m := range req.Messages {
		if m.Role == "user" || m.Role == "assistant" {
			messages = append(messages, provider.Message{Role: m.Role, Content: m.Content})
		}
	}

	model := req.Model
	if model == "" {
		model = defaultModel(chosenProvider)
	}

	// ── Tool-call loop ────────────────────────────────────────────────────────
	const maxIter = 10 // safety cap on tool-call iterations
	for iter := 0; iter < maxIter; iter++ {
		select {
		case <-r.Context().Done():
			return
		default:
		}

		stream, err := llm.Complete(r.Context(), provider.CompletionRequest{
			Model:       model,
			Messages:    messages,
			Tools:       nexusToolDefs,
			Temperature: 1.0,
			MaxTokens:   4096,
			Stream:      true,
		})
		if err != nil {
			slog.Error("nexus-ai: Complete() error", "iter", iter, "model", model, "provider", chosenProvider, "err", err)
			sseErr("provider error: " + err.Error())
			return
		}

		reply := ""
		var pendingCalls []provider.ToolCall
		eventCount := 0

		for event := range stream {
			eventCount++
			switch event.Type {
			case provider.EventDelta:
				reply += event.Delta
				emit(fmt.Sprintf(`{"type":"delta","content":%q}`, event.Delta))
			case provider.EventToolCall:
				if event.ToolCall != nil {
					slog.Info("nexus-ai: tool_call", "tool", event.ToolCall.Name, "id", event.ToolCall.ID)
					pendingCalls = append(pendingCalls, *event.ToolCall)
				}
			case provider.EventError:
				msg := "model error"
				if event.Error != nil {
					msg = event.Error.Error()
				}
				slog.Error("nexus-ai: stream EventError", "iter", iter, "err", msg)
				sseErr(msg)
				return
			case provider.EventDone:
				slog.Info("nexus-ai: stream done", "iter", iter, "events", eventCount, "reply_len", len(reply), "tool_calls", len(pendingCalls))
			}
		}
		slog.Info("nexus-ai: stream closed", "iter", iter, "events", eventCount, "reply_len", len(reply), "tool_calls", len(pendingCalls))

		if len(pendingCalls) == 0 {
			// No more tool calls — conversation turn is complete.
			emit(`{"type":"run_completed"}`)
			return
		}

		// Append the assistant message with tool calls so the next loop has context.
		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   reply,
			ToolCalls: pendingCalls,
		})

		// Execute each tool call and append the result.
		for _, call := range pendingCalls {
			label := toolStartLabel(call.Name, call.Input)
			emit(fmt.Sprintf(`{"type":"tool_started","tool":%q,"label":%q}`, call.Name, label))

			result, execErr := h.executeTool(r.Context(), ws, uid, call.Name, call.Input)
			if execErr != nil {
				emit(fmt.Sprintf(`{"type":"tool_completed","tool":%q,"label":%q,"error":%q}`,
					call.Name, "Error", execErr.Error()))
				resultJSON, _ := json.Marshal(map[string]string{"error": execErr.Error()})
				messages = append(messages, provider.Message{
					Role: "tool", ToolCallID: call.ID, ToolName: call.Name,
					Content: string(resultJSON),
				})
				continue
			}

			// Emit the completed event with optional navigation link.
			completedEvt := map[string]any{
				"type":  "tool_completed",
				"tool":  call.Name,
				"label": result.Label,
			}
			if result.ResultID != "" {
				completedEvt["result"] = map[string]string{"id": result.ResultID, "name": result.ResultName}
			}
			if result.Link != "" {
				completedEvt["link"] = result.Link
			}
			evtJSON, _ := json.Marshal(completedEvt)
			emit(string(evtJSON))

			resultJSON, _ := json.Marshal(result.Data)
			messages = append(messages, provider.Message{
				Role: "tool", ToolCallID: call.ID, ToolName: call.Name,
				Content: string(resultJSON),
			})
		}
	}

	// Shouldn't reach here unless the model is stuck in a tool loop.
	sseErr("maximum tool iterations reached")
}

// toolResult holds the outcome of a tool execution.
type toolResult struct {
	Label      string
	ResultID   string
	ResultName string
	Link       string
	Data       any
}

// executeTool dispatches a tool call by name and executes it against the DB.
func (h *NexusAIHandler) executeTool(ctx context.Context, ws, uid, name string, input json.RawMessage) (*toolResult, error) {
	switch name {
	case "list_available_models":
		return h.toolListModels(ctx, ws)
	case "list_agents":
		return h.toolListAgents(ctx, ws)
	case "list_workflows":
		return h.toolListWorkflows(ctx, ws)
	case "list_tools":
		return h.toolListTools(ctx, ws)
	case "list_connectors":
		return h.toolListConnectors(ctx, ws)
	case "create_agent":
		return h.toolCreateAgent(ctx, ws, uid, input)
	case "create_workflow":
		return h.toolCreateWorkflow(ctx, ws, uid, input)
	case "save_workflow_graph":
		return h.toolSaveGraph(ctx, ws, input)
	case "create_trigger":
		return h.toolCreateTrigger(ctx, ws, uid, input)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func (h *NexusAIHandler) toolListModels(ctx context.Context, ws string) (*toolResult, error) {
	repo := repository.NewProviderRepository(h.pool)
	creds, err := repo.List(ctx, ws)
	if err != nil {
		return nil, err
	}
	type entry struct {
		Provider     string `json:"provider"`
		DisplayName  string `json:"display_name"`
		DefaultModel string `json:"suggested_model"`
		Active       bool   `json:"active"`
	}
	var out []entry
	for _, c := range creds {
		out = append(out, entry{
			Provider:     c.Provider,
			DisplayName:  c.DisplayName,
			DefaultModel: defaultModel(c.Provider),
			Active:       c.IsActive,
		})
	}
	return &toolResult{Label: "Listed available providers", Data: out}, nil
}

func (h *NexusAIHandler) toolListTools(ctx context.Context, ws string) (*toolResult, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT id, name, description, type, risk_level, requires_approval, enabled
		 FROM tools
		 WHERE workspace_id IS NULL OR workspace_id=$1::uuid
		 ORDER BY type, name`, ws)
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}
	defer rows.Close()
	type entry struct {
		ID               string `json:"id"`
		Name             string `json:"name"`
		Description      string `json:"description"`
		Type             string `json:"type"`
		RiskLevel        string `json:"risk_level"`
		RequiresApproval bool   `json:"requires_approval"`
		Enabled          bool   `json:"enabled"`
	}
	var out []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.ID, &e.Name, &e.Description, &e.Type, &e.RiskLevel, &e.RequiresApproval, &e.Enabled); err != nil {
			continue
		}
		out = append(out, e)
	}
	return &toolResult{Label: fmt.Sprintf("Listed %d tools", len(out)), Data: out}, nil
}

func (h *NexusAIHandler) toolListConnectors(ctx context.Context, ws string) (*toolResult, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT id, name, provider, status FROM connectors WHERE workspace_id=$1::uuid ORDER BY name`, ws)
	if err != nil {
		return nil, fmt.Errorf("failed to list connectors: %w", err)
	}
	defer rows.Close()
	type entry struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Provider string `json:"provider"`
		Status   string `json:"status"`
	}
	var out []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.ID, &e.Name, &e.Provider, &e.Status); err != nil {
			continue
		}
		out = append(out, e)
	}
	return &toolResult{Label: fmt.Sprintf("Listed %d connectors", len(out)), Data: out}, nil
}

func (h *NexusAIHandler) toolListAgents(ctx context.Context, ws string) (*toolResult, error) {
	repo := repository.NewAgentRepository(h.pool)
	agents, err := repo.List(ctx, ws)
	if err != nil {
		return nil, err
	}
	type entry struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Provider    string `json:"provider"`
		Model       string `json:"model"`
	}
	var out []entry
	for _, a := range agents {
		out = append(out, entry{ID: a.ID, Name: a.Name, Description: a.Description, Provider: a.Provider, Model: a.Model})
	}
	return &toolResult{Label: fmt.Sprintf("Listed %d agents", len(out)), Data: out}, nil
}

func (h *NexusAIHandler) toolListWorkflows(ctx context.Context, ws string) (*toolResult, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT id::text, name, description, mode, status FROM workflows
		 WHERE workspace_id=$1::uuid ORDER BY created_at DESC`, ws)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}
	defer rows.Close()
	type entry struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Mode        string `json:"mode"`
		Status      string `json:"status"`
	}
	var out []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.ID, &e.Name, &e.Description, &e.Mode, &e.Status); err != nil {
			continue
		}
		out = append(out, e)
	}
	return &toolResult{Label: fmt.Sprintf("Listed %d workflows", len(out)), Data: out}, nil
}

func (h *NexusAIHandler) toolCreateAgent(ctx context.Context, ws, uid string, input json.RawMessage) (*toolResult, error) {
	var args struct {
		Name                    string   `json:"name"`
		Description             string   `json:"description"`
		Instructions            string   `json:"instructions"`
		Provider                string   `json:"provider"`
		Model                   string   `json:"model"`
		Temperature             float64  `json:"temperature"`
		MaxTokens               int      `json:"max_tokens"`
		MaxSteps                int      `json:"max_steps"`
		MaxToolCalls            int      `json:"max_tool_calls"`
		MaxDurationSecs         int      `json:"max_duration_secs"`
		MemoryEnabled           bool     `json:"memory_enabled"`
		MemoryScope             string   `json:"memory_scope"`
		ContextRetrievalEnabled bool     `json:"context_retrieval_enabled"`
		ToolIDs                 []string `json:"tool_ids"`
		ToolNames               []string `json:"tool_names"`
		ConnectorIDs            []string `json:"connector_ids"`
		MaxChunks               int      `json:"max_chunks"`
		MinScore                float64  `json:"min_score"`
		Status                  string   `json:"status"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid create_agent input: %w", err)
	}
	if args.Name == "" || args.Instructions == "" || args.Provider == "" || args.Model == "" {
		return nil, fmt.Errorf("create_agent requires name, instructions, provider, model")
	}
	// Defaults
	if args.Temperature == 0 {
		args.Temperature = 0.7
	}
	if args.MaxTokens == 0 {
		args.MaxTokens = 4096
	}
	if args.MaxSteps == 0 {
		args.MaxSteps = 10
	}
	if args.MaxToolCalls == 0 {
		args.MaxToolCalls = 20
	}
	if args.MaxDurationSecs == 0 {
		args.MaxDurationSecs = 300
	}
	if args.MemoryScope == "" {
		args.MemoryScope = "conversation"
	}
	if args.Status == "" {
		args.Status = "active"
	}
	if args.MaxChunks == 0 {
		args.MaxChunks = 8
	}
	if args.MinScore == 0 {
		args.MinScore = 0.5
	}

	a := &domain.Agent{
		ID:                      uuid.NewString(),
		WorkspaceID:             ws,
		Name:                    args.Name,
		Description:             args.Description,
		Instructions:            args.Instructions,
		Provider:                args.Provider,
		Model:                   args.Model,
		Temperature:             args.Temperature,
		MaxTokens:               args.MaxTokens,
		MemoryEnabled:           args.MemoryEnabled,
		MemoryScope:             args.MemoryScope,
		ContextRetrievalEnabled: args.ContextRetrievalEnabled,
		MaxSteps:                args.MaxSteps,
		MaxToolCalls:            args.MaxToolCalls,
		MaxDurationSecs:         args.MaxDurationSecs,
		Status:                  args.Status,
		CreatedBy:               uid,
	}

	repo := repository.NewAgentRepository(h.pool)
	if err := repo.Create(ctx, a); err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	// Attach tools if specified
	if len(args.ToolIDs) > 0 || len(args.ToolNames) > 0 {
		tx, err := h.pool.Begin(ctx)
		if err == nil {
			for _, id := range args.ToolIDs {
				tx.Exec(ctx, //nolint:errcheck
					`INSERT INTO agent_tools(agent_id,tool_id,enabled)
					 SELECT $1::uuid,id,true FROM tools WHERE id=$2::uuid AND (workspace_id IS NULL OR workspace_id=$3::uuid)`,
					a.ID, id, ws)
			}
			for _, name := range args.ToolNames {
				tx.Exec(ctx, //nolint:errcheck
					`INSERT INTO agent_tools(agent_id,tool_id,enabled)
					 SELECT $1::uuid,id,true FROM tools WHERE name=$2 AND (workspace_id IS NULL OR workspace_id=$3::uuid)
					 ON CONFLICT DO NOTHING`,
					a.ID, name, ws)
			}
			tx.Commit(ctx) //nolint:errcheck
		}
	}

	// Attach connectors if specified
	if args.ContextRetrievalEnabled && len(args.ConnectorIDs) > 0 {
		tx, err := h.pool.Begin(ctx)
		if err == nil {
			tx.Exec(ctx, `DELETE FROM agent_connectors WHERE agent_id=$1::uuid`, a.ID) //nolint:errcheck
			for _, cid := range args.ConnectorIDs {
				tx.Exec(ctx, //nolint:errcheck
					`INSERT INTO agent_connectors(agent_id,connector_id,max_chunks,min_score)
					 VALUES($1::uuid,$2::uuid,$3,$4) ON CONFLICT(agent_id,connector_id) DO NOTHING`,
					a.ID, cid, args.MaxChunks, args.MinScore)
			}
			tx.Commit(ctx) //nolint:errcheck
		}
	}

	// Build a brief summary of what was configured
	extras := []string{}
	if args.MemoryEnabled {
		extras = append(extras, "memory:"+args.MemoryScope)
	}
	if args.ContextRetrievalEnabled && len(args.ConnectorIDs) > 0 {
		extras = append(extras, fmt.Sprintf("context(%d connectors)", len(args.ConnectorIDs)))
	}
	if len(args.ToolIDs)+len(args.ToolNames) > 0 {
		extras = append(extras, fmt.Sprintf("%d tools", len(args.ToolIDs)+len(args.ToolNames)))
	}
	label := fmt.Sprintf("Created agent \"%s\"", a.Name)
	if len(extras) > 0 {
		label += " [" + strings.Join(extras, ", ") + "]"
	}

	return &toolResult{
		Label:      label,
		ResultID:   a.ID,
		ResultName: a.Name,
		Link:       "/agents",
		Data:       map[string]string{"id": a.ID, "name": a.Name},
	}, nil
}

func (h *NexusAIHandler) toolCreateWorkflow(ctx context.Context, ws, uid string, input json.RawMessage) (*toolResult, error) {
	var args struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Mode        string `json:"mode"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid create_workflow input: %w", err)
	}
	if args.Name == "" {
		return nil, fmt.Errorf("create_workflow requires name")
	}
	if args.Mode == "" {
		args.Mode = "pipeline"
	}

	id := uuid.NewString()
	_, err := h.pool.Exec(ctx,
		`INSERT INTO workflows(id,workspace_id,name,description,mode,status,created_by)
		 VALUES($1::uuid,$2::uuid,$3,$4,$5,'active',$6::uuid)`,
		id, ws, args.Name, args.Description, args.Mode, uid)
	if err != nil {
		return nil, fmt.Errorf("failed to create workflow: %w", err)
	}

	return &toolResult{
		Label:      fmt.Sprintf("Created group \"%s\"", args.Name),
		ResultID:   id,
		ResultName: args.Name,
		Link:       "/workflows/" + id,
		Data:       map[string]string{"id": id, "name": args.Name},
	}, nil
}

func (h *NexusAIHandler) toolSaveGraph(ctx context.Context, ws string, input json.RawMessage) (*toolResult, error) {
	var args struct {
		WorkflowID string `json:"workflow_id"`
		Nodes      []struct {
			ID        string         `json:"id"`
			NodeType  string         `json:"node_type"`
			AgentID   string         `json:"agent_id"`
			PositionX float64        `json:"position_x"`
			PositionY float64        `json:"position_y"`
			Config    map[string]any `json:"config"`
		} `json:"nodes"`
		Edges []struct {
			SourceNodeID string `json:"source_node_id"`
			TargetNodeID string `json:"target_node_id"`
			Label        string `json:"label"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid save_workflow_graph input: %w", err)
	}
	if args.WorkflowID == "" {
		return nil, fmt.Errorf("save_workflow_graph requires workflow_id")
	}

	// Verify workflow belongs to this workspace and fetch name for auto-tagging.
	var workflowName string
	if err := h.pool.QueryRow(ctx,
		`SELECT name FROM workflows WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		args.WorkflowID, ws).Scan(&workflowName); err != nil {
		return nil, fmt.Errorf("workflow not found")
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Delete existing graph.
	if _, err := tx.Exec(ctx,
		`DELETE FROM workflow_nodes WHERE workflow_id=$1::uuid`, args.WorkflowID); err != nil {
		return nil, fmt.Errorf("failed to clear graph: %w", err)
	}

	// Insert nodes and build client_id → server_uuid map.
	clientToServer := make(map[string]string, len(args.Nodes))
	for _, n := range args.Nodes {
		serverID := uuid.NewString()
		if n.ID != "" {
			clientToServer[n.ID] = serverID
		}

		cfgJSON, _ := json.Marshal(n.Config)
		if len(cfgJSON) == 0 || string(cfgJSON) == "null" {
			cfgJSON = []byte(`{}`)
		}

		if n.AgentID != "" {
			_, err = tx.Exec(ctx,
				`INSERT INTO workflow_nodes(id,workflow_id,node_type,agent_id,position_x,position_y,config)
				 VALUES($1::uuid,$2::uuid,$3,$4::uuid,$5,$6,$7::jsonb)`,
				serverID, args.WorkflowID, n.NodeType, n.AgentID, n.PositionX, n.PositionY, string(cfgJSON))
			if err == nil {
				// Auto-tag the agent with the workflow name
				tx.Exec(ctx, //nolint:errcheck
					`UPDATE agents SET tags = array_append(tags, $1), updated_at=NOW()
					 WHERE id=$2::uuid AND NOT ($1 = ANY(tags))`,
					workflowName, n.AgentID)
			}
		} else {
			_, err = tx.Exec(ctx,
				`INSERT INTO workflow_nodes(id,workflow_id,node_type,position_x,position_y,config)
				 VALUES($1::uuid,$2::uuid,$3,$4,$5,$6::jsonb)`,
				serverID, args.WorkflowID, n.NodeType, n.PositionX, n.PositionY, string(cfgJSON))
		}
		if err != nil {
			return nil, fmt.Errorf("failed to insert node %q: %w", n.ID, err)
		}
	}

	// Insert edges.
	for _, e := range args.Edges {
		srcID, srcOK := clientToServer[e.SourceNodeID]
		tgtID, tgtOK := clientToServer[e.TargetNodeID]
		if !srcOK || !tgtOK {
			return nil, fmt.Errorf("edge references unknown node id (src=%q tgt=%q)", e.SourceNodeID, e.TargetNodeID)
		}
		if e.Label != "" {
			_, err = tx.Exec(ctx,
				`INSERT INTO workflow_edges(id,workflow_id,source_node_id,target_node_id,label)
				 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5)`,
				uuid.NewString(), args.WorkflowID, srcID, tgtID, e.Label)
		} else {
			_, err = tx.Exec(ctx,
				`INSERT INTO workflow_edges(id,workflow_id,source_node_id,target_node_id)
				 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid)`,
				uuid.NewString(), args.WorkflowID, srcID, tgtID)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to insert edge: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit graph: %w", err)
	}

	return &toolResult{
		Label: fmt.Sprintf("Workflow graph saved (%d nodes, %d edges)", len(args.Nodes), len(args.Edges)),
		Link:  "/workflows/" + args.WorkflowID,
		Data:  map[string]any{"ok": true, "workflow_id": args.WorkflowID, "node_count": len(args.Nodes), "edge_count": len(args.Edges)},
	}, nil
}

func (h *NexusAIHandler) toolCreateTrigger(ctx context.Context, ws, uid string, input json.RawMessage) (*toolResult, error) {
	var args struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		TargetType    string `json:"target_type"`
		TargetID      string `json:"target_id"`
		InputTemplate string `json:"input_template"`
		Secret        string `json:"secret"`
		IsActive      *bool  `json:"is_active"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid create_trigger input: %w", err)
	}
	if args.Name == "" {
		return nil, fmt.Errorf("create_trigger requires name")
	}
	if args.TargetType != "agent" && args.TargetType != "workflow" {
		return nil, fmt.Errorf("target_type must be 'agent' or 'workflow'")
	}
	if args.TargetID == "" {
		return nil, fmt.Errorf("create_trigger requires target_id")
	}
	if args.InputTemplate == "" {
		args.InputTemplate = "{{.RawBody}}"
	}
	if _, err := template.New("").Parse(args.InputTemplate); err != nil {
		return nil, fmt.Errorf("invalid input_template: %w", err)
	}
	isActive := true
	if args.IsActive != nil {
		isActive = *args.IsActive
	}

	// Verify target exists in this workspace
	var targetName string
	if args.TargetType == "agent" {
		if err := h.pool.QueryRow(ctx,
			`SELECT name FROM agents WHERE id=$1::uuid AND workspace_id=$2::uuid`,
			args.TargetID, ws).Scan(&targetName); err != nil {
			return nil, fmt.Errorf("agent not found: %s", args.TargetID)
		}
	} else {
		if err := h.pool.QueryRow(ctx,
			`SELECT name FROM workflows WHERE id=$1::uuid AND workspace_id=$2::uuid`,
			args.TargetID, ws).Scan(&targetName); err != nil {
			return nil, fmt.Errorf("workflow not found: %s", args.TargetID)
		}
	}

	t := &domain.WebhookTrigger{
		ID:            uuid.NewString(),
		WorkspaceID:   ws,
		Name:          args.Name,
		Description:   args.Description,
		TargetType:    args.TargetType,
		TargetID:      args.TargetID,
		InputTemplate: args.InputTemplate,
		Secret:        args.Secret,
		IsActive:      isActive,
		CreatedBy:     uid,
	}
	repo := repository.NewWebhookTriggerRepository(h.pool)
	if err := repo.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("failed to create trigger: %w", err)
	}

	return &toolResult{
		Label:      fmt.Sprintf("Created trigger \"%s\" → %s \"%s\"", t.Name, args.TargetType, targetName),
		ResultID:   t.ID,
		ResultName: t.Name,
		Link:       "/triggers",
		Data: map[string]any{
			"id":          t.ID,
			"name":        t.Name,
			"webhook_url": "/webhook/" + t.ID,
			"target_type": t.TargetType,
			"target_id":   t.TargetID,
			"target_name": targetName,
			"is_active":   t.IsActive,
		},
	}, nil
}

// toolStartLabel generates a human-readable label for the tool_started SSE event.
func toolStartLabel(toolName string, input json.RawMessage) string {
	var m map[string]any
	_ = json.Unmarshal(input, &m)
	switch toolName {
	case "create_agent":
		if n, ok := m["name"].(string); ok && n != "" {
			return fmt.Sprintf("Creating agent \"%s\"...", n)
		}
		return "Creating agent..."
	case "create_workflow":
		if n, ok := m["name"].(string); ok && n != "" {
			return fmt.Sprintf("Creating workflow \"%s\"...", n)
		}
		return "Creating workflow..."
	case "save_workflow_graph":
		return "Saving workflow graph..."
	case "create_trigger":
		if n, ok := m["name"].(string); ok && n != "" {
			return fmt.Sprintf("Creating trigger \"%s\"...", n)
		}
		return "Creating webhook trigger..."
	case "list_available_models":
		return "Checking available models..."
	case "list_agents":
		return "Listing existing agents..."
	case "list_tools":
		return "Listing available tools..."
	case "list_connectors":
		return "Listing available connectors..."
	}
	return strings.ReplaceAll(toolName, "_", " ") + "..."
}
