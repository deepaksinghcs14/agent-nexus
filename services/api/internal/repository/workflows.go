package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WorkflowRepository owns the workflows / workflow_members tables and the
// visual graph (workflow_nodes / workflow_edges).
type WorkflowRepository struct{ pool *pgxpool.Pool }

func NewWorkflowRepository(p *pgxpool.Pool) *WorkflowRepository { return &WorkflowRepository{p} }

// WorkflowSummary is the API shape of a workflow row plus its member agents.
type WorkflowSummary struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Mode        string    `json:"mode"`
	Status      string    `json:"status"`
	AgentIDs    []string  `json:"agent_ids"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WorkflowGraphNode / WorkflowGraphEdge are the persisted graph shapes.
type WorkflowGraphNode struct {
	ID        string          `json:"id"`
	NodeType  string          `json:"node_type"`
	AgentID   string          `json:"agent_id,omitempty"`
	PositionX float64         `json:"position_x"`
	PositionY float64         `json:"position_y"`
	Config    json.RawMessage `json:"config"`
}

type WorkflowGraphEdge struct {
	ID           string `json:"id"`
	SourceNodeID string `json:"source_node_id"`
	TargetNodeID string `json:"target_node_id"`
	Label        string `json:"label,omitempty"`
}

// ErrUnknownEdgeNode is returned by SaveGraph when an edge references a node
// id that is not part of the submitted node set — a client error, not an
// internal one.
var ErrUnknownEdgeNode = errors.New("edge references unknown node id")

func (r *WorkflowRepository) Get(ctx context.Context, id, workspaceID string) (*WorkflowSummary, error) {
	var wf WorkflowSummary
	if err := r.pool.QueryRow(ctx,
		`SELECT id::text,workspace_id::text,name,description,mode,status,created_by::text,created_at,updated_at
		 FROM workflows WHERE id=$1::uuid AND workspace_id=$2::uuid`, id, workspaceID).
		Scan(&wf.ID, &wf.WorkspaceID, &wf.Name, &wf.Description, &wf.Mode, &wf.Status, &wf.CreatedBy, &wf.CreatedAt, &wf.UpdatedAt); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT agent_id::text FROM workflow_members WHERE group_id=$1::uuid ORDER BY position`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	wf.AgentIDs = []string{}
	for rows.Next() {
		var a string
		if rows.Scan(&a) == nil {
			wf.AgentIDs = append(wf.AgentIDs, a)
		}
	}
	return &wf, rows.Err()
}

func (r *WorkflowRepository) List(ctx context.Context, workspaceID string) ([]*WorkflowSummary, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text FROM workflows WHERE workspace_id=$1::uuid ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []*WorkflowSummary{}
	for _, id := range ids {
		if wf, err := r.Get(ctx, id, workspaceID); err == nil {
			out = append(out, wf)
		}
	}
	return out, nil
}

// Save creates or updates a workflow and replaces its member list. In
// supervisor mode the first agent is the supervisor.
func (r *WorkflowRepository) Save(ctx context.Context, wf *WorkflowSummary, create bool) error {
	return WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		var err error
		if create {
			_, err = tx.Exec(ctx,
				`INSERT INTO workflows(id,workspace_id,name,description,mode,status,created_by)VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7::uuid)`,
				wf.ID, wf.WorkspaceID, wf.Name, wf.Description, wf.Mode, wf.Status, wf.CreatedBy)
		} else {
			_, err = tx.Exec(ctx,
				`UPDATE workflows SET name=$3,description=$4,mode=$5,status=$6,updated_at=NOW() WHERE id=$1::uuid AND workspace_id=$2::uuid`,
				wf.ID, wf.WorkspaceID, wf.Name, wf.Description, wf.Mode, wf.Status)
			if err == nil {
				_, err = tx.Exec(ctx, `DELETE FROM workflow_members WHERE group_id=$1::uuid`, wf.ID)
			}
		}
		if err != nil {
			return err
		}
		for i, a := range wf.AgentIDs {
			role := "member"
			if wf.Mode == "supervisor" && i == 0 {
				role = "supervisor"
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO workflow_members(group_id,agent_id,position,role)VALUES($1::uuid,$2::uuid,$3,$4)`,
				wf.ID, a, i, role); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *WorkflowRepository) Delete(ctx context.Context, id, workspaceID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM workflows WHERE id=$1::uuid AND workspace_id=$2::uuid`, id, workspaceID)
	return tag.RowsAffected() > 0, err
}

func (r *WorkflowRepository) Exists(ctx context.Context, id, workspaceID string) bool {
	var ok bool
	_ = r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM workflows WHERE id=$1::uuid AND workspace_id=$2::uuid)`, id, workspaceID).Scan(&ok)
	return ok
}

func (r *WorkflowRepository) GetGraph(ctx context.Context, workflowID string) ([]WorkflowGraphNode, []WorkflowGraphEdge, error) {
	nodeRows, err := r.pool.Query(ctx,
		`SELECT id::text, node_type, COALESCE(agent_id::text,''), position_x, position_y, config::text
		 FROM workflow_nodes WHERE workflow_id=$1::uuid ORDER BY created_at`, workflowID)
	if err != nil {
		return nil, nil, err
	}
	defer nodeRows.Close()
	nodes := []WorkflowGraphNode{}
	for nodeRows.Next() {
		var n WorkflowGraphNode
		var cfg string
		if err := nodeRows.Scan(&n.ID, &n.NodeType, &n.AgentID, &n.PositionX, &n.PositionY, &cfg); err != nil {
			return nil, nil, err
		}
		n.Config = json.RawMessage(cfg)
		nodes = append(nodes, n)
	}
	if err := nodeRows.Err(); err != nil {
		return nil, nil, err
	}

	edgeRows, err := r.pool.Query(ctx,
		`SELECT id::text, source_node_id::text, target_node_id::text, COALESCE(label,'')
		 FROM workflow_edges WHERE workflow_id=$1::uuid ORDER BY created_at`, workflowID)
	if err != nil {
		return nil, nil, err
	}
	defer edgeRows.Close()
	edges := []WorkflowGraphEdge{}
	for edgeRows.Next() {
		var e WorkflowGraphEdge
		if err := edgeRows.Scan(&e.ID, &e.SourceNodeID, &e.TargetNodeID, &e.Label); err != nil {
			return nil, nil, err
		}
		edges = append(edges, e)
	}
	return nodes, edges, edgeRows.Err()
}

// SaveGraph replaces the workflow's graph. Nodes are upserted by their
// client-supplied id (a fresh UUID is minted when the client id isn't one) so
// node identity stays stable across saves — canvas selection state survives
// and SSE node_id events from a run always match what's on screen. Edges have
// no client-visible identity and are fully replaced. The WHERE guard on the
// upsert means a spoofed id belonging to another workflow cannot be hijacked.
func (r *WorkflowRepository) SaveGraph(ctx context.Context, workflowID string, nodes []WorkflowGraphNode, edges []WorkflowGraphEdge) error {
	return WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		resolvedID := make(map[string]string, len(nodes))
		nodeIDs := make([]string, 0, len(nodes))
		for _, n := range nodes {
			id := n.ID
			if _, perr := uuid.Parse(id); perr != nil {
				id = uuid.NewString()
			}
			resolvedID[n.ID] = id
			nodeIDs = append(nodeIDs, id)
		}

		// Remove nodes absent from the incoming set — cascades to their edges.
		if _, err := tx.Exec(ctx,
			`DELETE FROM workflow_nodes WHERE workflow_id=$1::uuid AND NOT (id = ANY($2::uuid[]))`,
			workflowID, nodeIDs); err != nil {
			return err
		}

		for _, n := range nodes {
			id := resolvedID[n.ID]
			cfg := n.Config
			if len(cfg) == 0 {
				cfg = json.RawMessage(`{}`)
			}
			var err error
			if n.AgentID != "" {
				_, err = tx.Exec(ctx,
					`INSERT INTO workflow_nodes(id,workflow_id,node_type,agent_id,position_x,position_y,config)
					 VALUES($1::uuid,$2::uuid,$3,$4::uuid,$5,$6,$7::jsonb)
					 ON CONFLICT (id) DO UPDATE SET
					   node_type=EXCLUDED.node_type, agent_id=EXCLUDED.agent_id,
					   position_x=EXCLUDED.position_x, position_y=EXCLUDED.position_y, config=EXCLUDED.config
					 WHERE workflow_nodes.workflow_id = EXCLUDED.workflow_id`,
					id, workflowID, n.NodeType, n.AgentID, n.PositionX, n.PositionY, string(cfg))
			} else {
				_, err = tx.Exec(ctx,
					`INSERT INTO workflow_nodes(id,workflow_id,node_type,position_x,position_y,config)
					 VALUES($1::uuid,$2::uuid,$3,$4,$5,$6::jsonb)
					 ON CONFLICT (id) DO UPDATE SET
					   node_type=EXCLUDED.node_type, agent_id=NULL,
					   position_x=EXCLUDED.position_x, position_y=EXCLUDED.position_y, config=EXCLUDED.config
					 WHERE workflow_nodes.workflow_id = EXCLUDED.workflow_id`,
					id, workflowID, n.NodeType, n.PositionX, n.PositionY, string(cfg))
			}
			if err != nil {
				return err
			}
		}

		// Edges: fully replace using the resolved (stable) node ids.
		if _, err := tx.Exec(ctx, `DELETE FROM workflow_edges WHERE workflow_id=$1::uuid`, workflowID); err != nil {
			return err
		}
		for _, e := range edges {
			srcID, srcOK := resolvedID[e.SourceNodeID]
			tgtID, tgtOK := resolvedID[e.TargetNodeID]
			if !srcOK || !tgtOK {
				return ErrUnknownEdgeNode
			}
			var err error
			if e.Label != "" {
				_, err = tx.Exec(ctx,
					`INSERT INTO workflow_edges(id,workflow_id,source_node_id,target_node_id,label)
					 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5)`,
					uuid.NewString(), workflowID, srcID, tgtID, e.Label)
			} else {
				_, err = tx.Exec(ctx,
					`INSERT INTO workflow_edges(id,workflow_id,source_node_id,target_node_id)
					 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid)`,
					uuid.NewString(), workflowID, srcID, tgtID)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}
