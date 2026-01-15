//go:build integration

package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/daemon/api"
	"github.com/zero-day-ai/gibson/internal/graphrag/engine"
	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/graphrag/taxonomy"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestGraphEventIntegrationFlow tests the full event flow from EventBus → GraphEventSubscriber → TaxonomyGraphEngine → Neo4j.
//
// This integration test requires Neo4j to be running. Start it with:
//   docker-compose -f build/docker-compose.yml up -d neo4j
//
// Run with: go test -tags=integration -v ./internal/daemon -run TestGraphEventIntegrationFlow
func TestGraphEventIntegrationFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create Neo4j client
	graphClient, err := createTestNeo4jClient(t)
	if err != nil {
		t.Skipf("Neo4j not available, skipping test: %v", err)
		return
	}
	defer graphClient.Close(ctx)

	// Verify Neo4j connectivity
	health := graphClient.Health(ctx)
	if health.State != types.HealthStateHealthy {
		t.Skipf("Neo4j is not healthy, skipping test: %s", health.Message)
		return
	}

	// Clean up test data before running tests
	cleanupGraphTestData(t, ctx, graphClient)
	defer cleanupGraphTestData(t, ctx, graphClient)

	// Load taxonomy and create engine
	loader := taxonomy.NewTaxonomyLoader()
	tax, err := loader.Load()
	require.NoError(t, err, "Failed to load taxonomy")

	registry, err := taxonomy.NewTaxonomyRegistry(tax)
	require.NoError(t, err, "Failed to create taxonomy registry")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	taxonomyEngine := engine.NewTaxonomyGraphEngine(registry, graphClient, logger)

	// Create EventBus
	eventBus := NewEventBus(logger)
	defer eventBus.Close()

	// Create and start GraphEventSubscriber
	subscriber := NewGraphEventSubscriber(taxonomyEngine, eventBus, logger)
	subscriber.Start(ctx)
	defer subscriber.Stop()

	// Run test cases
	t.Run("MissionStartedCreatesNode", func(t *testing.T) {
		testMissionStartedCreatesNode(t, ctx, eventBus, graphClient)
	})

	t.Run("AgentRunWithPartOfRelationship", func(t *testing.T) {
		testAgentRunWithPartOfRelationship(t, ctx, eventBus, graphClient)
	})

	t.Run("ToolExecutionWithExecutedByRelationship", func(t *testing.T) {
		testToolExecutionWithExecutedByRelationship(t, ctx, eventBus, graphClient)
	})

	t.Run("GracefulDegradationWhenNeo4jUnavailable", func(t *testing.T) {
		testGracefulDegradationWhenNeo4jUnavailable(t, ctx, eventBus, graphClient, registry, logger)
	})
}

// testMissionStartedCreatesNode verifies that mission.started event creates a Mission node in Neo4j.
//
// Test flow:
// 1. Publish mission.started event to EventBus
// 2. Wait for event processing
// 3. Query Neo4j to verify Mission node was created
// 4. Verify node properties match event data
func testMissionStartedCreatesNode(t *testing.T, ctx context.Context, eventBus *EventBus, graphClient graph.GraphClient) {
	missionID := fmt.Sprintf("test-mission-%d", time.Now().UnixNano())
	traceID := fmt.Sprintf("trace-%d", time.Now().UnixNano())
	timestamp := time.Now()

	// Expected node ID from taxonomy template: "mission:{mission_id}"
	expectedNodeID := "mission:" + missionID

	// Publish mission.started event
	event := api.EventData{
		EventType: EventTypeMissionStarted,
		Timestamp: timestamp,
		Source:    "test",
		MissionEvent: &api.MissionEventData{
			MissionID: missionID,
			Payload: map[string]any{
				"trace_id": traceID,
			},
		},
		Metadata: map[string]any{
			"trace_id": traceID,
		},
	}

	err := eventBus.Publish(ctx, event)
	require.NoError(t, err, "Failed to publish mission.started event")

	// Wait for event processing (subscriber processes asynchronously)
	time.Sleep(500 * time.Millisecond)

	// Query Neo4j to verify Mission node was created
	result, err := graphClient.Query(ctx, `
		MATCH (m:Mission {id: $id})
		RETURN m.id as id, m.name as name, m.trace_id as trace_id, m.status as status
	`, map[string]any{"id": expectedNodeID})
	require.NoError(t, err, "Failed to query Mission node")
	require.Len(t, result.Records, 1, "Expected exactly one Mission node")

	// Verify node properties
	record := result.Records[0]
	assert.Equal(t, expectedNodeID, record["id"], "Mission node ID should match")
	assert.Equal(t, missionID, record["name"], "Mission name should match mission_id")
	assert.Equal(t, traceID, record["trace_id"], "Mission trace_id should match")
	assert.Equal(t, "running", record["status"], "Mission status should be 'running'")

	t.Logf("✓ Mission node created successfully: %s", expectedNodeID)
}

// testAgentRunWithPartOfRelationship verifies that agent.started event creates an AgentRun node
// with a PART_OF relationship to the Mission node.
//
// Test flow:
// 1. Create a mission
// 2. Publish agent.started event with mission_id
// 3. Verify AgentRun node was created
// 4. Verify PART_OF relationship exists between AgentRun and Mission
func testAgentRunWithPartOfRelationship(t *testing.T, ctx context.Context, eventBus *EventBus, graphClient graph.GraphClient) {
	missionID := fmt.Sprintf("test-mission-agent-%d", time.Now().UnixNano())
	traceID := fmt.Sprintf("trace-agent-%d", time.Now().UnixNano())
	spanID := fmt.Sprintf("span-agent-%d", time.Now().UnixNano())
	timestamp := time.Now()

	// Expected node IDs from taxonomy templates
	missionNodeID := "mission:" + missionID
	agentNodeID := "agent_run:" + traceID + ":" + spanID

	// 1. Create mission first
	missionEvent := api.EventData{
		EventType: EventTypeMissionStarted,
		Timestamp: timestamp,
		Source:    "test",
		MissionEvent: &api.MissionEventData{
			MissionID: missionID,
		},
		Metadata: map[string]any{
			"trace_id": traceID,
		},
	}
	err := eventBus.Publish(ctx, missionEvent)
	require.NoError(t, err)
	time.Sleep(300 * time.Millisecond)

	// 2. Publish agent.started event
	agentEvent := api.EventData{
		EventType: EventTypeAgentStarted,
		Timestamp: timestamp,
		Source:    "test",
		AgentEvent: &api.AgentEventData{
			AgentID:   spanID,
			AgentName: "test-agent",
			Metadata: map[string]any{
				"mission_id": missionID,
				"trace_id":   traceID,
				"span_id":    spanID,
			},
		},
		Metadata: map[string]any{
			"trace_id": traceID,
			"span_id":  spanID,
		},
	}
	err = eventBus.Publish(ctx, agentEvent)
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	// 3. Verify AgentRun node and PART_OF relationship
	result, err := graphClient.Query(ctx, `
		MATCH (a:AgentRun {id: $agent_id})-[:PART_OF]->(m:Mission {id: $mission_id})
		RETURN a.id as agent_id, a.agent_name as agent_name, m.id as mission_id
	`, map[string]any{
		"agent_id":   agentNodeID,
		"mission_id": missionNodeID,
	})
	require.NoError(t, err, "Failed to query AgentRun with PART_OF relationship")
	require.Len(t, result.Records, 1, "Expected AgentRun with PART_OF relationship to Mission")

	record := result.Records[0]
	assert.Equal(t, agentNodeID, record["agent_id"], "AgentRun node ID should match")
	assert.Equal(t, "test-agent", record["agent_name"], "Agent name should match")
	assert.Equal(t, missionNodeID, record["mission_id"], "Mission ID should match")

	t.Logf("✓ AgentRun node created with PART_OF relationship: %s -> %s", agentNodeID, missionNodeID)
}

// testToolExecutionWithExecutedByRelationship verifies that tool.call.started event creates
// a ToolExecution node with an EXECUTED_BY relationship to the AgentRun node.
//
// Test flow:
// 1. Create mission and agent
// 2. Publish tool.call.started event with parent_span_id pointing to agent
// 3. Verify ToolExecution node was created
// 4. Verify EXECUTED_BY relationship exists between ToolExecution and AgentRun
func testToolExecutionWithExecutedByRelationship(t *testing.T, ctx context.Context, eventBus *EventBus, graphClient graph.GraphClient) {
	missionID := fmt.Sprintf("test-mission-tool-%d", time.Now().UnixNano())
	traceID := fmt.Sprintf("trace-tool-%d", time.Now().UnixNano())
	agentSpanID := fmt.Sprintf("span-agent-%d", time.Now().UnixNano())
	toolSpanID := fmt.Sprintf("span-tool-%d", time.Now().UnixNano())
	timestamp := time.Now()

	// Expected node IDs
	agentNodeID := "agent_run:" + traceID + ":" + agentSpanID
	toolNodeID := "tool_execution:" + traceID + ":" + toolSpanID

	// 1. Create mission
	missionEvent := api.EventData{
		EventType: EventTypeMissionStarted,
		Timestamp: timestamp,
		Source:    "test",
		MissionEvent: &api.MissionEventData{
			MissionID: missionID,
		},
		Metadata: map[string]any{
			"trace_id": traceID,
		},
	}
	err := eventBus.Publish(ctx, missionEvent)
	require.NoError(t, err)
	time.Sleep(300 * time.Millisecond)

	// 2. Create agent
	agentEvent := api.EventData{
		EventType: EventTypeAgentStarted,
		Timestamp: timestamp,
		Source:    "test",
		AgentEvent: &api.AgentEventData{
			AgentID:   agentSpanID,
			AgentName: "tool-test-agent",
			Metadata: map[string]any{
				"mission_id": missionID,
				"trace_id":   traceID,
				"span_id":    agentSpanID,
			},
		},
		Metadata: map[string]any{
			"trace_id": traceID,
			"span_id":  agentSpanID,
		},
	}
	err = eventBus.Publish(ctx, agentEvent)
	require.NoError(t, err)
	time.Sleep(300 * time.Millisecond)

	// 3. Publish tool.call.started event
	// Note: Tool events don't have a dedicated structure in api.EventData yet,
	// so we use Metadata to pass tool-specific fields
	toolEvent := api.EventData{
		EventType: EventTypeToolCallStarted,
		Timestamp: timestamp,
		Source:    "test",
		Metadata: map[string]any{
			"tool_name":      "nmap",
			"trace_id":       traceID,
			"span_id":        toolSpanID,
			"parent_span_id": agentSpanID,
		},
	}
	err = eventBus.Publish(ctx, toolEvent)
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	// 4. Verify ToolExecution node and EXECUTED_BY relationship
	result, err := graphClient.Query(ctx, `
		MATCH (t:ToolExecution {id: $tool_id})-[:EXECUTED_BY]->(a:AgentRun {id: $agent_id})
		RETURN t.id as tool_id, t.tool_name as tool_name, a.agent_name as agent_name
	`, map[string]any{
		"tool_id":  toolNodeID,
		"agent_id": agentNodeID,
	})
	require.NoError(t, err, "Failed to query ToolExecution with EXECUTED_BY relationship")
	require.Len(t, result.Records, 1, "Expected ToolExecution with EXECUTED_BY relationship")

	record := result.Records[0]
	assert.Equal(t, toolNodeID, record["tool_id"], "ToolExecution node ID should match")
	assert.Equal(t, "nmap", record["tool_name"], "Tool name should match")
	assert.Equal(t, "tool-test-agent", record["agent_name"], "Agent name should match")

	t.Logf("✓ ToolExecution node created with EXECUTED_BY relationship: %s -> %s", toolNodeID, agentNodeID)
}

// testGracefulDegradationWhenNeo4jUnavailable verifies that the system continues to operate
// even when Neo4j is unavailable or returns errors.
//
// Test flow:
// 1. Create a mock graph client that always returns errors
// 2. Create a new subscriber with the failing client
// 3. Publish events through EventBus
// 4. Verify that events are processed without propagating errors
// 5. Verify that mission execution would continue (event bus doesn't block)
func testGracefulDegradationWhenNeo4jUnavailable(
	t *testing.T,
	ctx context.Context,
	eventBus *EventBus,
	graphClient graph.GraphClient,
	registry taxonomy.TaxonomyRegistry,
	logger *slog.Logger,
) {
	// Create a failing graph client
	failingClient := &failingGraphClient{}

	// Create engine with failing client
	failingEngine := engine.NewTaxonomyGraphEngine(registry, failingClient, logger)

	// Create a new event bus for this test to avoid interference
	testEventBus := NewEventBus(logger)
	defer testEventBus.Close()

	// Create subscriber with failing engine
	failingSubscriber := NewGraphEventSubscriber(failingEngine, testEventBus, logger)
	failingSubscriber.Start(ctx)
	defer failingSubscriber.Stop()

	// Publish events - these should be processed without errors propagating back
	missionID := fmt.Sprintf("test-mission-degraded-%d", time.Now().UnixNano())
	event := api.EventData{
		EventType: EventTypeMissionStarted,
		Timestamp: time.Now(),
		Source:    "test",
		MissionEvent: &api.MissionEventData{
			MissionID: missionID,
		},
	}

	// This should not return an error - events are published successfully
	err := testEventBus.Publish(ctx, event)
	assert.NoError(t, err, "Event should be published successfully even with failing graph client")

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Verify that the event bus is still operational
	// (can accept subscriptions and publish events)
	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()

	eventChan, cleanup := testEventBus.Subscribe(subCtx, []string{EventTypeMissionStarted}, "")
	defer cleanup()

	// Publish another event
	event2 := api.EventData{
		EventType: EventTypeMissionStarted,
		Timestamp: time.Now(),
		Source:    "test",
		MissionEvent: &api.MissionEventData{
			MissionID: missionID + "-2",
		},
	}
	err = testEventBus.Publish(ctx, event2)
	require.NoError(t, err)

	// Verify event was received by subscriber (event bus still works)
	select {
	case receivedEvent := <-eventChan:
		assert.Equal(t, EventTypeMissionStarted, receivedEvent.EventType)
		assert.Equal(t, missionID+"-2", receivedEvent.MissionEvent.MissionID)
		t.Logf("✓ Event bus continues to operate despite graph client failures")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for event - event bus may have blocked")
	}

	t.Logf("✓ Graceful degradation verified: mission execution continues when Neo4j unavailable")
}

// createTestNeo4jClient creates a Neo4j client for integration testing.
func createTestNeo4jClient(t *testing.T) (graph.GraphClient, error) {
	client, err := graph.NewNeo4jClient(graph.GraphClientConfig{
		URI:                     getEnvOrDefault("NEO4J_URI", "bolt://localhost:7687"),
		Username:                getEnvOrDefault("NEO4J_USER", "neo4j"),
		Password:                getEnvOrDefault("NEO4J_PASSWORD", "gibson-dev-2024"),
		MaxConnectionPoolSize:   10,
		ConnectionTimeout:       10 * time.Second,
		MaxTransactionRetryTime: 30 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		return nil, err
	}

	return client, nil
}

// cleanupGraphTestData removes test data from Neo4j.
func cleanupGraphTestData(t *testing.T, ctx context.Context, client graph.GraphClient) {
	// Delete all test nodes (nodes with IDs starting with test patterns)
	_, err := client.Query(ctx, `
		MATCH (n)
		WHERE n.id STARTS WITH 'mission:test-'
		   OR n.id STARTS WITH 'agent_run:trace-'
		   OR n.id STARTS WITH 'tool_execution:trace-'
		DETACH DELETE n
	`, nil)
	if err != nil {
		t.Logf("Warning: Failed to clean up test data: %v", err)
	}
}

// getEnvOrDefault returns the value of an environment variable or a default value.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// failingGraphClient is a mock graph client that always returns errors.
type failingGraphClient struct{}

func (f *failingGraphClient) Connect(ctx context.Context) error {
	return fmt.Errorf("mock error: connection failed")
}

func (f *failingGraphClient) Close(ctx context.Context) error {
	return nil
}

func (f *failingGraphClient) Query(ctx context.Context, cypher string, params map[string]any) (graph.QueryResult, error) {
	return graph.QueryResult{}, fmt.Errorf("mock error: query execution failed")
}

func (f *failingGraphClient) Health(ctx context.Context) types.HealthStatus {
	return types.HealthStatus{
		State:     types.HealthStateUnhealthy,
		Message:   "mock error: neo4j unavailable",
		CheckedAt: time.Now(),
	}
}

func (f *failingGraphClient) CreateNode(ctx context.Context, labels []string, props map[string]any) (string, error) {
	return "", fmt.Errorf("mock error: create node failed")
}

func (f *failingGraphClient) CreateRelationship(ctx context.Context, fromID, toID, relType string, props map[string]any) error {
	return fmt.Errorf("mock error: create relationship failed")
}

func (f *failingGraphClient) DeleteNode(ctx context.Context, nodeID string) error {
	return fmt.Errorf("mock error: delete node failed")
}
