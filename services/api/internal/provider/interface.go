package provider

import (
	"context"
	"encoding/json"
)

// Provider is the interface every LLM adapter must implement.
type Provider interface {
	// Complete streams a chat completion. Callers read from the returned channel
	// until it is closed or an error event is received.
	Complete(ctx context.Context, req CompletionRequest) (<-chan CompletionEvent, error)

	// Embed returns a vector embedding for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// Models returns the list of models available for this provider credential.
	Models(ctx context.Context) ([]ModelInfo, error)

	// Name returns the canonical provider name (anthropic | openai | gemini | nvidia | ollama).
	Name() string
}

// CompletionRequest is the normalised request sent to any provider.
type CompletionRequest struct {
	Model               string
	Messages            []Message
	Tools               []ToolDefinition
	Temperature         float64
	MaxTokens           int
	Stream              bool
	StableSystemContent string // non-empty → Anthropic caches this prefix via cache_control
}

// Message is a normalised chat message.
type Message struct {
	Role       string     `json:"role"`                   // system | user | assistant | tool
	Content    string     `json:"content"`                // message content
	ToolCallID string     `json:"tool_call_id,omitempty"` // set when Role == "tool"
	ToolName   string     `json:"name,omitempty"`         // set when Role == "tool"
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // set when Role == "assistant" and model called tools
	IsError    bool       `json:"is_error,omitempty"`     // set when Role == "tool" and the tool call failed
	// Images and Audio carry inline media for this message only. Callers set
	// these on the current turn's freshly-built user message (e.g. a WhatsApp
	// inbound image/voice note) — conversation history reloaded from the DB
	// on a later turn is text-only, so these never round-trip back in.
	// Audio is consumed only by clients whose API accepts the format the
	// source actually sends (see gemini/client.go); others fall back to
	// Content's placeholder/caption text.
	Images []MediaBlock `json:"images,omitempty"`
	Audio  []MediaBlock `json:"audio,omitempty"`
}

// MediaBlock is inline binary media attached to a single message.
type MediaBlock struct {
	MIMEType string `json:"mime_type"`
	Data     []byte `json:"data"`
}

// ToolDefinition describes a tool the model may call.
type ToolDefinition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// ToolCall is a model-requested tool invocation.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"arguments"`
	// ThoughtSignature is Gemini's opaque reasoning token attached to a
	// function call; it must be echoed back verbatim on the next turn or the
	// API rejects the request. Other providers leave it empty. The json tag
	// keeps it intact across wait-state snapshots.
	ThoughtSignature string `json:"thought_signature,omitempty"`
}

// CompletionEvent is one event from the streaming response.
type CompletionEvent struct {
	Type     EventType
	Delta    string    // set when Type == EventDelta
	ToolCall *ToolCall // set when Type == EventToolCall
	Usage    *Usage    // set when Type == EventDone
	Error    error     // set when Type == EventError
}

type EventType string

const (
	EventDelta    EventType = "delta"
	EventToolCall EventType = "tool_call"
	EventDone     EventType = "done"
	EventError    EventType = "error"
)

// Usage holds token counts from the completed response.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// ModelInfo describes a single model.
type ModelInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ContextWindow  int    `json:"context_window"`
	SupportsTools  bool   `json:"supports_tools"`
	SupportsVision bool   `json:"supports_vision"`
}
