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

// TestNeo4jExecutionGraphStore_GraphStructure_LLMCallWithoutAgent tests that
// LLMCall nodes are created even when AgentExecution doesn't exist (graceful degradation)
func TestNeo4jExecutionGraphStore_GraphStructure_LLMCallWithoutAgent(t *testing.T) {
	mockClient := graph.NewMockGraphClient()
	require.NoError(t, mockClient.Connect(context.Background()))

	store := NewNeo4jExecutionGraphStore(mockClient)

	missionID := types.NewID()

	// Record an LLM call WITHOUT first creating an AgentExecution
	llmEvent := LLMCallEvent{
		AgentName: "test-agent",
		MissionID: missionID,
		Model:     "gpt-4",
		Provider:  "openai",
		Slot:      "primary",
		TokensIn:  100,
		TokensOut: 200,
		CostUSD:   0.05,
		LatencyMs: 1500,
		TraceID:   "trace-123",
		SpanID:    "span-llm-1",
		Timestamp: time.Now(),
	}

	err := store.RecordLLMCall(context.Background(), llmEvent)
	assert.NoError(t, err, "RecordLLMCall should succeed even without AgentExecution")

	// Verify the LLMCall node was created (graceful degradation)
	writeCalls := mockClient.GetCallsByMethod("ExecuteWrite")
	require.GreaterOrEqual(t, len(writeCalls), 1, "ExecuteWrite should have been called")

	// Verify that the query creates the LLMCall node
	lastCall := writeCalls[len(writeCalls)-1]
	require.Len(t, lastCall.Args, 2, "ExecuteWrite should have cypher and params")

	cypher, ok := lastCall.Args[0].(string)
	require.True(t, ok, "First arg should be cypher string")
	assert.Contains(t, cypher, "MERGE (l:LLMCall", "Should create LLMCall node")
	assert.Contains(t, cypher, "OPTIONAL MATCH (a:AgentExecution", "Should use OPTIONAL MATCH for graceful degradation")
}

// TestNeo4jExecutionGraphStore_GraphStructure_ToolCallWithoutAgent tests that
// ToolCall nodes are created even when AgentExecution doesn't exist (graceful degradation)
func TestNeo4jExecutionGraphStore_GraphStructure_ToolCallWithoutAgent(t *testing.T) {
	mockClient := graph.NewMockGraphClient()
	require.NoError(t, mockClient.Connect(context.Background()))

	store := NewNeo4jExecutionGraphStore(mockClient)

	missionID := types.NewID()

	// Record a tool call WITHOUT first creating an AgentExecution
	toolEvent := ToolCallEvent{
		AgentName:  "test-agent",
		MissionID:  missionID,
		ToolName:   "http-client",
		DurationMs: 500,
		Success:    true,
		Error:      "",
		TraceID:    "trace-123",
		SpanID:     "span-tool-1",
		Timestamp:  time.Now(),
	}

	err := store.RecordToolCall(context.Background(), toolEvent)
	assert.NoError(t, err, "RecordToolCall should succeed even without AgentExecution")

	// Verify the ToolCall node was created (graceful degradation)
	writeCalls := mockClient.GetCallsByMethod("ExecuteWrite")
	require.GreaterOrEqual(t, len(writeCalls), 1, "ExecuteWrite should have been called")

	// Verify that the query creates the ToolCall node
	lastCall := writeCalls[len(writeCalls)-1]
	require.Len(t, lastCall.Args, 2, "ExecuteWrite should have cypher and params")

	cypher, ok := lastCall.Args[0].(string)
	require.True(t, ok, "First arg should be cypher string")
	assert.Contains(t, cypher, "MERGE (t:ToolCall", "Should create ToolCall node")
	assert.Contains(t, cypher, "OPTIONAL MATCH (a:AgentExecution", "Should use OPTIONAL MATCH for graceful degradation")
}

// TestNeo4jExecutionGraphStore_GraphStructure_AgentWithoutMission tests that
// AgentExecution nodes are created even when Mission doesn't exist (graceful degradation)
func TestNeo4jExecutionGraphStore_GraphStructure_AgentWithoutMission(t *testing.T) {
	mockClient := graph.NewMockGraphClient()
	require.NoError(t, mockClient.Connect(context.Background()))

	store := NewNeo4jExecutionGraphStore(mockClient)

	missionID := types.NewID()

	// Record agent start WITHOUT first creating a Mission
	agentEvent := AgentStartEvent{
		AgentName: "test-agent",
		TaskID:    "task-1",
		MissionID: missionID,
		TraceID:   "trace-123",
		SpanID:    "span-agent-1",
		StartedAt: time.Now(),
	}

	err := store.RecordAgentStart(context.Background(), agentEvent)
	assert.NoError(t, err, "RecordAgentStart should succeed even without Mission")

	// Verify the AgentExecution node was created (graceful degradation)
	writeCalls := mockClient.GetCallsByMethod("ExecuteWrite")
	require.GreaterOrEqual(t, len(writeCalls), 1, "ExecuteWrite should have been called")

	// Verify that the query creates the AgentExecution node
	lastCall := writeCalls[len(writeCalls)-1]
	require.Len(t, lastCall.Args, 2, "ExecuteWrite should have cypher and params")

	cypher, ok := lastCall.Args[0].(string)
	require.True(t, ok, "First arg should be cypher string")
	assert.Contains(t, cypher, "MERGE (a:AgentExecution", "Should create AgentExecution node")
	assert.Contains(t, cypher, "OPTIONAL MATCH (m:Mission", "Should use OPTIONAL MATCH for graceful degradation")
}

// TestNeo4jExecutionGraphStore_GraphStructure_CompleteGraph tests that when all
// nodes exist, relationships are properly created
func TestNeo4jExecutionGraphStore_GraphStructure_CompleteGraph(t *testing.T) {
	mockClient := graph.NewMockGraphClient()
	require.NoError(t, mockClient.Connect(context.Background()))

	store := NewNeo4jExecutionGraphStore(mockClient)

	missionID := types.NewID()
	now := time.Now()

	// Step 1: Create Mission node
	missionEvent := MissionStartEvent{
		MissionID:   missionID,
		MissionName: "test-mission",
		TraceID:     "trace-123",
		StartedAt:   now,
	}
	err := store.RecordMissionStart(context.Background(), missionEvent)
	require.NoError(t, err)

	// Step 2: Create AgentExecution node (should link to Mission)
	agentEvent := AgentStartEvent{
		AgentName: "test-agent",
		TaskID:    "task-1",
		MissionID: missionID,
		TraceID:   "trace-123",
		SpanID:    "span-agent-1",
		StartedAt: now.Add(time.Second),
	}
	err = store.RecordAgentStart(context.Background(), agentEvent)
	require.NoError(t, err)

	// Step 3: Create LLMCall node (should link to AgentExecution)
	llmEvent := LLMCallEvent{
		AgentName: "test-agent",
		MissionID: missionID,
		Model:     "gpt-4",
		Provider:  "openai",
		Slot:      "primary",
		TokensIn:  100,
		TokensOut: 200,
		CostUSD:   0.05,
		LatencyMs: 1500,
		TraceID:   "trace-123",
		SpanID:    "span-llm-1",
		Timestamp: now.Add(2 * time.Second),
	}
	err = store.RecordLLMCall(context.Background(), llmEvent)
	require.NoError(t, err)

	// Step 4: Create ToolCall node (should link to AgentExecution)
	toolEvent := ToolCallEvent{
		AgentName:  "test-agent",
		MissionID:  missionID,
		ToolName:   "http-client",
		DurationMs: 500,
		Success:    true,
		Error:      "",
		TraceID:    "trace-123",
		SpanID:     "span-tool-1",
		Timestamp:  now.Add(3 * time.Second),
	}
	err = store.RecordToolCall(context.Background(), toolEvent)
	require.NoError(t, err)

	// Verify all write operations were executed
	writeCalls := mockClient.GetCallsByMethod("ExecuteWrite")
	require.Equal(t, 4, len(writeCalls), "Should have 4 write operations")

	// Verify Mission creation
	missionCall := writeCalls[0]
	missionCypher, ok := missionCall.Args[0].(string)
	require.True(t, ok)
	assert.Contains(t, missionCypher, "MERGE (m:Mission", "Mission node should be created")

	// Verify AgentExecution creation and Mission relationship
	agentCall := writeCalls[1]
	agentCypher, ok := agentCall.Args[0].(string)
	require.True(t, ok)
	assert.Contains(t, agentCypher, "MERGE (a:AgentExecution", "AgentExecution node should be created")
	assert.Contains(t, agentCypher, "OPTIONAL MATCH (m:Mission", "Should check for Mission")
	assert.Contains(t, agentCypher, "MERGE (m)-[:HAS_EXECUTION]->(a)", "Should create HAS_EXECUTION relationship")

	// Verify LLMCall creation and AgentExecution relationship
	llmCall := writeCalls[2]
	llmCypher, ok := llmCall.Args[0].(string)
	require.True(t, ok)
	assert.Contains(t, llmCypher, "MERGE (l:LLMCall", "LLMCall node should be created")
	assert.Contains(t, llmCypher, "OPTIONAL MATCH (a:AgentExecution", "Should check for AgentExecution")
	assert.Contains(t, llmCypher, "MERGE (a)-[:MADE_LLM_CALL]->(l)", "Should create MADE_LLM_CALL relationship")

	// Verify ToolCall creation and AgentExecution relationship
	toolCall := writeCalls[3]
	toolCypher, ok := toolCall.Args[0].(string)
	require.True(t, ok)
	assert.Contains(t, toolCypher, "MERGE (t:ToolCall", "ToolCall node should be created")
	assert.Contains(t, toolCypher, "OPTIONAL MATCH (a:AgentExecution", "Should check for AgentExecution")
	assert.Contains(t, toolCypher, "MERGE (a)-[:CALLED_TOOL]->(t)", "Should create CALLED_TOOL relationship")
}

// TestNeo4jExecutionGraphStore_GraphStructure_RelationshipTypes verifies that
// all relationship types are correctly defined in the Cypher queries
func TestNeo4jExecutionGraphStore_GraphStructure_RelationshipTypes(t *testing.T) {
	mockClient := graph.NewMockGraphClient()
	require.NoError(t, mockClient.Connect(context.Background()))

	store := NewNeo4jExecutionGraphStore(mockClient)

	missionID := types.NewID()
	now := time.Now()

	tests := []struct {
		name                string
		operation           func() error
		expectedRelType     string
		expectedDescription string
	}{
		{
			name: "Mission to AgentExecution relationship",
			operation: func() error {
				return store.RecordAgentStart(context.Background(), AgentStartEvent{
					AgentName: "agent-1",
					TaskID:    "task-1",
					MissionID: missionID,
					TraceID:   "trace-123",
					SpanID:    "span-1",
					StartedAt: now,
				})
			},
			expectedRelType:     "HAS_EXECUTION",
			expectedDescription: "Mission -[:HAS_EXECUTION]-> AgentExecution",
		},
		{
			name: "AgentExecution to LLMCall relationship",
			operation: func() error {
				return store.RecordLLMCall(context.Background(), LLMCallEvent{
					AgentName: "agent-1",
					MissionID: missionID,
					Model:     "gpt-4",
					Provider:  "openai",
					TraceID:   "trace-123",
					SpanID:    "span-llm-1",
					Timestamp: now,
				})
			},
			expectedRelType:     "MADE_LLM_CALL",
			expectedDescription: "AgentExecution -[:MADE_LLM_CALL]-> LLMCall",
		},
		{
			name: "AgentExecution to ToolCall relationship",
			operation: func() error {
				return store.RecordToolCall(context.Background(), ToolCallEvent{
					AgentName:  "agent-1",
					MissionID:  missionID,
					ToolName:   "http-client",
					DurationMs: 100,
					Success:    true,
					TraceID:    "trace-123",
					SpanID:     "span-tool-1",
					Timestamp:  now,
				})
			},
			expectedRelType:     "CALLED_TOOL",
			expectedDescription: "AgentExecution -[:CALLED_TOOL]-> ToolCall",
		},
		{
			name: "AgentExecution to AgentExecution delegation",
			operation: func() error {
				return store.RecordDelegation(context.Background(), DelegationEvent{
					FromAgent: "agent-1",
					ToAgent:   "agent-2",
					TaskID:    "task-2",
					MissionID: missionID,
					TraceID:   "trace-123",
					SpanID:    "span-delegation",
					Timestamp: now,
				})
			},
			expectedRelType:     "DELEGATED_TO",
			expectedDescription: "AgentExecution -[:DELEGATED_TO]-> AgentExecution",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock to clear previous calls
			mockClient.Reset()
			require.NoError(t, mockClient.Connect(context.Background()))

			// Execute the operation
			err := tt.operation()
			require.NoError(t, err)

			// Get the write call
			writeCalls := mockClient.GetCallsByMethod("ExecuteWrite")
			require.Equal(t, 1, len(writeCalls), "Should have exactly one write call")

			// Verify the relationship type is in the Cypher query
			cypher, ok := writeCalls[0].Args[0].(string)
			require.True(t, ok, "First arg should be cypher string")
			assert.Contains(t, cypher, tt.expectedRelType,
				"Cypher should contain relationship type %s for %s", tt.expectedRelType, tt.expectedDescription)
		})
	}
}

// TestNeo4jExecutionGraphStore_GraphStructure_OrderingIndependence verifies that
// the graph structure is resilient to events arriving out of order
func TestNeo4jExecutionGraphStore_GraphStructure_OrderingIndependence(t *testing.T) {
	testCases := []struct {
		name        string
		operations  []func(*Neo4jExecutionGraphStore, types.ID) error
		description string
	}{
		{
			name: "LLMCall before AgentExecution",
			operations: []func(*Neo4jExecutionGraphStore, types.ID) error{
				func(store *Neo4jExecutionGraphStore, missionID types.ID) error {
					return store.RecordLLMCall(context.Background(), LLMCallEvent{
						AgentName: "test-agent",
						MissionID: missionID,
						Model:     "gpt-4",
						Provider:  "openai",
						TraceID:   "trace-123",
						SpanID:    "span-llm-1",
						Timestamp: time.Now(),
					})
				},
				func(store *Neo4jExecutionGraphStore, missionID types.ID) error {
					return store.RecordAgentStart(context.Background(), AgentStartEvent{
						AgentName: "test-agent",
						TaskID:    "task-1",
						MissionID: missionID,
						TraceID:   "trace-123",
						SpanID:    "span-agent-1",
						StartedAt: time.Now(),
					})
				},
			},
			description: "LLMCall should be created even if AgentExecution arrives later",
		},
		{
			name: "ToolCall before AgentExecution",
			operations: []func(*Neo4jExecutionGraphStore, types.ID) error{
				func(store *Neo4jExecutionGraphStore, missionID types.ID) error {
					return store.RecordToolCall(context.Background(), ToolCallEvent{
						AgentName:  "test-agent",
						MissionID:  missionID,
						ToolName:   "http-client",
						DurationMs: 100,
						Success:    true,
						TraceID:    "trace-123",
						SpanID:     "span-tool-1",
						Timestamp:  time.Now(),
					})
				},
				func(store *Neo4jExecutionGraphStore, missionID types.ID) error {
					return store.RecordAgentStart(context.Background(), AgentStartEvent{
						AgentName: "test-agent",
						TaskID:    "task-1",
						MissionID: missionID,
						TraceID:   "trace-123",
						SpanID:    "span-agent-1",
						StartedAt: time.Now(),
					})
				},
			},
			description: "ToolCall should be created even if AgentExecution arrives later",
		},
		{
			name: "AgentExecution before Mission",
			operations: []func(*Neo4jExecutionGraphStore, types.ID) error{
				func(store *Neo4jExecutionGraphStore, missionID types.ID) error {
					return store.RecordAgentStart(context.Background(), AgentStartEvent{
						AgentName: "test-agent",
						TaskID:    "task-1",
						MissionID: missionID,
						TraceID:   "trace-123",
						SpanID:    "span-agent-1",
						StartedAt: time.Now(),
					})
				},
				func(store *Neo4jExecutionGraphStore, missionID types.ID) error {
					return store.RecordMissionStart(context.Background(), MissionStartEvent{
						MissionID:   missionID,
						MissionName: "test-mission",
						TraceID:     "trace-123",
						StartedAt:   time.Now(),
					})
				},
			},
			description: "AgentExecution should be created even if Mission arrives later",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := graph.NewMockGraphClient()
			require.NoError(t, mockClient.Connect(context.Background()))

			store := NewNeo4jExecutionGraphStore(mockClient)
			missionID := types.NewID()

			// Execute operations in the specified order
			for i, op := range tc.operations {
				err := op(store, missionID)
				assert.NoError(t, err, "Operation %d should succeed: %s", i, tc.description)
			}

			// Verify all operations completed successfully
			writeCalls := mockClient.GetCallsByMethod("ExecuteWrite")
			assert.Equal(t, len(tc.operations), len(writeCalls),
				"All operations should have executed writes")
		})
	}
}
