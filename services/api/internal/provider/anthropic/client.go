// Package anthropic adapts the official anthropic-sdk-go to the
// provider.Provider interface. The SDK owns the wire format; this shim keeps
// the platform's normalisation: prompt-caching on the stable system prefix
// and tools block, grouped tool results, and streamed deltas.
package anthropic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
)

type Client struct {
	apiKey  string
	baseURL string
	sdk     sdk.Client
}

func New(apiKey, baseURL string) *Client {
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return &Client{apiKey: apiKey, baseURL: baseURL, sdk: sdk.NewClient(opts...)}
}

func (c *Client) Complete(ctx context.Context, req provider.CompletionRequest) (<-chan provider.CompletionEvent, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("anthropic: api key is required")
	}
	params := buildParams(req)
	events := make(chan provider.CompletionEvent, 32)
	go func() {
		defer close(events)
		stream := c.sdk.Messages.NewStreaming(ctx, params)
		acc := sdk.Message{}
		for stream.Next() {
			ev := stream.Current()
			if err := acc.Accumulate(ev); err != nil {
				events <- provider.CompletionEvent{Type: provider.EventError, Error: fmt.Errorf("anthropic: %w", err)}
				return
			}
			// Emit text deltas live for token-by-token streaming.
			if ev.Type == "content_block_delta" && ev.Delta.Type == "text_delta" {
				events <- provider.CompletionEvent{Type: provider.EventDelta, Delta: ev.Delta.Text}
			}
		}
		if err := stream.Err(); err != nil {
			events <- provider.CompletionEvent{Type: provider.EventError, Error: fmt.Errorf("anthropic: %w", err)}
			return
		}
		// Tool calls come from the accumulated message once complete.
		for _, block := range acc.Content {
			if tu := block.AsToolUse(); tu.Type == "tool_use" {
				input := json.RawMessage(tu.Input)
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				events <- provider.CompletionEvent{
					Type:     provider.EventToolCall,
					ToolCall: &provider.ToolCall{ID: tu.ID, Name: tu.Name, Input: input},
				}
			}
		}
		events <- provider.CompletionEvent{Type: provider.EventDone, Usage: &provider.Usage{
			InputTokens:  int(acc.Usage.InputTokens + acc.Usage.CacheCreationInputTokens + acc.Usage.CacheReadInputTokens),
			OutputTokens: int(acc.Usage.OutputTokens),
		}}
	}()
	return events, nil
}

// buildParams converts the normalised request, preserving the previous
// hand-rolled behaviours: cache_control on the stable system prefix and the
// last tool, tool results grouped into single user turns.
func buildParams(req provider.CompletionRequest) sdk.MessageNewParams {
	maxTokens := int64(req.MaxTokens)
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	params := sdk.MessageNewParams{
		Model:       sdk.Model(req.Model),
		MaxTokens:   maxTokens,
		Temperature: sdk.Float(req.Temperature),
	}

	var systemParts []string
	msgs := req.Messages
	i := 0
	for i < len(msgs) {
		msg := msgs[i]
		switch msg.Role {
		case "system":
			if msg.Content != "" {
				systemParts = append(systemParts, msg.Content)
			}
			i++
		case "user":
			if len(msg.Images) == 0 {
				params.Messages = append(params.Messages, sdk.NewUserMessage(sdk.NewTextBlock(msg.Content)))
			} else {
				blocks := []sdk.ContentBlockParamUnion{sdk.NewTextBlock(msg.Content)}
				for _, img := range msg.Images {
					blocks = append(blocks, sdk.NewImageBlockBase64(img.MIMEType, base64.StdEncoding.EncodeToString(img.Data)))
				}
				params.Messages = append(params.Messages, sdk.NewUserMessage(blocks...))
			}
			i++
		case "assistant":
			if len(msg.ToolCalls) == 0 {
				params.Messages = append(params.Messages, sdk.NewAssistantMessage(sdk.NewTextBlock(msg.Content)))
				i++
				continue
			}
			blocks := []sdk.ContentBlockParamUnion{}
			if msg.Content != "" {
				blocks = append(blocks, sdk.NewTextBlock(msg.Content))
			}
			for _, tc := range msg.ToolCalls {
				var input any
				if err := json.Unmarshal(tc.Input, &input); err != nil {
					input = map[string]any{}
				}
				blocks = append(blocks, sdk.NewToolUseBlock(tc.ID, input, tc.Name))
			}
			params.Messages = append(params.Messages, sdk.NewAssistantMessage(blocks...))
			i++
			// Group immediately following tool results into one user turn.
			var results []sdk.ContentBlockParamUnion
			for i < len(msgs) && msgs[i].Role == "tool" {
				tm := msgs[i]
				results = append(results, sdk.NewToolResultBlock(tm.ToolCallID, tm.Content, tm.IsError))
				i++
			}
			if len(results) > 0 {
				params.Messages = append(params.Messages, sdk.NewUserMessage(results...))
			}
		case "tool":
			params.Messages = append(params.Messages, sdk.NewUserMessage(
				sdk.NewToolResultBlock(msg.ToolCallID, msg.Content, msg.IsError)))
			i++
		default:
			i++
		}
	}

	system := strings.Join(systemParts, "\n\n")
	if req.StableSystemContent != "" && system != "" {
		// Cache breakpoint after the stable prefix; dynamic remainder uncached.
		blocks := []sdk.TextBlockParam{{
			Text:         req.StableSystemContent,
			CacheControl: sdk.NewCacheControlEphemeralParam(),
		}}
		if dynamic := strings.TrimPrefix(system, req.StableSystemContent); strings.TrimSpace(dynamic) != "" {
			blocks = append(blocks, sdk.TextBlockParam{Text: dynamic})
		}
		params.System = blocks
	} else if system != "" {
		params.System = []sdk.TextBlockParam{{Text: system}}
	}

	if len(req.Tools) > 0 {
		tools := make([]sdk.ToolUnionParam, 0, len(req.Tools))
		for idx, t := range req.Tools {
			var schema map[string]any
			_ = json.Unmarshal(t.InputSchema, &schema)
			in := sdk.ToolInputSchemaParam{}
			if props, ok := schema["properties"]; ok {
				in.Properties = props
			}
			if reqd, ok := schema["required"].([]any); ok {
				for _, r := range reqd {
					if s, ok := r.(string); ok {
						in.Required = append(in.Required, s)
					}
				}
			}
			tp := sdk.ToolParam{
				Name:        t.Name,
				Description: sdk.String(t.Description),
				InputSchema: in,
			}
			// Cache the tools block — schemas are stable within a conversation.
			if idx == len(req.Tools)-1 {
				tp.CacheControl = sdk.NewCacheControlEphemeralParam()
			}
			tools = append(tools, sdk.ToolUnionParam{OfTool: &tp})
		}
		params.Tools = tools
	}
	return params
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, fmt.Errorf("anthropic: Embed not supported")
}

func (c *Client) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	if c.apiKey == "" {
		return anthropicFallback(), nil
	}
	page, err := c.sdk.Models.List(ctx, sdk.ModelListParams{})
	if err != nil {
		return anthropicFallback(), nil
	}
	models := make([]provider.ModelInfo, 0)
	for _, m := range page.Data {
		models = append(models, provider.ModelInfo{
			ID:             string(m.ID),
			Name:           m.DisplayName,
			ContextWindow:  200_000,
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
		{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", ContextWindow: 200_000, SupportsTools: true, SupportsVision: true},
		{ID: "claude-opus-4-8", Name: "Claude Opus 4.8", ContextWindow: 200_000, SupportsTools: true, SupportsVision: true},
		{ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", ContextWindow: 200_000, SupportsTools: true, SupportsVision: true},
	}
}

func (c *Client) Name() string { return "anthropic" }
