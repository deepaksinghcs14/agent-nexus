package native

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
)

func jiraTestServer(t *testing.T, wantAuth string, handler http.HandlerFunc) (*httptest.Server, jiraCreds) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, wantAuth) {
			t.Errorf("Authorization = %q, want prefix %q", got, wantAuth)
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, jiraCreds{BaseURL: srv.URL, Email: "me@org.com", Token: "tok"}
}

func TestJiraAuthHeader(t *testing.T) {
	// Cloud: email set → Basic. Data Center: no email → Bearer PAT.
	for _, tc := range []struct {
		email, wantPrefix string
	}{
		{"me@org.com", "Basic "},
		{"", "Bearer tok"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.Header.Get("Authorization"), tc.wantPrefix) {
				t.Errorf("email=%q: Authorization = %q, want prefix %q", tc.email, r.Header.Get("Authorization"), tc.wantPrefix)
			}
			w.Write([]byte(`{}`)) //nolint:errcheck
		}))
		_, _, err := jiraRequest(context.Background(),
			jiraCreds{BaseURL: srv.URL, Email: tc.email, Token: "tok"}, http.MethodGet, "/x", nil)
		if err != nil {
			t.Fatal(err)
		}
		srv.Close()
	}
}

func TestJiraGetIssue(t *testing.T) {
	srv, creds := jiraTestServer(t, "Basic ", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/rest/api/2/issue/PROJ-1") {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{
			"summary":"Fix login","description":"Users cannot log in",
			"status":{"name":"To Do"},"issuetype":{"name":"Bug"},
			"comment":{"comments":[{"author":{"displayName":"Ana"},"body":"repro attached","created":"2026-01-01"}]}}}`))
	})
	_ = srv
	tool := &JiraGetIssueTool{cfg: &config.Config{}}
	out, err := tool.run(context.Background(), creds, map[string]any{"key": "PROJ-1"})
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["summary"] != "Fix login" || m["status"] != "To Do" || m["description"] != "Users cannot log in" {
		t.Errorf("unexpected issue: %v", m)
	}
	if cs := m["comments"].([]map[string]any); len(cs) != 1 || cs[0]["author"] != "Ana" {
		t.Errorf("unexpected comments: %v", m["comments"])
	}
}

func TestJiraSearchFallsBackToLegacyEndpoint(t *testing.T) {
	var paths []string
	srv, creds := jiraTestServer(t, "Basic ", func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/rest/api/2/search/jql" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-2","fields":{"summary":"Add SSO","status":{"name":"In Progress"},"issuetype":{"name":"Story"}}}]}`)) //nolint:errcheck
	})
	_ = srv
	tool := &JiraSearchTool{cfg: &config.Config{}}
	out, err := tool.run(context.Background(), creds, map[string]any{"jql": "project = PROJ"})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[1] != "/rest/api/2/search" {
		t.Errorf("expected fallback to legacy search, got %v", paths)
	}
	m := out.(map[string]any)
	if m["count"] != 1 {
		t.Errorf("count = %v", m["count"])
	}
	issue := m["issues"].([]map[string]any)[0]
	if issue["key"] != "PROJ-2" || issue["status"] != "In Progress" {
		t.Errorf("unexpected issue: %v", issue)
	}
	if _, hasDesc := issue["description"]; hasDesc {
		t.Error("search results must not carry descriptions")
	}
}

func TestJiraTransitionMatchesByTargetStatus(t *testing.T) {
	var posted map[string]any
	srv, creds := jiraTestServer(t, "Basic ", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"transitions":[
				{"id":"11","name":"Start work","to":{"name":"In Progress"}},
				{"id":"21","name":"Send to review","to":{"name":"In Review"}}]}`))
			return
		}
		json.NewDecoder(r.Body).Decode(&posted) //nolint:errcheck
		w.WriteHeader(http.StatusNoContent)
	})
	_ = srv
	tool := &JiraTransitionIssueTool{cfg: &config.Config{}}
	out, err := tool.run(context.Background(), creds, map[string]any{"key": "PROJ-1", "status": "in review"})
	if err != nil {
		t.Fatal(err)
	}
	if tr := posted["transition"].(map[string]any); tr["id"] != "21" {
		t.Errorf("transition id = %v, want 21", tr["id"])
	}
	if out.(map[string]any)["status"] != "In Review" {
		t.Errorf("status = %v", out.(map[string]any)["status"])
	}

	// Unknown status surfaces the available transitions.
	_, err = tool.run(context.Background(), creds, map[string]any{"key": "PROJ-1", "status": "Done"})
	if err == nil || !strings.Contains(err.Error(), "In Review") {
		t.Errorf("expected available-transitions error, got %v", err)
	}
}

func TestJiraMissingCreds(t *testing.T) {
	tool := &JiraGetIssueTool{cfg: &config.Config{}}
	if _, err := tool.run(context.Background(), jiraCreds{}, map[string]any{"key": "PROJ-1"}); err == nil {
		t.Fatal("expected missing-credentials error")
	}
}
