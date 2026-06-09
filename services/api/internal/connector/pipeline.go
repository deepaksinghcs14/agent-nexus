package connector

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pipeline orchestrates fetch → chunk → embed → upsert for a connector sync.
type Pipeline struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewPipeline(pool *pgxpool.Pool, cfg *config.Config) *Pipeline {
	return &Pipeline{pool: pool, cfg: cfg}
}

// Sync fetches documents via fetchProvider, chunks and embeds them,
// and upserts the results into connector_documents and connector_chunks.
// embedder may be nil — if so, chunks are stored without embeddings (retrieval falls back to recency order).
func (p *Pipeline) Sync(ctx context.Context, connectorID, workspaceID string, fetchProvider Provider, embedder provider.Provider) error {
	repo := repository.NewConnectorRepository(p.pool)

	conn, err := repo.Get(ctx, connectorID)
	if err != nil {
		return fmt.Errorf("pipeline: load connector: %w", err)
	}

	var cfg map[string]any
	if len(conn.Config) > 0 {
		_ = json.Unmarshal(conn.Config, &cfg)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}

	docs, err := fetchProvider.Fetch(ctx, cfg)
	if err != nil {
		return fmt.Errorf("pipeline: fetch documents: %w", err)
	}

	docsFound := len(docs)
	docsIndexed := 0

	for _, doc := range docs {
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(doc.Content)))

		// Look up existing document to reuse its ID (so chunk foreign key stays valid)
		var existingID, existingHash string
		_ = p.pool.QueryRow(ctx,
			`SELECT id::text, content_hash FROM connector_documents WHERE connector_id=$1::uuid AND source_document_id=$2`,
			connectorID, doc.SourceDocumentID).Scan(&existingID, &existingHash)

		if existingHash == hash && existingID != "" {
			docsIndexed++
			continue
		}

		docID := existingID
		if docID == "" {
			docID = uuid.NewString()
		}
		now := time.Now()
		docRecord := &domain.ConnectorDocument{
			ID:               docID,
			ConnectorID:      connectorID,
			WorkspaceID:      workspaceID,
			Source:           doc.Source,
			SourceDocumentID: doc.SourceDocumentID,
			Title:            doc.Title,
			URL:              doc.URL,
			Author:           doc.Author,
			ContentHash:      hash,
			IndexedAt:        &now,
			Metadata:         json.RawMessage(`{}`),
		}
		if err := repo.UpsertDocument(ctx, docRecord); err != nil {
			continue
		}

		// Delete stale chunks before re-indexing
		p.pool.Exec(ctx, `DELETE FROM connector_chunks WHERE document_id=$1::uuid`, docID) //nolint:errcheck

		chunks := chunkText(doc.Content, 2048, 256)
		for i, chunkContent := range chunks {
			chunkID := uuid.NewString()
			if embedder != nil {
				emb, embErr := embedder.Embed(ctx, chunkContent)
				if embErr == nil && len(emb) > 0 {
					vecStr := formatVector(emb)
					p.pool.Exec(ctx, //nolint:errcheck
						`INSERT INTO connector_chunks(id, document_id, chunk_index, content, embedding)
						 VALUES($1::uuid, $2::uuid, $3, $4, $5::vector)
						 ON CONFLICT(document_id, chunk_index) DO UPDATE SET content=$4, embedding=$5::vector`,
						chunkID, docID, i, chunkContent, vecStr)
					continue
				}
			}
			p.pool.Exec(ctx, //nolint:errcheck
				`INSERT INTO connector_chunks(id, document_id, chunk_index, content)
				 VALUES($1::uuid, $2::uuid, $3, $4)
				 ON CONFLICT(document_id, chunk_index) DO UPDATE SET content=$4`,
				chunkID, docID, i, chunkContent)
		}
		docsIndexed++
	}

	p.pool.Exec(ctx, `UPDATE connectors SET status='connected', updated_at=NOW() WHERE id=$1::uuid`, connectorID) //nolint:errcheck

	_ = docsFound // used by caller via sync job record
	_ = docsIndexed
	return nil
}

// chunkText splits text into chunks of approximately `size` runes with `overlap` rune overlap.
func chunkText(text string, size, overlap int) []string {
	runes := []rune(text)
	total := len(runes)
	if total <= size {
		return []string{text}
	}
	var chunks []string
	start := 0
	for start < total {
		end := start + size
		if end > total {
			end = total
		}
		chunks = append(chunks, string(runes[start:end]))
		if end == total {
			break
		}
		next := end - overlap
		if next <= start {
			next = start + 1
		}
		start = next
	}
	return chunks
}

// formatVector formats a []float32 as a pgvector literal: "[1.0,2.0,3.0]"
func formatVector(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
