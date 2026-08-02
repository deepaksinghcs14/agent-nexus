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

func (r *Retriever) Retrieve(ctx context.Context, workspaceID string, connectorIDs []string, embedding []float32, limit int, minScore float64, query string) ([]Chunk, error) {
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
			   AND c.id = ANY($2::uuid[])
			   AND cc.embedding IS NOT NULL
			   AND 1 - (cc.embedding <=> $4::vector) >= $5
			 ORDER BY cc.embedding <=> $4::vector
			 LIMIT $3`,
			workspaceID, uuidArray(connectorIDs), limit, vecStr, minScore)
		// If semantic search returned 0 results (e.g. embeddings not yet populated),
		// fall through to keyword search below.
		if err == nil {
			var semanticChunks []Chunk
			for rows.Next() {
				var c Chunk
				if scanErr := rows.Scan(&c.Content, &c.Title, &c.URL, &c.Source, &c.Score); scanErr != nil {
					rows.Close()
					return nil, scanErr
				}
				semanticChunks = append(semanticChunks, c)
			}
			rows.Close()
			if rowErr := rows.Err(); rowErr != nil {
				return nil, rowErr
			}
			if len(semanticChunks) > 0 {
				return semanticChunks, nil
			}
			// Zero semantic results — fall through to keyword search.
		}
	}

	// No embedding or semantic search returned nothing — use full-text + ILIKE
	// keyword search. Terms are OR-combined (any-match) so multi-word queries
	// still recall; ts_rank_cd naturally ranks chunks matching more terms higher.
	rows, err = r.pool.Query(ctx,
		`SELECT cc.content, cd.title, cd.url, cd.source,
		        COALESCE(ts_rank_cd(
		            to_tsvector('english', cc.content || ' ' || cd.title),
		            to_tsquery('english', $5)
		        ), 0)::float AS score
		 FROM connector_chunks cc
		 JOIN connector_documents cd ON cd.id = cc.document_id
		 JOIN connectors c ON c.id = cd.connector_id
		 WHERE cd.workspace_id = $1::uuid
		   AND c.status IN ('connected', 'syncing')
		   AND c.id = ANY($2::uuid[])
		   AND (
		       to_tsvector('english', cc.content || ' ' || cd.title) @@ to_tsquery('english', $5)
		       OR cd.title ILIKE '%' || $4 || '%'
		   )
		 ORDER BY score DESC, cd.title
		 LIMIT $3`,
		workspaceID, uuidArray(connectorIDs), limit, query, orTSQuery(query))
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

// orTSQuery converts free text into a safe OR-combined tsquery expression:
// "webhook rate limiting" → "webhook | rate | limiting". Non-alphanumeric
// characters SPLIT terms (they must not be silently stripped: to_tsvector
// tokenizes "aadhaar-qr-scanner-sdk" into its parts, so the glued-together
// "aadhaarqrscannersdk" would match nothing) and duplicates are dropped.
func orTSQuery(query string) string {
	var terms []string
	seen := map[string]bool{}
	var b strings.Builder
	flush := func() {
		if b.Len() > 1 && !seen[b.String()] {
			seen[b.String()] = true
			terms = append(terms, b.String())
		}
		b.Reset()
	}
	for _, r := range strings.ToLower(query) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	if len(terms) == 0 {
		return "zzzznomatch"
	}
	return strings.Join(terms, " | ")
}

// uuidArray must never return nil: pgx binds a nil `any` as SQL NULL, and
// `id = ANY(NULL)` evaluates to NULL (not false) — the "no connectors"
// case would then match every row instead of none.
func uuidArray(values []string) string {
	return "{" + strings.Join(values, ",") + "}"
}

func formatVec(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
