package main

// Cancel() on the API used to be a no-op against the runner: it had no way
// to stop the claude subprocess, so a "cancelled" session kept burning
// tokens for up to SESSION_TIMEOUT_MIN regardless. handleCancel is the fix —
// these tests exercise it directly against the session registry, the same
// way handleLaunch's join logic is exercised.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer(secret string) *server {
	return &server{sessions: map[string]*session{}, callbackSecret: secret}
}

func TestHandleCancelStopsRegisteredSession(t *testing.T) {
	s := newTestServer("shh")
	ctx, cancel := context.WithCancel(context.Background())
	s.sessions["T-1|owner/repo"] = &session{key: "T-1|owner/repo", cancel: cancel}

	body, _ := json.Marshal(map[string]string{"session_key": "T-1|owner/repo"})
	req := httptest.NewRequest(http.MethodPost, "/sessions/cancel", bytes.NewReader(body))
	req.Header.Set("X-Runner-Secret", "shh")
	w := httptest.NewRecorder()
	s.handleCancel(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Cancelled bool `json:"cancelled"`
	}
	if json.Unmarshal(w.Body.Bytes(), &resp) != nil || !resp.Cancelled {
		t.Fatalf("body = %s, want cancelled:true", w.Body.String())
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("session's context was not cancelled")
	}
}

func TestHandleCancelUnknownSessionIsNoopOK(t *testing.T) {
	s := newTestServer("shh")
	body, _ := json.Marshal(map[string]string{"session_key": "nope"})
	req := httptest.NewRequest(http.MethodPost, "/sessions/cancel", bytes.NewReader(body))
	req.Header.Set("X-Runner-Secret", "shh")
	w := httptest.NewRecorder()
	s.handleCancel(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (idempotent no-op)", w.Code)
	}
	var resp struct {
		Cancelled bool `json:"cancelled"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp) //nolint:errcheck
	if resp.Cancelled {
		t.Fatal("cancelled = true for an unknown session")
	}
}

func TestHandleCancelDoneSessionIsNoop(t *testing.T) {
	s := newTestServer("shh")
	_, cancel := context.WithCancel(context.Background())
	s.sessions["T-1|owner/repo"] = &session{key: "T-1|owner/repo", cancel: cancel, done: true}

	body, _ := json.Marshal(map[string]string{"session_key": "T-1|owner/repo"})
	req := httptest.NewRequest(http.MethodPost, "/sessions/cancel", bytes.NewReader(body))
	req.Header.Set("X-Runner-Secret", "shh")
	w := httptest.NewRecorder()
	s.handleCancel(w, req)

	var resp struct {
		Cancelled bool `json:"cancelled"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp) //nolint:errcheck
	if resp.Cancelled {
		t.Fatal("cancelled = true for a session already marked done")
	}
}

func TestHandleCancelRequiresSecret(t *testing.T) {
	s := newTestServer("shh")
	_, cancel := context.WithCancel(context.Background())
	s.sessions["T-1|owner/repo"] = &session{key: "T-1|owner/repo", cancel: cancel}

	body, _ := json.Marshal(map[string]string{"session_key": "T-1|owner/repo"})

	for _, secret := range []string{"", "wrong"} {
		req := httptest.NewRequest(http.MethodPost, "/sessions/cancel", bytes.NewReader(body))
		if secret != "" {
			req.Header.Set("X-Runner-Secret", secret)
		}
		w := httptest.NewRecorder()
		s.handleCancel(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("secret %q: status = %d, want 404 (fail closed)", secret, w.Code)
		}
	}
}

func TestHandleCancelFailsClosedWhenSecretUnset(t *testing.T) {
	s := newTestServer("") // RUNNER_CALLBACK_SECRET not configured
	body, _ := json.Marshal(map[string]string{"session_key": "anything"})
	req := httptest.NewRequest(http.MethodPost, "/sessions/cancel", bytes.NewReader(body))
	req.Header.Set("X-Runner-Secret", "")
	w := httptest.NewRecorder()
	s.handleCancel(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when no secret is configured", w.Code)
	}
}

func TestSummarizeStreamEvent(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "tool use",
			line: `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{}}]}}`,
			want: "using Read",
		},
		{
			name: "text only",
			line: `{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}`,
			want: "writing a response",
		},
		{
			name: "system init event ignored",
			line: `{"type":"system","subtype":"init","session_id":"x"}`,
			want: "",
		},
		{
			name: "result event ignored",
			line: `{"type":"result","subtype":"success","is_error":false}`,
			want: "",
		},
		{
			name: "rate limit event ignored",
			line: `{"type":"rate_limit_event","rate_limit_info":{}}`,
			want: "",
		},
		{
			name: "malformed line ignored",
			line: `not json`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := summarizeStreamEvent(tc.line); got != tc.want {
				t.Fatalf("summarizeStreamEvent(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}
