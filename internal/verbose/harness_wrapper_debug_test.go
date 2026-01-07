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

// mockDebugHarness is a mock harness for debug-level testing.
type mockDebugHarness struct {
	missionCtx   harness.MissionContext
	completeResp *llm.CompletionResponse
	completeErr  error
	toolsResp    *llm.CompletionResponse
	toolsErr     error
}

func (m *mockDebugHarness) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return m.completeResp, m.completeErr
}

func (m *mockDebugHarness) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return m.toolsResp, m.toolsErr
}

func (m *mockDebugHarness) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}

func (m *mockDebugHarness) CallTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *mockDebugHarness) QueryPlugin(ctx context.Context, name string, method string, params map[string]any) (any, error) {
	return nil, nil
}

func (m *mockDebugHarness) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
	return agent.Result{}, nil
}

func (m *mockDebugHarness) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	return nil
}

func (m *mockDebugHarness) GetFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return nil, nil
}

func (m *mockDebugHarness) Memory() memory.MemoryStore              { return nil }
func (m *mockDebugHarness) Mission() harness.MissionContext         { return m.missionCtx }
func (m *mockDebugHarness) Target() harness.TargetInfo              { return harness.TargetInfo{} }
func (m *mockDebugHarness) ListTools() []harness.ToolDescriptor     { return nil }
func (m *mockDebugHarness) ListPlugins() []harness.PluginDescriptor { return nil }
func (m *mockDebugHarness) ListAgents() []harness.AgentDescriptor   { return nil }
func (m *mockDebugHarness) Tracer() trace.Tracer                    { return noop.NewTracerProvider().Tracer("test") }
func (m *mockDebugHarness) Logger() *slog.Logger                    { return slog.Default() }
func (m *mockDebugHarness) Metrics() harness.MetricsRecorder        { return nil }
func (m *mockDebugHarness) TokenUsage() *llm.TokenTracker           { return nil }

// TestVerboseHarnessWrapper_DebugLevel_PromptPreview tests that prompt content is included at Debug level.
func TestVerboseHarnessWrapper_DebugLevel_PromptPreview(t *testing.T) {
	// Setup
	bus := NewDefaultVerboseEventBus()
	defer bus.Close()

	mockHarness := &mockDebugHarness{
		missionCtx: harness.MissionContext{
			ID:           types.NewID(),
			Name:         "debug-test",
			CurrentAgent: "test-agent",
		},
		completeResp: &llm.CompletionResponse{
			Model: "test-model",
			Message: llm.Message{
				Role:    llm.RoleAssistant,
				Content: "This is the response content from the LLM",
			},
			Usage: llm.CompletionTokenUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
			},
			FinishReason: llm.FinishReasonStop,
		},
	}

	wrapper := NewVerboseHarnessWrapper(mockHarness, bus, LevelDebug)

	// Subscribe to events
	ctx := context.Background()
	eventChan, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	// Execute
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "This is a very long user message that should be truncated when included in the debug event"},
		{Role: llm.RoleAssistant, Content: "Previous assistant response"},
	}

	resp, err := wrapper.Complete(ctx, "primary", messages)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Check for started event with prompt preview
	select {
	case event := <-eventChan:
		assert.Equal(t, EventLLMRequestStarted, event.Type)
		assert.Equal(t, LevelDebug, event.Level)

		payload, ok := event.Payload.(LLMRequestStartedData)
		require.True(t, ok)
		assert.Equal(t, "primary", payload.SlotName)
		assert.Equal(t, 2, payload.MessageCount)
		assert.NotEmpty(t, payload.PromptPreview, "Debug level should include prompt preview")
		assert.Contains(t, payload.PromptPreview, "user:")
		assert.Contains(t, payload.PromptPreview, "assistant:")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected started event not received")
	}

	// Check for completed event with response preview
	select {
	case event := <-eventChan:
		assert.Equal(t, EventLLMRequestCompleted, event.Type)
		assert.Equal(t, LevelDebug, event.Level)

		payload, ok := event.Payload.(LLMRequestCompletedData)
		require.True(t, ok)
		assert.Equal(t, "test-model", payload.Model)
		assert.NotEmpty(t, payload.ResponsePreview, "Debug level should include response preview")
		assert.Contains(t, payload.ResponsePreview, "This is the response content")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected completed event not received")
	}
}

// TestVerboseHarnessWrapper_VeryVerboseLevel_NoPreview tests that previews are NOT included at VeryVerbose level.
func TestVerboseHarnessWrapper_VeryVerboseLevel_NoPreview(t *testing.T) {
	// Setup
	bus := NewDefaultVerboseEventBus()
	defer bus.Close()

	mockHarness := &mockDebugHarness{
		missionCtx: harness.MissionContext{
			ID:           types.NewID(),
			Name:         "verbose-test",
			CurrentAgent: "test-agent",
		},
		completeResp: &llm.CompletionResponse{
			Model: "test-model",
			Message: llm.Message{
				Role:    llm.RoleAssistant,
				Content: "Response content",
			},
			Usage: llm.CompletionTokenUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
			},
			FinishReason: llm.FinishReasonStop,
		},
	}

	wrapper := NewVerboseHarnessWrapper(mockHarness, bus, LevelVeryVerbose)

	// Subscribe to events
	ctx := context.Background()
	eventChan, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	// Execute
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "User message"},
	}

	resp, err := wrapper.Complete(ctx, "primary", messages)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Check for started event WITHOUT prompt preview
	select {
	case event := <-eventChan:
		assert.Equal(t, EventLLMRequestStarted, event.Type)
		assert.Equal(t, LevelVeryVerbose, event.Level)

		payload, ok := event.Payload.(LLMRequestStartedData)
		require.True(t, ok)
		assert.Empty(t, payload.PromptPreview, "VeryVerbose level should NOT include prompt preview")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected started event not received")
	}

	// Check for completed event WITHOUT response preview
	select {
	case event := <-eventChan:
		assert.Equal(t, EventLLMRequestCompleted, event.Type)
		assert.Equal(t, LevelVeryVerbose, event.Level)

		payload, ok := event.Payload.(LLMRequestCompletedData)
		require.True(t, ok)
		assert.Empty(t, payload.ResponsePreview, "VeryVerbose level should NOT include response preview")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected completed event not received")
	}
}

// TestVerboseHarnessWrapper_DebugLevel_ErrorDetails tests error details at Debug level.
func TestVerboseHarnessWrapper_DebugLevel_ErrorDetails(t *testing.T) {
	// Setup
	bus := NewDefaultVerboseEventBus()
	defer bus.Close()

	expectedError := types.NewError("LLM_ERROR", "detailed error message with stack trace")

	mockHarness := &mockDebugHarness{
		missionCtx: harness.MissionContext{
			ID:           types.NewID(),
			Name:         "error-test",
			CurrentAgent: "test-agent",
		},
		completeResp: nil,
		completeErr:  expectedError,
	}

	wrapper := NewVerboseHarnessWrapper(mockHarness, bus, LevelDebug)

	// Subscribe to events
	ctx := context.Background()
	eventChan, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	// Execute
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Test message"},
	}

	resp, err := wrapper.Complete(ctx, "primary", messages)
	assert.Error(t, err)
	assert.Nil(t, resp)

	// Skip the started event
	<-eventChan

	// Check for failed event with error details
	select {
	case event := <-eventChan:
		assert.Equal(t, EventLLMRequestFailed, event.Type)
		assert.Equal(t, LevelDebug, event.Level)

		payload, ok := event.Payload.(LLMRequestFailedData)
		require.True(t, ok)
		assert.Equal(t, "primary", payload.SlotName)
		assert.NotEmpty(t, payload.Error)
		assert.NotEmpty(t, payload.ErrorDetails, "Debug level should include full error details")
		assert.Contains(t, payload.ErrorDetails, "detailed error message")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected failed event not received")
	}
}

// TestVerboseHarnessWrapper_CompleteWithTools_DebugLevel tests tool count at Debug level.
func TestVerboseHarnessWrapper_CompleteWithTools_DebugLevel(t *testing.T) {
	// Setup
	bus := NewDefaultVerboseEventBus()
	defer bus.Close()

	mockHarness := &mockDebugHarness{
		missionCtx: harness.MissionContext{
			ID:           types.NewID(),
			Name:         "tools-test",
			CurrentAgent: "test-agent",
		},
		toolsResp: &llm.CompletionResponse{
			Model: "test-model",
			Message: llm.Message{
				Role:    llm.RoleAssistant,
				Content: "Response with tools",
			},
			Usage: llm.CompletionTokenUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
			},
			FinishReason: llm.FinishReasonStop,
		},
	}

	wrapper := NewVerboseHarnessWrapper(mockHarness, bus, LevelDebug)

	// Subscribe to events
	ctx := context.Background()
	eventChan, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	// Execute with tools
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Use tools to help"},
	}

	tools := []llm.ToolDef{
		{Name: "tool1", Description: "Tool 1"},
		{Name: "tool2", Description: "Tool 2"},
		{Name: "tool3", Description: "Tool 3"},
	}

	resp, err := wrapper.CompleteWithTools(ctx, "primary", messages, tools)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Check for started event with tool count
	select {
	case event := <-eventChan:
		assert.Equal(t, EventLLMRequestStarted, event.Type)
		assert.Equal(t, LevelDebug, event.Level)

		payload, ok := event.Payload.(LLMRequestStartedData)
		require.True(t, ok)
		assert.Equal(t, 3, payload.ToolCount, "Debug level should include tool count")
		assert.NotEmpty(t, payload.PromptPreview, "Debug level should include prompt preview")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected started event not received")
	}
}

// TestTruncatePromptContent tests the prompt truncation helper.
func TestTruncatePromptContent(t *testing.T) {
	tests := []struct {
		name     string
		messages []llm.Message
		maxChars int
		expected string
	}{
		{
			name:     "empty messages",
			messages: []llm.Message{},
			maxChars: 100,
			expected: "",
		},
		{
			name: "single message under limit",
			messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Hello"},
			},
			maxChars: 100,
			expected: "user: Hello",
		},
		{
			name: "multiple messages under limit",
			messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Hello"},
				{Role: llm.RoleAssistant, Content: "Hi there"},
			},
			maxChars: 100,
			expected: "user: Hello | assistant: Hi there",
		},
		{
			name: "truncated content",
			messages: []llm.Message{
				{Role: llm.RoleUser, Content: "This is a very long message that should be truncated"},
			},
			maxChars: 30,
			expected: "user: This is a very long m...",
		},
		{
			name: "very small limit",
			messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Hello world"},
			},
			maxChars: 5,
			expected: "us...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncatePromptContent(tt.messages, tt.maxChars)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTruncateContent tests the content truncation helper.
func TestTruncateContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		maxChars int
		expected string
	}{
		{
			name:     "content under limit",
			content:  "Short text",
			maxChars: 100,
			expected: "Short text",
		},
		{
			name:     "content at limit",
			content:  "Exactly",
			maxChars: 7,
			expected: "Exactly",
		},
		{
			name:     "content over limit",
			content:  "This is a very long text that needs truncation",
			maxChars: 20,
			expected: "This is a very lo...",
		},
		{
			name:     "very small limit",
			content:  "Hello",
			maxChars: 3,
			expected: "Hel",
		},
		{
			name:     "limit of 3",
			content:  "Hello world",
			maxChars: 3,
			expected: "Hel",
		},
		{
			name:     "limit of 4",
			content:  "Hello world",
			maxChars: 4,
			expected: "H...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateContent(tt.content, tt.maxChars)
			assert.Equal(t, tt.expected, result)
		})
	}
}
