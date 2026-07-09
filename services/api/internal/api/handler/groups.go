package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

type WorkflowsHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
	repo *repository.WorkflowRepository
}

func NewWorkflowsHandler(p *pgxpool.Pool, c *config.Config) *WorkflowsHandler {
	return &WorkflowsHandler{pool: p, cfg: c, repo: repository.NewWorkflowRepository(p)}
}

type groupRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Mode        string   `json:"mode"`
	Status      string   `json:"status"`
	AgentIDs    []string `json:"agent_ids"`
}

func (h *WorkflowsHandler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.repo.List(r.Context(), middleware.WorkspaceIDFromCtx(r.Context()))
	if err != nil {
		errs.Write(w, errs.Internal("failed to list workflows"))
		return
	}
	errs.WriteJSON(w, 200, map[string]any{"data": list})
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
	wf := &repository.WorkflowSummary{
		ID:          id,
		WorkspaceID: middleware.WorkspaceIDFromCtx(r.Context()),
		Name:        q.Name,
		Description: q.Description,
		Mode:        q.Mode,
		Status:      q.Status,
		AgentIDs:    q.AgentIDs,
		CreatedBy:   middleware.UserIDFromCtx(r.Context()),
	}
	if err := h.repo.Save(r.Context(), wf, create); err != nil {
		errs.Write(w, errs.Internal("failed to save workflow"))
		return
	}
	saved, _ := h.repo.Get(r.Context(), id, wf.WorkspaceID)
	status := 200
	if create {
		status = 201
	}
	errs.WriteJSON(w, status, saved)
}

func (h *WorkflowsHandler) Create(w http.ResponseWriter, r *http.Request) {
	h.save(w, r, uuid.NewString(), true)
}

func (h *WorkflowsHandler) Get(w http.ResponseWriter, r *http.Request) {
	wf, err := h.repo.Get(r.Context(), chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context()))
	if err != nil {
		errs.Write(w, errs.NotFound("workflow not found"))
		return
	}
	errs.WriteJSON(w, 200, wf)
}

func (h *WorkflowsHandler) Update(w http.ResponseWriter, r *http.Request) {
	h.save(w, r, chi.URLParam(r, "id"), false)
}

func (h *WorkflowsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	found, err := h.repo.Delete(r.Context(), chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context()))
	if err != nil {
		errs.Write(w, errs.Internal("failed to delete workflow"))
		return
	}
	if !found {
		errs.Write(w, errs.NotFound("workflow not found"))
		return
	}
	w.WriteHeader(204)
}

func (h *WorkflowsHandler) Run(w http.ResponseWriter, r *http.Request) {
	wf, err := h.repo.Get(r.Context(), chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context()))
	if err != nil {
		errs.Write(w, errs.NotFound("workflow not found"))
		return
	}
	errs.WriteJSON(w, 202, map[string]any{"workflow": wf, "status": "accepted", "message": "workflow run queued"})
}

// ─────────────────────────────────────────────────────────────────────────────
// Workflow graph endpoints
// ─────────────────────────────────────────────────────────────────────────────

// saveGraphRequest is the body accepted by SaveGraph.
type saveGraphRequest struct {
	Nodes []saveGraphNode `json:"nodes"`
	Edges []saveGraphEdge `json:"edges"`
}

type saveGraphNode struct {
	// ID is the client-side identifier used to cross-reference edges.
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
func (h *WorkflowsHandler) GetGraph(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "id")
	if !h.repo.Exists(r.Context(), workflowID, middleware.WorkspaceIDFromCtx(r.Context())) {
		errs.Write(w, errs.NotFound("workflow not found"))
		return
	}
	nodes, edges, err := h.repo.GetGraph(r.Context(), workflowID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to load graph"))
		return
	}
	errs.WriteJSON(w, 200, map[string]any{"nodes": nodes, "edges": edges})
}

// SaveGraph handles PUT /api/v1/workflows/{id}/graph. Validation warnings are
// non-blocking: the save succeeds and the warnings ride along in the response
// so builders learn about a broken graph immediately instead of at run time.
func (h *WorkflowsHandler) SaveGraph(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "id")
	ws := middleware.WorkspaceIDFromCtx(r.Context())

	var req saveGraphRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if !h.repo.Exists(r.Context(), workflowID, ws) {
		errs.Write(w, errs.NotFound("workflow not found"))
		return
	}

	warnings := validateWorkflowGraph(req.Nodes, req.Edges)

	nodes := make([]repository.WorkflowGraphNode, 0, len(req.Nodes))
	for _, n := range req.Nodes {
		agentID := ""
		if n.AgentID != nil {
			agentID = *n.AgentID
		}
		nodes = append(nodes, repository.WorkflowGraphNode{
			ID: n.ID, NodeType: n.NodeType, AgentID: agentID,
			PositionX: n.PositionX, PositionY: n.PositionY, Config: n.Config,
		})
	}
	edges := make([]repository.WorkflowGraphEdge, 0, len(req.Edges))
	for _, e := range req.Edges {
		edges = append(edges, repository.WorkflowGraphEdge{
			SourceNodeID: e.SourceNodeID, TargetNodeID: e.TargetNodeID, Label: e.Label,
		})
	}

	if err := h.repo.SaveGraph(r.Context(), workflowID, nodes, edges); err != nil {
		if errors.Is(err, repository.ErrUnknownEdgeNode) {
			errs.Write(w, errs.BadRequest(err.Error()))
			return
		}
		errs.Write(w, errs.Internal("failed to save graph"))
		return
	}
	errs.WriteJSON(w, 200, map[string]any{"ok": true, "warnings": warnings})
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
