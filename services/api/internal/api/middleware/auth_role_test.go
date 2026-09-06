package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withRole(role, method string) *http.Request {
	req := httptest.NewRequest(method, "/agents", nil)
	return req.WithContext(context.WithValue(req.Context(), ContextKeyRole, role))
}

func TestBlockViewerWritesBlocksMutations(t *testing.T) {
	h := BlockViewerWrites(okHandler())

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, withRole("viewer", method))
		if rec.Code != http.StatusForbidden {
			t.Errorf("viewer %s: got %d, want 403", method, rec.Code)
		}
	}
}

func TestBlockViewerWritesAllowsReadsAndOtherRoles(t *testing.T) {
	h := BlockViewerWrites(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withRole("viewer", http.MethodGet))
	if rec.Code != http.StatusOK {
		t.Errorf("viewer GET: got %d, want 200", rec.Code)
	}

	for _, role := range []string{"owner", "admin", "member", ""} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, withRole(role, http.MethodPost))
		if rec.Code != http.StatusOK {
			t.Errorf("role %q POST: got %d, want 200", role, rec.Code)
		}
	}
}
