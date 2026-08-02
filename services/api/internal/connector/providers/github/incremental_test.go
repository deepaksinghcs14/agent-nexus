package github

// A GitHub connector sync used to re-walk a repo's entire tree and re-fetch
// every file's content on every single sync — the repo-info response
// already carries pushed_at, discarded until now. syncRepo takes its `do`
// HTTP function as a parameter specifically so these tests can supply canned
// responses without touching the network or the package-level httpClient.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/connector"
)

func jsonResp(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
}

func TestSyncRepoSkipsUnchangedRepo(t *testing.T) {
	pushedAt := time.Now().Add(-48 * time.Hour).UTC()
	lastSyncedAt := pushedAt.Add(time.Hour) // synced after the repo's last push

	treeCalled := false
	do := func(u string) (*http.Response, error) {
		switch {
		case strings.Contains(u, "/git/trees/"):
			treeCalled = true
			return jsonResp(`{"tree":[]}`), nil
		case strings.HasSuffix(u, "/repos/o/r"):
			return jsonResp(fmt.Sprintf(`{"default_branch":"main","pushed_at":%q}`, pushedAt.Format(time.RFC3339))), nil
		}
		return nil, fmt.Errorf("unexpected request: %s", u)
	}

	c := New()
	emitted := 0
	err := c.syncRepo(context.Background(), do, "o", "r", "", connector.FetchOpts{LastSyncedAt: lastSyncedAt},
		func(connector.Document) error { emitted++; return nil })
	if err != nil {
		t.Fatalf("syncRepo: %v", err)
	}
	if treeCalled {
		t.Fatal("tree API was called for a repo unchanged since the last sync")
	}
	if emitted != 0 {
		t.Fatalf("emitted %d documents, want 0", emitted)
	}
}

func TestSyncRepoWalksChangedRepo(t *testing.T) {
	pushedAt := time.Now().UTC()
	lastSyncedAt := pushedAt.Add(-48 * time.Hour) // synced well before the latest push

	treeCalled := false
	do := func(u string) (*http.Response, error) {
		switch {
		case strings.Contains(u, "/git/trees/"):
			treeCalled = true
			return jsonResp(`{"tree":[]}`), nil
		case strings.HasSuffix(u, "/repos/o/r"):
			return jsonResp(fmt.Sprintf(`{"default_branch":"main","pushed_at":%q}`, pushedAt.Format(time.RFC3339))), nil
		}
		return nil, fmt.Errorf("unexpected request: %s", u)
	}

	c := New()
	if err := c.syncRepo(context.Background(), do, "o", "r", "", connector.FetchOpts{LastSyncedAt: lastSyncedAt},
		func(connector.Document) error { return nil }); err != nil {
		t.Fatalf("syncRepo: %v", err)
	}
	if !treeCalled {
		t.Fatal("tree API was not called for a repo that changed since the last sync")
	}
}

func TestSyncRepoIgnoresSkipWhileResumingFromCheckpoint(t *testing.T) {
	// A checkpoint means the LAST sync attempt didn't finish — "unchanged
	// since last success" is the wrong question in that case, so the walk
	// must proceed regardless of pushed_at.
	pushedAt := time.Now().Add(-48 * time.Hour).UTC()
	lastSyncedAt := pushedAt.Add(time.Hour)

	treeCalled := false
	do := func(u string) (*http.Response, error) {
		switch {
		case strings.Contains(u, "/git/trees/"):
			treeCalled = true
			return jsonResp(`{"tree":[]}`), nil
		case strings.HasSuffix(u, "/repos/o/r"):
			return jsonResp(fmt.Sprintf(`{"default_branch":"main","pushed_at":%q}`, pushedAt.Format(time.RFC3339))), nil
		}
		return nil, fmt.Errorf("unexpected request: %s", u)
	}

	c := New()
	opts := connector.FetchOpts{LastSyncedAt: lastSyncedAt, Checkpoint: []byte(`{"processed_count":3}`)}
	if err := c.syncRepo(context.Background(), do, "o", "r", "", opts, func(connector.Document) error { return nil }); err != nil {
		t.Fatalf("syncRepo: %v", err)
	}
	if !treeCalled {
		t.Fatal("tree API was skipped despite an active checkpoint")
	}
}
