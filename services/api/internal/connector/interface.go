package connector

import (
	"context"
	"encoding/json"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
)

// Document is the normalised output from any connector provider.
type Document struct {
	Source           string
	SourceDocumentID string
	Title            string
	URL              string
	Author           string
	Content          string
	Metadata         map[string]any
}

// Provider fetches all documents in a single blocking call.
// Implement StreamProvider instead for large sources that benefit from checkpointing.
type Provider interface {
	Fetch(ctx context.Context, cfg map[string]any) ([]Document, error)
}

// FetchOpts carries resume state passed to StreamProvider.FetchStream.
type FetchOpts struct {
	// Checkpoint is the saved state from a previous interrupted/failed run.
	// Nil or '{}' on the first run — providers must treat both as "start from scratch".
	Checkpoint json.RawMessage

	// SaveCheckpoint persists provider-specific state so sync can resume after a crash.
	// v must be JSON-serialisable. Call after each logical batch (e.g. a space page).
	// May be nil for providers that don't support checkpointing.
	SaveCheckpoint func(v any)

	// LastSyncedAt is when this connector last completed successfully — zero
	// value on the first sync or if unknown. Unlike Checkpoint (crash-resume
	// state, cleared on every successful completion) this persists across
	// successful syncs, so a provider can compare it against a source's own
	// modified-time signal to skip unchanged content instead of re-fetching
	// everything on every sync.
	LastSyncedAt time.Time
}

// StreamProvider is implemented by providers that emit documents one at a time and support
// checkpoint-based resumption after a pod restart.
//
// The pipeline prefers StreamProvider over Provider when both are implemented.
// emit is called for each document; returning an error from emit stops the fetch immediately.
type StreamProvider interface {
	FetchStream(ctx context.Context, cfg map[string]any, opts FetchOpts, emit func(Document) error) error
}

// Connector wraps domain.Connector with its resolved provider.
type Connector struct {
	Meta     domain.Connector
	Provider Provider
}
