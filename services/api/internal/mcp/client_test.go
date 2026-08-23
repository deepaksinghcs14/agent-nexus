package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractSSEData(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{"space after colon", "event: message\ndata: {\"a\":1}\n\n", `{"a":1}`, false},
		{"no space after colon", "data:{\"a\":1}\n\n", `{"a":1}`, false},
		{"multi-line data joined per spec", "data: {\"a\":\ndata: 1}\n\n", "{\"a\":\n1}", false},
		{"no data line at all", "event: message\n: keepalive\n\n", "", true},
		{"empty body", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := extractSSEData(strings.NewReader(c.body))
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

// A 401/403 from the MCP server (the exact case the tool-test button exists
// to surface) must not be reported as an SSE/JSON parse failure — the caller
// needs the real rejection to fix their auth, not "no data line in SSE response".
func TestDoHTTPReportsAuthFailureInsteadOfParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &Client{url: srv.URL, transport: "http"}
	_, _, _, _, err := c.doHTTP(context.Background(), newReq("tools/call", map[string]any{}), "")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected the status code in the error, got: %v", err)
	}
	if strings.Contains(err.Error(), "no data line") {
		t.Fatalf("auth failure leaked as a parse error: %v", err)
	}
}

func TestDoHTTPParsesJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rpcResp{Result: json.RawMessage(`{"ok":true}`)})
	}))
	defer srv.Close()

	c := &Client{url: srv.URL, transport: "http"}
	r, _, _, _, err := c.doHTTP(context.Background(), newReq("tools/call", map[string]any{}), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(r.Result) != `{"ok":true}` {
		t.Fatalf("got result %q", r.Result)
	}
}

// Some servers echo back Content-Type: text/event-stream (matching our
// Accept header) but never actually stream — they just write one plain JSON
// object with no "data:" framing at all. That must parse fine rather than
// failing with "no data line in SSE response".
func TestDoHTTPParsesPlainJSONDespiteEventStreamContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_ = json.NewEncoder(w).Encode(rpcResp{Result: json.RawMessage(`{"ok":true}`)})
	}))
	defer srv.Close()

	c := &Client{url: srv.URL, transport: "http"}
	r, _, _, _, err := c.doHTTP(context.Background(), newReq("tools/call", map[string]any{}), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(r.Result) != `{"ok":true}` {
		t.Fatalf("got result %q", r.Result)
	}
}

// Real SSE framing (a server that genuinely streams) must still work.
func TestDoHTTPParsesRealSSEStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		body, _ := json.Marshal(rpcResp{Result: json.RawMessage(`{"ok":true}`)})
		_, _ = w.Write([]byte("event: message\ndata: " + string(body) + "\n\n"))
	}))
	defer srv.Close()

	c := &Client{url: srv.URL, transport: "http"}
	r, _, _, _, err := c.doHTTP(context.Background(), newReq("tools/call", map[string]any{}), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(r.Result) != `{"ok":true}` {
		t.Fatalf("got result %q", r.Result)
	}
}
