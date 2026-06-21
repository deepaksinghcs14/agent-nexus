package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/runtime/memory"
	"github.com/jackc/pgx/v5/pgxpool"
)

func memoryEmbedding(ctx context.Context, llm provider.Provider, text string) []float32 {
	if llm == nil || text == "" {
		return nil
	}
	embedding, err := llm.Embed(ctx, text)
	if err != nil {
		return nil
	}
	return embedding
}

func appendMemoryContext(messages []provider.Message, memories []domain.Memory, active map[string]bool) int {
	if len(messages) == 0 || len(memories) == 0 {
		return 0
	}
	blocks := make([]string, 0, len(memories))
	for _, m := range memories {
		if active != nil {
			if active[m.ID] {
				continue
			}
			active[m.ID] = true
		}
		content := strings.TrimSpace(m.Content)
		if len(content) > 300 {
			content = content[:300] + "…"
		}
		blocks = append(blocks, fmt.Sprintf("[Memory]\nscope=%s relevance=%.2f importance=%.2f\n%s",
			m.Scope, m.RelevanceScore, m.ImportanceScore, content))
	}
	if len(blocks) == 0 {
		return 0
	}
	messages[0].Content += "\n\n" + strings.Join(blocks, "\n\n")
	return len(blocks)
}

func shouldRunMemoryExtractor(a *domain.Agent, explicitSaveCalled bool) bool {
	if a == nil || !a.MemoryEnabled {
		return false
	}
	switch a.MemorySaveMode {
	case "extractor":
		return true
	case "hybrid", "":
		return !explicitSaveCalled
	default:
		return false
	}
}

func runMemoryExtractor(ctx context.Context, pool *pgxpool.Pool, llm provider.Provider, a *domain.Agent, ws, uid, convID, runID, input, reply string) (int, error) {
	candidates, err := memory.ExtractCandidates(ctx, llm, a.Model, input, reply)
	if err != nil {
		return 0, err
	}
	engine := memory.NewEngine(pool)
	saved := 0
	for _, candidate := range candidates {
		result, err := engine.SaveCandidate(ctx, memory.SaveRequest{
			Agent:          a,
			WorkspaceID:    ws,
			UserID:         uid,
			ConversationID: convID,
			RunID:          runID,
			Source:         "extractor",
			Candidate:      candidate,
			Embedding:      memoryEmbedding(ctx, llm, candidate.Content),
		})
		if err != nil {
			return saved, err
		}
		if result.Saved {
			saved++
		}
	}
	return saved, nil
}
