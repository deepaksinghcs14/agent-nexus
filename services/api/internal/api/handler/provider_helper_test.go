package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/encrypt"
)

// providerFromCredential is pure given a credential struct (the OAuth path is
// the only one that touches the repository), so it tests without a DB.
func TestProviderFromCredential(t *testing.T) {
	cfg := &config.Config{EncryptionKey: strings.Repeat("k", 32)}
	encKey, err := encrypt.Encrypt([]byte(cfg.EncryptionKey), "sk-test-123")
	if err != nil {
		t.Fatalf("encrypt fixture key: %v", err)
	}

	cases := []struct {
		name     string
		cred     domain.ProviderCredential
		encKey   string
		wantName string
		wantErr  string
	}{
		{name: "anthropic api key", cred: domain.ProviderCredential{Provider: "anthropic"}, encKey: encKey, wantName: "anthropic"},
		{name: "openai api key", cred: domain.ProviderCredential{Provider: "openai"}, encKey: encKey, wantName: "openai"},
		{name: "gemini api key", cred: domain.ProviderCredential{Provider: "gemini"}, encKey: encKey, wantName: "gemini"},
		// The keyless-ollama case is the one the run loop's old copy of this
		// switch got wrong — it failed decryption instead of constructing.
		{name: "keyless ollama", cred: domain.ProviderCredential{Provider: "ollama", BaseURL: "http://localhost:11434"}, encKey: "", wantName: "ollama"},
		{name: "keyless anthropic rejected", cred: domain.ProviderCredential{Provider: "anthropic"}, encKey: "", wantErr: "no api key"},
		{name: "unsupported provider", cred: domain.ProviderCredential{Provider: "clippy"}, encKey: encKey, wantErr: "not supported"},
		{name: "corrupt key", cred: domain.ProviderCredential{Provider: "anthropic"}, encKey: "not-base64!", wantErr: "decrypt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := providerFromCredential(context.Background(), cfg, nil, &tc.cred, tc.encKey)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.Name() != tc.wantName {
				t.Fatalf("provider name = %q, want %q", p.Name(), tc.wantName)
			}
		})
	}
}
