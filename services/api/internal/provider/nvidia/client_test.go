package nvidia

import (
	"context"
	"strings"
	"testing"
)

// openai.Client.Models() filters to gpt-/o1-/o3-/o4- prefixes and silently
// falls back to fake OpenAI model data on any list error — both wrong for
// NVIDIA's org/model-name catalog. This must return NVIDIA's own models,
// not something inherited from the embedded openai.Client.
func TestModelsReturnsNvidiaCatalogNotOpenAIFallback(t *testing.T) {
	c := New("fake-key", "https://integrate.api.nvidia.com")
	models, err := c.Models(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected a non-empty model list")
	}
	for _, m := range models {
		if strings.HasPrefix(m.ID, "gpt-") || strings.HasPrefix(m.ID, "o1") {
			t.Fatalf("got an OpenAI-shaped model id %q — Models() is falling through to the embedded openai.Client's fallback", m.ID)
		}
	}
}

func TestName(t *testing.T) {
	if got := New("k", "").Name(); got != "nvidia" {
		t.Fatalf("got %q, want \"nvidia\"", got)
	}
}
