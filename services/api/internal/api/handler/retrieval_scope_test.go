package handler

// An agent with retrieval enabled and no connectors attached must retrieve
// NOTHING. uuidArray bound SQL NULL for an empty list and the query's
// "$2::uuid[] IS NULL OR" disjunct then dropped the connector filter, so such
// an agent got keyword hits from every connector in its workspace.

import (
	"context"
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
