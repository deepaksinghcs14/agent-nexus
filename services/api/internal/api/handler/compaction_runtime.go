package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/jackc/pgx/v5/pgxpool"
)

// compactConversation reads the last 8 messages for convID, folds them into existingCompaction,
// and returns a new rolling summary produced by the LLM (temp=0, max 500 tokens).
// Returns ("", nil) if there are no messages to compact.
func compactConversation(ctx context.Context, pool *pgxpool.Pool, llm provider.Provider,
	model, convID, existingCompaction string) (string, error) {

	rows, err := pool.Query(ctx,
		`SELECT role, content FROM messages
		 WHERE conversation_id=$1::uuid AND (role='user' OR role='assistant')
		 ORDER BY created_at DESC LIMIT 8`,
		convID)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var msgs []struct{ role, content string }
	for rows.Next() {
		var role, content string
		if rows.Scan(&role, &content) == nil {
			msgs = append(msgs, struct{ role, content string }{role, content})
		}
	}
	if len(msgs) == 0 {
		return "", nil
	}
	// Reverse to chronological order (query returned newest-first).
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	var transcript strings.Builder
	for _, m := range msgs {
		label := "User"
		if m.role == "assistant" {
			label = "Assistant"
		}
		content := m.content
		if len(content) > 600 {
			content = content[:600] + "…"
		}
		fmt.Fprintf(&transcript, "%s: %s\n\n", label, content)
	}

	userContent := transcript.String()
	if existingCompaction != "" {
		userContent = "Existing compaction:\n" + existingCompaction + "\n\nRecent messages to fold in:\n" + userContent
	}

	stream, err := llm.Complete(ctx, provider.CompletionRequest{
		Model: model,
		Messages: []provider.Message{
			{
				Role: "system",
				Content: "You are a conversation compactor. Given an existing compaction (if any) and recent messages, " +
					"produce an updated rolling summary in 3-5 sentences. Write in third person. " +
					"Preserve decisions, tasks, preferences, and open threads. Drop pleasantries. " +
					"Return ONLY the compacted text — no preamble.",
			},
			{Role: "user", Content: userContent},
		},
		Temperature: 0,
		MaxTokens:   500,
		Stream:      true,
	})
	if err != nil {
		return "", err
	}

	var result strings.Builder
	for event := range stream {
		switch event.Type {
		case provider.EventDelta:
			result.WriteString(event.Delta)
		case provider.EventError:
			return "", event.Error
		}
	}
	return strings.TrimSpace(result.String()), nil
}
