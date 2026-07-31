package handler

// Regression test for a real Postgres GROUP BY bug in ListRepoCatalog's SQL:
// a correlated EXISTS subquery referenced rc.workspace_id, which isn't in the
// GROUP BY list, so Postgres rejected the query outright ("column
// rc.workspace_id must appear in the GROUP BY clause"). go build/go vet and
// the plain unit tests never catch this class of bug because they never
// execute the SQL against a real database — only this suite does.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
)

func TestListRepoCatalogQueryIsValid(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	uid := uuid.NewString()
	ws := uuid.NewString()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture insert failed: %v\n%s", err, sql)
		}
	}
	mustExec(`INSERT INTO users(id,email,password_hash) VALUES($1::uuid,$2,'x')`, uid, uid+"@test.local")
	mustExec(`INSERT INTO workspaces(id,name,owner_id) VALUES($1::uuid,$2,$3::uuid)`, ws, "repo-catalog-test-"+ws[:8], uid)
	mustExec(`INSERT INTO repo_catalog(workspace_id,repo,default_branch,sessions_enabled,repo_map)
		VALUES($1::uuid,'acme/widgets','main',true,'- cmd/ entrypoint')`, ws)

	h := NewWorkspaceHandler(pool, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/repo-catalog", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyWorkspaceID, ws))
	w := httptest.NewRecorder()

	h.ListRepoCatalog(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		Data []repoCatalogEntry `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v\n%s", err, w.Body.String())
	}
	if len(out.Data) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(out.Data))
	}
	if out.Data[0].Repo != "acme/widgets" || out.Data[0].RepoMap == "" {
		t.Fatalf("unexpected entry: %+v", out.Data[0])
	}
	if out.Data[0].MapGenerating {
		t.Fatalf("expected map_generating=false with no in-flight run, got true")
	}
}
