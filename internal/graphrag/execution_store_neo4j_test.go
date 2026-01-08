package graphrag

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/types"
)

func TestNeo4jExecutionGraphStore_RecordMissionStart(t *testing.T) {
	mockClient := graph.NewMockGraphClient()
	require.NoError(t, mockClient.Connect(context.Background()))

	store := NewNeo4jExecutionGraphStore(mockClient)

	event := MissionStartEvent{
		MissionID:   types.NewID(),
		MissionName: "test-mission",
		TraceID:     "trace-123",
		StartedAt:   time.Now(),
	}

	err := store.RecordMissionStart(context.Background(), event)
	assert.NoError(t, err, "RecordMissionStart should not return error even if write fails")
}

func TestNeo4jExecutionGraphStore_RecordMissionComplete(t *testing.T) {
	mockClient := graph.NewMockGraphClient()
	require.NoError(t, mockClient.Connect(context.Background()))

	store := NewNeo4jExecutionGraphStore(mockClient)

	event := MissionCompleteEvent{
		MissionID:     types.NewID(),
		Status:        "success",
		FindingsCount: 5,
		TraceID:       "trace-123",
		CompletedAt:   time.Now(),
		DurationMs:    5000,
	}

	err := store.RecordMissionComplete(context.Background(), event)
	assert.NoError(t, err, "RecordMissionComplete should not return error even if write fails")
}

func TestNeo4jExecutionGraphStore_RecordAgentStart(t *testing.T) {
	mockClient := graph.NewMockGraphClient()
	require.NoError(t, mockClient.Connect(context.Background()))

	store := NewNeo4jExecutionGraphStore(mockClient)

	event := AgentStartEvent{
		AgentName: "test-agent",
		TaskID:    "task-1",
		MissionID: types.NewID(),
		TraceID:   "trace-123",
		SpanID:    "span-456",
		StartedAt: time.Now(),
	}

	err := store.RecordAgentStart(context.Background(), event)
	assert.NoError(t, err, "RecordAgentStart should not return error even if write fails")
}

func TestNeo4jExecutionGraphStore_RecordAgentComplete(t *testing.T) {
	mockClient := graph.NewMockGraphClient()
	require.NoError(t, mockClient.Connect(context.Background()))

	store := NewNeo4jExecutionGraphStore(mockClient)

	event := AgentCompleteEvent{
		AgentName:     "test-agent",
		TaskID:        "task-1",
		MissionID:     types.NewID(),
		Status:        "success",
		FindingsCount: 3,
		TraceID:       "trace-123",
		SpanID:        "span-456",
		CompletedAt:   time.Now(),
		DurationMs:    2000,
	}

	err := store.RecordAgentComplete(context.Background(), event)
	assert.NoError(t, err, "RecordAgentComplete should not return error even if write fails")
}

func TestNeo4jExecutionGraphStore_RecordLLMCall(t *testing.T) {
	mockClient := graph.NewMockGraphClient()
	require.NoError(t, mockClient.Connect(context.Background()))

	store := NewNeo4jExecutionGraphStore(mockClient)

	event := LLMCallEvent{
		AgentName: "test-agent",
		MissionID: types.NewID(),
		Model:     "gpt-4",
		Provider:  "openai",
		Slot:      "primary",
		TokensIn:  100,
		TokensOut: 200,
		CostUSD:   0.05,
		LatencyMs: 1500,
		TraceID:   "trace-123",
		SpanID:    "span-789",
		Timestamp: time.Now(),
	}

	err := store.RecordLLMCall(context.Background(), event)
	assert.NoError(t, err, "RecordLLMCall should not return error even if write fails")
}

func TestNeo4jExecutionGraphStore_RecordToolCall(t *testing.T) {
	mockClient := graph.NewMockGraphClient()
	require.NoError(t, mockClient.Connect(context.Background()))

	store := NewNeo4jExecutionGraphStore(mockClient)

	event := ToolCallEvent{
		AgentName:  "test-agent",
		MissionID:  types.NewID(),
		ToolName:   "http-client",
		DurationMs: 500,
		Success:    true,
		Error:      "",
		TraceID:    "trace-123",
		SpanID:     "span-101",
		Timestamp:  time.Now(),
	}

	err := store.RecordToolCall(context.Background(), event)
	assert.NoError(t, err, "RecordToolCall should not return error even if write fails")
}

func TestNeo4jExecutionGraphStore_RecordDelegation(t *testing.T) {
	mockClient := graph.NewMockGraphClient()
	require.NoError(t, mockClient.Connect(context.Background()))

	store := NewNeo4jExecutionGraphStore(mockClient)

	event := DelegationEvent{
		FromAgent: "agent-1",
		ToAgent:   "agent-2",
		TaskID:    "task-1",
		MissionID: types.NewID(),
		TraceID:   "trace-123",
		SpanID:    "span-202",
		Timestamp: time.Now(),
	}

	err := store.RecordDelegation(context.Background(), event)
	assert.NoError(t, err, "RecordDelegation should not return error even if write fails")
}

func TestNeo4jExecutionGraphStore_Close(t *testing.T) {
	mockClient := graph.NewMockGraphClient()
	store := NewNeo4jExecutionGraphStore(mockClient)

	err := store.Close()
	assert.NoError(t, err, "Close should always succeed (no-op)")
}

func TestNeo4jExecutionGraphStore_InterfaceCompliance(t *testing.T) {
	// Verify that Neo4jExecutionGraphStore implements ExecutionGraphStore
	var _ ExecutionGraphStore = (*Neo4jExecutionGraphStore)(nil)
}

// TestNeo4jExecutionGraphStore_GracefulErrorHandling verifies that the store
// gracefully handles errors without failing the mission
func TestNeo4jExecutionGraphStore_GracefulErrorHandling(t *testing.T) {
	// Create a client that will fail writes
	mockClient := graph.NewMockGraphClient()
	// Don't connect - this will cause writes to fail

	store := NewNeo4jExecutionGraphStore(mockClient)

	// All operations should log warnings but not return errors
	event := MissionStartEvent{
		MissionID:   types.NewID(),
		MissionName: "test-mission",
		TraceID:     "trace-123",
		StartedAt:   time.Now(),
	}

	err := store.RecordMissionStart(context.Background(), event)
	assert.NoError(t, err, "Should handle errors gracefully and not fail the mission")
}
