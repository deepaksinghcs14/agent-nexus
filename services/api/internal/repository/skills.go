package repository

import (
	"context"
	"fmt"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SkillRepository struct{ pool *pgxpool.Pool }

func NewSkillRepository(pool *pgxpool.Pool) *SkillRepository { return &SkillRepository{pool: pool} }

const skillSelect = `
SELECT id::text, COALESCE(workspace_id::text,''), name, description, content, source, enabled,
       COALESCE(created_by::text,''), created_at, updated_at
FROM skills`

func scanSkill(row interface{ Scan(...any) error }) (domain.Skill, error) {
	var s domain.Skill
	err := row.Scan(&s.ID, &s.WorkspaceID, &s.Name, &s.Description, &s.Content, &s.Source, &s.Enabled, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (r *SkillRepository) List(ctx context.Context, workspaceID string) ([]domain.Skill, error) {
	rows, err := r.pool.Query(ctx, skillSelect+` WHERE workspace_id=$1::uuid OR workspace_id IS NULL ORDER BY source DESC, name ASC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Skill{}
	for rows.Next() {
		s, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *SkillRepository) Get(ctx context.Context, id, workspaceID string) (domain.Skill, error) {
	return scanSkill(r.pool.QueryRow(ctx, skillSelect+` WHERE id=$1::uuid AND (workspace_id=$2::uuid OR workspace_id IS NULL)`, id, workspaceID))
}

func (r *SkillRepository) Create(ctx context.Context, s *domain.Skill) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO skills(id,workspace_id,name,description,content,source,enabled,created_by)
		 VALUES($1::uuid,$2::uuid,$3,$4,$5,'manual',$6,$7::uuid)
		 RETURNING created_at, updated_at`,
		s.ID, s.WorkspaceID, s.Name, s.Description, s.Content, s.Enabled, s.CreatedBy,
	).Scan(&s.CreatedAt, &s.UpdatedAt)
}

func (r *SkillRepository) Update(ctx context.Context, s *domain.Skill) error {
	if s.Source == "managed" {
		return fmt.Errorf("managed skills are immutable")
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE skills SET name=$1, description=$2, content=$3, enabled=$4, updated_at=NOW()
		 WHERE id=$5::uuid AND workspace_id=$6::uuid AND source <> 'managed'`,
		s.Name, s.Description, s.Content, s.Enabled, s.ID, s.WorkspaceID)
	return err
}

func (r *SkillRepository) Delete(ctx context.Context, id, workspaceID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM skills WHERE id=$1::uuid AND workspace_id=$2::uuid AND source <> 'managed'`, id, workspaceID)
	return err
}

func (r *SkillRepository) ListForAgent(ctx context.Context, agentID string) ([]domain.AgentSkill, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ask.id::text, ask.agent_id::text, ask.skill_id::text, ask.enabled, ask.order_index, ask.created_at,
		       s.id::text, COALESCE(s.workspace_id::text,''), s.name, s.description, s.content, s.source,
		       s.enabled, COALESCE(s.created_by::text,''), s.created_at, s.updated_at
		FROM agent_skills ask
		JOIN skills s ON s.id=ask.skill_id
		WHERE ask.agent_id=$1::uuid
		ORDER BY ask.order_index ASC, ask.created_at ASC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AgentSkill{}
	for rows.Next() {
		var a domain.AgentSkill
		var s domain.Skill
		if err := rows.Scan(&a.ID, &a.AgentID, &a.SkillID, &a.Enabled, &a.OrderIndex, &a.CreatedAt,
			&s.ID, &s.WorkspaceID, &s.Name, &s.Description, &s.Content, &s.Source, &s.Enabled, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		a.Skill = &s
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *SkillRepository) SetForAgent(ctx context.Context, agentID string, assignments []domain.AgentSkillAssignment) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `DELETE FROM agent_skills WHERE agent_id=$1::uuid`, agentID); err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for _, a := range assignments {
		if a.SkillID == "" {
			continue
		}
		batch.Queue(`INSERT INTO agent_skills(agent_id,skill_id,enabled,order_index) VALUES($1::uuid,$2::uuid,$3,$4)`,
			agentID, a.SkillID, a.Enabled, a.OrderIndex)
	}
	br := tx.SendBatch(ctx, batch)
	if err := br.Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
