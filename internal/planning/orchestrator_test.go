package planning

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// Mock implementations for testing

type mockStrategicPlanner struct {
	planFunc func(ctx context.Context, input StrategicPlanInput) (*StrategicPlan, error)
}

func (m *mockStrategicPlanner) Plan(ctx context.Context, input StrategicPlanInput) (*StrategicPlan, error) {
	if m.planFunc != nil {
		return m.planFunc(ctx, input)
	}
	return nil, fmt.Errorf("mock plan not implemented")
}

type mockStepScorer struct {
	scoreFunc func(ctx context.Context, input ScoreInput) (*StepScore, error)
}

func (m *mockStepScorer) Score(ctx context.Context, input ScoreInput) (*StepScore, error) {
	if m.scoreFunc != nil {
		return m.scoreFunc(ctx, input)
	}
	return nil, fmt.Errorf("mock score not implemented")
}

type mockTacticalReplanner struct {
	replanFunc func(ctx context.Context, input ReplanInput) (*ReplanResult, error)
}

func (m *mockTacticalReplanner) Replan(ctx context.Context, input ReplanInput) (*ReplanResult, error) {
	if m.replanFunc != nil {
		return m.replanFunc(ctx, input)
	}
	return nil, fmt.Errorf("mock replan not implemented")
}

type mockAgentHarness struct {
	harness.AgentHarness
}

// Helper functions

func createTestWorkflow() *workflow.Workflow {
	return &workflow.Workflow{
		ID:   types.NewID(),
		Name: "test-workflow",
		Nodes: map[string]*workflow.WorkflowNode{
			"node1": {
				ID:           "node1",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "test-agent",
				Dependencies: []string{},
				Timeout:      30 * time.Second,
			},
			"node2": {
				ID:           "node2",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "test-agent",
				Dependencies: []string{"node1"},
				Timeout:      30 * time.Second,
			},
			"node3": {
				ID:           "node3",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "test-agent",
				Dependencies: []string{"node1"},
				Timeout:      30 * time.Second,
			},
		},
		EntryPoints: []string{"node1"},
		ExitPoints:  []string{"node2", "node3"},
		CreatedAt:   time.Now(),
	}
}

func createTestPlan(missionID types.ID, nodeIDs []string) *StrategicPlan {
	steps := make([]PlannedStep, 0, len(nodeIDs))
	budgets := make(map[string]int)

	for i, nodeID := range nodeIDs {
		steps = append(steps, PlannedStep{
			NodeID:          nodeID,
			Priority:        i,
			AllocatedTokens: 1000,
			Rationale:       "test step",
		})
		budgets[nodeID] = 1000
	}

	return &StrategicPlan{
		ID:              types.NewID(),
		MissionID:       missionID,
		OrderedSteps:    steps,
		StepBudgets:     budgets,
		EstimatedTokens: len(nodeIDs) * 1000,
		Status:          PlanStatusApproved,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

// Tests

func TestNewPlanningOrchestrator(t *testing.T) {
	t.Run("creates orchestrator with defaults", func(t *testing.T) {
		orchestrator := NewPlanningOrchestrator()

		if orchestrator == nil {
			t.Fatal("expected orchestrator, got nil")
		}

		if orchestrator.strategicPlanner == nil {
			t.Error("expected default strategic planner")
		}

		if orchestrator.fallbackPlanner == nil {
			t.Error("expected default fallback planner")
		}

		// Scorer is nil by default, must be configured via WithScorer
		// This is intentional as there's no default scorer implementation yet

		if orchestrator.budgetManager == nil {
			t.Error("expected default budget manager")
		}

		if orchestrator.eventEmitter == nil {
			t.Error("expected default event emitter")
		}

		if orchestrator.planningTimeout != 30*time.Second {
			t.Errorf("expected timeout 30s, got %v", orchestrator.planningTimeout)
		}
	})

	t.Run("applies custom options", func(t *testing.T) {
		customPlanner := &mockStrategicPlanner{}
		customScorer := &mockStepScorer{}
		customTimeout := 60 * time.Second

		orchestrator := NewPlanningOrchestrator(
			WithStrategicPlanner(customPlanner),
			WithScorer(customScorer),
			WithPlanningTimeout(customTimeout),
		)

		if orchestrator.strategicPlanner != customPlanner {
			t.Error("custom strategic planner not applied")
		}

		if orchestrator.scorer != customScorer {
			t.Error("custom scorer not applied")
		}

		if orchestrator.planningTimeout != customTimeout {
			t.Error("custom timeout not applied")
		}
	})
}

func TestPlanningOrchestrator_PreExecute(t *testing.T) {
	t.Run("successful planning with primary planner", func(t *testing.T) {
		ctx := context.Background()
		wf := createTestWorkflow()
		missionID := types.NewID()
		goal := "test mission goal"

		expectedPlan := createTestPlan(missionID, []string{"node1", "node2", "node3"})

		mockPlanner := &mockStrategicPlanner{
			planFunc: func(ctx context.Context, input StrategicPlanInput) (*StrategicPlan, error) {
				return expectedPlan, nil
			},
		}

		orchestrator := NewPlanningOrchestrator(WithStrategicPlanner(mockPlanner))

		plan, err := orchestrator.PreExecute(ctx, missionID, goal, wf, &mockAgentHarness{}, 10000)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if plan == nil {
			t.Fatal("expected plan, got nil")
		}

		if plan.MissionID != missionID {
			t.Errorf("expected mission ID %v, got %v", missionID, plan.MissionID)
		}

		if orchestrator.originalGoal != goal {
			t.Errorf("expected goal %q, got %q", goal, orchestrator.originalGoal)
		}

		if orchestrator.currentPlan != plan {
			t.Error("current plan not stored in orchestrator")
		}
	})

	t.Run("falls back to default planner on primary failure", func(t *testing.T) {
		ctx := context.Background()
		wf := createTestWorkflow()
		missionID := types.NewID()
		goal := "test mission goal"

		failingPlanner := &mockStrategicPlanner{
			planFunc: func(ctx context.Context, input StrategicPlanInput) (*StrategicPlan, error) {
				return nil, errors.New("planning failed")
			},
		}

		orchestrator := NewPlanningOrchestrator(WithStrategicPlanner(failingPlanner))

		plan, err := orchestrator.PreExecute(ctx, missionID, goal, wf, &mockAgentHarness{}, 10000)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if plan == nil {
			t.Fatal("expected fallback plan, got nil")
		}

		// Fallback planner should produce a valid plan
		if len(plan.OrderedSteps) != 3 {
			t.Errorf("expected 3 steps from fallback, got %d", len(plan.OrderedSteps))
		}
	})

	t.Run("validates inputs", func(t *testing.T) {
		ctx := context.Background()
		orchestrator := NewPlanningOrchestrator()

		tests := []struct {
			name      string
			workflow  *workflow.Workflow
			goal      string
			expectErr string
		}{
			{
				name:      "nil workflow",
				workflow:  nil,
				goal:      "test",
				expectErr: "workflow cannot be nil",
			},
			{
				name:      "empty goal",
				workflow:  createTestWorkflow(),
				goal:      "",
				expectErr: "goal cannot be empty",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := orchestrator.PreExecute(ctx, types.NewID(), tt.goal, tt.workflow, &mockAgentHarness{}, 10000)
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if err.Error() != tt.expectErr {
					t.Errorf("expected error %q, got %q", tt.expectErr, err.Error())
				}
			})
		}
	})

	t.Run("validates plan against bounds", func(t *testing.T) {
		ctx := context.Background()
		wf := createTestWorkflow()
		missionID := types.NewID()

		// Create a plan with an invalid node
		invalidPlan := &StrategicPlan{
			ID:        types.NewID(),
			MissionID: missionID,
			OrderedSteps: []PlannedStep{
				{NodeID: "invalid-node", Priority: 0, AllocatedTokens: 1000},
			},
			StepBudgets:     map[string]int{"invalid-node": 1000},
			EstimatedTokens: 1000,
			Status:          PlanStatusDraft,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		mockPlanner := &mockStrategicPlanner{
			planFunc: func(ctx context.Context, input StrategicPlanInput) (*StrategicPlan, error) {
				return invalidPlan, nil
			},
		}

		orchestrator := NewPlanningOrchestrator(WithStrategicPlanner(mockPlanner))

		_, err := orchestrator.PreExecute(ctx, missionID, "test", wf, &mockAgentHarness{}, 10000)

		if err == nil {
			t.Fatal("expected validation error, got nil")
		}

		if !strings.Contains(err.Error(), "validation failed") {
			t.Errorf("expected validation error, got: %v", err)
		}
	})
}

func TestPlanningOrchestrator_OnStepComplete(t *testing.T) {
	t.Run("scores completed step successfully", func(t *testing.T) {
		ctx := context.Background()
		missionID := types.NewID()
		nodeID := "node1"

		mockScorer := &mockStepScorer{
			scoreFunc: func(ctx context.Context, input ScoreInput) (*StepScore, error) {
				return &StepScore{
					NodeID:        nodeID,
					MissionID:     missionID,
					Success:       true,
					Confidence:    0.9,
					ScoringMethod: "deterministic",
					FindingsCount: 2,
					ShouldReplan:  false,
					ScoredAt:      time.Now(),
					TokensUsed:    0,
				}, nil
			},
		}

		orchestrator := NewPlanningOrchestrator(WithScorer(mockScorer))
		orchestrator.missionID = missionID
		orchestrator.originalGoal = "test goal"
		orchestrator.currentPlan = createTestPlan(missionID, []string{"node1", "node2"})

		result := &workflow.NodeResult{
			NodeID:      nodeID,
			Status:      workflow.NodeStatusCompleted,
			CompletedAt: time.Now(),
		}

		score, err := orchestrator.OnStepComplete(ctx, nodeID, result, nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if score == nil {
			t.Fatal("expected score, got nil")
		}

		if !score.Success {
			t.Error("expected success=true")
		}

		if orchestrator.stepsScored != 1 {
			t.Errorf("expected stepsScored=1, got %d", orchestrator.stepsScored)
		}

		if orchestrator.successfulSteps != 1 {
			t.Errorf("expected successfulSteps=1, got %d", orchestrator.successfulSteps)
		}
	})

	t.Run("tracks failed steps in failure memory", func(t *testing.T) {
		ctx := context.Background()
		missionID := types.NewID()
		nodeID := "node1"

		mockScorer := &mockStepScorer{
			scoreFunc: func(ctx context.Context, input ScoreInput) (*StepScore, error) {
				return &StepScore{
					NodeID:        nodeID,
					MissionID:     missionID,
					Success:       false,
					Confidence:    0.3,
					ScoringMethod: "deterministic",
					FindingsCount: 0,
					ShouldReplan:  true,
					ReplanReason:  "step failed",
					ScoredAt:      time.Now(),
					TokensUsed:    0,
				}, nil
			},
		}

		orchestrator := NewPlanningOrchestrator(WithScorer(mockScorer))
		orchestrator.missionID = missionID
		orchestrator.originalGoal = "test goal"
		orchestrator.currentPlan = createTestPlan(missionID, []string{"node1", "node2"})

		result := &workflow.NodeResult{
			NodeID:      nodeID,
			Status:      workflow.NodeStatusFailed,
			CompletedAt: time.Now(),
		}

		score, err := orchestrator.OnStepComplete(ctx, nodeID, result, nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if score.Success {
			t.Error("expected success=false")
		}

		// Check failure memory was updated
		attempts := orchestrator.failureMemory.GetAttempts(nodeID)
		if len(attempts) != 1 {
			t.Errorf("expected 1 attempt in failure memory, got %d", len(attempts))
		}

		if orchestrator.successfulSteps != 0 {
			t.Errorf("expected successfulSteps=0, got %d", orchestrator.successfulSteps)
		}
	})

	t.Run("applies agent hints to score", func(t *testing.T) {
		ctx := context.Background()
		missionID := types.NewID()
		nodeID := "node1"

		mockScorer := &mockStepScorer{
			scoreFunc: func(ctx context.Context, input ScoreInput) (*StepScore, error) {
				return &StepScore{
					NodeID:        nodeID,
					MissionID:     missionID,
					Success:       true,
					Confidence:    0.8,
					ScoringMethod: "deterministic",
					ShouldReplan:  false,
					ScoredAt:      time.Now(),
					TokensUsed:    0,
				}, nil
			},
		}

		orchestrator := NewPlanningOrchestrator(WithScorer(mockScorer))
		orchestrator.missionID = missionID
		orchestrator.originalGoal = "test goal"
		orchestrator.currentPlan = createTestPlan(missionID, []string{"node1", "node2"})

		result := &workflow.NodeResult{
			NodeID:      nodeID,
			Status:      workflow.NodeStatusCompleted,
			CompletedAt: time.Now(),
		}

		hints := &harness.StepHints{
			Confidence:    0.95,
			SuggestedNext: []string{"node3"},
			ReplanReason:  "found better path",
			KeyFindings:   []string{"important discovery"},
		}

		score, err := orchestrator.OnStepComplete(ctx, nodeID, result, hints)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if score.AgentHints == nil {
			t.Fatal("expected agent hints in score")
		}

		if score.AgentHints.Confidence != hints.Confidence {
			t.Errorf("expected confidence %.2f, got %.2f", hints.Confidence, score.AgentHints.Confidence)
		}

		// Agent hint should override replan decision
		if !score.ShouldReplan {
			t.Error("expected shouldReplan=true from agent hint")
		}

		if score.ReplanReason != hints.ReplanReason {
			t.Errorf("expected replan reason %q, got %q", hints.ReplanReason, score.ReplanReason)
		}
	})

	t.Run("validates inputs", func(t *testing.T) {
		ctx := context.Background()
		orchestrator := NewPlanningOrchestrator()

		_, err := orchestrator.OnStepComplete(ctx, "node1", nil, nil)

		if err == nil {
			t.Fatal("expected error for nil result, got nil")
		}

		if !strings.Contains(err.Error(), "result cannot be nil") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestPlanningOrchestrator_HandleReplan(t *testing.T) {
	t.Run("performs successful replan with reorder action", func(t *testing.T) {
		ctx := context.Background()
		missionID := types.NewID()
		wf := createTestWorkflow()
		state := workflow.NewWorkflowState(wf)

		triggerScore := &StepScore{
			NodeID:       "node1",
			MissionID:    missionID,
			Success:      false,
			ShouldReplan: true,
			ReplanReason: "failed to make progress",
			ScoredAt:     time.Now(),
		}

		// Create a reorder that respects dependencies (node1 must come before node2 and node3)
		newPlan := createTestPlan(missionID, []string{"node1", "node3", "node2"})

		mockReplanner := &mockTacticalReplanner{
			replanFunc: func(ctx context.Context, input ReplanInput) (*ReplanResult, error) {
				return &ReplanResult{
					ModifiedPlan: newPlan,
					Action:       ReplanActionReorder,
					Rationale:    "reordered steps for better results",
				}, nil
			},
		}

		orchestrator := NewPlanningOrchestrator(WithReplanner(mockReplanner))
		orchestrator.missionID = missionID
		orchestrator.originalGoal = "test goal"
		orchestrator.currentPlan = createTestPlan(missionID, []string{"node1", "node2", "node3"})

		// Extract bounds for replanning
		bounds, _ := ExtractBounds(wf)
		orchestrator.bounds = bounds

		result, err := orchestrator.HandleReplan(ctx, triggerScore, state)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Action != ReplanActionReorder {
			t.Errorf("expected action reorder, got %v", result.Action)
		}

		// Check that replan was recorded in failure memory
		replanCount := orchestrator.failureMemory.ReplanCount()
		if replanCount != 1 {
			t.Errorf("expected replan count 1, got %d", replanCount)
		}

		// Check that current plan was updated
		if orchestrator.currentPlan != newPlan {
			t.Error("current plan was not updated")
		}
	})

	t.Run("rejects replan when count exceeds maximum", func(t *testing.T) {
		ctx := context.Background()
		missionID := types.NewID()
		wf := createTestWorkflow()
		state := workflow.NewWorkflowState(wf)

		triggerScore := &StepScore{
			NodeID:       "node1",
			MissionID:    missionID,
			Success:      false,
			ShouldReplan: true,
			ReplanReason: "failed",
			ScoredAt:     time.Now(),
		}

		mockReplanner := &mockTacticalReplanner{
			replanFunc: func(ctx context.Context, input ReplanInput) (*ReplanResult, error) {
				t.Fatal("replanner should not be called when limit exceeded")
				return nil, nil
			},
		}

		orchestrator := NewPlanningOrchestrator(WithReplanner(mockReplanner))
		orchestrator.missionID = missionID

		// Simulate maximum replans already reached
		for i := 0; i < MaxReplans; i++ {
			orchestrator.failureMemory.RecordReplan(ReplanRecord{
				Timestamp:   time.Now(),
				TriggerNode: "nodeX",
				Rationale:   "test replan",
			})
		}

		result, err := orchestrator.HandleReplan(ctx, triggerScore, state)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Action != ReplanActionFail {
			t.Errorf("expected action fail, got %v", result.Action)
		}

		if !strings.Contains(result.Rationale, "Maximum replan count") {
			t.Errorf("unexpected rationale: %v", result.Rationale)
		}
	})

	t.Run("continues when replanner is disabled", func(t *testing.T) {
		ctx := context.Background()
		wf := createTestWorkflow()
		state := workflow.NewWorkflowState(wf)

		triggerScore := &StepScore{
			NodeID:       "node1",
			Success:      false,
			ShouldReplan: true,
			ReplanReason: "failed",
			ScoredAt:     time.Now(),
		}

		orchestrator := NewPlanningOrchestrator() // No replanner configured

		result, err := orchestrator.HandleReplan(ctx, triggerScore, state)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Action != ReplanActionContinue {
			t.Errorf("expected action continue, got %v", result.Action)
		}

		if !strings.Contains(result.Rationale, "disabled") {
			t.Errorf("unexpected rationale: %v", result.Rationale)
		}
	})

	t.Run("handles skip action", func(t *testing.T) {
		ctx := context.Background()
		missionID := types.NewID()
		wf := createTestWorkflow()
		state := workflow.NewWorkflowState(wf)

		triggerScore := &StepScore{
			NodeID:       "node2",
			MissionID:    missionID,
			Success:      false,
			ShouldReplan: true,
			ReplanReason: "node not applicable",
			ScoredAt:     time.Now(),
		}

		modifiedPlan := createTestPlan(missionID, []string{"node2"}) // Just the node to skip

		mockReplanner := &mockTacticalReplanner{
			replanFunc: func(ctx context.Context, input ReplanInput) (*ReplanResult, error) {
				return &ReplanResult{
					ModifiedPlan: modifiedPlan,
					Action:       ReplanActionSkip,
					Rationale:    "skip unnecessary node",
				}, nil
			},
		}

		orchestrator := NewPlanningOrchestrator(WithReplanner(mockReplanner))
		orchestrator.missionID = missionID
		orchestrator.currentPlan = createTestPlan(missionID, []string{"node1", "node2", "node3"})

		bounds, _ := ExtractBounds(wf)
		orchestrator.bounds = bounds

		result, err := orchestrator.HandleReplan(ctx, triggerScore, state)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Action != ReplanActionSkip {
			t.Errorf("expected action skip, got %v", result.Action)
		}
	})

	t.Run("validates inputs", func(t *testing.T) {
		ctx := context.Background()
		orchestrator := NewPlanningOrchestrator(WithReplanner(&mockTacticalReplanner{}))

		tests := []struct {
			name         string
			triggerScore *StepScore
			state        *workflow.WorkflowState
			expectErr    string
		}{
			{
				name:         "nil trigger score",
				triggerScore: nil,
				state:        &workflow.WorkflowState{},
				expectErr:    "trigger score cannot be nil",
			},
			{
				name: "nil workflow state",
				triggerScore: &StepScore{
					NodeID:       "node1",
					ShouldReplan: true,
				},
				state:     nil,
				expectErr: "workflow state cannot be nil",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := orchestrator.HandleReplan(ctx, tt.triggerScore, tt.state)
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.expectErr) {
					t.Errorf("expected error containing %q, got %q", tt.expectErr, err.Error())
				}
			})
		}
	})
}

func TestPlanningOrchestrator_GetPlanningContext(t *testing.T) {
	t.Run("returns planning context for valid node", func(t *testing.T) {
		missionID := types.NewID()
		orchestrator := NewPlanningOrchestrator()
		orchestrator.originalGoal = "test goal"
		orchestrator.currentPlan = createTestPlan(missionID, []string{"node1", "node2", "node3"})

		ctx := orchestrator.GetPlanningContext("node2")

		if ctx == nil {
			t.Fatal("expected planning context, got nil")
		}

		if ctx.OriginalGoal != "test goal" {
			t.Errorf("expected goal %q, got %q", "test goal", ctx.OriginalGoal)
		}

		if ctx.CurrentPosition != 1 {
			t.Errorf("expected position 1, got %d", ctx.CurrentPosition)
		}

		if ctx.TotalSteps != 3 {
			t.Errorf("expected total steps 3, got %d", ctx.TotalSteps)
		}

		if len(ctx.RemainingSteps) != 1 {
			t.Errorf("expected 1 remaining step, got %d", len(ctx.RemainingSteps))
		}

		if ctx.RemainingSteps[0] != "node3" {
			t.Errorf("expected remaining step node3, got %v", ctx.RemainingSteps[0])
		}

		if ctx.StepBudget != 1000 {
			t.Errorf("expected step budget 1000, got %d", ctx.StepBudget)
		}
	})

	t.Run("returns nil for invalid node", func(t *testing.T) {
		missionID := types.NewID()
		orchestrator := NewPlanningOrchestrator()
		orchestrator.currentPlan = createTestPlan(missionID, []string{"node1", "node2"})

		ctx := orchestrator.GetPlanningContext("invalid-node")

		if ctx != nil {
			t.Error("expected nil context for invalid node")
		}
	})

	t.Run("returns nil when no plan exists", func(t *testing.T) {
		orchestrator := NewPlanningOrchestrator()
		orchestrator.currentPlan = nil

		ctx := orchestrator.GetPlanningContext("node1")

		if ctx != nil {
			t.Error("expected nil context when no plan exists")
		}
	})
}

func TestPlanningOrchestrator_GetReport(t *testing.T) {
	t.Run("generates report with metrics", func(t *testing.T) {
		missionID := types.NewID()
		orchestrator := NewPlanningOrchestrator()
		orchestrator.missionID = missionID
		orchestrator.currentPlan = createTestPlan(missionID, []string{"node1", "node2", "node3"})
		orchestrator.stepsScored = 3
		orchestrator.successfulSteps = 2

		report := orchestrator.GetReport()

		if report == nil {
			t.Fatal("expected report, got nil")
		}

		if report.MissionID != missionID {
			t.Errorf("expected mission ID %v, got %v", missionID, report.MissionID)
		}

		if report.StepsScored != 3 {
			t.Errorf("expected 3 steps scored, got %d", report.StepsScored)
		}

		expectedEffectiveness := (2.0 / 3.0) * 100
		// Allow slight floating point imprecision
		if report.EffectivenessRate < expectedEffectiveness-0.01 || report.EffectivenessRate > expectedEffectiveness+0.01 {
			t.Errorf("expected effectiveness %.2f, got %.2f", expectedEffectiveness, report.EffectivenessRate)
		}

		if report.BudgetReport == nil {
			t.Error("expected budget report")
		}
	})

	t.Run("handles zero steps scored", func(t *testing.T) {
		orchestrator := NewPlanningOrchestrator()
		orchestrator.stepsScored = 0
		orchestrator.successfulSteps = 0

		report := orchestrator.GetReport()

		if report.EffectivenessRate != 0.0 {
			t.Errorf("expected effectiveness 0.0, got %.2f", report.EffectivenessRate)
		}
	})
}

func TestPlanningOrchestrator_Integration(t *testing.T) {
	t.Run("complete planning lifecycle", func(t *testing.T) {
		ctx := context.Background()
		wf := createTestWorkflow()
		missionID := types.NewID()
		goal := "complete test mission"

		// Create orchestrator with all components including scorer
		mockScorer := &mockStepScorer{
			scoreFunc: func(ctx context.Context, input ScoreInput) (*StepScore, error) {
				// Simple deterministic scoring based on node result status
				success := input.NodeResult.Status == workflow.NodeStatusCompleted
				return &StepScore{
					NodeID:        input.NodeID,
					MissionID:     missionID,
					Success:       success,
					Confidence:    0.9,
					ScoringMethod: "mock",
					ShouldReplan:  false,
					ScoredAt:      time.Now(),
					TokensUsed:    0,
				}, nil
			},
		}
		orchestrator := NewPlanningOrchestrator(WithScorer(mockScorer))

		// Step 1: Pre-execute planning
		plan, err := orchestrator.PreExecute(ctx, missionID, goal, wf, &mockAgentHarness{}, 10000)
		if err != nil {
			t.Fatalf("PreExecute failed: %v", err)
		}

		if plan == nil {
			t.Fatal("expected plan from PreExecute")
		}

		// Step 2: Score a successful step
		result1 := &workflow.NodeResult{
			NodeID:      "node1",
			Status:      workflow.NodeStatusCompleted,
			CompletedAt: time.Now(),
		}

		score1, err := orchestrator.OnStepComplete(ctx, "node1", result1, nil)
		if err != nil {
			t.Fatalf("OnStepComplete failed: %v", err)
		}

		if !score1.Success {
			t.Error("expected successful step")
		}

		// Step 3: Score a failed step that triggers replanning
		result2 := &workflow.NodeResult{
			NodeID:      "node2",
			Status:      workflow.NodeStatusFailed,
			CompletedAt: time.Now(),
		}

		score2, err := orchestrator.OnStepComplete(ctx, "node2", result2, nil)
		if err != nil {
			t.Fatalf("OnStepComplete failed: %v", err)
		}

		if score2.Success {
			t.Error("expected failed step")
		}

		// Step 4: Get final report
		report := orchestrator.GetReport()

		if report.StepsScored != 2 {
			t.Errorf("expected 2 steps scored, got %d", report.StepsScored)
		}

		effectivenessRate := 50.0
		if report.EffectivenessRate != effectivenessRate {
			t.Errorf("expected effectiveness %.2f, got %.2f", effectivenessRate, report.EffectivenessRate)
		}
	})
}
