package handler

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StartEmbeddingBackfill periodically embeds connector chunks that have no
// vector. Chunks end up vectorless whenever the embedder was unreachable at
// sync time (Ollama down, model not pulled yet) — and the sync pipeline never
// revisits unchanged documents, so without this sweep those chunks would stay
// keyword-only forever. Safe to run concurrently with syncs; each update is
// independent and idempotent.
func StartEmbeddingBackfill(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config) {
	embedder := buildEmbedder(cfg)
	if embedder == nil {
		slog.Info("embedding backfill disabled (EMBED_OLLAMA_URL unset)")
		return
	}
	go func() {
		// Immediate sweep at startup, then a slow tick to catch chunks from
		// syncs that ran while the embedder was down.
		sweepEmbeddings(ctx, pool, cfg)
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweepEmbeddings(ctx, pool, cfg)
			}
		}
	}()
}

func sweepEmbeddings(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config) {
	embedder := buildEmbedder(cfg)
	if embedder == nil {
		return
	}
	const batch = 64
	total := 0
	for {
		if ctx.Err() != nil {
			return
		}
		rows, err := pool.Query(ctx, `
			SELECT id::text, content FROM connector_chunks
			WHERE embedding IS NULL AND content <> ''
			ORDER BY id LIMIT $1`, batch)
		if err != nil {
			slog.Warn("embedding backfill query failed", "error", err)
			return
		}
		type chunk struct{ id, content string }
		var chunks []chunk
		for rows.Next() {
			var c chunk
			if rows.Scan(&c.id, &c.content) == nil {
				chunks = append(chunks, c)
			}
		}
		rows.Close()
		if len(chunks) == 0 {
			break
		}
		for _, c := range chunks {
			vec, err := embedder.Embed(ctx, c.content)
			if err != nil || len(vec) == 0 {
				// Embedder unreachable — stop quietly; the next tick retries.
				if total > 0 {
					slog.Info("embedding backfill paused", "embedded", total, "error", err)
				}
				return
			}
			if _, err := pool.Exec(ctx,
				`UPDATE connector_chunks SET embedding=$2::vector WHERE id=$1::uuid`,
				c.id, vecLiteral(vec)); err != nil {
				slog.Warn("embedding backfill update failed", "chunk_id", c.id, "error", err)
				return
			}
			total++
		}
	}
	if total > 0 {
		slog.Info("embedding backfill complete", "embedded", total)
	}
}

func vecLiteral(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
