package orchestrator

// This file provides helper functions to convert from SDK error types
// to orchestrator-specific summary types, avoiding tight coupling.

// ConvertRecoveryHint converts an SDK RecoveryHint (or similar structure)
// to the orchestrator's RecoveryHintSummary. This keeps the orchestrator
// decoupled from SDK internal types.
//
// Expected input format from SDK toolerr.RecoveryHint:
//
//	type RecoveryHint struct {
//	    Strategy    RecoveryStrategy       `json:"strategy"`
//	    Alternative string                 `json:"alternative,omitempty"`
//	    Params      map[string]any         `json:"params,omitempty"`
//	    Reason      string                 `json:"reason"`
//	    Confidence  float64                `json:"confidence"`
//	    Priority    int                    `json:"priority"`
//	}
func ConvertRecoveryHint(strategy, alternative, reason string, params map[string]any, priority int) RecoveryHintSummary {
	return RecoveryHintSummary{
		Strategy:    strategy,
		Alternative: alternative,
		Params:      params,
		Reason:      reason,
		Priority:    priority,
	}
}

// ConvertRecoveryHints converts a slice of SDK recovery hints to orchestrator summaries.
// This is a convenience function for batch conversion.
func ConvertRecoveryHints(hints []map[string]any) []RecoveryHintSummary {
	if len(hints) == 0 {
		return nil
	}

	summaries := make([]RecoveryHintSummary, 0, len(hints))
	for _, hint := range hints {
		summary := RecoveryHintSummary{}

		// Extract fields from the generic map
		if strategy, ok := hint["strategy"].(string); ok {
			summary.Strategy = strategy
		}
		if alternative, ok := hint["alternative"].(string); ok {
			summary.Alternative = alternative
		}
		if params, ok := hint["params"].(map[string]any); ok {
			summary.Params = params
		}
		if reason, ok := hint["reason"].(string); ok {
			summary.Reason = reason
		}
		if priority, ok := hint["priority"].(int); ok {
			summary.Priority = priority
		}

		summaries = append(summaries, summary)
	}

	return summaries
}

// EnrichExecutionFailure populates the enhanced error fields on an ExecutionFailure.
// This should be called when converting from an AgentExecution to ExecutionFailure
// with structured error information available.
//
// Parameters:
//   - failure: The ExecutionFailure to enrich
//   - errorClass: Error classification (infrastructure/semantic/transient/permanent)
//   - errorCode: Specific error code (BINARY_NOT_FOUND, TIMEOUT, etc.)
//   - hints: Recovery hints as generic maps (to avoid SDK coupling)
//   - partialResults: Any salvageable data from the failed execution
//   - context: Additional failure context
func EnrichExecutionFailure(
	failure *ExecutionFailure,
	errorClass string,
	errorCode string,
	hints []map[string]any,
	partialResults map[string]any,
	context map[string]any,
) {
	if failure == nil {
		return
	}

	failure.ErrorClass = errorClass
	failure.ErrorCode = errorCode
	failure.RecoveryHints = ConvertRecoveryHints(hints)
	failure.PartialResults = partialResults
	failure.FailureContext = context
}
