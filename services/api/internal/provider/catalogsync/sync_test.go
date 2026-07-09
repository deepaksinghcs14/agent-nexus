package catalogsync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
)

func TestFetchMapsProvidersAndDeprecation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{
		 "google": {"models": {
		   "gemini-2.5-flash": {"id":"gemini-2.5-flash","cost":{"input":0.3,"output":2.5},"limit":{"context":1048576},"deprecated":null},
		   "gemini-2.0-flash": {"id":"gemini-2.0-flash","cost":{"input":0.1,"output":0.4},"limit":{"context":1048576},"deprecated":"2025-12-31"}
		 }},
		 "anthropic": {"models": {"claude-opus-4-5": {"id":"claude-opus-4-5","cost":{"input":5,"output":25},"limit":{"context":200000}}}},
		 "unrelated-provider": {"models": {"x": {"id":"x","cost":{"input":1,"output":1},"limit":{"context":1}}}}
		}`)
	}))
	defer srv.Close()
	orig := sourceURL
	sourceURL = srv.URL
	defer func() { sourceURL = orig }()

	specs, err := Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 3 {
		t.Fatalf("specs = %d, want 3 (unsupported providers skipped)", len(specs))
	}
	byID := map[string]provider.ModelSpec{}
	for _, s := range specs {
		byID[s.Match] = s
	}
	if !byID["gemini-2.0-flash"].Deprecated {
		t.Fatal("date-valued deprecated field must mark the model deprecated")
	}
	if byID["gemini-2.5-flash"].Deprecated {
		t.Fatal("null deprecated field must not mark the model deprecated")
	}
	if byID["claude-opus-4-5"].InUSDPerMTok != 5 || byID["claude-opus-4-5"].ContextWindow != 200000 {
		t.Fatalf("pricing/context not mapped: %+v", byID["claude-opus-4-5"])
	}

	// Overlay precedence: synced price beats the static table; deprecation is queryable.
	provider.SetCatalogOverlay(specs)
	defer provider.SetCatalogOverlay(nil)
	if spec := provider.LookupModel("gemini-2.5-flash"); spec == nil || spec.InUSDPerMTok != 0.3 {
		t.Fatalf("overlay did not win lookup: %+v", spec)
	}
	if !provider.IsDeprecatedModel("gemini-2.0-flash") || provider.IsDeprecatedModel("gemini-2.5-flash") {
		t.Fatal("IsDeprecatedModel wrong")
	}
}
