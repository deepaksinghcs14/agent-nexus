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
	"github.com/deepaksingh/agent-nexus/services/api/internal/connector/syncstate"
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

// Sync fetches documents via fetchProvider, chunks and embeds them, and upserts the results
// into connector_documents and connector_chunks.
//
// If fetchProvider also implements StreamProvider the streaming path is taken, which gives:
//   - Live progress updates (documents_found/indexed written to DB every few docs)
//   - Checkpoint-based resumption after a pod restart
//
// embedder may be nil — if so, chunks are stored without embeddings.
// rep must not be nil; use syncstate.New() before calling Sync.
func (p *Pipeline) Sync(
	ctx context.Context,
	connectorID, workspaceID string,
	fetchProvider Provider,
	embedder provider.Provider,
	rep *syncstate.Reporter,
) error {
	log := rep.Logger()
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

	// emit is the shared per-document processor used by both batch and streaming paths.
	emit := func(doc Document) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(doc.Content)))

		// Reuse existing document ID so chunk foreign keys survive re-syncs.
		var existingID, existingHash string
		_ = p.pool.QueryRow(ctx,
			`SELECT id::text, content_hash FROM connector_documents WHERE connector_id=$1::uuid AND source_document_id=$2`,
			connectorID, doc.SourceDocumentID,
		).Scan(&existingID, &existingHash)

		if existingHash == hash && existingID != "" {
			log.Debug("document unchanged, skipping", "source_doc_id", doc.SourceDocumentID)
			rep.Inc(1, 0) // found but not re-indexed
			return nil
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
			log.Warn("document upsert failed", "source_doc_id", doc.SourceDocumentID, "error", err)
			return nil // continue with next doc on upsert error
		}

		// Remove stale chunks before re-indexing.
		p.pool.Exec(ctx, `DELETE FROM connector_chunks WHERE document_id=$1::uuid`, docID) //nolint:errcheck

		chunks := chunkText(doc.Content, 2048, 256)
		for i, chunkContent := range chunks {
			chunkID := uuid.NewString()
			if embedder != nil {
				emb, embErr := embedder.Embed(ctx, chunkContent)
				if embErr == nil && len(emb) > 0 {
					vecStr := formatVector(emb)
					p.pool.Exec(ctx, //nolint:errcheck
						`INSERT INTO connector_chunks(id,document_id,chunk_index,content,embedding)
						 VALUES($1::uuid,$2::uuid,$3,$4,$5::vector)
						 ON CONFLICT(document_id,chunk_index) DO UPDATE SET content=$4,embedding=$5::vector`,
						chunkID, docID, i, chunkContent, vecStr)
					continue
				}
			}
			p.pool.Exec(ctx, //nolint:errcheck
				`INSERT INTO connector_chunks(id,document_id,chunk_index,content)
				 VALUES($1::uuid,$2::uuid,$3,$4)
				 ON CONFLICT(document_id,chunk_index) DO UPDATE SET content=$4`,
				chunkID, docID, i, chunkContent)
		}

		log.Debug("document indexed", "title", doc.Title, "chunks", len(chunks))
		rep.Inc(1, 1)
		return nil
	}

	// Prefer StreamProvider — gives live progress and checkpoint resumption.
	if sp, ok := fetchProvider.(StreamProvider); ok {
		cp := syncstate.LoadLastCheckpoint(ctx, p.pool, connectorID)
		hasCheckpoint := len(cp) > 2 // more than just '{}'
		log.Info("starting streaming sync", "has_checkpoint", hasCheckpoint)

		opts := FetchOpts{
			Checkpoint:     cp,
			SaveCheckpoint: rep.Checkpoint,
		}
		if err := sp.FetchStream(ctx, cfg, opts, emit); err != nil {
			return fmt.Errorf("pipeline: fetch stream: %w", err)
		}
		log.Info("streaming sync complete", "found", rep.Found(), "indexed", rep.Indexed())
		return nil
	}

	// Fall back to batch provider (e.g. filesystem — small, fast, no need for streaming).
	log.Info("starting batch sync")
	docs, err := fetchProvider.Fetch(ctx, cfg)
	if err != nil {
		return fmt.Errorf("pipeline: fetch documents: %w", err)
	}
	log.Info("fetched documents", "count", len(docs))
	for _, doc := range docs {
		if err := emit(doc); err != nil {
			log.Warn("emit error", "error", err)
			if ctx.Err() != nil {
				break
			}
		}
	}
	log.Info("batch sync complete", "found", rep.Found(), "indexed", rep.Indexed())
	return nil
}

// chunkText splits text into chunks of approximately size runes with overlap rune overlap.
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
