package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
)

type Client struct {
	apiKey  string
	baseURL string
}

func New(apiKey, baseURL string) *Client {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &Client{apiKey: apiKey, baseURL: baseURL}
}

func (c *Client) Complete(ctx context.Context, req provider.CompletionRequest) (<-chan provider.CompletionEvent, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("openai: api key is required")
	}
	body := map[string]any{
		"model":       req.Model,
		"messages":    openAIMessages(req.Messages),
		"temperature": req.Temperature,
		"stream":      true,
		"stream_options": map[string]any{
			"include_usage": true,
		},
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		body["tools"] = openAITools(req.Tools)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		defer res.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("openai: %s: %s", res.Status, strings.TrimSpace(string(b)))
	}

	events := make(chan provider.CompletionEvent)
	go streamOpenAI(res.Body, events)
	return events, nil
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, fmt.Errorf("openai: Embed not implemented")
}

func (c *Client) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	if c.apiKey == "" {
		return openAIFallback(), nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.baseURL, "/")+"/v1/models", nil)
	if err != nil {
		return openAIFallback(), nil
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return openAIFallback(), nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return openAIFallback(), nil
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return openAIFallback(), nil
	}
	models := make([]provider.ModelInfo, 0)
	for _, m := range body.Data {
		if !strings.HasPrefix(m.ID, "gpt-") && !strings.HasPrefix(m.ID, "o1") && !strings.HasPrefix(m.ID, "o3") && !strings.HasPrefix(m.ID, "o4") {
			continue
		}
		if strings.Contains(m.ID, "instruct") || strings.Contains(m.ID, "0301") || strings.Contains(m.ID, "0314") || strings.Contains(m.ID, "0613") {
			continue
		}
		supportsVision := strings.Contains(m.ID, "gpt-4") || strings.Contains(m.ID, "4o")
		models = append(models, provider.ModelInfo{
			ID:             m.ID,
			Name:           m.ID,
			ContextWindow:  128000,
			SupportsTools:  true,
			SupportsVision: supportsVision,
		})
	}
	if len(models) == 0 {
		return openAIFallback(), nil
	}
	return models, nil
}

func openAIFallback() []provider.ModelInfo {
	return []provider.ModelInfo{
		{ID: "gpt-4o", Name: "GPT-4o", ContextWindow: 128000, SupportsTools: true, SupportsVision: true},
		{ID: "gpt-4o-mini", Name: "GPT-4o Mini", ContextWindow: 128000, SupportsTools: true, SupportsVision: true},
		{ID: "o1", Name: "o1", ContextWindow: 200000, SupportsTools: false, SupportsVision: false},
	}
}

func (c *Client) Name() string { return "openai" }

// openAIMessages converts provider.Message slice into OpenAI API message format.
// Handles assistant messages with tool_calls and tool result messages.
func openAIMessages(messages []provider.Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			out = append(out, map[string]any{"role": "system", "content": msg.Content})
		case "user":
			out = append(out, map[string]any{"role": "user", "content": msg.Content})
		case "assistant":
			if len(msg.ToolCalls) == 0 {
				out = append(out, map[string]any{"role": "assistant", "content": msg.Content})
				continue
			}
			// OpenAI requires tool_calls in its own format.
			toolCalls := make([]map[string]any, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				args := string(tc.Input)
				if args == "" {
					args = "{}"
				}
				toolCalls = append(toolCalls, map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": args,
					},
				})
			}
			m := map[string]any{
				"role":       "assistant",
				"tool_calls": toolCalls,
			}
			if msg.Content != "" {
				m["content"] = msg.Content
			}
			out = append(out, m)
		case "tool":
			out = append(out, map[string]any{
				"role":         "tool",
				"tool_call_id": msg.ToolCallID,
				"content":      msg.Content,
			})
		}
	}
	return out
}

func streamOpenAI(body io.ReadCloser, events chan<- provider.CompletionEvent) {
	defer body.Close()
	defer close(events)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)

	// Accumulate tool call arguments by index (multiple tool calls can stream in parallel).
	type pendingCall struct {
		id   string
		name string
		args strings.Builder
	}
	pending := map[int]*pendingCall{}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			events <- provider.CompletionEvent{Type: provider.EventDone}
			return
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}

		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			events <- provider.CompletionEvent{Type: provider.EventError, Error: err}
			return
		}

		for _, choice := range chunk.Choices {
			// Accumulate text delta.
			if choice.Delta.Content != "" {
				events <- provider.CompletionEvent{Type: provider.EventDelta, Delta: choice.Delta.Content}
			}

			// Accumulate tool call deltas.
			for _, tc := range choice.Delta.ToolCalls {
				p, ok := pending[tc.Index]
				if !ok {
					p = &pendingCall{}
					pending[tc.Index] = p
				}
				if tc.ID != "" {
					p.id = tc.ID
				}
				if tc.Function.Name != "" {
					p.name = tc.Function.Name
				}
				p.args.WriteString(tc.Function.Arguments)
			}

			// When finish_reason == "tool_calls", emit all accumulated tool calls.
			if choice.FinishReason == "tool_calls" {
				for _, p := range pending {
					argsRaw := json.RawMessage(p.args.String())
					if len(argsRaw) == 0 {
						argsRaw = json.RawMessage("{}")
					}
					events <- provider.CompletionEvent{
						Type: provider.EventToolCall,
						ToolCall: &provider.ToolCall{
							ID:    p.id,
							Name:  p.name,
							Input: argsRaw,
						},
					}
				}
				pending = map[int]*pendingCall{}
			}
		}

		if chunk.Usage != nil {
			events <- provider.CompletionEvent{
				Type: provider.EventDone,
				Usage: &provider.Usage{
					InputTokens:  chunk.Usage.PromptTokens,
					OutputTokens: chunk.Usage.CompletionTokens,
				},
			}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		events <- provider.CompletionEvent{Type: provider.EventError, Error: err}
		return
	}
	events <- provider.CompletionEvent{Type: provider.EventDone}
}

func openAITools(tools []provider.ToolDefinition) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  json.RawMessage(t.InputSchema),
			},
		})
	}
	return out
}
