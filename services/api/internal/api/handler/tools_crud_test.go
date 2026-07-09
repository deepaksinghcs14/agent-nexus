package handler

// HTTP-level CRUD tests for the tools handler — exercises the full
// router → handler → ToolRepository → Postgres path, including workspace
// scoping (one workspace must not see or mutate another's tools).

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// toolsRouter mounts the handler under a real chi router so URL params and
// method routing behave exactly as in production.
func toolsRouter(h *ToolsHandler) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/tools", h.List)
	r.Post("/tools", h.Create)
	r.Get("/tools/{id}", h.Get)
	r.Put("/tools/{id}", h.Update)
	r.Delete("/tools/{id}", h.Delete)
	return r
}

func doJSON(t *testing.T, r http.Handler, method, path, ws string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyWorkspaceID, ws))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestToolsCRUDRoundTrip(t *testing.T) {
	fx := newLoopFixture(t, nil, "")
	h := NewToolsHandler(fx.pool, fx.h.cfg)
	r := toolsRouter(h)

	// Create
	rec := doJSON(t, r, http.MethodPost, "/tools", fx.ws, map[string]any{
		"name":        "crud_probe",
		"description": "crud test tool",
		"type":        "http",
		"config":      map[string]any{"url": "https://example.com"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", rec.Code, rec.Body.String())
	}
	var created domain.Tool
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("create response unparseable: %v %s", err, rec.Body.String())
	}
	if created.RiskLevel != "low" || created.TimeoutMs != 30000 {
		t.Fatalf("defaults not applied: %+v", created)
	}

	// Get — visible in the owning workspace, 404 elsewhere.
	if rec := doJSON(t, r, http.MethodGet, "/tools/"+created.ID, fx.ws, nil); rec.Code != http.StatusOK {
		t.Fatalf("get status = %d", rec.Code)
	}
	if rec := doJSON(t, r, http.MethodGet, "/tools/"+created.ID, uuid.NewString(), nil); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace get status = %d, want 404", rec.Code)
	}

	// Update — config change persists; other workspaces get 404.
	upd := map[string]any{
		"name": "crud_probe", "description": "updated", "type": "http",
		"config": map[string]any{"url": "https://example.org"},
	}
	if rec := doJSON(t, r, http.MethodPut, "/tools/"+created.ID, fx.ws, upd); rec.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodPut, "/tools/"+created.ID, uuid.NewString(), upd); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace update status = %d, want 404", rec.Code)
	}
	var cfgJSON string
	_ = fx.pool.QueryRow(context.Background(), `SELECT config::text FROM tools WHERE id=$1::uuid`, created.ID).Scan(&cfgJSON)
	if !bytes.Contains([]byte(cfgJSON), []byte("example.org")) {
		t.Fatalf("config column not updated: %s", cfgJSON)
	}

	// List includes it.
	rec = doJSON(t, r, http.MethodGet, "/tools", fx.ws, nil)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("crud_probe")) {
		t.Fatalf("list missing tool: %d %s", rec.Code, rec.Body.String())
	}

	// Delete — cross-workspace refused, own workspace succeeds.
	if rec := doJSON(t, r, http.MethodDelete, "/tools/"+created.ID, uuid.NewString(), nil); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace delete status = %d, want 404", rec.Code)
	}
	if rec := doJSON(t, r, http.MethodDelete, "/tools/"+created.ID, fx.ws, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if rec := doJSON(t, r, http.MethodGet, "/tools/"+created.ID, fx.ws, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("tool still readable after delete: %d", rec.Code)
	}
}
