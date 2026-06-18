package agent

import (
	"fmt"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
)

// Builder assembles the messages slice from agent config, memories, context chunks, and history.
type Builder struct{}

func NewBuilder() *Builder { return &Builder{} }

func (b *Builder) Build(req BuildRequest) []provider.Message {
	now := time.Now().UTC()
	system := fmt.Sprintf("Current date and time (UTC): %s\n\n", now.Format("2006-01-02 15:04 Monday")) + req.SystemInstructions
	if len(req.Skills) > 0 {
		system += "\n\nAdditional instructions:\n"
		for _, s := range req.Skills {
			if s != "" {
				system += "\n" + s + "\n"
			}
		}
	}
	if len(req.MemorySummaries) > 0 {
		system += "\n\nRelevant memory:\n"
		for _, m := range req.MemorySummaries {
			if m != "" {
				system += "- " + m + "\n"
			}
		}
	}
	if len(req.ContextChunks) > 0 {
		system += "\n\nRelevant context:\n"
		for _, c := range req.ContextChunks {
			if c != "" {
				system += "- " + c + "\n"
			}
		}
	}
	if req.MemoryEnabled {
		system += "\n\nMemory policy:\n"
		if req.MemorySaveMode != "extractor" {
			system += "- Use native_save_memory only for stable long-term preferences, durable facts, goals, or reusable decisions.\n"
		}
		system += "- Do not save transient chat, secrets, credentials, private irrelevant details, or one-off requests.\n"
		system += "- Keep saved memories compact and self-contained.\n"
	}

	messages := []provider.Message{}
	if system != "" {
		messages = append(messages, provider.Message{Role: "system", Content: system})
	}
	messages = append(messages, req.History...)
	if req.UserMessage != "" {
		messages = append(messages, provider.Message{Role: "user", Content: req.UserMessage})
	}
	return messages
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
}
