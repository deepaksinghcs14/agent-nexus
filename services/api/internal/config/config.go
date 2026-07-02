package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port                    string
	DatabaseURL             string
	JWTSecret               string
	EncryptionKey           string
	CORSOrigins             []string
	LogLevel                string
	StoragePath             string
	GoogleOAuthClientID     string
	GoogleOAuthClientSecret string
	PublicAPIURL            string // base URL this API is reachable at (for OAuth redirect URIs)
	PublicAppURL            string // base URL of the frontend app (used in LLM-generated links)
	WhatsAppAdapterURL      string // internal URL for the WhatsApp Web adapter service
	LogStreamIngestToken    string // shared secret for local process log forwarding
	DemoMode                bool   // when true, restricts dangerous capabilities for public hosted instances
	EmbedOllamaURL          string // Ollama base URL used for embeddings (e.g. http://localhost:11434)
	EmbedModel              string // embedding model name (default: nomic-embed-text)
	RunnerURL               string // base URL of the repo-session runner service (empty = sessions disabled)
	RunnerCallbackSecret    string // shared secret the runner presents on session callbacks
	GithubToken             string // token for GitHub API tools (PR creation, branch diffs)
	GithubAPIURL            string // GitHub API base URL (override for tests / GHE)
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:                    getEnv("PORT", "8080"),
		DatabaseURL:             getEnv("DATABASE_URL", ""),
		JWTSecret:               getEnv("JWT_SECRET", ""),
		EncryptionKey:           getEnv("ENCRYPTION_KEY", ""),
		LogLevel:                getEnv("LOG_LEVEL", "info"),
		StoragePath:             getEnv("STORAGE_PATH", "./data/files"),
		GoogleOAuthClientID:     getEnv("GOOGLE_OAUTH_CLIENT_ID", ""),
		GoogleOAuthClientSecret: getEnv("GOOGLE_OAUTH_CLIENT_SECRET", ""),
		PublicAPIURL:            getEnv("PUBLIC_API_URL", "http://localhost:8080"),
		PublicAppURL:            getEnv("PUBLIC_APP_URL", "http://localhost:3000"),
		WhatsAppAdapterURL:      getEnv("WHATSAPP_ADAPTER_URL", "http://127.0.0.1:18901"),
		LogStreamIngestToken:    getEnv("LOG_STREAM_INGEST_TOKEN", ""),
		DemoMode:                getEnvBool("DEMO_MODE", false),
		EmbedOllamaURL:          getEnv("EMBED_OLLAMA_URL", "http://localhost:11434"),
		EmbedModel:              getEnv("EMBED_MODEL", "nomic-embed-text"),
		RunnerURL:               getEnv("RUNNER_URL", ""),
		RunnerCallbackSecret:    getEnv("RUNNER_CALLBACK_SECRET", ""),
		GithubToken:             getEnv("GITHUB_TOKEN", ""),
		GithubAPIURL:            getEnv("GITHUB_API_URL", "https://api.github.com"),
	}

	origins := getEnv("CORS_ORIGINS", "http://localhost:3000")
	cfg.CORSOrigins = strings.Split(origins, ",")

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if len(c.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}
	if c.EncryptionKey == "" {
		return fmt.Errorf("ENCRYPTION_KEY is required")
	}
	if len(c.EncryptionKey) != 32 {
		return fmt.Errorf("ENCRYPTION_KEY must be exactly 32 characters (AES-256)")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "true" || v == "1" || v == "yes" {
		return true
	}
	if v == "false" || v == "0" || v == "no" {
		return false
	}
	return fallback
}
