package provider

import (
	"sort"
	"strings"
	"sync"
)

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
	// Deprecated marks models the provider has retired or scheduled for
	// shutdown — surfaced so model pickers can hide them.
	Deprecated bool
}

// overlay holds specs synced at runtime (see catalogsync); it is consulted
// before the static Catalog so community-maintained data wins when present.
var (
	overlayMu sync.RWMutex
	overlay   []ModelSpec
)

// SetCatalogOverlay replaces the runtime overlay. Entries are sorted
// longest-match-first so specific ids beat family fragments.
func SetCatalogOverlay(specs []ModelSpec) {
	sorted := make([]ModelSpec, len(specs))
	copy(sorted, specs)
	sort.Slice(sorted, func(i, j int) bool { return len(sorted[i].Match) > len(sorted[j].Match) })
	overlayMu.Lock()
	overlay = sorted
	overlayMu.Unlock()
}

// nonChatModelPatterns lists model-id fragments for families that appear in
// provider listings (and may even advertise generateContent-style support)
// but reject standard chat completions in favor of a dedicated API — e.g.
// Google's deep-research models require the Interactions API. Provider
// metadata can't be trusted for these, so they are filtered by id.
var nonChatModelPatterns = []string{"deep-research"}

// IsNonChatModel reports whether the model belongs to a family that cannot
// serve standard chat completions and must be hidden from model pickers.
func IsNonChatModel(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	for _, p := range nonChatModelPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// IsDeprecatedModel reports whether the synced catalog marks the model
// deprecated. Unknown models are not deprecated.
func IsDeprecatedModel(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	overlayMu.RLock()
	defer overlayMu.RUnlock()
	for i := range overlay {
		if strings.Contains(lower, overlay[i].Match) {
			return overlay[i].Deprecated
		}
	}
	return false
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

// LookupModel returns the best catalog row for the model name — runtime
// overlay first (fresh, community-synced), then the static table — or nil
// when the model is unknown.
func LookupModel(model string) *ModelSpec {
	lower := strings.ToLower(strings.TrimSpace(model))
	overlayMu.RLock()
	for i := range overlay {
		if strings.Contains(lower, overlay[i].Match) {
			spec := overlay[i]
			overlayMu.RUnlock()
			return &spec
		}
	}
	overlayMu.RUnlock()
	for i := range Catalog {
		if strings.Contains(lower, Catalog[i].Match) {
			return &Catalog[i]
		}
	}
	return nil
}
