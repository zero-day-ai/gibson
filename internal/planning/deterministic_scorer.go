package planning

import (
	"context"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// DeterministicScorer implements StepScorer using rule-based checks without LLM calls.
// It provides fast, reliable scoring (< 1ms) based on deterministic criteria.
type DeterministicScorer struct{}

// NewDeterministicScorer creates a new deterministic scorer
func NewDeterministicScorer() *DeterministicScorer {
	return &DeterministicScorer{}
}

// Score evaluates a completed step using deterministic rule-based checks.
// Rules are applied in the following order:
// 1. If findings count > 0 → success=true, confidence=0.9
// 2. If tool exit code 0 (success status) → success=true, confidence=0.7
// 3. If budget within limits → continue (doesn't affect success determination)
// 4. If no progress for 3 consecutive turns → shouldReplan=true
// 5. If confidence < 0.3 AND no findings → shouldReplan=true
func (s *DeterministicScorer) Score(ctx context.Context, input ScoreInput) (*StepScore, error) {
	score := &StepScore{
		NodeID:        input.NodeID,
		ScoringMethod: "deterministic",
		ScoredAt:      time.Now(),
		TokensUsed:    0, // No LLM calls
	}

	// Extract finding IDs from the node result
	findingIDs := make([]types.ID, len(input.NodeResult.Findings))
	for i, finding := range input.NodeResult.Findings {
		findingIDs[i] = finding.ID
	}
	score.FindingsCount = len(input.NodeResult.Findings)
	score.FindingIDs = findingIDs

	// Rule 1: Findings count > 0 → high confidence success
	if len(input.NodeResult.Findings) > 0 {
		score.Success = true
		score.Confidence = 0.9
		return score, nil
	}

	// Rule 2: Check execution status - successful completion
	if input.NodeResult.Status == workflow.NodeStatusCompleted && input.NodeResult.Error == nil {
		score.Success = true
		score.Confidence = 0.7

		// Check for explicit success indicators in output
		if output, ok := input.NodeResult.Output["exit_code"].(int); ok && output == 0 {
			score.Confidence = 0.75 // Slightly higher confidence with explicit success code
		}

		return score, nil
	}

	// If we reach here, the step either failed or completed with low confidence
	// Set initial low confidence
	score.Success = false
	score.Confidence = 0.2

	// Check for explicit failure indicators
	if input.NodeResult.Status == workflow.NodeStatusFailed || input.NodeResult.Error != nil {
		score.Confidence = 0.1
	}

	// Rule 4: Check for no progress across consecutive attempts
	if len(input.AttemptHistory) >= 3 {
		// Check if last 3 attempts all failed with no findings
		noProgressCount := 0
		for i := len(input.AttemptHistory) - 3; i < len(input.AttemptHistory); i++ {
			if !input.AttemptHistory[i].Success && input.AttemptHistory[i].FindingsCount == 0 {
				noProgressCount++
			}
		}

		if noProgressCount >= 3 {
			score.ShouldReplan = true
			score.ReplanReason = "No progress detected in last 3 consecutive attempts - replanning recommended"
			return score, nil
		}
	}

	// Rule 5: Low confidence AND no findings → replan
	if score.Confidence < 0.3 && len(input.NodeResult.Findings) == 0 {
		score.ShouldReplan = true
		score.ReplanReason = "Low confidence execution with no findings discovered"
		return score, nil
	}

	return score, nil
}

// Rule 3 (budget within limits) is intentionally handled elsewhere in the planning system.
// The budget manager enforces limits during execution, not during scoring.
// This keeps the scorer focused on execution success assessment.
