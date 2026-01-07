package planning

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// mockEvalBridge implements EvalFeedbackBridge for testing
type mockEvalBridge struct {
	feedback       map[string]*EvalFeedback
	aggregateScore float64
	shouldReplan   bool
	replanReason   string
}

func newMockEvalBridge() *mockEvalBridge {
	return &mockEvalBridge{
		feedback: make(map[string]*EvalFeedback),
	}
}

func (m *mockEvalBridge) GetLastFeedback(nodeID string) *EvalFeedback {
	return m.feedback[nodeID]
}

func (m *mockEvalBridge) GetAggregateScore() float64 {
	return m.aggregateScore
}

func (m *mockEvalBridge) ShouldTriggerReplan(nodeID string) (bool, string) {
	return m.shouldReplan, m.replanReason
}

func (m *mockEvalBridge) setFeedback(nodeID string, score float64, recommendation string, isAlert bool, alertLevel string) {
	m.feedback[nodeID] = &EvalFeedback{
		NodeID:         nodeID,
		Score:          score,
		ScorerScores:   map[string]float64{"test_scorer": score},
		Recommendation: recommendation,
		Reasoning:      "Mock feedback reasoning",
		IsAlert:        isAlert,
		AlertLevel:     alertLevel,
	}
}

// Helper function to create a basic scorer for tests
func createTestScorer() *mockStepScorer {
	return &mockStepScorer{
		scoreFunc: func(ctx context.Context, input ScoreInput) (*StepScore, error) {
			return &StepScore{
				NodeID:     input.NodeID,
				Success:    true,
				Confidence: 0.8,
			}, nil
		},
	}
}

func TestWithEvalFeedback(t *testing.T) {
	bridge := newMockEvalBridge()
	orch := NewPlanningOrchestrator(
		WithScorer(createTestScorer()),
		WithEvalFeedback(bridge),
	)

	assert.NotNil(t, orch.evalBridge, "eval bridge should be set")
	assert.Equal(t, bridge, orch.evalBridge, "eval bridge should match provided bridge")
}

func TestGetEvalAdjustedScore_NoEvalBridge(t *testing.T) {
	// Orchestrator without eval bridge
	orch := NewPlanningOrchestrator(
		WithScorer(createTestScorer()),
	)

	originalScore := &StepScore{
		NodeID:     "test_node",
		Success:    true,
		Confidence: 0.8,
	}

	adjustedScore, evalApplied := orch.getEvalAdjustedScore(context.Background(), "test_node", originalScore)

	assert.False(t, evalApplied, "eval should not be applied when bridge is nil")
	assert.Equal(t, originalScore, adjustedScore, "score should be unchanged")
}

func TestGetEvalAdjustedScore_NoFeedback(t *testing.T) {
	// Orchestrator with eval bridge but no feedback
	bridge := newMockEvalBridge()
	orch := NewPlanningOrchestrator(
		WithScorer(createTestScorer()),
		WithEvalFeedback(bridge),
	)

	originalScore := &StepScore{
		NodeID:     "test_node",
		Success:    true,
		Confidence: 0.8,
	}

	adjustedScore, evalApplied := orch.getEvalAdjustedScore(context.Background(), "test_node", originalScore)

	assert.False(t, evalApplied, "eval should not be applied when no feedback available")
	assert.Equal(t, originalScore, adjustedScore, "score should be unchanged")
}

func TestGetEvalAdjustedScore_VeryLowScore(t *testing.T) {
	// Test that very low eval scores trigger replanning and reduce confidence
	bridge := newMockEvalBridge()
	bridge.setFeedback("test_node", 0.2, "reconsider", false, "")

	orch := NewPlanningOrchestrator(
		WithScorer(createTestScorer()),
		WithEvalFeedback(bridge),
	)

	originalScore := &StepScore{
		NodeID:     "test_node",
		Success:    true,
		Confidence: 0.8,
	}

	adjustedScore, evalApplied := orch.getEvalAdjustedScore(context.Background(), "test_node", originalScore)

	assert.True(t, evalApplied, "eval should be applied")
	assert.True(t, adjustedScore.ShouldReplan, "should trigger replan for very low eval score")
	assert.Less(t, adjustedScore.Confidence, originalScore.Confidence, "confidence should be reduced")
	assert.Contains(t, adjustedScore.ReplanReason, "Eval score very low", "replan reason should mention eval score")
}

func TestGetEvalAdjustedScore_LowScore(t *testing.T) {
	// Test that moderately low eval scores reduce confidence but don't auto-trigger replan
	bridge := newMockEvalBridge()
	bridge.setFeedback("test_node", 0.4, "continue", false, "")

	orch := NewPlanningOrchestrator(
		WithScorer(createTestScorer()),
		WithEvalFeedback(bridge),
	)

	originalScore := &StepScore{
		NodeID:     "test_node",
		Success:    true,
		Confidence: 0.8,
	}

	adjustedScore, evalApplied := orch.getEvalAdjustedScore(context.Background(), "test_node", originalScore)

	assert.True(t, evalApplied, "eval should be applied")
	assert.False(t, adjustedScore.ShouldReplan, "should not auto-trigger replan for moderate score")
	assert.Less(t, adjustedScore.Confidence, originalScore.Confidence, "confidence should be reduced")
	assert.InDelta(t, 0.56, adjustedScore.Confidence, 0.01, "confidence should be reduced by 30%")
}

func TestGetEvalAdjustedScore_HighScore(t *testing.T) {
	// Test that high eval scores reinforce confidence
	bridge := newMockEvalBridge()
	bridge.setFeedback("test_node", 0.9, "continue", false, "")

	orch := NewPlanningOrchestrator(
		WithScorer(createTestScorer()),
		WithEvalFeedback(bridge),
	)

	originalScore := &StepScore{
		NodeID:     "test_node",
		Success:    true,
		Confidence: 0.8,
	}

	adjustedScore, evalApplied := orch.getEvalAdjustedScore(context.Background(), "test_node", originalScore)

	assert.True(t, evalApplied, "eval should be applied")
	assert.Greater(t, adjustedScore.Confidence, originalScore.Confidence, "confidence should be increased")
	assert.LessOrEqual(t, adjustedScore.Confidence, 1.0, "confidence should not exceed 1.0")
}

func TestGetEvalAdjustedScore_CriticalAlert(t *testing.T) {
	// Test that critical alerts override success and trigger replan
	bridge := newMockEvalBridge()
	bridge.setFeedback("test_node", 0.5, "abort", true, "critical")

	orch := NewPlanningOrchestrator(
		WithScorer(createTestScorer()),
		WithEvalFeedback(bridge),
	)

	originalScore := &StepScore{
		NodeID:     "test_node",
		Success:    true,
		Confidence: 0.9,
	}

	adjustedScore, evalApplied := orch.getEvalAdjustedScore(context.Background(), "test_node", originalScore)

	assert.True(t, evalApplied, "eval should be applied")
	assert.False(t, adjustedScore.Success, "critical alert should override success")
	assert.True(t, adjustedScore.ShouldReplan, "critical alert should trigger replan")
	assert.Contains(t, adjustedScore.ReplanReason, "Critical eval alert", "replan reason should mention critical alert")
}

func TestGetEvalAdjustedScore_BridgeRecommendReplan(t *testing.T) {
	// Test that bridge's ShouldTriggerReplan recommendation is respected
	bridge := newMockEvalBridge()
	bridge.setFeedback("test_node", 0.6, "reconsider", false, "")
	bridge.shouldReplan = true
	bridge.replanReason = "Pattern of degrading scores detected"

	orch := NewPlanningOrchestrator(
		WithScorer(createTestScorer()),
		WithEvalFeedback(bridge),
	)

	originalScore := &StepScore{
		NodeID:     "test_node",
		Success:    true,
		Confidence: 0.8,
	}

	adjustedScore, evalApplied := orch.getEvalAdjustedScore(context.Background(), "test_node", originalScore)

	assert.True(t, evalApplied, "eval should be applied")
	assert.True(t, adjustedScore.ShouldReplan, "should respect bridge's replan recommendation")
	assert.Contains(t, adjustedScore.ReplanReason, "Pattern of degrading scores", "should include bridge's reason")
}

func TestGetEvalFeedbackContext_NoEvalBridge(t *testing.T) {
	orch := NewPlanningOrchestrator(
		WithScorer(createTestScorer()),
	)

	context := orch.getEvalFeedbackContext("test_node")
	assert.Empty(t, context, "should return empty string when no eval bridge")
}

func TestGetEvalFeedbackContext_NoFeedback(t *testing.T) {
	bridge := newMockEvalBridge()
	orch := NewPlanningOrchestrator(
		WithScorer(createTestScorer()),
		WithEvalFeedback(bridge),
	)

	context := orch.getEvalFeedbackContext("test_node")
	assert.Empty(t, context, "should return empty string when no feedback available")
}

func TestGetEvalFeedbackContext_WithFeedback(t *testing.T) {
	bridge := newMockEvalBridge()
	bridge.setFeedback("test_node", 0.3, "reconsider", true, "warning")

	orch := NewPlanningOrchestrator(
		WithScorer(createTestScorer()),
		WithEvalFeedback(bridge),
	)

	context := orch.getEvalFeedbackContext("test_node")

	assert.NotEmpty(t, context, "should return feedback context")
	assert.Contains(t, context, "test_node", "should mention node ID")
	assert.Contains(t, context, "0.30", "should include score")
	assert.Contains(t, context, "reconsider", "should include recommendation")
	assert.Contains(t, context, "Mock feedback reasoning", "should include reasoning")
	assert.Contains(t, context, "test_scorer: 0.30", "should include scorer scores")
	assert.Contains(t, context, "ALERT: warning level", "should mention alert level")
}

func TestOnStepComplete_WithEvalIntegration(t *testing.T) {
	// Integration test for OnStepComplete with eval feedback
	bridge := newMockEvalBridge()
	bridge.setFeedback("test_node", 0.25, "reconsider", false, "")

	scorer := &mockStepScorer{
		scoreFunc: func(ctx context.Context, input ScoreInput) (*StepScore, error) {
			return &StepScore{
				NodeID:     "test_node",
				Success:    true,
				Confidence: 0.8,
			}, nil
		},
	}

	orch := NewPlanningOrchestrator(
		WithScorer(scorer),
		WithEvalFeedback(bridge),
	)

	// Set up minimal state
	orch.missionID = types.NewID()
	orch.originalGoal = "Test goal"
	orch.currentPlan = &StrategicPlan{
		ID: types.NewID(),
	}

	result := &workflow.NodeResult{
		Status: workflow.NodeStatusCompleted,
		Output: map[string]any{"result": "success"},
	}

	score, err := orch.OnStepComplete(context.Background(), "test_node", result, nil)

	require.NoError(t, err, "OnStepComplete should succeed")
	assert.NotNil(t, score, "should return a score")
	assert.True(t, score.ShouldReplan, "should trigger replan due to low eval score")
	assert.Less(t, score.Confidence, 0.8, "confidence should be reduced by eval feedback")
}

func TestHandleReplan_WithEvalContext(t *testing.T) {
	// Integration test for HandleReplan with eval context
	bridge := newMockEvalBridge()
	bridge.setFeedback("failed_node", 0.2, "abort", true, "critical")

	scorer := createTestScorer()

	// Create a replanner that captures the input
	var capturedInput ReplanInput
	replanner := &mockTacticalReplanner{
		replanFunc: func(ctx context.Context, input ReplanInput) (*ReplanResult, error) {
			capturedInput = input
			// Return a modified plan with the failed node marked for skipping
			modifiedPlan := *input.CurrentPlan
			modifiedPlan.OrderedSteps = []PlannedStep{
				{NodeID: "failed_node", Priority: 1}, // Will be skipped
			}
			return &ReplanResult{
				Action:       ReplanActionSkip,
				Rationale:    "Skipping due to repeated failures",
				ModifiedPlan: &modifiedPlan,
			}, nil
		},
	}

	orch := NewPlanningOrchestrator(
		WithScorer(scorer),
		WithReplanner(replanner),
		WithEvalFeedback(bridge),
	)

	// Set up state
	orch.missionID = types.NewID()
	orch.originalGoal = "Test goal"
	orch.currentPlan = &StrategicPlan{
		ID: types.NewID(),
		OrderedSteps: []PlannedStep{
			{NodeID: "failed_node", Priority: 1},
			{NodeID: "next_node", Priority: 2},
		},
	}
	orch.bounds = &PlanningBounds{
		AllowedNodes: map[string]bool{
			"failed_node": true,
			"next_node":   true,
		},
	}

	triggerScore := &StepScore{
		NodeID:       "failed_node",
		Success:      false,
		Confidence:   0.3,
		ShouldReplan: true,
		ReplanReason: "Critical eval alert",
	}

	// Create workflow for state
	wf := &workflow.Workflow{
		Nodes: map[string]*workflow.WorkflowNode{
			"failed_node": {},
			"next_node":   {},
		},
	}
	state := workflow.NewWorkflowState(wf)

	result, err := orch.HandleReplan(context.Background(), triggerScore, state)

	require.NoError(t, err, "HandleReplan should succeed")
	assert.NotNil(t, result, "should return a result")

	// Verify that replanner received eval context
	assert.NotEmpty(t, capturedInput.EvalContext, "replanner should receive eval context")
	assert.Contains(t, capturedInput.EvalContext, "failed_node", "eval context should mention node")
	assert.Contains(t, capturedInput.EvalContext, "0.20", "eval context should include score")
	assert.Contains(t, capturedInput.EvalContext, "ALERT: critical", "eval context should mention alert")
}
