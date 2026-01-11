package workflow

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// ============================================================================
// Recording Tracer for Testing Agent Execution Spans
// ============================================================================

// RecordedSpan captures all information about a span for verification in tests
type RecordedSpan struct {
	Name       string
	Attributes map[string]string
	Status     codes.Code
	StatusDesc string
	Error      error
	Events     []string
	Ended      bool
}

// recordingSpan implements trace.Span and records all operations
type recordingSpan struct {
	trace.Span // Embed noop span to satisfy private interface methods
	mu         sync.Mutex
	recorded   *RecordedSpan
	spanCtx    trace.SpanContext
	tracer     *RecordingTracer
	isRecording bool
}

func (s *recordingSpan) End(options ...trace.SpanEndOption) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recorded.Ended = true
}

func (s *recordingSpan) AddEvent(name string, options ...trace.EventOption) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recorded.Events = append(s.recorded.Events, name)
}

func (s *recordingSpan) AddLink(link trace.Link) {}

func (s *recordingSpan) IsRecording() bool {
	return s.isRecording
}

func (s *recordingSpan) RecordError(err error, options ...trace.EventOption) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recorded.Error = err
}

func (s *recordingSpan) SpanContext() trace.SpanContext {
	return s.spanCtx
}

func (s *recordingSpan) SetStatus(code codes.Code, description string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recorded.Status = code
	s.recorded.StatusDesc = description
}

func (s *recordingSpan) SetName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recorded.Name = name
}

func (s *recordingSpan) SetAttributes(kv ...attribute.KeyValue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, attr := range kv {
		s.recorded.Attributes[string(attr.Key)] = attr.Value.AsString()
	}
}

func (s *recordingSpan) TracerProvider() trace.TracerProvider {
	return s.tracer.provider
}

// RecordingTracer implements trace.Tracer and records all spans created
type RecordingTracer struct {
	trace.Tracer // Embed noop tracer to satisfy private interface methods
	mu       sync.Mutex
	spans    []*RecordedSpan
	provider trace.TracerProvider
}

func NewRecordingTracer() *RecordingTracer {
	return &RecordingTracer{
		Tracer: noop.NewTracerProvider().Tracer("recording"),
		spans:  make([]*RecordedSpan, 0),
	}
}

func (t *RecordingTracer) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Create recorded span data
	recorded := &RecordedSpan{
		Name:       spanName,
		Attributes: make(map[string]string),
		Status:     codes.Unset,
	}

	// Parse span start options to extract attributes
	cfg := trace.NewSpanStartConfig(opts...)
	for _, attr := range cfg.Attributes() {
		recorded.Attributes[string(attr.Key)] = attr.Value.AsString()
	}

	t.spans = append(t.spans, recorded)

	// Create span implementation
	span := &recordingSpan{
		Span:        noop.Span{},
		recorded:    recorded,
		tracer:      t,
		isRecording: true,
		spanCtx:     trace.NewSpanContext(trace.SpanContextConfig{}),
	}

	return trace.ContextWithSpan(ctx, span), span
}

// GetSpans returns all recorded spans (thread-safe)
func (t *RecordingTracer) GetSpans() []*RecordedSpan {
	t.mu.Lock()
	defer t.mu.Unlock()
	// Return a copy to prevent concurrent modification
	spans := make([]*RecordedSpan, len(t.spans))
	copy(spans, t.spans)
	return spans
}

// GetSpanByName returns the first span with the given name
func (t *RecordingTracer) GetSpanByName(name string) *RecordedSpan {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, span := range t.spans {
		if span.Name == name {
			return span
		}
	}
	return nil
}

// Reset clears all recorded spans
func (t *RecordingTracer) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.spans = make([]*RecordedSpan, 0)
}

// ============================================================================
// Agent Execution Span Tests
// ============================================================================

func TestExecuteAgentNode_SpanCreation(t *testing.T) {
	tests := []struct {
		name              string
		agentName         string
		missionID         types.ID
		agentTask         *agent.Task
		delegateFunc      func(ctx context.Context, name string, task agent.Task) (agent.Result, error)
		wantSpanName      string
		wantAttributes    map[string]string
		wantStatus        codes.Code
		wantStatusPattern string
	}{
		{
			name:      "successful agent execution creates span with correct attributes",
			agentName: "test-agent",
			missionID: types.NewID(),
			agentTask: &agent.Task{
				ID:   types.NewID(),
				Name: "test-task",
			},
			delegateFunc: func(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
				result := agent.NewResult(task.ID)
				result.Complete(map[string]any{"status": "success"})
				return result, nil
			},
			wantSpanName: "gibson.agent.execute",
			wantStatus:   codes.Ok,
		},
		{
			name:      "failed agent execution sets error status",
			agentName: "failing-agent",
			missionID: types.NewID(),
			agentTask: &agent.Task{
				ID:   types.NewID(),
				Name: "failing-task",
			},
			delegateFunc: func(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
				return agent.Result{}, errors.New("agent execution failed")
			},
			wantSpanName: "gibson.agent.execute",
			wantStatus:   codes.Error,
		},
		{
			name:      "agent result with failed status sets error span status",
			agentName: "status-failed-agent",
			missionID: types.NewID(),
			agentTask: &agent.Task{
				ID:   types.NewID(),
				Name: "failed-status-task",
			},
			delegateFunc: func(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
				result := agent.NewResult(task.ID)
				result.Fail(errors.New("test error message"))
				return result, nil
			},
			wantSpanName: "gibson.agent.execute",
			wantStatus:   codes.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create recording tracer
			recordingTracer := NewRecordingTracer()

			// Create mock harness with recording tracer
			mock := NewMockAgentHarness()
			mock.tracer = recordingTracer
			mock.missionID = tt.missionID
			mock.DelegateToAgentFunc = tt.delegateFunc

			// Create executor
			executor := NewWorkflowExecutor()

			// Create agent node
			node := &WorkflowNode{
				ID:        "test-node",
				Type:      NodeTypeAgent,
				Name:      "Test Agent Node",
				AgentName: tt.agentName,
				AgentTask: tt.agentTask,
			}

			// Execute the agent node
			ctx := context.Background()
			_, _ = executor.executeAgentNode(ctx, node, mock)

			// Verify DelegateToAgent was called (or not for validation errors)
			if tt.wantStatus == codes.Ok || tt.delegateFunc != nil {
				require.Equal(t, 1, len(mock.DelegateToAgentCalls), "DelegateToAgent should be called")
			}

			// Get recorded spans
			spans := recordingTracer.GetSpans()
			require.NotEmpty(t, spans, "Expected at least one span to be recorded")

			// Find the agent execution span
			var agentSpan *RecordedSpan
			for _, span := range spans {
				if span.Name == "gibson.agent.execute" {
					agentSpan = span
					break
				}
			}
			require.NotNil(t, agentSpan, "gibson.agent.execute span should exist")

			// Verify span name
			assert.Equal(t, tt.wantSpanName, agentSpan.Name, "Span name mismatch")

			// Verify required attributes
			assert.Contains(t, agentSpan.Attributes, "gibson.mission.id", "Span should have gibson.mission.id attribute")
			assert.Contains(t, agentSpan.Attributes, "gibson.agent.name", "Span should have gibson.agent.name attribute")
			assert.Equal(t, tt.agentName, agentSpan.Attributes["gibson.agent.name"], "Agent name attribute mismatch")

			// Verify task ID attribute if task is provided
			if tt.agentTask != nil {
				assert.Contains(t, agentSpan.Attributes, "gibson.task.id", "Span should have gibson.task.id attribute when AgentTask is not nil")
				assert.Equal(t, tt.agentTask.ID.String(), agentSpan.Attributes["gibson.task.id"], "Task ID attribute mismatch")
			}

			// Verify span status
			assert.Equal(t, tt.wantStatus, agentSpan.Status, "Span status mismatch")

			// Verify span was ended
			assert.True(t, agentSpan.Ended, "Span should be ended")

			// For error cases, verify status description contains error info
			if tt.wantStatus == codes.Error {
				assert.NotEmpty(t, agentSpan.StatusDesc, "Error span should have status description")
			}
		})
	}
}

func TestExecuteAgentNode_SpanAttributesWithMissionID(t *testing.T) {
	// Create a specific mission ID for testing
	missionID := types.NewID()

	// Create recording tracer
	recordingTracer := NewRecordingTracer()

	// Create mock harness with recording tracer and mission ID
	mock := NewMockAgentHarness()
	mock.tracer = recordingTracer
	mock.missionID = missionID
	mock.DelegateToAgentFunc = func(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
		result := agent.NewResult(task.ID)
		result.Complete(map[string]any{"status": "success"})
		return result, nil
	}

	// Create executor
	executor := NewWorkflowExecutor()

	// Create agent node
	taskID := types.NewID()
	node := &WorkflowNode{
		ID:        "test-node",
		Type:      NodeTypeAgent,
		Name:      "Test Agent Node",
		AgentName: "mission-test-agent",
		AgentTask: &agent.Task{
			ID:   taskID,
			Name: "mission-test-task",
		},
	}

	// Execute the agent node
	ctx := context.Background()
	result, err := executor.executeAgentNode(ctx, node, mock)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Get the agent execution span
	agentSpan := recordingTracer.GetSpanByName("gibson.agent.execute")
	require.NotNil(t, agentSpan, "gibson.agent.execute span should exist")

	// Verify all attributes
	assert.Equal(t, missionID.String(), agentSpan.Attributes["gibson.mission.id"], "Mission ID should match")
	assert.Equal(t, "mission-test-agent", agentSpan.Attributes["gibson.agent.name"], "Agent name should match")
	assert.Equal(t, taskID.String(), agentSpan.Attributes["gibson.task.id"], "Task ID should match")

	// Verify span completed successfully
	assert.Equal(t, codes.Ok, agentSpan.Status)
	assert.True(t, agentSpan.Ended)
}

func TestExecuteAgentNode_SpanWithoutTaskID(t *testing.T) {
	// Test that span is created even when AgentTask is nil (though this triggers validation error)
	recordingTracer := NewRecordingTracer()

	mock := NewMockAgentHarness()
	mock.tracer = recordingTracer
	mock.missionID = types.NewID()

	executor := NewWorkflowExecutor()

	// Create node WITHOUT AgentTask
	node := &WorkflowNode{
		ID:        "test-node",
		Type:      NodeTypeAgent,
		Name:      "Test Agent Node",
		AgentName: "test-agent",
		AgentTask: nil, // No task
	}

	ctx := context.Background()
	_, err := executor.executeAgentNode(ctx, node, mock)

	// Should get validation error
	require.Error(t, err)

	// But span should still be created and have mission ID and agent name
	agentSpan := recordingTracer.GetSpanByName("gibson.agent.execute")
	require.NotNil(t, agentSpan, "gibson.agent.execute span should exist even for validation errors")

	// Should have mission ID and agent name, but NOT task ID
	assert.Contains(t, agentSpan.Attributes, "gibson.mission.id")
	assert.Contains(t, agentSpan.Attributes, "gibson.agent.name")
	assert.NotContains(t, agentSpan.Attributes, "gibson.task.id", "Should not have task ID when AgentTask is nil")

	// Span should have error status due to validation failure
	assert.Equal(t, codes.Error, agentSpan.Status)
	assert.True(t, agentSpan.Ended)
}

func TestExecuteAgentNode_SpanWithEmptyAgentName(t *testing.T) {
	// Test validation error when agent name is empty
	recordingTracer := NewRecordingTracer()

	mock := NewMockAgentHarness()
	mock.tracer = recordingTracer
	mock.missionID = types.NewID()

	executor := NewWorkflowExecutor()

	// Create node with empty agent name
	node := &WorkflowNode{
		ID:        "test-node",
		Type:      NodeTypeAgent,
		Name:      "Test Agent Node",
		AgentName: "", // Empty agent name
		AgentTask: &agent.Task{
			ID:   types.NewID(),
			Name: "test-task",
		},
	}

	ctx := context.Background()
	_, err := executor.executeAgentNode(ctx, node, mock)

	// Should get validation error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent_name is required")

	// Span should be created with error status
	agentSpan := recordingTracer.GetSpanByName("gibson.agent.execute")
	require.NotNil(t, agentSpan, "gibson.agent.execute span should exist for validation errors")

	assert.Equal(t, codes.Error, agentSpan.Status)
	assert.Contains(t, agentSpan.StatusDesc, "agent_name is required")
	assert.True(t, agentSpan.Ended)
}

func TestExecuteAgentNode_ConcurrentSpanCreation(t *testing.T) {
	// Test that spans are created correctly when multiple agent nodes execute concurrently
	recordingTracer := NewRecordingTracer()

	mock := NewMockAgentHarness()
	mock.tracer = recordingTracer
	mock.missionID = types.NewID()

	// Add delay to simulate concurrent execution
	mock.DelegateToAgentFunc = func(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
		time.Sleep(50 * time.Millisecond)
		result := agent.NewResult(task.ID)
		result.Complete(map[string]any{"status": "success", "agent": name})
		return result, nil
	}

	executor := NewWorkflowExecutor()

	// Create multiple agent nodes
	numAgents := 5
	var wg sync.WaitGroup
	wg.Add(numAgents)

	for i := 0; i < numAgents; i++ {
		go func(index int) {
			defer wg.Done()

			node := &WorkflowNode{
				ID:        types.NewID().String(),
				Type:      NodeTypeAgent,
				Name:      "Concurrent Agent Node",
				AgentName: types.NewID().String(),
				AgentTask: &agent.Task{
					ID:   types.NewID(),
					Name: "concurrent-task",
				},
			}

			ctx := context.Background()
			_, err := executor.executeAgentNode(ctx, node, mock)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Verify that all spans were created
	spans := recordingTracer.GetSpans()
	agentSpanCount := 0
	for _, span := range spans {
		if span.Name == "gibson.agent.execute" {
			agentSpanCount++
			// Verify each span has required attributes
			assert.Contains(t, span.Attributes, "gibson.mission.id")
			assert.Contains(t, span.Attributes, "gibson.agent.name")
			assert.Contains(t, span.Attributes, "gibson.task.id")
			assert.Equal(t, codes.Ok, span.Status)
			assert.True(t, span.Ended)
		}
	}

	assert.Equal(t, numAgents, agentSpanCount, "Should create one span per agent execution")
}
