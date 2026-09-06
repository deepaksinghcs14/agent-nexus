package handler

// Integration tests for the shared agent loop (executeRun). They run against
// a real Postgres (schema via migrate.Run) with a scripted fake provider
// injected through RunsHandler.providerOverride, so the loop's actual SQL,
// tool dispatch, gating, and SSE emission are exercised end to end.
//
// Configure with TEST_DATABASE_URL. Without it the tests skip locally but
// fail in CI (CI must never silently skip the loop suite).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/migrate"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools/native"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── fake provider ────────────────────────────────────────────────────────────

type fakeTurn struct {
	deltas    []string
	toolCalls []provider.ToolCall
}

// fakeProvider replays scripted turns and records every request it receives.
type fakeProvider struct {
	mu       sync.Mutex
	turns    []fakeTurn
	requests []provider.CompletionRequest
}

func (f *fakeProvider) Complete(ctx context.Context, req provider.CompletionRequest) (<-chan provider.CompletionEvent, error) {
	// Real providers abort on a cancelled ctx (it flows into their HTTP
	// request); the fake must too, or a cancelled run silently keeps looping
	// in tests that assert the loop actually stopped.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.requests = append(f.requests, req)
	var turn fakeTurn
	if len(f.turns) > 0 {
		turn = f.turns[0]
		f.turns = f.turns[1:]
	} else {
		turn = fakeTurn{deltas: []string{"fallback reply"}}
	}
	f.mu.Unlock()

	ch := make(chan provider.CompletionEvent, len(turn.deltas)+len(turn.toolCalls)+1)
	for _, d := range turn.deltas {
		ch <- provider.CompletionEvent{Type: provider.EventDelta, Delta: d}
	}
	for i := range turn.toolCalls {
		tc := turn.toolCalls[i]
		ch <- provider.CompletionEvent{Type: provider.EventToolCall, ToolCall: &tc}
	}
	ch <- provider.CompletionEvent{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 10, OutputTokens: 5}}
	close(ch)
	return ch, nil
}

func (f *fakeProvider) Embed(context.Context, string) ([]float32, error) { return nil, nil }
func (f *fakeProvider) Models(context.Context) ([]provider.ModelInfo, error) {
	return nil, nil
}
func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) recorded() []provider.CompletionRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]provider.CompletionRequest, len(f.requests))
	copy(out, f.requests)
	return out
}

// ── harness ──────────────────────────────────────────────────────────────────

var (
	loopPoolOnce sync.Once
	loopPool     *pgxpool.Pool
	loopPoolErr  error
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI — the loop suite must not be skipped")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping loop integration tests")
	}
	loopPoolOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		loopPool, loopPoolErr = pgxpool.New(ctx, dsn)
		if loopPoolErr != nil {
			return
		}
		if loopPoolErr = loopPool.Ping(ctx); loopPoolErr != nil {
			return
		}
		loopPoolErr = migrate.Run(ctx, loopPool)
	})
	if loopPoolErr != nil {
		t.Fatalf("loop test database not usable: %v", loopPoolErr)
	}
	return loopPool
}

type loopFixture struct {
	h       *InvokeHandler
	fake    *fakeProvider
	pool    *pgxpool.Pool
	ws, uid string
	agent   *domain.Agent
	convID  string
	runID   string
	events  []map[string]any
	mu      sync.Mutex
}

// emit collects every SSE event as parsed JSON for ordered assertions.
func (fx *loopFixture) emit(s string) {
	var m map[string]any
	if json.Unmarshal([]byte(s), &m) != nil {
		m = map[string]any{"type": "unparsed", "raw": s}
	}
	fx.mu.Lock()
	fx.events = append(fx.events, m)
	fx.mu.Unlock()
}

func (fx *loopFixture) eventTypes() []string {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	out := make([]string, 0, len(fx.events))
	for _, e := range fx.events {
		t, _ := e["type"].(string)
		out = append(out, t)
	}
	return out
}

func (fx *loopFixture) firstEvent(typ string) map[string]any {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	for _, e := range fx.events {
		if e["type"] == typ {
			return e
		}
	}
	return nil
}

// newLoopFixture provisions an isolated workspace/user/agent/conversation/run
// and a handler wired to the fake provider.
func newLoopFixture(t *testing.T, turns []fakeTurn, agentPatch string) *loopFixture {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	fx := &loopFixture{pool: pool, fake: &fakeProvider{turns: turns}}
	fx.uid = uuid.NewString()
	fx.ws = uuid.NewString()
	fx.convID = uuid.NewString()
	fx.runID = uuid.NewString()
	agentID := uuid.NewString()

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture insert failed: %v\n%s", err, sql)
		}
	}
	mustExec(`INSERT INTO users(id,email,password_hash) VALUES($1::uuid,$2,'x')`, fx.uid, fx.uid+"@test.local")
	mustExec(`INSERT INTO workspaces(id,name,owner_id) VALUES($1::uuid,$2,$3::uuid)`, fx.ws, "loop-test-"+fx.ws[:8], fx.uid)
	mustExec(`INSERT INTO agents(id,workspace_id,name,instructions,provider,model,created_by,max_steps) VALUES($1::uuid,$2::uuid,'Loop Test Agent','You are a test agent.','anthropic','claude-test',$3::uuid,10)`, agentID, fx.ws, fx.uid)
	if agentPatch != "" {
		mustExec(`UPDATE agents SET `+agentPatch+` WHERE id=$1::uuid`, agentID)
	}
	mustExec(`INSERT INTO conversations(id,workspace_id,agent_id,user_id,title) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'loop test')`, fx.convID, fx.ws, agentID, fx.uid)
	mustExec(`INSERT INTO messages(id,conversation_id,role,content) VALUES($1::uuid,$2::uuid,'user','test input')`, uuid.NewString(), fx.convID)
	mustExec(`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,'test input','running')`, fx.runID, fx.ws, agentID, fx.convID, fx.uid)

	// 127.0.0.1 allowlisted for the webhook node test's local httptest server —
	// the SSRF guard otherwise blocks it as a loopback address.
	cfg := &config.Config{EncryptionKey: strings.Repeat("k", 32), HTTPToolAllowHosts: []string{"127.0.0.1"}}
	reg := tools.NewRegistry()
	exec := tools.NewExecutor(reg)
	runs := NewRunsHandler(pool, cfg, reg, exec)
	runs.providerOverride = fx.fake
	invoke := NewInvokeHandler(pool, cfg, runs, reg, exec)
	runs.SetInvokeHandler(invoke)
	fx.h = invoke

	var a domain.Agent
	row := pool.QueryRow(ctx, `SELECT id::text, workspace_id::text, name, instructions, provider, model, temperature, max_tokens, memory_enabled, memory_scope, context_retrieval_enabled, COALESCE(agentic_rag,false), max_steps, status, COALESCE(lazy_tool_loading,false), COALESCE(max_history_messages,20), COALESCE(memory_save_mode,'hybrid'), COALESCE(compaction_threshold,0), COALESCE(compaction_token_threshold,0) FROM agents WHERE id=$1::uuid`, agentID)
	if err := row.Scan(&a.ID, &a.WorkspaceID, &a.Name, &a.Instructions, &a.Provider, &a.Model, &a.Temperature, &a.MaxTokens, &a.MemoryEnabled, &a.MemoryScope, &a.ContextRetrievalEnabled, &a.AgenticRAG, &a.MaxSteps, &a.Status, &a.LazyToolLoading, &a.MaxHistoryMessages, &a.MemorySaveMode, &a.CompactionThreshold, &a.CompactionTokenThreshold); err != nil {
		t.Fatalf("load agent: %v", err)
	}
	fx.agent = &a
	return fx
}

// attachCodeTool creates a workspace `code` tool and attaches it to the agent.
func (fx *loopFixture) attachCodeTool(t *testing.T, name, jsCode string, requiresApproval bool) {
	t.Helper()
	toolID := uuid.NewString()
	cfgJSON, _ := json.Marshal(map[string]string{"code": jsCode})
	if _, err := fx.pool.Exec(context.Background(),
		`INSERT INTO tools(id,workspace_id,name,description,type,input_schema,output_schema,config,risk_level,requires_approval,timeout_ms,enabled)
		 VALUES($1::uuid,$2::uuid,$3,'test tool','code','{}','{}',$4,'low',$5,5000,true)`,
		toolID, fx.ws, name, cfgJSON, requiresApproval); err != nil {
		t.Fatalf("create tool: %v", err)
	}
	if _, err := fx.pool.Exec(context.Background(),
		`INSERT INTO agent_tools(agent_id,tool_id,enabled) VALUES($1::uuid,$2::uuid,true)`,
		fx.agent.ID, toolID); err != nil {
		t.Fatalf("attach tool: %v", err)
	}
}

func (fx *loopFixture) run(opts invokeOpts) {
	fx.h.executeRun(context.Background(), fx.agent, fx.ws, fx.uid, fx.runID, fx.convID, "test input", nil, fx.emit, opts)
}

func (fx *loopFixture) runWithDelegates(handlers map[string]func(context.Context, json.RawMessage) string, opts invokeOpts) {
	fx.h.executeRun(context.Background(), fx.agent, fx.ws, fx.uid, fx.runID, fx.convID, "test input", handlers, fx.emit, opts)
}

func (fx *loopFixture) runRow(t *testing.T) (status, output string) {
	t.Helper()
	if err := fx.pool.QueryRow(context.Background(),
		`SELECT status, COALESCE(output,'') FROM runs WHERE id=$1::uuid`, fx.runID).Scan(&status, &output); err != nil {
		t.Fatalf("read run row: %v", err)
	}
	return status, output
}

func containsInOrder(haystack, subseq []string) bool {
	i := 0
	for _, h := range haystack {
		if i < len(subseq) && h == subseq[i] {
			i++
		}
	}
	return i == len(subseq)
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestLoopSimpleReply(t *testing.T) {
	fx := newLoopFixture(t, []fakeTurn{{deltas: []string{"hello ", "world"}}}, "")
	fx.run(invokeOpts{})

	status, output := fx.runRow(t)
	if status != "success" || output != "hello world" {
		t.Fatalf("run row = %q/%q, want success/hello world", status, output)
	}
	if !containsInOrder(fx.eventTypes(), []string{"run_started", "delta", "run_completed"}) {
		t.Fatalf("event order wrong: %v", fx.eventTypes())
	}
	// Assistant message persisted and conversation counters bumped.
	var msgCount int
	if err := fx.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM messages WHERE conversation_id=$1::uuid AND role='assistant' AND content='hello world'`,
		fx.convID).Scan(&msgCount); err != nil || msgCount != 1 {
		t.Fatalf("assistant message rows = %d (err %v), want 1", msgCount, err)
	}
	var convMsgs int
	_ = fx.pool.QueryRow(context.Background(),
		`SELECT message_count FROM conversations WHERE id=$1::uuid`, fx.convID).Scan(&convMsgs)
	if convMsgs == 0 {
		t.Fatal("conversation message_count was not bumped for the assistant turn")
	}
}

func TestLoopToolCallRoundTrip(t *testing.T) {
	call := provider.ToolCall{ID: "c1", Name: "adder", Input: json.RawMessage(`{"a":2,"b":3}`)}
	fx := newLoopFixture(t, []fakeTurn{
		{toolCalls: []provider.ToolCall{call}},
		{deltas: []string{"sum is 5"}},
	}, "")
	fx.attachCodeTool(t, "adder", "function run(input){ return { sum: input.a + input.b } }", false)
	fx.run(invokeOpts{})

	status, output := fx.runRow(t)
	if status != "success" || output != "sum is 5" {
		t.Fatalf("run row = %q/%q, want success/sum is 5", status, output)
	}
	if !containsInOrder(fx.eventTypes(), []string{"run_started", "tool_started", "tool_call", "delta", "run_completed"}) {
		t.Fatalf("event order wrong: %v", fx.eventTypes())
	}
	// Second provider request must carry the tool result back to the model.
	reqs := fx.fake.recorded()
	if len(reqs) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(reqs))
	}
	var sawToolResult bool
	for _, m := range reqs[1].Messages {
		if m.Role == "tool" && m.ToolName == "adder" && strings.Contains(m.Content, `"sum"`) {
			sawToolResult = true
		}
	}
	if !sawToolResult {
		t.Fatalf("tool result missing from second request: %+v", reqs[1].Messages)
	}
	// Tool step persisted on the run.
	var stepCount int
	_ = fx.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM run_steps WHERE run_id=$1::uuid AND tool_name='adder'`, fx.runID).Scan(&stepCount)
	if stepCount != 1 {
		t.Fatalf("adder tool steps = %d, want 1", stepCount)
	}
}

func TestLoopLazyGateRejectsUnrequestedTool(t *testing.T) {
	call := provider.ToolCall{ID: "c1", Name: "ghost_tool", Input: json.RawMessage(`{}`)}
	fx := newLoopFixture(t, []fakeTurn{
		{toolCalls: []provider.ToolCall{call}},
		{deltas: []string{"done"}},
	}, "lazy_tool_loading=true")
	fx.run(invokeOpts{})

	status, _ := fx.runRow(t)
	if status != "success" {
		t.Fatalf("run status = %q, want success", status)
	}
	evt := fx.firstEvent("tool_call")
	if evt == nil {
		t.Fatal("no tool_call event emitted for the gated call")
	}
	out := fmt.Sprintf("%v", evt["output"])
	if !strings.Contains(out, "not active this turn") && !strings.Contains(out, "not found") {
		t.Fatalf("gated tool_call output does not carry the lazy-gate error: %v", out)
	}
}

func TestLoopSupervisorDelegates(t *testing.T) {
	call := provider.ToolCall{ID: "c1", Name: "delegate_researcher", Input: json.RawMessage(`{"task":"dig"}`)}
	fx := newLoopFixture(t, []fakeTurn{
		{toolCalls: []provider.ToolCall{call}},
		{deltas: []string{"synthesized"}},
	}, "")

	var delegateCalled bool
	handlers := map[string]func(context.Context, json.RawMessage) string{
		"delegate_researcher": func(context.Context, json.RawMessage) string {
			delegateCalled = true
			return "research findings"
		},
	}
	fx.runWithDelegates(handlers, invokeOpts{
		delegateToolDefs: []provider.ToolDefinition{
			{Name: "delegate_researcher", Description: "Researches things", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
		noApprovalPark:   true,
		disableUserInput: true,
	})

	if !delegateCalled {
		t.Fatal("delegate handler was never invoked")
	}
	status, output := fx.runRow(t)
	if status != "success" || output != "synthesized" {
		t.Fatalf("run row = %q/%q, want success/synthesized", status, output)
	}
	reqs := fx.fake.recorded()
	if len(reqs) == 0 {
		t.Fatal("no provider requests recorded")
	}
	// Delegate tool offered to the model and team block injected in the prompt.
	var offered bool
	for _, td := range reqs[0].Tools {
		if td.Name == "delegate_researcher" {
			offered = true
		}
	}
	if !offered {
		t.Fatalf("delegate tool not offered to the model: %+v", reqs[0].Tools)
	}
	sys := reqs[0].StableSystemContent
	for _, m := range reqs[0].Messages {
		if m.Role == "system" {
			sys += m.Content
		}
	}
	if !strings.Contains(sys, "Available Team Agents") {
		t.Fatal("team-agent block missing from system prompt")
	}
}

func TestLoopApprovalGateApproved(t *testing.T) {
	call := provider.ToolCall{ID: "c1", Name: "risky", Input: json.RawMessage(`{}`)}
	fx := newLoopFixture(t, []fakeTurn{
		{toolCalls: []provider.ToolCall{call}},
		{deltas: []string{"approved and done"}},
	}, "")
	fx.attachCodeTool(t, "risky", "function run(){ return { ok: true } }", true)

	// Approve as soon as the loop registers the wait.
	go func() {
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if SendApprovalDecision(fx.runID, ApprovalDecision{Decision: "approved"}) {
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
	}()
	fx.run(invokeOpts{})

	status, output := fx.runRow(t)
	if status != "success" || output != "approved and done" {
		t.Fatalf("run row = %q/%q, want success/approved and done", status, output)
	}
	if fx.firstEvent("approval_required") == nil {
		t.Fatalf("approval_required never emitted: %v", fx.eventTypes())
	}
	evt := fx.firstEvent("tool_call")
	if evt == nil || !strings.Contains(fmt.Sprintf("%v", evt["output"]), "ok") {
		t.Fatalf("approved tool did not execute: %v", evt)
	}
}

// Cancel used to be a bare status write: it never stopped the goroutine
// executing the run, and 'user_input_wait' was missing from its filter
// entirely, so a run parked on native_ask_user could not be cancelled at
// all. This exercises the full mechanism — HTTP Cancel handler, the
// runCancels registry, and the ctx.Done arm on the user-input wait.
func TestCancelStopsUserInputWait(t *testing.T) {
	call := provider.ToolCall{ID: "c1", Name: "native_ask_user", Input: json.RawMessage(`{"question":"which repo?"}`)}
	fx := newLoopFixture(t, []fakeTurn{
		{toolCalls: []provider.ToolCall{call}},
		{deltas: []string{"should never run"}},
	}, "")
	// native_ask_user isn't in this fixture's (deliberately empty) registry —
	// register just the one tool this test needs.
	fx.h.registry.Register(native.NewAskUserTool())

	done := make(chan struct{})
	go func() {
		fx.run(invokeOpts{})
		close(done)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if status, _ := fx.runRow(t); status == "user_input_wait" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("run never reached user_input_wait")
		}
		time.Sleep(10 * time.Millisecond)
	}

	tn := tenant{userID: fx.uid, wsID: fx.ws}
	req := tn.request(http.MethodPost, "/runs/"+fx.runID+"/cancel", "id", fx.runID, "")
	w := httptest.NewRecorder()
	fx.h.runs.Cancel(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, want 200: %s", w.Code, w.Body.String())
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("executeRun did not return after cancel — a wait ignored ctx.Done")
	}

	status, _ := fx.runRow(t)
	if status != "cancelled" {
		t.Fatalf("final status = %q, want cancelled", status)
	}
	// One request for the native_ask_user turn; the cancel must stop the loop
	// before it asks the model again for the "should never run" turn.
	if n := len(fx.fake.recorded()); n != 1 {
		t.Fatalf("provider calls = %d, want 1 — the loop kept running after cancel", n)
	}
}
