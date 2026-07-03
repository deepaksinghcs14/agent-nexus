package connector

import (
	"encoding/json"
	"strings"
	"testing"
)

var testKey = []byte("0123456789abcdef0123456789abcdef")

func TestEncryptDecryptRoundtrip(t *testing.T) {
	raw := json.RawMessage(`{"token":"ghp_secret123","url":"https://example.com"}`)
	enc := EncryptConfigSecrets(testKey, raw)

	if strings.Contains(string(enc), "ghp_secret123") {
		t.Fatalf("plaintext token survived encryption: %s", enc)
	}
	var cfg map[string]any
	if err := json.Unmarshal(enc, &cfg); err != nil {
		t.Fatal(err)
	}
	if v := cfg["token"].(string); !strings.HasPrefix(v, encPrefix) {
		t.Fatalf("token not marked encrypted: %s", v)
	}
	if cfg["url"] != "https://example.com" {
		t.Fatalf("non-secret field modified: %v", cfg["url"])
	}

	DecryptConfigSecrets(testKey, cfg)
	if cfg["token"] != "ghp_secret123" {
		t.Fatalf("roundtrip failed: %v", cfg["token"])
	}
}

func TestEncryptIsIdempotent(t *testing.T) {
	raw := json.RawMessage(`{"token":"ghp_secret123"}`)
	once := EncryptConfigSecrets(testKey, raw)
	twice := EncryptConfigSecrets(testKey, once)
	if string(once) != string(twice) {
		t.Fatalf("double encryption changed the value:\n%s\n%s", once, twice)
	}
}

func TestDecryptLeavesLegacyPlaintext(t *testing.T) {
	cfg := map[string]any{"token": "plain_legacy_token"}
	DecryptConfigSecrets(testKey, cfg)
	if cfg["token"] != "plain_legacy_token" {
		t.Fatalf("legacy plaintext altered: %v", cfg["token"])
	}
}

func TestRedactHidesBothForms(t *testing.T) {
	for _, raw := range []string{
		`{"token":"ghp_plain","purpose":"x"}`,
		`{"token":"` + encPrefix + `abc","purpose":"x"}`,
	} {
		red := RedactConfigSecrets(json.RawMessage(raw))
		if strings.Contains(string(red), "ghp_plain") || strings.Contains(string(red), encPrefix) {
			t.Fatalf("secret leaked through redaction: %s", red)
		}
		if !strings.Contains(string(red), `"purpose":"x"`) {
			t.Fatalf("non-secret field lost: %s", red)
		}
	}
}

func TestBadKeyKeepsWorking(t *testing.T) {
	raw := json.RawMessage(`{"token":"ghp_x"}`)
	if out := EncryptConfigSecrets([]byte("short"), raw); string(out) != string(raw) {
		t.Fatalf("unusable key should leave config unchanged: %s", out)
	}
	cfg := map[string]any{"token": "ghp_x"}
	DecryptConfigSecrets([]byte("short"), cfg)
	if cfg["token"] != "ghp_x" {
		t.Fatalf("unusable key mangled plaintext: %v", cfg["token"])
	}
}
