package observability

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// MockAgentHarness is a mock implementation of harness.AgentHarness for testing.
type MockAgentHarness struct {
	CompleteFunc          func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error)
	CompleteWithToolsFunc func(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...harness.CompletionOption) (*llm.CompletionResponse, error)
	StreamFunc            func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (<-chan llm.StreamChunk, error)
	CallToolFunc          func(ctx context.Context, name string, input map[string]any) (map[string]any, error)
	QueryPluginFunc       func(ctx context.Context, name string, method string, params map[string]any) (any, error)
	DelegateToAgentFunc   func(ctx context.Context, name string, task agent.Task) (agent.Result, error)
	SubmitFindingFunc     func(ctx context.Context, finding agent.Finding) error
	GetFindingsFunc       func(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error)
	MemoryFunc            func() memory.MemoryStore
	MissionFunc           func() harness.MissionContext
	TargetFunc            func() harness.TargetInfo
	ListToolsFunc         func() []harness.ToolDescriptor
	ListPluginsFunc       func() []harness.PluginDescriptor
	ListAgentsFunc        func() []harness.AgentDescriptor
	TracerFunc            func() oteltrace.Tracer
	LoggerFunc            func() *slog.Logger
	MetricsFunc           func() harness.MetricsRecorder
	TokenUsageFunc        func() *llm.TokenTracker
}

func (m *MockAgentHarness) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	if m.CompleteFunc != nil {
		return m.CompleteFunc(ctx, slot, messages, opts...)
	}
	return &llm.CompletionResponse{
		ID:    "test-completion",
		Model: "test-model",
		Message: llm.Message{
			Role:    llm.RoleAssistant,
			Content: "Test response",
		},
		FinishReason: llm.FinishReasonStop,
		Usage: llm.CompletionTokenUsage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}, nil
}

func (m *MockAgentHarness) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	if m.CompleteWithToolsFunc != nil {
		return m.CompleteWithToolsFunc(ctx, slot, messages, tools, opts...)
	}
	return &llm.CompletionResponse{
		ID:    "test-completion-tools",
		Model: "test-model",
		Message: llm.Message{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{
					ID:        "tool-call-1",
					Type:      "function",
					Name:      "test_tool",
					Arguments: `{"arg": "value"}`,
				},
			},
		},
		FinishReason: llm.FinishReasonToolCalls,
		Usage: llm.CompletionTokenUsage{
			PromptTokens:     100,
			CompletionTokens: 30,
			TotalTokens:      130,
		},
	}, nil
}

func (m *MockAgentHarness) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (<-chan llm.StreamChunk, error) {
	if m.StreamFunc != nil {
		return m.StreamFunc(ctx, slot, messages, opts...)
	}

	ch := make(chan llm.StreamChunk, 3)
	go func() {
		defer close(ch)
		ch <- llm.StreamChunk{
			Delta: llm.StreamDelta{
				Role:    llm.RoleAssistant,
				Content: "Hello",
			},
		}
		ch <- llm.StreamChunk{
			Delta: llm.StreamDelta{
				Content: " world",
			},
		}
		ch <- llm.StreamChunk{
			FinishReason: llm.FinishReasonStop,
		}
	}()

	return ch, nil
}

func (m *MockAgentHarness) CallTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	if m.CallToolFunc != nil {
		return m.CallToolFunc(ctx, name, input)
	}
	return map[string]any{"result": "success"}, nil
}

func (m *MockAgentHarness) QueryPlugin(ctx context.Context, name string, method string, params map[string]any) (any, error) {
	if m.QueryPluginFunc != nil {
		return m.QueryPluginFunc(ctx, name, method, params)
	}
	return map[string]any{"status": "ok"}, nil
}

func (m *MockAgentHarness) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
	if m.DelegateToAgentFunc != nil {
		return m.DelegateToAgentFunc(ctx, name, task)
	}
	return agent.Result{
		TaskID:      task.ID,
		Status:      agent.ResultStatusCompleted,
		Output:      map[string]any{"delegated": true},
		Findings:    []agent.Finding{},
		StartedAt:   time.Now(),
		CompletedAt: time.Now().Add(time.Second),
		Metrics: agent.TaskMetrics{
			Duration:   time.Second,
			LLMCalls:   1,
			ToolCalls:  2,
			TokensUsed: 150,
		},
	}, nil
}

func (m *MockAgentHarness) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	if m.SubmitFindingFunc != nil {
		return m.SubmitFindingFunc(ctx, finding)
	}
	return nil
}

func (m *MockAgentHarness) GetFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	if m.GetFindingsFunc != nil {
		return m.GetFindingsFunc(ctx, filter)
	}
	return []agent.Finding{}, nil
}

func (m *MockAgentHarness) Memory() memory.MemoryStore {
	if m.MemoryFunc != nil {
		return m.MemoryFunc()
	}
	return nil
}

func (m *MockAgentHarness) Mission() harness.MissionContext {
	if m.MissionFunc != nil {
		return m.MissionFunc()
	}
	return harness.MissionContext{
		ID:           types.NewID(),
		Name:         "test-mission",
		CurrentAgent: "test-agent",
		Phase:        "testing",
	}
}

func (m *MockAgentHarness) Target() harness.TargetInfo {
	if m.TargetFunc != nil {
		return m.TargetFunc()
	}
	return harness.TargetInfo{
		ID:   types.NewID(),
		Name: "test-target",
		URL:  "https://example.com",
		Type: "web",
	}
}

func (m *MockAgentHarness) ListTools() []harness.ToolDescriptor {
	if m.ListToolsFunc != nil {
		return m.ListToolsFunc()
	}
	return []harness.ToolDescriptor{}
}

func (m *MockAgentHarness) ListPlugins() []harness.PluginDescriptor {
	if m.ListPluginsFunc != nil {
		return m.ListPluginsFunc()
	}
	return []harness.PluginDescriptor{}
}

func (m *MockAgentHarness) ListAgents() []harness.AgentDescriptor {
	if m.ListAgentsFunc != nil {
		return m.ListAgentsFunc()
	}
	return []harness.AgentDescriptor{}
}

func (m *MockAgentHarness) Tracer() oteltrace.Tracer {
	if m.TracerFunc != nil {
		return m.TracerFunc()
	}
	// Return a no-op tracer by default
	provider := trace.NewTracerProvider()
	return provider.Tracer("test")
}

func (m *MockAgentHarness) Logger() *slog.Logger {
	if m.LoggerFunc != nil {
		return m.LoggerFunc()
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func (m *MockAgentHarness) Metrics() harness.MetricsRecorder {
	if m.MetricsFunc != nil {
		return m.MetricsFunc()
	}
	return harness.NewNoOpMetricsRecorder()
}

func (m *MockAgentHarness) TokenUsage() *llm.TokenTracker {
	if m.TokenUsageFunc != nil {
		return m.TokenUsageFunc()
	}
	return nil
}

// setupTestHarness creates a traced harness with an in-memory span exporter for testing.
func setupTestHarness(t *testing.T, mockInner *MockAgentHarness) (*TracedAgentHarness, *tracetest.InMemoryExporter) {
	t.Helper()

	// Create in-memory span exporter
	exporter := tracetest.NewInMemoryExporter()

	// Create tracer provider with exporter
	tp := trace.NewTracerProvider(
		trace.WithSyncer(exporter),
	)

	// Create tracer
	tracer := tp.Tracer("test-harness")

	// Create traced logger
	logger := NewTracedLogger(
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}),
		"test-mission-id",
		"test-agent",
	)

	// Create traced harness
	traced := NewTracedAgentHarness(
		mockInner,
		WithTracer(tracer),
		WithLogger(logger),
		WithPromptCapture(true),
	)

	return traced, exporter
}

func TestTracedAgentHarness_Complete(t *testing.T) {
	t.Run("successful completion creates span with GenAI attributes", func(t *testing.T) {
		// Setup
		mock := &MockAgentHarness{}
		traced, exporter := setupTestHarness(t, mock)
		ctx := context.Background()

		// Execute
		messages := []llm.Message{
			llm.NewUserMessage("Test prompt"),
		}
		resp, err := traced.Complete(ctx, "primary", messages)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "test-model", resp.Model)
		assert.Equal(t, 100, resp.Usage.PromptTokens)
		assert.Equal(t, 50, resp.Usage.CompletionTokens)

		// Verify span was created
		spans := exporter.GetSpans()
		require.Len(t, spans, 1)

		span := spans[0]
		assert.Equal(t, SpanGenAIChat, span.Name)

		// Verify attributes
		attrs := span.Attributes
		hasModel := false
		hasInputTokens := false
		hasOutputTokens := false
		hasSlot := false
		hasTurn := false

		for _, attr := range attrs {
			switch string(attr.Key) {
			case "gen_ai.request.model":
				hasModel = true
				assert.Equal(t, "test-model", attr.Value.AsString())
			case "gen_ai.usage.input_tokens":
				hasInputTokens = true
				assert.Equal(t, int64(100), attr.Value.AsInt64())
			case "gen_ai.usage.output_tokens":
				hasOutputTokens = true
				assert.Equal(t, int64(50), attr.Value.AsInt64())
			case "gibson.llm.slot":
				hasSlot = true
				assert.Equal(t, "primary", attr.Value.AsString())
			case "gibson.turn.number":
				hasTurn = true
				assert.Equal(t, int64(1), attr.Value.AsInt64())
			}
		}

		assert.True(t, hasModel, "span should have model attribute")
		assert.True(t, hasInputTokens, "span should have input tokens attribute")
		assert.True(t, hasOutputTokens, "span should have output tokens attribute")
		assert.True(t, hasSlot, "span should have slot attribute")
		assert.True(t, hasTurn, "span should have turn number attribute")
	})

	t.Run("completion error records error on span", func(t *testing.T) {
		// Setup
		expectedErr := errors.New("completion failed")
		mock := &MockAgentHarness{
			CompleteFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
				return nil, expectedErr
			},
		}
		traced, exporter := setupTestHarness(t, mock)
		ctx := context.Background()

		// Execute
		messages := []llm.Message{
			llm.NewUserMessage("Test prompt"),
		}
		resp, err := traced.Complete(ctx, "primary", messages)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, expectedErr, err)

		// Verify span was created with error
		spans := exporter.GetSpans()
		require.Len(t, spans, 1)

		span := spans[0]
		assert.Equal(t, SpanGenAIChat, span.Name)

		// Verify error was recorded
		require.Len(t, span.Events, 1)
		event := span.Events[0]
		assert.Equal(t, "exception", event.Name)

		// Verify error attributes
		hasErrorAttr := false
		for _, attr := range span.Attributes {
			if string(attr.Key) == "error" {
				hasErrorAttr = true
				assert.True(t, attr.Value.AsBool())
			}
		}
		assert.True(t, hasErrorAttr, "span should have error attribute")
	})
}

func TestTracedAgentHarness_CompleteWithTools(t *testing.T) {
	t.Run("successful completion with tools includes tool definitions", func(t *testing.T) {
		// Setup
		mock := &MockAgentHarness{}
		traced, exporter := setupTestHarness(t, mock)
		ctx := context.Background()

		// Execute
		messages := []llm.Message{
			llm.NewUserMessage("Test prompt"),
		}
		tools := []llm.ToolDef{
			{
				Name:        "test_tool",
				Description: "A test tool",
			},
			{
				Name:        "another_tool",
				Description: "Another test tool",
			},
		}
		resp, err := traced.CompleteWithTools(ctx, "primary", messages, tools)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Len(t, resp.Message.ToolCalls, 1)

		// Verify span was created
		spans := exporter.GetSpans()
		require.Len(t, spans, 1)

		span := spans[0]
		assert.Equal(t, SpanGenAIChat, span.Name)

		// Verify tool attributes
		hasToolCount := false
		hasToolNames := false
		hasToolCallCount := false

		for _, attr := range span.Attributes {
			switch string(attr.Key) {
			case "gen_ai.tools.count":
				hasToolCount = true
				assert.Equal(t, int64(2), attr.Value.AsInt64())
			case "gen_ai.tools.names":
				hasToolNames = true
				toolNames := attr.Value.AsStringSlice()
				assert.Len(t, toolNames, 2)
				assert.Contains(t, toolNames, "test_tool")
				assert.Contains(t, toolNames, "another_tool")
			case "gen_ai.response.tool_calls.count":
				hasToolCallCount = true
				assert.Equal(t, int64(1), attr.Value.AsInt64())
			}
		}

		assert.True(t, hasToolCount, "span should have tool count attribute")
		assert.True(t, hasToolNames, "span should have tool names attribute")
		assert.True(t, hasToolCallCount, "span should have tool call count attribute")
	})
}

func TestTracedAgentHarness_CallTool(t *testing.T) {
	t.Run("successful tool call creates gen_ai.tool span", func(t *testing.T) {
		// Setup
		mock := &MockAgentHarness{}
		traced, exporter := setupTestHarness(t, mock)
		ctx := context.Background()

		// Execute
		input := map[string]any{"arg": "value"}
		result, err := traced.CallTool(ctx, "test_tool", input)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "success", result["result"])

		// Verify span was created
		spans := exporter.GetSpans()
		require.Len(t, spans, 1)

		span := spans[0]
		assert.Equal(t, SpanGenAITool, span.Name)

		// Verify tool name attribute
		hasToolName := false
		hasDuration := false

		for _, attr := range span.Attributes {
			switch string(attr.Key) {
			case "gibson.tool.name":
				hasToolName = true
				assert.Equal(t, "test_tool", attr.Value.AsString())
			case "gibson.tool.duration_ms":
				hasDuration = true
			}
		}

		assert.True(t, hasToolName, "span should have tool name attribute")
		assert.True(t, hasDuration, "span should have duration attribute")
	})

	t.Run("tool error records error on span", func(t *testing.T) {
		// Setup
		expectedErr := errors.New("tool execution failed")
		mock := &MockAgentHarness{
			CallToolFunc: func(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
				return nil, expectedErr
			},
		}
		traced, exporter := setupTestHarness(t, mock)
		ctx := context.Background()

		// Execute
		result, err := traced.CallTool(ctx, "failing_tool", map[string]any{})

		// Assert
		assert.Error(t, err)
		assert.Nil(t, result)

		// Verify span with error
		spans := exporter.GetSpans()
		require.Len(t, spans, 1)

		span := spans[0]
		require.Len(t, span.Events, 1)
		assert.Equal(t, "exception", span.Events[0].Name)
	})
}

func TestTracedAgentHarness_QueryPlugin(t *testing.T) {
	t.Run("successful plugin query creates gibson.plugin.query span", func(t *testing.T) {
		// Setup
		mock := &MockAgentHarness{}
		traced, exporter := setupTestHarness(t, mock)
		ctx := context.Background()

		// Execute
		params := map[string]any{"param": "value"}
		result, err := traced.QueryPlugin(ctx, "test_plugin", "test_method", params)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify span was created
		spans := exporter.GetSpans()
		require.Len(t, spans, 1)

		span := spans[0]
		assert.Equal(t, SpanPluginQuery, span.Name)

		// Verify plugin attributes
		hasPluginName := false
		hasMethod := false

		for _, attr := range span.Attributes {
			switch string(attr.Key) {
			case "gibson.plugin.name":
				hasPluginName = true
				assert.Equal(t, "test_plugin", attr.Value.AsString())
			case "gibson.plugin.method":
				hasMethod = true
				assert.Equal(t, "test_method", attr.Value.AsString())
			}
		}

		assert.True(t, hasPluginName, "span should have plugin name attribute")
		assert.True(t, hasMethod, "span should have plugin method attribute")
	})
}

func TestTracedAgentHarness_DelegateToAgent(t *testing.T) {
	t.Run("successful delegation creates gibson.agent.delegate span", func(t *testing.T) {
		// Setup
		mock := &MockAgentHarness{}
		traced, exporter := setupTestHarness(t, mock)
		ctx := context.Background()

		// Execute
		task := agent.NewTask("test-task", "Test task description", map[string]any{})
		result, err := traced.DelegateToAgent(ctx, "target_agent", task)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, agent.ResultStatusCompleted, result.Status)

		// Verify span was created
		spans := exporter.GetSpans()
		require.Len(t, spans, 1)

		span := spans[0]
		assert.Equal(t, SpanAgentDelegate, span.Name)

		// Verify delegation attributes
		hasTargetAgent := false
		hasTaskID := false
		hasTaskName := false
		hasResultStatus := false

		for _, attr := range span.Attributes {
			switch string(attr.Key) {
			case "gibson.delegation.target_agent":
				hasTargetAgent = true
				assert.Equal(t, "target_agent", attr.Value.AsString())
			case "gibson.delegation.task_id":
				hasTaskID = true
				assert.Equal(t, task.ID.String(), attr.Value.AsString())
			case "gibson.task.name":
				hasTaskName = true
				assert.Equal(t, "test-task", attr.Value.AsString())
			case "gibson.delegation.result_status":
				hasResultStatus = true
				assert.Equal(t, string(agent.ResultStatusCompleted), attr.Value.AsString())
			}
		}

		assert.True(t, hasTargetAgent, "span should have target agent attribute")
		assert.True(t, hasTaskID, "span should have task ID attribute")
		assert.True(t, hasTaskName, "span should have task name attribute")
		assert.True(t, hasResultStatus, "span should have result status attribute")
	})
}

func TestTracedAgentHarness_SubmitFinding(t *testing.T) {
	t.Run("successful finding submission creates gibson.finding.submit span", func(t *testing.T) {
		// Setup
		mock := &MockAgentHarness{}
		traced, exporter := setupTestHarness(t, mock)
		ctx := context.Background()

		// Execute
		finding := agent.NewFinding(
			"SQL Injection",
			"Found SQL injection vulnerability",
			agent.SeverityHigh,
		).WithCategory("injection").WithConfidence(0.95)

		err := traced.SubmitFinding(ctx, finding)

		// Assert
		require.NoError(t, err)

		// Verify span was created
		spans := exporter.GetSpans()
		require.Len(t, spans, 1)

		span := spans[0]
		assert.Equal(t, SpanFindingSubmit, span.Name)

		// Verify finding attributes
		hasFindingID := false
		hasSeverity := false
		hasCategory := false
		hasConfidence := false

		for _, attr := range span.Attributes {
			switch string(attr.Key) {
			case "gibson.finding.id":
				hasFindingID = true
				assert.Equal(t, finding.ID.String(), attr.Value.AsString())
			case "gibson.finding.severity":
				hasSeverity = true
				assert.Equal(t, string(agent.SeverityHigh), attr.Value.AsString())
			case "gibson.finding.category":
				hasCategory = true
				assert.Equal(t, "injection", attr.Value.AsString())
			case "gibson.finding.confidence":
				hasConfidence = true
				assert.Equal(t, 0.95, attr.Value.AsFloat64())
			}
		}

		assert.True(t, hasFindingID, "span should have finding ID attribute")
		assert.True(t, hasSeverity, "span should have severity attribute")
		assert.True(t, hasCategory, "span should have category attribute")
		assert.True(t, hasConfidence, "span should have confidence attribute")
	})
}

func TestTracedAgentHarness_Stream(t *testing.T) {
	t.Run("successful stream creates gen_ai.chat.stream span", func(t *testing.T) {
		// Setup
		mock := &MockAgentHarness{}
		traced, exporter := setupTestHarness(t, mock)
		ctx := context.Background()

		// Execute
		messages := []llm.Message{
			llm.NewUserMessage("Test prompt"),
		}
		chunkChan, err := traced.Stream(ctx, "primary", messages)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, chunkChan)

		// Consume all chunks
		chunks := []llm.StreamChunk{}
		for chunk := range chunkChan {
			chunks = append(chunks, chunk)
		}

		// Should have received chunks
		assert.Len(t, chunks, 3)

		// Verify span was created
		spans := exporter.GetSpans()
		require.Len(t, spans, 1)

		span := spans[0]
		assert.Equal(t, SpanGenAIChatStream, span.Name)

		// Verify slot attribute
		hasSlot := false
		for _, attr := range span.Attributes {
			if string(attr.Key) == "gibson.llm.slot" {
				hasSlot = true
				assert.Equal(t, "primary", attr.Value.AsString())
			}
		}

		assert.True(t, hasSlot, "span should have slot attribute")
	})
}

func TestTracedAgentHarness_PassthroughMethods(t *testing.T) {
	t.Run("Memory returns inner harness memory", func(t *testing.T) {
		mock := &MockAgentHarness{}
		traced, _ := setupTestHarness(t, mock)

		// Memory should return nil (from mock)
		mem := traced.Memory()
		assert.Nil(t, mem)
	})

	t.Run("Mission returns inner harness mission", func(t *testing.T) {
		mock := &MockAgentHarness{}
		traced, _ := setupTestHarness(t, mock)

		mission := traced.Mission()
		assert.Equal(t, "test-mission", mission.Name)
		assert.Equal(t, "test-agent", mission.CurrentAgent)
	})

	t.Run("Target returns inner harness target", func(t *testing.T) {
		mock := &MockAgentHarness{}
		traced, _ := setupTestHarness(t, mock)

		target := traced.Target()
		assert.Equal(t, "test-target", target.Name)
		assert.Equal(t, "https://example.com", target.URL)
	})

	t.Run("ListTools returns inner harness tools", func(t *testing.T) {
		mock := &MockAgentHarness{}
		traced, _ := setupTestHarness(t, mock)

		tools := traced.ListTools()
		assert.NotNil(t, tools)
		assert.Len(t, tools, 0)
	})

	t.Run("ListPlugins returns inner harness plugins", func(t *testing.T) {
		mock := &MockAgentHarness{}
		traced, _ := setupTestHarness(t, mock)

		plugins := traced.ListPlugins()
		assert.NotNil(t, plugins)
		assert.Len(t, plugins, 0)
	})

	t.Run("ListAgents returns inner harness agents", func(t *testing.T) {
		mock := &MockAgentHarness{}
		traced, _ := setupTestHarness(t, mock)

		agents := traced.ListAgents()
		assert.NotNil(t, agents)
		assert.Len(t, agents, 0)
	})

	t.Run("Tracer returns configured tracer", func(t *testing.T) {
		mock := &MockAgentHarness{}
		traced, _ := setupTestHarness(t, mock)

		tracer := traced.Tracer()
		assert.NotNil(t, tracer)
	})

	t.Run("Logger returns configured logger", func(t *testing.T) {
		mock := &MockAgentHarness{}
		traced, _ := setupTestHarness(t, mock)

		logger := traced.Logger()
		assert.NotNil(t, logger)
	})

	t.Run("Metrics returns configured metrics", func(t *testing.T) {
		mock := &MockAgentHarness{}
		traced, _ := setupTestHarness(t, mock)

		metrics := traced.Metrics()
		assert.NotNil(t, metrics)
	})
}

func TestTracedAgentHarness_TurnCounter(t *testing.T) {
	t.Run("turn counter increments across multiple completions", func(t *testing.T) {
		// Setup
		mock := &MockAgentHarness{}
		traced, exporter := setupTestHarness(t, mock)
		ctx := context.Background()

		messages := []llm.Message{
			llm.NewUserMessage("Test prompt"),
		}

		// Execute multiple completions
		_, err := traced.Complete(ctx, "primary", messages)
		require.NoError(t, err)

		_, err = traced.Complete(ctx, "primary", messages)
		require.NoError(t, err)

		_, err = traced.Complete(ctx, "primary", messages)
		require.NoError(t, err)

		// Verify spans
		spans := exporter.GetSpans()
		require.Len(t, spans, 3)

		// Check turn numbers
		turnNumbers := make([]int64, 3)
		for i, span := range spans {
			for _, attr := range span.Attributes {
				if string(attr.Key) == "gibson.turn.number" {
					turnNumbers[i] = attr.Value.AsInt64()
				}
			}
		}

		assert.Equal(t, int64(1), turnNumbers[0])
		assert.Equal(t, int64(2), turnNumbers[1])
		assert.Equal(t, int64(3), turnNumbers[2])
	})
}

func TestTracedAgentHarness_PromptCapture(t *testing.T) {
	t.Run("prompt capture includes full messages when enabled", func(t *testing.T) {
		// Setup
		mock := &MockAgentHarness{}
		exporter := tracetest.NewInMemoryExporter()
		tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
		tracer := tp.Tracer("test-harness")

		traced := NewTracedAgentHarness(
			mock,
			WithTracer(tracer),
			WithPromptCapture(true), // Enable prompt capture
		)

		ctx := context.Background()

		// Execute
		messages := []llm.Message{
			llm.NewSystemMessage("You are a helpful assistant"),
			llm.NewUserMessage("What is 2+2?"),
		}
		_, err := traced.Complete(ctx, "primary", messages)
		require.NoError(t, err)

		// Verify span contains prompt
		spans := exporter.GetSpans()
		require.Len(t, spans, 1)

		hasPrompt := false
		for _, attr := range spans[0].Attributes {
			if string(attr.Key) == GenAIPrompt {
				hasPrompt = true
				prompt := attr.Value.AsString()
				assert.Contains(t, prompt, "You are a helpful assistant")
				assert.Contains(t, prompt, "What is 2+2?")
			}
		}

		assert.True(t, hasPrompt, "span should have prompt attribute when capture is enabled")
	})

	t.Run("prompt capture can be disabled", func(t *testing.T) {
		// Setup
		mock := &MockAgentHarness{}
		exporter := tracetest.NewInMemoryExporter()
		tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
		tracer := tp.Tracer("test-harness")

		traced := NewTracedAgentHarness(
			mock,
			WithTracer(tracer),
			WithPromptCapture(false), // Explicitly disable prompt capture
		)

		ctx := context.Background()

		// Execute
		messages := []llm.Message{
			llm.NewUserMessage("Test prompt"),
		}
		_, err := traced.Complete(ctx, "primary", messages)
		require.NoError(t, err)

		// Verify span does NOT contain prompt
		spans := exporter.GetSpans()
		require.Len(t, spans, 1)

		for _, attr := range spans[0].Attributes {
			assert.NotEqual(t, GenAIPrompt, string(attr.Key), "span should not have prompt when capture is disabled")
		}
	})
}

func TestTracedAgentHarness_MetricsRecording(t *testing.T) {
	t.Run("records LLM completion metrics", func(t *testing.T) {
		// Setup - create a mock metrics recorder to verify calls
		var recordedCounters []string
		var recordedHistograms []string
		mockMetrics := &MockMetricsRecorder{
			RecordCounterFunc: func(name string, value int64, labels map[string]string) {
				recordedCounters = append(recordedCounters, name)
			},
			RecordHistogramFunc: func(name string, value float64, labels map[string]string) {
				recordedHistograms = append(recordedHistograms, name)
			},
		}

		mock := &MockAgentHarness{
			MetricsFunc: func() harness.MetricsRecorder {
				return mockMetrics
			},
		}

		traced, _ := setupTestHarness(t, mock)
		traced.metrics = mockMetrics // Override with our mock

		ctx := context.Background()

		// Execute
		messages := []llm.Message{
			llm.NewUserMessage("Test"),
		}
		_, err := traced.Complete(ctx, "primary", messages)
		require.NoError(t, err)

		// Verify metrics were recorded
		assert.Contains(t, recordedCounters, MetricLLMCompletions)
		assert.Contains(t, recordedCounters, MetricLLMTokensInput)
		assert.Contains(t, recordedCounters, MetricLLMTokensOutput)
		assert.Contains(t, recordedHistograms, MetricLLMLatency)
	})

	t.Run("records tool call metrics", func(t *testing.T) {
		// Setup
		var recordedCounters []string
		var recordedHistograms []string
		mockMetrics := &MockMetricsRecorder{
			RecordCounterFunc: func(name string, value int64, labels map[string]string) {
				recordedCounters = append(recordedCounters, name)
			},
			RecordHistogramFunc: func(name string, value float64, labels map[string]string) {
				recordedHistograms = append(recordedHistograms, name)
			},
		}

		mock := &MockAgentHarness{
			MetricsFunc: func() harness.MetricsRecorder {
				return mockMetrics
			},
		}

		traced, _ := setupTestHarness(t, mock)
		traced.metrics = mockMetrics

		ctx := context.Background()

		// Execute
		_, err := traced.CallTool(ctx, "test_tool", map[string]any{})
		require.NoError(t, err)

		// Verify metrics were recorded
		assert.Contains(t, recordedCounters, MetricToolCalls)
		assert.Contains(t, recordedHistograms, MetricToolDuration)
	})

	t.Run("records finding submission metrics", func(t *testing.T) {
		// Setup
		var recordedCounters []string
		mockMetrics := &MockMetricsRecorder{
			RecordCounterFunc: func(name string, value int64, labels map[string]string) {
				recordedCounters = append(recordedCounters, name)
			},
		}

		mock := &MockAgentHarness{
			MetricsFunc: func() harness.MetricsRecorder {
				return mockMetrics
			},
		}

		traced, _ := setupTestHarness(t, mock)
		traced.metrics = mockMetrics

		ctx := context.Background()

		// Execute
		finding := agent.NewFinding("Test", "Test finding", agent.SeverityHigh)
		err := traced.SubmitFinding(ctx, finding)
		require.NoError(t, err)

		// Verify metrics were recorded
		assert.Contains(t, recordedCounters, MetricFindingsSubmitted)
	})
}

// MockMetricsRecorder is a mock implementation for testing metrics recording
type MockMetricsRecorder struct {
	RecordCounterFunc   func(name string, value int64, labels map[string]string)
	RecordGaugeFunc     func(name string, value float64, labels map[string]string)
	RecordHistogramFunc func(name string, value float64, labels map[string]string)
}

func (m *MockMetricsRecorder) RecordCounter(name string, value int64, labels map[string]string) {
	if m.RecordCounterFunc != nil {
		m.RecordCounterFunc(name, value, labels)
	}
}

func (m *MockMetricsRecorder) RecordGauge(name string, value float64, labels map[string]string) {
	if m.RecordGaugeFunc != nil {
		m.RecordGaugeFunc(name, value, labels)
	}
}

func (m *MockMetricsRecorder) RecordHistogram(name string, value float64, labels map[string]string) {
	if m.RecordHistogramFunc != nil {
		m.RecordHistogramFunc(name, value, labels)
	}
}

// Ensure MockMetricsRecorder implements MetricsRecorder
var _ harness.MetricsRecorder = (*MockMetricsRecorder)(nil)
