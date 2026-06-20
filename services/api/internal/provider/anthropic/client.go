package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
		baseURL = "https://api.anthropic.com"
	}
	return &Client{apiKey: apiKey, baseURL: baseURL}
}

func (c *Client) Complete(ctx context.Context, req provider.CompletionRequest) (<-chan provider.CompletionEvent, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("anthropic: api key is required")
	}
	system, messages := anthropicMessages(req.Messages)
	body := map[string]any{
		"model":       req.Model,
		"messages":    messages,
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
		"stream":      true,
	}
	if req.MaxTokens <= 0 {
		body["max_tokens"] = 4096
	}

	// Build system field: structured array with cache_control when a stable prefix is provided.
	if req.StableSystemContent != "" && system != "" {
		dynamicPart := strings.TrimPrefix(system, req.StableSystemContent)
		sysBlocks := []map[string]any{
			{
				"type":          "text",
				"text":          req.StableSystemContent,
				"cache_control": map[string]string{"type": "ephemeral"},
			},
		}
		if strings.TrimSpace(dynamicPart) != "" {
			sysBlocks = append(sysBlocks, map[string]any{
				"type": "text",
				"text": dynamicPart,
			})
		}
		body["system"] = sysBlocks
	} else if system != "" {
		body["system"] = system
	}

	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			var schema any
			_ = json.Unmarshal(t.InputSchema, &schema)
			tools = append(tools, map[string]any{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": schema,
			})
		}
		// Cache the tools block — schemas are stable within a conversation.
		tools[len(tools)-1]["cache_control"] = map[string]string{"type": "ephemeral"}
		body["tools"] = tools
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.apiKey)
	httpReq.Header.Set("Anthropic-Version", "2023-06-01")
	httpReq.Header.Set("Anthropic-Beta", "prompt-caching-2024-07-16")
	res, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		defer res.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("anthropic: %s: %s", res.Status, strings.TrimSpace(string(b)))
	}
	events := make(chan provider.CompletionEvent)
	go streamAnthropic(res.Body, events)
	return events, nil
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, fmt.Errorf("anthropic: Embed not implemented")
}

func (c *Client) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	if c.apiKey == "" {
		return anthropicFallback(), nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.baseURL, "/")+"/v1/models", nil)
	if err != nil {
		return anthropicFallback(), nil
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Anthropic-Version", "2023-06-01")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return anthropicFallback(), nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return anthropicFallback(), nil
	}
	var body struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return anthropicFallback(), nil
	}
	models := make([]provider.ModelInfo, 0, len(body.Data))
	for _, m := range body.Data {
		name := m.DisplayName
		if name == "" {
			name = m.ID
		}
		models = append(models, provider.ModelInfo{
			ID:             m.ID,
			Name:           name,
			ContextWindow:  200000,
			SupportsTools:  true,
			SupportsVision: true,
		})
	}
	if len(models) == 0 {
		return anthropicFallback(), nil
	}
	return models, nil
}

func anthropicFallback() []provider.ModelInfo {
	return []provider.ModelInfo{
		{ID: "claude-opus-4-8", Name: "Claude Opus 4.8", ContextWindow: 200000, SupportsTools: true, SupportsVision: true},
		{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", ContextWindow: 200000, SupportsTools: true, SupportsVision: true},
		{ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", ContextWindow: 200000, SupportsTools: true, SupportsVision: true},
	}
}

func (c *Client) Name() string { return "anthropic" }

// anthropicMessages converts provider.Message slice into Anthropic API format.
// Returns (system prompt, messages array).
// Handles assistant messages with tool_calls and tool result messages using
// Anthropic's content-array format.
func anthropicMessages(messages []provider.Message) (string, []map[string]any) {
	var systemParts []string
	out := make([]map[string]any, 0, len(messages))

	i := 0
	for i < len(messages) {
		msg := messages[i]
		switch msg.Role {
		case "system":
			if msg.Content != "" {
				systemParts = append(systemParts, msg.Content)
			}
			i++

		case "user":
			out = append(out, map[string]any{"role": "user", "content": msg.Content})
			i++

		case "assistant":
			if len(msg.ToolCalls) == 0 {
				out = append(out, map[string]any{"role": "assistant", "content": msg.Content})
				i++
				continue
			}
			// Build a content array: optional text block + one tool_use per call.
			content := make([]map[string]any, 0)
			if msg.Content != "" {
				content = append(content, map[string]any{"type": "text", "text": msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				var input any
				if err := json.Unmarshal(tc.Input, &input); err != nil {
					input = map[string]any{}
				}
				content = append(content, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": input,
				})
			}
			out = append(out, map[string]any{"role": "assistant", "content": content})
			i++

			// Collect the immediately following tool result messages and group
			// them as a single user message with tool_result content blocks.
			var toolResults []map[string]any
			for i < len(messages) && messages[i].Role == "tool" {
				tm := messages[i]
				tr := map[string]any{
					"type":        "tool_result",
					"tool_use_id": tm.ToolCallID,
					"content":     tm.Content,
				}
				if tm.IsError {
					tr["is_error"] = true
				}
				toolResults = append(toolResults, tr)
				i++
			}
			if len(toolResults) > 0 {
				out = append(out, map[string]any{"role": "user", "content": toolResults})
			}

		case "tool":
			// Orphaned tool result (no preceding assistant turn with tool_calls).
			tr := map[string]any{
				"type":        "tool_result",
				"tool_use_id": msg.ToolCallID,
				"content":     msg.Content,
			}
			if msg.IsError {
				tr["is_error"] = true
			}
			out = append(out, map[string]any{
				"role":    "user",
				"content": []map[string]any{tr},
			})
			i++

		default:
			i++
		}
	}
	return strings.Join(systemParts, "\n\n"), out
}

func streamAnthropic(body io.ReadCloser, events chan<- provider.CompletionEvent) {
	defer body.Close()
	defer close(events)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)

	usage := provider.Usage{}

	// State for accumulating tool use blocks across multiple delta events.
	var (
		currentToolID   string
		currentToolName string
		toolInputBuf    strings.Builder
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var event struct {
			Type  string `json:"type"`
			Index int    `json:"index"`
			// content_block_start
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			// content_block_delta
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
			// message_start
			Message struct {
				Usage struct {
					InputTokens int `json:"input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			// message_delta
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			// error
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.Unmarshal([]byte(data), &event); err != nil {
			events <- provider.CompletionEvent{Type: provider.EventError, Error: err}
			return
		}

		switch event.Type {
		case "message_start":
			usage.InputTokens = event.Message.Usage.InputTokens

		case "content_block_start":
			if event.ContentBlock.Type == "tool_use" {
				currentToolID = event.ContentBlock.ID
				currentToolName = event.ContentBlock.Name
				toolInputBuf.Reset()
			}

		case "content_block_delta":
			switch event.Delta.Type {
			case "text_delta":
				if event.Delta.Text != "" {
					events <- provider.CompletionEvent{Type: provider.EventDelta, Delta: event.Delta.Text}
				}
			case "input_json_delta":
				toolInputBuf.WriteString(event.Delta.PartialJSON)
			}

		case "content_block_stop":
			if currentToolID != "" {
				inputRaw := json.RawMessage(toolInputBuf.String())
				if len(inputRaw) == 0 {
					inputRaw = json.RawMessage("{}")
				}
				events <- provider.CompletionEvent{
					Type: provider.EventToolCall,
					ToolCall: &provider.ToolCall{
						ID:    currentToolID,
						Name:  currentToolName,
						Input: inputRaw,
					},
				}
				currentToolID = ""
				currentToolName = ""
				toolInputBuf.Reset()
			}

		case "message_delta":
			if event.Usage.OutputTokens > 0 {
				usage.OutputTokens = event.Usage.OutputTokens
			}

		case "message_stop":
			events <- provider.CompletionEvent{Type: provider.EventDone, Usage: &usage}
			return

		case "error":
			events <- provider.CompletionEvent{Type: provider.EventError, Error: errors.New(event.Error.Message)}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		events <- provider.CompletionEvent{Type: provider.EventError, Error: err}
		return
	}
	events <- provider.CompletionEvent{Type: provider.EventDone, Usage: &usage}
}
