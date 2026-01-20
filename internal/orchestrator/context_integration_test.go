package orchestrator_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/orchestrator"
	"github.com/zero-day-ai/gibson/internal/payload"
	"github.com/zero-day-ai/gibson/internal/types"
)

// setupTestDB creates a temporary database for testing
func setupTestDB(t *testing.T) (*database.DB, func()) {
	t.Helper()

	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "gibson-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")

	// Open database
	db, err := database.Open(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to open database: %v", err)
	}

	// Initialize schema and run migrations
	if err := db.InitSchema(); err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to initialize schema: %v", err)
	}

	// Cleanup function
	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

// TestPayloadContextInjection_Integration tests the full payload context injection flow
func TestPayloadContextInjection_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()

	// Setup: Create test database
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create payload store
	payloadStore := payload.NewPayloadStore(db)

	// Create and save test payloads with different categories and target types
	testPayloads := []payload.Payload{
		payload.NewPayload("jailbreak-1", "Ignore previous instructions", payload.CategoryJailbreak).
			WithDescription("Basic jailbreak payload").
			WithTargetTypes("openai", "anthropic").
			WithSeverity(agent.SeverityHigh).
			WithSuccessIndicators(
				payload.SuccessIndicator{
					Type:   payload.IndicatorContains,
					Value:  "success",
					Weight: 1.0,
				},
			),
		payload.NewPayload("jailbreak-2", "DAN prompt", payload.CategoryJailbreak).
			WithDescription("DAN jailbreak technique").
			WithTargetTypes("openai").
			WithSeverity(agent.SeverityCritical).
			WithSuccessIndicators(
				payload.SuccessIndicator{
					Type:   payload.IndicatorContains,
					Value:  "DAN",
					Weight: 1.0,
				},
			),
		payload.NewPayload("prompt-injection-1", "SQL injection style prompt", payload.CategoryPromptInjection).
			WithDescription("Prompt injection payload").
			WithTargetTypes("openai", "anthropic", "rag").
			WithSeverity(agent.SeverityMedium).
			WithSuccessIndicators(
				payload.SuccessIndicator{
					Type:   payload.IndicatorContains,
					Value:  "injected",
					Weight: 1.0,
				},
			),
		payload.NewPayload("data-extraction-1", "Extract training data", payload.CategoryDataExtraction).
			WithDescription("Data extraction payload").
			WithTargetTypes("openai", "anthropic").
			WithSeverity(agent.SeverityHigh).
			WithSuccessIndicators(
				payload.SuccessIndicator{
					Type:   payload.IndicatorContains,
					Value:  "extracted",
					Weight: 1.0,
				},
			),
	}

	// Save all payloads
	for _, p := range testPayloads {
		err := payloadStore.Save(ctx, &p)
		require.NoError(t, err, "failed to save payload: %s", p.Name)
	}

	t.Run("InjectPayloadContext_WithTargetType", func(t *testing.T) {
		// Create context injector
		injector := orchestrator.NewPayloadContextInjector(payloadStore)

		// Create observation state
		state := &orchestrator.ObservationState{
			MissionInfo: orchestrator.MissionInfo{
				ID:        types.NewID().String(),
				Name:      "Test Mission",
				Objective: "Test payload context injection",
			},
		}

		// Inject context for OpenAI target type
		err := injector.InjectPayloadContext(ctx, state, "openai")
		require.NoError(t, err, "failed to inject payload context")

		// Verify payload context was injected
		assert.NotEmpty(t, state.PayloadContext, "payload context should not be empty")
		assert.Contains(t, state.PayloadContext, "Available Payloads", "should contain payload header")
		assert.Contains(t, state.PayloadContext, "jailbreak", "should mention jailbreak category")
		assert.Contains(t, state.PayloadContext, "prompt_injection", "should mention prompt injection category")
		assert.Contains(t, state.PayloadContext, "data_extraction", "should mention data extraction category")
		assert.Contains(t, state.PayloadContext, "payload_search", "should mention payload_search tool")
		assert.Contains(t, state.PayloadContext, "payload_execute", "should mention payload_execute tool")
	})

	t.Run("InjectPayloadContext_EmptyTargetType", func(t *testing.T) {
		// Empty target type should return all payloads (those with empty TargetTypes array)
		injector := orchestrator.NewPayloadContextInjector(payloadStore)

		state := &orchestrator.ObservationState{
			MissionInfo: orchestrator.MissionInfo{
				ID:        types.NewID().String(),
				Name:      "Test Mission",
				Objective: "Test with empty target type",
			},
		}

		err := injector.InjectPayloadContext(ctx, state, "")
		require.NoError(t, err, "failed to inject payload context with empty target type")

		// Should still inject context with all enabled payloads
		assert.NotEmpty(t, state.PayloadContext, "payload context should not be empty even with empty target type")
	})

	t.Run("InjectPayloadContext_NoMatchingPayloads", func(t *testing.T) {
		// Create injector
		injector := orchestrator.NewPayloadContextInjector(payloadStore)

		state := &orchestrator.ObservationState{
			MissionInfo: orchestrator.MissionInfo{
				ID:        types.NewID().String(),
				Name:      "Test Mission",
				Objective: "Test with no matching payloads",
			},
		}

		// Use a target type that doesn't match any payloads
		err := injector.InjectPayloadContext(ctx, state, "nonexistent-target-type")
		require.NoError(t, err, "should not error even when no payloads match")

		// Context should be empty when no payloads match
		assert.Empty(t, state.PayloadContext, "payload context should be empty when no payloads match")
	})

	t.Run("InjectPayloadContext_NilState", func(t *testing.T) {
		injector := orchestrator.NewPayloadContextInjector(payloadStore)

		// Should return error for nil state
		err := injector.InjectPayloadContext(ctx, nil, "openai")
		assert.Error(t, err, "should error on nil observation state")
		assert.Contains(t, err.Error(), "cannot be nil", "error should mention nil state")
	})

	t.Run("BuildObservationPrompt_WithPayloadContext", func(t *testing.T) {
		// Create context injector
		injector := orchestrator.NewPayloadContextInjector(payloadStore)

		// Create observation state
		state := &orchestrator.ObservationState{
			MissionInfo: orchestrator.MissionInfo{
				ID:        types.NewID().String(),
				Name:      "Test Mission",
				Objective: "Test prompt building with payload context",
			},
			GraphSummary: orchestrator.GraphSummary{
				TotalNodes:     5,
				CompletedNodes: 2,
				FailedNodes:    0,
			},
			ReadyNodes: []orchestrator.NodeSummary{
				{
					ID:   "node-1",
					Type: "agent",
					Name: "Test Agent",
				},
			},
			ResourceConstraints: orchestrator.ResourceConstraints{
				MaxConcurrent: 10,
			},
		}

		// Inject payload context
		err := injector.InjectPayloadContext(ctx, state, "openai")
		require.NoError(t, err, "failed to inject payload context")

		// Build the observation prompt (this is what gets sent to the LLM)
		prompt := orchestrator.BuildObservationPrompt(state)

		// Verify prompt includes payload context
		assert.NotEmpty(t, prompt, "prompt should not be empty")
		assert.Contains(t, prompt, "Available Payloads", "prompt should include payload section")
		assert.Contains(t, prompt, "jailbreak", "prompt should mention jailbreak payloads")
		assert.Contains(t, prompt, "payload_search", "prompt should instruct use of payload_search tool")
		assert.Contains(t, prompt, "payload_execute", "prompt should instruct use of payload_execute tool")
	})
}

// TestPayloadSummary_Integration tests the payload summary generation
func TestPayloadSummary_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()

	// Setup: Create test database
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create payload store
	payloadStore := payload.NewPayloadStore(db)

	// Create test payloads
	testPayloads := []payload.Payload{
		payload.NewPayload("p1", "test1", payload.CategoryJailbreak).
			WithTargetTypes("openai").
			WithSeverity(agent.SeverityHigh).
			WithSuccessIndicators(
				payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "test", Weight: 1.0},
			),
		payload.NewPayload("p2", "test2", payload.CategoryJailbreak, payload.CategoryPromptInjection).
			WithTargetTypes("openai").
			WithSeverity(agent.SeverityCritical).
			WithSuccessIndicators(
				payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "test", Weight: 1.0},
			),
		payload.NewPayload("p3", "test3", payload.CategoryDataExtraction).
			WithTargetTypes("anthropic").
			WithSeverity(agent.SeverityMedium).
			WithSuccessIndicators(
				payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "test", Weight: 1.0},
			),
	}

	for _, p := range testPayloads {
		err := payloadStore.Save(ctx, &p)
		require.NoError(t, err, "failed to save payload")
	}

	t.Run("GetSummaryForTargetType_OpenAI", func(t *testing.T) {
		summary, err := payloadStore.GetSummaryForTargetType(ctx, "openai")
		require.NoError(t, err, "failed to get summary")

		// Should return 2 payloads for openai
		assert.Equal(t, 2, summary.Total, "should have 2 payloads for openai")
		assert.Equal(t, 2, summary.ByCategory[payload.CategoryJailbreak], "should have 2 jailbreak payloads")
		assert.Equal(t, 1, summary.ByCategory[payload.CategoryPromptInjection], "should have 1 prompt injection payload")
		assert.Equal(t, 1, summary.BySeverity[agent.SeverityHigh], "should have 1 high severity payload")
		assert.Equal(t, 1, summary.BySeverity[agent.SeverityCritical], "should have 1 critical severity payload")
	})

	t.Run("GetSummaryForTargetType_Anthropic", func(t *testing.T) {
		summary, err := payloadStore.GetSummaryForTargetType(ctx, "anthropic")
		require.NoError(t, err, "failed to get summary")

		// Should return 1 payload for anthropic
		assert.Equal(t, 1, summary.Total, "should have 1 payload for anthropic")
		assert.Equal(t, 1, summary.ByCategory[payload.CategoryDataExtraction], "should have 1 data extraction payload")
		assert.Equal(t, 1, summary.BySeverity[agent.SeverityMedium], "should have 1 medium severity payload")
	})

	t.Run("GetSummaryForTargetType_Empty", func(t *testing.T) {
		summary, err := payloadStore.GetSummaryForTargetType(ctx, "nonexistent")
		require.NoError(t, err, "should not error for nonexistent target type")

		// Should return empty summary
		assert.Equal(t, 0, summary.Total, "should have 0 payloads for nonexistent target type")
	})
}

// TestEndToEnd_PayloadContextInjection tests the full orchestrator with payload context
func TestEndToEnd_PayloadContextInjection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()

	// Setup: Create test database
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create payload store
	payloadStore := payload.NewPayloadStore(db)

	var err error

	// Create test payload
	testPayload := payload.NewPayload("test-jailbreak", "Test jailbreak", payload.CategoryJailbreak).
		WithDescription("Test payload for integration").
		WithTargetTypes("test-target").
		WithSeverity(agent.SeverityHigh).
		WithSuccessIndicators(
			payload.SuccessIndicator{
				Type:   payload.IndicatorContains,
				Value:  "success",
				Weight: 1.0,
			},
		)

	err = payloadStore.Save(ctx, &testPayload)
	require.NoError(t, err, "failed to save test payload")

	// Create context injector
	injector := orchestrator.NewPayloadContextInjector(payloadStore)

	// Create observation state (simulating orchestrator observation)
	state := &orchestrator.ObservationState{
		MissionInfo: orchestrator.MissionInfo{
			ID:          types.NewID().String(),
			Name:        "End-to-End Test",
			Objective:   "Test full payload context injection flow",
			Status:      "running",
			TimeElapsed: "5m",
		},
		GraphSummary: orchestrator.GraphSummary{
			TotalNodes:     3,
			CompletedNodes: 1,
			FailedNodes:    0,
		},
		ReadyNodes: []orchestrator.NodeSummary{
			{
				ID:        "node-1",
				Type:      "agent",
				Name:      "Security Test Agent",
				AgentName: "test-agent",
			},
		},
		ResourceConstraints: orchestrator.ResourceConstraints{
			MaxConcurrent:   5,
			CurrentRunning:  0,
			TotalIterations: 2,
		},
	}

	// Inject payload context
	err = injector.InjectPayloadContext(ctx, state, "test-target")
	require.NoError(t, err, "failed to inject payload context")

	// Verify the context was injected
	assert.NotEmpty(t, state.PayloadContext, "payload context should be populated")
	assert.Contains(t, state.PayloadContext, "1 attack payload", "should report 1 payload available")
	assert.Contains(t, state.PayloadContext, "jailbreak", "should mention jailbreak category")

	// Build full prompt as orchestrator would
	prompt := orchestrator.BuildObservationPrompt(state)

	// Verify the prompt includes everything needed for the LLM
	assert.NotEmpty(t, prompt, "prompt should not be empty")
	assert.Contains(t, prompt, "Test full payload context injection flow", "should include mission objective")
	assert.Contains(t, prompt, "Available Payloads", "should include payload section")
	assert.Contains(t, prompt, "Ready Nodes", "should include ready nodes section")
	assert.Contains(t, prompt, "payload_search", "should instruct on payload search")
	assert.Contains(t, prompt, "payload_execute", "should instruct on payload execution")

	t.Logf("Generated prompt length: %d characters", len(prompt))
	t.Logf("Payload context section:\n%s", state.PayloadContext)
}
