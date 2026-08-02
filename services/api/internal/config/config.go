package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                    string
	DatabaseURL             string
	JWTSecret               string
	EncryptionKey           string
	CORSOrigins             []string
	StoragePath             string
	GoogleOAuthClientID     string
	GoogleOAuthClientSecret string
	PublicAPIURL            string   // base URL this API is reachable at (for OAuth redirect URIs)
	PublicAppURL            string   // base URL of the frontend app (used in LLM-generated links)
	WhatsAppAdapterURL      string   // internal URL for the WhatsApp Web adapter service
	WhatsAppInternalToken   string   // shared secret the adapter presents on /internal/whatsapp/* (auto-generated in start-api.sh)
	LogStreamIngestToken    string   // shared secret for local process log forwarding
	DemoMode                bool     // when true, restricts dangerous capabilities for public hosted instances
	EmbedOllamaURL          string   // Ollama base URL used for embeddings (e.g. http://localhost:11434)
	EmbedModel              string   // embedding model name (default: nomic-embed-text)
	RunnerURL               string   // base URL of the repo-session runner service (empty = sessions disabled)
	RunnerCallbackSecret    string   // shared secret the runner presents on session callbacks
	SessionCallbackURL      string   // callback URL the RUNNER uses to reach this API (defaults to PUBLIC_API_URL; override when the runner reaches the API over an internal network, e.g. http://api:8080 in Docker)
	GithubToken             string   // token for GitHub API tools (PR creation, branch diffs)
	GithubAPIURL            string   // GitHub API base URL (override for tests / GHE)
	SessionWaitTimeoutMin   int      // minutes before a session_wait run with no callback is failed as crashed (0 = default 240)
	RequireSessionApproval  bool     // gate native_launch_repo_session / native_create_pull_request behind a human approval
	HTTPToolAllowHosts      []string // hostnames native_http_request may reach despite resolving to a private/internal address
	RateLimitAuthPerMin     int      // per-client requests/min on login, register, refresh and OAuth callbacks (0 = disabled)
	RateLimitIngressPerMin  int      // per-client requests/min on public webhook and gateway ingress (0 = disabled)
	TrustedProxyCount       int      // reverse proxies in front of this API; 0 = none, ignore X-Forwarded-For entirely
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:                    getEnv("PORT", "8080"),
		DatabaseURL:             getEnv("DATABASE_URL", ""),
		JWTSecret:               getEnv("JWT_SECRET", ""),
		EncryptionKey:           getEnv("ENCRYPTION_KEY", ""),
		StoragePath:             getEnv("STORAGE_PATH", "./data/files"),
		GoogleOAuthClientID:     getEnv("GOOGLE_OAUTH_CLIENT_ID", ""),
		GoogleOAuthClientSecret: getEnv("GOOGLE_OAUTH_CLIENT_SECRET", ""),
		PublicAPIURL:            getEnv("PUBLIC_API_URL", "http://localhost:8080"),
		PublicAppURL:            getEnv("PUBLIC_APP_URL", "http://localhost:3000"),
		WhatsAppAdapterURL:      getEnv("WHATSAPP_ADAPTER_URL", "http://127.0.0.1:18901"),
		WhatsAppInternalToken:   getEnv("WHATSAPP_INTERNAL_TOKEN", ""),
		LogStreamIngestToken:    getEnv("LOG_STREAM_INGEST_TOKEN", ""),
		DemoMode:                getEnvBool("DEMO_MODE", false),
		EmbedOllamaURL:          getEnv("EMBED_OLLAMA_URL", "http://localhost:11434"),
		EmbedModel:              getEnv("EMBED_MODEL", "nomic-embed-text"),
		RunnerURL:               getEnv("RUNNER_URL", ""),
		RunnerCallbackSecret:    getEnv("RUNNER_CALLBACK_SECRET", ""),
		SessionCallbackURL:      getEnv("SESSION_CALLBACK_URL", ""),
		GithubToken:             getEnv("GITHUB_TOKEN", ""),
		GithubAPIURL:            getEnv("GITHUB_API_URL", "https://api.github.com"),
		SessionWaitTimeoutMin:   getEnvInt("SESSION_WAIT_TIMEOUT_MIN", 240),
		RequireSessionApproval:  getEnvBool("REQUIRE_SESSION_APPROVAL", false),
		RateLimitAuthPerMin:     getEnvInt("RATE_LIMIT_AUTH_PER_MIN", 20),
		RateLimitIngressPerMin:  getEnvInt("RATE_LIMIT_INGRESS_PER_MIN", 120),
		// Defaults to 1: the canonical deployment (Railway, and most PaaS) puts
		// exactly one edge proxy in front, which appends the real client IP.
		// Set to 0 when the API is exposed directly — otherwise a caller can
		// forge X-Forwarded-For to dodge rate limits and fake audit-log IPs.
		TrustedProxyCount: getEnvInt("TRUSTED_PROXY_COUNT", 1),
	}

	origins := getEnv("CORS_ORIGINS", "http://localhost:3000")
	cfg.CORSOrigins = strings.Split(origins, ",")

	// Empty by default — every private/loopback destination is blocked unless
	// an operator names the internal hosts their agents legitimately call.
	if hosts := strings.TrimSpace(getEnv("HTTP_TOOL_ALLOW_HOSTS", "")); hosts != "" {
		cfg.HTTPToolAllowHosts = strings.Split(hosts, ",")
	}

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

func getEnvInt(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
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
