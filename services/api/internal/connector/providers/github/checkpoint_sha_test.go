package github

// The resume checkpoint used to be purely positional (ProcessedCount, an
// offset into a freshly re-fetched tree listing) — fetched by branch NAME,
// not a pinned SHA. Commits landing between a crash and its resume shift
// the tree's blob order, so "skip the first N blobs of the new listing"
// stops meaning "skip the ones already processed": files get silently
// skipped or reprocessed. These tests exercise the tree-SHA guard that
// invalidates a stale offset instead of trusting it blindly.

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/deepaksingh/agent-nexus/services/api/internal/connector"
)

func contentResp(text string) *http.Response {
	body := fmt.Sprintf(`{"content":%q,"encoding":"base64"}`, base64.StdEncoding.EncodeToString([]byte(text)))
	return jsonResp(body)
}

func TestSyncRepoRestartsWhenTreeSHAChanged(t *testing.T) {
	do := func(u string) (*http.Response, error) {
		switch {
		case strings.Contains(u, "/git/trees/"):
			return jsonResp(`{"sha":"tree-new","tree":[
				{"path":"a.txt","type":"blob","size":5},
				{"path":"b.txt","type":"blob","size":5}
			]}`), nil
		case strings.Contains(u, "/contents/a.txt"):
			return contentResp("A"), nil
		case strings.Contains(u, "/contents/b.txt"):
			return contentResp("B"), nil
		}
		return nil, fmt.Errorf("unexpected request: %s", u)
	}

	c := New()
	var emitted []string
	// Checkpoint claims blob 0 (a.txt) is already processed, but under a
	// DIFFERENT tree SHA — simulating a commit that landed between crash
	// and resume.
	opts := connector.FetchOpts{Checkpoint: []byte(`{"processed_count":1,"tree_sha":"tree-old"}`)}
	if err := c.syncRepo(context.Background(), do, "o", "r", "main", opts,
		func(doc connector.Document) error { emitted = append(emitted, doc.SourceDocumentID); return nil }); err != nil {
		t.Fatalf("syncRepo: %v", err)
	}
	if len(emitted) != 2 {
		t.Fatalf("emitted %d docs, want 2 (a stale tree_sha must restart from 0, not trust the offset): %v", len(emitted), emitted)
	}
}

func TestSyncRepoTrustsOffsetWhenTreeSHAMatches(t *testing.T) {
	do := func(u string) (*http.Response, error) {
		switch {
		case strings.Contains(u, "/git/trees/"):
			return jsonResp(`{"sha":"tree-same","tree":[
				{"path":"a.txt","type":"blob","size":5},
				{"path":"b.txt","type":"blob","size":5}
			]}`), nil
		case strings.Contains(u, "/contents/b.txt"):
			return contentResp("B"), nil
		}
		return nil, fmt.Errorf("unexpected request (a.txt should have been skipped): %s", u)
	}

	c := New()
	var emitted []string
	opts := connector.FetchOpts{Checkpoint: []byte(`{"processed_count":1,"tree_sha":"tree-same"}`)}
	if err := c.syncRepo(context.Background(), do, "o", "r", "main", opts,
		func(doc connector.Document) error { emitted = append(emitted, doc.SourceDocumentID); return nil }); err != nil {
		t.Fatalf("syncRepo: %v", err)
	}
	if len(emitted) != 1 || !strings.HasSuffix(emitted[0], "b.txt") {
		t.Fatalf("emitted = %v, want only b.txt (a.txt already processed under the same tree)", emitted)
	}
}
