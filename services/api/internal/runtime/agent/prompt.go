package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
)

const (
	maxMemoryChars      = 300
	maxContextChunkChars = 500
)

// Builder assembles the messages slice from agent config, memories, context chunks, and history.
type Builder struct{}

func NewBuilder() *Builder { return &Builder{} }

// Build returns the full messages slice and the stable system portion.
// The stable portion (agent instructions + skills + memory policy) never changes between
// turns and is suitable for Anthropic prompt caching via cache_control.
func (b *Builder) Build(req BuildRequest) ([]provider.Message, string) {
	// Stable: derived purely from agent config — identical across all turns of a conversation.
	stable := req.SystemInstructions
	if len(req.Skills) > 0 {
		stable += "\n\nAdditional instructions:\n"
		for _, s := range req.Skills {
			if s != "" {
				stable += "\n" + s + "\n"
			}
		}
	}
	if req.MemoryEnabled {
		stable += "\n\nMemory policy:\n"
		if req.MemorySaveMode != "extractor" {
			stable += "- Use native_save_memory only for stable long-term preferences, durable facts, goals, or reusable decisions.\n"
		}
		stable += "- Do not save transient chat, secrets, credentials, private irrelevant details, or one-off requests.\n"
		stable += "- Keep saved memories compact and self-contained.\n"
	}

	if len(req.SelfManagementGroups) > 0 {
		stable += "\n\n## Self-Management Capabilities\n"
		stable += "You have tools to dynamically create, invoke, and destroy resources during this run:\n"
		for _, g := range req.SelfManagementGroups {
			switch g {
			case "agents":
				stable += "- **Agents**: native_list_agents, native_call_agent(agent_id, task), native_create_agent(name, instructions, provider, model, ...), native_delete_agent(agent_id)\n"
			case "skills":
				stable += "- **Skills**: native_list_skills, native_create_skill(name, content, attach_to_self, ephemeral), native_delete_skill(skill_id)\n"
			case "http_tools":
				stable += "- **HTTP Tools**: native_list_http_tools, native_create_http_tool(name, url, method, ...), native_delete_tool(tool_id)\n"
			}
		}
		stable += "\nGuidance:\n"
		if strings.Contains(strings.Join(req.SelfManagementGroups, ","), "agents") {
			stable += "- Call multiple native_call_agent in one response to run sub-agents in parallel.\n"
			stable += "- Create ephemeral agents for one-off specialist tasks (auto-deleted when this run ends).\n"
		}
		if strings.Contains(strings.Join(req.SelfManagementGroups, ","), "skills") {
			stable += "- Use native_create_skill with attach_to_self=true to inject domain knowledge into your own context.\n"
		}
		if strings.Contains(strings.Join(req.SelfManagementGroups, ","), "http_tools") {
			stable += "- Create an HTTP tool to call an external API, then use it in the next turn.\n"
		}
	}

	// Dynamic: timestamp + retrieved content — changes each turn.
	now := time.Now().UTC()
	dynamic := fmt.Sprintf("Current date and time (UTC): %s", now.Format("2006-01-02 15:04 Monday"))
	if len(req.MemorySummaries) > 0 {
		dynamic += "\n\nRelevant memory:\n"
		for _, m := range req.MemorySummaries {
			if m != "" {
				if len(m) > maxMemoryChars {
					m = m[:maxMemoryChars] + "…"
				}
				dynamic += "- " + m + "\n"
			}
		}
	}
	if len(req.ContextChunks) > 0 {
		dynamic += "\n\nRelevant context:\n"
		for _, c := range req.ContextChunks {
			if c != "" {
				if len(c) > maxContextChunkChars {
					c = c[:maxContextChunkChars] + "…"
				}
				dynamic += "- " + c + "\n"
			}
		}
	}

	system := stable
	if dynamic != "" {
		if system != "" {
			system += "\n\n"
		}
		system += dynamic
	}

	messages := []provider.Message{}
	if system != "" {
		messages = append(messages, provider.Message{Role: "system", Content: system})
	}
	messages = append(messages, req.History...)
	if req.UserMessage != "" {
		messages = append(messages, provider.Message{Role: "user", Content: req.UserMessage})
	}
	return messages, stable
}

type BuildRequest struct {
	SystemInstructions      string
	Skills                  []string
	MemorySummaries         []string
	ContextChunks           []string
	History                 []provider.Message
	UserMessage             string
	MemoryEnabled           bool
	MemorySaveMode          string
	// SelfManagementGroups lists which capability groups are available (e.g. "agents", "skills", "http_tools").
	// When non-empty, a capabilities block is injected into the stable system prompt.
	SelfManagementGroups    []string
}
