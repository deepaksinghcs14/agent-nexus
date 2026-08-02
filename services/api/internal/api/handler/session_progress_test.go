package handler

// The runner used to emit no intermediate signal at all — one terminal
// callback after minutes to hours with nothing in between. SessionProgress
// is the debounced, best-effort update the runner now posts periodically;
// it writes into runs.metadata, the same column Cancel reads to find the
// runner's session key.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
)

func TestSessionProgressPersistsToRunMetadata(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tn := newTenant(t, pool)

	agentID, runID, convID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture insert failed: %v\n%s", err, sql)
		}
	}
	mustExec(`INSERT INTO agents(id,workspace_id,name,instructions,provider,model,created_by)
	          VALUES($1::uuid,$2::uuid,'Progress Test Agent','x','anthropic','claude-test',$3::uuid)`,
		agentID, tn.wsID, tn.userID)
	mustExec(`INSERT INTO conversations(id,workspace_id,agent_id,user_id,title) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'t')`,
		convID, tn.wsID, agentID, tn.userID)
	mustExec(`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,'test','session_wait')`,
		runID, tn.wsID, agentID, convID, tn.userID)

	cfg := &config.Config{RunnerCallbackSecret: "shh"}
	reg := tools.NewRegistry()
	exec := tools.NewExecutor(reg)
	runs := NewRunsHandler(pool, cfg, reg, exec)
	h := NewInvokeHandler(pool, cfg, runs, reg, exec)

	body, _ := json.Marshal(map[string]string{"run_id": runID, "summary": "using native_read_file"})
	req := httptest.NewRequest(http.MethodPost, "/internal/sessions/progress", bytes.NewReader(body))
	req.Header.Set("X-Runner-Secret", "shh")
	w := httptest.NewRecorder()
	h.SessionProgress(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT metadata FROM runs WHERE id=$1::uuid`, runID).Scan(&raw); err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	var meta struct {
		SessionProgress string `json:"session_progress"`
	}
	if json.Unmarshal(raw, &meta) != nil || meta.SessionProgress != "using native_read_file" {
		t.Fatalf("metadata = %s, want session_progress=using native_read_file", raw)
	}
}

func TestSessionProgressRejectsWrongSecret(t *testing.T) {
	pool := testPool(t)
	cfg := &config.Config{RunnerCallbackSecret: "shh"}
	reg := tools.NewRegistry()
	exec := tools.NewExecutor(reg)
	runs := NewRunsHandler(pool, cfg, reg, exec)
	h := NewInvokeHandler(pool, cfg, runs, reg, exec)

	body, _ := json.Marshal(map[string]string{"run_id": uuid.NewString(), "summary": "x"})
	req := httptest.NewRequest(http.MethodPost, "/internal/sessions/progress", bytes.NewReader(body))
	req.Header.Set("X-Runner-Secret", "wrong")
	w := httptest.NewRecorder()
	h.SessionProgress(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestSessionProgressDoesNotResurrectCancelledRun(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tn := newTenant(t, pool)

	agentID, runID, convID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture insert failed: %v\n%s", err, sql)
		}
	}
	mustExec(`INSERT INTO agents(id,workspace_id,name,instructions,provider,model,created_by)
	          VALUES($1::uuid,$2::uuid,'Progress Test Agent 2','x','anthropic','claude-test',$3::uuid)`,
		agentID, tn.wsID, tn.userID)
	mustExec(`INSERT INTO conversations(id,workspace_id,agent_id,user_id,title) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'t')`,
		convID, tn.wsID, agentID, tn.userID)
	mustExec(`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,'test','cancelled')`,
		runID, tn.wsID, agentID, convID, tn.userID)

	cfg := &config.Config{RunnerCallbackSecret: "shh"}
	reg := tools.NewRegistry()
	exec := tools.NewExecutor(reg)
	runs := NewRunsHandler(pool, cfg, reg, exec)
	h := NewInvokeHandler(pool, cfg, runs, reg, exec)

	body, _ := json.Marshal(map[string]string{"run_id": runID, "summary": "using native_read_file"})
	req := httptest.NewRequest(http.MethodPost, "/internal/sessions/progress", bytes.NewReader(body))
	req.Header.Set("X-Runner-Secret", "shh")
	w := httptest.NewRecorder()
	h.SessionProgress(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (still accepted, just a no-op write)", w.Code)
	}
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT metadata FROM runs WHERE id=$1::uuid`, runID).Scan(&raw); err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if string(raw) != "{}" {
		t.Fatalf("metadata = %s, want unchanged {} — a cancelled run's metadata must not be written", raw)
	}
}
