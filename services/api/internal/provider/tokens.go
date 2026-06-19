package provider

import (
	"encoding/json"
	"strings"
)

// contextWindows maps model name substrings to their context window size in tokens.
// Longest / most-specific entries should be checked first (see ContextWindow below).
var contextWindows = []struct {
	substr string
	tokens int
}{
	// Anthropic
	{"claude-3-5-haiku", 200_000},
	{"claude-3-5-sonnet", 200_000},
	{"claude-3-7-sonnet", 200_000},
	{"claude-opus-4", 200_000},
	{"claude-sonnet-4", 200_000},
	{"claude-haiku-4", 200_000},
	{"claude-fable-5", 200_000},
	{"claude", 200_000}, // catch-all for any Claude model

	// OpenAI
	{"gpt-4o-mini", 128_000},
	{"gpt-4o", 128_000},
	{"gpt-4-turbo", 128_000},
	{"gpt-4-32k", 32_768},
	{"gpt-4", 8_192},
	{"gpt-3.5-turbo-16k", 16_385},
	{"gpt-3.5-turbo", 16_385},
	{"o1-mini", 128_000},
	{"o1-preview", 128_000},
	{"o1", 200_000},
	{"o3", 200_000},

	// Google
	{"gemini-2.0-flash", 1_000_000},
	{"gemini-1.5-pro", 1_000_000},
	{"gemini-1.5-flash", 1_000_000},
	{"gemini-1.0-pro", 30_720},
	{"gemini", 1_000_000}, // catch-all

	// Ollama / local models — conservative default
	{"llama", 8_192},
	{"mistral", 8_192},
	{"phi", 8_192},
	{"qwen", 8_192},
}

// ContextWindow returns the context window size in tokens for the given model name.
// Returns 8 192 if the model is not recognised (safe conservative default).
func ContextWindow(model string) int {
	lower := strings.ToLower(model)
	for _, entry := range contextWindows {
		if strings.Contains(lower, entry.substr) {
			return entry.tokens
		}
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
