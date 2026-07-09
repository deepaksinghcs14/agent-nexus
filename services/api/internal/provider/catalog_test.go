package provider

import "testing"

// Ordering matters: specific fragments must win over family catch-alls, and
// both consumers (context window + pricing) read the same rows.
func TestModelCatalogLookup(t *testing.T) {
	cases := []struct {
		model      string
		wantWindow int
		wantIn     float64
	}{
		{"claude-sonnet-4-6", 200_000, 3.00},
		{"claude-fable-5", 200_000, 0},          // window known, price falls back
		{"gpt-4o-mini-2024", 128_000, 0.15},     // specific beats gpt-4o
		{"gpt-4", 8_192, 30.00},
		{"o1-pro", 200_000, 150.00},             // beats bare o1
		{"gemini-1.5-pro-latest", 1_000_000, 1.25},
		{"llama3.2", 8_192, 0},
	}
	for _, tc := range cases {
		spec := LookupModel(tc.model)
		if spec == nil {
			t.Fatalf("%s: no catalog match", tc.model)
		}
		if spec.ContextWindow != tc.wantWindow || spec.InUSDPerMTok != tc.wantIn {
			t.Errorf("%s: window/in = %d/%v, want %d/%v", tc.model, spec.ContextWindow, spec.InUSDPerMTok, tc.wantWindow, tc.wantIn)
		}
	}
	if LookupModel("totally-unknown-model") != nil {
		t.Error("unknown model should return nil")
	}
	if w := ContextWindow("totally-unknown-model"); w != 8_192 {
		t.Errorf("unknown model window = %d, want 8192 default", w)
	}
}
