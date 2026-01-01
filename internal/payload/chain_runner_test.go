package payload

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestNewChainRunner tests chain runner creation
func TestNewChainRunner(t *testing.T) {
	db, payloadStore, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	executionStore := NewExecutionStore(db)

	// For executor tests, we need a finding store which would require finding.Database
	// Since we only need the runner, we can skip the full executor setup
	executor := &mockExecutor{}

	runner := NewChainRunnerWithDefaults(executor, registry, executionStore)

	require.NotNil(t, runner)
	_ = payloadStore // Silence unused var
}

// mockExecutor implements PayloadExecutor for chain runner tests
type mockExecutor struct{}

func (m *mockExecutor) Execute(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error) {
	// Return a simple successful result
	result := NewExecutionResult(types.NewID())
	result.Success = true
	result.ConfidenceScore = 0.9
	result.Response = "Simulated response"
	result.InstantiatedText = req.Parameters["input"].(string)
	return result, nil
}

func (m *mockExecutor) ExecuteDryRun(ctx context.Context, req *ExecutionRequest) (*DryRunResult, error) {
	return NewDryRunResult(), nil
}

func (m *mockExecutor) ValidateParameters(payload *Payload, params map[string]interface{}) error {
	return nil
}

// TestChainRunner_Execute tests basic chain execution
func TestChainRunner_Execute(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	executionStore := NewExecutionStore(db)
	executor := &mockExecutor{}

	ctx := context.Background()

	t.Run("execute simple sequential chain", func(t *testing.T) {
		// Create test payloads
		payload1 := createTestPayload("chain-payload-1")
		payload1.Template = "Stage 1: {{input}}"
		payload1.Parameters = []ParameterDef{
			{Name: "input", Type: ParameterTypeString, Required: true},
		}
		err := registry.Register(ctx, payload1)
		require.NoError(t, err)

		payload2 := createTestPayload("chain-payload-2")
		payload2.Template = "Stage 2: {{input}}"
		payload2.Parameters = []ParameterDef{
			{Name: "input", Type: ParameterTypeString, Required: true},
		}
		err = registry.Register(ctx, payload2)
		require.NoError(t, err)

		// Create attack chain
		chain := createTestChain("test-chain-sequential", payload1.ID, payload2.ID)

		// For now, since we don't have a chain store, we'll test the error case
		runner := NewChainRunnerWithDefaults(executor, registry, executionStore)

		req := NewChainExecutionRequest(chain.ID, types.NewID(), types.NewID())
		req.Parameters = map[string]interface{}{
			"input": "test",
		}

		// Execute - will fail because chain store is not implemented
		result, err := runner.Execute(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "chain store not implemented")
		assert.Nil(t, result)
	})

	t.Run("execute with nil request", func(t *testing.T) {
		runner := NewChainRunnerWithDefaults(executor, registry, executionStore)

		result, err := runner.Execute(ctx, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
		assert.Nil(t, result)
	})

	t.Run("execute with empty chain ID", func(t *testing.T) {
		runner := NewChainRunnerWithDefaults(executor, registry, executionStore)

		req := NewChainExecutionRequest("", types.NewID(), types.NewID())
		result, err := runner.Execute(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "chain ID is required")
		assert.Nil(t, result)
	})

	t.Run("execute with empty target ID", func(t *testing.T) {
		runner := NewChainRunnerWithDefaults(executor, registry, executionStore)

		req := NewChainExecutionRequest(types.NewID(), "", types.NewID())
		result, err := runner.Execute(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "target ID is required")
		assert.Nil(t, result)
	})
}

// TestChainRunner_ExecutionPlan tests execution plan building
func TestChainRunner_ExecutionPlan(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	executionStore := NewExecutionStore(db)
	executor := &mockExecutor{}

	runner := NewChainRunnerWithDefaults(executor, registry, executionStore)

	// Access the internal runner to test plan building
	internalRunner := runner.(*chainRunner)

	t.Run("build plan for simple sequential chain", func(t *testing.T) {
		chain := &AttackChain{
			ID:   types.NewID(),
			Name: "Sequential Test",
			Stages: map[string]*ChainStage{
				"stage1": {
					ID:           "stage1",
					Name:         "Stage 1",
					PayloadID:    types.NewID(),
					Dependencies: []string{},
				},
				"stage2": {
					ID:           "stage2",
					Name:         "Stage 2",
					PayloadID:    types.NewID(),
					Dependencies: []string{"stage1"},
				},
				"stage3": {
					ID:           "stage3",
					Name:         "Stage 3",
					PayloadID:    types.NewID(),
					Dependencies: []string{"stage2"},
				},
			},
			EntryStages: []string{"stage1"},
			Enabled:     true,
		}

		plan, err := internalRunner.buildExecutionPlan(chain)
		require.NoError(t, err)
		assert.NotNil(t, plan)
		assert.Equal(t, 3, len(plan.stages))
		assert.Equal(t, 3, len(plan.levels))

		// Stage 1 should be at level 0
		assert.Equal(t, 1, len(plan.levels[0]))
		assert.Equal(t, "stage1", plan.levels[0][0].ID)

		// Stage 2 should be at level 1
		assert.Equal(t, 1, len(plan.levels[1]))
		assert.Equal(t, "stage2", plan.levels[1][0].ID)

		// Stage 3 should be at level 2
		assert.Equal(t, 1, len(plan.levels[2]))
		assert.Equal(t, "stage3", plan.levels[2][0].ID)
	})

	t.Run("build plan for parallel stages", func(t *testing.T) {
		chain := &AttackChain{
			ID:   types.NewID(),
			Name: "Parallel Test",
			Stages: map[string]*ChainStage{
				"stage1": {
					ID:           "stage1",
					Name:         "Stage 1",
					PayloadID:    types.NewID(),
					Dependencies: []string{},
					Parallel:     true,
				},
				"stage2": {
					ID:           "stage2",
					Name:         "Stage 2",
					PayloadID:    types.NewID(),
					Dependencies: []string{},
					Parallel:     true,
				},
				"stage3": {
					ID:           "stage3",
					Name:         "Stage 3",
					PayloadID:    types.NewID(),
					Dependencies: []string{"stage1", "stage2"},
				},
			},
			EntryStages: []string{"stage1", "stage2"},
			Enabled:     true,
		}

		plan, err := internalRunner.buildExecutionPlan(chain)
		require.NoError(t, err)
		assert.NotNil(t, plan)
		assert.Equal(t, 3, len(plan.stages))

		// Stages 1 and 2 should be at level 0 (can run in parallel)
		assert.Equal(t, 2, len(plan.levels[0]))

		// Stage 3 should be at level 1 (depends on both)
		assert.Equal(t, 1, len(plan.levels[1]))
		assert.Equal(t, "stage3", plan.levels[1][0].ID)
	})

	t.Run("build plan detects circular dependency", func(t *testing.T) {
		chain := &AttackChain{
			ID:   types.NewID(),
			Name: "Circular Test",
			Stages: map[string]*ChainStage{
				"stage1": {
					ID:           "stage1",
					Name:         "Stage 1",
					PayloadID:    types.NewID(),
					Dependencies: []string{"stage2"},
				},
				"stage2": {
					ID:           "stage2",
					Name:         "Stage 2",
					PayloadID:    types.NewID(),
					Dependencies: []string{"stage1"},
				},
			},
			EntryStages: []string{"stage1"},
			Enabled:     true,
		}

		_, err := internalRunner.buildExecutionPlan(chain)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "circular dependency")
	})

	t.Run("build plan with missing stage", func(t *testing.T) {
		chain := &AttackChain{
			ID:   types.NewID(),
			Name: "Missing Stage Test",
			Stages: map[string]*ChainStage{
				"stage1": {
					ID:           "stage1",
					Name:         "Stage 1",
					PayloadID:    types.NewID(),
					Dependencies: []string{"nonexistent"},
				},
			},
			EntryStages: []string{"stage1"},
			Enabled:     true,
		}

		_, err := internalRunner.buildExecutionPlan(chain)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// TestChainRunner_PauseResume tests pause and resume functionality
func TestChainRunner_PauseResume(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	executionStore := NewExecutionStore(db)
	executor := &mockExecutor{}
	runner := NewChainRunnerWithDefaults(executor, registry, executionStore)

	ctx := context.Background()

	t.Run("pause non-existent execution", func(t *testing.T) {
		nonExistentID := types.NewID()
		err := runner.Pause(ctx, nonExistentID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("resume non-existent execution", func(t *testing.T) {
		nonExistentID := types.NewID()
		err := runner.Resume(ctx, nonExistentID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("get progress for non-existent execution", func(t *testing.T) {
		nonExistentID := types.NewID()
		_, err := runner.GetProgress(ctx, nonExistentID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// TestChainProgress tests chain progress tracking
func TestChainProgress(t *testing.T) {
	t.Run("create new chain progress", func(t *testing.T) {
		chainExecID := types.NewID()
		chainID := types.NewID()
		totalStages := 5

		progress := NewChainProgress(chainExecID, chainID, totalStages)
		require.NotNil(t, progress)
		assert.Equal(t, chainExecID, progress.ChainExecutionID)
		assert.Equal(t, chainID, progress.ChainID)
		assert.Equal(t, totalStages, progress.TotalStages)
		assert.Equal(t, ChainStatusPending, progress.Status)
		assert.Equal(t, 0, progress.CompletedStages)
	})

	t.Run("start chain progress", func(t *testing.T) {
		progress := NewChainProgress(types.NewID(), types.NewID(), 3)
		progress.Start()

		assert.Equal(t, ChainStatusRunning, progress.Status)
		assert.False(t, progress.StartedAt.IsZero())
	})

	t.Run("complete chain progress", func(t *testing.T) {
		progress := NewChainProgress(types.NewID(), types.NewID(), 3)
		progress.Start()
		progress.Complete()

		assert.Equal(t, ChainStatusCompleted, progress.Status)
		assert.NotNil(t, progress.CompletedAt)
	})

	t.Run("fail chain progress", func(t *testing.T) {
		progress := NewChainProgress(types.NewID(), types.NewID(), 3)
		progress.Start()

		err := fmt.Errorf("test error")
		progress.Fail(err)

		assert.Equal(t, ChainStatusFailed, progress.Status)
		assert.Equal(t, "test error", progress.ErrorMessage)
		assert.NotNil(t, progress.CompletedAt)
	})

	t.Run("pause and resume chain progress", func(t *testing.T) {
		progress := NewChainProgress(types.NewID(), types.NewID(), 3)
		progress.Start()
		assert.Equal(t, ChainStatusRunning, progress.Status)

		progress.Pause()
		assert.Equal(t, ChainStatusPaused, progress.Status)

		progress.Resume()
		assert.Equal(t, ChainStatusRunning, progress.Status)
	})

	t.Run("add stage result", func(t *testing.T) {
		progress := NewChainProgress(types.NewID(), types.NewID(), 3)
		progress.Start()

		result := StageResult{
			StageID:        types.NewID(),
			StageName:      "Test Stage",
			Success:        true,
			FindingCreated: true,
			Duration:       100 * time.Millisecond,
			TokensUsed:     50,
			Cost:           0.001,
		}

		progress.AddStageResult(result)

		assert.Equal(t, 1, progress.CompletedStages)
		assert.Equal(t, 1, progress.TotalExecutions)
		assert.Equal(t, 1, progress.SuccessfulAttacks)
		assert.Equal(t, 1, progress.TotalFindings)
		assert.Equal(t, int64(100), int64(progress.TotalDuration/time.Millisecond))
		assert.Equal(t, 50, progress.TotalTokensUsed)
		assert.Equal(t, 0.001, progress.TotalCost)
	})

	t.Run("calculate percent complete", func(t *testing.T) {
		progress := NewChainProgress(types.NewID(), types.NewID(), 4)

		assert.Equal(t, 0.0, progress.PercentComplete())

		progress.CompletedStages = 2
		assert.Equal(t, 50.0, progress.PercentComplete())

		progress.CompletedStages = 4
		assert.Equal(t, 100.0, progress.PercentComplete())
	})

	t.Run("set current stage", func(t *testing.T) {
		progress := NewChainProgress(types.NewID(), types.NewID(), 3)
		stageID := types.NewID()

		progress.SetCurrentStage(1, stageID)

		assert.Equal(t, 1, progress.CurrentStageIndex)
		assert.NotNil(t, progress.CurrentStageID)
		assert.Equal(t, stageID, *progress.CurrentStageID)
	})
}

// TestStageResult tests stage result creation and handling
func TestStageResult(t *testing.T) {
	t.Run("create new stage result", func(t *testing.T) {
		stageID := types.NewID()
		stageName := "Test Stage"
		stageIndex := 0
		payloadID := types.NewID()
		executionID := types.NewID()

		result := NewStageResult(stageID, stageName, stageIndex, payloadID, executionID)
		require.NotNil(t, result)

		assert.Equal(t, stageID, result.StageID)
		assert.Equal(t, stageName, result.StageName)
		assert.Equal(t, stageIndex, result.StageIndex)
		assert.Equal(t, payloadID, result.PayloadID)
		assert.Equal(t, executionID, result.ExecutionID)
		assert.Equal(t, ExecutionStatusCompleted, result.Status)
		assert.False(t, result.StartedAt.IsZero())
		assert.False(t, result.CompletedAt.IsZero())
	})
}

// TestChainResult tests chain result handling
func TestChainResult(t *testing.T) {
	t.Run("create new chain result", func(t *testing.T) {
		chainExecID := types.NewID()
		chainID := types.NewID()

		result := NewChainResult(chainExecID, chainID)
		require.NotNil(t, result)

		assert.Equal(t, chainExecID, result.ChainExecutionID)
		assert.Equal(t, chainID, result.ChainID)
		assert.Equal(t, ChainStatusCompleted, result.Status)
		assert.Equal(t, 0, result.StagesExecuted)
		assert.Equal(t, 0, result.SuccessfulStages)
		assert.Equal(t, 0, result.FailedStages)
	})

	t.Run("add stage results", func(t *testing.T) {
		result := NewChainResult(types.NewID(), types.NewID())

		// Add successful stage
		successResult := StageResult{
			StageID:        types.NewID(),
			StageName:      "Success Stage",
			Success:        true,
			FindingCreated: true,
			Duration:       100 * time.Millisecond,
			TokensUsed:     50,
			Cost:           0.001,
		}
		result.AddStageResult(successResult)

		assert.Equal(t, 1, result.StagesExecuted)
		assert.Equal(t, 1, result.SuccessfulStages)
		assert.Equal(t, 0, result.FailedStages)
		assert.Equal(t, 1, result.TotalFindings)

		// Add failed stage
		failResult := StageResult{
			StageID:        types.NewID(),
			StageName:      "Fail Stage",
			Success:        false,
			FindingCreated: false,
			Duration:       50 * time.Millisecond,
			TokensUsed:     25,
			Cost:           0.0005,
		}
		result.AddStageResult(failResult)

		assert.Equal(t, 2, result.StagesExecuted)
		assert.Equal(t, 1, result.SuccessfulStages)
		assert.Equal(t, 1, result.FailedStages)
		assert.Equal(t, 1, result.TotalFindings)
		assert.Equal(t, 75, result.TotalTokensUsed)
		assert.Equal(t, 0.0015, result.TotalCost)
	})

	t.Run("is success determination", func(t *testing.T) {
		result := NewChainResult(types.NewID(), types.NewID())

		// Initially successful (no stages)
		assert.True(t, result.IsSuccess())

		// Add successful stage
		successResult := StageResult{
			StageID:   types.NewID(),
			StageName: "Success Stage",
			Success:   true,
		}
		result.AddStageResult(successResult)
		assert.True(t, result.IsSuccess())

		// Add failed stage
		failResult := StageResult{
			StageID:   types.NewID(),
			StageName: "Fail Stage",
			Success:   false,
		}
		result.AddStageResult(failResult)
		assert.False(t, result.IsSuccess())
	})
}

// TestChainExecutionRequest tests chain execution request creation
func TestChainExecutionRequest(t *testing.T) {
	t.Run("create new chain execution request", func(t *testing.T) {
		chainID := types.NewID()
		targetID := types.NewID()
		agentID := types.NewID()

		req := NewChainExecutionRequest(chainID, targetID, agentID)
		require.NotNil(t, req)

		assert.Equal(t, chainID, req.ChainID)
		assert.Equal(t, targetID, req.TargetID)
		assert.Equal(t, agentID, req.AgentID)
		assert.Equal(t, 5*time.Minute, req.Timeout)
		assert.True(t, req.StopOnFailure)
		assert.False(t, req.ContinueOnError)
		assert.Equal(t, 0, req.MaxRetries)
		assert.False(t, req.ParallelStages)
	})
}

// TestConditionEvaluation tests condition evaluation
func TestConditionEvaluation(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	executionStore := NewExecutionStore(db)
	executor := &mockExecutor{}

	runner := NewChainRunnerWithDefaults(executor, registry, executionStore)

	// Access the internal runner
	internalRunner := runner.(*chainRunner)

	t.Run("evaluate success rate condition", func(t *testing.T) {
		progress := NewChainProgress(types.NewID(), types.NewID(), 3)
		progress.TotalExecutions = 4
		progress.SuccessfulAttacks = 3 // 75% success rate

		condition := &StageCondition{
			Type:      ConditionTypeSuccessRate,
			Threshold: 0.5, // Require 50% success
		}

		shouldExecute, err := internalRunner.evaluateCondition(condition, progress)
		require.NoError(t, err)
		assert.True(t, shouldExecute)

		// Test with higher threshold
		condition.Threshold = 0.8 // Require 80% success
		shouldExecute, err = internalRunner.evaluateCondition(condition, progress)
		require.NoError(t, err)
		assert.False(t, shouldExecute)
	})

	t.Run("evaluate with no previous executions", func(t *testing.T) {
		progress := NewChainProgress(types.NewID(), types.NewID(), 3)

		condition := &StageCondition{
			Type:      ConditionTypeSuccessRate,
			Threshold: 0.9,
		}

		shouldExecute, err := internalRunner.evaluateCondition(condition, progress)
		require.NoError(t, err)
		assert.True(t, shouldExecute) // Should proceed when no previous executions
	})

	t.Run("evaluate unsupported condition type", func(t *testing.T) {
		progress := NewChainProgress(types.NewID(), types.NewID(), 3)

		condition := &StageCondition{
			Type: ConditionType("unsupported"),
		}

		shouldExecute, err := internalRunner.evaluateCondition(condition, progress)
		assert.Error(t, err)
		assert.False(t, shouldExecute)
	})
}

// Helper function to create a test attack chain
func createTestChain(name string, payload1ID, payload2ID types.ID) *AttackChain {
	return &AttackChain{
		ID:          types.NewID(),
		Name:        name,
		Description: "Test attack chain",
		Stages: map[string]*ChainStage{
			"stage1": {
				ID:           "stage1",
				Name:         "Stage 1",
				Description:  "First stage",
				PayloadID:    payload1ID,
				Parameters:   map[string]any{},
				Dependencies: []string{},
				OnFailure:    FailureActionStop,
				Parallel:     false,
			},
			"stage2": {
				ID:           "stage2",
				Name:         "Stage 2",
				Description:  "Second stage",
				PayloadID:    payload2ID,
				Parameters:   map[string]any{},
				Dependencies: []string{"stage1"},
				OnFailure:    FailureActionStop,
				Parallel:     false,
			},
		},
		EntryStages: []string{"stage1"},
		Enabled:     true,
		Metadata: ChainMetadata{
			Author:  "test",
			Version: "1.0.0",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}
