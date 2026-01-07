package harness

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"go.opentelemetry.io/otel/trace"
)

// ─── Mock Harness Implementation ─────────────────────────────────────────────

// mockAgentHarness is a mock implementation of AgentHarness for testing.
type mockAgentHarness struct {
	mock.Mock
}

func (m *mockAgentHarness) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...CompletionOption) (*llm.CompletionResponse, error) {
	args := m.Called(ctx, slot, messages, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*llm.CompletionResponse), args.Error(1)
}

func (m *mockAgentHarness) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...CompletionOption) (*llm.CompletionResponse, error) {
	args := m.Called(ctx, slot, messages, tools, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*llm.CompletionResponse), args.Error(1)
}

func (m *mockAgentHarness) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...CompletionOption) (<-chan llm.StreamChunk, error) {
	args := m.Called(ctx, slot, messages, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(<-chan llm.StreamChunk), args.Error(1)
}

func (m *mockAgentHarness) CallTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	args := m.Called(ctx, name, input)
	return args.Get(0).(map[string]any), args.Error(1)
}

func (m *mockAgentHarness) ListTools() []ToolDescriptor {
	args := m.Called()
	return args.Get(0).([]ToolDescriptor)
}

func (m *mockAgentHarness) QueryPlugin(ctx context.Context, name string, method string, params map[string]any) (any, error) {
	args := m.Called(ctx, name, method, params)
	return args.Get(0), args.Error(1)
}

func (m *mockAgentHarness) ListPlugins() []PluginDescriptor {
	args := m.Called()
	return args.Get(0).([]PluginDescriptor)
}

func (m *mockAgentHarness) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
	args := m.Called(ctx, name, task)
	return args.Get(0).(agent.Result), args.Error(1)
}

func (m *mockAgentHarness) ListAgents() []AgentDescriptor {
	args := m.Called()
	return args.Get(0).([]AgentDescriptor)
}

func (m *mockAgentHarness) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	args := m.Called(ctx, finding)
	return args.Error(0)
}

func (m *mockAgentHarness) GetFindings(ctx context.Context, filter FindingFilter) ([]agent.Finding, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]agent.Finding), args.Error(1)
}

func (m *mockAgentHarness) Memory() memory.MemoryStore {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(memory.MemoryStore)
}

func (m *mockAgentHarness) Mission() MissionContext {
	args := m.Called()
	return args.Get(0).(MissionContext)
}

func (m *mockAgentHarness) Target() TargetInfo {
	args := m.Called()
	return args.Get(0).(TargetInfo)
}

func (m *mockAgentHarness) Tracer() trace.Tracer {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(trace.Tracer)
}

func (m *mockAgentHarness) Logger() *slog.Logger {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*slog.Logger)
}

func (m *mockAgentHarness) Metrics() MetricsRecorder {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(MetricsRecorder)
}

func (m *mockAgentHarness) TokenUsage() *llm.TokenTracker {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*llm.TokenTracker)
}

func (m *mockAgentHarness) PlanContext() *PlanningContext {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*PlanningContext)
}

func (m *mockAgentHarness) GetStepBudget() int {
	args := m.Called()
	return args.Int(0)
}

func (m *mockAgentHarness) SignalReplanRecommended(ctx context.Context, reason string) error {
	args := m.Called(ctx, reason)
	return args.Error(0)
}

func (m *mockAgentHarness) ReportStepHints(ctx context.Context, hints *StepHints) error {
	args := m.Called(ctx, hints)
	return args.Error(0)
}

// ─── Test Cases ──────────────────────────────────────────────────────────────

func TestNewPlanningHarnessWrapper(t *testing.T) {
	tests := []struct {
		name             string
		planContext      *PlanningContext
		expectedBudget   int
		expectNilContext bool
	}{
		{
			name: "with_planning_context",
			planContext: &PlanningContext{
				OriginalGoal:           "Test mission",
				CurrentPosition:        2,
				TotalSteps:             5,
				RemainingSteps:         []string{"step3", "step4", "step5"},
				StepBudget:             5000,
				MissionBudgetRemaining: 20000,
			},
			expectedBudget:   5000,
			expectNilContext: false,
		},
		{
			name:             "without_planning_context",
			planContext:      nil,
			expectedBudget:   0,
			expectNilContext: true,
		},
		{
			name: "zero_budget",
			planContext: &PlanningContext{
				OriginalGoal:           "Test mission",
				CurrentPosition:        0,
				TotalSteps:             1,
				RemainingSteps:         []string{},
				StepBudget:             0,
				MissionBudgetRemaining: 0,
			},
			expectedBudget:   0,
			expectNilContext: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := new(mockAgentHarness)
			wrapper := NewPlanningHarnessWrapper(inner, tt.planContext)

			assert.NotNil(t, wrapper)
			assert.Equal(t, inner, wrapper.inner)
			assert.Equal(t, tt.expectedBudget, wrapper.tokensBudget)
			assert.Equal(t, 0, wrapper.tokensUsed)
			assert.False(t, wrapper.replanSignaled)
			assert.Empty(t, wrapper.replanReason)
			assert.NotNil(t, wrapper.hints)

			if tt.expectNilContext {
				assert.Nil(t, wrapper.planContext)
			} else {
				assert.Equal(t, tt.planContext, wrapper.planContext)
			}
		})
	}
}

func TestPlanningHarnessWrapper_PlanContext(t *testing.T) {
	ctx := &PlanningContext{
		OriginalGoal:    "Test goal",
		CurrentPosition: 1,
		TotalSteps:      3,
	}

	inner := &mockAgentHarness{}
	wrapper := NewPlanningHarnessWrapper(inner, ctx)

	retrieved := wrapper.PlanContext()
	assert.Equal(t, ctx, retrieved)
}

func TestPlanningHarnessWrapper_GetStepBudget(t *testing.T) {
	tests := []struct {
		name     string
		budget   int
		expected int
	}{
		{"positive_budget", 5000, 5000},
		{"zero_budget", 0, 0},
		{"large_budget", 100000, 100000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &PlanningContext{StepBudget: tt.budget}
			inner := &mockAgentHarness{}
			wrapper := NewPlanningHarnessWrapper(inner, ctx)

			assert.Equal(t, tt.expected, wrapper.GetStepBudget())
		})
	}
}

func TestPlanningHarnessWrapper_SignalReplanRecommended(t *testing.T) {
	inner := &mockAgentHarness{}
	ctx := &PlanningContext{StepBudget: 5000}
	wrapper := NewPlanningHarnessWrapper(inner, ctx)

	// Initially not signaled
	signaled, reason := wrapper.IsReplanSignaled()
	assert.False(t, signaled)
	assert.Empty(t, reason)

	// Signal replan
	err := wrapper.SignalReplanRecommended(context.Background(), "Found OAuth requirement")
	require.NoError(t, err)

	// Verify signal was recorded
	signaled, reason = wrapper.IsReplanSignaled()
	assert.True(t, signaled)
	assert.Equal(t, "Found OAuth requirement", reason)

	// Signal again with different reason (should update)
	err = wrapper.SignalReplanRecommended(context.Background(), "All vectors blocked")
	require.NoError(t, err)

	signaled, reason = wrapper.IsReplanSignaled()
	assert.True(t, signaled)
	assert.Equal(t, "All vectors blocked", reason)
}

func TestPlanningHarnessWrapper_ReportStepHints(t *testing.T) {
	inner := &mockAgentHarness{}
	ctx := &PlanningContext{StepBudget: 5000}
	wrapper := NewPlanningHarnessWrapper(inner, ctx)

	// Create hints
	hints := NewStepHints().
		WithConfidence(0.95).
		WithSuggestion("exploit_sqli").
		WithKeyFinding("Found SQL injection")

	// Report hints
	err := wrapper.ReportStepHints(context.Background(), hints)
	require.NoError(t, err)

	// Retrieve and verify
	retrieved := wrapper.GetCollectedHints()
	assert.Equal(t, 0.95, retrieved.Confidence)
	assert.Contains(t, retrieved.SuggestedNext, "exploit_sqli")
	assert.Contains(t, retrieved.KeyFindings, "Found SQL injection")
}

func TestPlanningHarnessWrapper_Complete_TracksTokens(t *testing.T) {
	inner := &mockAgentHarness{}
	ctx := &PlanningContext{StepBudget: 5000}
	wrapper := NewPlanningHarnessWrapper(inner, ctx)

	// Setup mock response
	response := &llm.CompletionResponse{
		Usage: llm.CompletionTokenUsage{
			PromptTokens:     100,
			CompletionTokens: 200,
			TotalTokens:      300,
		},
	}
	inner.On("Complete", mock.Anything, "primary", mock.Anything, mock.Anything).Return(response, nil)

	// Call Complete
	messages := []llm.Message{}
	resp, err := wrapper.Complete(context.Background(), "primary", messages)

	require.NoError(t, err)
	assert.Equal(t, response, resp)
	assert.Equal(t, 300, wrapper.GetTokensUsed())

	// Call again to verify accumulation
	inner.On("Complete", mock.Anything, "secondary", mock.Anything, mock.Anything).Return(response, nil)
	_, err = wrapper.Complete(context.Background(), "secondary", messages)
	require.NoError(t, err)
	assert.Equal(t, 600, wrapper.GetTokensUsed())
}

func TestPlanningHarnessWrapper_CompleteWithTools_TracksTokens(t *testing.T) {
	inner := &mockAgentHarness{}
	ctx := &PlanningContext{StepBudget: 10000}
	wrapper := NewPlanningHarnessWrapper(inner, ctx)

	// Setup mock response
	response := &llm.CompletionResponse{
		Usage: llm.CompletionTokenUsage{
			PromptTokens:     500,
			CompletionTokens: 1000,
			TotalTokens:      1500,
		},
	}
	inner.On("CompleteWithTools", mock.Anything, "primary", mock.Anything, mock.Anything, mock.Anything).Return(response, nil)

	// Call CompleteWithTools
	messages := []llm.Message{}
	tools := []llm.ToolDef{}
	resp, err := wrapper.CompleteWithTools(context.Background(), "primary", messages, tools)

	require.NoError(t, err)
	assert.Equal(t, response, resp)
	assert.Equal(t, 1500, wrapper.GetTokensUsed())
}

func TestPlanningHarnessWrapper_Complete_PropagatesError(t *testing.T) {
	inner := &mockAgentHarness{}
	ctx := &PlanningContext{StepBudget: 5000}
	wrapper := NewPlanningHarnessWrapper(inner, ctx)

	// Setup mock to return error
	expectedErr := assert.AnError
	inner.On("Complete", mock.Anything, "primary", mock.Anything, mock.Anything).Return(nil, expectedErr)

	// Call Complete
	messages := []llm.Message{}
	resp, err := wrapper.Complete(context.Background(), "primary", messages)

	assert.Nil(t, resp)
	assert.Equal(t, expectedErr, err)
	assert.Equal(t, 0, wrapper.GetTokensUsed()) // No tokens tracked on error
}

func TestPlanningHarnessWrapper_Delegation(t *testing.T) {
	t.Run("CallTool", func(t *testing.T) {
		inner := &mockAgentHarness{}
		wrapper := NewPlanningHarnessWrapper(inner, nil)

		expected := map[string]any{"result": "success"}
		inner.On("CallTool", mock.Anything, "test_tool", mock.Anything).Return(expected, nil)

		result, err := wrapper.CallTool(context.Background(), "test_tool", map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, expected, result)
		inner.AssertExpectations(t)
	})

	t.Run("ListTools", func(t *testing.T) {
		inner := &mockAgentHarness{}
		wrapper := NewPlanningHarnessWrapper(inner, nil)

		expected := []ToolDescriptor{{Name: "test_tool"}}
		inner.On("ListTools").Return(expected)

		result := wrapper.ListTools()
		assert.Equal(t, expected, result)
		inner.AssertExpectations(t)
	})

	t.Run("QueryPlugin", func(t *testing.T) {
		inner := &mockAgentHarness{}
		wrapper := NewPlanningHarnessWrapper(inner, nil)

		expected := map[string]any{"data": "plugin_result"}
		inner.On("QueryPlugin", mock.Anything, "test_plugin", "query", mock.Anything).Return(expected, nil)

		result, err := wrapper.QueryPlugin(context.Background(), "test_plugin", "query", map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, expected, result)
		inner.AssertExpectations(t)
	})

	t.Run("ListPlugins", func(t *testing.T) {
		inner := &mockAgentHarness{}
		wrapper := NewPlanningHarnessWrapper(inner, nil)

		expected := []PluginDescriptor{{Name: "test_plugin"}}
		inner.On("ListPlugins").Return(expected)

		result := wrapper.ListPlugins()
		assert.Equal(t, expected, result)
		inner.AssertExpectations(t)
	})

	t.Run("DelegateToAgent", func(t *testing.T) {
		inner := &mockAgentHarness{}
		wrapper := NewPlanningHarnessWrapper(inner, nil)

		task := agent.Task{}
		expected := agent.Result{}
		inner.On("DelegateToAgent", mock.Anything, "test_agent", task).Return(expected, nil)

		result, err := wrapper.DelegateToAgent(context.Background(), "test_agent", task)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
		inner.AssertExpectations(t)
	})

	t.Run("ListAgents", func(t *testing.T) {
		inner := &mockAgentHarness{}
		wrapper := NewPlanningHarnessWrapper(inner, nil)

		expected := []AgentDescriptor{{Name: "test_agent"}}
		inner.On("ListAgents").Return(expected)

		result := wrapper.ListAgents()
		assert.Equal(t, expected, result)
		inner.AssertExpectations(t)
	})

	t.Run("SubmitFinding", func(t *testing.T) {
		inner := &mockAgentHarness{}
		wrapper := NewPlanningHarnessWrapper(inner, nil)

		finding := agent.Finding{}
		inner.On("SubmitFinding", mock.Anything, finding).Return(nil)

		err := wrapper.SubmitFinding(context.Background(), finding)
		require.NoError(t, err)
		inner.AssertExpectations(t)
	})

	t.Run("GetFindings", func(t *testing.T) {
		inner := &mockAgentHarness{}
		wrapper := NewPlanningHarnessWrapper(inner, nil)

		filter := FindingFilter{}
		expected := []agent.Finding{{}}
		inner.On("GetFindings", mock.Anything, filter).Return(expected, nil)

		result, err := wrapper.GetFindings(context.Background(), filter)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
		inner.AssertExpectations(t)
	})
}

func TestPlanningHarnessWrapper_ContextAccessors(t *testing.T) {
	inner := &mockAgentHarness{}
	wrapper := NewPlanningHarnessWrapper(inner, nil)

	t.Run("Memory", func(t *testing.T) {
		// Create a nil memory store for testing
		var expectedMemory memory.MemoryStore = nil
		inner.On("Memory").Return(expectedMemory)

		result := wrapper.Memory()
		assert.Equal(t, expectedMemory, result)
		inner.AssertExpectations(t)
	})

	t.Run("Mission", func(t *testing.T) {
		expected := MissionContext{Name: "test_mission"}
		inner.On("Mission").Return(expected)

		result := wrapper.Mission()
		assert.Equal(t, expected, result)
		inner.AssertExpectations(t)
	})

	t.Run("Target", func(t *testing.T) {
		expected := TargetInfo{URL: "https://example.com"}
		inner.On("Target").Return(expected)

		result := wrapper.Target()
		assert.Equal(t, expected, result)
		inner.AssertExpectations(t)
	})

	t.Run("Tracer", func(t *testing.T) {
		var expected trace.Tracer = nil
		inner.On("Tracer").Return(expected)

		result := wrapper.Tracer()
		assert.Equal(t, expected, result)
		inner.AssertExpectations(t)
	})

	t.Run("Logger", func(t *testing.T) {
		expected := slog.Default()
		inner.On("Logger").Return(expected)

		result := wrapper.Logger()
		assert.Equal(t, expected, result)
		inner.AssertExpectations(t)
	})

	t.Run("Metrics", func(t *testing.T) {
		var expected MetricsRecorder = nil
		inner.On("Metrics").Return(expected)

		result := wrapper.Metrics()
		assert.Equal(t, expected, result)
		inner.AssertExpectations(t)
	})

	t.Run("TokenUsage", func(t *testing.T) {
		var expected *llm.TokenTracker = nil
		inner.On("TokenUsage").Return(expected)

		result := wrapper.TokenUsage()
		assert.Equal(t, expected, result)
		inner.AssertExpectations(t)
	})
}

func TestPlanningHarnessWrapper_ConcurrentAccess(t *testing.T) {
	inner := &mockAgentHarness{}
	ctx := &PlanningContext{StepBudget: 100000}
	wrapper := NewPlanningHarnessWrapper(inner, ctx)

	// Setup mock to allow many calls
	response := &llm.CompletionResponse{
		Usage: llm.CompletionTokenUsage{TotalTokens: 100},
	}
	inner.On("Complete", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(response, nil)

	// Run concurrent operations
	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			// Each goroutine performs multiple operations
			_, _ = wrapper.Complete(context.Background(), "primary", []llm.Message{})
			_ = wrapper.SignalReplanRecommended(context.Background(), "test reason")
			_ = wrapper.ReportStepHints(context.Background(), NewStepHints())
			_ = wrapper.GetTokensUsed()
			_, _ = wrapper.IsReplanSignaled()
			_ = wrapper.GetCollectedHints()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify final state is consistent
	tokensUsed := wrapper.GetTokensUsed()
	assert.Equal(t, 1000, tokensUsed) // 10 goroutines * 100 tokens each

	signaled, _ := wrapper.IsReplanSignaled()
	assert.True(t, signaled) // At least one signal was recorded

	hints := wrapper.GetCollectedHints()
	assert.NotNil(t, hints) // Hints were recorded
}

func TestPlanningHarnessWrapper_InterfaceCompliance(t *testing.T) {
	// Verify the wrapper implements both interfaces
	var _ AgentHarness = (*PlanningHarnessWrapper)(nil)
	var _ PlanningContextProvider = (*PlanningHarnessWrapper)(nil)
}

func TestPlanningHarnessWrapper_Stream_NoTokenTracking(t *testing.T) {
	inner := &mockAgentHarness{}
	wrapper := NewPlanningHarnessWrapper(inner, nil)

	// Setup mock
	ch := make(<-chan llm.StreamChunk)
	inner.On("Stream", mock.Anything, "primary", mock.Anything, mock.Anything).Return(ch, nil)

	// Call Stream
	result, err := wrapper.Stream(context.Background(), "primary", []llm.Message{})
	require.NoError(t, err)
	assert.Equal(t, ch, result)

	// Verify no token tracking (since streaming is not tracked)
	assert.Equal(t, 0, wrapper.GetTokensUsed())
	inner.AssertExpectations(t)
}
