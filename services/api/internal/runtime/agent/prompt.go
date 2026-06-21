package agent

import (
	"fmt"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
)

const (
	maxMemoryChars       = 300
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
	stable += "\n\nNever promise future work unless an available tool has started it and returned a confirmed result."
	stable += "\n\nMulti-step tasks: when the user gives you a numbered list of steps or asks you to create/build multiple things, execute ALL steps without pausing to ask for confirmation or summarising mid-way. Only respond to the user after every step is done."
	if len(req.Skills) > 0 {
		stable += "\n\nAdditional instructions:\n"
		for _, s := range req.Skills {
			if s != "" {
				stable += "\n" + s + "\n"
			}
		}
	}
	if req.HasCallAgent {
		stable += "\n\nParallelism rule: when you need to call native_call_agent for multiple independent tasks, return ALL of those calls in a single response — they execute concurrently and are much faster than calling them one at a time. Only chain calls sequentially when the output of one is required as input to the next."
	}
	if req.HasCreateAgent && req.HasCallAgent {
		stable += "\n\nDelegation pattern: native_create_agent only registers an agent — it does NOT run it. After creating agents you MUST call native_call_agent with each returned agent_id before finishing. Never end a turn after create_agent calls without immediately following up with call_agent calls."
	}
	if req.LazyToolLoading {
		stable += "\n\nLazy tool loading is ON. You start with only a small set of meta-tools visible. Strategy for multi-step tasks: (1) call native_list_tools ONCE at the start to see the COMPLETE list of all tools available; (2) in the same turn, call native_request_tool for EVERY tool you will need — batch all requests together; (3) from the next turn onward, all requested tools are active and you can call them directly. Never invent or guess tool names. Never stop between steps to ask the user what to do next — execute ALL steps until the task is fully complete, then give a final summary."
	}
	stable += "\n\nMemory: use native_list_memories to inspect, native_request_memory to load, and native_save_memory to store only durable, non-sensitive facts."
	stable += "\n\nFor any question about the user's identity, preferences, goals, projects, or prior decisions, first call native_list_memories before answering."
	if req.MemoryEnabled {
		stable += "\n\nMemory policy: save only stable preferences, durable facts, goals, or reusable decisions; keep them compact and never save secrets or transient chat."
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
	if req.ConvCompaction != "" {
		dynamic += "\n\nEarlier conversation (compacted):\n" + req.ConvCompaction
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
	SystemInstructions string
	Skills             []string
	MemorySummaries    []string
	ContextChunks      []string
	History            []provider.Message
	UserMessage        string
	MemoryEnabled      bool
	MemorySaveMode     string
	// HasCallAgent, when true, injects a parallelism instruction telling the LLM to
	// batch independent native_call_agent calls in a single response for concurrent execution.
	HasCallAgent bool
	// HasCreateAgent, when true alongside HasCallAgent, injects a delegation pattern reminder
	// that create_agent does not run the agent — call_agent must follow immediately.
	HasCreateAgent bool
	// LazyToolLoading requires tool schemas to be requested before use.
	LazyToolLoading bool
	// ConvCompaction is a rolling LLM-generated summary of older conversation turns.
	// Injected into the dynamic section so it doesn't break Anthropic prompt-cache on the stable prefix.
	ConvCompaction string
}
