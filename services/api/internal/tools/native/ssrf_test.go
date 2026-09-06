package native

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBlockedIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},       // loopback
		{"::1", true},             // loopback v6
		{"10.1.2.3", true},        // private
		{"192.168.1.1", true},     // private
		{"172.16.0.1", true},      // private
		{"169.254.169.254", true}, // cloud metadata
		{"fd00::1", true},         // unique local v6
		{"0.0.0.0", true},         // unspecified
		{"224.0.0.1", true},       // multicast
		{"100.64.0.1", true},      // carrier-grade NAT
		{"8.8.8.8", false},        // public
		{"93.184.216.34", false},  // public
		{"2606:4700::1", false},   // public v6
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("bad test IP %q", c.ip)
		}
		if got := blockedIP(ip); got != c.want {
			t.Errorf("blockedIP(%s) = %v, want %v", c.ip, got, c.want)
		}
	}
}

// The remaining tests dial real listeners: the guard lives in the dialer, so
// only an actual connection attempt proves it fires.

func TestSSRFBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("SECRET-INTERNAL-DATA")) //nolint:errcheck
	}))
	defer srv.Close()

	if out, err := NewHTTPRequestTool(nil).Execute(map[string]any{"url": srv.URL}); err == nil {
		t.Fatalf("expected loopback to be blocked, got response: %v", out)
	} else if !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected a block error, got: %v", err)
	}
}

// A URL-level allowlist would pass this and then follow the 302 straight to the
// internal service; the dial-time check catches the redirect hop too.
func TestSSRFBlocksRedirectToLoopback(t *testing.T) {
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("SECRET")) //nolint:errcheck
	}))
	defer internal.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL, http.StatusFound)
	}))
	defer redirector.Close()

	if _, err := NewHTTPRequestTool(nil).Execute(map[string]any{"url": redirector.URL}); err == nil {
		t.Fatal("expected redirect to loopback to be blocked")
	}
}

func TestSSRFAllowlistPermitsHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok")) //nolint:errcheck
	}))
	defer srv.Close()

	host, _, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("parse test server host: %v", err)
	}

	tool := &HTTPRequestTool{client: SafeHTTPClient([]string{host}, 10*time.Second)}
	out, err := tool.Execute(map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("allowlisted host should be reachable, got: %v", err)
	}
	if body := out.(map[string]any)["body"].(string); body != "ok" {
		t.Fatalf("unexpected body: %q", body)
	}
}
