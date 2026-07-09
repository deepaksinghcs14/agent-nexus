package provider

import "encoding/json"

// ContextWindow returns the context window size in tokens for the given model name.
// Returns 8 192 if the model is not recognised (safe conservative default).
func ContextWindow(model string) int {
	if spec := LookupModel(model); spec != nil && spec.ContextWindow > 0 {
		return spec.ContextWindow
	}
	return 8_192
}

// EstimateTokens returns a rough token count for a slice of messages.
// Heuristic: len(JSON bytes) / 4 — accurate enough for truncation decisions.
func EstimateTokens(messages []Message) int {
	b, _ := json.Marshal(messages)
	return len(b) / 4
}

// TruncateMessages drops the oldest non-system messages until the estimated token
// count fits within the model's context window minus output headroom and a safety
// margin. The system prompt (index 0, role "system") is never dropped.
// Returns the original slice unchanged if no truncation is needed.
func TruncateMessages(messages []Message, model string, maxOutputTokens int) ([]Message, int) {
	if len(messages) == 0 {
		return messages, 0
	}

	const safetyMargin = 2_000
	budget := ContextWindow(model) - maxOutputTokens - safetyMargin
	if budget <= 0 {
		budget = 4_000 // absolute floor
	}

	if EstimateTokens(messages) <= budget {
		return messages, 0
	}

	// Find first non-system index to preserve the system prompt.
	firstNonSystem := 0
	for firstNonSystem < len(messages) && messages[firstNonSystem].Role == "system" {
		firstNonSystem++
	}

	// Nothing to drop (only system prompts remain).
	if firstNonSystem >= len(messages)-1 {
		return messages, 0
	}

	// Drop from the front of non-system messages until we fit.
	dropped := 0
	for len(messages) > firstNonSystem+1 && EstimateTokens(messages) > budget {
		// Remove messages[firstNonSystem]
		messages = append(messages[:firstNonSystem], messages[firstNonSystem+1:]...)
		dropped++
	}

	return messages, dropped
}
