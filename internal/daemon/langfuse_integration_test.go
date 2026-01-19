package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/observability"
	"github.com/zero-day-ai/gibson/internal/orchestrator"
	"github.com/zero-day-ai/gibson/internal/types"
)

// mockLangfuseServer creates a test HTTP server that captures all Langfuse events.
// This allows us to verify the complete event hierarchy for integration testing.
type mockLangfuseServer struct {
	*httptest.Server
	mu     sync.Mutex
	events []map[string]any
}

func newMockLangfuseServerForIntegration() *mockLangfuseServer {
	mock := &mockLangfuseServer{
		events: make([]map[string]any, 0),
	}

	mock.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authentication
		username, password, ok := r.BasicAuth()
		if !ok || username != "test-public-key" || password != "test-secret-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Parse request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Extract batch events
		batch, ok := payload["batch"].([]any)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Store events
		mock.mu.Lock()
		for _, event := range batch {
			if eventMap, ok := event.(map[string]any); ok {
				mock.events = append(mock.events, eventMap)
			}
		}
		mock.mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))

	return mock
}

func (m *mockLangfuseServer) getEvents() []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]map[string]any{}, m.events...)
}

func (m *mockLangfuseServer) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = make([]map[string]any, 0)
}

// TestLangfuseIntegration_MiniMission tests a complete mini-mission flow through the observability system.
// This test validates that all expected Langfuse events are generated and properly linked.
func TestLangfuseIntegration_MiniMission(t *testing.T) {
	// Start mock Langfuse server
	server := newMockLangfuseServerForIntegration()
	defer server.Close()

	// Create mission tracer
	tracer, err := observability.NewMissionTracer(observability.LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)
	defer tracer.Close()

	// Create test mission
	missionID := types.NewID()
	mission := schema.NewMission(
		missionID,
		"Integration Test Mission",
		"Test mission for integration testing",
		"Validate complete Langfuse event hierarchy",
		"target-123",
		"yaml: test",
	)

	ctx := context.Background()

	// Create DecisionLogWriterAdapter
	adapter, err := observability.NewDecisionLogWriterAdapter(ctx, tracer, mission)
	require.NoError(t, err)
	require.NotNil(t, adapter)

	// Clear events from trace creation
	time.Sleep(100 * time.Millisecond) // Wait for async trace creation
	server.reset()

	// Simulate mini-mission flow:
	// Iteration 1: Decision -> Execute Agent
	// Iteration 2: Agent uses Tool
	// Iteration 3: Decision -> Complete

	// --- Iteration 1: Decision to execute agent ---
	decision1 := &orchestrator.Decision{
		Reasoning:    "Execute reconnaissance agent to gather information",
		Action:       orchestrator.ActionExecuteAgent,
		TargetNodeID: "node-recon",
		Confidence:   0.95,
	}
	result1 := &orchestrator.ThinkResult{
		PromptTokens:     150,
		CompletionTokens: 50,
		TotalTokens:      200,
		Latency:          250 * time.Millisecond,
		Model:            "gpt-4",
		RawResponse:      `{"action":"execute_agent","node":"node-recon"}`,
	}

	err = adapter.LogDecision(ctx, decision1, result1, 1, missionID.String())
	require.NoError(t, err)

	// Simulate agent execution
	agentExec := schema.NewAgentExecution("node-recon", missionID)
	agentExec.WithConfig(map[string]any{"timeout": 300, "scan_depth": 2})
	agentExec.WithResult(map[string]any{"ports_found": []int{22, 80, 443}, "services": []string{"ssh", "http", "https"}})
	now := time.Now()
	agentExec.CompletedAt = &now
	agentExec.Status = schema.ExecutionStatusCompleted

	action1 := &orchestrator.ActionResult{
		Action:         orchestrator.ActionExecuteAgent,
		AgentExecution: agentExec,
		Metadata: map[string]interface{}{
			"agent_name": "recon-agent",
		},
	}

	err = adapter.LogAction(ctx, action1, 1, missionID.String())
	require.NoError(t, err)

	// --- Iteration 2: Agent uses tool (simulated directly via tracer) ---
	// NOTE: In the real flow, tool executions are logged by the harness middleware
	// For integration testing, we simulate the tool execution hierarchy
	// The adapter stores agent logs internally, so we create the log manually here

	agentLog := &observability.AgentExecutionLog{
		Execution:   agentExec,
		AgentName:   "recon-agent",
		Config:      make(map[string]any),
		Neo4jNodeID: "neo4j-exec-456",
		SpanID:      fmt.Sprintf("agent-exec-%s", agentExec.ID.String()),
	}

	toolExec := schema.NewToolExecution(agentExec.ID, "port-scanner")
	toolExec.WithInput(map[string]any{"target": "192.168.1.1", "ports": "1-1024"})
	toolExec.WithOutput(map[string]any{"open_ports": []int{22, 80, 443}, "scan_time": "2.3s"})
	toolNow := time.Now()
	toolExec.CompletedAt = &toolNow
	toolExec.Status = schema.ExecutionStatusCompleted

	toolLog := &observability.ToolExecutionLog{
		Execution:   toolExec,
		Neo4jNodeID: "neo4j-tool-789",
	}

	err = tracer.LogToolExecution(ctx, agentLog, toolLog)
	require.NoError(t, err)

	// --- Iteration 3: Decision to complete ---
	decision2 := &orchestrator.Decision{
		Reasoning:    "Mission objective achieved, sufficient data collected",
		Action:       orchestrator.ActionComplete,
		TargetNodeID: "",
		Confidence:   0.98,
	}
	result2 := &orchestrator.ThinkResult{
		PromptTokens:     180,
		CompletionTokens: 60,
		TotalTokens:      240,
		Latency:          300 * time.Millisecond,
		Model:            "gpt-4",
		RawResponse:      `{"action":"complete","reasoning":"objective achieved"}`,
	}

	err = adapter.LogDecision(ctx, decision2, result2, 3, missionID.String())
	require.NoError(t, err)

	// Complete mission with summary
	summary := &observability.MissionTraceSummary{
		Status:          schema.MissionStatusCompleted.String(),
		TotalDecisions:  2,
		TotalExecutions: 1,
		TotalTools:      1,
		TotalTokens:     440,
		TotalCost:       0.01,
		Duration:        2 * time.Minute,
		Outcome:         "Successfully completed reconnaissance and gathered target information",
		GraphStats: map[string]int{
			"nodes":         15,
			"relationships": 28,
			"findings":      3,
		},
	}

	err = adapter.Close(ctx, summary)
	require.NoError(t, err)

	// Wait for async event processing
	time.Sleep(200 * time.Millisecond)

	// --- Verify events ---
	events := server.getEvents()
	require.Greater(t, len(events), 0, "should have captured events")

	// Helper to find events by type
	findEventsByType := func(eventType string) []map[string]any {
		var found []map[string]any
		for _, event := range events {
			if event["type"] == eventType {
				found = append(found, event)
			}
		}
		return found
	}

	// Verify trace-create (from adapter constructor - already cleared)
	// This was reset before we started logging, so we won't find it

	// Verify generation-create events (decisions)
	generations := findEventsByType("generation-create")
	assert.GreaterOrEqual(t, len(generations), 2, "should have at least 2 generation events (decisions)")

	if len(generations) >= 2 {
		// Verify first decision
		gen1Body, ok := generations[0]["body"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, gen1Body["name"], "orchestrator-decision")
		assert.Equal(t, float64(150), gen1Body["promptTokens"])
		assert.Equal(t, float64(50), gen1Body["completionTokens"])
		assert.Equal(t, "gpt-4", gen1Body["model"])

		gen1Meta, ok := gen1Body["metadata"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(1), gen1Meta["iteration"])
		assert.Equal(t, "execute_agent", gen1Meta["action"])

		// Verify second decision
		gen2Body, ok := generations[1]["body"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, gen2Body["name"], "orchestrator-decision")
		assert.Equal(t, float64(180), gen2Body["promptTokens"])
		assert.Equal(t, float64(60), gen2Body["completionTokens"])

		gen2Meta, ok := gen2Body["metadata"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(3), gen2Meta["iteration"])
		assert.Equal(t, "complete", gen2Meta["action"])
	}

	// Verify span-create events (agent execution, tool execution, mission complete)
	spans := findEventsByType("span-create")
	assert.GreaterOrEqual(t, len(spans), 3, "should have at least 3 span events (agent, tool, complete)")

	// Find specific spans
	var agentSpan, toolSpan, completeSpan map[string]any
	for _, span := range spans {
		body, ok := span["body"].(map[string]any)
		if !ok {
			continue
		}
		name, ok := body["name"].(string)
		if !ok {
			continue
		}

		if len(name) >= 15 && name[:15] == "agent-execution" {
			agentSpan = span
		} else if len(name) >= 9 && name[:9] == "tool-call" {
			toolSpan = span
		} else if name == "mission-complete" {
			completeSpan = span
		}
	}

	// Verify agent execution span
	if agentSpan != nil {
		agentBody, ok := agentSpan["body"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, agentBody["name"], "recon-agent")
		assert.Equal(t, "DEFAULT", agentBody["level"])

		agentMeta, ok := agentBody["metadata"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "recon-agent", agentMeta["agent_name"])
		assert.Equal(t, agentExec.ID.String(), agentMeta["execution_id"])
	}

	// Verify tool execution span
	if toolSpan != nil {
		toolBody, ok := toolSpan["body"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, toolBody["name"], "port-scanner")

		// Verify parent linkage - tool should be child of agent
		if agentSpan != nil {
			agentBody, _ := agentSpan["body"].(map[string]any)
			agentSpanID, _ := agentBody["id"].(string)
			toolParentID, _ := toolBody["parentObservationId"].(string)
			assert.Equal(t, agentSpanID, toolParentID, "tool span should be child of agent span")
		}

		toolMeta, ok := toolBody["metadata"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "port-scanner", toolMeta["tool_name"])
		assert.Equal(t, toolExec.ID.String(), toolMeta["tool_execution_id"])
	}

	// Verify mission complete span
	if completeSpan != nil {
		completeBody, ok := completeSpan["body"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "mission-complete", completeBody["name"])
		assert.Equal(t, "DEFAULT", completeBody["level"])

		completeMeta, ok := completeBody["metadata"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, missionID.String(), completeMeta["mission_id"])
		assert.Equal(t, mission.Name, completeMeta["mission_name"])
		assert.Equal(t, schema.MissionStatusCompleted.String(), completeMeta["status"])
		assert.Equal(t, float64(2), completeMeta["total_decisions"])
		assert.Equal(t, float64(1), completeMeta["total_executions"])
		assert.Equal(t, float64(1), completeMeta["total_tools"])
		assert.Equal(t, float64(440), completeMeta["total_tokens"])
		assert.Equal(t, 0.01, completeMeta["total_cost_usd"])
		assert.Equal(t, summary.Outcome, completeMeta["outcome"])
	}

	// Verify trace hierarchy - all events should have the same traceId
	traceIDs := make(map[string]int)
	for _, event := range events {
		body, ok := event["body"].(map[string]any)
		if !ok {
			continue
		}
		traceID, ok := body["traceId"].(string)
		if ok && traceID != "" {
			traceIDs[traceID]++
		}
	}

	// Should have exactly one trace ID (all events belong to same mission)
	assert.Equal(t, 1, len(traceIDs), "all events should belong to same trace")

	expectedTraceID := fmt.Sprintf("mission-%s", missionID.String())
	assert.Contains(t, traceIDs, expectedTraceID, "trace ID should match mission ID")

	t.Logf("Integration test complete: captured %d events, verified trace hierarchy", len(events))
}

// TestLangfuseIntegration_TraceHierarchy specifically validates parent-child relationships.
func TestLangfuseIntegration_TraceHierarchy(t *testing.T) {
	server := newMockLangfuseServerForIntegration()
	defer server.Close()

	tracer, err := observability.NewMissionTracer(observability.LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)
	defer tracer.Close()

	missionID := types.NewID()
	mission := schema.NewMission(
		missionID,
		"Hierarchy Test",
		"Test trace hierarchy",
		"Validate parent-child relationships",
		"target-456",
		"yaml: hierarchy",
	)

	ctx := context.Background()
	adapter, err := observability.NewDecisionLogWriterAdapter(ctx, tracer, mission)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	server.reset()

	// Create agent execution
	agentExec := schema.NewAgentExecution("node-1", missionID)
	now := time.Now()
	agentExec.CompletedAt = &now
	agentExec.Status = schema.ExecutionStatusCompleted

	action := &orchestrator.ActionResult{
		Action:         orchestrator.ActionExecuteAgent,
		AgentExecution: agentExec,
		Metadata:       map[string]interface{}{"agent_name": "test-agent"},
	}

	err = adapter.LogAction(ctx, action, 1, missionID.String())
	require.NoError(t, err)

	// Create multiple tool executions under same agent
	// Create agent log manually for testing (in real flow, adapter stores this)
	agentLog := &observability.AgentExecutionLog{
		Execution:   agentExec,
		AgentName:   "test-agent",
		Config:      make(map[string]any),
		Neo4jNodeID: "neo4j-exec-test",
		SpanID:      fmt.Sprintf("agent-exec-%s", agentExec.ID.String()),
	}

	for i := 0; i < 3; i++ {
		toolExec := schema.NewToolExecution(agentExec.ID, fmt.Sprintf("tool-%d", i))
		toolExec.WithInput(map[string]any{"index": i})
		toolExec.WithOutput(map[string]any{"result": fmt.Sprintf("output-%d", i)})
		toolNow := time.Now()
		toolExec.CompletedAt = &toolNow
		toolExec.Status = schema.ExecutionStatusCompleted

		toolLog := &observability.ToolExecutionLog{
			Execution:   toolExec,
			Neo4jNodeID: fmt.Sprintf("neo4j-tool-%d", i),
		}

		err = tracer.LogToolExecution(ctx, agentLog, toolLog)
		require.NoError(t, err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify hierarchy
	events := server.getEvents()

	// Find agent span
	var agentSpanID string
	for _, event := range events {
		if event["type"] != "span-create" {
			continue
		}
		body, ok := event["body"].(map[string]any)
		if !ok {
			continue
		}
		name, ok := body["name"].(string)
		if !ok {
			continue
		}
		if len(name) >= 15 && name[:15] == "agent-execution" {
			agentSpanID, _ = body["id"].(string)
			break
		}
	}

	require.NotEmpty(t, agentSpanID, "agent span should exist")

	// Verify all tool spans are children of agent span
	toolSpanCount := 0
	for _, event := range events {
		if event["type"] != "span-create" {
			continue
		}
		body, ok := event["body"].(map[string]any)
		if !ok {
			continue
		}
		name, ok := body["name"].(string)
		if !ok {
			continue
		}
		if len(name) >= 9 && name[:9] == "tool-call" {
			parentID, ok := body["parentObservationId"].(string)
			assert.True(t, ok, "tool span should have parent ID")
			assert.Equal(t, agentSpanID, parentID, "tool span should be child of agent span")
			toolSpanCount++
		}
	}

	assert.Equal(t, 3, toolSpanCount, "should have 3 tool spans as children of agent")

	t.Logf("Hierarchy test complete: verified %d tool spans under agent span", toolSpanCount)
}

// TestLangfuseIntegration_ConcurrentDecisions tests that concurrent logging doesn't break trace hierarchy.
func TestLangfuseIntegration_ConcurrentDecisions(t *testing.T) {
	server := newMockLangfuseServerForIntegration()
	defer server.Close()

	tracer, err := observability.NewMissionTracer(observability.LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)
	defer tracer.Close()

	missionID := types.NewID()
	mission := schema.NewMission(
		missionID,
		"Concurrent Test",
		"Test concurrent logging",
		"Validate thread-safety",
		"target-789",
		"yaml: concurrent",
	)

	ctx := context.Background()
	adapter, err := observability.NewDecisionLogWriterAdapter(ctx, tracer, mission)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	server.reset()

	// Log decisions concurrently
	numDecisions := 10
	var wg sync.WaitGroup

	for i := 0; i < numDecisions; i++ {
		wg.Add(1)
		go func(iteration int) {
			defer wg.Done()

			decision := &orchestrator.Decision{
				Reasoning:    fmt.Sprintf("Decision %d", iteration),
				Action:       orchestrator.ActionExecuteAgent,
				TargetNodeID: fmt.Sprintf("node-%d", iteration),
				Confidence:   0.9,
			}

			result := &orchestrator.ThinkResult{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
				Model:            "gpt-4",
				RawResponse:      fmt.Sprintf(`{"iteration":%d}`, iteration),
			}

			err := adapter.LogDecision(ctx, decision, result, iteration, missionID.String())
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	// Verify all decisions logged
	events := server.getEvents()
	generationCount := 0
	traceIDs := make(map[string]int)

	for _, event := range events {
		if event["type"] == "generation-create" {
			generationCount++

			body, ok := event["body"].(map[string]any)
			if ok {
				traceID, ok := body["traceId"].(string)
				if ok {
					traceIDs[traceID]++
				}
			}
		}
	}

	assert.Equal(t, numDecisions, generationCount, "should have logged all decisions")
	assert.Equal(t, 1, len(traceIDs), "all decisions should belong to same trace")

	expectedTraceID := fmt.Sprintf("mission-%s", missionID.String())
	assert.Contains(t, traceIDs, expectedTraceID)

	t.Logf("Concurrent test complete: logged %d decisions with consistent trace ID", numDecisions)
}

// TestLangfuseIntegration_NilTracer validates that nil tracer is handled gracefully.
func TestLangfuseIntegration_NilTracer(t *testing.T) {
	missionID := types.NewID()
	mission := schema.NewMission(
		missionID,
		"Nil Tracer Test",
		"Test nil tracer handling",
		"Validate graceful degradation",
		"target-nil",
		"yaml: nil",
	)

	ctx := context.Background()

	// Attempt to create adapter with nil tracer
	adapter, err := observability.NewDecisionLogWriterAdapter(ctx, nil, mission)
	assert.Error(t, err, "should error when tracer is nil")
	assert.Nil(t, adapter, "adapter should be nil")
	assert.Contains(t, err.Error(), "tracer cannot be nil")
}

// TestLangfuseIntegration_DeterministicCleanu validates proper resource cleanup.
func TestLangfuseIntegration_DeterministicCleanup(t *testing.T) {
	server := newMockLangfuseServerForIntegration()
	defer server.Close()

	tracer, err := observability.NewMissionTracer(observability.LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)

	missionID := types.NewID()
	mission := schema.NewMission(
		missionID,
		"Cleanup Test",
		"Test cleanup",
		"Validate resource cleanup",
		"target-cleanup",
		"yaml: cleanup",
	)

	ctx := context.Background()
	adapter, err := observability.NewDecisionLogWriterAdapter(ctx, tracer, mission)
	require.NoError(t, err)

	// Log some events
	decision := &orchestrator.Decision{
		Reasoning:  "Test decision",
		Action:     orchestrator.ActionComplete,
		Confidence: 0.9,
	}

	result := &orchestrator.ThinkResult{
		TotalTokens: 100,
		Model:       "gpt-4",
		RawResponse: `{"action":"complete"}`,
	}

	err = adapter.LogDecision(ctx, decision, result, 1, missionID.String())
	require.NoError(t, err)

	// Close adapter
	summary := &observability.MissionTraceSummary{
		Status:   schema.MissionStatusCompleted.String(),
		Duration: 1 * time.Minute,
		Outcome:  "Cleanup test complete",
	}

	err = adapter.Close(ctx, summary)
	require.NoError(t, err)

	// Close tracer
	err = tracer.Close()
	require.NoError(t, err)

	// Should be safe to close multiple times
	err = tracer.Close()
	assert.NoError(t, err, "tracer.Close() should be idempotent")

	t.Logf("Cleanup test complete: all resources cleaned up successfully")
}
