package mcp

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

func TestNewPKCE(t *testing.T) {
	verifier, challenge := NewPKCE()
	if len(verifier) < 43 {
		t.Fatalf("verifier too short (%d chars) for RFC 7636", len(verifier))
	}
	sum := sha256.Sum256([]byte(verifier))
	if want := base64.RawURLEncoding.EncodeToString(sum[:]); challenge != want {
		t.Fatalf("challenge is not S256(verifier): got %s want %s", challenge, want)
	}
	v2, _ := NewPKCE()
	if v2 == verifier {
		t.Fatal("verifiers must be unique")
	}
}

func TestAuthorizeURL(t *testing.T) {
	meta := &ASMetadata{AuthorizationEndpoint: "https://as.example.com/authorize"}
	raw := AuthorizeURL(meta, "client-1", "https://api.example.com/cb", "srv.nonce", "chal", "https://mcp.example.com/mcp", "read write")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("unparseable URL: %v", err)
	}
	q := u.Query()
	for key, want := range map[string]string{
		"response_type":         "code",
		"client_id":             "client-1",
		"redirect_uri":          "https://api.example.com/cb",
		"state":                 "srv.nonce",
		"code_challenge":        "chal",
		"code_challenge_method": "S256",
		"resource":              "https://mcp.example.com/mcp",
		"scope":                 "read write",
	} {
		if got := q.Get(key); got != want {
			t.Errorf("%s: got %q want %q", key, got, want)
		}
	}
	// Endpoint that already carries query params must be joined with '&'.
	meta2 := &ASMetadata{AuthorizationEndpoint: "https://as.example.com/authorize?tenant=x"}
	if raw2 := AuthorizeURL(meta2, "c", "r", "s", "ch", "", ""); !strings.Contains(raw2, "tenant=x&") {
		t.Errorf("existing query params clobbered: %s", raw2)
	}
}
