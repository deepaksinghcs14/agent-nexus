package repository

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MemoryRepository struct{ pool *pgxpool.Pool }

func NewMemoryRepository(p *pgxpool.Pool) *MemoryRepository { return &MemoryRepository{p} }

const memoryCols = `id::text,workspace_id::text,COALESCE(agent_id::text,''),COALESCE(user_id::text,''),COALESCE(conversation_id::text,''),scope,content,relevance_score,importance_score,status,save_source,COALESCE(source_run_id::text,''),metadata,created_at,updated_at`

func (r *MemoryRepository) Store(c context.Context, m *domain.Memory, embedding []float32) error {
	if m.Status == "" {
		m.Status = "active"
	}
	if m.SaveSource == "" {
		m.SaveSource = "tool"
	}
	if m.Metadata == nil {
		m.Metadata = json.RawMessage(`{}`)
	}
	if m.ImportanceScore == 0 {
		m.ImportanceScore = m.RelevanceScore
	}
	if m.RelevanceScore == 0 {
		m.RelevanceScore = m.ImportanceScore
	}
	var e error
	if len(embedding) > 0 {
		_, e = r.pool.Exec(c, `INSERT INTO memories(id,workspace_id,agent_id,user_id,conversation_id,scope,content,embedding,relevance_score,importance_score,status,save_source,source_run_id,metadata)VALUES($1::uuid,$2::uuid,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,$6,$7,$8::vector,$9,$10,$11,$12,NULLIF($13,'')::uuid,$14::jsonb)`, m.ID, m.WorkspaceID, m.AgentID, m.UserID, m.ConversationID, m.Scope, m.Content, formatVec(embedding), m.RelevanceScore, m.ImportanceScore, m.Status, m.SaveSource, m.SourceRunID, m.Metadata)
	} else {
		_, e = r.pool.Exec(c, `INSERT INTO memories(id,workspace_id,agent_id,user_id,conversation_id,scope,content,relevance_score,importance_score,status,save_source,source_run_id,metadata)VALUES($1::uuid,$2::uuid,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,$6,$7,$8,$9,$10,$11,NULLIF($12,'')::uuid,$13::jsonb)`, m.ID, m.WorkspaceID, m.AgentID, m.UserID, m.ConversationID, m.Scope, m.Content, m.RelevanceScore, m.ImportanceScore, m.Status, m.SaveSource, m.SourceRunID, m.Metadata)
	}
	return e
}
func (r *MemoryRepository) Search(c context.Context, w, a, conversationID string, embedding []float32, l int, minScore float64) ([]domain.Memory, error) {
	if l <= 0 {
		l = 20
	}
	if len(embedding) > 0 {
		rows, e := r.pool.Query(c, `SELECT `+memoryCols+` FROM memories WHERE workspace_id=$1::uuid AND status='active' AND relevance_score >= $5 AND (scope='workspace' OR (scope='agent' AND agent_id=$2::uuid) OR (scope='conversation' AND conversation_id=NULLIF($3,'')::uuid)) AND embedding IS NOT NULL ORDER BY embedding <=> $6::vector LIMIT $4`, w, a, conversationID, l, minScore, formatVec(embedding))
		return scanMemories(rows, e)
	}
	rows, e := r.pool.Query(c, `SELECT `+memoryCols+` FROM memories WHERE workspace_id=$1::uuid AND status='active' AND relevance_score >= $5 AND (scope='workspace' OR (scope='agent' AND agent_id=$2::uuid) OR (scope='conversation' AND conversation_id=NULLIF($3,'')::uuid)) ORDER BY relevance_score DESC, importance_score DESC, created_at DESC LIMIT $4`, w, a, conversationID, l, minScore)
	return scanMemories(rows, e)
}
func (r *MemoryRepository) List(c context.Context, w, agentID, scope, query, status, source string) ([]domain.Memory, error) {
	rows, e := r.pool.Query(c, `SELECT `+memoryCols+` FROM memories WHERE workspace_id=$1::uuid AND ($2='' OR agent_id=NULLIF($2,'')::uuid) AND ($3='' OR scope=$3) AND ($4='' OR content ILIKE '%'||$4||'%') AND ($5='' OR status=$5) AND ($6='' OR save_source=$6) ORDER BY created_at DESC`, w, agentID, scope, query, status, source)
	return scanMemories(rows, e)
}
func (r *MemoryRepository) HasSimilar(c context.Context, w, a, conversationID, content string, embedding []float32, threshold float64) (bool, float64, error) {
	if threshold <= 0 {
		threshold = 0.88
	}
	if len(embedding) > 0 {
		var score float64
		err := r.pool.QueryRow(c, `SELECT COALESCE(MAX(1 - (embedding <=> $4::vector)), 0) FROM memories WHERE workspace_id=$1::uuid AND status IN ('active','pending') AND (scope='workspace' OR (scope='agent' AND agent_id=$2::uuid) OR (scope='conversation' AND conversation_id=NULLIF($3,'')::uuid)) AND embedding IS NOT NULL`, w, a, conversationID, formatVec(embedding)).Scan(&score)
		return score >= threshold, score, err
	}
	var exists bool
	err := r.pool.QueryRow(c, `SELECT EXISTS(SELECT 1 FROM memories WHERE workspace_id=$1::uuid AND status IN ('active','pending') AND (scope='workspace' OR (scope='agent' AND agent_id=$2::uuid) OR (scope='conversation' AND conversation_id=NULLIF($3,'')::uuid)) AND lower(content)=lower($4))`, w, a, conversationID, strings.TrimSpace(content)).Scan(&exists)
	if exists {
		return true, 1, err
	}
	return false, 0, err
}
func (r *MemoryRepository) SetStatus(c context.Context, id, workspaceID, status string) error {
	_, e := r.pool.Exec(c, `UPDATE memories SET status=$3, updated_at=NOW() WHERE id=$1::uuid AND workspace_id=$2::uuid`, id, workspaceID, status)
	return e
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
		if e := rows.Scan(&m.ID, &m.WorkspaceID, &m.AgentID, &m.UserID, &m.ConversationID, &m.Scope, &m.Content, &m.RelevanceScore, &m.ImportanceScore, &m.Status, &m.SaveSource, &m.SourceRunID, &m.Metadata, &m.CreatedAt, &m.UpdatedAt); e != nil {
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
	_, e := r.pool.Exec(c, `DELETE FROM memories WHERE workspace_id=$1::uuid AND ($2='' OR agent_id=NULLIF($2,'')::uuid)`, w, a)
	return e
}

func formatVec(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
