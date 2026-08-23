package handler

import (
	"context"
	"fmt"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/anthropic"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/gemini"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/nvidia"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/ollama"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/openai"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/encrypt"
)

// nvidiaDefaultBaseURL is NVIDIA's hosted NIM catalog. Self-hosted NIM
// deployments override this via cred.BaseURL.
const nvidiaDefaultBaseURL = "https://integrate.api.nvidia.com"

// providerFromCredential is the single place a stored provider credential
// becomes a client: gemini OAuth token exchange, keyless providers (ollama),
// then decrypt-and-construct for everything else. Both the run loop's
// provider lookup and the providers API delegate here.
func providerFromCredential(ctx context.Context, cfg *config.Config, providers *repository.ProviderRepository, cred *domain.ProviderCredential, encKey string) (provider.Provider, error) {
	if cred.AuthType == "oauth" && cred.Provider == "gemini" {
		if cfg.GoogleOAuthClientID == "" {
			return nil, fmt.Errorf("google oauth not configured")
		}
		accessToken, err := providers.GetDecryptedAccessToken(ctx, cred.ID, cfg.EncryptionKey, cfg.GoogleOAuthClientID, cfg.GoogleOAuthClientSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to get oauth access token: %w", err)
		}
		return gemini.New(accessToken, "oauth"), nil
	}
	if encKey == "" {
		switch cred.Provider {
		case "ollama":
			return ollama.New(cred.BaseURL), nil
		default:
			return nil, fmt.Errorf("no api key configured for %s", cred.Provider)
		}
	}
	apiKey, err := encrypt.Decrypt([]byte(cfg.EncryptionKey), encKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt provider credential")
	}
	switch cred.Provider {
	case "anthropic":
		return anthropic.New(apiKey, cred.BaseURL), nil
	case "openai":
		return openai.New(apiKey, cred.BaseURL), nil
	case "nvidia":
		baseURL := cred.BaseURL
		if baseURL == "" {
			baseURL = nvidiaDefaultBaseURL
		}
		return nvidia.New(apiKey, baseURL), nil
	case "gemini":
		return gemini.New(apiKey, "api_key"), nil
	case "ollama":
		return ollama.New(cred.BaseURL), nil
	default:
		return nil, fmt.Errorf("provider %q is not supported", cred.Provider)
	}
}

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
