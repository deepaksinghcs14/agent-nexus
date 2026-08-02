package context

import (
	"context"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Chunk is a retrieved piece of indexed connector content.
type Chunk struct {
	ID      string
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

// Retrieve blends semantic (embedding) and keyword search rather than
// treating them as strictly either/or: when an embedding is available, both
// run and the two independently-ranked lists are merged by reciprocal rank
// fusion (see fuseRankings) instead of one early-returning before the other
// is ever tried. With no embedding, this is keyword search alone — fusing a
// single list is a no-op on ordering, just relabels Score as an RRF value.
func (r *Retriever) Retrieve(ctx context.Context, workspaceID string, connectorIDs []string, embedding []float32, limit int, minScore float64, query string) ([]Chunk, error) {
	if limit <= 0 {
		limit = 8
	}

	var semanticChunks []Chunk
	if len(embedding) > 0 {
		var err error
		semanticChunks, err = r.semanticSearch(ctx, workspaceID, connectorIDs, embedding, limit, minScore)
		if err != nil {
			return nil, err
		}
	}

	keywordChunks, err := r.keywordSearch(ctx, workspaceID, connectorIDs, limit, query)
	if err != nil {
		return nil, err
	}

	fused := fuseRankings(semanticChunks, keywordChunks)
	if len(fused) > limit {
		fused = fused[:limit]
	}
	return fused, nil
}

// semanticSearch ranks by cosine similarity, filtered to minScore.
func (r *Retriever) semanticSearch(ctx context.Context, workspaceID string, connectorIDs []string, embedding []float32, limit int, minScore float64) ([]Chunk, error) {
	vecStr := formatVec(embedding)
	rows, err := r.pool.Query(ctx,
		`SELECT cc.id::text, cc.content, cd.title, cd.url, cd.source,
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
	if err != nil {
		// A vector-dimension mismatch or similar query error degrades to
		// keyword-only rather than failing the whole retrieval — but that
		// degradation is worth knowing about, not silent.
		slog.Warn("semantic search failed; continuing with keyword results only", "workspace_id", workspaceID, "error", err)
		return nil, nil //nolint:nilerr
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.Content, &c.Title, &c.URL, &c.Source, &c.Score); err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

// keywordSearch ranks by full-text match (to_tsquery @@), OR-combined across
// query terms so multi-word queries still recall. A title ILIKE substring
// match is a true fallback — only consulted when the tsquery match returns
// nothing at all — rather than a disjunct inside the ranked query: an
// ILIKE-only hit has no real ts_rank_cd (NULL, coerced to score 0) and used
// to still enter ranking and get returned as if it were a relevance match.
func (r *Retriever) keywordSearch(ctx context.Context, workspaceID string, connectorIDs []string, limit int, query string) ([]Chunk, error) {
	chunks, err := r.tsvectorMatch(ctx, workspaceID, connectorIDs, limit, query)
	if err != nil {
		return nil, err
	}
	if len(chunks) > 0 {
		return chunks, nil
	}
	return r.keywordQuery(ctx, workspaceID, connectorIDs, limit, `cd.title ILIKE '%' || $4 || '%'`, query)
}

// tsvectorMatch is a UNION of two single-table-indexed queries (content,
// title) rather than one query with an OR spanning both — Postgres cannot
// push an OR that mixes columns from two joined tables down to either
// side's index; it only does that when each disjunct is isolated in its
// own single-table WHERE clause, which UNION provides. Verified via EXPLAIN
// with enable_seqscan off: the OR form never touches
// connector_chunks_content_fts_idx / connector_documents_title_fts_idx
// (migration 078); this form uses both as Bitmap Index Scans. Both branches
// select the same overall ranking expression so a chunk matching both sides
// collapses to one row via plain UNION (not UNION ALL) instead of appearing
// twice.
func (r *Retriever) tsvectorMatch(ctx context.Context, workspaceID string, connectorIDs []string, limit int, query string) ([]Chunk, error) {
	tsq := orTSQuery(query)
	rows, err := r.pool.Query(ctx, `
		SELECT id, content, title, url, source, score FROM (
			SELECT cc.id::text AS id, cc.content AS content, cd.title AS title, cd.url AS url, cd.source AS source,
			       ts_rank_cd(to_tsvector('english', cc.content || ' ' || cd.title), to_tsquery('english', $4)) AS score
			FROM connector_chunks cc
			JOIN connector_documents cd ON cd.id = cc.document_id
			JOIN connectors c ON c.id = cd.connector_id
			WHERE cd.workspace_id = $1::uuid AND c.status IN ('connected', 'syncing') AND c.id = ANY($2::uuid[])
			  AND to_tsvector('english', cc.content) @@ to_tsquery('english', $4)
			UNION
			SELECT cc.id::text AS id, cc.content AS content, cd.title AS title, cd.url AS url, cd.source AS source,
			       ts_rank_cd(to_tsvector('english', cc.content || ' ' || cd.title), to_tsquery('english', $4)) AS score
			FROM connector_chunks cc
			JOIN connector_documents cd ON cd.id = cc.document_id
			JOIN connectors c ON c.id = cd.connector_id
			WHERE cd.workspace_id = $1::uuid AND c.status IN ('connected', 'syncing') AND c.id = ANY($2::uuid[])
			  AND to_tsvector('english', cd.title) @@ to_tsquery('english', $4)
		) matched
		ORDER BY score DESC, title
		LIMIT $3`,
		workspaceID, uuidArray(connectorIDs), limit, tsq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.Content, &c.Title, &c.URL, &c.Source, &c.Score); err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

func (r *Retriever) keywordQuery(ctx context.Context, workspaceID string, connectorIDs []string, limit int, matchClause, query string) ([]Chunk, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT cc.id::text, cc.content, cd.title, cd.url, cd.source,
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
		   AND (`+matchClause+`)
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
		if err := rows.Scan(&c.ID, &c.Content, &c.Title, &c.URL, &c.Source, &c.Score); err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

// rrfK is the standard reciprocal-rank-fusion damping constant. RRF only
// needs each list's rank order, not its raw score — which is what makes it
// the right tool here: cosine similarity ([0,1]) and ts_rank_cd (unbounded)
// aren't on a comparable scale, and normalizing one onto the other's range
// would be an arbitrary choice RRF avoids entirely.
const rrfK = 60.0

// fuseRankings merges any number of independently-ranked (best-first) chunk
// lists into one, deduped by ID, via reciprocal rank fusion:
// score = Σ 1/(rrfK + rank) over every list a chunk appears in (rank is
// 1-indexed). The fused Score overwrites whichever raw score the chunk
// arrived with — that's the value that actually decided the final order.
func fuseRankings(lists ...[]Chunk) []Chunk {
	scores := map[string]float64{}
	byID := map[string]Chunk{}
	var order []string
	for _, list := range lists {
		for i, c := range list {
			if _, seen := byID[c.ID]; !seen {
				order = append(order, c.ID)
				byID[c.ID] = c
			}
			scores[c.ID] += 1 / (rrfK + float64(i+1))
		}
	}
	fused := make([]Chunk, 0, len(order))
	for _, id := range order {
		c := byID[id]
		c.Score = scores[id]
		fused = append(fused, c)
	}
	sort.Slice(fused, func(i, j int) bool { return fused[i].Score > fused[j].Score })
	return fused
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
