package planning

import (
	"context"
	"fmt"
)

// EvalFeedbackBridge is an interface for connecting eval feedback to planning decisions.
// This interface is implemented by eval.PlanningFeedbackBridge (Task 9).
//
// The bridge receives real-time eval feedback during mission execution and can
// influence planning decisions, particularly triggering replanning when eval scores
// indicate poor agent performance.
type EvalFeedbackBridge interface {
	// GetLastFeedback returns the most recent eval feedback for a given node.
	// Returns nil if no feedback is available.
	GetLastFeedback(nodeID string) *EvalFeedback

	// GetAggregateScore returns the aggregated eval score across all scorers.
	// Returns 0.0 if no scores are available.
	GetAggregateScore() float64

	// ShouldTriggerReplan determines if current eval scores warrant replanning.
	// This considers warning/critical thresholds and feedback patterns.
	ShouldTriggerReplan(nodeID string) (bool, string)
}

// EvalFeedback represents eval feedback for a single step.
// This is a simplified version of the full eval.Feedback type.
type EvalFeedback struct {
	// NodeID is the workflow node this feedback is for
	NodeID string

	// Score is the eval score (0.0-1.0, higher is better)
	Score float64

	// ScorerScores contains individual scores from each configured scorer
	ScorerScores map[string]float64

	// Recommendation is the eval system's recommendation (continue, reconsider, abort)
	Recommendation string

	// Reasoning explains the score and recommendation
	Reasoning string

	// IsAlert indicates if this feedback triggered a warning or critical alert
	IsAlert bool

	// AlertLevel is "warning" or "critical" when IsAlert is true
	AlertLevel string
}

// WithEvalFeedback sets up eval feedback bridge for the orchestrator.
// The bridge enables real-time eval scores to influence planning decisions.
//
// When eval feedback is enabled:
//   - OnStepComplete will factor eval scores into step scoring
//   - Low eval scores can trigger automatic replanning
//   - Eval feedback context is passed to HandleReplan for better decisions
//
// Example usage:
//
//	bridge := eval.NewPlanningFeedbackBridge(orchestrator)
//	orch := NewPlanningOrchestrator(
//	    WithScorer(scorer),
//	    WithReplanner(replanner),
//	    WithEvalFeedback(bridge),
//	)
func WithEvalFeedback(bridge EvalFeedbackBridge) PlanningOrchestratorOption {
	return func(o *PlanningOrchestrator) {
		o.evalBridge = bridge
	}
}

// getEvalAdjustedScore adjusts the step score based on eval feedback.
// This is called by OnStepComplete to incorporate eval scores into planning decisions.
//
// The adjustment works as follows:
//   - If no eval feedback is available, returns the original score unchanged
//   - If eval score is high (>0.7), it may increase confidence in success
//   - If eval score is low (<0.5), it may decrease confidence or trigger replanning
//   - Critical alerts override the success assessment
//
// Returns the adjusted score and a boolean indicating if eval feedback was applied.
func (o *PlanningOrchestrator) getEvalAdjustedScore(
	ctx context.Context,
	nodeID string,
	originalScore *StepScore,
) (*StepScore, bool) {
	// Check if eval bridge is configured
	if o.evalBridge == nil {
		return originalScore, false
	}

	// Get eval feedback for this node
	feedback := o.evalBridge.GetLastFeedback(nodeID)
	if feedback == nil {
		// No feedback available yet - return original score
		return originalScore, false
	}

	// Create adjusted score (copy of original)
	adjusted := *originalScore

	// Apply eval score to confidence calculation
	// Eval score influences confidence - low eval scores reduce confidence
	if feedback.Score < 0.3 {
		// Very low eval score - strong negative signal
		adjusted.Confidence = adjusted.Confidence * 0.5
		adjusted.ShouldReplan = true
		if adjusted.ReplanReason == "" {
			adjusted.ReplanReason = fmt.Sprintf("Eval score very low (%.2f): %s", feedback.Score, feedback.Reasoning)
		} else {
			adjusted.ReplanReason = fmt.Sprintf("%s; Eval score very low (%.2f): %s",
				adjusted.ReplanReason, feedback.Score, feedback.Reasoning)
		}
	} else if feedback.Score < 0.5 {
		// Low eval score - moderate negative signal
		adjusted.Confidence = adjusted.Confidence * 0.7
		// Don't automatically trigger replan, but reduce confidence
	} else if feedback.Score > 0.7 && originalScore.Success {
		// High eval score reinforces success
		adjusted.Confidence = min(1.0, adjusted.Confidence*1.1)
	}

	// Handle critical alerts - these override success assessment
	if feedback.IsAlert && feedback.AlertLevel == "critical" {
		adjusted.Success = false
		adjusted.ShouldReplan = true
		adjusted.ReplanReason = fmt.Sprintf("Critical eval alert: %s", feedback.Reasoning)
	}

	// Check if eval bridge recommends replanning
	shouldReplan, reason := o.evalBridge.ShouldTriggerReplan(nodeID)
	if shouldReplan {
		adjusted.ShouldReplan = true
		if adjusted.ReplanReason == "" {
			adjusted.ReplanReason = reason
		} else {
			adjusted.ReplanReason = fmt.Sprintf("%s; %s", adjusted.ReplanReason, reason)
		}
	}

	return &adjusted, true
}

// getEvalFeedbackContext creates a feedback context string for replanning.
// This provides the tactical replanner with eval feedback to inform better decisions.
func (o *PlanningOrchestrator) getEvalFeedbackContext(nodeID string) string {
	if o.evalBridge == nil {
		return ""
	}

	feedback := o.evalBridge.GetLastFeedback(nodeID)
	if feedback == nil {
		return ""
	}

	// Build context string with eval insights
	context := fmt.Sprintf("Eval Feedback for %s:\n", nodeID)
	context += fmt.Sprintf("  Overall Score: %.2f\n", feedback.Score)
	context += fmt.Sprintf("  Recommendation: %s\n", feedback.Recommendation)
	context += fmt.Sprintf("  Reasoning: %s\n", feedback.Reasoning)

	if len(feedback.ScorerScores) > 0 {
		context += "  Individual Scorer Scores:\n"
		for scorer, score := range feedback.ScorerScores {
			context += fmt.Sprintf("    - %s: %.2f\n", scorer, score)
		}
	}

	if feedback.IsAlert {
		context += fmt.Sprintf("  ALERT: %s level\n", feedback.AlertLevel)
	}

	return context
}

// Helper function for min (Go 1.21+ has this in math.Min for floats)
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
