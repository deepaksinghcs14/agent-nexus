package handler

// Integration tests for the workflow engine (executeGroupRun): graph walking,
// condition routing, parallel/join, loop iteration, and the integration nodes
// (tool, webhook). Reuses the loop_test.go harness — real Postgres, scripted
// fake provider for agent nodes.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"errors"

	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/google/uuid"
)

type wfFixture struct {
	*loopFixture
	workflowID string
}

// wfNodeSpec/wfEdgeSpec describe a graph using short keys resolved to UUIDs.
type wfNodeSpec struct {
	key, typ string
	agent    bool // attach the fixture agent to this node
	config   map[string]any
}
type wfEdgeSpec struct{ from, to, label string }

func newWorkflowFixture(t *testing.T, turns []fakeTurn, nodes []wfNodeSpec, edges []wfEdgeSpec) *wfFixture {
	t.Helper()
	fx := &wfFixture{loopFixture: newLoopFixture(t, turns, ""), workflowID: uuid.NewString()}
	ctx := context.Background()

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := fx.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("workflow fixture insert failed: %v\n%s", err, sql)
		}
	}
	mustExec(`INSERT INTO workflows(id,workspace_id,name,created_by) VALUES($1::uuid,$2::uuid,$3,$4::uuid)`,
		fx.workflowID, fx.ws, "wf-test-"+fx.workflowID[:8], fx.uid)

	ids := map[string]string{}
	for _, n := range nodes {
		id := uuid.NewString()
		ids[n.key] = id
		cfg := n.config
		if cfg == nil {
			cfg = map[string]any{}
		}
		cfgJSON, _ := json.Marshal(cfg)
		if n.agent {
			mustExec(`INSERT INTO workflow_nodes(id,workflow_id,node_type,agent_id,position_x,position_y,config) VALUES($1::uuid,$2::uuid,$3,$4::uuid,0,0,$5::jsonb)`,
				id, fx.workflowID, n.typ, fx.agent.ID, string(cfgJSON))
		} else {
			mustExec(`INSERT INTO workflow_nodes(id,workflow_id,node_type,position_x,position_y,config) VALUES($1::uuid,$2::uuid,$3,0,0,$4::jsonb)`,
				id, fx.workflowID, n.typ, string(cfgJSON))
		}
	}
	for _, e := range edges {
		if e.label != "" {
			mustExec(`INSERT INTO workflow_edges(id,workflow_id,source_node_id,target_node_id,label) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5)`,
				uuid.NewString(), fx.workflowID, ids[e.from], ids[e.to], e.label)
		} else {
			mustExec(`INSERT INTO workflow_edges(id,workflow_id,source_node_id,target_node_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid)`,
				uuid.NewString(), fx.workflowID, ids[e.from], ids[e.to])
		}
	}
	return fx
}

func (fx *wfFixture) runWorkflow() {
	fx.h.executeGroupRun(context.Background(), fx.workflowID, fx.ws, fx.uid, fx.runID, fx.convID, "test input", nil, fx.emit)
}

func TestWorkflowLinear(t *testing.T) {
	fx := newWorkflowFixture(t,
		[]fakeTurn{{deltas: []string{"agent says hi"}}},
		[]wfNodeSpec{
			{key: "s", typ: "start"},
			{key: "a", typ: "agent", agent: true},
			{key: "e", typ: "end"},
		},
		[]wfEdgeSpec{{from: "s", to: "a"}, {from: "a", to: "e"}},
	)
	fx.runWorkflow()

	status, output := fx.runRow(t)
	if status != "success" || output != "agent says hi" {
		t.Fatalf("workflow run = %q/%q, want success/agent says hi", status, output)
	}
	if !containsInOrder(fx.eventTypes(), []string{"node_started", "node_completed", "run_completed"}) {
		t.Fatalf("event order wrong: %v", fx.eventTypes())
	}
	// Checkpoint must be cleaned up on success.
	var cp int
	_ = fx.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM workflow_checkpoints WHERE run_id=$1::uuid`, fx.runID).Scan(&cp)
	if cp != 0 {
		t.Fatalf("checkpoint rows after success = %d, want 0", cp)
	}
}

func TestWorkflowConditionRouting(t *testing.T) {
	fx := newWorkflowFixture(t,
		[]fakeTurn{{deltas: []string{"verdict: YES"}}, {deltas: []string{"yes branch ran"}}},
		[]wfNodeSpec{
			{key: "s", typ: "start"},
			{key: "classify", typ: "agent", agent: true},
			{key: "cond", typ: "condition", config: map[string]any{"expression": "contains:YES"}},
			{key: "yes", typ: "agent", agent: true},
			{key: "no", typ: "agent", agent: true},
			{key: "e", typ: "end"},
		},
		[]wfEdgeSpec{
			{from: "s", to: "classify"},
			{from: "classify", to: "cond"},
			{from: "cond", to: "yes", label: "yes"},
			{from: "cond", to: "no", label: "no"},
			{from: "yes", to: "e"},
			{from: "no", to: "e"},
		},
	)
	fx.runWorkflow()

	status, output := fx.runRow(t)
	if status != "success" || output != "yes branch ran" {
		t.Fatalf("workflow run = %q/%q, want success/yes branch ran", status, output)
	}
	routed := fx.firstEvent("node_routed")
	if routed == nil || routed["result"] != "yes" {
		t.Fatalf("condition did not route yes: %v", routed)
	}
	// Only classify + yes agents ran — two provider calls, not three.
	if n := len(fx.fake.recorded()); n != 2 {
		t.Fatalf("provider calls = %d, want 2 (no-branch agent must not run)", n)
	}
}

func TestWorkflowParallelJoin(t *testing.T) {
	fx := newWorkflowFixture(t,
		[]fakeTurn{{deltas: []string{"branch output A"}}, {deltas: []string{"branch output B"}}},
		[]wfNodeSpec{
			{key: "s", typ: "start"},
			{key: "par", typ: "parallel"},
			{key: "a", typ: "agent", agent: true},
			{key: "b", typ: "agent", agent: true},
			{key: "join", typ: "join"},
			{key: "e", typ: "end"},
		},
		[]wfEdgeSpec{
			{from: "s", to: "par"},
			{from: "par", to: "a"},
			{from: "par", to: "b"},
			{from: "a", to: "join"},
			{from: "b", to: "join"},
			{from: "join", to: "e"},
		},
	)
	fx.runWorkflow()

	status, output := fx.runRow(t)
	if status != "success" {
		t.Fatalf("workflow status = %q, want success", status)
	}
	// Join concatenates both branch outputs (order is scheduling-dependent).
	if !strings.Contains(output, "branch output") || !strings.Contains(output, "---") {
		t.Fatalf("join output does not contain concatenated branches: %q", output)
	}
	if n := len(fx.fake.recorded()); n != 2 {
		t.Fatalf("provider calls = %d, want 2", n)
	}
}

func TestWorkflowLoopNode(t *testing.T) {
	fx := newWorkflowFixture(t,
		[]fakeTurn{{deltas: []string{"still working"}}, {deltas: []string{"all done"}}},
		[]wfNodeSpec{
			{key: "s", typ: "start"},
			{key: "a", typ: "agent", agent: true},
			{key: "loop", typ: "loop", config: map[string]any{"exit_condition": "contains:done", "max_iterations": 5}},
			{key: "e", typ: "end"},
		},
		[]wfEdgeSpec{
			{from: "s", to: "a"},
			{from: "a", to: "loop"},
			{from: "loop", to: "a", label: "loop"},
			{from: "loop", to: "e", label: "exit"},
		},
	)
	fx.runWorkflow()

	status, output := fx.runRow(t)
	if status != "success" || output != "all done" {
		t.Fatalf("workflow run = %q/%q, want success/all done", status, output)
	}
	// Agent ran twice: first output failed the exit condition, second passed.
	if n := len(fx.fake.recorded()); n != 2 {
		t.Fatalf("provider calls = %d, want 2 (one loop-back)", n)
	}
}

// The workflow tool node type was removed; legacy graphs that still contain
// one hit the walk's default case, which reports an explicit error event and
// passes the input through unchanged so the rest of the workflow still runs.
func TestWorkflowLegacyToolNodePassesThrough(t *testing.T) {
	fx := newWorkflowFixture(t, nil,
		[]wfNodeSpec{
			{key: "s", typ: "start"},
			{key: "tool", typ: "tool", config: map[string]any{
				"tool_name": "wf_upper",
				"args":      map[string]any{"text": "{{input}}"},
			}},
			{key: "e", typ: "end"},
		},
		[]wfEdgeSpec{{from: "s", to: "tool"}, {from: "tool", to: "e"}},
	)
	fx.runWorkflow()

	status, output := fx.runRow(t)
	if status != "success" || output != "test input" {
		t.Fatalf("workflow run = %q/%q, want success with the input passed through", status, output)
	}
	// No LLM involved.
	if n := len(fx.fake.recorded()); n != 0 {
		t.Fatalf("provider calls = %d, want 0", n)
	}
}

func TestWorkflowWebhookNodeAndEndDelivery(t *testing.T) {
	var received []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = append(received, string(body))
		fmt.Fprint(w, `{"ack":true}`)
	}))
	defer srv.Close()

	fx := newWorkflowFixture(t, nil,
		[]wfNodeSpec{
			{key: "s", typ: "start"},
			{key: "hook", typ: "webhook", config: map[string]any{"url": srv.URL}},
			{key: "e", typ: "end", config: map[string]any{"webhook_url": srv.URL}},
		},
		[]wfEdgeSpec{{from: "s", to: "hook"}, {from: "hook", to: "e"}},
	)
	fx.runWorkflow()

	status, output := fx.runRow(t)
	if status != "success" || !strings.Contains(output, `"ack":true`) {
		t.Fatalf("workflow run = %q/%q, want success with webhook ack", status, output)
	}
	// Mid-flow node + end-node delivery = two POSTs, first carrying the input.
	if len(received) != 2 {
		t.Fatalf("webhook POSTs = %d, want 2", len(received))
	}
	if !strings.Contains(received[0], "test input") {
		t.Fatalf("webhook payload missing input: %s", received[0])
	}
	delivery := fx.firstEvent("node_delivery")
	if delivery == nil || delivery["ok"] != true {
		t.Fatalf("node_delivery not emitted ok: %v", delivery)
	}
}

// SaveGraph/GetGraph round trip through the WorkflowRepository: stable node
// ids across saves, edge replacement, and the unknown-edge sentinel.
func TestWorkflowGraphSaveRoundTrip(t *testing.T) {
	fx := newWorkflowFixture(t, nil, []wfNodeSpec{}, []wfEdgeSpec{})
	repo := repository.NewWorkflowRepository(fx.pool)
	ctx := context.Background()

	startID, agentID := uuid.NewString(), uuid.NewString()
	nodes := []repository.WorkflowGraphNode{
		{ID: startID, NodeType: "start", Config: json.RawMessage(`{}`)},
		{ID: agentID, NodeType: "agent", AgentID: fx.agent.ID, PositionX: 100, Config: json.RawMessage(`{"label":"A"}`)},
	}
	edges := []repository.WorkflowGraphEdge{{SourceNodeID: startID, TargetNodeID: agentID}}
	if err := repo.SaveGraph(ctx, fx.workflowID, nodes, edges); err != nil {
		t.Fatalf("first save: %v", err)
	}

	// Re-save with moved node: id must be stable, position updated.
	nodes[1].PositionX = 300
	if err := repo.SaveGraph(ctx, fx.workflowID, nodes, edges); err != nil {
		t.Fatalf("second save: %v", err)
	}
	gotNodes, gotEdges, err := repo.GetGraph(ctx, fx.workflowID)
	if err != nil || len(gotNodes) != 2 || len(gotEdges) != 1 {
		t.Fatalf("graph = %d nodes / %d edges (err %v), want 2/1", len(gotNodes), len(gotEdges), err)
	}
	var moved bool
	for _, n := range gotNodes {
		if n.ID == agentID && n.PositionX == 300 && n.AgentID == fx.agent.ID {
			moved = true
		}
	}
	if !moved {
		t.Fatalf("agent node id not stable or position not updated: %+v", gotNodes)
	}

	// Edge referencing an unknown node must return the typed sentinel.
	badEdges := []repository.WorkflowGraphEdge{{SourceNodeID: startID, TargetNodeID: "ghost"}}
	if err := repo.SaveGraph(ctx, fx.workflowID, nodes, badEdges); !errors.Is(err, repository.ErrUnknownEdgeNode) {
		t.Fatalf("bad edge err = %v, want ErrUnknownEdgeNode", err)
	}
}

// walkBranch is a closure — its "return" only unwinds the current branch, not
// the whole walk — so a failed node used to fall straight through to the
// unguarded success write after the top-level walkBranch call. A parallel
// graph is the only shape that exercises both halves of the fix: the
// loop-top abort (stops the sibling branch and the node after the join) and
// the post-walk guard (stops the success write itself).
func TestWorkflowFailedNodeFailsRunAndStopsWalk(t *testing.T) {
	fx := newWorkflowFixture(t,
		// Two empty turns per agent node: completeWithEmptyRetry retries once
		// on an empty reply, then fails the sub-run. A 5th provider call means
		// the "after" node ran on an already-failed workflow.
		[]fakeTurn{{}, {}, {}, {}},
		[]wfNodeSpec{
			{key: "s", typ: "start"},
			{key: "par", typ: "parallel"},
			{key: "a", typ: "agent", agent: true},
			{key: "b", typ: "agent", agent: true},
			{key: "join", typ: "join"},
			{key: "after", typ: "agent", agent: true},
			{key: "e", typ: "end"},
		},
		[]wfEdgeSpec{
			{from: "s", to: "par"}, {from: "par", to: "a"}, {from: "par", to: "b"},
			{from: "a", to: "join"}, {from: "b", to: "join"},
			{from: "join", to: "after"}, {from: "after", to: "e"},
		},
	)
	fx.runWorkflow()

	status, _ := fx.runRow(t)
	if status != "failed" {
		t.Fatalf("workflow status = %q, want failed", status)
	}
	if n := len(fx.fake.recorded()); n != 4 {
		t.Fatalf("provider calls = %d, want 4 (node after join must not run)", n)
	}
	// The failing node's reason must survive — not be clobbered by the defer's
	// generic "workflow terminated unexpectedly".
	var errMsg string
	_ = fx.pool.QueryRow(context.Background(),
		`SELECT COALESCE(error_message,'') FROM runs WHERE id=$1::uuid`, fx.runID).Scan(&errMsg)
	if !strings.Contains(errMsg, "empty response") {
		t.Fatalf("error_message = %q, want the failing node's reason", errMsg)
	}
}

// The same fallthrough exists on the pre-walk paths (load nodes/edges, no
// start node) — they call failRun and return from executeGroupRun directly,
// but without the runMarked flag the terminal defer overwrote their specific
// reason with "workflow terminated unexpectedly".
func TestWorkflowNoStartNodePreservesErrorMessage(t *testing.T) {
	fx := newWorkflowFixture(t, nil, []wfNodeSpec{}, []wfEdgeSpec{})
	fx.runWorkflow()

	status, _ := fx.runRow(t)
	if status != "failed" {
		t.Fatalf("workflow status = %q, want failed", status)
	}
	var errMsg string
	_ = fx.pool.QueryRow(context.Background(),
		`SELECT COALESCE(error_message,'') FROM runs WHERE id=$1::uuid`, fx.runID).Scan(&errMsg)
	if errMsg != "workflow has no start node" {
		t.Fatalf("error_message = %q, want %q", errMsg, "workflow has no start node")
	}
}
