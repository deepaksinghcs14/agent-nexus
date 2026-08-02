package handler

// An agent with retrieval enabled and no connectors attached must retrieve
// NOTHING. uuidArray bound SQL NULL for an empty list and the query's
// "$2::uuid[] IS NULL OR" disjunct then dropped the connector filter, so such
// an agent got keyword hits from every connector in its workspace.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	contextretrieval "github.com/deepaksingh/agent-nexus/services/api/internal/runtime/context"
)

func TestRetrieveWithNoConnectorsReturnsNothing(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tn := newTenant(t, pool)

	connID, docID := uuid.NewString(), uuid.NewString()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture insert failed: %v\n%s", err, sql)
		}
	}
	mustExec(`INSERT INTO connectors(id,workspace_id,name,provider,status,created_by)
	          VALUES($1::uuid,$2::uuid,'Secret Docs','filesystem','connected',$3::uuid)`, connID, tn.wsID, tn.userID)
	mustExec(`INSERT INTO connector_documents(id,connector_id,workspace_id,source,source_document_id,title)
	          VALUES($1::uuid,$2::uuid,$3::uuid,'filesystem','doc-1','Quarterly revenue')`, docID, connID, tn.wsID)
	mustExec(`INSERT INTO connector_chunks(document_id,chunk_index,content)
	          VALUES($1::uuid,0,'revenue was 4.2 million')`, docID)

	r := contextretrieval.NewRetriever(pool)

	// nil embedding → keyword branch, the one the leak actually took.
	got, err := r.Retrieve(ctx, tn.wsID, nil, nil, 8, 0, "revenue")
	if err != nil {
		t.Fatalf("retrieve with no connectors: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("no connectors attached: got %d chunks, want 0", len(got))
	}

	// Positive control: without it this passes even if the filter matches nothing ever.
	got, err = r.Retrieve(ctx, tn.wsID, []string{connID}, nil, 8, 0, "revenue")
	if err != nil {
		t.Fatalf("retrieve with connector: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("connector attached: got %d chunks, want 1", len(got))
	}
}

// embeddingLiteral builds a 768-dim pgvector literal with 1.0 at axis and 0
// elsewhere — two chunks sharing an axis are "semantically identical";
// orthogonal axes have cosine similarity 0, reliably below any positive
// minScore.
func embeddingLiteral(axis int) string {
	dims := make([]string, 768)
	for i := range dims {
		dims[i] = "0"
	}
	dims[axis] = "1"
	return "[" + strings.Join(dims, ",") + "]"
}

// Retrieve used to treat semantic and keyword search as strictly either/or:
// one semantic hit early-returned before keyword ever ran, and minScore was
// silently unapplied in the keyword branch. This proves the two are
// genuinely fused (not semantic-with-keyword-as-tiebreak) and that a
// semantic-minScore-filtered chunk still surfaces via a real keyword match.
func TestRetrieveFusesSemanticAndKeywordResults(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tn := newTenant(t, pool)

	connID := uuid.NewString()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture insert failed: %v\n%s", err, sql)
		}
	}
	mustExec(`INSERT INTO connectors(id,workspace_id,name,provider,status,created_by)
	          VALUES($1::uuid,$2::uuid,'Fusion Docs','filesystem','connected',$3::uuid)`, connID, tn.wsID, tn.userID)

	insertChunk := func(docID, title, content string, embedAxis int) string {
		t.Helper()
		mustExec(`INSERT INTO connector_documents(id,connector_id,workspace_id,source,source_document_id,title)
		          VALUES($1::uuid,$2::uuid,$3::uuid,'filesystem',$4,$5)`, docID, connID, tn.wsID, docID, title)
		var chunkID string
		if err := pool.QueryRow(ctx,
			`INSERT INTO connector_chunks(document_id,chunk_index,content,embedding) VALUES($1::uuid,0,$2,$3::vector) RETURNING id::text`,
			docID, content, embeddingLiteral(embedAxis)).Scan(&chunkID); err != nil {
			t.Fatalf("insert chunk: %v", err)
		}
		return chunkID
	}

	// Close to the query embedding (axis 0) AND matches the keyword "unicorn".
	bothID := insertChunk(uuid.NewString(), "Both", "the magic unicorn galloped", 0)
	// Close to the query embedding but no keyword match.
	semanticOnlyID := insertChunk(uuid.NewString(), "SemanticOnly", "completely unrelated filler text", 0)
	// Orthogonal embedding (cosine 0, fails minScore) but matches the keyword.
	keywordOnlyID := insertChunk(uuid.NewString(), "KeywordOnly", "a lovely unicorn story here", 1)

	queryEmbedding := make([]float32, 768)
	queryEmbedding[0] = 1

	got, err := contextretrieval.NewRetriever(pool).Retrieve(ctx, tn.wsID, []string{connID}, queryEmbedding, 8, 0.5, "unicorn")
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}

	ids := make(map[string]int, len(got))
	for i, c := range got {
		ids[c.ID] = i
	}
	if len(got) != 3 {
		t.Fatalf("got %d chunks, want 3 (both + semantic-only + keyword-only): %+v", len(got), got)
	}
	if _, ok := ids[semanticOnlyID]; !ok {
		t.Fatal("semantic-only chunk missing — semantic results were dropped once a keyword match existed")
	}
	if _, ok := ids[keywordOnlyID]; !ok {
		t.Fatal("keyword-only chunk missing — a real keyword match was dropped by the semantic minScore filter it was never subject to")
	}
	if ids[bothID] != 0 {
		t.Fatalf("chunk matching both signals ranked at position %d, want 0 (fused first): %s", ids[bothID], fmt.Sprintf("%+v", got))
	}
}

// keywordSearch's match clause used to be a single cross-table tsvector
// expression (to_tsvector(cc.content || ' ' || cd.title)) — no functional
// index can cover an expression spanning two tables, so every keyword query
// seq-scanned connector_chunks (migration 078 adds indexes; tsvectorMatch's
// UNION of two single-table predicates is what actually makes them
// reachable — a same-query OR spanning the join, verified via EXPLAIN,
// never touches either index regardless of table size). This proves the
// UNION still matches on either side.
func TestKeywordSearchMatchesContentOrTitle(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tn := newTenant(t, pool)

	connID := uuid.NewString()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture insert failed: %v\n%s", err, sql)
		}
	}
	mustExec(`INSERT INTO connectors(id,workspace_id,name,provider,status,created_by)
	          VALUES($1::uuid,$2::uuid,'Split Predicate Docs','filesystem','connected',$3::uuid)`, connID, tn.wsID, tn.userID)

	contentOnlyID, titleOnlyID := uuid.NewString(), uuid.NewString()
	mustExec(`INSERT INTO connector_documents(id,connector_id,workspace_id,source,source_document_id,title)
	          VALUES($1::uuid,$2::uuid,$3::uuid,'filesystem',$1,'Unrelated Title')`, contentOnlyID, connID, tn.wsID)
	mustExec(`INSERT INTO connector_chunks(document_id,chunk_index,content) VALUES($1::uuid,0,'the secret keyword is quokka')`, contentOnlyID)

	mustExec(`INSERT INTO connector_documents(id,connector_id,workspace_id,source,source_document_id,title)
	          VALUES($1::uuid,$2::uuid,$3::uuid,'filesystem',$1,'quokka field guide')`, titleOnlyID, connID, tn.wsID)
	mustExec(`INSERT INTO connector_chunks(document_id,chunk_index,content) VALUES($1::uuid,0,'nothing relevant in here')`, titleOnlyID)

	got, err := contextretrieval.NewRetriever(pool).Retrieve(ctx, tn.wsID, []string{connID}, nil, 8, 0, "quokka")
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d chunks, want 2 (content-only match + title-only match): %+v", len(got), got)
	}
}

// A prior rewrite of this file (the RRF fusion commit) bound the match
// clause's to_tsquery(...) to the RAW query string instead of the
// OR-combined form orTSQuery produces — to_tsquery rejects free text with
// more than one term ("multi word query" isn't valid tsquery syntax), so
// every multi-word keyword search returned a Postgres error. Single-word
// queries (used by every other test in this file) don't exercise the
// to_tsquery parser's operator requirement, which is how this shipped
// unnoticed.
func TestKeywordSearchHandlesMultiWordQuery(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tn := newTenant(t, pool)

	connID, docID := uuid.NewString(), uuid.NewString()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture insert failed: %v\n%s", err, sql)
		}
	}
	mustExec(`INSERT INTO connectors(id,workspace_id,name,provider,status,created_by)
	          VALUES($1::uuid,$2::uuid,'Multi Word Docs','filesystem','connected',$3::uuid)`, connID, tn.wsID, tn.userID)
	mustExec(`INSERT INTO connector_documents(id,connector_id,workspace_id,source,source_document_id,title)
	          VALUES($1::uuid,$2::uuid,$3::uuid,'filesystem',$1,'Rate limiting guide')`, docID, connID, tn.wsID)
	mustExec(`INSERT INTO connector_chunks(document_id,chunk_index,content) VALUES($1::uuid,0,'configure webhook rate limiting here')`, docID)

	got, err := contextretrieval.NewRetriever(pool).Retrieve(ctx, tn.wsID, []string{connID}, nil, 8, 0, "webhook rate limiting")
	if err != nil {
		t.Fatalf("retrieve with a multi-word query errored: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d chunks, want 1", len(got))
	}
}
