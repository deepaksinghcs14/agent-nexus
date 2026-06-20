package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
)

type AgentRepository struct {
	pool *pgxpool.Pool
}

func NewAgentRepository(pool *pgxpool.Pool) *AgentRepository {
	return &AgentRepository{pool: pool}
}

func (r *AgentRepository) List(ctx context.Context, workspaceID string) ([]domain.Agent, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id::text, workspace_id::text, name, description, instructions, provider, model,
		        temperature, max_tokens, memory_enabled, memory_scope, memory_save_mode, memory_review_policy,
		        max_memories, min_relevance_score, memory_min_importance, memory_dedupe_threshold,
		        context_retrieval_enabled, max_steps, max_tool_calls, max_duration_secs,
		        max_history_messages, lazy_tool_loading, compaction_threshold, compaction_token_threshold,
		        status, created_by::text, created_at, updated_at
		 FROM agents
		 WHERE workspace_id = $1::uuid AND status != 'archived'
		 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []domain.Agent
	for rows.Next() {
		var a domain.Agent
		if err := rows.Scan(
			&a.ID, &a.WorkspaceID, &a.Name, &a.Description, &a.Instructions,
			&a.Provider, &a.Model, &a.Temperature, &a.MaxTokens,
			&a.MemoryEnabled, &a.MemoryScope, &a.MemorySaveMode, &a.MemoryReviewPolicy,
			&a.MaxMemories, &a.MinRelevanceScore, &a.MemoryMinImportance, &a.MemoryDedupeThreshold,
			&a.ContextRetrievalEnabled,
			&a.MaxSteps, &a.MaxToolCalls, &a.MaxDurationSecs,
			&a.MaxHistoryMessages, &a.LazyToolLoading,
			&a.CompactionThreshold, &a.CompactionTokenThreshold,
			&a.Status, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	if agents == nil {
		agents = []domain.Agent{}
	}
	return agents, rows.Err()
}

func (r *AgentRepository) Create(ctx context.Context, a *domain.Agent) error {
	normalizeAgentMemoryDefaults(a)
	_, err := r.pool.Exec(ctx,
		`INSERT INTO agents (id, workspace_id, name, description, instructions, provider, model,
		                     temperature, max_tokens, memory_enabled, memory_scope, memory_save_mode, memory_review_policy,
		                     max_memories, min_relevance_score, memory_min_importance, memory_dedupe_threshold,
		                     context_retrieval_enabled, max_steps, max_tool_calls,
		                     max_duration_secs, max_history_messages, lazy_tool_loading,
		                     compaction_threshold, compaction_token_threshold,
		                     status, created_by, created_at, updated_at)
		 VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27::uuid, NOW(), NOW())`,
		a.ID, a.WorkspaceID, a.Name, a.Description, a.Instructions,
		a.Provider, a.Model, a.Temperature, a.MaxTokens,
		a.MemoryEnabled, a.MemoryScope, a.MemorySaveMode, a.MemoryReviewPolicy,
		a.MaxMemories, a.MinRelevanceScore, a.MemoryMinImportance, a.MemoryDedupeThreshold,
		a.ContextRetrievalEnabled,
		a.MaxSteps, a.MaxToolCalls, a.MaxDurationSecs,
		a.MaxHistoryMessages, a.LazyToolLoading,
		a.CompactionThreshold, a.CompactionTokenThreshold,
		a.Status, a.CreatedBy)
	return err
}

func (r *AgentRepository) Get(ctx context.Context, id, workspaceID string) (*domain.Agent, error) {
	var a domain.Agent
	err := r.pool.QueryRow(ctx,
		`SELECT id::text, workspace_id::text, name, description, instructions, provider, model,
		        temperature, max_tokens, memory_enabled, memory_scope, memory_save_mode, memory_review_policy,
		        max_memories, min_relevance_score, memory_min_importance, memory_dedupe_threshold,
		        context_retrieval_enabled, max_steps, max_tool_calls, max_duration_secs,
		        max_history_messages, lazy_tool_loading, compaction_threshold, compaction_token_threshold,
		        status, created_by::text, created_at, updated_at
		 FROM agents WHERE id = $1::uuid AND workspace_id = $2::uuid`, id, workspaceID).
		Scan(&a.ID, &a.WorkspaceID, &a.Name, &a.Description, &a.Instructions,
			&a.Provider, &a.Model, &a.Temperature, &a.MaxTokens,
			&a.MemoryEnabled, &a.MemoryScope, &a.MemorySaveMode, &a.MemoryReviewPolicy,
			&a.MaxMemories, &a.MinRelevanceScore, &a.MemoryMinImportance, &a.MemoryDedupeThreshold,
			&a.ContextRetrievalEnabled,
			&a.MaxSteps, &a.MaxToolCalls, &a.MaxDurationSecs,
			&a.MaxHistoryMessages, &a.LazyToolLoading,
			&a.CompactionThreshold, &a.CompactionTokenThreshold,
			&a.Status, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("agent not found")
	}
	return &a, err
}

func (r *AgentRepository) Update(ctx context.Context, a *domain.Agent) error {
	normalizeAgentMemoryDefaults(a)
	_, err := r.pool.Exec(ctx,
		`UPDATE agents SET name=$3, description=$4, instructions=$5, provider=$6, model=$7,
		                   temperature=$8, max_tokens=$9, memory_enabled=$10, memory_scope=$11,
		                   memory_save_mode=$12, memory_review_policy=$13,
		                   max_memories=$14, min_relevance_score=$15,
		                   memory_min_importance=$16, memory_dedupe_threshold=$17,
		                   context_retrieval_enabled=$18, max_steps=$19, max_tool_calls=$20,
		                   max_duration_secs=$21, max_history_messages=$22, lazy_tool_loading=$23,
		                   compaction_threshold=$24, compaction_token_threshold=$25,
		                   status=$26, updated_at=NOW()
		 WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		a.ID, a.WorkspaceID, a.Name, a.Description, a.Instructions,
		a.Provider, a.Model, a.Temperature, a.MaxTokens,
		a.MemoryEnabled, a.MemoryScope, a.MemorySaveMode, a.MemoryReviewPolicy,
		a.MaxMemories, a.MinRelevanceScore, a.MemoryMinImportance, a.MemoryDedupeThreshold,
		a.ContextRetrievalEnabled,
		a.MaxSteps, a.MaxToolCalls, a.MaxDurationSecs,
		a.MaxHistoryMessages, a.LazyToolLoading,
		a.CompactionThreshold, a.CompactionTokenThreshold, a.Status)
	return err
}

func normalizeAgentMemoryDefaults(a *domain.Agent) {
	if a.MemoryScope == "" {
		a.MemoryScope = "conversation"
	}
	if a.MemorySaveMode == "" {
		a.MemorySaveMode = "hybrid"
	}
	if a.MemoryReviewPolicy == "" {
		a.MemoryReviewPolicy = "uncertain"
	}
	if a.MaxMemories == 0 {
		a.MaxMemories = 5
	}
	if a.MinRelevanceScore == 0 {
		a.MinRelevanceScore = 0.70
	}
	if a.MemoryMinImportance == 0 {
		a.MemoryMinImportance = 0.70
	}
	if a.MemoryDedupeThreshold == 0 {
		a.MemoryDedupeThreshold = 0.88
	}
}

func (r *AgentRepository) Delete(ctx context.Context, id, workspaceID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE agents SET status='archived', updated_at=NOW() WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		id, workspaceID)
	return err
}
