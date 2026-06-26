// Package syncstate provides live progress tracking and checkpoint persistence for connector syncs.
// It is the single source of truth for sync job state during a running sync — all DB writes
// (progress, checkpoint, final status) go through the Reporter so the handler goroutine stays simple.
package syncstate

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	flushEveryDocs = 10              // flush to DB after every N documents processed
	flushInterval  = 5 * time.Second // or after this much wall time, whichever comes first
)

// Reporter wraps a connector_sync_jobs row and provides:
//   - Debounced live progress writes (documents_found / documents_indexed)
//   - Checkpoint persistence (provider-specific resume state stored in JSONB)
//   - Final status marking (Complete / Fail)
type Reporter struct {
	jobID       string
	connectorID string
	pool        *pgxpool.Pool
	logger      *slog.Logger

	mu         sync.Mutex
	found      int
	indexed    int
	checkpoint json.RawMessage
	lastFlush  time.Time
	dirtyCount int // docs processed since last flush
}

// New creates a Reporter for an already-inserted sync job row.
func New(pool *pgxpool.Pool, jobID, connectorID string) *Reporter {
	return &Reporter{
		jobID:       jobID,
		connectorID: connectorID,
		pool:        pool,
		logger:      slog.With("connector_id", connectorID, "job_id", jobID),
		checkpoint:  json.RawMessage(`{}`),
		lastFlush:   time.Now(),
	}
}

// Logger returns a structured logger pre-tagged with connector_id and job_id.
func (r *Reporter) Logger() *slog.Logger { return r.logger }

// Inc increments the found/indexed counters and flushes to DB if the debounce threshold is hit.
// Call once per document: Inc(1, 1) when indexed, Inc(1, 0) when skipped (unchanged hash).
func (r *Reporter) Inc(found, indexed int) {
	r.mu.Lock()
	r.found += found
	r.indexed += indexed
	r.dirtyCount += found
	shouldFlush := r.dirtyCount >= flushEveryDocs || time.Since(r.lastFlush) >= flushInterval
	r.mu.Unlock()
	if shouldFlush {
		r.flush()
	}
}

// Checkpoint saves provider-specific state to DB immediately so a pod restart can resume.
// v must be JSON-serialisable. Call after processing each logical batch (e.g. a space page).
func (r *Reporter) Checkpoint(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		r.logger.Warn("syncstate: checkpoint marshal failed", "error", err)
		return
	}
	r.mu.Lock()
	r.checkpoint = b
	r.mu.Unlock()
	r.flush() // always flush on checkpoint — this is the crash-safety boundary
}

// Found returns the current found count (thread-safe).
func (r *Reporter) Found() int { r.mu.Lock(); defer r.mu.Unlock(); return r.found }

// Indexed returns the current indexed count (thread-safe).
func (r *Reporter) Indexed() int { r.mu.Lock(); defer r.mu.Unlock(); return r.indexed }

// Complete marks the job as successfully completed with final counts.
// The checkpoint is cleared on success — next sync starts fresh.
func (r *Reporter) Complete() {
	r.mu.Lock()
	found, indexed := r.found, r.indexed
	r.mu.Unlock()
	r.logger.Info("sync completed", "documents_found", found, "documents_indexed", indexed)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r.pool.Exec(ctx, //nolint:errcheck
		`UPDATE connector_sync_jobs SET status='completed', completed_at=NOW(), documents_found=$2, documents_indexed=$3, checkpoint='{}' WHERE id=$1::uuid`,
		r.jobID, found, indexed,
	)
	r.pool.Exec(ctx, `UPDATE connectors SET status='connected', updated_at=NOW() WHERE id=$1::uuid`, r.connectorID) //nolint:errcheck
}

// Fail marks the job as failed. The checkpoint is preserved so the user can re-trigger
// and the pipeline will resume from where it left off.
func (r *Reporter) Fail(err error) {
	r.mu.Lock()
	found, indexed, cp := r.found, r.indexed, r.checkpoint
	r.mu.Unlock()
	r.logger.Error("sync failed", "error", err, "documents_found", found, "documents_indexed", indexed)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r.pool.Exec(ctx, //nolint:errcheck
		`UPDATE connector_sync_jobs SET status='failed', completed_at=NOW(), documents_found=$2, documents_indexed=$3, error_message=$4, checkpoint=$5 WHERE id=$1::uuid`,
		r.jobID, found, indexed, err.Error(), cp,
	)
	r.pool.Exec(ctx, `UPDATE connectors SET status='error', updated_at=NOW() WHERE id=$1::uuid`, r.connectorID) //nolint:errcheck
}

// LoadLastCheckpoint returns the checkpoint saved by the most recent interrupted or failed
// sync job for this connector. Returns nil (not '{}') if none exists.
// Callers should treat a nil or empty checkpoint as "start from the beginning".
func LoadLastCheckpoint(ctx context.Context, pool *pgxpool.Pool, connectorID string) json.RawMessage {
	var cp json.RawMessage
	_ = pool.QueryRow(ctx,
		`SELECT checkpoint FROM connector_sync_jobs
		 WHERE connector_id=$1::uuid AND status IN ('interrupted','failed') AND checkpoint != '{}'::jsonb
		 ORDER BY created_at DESC LIMIT 1`,
		connectorID,
	).Scan(&cp)
	return cp
}

// flush writes current progress and checkpoint to the DB (best-effort, non-blocking on error).
func (r *Reporter) flush() {
	r.mu.Lock()
	found, indexed, cp := r.found, r.indexed, r.checkpoint
	r.lastFlush = time.Now()
	r.dirtyCount = 0
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r.pool.Exec(ctx, //nolint:errcheck
		`UPDATE connector_sync_jobs SET documents_found=$2, documents_indexed=$3, checkpoint=$4 WHERE id=$1::uuid`,
		r.jobID, found, indexed, cp,
	)
}
