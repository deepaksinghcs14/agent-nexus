package agent

import "github.com/agentNexus/agent-nexus/services/api/internal/provider"

// Builder assembles the messages slice from agent config, memories, context chunks, and history.
type Builder struct{}

func NewBuilder() *Builder { return &Builder{} }

func (b *Builder) Build(req BuildRequest) []provider.Message {
	system := req.SystemInstructions
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
	MemorySummaries    []string
	ContextChunks      []string
	History            []provider.Message
	UserMessage        string
}
