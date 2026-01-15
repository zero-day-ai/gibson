package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockEventBusPublisher is a mock implementation of EventBusPublisher for testing
type mockEventBusPublisher struct {
	events []interface{}
}

func (m *mockEventBusPublisher) Publish(ctx context.Context, event interface{}) error {
	m.events = append(m.events, event)
	return nil
}

// TestAgentEventEmission verifies that agent lifecycle events are emitted correctly
func TestAgentEventEmission(t *testing.T) {
	// Create mock event bus
	mockBus := &mockEventBusPublisher{
		events: make([]interface{}, 0),
	}

	// Create executor with event bus
	executor := NewWorkflowExecutor(
		WithEventBus(mockBus),
	)

	// Verify eventBus is set
	assert.NotNil(t, executor.eventBus, "eventBus should be configured")

	// Test emitAgentStarted
	ctx := context.Background()
	executor.emitAgentStarted(ctx, "test-agent", "task-123", "parent-span-456")

	assert.Equal(t, 1, len(mockBus.events), "Should have emitted 1 event")

	// Verify event structure
	eventData, ok := mockBus.events[0].(map[string]interface{})
	assert.True(t, ok, "Event should be a map")
	assert.Equal(t, "agent.started", eventData["type"], "Event type should be agent.started")

	data, ok := eventData["data"].(map[string]interface{})
	assert.True(t, ok, "Event data should be a map")
	assert.Equal(t, "test-agent", data["agent_name"], "Agent name should match")
	assert.Equal(t, "task-123", data["task_id"], "Task ID should match")

	// Test emitAgentCompleted
	mockBus.events = make([]interface{}, 0)
	executor.emitAgentCompleted(ctx, "test-agent", 5*time.Second, "Test completed successfully")

	assert.Equal(t, 1, len(mockBus.events), "Should have emitted 1 event")

	eventData, ok = mockBus.events[0].(map[string]interface{})
	assert.True(t, ok, "Event should be a map")
	assert.Equal(t, "agent.completed", eventData["type"], "Event type should be agent.completed")

	data, ok = eventData["data"].(map[string]interface{})
	assert.True(t, ok, "Event data should be a map")
	assert.Equal(t, "test-agent", data["agent_name"], "Agent name should match")
	assert.Equal(t, int64(5000), data["duration"], "Duration should be 5000ms")
	assert.Equal(t, "Test completed successfully", data["output_summary"], "Output summary should match")

	// Test emitAgentFailed
	mockBus.events = make([]interface{}, 0)
	executor.emitAgentFailed(ctx, "test-agent", 2*time.Second, "Test error message")

	assert.Equal(t, 1, len(mockBus.events), "Should have emitted 1 event")

	eventData, ok = mockBus.events[0].(map[string]interface{})
	assert.True(t, ok, "Event should be a map")
	assert.Equal(t, "agent.failed", eventData["type"], "Event type should be agent.failed")

	data, ok = eventData["data"].(map[string]interface{})
	assert.True(t, ok, "Event data should be a map")
	assert.Equal(t, "test-agent", data["agent_name"], "Agent name should match")
	assert.Equal(t, int64(2000), data["duration"], "Duration should be 2000ms")
	assert.Equal(t, "Test error message", data["error"], "Error message should match")
}

// TestNoEventBusConfigured verifies that execution works when no event bus is configured
func TestNoEventBusConfigured(t *testing.T) {
	// Create executor without event bus
	executor := NewWorkflowExecutor()

	// Verify eventBus is nil
	assert.Nil(t, executor.eventBus, "eventBus should be nil when not configured")

	// Test that emit functions don't panic when eventBus is nil
	ctx := context.Background()

	// These should all be no-ops and not panic
	assert.NotPanics(t, func() {
		executor.emitAgentStarted(ctx, "test-agent", "task-123", "parent-span-456")
	}, "emitAgentStarted should not panic when eventBus is nil")

	assert.NotPanics(t, func() {
		executor.emitAgentCompleted(ctx, "test-agent", 5*time.Second, "Test completed")
	}, "emitAgentCompleted should not panic when eventBus is nil")

	assert.NotPanics(t, func() {
		executor.emitAgentFailed(ctx, "test-agent", 2*time.Second, "Test error")
	}, "emitAgentFailed should not panic when eventBus is nil")
}
