package planning

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

func TestDeterministicScorer_Score_WithFindings(t *testing.T) {
	scorer := NewDeterministicScorer()

	// Create test finding
	finding := agent.Finding{
		ID:       types.NewID(),
		Title:    "Test Vulnerability",
		Severity: agent.SeverityHigh,
	}

	input := ScoreInput{
		NodeID: "test-node",
		NodeResult: &workflow.NodeResult{
			NodeID:   "test-node",
			Status:   workflow.NodeStatusCompleted,
			Findings: []agent.Finding{finding},
		},
		OriginalGoal:   "Test mission goal",
		AttemptHistory: []AttemptRecord{},
	}

	score, err := scorer.Score(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, score)

	// Rule 1: findings count > 0 → success=true, confidence=0.9
	assert.True(t, score.Success)
	assert.Equal(t, 0.9, score.Confidence)
	assert.Equal(t, "deterministic", score.ScoringMethod)
	assert.Equal(t, 1, score.FindingsCount)
	assert.Len(t, score.FindingIDs, 1)
	assert.Equal(t, finding.ID, score.FindingIDs[0])
	assert.False(t, score.ShouldReplan)
	assert.Equal(t, 0, score.TokensUsed)
}

func TestDeterministicScorer_Score_SuccessfulCompletion(t *testing.T) {
	scorer := NewDeterministicScorer()

	input := ScoreInput{
		NodeID: "test-node",
		NodeResult: &workflow.NodeResult{
			NodeID:   "test-node",
			Status:   workflow.NodeStatusCompleted,
			Error:    nil,
			Findings: []agent.Finding{},
			Output:   map[string]any{},
		},
		OriginalGoal:   "Test mission goal",
		AttemptHistory: []AttemptRecord{},
	}

	score, err := scorer.Score(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, score)

	// Rule 2: successful completion → success=true, confidence=0.7
	assert.True(t, score.Success)
	assert.Equal(t, 0.7, score.Confidence)
	assert.Equal(t, "deterministic", score.ScoringMethod)
	assert.Equal(t, 0, score.FindingsCount)
	assert.Len(t, score.FindingIDs, 0)
	assert.False(t, score.ShouldReplan)
}

func TestDeterministicScorer_Score_SuccessfulWithExitCode(t *testing.T) {
	scorer := NewDeterministicScorer()

	input := ScoreInput{
		NodeID: "test-node",
		NodeResult: &workflow.NodeResult{
			NodeID:   "test-node",
			Status:   workflow.NodeStatusCompleted,
			Error:    nil,
			Findings: []agent.Finding{},
			Output: map[string]any{
				"exit_code": 0,
			},
		},
		OriginalGoal:   "Test mission goal",
		AttemptHistory: []AttemptRecord{},
	}

	score, err := scorer.Score(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, score)

	// Rule 2: successful completion with exit code 0 → confidence=0.75
	assert.True(t, score.Success)
	assert.Equal(t, 0.75, score.Confidence)
	assert.Equal(t, "deterministic", score.ScoringMethod)
	assert.False(t, score.ShouldReplan)
}

func TestDeterministicScorer_Score_Failed(t *testing.T) {
	scorer := NewDeterministicScorer()

	input := ScoreInput{
		NodeID: "test-node",
		NodeResult: &workflow.NodeResult{
			NodeID:   "test-node",
			Status:   workflow.NodeStatusFailed,
			Error:    &workflow.NodeError{Code: "test_error", Message: "Test error message"},
			Findings: []agent.Finding{},
			Output:   map[string]any{},
		},
		OriginalGoal:   "Test mission goal",
		AttemptHistory: []AttemptRecord{},
	}

	score, err := scorer.Score(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, score)

	// Failed node → success=false, low confidence
	assert.False(t, score.Success)
	assert.Equal(t, 0.1, score.Confidence)
	assert.Equal(t, "deterministic", score.ScoringMethod)
	assert.Equal(t, 0, score.FindingsCount)
}

func TestDeterministicScorer_Score_NoProgressThreeAttempts(t *testing.T) {
	scorer := NewDeterministicScorer()

	// Create history of 3 failed attempts with no findings
	history := []AttemptRecord{
		{
			Timestamp:     time.Now().Add(-6 * time.Minute),
			Approach:      "Attempt 1",
			Result:        "Failed",
			Success:       false,
			FindingsCount: 0,
		},
		{
			Timestamp:     time.Now().Add(-4 * time.Minute),
			Approach:      "Attempt 2",
			Result:        "Failed",
			Success:       false,
			FindingsCount: 0,
		},
		{
			Timestamp:     time.Now().Add(-2 * time.Minute),
			Approach:      "Attempt 3",
			Result:        "Failed",
			Success:       false,
			FindingsCount: 0,
		},
	}

	input := ScoreInput{
		NodeID: "test-node",
		NodeResult: &workflow.NodeResult{
			NodeID:   "test-node",
			Status:   workflow.NodeStatusFailed,
			Error:    &workflow.NodeError{Code: "test_error", Message: "Test error"},
			Findings: []agent.Finding{},
			Output:   map[string]any{},
		},
		OriginalGoal:   "Test mission goal",
		AttemptHistory: history,
	}

	score, err := scorer.Score(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, score)

	// Rule 4: no progress for 3 turns → shouldReplan=true
	assert.False(t, score.Success)
	assert.True(t, score.ShouldReplan)
	assert.Contains(t, score.ReplanReason, "No progress detected")
	assert.Contains(t, score.ReplanReason, "3 consecutive attempts")
}

func TestDeterministicScorer_Score_NoProgressWithSomeSuccess(t *testing.T) {
	scorer := NewDeterministicScorer()

	// Create history with one successful attempt - should NOT trigger replan via Rule 4
	// However, it will still trigger via Rule 5 (low confidence + no findings)
	history := []AttemptRecord{
		{
			Timestamp:     time.Now().Add(-6 * time.Minute),
			Approach:      "Attempt 1",
			Result:        "Failed",
			Success:       false,
			FindingsCount: 0,
		},
		{
			Timestamp:     time.Now().Add(-4 * time.Minute),
			Approach:      "Attempt 2",
			Result:        "Success",
			Success:       true,
			FindingsCount: 1,
		},
		{
			Timestamp:     time.Now().Add(-2 * time.Minute),
			Approach:      "Attempt 3",
			Result:        "Failed",
			Success:       false,
			FindingsCount: 0,
		},
	}

	input := ScoreInput{
		NodeID: "test-node",
		NodeResult: &workflow.NodeResult{
			NodeID:   "test-node",
			Status:   workflow.NodeStatusFailed,
			Error:    &workflow.NodeError{Code: "test_error", Message: "Test error"},
			Findings: []agent.Finding{},
			Output:   map[string]any{},
		},
		OriginalGoal:   "Test mission goal",
		AttemptHistory: history,
	}

	score, err := scorer.Score(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, score)

	// Should NOT replan via Rule 4 (3 consecutive failures) because middle attempt succeeded
	// BUT should still replan via Rule 5 (low confidence + no findings)
	assert.False(t, score.Success)
	assert.True(t, score.ShouldReplan)
	assert.Contains(t, score.ReplanReason, "Low confidence")
}

func TestDeterministicScorer_Score_LowConfidenceNoFindings(t *testing.T) {
	scorer := NewDeterministicScorer()

	input := ScoreInput{
		NodeID: "test-node",
		NodeResult: &workflow.NodeResult{
			NodeID:   "test-node",
			Status:   workflow.NodeStatusFailed,
			Error:    &workflow.NodeError{Code: "test_error", Message: "Test error"},
			Findings: []agent.Finding{},
			Output:   map[string]any{},
		},
		OriginalGoal:   "Test mission goal",
		AttemptHistory: []AttemptRecord{}, // Only one attempt
	}

	score, err := scorer.Score(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, score)

	// Rule 5: confidence < 0.3 AND no findings → shouldReplan=true
	assert.False(t, score.Success)
	assert.Less(t, score.Confidence, 0.3)
	assert.True(t, score.ShouldReplan)
	assert.Contains(t, score.ReplanReason, "Low confidence")
	assert.Contains(t, score.ReplanReason, "no findings")
}

func TestDeterministicScorer_Score_LowConfidenceWithFindings(t *testing.T) {
	scorer := NewDeterministicScorer()

	// Create test finding
	finding := agent.Finding{
		ID:       types.NewID(),
		Title:    "Test Vulnerability",
		Severity: agent.SeverityHigh,
	}

	// This is a contradictory case: findings present but status failed
	// This shouldn't happen in practice but we test the priority
	input := ScoreInput{
		NodeID: "test-node",
		NodeResult: &workflow.NodeResult{
			NodeID:   "test-node",
			Status:   workflow.NodeStatusCompleted, // Completed with findings
			Findings: []agent.Finding{finding},
			Output:   map[string]any{},
		},
		OriginalGoal:   "Test mission goal",
		AttemptHistory: []AttemptRecord{},
	}

	score, err := scorer.Score(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, score)

	// Rule 1 takes priority: findings → success
	assert.True(t, score.Success)
	assert.Equal(t, 0.9, score.Confidence)
	assert.False(t, score.ShouldReplan)
}

func TestDeterministicScorer_Score_FastExecution(t *testing.T) {
	scorer := NewDeterministicScorer()

	input := ScoreInput{
		NodeID: "test-node",
		NodeResult: &workflow.NodeResult{
			NodeID:   "test-node",
			Status:   workflow.NodeStatusCompleted,
			Findings: []agent.Finding{},
			Output:   map[string]any{},
		},
		OriginalGoal:   "Test mission goal",
		AttemptHistory: []AttemptRecord{},
	}

	// Test that scoring is fast (< 1ms requirement)
	start := time.Now()
	score, err := scorer.Score(context.Background(), input)
	duration := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, score)

	// Should complete in less than 1ms
	assert.Less(t, duration, time.Millisecond, "Scoring should complete in < 1ms")
	assert.Equal(t, 0, score.TokensUsed, "Deterministic scorer should use 0 tokens")
}

func TestDeterministicScorer_Score_NodeIDPopulated(t *testing.T) {
	scorer := NewDeterministicScorer()

	input := ScoreInput{
		NodeID: "test-node-123",
		NodeResult: &workflow.NodeResult{
			NodeID:   "test-node-123",
			Status:   workflow.NodeStatusCompleted,
			Findings: []agent.Finding{},
			Output:   map[string]any{},
		},
		OriginalGoal:   "Test mission goal",
		AttemptHistory: []AttemptRecord{},
	}

	score, err := scorer.Score(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, score)

	// Verify NodeID is populated in the score
	assert.Equal(t, "test-node-123", score.NodeID)
}

func TestDeterministicScorer_Score_TimestampSet(t *testing.T) {
	scorer := NewDeterministicScorer()

	input := ScoreInput{
		NodeID: "test-node",
		NodeResult: &workflow.NodeResult{
			NodeID:   "test-node",
			Status:   workflow.NodeStatusCompleted,
			Findings: []agent.Finding{},
			Output:   map[string]any{},
		},
		OriginalGoal:   "Test mission goal",
		AttemptHistory: []AttemptRecord{},
	}

	before := time.Now()
	score, err := scorer.Score(context.Background(), input)
	after := time.Now()

	require.NoError(t, err)
	require.NotNil(t, score)

	// Verify timestamp is set and within reasonable bounds
	assert.False(t, score.ScoredAt.IsZero())
	assert.True(t, score.ScoredAt.After(before) || score.ScoredAt.Equal(before))
	assert.True(t, score.ScoredAt.Before(after) || score.ScoredAt.Equal(after))
}

func TestDeterministicScorer_Score_EmptyAttemptHistory(t *testing.T) {
	scorer := NewDeterministicScorer()

	input := ScoreInput{
		NodeID: "test-node",
		NodeResult: &workflow.NodeResult{
			NodeID:   "test-node",
			Status:   workflow.NodeStatusCompleted,
			Findings: []agent.Finding{},
			Output:   map[string]any{},
		},
		OriginalGoal:   "Test mission goal",
		AttemptHistory: []AttemptRecord{}, // Empty history
	}

	score, err := scorer.Score(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, score)

	// Should handle empty history gracefully
	assert.True(t, score.Success)
	assert.False(t, score.ShouldReplan)
}

func TestDeterministicScorer_Score_ExactlyThreeFailedAttempts(t *testing.T) {
	scorer := NewDeterministicScorer()

	// Exactly 3 failed attempts
	history := []AttemptRecord{
		{Timestamp: time.Now().Add(-6 * time.Minute), Success: false, FindingsCount: 0},
		{Timestamp: time.Now().Add(-4 * time.Minute), Success: false, FindingsCount: 0},
		{Timestamp: time.Now().Add(-2 * time.Minute), Success: false, FindingsCount: 0},
	}

	input := ScoreInput{
		NodeID: "test-node",
		NodeResult: &workflow.NodeResult{
			NodeID:   "test-node",
			Status:   workflow.NodeStatusFailed,
			Findings: []agent.Finding{},
			Output:   map[string]any{},
		},
		OriginalGoal:   "Test mission goal",
		AttemptHistory: history,
	}

	score, err := scorer.Score(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, score)

	// Should trigger replan
	assert.True(t, score.ShouldReplan)
	assert.Contains(t, score.ReplanReason, "No progress")
}

func TestDeterministicScorer_Score_MoreThanThreeFailedAttempts(t *testing.T) {
	scorer := NewDeterministicScorer()

	// 5 failed attempts - should still trigger based on last 3
	history := []AttemptRecord{
		{Timestamp: time.Now().Add(-10 * time.Minute), Success: false, FindingsCount: 0},
		{Timestamp: time.Now().Add(-8 * time.Minute), Success: false, FindingsCount: 0},
		{Timestamp: time.Now().Add(-6 * time.Minute), Success: false, FindingsCount: 0},
		{Timestamp: time.Now().Add(-4 * time.Minute), Success: false, FindingsCount: 0},
		{Timestamp: time.Now().Add(-2 * time.Minute), Success: false, FindingsCount: 0},
	}

	input := ScoreInput{
		NodeID: "test-node",
		NodeResult: &workflow.NodeResult{
			NodeID:   "test-node",
			Status:   workflow.NodeStatusFailed,
			Findings: []agent.Finding{},
			Output:   map[string]any{},
		},
		OriginalGoal:   "Test mission goal",
		AttemptHistory: history,
	}

	score, err := scorer.Score(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, score)

	// Should trigger replan based on last 3
	assert.True(t, score.ShouldReplan)
}
