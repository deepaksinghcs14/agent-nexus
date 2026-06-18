package cost

import "strings"

// modelPricing maps a model-ID prefix to [inputPricePerMTok, outputPricePerMTok] in USD.
// Matched longest-prefix-first so "gpt-4o-mini" beats "gpt-4o".
var modelPricing = []struct {
	prefix string
	in     float64
	out    float64
}{
	// Anthropic — Claude 4 family
	{"claude-opus-4", 15.00, 75.00},
	{"claude-sonnet-4", 3.00, 15.00},
	{"claude-haiku-4", 0.80, 4.00},
	// Anthropic — Claude 3.5 family
	{"claude-3-5-sonnet", 3.00, 15.00},
	{"claude-3-5-haiku", 0.80, 4.00},
	// Anthropic — Claude 3 family
	{"claude-3-opus", 15.00, 75.00},
	{"claude-3-sonnet", 3.00, 15.00},
	{"claude-3-haiku", 0.25, 1.25},
	// OpenAI — GPT-4o
	{"gpt-4o-mini", 0.15, 0.60},
	{"gpt-4o", 2.50, 10.00},
	// OpenAI — GPT-4
	{"gpt-4-turbo", 10.00, 30.00},
	{"gpt-4", 30.00, 60.00},
	// OpenAI — GPT-3.5
	{"gpt-3.5-turbo", 0.50, 1.50},
	// OpenAI — o-series
	{"o1-mini", 1.10, 4.40},
	{"o1-pro", 150.00, 600.00},
	{"o1", 15.00, 60.00},
	{"o3-mini", 1.10, 4.40},
	{"o3", 10.00, 40.00},
	// Google Gemini 2.5
	{"gemini-2.5-flash", 0.075, 0.30},
	{"gemini-2.5-pro", 1.25, 10.00},
	// Google Gemini 1.5
	{"gemini-1.5-flash", 0.075, 0.30},
	{"gemini-1.5-pro", 1.25, 5.00},
	// Google Gemini 1.0
	{"gemini-pro", 0.50, 1.50},
}

// providerDefaults is a fallback when no model prefix matches.
var providerDefaults = map[string][2]float64{
	"anthropic": {3.00, 15.00},
	"openai":    {2.50, 10.00},
	"gemini":    {1.25, 5.00},
	"ollama":    {0, 0},
}

// Estimate returns the estimated cost in USD for a completed run.
// inputTokens and outputTokens are the total token counts for the run.
func Estimate(provider, model string, inputTokens, outputTokens int) float64 {
	m := strings.ToLower(strings.TrimSpace(model))
	for _, p := range modelPricing {
		if strings.HasPrefix(m, p.prefix) {
			return (float64(inputTokens)*p.in + float64(outputTokens)*p.out) / 1_000_000
		}
	}
	if def, ok := providerDefaults[strings.ToLower(provider)]; ok {
		return (float64(inputTokens)*def[0] + float64(outputTokens)*def[1]) / 1_000_000
	}
	return 0
}
