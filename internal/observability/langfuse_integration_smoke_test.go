package observability

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestLangfuseIntegration_SmokeTest is a minimal smoke test to validate the integration test
// patterns compile and work. This is separated from the full integration test in daemon
// to avoid pre-existing build errors in that package.
func TestLangfuseIntegration_SmokeTest(t *testing.T) {
	// Start mock server
	server := newMockLangfuseServer()
	defer server.Close()

	// Create tracer
	tracer, err := NewMissionTracer(LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)
	defer tracer.Close()

	// Create mission
	missionID := types.NewID()
	mission := schema.NewMission(
		missionID,
		"Smoke Test Mission",
		"Smoke test for integration patterns",
		"Validate basic flow",
		"target-smoke",
		"yaml: smoke",
	)

	ctx := context.Background()

	// Create adapter
	adapter, err := NewDecisionLogWriterAdapter(ctx, tracer, mission)
	require.NoError(t, err)
	require.NotNil(t, adapter)

	time.Sleep(50 * time.Millisecond)
	server.reset()

	// Simulate agent execution
	agentExec := schema.NewAgentExecution("node-1", missionID)
	agentExec.WithConfig(map[string]any{"test": true})
	agentExec.WithResult(map[string]any{"status": "ok"})
	now := time.Now()
	agentExec.CompletedAt = &now
	agentExec.Status = schema.ExecutionStatusCompleted

	// Create agent log for tool execution parent
	agentLog := &AgentExecutionLog{
		Execution:   agentExec,
		AgentName:   "smoke-agent",
		Config:      make(map[string]any),
		Neo4jNodeID: "neo4j-smoke",
		SpanID:      fmt.Sprintf("agent-exec-%s", agentExec.ID.String()),
	}

	// Log agent execution via tracer directly
	err = tracer.LogAgentExecution(ctx, adapter.trace, agentLog)
	require.NoError(t, err)

	// Log tool execution
	toolExec := schema.NewToolExecution(agentExec.ID, "smoke-tool")
	toolExec.WithInput(map[string]any{"test": true})
	toolExec.WithOutput(map[string]any{"result": "pass"})
	toolNow := time.Now()
	toolExec.CompletedAt = &toolNow
	toolExec.Status = schema.ExecutionStatusCompleted

	toolLog := &ToolExecutionLog{
		Execution:   toolExec,
		Neo4jNodeID: "neo4j-tool-smoke",
	}

	err = tracer.LogToolExecution(ctx, agentLog, toolLog)
	require.NoError(t, err)

	// Close with summary
	summary := &MissionTraceSummary{
		Status:   schema.MissionStatusCompleted.String(),
		Duration: 1 * time.Minute,
		Outcome:  "Smoke test passed",
	}

	err = adapter.Close(ctx, summary)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify events
	events := server.getEvents()
	assert.Greater(t, len(events), 0, "should have captured events")

	// Verify we have span events
	spanCount := 0
	for _, event := range events {
		if event["type"] == "span-create" {
			spanCount++
		}
	}
	assert.GreaterOrEqual(t, spanCount, 2, "should have at least 2 spans (agent, tool)")

	t.Logf("Smoke test complete: captured %d events with %d spans", len(events), spanCount)
}
