package context

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Chunk is a retrieved piece of indexed connector content.
type Chunk struct {
	Content string
	Title   string
	URL     string
	Source  string
	Score   float64
}

// Retriever queries connector_chunks by embedding similarity.
type Retriever struct {
	pool *pgxpool.Pool
}

func NewRetriever(pool *pgxpool.Pool) *Retriever { return &Retriever{pool: pool} }

func (r *Retriever) Retrieve(ctx context.Context, workspaceID string, connectorIDs []string, embedding []float32, limit int) ([]Chunk, error) {
	if limit <= 0 {
		limit = 8
	}

	var rows interface {
		Next() bool
		Scan(...any) error
		Close()
		Err() error
	}
	var err error

	if len(embedding) > 0 {
		vecStr := formatVec(embedding)
		rows, err = r.pool.Query(ctx,
			`SELECT cc.content, cd.title, cd.url, cd.source,
			        1 - (cc.embedding <=> $4::vector) AS score
			 FROM connector_chunks cc
			 JOIN connector_documents cd ON cd.id = cc.document_id
			 JOIN connectors c ON c.id = cd.connector_id
			 WHERE cd.workspace_id = $1::uuid
			   AND c.status IN ('connected', 'syncing')
			   AND ($2::uuid[] IS NULL OR c.id = ANY($2::uuid[]))
			   AND cc.embedding IS NOT NULL
			 ORDER BY cc.embedding <=> $4::vector
			 LIMIT $3`,
			workspaceID, uuidArray(connectorIDs), limit, vecStr)
	} else {
		rows, err = r.pool.Query(ctx,
			`SELECT cc.content, cd.title, cd.url, cd.source, 0::float
			 FROM connector_chunks cc
			 JOIN connector_documents cd ON cd.id = cc.document_id
			 JOIN connectors c ON c.id = cd.connector_id
			 WHERE cd.workspace_id = $1::uuid
			   AND c.status IN ('connected', 'syncing')
			   AND ($2::uuid[] IS NULL OR c.id = ANY($2::uuid[]))
			 ORDER BY cc.created_at DESC
			 LIMIT $3`,
			workspaceID, uuidArray(connectorIDs), limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.Content, &c.Title, &c.URL, &c.Source, &c.Score); err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

func uuidArray(values []string) any {
	if len(values) == 0 {
		return nil
	}
	return "{" + strings.Join(values, ",") + "}"
}

func formatVec(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
