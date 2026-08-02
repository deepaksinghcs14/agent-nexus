package filesystem

// A filesystem connector sync used to re-read every file's bytes on every
// sync. os.DirEntry.Info().ModTime() is already available for free during
// the walk — this exercises the skip-unchanged branch built on it.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/connector"
)

func TestFetchStreamSkipsFilesOlderThanLastSync(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.txt")
	newf := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(old, []byte("unchanged"), 0o644); err != nil {
		t.Fatalf("write old.txt: %v", err)
	}
	if err := os.WriteFile(newf, []byte("changed"), 0o644); err != nil {
		t.Fatalf("write new.txt: %v", err)
	}

	cutoff := time.Now()
	// old.txt was written before cutoff (already true); make sure new.txt's
	// mtime is unambiguously after it, since a fast filesystem can otherwise
	// give both files the same timestamp resolution.
	future := cutoff.Add(time.Hour)
	if err := os.Chtimes(newf, future, future); err != nil {
		t.Fatalf("chtimes new.txt: %v", err)
	}

	c := New()
	var emitted []string
	err := c.FetchStream(context.Background(), map[string]any{"path": dir},
		connector.FetchOpts{LastSyncedAt: cutoff},
		func(doc connector.Document) error { emitted = append(emitted, doc.SourceDocumentID); return nil })
	if err != nil {
		t.Fatalf("FetchStream: %v", err)
	}
	if len(emitted) != 1 || emitted[0] != "new.txt" {
		t.Fatalf("emitted = %v, want only new.txt", emitted)
	}
}

func TestFetchStreamEmitsEverythingOnFirstSync(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatalf("write b.txt: %v", err)
	}

	c := New()
	var emitted []string
	// LastSyncedAt zero value = first sync, no skip.
	err := c.FetchStream(context.Background(), map[string]any{"path": dir}, connector.FetchOpts{},
		func(doc connector.Document) error { emitted = append(emitted, doc.SourceDocumentID); return nil })
	if err != nil {
		t.Fatalf("FetchStream: %v", err)
	}
	if len(emitted) != 2 {
		t.Fatalf("emitted = %v, want 2 files on a first sync with no LastSyncedAt", emitted)
	}
}
