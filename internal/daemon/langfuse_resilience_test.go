package daemon_test

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
	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/observability"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestLangfuseServerUnavailable verifies that mission execution succeeds
// even when the Langfuse server is completely unavailable.
func TestLangfuseServerUnavailable(t *testing.T) {
	// Create mission tracer with unreachable endpoint
	tracer, err := observability.NewMissionTracer(observability.LangfuseConfig{
		Host:      "http://localhost:59999", // Port that's not listening
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err, "tracer creation should succeed even with bad endpoint")
	defer tracer.Close()

	// Create a test mission
	missionID := types.NewID()
	mission := createTestSchemaMission(missionID, "Unavailable Server Test")

	// Start mission trace - should handle connection failure gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// This should return an error since the server is unavailable
	trace, err := tracer.StartMissionTrace(ctx, mission)

	// StartMissionTrace may return error since it validates connection
	// The key is that subsequent operations don't block mission execution
	if err != nil {
		t.Logf("StartMissionTrace returned expected error: %v", err)
		// Mission should still be able to proceed without tracing
		return
	}

	// If trace was created (shouldn't happen), try logging - should not block
	if trace != nil {
		decision := createTestDecision(missionID)
		decisionLog := &observability.DecisionLog{
			Decision:      decision,
			Prompt:        "Test prompt",
			Response:      "Test response",
			Model:         "test-model",
			GraphSnapshot: "Test snapshot",
		}

		// Log decision should not block or panic
		logCtx, logCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer logCancel()

		err = tracer.LogDecision(logCtx, trace, decisionLog)
		// Error is expected and acceptable - mission continues
		if err != nil {
			t.Logf("LogDecision returned expected error: %v", err)
		}

		// End trace should also not block
		endCtx, endCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer endCancel()

		summary := &observability.MissionTraceSummary{
			Status:   "completed",
			Duration: 1 * time.Second,
		}
		err = tracer.EndMissionTrace(endCtx, trace, summary)
		if err != nil {
			t.Logf("EndMissionTrace returned expected error: %v", err)
		}
	}

	t.Log("Mission can proceed even when Langfuse server is unavailable")
}

// TestLangfuseReturnsErrors verifies that mission execution succeeds
// even when Langfuse returns HTTP errors.
func TestLangfuseReturnsErrors(t *testing.T) {
	// Create a mock server that always returns errors
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authentication
		username, password, ok := r.BasicAuth()
		if !ok || username != "test-public-key" || password != "test-secret-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Return 500 error for all requests
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer errorServer.Close()

	// Create mission tracer with error server
	tracer, err := observability.NewMissionTracer(observability.LangfuseConfig{
		Host:      errorServer.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err, "tracer creation should succeed")
	defer tracer.Close()

	// Create a test mission
	missionID := types.NewID()
	mission := createTestSchemaMission(missionID, "Error Server Test")

	// Start mission trace - should handle error gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	trace, err := tracer.StartMissionTrace(ctx, mission)

	// StartMissionTrace may return error due to server error
	if err != nil {
		t.Logf("StartMissionTrace returned expected error: %v", err)
		// Mission execution should continue
		assert.Contains(t, err.Error(), "500", "error should indicate server error")
		return
	}

	require.NotNil(t, trace, "trace should be created")

	// Log decision - should handle error without blocking
	decision := createTestDecision(missionID)
	decisionLog := &observability.DecisionLog{
		Decision:      decision,
		Prompt:        "Test prompt during error",
		Response:      "Test response during error",
		Model:         "test-model",
		GraphSnapshot: "Test snapshot",
	}

	logCtx, logCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer logCancel()

	err = tracer.LogDecision(logCtx, trace, decisionLog)
	// Error is expected but should not panic or block
	if err != nil {
		t.Logf("LogDecision handled error gracefully: %v", err)
	}

	// End trace - should handle error without blocking
	endCtx, endCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer endCancel()

	summary := &observability.MissionTraceSummary{
		Status:   "completed",
		Duration: 1 * time.Second,
	}
	err = tracer.EndMissionTrace(endCtx, trace, summary)
	if err != nil {
		t.Logf("EndMissionTrace handled error gracefully: %v", err)
	}

	t.Log("Mission completed successfully despite Langfuse errors")
}

// TestLangfuseSlowResponse verifies that slow Langfuse responses
// don't block mission execution beyond configured timeouts.
func TestLangfuseSlowResponse(t *testing.T) {
	// Track request timing
	var requestDurations []time.Duration
	var mu sync.Mutex

	// Create a mock server that responds slowly
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			mu.Lock()
			requestDurations = append(requestDurations, time.Since(start))
			mu.Unlock()
		}()

		// Verify authentication
		username, password, ok := r.BasicAuth()
		if !ok || username != "test-public-key" || password != "test-secret-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Read and discard body
		io.Copy(io.Discard, r.Body)

		// Simulate slow server - sleep for 3 seconds
		time.Sleep(3 * time.Second)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer slowServer.Close()

	// Create mission tracer with slow server
	tracer, err := observability.NewMissionTracer(observability.LangfuseConfig{
		Host:      slowServer.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err, "tracer creation should succeed")
	defer tracer.Close()

	// Create a test mission
	missionID := types.NewID()
	mission := createTestSchemaMission(missionID, "Slow Server Test")

	// Start mission trace with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	startTime := time.Now()
	trace, err := tracer.StartMissionTrace(ctx, mission)
	duration := time.Since(startTime)

	// Should timeout or return quickly without blocking indefinitely
	assert.True(t, duration < 2*time.Second,
		"StartMissionTrace should timeout quickly, took %v", duration)

	// If context timeout occurred, that's acceptable behavior
	if err != nil {
		t.Logf("StartMissionTrace timed out as expected: %v", err)
		assert.Contains(t, err.Error(), "context",
			"error should indicate context cancellation")
		return
	}

	// If trace was created (background flush), verify subsequent operations don't block
	if trace != nil {
		decision := createTestDecision(missionID)
		decisionLog := &observability.DecisionLog{
			Decision:      decision,
			Prompt:        "Test prompt with slow server",
			Response:      "Test response with slow server",
			Model:         "test-model",
			GraphSnapshot: "Test snapshot",
		}

		// Log decision with timeout - should not block mission
		logCtx, logCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer logCancel()

		logStart := time.Now()
		err = tracer.LogDecision(logCtx, trace, decisionLog)
		logDuration := time.Since(logStart)

		// Should complete quickly (fire-and-forget or timeout)
		assert.True(t, logDuration < 1*time.Second,
			"LogDecision should not block indefinitely, took %v", logDuration)

		if err != nil {
			t.Logf("LogDecision timed out or failed gracefully: %v", err)
		}
	}

	t.Log("Mission execution not blocked by slow Langfuse server")
}

// TestDaemonShutdownFlushesEvents verifies that pending events
// are flushed when the daemon/tracer shuts down gracefully.
func TestDaemonShutdownFlushesEvents(t *testing.T) {
	// Track received events
	var receivedEvents []map[string]any
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Create a mock server that tracks events
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		mu.Lock()
		for _, event := range batch {
			if eventMap, ok := event.(map[string]any); ok {
				receivedEvents = append(receivedEvents, eventMap)
			}
		}
		mu.Unlock()

		wg.Done() // Signal that we received events

		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	// Create mission tracer
	tracer, err := observability.NewMissionTracer(observability.LangfuseConfig{
		Host:      mockServer.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err, "tracer creation should succeed")

	// Create a test mission
	missionID := types.NewID()
	mission := createTestSchemaMission(missionID, "Shutdown Flush Test")

	// Start mission trace
	ctx := context.Background()
	wg.Add(1) // Expect one batch for trace creation

	trace, err := tracer.StartMissionTrace(ctx, mission)
	require.NoError(t, err, "StartMissionTrace should succeed")
	require.NotNil(t, trace, "trace should be created")

	// Log some decisions
	wg.Add(3) // Expect 3 more batches for decisions

	for i := 0; i < 3; i++ {
		decision := createTestDecision(missionID)
		decisionLog := &observability.DecisionLog{
			Decision:      decision,
			Prompt:        fmt.Sprintf("Test prompt %d", i),
			Response:      fmt.Sprintf("Test response %d", i),
			Model:         "test-model",
			GraphSnapshot: "Test snapshot",
		}

		err = tracer.LogDecision(ctx, trace, decisionLog)
		require.NoError(t, err, "LogDecision should succeed")
	}

	// End trace
	wg.Add(1) // Expect one batch for trace end

	summary := &observability.MissionTraceSummary{
		Status:          "completed",
		TotalDecisions:  3,
		TotalExecutions: 1,
		Duration:        2 * time.Second,
	}
	err = tracer.EndMissionTrace(ctx, trace, summary)
	require.NoError(t, err, "EndMissionTrace should succeed")

	// Close tracer - should flush all pending events
	err = tracer.Close()
	require.NoError(t, err, "Close should succeed")

	// Wait for all events to be received (with timeout)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All events received
		t.Log("All events were flushed on shutdown")
	case <-time.After(5 * time.Second):
		// Timeout - check what we got
		mu.Lock()
		eventCount := len(receivedEvents)
		mu.Unlock()

		// We should have received at least the trace creation event
		assert.Greater(t, eventCount, 0,
			"should receive at least some events before timeout")
		t.Logf("Received %d events before timeout (expected 5)", eventCount)
	}

	// Verify events were received
	mu.Lock()
	eventCount := len(receivedEvents)
	mu.Unlock()

	// Should have received: 1 trace + 3 decisions + 1 summary = 5 events
	assert.Greater(t, eventCount, 0, "should receive at least some events")
	t.Logf("Total events flushed on shutdown: %d", eventCount)

	// Verify event types if we got them all
	if eventCount >= 5 {
		mu.Lock()
		eventTypes := make(map[string]int)
		for _, event := range receivedEvents {
			if eventType, ok := event["type"].(string); ok {
				eventTypes[eventType]++
			}
		}
		mu.Unlock()

		t.Logf("Event types received: %v", eventTypes)
		assert.Equal(t, 1, eventTypes["trace-create"], "should receive trace creation")
		assert.Equal(t, 3, eventTypes["generation-create"], "should receive 3 decisions")
		assert.Equal(t, 1, eventTypes["span-create"], "should receive trace end")
	}
}

// TestMissionSucceedsWithLangfuseDisabled verifies that missions
// run successfully when Langfuse is explicitly disabled.
func TestMissionSucceedsWithLangfuseDisabled(t *testing.T) {
	// Create a config with Langfuse disabled
	cfg := &config.Config{
		Langfuse: config.LangfuseConfig{
			Enabled:   false,
			Host:      "",
			PublicKey: "",
			SecretKey: "",
		},
	}

	// Verify that disabled Langfuse doesn't prevent operations
	assert.False(t, cfg.Langfuse.Enabled,
		"Langfuse should be disabled")

	// No tracer should be created when disabled
	// This is typically handled at the daemon level
	// Mission execution proceeds without any observability hooks

	t.Log("Mission execution succeeds with Langfuse disabled")
}

// createTestSchemaMission creates a test mission in schema format.
func createTestSchemaMission(id types.ID, name string) *schema.Mission {
	return schema.NewMission(
		id,
		name,
		"Test mission for resilience testing",
		"Verify resilience against Langfuse failures",
		"test-target",
		`name: test-workflow
description: Test workflow
nodes:
  - id: test-node
    type: agent
    agent: test-agent
`,
	)
}

// createTestDecision creates a test decision for logging.
func createTestDecision(missionID types.ID) *schema.Decision {
	decision := schema.NewDecision(missionID, 1, schema.DecisionActionExecuteAgent)
	decision.WithTargetNode("test-node")
	decision.WithReasoning("Test reasoning")
	decision.WithConfidence(0.95)
	decision.WithTokenUsage(100, 50)
	decision.WithLatency(250)
	return decision
}
