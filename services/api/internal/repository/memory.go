package repository

import (
	"context"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MemoryRepository struct{ pool *pgxpool.Pool }

func NewMemoryRepository(p *pgxpool.Pool) *MemoryRepository { return &MemoryRepository{p} }

const memoryCols = `id::text,workspace_id::text,COALESCE(agent_id::text,''),COALESCE(user_id::text,''),scope,content,relevance_score,COALESCE(source_run_id::text,''),created_at,updated_at`

func (r *MemoryRepository) Store(c context.Context, m *domain.Memory, _ []float32) error {
	_, e := r.pool.Exec(c, `INSERT INTO memories(id,workspace_id,agent_id,user_id,scope,content,relevance_score,source_run_id)VALUES($1::uuid,$2::uuid,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,$5,$6,$7,NULLIF($8,'')::uuid)`, m.ID, m.WorkspaceID, m.AgentID, m.UserID, m.Scope, m.Content, m.RelevanceScore, m.SourceRunID)
	return e
}
func (r *MemoryRepository) Search(c context.Context, w, a string, _ []float32, l int) ([]domain.Memory, error) {
	if l <= 0 {
		l = 20
	}
	rows, e := r.pool.Query(c, `SELECT `+memoryCols+` FROM memories WHERE workspace_id=$1::uuid AND ($2='' OR agent_id=$2::uuid) ORDER BY relevance_score DESC,created_at DESC LIMIT $3`, w, a, l)
	return scanMemories(rows, e)
}
func (r *MemoryRepository) List(c context.Context, w string) ([]domain.Memory, error) {
	rows, e := r.pool.Query(c, `SELECT `+memoryCols+` FROM memories WHERE workspace_id=$1::uuid ORDER BY created_at DESC`, w)
	return scanMemories(rows, e)
}
func scanMemories(rows interface {
	Next() bool
	Scan(...any) error
	Close()
	Err() error
}, e error) ([]domain.Memory, error) {
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	a := []domain.Memory{}
	for rows.Next() {
		var m domain.Memory
		if e := rows.Scan(&m.ID, &m.WorkspaceID, &m.AgentID, &m.UserID, &m.Scope, &m.Content, &m.RelevanceScore, &m.SourceRunID, &m.CreatedAt, &m.UpdatedAt); e != nil {
			return nil, e
		}
		a = append(a, m)
	}
	return a, rows.Err()
}
func (r *MemoryRepository) Delete(c context.Context, id string) error {
	_, e := r.pool.Exec(c, `DELETE FROM memories WHERE id=$1::uuid`, id)
	return e
}
func (r *MemoryRepository) BulkDelete(c context.Context, w, a string) error {
	_, e := r.pool.Exec(c, `DELETE FROM memories WHERE workspace_id=$1::uuid AND ($2='' OR agent_id=$2::uuid)`, w, a)
	return e
}
