package verbose

import (
	"encoding/json"
)

// JSONVerboseFormatter formats events as single-line JSON.
// Each event is output as a complete JSON object on a single line,
// making it suitable for log aggregation and machine parsing.
type JSONVerboseFormatter struct{}

// NewJSONVerboseFormatter creates a new JSON formatter.
func NewJSONVerboseFormatter() *JSONVerboseFormatter {
	return &JSONVerboseFormatter{}
}

// Format converts a VerboseEvent to a JSON string.
// Returns a single-line JSON object (no pretty printing) with a trailing newline.
func (f *JSONVerboseFormatter) Format(event VerboseEvent) string {
	// Marshal to JSON without indentation (compact format)
	data, err := json.Marshal(event)
	if err != nil {
		// Fallback to error object if marshaling fails
		errorEvent := map[string]interface{}{
			"type":      "error",
			"message":   "Failed to marshal verbose event",
			"error":     err.Error(),
			"timestamp": event.Timestamp,
		}
		data, _ = json.Marshal(errorEvent)
	}

	// Add trailing newline for line-oriented output
	return string(data) + "\n"
}

// Ensure JSONVerboseFormatter implements VerboseFormatter at compile time
var _ VerboseFormatter = (*JSONVerboseFormatter)(nil)
