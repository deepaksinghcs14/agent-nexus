package handler

import (
	"context"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/ollama"
)

// buildEmbedder returns an Ollama client configured for embedding using the
// local bundled Ollama instance. Returns nil if EmbedOllamaURL is not set.
func buildEmbedder(cfg *config.Config) provider.Provider {
	if cfg.EmbedOllamaURL == "" {
		return nil
	}
	return ollama.NewEmbedder(cfg.EmbedOllamaURL, cfg.EmbedModel)
}

// tryEmbed attempts to embed text using the dedicated embed client first,
// falling back to the agent's LLM if the embed client is unavailable.
func tryEmbed(ctx context.Context, cfg *config.Config, llm provider.Provider, text string) []float32 {
	if cfg.EmbedOllamaURL != "" {
		emb := ollama.NewEmbedder(cfg.EmbedOllamaURL, cfg.EmbedModel)
		if v, err := emb.Embed(ctx, text); err == nil {
			return v
		}
	}
	v, _ := llm.Embed(ctx, text)
	return v
}
