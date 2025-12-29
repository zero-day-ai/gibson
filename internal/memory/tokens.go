package memory

import (
	"encoding/json"
)

// estimateTokens estimates the token count for a value using JSON serialization.
// Uses a simple heuristic of 4 characters per token for speed.
// This is designed to be fast (<1ms) rather than perfectly accurate.
func estimateTokens(value any) int {
	// Handle nil values
	if value == nil {
		return 1 // Minimal token count for nil
	}

	// JSON serialize the value
	data, err := json.Marshal(value)
	if err != nil {
		// If JSON serialization fails, estimate based on type
		// This shouldn't happen for most values, but provides fallback
		return 10 // Conservative estimate
	}

	// Approximate: 4 characters per token
	// This is roughly accurate for English text with typical LLM tokenization
	return len(data) / 4
}

// EstimateStringTokens estimates the token count for a string.
// Uses the same 4 characters per token heuristic.
func EstimateStringTokens(s string) int {
	if len(s) == 0 {
		return 0
	}
	return len(s) / 4
}
