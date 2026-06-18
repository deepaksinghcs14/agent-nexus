package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Engine retrieves and stores memories for an agent run.
type Engine struct {
	memories *repository.MemoryRepository
}

func NewEngine(pool *pgxpool.Pool) *Engine {
	return &Engine{memories: repository.NewMemoryRepository(pool)}
}

type Candidate struct {
	Content         string          `json:"content"`
	ImportanceScore float64         `json:"importance_score"`
	Reason          string          `json:"reason"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
}

type SaveRequest struct {
	Agent          *domain.Agent
	WorkspaceID    string
	UserID         string
	ConversationID string
	RunID          string
	Source         string
	Candidate      Candidate
	Embedding      []float32
}

type SaveResult struct {
	Saved           bool    `json:"saved"`
	Status          string  `json:"status,omitempty"`
	MemoryID        string  `json:"memory_id,omitempty"`
	DroppedReason   string  `json:"dropped_reason,omitempty"`
	DuplicateScore  float64 `json:"duplicate_score,omitempty"`
	ImportanceScore float64 `json:"importance_score,omitempty"`
}

func (e *Engine) Retrieve(ctx context.Context, agentID, workspaceID, conversationID string, embedding []float32, limit int, minScore float64) ([]domain.Memory, error) {
	if limit <= 0 {
		limit = 8
	}
	return e.memories.Search(ctx, workspaceID, agentID, conversationID, embedding, limit, minScore)
}

func (e *Engine) Store(ctx context.Context, m *domain.Memory, embedding []float32) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	return e.memories.Store(ctx, m, embedding)
}

func (e *Engine) Summarise(ctx context.Context, runID string) error {
	return nil
}

func (e *Engine) SaveCandidate(ctx context.Context, req SaveRequest) (SaveResult, error) {
	if req.Agent == nil {
		return SaveResult{}, fmt.Errorf("memory: agent is required")
	}
	content := strings.TrimSpace(req.Candidate.Content)
	if content == "" {
		return SaveResult{Saved: false, DroppedReason: "empty_content"}, nil
	}
	if len(content) > 2000 {
		content = content[:2000]
	}

	minImportance := req.Agent.MemoryMinImportance
	if minImportance <= 0 {
		minImportance = 0.70
	}
	importance := req.Candidate.ImportanceScore
	if importance <= 0 {
		importance = 0.70
	}
	if importance < minImportance {
		return SaveResult{Saved: false, DroppedReason: "below_min_importance", ImportanceScore: importance}, nil
	}

	dedupeThreshold := req.Agent.MemoryDedupeThreshold
	if dedupeThreshold <= 0 {
		dedupeThreshold = 0.88
	}
	agentID := req.Agent.ID
	conversationID := req.ConversationID
	if req.Agent.MemoryScope == string(domain.MemoryScopeWorkspace) {
		agentID = ""
	}
	if req.Agent.MemoryScope != string(domain.MemoryScopeConversation) {
		conversationID = ""
	}
	duplicate, duplicateScore, err := e.memories.HasSimilar(ctx, req.WorkspaceID, req.Agent.ID, req.ConversationID, content, req.Embedding, dedupeThreshold)
	if err != nil {
		return SaveResult{}, err
	}
	if duplicate {
		return SaveResult{Saved: false, DroppedReason: "duplicate", DuplicateScore: duplicateScore, ImportanceScore: importance}, nil
	}

	status := "active"
	switch req.Agent.MemoryReviewPolicy {
	case "all":
		status = "pending"
	case "uncertain", "":
		if importance < 0.85 || (duplicateScore > 0 && duplicateScore >= dedupeThreshold-0.05) {
			status = "pending"
		}
	}

	metadata := req.Candidate.Metadata
	if len(metadata) == 0 {
		b, _ := json.Marshal(map[string]any{"reason": req.Candidate.Reason})
		metadata = b
	}
	mem := &domain.Memory{
		ID:              uuid.NewString(),
		WorkspaceID:     req.WorkspaceID,
		AgentID:         agentID,
		UserID:          req.UserID,
		ConversationID:  conversationID,
		Scope:           domain.MemoryScope(req.Agent.MemoryScope),
		Content:         content,
		RelevanceScore:  importance,
		ImportanceScore: importance,
		Status:          status,
		SaveSource:      req.Source,
		SourceRunID:     req.RunID,
		Metadata:        metadata,
	}
	if mem.Scope == "" {
		mem.Scope = domain.MemoryScopeAgent
	}
	if err := e.Store(ctx, mem, req.Embedding); err != nil {
		return SaveResult{}, err
	}
	return SaveResult{Saved: true, Status: status, MemoryID: mem.ID, DuplicateScore: duplicateScore, ImportanceScore: importance}, nil
}

func ExtractCandidates(ctx context.Context, llm provider.Provider, model, userInput, assistantReply string) ([]Candidate, error) {
	if llm == nil || strings.TrimSpace(userInput) == "" || strings.TrimSpace(assistantReply) == "" {
		return nil, nil
	}
	stream, err := llm.Complete(ctx, provider.CompletionRequest{
		Model: model,
		Messages: []provider.Message{
			{Role: "system", Content: "Extract only durable long-term memories from the conversation. Return JSON only: {\"memories\":[{\"content\":\"...\",\"importance_score\":0.0,\"reason\":\"...\"}]}. Return an empty memories array for transient facts, secrets, credentials, or one-off requests."},
			{Role: "user", Content: "User:\n" + userInput + "\n\nAssistant:\n" + assistantReply},
		},
		Temperature: 0,
		MaxTokens:   800,
		Stream:      true,
	})
	if err != nil {
		return nil, err
	}
	var raw string
	for event := range stream {
		switch event.Type {
		case provider.EventDelta:
			raw += event.Delta
		case provider.EventError:
			return nil, event.Error
		}
	}
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	var parsed struct {
		Memories []Candidate `json:"memories"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		return nil, err
	}
	return parsed.Memories, nil
}
