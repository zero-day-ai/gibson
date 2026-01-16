package orchestrator

import (
	"encoding/json"
	"testing"
)

// TestConvertRecoveryHint tests the conversion of individual recovery hints
func TestConvertRecoveryHint(t *testing.T) {
	t.Run("converts complete recovery hint", func(t *testing.T) {
		params := map[string]any{
			"timing": 2,
			"ports":  "22,80,443",
		}

		hint := ConvertRecoveryHint(
			"modify_params",
			"",
			"slower timing may help on congested networks",
			params,
			1,
		)

		if hint.Strategy != "modify_params" {
			t.Errorf("expected strategy 'modify_params', got '%s'", hint.Strategy)
		}
		if hint.Alternative != "" {
			t.Errorf("expected empty alternative, got '%s'", hint.Alternative)
		}
		if hint.Reason != "slower timing may help on congested networks" {
			t.Errorf("expected specific reason, got '%s'", hint.Reason)
		}
		if hint.Priority != 1 {
			t.Errorf("expected priority 1, got %d", hint.Priority)
		}
		if len(hint.Params) != 2 {
			t.Errorf("expected 2 params, got %d", len(hint.Params))
		}
	})

	t.Run("converts hint with alternative tool", func(t *testing.T) {
		hint := ConvertRecoveryHint(
			"use_alternative_tool",
			"masscan",
			"masscan can perform similar port scanning",
			nil,
			1,
		)

		if hint.Strategy != "use_alternative_tool" {
			t.Errorf("expected strategy 'use_alternative_tool', got '%s'", hint.Strategy)
		}
		if hint.Alternative != "masscan" {
			t.Errorf("expected alternative 'masscan', got '%s'", hint.Alternative)
		}
		if hint.Params != nil {
			t.Error("expected nil params")
		}
	})

	t.Run("converts minimal hint", func(t *testing.T) {
		hint := ConvertRecoveryHint(
			"retry",
			"",
			"transient error, may succeed on retry",
			nil,
			0,
		)

		if hint.Strategy != "retry" {
			t.Errorf("expected strategy 'retry', got '%s'", hint.Strategy)
		}
		if hint.Alternative != "" {
			t.Error("expected empty alternative")
		}
		if hint.Priority != 0 {
			t.Errorf("expected priority 0, got %d", hint.Priority)
		}
	})
}

// TestConvertRecoveryHints tests batch conversion from generic maps
func TestConvertRecoveryHints(t *testing.T) {
	t.Run("converts multiple hints from maps", func(t *testing.T) {
		hints := []map[string]any{
			{
				"strategy":    "use_alternative_tool",
				"alternative": "masscan",
				"reason":      "masscan can perform similar port scanning",
				"priority":    1,
			},
			{
				"strategy": "modify_params",
				"params": map[string]any{
					"timing": 2,
				},
				"reason":   "slower timing may help",
				"priority": 2,
			},
			{
				"strategy": "retry_with_backoff",
				"reason":   "transient network error",
				"priority": 3,
			},
		}

		summaries := ConvertRecoveryHints(hints)

		if len(summaries) != 3 {
			t.Fatalf("expected 3 summaries, got %d", len(summaries))
		}

		// Check first hint
		if summaries[0].Strategy != "use_alternative_tool" {
			t.Errorf("first hint strategy incorrect: %s", summaries[0].Strategy)
		}
		if summaries[0].Alternative != "masscan" {
			t.Errorf("first hint alternative incorrect: %s", summaries[0].Alternative)
		}
		if summaries[0].Priority != 1 {
			t.Errorf("first hint priority incorrect: %d", summaries[0].Priority)
		}

		// Check second hint
		if summaries[1].Strategy != "modify_params" {
			t.Errorf("second hint strategy incorrect: %s", summaries[1].Strategy)
		}
		if len(summaries[1].Params) != 1 {
			t.Errorf("second hint should have 1 param, got %d", len(summaries[1].Params))
		}
		if summaries[1].Params["timing"] != 2 {
			t.Error("second hint params not converted correctly")
		}

		// Check third hint
		if summaries[2].Strategy != "retry_with_backoff" {
			t.Errorf("third hint strategy incorrect: %s", summaries[2].Strategy)
		}
	})

	t.Run("handles empty hint list", func(t *testing.T) {
		hints := []map[string]any{}
		summaries := ConvertRecoveryHints(hints)

		if summaries != nil {
			t.Error("expected nil for empty hint list")
		}
	})

	t.Run("handles nil hint list", func(t *testing.T) {
		var hints []map[string]any
		summaries := ConvertRecoveryHints(hints)

		if summaries != nil {
			t.Error("expected nil for nil hint list")
		}
	})

	t.Run("handles hints with missing fields gracefully", func(t *testing.T) {
		hints := []map[string]any{
			{
				"strategy": "retry",
				// Missing reason, alternative, params, priority
			},
			{
				"reason": "incomplete hint",
				// Missing strategy
			},
		}

		summaries := ConvertRecoveryHints(hints)

		if len(summaries) != 2 {
			t.Fatalf("expected 2 summaries, got %d", len(summaries))
		}

		// First hint should have strategy but empty other fields
		if summaries[0].Strategy != "retry" {
			t.Error("first hint strategy not set")
		}
		if summaries[0].Reason != "" {
			t.Error("first hint should have empty reason")
		}

		// Second hint should have reason but empty strategy
		if summaries[1].Strategy != "" {
			t.Error("second hint should have empty strategy")
		}
		if summaries[1].Reason != "incomplete hint" {
			t.Error("second hint reason not set")
		}
	})
}

// TestEnrichExecutionFailure tests enriching an ExecutionFailure with error details
func TestEnrichExecutionFailure(t *testing.T) {
	t.Run("enriches failure with complete error information", func(t *testing.T) {
		failure := &ExecutionFailure{
			NodeID:     "node-123",
			NodeName:   "nmap_scan",
			AgentName:  "nmap",
			Attempt:    1,
			Error:      "nmap binary not found",
			CanRetry:   true,
			MaxRetries: 3,
		}

		hints := []map[string]any{
			{
				"strategy":    "use_alternative_tool",
				"alternative": "masscan",
				"reason":      "masscan can perform similar scanning",
				"priority":    1,
			},
		}

		partialResults := map[string]any{
			"scanned_hosts": 3,
			"completed":     false,
		}

		context := map[string]any{
			"attempted_path": "/usr/bin/nmap",
		}

		EnrichExecutionFailure(
			failure,
			"infrastructure",
			"BINARY_NOT_FOUND",
			hints,
			partialResults,
			context,
		)

		// Verify enrichment
		if failure.ErrorClass != "infrastructure" {
			t.Errorf("error class not set, got '%s'", failure.ErrorClass)
		}
		if failure.ErrorCode != "BINARY_NOT_FOUND" {
			t.Errorf("error code not set, got '%s'", failure.ErrorCode)
		}
		if len(failure.RecoveryHints) != 1 {
			t.Errorf("expected 1 recovery hint, got %d", len(failure.RecoveryHints))
		}
		if failure.RecoveryHints[0].Alternative != "masscan" {
			t.Error("recovery hint not converted properly")
		}
		if len(failure.PartialResults) != 2 {
			t.Errorf("expected 2 partial results, got %d", len(failure.PartialResults))
		}
		if failure.PartialResults["scanned_hosts"] != 3 {
			t.Error("partial results not set correctly")
		}
		if len(failure.FailureContext) != 1 {
			t.Errorf("expected 1 context entry, got %d", len(failure.FailureContext))
		}

		// Verify original fields are unchanged
		if failure.NodeID != "node-123" {
			t.Error("original NodeID was modified")
		}
		if failure.Error != "nmap binary not found" {
			t.Error("original Error was modified")
		}
	})

	t.Run("handles nil failure gracefully", func(t *testing.T) {
		// Should not panic
		EnrichExecutionFailure(
			nil,
			"infrastructure",
			"BINARY_NOT_FOUND",
			nil,
			nil,
			nil,
		)
		// Test passes if no panic occurs
	})

	t.Run("handles nil/empty enrichment data", func(t *testing.T) {
		failure := &ExecutionFailure{
			NodeID:   "node-123",
			NodeName: "test_node",
			Error:    "some error",
		}

		EnrichExecutionFailure(failure, "", "", nil, nil, nil)

		// Should set empty values without panic
		if failure.ErrorClass != "" {
			t.Error("error class should be empty string")
		}
		if failure.ErrorCode != "" {
			t.Error("error code should be empty string")
		}
		if failure.RecoveryHints != nil {
			t.Error("recovery hints should be nil")
		}
		if failure.PartialResults != nil {
			t.Error("partial results should be nil")
		}
		if failure.FailureContext != nil {
			t.Error("failure context should be nil")
		}
	})
}

// TestRecoveryHintSummaryJSON tests JSON serialization of RecoveryHintSummary
func TestRecoveryHintSummaryJSON(t *testing.T) {
	t.Run("serializes complete hint to JSON", func(t *testing.T) {
		hint := RecoveryHintSummary{
			Strategy:    "use_alternative_tool",
			Alternative: "masscan",
			Params: map[string]any{
				"timing": 2,
			},
			Reason:   "masscan is faster",
			Priority: 1,
		}

		data, err := json.Marshal(hint)
		if err != nil {
			t.Fatalf("failed to marshal hint: %v", err)
		}

		var decoded RecoveryHintSummary
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal hint: %v", err)
		}

		if decoded.Strategy != hint.Strategy {
			t.Error("strategy not preserved through JSON round-trip")
		}
		if decoded.Alternative != hint.Alternative {
			t.Error("alternative not preserved through JSON round-trip")
		}
		if decoded.Reason != hint.Reason {
			t.Error("reason not preserved through JSON round-trip")
		}
		if decoded.Priority != hint.Priority {
			t.Error("priority not preserved through JSON round-trip")
		}
	})

	t.Run("omits empty fields in JSON", func(t *testing.T) {
		hint := RecoveryHintSummary{
			Strategy: "retry",
			Reason:   "transient error",
		}

		data, err := json.Marshal(hint)
		if err != nil {
			t.Fatalf("failed to marshal hint: %v", err)
		}

		jsonStr := string(data)

		// Check that omitempty fields are not present
		if containsKey(jsonStr, "alternative") {
			t.Error("alternative should be omitted when empty")
		}
		if containsKey(jsonStr, "params") {
			t.Error("params should be omitted when nil")
		}
		if containsKey(jsonStr, "priority") {
			t.Error("priority should be omitted when zero")
		}
	})
}

// TestExecutionFailureJSON tests JSON serialization of ExecutionFailure
func TestExecutionFailureJSON(t *testing.T) {
	t.Run("serializes enhanced failure to JSON", func(t *testing.T) {
		failure := &ExecutionFailure{
			NodeID:     "node-123",
			NodeName:   "test_node",
			Error:      "test error",
			ErrorClass: "infrastructure",
			ErrorCode:  "BINARY_NOT_FOUND",
			RecoveryHints: []RecoveryHintSummary{
				{
					Strategy:    "use_alternative_tool",
					Alternative: "masscan",
					Reason:      "alternative tool available",
					Priority:    1,
				},
			},
			PartialResults: map[string]any{
				"count": 5,
			},
			FailureContext: map[string]any{
				"path": "/usr/bin/tool",
			},
		}

		data, err := json.Marshal(failure)
		if err != nil {
			t.Fatalf("failed to marshal failure: %v", err)
		}

		var decoded ExecutionFailure
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal failure: %v", err)
		}

		if decoded.ErrorClass != failure.ErrorClass {
			t.Error("error_class not preserved through JSON round-trip")
		}
		if decoded.ErrorCode != failure.ErrorCode {
			t.Error("error_code not preserved through JSON round-trip")
		}
		if len(decoded.RecoveryHints) != 1 {
			t.Error("recovery_hints not preserved through JSON round-trip")
		}
		if len(decoded.PartialResults) != 1 {
			t.Error("partial_results not preserved through JSON round-trip")
		}
		if len(decoded.FailureContext) != 1 {
			t.Error("failure_context not preserved through JSON round-trip")
		}
	})

	t.Run("omits enhanced fields when empty", func(t *testing.T) {
		failure := &ExecutionFailure{
			NodeID:   "node-123",
			NodeName: "test_node",
			Error:    "test error",
			// All enhanced fields omitted
		}

		data, err := json.Marshal(failure)
		if err != nil {
			t.Fatalf("failed to marshal failure: %v", err)
		}

		jsonStr := string(data)

		// Check that omitempty fields are not present
		if containsKey(jsonStr, "error_class") {
			t.Error("error_class should be omitted when empty")
		}
		if containsKey(jsonStr, "error_code") {
			t.Error("error_code should be omitted when empty")
		}
		if containsKey(jsonStr, "recovery_hints") {
			t.Error("recovery_hints should be omitted when nil")
		}
		if containsKey(jsonStr, "partial_results") {
			t.Error("partial_results should be omitted when nil")
		}
		if containsKey(jsonStr, "failure_context") {
			t.Error("failure_context should be omitted when nil")
		}
	})
}

// Helper function to check if a JSON string contains a key
func containsKey(jsonStr, key string) bool {
	// Simple string search for the key in JSON format
	searchStr := `"` + key + `"`
	return len(jsonStr) > 0 && len(key) > 0 && containsSubstring(jsonStr, searchStr)
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
