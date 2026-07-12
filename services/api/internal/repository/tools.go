package repository

import (
	"context"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ToolRepository struct{ pool *pgxpool.Pool }

func NewToolRepository(p *pgxpool.Pool) *ToolRepository { return &ToolRepository{p} }

const toolCols = `id::text,COALESCE(workspace_id::text,''),name,description,type,input_schema,output_schema,config,risk_level,requires_approval,timeout_ms,enabled,COALESCE(category,''),created_at,updated_at`

func scanTool(r interface{ Scan(...any) error }) (domain.Tool, error) {
	var t domain.Tool
	e := r.Scan(&t.ID, &t.WorkspaceID, &t.Name, &t.Description, &t.Type, &t.InputSchema, &t.OutputSchema, &t.Config, &t.RiskLevel, &t.RequiresApproval, &t.TimeoutMs, &t.Enabled, &t.Category, &t.CreatedAt, &t.UpdatedAt)
	return t, e
}

// List returns global tools plus the workspace's own, ordered for the UI.
func (r *ToolRepository) List(c context.Context, w string) ([]domain.Tool, error) {
	rows, e := r.pool.Query(c, `SELECT `+toolCols+` FROM tools WHERE workspace_id IS NULL OR workspace_id=$1::uuid ORDER BY type,name`, w)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	a := []domain.Tool{}
	for rows.Next() {
		t, e := scanTool(rows)
		if e != nil {
			return nil, e
		}
		a = append(a, t)
	}
	return a, rows.Err()
}

// Create inserts a workspace tool and fills the row's timestamps back into t.
func (r *ToolRepository) Create(c context.Context, t *domain.Tool) error {
	return r.pool.QueryRow(c,
		`INSERT INTO tools(id,workspace_id,name,description,type,input_schema,output_schema,config,risk_level,requires_approval,timeout_ms,enabled,category)
		 VALUES($1::uuid,NULLIF($2,'')::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NULLIF($13,'')) RETURNING created_at,updated_at`,
		t.ID, t.WorkspaceID, t.Name, t.Description, t.Type, t.InputSchema, t.OutputSchema, t.Config, t.RiskLevel, t.RequiresApproval, t.TimeoutMs, t.Enabled, t.Category).
		Scan(&t.CreatedAt, &t.UpdatedAt)
}

// Get is workspace-scoped: global tools (NULL workspace) and the caller's own
// are visible; other workspaces' tools are not.
func (r *ToolRepository) Get(c context.Context, id, workspaceID string) (*domain.Tool, error) {
	t, e := scanTool(r.pool.QueryRow(c, `SELECT `+toolCols+` FROM tools WHERE id=$1::uuid AND (workspace_id IS NULL OR workspace_id=$2::uuid)`, id, workspaceID))
	return &t, e
}

// Update modifies a tool owned by the workspace. Returns false when no row
// matched (unknown id, global tool, or another workspace's tool).
func (r *ToolRepository) Update(c context.Context, t *domain.Tool) (bool, error) {
	tag, e := r.pool.Exec(c,
		`UPDATE tools SET name=$3,description=$4,type=$5,input_schema=$6,output_schema=$7,config=$8,risk_level=$9,requires_approval=$10,timeout_ms=$11,enabled=$12,category=NULLIF($13,''),updated_at=NOW()
		 WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		t.ID, t.WorkspaceID, t.Name, t.Description, t.Type, t.InputSchema, t.OutputSchema, t.Config, t.RiskLevel, t.RequiresApproval, t.TimeoutMs, t.Enabled, t.Category)
	return tag.RowsAffected() > 0, e
}

// Delete removes a tool owned by the workspace. Returns false when no row
// matched — global tools cannot be deleted through this path.
func (r *ToolRepository) Delete(c context.Context, id, workspaceID string) (bool, error) {
	tag, e := r.pool.Exec(c, `DELETE FROM tools WHERE id=$1::uuid AND workspace_id=$2::uuid`, id, workspaceID)
	return tag.RowsAffected() > 0, e
}
