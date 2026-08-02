package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
)

// These endpoints hand out decrypted Baileys session credentials, so an
// unauthenticated caller could take over the linked WhatsApp account. They are
// reachable on the public API surface — "internal only" was never enforced by
// anything but the URL prefix.
func TestWhatsAppCredsRequireToken(t *testing.T) {
	h := NewWhatsAppCredsHandler(nil, &config.Config{WhatsAppInternalToken: "correct-token"})

	handlers := map[string]http.HandlerFunc{
		"Get":       h.Get,
		"Put":       h.Put,
		"Delete":    h.Delete,
		"PutLIDMap": h.PutLIDMap,
	}

	for name, fn := range handlers {
		t.Run(name+" rejects a missing token", func(t *testing.T) {
			rec := httptest.NewRecorder()
			fn(rec, httptest.NewRequest(http.MethodGet, "/internal/whatsapp/wa-1/credentials", nil))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("got %d, want 401", rec.Code)
			}
		})

		t.Run(name+" rejects a wrong token", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/internal/whatsapp/wa-1/credentials", nil)
			req.Header.Set("X-WhatsApp-Token", "guess")
			rec := httptest.NewRecorder()
			fn(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("got %d, want 401", rec.Code)
			}
		})
	}
}

// Fail closed: with no token configured the endpoints must not fall open.
func TestWhatsAppCredsFailClosedWhenUnconfigured(t *testing.T) {
	h := NewWhatsAppCredsHandler(nil, &config.Config{WhatsAppInternalToken: ""})
	req := httptest.NewRequest(http.MethodGet, "/internal/whatsapp/wa-1/credentials", nil)
	req.Header.Set("X-WhatsApp-Token", "anything")
	rec := httptest.NewRecorder()
	h.Get(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404 when the token is unconfigured", rec.Code)
	}
}
