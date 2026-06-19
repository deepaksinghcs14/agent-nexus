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
	if req.MemoryEnabled {
		stable += "\n\nMemory policy:\n"
		if req.MemorySaveMode != "extractor" {
			stable += "- Use native_save_memory only for stable long-term preferences, durable facts, goals, or reusable decisions.\n"
		}
		stable += "- Do not save transient chat, secrets, credentials, private irrelevant details, or one-off requests.\n"
		stable += "- Keep saved memories compact and self-contained.\n"
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
}
