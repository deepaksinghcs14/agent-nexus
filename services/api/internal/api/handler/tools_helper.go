package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

// loadAgentToolDefs returns the tool definitions to pass to the LLM and a name→Tool map
// for risk/approval checks. Only tools enabled for the agent are returned.
func loadAgentToolDefs(ctx context.Context, pool *pgxpool.Pool, agentID string) ([]provider.ToolDefinition, map[string]domain.Tool, error) {
	rows, err := pool.Query(ctx,
		`SELECT t.name, t.description, t.type, t.input_schema, t.config, t.risk_level, t.requires_approval, t.timeout_ms
		 FROM agent_tools at
		 JOIN tools t ON t.id = at.tool_id
		 WHERE at.agent_id=$1::uuid AND COALESCE(at.enabled, t.enabled) = true
		 ORDER BY t.name`,
		agentID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var defs []provider.ToolDefinition
	nameMap := map[string]domain.Tool{}

	for rows.Next() {
		var t domain.Tool
		var inputSchema, cfg []byte
		if err := rows.Scan(&t.Name, &t.Description, &t.Type, &inputSchema, &cfg, &t.RiskLevel, &t.RequiresApproval, &t.TimeoutMs); err != nil {
			continue
		}
		t.InputSchema = json.RawMessage(inputSchema)
		if len(cfg) > 0 {
			t.Config = json.RawMessage(cfg)
		}
		defs = append(defs, provider.ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
		nameMap[t.Name] = t
	}
	return defs, nameMap, rows.Err()
}

// loadWorkspaceToolCatalog returns every enabled tool visible in the workspace, regardless of
// whether it's attached to any particular agent via agent_tools. Used to populate discovery
// (native_list_tools) and to let native_request_tool activate a workspace tool the agent wasn't
// pre-wired with — loadAgentToolDefs stays attachment-scoped since it also drives which tools
// are actually offered to the LLM by default and the risk/approval map.
func loadWorkspaceToolCatalog(ctx context.Context, pool *pgxpool.Pool, workspaceID string) ([]provider.ToolDefinition, map[string]domain.Tool, error) {
	rows, err := pool.Query(ctx,
		`SELECT t.name, t.description, t.type, t.input_schema, t.config, t.risk_level, t.requires_approval, t.timeout_ms
		 FROM tools t
		 WHERE (t.workspace_id IS NULL OR t.workspace_id=$1::uuid) AND t.enabled = true
		 ORDER BY t.name`,
		workspaceID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var defs []provider.ToolDefinition
	nameMap := map[string]domain.Tool{}
	for rows.Next() {
		var t domain.Tool
		var inputSchema, cfg []byte
		if err := rows.Scan(&t.Name, &t.Description, &t.Type, &inputSchema, &cfg, &t.RiskLevel, &t.RequiresApproval, &t.TimeoutMs); err != nil {
			continue
		}
		t.InputSchema = json.RawMessage(inputSchema)
		if len(cfg) > 0 {
			t.Config = json.RawMessage(cfg)
		}
		defs = append(defs, provider.ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
		nameMap[t.Name] = t
	}
	return defs, nameMap, rows.Err()
}

var metaToolNames = []string{
	"native_list_tools",
	"native_request_tool",
	"native_list_agent_skills",
	"native_request_skill",
	"native_list_memories",
	"native_request_memory",
	"native_save_memory",
}

// metaToolNameSet returns the set of always-active meta-tool names for use in AlwaysActiveTools.
func metaToolNameSet() map[string]bool {
	s := make(map[string]bool, len(metaToolNames))
	for _, n := range metaToolNames {
		s[n] = true
	}
	return s
}

// lazyMetaToolDefs returns the meta-tools always included in lazy-loading mode.
func lazyMetaToolDefs(reg *tools.Registry) []provider.ToolDefinition {
	names := metaToolNames
	var defs []provider.ToolDefinition
	for _, name := range names {
		t, err := reg.Get(name)
		if err != nil {
			continue
		}
		d := t.Definition()
		defs = append(defs, provider.ToolDefinition{
			Name:        d.Name,
			Description: d.Description,
			InputSchema: d.InputSchema,
		})
	}
	return defs
}

func ensureMemoryToolDefs(defs []provider.ToolDefinition, nameMap map[string]domain.Tool) ([]provider.ToolDefinition, map[string]domain.Tool) {
	if nameMap == nil {
		nameMap = map[string]domain.Tool{}
	}
	add := func(name, description string, schema json.RawMessage, risk string, timeoutMs int) {
		if _, ok := nameMap[name]; ok {
			return
		}
		defs = append(defs, provider.ToolDefinition{
			Name:        name,
			Description: description,
			InputSchema: schema,
		})
		nameMap[name] = domain.Tool{
			Name:             name,
			Type:             "native",
			InputSchema:      schema,
			RiskLevel:        risk,
			RequiresApproval: false,
			TimeoutMs:        timeoutMs,
			Enabled:          true,
		}
	}

	memorySearchSchema := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Optional search text. Leave empty to list the most recent relevant memories."},"limit":{"type":"integer","minimum":1,"maximum":10,"description":"Maximum number of memories to return. Default 5."}}}`)
	memoryRequestSchema := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search text describing the memories you want to load into the current run context."},"limit":{"type":"integer","minimum":1,"maximum":10,"description":"Maximum number of memories to inject. Default 5."}},"required":["query"]}`)
	memorySaveSchema := json.RawMessage(`{"type":"object","properties":{"content":{"type":"string","description":"A compact durable fact, preference, goal, or decision to remember. Do not include secrets or transient chat."},"importance_score":{"type":"number","description":"0 to 1 score for long-term usefulness."},"reason":{"type":"string","description":"Short reason this should be remembered."}},"required":["content","importance_score","reason"]}`)

	add("native_list_memories", "List relevant memories on demand. Use this when earlier preferences, facts, or decisions matter.", memorySearchSchema, "low", 2000)
	add("native_request_memory", "Search memories and inject the matching results into the current run context for subsequent turns.", memoryRequestSchema, "low", 2000)
	add("native_save_memory", "Save a durable memory for future runs when the user reveals a stable preference, fact, goal, or reusable decision.", memorySaveSchema, "low", 5000)
	return defs, nameMap
}

// summarizeToolCall returns a compact, single-line description of a tool call's effect for
// inclusion in conversation history. Only the bare assistant reply text is normally persisted
// across runs, which lets the model mistake "what I said" for "what I actually did" on a
// follow-up turn (e.g. repeating a send request gets answered with the prior reply text instead
// of a new send). Logging real actions here gives future turns the missing context.
func summarizeToolCall(name string, input json.RawMessage, errMsg string) string {
	in := strings.TrimSpace(string(input))
	if len(in) > 160 {
		in = in[:160] + "…"
	}
	if errMsg != "" {
		return fmt.Sprintf("%s(%s) → failed: %s", name, in, errMsg)
	}
	return fmt.Sprintf("%s(%s) → done", name, in)
}

// injectActionLog parses the action-log JSON stored in an assistant message's tool_calls
// column (a plain []string of summarizeToolCall entries) and prepends a compact note to the
// content used to rebuild LLM context for a later run. The raw DB content (and anything
// returned by ListMessages for display in the UI) is left untouched — this only affects
// what the model sees when conversation history is reloaded for a new turn.
func injectActionLog(role, content, toolCallsRaw string) string {
	if role != "assistant" || toolCallsRaw == "" {
		return content
	}
	var actions []string
	if err := json.Unmarshal([]byte(toolCallsRaw), &actions); err != nil || len(actions) == 0 {
		return content
	}
	return "[actions taken: " + strings.Join(actions, "; ") + "]\n" + content
}

// onDemandSkillsInstructions builds the system-prompt section describing on-demand skills.
// Listing each skill's name + description up front (rather than a generic "check skills if
// relevant" pointer) is what actually gets the model to activate the right skill before
// answering — without this, a model can satisfy a request like "send a message to X" by just
// replying conversationally, never realizing a skill exists that says it must use a tool instead.
func onDemandSkillsInstructions(onDemand map[string]domain.Skill) string {
	if len(onDemand) == 0 {
		return ""
	}
	names := make([]string, 0, len(onDemand))
	for name := range onDemand {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("\n\nAdditional on-demand skills are available. Before responding to any request, check whether one of these matches — if so, call native_request_skill(name) FIRST and follow its instructions; do not answer the request yourself or claim you can't do it.\n")
	for _, name := range names {
		b.WriteString("- " + name + ": " + onDemand[name].Description + "\n")
	}
	return b.String()
}

// lazyToolNotActiveError builds the error returned to the model when it calls a tool that
// wasn't actually offered this turn under lazy tool loading — forcing it through
// native_request_tool/native_request_skill instead of guessing names from native_list_tools.
func lazyToolNotActiveError(toolName string, skillToolMap map[string]string) string {
	if skillName, ok := skillToolMap[toolName]; ok {
		return fmt.Sprintf("tool %q is not active yet — it belongs to on-demand skill %q. Call native_request_skill(%q) (or native_request_tool(%q)) first, then call %q again next turn.",
			toolName, skillName, skillName, toolName, toolName)
	}
	return fmt.Sprintf("tool %q is not active this turn — call native_request_tool(%q) first, then call it again next turn.", toolName, toolName)
}

// unattachedCatalogToolError distinguishes "this tool exists in the workspace but isn't
// enabled for this agent" from a genuinely unknown/hallucinated tool name. It only fires for
// non-native tool types (http/mcp/code) — those have no entry in the native Go registry, so
// without this check a call to one that's visible via native_list_tools (workspace-wide
// discovery) but not attached would otherwise fall through to the executor and fail with the
// unhelpful "tool ... not found in registry". Native tools missing from dbTools still resolve
// fine via the registry fallback, so they're left alone here.
func unattachedCatalogToolError(toolName string, catalog map[string]domain.Tool, lazy bool) (string, bool) {
	t, ok := catalog[toolName]
	if !ok || t.Type == "native" {
		return "", false
	}
	if lazy {
		return fmt.Sprintf("tool %q exists in the workspace (type=%s) but isn't active this turn — call native_request_tool(%q) first, then call it again next turn.", toolName, t.Type, toolName), true
	}
	return fmt.Sprintf("tool %q exists in the workspace (type=%s) but is not attached to this agent, so it can't be called. Ask an admin to attach it from the agent's Tools tab, or enable lazy tool loading so the agent can self-request it.", toolName, t.Type), true
}

// dedupeToolDefs removes repeated tool names while preserving the first
// declaration order. Gemini rejects duplicate function declarations outright,
// and the same rule keeps the other providers' tool lists clean.
func dedupeToolDefs(defs []provider.ToolDefinition) []provider.ToolDefinition {
	if len(defs) < 2 {
		return defs
	}
	seen := make(map[string]struct{}, len(defs))
	out := make([]provider.ToolDefinition, 0, len(defs))
	for _, def := range defs {
		if def.Name == "" {
			continue
		}
		if _, ok := seen[def.Name]; ok {
			continue
		}
		seen[def.Name] = struct{}{}
		out = append(out, def)
	}
	return out
}
