// Package gemini adapts Google's official genai SDK to the provider.Provider
// interface. The SDK owns the wire format — streaming framing, tool-call
// encoding, thought signatures, schema handling — so provider API churn lands
// in a vendored dependency bump instead of a hand-rolled HTTP client.
package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"google.golang.org/genai"
)

type Client struct {
	apiKey   string
	authType string // "api_key" | "oauth"

	once   sync.Once
	sdk    *genai.Client
	sdkErr error
}

func New(apiKey, authType string) *Client {
	if authType == "" {
		authType = "api_key"
	}
	return &Client{apiKey: apiKey, authType: authType}
}

// oauthTransport swaps the SDK's x-goog-api-key header for an OAuth bearer
// token. The SDK hard-requires an API key for the Gemini backend, so OAuth
// credentials ride a placeholder key that never reaches the wire.
type oauthTransport struct{ token string }

func (t oauthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header.Del("x-goog-api-key")
	r.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(r)
}

// client lazily constructs the SDK client (its constructor wants a context).
func (c *Client) client(ctx context.Context) (*genai.Client, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("gemini: api key or oauth token is required")
	}
	c.once.Do(func() {
		cfg := &genai.ClientConfig{Backend: genai.BackendGeminiAPI, APIKey: c.apiKey}
		if c.authType == "oauth" {
			cfg.APIKey = "oauth-placeholder" // satisfies SDK validation; stripped by the transport
			cfg.HTTPClient = &http.Client{Transport: oauthTransport{token: c.apiKey}}
		}
		c.sdk, c.sdkErr = genai.NewClient(ctx, cfg)
	})
	return c.sdk, c.sdkErr
}

// callSeq generates IDs for tool calls when Gemini doesn't provide one.
var callSeq atomic.Int64

// syntheticIDPrefix marks tool-call IDs we minted (vs IDs Gemini populated,
// which must be echoed back on the matching functionResponse).
const syntheticIDPrefix = "gemini-"

func (c *Client) Complete(ctx context.Context, req provider.CompletionRequest) (<-chan provider.CompletionEvent, error) {
	sdk, err := c.client(ctx)
	if err != nil {
		return nil, err
	}
	contents, config := buildRequest(req)

	events := make(chan provider.CompletionEvent, 32)
	go func() {
		defer close(events)
		var usage provider.Usage
		for resp, err := range sdk.Models.GenerateContentStream(ctx, req.Model, contents, config) {
			if err != nil {
				events <- provider.CompletionEvent{Type: provider.EventError, Error: fmt.Errorf("gemini: %w", err)}
				return
			}
			if resp.UsageMetadata != nil && resp.UsageMetadata.PromptTokenCount > 0 {
				usage = provider.Usage{
					InputTokens:  int(resp.UsageMetadata.PromptTokenCount),
					OutputTokens: int(resp.UsageMetadata.CandidatesTokenCount),
				}
			}
			if len(resp.Candidates) == 0 {
				continue
			}
			cand := resp.Candidates[0]
			switch cand.FinishReason {
			case genai.FinishReasonSafety, genai.FinishReasonRecitation, genai.FinishReasonProhibitedContent, genai.FinishReasonSPII:
				events <- provider.CompletionEvent{Type: provider.EventError, Error: fmt.Errorf("gemini: response blocked (%s)", cand.FinishReason)}
				return
			}
			if cand.Content == nil {
				continue
			}
			for _, part := range cand.Content.Parts {
				// Skip internal reasoning parts — only emit response content.
				if part.Thought {
					continue
				}
				if part.Text != "" {
					events <- provider.CompletionEvent{Type: provider.EventDelta, Delta: part.Text}
				}
				if part.FunctionCall != nil {
					args, _ := json.Marshal(part.FunctionCall.Args)
					if len(args) == 0 || string(args) == "null" {
						args = json.RawMessage("{}")
					}
					id := part.FunctionCall.ID
					if id == "" {
						id = fmt.Sprintf("%s%s-%d", syntheticIDPrefix, part.FunctionCall.Name, callSeq.Add(1))
					}
					events <- provider.CompletionEvent{
						Type: provider.EventToolCall,
						ToolCall: &provider.ToolCall{
							ID:               id,
							Name:             part.FunctionCall.Name,
							Input:            args,
							ThoughtSignature: string(part.ThoughtSignature),
						},
					}
				}
			}
		}
		events <- provider.CompletionEvent{Type: provider.EventDone, Usage: &usage}
	}()
	return events, nil
}

// buildRequest converts a normalised CompletionRequest into SDK contents +
// config, preserving the semantics of the previous hand-rolled builder:
// system messages merge into SystemInstruction, assistant tool calls become
// model functionCall parts (with thought signatures echoed), and consecutive
// tool results group into a single user turn of functionResponse parts.
func buildRequest(req provider.CompletionRequest) ([]*genai.Content, *genai.GenerateContentConfig) {
	var contents []*genai.Content
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
			contents = append(contents, genai.NewContentFromText(msg.Content, genai.RoleUser))
			i++

		case "assistant":
			if len(msg.ToolCalls) == 0 {
				contents = append(contents, genai.NewContentFromText(msg.Content, genai.RoleModel))
				i++
				continue
			}
			parts := []*genai.Part{}
			if msg.Content != "" {
				parts = append(parts, &genai.Part{Text: msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				var args map[string]any
				if err := json.Unmarshal(tc.Input, &args); err != nil || args == nil {
					args = map[string]any{}
				}
				fc := &genai.FunctionCall{Name: tc.Name, Args: args}
				if !strings.HasPrefix(tc.ID, syntheticIDPrefix) {
					fc.ID = tc.ID
				}
				parts = append(parts, &genai.Part{
					FunctionCall:     fc,
					ThoughtSignature: []byte(tc.ThoughtSignature),
				})
			}
			contents = append(contents, &genai.Content{Role: genai.RoleModel, Parts: parts})
			i++

			// Group all immediately following tool results into one user turn.
			var frParts []*genai.Part
			for i < len(msgs) && msgs[i].Role == "tool" {
				frParts = append(frParts, functionResponsePart(msgs[i]))
				i++
			}
			if len(frParts) > 0 {
				contents = append(contents, &genai.Content{Role: genai.RoleUser, Parts: frParts})
			}

		case "tool":
			// Orphaned tool result (no preceding assistant turn with ToolCalls).
			contents = append(contents, &genai.Content{Role: genai.RoleUser, Parts: []*genai.Part{functionResponsePart(msg)}})
			i++

		default:
			i++
		}
	}

	config := &genai.GenerateContentConfig{
		Temperature: genai.Ptr(float32(req.Temperature)),
	}
	if req.MaxTokens > 0 {
		config.MaxOutputTokens = int32(req.MaxTokens)
	}
	// Cap thinking budget for structured/deterministic tasks (temperature
	// ≤ 0.15) so thinking tokens don't consume the output budget. Gemini 2.5
	// Flash's minimum thinkingBudget is 1024; 0 is rejected.
	if req.Temperature <= 0.15 {
		config.ThinkingConfig = &genai.ThinkingConfig{ThinkingBudget: genai.Ptr(int32(1024))}
	}
	if system != "" {
		config.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: system}}}
	}
	if len(req.Tools) > 0 {
		seen := make(map[string]struct{}, len(req.Tools))
		decls := make([]*genai.FunctionDeclaration, 0, len(req.Tools))
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
			// ParametersJsonSchema accepts standard JSON Schema directly —
			// no more sanitising to Gemini's restricted Schema subset.
			decls = append(decls, &genai.FunctionDeclaration{
				Name:                 t.Name,
				Description:          t.Description,
				ParametersJsonSchema: schema,
			})
		}
		config.Tools = []*genai.Tool{{FunctionDeclarations: decls}}
	}
	return contents, config
}

func functionResponsePart(msg provider.Message) *genai.Part {
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
	fr := &genai.FunctionResponse{Name: msg.ToolName, Response: response}
	if msg.ToolCallID != "" && !strings.HasPrefix(msg.ToolCallID, syntheticIDPrefix) {
		fr.ID = msg.ToolCallID
	}
	return &genai.Part{FunctionResponse: fr}
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, fmt.Errorf("gemini: Embed not implemented")
}

func (c *Client) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	sdk, err := c.client(ctx)
	if err != nil {
		return geminiFallback(), nil
	}
	models := make([]provider.ModelInfo, 0)
	for m, err := range sdk.Models.All(ctx) {
		if err != nil {
			return geminiFallback(), nil
		}
		supportsGenerate := len(m.SupportedActions) == 0 // no data → don't filter
		for _, a := range m.SupportedActions {
			if a == "generateContent" {
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
