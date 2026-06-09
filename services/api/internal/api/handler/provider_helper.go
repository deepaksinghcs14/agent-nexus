package handler

import (
	"context"
	"fmt"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/anthropic"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/gemini"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/ollama"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/openai"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/encrypt"
	"github.com/jackc/pgx/v5/pgxpool"
)

// buildAnyProvider returns the first active provider credential found for the workspace.
// Tries openai → anthropic → gemini → ollama in order. Used for embedding during connector sync.
func buildAnyProvider(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, workspaceID string) (provider.Provider, error) {
	provRepo := repository.NewProviderRepository(pool)
	for _, name := range []string{"openai", "anthropic", "gemini", "ollama"} {
		cred, encKey, err := provRepo.GetActiveByProvider(ctx, workspaceID, name)
		if err != nil {
			continue
		}
		// Skip OAuth providers — they need token refresh logic not needed for embedding-only use
		if cred.AuthType == "oauth" {
			continue
		}
		apiKey, err := encrypt.Decrypt([]byte(cfg.EncryptionKey), encKey)
		if err != nil {
			continue
		}
		switch cred.Provider {
		case "openai":
			return openai.New(apiKey, cred.BaseURL), nil
		case "anthropic":
			return anthropic.New(apiKey, cred.BaseURL), nil
		case "gemini":
			return gemini.New(apiKey, "api_key"), nil
		case "ollama":
			return ollama.New(cred.BaseURL), nil
		}
	}
	return nil, fmt.Errorf("no active provider configured for workspace %s", workspaceID)
}
