package planning

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// TestStrategicPlannerInterface_LLMAndDefaultImplementations verifies both planners implement the interface
func TestStrategicPlannerInterface_LLMAndDefaultImplementations(t *testing.T) {
	// Verify DefaultOrderPlanner implements StrategicPlanner
	var _ StrategicPlanner = (*DefaultOrderPlanner)(nil)

	// Verify LLMStrategicPlanner implements StrategicPlanner (when properly initialized)
	mockHarness := &MockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{
				Message: llm.Message{Content: "{}"},
				Usage:   llm.CompletionTokenUsage{TotalTokens: 100},
			}, nil
		},
	}
	budgetMgr := NewDefaultBudgetManager()
	budgetMgr.Allocate(10000, 1.0)

	var _ StrategicPlanner = NewLLMStrategicPlanner(mockHarness, "primary", budgetMgr)
	var _ StrategicPlanner = NewDefaultOrderPlanner()
}

// TestStrategicPlanner_FallbackToDefault tests using default planner when LLM fails
func TestStrategicPlanner_FallbackToDefault(t *testing.T) {
	wf := &workflow.Workflow{
		Nodes: map[string]*workflow.WorkflowNode{
			"step1": {
				ID:           "step1",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "agent1",
				Timeout:      30 * time.Second,
				Dependencies: []string{},
			},
			"step2": {
				ID:           "step2",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "agent2",
				Timeout:      60 * time.Second,
				Dependencies: []string{"step1"},
			},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	budget := &BudgetAllocation{
		StrategicPlanning: 1000,
		Execution:         8000,
		TotalTokens:       10000,
	}

	target := harness.TargetInfo{
		Type: "llm",
		Name: "test-target",
	}

	input := StrategicPlanInput{
		Workflow:     wf,
		Bounds:       bounds,
		Target:       target,
		Budget:       budget,
		OriginalGoal: "Test goal",
	}

	// Test 1: LLM planner fails
	failingHarness := &MockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return nil, errors.New("LLM service unavailable")
		},
		targetInfo: target,
	}

	budgetMgr := NewDefaultBudgetManager()
	budgetMgr.Allocate(10000, 1.0)

	llmPlanner := NewLLMStrategicPlanner(failingHarness, "primary", budgetMgr)
	llmPlan, llmErr := llmPlanner.Plan(context.Background(), input)

	// LLM planner should return error
	assert.Error(t, llmErr)
	assert.Nil(t, llmPlan)
	assert.Contains(t, llmErr.Error(), "LLM completion failed")

	// Test 2: Fall back to default planner
	defaultPlanner := NewDefaultOrderPlanner()
	defaultPlan, defaultErr := defaultPlanner.Plan(context.Background(), input)

	// Default planner should succeed
	require.NoError(t, defaultErr)
	require.NotNil(t, defaultPlan)
	assert.Equal(t, 2, len(defaultPlan.OrderedSteps))
	assert.Equal(t, PlanStatusDraft, defaultPlan.Status)

	// Verify fallback plan is valid
	assert.Equal(t, "step1", defaultPlan.OrderedSteps[0].NodeID)
	assert.Equal(t, "step2", defaultPlan.OrderedSteps[1].NodeID)
	assert.Equal(t, 0, defaultPlan.OrderedSteps[0].Priority)
	assert.Equal(t, 1, defaultPlan.OrderedSteps[1].Priority)
}

// TestStrategicPlanner_BudgetExhaustion tests scenarios where planning budget is exhausted
func TestStrategicPlanner_BudgetExhaustion(t *testing.T) {
	tests := []struct {
		name                string
		planningBudget      int
		mockLLMTokens       int
		expectPlanningError bool
	}{
		{
			name:                "sufficient budget",
			planningBudget:      1000,
			mockLLMTokens:       600, // Less than planning budget to avoid exhaustion
			expectPlanningError: false,
		},
		{
			name:                "zero planning budget",
			planningBudget:      0,
			mockLLMTokens:       500,
			expectPlanningError: true,
		},
		{
			name:                "budget exhausted after LLM call",
			planningBudget:      500, // Enough for estimate, but will be exhausted after actual usage
			mockLLMTokens:       600, // More than planning budget - will fail on Consume()
			expectPlanningError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf := &workflow.Workflow{
				Nodes: map[string]*workflow.WorkflowNode{
					"test": {
						ID:        "test",
						Type:      workflow.NodeTypeAgent,
						AgentName: "test-agent",
						Timeout:   30 * time.Second,
					},
				},
			}

			bounds, err := ExtractBounds(wf)
			require.NoError(t, err)

			budget := &BudgetAllocation{
				StrategicPlanning: tt.planningBudget,
				Execution:         5000,
				TotalTokens:       tt.planningBudget + 5000,
			}

			// Create mock harness that returns valid JSON
			llmResponse := `{
  "ordered_steps": [
    {"node_id": "test", "priority": 1, "allocated_tokens": 1000, "rationale": "Test step"}
  ],
  "skipped_nodes": [],
  "parallel_groups": [],
  "rationale": "Test plan",
  "memory_influences": []
}`

			mockHarness := &MockHarness{
				completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
					return &llm.CompletionResponse{
						Message: llm.Message{
							Role:    llm.RoleAssistant,
							Content: llmResponse,
						},
						Usage: llm.CompletionTokenUsage{
							PromptTokens:     tt.mockLLMTokens / 2,
							CompletionTokens: tt.mockLLMTokens / 2,
							TotalTokens:      tt.mockLLMTokens,
						},
					}, nil
				},
				targetInfo: harness.TargetInfo{Type: "test"},
			}

			budgetMgr := NewDefaultBudgetManager()
			// Allocate the actual total budget from the test case
			budgetMgr.Allocate(budget.TotalTokens, 1.0)

			planner := NewLLMStrategicPlanner(mockHarness, "primary", budgetMgr)

			input := StrategicPlanInput{
				Workflow:     wf,
				Bounds:       bounds,
				Target:       mockHarness.targetInfo,
				Budget:       budget,
				OriginalGoal: "Test goal",
			}

			plan, err := planner.Plan(context.Background(), input)

			if tt.expectPlanningError {
				assert.Error(t, err)
				assert.Nil(t, plan)
				// Verify error mentions budget
				if err != nil {
					errMsg := err.Error()
					assert.True(t,
						contains(errMsg, "budget") || contains(errMsg, "exhausted") || contains(errMsg, "no strategic planning budget"),
						"error should mention budget: %s", errMsg)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, plan)
			}
		})
	}
}

// TestStrategicPlanner_InvalidPlanRejection tests that invalid plans are rejected
func TestStrategicPlanner_InvalidPlanRejection(t *testing.T) {
	tests := []struct {
		name        string
		llmResponse string
		wantErr     string
	}{
		{
			name: "node not in workflow",
			llmResponse: `{
  "ordered_steps": [
    {"node_id": "valid", "priority": 1, "allocated_tokens": 1000, "rationale": "Valid"},
    {"node_id": "invalid_node", "priority": 2, "allocated_tokens": 1000, "rationale": "Invalid"}
  ],
  "skipped_nodes": [],
  "parallel_groups": [],
  "rationale": "Test",
  "memory_influences": []
}`,
			wantErr: "invalid node",
		},
		{
			name: "skip decision for non-existent node",
			llmResponse: `{
  "ordered_steps": [
    {"node_id": "valid", "priority": 1, "allocated_tokens": 2000, "rationale": "Valid"}
  ],
  "skipped_nodes": [
    {"node_id": "nonexistent", "reason": "Not needed", "condition": ""}
  ],
  "parallel_groups": [],
  "rationale": "Test",
  "memory_influences": []
}`,
			wantErr: "invalid node",
		},
		{
			name: "budget exceeded",
			llmResponse: `{
  "ordered_steps": [
    {"node_id": "valid", "priority": 1, "allocated_tokens": 10000, "rationale": "Too much budget"}
  ],
  "skipped_nodes": [],
  "parallel_groups": [],
  "rationale": "Over budget",
  "memory_influences": []
}`,
			wantErr: "exceeds execution budget",
		},
		{
			name: "parallel group exceeds max parallelism",
			llmResponse: `{
  "ordered_steps": [
    {"node_id": "node1", "priority": 1, "allocated_tokens": 500, "rationale": "Step 1"},
    {"node_id": "node2", "priority": 1, "allocated_tokens": 500, "rationale": "Step 2"},
    {"node_id": "node3", "priority": 1, "allocated_tokens": 500, "rationale": "Step 3"}
  ],
  "skipped_nodes": [],
  "parallel_groups": [["node1", "node2", "node3"]],
  "rationale": "Too much parallelism",
  "memory_influences": []
}`,
			wantErr: "exceeds max parallelism",
		},
		{
			name: "parallel group with invalid node",
			llmResponse: `{
  "ordered_steps": [
    {"node_id": "valid", "priority": 1, "allocated_tokens": 1000, "rationale": "Valid"}
  ],
  "skipped_nodes": [],
  "parallel_groups": [["valid", "invalid_node"]],
  "rationale": "Test",
  "memory_influences": []
}`,
			wantErr: "invalid node", // Will actually fail on max parallelism first if bounds.MaxParallel is 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create workflow with limited nodes
			wf := &workflow.Workflow{
				Nodes: map[string]*workflow.WorkflowNode{
					"valid": {ID: "valid", Type: workflow.NodeTypeAgent, AgentName: "test"},
					"node1": {ID: "node1", Type: workflow.NodeTypeAgent, AgentName: "test1"},
					"node2": {ID: "node2", Type: workflow.NodeTypeAgent, AgentName: "test2"},
					"node3": {ID: "node3", Type: workflow.NodeTypeAgent, AgentName: "test3"},
				},
			}

			bounds, err := ExtractBounds(wf)
			require.NoError(t, err)

			// Set low max parallelism for the parallel group test
			if contains(tt.name, "parallel group exceeds") {
				bounds.MaxParallel = 2
			}
			// For the invalid node test, we need higher max parallelism to reach the invalid node check
			if contains(tt.name, "parallel group with invalid") {
				bounds.MaxParallel = 5 // High enough to pass parallelism check
			}

			budget := &BudgetAllocation{
				StrategicPlanning: 1000,
				Execution:         5000,
				TotalTokens:       6000,
			}

			mockHarness := &MockHarness{
				completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
					return &llm.CompletionResponse{
						Message: llm.Message{Content: tt.llmResponse},
						Usage:   llm.CompletionTokenUsage{TotalTokens: 400},
					}, nil
				},
				targetInfo: harness.TargetInfo{Type: "test"},
			}

			budgetMgr := NewDefaultBudgetManager()
			budgetMgr.Allocate(6000, 1.0)

			planner := NewLLMStrategicPlanner(mockHarness, "primary", budgetMgr)

			input := StrategicPlanInput{
				Workflow:     wf,
				Bounds:       bounds,
				Target:       mockHarness.targetInfo,
				Budget:       budget,
				OriginalGoal: "Test",
			}

			plan, err := planner.Plan(context.Background(), input)

			require.Error(t, err)
			assert.Nil(t, plan)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestStrategicPlanner_MemoryIntegration tests memory query integration
func TestStrategicPlanner_MemoryIntegration(t *testing.T) {
	wf := &workflow.Workflow{
		Nodes: map[string]*workflow.WorkflowNode{
			"recon": {
				ID:        "recon",
				Type:      workflow.NodeTypeAgent,
				AgentName: "recon-agent",
				Timeout:   30 * time.Second,
			},
			"attack": {
				ID:           "attack",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "attack-agent",
				Timeout:      60 * time.Second,
				Dependencies: []string{"recon"},
			},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	budget := &BudgetAllocation{
		StrategicPlanning: 2000,
		Execution:         8000,
		TotalTokens:       10000,
	}

	target := harness.TargetInfo{
		Type:     "llm",
		Name:     "gpt-4",
		Provider: "openai",
		URL:      "https://api.openai.com/v1/chat/completions",
	}

	// Create memory context with historical data
	memoryContext := &MemoryQueryResult{
		QueryID:       "test-query-123",
		TargetProfile: "type=llm,name=gpt-4,provider=openai",
		SuccessfulTechniques: []TechniqueRecord{
			{
				Technique:    "prompt-injection",
				SuccessRate:  0.85,
				AvgTokenCost: 1500,
				LastUsed:     time.Now().Add(-24 * time.Hour),
			},
			{
				Technique:    "jailbreak",
				SuccessRate:  0.72,
				AvgTokenCost: 2000,
				LastUsed:     time.Now().Add(-48 * time.Hour),
			},
		},
		FailedApproaches: []ApproachRecord{
			{
				Approach:      "SQL injection",
				FailureReason: "Not applicable to LLM targets",
				TargetType:    "llm",
				LastAttempted: time.Now().Add(-72 * time.Hour),
			},
		},
		SimilarMissions: []MissionSummary{
			{
				ID:            types.NewID(),
				TargetType:    "llm",
				Techniques:    []string{"prompt-injection", "context-overflow"},
				Success:       true,
				FindingsCount: 5,
			},
		},
		RelevantFindings: []FindingSummary{
			{
				ID:        types.NewID(),
				Title:     "Prompt Injection via Role Confusion",
				Category:  "injection",
				Severity:  "high",
				Technique: "prompt-injection",
			},
		},
		QueryLatency: 45 * time.Millisecond,
		ResultCount:  5,
	}

	var capturedMessages []llm.Message

	// Valid LLM response that references memory context
	llmResponse := `{
  "ordered_steps": [
    {"node_id": "recon", "priority": 1, "allocated_tokens": 2000, "rationale": "Start with reconnaissance"},
    {"node_id": "attack", "priority": 2, "allocated_tokens": 6000, "rationale": "Focus on prompt-injection based on 85% success rate"}
  ],
  "skipped_nodes": [],
  "parallel_groups": [],
  "rationale": "Prioritize prompt-injection and jailbreak techniques based on historical success rates",
  "memory_influences": ["prompt-injection: 85% success rate", "jailbreak: 72% success rate", "5 similar missions with 5 findings"]
}`

	mockHarness := &MockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			capturedMessages = messages
			return &llm.CompletionResponse{
				Message: llm.Message{Content: llmResponse},
				Usage:   llm.CompletionTokenUsage{TotalTokens: 800},
			}, nil
		},
		targetInfo: target,
	}

	budgetMgr := NewDefaultBudgetManager()
	budgetMgr.Allocate(10000, 1.0)

	planner := NewLLMStrategicPlanner(mockHarness, "primary", budgetMgr)

	input := StrategicPlanInput{
		Workflow:      wf,
		Bounds:        bounds,
		Target:        target,
		MemoryContext: memoryContext,
		Budget:        budget,
		OriginalGoal:  "Discover prompt injection vulnerabilities in GPT-4",
	}

	plan, err := planner.Plan(context.Background(), input)

	// Verify planning succeeded
	require.NoError(t, err)
	require.NotNil(t, plan)

	// Verify plan includes memory influences
	assert.NotEmpty(t, plan.MemoryInfluences)
	assert.Contains(t, plan.Rationale, "success rate")

	// Verify prompt included memory context
	require.Len(t, capturedMessages, 2)
	userPrompt := capturedMessages[1].Content

	assert.Contains(t, userPrompt, "prompt-injection")
	assert.Contains(t, userPrompt, "85%")
	assert.Contains(t, userPrompt, "jailbreak")
	assert.Contains(t, userPrompt, "72%")
	assert.Contains(t, userPrompt, "SQL injection")
	assert.Contains(t, userPrompt, "Not applicable to LLM")
	assert.Contains(t, userPrompt, "1 past missions") // We created 1 similar mission in the test data
}

// TestStrategicPlanner_BoundsValidation tests comprehensive bounds checking
func TestStrategicPlanner_BoundsValidation(t *testing.T) {
	t.Run("max duration enforcement", func(t *testing.T) {
		wf := &workflow.Workflow{
			Nodes: map[string]*workflow.WorkflowNode{
				"step1": {
					ID:        "step1",
					Type:      workflow.NodeTypeAgent,
					AgentName: "agent1",
					Timeout:   5 * time.Minute,
				},
			},
		}

		bounds, err := ExtractBounds(wf)
		require.NoError(t, err)

		// Bounds should capture max duration
		assert.Equal(t, 5*time.Minute, bounds.MaxDuration)

		budget := &BudgetAllocation{
			StrategicPlanning: 1000,
			Execution:         5000,
		}

		planner := NewDefaultOrderPlanner()
		input := StrategicPlanInput{
			Workflow:     wf,
			Bounds:       bounds,
			Budget:       budget,
			OriginalGoal: "Test",
		}

		plan, err := planner.Plan(context.Background(), input)
		require.NoError(t, err)

		// Plan should respect max duration
		assert.LessOrEqual(t, plan.EstimatedDuration, bounds.MaxDuration)
	})

	t.Run("allowed agents enforcement", func(t *testing.T) {
		wf := &workflow.Workflow{
			Nodes: map[string]*workflow.WorkflowNode{
				"step1": {
					ID:        "step1",
					Type:      workflow.NodeTypeAgent,
					AgentName: "allowed-agent",
				},
			},
		}

		bounds, err := ExtractBounds(wf)
		require.NoError(t, err)

		assert.Contains(t, bounds.AllowedAgents, "allowed-agent")
		assert.NotContains(t, bounds.AllowedAgents, "disallowed-agent")
	})

	t.Run("allowed tools enforcement", func(t *testing.T) {
		wf := &workflow.Workflow{
			Nodes: map[string]*workflow.WorkflowNode{
				"tool1": {
					ID:       "tool1",
					Type:     workflow.NodeTypeTool,
					ToolName: "nmap",
				},
			},
		}

		bounds, err := ExtractBounds(wf)
		require.NoError(t, err)

		assert.Contains(t, bounds.AllowedTools, "nmap")
		assert.NotContains(t, bounds.AllowedTools, "sqlmap")
	})

	t.Run("max parallel enforcement", func(t *testing.T) {
		wf := &workflow.Workflow{
			Nodes: map[string]*workflow.WorkflowNode{
				"parallel": {
					ID:   "parallel",
					Type: workflow.NodeTypeParallel,
					SubNodes: []*workflow.WorkflowNode{
						{ID: "p1", Type: workflow.NodeTypeAgent, AgentName: "a1"},
						{ID: "p2", Type: workflow.NodeTypeAgent, AgentName: "a2"},
						{ID: "p3", Type: workflow.NodeTypeAgent, AgentName: "a3"},
					},
				},
			},
		}

		bounds, err := ExtractBounds(wf)
		require.NoError(t, err)

		// Should detect max parallelism from parallel node
		assert.Equal(t, 3, bounds.MaxParallel)
	})
}

// TestStrategicPlanner_PlanStatusLifecycle tests plan status transitions
func TestStrategicPlanner_PlanStatusLifecycle(t *testing.T) {
	statuses := []PlanStatus{
		PlanStatusDraft,
		PlanStatusApproved,
		PlanStatusExecuting,
		PlanStatusCompleted,
		PlanStatusFailed,
		PlanStatusCancelled,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			assert.Equal(t, string(status), status.String())

			// Test terminal states
			isTerminal := status.IsTerminal()
			switch status {
			case PlanStatusCompleted, PlanStatusFailed, PlanStatusCancelled:
				assert.True(t, isTerminal, "%s should be terminal", status)
			default:
				assert.False(t, isTerminal, "%s should not be terminal", status)
			}
		})
	}
}

// TestStrategicPlanner_PlanMetadata tests plan metadata initialization
func TestStrategicPlanner_PlanMetadata(t *testing.T) {
	wf := &workflow.Workflow{
		Nodes: map[string]*workflow.WorkflowNode{
			"test": {
				ID:        "test",
				Type:      workflow.NodeTypeAgent,
				AgentName: "test-agent",
				Timeout:   30 * time.Second,
			},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	budget := &BudgetAllocation{
		Execution: 1000,
	}

	planner := NewDefaultOrderPlanner()
	input := StrategicPlanInput{
		Workflow:     wf,
		Bounds:       bounds,
		Budget:       budget,
		OriginalGoal: "Test goal",
	}

	startTime := time.Now()
	plan, err := planner.Plan(context.Background(), input)
	require.NoError(t, err)

	// Verify metadata
	assert.NotEmpty(t, plan.ID)
	assert.Equal(t, PlanStatusDraft, plan.Status)
	assert.Equal(t, 0, plan.ReplanCount)
	assert.True(t, plan.CreatedAt.After(startTime) || plan.CreatedAt.Equal(startTime))
	assert.True(t, plan.UpdatedAt.After(startTime) || plan.UpdatedAt.Equal(startTime))
	assert.Equal(t, plan.CreatedAt, plan.UpdatedAt) // Should be equal for new plan
}

// TestStrategicPlanner_EmptyMemoryContext tests planning without memory
func TestStrategicPlanner_EmptyMemoryContext(t *testing.T) {
	wf := &workflow.Workflow{
		Nodes: map[string]*workflow.WorkflowNode{
			"step1": {
				ID:        "step1",
				Type:      workflow.NodeTypeAgent,
				AgentName: "agent1",
				Timeout:   30 * time.Second,
			},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	budget := &BudgetAllocation{
		StrategicPlanning: 1000,
		Execution:         5000,
	}

	llmResponse := `{
  "ordered_steps": [
    {"node_id": "step1", "priority": 1, "allocated_tokens": 2000, "rationale": "Single step"}
  ],
  "skipped_nodes": [],
  "parallel_groups": [],
  "rationale": "No memory context available",
  "memory_influences": []
}`

	mockHarness := &MockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{
				Message: llm.Message{Content: llmResponse},
				Usage:   llm.CompletionTokenUsage{TotalTokens: 300},
			}, nil
		},
		targetInfo: harness.TargetInfo{Type: "test"},
	}

	budgetMgr := NewDefaultBudgetManager()
	budgetMgr.Allocate(6000, 1.0)

	planner := NewLLMStrategicPlanner(mockHarness, "primary", budgetMgr)

	// No memory context provided
	input := StrategicPlanInput{
		Workflow:      wf,
		Bounds:        bounds,
		Target:        mockHarness.targetInfo,
		MemoryContext: nil, // No memory
		Budget:        budget,
		OriginalGoal:  "Test without memory",
	}

	plan, err := planner.Plan(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Empty(t, plan.MemoryInfluences)
}

// TestStrategicPlanner_ContextCancellation tests context cancellation handling
func TestStrategicPlanner_ContextCancellation(t *testing.T) {
	wf := &workflow.Workflow{
		Nodes: map[string]*workflow.WorkflowNode{
			"test": {
				ID:        "test",
				Type:      workflow.NodeTypeAgent,
				AgentName: "test-agent",
			},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	budget := &BudgetAllocation{
		StrategicPlanning: 1000,
		Execution:         5000,
	}

	// Mock harness that checks for context cancellation
	mockHarness := &MockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				return &llm.CompletionResponse{
					Message: llm.Message{Content: "{}"},
					Usage:   llm.CompletionTokenUsage{TotalTokens: 100},
				}, nil
			}
		},
		targetInfo: harness.TargetInfo{Type: "test"},
	}

	budgetMgr := NewDefaultBudgetManager()
	budgetMgr.Allocate(6000, 1.0)

	planner := NewLLMStrategicPlanner(mockHarness, "primary", budgetMgr)

	input := StrategicPlanInput{
		Workflow:     wf,
		Bounds:       bounds,
		Target:       mockHarness.targetInfo,
		Budget:       budget,
		OriginalGoal: "Test",
	}

	// Cancel context before planning
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	plan, err := planner.Plan(ctx, input)

	// Should propagate context cancellation
	assert.Error(t, err)
	assert.Nil(t, plan)
}

// TestPlannedStep_Structure tests PlannedStep field initialization
func TestPlannedStep_Structure(t *testing.T) {
	step := PlannedStep{
		NodeID:          "test-node",
		Priority:        1,
		AllocatedTokens: 500,
		Rationale:       "Test rationale",
	}

	assert.Equal(t, "test-node", step.NodeID)
	assert.Equal(t, 1, step.Priority)
	assert.Equal(t, 500, step.AllocatedTokens)
	assert.Equal(t, "Test rationale", step.Rationale)
}

// TestSkipDecision_Structure tests SkipDecision field initialization
func TestSkipDecision_Structure(t *testing.T) {
	skip := SkipDecision{
		NodeID:    "skipped-node",
		Reason:    "Not applicable to target",
		Condition: "If target type changes",
	}

	assert.Equal(t, "skipped-node", skip.NodeID)
	assert.Equal(t, "Not applicable to target", skip.Reason)
	assert.Equal(t, "If target type changes", skip.Condition)
}

// Helper function to check if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
