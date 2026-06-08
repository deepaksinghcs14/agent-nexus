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

	"github.com/agentNexus/agent-nexus/services/api/internal/provider"
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
	if system != "" {
		body["system"] = system
	}
	if req.MaxTokens <= 0 {
		body["max_tokens"] = 4096
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

func anthropicMessages(messages []provider.Message) (string, []map[string]string) {
	systemParts := []string{}
	out := []map[string]string{}
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			if msg.Content != "" {
				systemParts = append(systemParts, msg.Content)
			}
		case "user", "assistant":
			out = append(out, map[string]string{"role": msg.Role, "content": msg.Content})
		case "tool":
			out = append(out, map[string]string{"role": "user", "content": msg.Content})
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
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Message struct {
				Usage struct {
					InputTokens int `json:"input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
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
		case "content_block_delta":
			if event.Delta.Text != "" {
				events <- provider.CompletionEvent{Type: provider.EventDelta, Delta: event.Delta.Text}
			}
		case "message_delta":
			if event.Usage.OutputTokens > 0 {
				usage.OutputTokens = event.Usage.OutputTokens
			}
		case "error":
			events <- provider.CompletionEvent{Type: provider.EventError, Error: errors.New(event.Error.Message)}
			return
		case "message_stop":
			events <- provider.CompletionEvent{Type: provider.EventDone, Usage: &usage}
			return
		}
	}
	if err := scanner.Err(); err != nil {
		events <- provider.CompletionEvent{Type: provider.EventError, Error: err}
		return
	}
	events <- provider.CompletionEvent{Type: provider.EventDone, Usage: &usage}
}
