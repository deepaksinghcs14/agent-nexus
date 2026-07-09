package provider

import "strings"

// ModelSpec is one row of the model catalog: everything the platform knows
// about a model family, keyed by a name fragment. Previously this data was
// split across two drifting tables (context windows here, pricing in
// runtime/cost) that had to be updated in tandem; this is now the single
// place to register a model.
type ModelSpec struct {
	// Match is the lowercase name fragment; entries are checked in order, so
	// more specific fragments must come before their prefixes.
	Match string
	// ContextWindow in tokens; 0 = unknown (callers fall back to a default).
	ContextWindow int
	// InUSDPerMTok / OutUSDPerMTok are USD per million tokens; both 0 =
	// unknown (callers fall back to per-provider defaults).
	InUSDPerMTok  float64
	OutUSDPerMTok float64
}

// Catalog is ordered most-specific-first within each family.
var Catalog = []ModelSpec{
	// Anthropic
	{Match: "claude-fable-5", ContextWindow: 200_000},
	{Match: "claude-opus-4", ContextWindow: 200_000, InUSDPerMTok: 15.00, OutUSDPerMTok: 75.00},
	{Match: "claude-sonnet-4", ContextWindow: 200_000, InUSDPerMTok: 3.00, OutUSDPerMTok: 15.00},
	{Match: "claude-haiku-4", ContextWindow: 200_000, InUSDPerMTok: 0.80, OutUSDPerMTok: 4.00},
	{Match: "claude-3-7-sonnet", ContextWindow: 200_000, InUSDPerMTok: 3.00, OutUSDPerMTok: 15.00},
	{Match: "claude-3-5-sonnet", ContextWindow: 200_000, InUSDPerMTok: 3.00, OutUSDPerMTok: 15.00},
	{Match: "claude-3-5-haiku", ContextWindow: 200_000, InUSDPerMTok: 0.80, OutUSDPerMTok: 4.00},
	{Match: "claude-3-opus", ContextWindow: 200_000, InUSDPerMTok: 15.00, OutUSDPerMTok: 75.00},
	{Match: "claude-3-sonnet", ContextWindow: 200_000, InUSDPerMTok: 3.00, OutUSDPerMTok: 15.00},
	{Match: "claude-3-haiku", ContextWindow: 200_000, InUSDPerMTok: 0.25, OutUSDPerMTok: 1.25},
	{Match: "claude", ContextWindow: 200_000}, // family catch-all

	// OpenAI
	{Match: "gpt-4o-mini", ContextWindow: 128_000, InUSDPerMTok: 0.15, OutUSDPerMTok: 0.60},
	{Match: "gpt-4o", ContextWindow: 128_000, InUSDPerMTok: 2.50, OutUSDPerMTok: 10.00},
	{Match: "gpt-4-turbo", ContextWindow: 128_000, InUSDPerMTok: 10.00, OutUSDPerMTok: 30.00},
	{Match: "gpt-4-32k", ContextWindow: 32_768, InUSDPerMTok: 30.00, OutUSDPerMTok: 60.00},
	{Match: "gpt-4", ContextWindow: 8_192, InUSDPerMTok: 30.00, OutUSDPerMTok: 60.00},
	{Match: "gpt-3.5-turbo-16k", ContextWindow: 16_385, InUSDPerMTok: 0.50, OutUSDPerMTok: 1.50},
	{Match: "gpt-3.5-turbo", ContextWindow: 16_385, InUSDPerMTok: 0.50, OutUSDPerMTok: 1.50},
	{Match: "o1-mini", ContextWindow: 128_000, InUSDPerMTok: 1.10, OutUSDPerMTok: 4.40},
	{Match: "o1-preview", ContextWindow: 128_000, InUSDPerMTok: 15.00, OutUSDPerMTok: 60.00},
	{Match: "o1-pro", ContextWindow: 200_000, InUSDPerMTok: 150.00, OutUSDPerMTok: 600.00},
	{Match: "o1", ContextWindow: 200_000, InUSDPerMTok: 15.00, OutUSDPerMTok: 60.00},
	{Match: "o3-mini", ContextWindow: 200_000, InUSDPerMTok: 1.10, OutUSDPerMTok: 4.40},
	{Match: "o3", ContextWindow: 200_000, InUSDPerMTok: 10.00, OutUSDPerMTok: 40.00},

	// Google
	{Match: "gemini-2.5-flash", ContextWindow: 1_000_000, InUSDPerMTok: 0.075, OutUSDPerMTok: 0.30},
	{Match: "gemini-2.5-pro", ContextWindow: 1_000_000, InUSDPerMTok: 1.25, OutUSDPerMTok: 10.00},
	{Match: "gemini-2.0-flash", ContextWindow: 1_000_000},
	{Match: "gemini-1.5-flash", ContextWindow: 1_000_000, InUSDPerMTok: 0.075, OutUSDPerMTok: 0.30},
	{Match: "gemini-1.5-pro", ContextWindow: 1_000_000, InUSDPerMTok: 1.25, OutUSDPerMTok: 5.00},
	{Match: "gemini-1.0-pro", ContextWindow: 30_720},
	{Match: "gemini-pro", ContextWindow: 1_000_000, InUSDPerMTok: 0.50, OutUSDPerMTok: 1.50},
	{Match: "gemini", ContextWindow: 1_000_000}, // family catch-all

	// Ollama / local models — conservative defaults, no cost
	{Match: "llama", ContextWindow: 8_192},
	{Match: "mistral", ContextWindow: 8_192},
	{Match: "phi", ContextWindow: 8_192},
	{Match: "qwen", ContextWindow: 8_192},
}

// LookupModel returns the first catalog row whose fragment appears in the
// model name, or nil when the model is unknown.
func LookupModel(model string) *ModelSpec {
	lower := strings.ToLower(strings.TrimSpace(model))
	for i := range Catalog {
		if strings.Contains(lower, Catalog[i].Match) {
			return &Catalog[i]
		}
	}
	return nil
}
