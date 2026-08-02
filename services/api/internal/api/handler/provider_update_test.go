package handler

// Editing a provider without resupplying its API key must not destroy the key.
// The update handler passes an empty encKey whenever the request omits
// api_key, and the UPDATE wrote that column unconditionally — so renaming a
// credential, or just toggling is_active, silently wiped the secret. The
// credential then stays visibly "active" in the UI while every agent run using
// it fails, and the key is unrecoverable.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/encrypt"
)

func TestProviderUpdatePreservesAPIKeyWhenOmitted(t *testing.T) {
	pool := testPool(t)
	owner := newTenant(t, pool)
	providerID := seedProvider(t, pool, owner)

	h := NewProvidersHandler(pool, &config.Config{EncryptionKey: testEncKey})

	// Rename only — no api_key in the body.
	rec := httptest.NewRecorder()
	h.Update(rec, owner.request(http.MethodPut, "/providers/"+providerID,
		"id", providerID, `{"display_name":"renamed"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("update got %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var stored string
	if err := pool.QueryRow(context.Background(),
		`SELECT encrypted_key FROM provider_credentials WHERE id=$1::uuid`, providerID).Scan(&stored); err != nil {
		t.Fatalf("reread credential: %v", err)
	}
	if stored == "" {
		t.Fatal("API key was wiped by an update that did not supply one")
	}
	plain, err := encrypt.Decrypt([]byte(testEncKey), stored)
	if err != nil {
		t.Fatalf("stored key no longer decrypts: %v", err)
	}
	if plain != "sk-victim-secret-key" {
		t.Fatalf("stored key changed: got %q", plain)
	}
}

func TestProviderUpdateReplacesAPIKeyWhenSupplied(t *testing.T) {
	pool := testPool(t)
	owner := newTenant(t, pool)
	providerID := seedProvider(t, pool, owner)

	h := NewProvidersHandler(pool, &config.Config{EncryptionKey: testEncKey})

	rec := httptest.NewRecorder()
	h.Update(rec, owner.request(http.MethodPut, "/providers/"+providerID,
		"id", providerID, `{"api_key":"sk-rotated-key"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("update got %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var stored string
	if err := pool.QueryRow(context.Background(),
		`SELECT encrypted_key FROM provider_credentials WHERE id=$1::uuid`, providerID).Scan(&stored); err != nil {
		t.Fatalf("reread credential: %v", err)
	}
	plain, err := encrypt.Decrypt([]byte(testEncKey), stored)
	if err != nil {
		t.Fatalf("rotated key does not decrypt: %v", err)
	}
	if plain != "sk-rotated-key" {
		t.Fatalf("key rotation did not take effect: got %q", plain)
	}
}
