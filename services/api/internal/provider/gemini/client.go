package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

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

// callSeq generates monotonically increasing IDs for tool calls (Gemini doesn't provide them).
var callSeq atomic.Int64

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
				FinishReason string `json:"finishReason"`
				Content      struct {
					Parts []struct {
						Text         string `json:"text"`
						Thought      bool   `json:"thought"`
						FunctionCall *struct {
							Name string          `json:"name"`
							Args json.RawMessage `json:"args"`
						} `json:"functionCall"`
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
			cand := chunk.Candidates[0]
			if cand.FinishReason != "" && cand.FinishReason != "STOP" && cand.FinishReason != "MAX_TOKENS" {
				slog.Warn("gemini non-normal finish", "reason", cand.FinishReason)
			}
			switch cand.FinishReason {
			case "SAFETY", "RECITATION", "PROHIBITED_CONTENT", "SPII":
				events <- provider.CompletionEvent{Type: provider.EventError, Error: fmt.Errorf("gemini: response blocked (%s)", cand.FinishReason)}
				return
			}
			slog.Debug("gemini chunk", "finish_reason", cand.FinishReason, "parts", len(cand.Content.Parts))
			for _, part := range cand.Content.Parts {
				// Skip internal thinking/reasoning parts — only emit actual response content.
				if part.Thought {
					continue
				}
				if part.Text != "" {
					events <- provider.CompletionEvent{Type: provider.EventDelta, Delta: part.Text}
				}
				if part.FunctionCall != nil {
					args := part.FunctionCall.Args
					if len(args) == 0 {
						args = json.RawMessage("{}")
					}
					seq := callSeq.Add(1)
					events <- provider.CompletionEvent{
						Type: provider.EventToolCall,
						ToolCall: &provider.ToolCall{
							ID:    fmt.Sprintf("gemini-%s-%d", part.FunctionCall.Name, seq),
							Name:  part.FunctionCall.Name,
							Input: args,
						},
					}
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
	if err := scanner.Err(); err != nil {
		slog.Error("gemini stream scanner error", "err", err)
		events <- provider.CompletionEvent{Type: provider.EventError, Error: fmt.Errorf("gemini: stream error: %w", err)}
		return
	}
	slog.Debug("gemini stream done", "input_tokens", usage.InputTokens, "output_tokens", usage.OutputTokens)
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

// geminiRequest converts a CompletionRequest into Gemini API body format.
// Handles system instructions, tool definitions, and the full conversation
// history including assistant turns with function calls and function results.
func geminiRequest(req provider.CompletionRequest) map[string]any {
	contents := []map[string]any{}
	system := ""

	msgs := req.Messages
	i := 0
	for i < len(msgs) {
		msg := msgs[i]
		switch msg.Role {
		case "system":
			system = strings.TrimSpace(system + "\n\n" + msg.Content)
			i++

		case "user":
			contents = append(contents, geminiContent("user", msg.Content))
			i++

		case "assistant":
			if len(msg.ToolCalls) == 0 {
				contents = append(contents, geminiContent("model", msg.Content))
				i++
				continue
			}
			// Build a model turn with optional text part + one functionCall part per tool call.
			parts := []map[string]any{}
			if msg.Content != "" {
				parts = append(parts, map[string]any{"text": msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				var args any
				if err := json.Unmarshal(tc.Input, &args); err != nil {
					args = map[string]any{}
				}
				parts = append(parts, map[string]any{
					"functionCall": map[string]any{
						"name": tc.Name,
						"args": args,
					},
				})
			}
			contents = append(contents, map[string]any{"role": "model", "parts": parts})
			i++

			// Collect all immediately following tool result messages and group them
			// into a single user turn with one functionResponse part each.
			var frParts []map[string]any
			for i < len(msgs) && msgs[i].Role == "tool" {
				tm := msgs[i]
				var result any
				if err := json.Unmarshal([]byte(tm.Content), &result); err != nil {
					result = tm.Content
				}
				var response map[string]any
				if tm.IsError {
					response = map[string]any{"error": result, "note": "This function call failed. Retry with corrected arguments."}
				} else {
					response = map[string]any{"output": result}
				}
				frParts = append(frParts, map[string]any{
					"functionResponse": map[string]any{
						"name":     tm.ToolName,
						"response": response,
					},
				})
				i++
			}
			if len(frParts) > 0 {
				contents = append(contents, map[string]any{"role": "user", "parts": frParts})
			}

		case "tool":
			// Orphaned tool result (no preceding assistant turn with ToolCalls).
			var result any
			if err := json.Unmarshal([]byte(msg.Content), &result); err != nil {
				result = msg.Content
			}
			var response map[string]any
			if msg.IsError {
				response = map[string]any{"error": result, "note": "This function call failed. Retry with corrected arguments."}
			} else {
				response = map[string]any{"output": result}
			}
			contents = append(contents, map[string]any{
				"role": "user",
				"parts": []map[string]any{{
					"functionResponse": map[string]any{
						"name":     msg.ToolName,
						"response": response,
					},
				}},
			})
			i++

		default:
			i++
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
	// Cap thinking budget for structured/deterministic tasks (temperature ≤ 0.15) so
	// thinking tokens don't consume the entire output token budget.
	// Gemini 2.5 Flash minimum thinkingBudget is 1024; 0 is rejected.
	if req.Temperature <= 0.15 {
		body["generationConfig"].(map[string]any)["thinkingConfig"] = map[string]any{"thinkingBudget": 1024}
	}
	if system != "" {
		body["systemInstruction"] = map[string]any{"parts": []map[string]string{{"text": system}}}
	}
	if len(req.Tools) > 0 {
		seen := make(map[string]struct{}, len(req.Tools))
		functionDecls := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			if t.Name == "" {
				continue
			}
			if _, ok := seen[t.Name]; ok {
				continue
			}
			seen[t.Name] = struct{}{}
			var schema any
			_ = json.Unmarshal(t.InputSchema, &schema)
			functionDecls = append(functionDecls, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  schema,
			})
		}
		body["tools"] = []map[string]any{{"function_declarations": functionDecls}}
	}
	return body
}

func geminiContent(role, text string) map[string]any {
	return map[string]any{
		"role":  role,
		"parts": []map[string]string{{"text": text}},
	}
}
