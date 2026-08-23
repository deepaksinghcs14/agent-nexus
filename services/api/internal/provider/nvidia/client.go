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
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/openai"
)

type Client struct {
	*openai.Client
	apiKey  string
	baseURL string
}

func New(apiKey, baseURL string) *Client {
	return &Client{Client: openai.New(apiKey, baseURL), apiKey: apiKey, baseURL: baseURL}
}

func (c *Client) Name() string { return "nvidia" }

var httpClient = &http.Client{Timeout: 15 * time.Second}

// nonChatSubstrings excludes NIM catalog entries that share the /v1/models
// listing but can't serve chat completions: embeddings, guardrail/safety
// classifiers, reward models, retrievers, translation, parsing, and other
// non-LLM utility models (detectors, calibration, CLIP, grounding VLMs).
var nonChatSubstrings = []string{
	"embed", "bge", "guard", "safety", "reward", "retriever", "translate",
	"-parse", "detector", "calibration", "nvclip", "kosmos", "deplot", "diffusion",
}

func isChatModel(id string) bool {
	lower := strings.ToLower(id)
	for _, s := range nonChatSubstrings {
		if strings.Contains(lower, s) {
			return false
		}
	}
	return true
}

// Models lists NVIDIA's live catalog. Unlike inference, catalog listing on
// the hosted endpoint doesn't require a valid key, but self-hosted NIM might,
// so the key is sent when present. Falls back to a small curated list if the
// request fails — the live catalog moves independently of this repo.
func (c *Client) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.baseURL, "/")+"/v1/models", nil)
	if err != nil {
		return staticModels(), nil
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return staticModels(), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return staticModels(), nil
	}

	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&page) != nil || len(page.Data) == 0 {
		return staticModels(), nil
	}

	models := make([]provider.ModelInfo, 0, len(page.Data))
	for _, m := range page.Data {
		if !isChatModel(m.ID) {
			continue
		}
		vision := strings.Contains(m.ID, "vision") || strings.Contains(m.ID, "-vl") || strings.HasSuffix(m.ID, "vl")
		models = append(models, provider.ModelInfo{
			ID: m.ID, Name: m.ID, ContextWindow: 128_000, SupportsTools: true, SupportsVision: vision,
		})
	}
	if len(models) == 0 {
		return staticModels(), nil
	}
	return models, nil
}

func staticModels() []provider.ModelInfo {
	return []provider.ModelInfo{
		{ID: "meta/llama-3.1-70b-instruct", Name: "Llama 3.1 70B Instruct", ContextWindow: 128_000, SupportsTools: true},
		{ID: "meta/llama-3.1-8b-instruct", Name: "Llama 3.1 8B Instruct", ContextWindow: 128_000, SupportsTools: true},
		{ID: "nvidia/llama-3.1-nemotron-70b-instruct", Name: "Llama 3.1 Nemotron 70B Instruct", ContextWindow: 128_000, SupportsTools: true},
	}
}
