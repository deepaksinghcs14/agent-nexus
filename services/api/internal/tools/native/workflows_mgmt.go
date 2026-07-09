package native

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── native_list_workflows ─────────────────────────────────────────────────────

type ListWorkflowsTool struct {
	tools.RequiresRunContext
	pool *pgxpool.Pool
}

func NewListWorkflowsTool(pool *pgxpool.Pool) *ListWorkflowsTool {
	return &ListWorkflowsTool{pool: pool}
}

func (t *ListWorkflowsTool) Definition() domain.Tool {
	return domain.Tool{
		Name:             "native_list_workflows",
		Description:      "List all active workflows in the workspace.",
		Type:             "native",
		InputSchema:      json.RawMessage(`{"type":"object","properties":{}}`),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}
func (t *ListWorkflowsTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, _ map[string]any) (any, error) {
	rows, err := t.pool.Query(ctx,
		`SELECT id::text, name, COALESCE(description,''), mode FROM workflows
		 WHERE workspace_id=$1::uuid AND status='active' ORDER BY name`,
		execCtx.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type entry struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Mode        string `json:"mode"`
	}
	var out []entry
	for rows.Next() {
		var e entry
		if rows.Scan(&e.ID, &e.Name, &e.Description, &e.Mode) == nil {
			out = append(out, e)
		}
	}
	if out == nil {
		out = []entry{}
	}
	return map[string]any{"workflows": out, "count": len(out)}, nil
}

// ── native_create_workflow ────────────────────────────────────────────────────

type CreateWorkflowTool struct {
	tools.RequiresRunContext
	pool *pgxpool.Pool
}

func NewCreateWorkflowTool(pool *pgxpool.Pool) *CreateWorkflowTool {
	return &CreateWorkflowTool{pool: pool}
}

func (t *CreateWorkflowTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":        map[string]any{"type": "string", "description": "Workflow name."},
			"description": map[string]any{"type": "string", "description": "Short description of what this workflow does."},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"pipeline", "supervisor"},
				"description": "Execution mode: 'pipeline' runs agents sequentially, 'supervisor' uses a supervisor agent to coordinate.",
			},
			"agent_ids": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Agent UUIDs to add as workflow members. Required: provide at least one agent for pipeline mode and at least two agents for supervisor mode.",
			},
			"ephemeral": map[string]any{"type": "boolean", "description": "Default true (auto-deletes at run end). Set false at creation time, or call native_promote_resource after creation once you know it's worth keeping."},
		},
		"required": []string{"name", "mode", "agent_ids"},
	})
	return domain.Tool{
		Name:             "native_create_workflow",
		Description:      "Create a new workflow. Workflows are temporary by default and auto-delete when this run ends unless ephemeral=false is explicitly provided. mode must be 'pipeline' or 'supervisor'. Provide agent_ids for the agents that should execute in the workflow.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        10000,
		Enabled:          true,
	}
}
func (t *CreateWorkflowTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	name, _ := input["name"].(string)
	description, _ := input["description"].(string)
	mode, _ := input["mode"].(string)
	ephemeral := ephemeralFromInput(input)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if mode != "pipeline" && mode != "supervisor" {
		return nil, fmt.Errorf("mode must be 'pipeline' or 'supervisor'")
	}

	var agentIDs []string
	if ids, ok := input["agent_ids"].([]any); ok {
		for _, v := range ids {
			if s, ok := v.(string); ok && s != "" {
				agentIDs = append(agentIDs, s)
			}
		}
	}
	if len(agentIDs) == 0 {
		return nil, fmt.Errorf("agent_ids is required and must include at least one agent")
	}
	if mode == "supervisor" && len(agentIDs) < 2 {
		return nil, fmt.Errorf("supervisor workflows require at least two agent_ids: first supervisor, then one or more member agents")
	}

	workflowID := uuid.NewString()
	_, err := t.pool.Exec(ctx, `
		INSERT INTO workflows(id, workspace_id, name, description, mode, status, created_by, source_run_id, ephemeral)
		VALUES($1::uuid, $2::uuid, $3, $4, $5, 'active', $6::uuid,
		  CASE WHEN $7='' THEN NULL ELSE $7::uuid END, $8)`,
		workflowID, execCtx.WorkspaceID, name, description, mode,
		execCtx.UserID, execCtx.RunID, ephemeral)
	if err != nil {
		return nil, fmt.Errorf("create workflow: %w", err)
	}

	// Insert workflow_members and build workflow_nodes + workflow_edges so the
	// visual canvas has something to render immediately.
	nodeIDs := make([]string, len(agentIDs))
	for i, agentID := range agentIDs {
		role := "member"
		if mode == "supervisor" && i == 0 {
			role = "supervisor"
		}
		t.pool.Exec(ctx, //nolint:errcheck
			`INSERT INTO workflow_members(id, group_id, agent_id, position, role)
			 VALUES($1::uuid, $2::uuid, $3::uuid, $4, $5)
			 ON CONFLICT DO NOTHING`,
			uuid.NewString(), workflowID, agentID, i, role)

		nodeID := uuid.NewString()
		nodeIDs[i] = nodeID
		agentIDPtr := agentID

		var posX, posY float64
		if mode == "pipeline" {
			posX = float64(i) * 280
			posY = 100
		} else {
			// supervisor: first node (supervisor) centred at top, members fan out below
			if i == 0 {
				posX = float64(max(len(agentIDs)-1, 0)) * 140
				posY = 0
			} else {
				posX = float64(i-1) * 280
				posY = 180
			}
		}

		t.pool.Exec(ctx, //nolint:errcheck
			`INSERT INTO workflow_nodes(id, workflow_id, node_type, agent_id, position_x, position_y, config)
			 VALUES($1::uuid, $2::uuid, 'agent', $3::uuid, $4, $5, '{}')`,
			nodeID, workflowID, agentIDPtr, posX, posY)
	}

	// Create edges
	if mode == "pipeline" {
		for i := 0; i < len(nodeIDs)-1; i++ {
			t.pool.Exec(ctx, //nolint:errcheck
				`INSERT INTO workflow_edges(id, workflow_id, source_node_id, target_node_id, label)
				 VALUES($1::uuid, $2::uuid, $3::uuid, $4::uuid, '')`,
				uuid.NewString(), workflowID, nodeIDs[i], nodeIDs[i+1])
		}
	} else if mode == "supervisor" && len(nodeIDs) > 1 {
		// supervisor → each member
		for i := 1; i < len(nodeIDs); i++ {
			t.pool.Exec(ctx, //nolint:errcheck
				`INSERT INTO workflow_edges(id, workflow_id, source_node_id, target_node_id, label)
				 VALUES($1::uuid, $2::uuid, $3::uuid, $4::uuid, '')`,
				uuid.NewString(), workflowID, nodeIDs[0], nodeIDs[i])
		}
	}

	return map[string]any{
		"workflow_id":  workflowID,
		"name":         name,
		"mode":         mode,
		"member_count": len(agentIDs),
		"ephemeral":    ephemeral,
	}, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── native_save_workflow_graph ────────────────────────────────────────────────

type SaveWorkflowGraphTool struct {
	tools.RequiresRunContext
	pool *pgxpool.Pool
}

func NewSaveWorkflowGraphTool(pool *pgxpool.Pool) *SaveWorkflowGraphTool {
	return &SaveWorkflowGraphTool{pool: pool}
}

func (t *SaveWorkflowGraphTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workflow_id": map[string]any{"type": "string", "description": "Workflow UUID returned by native_create_workflow."},
			"nodes": map[string]any{
				"type":        "array",
				"description": "Complete list of workflow graph nodes. Always include one start node and one end node.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":         map[string]any{"type": "string", "description": "Client-side node ID used by edges, e.g. start, research, condition_1."},
						"node_type":  map[string]any{"type": "string", "enum": []string{"start", "end", "agent", "supervisor", "condition", "parallel", "join", "loop", "tool", "webhook", "gateway"}},
						"agent_id":   map[string]any{"type": "string", "description": "Required for agent and supervisor nodes."},
						"position_x": map[string]any{"type": "number", "description": "Canvas X position."},
						"position_y": map[string]any{"type": "number", "description": "Canvas Y position."},
						"config":     map[string]any{"type": "object", "description": "Node config: condition uses {expression}; loop uses {exit_condition,max_iterations}; tool uses {tool_name,args} (approval-gated tools are refused at run time); webhook uses {url,method,headers,payload_template}; gateway uses {channel_id,peer_id,peer_kind,message_template}; end optionally delivers via {webhook_url} and/or {gateway_channel_id,gateway_peer_id}; label is optional for display. String values in tool args and templates may use {{input}} and {{original_input}}."},
					},
					"required": []string{"id", "node_type", "position_x", "position_y", "config"},
				},
			},
			"edges": map[string]any{
				"type":        "array",
				"description": "Directed edges using client-side node IDs. Use labels yes/no for conditions, loop/exit for loops, delegate for supervisor team edges.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"source_node_id": map[string]any{"type": "string"},
						"target_node_id": map[string]any{"type": "string"},
						"label":          map[string]any{"type": "string", "description": "Optional edge label."},
					},
					"required": []string{"source_node_id", "target_node_id"},
				},
			},
		},
		"required": []string{"workflow_id", "nodes", "edges"},
	})
	return domain.Tool{
		Name:             "native_save_workflow_graph",
		Description:      "Replace a workflow's graph with rich control-flow nodes and edges: start, end, agent, supervisor, condition, parallel, join, loop, plus integration nodes — tool (run one workspace tool), webhook (POST output to a URL), and gateway (send output as a chat message).",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        10000,
		Enabled:          true,
	}
}

func (t *SaveWorkflowGraphTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	workflowID, _ := input["workflow_id"].(string)
	if workflowID == "" {
		return nil, fmt.Errorf("workflow_id is required")
	}

	nodes, err := parseWorkflowGraphNodes(input["nodes"])
	if err != nil {
		return nil, err
	}
	edges, err := parseWorkflowGraphEdges(input["edges"])
	if err != nil {
		return nil, err
	}
	if err := validateWorkflowGraph(ctx, t.pool, execCtx.WorkspaceID, nodes, edges); err != nil {
		return nil, err
	}

	var workflowName string
	if err := t.pool.QueryRow(ctx,
		`SELECT name FROM workflows WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		workflowID, execCtx.WorkspaceID).Scan(&workflowName); err != nil {
		return nil, fmt.Errorf("workflow not found")
	}

	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("save workflow graph: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM workflow_nodes WHERE workflow_id=$1::uuid`, workflowID); err != nil {
		return nil, fmt.Errorf("save workflow graph: clear existing graph: %w", err)
	}

	clientToServer := make(map[string]string, len(nodes))
	for _, node := range nodes {
		serverID := uuid.NewString()
		clientToServer[node.ID] = serverID
		cfgJSON, _ := json.Marshal(node.Config)
		if len(cfgJSON) == 0 || string(cfgJSON) == "null" {
			cfgJSON = []byte(`{}`)
		}

		if node.AgentID != "" {
			_, err = tx.Exec(ctx,
				`INSERT INTO workflow_nodes(id,workflow_id,node_type,agent_id,position_x,position_y,config)
				 VALUES($1::uuid,$2::uuid,$3,$4::uuid,$5,$6,$7::jsonb)`,
				serverID, workflowID, node.Type, node.AgentID, node.PositionX, node.PositionY, string(cfgJSON))
			if err == nil {
				tx.Exec(ctx, //nolint:errcheck
					`UPDATE agents SET tags = array_append(tags, $1), updated_at=NOW()
					 WHERE id=$2::uuid AND NOT ($1 = ANY(tags))`,
					workflowName, node.AgentID)
			}
		} else {
			_, err = tx.Exec(ctx,
				`INSERT INTO workflow_nodes(id,workflow_id,node_type,position_x,position_y,config)
				 VALUES($1::uuid,$2::uuid,$3,$4,$5,$6::jsonb)`,
				serverID, workflowID, node.Type, node.PositionX, node.PositionY, string(cfgJSON))
		}
		if err != nil {
			return nil, fmt.Errorf("save workflow graph: insert node %q: %w", node.ID, err)
		}
	}

	for _, edge := range edges {
		srcID := clientToServer[edge.Source]
		tgtID := clientToServer[edge.Target]
		_, err := tx.Exec(ctx,
			`INSERT INTO workflow_edges(id,workflow_id,source_node_id,target_node_id,label)
			 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5)`,
			uuid.NewString(), workflowID, srcID, tgtID, edge.Label)
		if err != nil {
			return nil, fmt.Errorf("save workflow graph: insert edge %q -> %q: %w", edge.Source, edge.Target, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("save workflow graph: commit: %w", err)
	}

	return map[string]any{
		"ok":          true,
		"workflow_id": workflowID,
		"node_count":  len(nodes),
		"edge_count":  len(edges),
	}, nil
}

type workflowGraphNodeInput struct {
	ID        string
	Type      string
	AgentID   string
	PositionX float64
	PositionY float64
	Config    map[string]any
}

type workflowGraphEdgeInput struct {
	Source string
	Target string
	Label  string
}

func parseWorkflowGraphNodes(raw any) ([]workflowGraphNodeInput, error) {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil, fmt.Errorf("nodes is required and must include at least a start and end node")
	}
	nodes := make([]workflowGraphNodeInput, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("nodes must be objects")
		}
		id, _ := m["id"].(string)
		nodeType, _ := m["node_type"].(string)
		if id == "" {
			return nil, fmt.Errorf("node id is required")
		}
		if seen[id] {
			return nil, fmt.Errorf("duplicate node id %q", id)
		}
		seen[id] = true
		cfg, _ := m["config"].(map[string]any)
		if cfg == nil {
			cfg = map[string]any{}
		}
		nodes = append(nodes, workflowGraphNodeInput{
			ID:        id,
			Type:      nodeType,
			AgentID:   stringValue(m["agent_id"]),
			PositionX: numberValue(m["position_x"]),
			PositionY: numberValue(m["position_y"]),
			Config:    cfg,
		})
	}
	return nodes, nil
}

func parseWorkflowGraphEdges(raw any) ([]workflowGraphEdgeInput, error) {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil, fmt.Errorf("edges is required")
	}
	edges := make([]workflowGraphEdgeInput, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("edges must be objects")
		}
		source, _ := m["source_node_id"].(string)
		target, _ := m["target_node_id"].(string)
		if source == "" || target == "" {
			return nil, fmt.Errorf("edge source_node_id and target_node_id are required")
		}
		edges = append(edges, workflowGraphEdgeInput{
			Source: source,
			Target: target,
			Label:  stringValue(m["label"]),
		})
	}
	return edges, nil
}

func validateWorkflowGraph(ctx context.Context, pool *pgxpool.Pool, workspaceID string, nodes []workflowGraphNodeInput, edges []workflowGraphEdgeInput) error {
	nodeByID := map[string]workflowGraphNodeInput{}
	outgoing := map[string][]workflowGraphEdgeInput{}
	incoming := map[string][]workflowGraphEdgeInput{}
	startCount, endCount := 0, 0

	for _, node := range nodes {
		switch node.Type {
		case "start":
			startCount++
		case "end":
			endCount++
		case "agent", "supervisor":
			if node.AgentID == "" {
				return fmt.Errorf("%s node %q requires agent_id", node.Type, node.ID)
			}
			var exists bool
			if err := pool.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM agents WHERE id=$1::uuid AND workspace_id=$2::uuid AND status='active')`,
				node.AgentID, workspaceID).Scan(&exists); err != nil || !exists {
				return fmt.Errorf("%s node %q references unknown or inactive agent", node.Type, node.ID)
			}
		case "condition", "parallel", "join", "loop":
		case "tool":
			toolName, _ := node.Config["tool_name"].(string)
			if toolName == "" {
				return fmt.Errorf("tool node %q requires config.tool_name", node.ID)
			}
			var exists bool
			if err := pool.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM tools WHERE name=$1 AND (workspace_id IS NULL OR workspace_id=$2::uuid) AND enabled=true)`,
				toolName, workspaceID).Scan(&exists); err != nil || !exists {
				return fmt.Errorf("tool node %q references unknown or disabled tool %q", node.ID, toolName)
			}
		case "webhook":
			if u, _ := node.Config["url"].(string); u == "" {
				return fmt.Errorf("webhook node %q requires config.url", node.ID)
			}
		case "gateway":
			chID, _ := node.Config["channel_id"].(string)
			peerID, _ := node.Config["peer_id"].(string)
			if chID == "" || peerID == "" {
				return fmt.Errorf("gateway node %q requires config.channel_id and config.peer_id", node.ID)
			}
			var exists bool
			if err := pool.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM gateway_channels WHERE id=$1::uuid AND workspace_id=$2::uuid)`,
				chID, workspaceID).Scan(&exists); err != nil || !exists {
				return fmt.Errorf("gateway node %q references unknown gateway channel", node.ID)
			}
		default:
			return fmt.Errorf("unsupported node_type %q for node %q", node.Type, node.ID)
		}
		nodeByID[node.ID] = node
	}
	if startCount != 1 {
		return fmt.Errorf("workflow graph must include exactly one start node")
	}
	if endCount != 1 {
		return fmt.Errorf("workflow graph must include exactly one end node")
	}

	for _, edge := range edges {
		if _, ok := nodeByID[edge.Source]; !ok {
			return fmt.Errorf("edge references unknown source node %q", edge.Source)
		}
		if _, ok := nodeByID[edge.Target]; !ok {
			return fmt.Errorf("edge references unknown target node %q", edge.Target)
		}
		outgoing[edge.Source] = append(outgoing[edge.Source], edge)
		incoming[edge.Target] = append(incoming[edge.Target], edge)
	}

	for _, node := range nodes {
		switch node.Type {
		case "condition":
			if stringValue(node.Config["expression"]) == "" {
				return fmt.Errorf("condition node %q requires config.expression", node.ID)
			}
			if len(outgoing[node.ID]) == 0 {
				return fmt.Errorf("condition node %q requires outgoing branch edges", node.ID)
			}
		case "parallel":
			if len(outgoing[node.ID]) < 2 {
				return fmt.Errorf("parallel node %q requires at least two outgoing edges", node.ID)
			}
		case "join":
			if len(incoming[node.ID]) < 2 {
				return fmt.Errorf("join node %q requires at least two incoming edges", node.ID)
			}
		case "loop":
			if stringValue(node.Config["exit_condition"]) == "" {
				return fmt.Errorf("loop node %q requires config.exit_condition", node.ID)
			}
			if numberValue(node.Config["max_iterations"]) <= 0 {
				return fmt.Errorf("loop node %q requires config.max_iterations greater than zero", node.ID)
			}
			hasLoopOrExit := false
			for _, edge := range outgoing[node.ID] {
				if edge.Label == "loop" || edge.Label == "exit" {
					hasLoopOrExit = true
				}
			}
			if !hasLoopOrExit {
				return fmt.Errorf("loop node %q requires a loop or exit edge", node.ID)
			}
		}
	}
	return nil
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func numberValue(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

// ── native_run_workflow ───────────────────────────────────────────────────────

type RunWorkflowTool struct {
	tools.RequiresRunContext
	pool *pgxpool.Pool
}

func NewRunWorkflowTool(pool *pgxpool.Pool) *RunWorkflowTool { return &RunWorkflowTool{pool: pool} }

func (t *RunWorkflowTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workflow_id": map[string]any{"type": "string", "description": "UUID of the workflow to run."},
			"input":       map[string]any{"type": "string", "description": "Input text to pass to the workflow."},
		},
		"required": []string{"workflow_id", "input"},
	})
	return domain.Tool{
		Name:             "native_run_workflow",
		Description:      "Run a workflow and wait for the result. Returns the workflow run_id, final status, and output.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        900000,
		Enabled:          true,
	}
}
func (t *RunWorkflowTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	workflowID, _ := input["workflow_id"].(string)
	userInput, _ := input["input"].(string)
	if workflowID == "" || userInput == "" {
		return nil, fmt.Errorf("workflow_id and input are required")
	}
	if execCtx.RunWorkflow == nil {
		return nil, fmt.Errorf("workflow execution not available in this context")
	}
	runID, err := execCtx.RunWorkflow(ctx, workflowID, userInput)
	if err != nil {
		return nil, fmt.Errorf("native_run_workflow: %w", err)
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()

	for {
		var status, output, errMsg string
		if err := t.pool.QueryRow(ctx,
			`SELECT status, COALESCE(output,''), COALESCE(error_message,'') FROM runs WHERE id=$1::uuid`,
			runID).Scan(&status, &output, &errMsg); err != nil {
			return nil, fmt.Errorf("native_run_workflow: read workflow run: %w", err)
		}

		switch status {
		case "success":
			return map[string]any{
				"run_id":      runID,
				"workflow_id": workflowID,
				"status":      status,
				"output":      output,
			}, nil
		case "failed", "cancelled":
			if errMsg == "" {
				errMsg = "workflow run ended with status " + status
			}
			return map[string]any{
				"run_id":      runID,
				"workflow_id": workflowID,
				"status":      status,
				"error":       errMsg,
			}, fmt.Errorf("native_run_workflow: %s", errMsg)
		}

		select {
		case <-ctx.Done():
			return map[string]any{
				"run_id":      runID,
				"workflow_id": workflowID,
				"status":      status,
				"note":        "Workflow run is still running.",
			}, ctx.Err()
		case <-deadline.C:
			return map[string]any{
				"run_id":      runID,
				"workflow_id": workflowID,
				"status":      status,
				"note":        "Workflow run is still running after 15 minutes.",
			}, fmt.Errorf("native_run_workflow: timed out waiting for workflow run %s", runID)
		case <-ticker.C:
		}
	}
}

// ── native_delete_workflow ────────────────────────────────────────────────────

type DeleteWorkflowTool struct {
	tools.RequiresRunContext
	pool *pgxpool.Pool
}

func NewDeleteWorkflowTool(pool *pgxpool.Pool) *DeleteWorkflowTool {
	return &DeleteWorkflowTool{pool: pool}
}

func (t *DeleteWorkflowTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workflow_id": map[string]any{"type": "string", "description": "UUID of the workflow to delete."},
		},
		"required": []string{"workflow_id"},
	})
	return domain.Tool{
		Name:             "native_delete_workflow",
		Description:      "Delete a workflow. Only workflows created by the current run can be deleted.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "high",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}
func (t *DeleteWorkflowTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	workflowID, _ := input["workflow_id"].(string)
	if workflowID == "" {
		return nil, fmt.Errorf("workflow_id is required")
	}
	var srcRunID string
	err := t.pool.QueryRow(ctx,
		`SELECT COALESCE(source_run_id::text,'') FROM workflows WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		workflowID, execCtx.WorkspaceID).Scan(&srcRunID)
	if err != nil {
		return nil, fmt.Errorf("workflow not found")
	}
	if srcRunID != execCtx.RunID {
		return nil, fmt.Errorf("permission denied: can only delete workflows created by the current run")
	}
	if _, err := t.pool.Exec(ctx, `DELETE FROM workflows WHERE id=$1::uuid`, workflowID); err != nil {
		return nil, err
	}
	return map[string]any{"deleted": true, "workflow_id": workflowID}, nil
}
