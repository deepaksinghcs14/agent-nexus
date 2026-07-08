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
	pool   *pgxpool.Pool
	cfg    *config.Config
	runs   *RunsHandler   // reuse providerFor credential lookup + decryption
	invoke *InvokeHandler // used by toolRunWorkflow to fire executeGroupRun
}

func NewNexusAIHandler(pool *pgxpool.Pool, cfg *config.Config, runs *RunsHandler) *NexusAIHandler {
	return &NexusAIHandler{pool: pool, cfg: cfg, runs: runs}
}

// SetInvokeHandler wires the InvokeHandler after construction (avoids circular init).
func (h *NexusAIHandler) SetInvokeHandler(invoke *InvokeHandler) {
	h.invoke = invoke
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
- list_mcp_servers: See all connected MCP servers (e.g. Jira, GitHub, Slack) and how many tools each exposes.
- list_connectors: See all connected data sources whose knowledge can be retrieved by agents.
- create_agent: Create a fully configured AI agent — including tools, memory, and context retrieval.
- create_workflow: Create a named workflow (the container for a multi-agent graph).
- save_workflow_graph: Define the visual workflow canvas — nodes and edges — for a workflow.
- create_trigger: Create a webhook trigger that fires an agent or workflow from an inbound HTTP POST. Returns the public webhook URL.
- list_gateway_channels: See all gateway channels (WhatsApp and HTTP) configured in this workspace.
- create_gateway_channel: Create a new gateway channel (http or whatsapp) and connect it to an agent.
- list_skills: See all skills (named, reusable instruction blocks) available in this workspace.
- create_skill: Create a new skill — a named block of reusable instructions that agents can be equipped with.
- attach_skills_to_agent: Attach one or more skills to an agent (replaces the current skill set).
- update_agent: Update an existing agent's name, description, instructions, model, or status. Only the fields you provide are changed.
- attach_tool_to_agent: Attach a single tool to an agent by tool name. No-op if already attached.
- detach_tool_from_agent: Remove a tool from an agent by tool name.
- run_workflow: Start a workflow run in the background and return the run ID immediately.
- create_code_tool: Create a custom JavaScript tool that runs in a sandboxed JS engine. Receives the tool call args as an 'input' object and must return a value. Permanent by default.

Guidelines:
- ALWAYS call list_available_models first — before creating any agent, you must know what providers and models are available in this workspace. Never assume a model exists; always verify.
- Pick the best available model using this priority: prefer the newest/most capable model from each provider. Ranking by provider: Anthropic → claude-opus-4-8 > claude-sonnet-4-6 > claude-haiku-4-5; OpenAI → gpt-4o > gpt-4o-mini; Gemini → gemini-2.5-pro > gemini-2.5-flash; Ollama → use whatever is listed. Always use the model ID exactly as returned by list_available_models — never guess or invent a model ID.
- ALWAYS call list_agents before creating an agent — reuse an existing agent if one already fits the user's requirements rather than creating a duplicate. If the user asks for "a research agent" and one already exists, use it.
- ALWAYS call list_workflows before creating a workflow — if one exists that matches the user's intent, ask the user: "A workflow called '<name>' already exists. Do you want to update it, or create a new one?" and wait for their answer before proceeding.
- Use distinct, descriptive names for every resource (agent or workflow) that reflect its specific purpose. Never use generic names like "Research Agent" or "Pipeline" — be specific, e.g. "Market Research Agent", "Daily News Pipeline".
- Always call list_tools before create_agent if the user mentions web search, file reading, HTTP requests, or any tool capability.
- When the user wants an agent connected to an external service that's set up as an MCP server (e.g. "give it Jira access", "let it post to Slack"), call list_mcp_servers and pass the matching server's id via mcp_server_ids — this attaches every tool that server exposes in one step. Only fall back to individual tool_names when the user wants a specific subset of a server's tools, not the whole integration.
- Always call list_connectors before create_agent if the user mentions documents, knowledge base, or context retrieval.
- Use sensible defaults: temperature=0.7, max_tokens=4096, max_steps=10, max_tool_calls=20, status="active".
- Enable memory (memory_enabled=true, memory_scope="agent") for agents that need to remember past interactions.
- Enable context retrieval (context_retrieval_enabled=true) for agents that need to search documents. Use agentic_rag=true when the agent should decide when/what to retrieve dynamically rather than getting a fixed pre-run injection.
- For workflows, always include a start node (node_type="start") and an end node (node_type="end").
- Use condition nodes for branching (yes/no edges), parallel+join nodes for concurrent execution, loop nodes for retry patterns.
- For supervisor workflows: use node_type="supervisor" for the coordinating agent (not "agent"). Connect start→supervisor→end with normal edges. Connect supervisor→each team agent node with edges labelled "delegate" — these agents become callable tools the supervisor uses at runtime. The supervisor's LLM decides when and how to call each team agent. Always assign a real agent to the supervisor node via agent_id. IMPORTANT: when writing the supervisor agent's instructions, do NOT include tool names like "delegate_researcher" — the runtime discovers connected team agents and injects their exact names automatically. Write the supervisor's instructions to describe coordination strategy, task sequencing rationale, expected output format, and quality criteria only. When creating team agents for a supervisor workflow, always populate the description field with one sentence stating the agent's specialty (e.g. "Fact-checks claims for accuracy and flags unverified assertions") — this description is shown to the supervisor LLM as the team agent's tool description at runtime.
- For node positions, use a horizontal layout: start at x=50 y=200, then space nodes 250px apart horizontally; branch parallel nodes ±150px vertically from the main axis. For supervisor team agents, place them to the right of the supervisor node at ±140px vertically.
- For gateway channels: always call list_agents first to get the agent_id. For http channels, only name/agent_id/channel_type are needed. For whatsapp channels, advise the user to scan the QR code in the Gateway → channel page after creation.
- For skills: always call list_skills before create_skill to avoid duplicates. Use attach_skills_to_agent to equip an agent with skills — this replaces the full skill set, so list existing skills first if the agent already has some.
- After creating resources, tell the user what was created and provide navigation links. Keep responses concise.
- If the user has no provider configured, tell them to go to Settings → Providers to add one.`

// agentBuilderModePrompt is appended to the system prompt when the request is in
// "agent_builder" mode (the "Describe your agent" panel on the create page). It
// steers Nexus to draft a single agent for form review rather than creating it.
const agentBuilderModePrompt = `AGENT BUILDER MODE IS ON.

You are helping the user create ONE agent through a form. Your job is to produce a
complete draft they can review and edit — you must NOT create, update, or run anything.

Rules for this mode:
- Discover first: call list_available_models, list_tools, list_connectors, and
  list_skills as needed so your picks reference real resources.
- Then call propose_agent (NEVER create_agent, create_workflow, or any create_* tool).
  propose_agent returns a draft to the form without saving — the user clicks Create.
- Ask AT MOST ONE brief clarifying question, and only if the request is too vague to
  draft at all. Otherwise draft with sensible defaults and note assumptions.
- Write detailed instructions (role, behaviour loop, tool usage, output format, edge
  cases) — never a one-liner.
- Only attach tools/skills/connectors that genuinely fit the described purpose. Fill the
  rationale object explaining why you chose the model, tools, memory, and context.
- After proposing, tell the user the draft is ready in the form to review and create.`

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
		Name:        "list_mcp_servers",
		Description: "List all connected MCP servers in this workspace, with each server's id and how many tools it exposes. Call this when the user wants an agent to have access to an entire MCP integration (e.g. 'give it Jira access', 'connect it to Slack') — pass the server's id as mcp_server_ids to create_agent/propose_agent/update_agent to attach ALL of that server's tools at once, instead of listing individual tool names.",
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
				"agentic_rag":{"type":"boolean","description":"Let the agent decide when/what to retrieve via native_retrieve_context tool instead of pre-run injection. Only used when context_retrieval_enabled=true."},
				"tool_ids":{"type":"array","items":{"type":"string"},"description":"List of tool UUIDs to attach. Get these from list_tools."},
				"tool_names":{"type":"array","items":{"type":"string"},"description":"List of tool names to attach (alternative to tool_ids, e.g. ['native_read_file','native_http_request']). Resolved by name."},
				"mcp_server_ids":{"type":"array","items":{"type":"string"},"description":"List of MCP server UUIDs. ALL tools belonging to each server are attached automatically — use this instead of listing individual tool_names when the user wants to give the agent an entire MCP integration (e.g. 'give it Jira access'). Get server IDs from list_mcp_servers."},
				"connector_ids":{"type":"array","items":{"type":"string"},"description":"List of connector UUIDs to enable for context retrieval. Requires context_retrieval_enabled=true. Get IDs from list_connectors."},
				"max_chunks":{"type":"integer","description":"Max context chunks to retrieve per run (default 8, only used when context_retrieval_enabled=true and agentic_rag=false)"},
				"min_score":{"type":"number","description":"Minimum similarity score for context retrieval, 0-1 (default 0.5, only used when context_retrieval_enabled=true and agentic_rag=false)"},
				"status":{"type":"string","enum":["active","paused","archived"],"description":"Agent status, default active"}
			},
			"required":["name","instructions","provider","model"]
		}`),
	},
	{
		Name:        "propose_agent",
		Description: "Draft a fully configured agent for the user to review WITHOUT creating it. Use this (never create_agent) in agent-builder mode: resolve the model, tools, skills, and context sources, then propose the complete spec. The UI shows it in an editable form and the user clicks Create. Accepts the same fields as create_agent, plus skill_names and an optional rationale object explaining each non-obvious choice.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Short, descriptive name for the agent"},
				"description":{"type":"string","description":"One-sentence description of what this agent does"},
				"instructions":{"type":"string","description":"Full system prompt — the agent's core instructions and behaviour"},
				"provider":{"type":"string","enum":["anthropic","openai","gemini","ollama"],"description":"LLM provider to use"},
				"model":{"type":"string","description":"Model ID from list_available_models. If omitted, a sensible default for the provider is used."},
				"temperature":{"type":"number","description":"Sampling temperature 0-2, default 0.7"},
				"max_tokens":{"type":"integer","description":"Max output tokens, default 4096"},
				"max_steps":{"type":"integer","description":"Maximum agentic steps per run, default 10"},
				"max_tool_calls":{"type":"integer","description":"Maximum total tool calls per run, default 20"},
				"max_duration_secs":{"type":"integer","description":"Max run duration in seconds, default 300"},
				"memory_enabled":{"type":"boolean","description":"Enable memory so the agent remembers past interactions"},
				"memory_scope":{"type":"string","enum":["conversation","agent","workspace"],"description":"Memory scope. Default: agent"},
				"context_retrieval_enabled":{"type":"boolean","description":"Enable retrieval-augmented context from connectors"},
				"agentic_rag":{"type":"boolean","description":"Let the agent decide when/what to retrieve. Only used when context_retrieval_enabled=true."},
				"tool_names":{"type":"array","items":{"type":"string"},"description":"Tool names to attach, e.g. ['native_read_file']. Get from list_tools."},
				"mcp_server_ids":{"type":"array","items":{"type":"string"},"description":"List of MCP server UUIDs. ALL tools belonging to each server are attached automatically — use this instead of listing individual tool_names when the user wants to give the agent an entire MCP integration (e.g. 'give it Jira access'). Get server IDs from list_mcp_servers."},
				"skill_names":{"type":"array","items":{"type":"string"},"description":"Skill names to attach. Get from list_skills."},
				"connector_ids":{"type":"array","items":{"type":"string"},"description":"Connector UUIDs for context retrieval. Requires context_retrieval_enabled=true. Get from list_connectors."},
				"max_chunks":{"type":"integer","description":"Max context chunks per run (default 8)"},
				"min_score":{"type":"number","description":"Minimum similarity score for retrieval, 0-1 (default 0.5)"},
				"status":{"type":"string","enum":["active","paused","archived"],"description":"Agent status, default active"},
				"rationale":{"type":"object","description":"Optional short explanations keyed by area, e.g. {\"model\":\"...\",\"tools\":\"...\",\"memory\":\"...\"}. Shown to the user next to the draft."}
			},
			"required":["name","instructions"]
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
	{
		Name:        "list_gateway_channels",
		Description: "List all gateway channels (WhatsApp and HTTP) in this workspace. Call this when the user wants to see existing channels or before creating a new one.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	},
	{
		Name:        "create_gateway_channel",
		Description: "Create a new gateway channel connected to an agent. HTTP channels accept webhook POSTs; WhatsApp channels need a QR scan after creation.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Descriptive name for the channel, e.g. 'Customer Support WhatsApp'"},
				"description":{"type":"string","description":"Optional description"},
				"agent_id":{"type":"string","description":"UUID of the agent that handles inbound messages. Get from list_agents."},
				"channel_type":{"type":"string","enum":["http","whatsapp"],"description":"'http' for webhook-based channels; 'whatsapp' for WhatsApp Business messaging"}
			},
			"required":["name","agent_id","channel_type"]
		}`),
	},
	{
		Name:        "list_skills",
		Description: "List all skills in this workspace. Skills are named, reusable instruction blocks that can be attached to agents. Call this before create_skill or attach_skills_to_agent.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	},
	{
		Name:        "list_memories",
		Description: "List recent memories stored in this workspace (workspace-scoped and agent-scoped). Useful for understanding what the user's agents already remember before drafting a new agent.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer","description":"Max memories to return (default 20)."}}}`),
	},
	{
		Name:        "create_skill",
		Description: "Create a new skill — a named block of reusable instructions that can be attached to agents to extend their capabilities.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Short descriptive name, e.g. 'Formal Tone', 'Spanish Language Support'"},
				"description":{"type":"string","description":"One sentence describing what this skill does"},
				"content":{"type":"string","description":"The instruction text injected into the agent's prompt when this skill is active"}
			},
			"required":["name","content"]
		}`),
	},
	{
		Name:        "attach_skills_to_agent",
		Description: "Attach skills to an agent. This REPLACES the agent's current skill set — call list_skills and check the agent's existing skills first if you want to add without removing.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"agent_id":{"type":"string","description":"UUID of the agent to update. Get from list_agents."},
				"skill_ids":{"type":"array","items":{"type":"string"},"description":"List of skill UUIDs to attach. Get from list_skills."}
			},
			"required":["agent_id","skill_ids"]
		}`),
	},
	{
		Name:        "update_agent",
		Description: "Update an existing agent's settings. Only the fields you provide are changed — all others are left untouched.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"agent_id":{"type":"string","description":"UUID of the agent to update. Get from list_agents."},
				"name":{"type":"string","description":"New name for the agent."},
				"description":{"type":"string","description":"New one-sentence description."},
				"instructions":{"type":"string","description":"New system prompt / instructions."},
				"model":{"type":"string","description":"New model ID, e.g. claude-sonnet-4-6. Use list_available_models to verify it exists."},
				"temperature":{"type":"number","description":"New sampling temperature 0-2."},
				"max_tokens":{"type":"integer","description":"New max output tokens."},
				"memory_enabled":{"type":"boolean","description":"Enable or disable memory."},
				"memory_scope":{"type":"string","enum":["conversation","agent","workspace"],"description":"New memory scope."},
				"context_retrieval_enabled":{"type":"boolean","description":"Enable or disable retrieval-augmented context from connected data sources."},
				"agentic_rag":{"type":"boolean","description":"Let the agent decide when/what to retrieve via native_retrieve_context instead of pre-run injection. Requires context_retrieval_enabled=true."},
				"connector_ids":{"type":"array","items":{"type":"string"},"description":"Replace the agent's context connectors with these UUIDs. Requires context_retrieval_enabled=true. Get IDs from list_connectors."},
				"mcp_server_ids":{"type":"array","items":{"type":"string"},"description":"List of MCP server UUIDs. ALL tools belonging to each server are attached to the agent (additive — existing tools are kept). Use this to give an already-created agent an entire MCP integration at once. Get server IDs from list_mcp_servers."},
				"status":{"type":"string","enum":["active","paused","archived"],"description":"New agent status."}
			},
			"required":["agent_id"]
		}`),
	},
	{
		Name:        "attach_tool_to_agent",
		Description: "Attach a tool to an agent by tool name. The tool must exist in the workspace (use list_tools to find names). Has no effect if the tool is already attached.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"agent_id":{"type":"string","description":"UUID of the agent. Get from list_agents."},
				"tool_name":{"type":"string","description":"Exact name of the tool to attach, e.g. 'native_read_file'. Get from list_tools."}
			},
			"required":["agent_id","tool_name"]
		}`),
	},
	{
		Name:        "detach_tool_from_agent",
		Description: "Remove a tool from an agent. The tool is detached but not deleted from the workspace.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"agent_id":{"type":"string","description":"UUID of the agent. Get from list_agents."},
				"tool_name":{"type":"string","description":"Exact name of the tool to detach, e.g. 'native_read_file'. Get from list_tools."}
			},
			"required":["agent_id","tool_name"]
		}`),
	},
	{
		Name:        "attach_connector_to_agent",
		Description: "Attach a data-source connector to an agent for context retrieval. Enables context retrieval on the agent. Has no effect if already attached.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"agent_id":{"type":"string","description":"UUID of the agent. Get from list_agents."},
				"connector_id":{"type":"string","description":"UUID of the connector. Get from list_connectors."}
			},
			"required":["agent_id","connector_id"]
		}`),
	},
	{
		Name:        "detach_connector_from_agent",
		Description: "Remove a data-source connector from an agent. The connector is detached but not deleted from the workspace.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"agent_id":{"type":"string","description":"UUID of the agent. Get from list_agents."},
				"connector_id":{"type":"string","description":"UUID of the connector. Get from list_connectors."}
			},
			"required":["agent_id","connector_id"]
		}`),
	},
	{
		Name:        "run_workflow",
		Description: "Start a workflow run in the background and return immediately with the run ID. Use this to trigger an existing workflow with a given input.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"workflow_id":{"type":"string","description":"UUID of the workflow to run. Get from list_workflows."},
				"input":{"type":"string","description":"The input text to pass to the workflow."}
			},
			"required":["workflow_id","input"]
		}`),
	},
	{
		Name:        "create_code_tool",
		Description: "Create a permanent custom JavaScript tool that runs in a sandboxed JS engine. The code receives `input` (the tool call args as an object) and must return a value. No network or filesystem access.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Tool name (automatically prefixed with 'code_' if not already)"},
				"description":{"type":"string","description":"What this tool does — shown to the LLM when deciding to call it"},
				"code":{"type":"string","description":"JavaScript function body. Receives 'input' object with the tool call args. Must return a value."},
				"input_schema":{"type":"string","description":"JSON string describing the input parameters schema. Defaults to empty object if omitted."}
			},
			"required":["name","description","code"]
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
		Mode     string `json:"mode"`     // optional — "agent_builder" drafts an agent for form review
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
	systemContent := nexusSystemPrompt(h.cfg.PublicAppURL)
	if req.Mode == "agent_builder" {
		systemContent += "\n\n" + agentBuilderModePrompt
	}
	messages := []provider.Message{
		{Role: "system", Content: systemContent},
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

			// Draft proposals (propose_agent) stream the resolved spec to the UI
			// so the create form can pre-fill for review. Nothing is persisted.
			if result.Draft != nil {
				draftJSON, _ := json.Marshal(map[string]any{"type": "agent_draft", "draft": result.Draft})
				emit(string(draftJSON))
			}

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
	// Draft, when non-nil, carries a proposed (not-yet-persisted) agent spec.
	// The Chat loop emits it as an "agent_draft" SSE event so the UI can
	// pre-fill the create form for user review. Set only by toolProposeAgent.
	Draft any
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
	case "list_mcp_servers":
		return h.toolListMCPServers(ctx, ws)
	case "list_connectors":
		return h.toolListConnectors(ctx, ws)
	case "create_agent":
		return h.toolCreateAgent(ctx, ws, uid, input)
	case "propose_agent":
		return h.toolProposeAgent(ctx, ws, input)
	case "create_workflow":
		return h.toolCreateWorkflow(ctx, ws, uid, input)
	case "save_workflow_graph":
		return h.toolSaveGraph(ctx, ws, input)
	case "create_trigger":
		return h.toolCreateTrigger(ctx, ws, uid, input)
	case "list_gateway_channels":
		return h.toolListGatewayChannels(ctx, ws)
	case "create_gateway_channel":
		return h.toolCreateGatewayChannel(ctx, ws, uid, input)
	case "list_skills":
		return h.toolListSkills(ctx, ws)
	case "list_memories":
		return h.toolListMemories(ctx, ws, input)
	case "create_skill":
		return h.toolCreateSkill(ctx, ws, uid, input)
	case "attach_skills_to_agent":
		return h.toolAttachSkills(ctx, ws, input)
	case "update_agent":
		return h.toolUpdateAgent(ctx, ws, input)
	case "attach_tool_to_agent":
		return h.toolAttachToolToAgent(ctx, ws, input)
	case "detach_tool_from_agent":
		return h.toolDetachToolFromAgent(ctx, ws, input)
	case "attach_connector_to_agent":
		return h.toolAttachConnectorToAgent(ctx, ws, input)
	case "detach_connector_from_agent":
		return h.toolDetachConnectorFromAgent(ctx, ws, input)
	case "run_workflow":
		return h.toolRunWorkflow(ctx, ws, uid, input)
	case "create_code_tool":
		return h.toolCreateCodeTool(ctx, ws, input)
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

func (h *NexusAIHandler) toolListMCPServers(ctx context.Context, ws string) (*toolResult, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT s.id::text, s.name, s.status, COUNT(t.id) AS tool_count
		 FROM mcp_servers s
		 LEFT JOIN tools t ON t.type='mcp' AND (t.config->>'server_id')=s.id::text
		 WHERE s.workspace_id=$1::uuid
		 GROUP BY s.id, s.name, s.status
		 ORDER BY s.name`, ws)
	if err != nil {
		return nil, fmt.Errorf("failed to list mcp servers: %w", err)
	}
	defer rows.Close()
	type entry struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Status    string `json:"status"`
		ToolCount int    `json:"tool_count"`
	}
	var out []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.ID, &e.Name, &e.Status, &e.ToolCount); err != nil {
			continue
		}
		out = append(out, e)
	}
	return &toolResult{Label: fmt.Sprintf("Listed %d MCP servers", len(out)), Data: out}, nil
}

// resolveMCPServerToolNames expands MCP server IDs into the flat list of tool names each
// server exposes, so "enable this whole MCP server" can reuse the existing name-based
// attach logic (agent_tools INSERT ... SELECT id FROM tools WHERE name=...) instead of
// needing a separate bulk-attach code path.
func resolveMCPServerToolNames(ctx context.Context, pool *pgxpool.Pool, ws string, serverIDs []string) []string {
	if len(serverIDs) == 0 {
		return nil
	}
	rows, err := pool.Query(ctx,
		`SELECT name FROM tools
		 WHERE type='mcp' AND (config->>'server_id')=ANY($1) AND (workspace_id IS NULL OR workspace_id=$2::uuid)`,
		serverIDs, ws)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			names = append(names, n)
		}
	}
	return names
}

// dedupeStrings removes repeated entries while preserving first-seen order — used after
// merging explicit tool_names with names resolved from mcp_server_ids, since the same tool
// could be named both ways.
func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
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
		AgenticRAG              bool     `json:"agentic_rag"`
		ToolIDs                 []string `json:"tool_ids"`
		ToolNames               []string `json:"tool_names"`
		MCPServerIDs            []string `json:"mcp_server_ids"`
		ConnectorIDs            []string `json:"connector_ids"`
		MaxChunks               int      `json:"max_chunks"`
		MinScore                float64  `json:"min_score"`
		Status                  string   `json:"status"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid create_agent input: %w", err)
	}
	args.ToolNames = dedupeStrings(append(args.ToolNames, resolveMCPServerToolNames(ctx, h.pool, ws, args.MCPServerIDs)...))
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
		AgenticRAG:              args.AgenticRAG,
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

// toolProposeAgent resolves a proposed agent spec (tools/skills/connectors →
// real workspace records) and returns it as a Draft WITHOUT writing to the DB.
// The Chat loop emits it as an "agent_draft" SSE event; the UI pre-fills the
// create form for the user to review and save.
func (h *NexusAIHandler) toolProposeAgent(ctx context.Context, ws string, input json.RawMessage) (*toolResult, error) {
	var args struct {
		Name                    string          `json:"name"`
		Description             string          `json:"description"`
		Instructions            string          `json:"instructions"`
		Provider                string          `json:"provider"`
		Model                   string          `json:"model"`
		Temperature             float64         `json:"temperature"`
		MaxTokens               int             `json:"max_tokens"`
		MaxSteps                int             `json:"max_steps"`
		MaxToolCalls            int             `json:"max_tool_calls"`
		MaxDurationSecs         int             `json:"max_duration_secs"`
		MemoryEnabled           bool            `json:"memory_enabled"`
		MemoryScope             string          `json:"memory_scope"`
		ContextRetrievalEnabled bool            `json:"context_retrieval_enabled"`
		AgenticRAG              bool            `json:"agentic_rag"`
		ToolNames               []string        `json:"tool_names"`
		MCPServerIDs            []string        `json:"mcp_server_ids"`
		SkillNames              []string        `json:"skill_names"`
		ConnectorIDs            []string        `json:"connector_ids"`
		MaxChunks               int             `json:"max_chunks"`
		MinScore                float64         `json:"min_score"`
		Status                  string          `json:"status"`
		Rationale               json.RawMessage `json:"rationale"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid propose_agent input: %w", err)
	}
	args.ToolNames = dedupeStrings(append(args.ToolNames, resolveMCPServerToolNames(ctx, h.pool, ws, args.MCPServerIDs)...))
	if args.Name == "" || args.Instructions == "" {
		return nil, fmt.Errorf("propose_agent requires name and instructions")
	}

	// Defaults (mirror toolCreateAgent so the drafted form matches what Create would produce).
	if args.Provider == "" {
		// Pick the first active provider by priority.
		repo := repository.NewProviderRepository(h.pool)
		if creds, err := repo.List(ctx, ws); err == nil {
			for _, prio := range providerPriority {
				for _, c := range creds {
					if c.IsActive && c.Provider == prio {
						args.Provider = c.Provider
						break
					}
				}
				if args.Provider != "" {
					break
				}
			}
		}
	}
	if args.Model == "" && args.Provider != "" {
		args.Model = defaultModel(args.Provider)
	}
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
		args.MemoryScope = "agent"
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

	// Resolve tools by name → {id, name, description}. Unknown names are dropped
	// from the resolved set but reported in "unresolved" so the model can react.
	type namedRef struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	}
	resolvedTools := []namedRef{}
	unresolved := []string{}
	for _, name := range args.ToolNames {
		var ref namedRef
		err := h.pool.QueryRow(ctx,
			`SELECT id::text, name, COALESCE(description,'') FROM tools
			 WHERE name=$1 AND (workspace_id IS NULL OR workspace_id=$2::uuid) LIMIT 1`,
			name, ws).Scan(&ref.ID, &ref.Name, &ref.Description)
		if err != nil {
			unresolved = append(unresolved, "tool:"+name)
			continue
		}
		resolvedTools = append(resolvedTools, ref)
	}

	// Resolve skills by name → {id, name}.
	resolvedSkills := []namedRef{}
	for _, name := range args.SkillNames {
		var ref namedRef
		err := h.pool.QueryRow(ctx,
			`SELECT id::text, name FROM skills
			 WHERE name=$1 AND (workspace_id IS NULL OR workspace_id=$2::uuid) AND enabled=true
			 ORDER BY workspace_id NULLS LAST LIMIT 1`,
			name, ws).Scan(&ref.ID, &ref.Name)
		if err != nil {
			unresolved = append(unresolved, "skill:"+name)
			continue
		}
		resolvedSkills = append(resolvedSkills, ref)
	}

	// Born self-managing: ensure the Agent Self-Management skill is attached so the
	// new agent can attach/detach its own tools, skills, and context afterward.
	hasSelfMgmt := false
	for _, s := range resolvedSkills {
		if strings.EqualFold(s.Name, "Agent Self-Management") {
			hasSelfMgmt = true
			break
		}
	}
	if !hasSelfMgmt {
		var ref namedRef
		if err := h.pool.QueryRow(ctx,
			`SELECT id::text, name FROM skills
			 WHERE name='Agent Self-Management' AND (workspace_id IS NULL OR workspace_id=$1::uuid) AND enabled=true
			 ORDER BY workspace_id NULLS LAST LIMIT 1`,
			ws).Scan(&ref.ID, &ref.Name); err == nil {
			resolvedSkills = append(resolvedSkills, ref)
		}
	}

	// Resolve connectors by id → {id, name}.
	resolvedConnectors := []namedRef{}
	if args.ContextRetrievalEnabled {
		for _, cid := range args.ConnectorIDs {
			var ref namedRef
			err := h.pool.QueryRow(ctx,
				`SELECT id::text, name FROM connectors WHERE id=$1::uuid AND workspace_id=$2::uuid`,
				cid, ws).Scan(&ref.ID, &ref.Name)
			if err != nil {
				unresolved = append(unresolved, "connector:"+cid)
				continue
			}
			resolvedConnectors = append(resolvedConnectors, ref)
		}
	}

	var rationale any
	if len(args.Rationale) > 0 {
		_ = json.Unmarshal(args.Rationale, &rationale)
	}

	draft := map[string]any{
		"name":                      args.Name,
		"description":               args.Description,
		"instructions":             args.Instructions,
		"provider":                  args.Provider,
		"model":                     args.Model,
		"temperature":               args.Temperature,
		"max_tokens":                args.MaxTokens,
		"max_steps":                 args.MaxSteps,
		"max_tool_calls":            args.MaxToolCalls,
		"max_duration_secs":         args.MaxDurationSecs,
		"memory_enabled":            args.MemoryEnabled,
		"memory_scope":              args.MemoryScope,
		"context_retrieval_enabled": args.ContextRetrievalEnabled,
		"agentic_rag":               args.AgenticRAG,
		"max_chunks":                args.MaxChunks,
		"min_score":                 args.MinScore,
		"status":                    args.Status,
		// New agents are born self-managing (see plan Phase 4).
		"lazy_tool_loading": true,
		"tools":             resolvedTools,
		"skills":            resolvedSkills,
		"connectors":        resolvedConnectors,
		"rationale":         rationale,
		"unresolved":        unresolved,
	}

	label := fmt.Sprintf("Drafted agent \"%s\" for review", args.Name)
	return &toolResult{
		Label: label,
		Data:  draft,
		Draft: draft,
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

func (h *NexusAIHandler) toolListGatewayChannels(ctx context.Context, ws string) (*toolResult, error) {
	repo := repository.NewGatewayRepository(h.pool)
	list, err := repo.ListChannels(ctx, ws)
	if err != nil {
		return nil, fmt.Errorf("failed to list gateway channels: %w", err)
	}
	type entry struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		ChannelType string `json:"channel_type"`
		AgentID     string `json:"agent_id"`
		AgentName   string `json:"agent_name,omitempty"`
		IsActive    bool   `json:"is_active"`
	}
	var out []entry
	for _, c := range list {
		out = append(out, entry{
			ID:          c.ID,
			Name:        c.Name,
			Description: c.Description,
			ChannelType: c.ChannelType,
			AgentID:     c.AgentID,
			AgentName:   c.AgentName,
			IsActive:    c.IsActive,
		})
	}
	return &toolResult{Label: fmt.Sprintf("Listed %d gateway channels", len(out)), Data: out}, nil
}

func (h *NexusAIHandler) toolCreateGatewayChannel(ctx context.Context, ws, uid string, input json.RawMessage) (*toolResult, error) {
	var args struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		AgentID     string `json:"agent_id"`
		ChannelType string `json:"channel_type"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid create_gateway_channel input: %w", err)
	}
	if args.Name == "" || args.AgentID == "" || args.ChannelType == "" {
		return nil, fmt.Errorf("create_gateway_channel requires name, agent_id, channel_type")
	}
	if args.ChannelType != "http" && args.ChannelType != "whatsapp" {
		return nil, fmt.Errorf("channel_type must be 'http' or 'whatsapp'")
	}

	// Verify agent exists in this workspace
	var agentName string
	if err := h.pool.QueryRow(ctx,
		`SELECT name FROM agents WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		args.AgentID, ws).Scan(&agentName); err != nil {
		return nil, fmt.Errorf("agent not found: %s", args.AgentID)
	}

	cfg := domain.GatewayChannelConfig{
		AccountID:    "default",
		DMPolicy:     "pairing",
		SessionScope: "per-channel-peer",
		GroupPolicy:  "disabled",
		HistoryLimit: 50,
	}
	if args.ChannelType == "whatsapp" {
		cfg.AdapterURL = h.cfg.WhatsAppAdapterURL
		cfg.AssistantEnabled = true
		cfg.ChatApprovalsEnabled = true
	}
	cfgBytes, _ := json.Marshal(cfg)

	c := &domain.GatewayChannel{
		ID:          uuid.NewString(),
		WorkspaceID: ws,
		AgentID:     args.AgentID,
		Name:        args.Name,
		Description: args.Description,
		ChannelType: args.ChannelType,
		Config:      cfgBytes,
		IsActive:    true,
		CreatedBy:   uid,
	}
	repo := repository.NewGatewayRepository(h.pool)
	if err := repo.CreateChannel(ctx, c); err != nil {
		return nil, fmt.Errorf("failed to create gateway channel: %w", err)
	}

	note := ""
	if args.ChannelType == "whatsapp" {
		note = " — scan the QR code at Gateway → " + c.Name + " → QR Login to connect WhatsApp"
	}
	return &toolResult{
		Label:      fmt.Sprintf("Created %s channel \"%s\"%s", args.ChannelType, c.Name, note),
		ResultID:   c.ID,
		ResultName: c.Name,
		Link:       "/gateway/channels/" + c.ID,
		Data: map[string]any{
			"id":           c.ID,
			"name":         c.Name,
			"channel_type": c.ChannelType,
			"agent_id":     c.AgentID,
			"agent_name":   agentName,
		},
	}, nil
}

func (h *NexusAIHandler) toolListSkills(ctx context.Context, ws string) (*toolResult, error) {
	repo := repository.NewSkillRepository(h.pool)
	list, err := repo.List(ctx, ws)
	if err != nil {
		return nil, fmt.Errorf("failed to list skills: %w", err)
	}
	type entry struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Enabled     bool   `json:"enabled"`
	}
	var out []entry
	for _, s := range list {
		out = append(out, entry{ID: s.ID, Name: s.Name, Description: s.Description, Enabled: s.Enabled})
	}
	return &toolResult{Label: fmt.Sprintf("Listed %d skills", len(out)), Data: out}, nil
}

func (h *NexusAIHandler) toolListMemories(ctx context.Context, ws string, input json.RawMessage) (*toolResult, error) {
	var args struct {
		Limit int `json:"limit"`
	}
	_ = json.Unmarshal(input, &args)
	if args.Limit <= 0 || args.Limit > 100 {
		args.Limit = 20
	}
	rows, err := h.pool.Query(ctx,
		`SELECT id::text, COALESCE(agent_id::text,''), scope, content, relevance_score
		 FROM memories WHERE workspace_id=$1::uuid
		 ORDER BY relevance_score DESC, created_at DESC LIMIT $2`,
		ws, args.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list memories: %w", err)
	}
	defer rows.Close()
	type entry struct {
		ID        string  `json:"id"`
		AgentID   string  `json:"agent_id,omitempty"`
		Scope     string  `json:"scope"`
		Content   string  `json:"content"`
		Relevance float64 `json:"relevance_score"`
	}
	var out []entry
	for rows.Next() {
		var e entry
		if rows.Scan(&e.ID, &e.AgentID, &e.Scope, &e.Content, &e.Relevance) == nil {
			out = append(out, e)
		}
	}
	if out == nil {
		out = []entry{}
	}
	return &toolResult{Label: fmt.Sprintf("Listed %d memories", len(out)), Data: out}, nil
}

func (h *NexusAIHandler) toolCreateSkill(ctx context.Context, ws, uid string, input json.RawMessage) (*toolResult, error) {
	var args struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid create_skill input: %w", err)
	}
	if args.Name == "" || args.Content == "" {
		return nil, fmt.Errorf("create_skill requires name and content")
	}

	s := &domain.Skill{
		ID:          uuid.NewString(),
		WorkspaceID: ws,
		Name:        args.Name,
		Description: args.Description,
		Content:     args.Content,
		Source:      "manual",
		Enabled:     true,
		CreatedBy:   uid,
	}
	repo := repository.NewSkillRepository(h.pool)
	if err := repo.Create(ctx, s); err != nil {
		return nil, fmt.Errorf("failed to create skill: %w", err)
	}
	return &toolResult{
		Label:      fmt.Sprintf("Created skill \"%s\"", s.Name),
		ResultID:   s.ID,
		ResultName: s.Name,
		Link:       "/skills",
		Data:       map[string]string{"id": s.ID, "name": s.Name},
	}, nil
}

func (h *NexusAIHandler) toolAttachSkills(ctx context.Context, ws string, input json.RawMessage) (*toolResult, error) {
	var args struct {
		AgentID  string   `json:"agent_id"`
		SkillIDs []string `json:"skill_ids"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid attach_skills_to_agent input: %w", err)
	}
	if args.AgentID == "" {
		return nil, fmt.Errorf("attach_skills_to_agent requires agent_id")
	}

	// Verify agent exists in this workspace
	var agentName string
	if err := h.pool.QueryRow(ctx,
		`SELECT name FROM agents WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		args.AgentID, ws).Scan(&agentName); err != nil {
		return nil, fmt.Errorf("agent not found: %s", args.AgentID)
	}

	assignments := make([]domain.AgentSkillAssignment, 0, len(args.SkillIDs))
	for i, sid := range args.SkillIDs {
		assignments = append(assignments, domain.AgentSkillAssignment{
			SkillID:    sid,
			Enabled:    true,
			OrderIndex: i,
		})
	}
	repo := repository.NewSkillRepository(h.pool)
	if err := repo.SetForAgent(ctx, args.AgentID, assignments); err != nil {
		return nil, fmt.Errorf("failed to attach skills: %w", err)
	}
	return &toolResult{
		Label: fmt.Sprintf("Attached %d skill(s) to agent \"%s\"", len(assignments), agentName),
		Link:  "/agents/" + args.AgentID,
		Data: map[string]any{
			"agent_id":   args.AgentID,
			"agent_name": agentName,
			"skill_count": len(assignments),
		},
	}, nil
}

func (h *NexusAIHandler) toolUpdateAgent(ctx context.Context, ws string, input json.RawMessage) (*toolResult, error) {
	var args struct {
		AgentID                 string   `json:"agent_id"`
		Name                    string   `json:"name"`
		Description             string   `json:"description"`
		Instructions            string   `json:"instructions"`
		Model                   string   `json:"model"`
		Status                  string   `json:"status"`
		Temperature             *float64 `json:"temperature"`
		MaxTokens               *int     `json:"max_tokens"`
		MemoryEnabled           *bool    `json:"memory_enabled"`
		MemoryScope             string   `json:"memory_scope"`
		ContextRetrievalEnabled *bool    `json:"context_retrieval_enabled"`
		AgenticRAG              *bool    `json:"agentic_rag"`
		ConnectorIDs            []string `json:"connector_ids"`
		MCPServerIDs            []string `json:"mcp_server_ids"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid update_agent input: %w", err)
	}
	if args.AgentID == "" {
		return nil, fmt.Errorf("update_agent requires agent_id")
	}
	if args.Status != "" && args.Status != "active" && args.Status != "paused" && args.Status != "archived" {
		return nil, fmt.Errorf("status must be 'active', 'paused', or 'archived'")
	}

	// Verify agent exists in workspace.
	var agentName string
	if err := h.pool.QueryRow(ctx,
		`SELECT name FROM agents WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		args.AgentID, ws).Scan(&agentName); err != nil {
		return nil, fmt.Errorf("agent not found: %s", args.AgentID)
	}

	// Build dynamic SET clause — only update provided fields.
	type setClause struct {
		col string
		val any
	}
	var clauses []setClause
	if args.Name != "" {
		clauses = append(clauses, setClause{"name", args.Name})
		agentName = args.Name // reflect new name in label
	}
	if args.Description != "" {
		clauses = append(clauses, setClause{"description", args.Description})
	}
	if args.Instructions != "" {
		clauses = append(clauses, setClause{"instructions", args.Instructions})
	}
	if args.Model != "" {
		clauses = append(clauses, setClause{"model", args.Model})
	}
	if args.Status != "" {
		clauses = append(clauses, setClause{"status", args.Status})
	}
	if args.Temperature != nil {
		clauses = append(clauses, setClause{"temperature", *args.Temperature})
	}
	if args.MaxTokens != nil {
		clauses = append(clauses, setClause{"max_tokens", *args.MaxTokens})
	}
	if args.MemoryEnabled != nil {
		clauses = append(clauses, setClause{"memory_enabled", *args.MemoryEnabled})
	}
	if args.MemoryScope != "" {
		clauses = append(clauses, setClause{"memory_scope", args.MemoryScope})
	}
	if args.ContextRetrievalEnabled != nil {
		clauses = append(clauses, setClause{"context_retrieval_enabled", *args.ContextRetrievalEnabled})
	}
	if args.AgenticRAG != nil {
		clauses = append(clauses, setClause{"agentic_rag", *args.AgenticRAG})
	}
	if len(clauses) == 0 && args.ConnectorIDs == nil && len(args.MCPServerIDs) == 0 {
		return nil, fmt.Errorf("update_agent: no fields to update")
	}

	setCols := make([]string, 0, len(clauses)+1)
	vals := make([]any, 0, len(clauses)+2)
	for i, c := range clauses {
		setCols = append(setCols, fmt.Sprintf("%s=$%d", c.col, i+1))
		vals = append(vals, c.val)
	}
	setCols = append(setCols, "updated_at=NOW()")
	vals = append(vals, args.AgentID, ws)
	query := fmt.Sprintf(
		`UPDATE agents SET %s WHERE id=$%d::uuid AND workspace_id=$%d::uuid`,
		strings.Join(setCols, ", "),
		len(clauses)+1,
		len(clauses)+2,
	)

	if len(clauses) > 0 {
		tag, err := h.pool.Exec(ctx, query, vals...)
		if err != nil {
			return nil, fmt.Errorf("update_agent: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil, fmt.Errorf("agent not found")
		}
	}

	// Replace the agent's context connectors when connector_ids is provided.
	if args.ConnectorIDs != nil {
		tx, err := h.pool.Begin(ctx)
		if err == nil {
			tx.Exec(ctx, `DELETE FROM agent_connectors WHERE agent_id=$1::uuid`, args.AgentID) //nolint:errcheck
			for _, cid := range args.ConnectorIDs {
				tx.Exec(ctx, //nolint:errcheck
					`INSERT INTO agent_connectors(agent_id,connector_id,max_chunks,min_score)
					 SELECT $1::uuid,$2::uuid,8,0.5 WHERE EXISTS(
					   SELECT 1 FROM connectors WHERE id=$2::uuid AND workspace_id=$3::uuid)
					 ON CONFLICT(agent_id,connector_id) DO NOTHING`,
					args.AgentID, cid, ws)
			}
			tx.Commit(ctx) //nolint:errcheck
		}
	}

	// Attach every tool belonging to the given MCP servers — additive, existing tools
	// (including ones from other servers) are left untouched.
	attachedToolCount := 0
	if toolNames := resolveMCPServerToolNames(ctx, h.pool, ws, args.MCPServerIDs); len(toolNames) > 0 {
		tx, err := h.pool.Begin(ctx)
		if err == nil {
			for _, name := range toolNames {
				tx.Exec(ctx, //nolint:errcheck
					`INSERT INTO agent_tools(agent_id,tool_id,enabled)
					 SELECT $1::uuid,id,true FROM tools WHERE name=$2 AND (workspace_id IS NULL OR workspace_id=$3::uuid)
					 ON CONFLICT DO NOTHING`,
					args.AgentID, name, ws)
			}
			tx.Commit(ctx) //nolint:errcheck
			attachedToolCount = len(toolNames)
		}
	}

	label := fmt.Sprintf("Updated agent \"%s\"", agentName)
	if attachedToolCount > 0 {
		label += fmt.Sprintf(" [attached %d MCP tool(s)]", attachedToolCount)
	}

	return &toolResult{
		Label: label,
		Link:  "/agents/" + args.AgentID,
		Data:  map[string]any{"updated": true, "agent_id": args.AgentID},
	}, nil
}

func (h *NexusAIHandler) toolAttachToolToAgent(ctx context.Context, ws string, input json.RawMessage) (*toolResult, error) {
	var args struct {
		AgentID  string `json:"agent_id"`
		ToolName string `json:"tool_name"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid attach_tool_to_agent input: %w", err)
	}
	if args.AgentID == "" || args.ToolName == "" {
		return nil, fmt.Errorf("attach_tool_to_agent requires agent_id and tool_name")
	}

	// Verify agent exists in workspace.
	var agentName string
	if err := h.pool.QueryRow(ctx,
		`SELECT name FROM agents WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		args.AgentID, ws).Scan(&agentName); err != nil {
		return nil, fmt.Errorf("agent not found: %s", args.AgentID)
	}

	tag, err := h.pool.Exec(ctx,
		`INSERT INTO agent_tools(agent_id, tool_id, enabled)
		 SELECT $1::uuid, id, true FROM tools
		 WHERE name=$2 AND (workspace_id IS NULL OR workspace_id=$3::uuid)
		 ON CONFLICT DO NOTHING`,
		args.AgentID, args.ToolName, ws)
	if err != nil {
		return nil, fmt.Errorf("attach_tool_to_agent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either tool not found or already attached — check which.
		var toolExists bool
		_ = h.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM tools WHERE name=$1 AND (workspace_id IS NULL OR workspace_id=$2::uuid))`,
			args.ToolName, ws).Scan(&toolExists)
		if !toolExists {
			return nil, fmt.Errorf("tool not found: %s", args.ToolName)
		}
		// Already attached — treat as success.
	}

	return &toolResult{
		Label: fmt.Sprintf("Attached tool \"%s\" to agent \"%s\"", args.ToolName, agentName),
		Link:  "/agents/" + args.AgentID,
		Data:  map[string]any{"attached": true, "agent_id": args.AgentID, "tool_name": args.ToolName},
	}, nil
}

func (h *NexusAIHandler) toolDetachToolFromAgent(ctx context.Context, ws string, input json.RawMessage) (*toolResult, error) {
	var args struct {
		AgentID  string `json:"agent_id"`
		ToolName string `json:"tool_name"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid detach_tool_from_agent input: %w", err)
	}
	if args.AgentID == "" || args.ToolName == "" {
		return nil, fmt.Errorf("detach_tool_from_agent requires agent_id and tool_name")
	}

	// Verify agent exists in workspace.
	var agentName string
	if err := h.pool.QueryRow(ctx,
		`SELECT name FROM agents WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		args.AgentID, ws).Scan(&agentName); err != nil {
		return nil, fmt.Errorf("agent not found: %s", args.AgentID)
	}

	_, err := h.pool.Exec(ctx,
		`DELETE FROM agent_tools
		 WHERE agent_id=$1::uuid
		   AND tool_id=(
		       SELECT id FROM tools
		       WHERE name=$2 AND (workspace_id IS NULL OR workspace_id=$3::uuid)
		       LIMIT 1
		   )`,
		args.AgentID, args.ToolName, ws)
	if err != nil {
		return nil, fmt.Errorf("detach_tool_from_agent: %w", err)
	}

	return &toolResult{
		Label: fmt.Sprintf("Detached tool \"%s\" from agent \"%s\"", args.ToolName, agentName),
		Link:  "/agents/" + args.AgentID,
		Data:  map[string]any{"detached": true, "agent_id": args.AgentID, "tool_name": args.ToolName},
	}, nil
}

func (h *NexusAIHandler) toolAttachConnectorToAgent(ctx context.Context, ws string, input json.RawMessage) (*toolResult, error) {
	var args struct {
		AgentID     string `json:"agent_id"`
		ConnectorID string `json:"connector_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid attach_connector_to_agent input: %w", err)
	}
	if args.AgentID == "" || args.ConnectorID == "" {
		return nil, fmt.Errorf("attach_connector_to_agent requires agent_id and connector_id")
	}

	var agentName, connectorName string
	if err := h.pool.QueryRow(ctx,
		`SELECT name FROM agents WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		args.AgentID, ws).Scan(&agentName); err != nil {
		return nil, fmt.Errorf("agent not found: %s", args.AgentID)
	}
	if err := h.pool.QueryRow(ctx,
		`SELECT name FROM connectors WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		args.ConnectorID, ws).Scan(&connectorName); err != nil {
		return nil, fmt.Errorf("connector not found: %s", args.ConnectorID)
	}

	if _, err := h.pool.Exec(ctx,
		`INSERT INTO agent_connectors(agent_id, connector_id, enabled)
		 VALUES($1::uuid, $2::uuid, true)
		 ON CONFLICT(agent_id, connector_id) DO UPDATE SET enabled=true`,
		args.AgentID, args.ConnectorID); err != nil {
		return nil, fmt.Errorf("attach_connector_to_agent: %w", err)
	}
	h.pool.Exec(ctx, //nolint:errcheck
		`UPDATE agents SET context_retrieval_enabled=true, updated_at=NOW()
		 WHERE id=$1::uuid AND context_retrieval_enabled=false`, args.AgentID)

	return &toolResult{
		Label: fmt.Sprintf("Attached connector \"%s\" to agent \"%s\"", connectorName, agentName),
		Link:  "/agents/" + args.AgentID,
		Data:  map[string]any{"attached": true, "agent_id": args.AgentID, "connector_id": args.ConnectorID},
	}, nil
}

func (h *NexusAIHandler) toolDetachConnectorFromAgent(ctx context.Context, ws string, input json.RawMessage) (*toolResult, error) {
	var args struct {
		AgentID     string `json:"agent_id"`
		ConnectorID string `json:"connector_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid detach_connector_from_agent input: %w", err)
	}
	if args.AgentID == "" || args.ConnectorID == "" {
		return nil, fmt.Errorf("detach_connector_from_agent requires agent_id and connector_id")
	}

	var agentName string
	if err := h.pool.QueryRow(ctx,
		`SELECT name FROM agents WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		args.AgentID, ws).Scan(&agentName); err != nil {
		return nil, fmt.Errorf("agent not found: %s", args.AgentID)
	}

	if _, err := h.pool.Exec(ctx,
		`DELETE FROM agent_connectors WHERE agent_id=$1::uuid AND connector_id=$2::uuid`,
		args.AgentID, args.ConnectorID); err != nil {
		return nil, fmt.Errorf("detach_connector_from_agent: %w", err)
	}

	return &toolResult{
		Label: fmt.Sprintf("Detached connector from agent \"%s\"", agentName),
		Link:  "/agents/" + args.AgentID,
		Data:  map[string]any{"detached": true, "agent_id": args.AgentID, "connector_id": args.ConnectorID},
	}, nil
}

func (h *NexusAIHandler) toolRunWorkflow(ctx context.Context, ws, uid string, input json.RawMessage) (*toolResult, error) {
	var args struct {
		WorkflowID string `json:"workflow_id"`
		Input      string `json:"input"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid run_workflow input: %w", err)
	}
	if args.WorkflowID == "" {
		return nil, fmt.Errorf("run_workflow requires workflow_id")
	}
	if args.Input == "" {
		return nil, fmt.Errorf("run_workflow requires input")
	}
	if h.invoke == nil {
		return nil, fmt.Errorf("run_workflow is not available (invoke handler not wired)")
	}

	runID, err := h.invoke.runWorkflowInline(ctx, ws, uid, args.WorkflowID, args.Input, "")
	if err != nil {
		return nil, fmt.Errorf("run_workflow: %w", err)
	}

	return &toolResult{
		Label: fmt.Sprintf("Started workflow run (run_id: %s)", runID),
		Link:  "/runs/" + runID,
		Data:  map[string]any{"run_id": runID, "status": "running", "workflow_id": args.WorkflowID},
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
	case "propose_agent":
		if n, ok := m["name"].(string); ok && n != "" {
			return fmt.Sprintf("Drafting agent \"%s\"...", n)
		}
		return "Drafting agent..."
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
	case "list_mcp_servers":
		return "Listing MCP servers..."
	case "list_connectors":
		return "Listing available connectors..."
	case "list_gateway_channels":
		return "Listing gateway channels..."
	case "create_gateway_channel":
		if n, ok := m["name"].(string); ok && n != "" {
			return fmt.Sprintf("Creating gateway channel \"%s\"...", n)
		}
		return "Creating gateway channel..."
	case "list_skills":
		return "Listing skills..."
	case "list_memories":
		return "Reviewing memories..."
	case "create_skill":
		if n, ok := m["name"].(string); ok && n != "" {
			return fmt.Sprintf("Creating skill \"%s\"...", n)
		}
		return "Creating skill..."
	case "attach_skills_to_agent":
		return "Attaching skills to agent..."
	case "update_agent":
		if id, ok := m["agent_id"].(string); ok && id != "" {
			return fmt.Sprintf("Updating agent %s...", id)
		}
		return "Updating agent..."
	case "attach_tool_to_agent":
		if n, ok := m["tool_name"].(string); ok && n != "" {
			return fmt.Sprintf("Attaching tool \"%s\"...", n)
		}
		return "Attaching tool to agent..."
	case "detach_tool_from_agent":
		if n, ok := m["tool_name"].(string); ok && n != "" {
			return fmt.Sprintf("Detaching tool \"%s\"...", n)
		}
		return "Detaching tool from agent..."
	case "attach_connector_to_agent":
		return "Attaching connector to agent..."
	case "detach_connector_from_agent":
		return "Detaching connector from agent..."
	case "run_workflow":
		if id, ok := m["workflow_id"].(string); ok && id != "" {
			return fmt.Sprintf("Running workflow %s...", id)
		}
		return "Running workflow..."
	case "create_code_tool":
		if n, ok := m["name"].(string); ok && n != "" {
			return fmt.Sprintf("Creating code tool \"%s\"...", n)
		}
		return "Creating code tool..."
	}
	return strings.ReplaceAll(toolName, "_", " ") + "..."
}

func (h *NexusAIHandler) toolCreateCodeTool(ctx context.Context, ws string, raw json.RawMessage) (*toolResult, error) {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Code        string `json:"code"`
		InputSchema string `json:"input_schema"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("invalid create_code_tool input: %w", err)
	}
	if in.Name == "" || in.Description == "" || in.Code == "" {
		return nil, fmt.Errorf("create_code_tool requires name, description, code")
	}
	if !strings.HasPrefix(in.Name, "code_") {
		in.Name = "code_" + in.Name
	}
	var inputSchemaJSON json.RawMessage
	if in.InputSchema != "" {
		var parsed any
		if err := json.Unmarshal([]byte(in.InputSchema), &parsed); err != nil {
			return nil, fmt.Errorf("input_schema is not valid JSON: %w", err)
		}
		inputSchemaJSON = json.RawMessage(in.InputSchema)
	} else {
		inputSchemaJSON = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	configJSON, _ := json.Marshal(map[string]any{"language": "javascript", "code": in.Code})
	toolID := uuid.NewString()
	_, err := h.pool.Exec(ctx, `
		INSERT INTO tools(id, workspace_id, name, description, type, input_schema, output_schema, config, risk_level, requires_approval, timeout_ms, enabled, ephemeral)
		VALUES($1::uuid, $2::uuid, $3, $4, 'code', $5::jsonb, '{"type":"object"}'::jsonb, $6::jsonb, 'medium', false, 30000, true, false)`,
		toolID, ws, in.Name, in.Description,
		json.RawMessage(inputSchemaJSON), json.RawMessage(configJSON))
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			var existingID string
			_ = h.pool.QueryRow(ctx, `SELECT id::text FROM tools WHERE workspace_id=$1::uuid AND name=$2`, ws, in.Name).Scan(&existingID)
			return &toolResult{
				Label:      fmt.Sprintf("Code tool %q already exists", in.Name),
				ResultID:   existingID,
				ResultName: in.Name,
				Data:       map[string]any{"tool_id": existingID, "name": in.Name, "already_existed": true},
			}, nil
		}
		return nil, fmt.Errorf("failed to create code tool: %w", err)
	}
	return &toolResult{
		Label:      fmt.Sprintf("Created code tool %q", in.Name),
		ResultID:   toolID,
		ResultName: in.Name,
		Link:       "/tools",
		Data:       map[string]any{"tool_id": toolID, "name": in.Name},
	}, nil
}
