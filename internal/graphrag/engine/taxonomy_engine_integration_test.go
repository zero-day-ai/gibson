//go:build integration

package engine

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/graphrag/taxonomy"
)

// TestTaxonomyEngineIntegration tests the full event flow with a real Neo4j database.
// This test requires Neo4j to be running. Start it with:
//   docker-compose -f build/docker-compose.yml up -d neo4j
//
// Set NEO4J_URI environment variable to override the default connection:
//   export NEO4J_URI="neo4j://localhost:7687"
func TestTaxonomyEngineIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Get Neo4j connection details from environment
	neo4jURI := os.Getenv("NEO4J_URI")
	if neo4jURI == "" {
		neo4jURI = "neo4j://localhost:7687"
	}

	neo4jUser := os.Getenv("NEO4J_USER")
	if neo4jUser == "" {
		neo4jUser = "neo4j"
	}

	neo4jPassword := os.Getenv("NEO4J_PASSWORD")
	if neo4jPassword == "" {
		neo4jPassword = "testpassword"
	}

	ctx := context.Background()

	// Create Neo4j client
	graphClient, err := graph.NewNeo4jClient(ctx, graph.Neo4jConfig{
		URI:      neo4jURI,
		Username: neo4jUser,
		Password: neo4jPassword,
	})
	require.NoError(t, err, "Failed to create Neo4j client")
	defer graphClient.Close(ctx)

	// Verify Neo4j connectivity
	health := graphClient.Health(ctx)
	require.True(t, health.Healthy, "Neo4j is not healthy: %s", health.Message)

	// Load taxonomy from embedded files
	loader, err := taxonomy.NewLoader()
	require.NoError(t, err, "Failed to create taxonomy loader")

	registry, err := loader.Load()
	require.NoError(t, err, "Failed to load taxonomy")

	// Create taxonomy engine
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	engine := NewTaxonomyGraphEngine(registry, graphClient, logger)

	// Clean up test data before running tests
	cleanupTestData(t, ctx, graphClient)

	t.Run("Mission Lifecycle", func(t *testing.T) {
		testMissionLifecycle(t, ctx, engine, graphClient)
	})

	t.Run("Agent Execution Flow", func(t *testing.T) {
		testAgentExecutionFlow(t, ctx, engine, graphClient)
	})

	t.Run("LLM Call Tracking", func(t *testing.T) {
		testLLMCallTracking(t, ctx, engine, graphClient)
	})

	t.Run("Tool Execution Tracking", func(t *testing.T) {
		testToolExecutionTracking(t, ctx, engine, graphClient)
	})

	t.Run("Agent Delegation", func(t *testing.T) {
		testAgentDelegation(t, ctx, engine, graphClient)
	})

	// Clean up test data after tests
	cleanupTestData(t, ctx, graphClient)
}

// testMissionLifecycle tests mission.started and mission.completed events
func testMissionLifecycle(t *testing.T, ctx context.Context, engine TaxonomyGraphEngine, graphClient graph.GraphClient) {
	missionID := "test-mission-" + time.Now().Format("20060102-150405")
	traceID := "trace-" + missionID

	// Test mission.started event
	t.Run("MissionStarted", func(t *testing.T) {
		eventData := map[string]any{
			"mission_id":    missionID,
			"trace_id":      traceID,
			"timestamp":     time.Now().Unix(),
			"workflow_name": "test-workflow",
			"target_id":     "example.com",
			"node_count":    5,
		}

		err := engine.HandleEvent(ctx, "mission.started", eventData)
		require.NoError(t, err, "Failed to handle mission.started event")

		// Verify Mission node was created
		cypher := `MATCH (m:mission {id: $id}) RETURN m`
		params := map[string]any{"id": "mission:" + missionID}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query Mission node")
		require.Len(t, result, 1, "Mission node should exist")

		mission := result[0]["m"].(map[string]any)
		assert.Equal(t, missionID, mission["name"], "Mission name mismatch")
		assert.Equal(t, "running", mission["status"], "Mission status should be running")
		assert.Equal(t, traceID, mission["trace_id"], "Trace ID mismatch")
		assert.Equal(t, "test-workflow", mission["workflow_name"], "Workflow name mismatch")
		assert.Equal(t, "example.com", mission["target"], "Target mismatch")
		assert.Equal(t, int64(5), mission["node_count"], "Node count mismatch")
	})

	// Test mission.completed event
	t.Run("MissionCompleted", func(t *testing.T) {
		eventData := map[string]any{
			"mission_id":     missionID,
			"timestamp":      time.Now().Unix(),
			"duration":       30000,
			"finding_count":  3,
			"nodes_executed": 5,
		}

		err := engine.HandleEvent(ctx, "mission.completed", eventData)
		require.NoError(t, err, "Failed to handle mission.completed event")

		// Verify Mission node was updated
		cypher := `MATCH (m:mission {id: $id}) RETURN m`
		params := map[string]any{"id": "mission:" + missionID}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query Mission node")
		require.Len(t, result, 1, "Mission node should exist")

		mission := result[0]["m"].(map[string]any)
		assert.Equal(t, "completed", mission["status"], "Mission status should be completed")
		assert.Equal(t, int64(30000), mission["duration_ms"], "Duration mismatch")
		assert.Equal(t, int64(3), mission["finding_count"], "Finding count mismatch")
		assert.Equal(t, int64(5), mission["nodes_executed"], "Nodes executed mismatch")
		assert.NotNil(t, mission["ended_at"], "ended_at should be set")
	})
}

// testAgentExecutionFlow tests agent.started, agent.completed events and PART_OF relationship
func testAgentExecutionFlow(t *testing.T, ctx context.Context, engine TaxonomyGraphEngine, graphClient graph.GraphClient) {
	missionID := "test-mission-agent-" + time.Now().Format("20060102-150405")
	traceID := "trace-" + missionID
	spanID := "span-001"

	// Create mission first
	missionData := map[string]any{
		"mission_id": missionID,
		"trace_id":   traceID,
		"timestamp":  time.Now().Unix(),
	}
	err := engine.HandleEvent(ctx, "mission.started", missionData)
	require.NoError(t, err, "Failed to create mission")

	// Test agent.started event
	t.Run("AgentStarted", func(t *testing.T) {
		eventData := map[string]any{
			"agent_name":       "bishop",
			"mission_id":       missionID,
			"trace_id":         traceID,
			"span_id":          spanID,
			"timestamp":        time.Now().Unix(),
			"task_description": "Enumerate subdomains",
			"target_id":        "example.com",
		}

		err := engine.HandleEvent(ctx, "agent.started", eventData)
		require.NoError(t, err, "Failed to handle agent.started event")

		// Verify AgentRun node was created
		agentRunID := "agent_run:" + traceID + ":" + spanID
		cypher := `MATCH (ar:agent_run {id: $id}) RETURN ar`
		params := map[string]any{"id": agentRunID}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query AgentRun node")
		require.Len(t, result, 1, "AgentRun node should exist")

		agentRun := result[0]["ar"].(map[string]any)
		assert.Equal(t, "bishop", agentRun["agent_name"], "Agent name mismatch")
		assert.Equal(t, missionID, agentRun["mission_id"], "Mission ID mismatch")
		assert.Equal(t, "running", agentRun["status"], "Agent status should be running")
		assert.Equal(t, "Enumerate subdomains", agentRun["task_description"], "Task description mismatch")
	})

	// Test PART_OF relationship
	t.Run("PartOfRelationship", func(t *testing.T) {
		agentRunID := "agent_run:" + traceID + ":" + spanID
		missionNodeID := "mission:" + missionID

		cypher := `
			MATCH (ar:agent_run {id: $agent_run_id})-[r:PART_OF]->(m:mission {id: $mission_id})
			RETURN r
		`
		params := map[string]any{
			"agent_run_id": agentRunID,
			"mission_id":   missionNodeID,
		}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query PART_OF relationship")
		require.Len(t, result, 1, "PART_OF relationship should exist")
	})

	// Test agent.completed event
	t.Run("AgentCompleted", func(t *testing.T) {
		eventData := map[string]any{
			"trace_id":      traceID,
			"span_id":       spanID,
			"timestamp":     time.Now().Unix(),
			"duration":      15000,
			"finding_count": 2,
			"success":       true,
		}

		err := engine.HandleEvent(ctx, "agent.completed", eventData)
		require.NoError(t, err, "Failed to handle agent.completed event")

		// Verify AgentRun node was updated
		agentRunID := "agent_run:" + traceID + ":" + spanID
		cypher := `MATCH (ar:agent_run {id: $id}) RETURN ar`
		params := map[string]any{"id": agentRunID}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query AgentRun node")
		require.Len(t, result, 1, "AgentRun node should exist")

		agentRun := result[0]["ar"].(map[string]any)
		assert.Equal(t, "completed", agentRun["status"], "Agent status should be completed")
		assert.Equal(t, int64(0), agentRun["exit_code"], "Exit code should be 0")
		assert.Equal(t, int64(15000), agentRun["duration_ms"], "Duration mismatch")
		assert.Equal(t, int64(2), agentRun["finding_count"], "Finding count mismatch")
	})
}

// testLLMCallTracking tests llm.request.started, llm.request.completed events and MADE_CALL relationship
func testLLMCallTracking(t *testing.T, ctx context.Context, engine TaxonomyGraphEngine, graphClient graph.GraphClient) {
	missionID := "test-mission-llm-" + time.Now().Format("20060102-150405")
	traceID := "trace-" + missionID
	agentSpanID := "span-agent-001"
	llmSpanID := "span-llm-001"
	timestamp := time.Now().Unix()

	// Create mission and agent first
	missionData := map[string]any{
		"mission_id": missionID,
		"trace_id":   traceID,
		"timestamp":  timestamp,
	}
	err := engine.HandleEvent(ctx, "mission.started", missionData)
	require.NoError(t, err, "Failed to create mission")

	agentData := map[string]any{
		"agent_name": "whistler",
		"mission_id": missionID,
		"trace_id":   traceID,
		"span_id":    agentSpanID,
		"timestamp":  timestamp,
	}
	err = engine.HandleEvent(ctx, "agent.started", agentData)
	require.NoError(t, err, "Failed to create agent")

	// Test llm.request.started event
	t.Run("LLMRequestStarted", func(t *testing.T) {
		eventData := map[string]any{
			"model":          "claude-3-5-sonnet-20241022",
			"trace_id":       traceID,
			"span_id":        llmSpanID,
			"parent_span_id": agentSpanID,
			"timestamp":      timestamp,
			"slot_name":      "reasoning",
			"provider":       "anthropic",
			"message_count":  5,
			"max_tokens":     4096,
			"temperature":    0.7,
			"stream":         false,
			"tool_count":     10,
		}

		err := engine.HandleEvent(ctx, "llm.request.started", eventData)
		require.NoError(t, err, "Failed to handle llm.request.started event")

		// Verify LLMCall node was created
		llmCallID := "llm_call:" + traceID + ":" + llmSpanID + ":" + string(rune(timestamp))
		cypher := `MATCH (lc:llm_call) WHERE lc.id STARTS WITH $id_prefix RETURN lc`
		params := map[string]any{"id_prefix": "llm_call:" + traceID + ":" + llmSpanID}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query LLMCall node")
		require.Len(t, result, 1, "LLMCall node should exist")

		llmCall := result[0]["lc"].(map[string]any)
		assert.Equal(t, "claude-3-5-sonnet-20241022", llmCall["model"], "Model mismatch")
		assert.Equal(t, "reasoning", llmCall["purpose"], "Purpose mismatch")
		assert.Equal(t, "anthropic", llmCall["provider"], "Provider mismatch")
		assert.Equal(t, int64(5), llmCall["message_count"], "Message count mismatch")
		assert.Equal(t, int64(4096), llmCall["max_tokens"], "Max tokens mismatch")
	})

	// Test MADE_CALL relationship
	t.Run("MadeCallRelationship", func(t *testing.T) {
		agentRunID := "agent_run:" + traceID + ":" + agentSpanID

		cypher := `
			MATCH (lc:llm_call)-[r:MADE_CALL]->(ar:agent_run {id: $agent_run_id})
			WHERE lc.trace_id = $trace_id
			RETURN r
		`
		params := map[string]any{
			"agent_run_id": agentRunID,
			"trace_id":     traceID,
		}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query MADE_CALL relationship")
		require.Len(t, result, 1, "MADE_CALL relationship should exist")
	})

	// Test llm.request.completed event
	t.Run("LLMRequestCompleted", func(t *testing.T) {
		eventData := map[string]any{
			"trace_id":       traceID,
			"span_id":        llmSpanID,
			"timestamp":      timestamp,
			"duration":       2500,
			"input_tokens":   1200,
			"output_tokens":  800,
			"stop_reason":    "end_turn",
			"cost_usd":       0.025,
		}

		err := engine.HandleEvent(ctx, "llm.request.completed", eventData)
		require.NoError(t, err, "Failed to handle llm.request.completed event")

		// Verify LLMCall node was updated
		cypher := `MATCH (lc:llm_call) WHERE lc.trace_id = $trace_id AND lc.span_id = $span_id RETURN lc`
		params := map[string]any{
			"trace_id": traceID,
			"span_id":  llmSpanID,
		}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query LLMCall node")
		require.Len(t, result, 1, "LLMCall node should exist")

		llmCall := result[0]["lc"].(map[string]any)
		assert.Equal(t, int64(2500), llmCall["latency_ms"], "Latency mismatch")
		assert.Equal(t, int64(1200), llmCall["prompt_tokens"], "Prompt tokens mismatch")
		assert.Equal(t, int64(800), llmCall["completion_tokens"], "Completion tokens mismatch")
		assert.Equal(t, "end_turn", llmCall["stop_reason"], "Stop reason mismatch")
		assert.Equal(t, 0.025, llmCall["cost_usd"], "Cost mismatch")
	})
}

// testToolExecutionTracking tests tool.call.started, tool.call.completed events and EXECUTED_BY relationship
func testToolExecutionTracking(t *testing.T, ctx context.Context, engine TaxonomyGraphEngine, graphClient graph.GraphClient) {
	missionID := "test-mission-tool-" + time.Now().Format("20060102-150405")
	traceID := "trace-" + missionID
	agentSpanID := "span-agent-001"
	toolSpanID := "span-tool-001"
	timestamp := time.Now().Unix()

	// Create mission and agent first
	missionData := map[string]any{
		"mission_id": missionID,
		"trace_id":   traceID,
		"timestamp":  timestamp,
	}
	err := engine.HandleEvent(ctx, "mission.started", missionData)
	require.NoError(t, err, "Failed to create mission")

	agentData := map[string]any{
		"agent_name": "bishop",
		"mission_id": missionID,
		"trace_id":   traceID,
		"span_id":    agentSpanID,
		"timestamp":  timestamp,
	}
	err = engine.HandleEvent(ctx, "agent.started", agentData)
	require.NoError(t, err, "Failed to create agent")

	// Test tool.call.started event
	t.Run("ToolCallStarted", func(t *testing.T) {
		eventData := map[string]any{
			"tool_name":       "nmap",
			"trace_id":        traceID,
			"span_id":         toolSpanID,
			"parent_span_id":  agentSpanID,
			"timestamp":       timestamp,
			"parameters":      "-sV -p- example.com",
			"parameter_size":  25,
		}

		err := engine.HandleEvent(ctx, "tool.call.started", eventData)
		require.NoError(t, err, "Failed to handle tool.call.started event")

		// Verify ToolExecution node was created
		cypher := `MATCH (te:tool_execution) WHERE te.trace_id = $trace_id AND te.span_id = $span_id RETURN te`
		params := map[string]any{
			"trace_id": traceID,
			"span_id":  toolSpanID,
		}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query ToolExecution node")
		require.Len(t, result, 1, "ToolExecution node should exist")

		toolExec := result[0]["te"].(map[string]any)
		assert.Equal(t, "nmap", toolExec["tool_name"], "Tool name mismatch")
		assert.Equal(t, "-sV -p- example.com", toolExec["command"], "Command mismatch")
		assert.Equal(t, int64(25), toolExec["parameter_size"], "Parameter size mismatch")
	})

	// Test EXECUTED_BY relationship
	t.Run("ExecutedByRelationship", func(t *testing.T) {
		agentRunID := "agent_run:" + traceID + ":" + agentSpanID

		cypher := `
			MATCH (te:tool_execution)-[r:EXECUTED_BY]->(ar:agent_run {id: $agent_run_id})
			WHERE te.trace_id = $trace_id AND te.span_id = $span_id
			RETURN r
		`
		params := map[string]any{
			"agent_run_id": agentRunID,
			"trace_id":     traceID,
			"span_id":      toolSpanID,
		}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query EXECUTED_BY relationship")
		require.Len(t, result, 1, "EXECUTED_BY relationship should exist")
	})

	// Test tool.call.completed event
	t.Run("ToolCallCompleted", func(t *testing.T) {
		eventData := map[string]any{
			"trace_id":    traceID,
			"span_id":     toolSpanID,
			"timestamp":   timestamp,
			"duration":    45000,
			"result_size": 2048,
			"success":     true,
		}

		err := engine.HandleEvent(ctx, "tool.call.completed", eventData)
		require.NoError(t, err, "Failed to handle tool.call.completed event")

		// Verify ToolExecution node was updated
		cypher := `MATCH (te:tool_execution) WHERE te.trace_id = $trace_id AND te.span_id = $span_id RETURN te`
		params := map[string]any{
			"trace_id": traceID,
			"span_id":  toolSpanID,
		}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query ToolExecution node")
		require.Len(t, result, 1, "ToolExecution node should exist")

		toolExec := result[0]["te"].(map[string]any)
		assert.Equal(t, int64(45000), toolExec["duration_ms"], "Duration mismatch")
		assert.Equal(t, int64(2048), toolExec["output_size"], "Output size mismatch")
		assert.Equal(t, true, toolExec["success"], "Success mismatch")
		assert.Equal(t, int64(0), toolExec["exit_code"], "Exit code should be 0")
	})
}

// testAgentDelegation tests agent.delegated event and DELEGATED_TO relationship
func testAgentDelegation(t *testing.T, ctx context.Context, engine TaxonomyGraphEngine, graphClient graph.GraphClient) {
	missionID := "test-mission-delegation-" + time.Now().Format("20060102-150405")
	traceID := "trace-" + missionID
	whistlerSpanID := "span-whistler-001"
	bishopSpanID := "span-bishop-001"
	timestamp := time.Now().Unix()

	// Create mission
	missionData := map[string]any{
		"mission_id": missionID,
		"trace_id":   traceID,
		"timestamp":  timestamp,
	}
	err := engine.HandleEvent(ctx, "mission.started", missionData)
	require.NoError(t, err, "Failed to create mission")

	// Create orchestrator agent (whistler)
	whistlerData := map[string]any{
		"agent_name": "whistler",
		"mission_id": missionID,
		"trace_id":   traceID,
		"span_id":    whistlerSpanID,
		"timestamp":  timestamp,
	}
	err = engine.HandleEvent(ctx, "agent.started", whistlerData)
	require.NoError(t, err, "Failed to create whistler agent")

	// Create delegate agent (bishop)
	bishopData := map[string]any{
		"agent_name": "bishop",
		"mission_id": missionID,
		"trace_id":   traceID,
		"span_id":    bishopSpanID,
		"timestamp":  timestamp,
	}
	err = engine.HandleEvent(ctx, "agent.started", bishopData)
	require.NoError(t, err, "Failed to create bishop agent")

	// Test agent.delegated event
	t.Run("AgentDelegated", func(t *testing.T) {
		eventData := map[string]any{
			"from_trace_id":    traceID,
			"from_span_id":     whistlerSpanID,
			"to_trace_id":      traceID,
			"to_span_id":       bishopSpanID,
			"task_description": "Enumerate subdomains for example.com",
			"timestamp":        timestamp,
		}

		err := engine.HandleEvent(ctx, "agent.delegated", eventData)
		require.NoError(t, err, "Failed to handle agent.delegated event")

		// Verify DELEGATED_TO relationship was created
		whistlerID := "agent_run:" + traceID + ":" + whistlerSpanID
		bishopID := "agent_run:" + traceID + ":" + bishopSpanID

		cypher := `
			MATCH (w:agent_run {id: $whistler_id})-[r:DELEGATED_TO]->(b:agent_run {id: $bishop_id})
			RETURN r
		`
		params := map[string]any{
			"whistler_id": whistlerID,
			"bishop_id":   bishopID,
		}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query DELEGATED_TO relationship")
		require.Len(t, result, 1, "DELEGATED_TO relationship should exist")

		rel := result[0]["r"].(map[string]any)
		assert.Equal(t, "Enumerate subdomains for example.com", rel["task_description"], "Task description mismatch")
	})
}

// cleanupTestData removes all test data from Neo4j
func cleanupTestData(t *testing.T, ctx context.Context, graphClient graph.GraphClient) {
	cypher := `
		MATCH (n)
		WHERE n.id STARTS WITH 'mission:test-' OR
		      n.id STARTS WITH 'agent_run:trace-test-' OR
		      n.id STARTS WITH 'llm_call:trace-test-' OR
		      n.id STARTS WITH 'tool_execution:trace-test-'
		DETACH DELETE n
	`
	_, err := graphClient.Query(ctx, cypher, nil)
	if err != nil {
		t.Logf("Warning: Failed to clean up test data: %v", err)
	}
}
