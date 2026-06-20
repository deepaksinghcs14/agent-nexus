package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
)

type streamedCompletion struct {
	Reply     string
	Usage     provider.Usage
	ToolCalls []provider.ToolCall
}

func collectStreamCompletion(ctx context.Context, stream <-chan provider.CompletionEvent, onDelta func(string)) (streamedCompletion, error) {
	out := streamedCompletion{}
	for event := range stream {
		switch event.Type {
		case provider.EventDelta:
			out.Reply += event.Delta
			if onDelta != nil {
				onDelta(event.Delta)
			}
		case provider.EventToolCall:
			if event.ToolCall != nil {
				out.ToolCalls = append(out.ToolCalls, *event.ToolCall)
			}
		case provider.EventDone:
			if event.Usage != nil {
				out.Usage = *event.Usage
			}
		case provider.EventError:
			if event.Error != nil {
				return streamedCompletion{}, event.Error
			}
			return streamedCompletion{}, fmt.Errorf("model call failed")
		}
	}
	if ctx.Err() != nil {
		return streamedCompletion{}, ctx.Err()
	}
	return out, nil
}

func completeWithEmptyRetry(
	ctx context.Context,
	llm provider.Provider,
	req provider.CompletionRequest,
	onDelta func(string),
	retryPrompt string,
) (streamedCompletion, error) {
	if retryPrompt == "" {
		retryPrompt = "Return a direct, non-empty reply."
	}

	for attempt := 0; attempt < 2; attempt++ {
		stream, err := llm.Complete(ctx, req)
		if err != nil {
			return streamedCompletion{}, err
		}
		out, err := collectStreamCompletion(ctx, stream, onDelta)
		if err != nil {
			return streamedCompletion{}, err
		}
		if len(out.ToolCalls) == 0 && strings.TrimSpace(out.Reply) == "" {
			if attempt == 0 {
				req.Messages = append(req.Messages, provider.Message{Role: "system", Content: retryPrompt})
				continue
			}
			return streamedCompletion{}, fmt.Errorf("model returned an empty response")
		}
		return out, nil
	}

	return streamedCompletion{}, fmt.Errorf("model returned an empty response")
}
