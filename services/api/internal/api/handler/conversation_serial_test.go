package handler

// executeRun used to launch with no serialisation per conversation at all:
// two runs on the same conversation raced concurrently, tearing message
// history (each loads history before the other's reply lands) and silently
// discarding one run's compaction write. conversationLocks fixes this inside
// executeRun itself, which — unlike a lock placed at gateway dispatch —
// also covers ResumeApprovedRun/ResumeSessionRun re-entering executeRun on
// the same conversation after a park.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/google/uuid"
)

// probeProvider records the peak number of concurrent in-flight Complete
// calls. Each call blocks until either a second call overlaps it (the gate
// channel closes) or a short grace period elapses — so "did two calls
// overlap" is decided by actual concurrency, not by sleep-timing luck.
type probeProvider struct {
	mu       sync.Mutex
	live     int
	peak     int
	gate     chan struct{}
	gateOnce sync.Once
}

func newProbeProvider() *probeProvider { return &probeProvider{gate: make(chan struct{})} }

func (p *probeProvider) Complete(context.Context, provider.CompletionRequest) (<-chan provider.CompletionEvent, error) {
	p.mu.Lock()
	p.live++
	if p.live > p.peak {
		p.peak = p.live
	}
	if p.live >= 2 {
		p.gateOnce.Do(func() { close(p.gate) })
	}
	p.mu.Unlock()

	select {
	case <-p.gate:
	case <-time.After(300 * time.Millisecond):
	}

	p.mu.Lock()
	p.live--
	p.mu.Unlock()

	ch := make(chan provider.CompletionEvent, 2)
	ch <- provider.CompletionEvent{Type: provider.EventDelta, Delta: "ok"}
	ch <- provider.CompletionEvent{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 1, OutputTokens: 1}}
	close(ch)
	return ch, nil
}

func (p *probeProvider) Embed(context.Context, string) ([]float32, error)     { return nil, nil }
func (p *probeProvider) Models(context.Context) ([]provider.ModelInfo, error) { return nil, nil }
func (p *probeProvider) Name() string                                        { return "probe" }

// insertRun adds a second run row directly (bypassing dispatch, which isn't
// under test here) so both goroutines below drive the real executeRun.
func insertRun(t *testing.T, fx *loopFixture, runID, convID, input string) {
	t.Helper()
	if _, err := fx.pool.Exec(context.Background(),
		`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,'running')`,
		runID, fx.ws, fx.agent.ID, convID, fx.uid, input); err != nil {
		t.Fatalf("insert run: %v", err)
	}
}

func TestExecuteRunSerializesSameConversation(t *testing.T) {
	fx := newLoopFixture(t, nil, "")
	probe := newProbeProvider()
	fx.h.runs.providerOverride = probe

	runID2 := uuid.NewString()
	insertRun(t, fx, runID2, fx.convID, "test input 2")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		fx.h.executeRun(context.Background(), fx.agent, fx.ws, fx.uid, fx.runID, fx.convID, "test input", nil, nil, invokeOpts{})
	}()
	go func() {
		defer wg.Done()
		fx.h.executeRun(context.Background(), fx.agent, fx.ws, fx.uid, runID2, fx.convID, "test input 2", nil, nil, invokeOpts{})
	}()
	wg.Wait()

	if probe.peak != 1 {
		t.Fatalf("peak concurrent model calls on one conversation = %d, want 1 (executeRun did not serialise)", probe.peak)
	}
}

func TestExecuteRunAllowsDifferentConversationsConcurrently(t *testing.T) {
	fx := newLoopFixture(t, nil, "")
	probe := newProbeProvider()
	fx.h.runs.providerOverride = probe

	conv2 := uuid.NewString()
	runID2 := uuid.NewString()
	if _, err := fx.pool.Exec(context.Background(),
		`INSERT INTO conversations(id,workspace_id,agent_id,user_id,title) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'loop test 2')`,
		conv2, fx.ws, fx.agent.ID, fx.uid); err != nil {
		t.Fatalf("insert second conversation: %v", err)
	}
	if _, err := fx.pool.Exec(context.Background(),
		`INSERT INTO messages(id,conversation_id,role,content) VALUES($1::uuid,$2::uuid,'user','test input 2')`,
		uuid.NewString(), conv2); err != nil {
		t.Fatalf("insert second conversation message: %v", err)
	}
	insertRun(t, fx, runID2, conv2, "test input 2")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		fx.h.executeRun(context.Background(), fx.agent, fx.ws, fx.uid, fx.runID, fx.convID, "test input", nil, nil, invokeOpts{})
	}()
	go func() {
		defer wg.Done()
		fx.h.executeRun(context.Background(), fx.agent, fx.ws, fx.uid, runID2, conv2, "test input 2", nil, nil, invokeOpts{})
	}()
	wg.Wait()

	// A regression that "simplifies" the per-conversation lock into a single
	// process-wide mutex would pass the serialisation test above but fail
	// here: two unrelated conversations must still run concurrently.
	if probe.peak != 2 {
		t.Fatalf("peak concurrent model calls on two conversations = %d, want 2 (lock is not per-conversation)", probe.peak)
	}
}
