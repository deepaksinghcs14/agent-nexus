package native

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
)

// SendMessageTool lets an agent push a freeform message to the user mid-run.
type SendMessageTool struct{ tools.RequiresRunContext }

func NewSendMessageTool() *SendMessageTool { return &SendMessageTool{} }

func (t *SendMessageTool) Definition() domain.Tool {
	return domain.Tool{
		Name:        "native_send_message",
		Description: "Send a message to the user right now, mid-run. Use this to share progress, partial findings, or estimated wait time — especially useful on chat/messaging channels where the user is waiting. Returns immediately; does not wait for a reply.",
		Type:        "native",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","description":"The message text to send to the user"}},"required":["text"]}`),
		RiskLevel:   "low",
		TimeoutMs:   5000,
	}
}

func (t *SendMessageTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	text, _ := input["text"].(string)
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}
	if execCtx.SendMessage == nil {
		return map[string]any{"sent": false, "reason": "no active channel"}, nil
	}
	if err := execCtx.SendMessage(ctx, text); err != nil {
		return map[string]any{"sent": false, "reason": err.Error()}, nil
	}
	return map[string]any{"sent": true}, nil
}

// AskUserTool pauses the run and waits for the user to answer a question.
type AskUserTool struct{ tools.RequiresRunContext }

func NewAskUserTool() *AskUserTool { return &AskUserTool{} }

func (t *AskUserTool) Definition() domain.Tool {
	return domain.Tool{
		Name:        "native_ask_user",
		Description: "Pause and ask the user a question. Execution resumes when the user replies. Returns {\"answer\": \"...\"}. Use for clarifications only the user can provide.",
		Type:        "native",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"question":{"type":"string","description":"The question to ask the user"}},"required":["question"]}`),
		RiskLevel:   "low",
		TimeoutMs:   1800000,
	}
}

func (t *AskUserTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	question, _ := input["question"].(string)
	if question == "" {
		return nil, fmt.Errorf("question is required")
	}
	if execCtx.WaitForUserInput == nil {
		return map[string]any{"answer": "", "error": "human input not available in this context"}, nil
	}
	answer, err := execCtx.WaitForUserInput(ctx, question)
	if err != nil {
		return nil, err
	}
	return map[string]any{"answer": answer}, nil
}
