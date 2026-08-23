package nvidia

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
)

func TestIsChatModel(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"meta/llama-3.1-70b-instruct", true},
		{"openai/gpt-oss-120b", true},
		{"meta/llama-3.2-11b-vision-instruct", true}, // a VLM, but chat-capable
		{"baai/bge-m3", false},                       // embedding — no "embed" substring, needs its own entry
		{"nvidia/nv-embedqa-mistral-7b-v2", false},
		{"snowflake/arctic-embed-l", false},
		{"meta/llama-guard-4-12b", false},
		{"nvidia/llama-3.1-nemoguard-8b-content-safety", false},
		{"nvidia/nemotron-4-340b-reward", false},
		{"nvidia/llama-3.2-nemoretriever-1b-vlm-embed-v1", false},
		{"nvidia/riva-translate-4b-instruct", false},
		{"nvidia/nemoretriever-parse", false},
		{"nvidia/ai-synthetic-video-detector", false},
		{"nvidia/ising-calibration-1.5-31b", false},
		{"nvidia/nvclip", false},
		{"microsoft/kosmos-2", false},
		{"google/deplot", false},
		{"google/diffusiongemma-26b-a4b-it", false},
	}
	for _, c := range cases {
		if got := isChatModel(c.id); got != c.want {
			t.Errorf("isChatModel(%q) = %v, want %v", c.id, got, c.want)
		}
	}
}

func TestModelsFetchesLiveCatalogAndFiltersNonChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]string{
				{"id": "meta/llama-3.1-70b-instruct"},
				{"id": "meta/llama-3.2-11b-vision-instruct"},
				{"id": "baai/bge-m3"},
				{"id": "meta/llama-guard-4-12b"},
			},
		})
	}))
	defer srv.Close()

	c := New("key", srv.URL)
	models, err := c.Models(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2 (embed + guard filtered out): %+v", len(models), models)
	}
	byID := map[string]provider.ModelInfo{}
	for _, m := range models {
		byID[m.ID] = m
	}
	if v, ok := byID["meta/llama-3.1-70b-instruct"]; !ok || v.SupportsVision {
		t.Errorf("llama-3.1-70b-instruct: got %+v, want present and SupportsVision=false", v)
	}
	if v, ok := byID["meta/llama-3.2-11b-vision-instruct"]; !ok || !v.SupportsVision {
		t.Errorf("llama-3.2-11b-vision-instruct: got %+v, want present and SupportsVision=true", v)
	}
}

func TestModelsFallsBackOnHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New("key", srv.URL)
	models, err := c.Models(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected the static fallback list, got none")
	}
	for _, m := range models {
		if strings.HasPrefix(m.ID, "gpt-") || strings.HasPrefix(m.ID, "o1") {
			t.Fatalf("got an OpenAI-shaped model id %q in the fallback — should be NVIDIA's own static list", m.ID)
		}
	}
}

func TestName(t *testing.T) {
	if got := New("k", "https://integrate.api.nvidia.com").Name(); got != "nvidia" {
		t.Fatalf("got %q, want \"nvidia\"", got)
	}
}
