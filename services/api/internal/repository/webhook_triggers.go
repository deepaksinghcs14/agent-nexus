package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
)

type WebhookTriggerRepository struct {
	pool *pgxpool.Pool
}

func NewWebhookTriggerRepository(pool *pgxpool.Pool) *WebhookTriggerRepository {
	return &WebhookTriggerRepository{pool: pool}
}

const webhookSelect = `
SELECT wt.id::text, wt.workspace_id::text, wt.name, wt.description,
       wt.target_type, wt.target_id::text,
       COALESCE(a.name, wf.name, ''),
       wt.input_template, COALESCE(wt.secret,''),
       wt.is_active, COALESCE(wt.created_by::text,''),
       wt.created_at, wt.updated_at, wt.last_triggered_at, wt.trigger_count
FROM webhook_triggers wt
LEFT JOIN agents a ON wt.target_type='agent' AND a.id=wt.target_id
LEFT JOIN workflows wf ON wt.target_type='workflow' AND wf.id=wt.target_id`

func scan(row interface{ Scan(...any) error }) (domain.WebhookTrigger, error) {
	var t domain.WebhookTrigger
	err := row.Scan(
		&t.ID, &t.WorkspaceID, &t.Name, &t.Description,
		&t.TargetType, &t.TargetID, &t.TargetName,
		&t.InputTemplate, &t.Secret,
		&t.IsActive, &t.CreatedBy,
		&t.CreatedAt, &t.UpdatedAt, &t.LastTriggeredAt, &t.TriggerCount,
	)
	return t, err
}

func (r *WebhookTriggerRepository) List(ctx context.Context, workspaceID string) ([]domain.WebhookTrigger, error) {
	rows, err := r.pool.Query(ctx,
		webhookSelect+` WHERE wt.workspace_id=$1::uuid ORDER BY wt.created_at DESC`,
		workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []domain.WebhookTrigger{}
	for rows.Next() {
		t, e := scan(rows)
		if e != nil {
			return nil, e
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (r *WebhookTriggerRepository) Get(ctx context.Context, id, workspaceID string) (domain.WebhookTrigger, error) {
	return scan(r.pool.QueryRow(ctx,
		webhookSelect+` WHERE wt.id=$1::uuid AND wt.workspace_id=$2::uuid`,
		id, workspaceID))
}

// GetByID loads a trigger without workspace scoping — used by the public inbound endpoint.
func (r *WebhookTriggerRepository) GetByID(ctx context.Context, id string) (domain.WebhookTrigger, error) {
	return scan(r.pool.QueryRow(ctx,
		webhookSelect+` WHERE wt.id=$1::uuid`,
		id))
}

func (r *WebhookTriggerRepository) Create(ctx context.Context, t *domain.WebhookTrigger) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO webhook_triggers
		   (id, workspace_id, name, description, target_type, target_id,
		    input_template, secret, is_active, created_by)
		 VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6::uuid,$7,$8,$9,$10::uuid)
		 RETURNING created_at, updated_at`,
		t.ID, t.WorkspaceID, t.Name, t.Description, t.TargetType, t.TargetID,
		t.InputTemplate, nullableString(t.Secret), t.IsActive, nullableString(t.CreatedBy),
	).Scan(&t.CreatedAt, &t.UpdatedAt)
}

func (r *WebhookTriggerRepository) Update(ctx context.Context, t *domain.WebhookTrigger) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE webhook_triggers
		 SET name=$1, description=$2, target_type=$3, target_id=$4::uuid,
		     input_template=$5, secret=$6, is_active=$7, updated_at=NOW()
		 WHERE id=$8::uuid AND workspace_id=$9::uuid`,
		t.Name, t.Description, t.TargetType, t.TargetID,
		t.InputTemplate, nullableString(t.Secret), t.IsActive,
		t.ID, t.WorkspaceID,
	)
	return err
}

func (r *WebhookTriggerRepository) Delete(ctx context.Context, id, workspaceID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM webhook_triggers WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		id, workspaceID)
	return err
}

func (r *WebhookTriggerRepository) IncrementTriggerCount(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE webhook_triggers
		 SET trigger_count=trigger_count+1, last_triggered_at=NOW(), updated_at=NOW()
		 WHERE id=$1::uuid`,
		id)
	return err
}

// nullableString converts an empty string to nil for nullable TEXT columns.
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
