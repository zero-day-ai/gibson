package planning

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// TestIntegration_HappyPath_FullPlanningFlow tests the complete planning cycle with all steps succeeding.
//
// This test verifies:
//  1. PreExecute generates a strategic plan
//  2. Execute steps according to plan
//  3. OnStepComplete scores each step
//  4. All steps succeed, no replanning needed
//  5. GetReport shows correct metrics
func TestIntegration_HappyPath_FullPlanningFlow(t *testing.T) {
	ctx := context.Background()

	// Create test workflow with 3 sequential steps
	wf := createTestWorkflow()
	missionID := types.NewID()
	goal := "Test complete planning flow with all steps succeeding"

	// Create orchestrator with default planner and deterministic scorer
	budgetMgr := NewDefaultBudgetManager()
	scorer := NewDeterministicScorer()

	orchestrator := NewPlanningOrchestrator(
		WithScorer(scorer),
		WithBudgetManager(budgetMgr),
	)

	// Step 1: PreExecute generates strategic plan using default planner
	plan, err := orchestrator.PreExecute(ctx, missionID, goal, wf, &mockAgentHarness{}, 10000)
	require.NoError(t, err)
	require.NotNil(t, plan)

	assert.Equal(t, 3, len(plan.OrderedSteps))
	assert.Equal(t, "node1", plan.OrderedSteps[0].NodeID)
	assert.Equal(t, "node2", plan.OrderedSteps[1].NodeID)
	assert.Equal(t, "node3", plan.OrderedSteps[2].NodeID)
	assert.Equal(t, PlanStatusApproved, plan.Status)

	// Step 2-4: Execute and score each step (all succeed)
	nodeResults := []struct {
		nodeID string
		status workflow.NodeStatus
	}{
		{"node1", workflow.NodeStatusCompleted},
		{"node2", workflow.NodeStatusCompleted},
		{"node3", workflow.NodeStatusCompleted},
	}

	for _, nr := range nodeResults {
		result := &workflow.NodeResult{
			NodeID:      nr.nodeID,
			Status:      nr.status,
			CompletedAt: time.Now(),
		}

		score, err := orchestrator.OnStepComplete(ctx, nr.nodeID, result, nil)
		require.NoError(t, err)
		require.NotNil(t, score)

		assert.True(t, score.Success, "Step %s should succeed", nr.nodeID)
		assert.False(t, score.ShouldReplan, "No replanning needed for successful steps")
		assert.GreaterOrEqual(t, score.Confidence, 0.5, "Reasonable confidence for successful steps")
	}

	// Step 5: GetReport shows correct metrics
	report := orchestrator.GetReport()
	require.NotNil(t, report)

	assert.Equal(t, missionID, report.MissionID)
	assert.Equal(t, 3, report.StepsScored)
	assert.Equal(t, 100.0, report.EffectivenessRate) // All 3 steps succeeded
	assert.Equal(t, 0, report.ReplanCount)
	assert.NotNil(t, report.BudgetReport)
}

// TestIntegration_ReplanScenario tests mid-execution replanning when a step fails.
func TestIntegration_ReplanScenario(t *testing.T) {
	ctx := context.Background()

	// Create workflow with 4 nodes to allow for meaningful reordering
	wf := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "replan-test-workflow",
		Nodes: map[string]*workflow.WorkflowNode{
			"recon": {
				ID:           "recon",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "recon-agent",
				Dependencies: []string{},
				Timeout:      30 * time.Second,
			},
			"attack-a": {
				ID:           "attack-a",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "attack-agent-a",
				Dependencies: []string{"recon"},
				Timeout:      60 * time.Second,
			},
			"attack-b": {
				ID:           "attack-b",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "attack-agent-b",
				Dependencies: []string{"recon"},
				Timeout:      60 * time.Second,
			},
		},
		EntryPoints: []string{"recon"},
		ExitPoints:  []string{"attack-a", "attack-b"},
		CreatedAt:   time.Now(),
	}

	missionID := types.NewID()
	goal := "Test replanning when mid-execution step fails"

	// Create orchestrator without replanner to test basic behavior
	budgetMgr := NewDefaultBudgetManager()
	scorer := NewDeterministicScorer()

	orchestrator := NewPlanningOrchestrator(
		WithScorer(scorer),
		WithBudgetManager(budgetMgr),
	)

	// Step 1: PreExecute generates initial plan
	plan, err := orchestrator.PreExecute(ctx, missionID, goal, wf, &mockAgentHarness{}, 15000)
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Equal(t, 3, len(plan.OrderedSteps))

	// Step 2: First step (recon) succeeds
	reconResult := &workflow.NodeResult{
		NodeID:      "recon",
		Status:      workflow.NodeStatusCompleted,
		CompletedAt: time.Now(),
	}

	reconScore, err := orchestrator.OnStepComplete(ctx, "recon", reconResult, nil)
	require.NoError(t, err)
	assert.True(t, reconScore.Success)
	assert.False(t, reconScore.ShouldReplan)

	// Step 3: Second step (attack-a) fails with low confidence
	attackAResult := &workflow.NodeResult{
		NodeID:      "attack-a",
		Status:      workflow.NodeStatusFailed,
		Error:       &workflow.NodeError{Message: "attack vector blocked"},
		CompletedAt: time.Now(),
	}

	// Step 4: OnStepComplete triggers shouldReplan
	attackAScore, err := orchestrator.OnStepComplete(ctx, "attack-a", attackAResult, nil)
	require.NoError(t, err)
	assert.False(t, attackAScore.Success)
	assert.True(t, attackAScore.ShouldReplan, "Failed step should trigger replanning")
	assert.Less(t, attackAScore.Confidence, 0.5)

	// Step 5: HandleReplan (without replanner, should continue)
	state := workflow.NewWorkflowState(wf)
	state.MarkNodeCompleted("recon", reconResult)
	state.MarkNodeFailed("attack-a", errors.New("attack vector blocked"))

	replanResult, err := orchestrator.HandleReplan(ctx, attackAScore, state)
	require.NoError(t, err)
	require.NotNil(t, replanResult)

	// Since no replanner is configured, should continue
	assert.Equal(t, ReplanActionContinue, replanResult.Action)

	// Step 6: Final report
	report := orchestrator.GetReport()
	require.NotNil(t, report)

	assert.Equal(t, missionID, report.MissionID)
	assert.Equal(t, 2, report.StepsScored) // recon + attack-a
}

// TestIntegration_BudgetExhaustion tests behavior when planning budget is exhausted.
func TestIntegration_BudgetExhaustion(t *testing.T) {
	ctx := context.Background()

	wf := createTestWorkflow()
	missionID := types.NewID()
	goal := "Test budget exhaustion fallback"

	// Create budget manager with very low budget
	budgetMgr := NewDefaultBudgetManager()

	orchestrator := NewPlanningOrchestrator(
		WithBudgetManager(budgetMgr),
	)

	// PreExecute with very low total budget (100 tokens total)
	plan, err := orchestrator.PreExecute(ctx, missionID, goal, wf, &mockAgentHarness{}, 100)

	// Should succeed using fallback planner
	require.NoError(t, err)
	require.NotNil(t, plan)

	// Verify plan was created (by fallback planner)
	assert.Equal(t, 3, len(plan.OrderedSteps))

	// Verify budget report shows consumption
	report := orchestrator.GetReport()
	require.NotNil(t, report)
	assert.NotNil(t, report.BudgetReport)
}

// TestIntegration_ScoringWithAgentHints tests that agent hints influence scoring.
func TestIntegration_ScoringWithAgentHints(t *testing.T) {
	ctx := context.Background()

	_ = createTestWorkflow()
	missionID := types.NewID()
	goal := "Test agent hints integration"

	scorer := NewDeterministicScorer()

	orchestrator := NewPlanningOrchestrator(
		WithScorer(scorer),
	)

	orchestrator.missionID = missionID
	orchestrator.originalGoal = goal
	orchestrator.currentPlan = createTestPlan(missionID, []string{"node1", "node2", "node3"})

	// Test Case 1: Agent provides high confidence hint - should NOT trigger replan
	t.Run("high confidence hint", func(t *testing.T) {
		result := &workflow.NodeResult{
			NodeID:      "node1",
			Status:      workflow.NodeStatusCompleted,
			CompletedAt: time.Now(),
		}

		hints := &harness.StepHints{
			Confidence:    0.95,
			SuggestedNext: []string{"node2"},
			KeyFindings:   []string{"Found vulnerability X"},
		}

		score, err := orchestrator.OnStepComplete(ctx, "node1", result, hints)
		require.NoError(t, err)
		require.NotNil(t, score)

		assert.True(t, score.Success)
		assert.False(t, score.ShouldReplan, "High confidence should not trigger replan")
		assert.NotNil(t, score.AgentHints)
		assert.Equal(t, 0.95, score.AgentHints.Confidence)
		assert.Equal(t, []string{"node2"}, score.AgentHints.SuggestedNext)
	})

	// Test Case 2: Agent explicitly requests replan
	t.Run("explicit replan request", func(t *testing.T) {
		result := &workflow.NodeResult{
			NodeID:      "node3",
			Status:      workflow.NodeStatusCompleted,
			CompletedAt: time.Now(),
		}

		hints := &harness.StepHints{
			Confidence:    0.7,
			ReplanReason:  "Target behavior changed, need to adjust strategy",
			SuggestedNext: []string{"alternative-path"},
		}

		score, err := orchestrator.OnStepComplete(ctx, "node3", result, hints)
		require.NoError(t, err)
		require.NotNil(t, score)

		// Explicit replan reason should override deterministic scoring
		assert.True(t, score.ShouldReplan, "Explicit replan reason should trigger replan")
		assert.Equal(t, hints.ReplanReason, score.ReplanReason)
	})
}

// TestIntegration_WorkflowStateDynamicModification tests DAG modifications during execution.
func TestIntegration_WorkflowStateDynamicModification(t *testing.T) {
	ctx := context.Background()

	wf := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "dynamic-modification-test",
		Nodes: map[string]*workflow.WorkflowNode{
			"step1": {
				ID:           "step1",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "agent-1",
				Dependencies: []string{},
				Timeout:      30 * time.Second,
			},
			"step2": {
				ID:           "step2",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "agent-2",
				Dependencies: []string{"step1"},
				Timeout:      30 * time.Second,
			},
			"step3": {
				ID:           "step3",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "agent-3",
				Dependencies: []string{"step1"},
				Timeout:      30 * time.Second,
			},
		},
		EntryPoints: []string{"step1"},
		ExitPoints:  []string{"step2", "step3"},
		CreatedAt:   time.Now(),
	}

	missionID := types.NewID()
	goal := "Test dynamic workflow modification"

	budgetMgr := NewDefaultBudgetManager()
	scorer := NewDeterministicScorer()

	orchestrator := NewPlanningOrchestrator(
		WithScorer(scorer),
		WithBudgetManager(budgetMgr),
	)

	// Generate initial plan
	plan, err := orchestrator.PreExecute(ctx, missionID, goal, wf, &mockAgentHarness{}, 12000)
	require.NoError(t, err)
	assert.Equal(t, 3, len(plan.OrderedSteps))

	// Create workflow state
	state := workflow.NewWorkflowState(wf)

	// Execute step1 successfully
	state.MarkNodeCompleted("step1", &workflow.NodeResult{
		NodeID:      "step1",
		Status:      workflow.NodeStatusCompleted,
		CompletedAt: time.Now(),
	})

	step1Score, err := orchestrator.OnStepComplete(ctx, "step1", &workflow.NodeResult{
		NodeID:      "step1",
		Status:      workflow.NodeStatusCompleted,
		CompletedAt: time.Now(),
	}, nil)
	require.NoError(t, err)
	assert.True(t, step1Score.Success)

	// Execute step2, but it fails
	state.MarkNodeFailed("step2", errors.New("step2 failed"))

	step2Score, err := orchestrator.OnStepComplete(ctx, "step2", &workflow.NodeResult{
		NodeID:      "step2",
		Status:      workflow.NodeStatusFailed,
		Error:       &workflow.NodeError{Message: "step2 failed"},
		CompletedAt: time.Now(),
	}, nil)
	require.NoError(t, err)
	assert.False(t, step2Score.Success)
	assert.True(t, step2Score.ShouldReplan)

	// Trigger replan (without replanner, should continue)
	replanResult, err := orchestrator.HandleReplan(ctx, step2Score, state)
	require.NoError(t, err)
	assert.Equal(t, ReplanActionContinue, replanResult.Action)
}
