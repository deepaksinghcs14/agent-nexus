package handler

import (
	"fmt"
	"net/http"
	"sync"
)

// safeEmitter serialises SSE writes to an http.ResponseWriter and drops
// emits after the handler returns. Run loops hand their emit func to
// background goroutines that outlive the request — async conversation
// compaction being the standing example — and a write/Flush on the response
// after the handler has returned panics inside net/http; a panic in a bare
// goroutine cannot be recovered by middleware and kills the process. The
// mutex also serialises emits from concurrent workflow parallel branches,
// which previously interleaved writes on the same response unsynchronised.
type safeEmitter struct {
	mu     sync.Mutex
	w      http.ResponseWriter
	f      http.Flusher
	closed bool
}

// newSafeEmitter wraps w; ok is false when w cannot stream (no Flusher).
func newSafeEmitter(w http.ResponseWriter) (*safeEmitter, bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	return &safeEmitter{w: w, f: f}, true
}

// emit writes one SSE data line; a no-op once close has been called.
func (e *safeEmitter) emit(s string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	fmt.Fprintf(e.w, "data: %s\n\n", s)
	e.f.Flush()
}

// close marks the response as gone; call it (defer) before the handler
// returns so stragglers turn into no-ops instead of writes to a dead writer.
func (e *safeEmitter) close() {
	e.mu.Lock()
	e.closed = true
	e.mu.Unlock()
}
