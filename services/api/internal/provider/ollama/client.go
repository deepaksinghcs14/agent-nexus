package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/agentNexus/agent-nexus/services/api/internal/provider"
)

type Client struct {
	baseURL string
}

func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &Client{baseURL: baseURL}
}

func (c *Client) Complete(ctx context.Context, req provider.CompletionRequest) (<-chan provider.CompletionEvent, error) {
	body := map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   true,
		"options": map[string]any{
			"temperature": req.Temperature,
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		defer res.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("ollama: %s: %s", res.Status, strings.TrimSpace(string(b)))
	}
	events := make(chan provider.CompletionEvent)
	go streamOllama(res.Body, events)
	return events, nil
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, fmt.Errorf("ollama: Embed not implemented")
}

func (c *Client) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.baseURL, "/")+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return nil, fmt.Errorf("ollama: %s: %s", res.Status, strings.TrimSpace(string(b)))
	}
	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	models := make([]provider.ModelInfo, 0, len(body.Models))
	for _, m := range body.Models {
		models = append(models, provider.ModelInfo{
			ID:             m.Name,
			Name:           m.Name,
			ContextWindow:  128000,
			SupportsTools:  false,
			SupportsVision: false,
		})
	}
	return models, nil
}

func (c *Client) Name() string { return "ollama" }

func streamOllama(body io.ReadCloser, events chan<- provider.CompletionEvent) {
	defer body.Close()
	defer close(events)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	for scanner.Scan() {
		var chunk struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Done      bool `json:"done"`
			Prompt    int  `json:"prompt_eval_count"`
			Generated int  `json:"eval_count"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			events <- provider.CompletionEvent{Type: provider.EventError, Error: err}
			return
		}
		if chunk.Message.Content != "" {
			events <- provider.CompletionEvent{Type: provider.EventDelta, Delta: chunk.Message.Content}
		}
		if chunk.Done {
			events <- provider.CompletionEvent{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: chunk.Prompt, OutputTokens: chunk.Generated}}
			return
		}
	}
	if err := scanner.Err(); err != nil {
		events <- provider.CompletionEvent{Type: provider.EventError, Error: err}
		return
	}
	events <- provider.CompletionEvent{Type: provider.EventDone}
}
