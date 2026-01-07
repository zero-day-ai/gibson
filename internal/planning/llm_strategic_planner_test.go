package planning

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/workflow"
	"go.opentelemetry.io/otel/trace"
)

// MockHarness implements harness.AgentHarness for testing LLM planner
type MockHarness struct {
	completeFunc func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error)
	targetInfo   harness.TargetInfo
}

func (m *MockHarness) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	if m.completeFunc != nil {
		return m.completeFunc(ctx, slot, messages, opts...)
	}
	return nil, errors.New("mock Complete not implemented")
}

func (m *MockHarness) Target() harness.TargetInfo {
	return m.targetInfo
}

// Stub implementations for other harness methods (not used in planning)
func (m *MockHarness) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return nil, errors.New("not implemented")
}
func (m *MockHarness) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (<-chan llm.StreamChunk, error) {
	return nil, errors.New("not implemented")
}
func (m *MockHarness) CallTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	return nil, errors.New("not implemented")
}
func (m *MockHarness) ListTools() []harness.ToolDescriptor { return nil }
func (m *MockHarness) QueryPlugin(ctx context.Context, name string, method string, params map[string]any) (any, error) {
	return nil, errors.New("not implemented")
}
func (m *MockHarness) ListPlugins() []harness.PluginDescriptor { return nil }
func (m *MockHarness) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
	return agent.Result{}, errors.New("not implemented")
}
func (m *MockHarness) ListAgents() []harness.AgentDescriptor { return nil }
func (m *MockHarness) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	return errors.New("not implemented")
}
func (m *MockHarness) GetFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return nil, errors.New("not implemented")
}
func (m *MockHarness) Memory() memory.MemoryStore                                       { return nil }
func (m *MockHarness) Mission() harness.MissionContext                                  { return harness.MissionContext{} }
func (m *MockHarness) Tracer() trace.Tracer                                             { return nil }
func (m *MockHarness) Logger() *slog.Logger                                             { return nil }
func (m *MockHarness) Metrics() harness.MetricsRecorder                                 { return nil }
func (m *MockHarness) TokenUsage() *llm.TokenTracker                                    { return nil }
func (m *MockHarness) PlanContext() *harness.PlanningContext                            { return nil }
func (m *MockHarness) GetStepBudget() int                                               { return 0 }
func (m *MockHarness) SignalReplanRecommended(ctx context.Context, reason string) error { return nil }
func (m *MockHarness) ReportStepHints(ctx context.Context, hints *harness.StepHints) error {
	return nil
}

// TestLLMStrategicPlanner_Plan_ValidResponse tests successful plan generation
func TestLLMStrategicPlanner_Plan_ValidResponse(t *testing.T) {
	// Create test workflow
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
				Dependencies: []string{"recon"},
				Timeout:      60 * time.Second,
			},
			"exploit": {
				ID:           "exploit",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "exploit-agent",
				Dependencies: []string{"attack"},
				Timeout:      90 * time.Second,
			},
		},
	}

	// Extract bounds
	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	// Create budget
	budget := &BudgetAllocation{
		StrategicPlanning: 1000,
		Execution:         8000,
		TotalTokens:       10000,
	}

	// Mock LLM response with valid JSON
	llmResponse := `{
  "ordered_steps": [
    {
      "node_id": "recon",
      "priority": 1,
      "allocated_tokens": 2000,
      "rationale": "Start with reconnaissance to gather target information"
    },
    {
      "node_id": "attack",
      "priority": 2,
      "allocated_tokens": 4000,
      "rationale": "Execute attacks based on recon findings"
    },
    {
      "node_id": "exploit",
      "priority": 3,
      "allocated_tokens": 2000,
      "rationale": "Exploit discovered vulnerabilities"
    }
  ],
  "skipped_nodes": [],
  "parallel_groups": [],
  "rationale": "Sequential execution following dependency chain",
  "memory_influences": ["technique: prompt-injection had 85% success rate"]
}`

	mockHarness := &MockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: llmResponse,
				},
				Usage: llm.CompletionTokenUsage{
					PromptTokens:     500,
					CompletionTokens: 300,
					TotalTokens:      800,
				},
			}, nil
		},
		targetInfo: harness.TargetInfo{
			Type: "llm",
			Name: "test-llm",
		},
	}

	budgetMgr := NewDefaultBudgetManager()
	budgetMgr.Allocate(10000, 1.0)

	planner := NewLLMStrategicPlanner(mockHarness, "primary", budgetMgr)

	input := StrategicPlanInput{
		Workflow:     wf,
		Bounds:       bounds,
		Target:       mockHarness.targetInfo,
		Budget:       budget,
		OriginalGoal: "Discover prompt injection vulnerabilities",
	}

	// Execute planning
	plan, err := planner.Plan(context.Background(), input)

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, plan)

	assert.Equal(t, 3, len(plan.OrderedSteps))
	assert.Equal(t, "recon", plan.OrderedSteps[0].NodeID)
	assert.Equal(t, 1, plan.OrderedSteps[0].Priority)
	assert.Equal(t, 2000, plan.OrderedSteps[0].AllocatedTokens)

	assert.Equal(t, "attack", plan.OrderedSteps[1].NodeID)
	assert.Equal(t, 2, plan.OrderedSteps[1].Priority)

	assert.Equal(t, "exploit", plan.OrderedSteps[2].NodeID)
	assert.Equal(t, 3, plan.OrderedSteps[2].Priority)

	assert.Equal(t, 8000, plan.EstimatedTokens)
	assert.Equal(t, PlanStatusDraft, plan.Status)
	assert.Equal(t, 0, plan.ReplanCount)
}

// TestLLMStrategicPlanner_Plan_WithSkippedNodes tests plan with skipped nodes
func TestLLMStrategicPlanner_Plan_WithSkippedNodes(t *testing.T) {
	wf := &workflow.Workflow{
		Nodes: map[string]*workflow.WorkflowNode{
			"scan":   {ID: "scan", Type: workflow.NodeTypeTool, ToolName: "nmap"},
			"attack": {ID: "attack", Type: workflow.NodeTypeAgent, AgentName: "attacker"},
			"linux":  {ID: "linux", Type: workflow.NodeTypeAgent, AgentName: "linux-exploit"},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	budget := &BudgetAllocation{
		StrategicPlanning: 1000,
		Execution:         5000,
	}

	// LLM skips linux-specific exploit for Windows target
	llmResponse := `{
  "ordered_steps": [
    {"node_id": "scan", "priority": 1, "allocated_tokens": 2000, "rationale": "Initial scan"},
    {"node_id": "attack", "priority": 2, "allocated_tokens": 3000, "rationale": "General attacks"}
  ],
  "skipped_nodes": [
    {
      "node_id": "linux",
      "reason": "Target is Windows-based, Linux exploits not applicable",
      "condition": "If OS re-identified as Linux"
    }
  ],
  "parallel_groups": [],
  "rationale": "Skip platform-specific exploits that don't match target",
  "memory_influences": []
}`

	mockHarness := &MockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{
				Message: llm.Message{Content: llmResponse},
				Usage:   llm.CompletionTokenUsage{TotalTokens: 500},
			}, nil
		},
		targetInfo: harness.TargetInfo{Type: "api", Name: "windows-api"},
	}

	budgetMgr := NewDefaultBudgetManager()
	budgetMgr.Allocate(10000, 1.0)

	planner := NewLLMStrategicPlanner(mockHarness, "primary", budgetMgr)
	input := StrategicPlanInput{
		Workflow:     wf,
		Bounds:       bounds,
		Target:       mockHarness.targetInfo,
		Budget:       budget,
		OriginalGoal: "Test API security",
	}

	plan, err := planner.Plan(context.Background(), input)

	require.NoError(t, err)
	assert.Equal(t, 2, len(plan.OrderedSteps))
	assert.Equal(t, 1, len(plan.SkippedNodes))
	assert.Equal(t, "linux", plan.SkippedNodes[0].NodeID)
	assert.Contains(t, plan.SkippedNodes[0].Reason, "Windows")
	assert.Equal(t, "If OS re-identified as Linux", plan.SkippedNodes[0].Condition)
}

// TestLLMStrategicPlanner_Plan_InvalidNodeRejected tests bounds validation
func TestLLMStrategicPlanner_Plan_InvalidNodeRejected(t *testing.T) {
	wf := &workflow.Workflow{
		Nodes: map[string]*workflow.WorkflowNode{
			"valid": {ID: "valid", Type: workflow.NodeTypeAgent, AgentName: "test"},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	// LLM tries to include a node not in the workflow
	llmResponse := `{
  "ordered_steps": [
    {"node_id": "valid", "priority": 1, "allocated_tokens": 2000, "rationale": "Valid step"},
    {"node_id": "invalid_node", "priority": 2, "allocated_tokens": 2000, "rationale": "Invalid step"}
  ],
  "skipped_nodes": [],
  "parallel_groups": [],
  "rationale": "Test plan",
  "memory_influences": []
}`

	mockHarness := &MockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{
				Message: llm.Message{Content: llmResponse},
				Usage:   llm.CompletionTokenUsage{TotalTokens: 400},
			}, nil
		},
		targetInfo: harness.TargetInfo{Type: "test"},
	}

	budgetMgr := NewDefaultBudgetManager()
	budgetMgr.Allocate(10000, 1.0)

	planner := NewLLMStrategicPlanner(mockHarness, "primary", budgetMgr)
	budget := &BudgetAllocation{StrategicPlanning: 1000, Execution: 5000}
	input := StrategicPlanInput{
		Workflow:     wf,
		Bounds:       bounds,
		Budget:       budget,
		OriginalGoal: "Test",
	}

	plan, err := planner.Plan(context.Background(), input)

	require.Error(t, err)
	assert.Nil(t, plan)
	assert.Contains(t, err.Error(), "invalid node")
	assert.Contains(t, err.Error(), "invalid_node")
}

// TestLLMStrategicPlanner_Plan_BudgetExceeded tests budget validation
func TestLLMStrategicPlanner_Plan_BudgetExceeded(t *testing.T) {
	wf := &workflow.Workflow{
		Nodes: map[string]*workflow.WorkflowNode{
			"step1": {ID: "step1", Type: workflow.NodeTypeAgent, AgentName: "test"},
			"step2": {ID: "step2", Type: workflow.NodeTypeAgent, AgentName: "test"},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	// LLM allocates more tokens than budget allows
	llmResponse := `{
  "ordered_steps": [
    {"node_id": "step1", "priority": 1, "allocated_tokens": 3000, "rationale": "Step 1"},
    {"node_id": "step2", "priority": 2, "allocated_tokens": 3000, "rationale": "Step 2"}
  ],
  "skipped_nodes": [],
  "parallel_groups": [],
  "rationale": "Over budget plan",
  "memory_influences": []
}`

	mockHarness := &MockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{
				Message: llm.Message{Content: llmResponse},
				Usage:   llm.CompletionTokenUsage{TotalTokens: 400},
			}, nil
		},
		targetInfo: harness.TargetInfo{Type: "test"},
	}

	budgetMgr := NewDefaultBudgetManager()
	budgetMgr.Allocate(10000, 1.0)

	planner := NewLLMStrategicPlanner(mockHarness, "primary", budgetMgr)
	budget := &BudgetAllocation{StrategicPlanning: 1000, Execution: 5000} // Only 5000 for execution
	input := StrategicPlanInput{
		Workflow:     wf,
		Bounds:       bounds,
		Budget:       budget,
		OriginalGoal: "Test",
	}

	plan, err := planner.Plan(context.Background(), input)

	require.Error(t, err)
	assert.Nil(t, plan)
	assert.Contains(t, err.Error(), "exceeds execution budget")
}

// TestLLMStrategicPlanner_Plan_MarkdownWrappedJSON tests JSON extraction
func TestLLMStrategicPlanner_Plan_MarkdownWrappedJSON(t *testing.T) {
	wf := &workflow.Workflow{
		Nodes: map[string]*workflow.WorkflowNode{
			"test": {ID: "test", Type: workflow.NodeTypeAgent, AgentName: "test"},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	// LLM wraps JSON in markdown code block
	llmResponse := "```json\n" + `{
  "ordered_steps": [
    {"node_id": "test", "priority": 1, "allocated_tokens": 1000, "rationale": "Test"}
  ],
  "skipped_nodes": [],
  "parallel_groups": [],
  "rationale": "Test plan",
  "memory_influences": []
}` + "\n```"

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
	budgetMgr.Allocate(10000, 1.0)

	planner := NewLLMStrategicPlanner(mockHarness, "primary", budgetMgr)
	budget := &BudgetAllocation{StrategicPlanning: 1000, Execution: 2000}
	input := StrategicPlanInput{
		Workflow:     wf,
		Bounds:       bounds,
		Budget:       budget,
		OriginalGoal: "Test",
	}

	plan, err := planner.Plan(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Equal(t, 1, len(plan.OrderedSteps))
}

// TestLLMStrategicPlanner_Plan_LLMError tests LLM failure handling
func TestLLMStrategicPlanner_Plan_LLMError(t *testing.T) {
	wf := &workflow.Workflow{
		Nodes: map[string]*workflow.WorkflowNode{
			"test": {ID: "test", Type: workflow.NodeTypeAgent, AgentName: "test"},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	mockHarness := &MockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return nil, errors.New("LLM API error")
		},
		targetInfo: harness.TargetInfo{Type: "test"},
	}

	budgetMgr := NewDefaultBudgetManager()
	budgetMgr.Allocate(10000, 1.0)

	planner := NewLLMStrategicPlanner(mockHarness, "primary", budgetMgr)
	budget := &BudgetAllocation{StrategicPlanning: 1000, Execution: 2000}
	input := StrategicPlanInput{
		Workflow:     wf,
		Bounds:       bounds,
		Budget:       budget,
		OriginalGoal: "Test",
	}

	plan, err := planner.Plan(context.Background(), input)

	require.Error(t, err)
	assert.Nil(t, plan)
	assert.Contains(t, err.Error(), "LLM completion failed")
}

// TestLLMStrategicPlanner_Plan_InvalidInput tests input validation
func TestLLMStrategicPlanner_Plan_InvalidInput(t *testing.T) {
	budgetMgr := NewDefaultBudgetManager()
	mockHarness := &MockHarness{}
	planner := NewLLMStrategicPlanner(mockHarness, "primary", budgetMgr)

	tests := []struct {
		name   string
		input  StrategicPlanInput
		errMsg string
	}{
		{
			name: "nil workflow",
			input: StrategicPlanInput{
				Workflow:     nil,
				Bounds:       &PlanningBounds{},
				OriginalGoal: "test",
			},
			errMsg: "workflow cannot be nil",
		},
		{
			name: "nil bounds",
			input: StrategicPlanInput{
				Workflow:     &workflow.Workflow{Nodes: map[string]*workflow.WorkflowNode{}},
				Bounds:       nil,
				OriginalGoal: "test",
			},
			errMsg: "bounds cannot be nil",
		},
		{
			name: "empty workflow",
			input: StrategicPlanInput{
				Workflow:     &workflow.Workflow{Nodes: map[string]*workflow.WorkflowNode{}},
				Bounds:       &PlanningBounds{AllowedNodes: map[string]bool{}},
				OriginalGoal: "test",
			},
			errMsg: "workflow must contain at least one node",
		},
		{
			name: "empty goal",
			input: StrategicPlanInput{
				Workflow: &workflow.Workflow{
					Nodes: map[string]*workflow.WorkflowNode{
						"test": {ID: "test"},
					},
				},
				Bounds:       &PlanningBounds{AllowedNodes: map[string]bool{"test": true}},
				OriginalGoal: "",
			},
			errMsg: "original goal cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := planner.Plan(context.Background(), tt.input)
			require.Error(t, err)
			assert.Nil(t, plan)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

// TestExtractJSON tests JSON extraction from various formats
func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain JSON",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "markdown with json hint",
			input:    "```json\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "markdown without hint",
			input:    "```\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "markdown with text",
			input:    "Here's the plan:\n```json\n{\"key\": \"value\"}\n```\nEnd",
			expected: `{"key": "value"}`,
		},
		{
			name:     "plain with whitespace",
			input:    "  \n  {\"key\": \"value\"}  \n  ",
			expected: `{"key": "value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSON(tt.input)
			assert.Equal(t, tt.expected, result)

			// Verify extracted JSON is valid
			var parsed map[string]any
			err := json.Unmarshal([]byte(result), &parsed)
			assert.NoError(t, err)
		})
	}
}

// TestLLMStrategicPlanner_PromptConstruction tests that prompts include all required context
func TestLLMStrategicPlanner_PromptConstruction(t *testing.T) {
	wf := &workflow.Workflow{
		Nodes: map[string]*workflow.WorkflowNode{
			"recon": {
				ID:           "recon",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "recon-agent",
				Timeout:      30 * time.Second,
				Dependencies: []string{},
			},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	// Create memory context
	memContext := &MemoryQueryResult{
		SuccessfulTechniques: []TechniqueRecord{
			{
				Technique:    "prompt-injection",
				SuccessRate:  0.85,
				AvgTokenCost: 1500,
			},
		},
		FailedApproaches: []ApproachRecord{
			{
				Approach:      "SQL injection on LLM",
				FailureReason: "Not applicable to LLM targets",
			},
		},
		ResultCount: 2,
	}

	var capturedMessages []llm.Message

	mockHarness := &MockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			capturedMessages = messages
			// Return valid response
			response := `{
  "ordered_steps": [{"node_id": "recon", "priority": 1, "allocated_tokens": 1000, "rationale": "Test"}],
  "skipped_nodes": [],
  "parallel_groups": [],
  "rationale": "Test",
  "memory_influences": []
}`
			return &llm.CompletionResponse{
				Message: llm.Message{Content: response},
				Usage:   llm.CompletionTokenUsage{TotalTokens: 400},
			}, nil
		},
		targetInfo: harness.TargetInfo{
			Type:     "llm",
			Name:     "gpt-4",
			Provider: "openai",
		},
	}

	budgetMgr := NewDefaultBudgetManager()
	budgetMgr.Allocate(10000, 1.0)

	planner := NewLLMStrategicPlanner(mockHarness, "primary", budgetMgr)
	budget := &BudgetAllocation{StrategicPlanning: 1000, Execution: 5000}

	originalGoal := "Discover prompt injection vulnerabilities in GPT-4"
	input := StrategicPlanInput{
		Workflow:      wf,
		Bounds:        bounds,
		Target:        mockHarness.targetInfo,
		MemoryContext: memContext,
		Budget:        budget,
		OriginalGoal:  originalGoal,
	}

	_, err = planner.Plan(context.Background(), input)
	require.NoError(t, err)

	// Verify prompt includes all required context
	require.Equal(t, 2, len(capturedMessages))

	systemPrompt := capturedMessages[0].Content
	assert.Contains(t, systemPrompt, "strategic security testing planner")
	assert.Contains(t, systemPrompt, "Only include workflow nodes")

	userPrompt := capturedMessages[1].Content
	assert.Contains(t, userPrompt, originalGoal)           // Original goal verbatim
	assert.Contains(t, userPrompt, "llm")                  // Target type
	assert.Contains(t, userPrompt, "gpt-4")                // Target name
	assert.Contains(t, userPrompt, "openai")               // Provider
	assert.Contains(t, userPrompt, "recon")                // Workflow node
	assert.Contains(t, userPrompt, "recon-agent")          // Agent name
	assert.Contains(t, userPrompt, "prompt-injection")     // Successful technique
	assert.Contains(t, userPrompt, "85%")                  // Success rate
	assert.Contains(t, userPrompt, "SQL injection on LLM") // Failed approach
	assert.Contains(t, userPrompt, "5000")                 // Execution budget
	assert.Contains(t, userPrompt, "ordered_steps")        // Response schema
	assert.Contains(t, userPrompt, "skipped_nodes")        // Response schema
}
