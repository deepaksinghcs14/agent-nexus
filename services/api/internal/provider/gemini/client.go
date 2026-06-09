package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
)

type Client struct {
	apiKey   string
	authType string // "api_key" | "oauth"
}

func New(apiKey, authType string) *Client {
	if authType == "" {
		authType = "api_key"
	}
	return &Client{apiKey: apiKey, authType: authType}
}

func (c *Client) Complete(ctx context.Context, req provider.CompletionRequest) (<-chan provider.CompletionEvent, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("gemini: api key or oauth token is required")
	}
	body := geminiRequest(req)
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	// streamGenerateContent?alt=sse gives true token-by-token streaming
	base := "https://generativelanguage.googleapis.com/v1beta/models/" + url.PathEscape(req.Model) + ":streamGenerateContent?alt=sse"
	var endpoint string
	if c.authType == "oauth" {
		endpoint = base
	} else {
		endpoint = base + "&key=" + url.QueryEscape(c.apiKey)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.authType == "oauth" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	res, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		return nil, fmt.Errorf("gemini: %s: %s", res.Status, strings.TrimSpace(string(raw)))
	}

	events := make(chan provider.CompletionEvent, 32)
	go func() {
		defer close(events)
		defer res.Body.Close()
		streamGemini(ctx, res.Body, events)
	}()
	return events, nil
}

func streamGemini(ctx context.Context, body io.Reader, events chan<- provider.CompletionEvent) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 512*1024), 512*1024)
	var usage provider.Usage

	for scanner.Scan() {
		if ctx.Err() != nil {
			events <- provider.CompletionEvent{Type: provider.EventError, Error: ctx.Err()}
			return
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}

		var chunk struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
			UsageMetadata struct {
				PromptTokenCount     int `json:"promptTokenCount"`
				CandidatesTokenCount int `json:"candidatesTokenCount"`
			} `json:"usageMetadata"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			events <- provider.CompletionEvent{Type: provider.EventError, Error: fmt.Errorf("gemini: %s", chunk.Error.Message)}
			return
		}
		if len(chunk.Candidates) > 0 {
			for _, part := range chunk.Candidates[0].Content.Parts {
				if part.Text != "" {
					events <- provider.CompletionEvent{Type: provider.EventDelta, Delta: part.Text}
				}
			}
		}
		if chunk.UsageMetadata.PromptTokenCount > 0 {
			usage = provider.Usage{
				InputTokens:  chunk.UsageMetadata.PromptTokenCount,
				OutputTokens: chunk.UsageMetadata.CandidatesTokenCount,
			}
		}
	}
	events <- provider.CompletionEvent{Type: provider.EventDone, Usage: &usage}
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, fmt.Errorf("gemini: Embed not implemented")
}

func (c *Client) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	if c.apiKey == "" {
		return geminiFallback(), nil
	}
	var endpoint string
	if c.authType == "oauth" {
		endpoint = "https://generativelanguage.googleapis.com/v1beta/models"
	} else {
		endpoint = "https://generativelanguage.googleapis.com/v1beta/models?key=" + url.QueryEscape(c.apiKey)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return geminiFallback(), nil
	}
	if c.authType == "oauth" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return geminiFallback(), nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return geminiFallback(), nil
	}
	var body struct {
		Models []struct {
			Name                       string   `json:"name"`
			DisplayName                string   `json:"displayName"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return geminiFallback(), nil
	}
	models := make([]provider.ModelInfo, 0)
	for _, m := range body.Models {
		supportsGenerate := false
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				supportsGenerate = true
				break
			}
		}
		if !supportsGenerate {
			continue
		}
		id := strings.TrimPrefix(m.Name, "models/")
		name := m.DisplayName
		if name == "" {
			name = id
		}
		models = append(models, provider.ModelInfo{
			ID:             id,
			Name:           name,
			ContextWindow:  1000000,
			SupportsTools:  true,
			SupportsVision: true,
		})
	}
	if len(models) == 0 {
		return geminiFallback(), nil
	}
	return models, nil
}

func geminiFallback() []provider.ModelInfo {
	return []provider.ModelInfo{
		{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", ContextWindow: 1000000, SupportsTools: true, SupportsVision: true},
		{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", ContextWindow: 1000000, SupportsTools: true, SupportsVision: true},
	}
}

func (c *Client) Name() string { return "gemini" }

func geminiRequest(req provider.CompletionRequest) map[string]any {
	contents := []map[string]any{}
	system := ""
	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			system = strings.TrimSpace(system + "\n\n" + msg.Content)
		case "assistant":
			contents = append(contents, geminiContent("model", msg.Content))
		case "user", "tool":
			contents = append(contents, geminiContent("user", msg.Content))
		}
	}
	body := map[string]any{
		"contents": contents,
		"generationConfig": map[string]any{
			"temperature": req.Temperature,
		},
	}
	if req.MaxTokens > 0 {
		body["generationConfig"].(map[string]any)["maxOutputTokens"] = req.MaxTokens
	}
	if system != "" {
		body["systemInstruction"] = map[string]any{"parts": []map[string]string{{"text": system}}}
	}
	return body
}

func geminiContent(role, text string) map[string]any {
	return map[string]any{
		"role":  role,
		"parts": []map[string]string{{"text": text}},
	}
}
