// Package nvidia adapts NVIDIA's hosted NIM catalog (and self-hosted NIM
// deployments) to the provider.Provider interface. NIM's chat completions
// wire protocol is OpenAI-compatible, so Complete/Embed are reused verbatim
// from the openai client via embedding — only Models is overridden, since
// openai.Client.Models() filters to OpenAI's own gpt-/o1-/o3-/o4- model-ID
// prefixes and silently falls back to fake OpenAI model data on any error,
// both of which are wrong for NVIDIA's org/model-name catalog.
package nvidia

import (
	"context"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/openai"
)

type Client struct {
	*openai.Client
}

func New(apiKey, baseURL string) *Client {
	return &Client{Client: openai.New(apiKey, baseURL)}
}

func (c *Client) Name() string { return "nvidia" }

// Models returns NVIDIA's curated catalog. NIM's /v1/models listing uses a
// different ID namespace (org/model-name) than openai.Client.Models() knows
// how to filter, so live discovery isn't wired up yet — this static list
// covers the common instruct models until that's worth building.
func (c *Client) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	return []provider.ModelInfo{
		{ID: "meta/llama-3.1-70b-instruct", Name: "Llama 3.1 70B Instruct", ContextWindow: 128_000, SupportsTools: true},
		{ID: "meta/llama-3.1-8b-instruct", Name: "Llama 3.1 8B Instruct", ContextWindow: 128_000, SupportsTools: true},
		{ID: "nvidia/llama-3.1-nemotron-70b-instruct", Name: "Llama 3.1 Nemotron 70B Instruct", ContextWindow: 128_000, SupportsTools: true},
	}, nil
}
