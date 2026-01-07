package verbose

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// TestVerboseHarnessWrapper_Integration tests the complete verbose harness wrapper flow.
func TestVerboseHarnessWrapper_Integration(t *testing.T) {
	// Create event bus
	bus := NewDefaultVerboseEventBus()
	defer bus.Close()

	// Subscribe to events
	eventCh, cleanup := bus.Subscribe(context.Background())
	defer cleanup()

	// Create mock harness with realistic responses
	mock := &mockHarnessMinimal{
		missionCtx: harness.MissionContext{
			ID:           types.NewID(),
			Name:         "integration-test",
			CurrentAgent: "test-agent",
		},
	}

	// Wrap with verbose harness
	wrapper := NewVerboseHarnessWrapper(mock, bus, LevelVerbose)

	// Test Complete
	ctx := context.Background()
	messages := []llm.Message{{Role: llm.RoleUser, Content: "test"}}
	_, err := wrapper.Complete(ctx, "primary", messages)
	require.NoError(t, err)

	// Collect events
	var events []VerboseEvent
	timeout := time.After(200 * time.Millisecond)
	collecting := true
	for collecting {
		select {
		case event := <-eventCh:
			events = append(events, event)
			if len(events) >= 2 {
				collecting = false
			}
		case <-timeout:
			collecting = false
		}
	}

	// Verify we got started and completed events
	require.GreaterOrEqual(t, len(events), 2, "Should have at least 2 events")
	assert.Equal(t, EventLLMRequestStarted, events[0].Type)
	assert.Equal(t, EventLLMRequestCompleted, events[1].Type)
}

// TestWrapHarnessFactory_Integration tests the factory wrapper.
func TestWrapHarnessFactory_Integration(t *testing.T) {
	bus := NewDefaultVerboseEventBus()
	defer bus.Close()

	// Create mock factory
	innerFactory := &mockFactoryMinimal{}

	// Wrap factory
	wrappedFactory := WrapHarnessFactory(innerFactory, bus, LevelVerbose)

	// Create harness
	missionCtx := harness.MissionContext{
		ID:           types.NewID(),
		Name:         "factory-test",
		CurrentAgent: "test-agent",
	}
	h, err := wrappedFactory.Create("test-agent", missionCtx, harness.TargetInfo{})
	require.NoError(t, err)

	// Verify it's wrapped
	_, ok := h.(*VerboseHarnessWrapper)
	assert.True(t, ok, "Expected VerboseHarnessWrapper")
}

// Minimal mock harness for integration tests
type mockHarnessMinimal struct {
	missionCtx harness.MissionContext
}

func (m *mockHarnessMinimal) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{
		Message: llm.Message{Role: llm.RoleAssistant, Content: "test response"},
		Model:   "test-model",
		Usage:   llm.CompletionTokenUsage{PromptTokens: 10, CompletionTokens: 20},
	}, nil
}
func (m *mockHarnessMinimal) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return m.Complete(ctx, slot, messages, opts...)
}
func (m *mockHarnessMinimal) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}
func (m *mockHarnessMinimal) CallTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	return map[string]any{}, nil
}
func (m *mockHarnessMinimal) QueryPlugin(ctx context.Context, name string, method string, params map[string]any) (any, error) {
	return nil, nil
}
func (m *mockHarnessMinimal) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
	return agent.Result{}, nil
}
func (m *mockHarnessMinimal) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	return nil
}
func (m *mockHarnessMinimal) GetFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return nil, nil
}
func (m *mockHarnessMinimal) Memory() memory.MemoryStore              { return nil }
func (m *mockHarnessMinimal) Mission() harness.MissionContext         { return m.missionCtx }
func (m *mockHarnessMinimal) Target() harness.TargetInfo              { return harness.TargetInfo{} }
func (m *mockHarnessMinimal) ListTools() []harness.ToolDescriptor     { return nil }
func (m *mockHarnessMinimal) ListPlugins() []harness.PluginDescriptor { return nil }
func (m *mockHarnessMinimal) ListAgents() []harness.AgentDescriptor   { return nil }
func (m *mockHarnessMinimal) Tracer() trace.Tracer                    { return noop.NewTracerProvider().Tracer("test") }
func (m *mockHarnessMinimal) Logger() *slog.Logger                    { return slog.Default() }
func (m *mockHarnessMinimal) Metrics() harness.MetricsRecorder        { return nil }
func (m *mockHarnessMinimal) TokenUsage() *llm.TokenTracker           { return nil }

// Minimal mock factory for integration tests
type mockFactoryMinimal struct{}

func (f *mockFactoryMinimal) Create(agentName string, missionCtx harness.MissionContext, target harness.TargetInfo) (harness.AgentHarness, error) {
	return &mockHarnessMinimal{missionCtx: missionCtx}, nil
}
func (f *mockFactoryMinimal) CreateChild(parent harness.AgentHarness, agentName string) (harness.AgentHarness, error) {
	return &mockHarnessMinimal{missionCtx: parent.Mission()}, nil
}
