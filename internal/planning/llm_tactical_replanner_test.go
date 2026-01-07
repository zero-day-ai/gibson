package planning

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// mockHarnessForReplanning implements harness.AgentHarness for testing replanning
type mockHarnessForReplanning struct {
	completeFunc func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error)
}

func (m *mockHarnessForReplanning) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	if m.completeFunc != nil {
		return m.completeFunc(ctx, slot, messages, opts...)
	}
	return nil, nil
}

func (m *mockHarnessForReplanning) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return nil, nil
}

func (m *mockHarnessForReplanning) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (<-chan llm.StreamChunk, error) {
	return nil, nil
}

func (m *mockHarnessForReplanning) CallTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	return nil, nil
}

func (m *mockHarnessForReplanning) ListTools() []harness.ToolDescriptor {
	return nil
}

func (m *mockHarnessForReplanning) QueryPlugin(ctx context.Context, name string, method string, params map[string]any) (any, error) {
	return nil, nil
}

func (m *mockHarnessForReplanning) ListPlugins() []harness.PluginDescriptor {
	return nil
}

func (m *mockHarnessForReplanning) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
	return agent.Result{}, nil
}

func (m *mockHarnessForReplanning) ListAgents() []harness.AgentDescriptor {
	return nil
}

func (m *mockHarnessForReplanning) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	return nil
}

func (m *mockHarnessForReplanning) GetFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return nil, nil
}

func (m *mockHarnessForReplanning) Memory() memory.MemoryStore {
	return nil
}

func (m *mockHarnessForReplanning) Mission() harness.MissionContext {
	return harness.MissionContext{}
}

func (m *mockHarnessForReplanning) Target() harness.TargetInfo {
	return harness.TargetInfo{}
}

func (m *mockHarnessForReplanning) Tracer() trace.Tracer {
	return noop.NewTracerProvider().Tracer("test")
}

func (m *mockHarnessForReplanning) Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}

func (m *mockHarnessForReplanning) Metrics() harness.MetricsRecorder {
	return nil
}

func (m *mockHarnessForReplanning) TokenUsage() *llm.TokenTracker {
	return nil
}

func (m *mockHarnessForReplanning) PlanContext() *harness.PlanningContext {
	return nil
}

func (m *mockHarnessForReplanning) GetStepBudget() int {
	return 0
}

func (m *mockHarnessForReplanning) SignalReplanRecommended(ctx context.Context, reason string) error {
	return nil
}

func (m *mockHarnessForReplanning) ReportStepHints(ctx context.Context, hints *harness.StepHints) error {
	return nil
}

func TestNewLLMTacticalReplanner(t *testing.T) {
	harness := &mockHarnessForReplanning{}

	tests := []struct {
		name               string
		opts               []LLMTacticalReplannerOption
		expectedMaxReplans int
		expectedSlot       string
	}{
		{
			name:               "default configuration",
			opts:               nil,
			expectedMaxReplans: 3,
			expectedSlot:       "primary",
		},
		{
			name: "with custom max replans",
			opts: []LLMTacticalReplannerOption{
				WithMaxReplans(5),
			},
			expectedMaxReplans: 5,
			expectedSlot:       "primary",
		},
		{
			name: "with custom slot",
			opts: []LLMTacticalReplannerOption{
				WithReplanSlot("reasoning"),
			},
			expectedMaxReplans: 3,
			expectedSlot:       "reasoning",
		},
		{
			name: "with all options",
			opts: []LLMTacticalReplannerOption{
				WithMaxReplans(10),
				WithReplanSlot("fast"),
				WithReplanBudget(NewDefaultBudgetManager()),
				WithReplanEvents(NewDefaultPlanningEventEmitter()),
			},
			expectedMaxReplans: 10,
			expectedSlot:       "fast",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			replanner := NewLLMTacticalReplanner(harness, tt.opts...)

			if replanner.maxReplans != tt.expectedMaxReplans {
				t.Errorf("maxReplans = %d, want %d", replanner.maxReplans, tt.expectedMaxReplans)
			}

			if replanner.llmSlot != tt.expectedSlot {
				t.Errorf("llmSlot = %q, want %q", replanner.llmSlot, tt.expectedSlot)
			}

			if replanner.harness == nil {
				t.Error("harness should not be nil")
			}
		})
	}
}

func TestLLMTacticalReplanner_Replan_InputValidation(t *testing.T) {
	// Mock LLM response for valid case
	llmResponse := map[string]any{
		"rationale": "Continue with current plan",
		"action":    "continue",
	}
	llmResponseJSON, _ := json.Marshal(llmResponse)

	harness := &mockHarnessForReplanning{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: string(llmResponseJSON),
				},
				Usage: llm.CompletionTokenUsage{
					TotalTokens: 100,
				},
			}, nil
		},
	}
	replanner := NewLLMTacticalReplanner(harness)

	missionID := types.NewID()
	currentPlan := &StrategicPlan{ID: types.NewID(), MissionID: missionID}
	triggerScore := &StepScore{Success: false, ReplanReason: "test"}
	failureMemory := NewFailureMemory()
	bounds := &PlanningBounds{AllowedNodes: map[string]bool{"node1": true}}

	tests := []struct {
		name          string
		input         ReplanInput
		expectError   bool
		errorContains string
	}{
		{
			name: "valid input",
			input: ReplanInput{
				CurrentPlan:   currentPlan,
				TriggerScore:  triggerScore,
				FailedNodeID:  "node1",
				FailureMemory: failureMemory,
				OriginalGoal:  "Test goal",
				Bounds:        bounds,
				MissionID:     missionID,
			},
			expectError: false,
		},
		{
			name: "nil current plan",
			input: ReplanInput{
				CurrentPlan:   nil,
				TriggerScore:  triggerScore,
				FailedNodeID:  "node1",
				FailureMemory: failureMemory,
				OriginalGoal:  "Test goal",
				Bounds:        bounds,
			},
			expectError:   true,
			errorContains: "current plan cannot be nil",
		},
		{
			name: "nil trigger score",
			input: ReplanInput{
				CurrentPlan:   currentPlan,
				TriggerScore:  nil,
				FailedNodeID:  "node1",
				FailureMemory: failureMemory,
				OriginalGoal:  "Test goal",
				Bounds:        bounds,
			},
			expectError:   true,
			errorContains: "trigger score cannot be nil",
		},
		{
			name: "empty failed node ID",
			input: ReplanInput{
				CurrentPlan:   currentPlan,
				TriggerScore:  triggerScore,
				FailedNodeID:  "",
				FailureMemory: failureMemory,
				OriginalGoal:  "Test goal",
				Bounds:        bounds,
			},
			expectError:   true,
			errorContains: "failed node ID cannot be empty",
		},
		{
			name: "nil failure memory",
			input: ReplanInput{
				CurrentPlan:   currentPlan,
				TriggerScore:  triggerScore,
				FailedNodeID:  "node1",
				FailureMemory: nil,
				OriginalGoal:  "Test goal",
				Bounds:        bounds,
			},
			expectError:   true,
			errorContains: "failure memory cannot be nil",
		},
		{
			name: "nil bounds",
			input: ReplanInput{
				CurrentPlan:   currentPlan,
				TriggerScore:  triggerScore,
				FailedNodeID:  "node1",
				FailureMemory: failureMemory,
				OriginalGoal:  "Test goal",
				Bounds:        nil,
			},
			expectError:   true,
			errorContains: "bounds cannot be nil",
		},
		{
			name: "empty original goal",
			input: ReplanInput{
				CurrentPlan:   currentPlan,
				TriggerScore:  triggerScore,
				FailedNodeID:  "node1",
				FailureMemory: failureMemory,
				OriginalGoal:  "",
				Bounds:        bounds,
			},
			expectError:   true,
			errorContains: "original goal cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := replanner.Replan(context.Background(), tt.input)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errorContains != "" && !containsSubstring(err.Error(), tt.errorContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errorContains)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestLLMTacticalReplanner_Replan_MaxReplanCount(t *testing.T) {
	harness := &mockHarnessForReplanning{}
	replanner := NewLLMTacticalReplanner(harness, WithMaxReplans(3))

	missionID := types.NewID()
	input := ReplanInput{
		CurrentPlan: &StrategicPlan{
			ID:        types.NewID(),
			MissionID: missionID,
		},
		TriggerScore: &StepScore{
			Success:      false,
			ReplanReason: "test failure",
		},
		FailedNodeID:  "node1",
		FailureMemory: NewFailureMemory(),
		OriginalGoal:  "Test goal",
		Bounds:        &PlanningBounds{AllowedNodes: map[string]bool{"node1": true}},
		ReplanCount:   3, // Already at max
		MissionID:     missionID,
	}

	result, err := replanner.Replan(context.Background(), input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Action != ReplanActionFail {
		t.Errorf("expected action=fail when max replans exceeded, got %s", result.Action)
	}

	if !containsSubstring(result.Rationale, "Maximum replan count") {
		t.Errorf("rationale should mention maximum replan count, got: %s", result.Rationale)
	}
}

func TestLLMTacticalReplanner_Replan_InsufficientBudget(t *testing.T) {
	harness := &mockHarnessForReplanning{}
	replanner := NewLLMTacticalReplanner(harness)

	missionID := types.NewID()
	input := ReplanInput{
		CurrentPlan: &StrategicPlan{
			ID:        types.NewID(),
			MissionID: missionID,
		},
		TriggerScore: &StepScore{
			Success:      false,
			ReplanReason: "test failure",
		},
		FailedNodeID:  "node1",
		FailureMemory: NewFailureMemory(),
		OriginalGoal:  "Test goal",
		Bounds:        &PlanningBounds{AllowedNodes: map[string]bool{"node1": true}},
		RemainingBudget: &BudgetAllocation{
			ReplanReserve: 0, // No budget left
		},
		ReplanCount: 1,
		MissionID:   missionID,
	}

	result, err := replanner.Replan(context.Background(), input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Action != ReplanActionContinue {
		t.Errorf("expected action=continue when budget exhausted, got %s", result.Action)
	}

	if !containsSubstring(result.Rationale, "budget") {
		t.Errorf("rationale should mention budget, got: %s", result.Rationale)
	}
}

func TestLLMTacticalReplanner_Replan_LLMContinueAction(t *testing.T) {
	missionID := types.NewID()

	// Mock LLM response for "continue" action
	llmResponse := map[string]any{
		"rationale": "Minor issue, continue with current plan",
		"action":    "continue",
	}
	llmResponseJSON, _ := json.Marshal(llmResponse)

	harness := &mockHarnessForReplanning{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: string(llmResponseJSON),
				},
				Usage: llm.CompletionTokenUsage{
					TotalTokens: 100,
				},
			}, nil
		},
	}

	replanner := NewLLMTacticalReplanner(harness)

	input := ReplanInput{
		CurrentPlan: &StrategicPlan{
			ID:        types.NewID(),
			MissionID: missionID,
			OrderedSteps: []PlannedStep{
				{NodeID: "node1", Priority: 0, AllocatedTokens: 100},
				{NodeID: "node2", Priority: 1, AllocatedTokens: 100},
			},
		},
		TriggerScore: &StepScore{
			Success:      false,
			ReplanReason: "minor issue",
		},
		FailedNodeID:  "node1",
		FailureMemory: NewFailureMemory(),
		OriginalGoal:  "Test goal",
		Bounds: &PlanningBounds{
			AllowedNodes: map[string]bool{"node1": true, "node2": true},
		},
		RemainingBudget: &BudgetAllocation{
			ReplanReserve: 1000,
			Execution:     5000,
		},
		ReplanCount: 0,
		MissionID:   missionID,
	}

	result, err := replanner.Replan(context.Background(), input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Action != ReplanActionContinue {
		t.Errorf("expected action=continue, got %s", result.Action)
	}

	if result.ModifiedPlan != nil {
		t.Error("modified plan should be nil for continue action")
	}

	if !containsSubstring(result.Rationale, "continue") {
		t.Errorf("rationale should mention continue, got: %s", result.Rationale)
	}
}

func TestLLMTacticalReplanner_Replan_LLMReorderAction(t *testing.T) {
	missionID := types.NewID()

	// Mock LLM response for "reorder" action
	llmResponse := map[string]any{
		"rationale": "Reordering to prioritize different approach",
		"action":    "reorder",
		"modified_plan": map[string]any{
			"ordered_steps": []map[string]any{
				{
					"node_id":          "node2",
					"priority":         0,
					"allocated_tokens": 150,
					"rationale":        "Try this first",
				},
				{
					"node_id":          "node1",
					"priority":         1,
					"allocated_tokens": 50,
					"rationale":        "Lower priority now",
				},
			},
		},
	}
	llmResponseJSON, _ := json.Marshal(llmResponse)

	harness := &mockHarnessForReplanning{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: string(llmResponseJSON),
				},
				Usage: llm.CompletionTokenUsage{
					TotalTokens: 200,
				},
			}, nil
		},
	}

	replanner := NewLLMTacticalReplanner(harness)

	input := ReplanInput{
		CurrentPlan: &StrategicPlan{
			ID:        types.NewID(),
			MissionID: missionID,
			OrderedSteps: []PlannedStep{
				{NodeID: "node1", Priority: 0, AllocatedTokens: 100},
				{NodeID: "node2", Priority: 1, AllocatedTokens: 100},
			},
		},
		TriggerScore: &StepScore{
			Success:      false,
			ReplanReason: "need different approach",
		},
		FailedNodeID:  "node1",
		FailureMemory: NewFailureMemory(),
		OriginalGoal:  "Test goal",
		Bounds: &PlanningBounds{
			AllowedNodes: map[string]bool{"node1": true, "node2": true},
		},
		RemainingBudget: &BudgetAllocation{
			ReplanReserve: 1000,
			Execution:     5000,
		},
		ReplanCount: 0,
		MissionID:   missionID,
	}

	result, err := replanner.Replan(context.Background(), input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Action != ReplanActionReorder {
		t.Errorf("expected action=reorder, got %s", result.Action)
	}

	if result.ModifiedPlan == nil {
		t.Fatal("modified plan should not be nil for reorder action")
	}

	if len(result.ModifiedPlan.OrderedSteps) != 2 {
		t.Errorf("expected 2 ordered steps, got %d", len(result.ModifiedPlan.OrderedSteps))
	}

	// Verify reordering - node2 should be first now
	if result.ModifiedPlan.OrderedSteps[0].NodeID != "node2" {
		t.Errorf("expected first step to be node2, got %s", result.ModifiedPlan.OrderedSteps[0].NodeID)
	}

	if result.ModifiedPlan.ReplanCount != 1 {
		t.Errorf("expected replan count=1, got %d", result.ModifiedPlan.ReplanCount)
	}
}

func TestLLMTacticalReplanner_Replan_LLMRetryAction(t *testing.T) {
	missionID := types.NewID()

	// Mock LLM response for "retry" action
	llmResponse := map[string]any{
		"rationale":    "Previous approach failed, trying alternative method",
		"action":       "retry",
		"new_approach": "Use different parameters and timeout",
	}
	llmResponseJSON, _ := json.Marshal(llmResponse)

	harness := &mockHarnessForReplanning{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: string(llmResponseJSON),
				},
				Usage: llm.CompletionTokenUsage{
					TotalTokens: 150,
				},
			}, nil
		},
	}

	replanner := NewLLMTacticalReplanner(harness)

	failureMemory := NewFailureMemory()
	// Record a previous attempt with a different approach
	failureMemory.RecordAttempt("node1", AttemptRecord{
		Timestamp: time.Now(),
		Approach:  "Standard parameters",
		Result:    "Failed",
		Success:   false,
	})

	input := ReplanInput{
		CurrentPlan: &StrategicPlan{
			ID:        types.NewID(),
			MissionID: missionID,
			OrderedSteps: []PlannedStep{
				{NodeID: "node1", Priority: 0, AllocatedTokens: 100},
			},
		},
		TriggerScore: &StepScore{
			Success:      false,
			ReplanReason: "execution failed",
		},
		FailedNodeID:  "node1",
		FailureMemory: failureMemory,
		OriginalGoal:  "Test goal",
		Bounds: &PlanningBounds{
			AllowedNodes: map[string]bool{"node1": true},
		},
		RemainingBudget: &BudgetAllocation{
			ReplanReserve: 1000,
			Execution:     5000,
		},
		ReplanCount: 0,
		MissionID:   missionID,
	}

	result, err := replanner.Replan(context.Background(), input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Action != ReplanActionRetry {
		t.Errorf("expected action=retry, got %s", result.Action)
	}

	if result.NewAttempt == nil {
		t.Fatal("new attempt should not be nil for retry action")
	}

	if result.NewAttempt.Approach != "Use different parameters and timeout" {
		t.Errorf("unexpected approach: %s", result.NewAttempt.Approach)
	}
}

func TestLLMTacticalReplanner_Replan_RetryLoopPrevention(t *testing.T) {
	missionID := types.NewID()

	// Mock LLM response trying to retry with same approach
	llmResponse := map[string]any{
		"rationale":    "Trying same thing again",
		"action":       "retry",
		"new_approach": "Standard parameters", // Same as previous attempt
	}
	llmResponseJSON, _ := json.Marshal(llmResponse)

	harness := &mockHarnessForReplanning{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: string(llmResponseJSON),
				},
				Usage: llm.CompletionTokenUsage{
					TotalTokens: 150,
				},
			}, nil
		},
	}

	replanner := NewLLMTacticalReplanner(harness)

	failureMemory := NewFailureMemory()
	// Record previous attempt with same approach
	failureMemory.RecordAttempt("node1", AttemptRecord{
		Timestamp: time.Now(),
		Approach:  "Standard parameters",
		Result:    "Failed",
		Success:   false,
	})

	input := ReplanInput{
		CurrentPlan: &StrategicPlan{
			ID:        types.NewID(),
			MissionID: missionID,
			OrderedSteps: []PlannedStep{
				{NodeID: "node1", Priority: 0, AllocatedTokens: 100},
			},
		},
		TriggerScore: &StepScore{
			Success:      false,
			ReplanReason: "execution failed",
		},
		FailedNodeID:  "node1",
		FailureMemory: failureMemory,
		OriginalGoal:  "Test goal",
		Bounds: &PlanningBounds{
			AllowedNodes: map[string]bool{"node1": true},
		},
		RemainingBudget: &BudgetAllocation{
			ReplanReserve: 1000,
			Execution:     5000,
		},
		ReplanCount: 0,
		MissionID:   missionID,
	}

	_, err := replanner.Replan(context.Background(), input)

	// Should return error because retry approach has already been tried
	if err == nil {
		t.Error("expected error for duplicate retry approach, got nil")
	}

	if !containsSubstring(err.Error(), "already been tried") {
		t.Errorf("error should mention retry loop, got: %v", err)
	}
}

func TestLLMTacticalReplanner_Replan_InvalidNodeInReorder(t *testing.T) {
	missionID := types.NewID()

	// Mock LLM response with invalid node ID
	llmResponse := map[string]any{
		"rationale": "Reordering",
		"action":    "reorder",
		"modified_plan": map[string]any{
			"ordered_steps": []map[string]any{
				{
					"node_id":          "invalid_node", // Not in bounds
					"priority":         0,
					"allocated_tokens": 100,
					"rationale":        "Test",
				},
			},
		},
	}
	llmResponseJSON, _ := json.Marshal(llmResponse)

	harness := &mockHarnessForReplanning{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: string(llmResponseJSON),
				},
				Usage: llm.CompletionTokenUsage{
					TotalTokens: 150,
				},
			}, nil
		},
	}

	replanner := NewLLMTacticalReplanner(harness)

	input := ReplanInput{
		CurrentPlan: &StrategicPlan{
			ID:        types.NewID(),
			MissionID: missionID,
			OrderedSteps: []PlannedStep{
				{NodeID: "node1", Priority: 0, AllocatedTokens: 100},
			},
		},
		TriggerScore: &StepScore{
			Success:      false,
			ReplanReason: "test",
		},
		FailedNodeID:  "node1",
		FailureMemory: NewFailureMemory(),
		OriginalGoal:  "Test goal",
		Bounds: &PlanningBounds{
			AllowedNodes: map[string]bool{"node1": true}, // invalid_node not in bounds
		},
		RemainingBudget: &BudgetAllocation{
			ReplanReserve: 1000,
			Execution:     5000,
		},
		ReplanCount: 0,
		MissionID:   missionID,
	}

	_, err := replanner.Replan(context.Background(), input)

	// Should return error because node is not in workflow bounds
	if err == nil {
		t.Error("expected error for invalid node, got nil")
	}

	if !containsSubstring(err.Error(), "invalid node") {
		t.Errorf("error should mention invalid node, got: %v", err)
	}
}

func TestLLMTacticalReplanner_Replan_LLMFailure(t *testing.T) {
	missionID := types.NewID()

	// Mock LLM failure
	harness := &mockHarnessForReplanning{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return nil, llm.NewProviderError("API error", nil)
		},
	}

	replanner := NewLLMTacticalReplanner(harness)

	input := ReplanInput{
		CurrentPlan: &StrategicPlan{
			ID:        types.NewID(),
			MissionID: missionID,
			OrderedSteps: []PlannedStep{
				{NodeID: "node1", Priority: 0, AllocatedTokens: 100},
			},
		},
		TriggerScore: &StepScore{
			Success:      false,
			ReplanReason: "test",
		},
		FailedNodeID:  "node1",
		FailureMemory: NewFailureMemory(),
		OriginalGoal:  "Test goal",
		Bounds: &PlanningBounds{
			AllowedNodes: map[string]bool{"node1": true},
		},
		RemainingBudget: &BudgetAllocation{
			ReplanReserve: 1000,
			Execution:     5000,
		},
		ReplanCount: 0,
		MissionID:   missionID,
	}

	result, err := replanner.Replan(context.Background(), input)

	// Should fall back to "continue" on LLM failure (optimistic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Action != ReplanActionContinue {
		t.Errorf("expected fallback to continue on LLM failure, got %s", result.Action)
	}

	if !containsSubstring(result.Rationale, "failed") {
		t.Errorf("rationale should mention failure, got: %s", result.Rationale)
	}
}

// Helper function to check if string contains substring
func containsSubstring(s, substr string) bool {
	return strings.Contains(s, substr)
}
