package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port                   string
	DatabaseURL            string
	JWTSecret              string
	EncryptionKey          string
	CORSOrigins            []string
	LogLevel               string
	StoragePath            string
	GoogleOAuthClientID     string
	GoogleOAuthClientSecret string
	PublicAPIURL            string // base URL this API is reachable at (for OAuth redirect URIs)
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
