// Package openai adapts the official openai-go SDK to the provider.Provider
// interface. Supports OpenAI-compatible endpoints via the BaseURL override.
package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

type Client struct {
	apiKey  string
	baseURL string
	sdk     sdk.Client
}

func New(apiKey, baseURL string) *Client {
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL != "" {
		// Stored base URLs omit the /v1 suffix (the old hand-rolled client
		// appended full paths); the SDK expects the versioned base.
		opts = append(opts, option.WithBaseURL(strings.TrimRight(baseURL, "/")+"/v1"))
	}
	return &Client{apiKey: apiKey, baseURL: baseURL, sdk: sdk.NewClient(opts...)}
}

func (c *Client) Complete(ctx context.Context, req provider.CompletionRequest) (<-chan provider.CompletionEvent, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("openai: api key is required")
	}
	params := buildParams(req)
	events := make(chan provider.CompletionEvent, 32)
	go func() {
		defer close(events)
		stream := c.sdk.Chat.Completions.NewStreaming(ctx, params)
		acc := sdk.ChatCompletionAccumulator{}
		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				events <- provider.CompletionEvent{Type: provider.EventDelta, Delta: chunk.Choices[0].Delta.Content}
			}
		}
		if err := stream.Err(); err != nil {
			events <- provider.CompletionEvent{Type: provider.EventError, Error: fmt.Errorf("openai: %w", err)}
			return
		}
		if len(acc.Choices) > 0 {
			for _, tc := range acc.Choices[0].Message.ToolCalls {
				fn := tc.AsFunction()
				args := fn.Function.Arguments
				if strings.TrimSpace(args) == "" {
					args = "{}"
				}
				events <- provider.CompletionEvent{
					Type:     provider.EventToolCall,
					ToolCall: &provider.ToolCall{ID: fn.ID, Name: fn.Function.Name, Input: json.RawMessage(args)},
				}
			}
		}
		events <- provider.CompletionEvent{Type: provider.EventDone, Usage: &provider.Usage{
			InputTokens:  int(acc.Usage.PromptTokens),
			OutputTokens: int(acc.Usage.CompletionTokens),
		}}
	}()
	return events, nil
}

func buildParams(req provider.CompletionRequest) sdk.ChatCompletionNewParams {
	params := sdk.ChatCompletionNewParams{
		Model:       shared.ChatModel(req.Model),
		Temperature: sdk.Float(req.Temperature),
		StreamOptions: sdk.ChatCompletionStreamOptionsParam{
			IncludeUsage: sdk.Bool(true),
		},
	}
	if req.MaxTokens > 0 {
		params.MaxTokens = sdk.Int(int64(req.MaxTokens))
	}
	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			params.Messages = append(params.Messages, sdk.SystemMessage(msg.Content))
		case "user":
			if len(msg.Images) == 0 {
				params.Messages = append(params.Messages, sdk.UserMessage(msg.Content))
			} else {
				parts := []sdk.ChatCompletionContentPartUnionParam{sdk.TextContentPart(msg.Content)}
				for _, img := range msg.Images {
					dataURL := "data:" + img.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
					parts = append(parts, sdk.ImageContentPart(sdk.ChatCompletionContentPartImageImageURLParam{URL: dataURL}))
				}
				params.Messages = append(params.Messages, sdk.UserMessage(parts))
			}
		case "assistant":
			if len(msg.ToolCalls) == 0 {
				params.Messages = append(params.Messages, sdk.AssistantMessage(msg.Content))
				continue
			}
			asst := sdk.ChatCompletionAssistantMessageParam{}
			if msg.Content != "" {
				asst.Content.OfString = sdk.String(msg.Content)
			}
			for _, tc := range msg.ToolCalls {
				args := string(tc.Input)
				if args == "" {
					args = "{}"
				}
				asst.ToolCalls = append(asst.ToolCalls, sdk.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &sdk.ChatCompletionMessageFunctionToolCallParam{
						ID: tc.ID,
						Function: sdk.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: args,
						},
					},
				})
			}
			params.Messages = append(params.Messages, sdk.ChatCompletionMessageParamUnion{OfAssistant: &asst})
		case "tool":
			content := msg.Content
			if msg.IsError {
				content += " (tool call failed — retry with corrected arguments)"
			}
			params.Messages = append(params.Messages, sdk.ToolMessage(content, msg.ToolCallID))
		}
	}
	if len(req.Tools) > 0 {
		for _, t := range req.Tools {
			var schema map[string]any
			_ = json.Unmarshal(t.InputSchema, &schema)
			params.Tools = append(params.Tools, sdk.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name:        t.Name,
				Description: sdk.String(t.Description),
				Parameters:  shared.FunctionParameters(schema),
			}))
		}
	}
	return params
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, fmt.Errorf("openai: Embed not implemented")
}

func (c *Client) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	if c.apiKey == "" {
		return openAIFallback(), nil
	}
	page, err := c.sdk.Models.List(ctx)
	if err != nil {
		return openAIFallback(), nil
	}
	models := make([]provider.ModelInfo, 0)
	for _, m := range page.Data {
		// Chat-capable families only — the listing includes embeddings, TTS,
		// image, and moderation models that can't serve completions.
		id := m.ID
		if !strings.HasPrefix(id, "gpt-") && !strings.HasPrefix(id, "o1") && !strings.HasPrefix(id, "o3") && !strings.HasPrefix(id, "o4") {
			continue
		}
		if strings.Contains(id, "instruct") || strings.Contains(id, "0301") || strings.Contains(id, "0314") || strings.Contains(id, "0613") {
			continue
		}
		supportsVision := strings.Contains(id, "gpt-4") || strings.Contains(id, "4o")
		models = append(models, provider.ModelInfo{
			ID: id, Name: id, ContextWindow: 128_000, SupportsTools: true, SupportsVision: supportsVision,
		})
	}
	if len(models) == 0 {
		return openAIFallback(), nil
	}
	return models, nil
}

func openAIFallback() []provider.ModelInfo {
	return []provider.ModelInfo{
		{ID: "gpt-4o", Name: "GPT-4o", ContextWindow: 128_000, SupportsTools: true, SupportsVision: true},
		{ID: "gpt-4o-mini", Name: "GPT-4o mini", ContextWindow: 128_000, SupportsTools: true, SupportsVision: true},
	}
}

func (c *Client) Name() string { return "openai" }
