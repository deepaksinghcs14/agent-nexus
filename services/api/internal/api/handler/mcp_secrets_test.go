package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMCPConfigSecretsRoundtrip(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	raw := json.RawMessage(`{"token":"sk-abc","env":{"JIRA_API_TOKEN":"tok-1","JIRA_URL":"https://org.atlassian.net"}}`)

	enc := encryptMCPConfig(key, raw)
	var cfg struct {
		Token string            `json:"token"`
		Env   map[string]string `json:"env"`
	}
	if err := json.Unmarshal(enc, &cfg); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cfg.Token, mcpEncPrefix) || !strings.HasPrefix(cfg.Env["JIRA_API_TOKEN"], mcpEncPrefix) {
		t.Errorf("secrets not encrypted: %s", enc)
	}
	// Idempotent: re-encrypting changes nothing.
	if again := encryptMCPConfig(key, enc); string(again) != string(enc) {
		t.Error("re-encryption must be a no-op")
	}

	dec := decryptMCPConfig(key, enc)
	if err := json.Unmarshal(dec, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "sk-abc" || cfg.Env["JIRA_API_TOKEN"] != "tok-1" || cfg.Env["JIRA_URL"] != "https://org.atlassian.net" {
		t.Errorf("roundtrip mismatch: %s", dec)
	}

	red := redactMCPConfig(enc)
	if strings.Contains(string(red), "sk-abc") || strings.Contains(string(red), mcpEncPrefix) {
		t.Errorf("redaction leaked a secret: %s", red)
	}
	if !strings.Contains(string(red), "JIRA_API_TOKEN") {
		t.Error("redaction must keep env keys visible")
	}
}

func TestMCPConfigLegacyPlaintextPassthrough(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	raw := json.RawMessage(`{"token":"plain-token"}`)
	// Decrypting legacy plaintext leaves it untouched.
	if dec := decryptMCPConfig(key, raw); string(dec) != string(raw) {
		t.Errorf("plaintext must pass through, got %s", dec)
	}
	// Unusable key: encrypt is a no-op rather than an error.
	if enc := encryptMCPConfig([]byte("short"), raw); string(enc) != string(raw) {
		t.Errorf("unusable key must be a no-op, got %s", enc)
	}
}
