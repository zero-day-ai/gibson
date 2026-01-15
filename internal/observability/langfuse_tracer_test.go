package observability

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
	"github.com/zero-day-ai/gibson/internal/types"
)

// mockLangfuseServer creates a test HTTP server that mimics Langfuse API behavior.
type mockLangfuseServer struct {
	*httptest.Server
	mu     sync.Mutex
	events []map[string]any
}

func newMockLangfuseServer() *mockLangfuseServer {
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

		// Verify content type
		if r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
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

func TestNewMissionTracer(t *testing.T) {
	tests := []struct {
		name        string
		config      LangfuseConfig
		expectError bool
	}{
		{
			name: "valid configuration",
			config: LangfuseConfig{
				Host:      "https://langfuse.example.com",
				PublicKey: "pk_test",
				SecretKey: "sk_test",
			},
			expectError: false,
		},
		{
			name: "missing host",
			config: LangfuseConfig{
				PublicKey: "pk_test",
				SecretKey: "sk_test",
			},
			expectError: true,
		},
		{
			name: "missing public key",
			config: LangfuseConfig{
				Host:      "https://langfuse.example.com",
				SecretKey: "sk_test",
			},
			expectError: true,
		},
		{
			name: "missing secret key",
			config: LangfuseConfig{
				Host:      "https://langfuse.example.com",
				PublicKey: "pk_test",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracer, err := NewMissionTracer(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, tracer)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tracer)
				assert.Equal(t, tt.config.Host, tracer.host)
				assert.Equal(t, tt.config.PublicKey, tracer.publicKey)
				assert.Equal(t, tt.config.SecretKey, tracer.secretKey)

				// Clean up
				err = tracer.Close()
				assert.NoError(t, err)
			}
		})
	}
}

func TestStartMissionTrace(t *testing.T) {
	server := newMockLangfuseServer()
	defer server.Close()

	tracer, err := NewMissionTracer(LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)
	defer tracer.Close()

	mission := schema.NewMission(
		types.NewID(),
		"Test Mission",
		"Test mission description",
		"Test objective",
		"target-123",
		"yaml: test",
	)

	ctx := context.Background()
	trace, err := tracer.StartMissionTrace(ctx, mission)

	// Verify trace creation
	require.NoError(t, err)
	assert.NotNil(t, trace)
	assert.Equal(t, fmt.Sprintf("mission-%s", mission.ID.String()), trace.TraceID)
	assert.Equal(t, mission.ID, trace.MissionID)
	assert.Equal(t, mission.Name, trace.Name)

	// Verify event was sent
	events := server.getEvents()
	require.Len(t, events, 1)

	event := events[0]
	assert.Equal(t, "trace-create", event["type"])

	body, ok := event["body"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, fmt.Sprintf("mission-%s", mission.ID.String()), body["id"])
	assert.Contains(t, body["name"], mission.Name)

	metadata, ok := body["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, mission.ID.String(), metadata["mission_id"])
	assert.Equal(t, mission.Name, metadata["mission_name"])
	assert.Equal(t, mission.Objective, metadata["objective"])
}

func TestStartMissionTrace_NilMission(t *testing.T) {
	server := newMockLangfuseServer()
	defer server.Close()

	tracer, err := NewMissionTracer(LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)
	defer tracer.Close()

	ctx := context.Background()
	trace, err := tracer.StartMissionTrace(ctx, nil)

	assert.Error(t, err)
	assert.Nil(t, trace)
	assert.Contains(t, err.Error(), "cannot be nil")
}

func TestLogDecision(t *testing.T) {
	server := newMockLangfuseServer()
	defer server.Close()

	tracer, err := NewMissionTracer(LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)
	defer tracer.Close()

	missionID := types.NewID()
	trace := &MissionTrace{
		TraceID:   fmt.Sprintf("mission-%s", missionID.String()),
		MissionID: missionID,
		Name:      "Test Mission",
		StartTime: time.Now(),
	}

	decision := schema.NewDecision(missionID, 1, schema.DecisionActionExecuteAgent)
	decision.WithTargetNode("node-1")
	decision.WithReasoning("This is the reasoning")
	decision.WithConfidence(0.95)
	decision.WithTokenUsage(100, 50)
	decision.WithLatency(250)

	log := &DecisionLog{
		Decision:      decision,
		Prompt:        "Test prompt with graph state",
		Response:      `{"action": "execute_agent", "node": "node-1"}`,
		Model:         "gpt-4",
		GraphSnapshot: "Graph state: 5 nodes, 10 relationships",
		Neo4jNodeID:   "neo4j-node-123",
	}

	ctx := context.Background()
	err = tracer.LogDecision(ctx, trace, log)

	// Verify logging succeeded
	require.NoError(t, err)

	// Verify event was sent
	events := server.getEvents()
	require.Len(t, events, 1)

	event := events[0]
	assert.Equal(t, "generation-create", event["type"])

	body, ok := event["body"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, body["name"], "orchestrator-decision")
	assert.Equal(t, trace.TraceID, body["traceId"])
	assert.Equal(t, log.Prompt, body["input"])
	assert.Equal(t, log.Response, body["output"])
	assert.Equal(t, log.Model, body["model"])
	assert.Equal(t, float64(100), body["promptTokens"])
	assert.Equal(t, float64(50), body["completionTokens"])

	metadata, ok := body["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, decision.ID.String(), metadata["decision_id"])
	assert.Equal(t, float64(1), metadata["iteration"])
	assert.Equal(t, decision.Action.String(), metadata["action"])
	assert.Equal(t, log.GraphSnapshot, metadata["graph_snapshot"])
	assert.Equal(t, log.Neo4jNodeID, metadata["neo4j_node_id"])
}

func TestLogAgentExecution(t *testing.T) {
	server := newMockLangfuseServer()
	defer server.Close()

	tracer, err := NewMissionTracer(LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)
	defer tracer.Close()

	missionID := types.NewID()
	trace := &MissionTrace{
		TraceID:   fmt.Sprintf("mission-%s", missionID.String()),
		MissionID: missionID,
		Name:      "Test Mission",
		StartTime: time.Now(),
	}

	execution := schema.NewAgentExecution("workflow-node-1", missionID)
	execution.WithConfig(map[string]any{"timeout": 300})
	execution.WithResult(map[string]any{"status": "success"})
	now := time.Now()
	execution.CompletedAt = &now
	execution.Status = schema.ExecutionStatusCompleted

	log := &AgentExecutionLog{
		Execution:   execution,
		AgentName:   "recon-agent",
		Config:      execution.ConfigUsed,
		Neo4jNodeID: "neo4j-exec-456",
	}

	ctx := context.Background()
	err = tracer.LogAgentExecution(ctx, trace, log)

	// Verify logging succeeded
	require.NoError(t, err)
	assert.NotEmpty(t, log.SpanID) // Span ID should be set

	// Verify event was sent
	events := server.getEvents()
	require.Len(t, events, 1)

	event := events[0]
	assert.Equal(t, "span-create", event["type"])

	body, ok := event["body"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, body["name"], "agent-execution")
	assert.Contains(t, body["name"], "recon-agent")
	assert.Equal(t, trace.TraceID, body["traceId"])
	assert.Equal(t, "DEFAULT", body["level"])

	metadata, ok := body["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, execution.ID.String(), metadata["execution_id"])
	assert.Equal(t, execution.WorkflowNodeID, metadata["workflow_node_id"])
	assert.Equal(t, "recon-agent", metadata["agent_name"])
	assert.Equal(t, log.Neo4jNodeID, metadata["neo4j_node_id"])
}

func TestLogToolExecution(t *testing.T) {
	server := newMockLangfuseServer()
	defer server.Close()

	tracer, err := NewMissionTracer(LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)
	defer tracer.Close()

	missionID := types.NewID()
	agentExecID := types.NewID()

	// Create parent agent execution log
	agentExec := schema.NewAgentExecution("workflow-node-1", missionID)
	agentExec.ID = agentExecID

	parentLog := &AgentExecutionLog{
		Execution:   agentExec,
		AgentName:   "recon-agent",
		Neo4jNodeID: "neo4j-exec-456",
		SpanID:      fmt.Sprintf("agent-exec-%s", agentExecID.String()),
	}

	// Create tool execution
	toolExec := schema.NewToolExecution(agentExecID, "nmap")
	toolExec.WithInput(map[string]any{"target": "192.168.1.1", "ports": "1-1000"})
	toolExec.WithOutput(map[string]any{"open_ports": []int{22, 80, 443}})
	now := time.Now()
	toolExec.CompletedAt = &now
	toolExec.Status = schema.ExecutionStatusCompleted

	log := &ToolExecutionLog{
		Execution:   toolExec,
		Neo4jNodeID: "neo4j-tool-789",
	}

	ctx := context.Background()
	err = tracer.LogToolExecution(ctx, parentLog, log)

	// Verify logging succeeded
	require.NoError(t, err)
	assert.NotEmpty(t, log.SpanID) // Span ID should be set

	// Verify event was sent
	events := server.getEvents()
	require.Len(t, events, 1)

	event := events[0]
	assert.Equal(t, "span-create", event["type"])

	body, ok := event["body"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, body["name"], "tool-call-nmap")
	assert.Equal(t, parentLog.SpanID, body["parentObservationId"]) // Should link to parent
	assert.Equal(t, "DEFAULT", body["level"])

	metadata, ok := body["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, toolExec.ID.String(), metadata["tool_execution_id"])
	assert.Equal(t, "nmap", metadata["tool_name"])
	assert.Equal(t, log.Neo4jNodeID, metadata["neo4j_node_id"])
}

func TestEndMissionTrace(t *testing.T) {
	server := newMockLangfuseServer()
	defer server.Close()

	tracer, err := NewMissionTracer(LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)
	defer tracer.Close()

	missionID := types.NewID()
	startTime := time.Now().Add(-5 * time.Minute)
	trace := &MissionTrace{
		TraceID:   fmt.Sprintf("mission-%s", missionID.String()),
		MissionID: missionID,
		Name:      "Test Mission",
		StartTime: startTime,
	}

	summary := &MissionTraceSummary{
		Status:          schema.MissionStatusCompleted.String(),
		TotalDecisions:  10,
		TotalExecutions: 8,
		TotalTools:      25,
		TotalTokens:     50000,
		TotalCost:       0.75,
		Duration:        5 * time.Minute,
		Outcome:         "Successfully completed reconnaissance and exploitation",
		GraphStats: map[string]int{
			"nodes":         100,
			"relationships": 250,
			"findings":      5,
		},
	}

	ctx := context.Background()
	err = tracer.EndMissionTrace(ctx, trace, summary)

	// Verify logging succeeded
	require.NoError(t, err)

	// Verify event was sent
	events := server.getEvents()
	require.Len(t, events, 1)

	event := events[0]
	assert.Equal(t, "span-create", event["type"])

	body, ok := event["body"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "mission-complete", body["name"])
	assert.Equal(t, trace.TraceID, body["traceId"])
	assert.Equal(t, "DEFAULT", body["level"])

	metadata, ok := body["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, trace.MissionID.String(), metadata["mission_id"])
	assert.Equal(t, trace.Name, metadata["mission_name"])
	assert.Equal(t, summary.Status, metadata["status"])
	assert.Equal(t, float64(summary.TotalDecisions), metadata["total_decisions"])
	assert.Equal(t, float64(summary.TotalExecutions), metadata["total_executions"])
	assert.Equal(t, float64(summary.TotalTools), metadata["total_tools"])
	assert.Equal(t, float64(summary.TotalTokens), metadata["total_tokens"])
	assert.Equal(t, summary.TotalCost, metadata["total_cost_usd"])
	assert.Equal(t, summary.Outcome, metadata["outcome"])
}

func TestEndMissionTrace_FailedStatus(t *testing.T) {
	server := newMockLangfuseServer()
	defer server.Close()

	tracer, err := NewMissionTracer(LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)
	defer tracer.Close()

	missionID := types.NewID()
	trace := &MissionTrace{
		TraceID:   fmt.Sprintf("mission-%s", missionID.String()),
		MissionID: missionID,
		Name:      "Failed Mission",
		StartTime: time.Now().Add(-2 * time.Minute),
	}

	summary := &MissionTraceSummary{
		Status:   schema.MissionStatusFailed.String(),
		Duration: 2 * time.Minute,
		Outcome:  "Mission failed due to timeout",
	}

	ctx := context.Background()
	err = tracer.EndMissionTrace(ctx, trace, summary)

	require.NoError(t, err)

	// Verify error level is set
	events := server.getEvents()
	require.Len(t, events, 1)

	body, ok := events[0]["body"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ERROR", body["level"])
}

func TestMissionTracer_ConcurrentLogging(t *testing.T) {
	server := newMockLangfuseServer()
	defer server.Close()

	tracer, err := NewMissionTracer(LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)
	defer tracer.Close()

	missionID := types.NewID()
	trace := &MissionTrace{
		TraceID:   fmt.Sprintf("mission-%s", missionID.String()),
		MissionID: missionID,
		Name:      "Concurrent Test",
		StartTime: time.Now(),
	}

	ctx := context.Background()
	numGoroutines := 10
	var wg sync.WaitGroup

	// Log multiple decisions concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(iteration int) {
			defer wg.Done()

			decision := schema.NewDecision(missionID, iteration, schema.DecisionActionExecuteAgent)
			decision.WithTokenUsage(100, 50)

			log := &DecisionLog{
				Decision:      decision,
				Prompt:        fmt.Sprintf("Prompt %d", iteration),
				Response:      fmt.Sprintf("Response %d", iteration),
				Model:         "gpt-4",
				GraphSnapshot: "state",
				Neo4jNodeID:   fmt.Sprintf("node-%d", iteration),
			}

			err := tracer.LogDecision(ctx, trace, log)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Verify all events were sent
	events := server.getEvents()
	assert.Len(t, events, numGoroutines)
}

func TestMissionTracer_AuthenticationFailure(t *testing.T) {
	server := newMockLangfuseServer()
	defer server.Close()

	// Create tracer with wrong credentials
	tracer, err := NewMissionTracer(LangfuseConfig{
		Host:      server.URL,
		PublicKey: "wrong-key",
		SecretKey: "wrong-secret",
	})
	require.NoError(t, err)
	defer tracer.Close()

	mission := schema.NewMission(
		types.NewID(),
		"Test Mission",
		"Description",
		"Objective",
		"target",
		"yaml",
	)

	ctx := context.Background()
	trace, err := tracer.StartMissionTrace(ctx, mission)

	// Should fail with authentication error
	assert.Error(t, err)
	assert.Nil(t, trace)
}

func TestGenerateEventID(t *testing.T) {
	id := "test-id-123456789"
	eventID1 := generateEventID("trace", id)
	eventID2 := generateEventID("trace", id)

	// Event IDs should be unique (include timestamp)
	assert.NotEqual(t, eventID1, eventID2)

	// Should contain prefix and ID
	assert.Contains(t, eventID1, "trace-")
	assert.Contains(t, eventID1, "test-id-123")
}

func TestMissionTracer_Close(t *testing.T) {
	server := newMockLangfuseServer()
	defer server.Close()

	tracer, err := NewMissionTracer(LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)

	// Close should succeed
	err = tracer.Close()
	assert.NoError(t, err)

	// Should be safe to close multiple times
	err = tracer.Close()
	assert.NoError(t, err)
}

func TestMissionTracer_ContextCancellation(t *testing.T) {
	server := newMockLangfuseServer()
	defer server.Close()

	tracer, err := NewMissionTracer(LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)
	defer tracer.Close()

	mission := schema.NewMission(
		types.NewID(),
		"Test Mission",
		"Description",
		"Objective",
		"target",
		"yaml",
	)

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = tracer.StartMissionTrace(ctx, mission)

	// Should fail due to cancelled context
	assert.Error(t, err)
}
