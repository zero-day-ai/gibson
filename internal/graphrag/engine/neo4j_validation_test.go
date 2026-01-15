//go:build integration

package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/graphrag/taxonomy"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestNeo4jGraphCreation validates that the TaxonomyGraphEngine can create
// nodes and relationships in Neo4j correctly.
func TestNeo4jGraphCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Connect to Neo4j
	graphClient, err := graph.NewNeo4jClient(graph.GraphClientConfig{
		URI:                     getEnvOrDefault("NEO4J_URI", "bolt://localhost:7687"),
		Username:                getEnvOrDefault("NEO4J_USER", "neo4j"),
		Password:                getEnvOrDefault("NEO4J_PASSWORD", "gibson-dev-2024"),
		MaxConnectionPoolSize:   10,
		ConnectionTimeout:       10 * time.Second,
		MaxTransactionRetryTime: 30 * time.Second,
	})
	require.NoError(t, err, "Failed to create Neo4j client")

	err = graphClient.Connect(ctx)
	require.NoError(t, err, "Failed to connect to Neo4j")
	defer graphClient.Close(ctx)

	health := graphClient.Health(ctx)
	require.Equal(t, types.HealthStateHealthy, health.State, "Neo4j is not healthy: %s", health.Message)

	// Load taxonomy and create engine
	loader := taxonomy.NewTaxonomyLoader()
	tax, err := loader.Load()
	require.NoError(t, err, "Failed to load taxonomy")

	registry, err := taxonomy.NewTaxonomyRegistry(tax)
	require.NoError(t, err, "Failed to create taxonomy registry")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	engine := NewTaxonomyGraphEngine(registry, graphClient, logger)

	// Test data with unique IDs
	testID := fmt.Sprintf("validation-test-%d", time.Now().UnixNano())
	missionID := "mission-" + testID
	traceID := "trace-" + testID
	agentSpanID := "agent-span-" + testID
	toolSpanID := "tool-span-" + testID
	timestamp := time.Now().Unix()

	// Cleanup first
	_, _ = graphClient.Query(ctx, `
		MATCH (n) WHERE n.trace_id STARTS WITH 'trace-validation-test' DETACH DELETE n
	`, nil)
	_, _ = graphClient.Query(ctx, `
		MATCH (n:Mission) WHERE n.id STARTS WITH 'mission:mission-validation-test' DETACH DELETE n
	`, nil)

	t.Run("Mission Creation", func(t *testing.T) {
		// Create mission using the event type from execution_events.yaml
		err := engine.HandleEvent(ctx, "mission.started", map[string]any{
			"mission_id": missionID,
			"trace_id":   traceID,
			"timestamp":  timestamp,
		})
		require.NoError(t, err, "HandleEvent mission.started failed")

		// Verify node exists
		result, err := graphClient.Query(ctx, `
			MATCH (m:Mission) WHERE m.trace_id = $trace_id
			RETURN m.name as name, m.status as status
		`, map[string]any{"trace_id": traceID})
		require.NoError(t, err)
		require.Len(t, result.Records, 1, "Expected 1 Mission node")
		t.Logf("✅ Mission created: name=%v, status=%v", result.Records[0]["name"], result.Records[0]["status"])
	})

	t.Run("AgentRun Creation with PART_OF", func(t *testing.T) {
		// Create agent run which should link to mission via PART_OF
		err := engine.HandleEvent(ctx, "agent.started", map[string]any{
			"agent_name": "test-agent",
			"mission_id": missionID,
			"trace_id":   traceID,
			"span_id":    agentSpanID,
			"timestamp":  timestamp,
		})
		require.NoError(t, err, "HandleEvent agent.started failed")

		// Verify AgentRun exists with PART_OF relationship
		result, err := graphClient.Query(ctx, `
			MATCH (a:AgentRun {span_id: $span_id})-[:PART_OF]->(m:Mission)
			RETURN a.agent_name as agent_name, m.name as mission_name
		`, map[string]any{"span_id": agentSpanID})
		require.NoError(t, err)
		require.Len(t, result.Records, 1, "Expected AgentRun with PART_OF relationship to Mission")
		t.Logf("✅ AgentRun created with PART_OF: agent=%v -> mission=%v",
			result.Records[0]["agent_name"], result.Records[0]["mission_name"])
	})

	t.Run("ToolExecution Creation with EXECUTED_BY", func(t *testing.T) {
		// Create tool execution linked to agent via EXECUTED_BY
		err := engine.HandleEvent(ctx, "tool.call.started", map[string]any{
			"tool_name":      "nmap",
			"trace_id":       traceID,
			"span_id":        toolSpanID,
			"parent_span_id": agentSpanID,
			"timestamp":      timestamp,
		})
		require.NoError(t, err, "HandleEvent tool.call.started failed")

		// Verify ToolExecution exists with EXECUTED_BY relationship
		result, err := graphClient.Query(ctx, `
			MATCH (t:ToolExecution {span_id: $span_id})-[:EXECUTED_BY]->(a:AgentRun)
			RETURN t.tool_name as tool_name, a.agent_name as agent_name
		`, map[string]any{"span_id": toolSpanID})
		require.NoError(t, err)
		require.Len(t, result.Records, 1, "Expected ToolExecution with EXECUTED_BY relationship")
		t.Logf("✅ ToolExecution created with EXECUTED_BY: tool=%v -> agent=%v",
			result.Records[0]["tool_name"], result.Records[0]["agent_name"])
	})

	t.Run("Verify Graph Structure", func(t *testing.T) {
		// Query the full execution graph
		result, err := graphClient.Query(ctx, `
			MATCH path = (m:Mission)<-[:PART_OF]-(a:AgentRun)<-[:EXECUTED_BY]-(t:ToolExecution)
			WHERE m.trace_id = $trace_id
			RETURN m.name as mission, a.agent_name as agent, t.tool_name as tool
		`, map[string]any{"trace_id": traceID})
		require.NoError(t, err)
		require.Len(t, result.Records, 1, "Expected complete execution graph")

		record := result.Records[0]
		t.Logf("✅ Complete graph: Mission(%v) <- AgentRun(%v) <- ToolExecution(%v)",
			record["mission"], record["agent"], record["tool"])
	})

	// Cleanup
	_, _ = graphClient.Query(ctx, `
		MATCH (n) WHERE n.trace_id = $trace_id DETACH DELETE n
	`, map[string]any{"trace_id": traceID})
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
