package cost

import (
	"strings"

	providerpkg "github.com/deepaksingh/agent-nexus/services/api/internal/provider"
)

// providerDefaults is a fallback when no model prefix matches.
var providerDefaults = map[string][2]float64{
	"anthropic": {3.00, 15.00},
	"openai":    {2.50, 10.00},
	"gemini":    {1.25, 5.00},
	"ollama":    {0, 0},
}

// Estimate returns the estimated cost in USD for a completed run.
// inputTokens and outputTokens are the total token counts for the run.
func Estimate(providerName, model string, inputTokens, outputTokens int) float64 {
	if spec := providerpkg.LookupModel(model); spec != nil && (spec.InUSDPerMTok > 0 || spec.OutUSDPerMTok > 0) {
		return (float64(inputTokens)*spec.InUSDPerMTok + float64(outputTokens)*spec.OutUSDPerMTok) / 1_000_000
	}
	if def, ok := providerDefaults[strings.ToLower(providerName)]; ok {
		return (float64(inputTokens)*def[0] + float64(outputTokens)*def[1]) / 1_000_000
	}
	return 0
}
