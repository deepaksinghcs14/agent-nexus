package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

type WorkflowsHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewWorkflowsHandler(p *pgxpool.Pool, c *config.Config) *WorkflowsHandler {
	return &WorkflowsHandler{p, c}
}

type groupRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Mode        string   `json:"mode"`
	Status      string   `json:"status"`
	AgentIDs    []string `json:"agent_ids"`
}

func (h *WorkflowsHandler) workflow(r *http.Request, id string) (map[string]any, error) {
	var gID, ws, name, desc, mode, status, createdBy string
	var created, updated any
	e := h.pool.QueryRow(r.Context(), `SELECT id::text,workspace_id::text,name,description,mode,status,created_by::text,created_at,updated_at FROM workflows WHERE id=$1::uuid AND workspace_id=$2::uuid`, id, middleware.WorkspaceIDFromCtx(r.Context())).Scan(&gID, &ws, &name, &desc, &mode, &status, &createdBy, &created, &updated)
	if e != nil {
		return nil, e
	}
	rows, e := h.pool.Query(r.Context(), `SELECT agent_id::text FROM workflow_members WHERE group_id=$1::uuid ORDER BY position`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var x string
		_ = rows.Scan(&x)
		ids = append(ids, x)
	}
	return map[string]any{"id": gID, "workspace_id": ws, "name": name, "description": desc, "mode": mode, "status": status, "agent_ids": ids, "created_by": createdBy, "created_at": created, "updated_at": updated}, nil
}
func (h *WorkflowsHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, e := h.pool.Query(r.Context(), `SELECT id::text FROM workflows WHERE workspace_id=$1::uuid ORDER BY created_at DESC`, middleware.WorkspaceIDFromCtx(r.Context()))
	if e != nil {
		errs.Write(w, errs.Internal("failed to list workflows"))
		return
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	a := []map[string]any{}
	for _, id := range ids {
		wf, e := h.workflow(r, id)
		if e == nil {
			a = append(a, wf)
		}
	}
	errs.WriteJSON(w, 200, map[string]any{"data": a})
}
func (h *WorkflowsHandler) save(w http.ResponseWriter, r *http.Request, id string, create bool) {
	var q groupRequest
	if json.NewDecoder(r.Body).Decode(&q) != nil || q.Name == "" {
		errs.Write(w, errs.BadRequest("name is required"))
		return
	}
	if q.Mode == "" {
		q.Mode = "pipeline"
	}
	if q.Status == "" {
		q.Status = "active"
	}
	tx, e := h.pool.Begin(r.Context())
	if e != nil {
		errs.Write(w, errs.Internal("failed to save workflow"))
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if create {
		_, e = tx.Exec(r.Context(), `INSERT INTO workflows(id,workspace_id,name,description,mode,status,created_by)VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7::uuid)`, id, middleware.WorkspaceIDFromCtx(r.Context()), q.Name, q.Description, q.Mode, q.Status, middleware.UserIDFromCtx(r.Context()))
	} else {
		_, e = tx.Exec(r.Context(), `UPDATE workflows SET name=$3,description=$4,mode=$5,status=$6,updated_at=NOW() WHERE id=$1::uuid AND workspace_id=$2::uuid`, id, middleware.WorkspaceIDFromCtx(r.Context()), q.Name, q.Description, q.Mode, q.Status)
		if e == nil {
			_, e = tx.Exec(r.Context(), `DELETE FROM workflow_members WHERE group_id=$1::uuid`, id)
		}
	}
	for i, a := range q.AgentIDs {
		if e != nil {
			break
		}
		_, e = tx.Exec(r.Context(), `INSERT INTO workflow_members(group_id,agent_id,position,role)VALUES($1::uuid,$2::uuid,$3,$4)`, id, a, i, map[bool]string{true: "supervisor", false: "member"}[q.Mode == "supervisor" && i == 0])
	}
	if e != nil || tx.Commit(r.Context()) != nil {
		errs.Write(w, errs.Internal("failed to save workflow"))
		return
	}
	wf, _ := h.workflow(r, id)
	errs.WriteJSON(w, map[bool]int{true: 201, false: 200}[create], wf)
}
func (h *WorkflowsHandler) Create(w http.ResponseWriter, r *http.Request) {
	h.save(w, r, uuid.NewString(), true)
}
func (h *WorkflowsHandler) Get(w http.ResponseWriter, r *http.Request) {
	wf, e := h.workflow(r, chi.URLParam(r, "id"))
	if e != nil {
		errs.Write(w, errs.NotFound("workflow not found"))
		return
	}
	errs.WriteJSON(w, 200, wf)
}
func (h *WorkflowsHandler) Update(w http.ResponseWriter, r *http.Request) {
	h.save(w, r, chi.URLParam(r, "id"), false)
}
func (h *WorkflowsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	t, e := h.pool.Exec(r.Context(), `DELETE FROM workflows WHERE id=$1::uuid AND workspace_id=$2::uuid`, chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context()))
	if e != nil {
		errs.Write(w, errs.Internal("failed to delete workflow"))
		return
	}
	if t.RowsAffected() == 0 {
		errs.Write(w, errs.NotFound("workflow not found"))
		return
	}
	w.WriteHeader(204)
}
func (h *WorkflowsHandler) Run(w http.ResponseWriter, r *http.Request) {
	wf, e := h.workflow(r, chi.URLParam(r, "id"))
	if e != nil {
		errs.Write(w, errs.NotFound("workflow not found"))
		return
	}
	errs.WriteJSON(w, 202, map[string]any{"workflow": wf, "status": "accepted", "message": "workflow run queued"})
}

// ─────────────────────────────────────────────────────────────────────────────
// Workflow graph types (local to this handler, not stored in domain package
// because they are only used by the graph endpoints and executeGroupRun).
// ─────────────────────────────────────────────────────────────────────────────

// WorkflowNode represents a single node in the visual workflow graph.
type WorkflowNode struct {
	ID        string          `json:"id"`
	NodeType  string          `json:"node_type"`
	AgentID   string          `json:"agent_id,omitempty"`
	PositionX float64         `json:"position_x"`
	PositionY float64         `json:"position_y"`
	Config    json.RawMessage `json:"config"`
}

// WorkflowEdge represents a directed connection between two nodes.
type WorkflowEdge struct {
	ID           string `json:"id"`
	SourceNodeID string `json:"source_node_id"`
	TargetNodeID string `json:"target_node_id"`
	Label        string `json:"label,omitempty"`
}

// saveGraphRequest is the body accepted by SaveGraph.
type saveGraphRequest struct {
	Nodes []saveGraphNode `json:"nodes"`
	Edges []saveGraphEdge `json:"edges"`
}

type saveGraphNode struct {
	// ID is the client-side identifier used to cross-reference edges.
	// The server replaces it with a new UUID on every save.
	ID        string          `json:"id"`
	NodeType  string          `json:"node_type"`
	AgentID   *string         `json:"agent_id"`
	PositionX float64         `json:"position_x"`
	PositionY float64         `json:"position_y"`
	Config    json.RawMessage `json:"config"`
}

type saveGraphEdge struct {
	SourceNodeID string `json:"source_node_id"`
	TargetNodeID string `json:"target_node_id"`
	Label        string `json:"label"`
}

// GetGraph handles GET /api/v1/workflows/{id}/graph
// Returns all nodes and edges for the workflow.
func (h *WorkflowsHandler) GetGraph(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "id")
	ws := middleware.WorkspaceIDFromCtx(r.Context())

	// Verify ownership
	var exists bool
	if err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM workflows WHERE id=$1::uuid AND workspace_id=$2::uuid)`,
		workflowID, ws).Scan(&exists); err != nil || !exists {
		errs.Write(w, errs.NotFound("workflow not found"))
		return
	}

	// Load nodes
	nodeRows, err := h.pool.Query(r.Context(),
		`SELECT id::text, node_type, COALESCE(agent_id::text,''), position_x, position_y, config::text
		 FROM workflow_nodes WHERE workflow_id=$1::uuid ORDER BY created_at`,
		workflowID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to load graph nodes"))
		return
	}
	defer nodeRows.Close()

	nodes := []WorkflowNode{}
	for nodeRows.Next() {
		var n WorkflowNode
		var configStr string
		if err := nodeRows.Scan(&n.ID, &n.NodeType, &n.AgentID, &n.PositionX, &n.PositionY, &configStr); err != nil {
			errs.Write(w, errs.Internal("failed to scan graph node"))
			return
		}
		n.Config = json.RawMessage(configStr)
		nodes = append(nodes, n)
	}
	if err := nodeRows.Err(); err != nil {
		errs.Write(w, errs.Internal("failed to read graph nodes"))
		return
	}

	// Load edges
	edgeRows, err := h.pool.Query(r.Context(),
		`SELECT id::text, source_node_id::text, target_node_id::text, COALESCE(label,'')
		 FROM workflow_edges WHERE workflow_id=$1::uuid ORDER BY created_at`,
		workflowID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to load graph edges"))
		return
	}
	defer edgeRows.Close()

	edges := []WorkflowEdge{}
	for edgeRows.Next() {
		var e WorkflowEdge
		if err := edgeRows.Scan(&e.ID, &e.SourceNodeID, &e.TargetNodeID, &e.Label); err != nil {
			errs.Write(w, errs.Internal("failed to scan graph edge"))
			return
		}
		edges = append(edges, e)
	}
	if err := edgeRows.Err(); err != nil {
		errs.Write(w, errs.Internal("failed to read graph edges"))
		return
	}

	errs.WriteJSON(w, 200, map[string]any{"nodes": nodes, "edges": edges})
}

// validateWorkflowGraph runs non-blocking sanity checks on a graph before
// it's saved. Returned as "warnings" in the SaveGraph response — the save
// still succeeds — so builders learn about a broken graph immediately
// instead of only discovering it the first time the workflow is run.
func validateWorkflowGraph(nodes []saveGraphNode, edges []saveGraphEdge) []string {
	var warnings []string

	adj := map[string][]saveGraphEdge{}
	for _, e := range edges {
		adj[e.SourceNodeID] = append(adj[e.SourceNodeID], e)
	}

	var startID string
	hasStart := false
	for _, n := range nodes {
		if n.NodeType == "start" {
			hasStart = true
			startID = n.ID
			break
		}
	}
	if !hasStart {
		warnings = append(warnings, "workflow has no start node")
	}

	for _, n := range nodes {
		switch n.NodeType {
		case "agent", "supervisor":
			if n.AgentID == nil || *n.AgentID == "" {
				warnings = append(warnings, fmt.Sprintf("%s node %q has no agent assigned", n.NodeType, n.ID))
			}
		case "condition":
			hasFallback := false
			for _, e := range adj[n.ID] {
				if e.Label == "no" || e.Label == "false" || e.Label == "" || e.Label == "*" {
					hasFallback = true
					break
				}
			}
			if !hasFallback {
				warnings = append(warnings, fmt.Sprintf("condition node %q has no fallback edge (\"no\") — it can silently stop at runtime", n.ID))
			}
		case "loop":
			hasLoopEdge := false
			for _, e := range adj[n.ID] {
				if e.Label == "loop" {
					hasLoopEdge = true
					break
				}
			}
			if !hasLoopEdge {
				warnings = append(warnings, fmt.Sprintf("loop node %q has no loop-back edge", n.ID))
			}
			var cfg map[string]any
			_ = json.Unmarshal(n.Config, &cfg)
			if _, ok := cfg["max_iterations"]; !ok {
				warnings = append(warnings, fmt.Sprintf("loop node %q has no max_iterations set", n.ID))
			}
		case "tool":
			var cfg map[string]any
			_ = json.Unmarshal(n.Config, &cfg)
			if name, _ := cfg["tool_name"].(string); name == "" {
				warnings = append(warnings, fmt.Sprintf("tool node %q has no tool selected", n.ID))
			}
		case "webhook":
			var cfg map[string]any
			_ = json.Unmarshal(n.Config, &cfg)
			if u, _ := cfg["url"].(string); u == "" {
				warnings = append(warnings, fmt.Sprintf("webhook node %q has no URL configured", n.ID))
			}
		case "gateway":
			var cfg map[string]any
			_ = json.Unmarshal(n.Config, &cfg)
			chID, _ := cfg["channel_id"].(string)
			peerID, _ := cfg["peer_id"].(string)
			if chID == "" || peerID == "" {
				warnings = append(warnings, fmt.Sprintf("gateway node %q needs a channel and recipient configured", n.ID))
			}
		}
	}

	if hasStart {
		reachable := map[string]bool{startID: true}
		queue := []string{startID}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, e := range adj[cur] {
				if !reachable[e.TargetNodeID] {
					reachable[e.TargetNodeID] = true
					queue = append(queue, e.TargetNodeID)
				}
			}
		}
		for _, n := range nodes {
			if !reachable[n.ID] {
				warnings = append(warnings, fmt.Sprintf("%s node %q is unreachable from the start node", n.NodeType, n.ID))
			}
		}
	}

	return warnings
}

// SaveGraph handles PUT /api/v1/workflows/{id}/graph
// Upserts nodes by their client-supplied id (the frontend always sends valid
// UUIDs — crypto.randomUUID() for new nodes, the server id for existing
// ones) instead of deleting and reinserting with freshly minted ids. This
// keeps node identity stable across saves, so canvas selection state isn't
// lost and SSE node_id events from a run always match what's on screen.
// Edges have no identity the frontend relies on, so they're fully replaced.
func (h *WorkflowsHandler) SaveGraph(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "id")
	ws := middleware.WorkspaceIDFromCtx(r.Context())

	var req saveGraphRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}

	// Verify workspace ownership
	var exists bool
	if err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM workflows WHERE id=$1::uuid AND workspace_id=$2::uuid)`,
		workflowID, ws).Scan(&exists); err != nil || !exists {
		errs.Write(w, errs.NotFound("workflow not found"))
		return
	}

	warnings := validateWorkflowGraph(req.Nodes, req.Edges)

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		errs.Write(w, errs.Internal("failed to start transaction"))
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	// Resolve each incoming node to the id it will be stored under: reuse the
	// client-supplied id when it's a valid UUID, otherwise mint one.
	resolvedID := make(map[string]string, len(req.Nodes))
	nodeIDs := make([]string, 0, len(req.Nodes))
	for _, n := range req.Nodes {
		id := n.ID
		if _, perr := uuid.Parse(id); perr != nil {
			id = uuid.NewString()
		}
		resolvedID[n.ID] = id
		nodeIDs = append(nodeIDs, id)
	}

	// Remove nodes that existed for this workflow but are absent from the
	// incoming set — cascades to their edges.
	if _, err := tx.Exec(r.Context(),
		`DELETE FROM workflow_nodes WHERE workflow_id=$1::uuid AND NOT (id = ANY($2::uuid[]))`,
		workflowID, nodeIDs); err != nil {
		errs.Write(w, errs.Internal("failed to prune removed nodes"))
		return
	}

	// Upsert each node by its resolved id. The WHERE guard on the UPDATE
	// branch means a spoofed id that already belongs to a different workflow
	// silently fails to update (and was never eligible for INSERT, since the
	// id already exists) rather than hijacking another workflow's node.
	for _, n := range req.Nodes {
		id := resolvedID[n.ID]
		configJSON := n.Config
		if len(configJSON) == 0 {
			configJSON = json.RawMessage(`{}`)
		}

		if n.AgentID != nil && *n.AgentID != "" {
			_, err = tx.Exec(r.Context(),
				`INSERT INTO workflow_nodes(id,workflow_id,node_type,agent_id,position_x,position_y,config)
				 VALUES($1::uuid,$2::uuid,$3,$4::uuid,$5,$6,$7::jsonb)
				 ON CONFLICT (id) DO UPDATE SET
				   node_type=EXCLUDED.node_type, agent_id=EXCLUDED.agent_id,
				   position_x=EXCLUDED.position_x, position_y=EXCLUDED.position_y, config=EXCLUDED.config
				 WHERE workflow_nodes.workflow_id = EXCLUDED.workflow_id`,
				id, workflowID, n.NodeType, *n.AgentID, n.PositionX, n.PositionY, string(configJSON))
		} else {
			_, err = tx.Exec(r.Context(),
				`INSERT INTO workflow_nodes(id,workflow_id,node_type,position_x,position_y,config)
				 VALUES($1::uuid,$2::uuid,$3,$4,$5,$6::jsonb)
				 ON CONFLICT (id) DO UPDATE SET
				   node_type=EXCLUDED.node_type, agent_id=NULL,
				   position_x=EXCLUDED.position_x, position_y=EXCLUDED.position_y, config=EXCLUDED.config
				 WHERE workflow_nodes.workflow_id = EXCLUDED.workflow_id`,
				id, workflowID, n.NodeType, n.PositionX, n.PositionY, string(configJSON))
		}
		if err != nil {
			errs.Write(w, errs.Internal("failed to save node"))
			return
		}
	}

	// Edges: fully replace using the resolved (stable) node ids.
	if _, err := tx.Exec(r.Context(),
		`DELETE FROM workflow_edges WHERE workflow_id=$1::uuid`, workflowID); err != nil {
		errs.Write(w, errs.Internal("failed to clear existing edges"))
		return
	}
	for _, e := range req.Edges {
		srcID, srcOK := resolvedID[e.SourceNodeID]
		tgtID, tgtOK := resolvedID[e.TargetNodeID]
		if !srcOK || !tgtOK {
			errs.Write(w, errs.BadRequest("edge references unknown node id"))
			return
		}
		label := e.Label // may be empty
		var insertErr error
		if label != "" {
			_, insertErr = tx.Exec(r.Context(),
				`INSERT INTO workflow_edges(id,workflow_id,source_node_id,target_node_id,label)
				 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5)`,
				uuid.NewString(), workflowID, srcID, tgtID, label)
		} else {
			_, insertErr = tx.Exec(r.Context(),
				`INSERT INTO workflow_edges(id,workflow_id,source_node_id,target_node_id)
				 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid)`,
				uuid.NewString(), workflowID, srcID, tgtID)
		}
		if insertErr != nil {
			errs.Write(w, errs.Internal("failed to insert edge"))
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		errs.Write(w, errs.Internal("failed to commit graph"))
		return
	}

	errs.WriteJSON(w, 200, map[string]any{"ok": true, "warnings": warnings})
}
