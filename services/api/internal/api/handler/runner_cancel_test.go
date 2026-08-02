package handler

// Cancel() used to stop only the API side: the run row flipped to
// 'cancelled' but nothing told the runner to stop the claude subprocess it
// had launched, so a "cancelled" coding session kept burning tokens for up
// to SESSION_TIMEOUT_MIN regardless. This exercises the fix: the runner
// session key persisted in runs.metadata at launch time is what lets Cancel
// find the right session to stop.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
)

func TestCancelNotifiesRunnerSession(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tn := newTenant(t, pool)

	var mu sync.Mutex
	var gotPath, gotSecret, gotBody string
	runner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.Path
		gotSecret = r.Header.Get("X-Runner-Secret")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"cancelled":true}`)) //nolint:errcheck
	}))
	defer runner.Close()

	agentID, runID, convID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture insert failed: %v\n%s", err, sql)
		}
	}
	mustExec(`INSERT INTO agents(id,workspace_id,name,instructions,provider,model,created_by)
	          VALUES($1::uuid,$2::uuid,'Cancel Test Agent','x','anthropic','claude-test',$3::uuid)`,
		agentID, tn.wsID, tn.userID)
	mustExec(`INSERT INTO conversations(id,workspace_id,agent_id,user_id,title) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'t')`,
		convID, tn.wsID, agentID, tn.userID)
	meta, _ := json.Marshal(map[string]string{"runner_session_key": "T-1|owner/repo"})
	mustExec(`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status,metadata)
	          VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,'test','session_wait',$6::jsonb)`,
		runID, tn.wsID, agentID, convID, tn.userID, meta)

	cfg := &config.Config{RunnerURL: runner.URL, RunnerCallbackSecret: "shh"}
	reg := tools.NewRegistry()
	exec := tools.NewExecutor(reg)
	h := NewRunsHandler(pool, cfg, reg, exec)

	req := tn.request(http.MethodPost, "/runs/"+runID+"/cancel", "id", runID, "")
	w := httptest.NewRecorder()
	h.Cancel(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, want 200: %s", w.Code, w.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if gotPath != "/sessions/cancel" {
		t.Fatalf("runner received path %q, want /sessions/cancel", gotPath)
	}
	if gotSecret != "shh" {
		t.Fatalf("runner received secret %q, want shh", gotSecret)
	}
	if gotBody != `{"session_key":"T-1|owner/repo"}` {
		t.Fatalf("runner received body %q", gotBody)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM runs WHERE id=$1::uuid`, runID).Scan(&status); err != nil || status != "cancelled" {
		t.Fatalf("run status = %q (err %v), want cancelled", status, err)
	}
}

func TestCancelSkipsRunnerCallWhenNoSessionKey(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tn := newTenant(t, pool)

	called := false
	runner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer runner.Close()

	agentID, runID, convID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture insert failed: %v\n%s", err, sql)
		}
	}
	mustExec(`INSERT INTO agents(id,workspace_id,name,instructions,provider,model,created_by)
	          VALUES($1::uuid,$2::uuid,'Cancel Test Agent 2','x','anthropic','claude-test',$3::uuid)`,
		agentID, tn.wsID, tn.userID)
	mustExec(`INSERT INTO conversations(id,workspace_id,agent_id,user_id,title) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'t')`,
		convID, tn.wsID, agentID, tn.userID)
	mustExec(`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,'test','running')`,
		runID, tn.wsID, agentID, convID, tn.userID)

	cfg := &config.Config{RunnerURL: runner.URL, RunnerCallbackSecret: "shh"}
	reg := tools.NewRegistry()
	exec := tools.NewExecutor(reg)
	h := NewRunsHandler(pool, cfg, reg, exec)

	req := tn.request(http.MethodPost, "/runs/"+runID+"/cancel", "id", runID, "")
	w := httptest.NewRecorder()
	h.Cancel(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if called {
		t.Fatal("runner was called for a run with no runner_session_key in metadata")
	}
}
