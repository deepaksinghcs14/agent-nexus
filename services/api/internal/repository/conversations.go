package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentNexus/agent-nexus/services/api/internal/domain"
)

type ConversationRepository struct {
	pool *pgxpool.Pool
}

func NewConversationRepository(pool *pgxpool.Pool) *ConversationRepository {
	return &ConversationRepository{pool: pool}
}

func (r *ConversationRepository) List(ctx context.Context, workspaceID string) ([]domain.Conversation, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id::text, workspace_id::text, agent_id::text, user_id::text, title,
		        message_count, token_count, created_at, updated_at
		 FROM conversations WHERE workspace_id = $1::uuid ORDER BY updated_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convs []domain.Conversation
	for rows.Next() {
		var c domain.Conversation
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.AgentID, &c.UserID, &c.Title,
			&c.MessageCount, &c.TokenCount, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		convs = append(convs, c)
	}
	if convs == nil {
		convs = []domain.Conversation{}
	}
	return convs, rows.Err()
}

func (r *ConversationRepository) Create(ctx context.Context, c *domain.Conversation) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO conversations (id, workspace_id, agent_id, user_id, title, message_count, token_count, created_at, updated_at)
		 VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, 0, 0, NOW(), NOW())`,
		c.ID, c.WorkspaceID, c.AgentID, c.UserID, c.Title)
	return err
}

func (r *ConversationRepository) Get(ctx context.Context, id, workspaceID string) (*domain.Conversation, error) {
	var c domain.Conversation
	err := r.pool.QueryRow(ctx,
		`SELECT id::text, workspace_id::text, agent_id::text, user_id::text, title,
		        message_count, token_count, created_at, updated_at
		 FROM conversations WHERE id=$1::uuid AND workspace_id=$2::uuid`, id, workspaceID).
		Scan(&c.ID, &c.WorkspaceID, &c.AgentID, &c.UserID, &c.Title,
			&c.MessageCount, &c.TokenCount, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("conversation not found")
	}
	return &c, err
}

func (r *ConversationRepository) Delete(ctx context.Context, id, workspaceID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM conversations WHERE id=$1::uuid AND workspace_id=$2::uuid`, id, workspaceID)
	return err
}

func (r *ConversationRepository) ListMessages(ctx context.Context, conversationID string) ([]domain.Message, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id::text, conversation_id::text, role, content, tokens, created_at
		 FROM messages WHERE conversation_id = $1::uuid ORDER BY created_at ASC`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []domain.Message
	for rows.Next() {
		var m domain.Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Tokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if msgs == nil {
		msgs = []domain.Message{}
	}
	return msgs, rows.Err()
}

func (r *ConversationRepository) AddMessage(ctx context.Context, m *domain.Message) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO messages (id, conversation_id, role, content, tokens, created_at)
		 VALUES ($1::uuid, $2::uuid, $3, $4, $5, NOW())`,
		m.ID, m.ConversationID, m.Role, m.Content, m.Tokens)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx,
		`UPDATE conversations SET message_count=message_count+1, token_count=token_count+$2, updated_at=NOW()
		 WHERE id=$1::uuid`, m.ConversationID, m.Tokens)
	return err
}
