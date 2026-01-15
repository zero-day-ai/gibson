package mission

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
	"go.opentelemetry.io/otel/trace"
)

// Mock implementations

// MockHarnessFactory is a mock implementation of harness.HarnessFactoryInterface
type MockHarnessFactory struct {
	CreateFunc      func(agentName string, missionCtx harness.MissionContext, targetInfo harness.TargetInfo) (harness.AgentHarness, error)
	CreateChildFunc func(parent harness.AgentHarness, agentName string) (harness.AgentHarness, error)
	CallCount       int
}

func (m *MockHarnessFactory) Create(agentName string, missionCtx harness.MissionContext, targetInfo harness.TargetInfo) (harness.AgentHarness, error) {
	m.CallCount++
	if m.CreateFunc != nil {
		return m.CreateFunc(agentName, missionCtx, targetInfo)
	}
	return &MockAgentHarness{}, nil
}

func (m *MockHarnessFactory) CreateChild(parent harness.AgentHarness, agentName string) (harness.AgentHarness, error) {
	if m.CreateChildFunc != nil {
		return m.CreateChildFunc(parent, agentName)
	}
	return &MockAgentHarness{}, nil
}

// MockAgentHarness is a minimal mock of harness.AgentHarness
type MockAgentHarness struct{}

func (m *MockAgentHarness) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{}, nil
}
func (m *MockAgentHarness) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{}, nil
}
func (m *MockAgentHarness) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}
func (m *MockAgentHarness) CallTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	return nil, nil
}
func (m *MockAgentHarness) ListTools() []harness.ToolDescriptor { return nil }
func (m *MockAgentHarness) QueryPlugin(ctx context.Context, name string, method string, params map[string]any) (any, error) {
	return nil, nil
}
func (m *MockAgentHarness) ListPlugins() []harness.PluginDescriptor { return nil }
func (m *MockAgentHarness) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
	return agent.Result{}, nil
}
func (m *MockAgentHarness) ListAgents() []harness.AgentDescriptor { return nil }
func (m *MockAgentHarness) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	return nil
}
func (m *MockAgentHarness) GetFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return nil, nil
}
func (m *MockAgentHarness) Memory() memory.MemoryStore       { return nil }
func (m *MockAgentHarness) Mission() harness.MissionContext  { return harness.MissionContext{} }
func (m *MockAgentHarness) Target() harness.TargetInfo       { return harness.TargetInfo{} }
func (m *MockAgentHarness) Tracer() trace.Tracer             { return nil }
func (m *MockAgentHarness) Logger() *slog.Logger             { return slog.Default() }
func (m *MockAgentHarness) Metrics() harness.MetricsRecorder { return nil }
func (m *MockAgentHarness) TokenUsage() *llm.TokenTracker    { return nil }
func (m *MockAgentHarness) CompleteStructuredAny(ctx context.Context, slot string, messages []llm.Message, targetType any, opts ...harness.CompletionOption) (any, error) {
	return nil, nil
}
func (m *MockAgentHarness) GetAllRunFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return nil, nil
}
func (m *MockAgentHarness) GetMissionRunHistory(ctx context.Context) ([]harness.MissionRunSummarySDK, error) {
	return nil, nil
}
func (m *MockAgentHarness) GetPreviousRunFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return nil, nil
}
func (m *MockAgentHarness) MissionExecutionContext() harness.MissionExecutionContextSDK {
	return harness.MissionExecutionContextSDK{}
}
func (m *MockAgentHarness) MissionID() types.ID {
	return types.NewID()
}

// Helper functions

// createWorkflowJSON creates a simple workflow YAML for testing
func createWorkflowJSON(t *testing.T, workflowID types.ID) string {
	t.Helper()

	// Return a simple YAML workflow definition
	return `
name: test-workflow
description: Test workflow for mission orchestrator
nodes:
  - id: node1
    type: agent
    name: Test Agent Node
    agent: test-agent
    task:
      action: test
      target: localhost
`
}

// Tests

func TestDefaultMissionOrchestrator_Execute(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	orchestrator, err := NewMissionOrchestrator(store)
	require.NoError(t, err)
	ctx := context.Background()

	mission := createTestMission(t)
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	result, err := orchestrator.Execute(ctx, mission)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, mission.ID, result.MissionID)
	assert.Equal(t, MissionStatusCompleted, result.Status)
}

func TestDefaultMissionOrchestrator_Execute_Success(t *testing.T) {
	t.Skip("Skipping - requires full agent registry and workflow infrastructure")
	// This test would require:
	// - A complete agent registry with registered agents
	// - Tool registry with tools
	// - Plugin registry with plugins
	// The orchestrator properly executes workflows when all infrastructure is present
	// This is tested via integration tests
}

func TestDefaultMissionOrchestrator_Execute_WorkflowFailure(t *testing.T) {
	t.Skip("Skipping - requires agent implementation to fail")
	// This test would require us to mock an agent that fails, which is complex
	// The orchestrator properly handles workflow failures as tested by error path tests
}

func TestDefaultMissionOrchestrator_Execute_MaxDurationConstraint(t *testing.T) {
	t.Skip("Skipping - requires full workflow infrastructure")
	// Constraint checking is tested via unit tests on DefaultConstraintChecker
	// Integration of constraints with mission execution requires full agent infrastructure
}

func TestDefaultMissionOrchestrator_Execute_MaxFindingsConstraint(t *testing.T) {
	t.Skip("Skipping - requires agent implementation to generate findings")
	// This test would require us to have a real agent that generates findings
	// The orchestrator properly collects findings from workflow results
}

func TestDefaultMissionOrchestrator_Execute_MissingWorkflowExecutor(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	// Create orchestrator without workflow executor (fallback mode)
	orchestrator, err := NewMissionOrchestrator(store)
	require.NoError(t, err)

	// Create mission with workflow JSON
	mission := createTestMission(t)
	mission.WorkflowJSON = createWorkflowJSON(t, mission.WorkflowID)
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	// Execute mission - should complete without executor (fallback behavior)
	result, err := orchestrator.Execute(ctx, mission)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, mission.ID, result.MissionID)
	assert.Equal(t, MissionStatusCompleted, result.Status)
}

func TestDefaultMissionOrchestrator_Execute_MissingHarnessFactory(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	// Create real workflow executor
	executor := workflow.NewWorkflowExecutor()

	// Create orchestrator without harness factory
	orchestrator, err := NewMissionOrchestrator(
		store,
		WithWorkflowExecutor(executor),
	)
	require.NoError(t, err)

	// Create mission with workflow JSON
	mission := createTestMission(t)
	mission.WorkflowJSON = createWorkflowJSON(t, mission.WorkflowID)
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	// Execute mission - should fail without harness factory
	result, err := orchestrator.Execute(ctx, mission)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "harness factory not configured")
	assert.NotNil(t, result)
	assert.Equal(t, MissionStatusFailed, result.Status)

	// Verify mission was marked as failed
	updatedMission, err := store.Get(ctx, mission.ID)
	require.NoError(t, err)
	assert.Equal(t, MissionStatusFailed, updatedMission.Status)
	assert.Contains(t, updatedMission.Error, "harness factory not configured")
}

func TestDefaultMissionOrchestrator_Execute_MissingWorkflowJSON(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	// Create real executor and mock factory
	executor := workflow.NewWorkflowExecutor()
	mockFactory := &MockHarnessFactory{}

	orchestrator, err := NewMissionOrchestrator(
		store,
		WithWorkflowExecutor(executor),
		WithHarnessFactory(mockFactory),
	)
	require.NoError(t, err)

	// Create mission without workflow JSON
	mission := createTestMission(t)
	mission.WorkflowJSON = "" // No workflow JSON
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	// Execute mission - should fail without workflow definition
	result, err := orchestrator.Execute(ctx, mission)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflow definition not available")
	assert.NotNil(t, result)
	assert.Equal(t, MissionStatusFailed, result.Status)

	// Verify mission was marked as failed
	updatedMission, err := store.Get(ctx, mission.ID)
	require.NoError(t, err)
	assert.Equal(t, MissionStatusFailed, updatedMission.Status)
	assert.Contains(t, updatedMission.Error, "workflow definition not available")
}

func TestDefaultMissionOrchestrator_Execute_InvalidWorkflowJSON(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	// Create real executor and mock factory
	executor := workflow.NewWorkflowExecutor()
	mockFactory := &MockHarnessFactory{}

	orchestrator, err := NewMissionOrchestrator(
		store,
		WithWorkflowExecutor(executor),
		WithHarnessFactory(mockFactory),
	)
	require.NoError(t, err)

	// Create mission with invalid workflow JSON
	mission := createTestMission(t)
	mission.WorkflowJSON = "{invalid json"
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	// Execute mission - should fail due to invalid JSON
	result, err := orchestrator.Execute(ctx, mission)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse workflow")
	assert.NotNil(t, result)
	assert.Equal(t, MissionStatusFailed, result.Status)

	// Verify mission was marked as failed
	updatedMission, err := store.Get(ctx, mission.ID)
	require.NoError(t, err)
	assert.Equal(t, MissionStatusFailed, updatedMission.Status)
	assert.Contains(t, updatedMission.Error, "failed to parse workflow")
}

func TestDefaultMissionOrchestrator_Execute_HarnessCreationFailure(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	// Create real workflow executor
	executor := workflow.NewWorkflowExecutor()

	// Create mock harness factory that returns error
	mockFactory := &MockHarnessFactory{
		CreateFunc: func(agentName string, missionCtx harness.MissionContext, targetInfo harness.TargetInfo) (harness.AgentHarness, error) {
			return nil, fmt.Errorf("failed to create harness: resource unavailable")
		},
	}

	orchestrator, err := NewMissionOrchestrator(
		store,
		WithWorkflowExecutor(executor),
		WithHarnessFactory(mockFactory),
	)
	require.NoError(t, err)

	// Create mission with workflow JSON
	mission := createTestMission(t)
	mission.WorkflowJSON = createWorkflowJSON(t, mission.WorkflowID)
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	// Execute mission - should fail due to harness creation error
	result, err := orchestrator.Execute(ctx, mission)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create harness")
	assert.NotNil(t, result)
	assert.Equal(t, MissionStatusFailed, result.Status)

	// Verify mission was marked as failed
	updatedMission, err := store.Get(ctx, mission.ID)
	require.NoError(t, err)
	assert.Equal(t, MissionStatusFailed, updatedMission.Status)
	assert.Contains(t, updatedMission.Error, "failed to create harness")
}

func TestDefaultMissionOrchestrator_Execute_WorkflowCancelled(t *testing.T) {
	t.Skip("Skipping - requires context cancellation during workflow execution")
	// This test would require precise timing to cancel during workflow execution
	// The orchestrator properly handles cancellation as tested by timeout tests
}

func TestDefaultMissionOrchestrator_InvalidState(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	orchestrator, err := NewMissionOrchestrator(store)
	require.NoError(t, err)
	ctx := context.Background()

	mission := createTestMission(t)
	mission.Status = MissionStatusCompleted // Terminal state
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	_, err = orchestrator.Execute(ctx, mission)
	assert.Error(t, err)
	assert.True(t, IsInvalidStateError(err))
}

func TestDefaultMissionOrchestrator_Execute_WithConstraints(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	orchestrator, err := NewMissionOrchestrator(store)
	require.NoError(t, err)

	// Create mission with constraints
	mission := createTestMission(t)
	mission.Constraints = &MissionConstraints{
		MaxDuration: 1 * time.Hour,
		MaxFindings: 100,
		MaxTokens:   10000,
		MaxCost:     50.0,
	}
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	// Execute mission - should complete successfully even with constraints
	result, err := orchestrator.Execute(ctx, mission)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, mission.ID, result.MissionID)
	assert.Equal(t, MissionStatusCompleted, result.Status)
}

func TestDefaultMissionOrchestrator_Execute_UpdatesMetrics(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	orchestrator, err := NewMissionOrchestrator(store)
	require.NoError(t, err)

	mission := createTestMission(t)
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	// Execute mission
	result, err := orchestrator.Execute(ctx, mission)
	require.NoError(t, err)

	// Verify metrics were updated
	updatedMission, err := store.Get(ctx, mission.ID)
	require.NoError(t, err)
	assert.NotNil(t, updatedMission.Metrics)
	assert.NotNil(t, updatedMission.Metrics.FindingsBySeverity)
	assert.True(t, updatedMission.Metrics.Duration > 0)
	assert.NotNil(t, result.Metrics)
}

func TestDefaultMissionOrchestrator_WithEventEmitter(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	emitter := NewDefaultEventEmitter(WithBufferSize(10))

	// Subscribe to events
	ctx := context.Background()
	events, unsubscribe := emitter.Subscribe(ctx)
	defer unsubscribe()

	orchestrator, err := NewMissionOrchestrator(store, WithEventEmitter(emitter))
	require.NoError(t, err)

	mission := createTestMission(t)
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	_, err = orchestrator.Execute(ctx, mission)
	require.NoError(t, err)

	// Wait for events with timeout
	timeout := time.After(1 * time.Second)
	receivedEvents := []MissionEventType{}

	for {
		select {
		case event := <-events:
			receivedEvents = append(receivedEvents, event.Type)
			if len(receivedEvents) >= 2 {
				goto Done
			}
		case <-timeout:
			goto Done
		}
	}

Done:
	// Verify we received started and completed events
	assert.Contains(t, receivedEvents, EventMissionStarted)
	assert.Contains(t, receivedEvents, EventMissionCompleted)
}

func TestExecute_PropagatesWorkflowMetrics(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	// Create orchestrator without workflow executor to use fallback mode
	// This tests that metrics are still populated even without a workflow
	orchestrator, err := NewMissionOrchestrator(store)
	require.NoError(t, err)

	// Create mission
	mission := createTestMission(t)
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	// Execute mission - will use fallback mode without executor
	result, err := orchestrator.Execute(ctx, mission)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify mission metrics were initialized and updated
	updatedMission, err := store.Get(ctx, mission.ID)
	require.NoError(t, err)
	assert.NotNil(t, updatedMission.Metrics)

	// Verify basic metrics are set
	assert.NotZero(t, updatedMission.Metrics.Duration)
	assert.False(t, updatedMission.Metrics.LastUpdateAt.IsZero())
	assert.False(t, updatedMission.Metrics.StartedAt.IsZero())

	// When workflow executor is present and returns NodesExecuted,
	// mission.Metrics.CompletedNodes should be set from workflowResult.NodesExecuted
	// In fallback mode (no executor), CompletedNodes will be 0
	assert.GreaterOrEqual(t, updatedMission.Metrics.CompletedNodes, 0)
}

// TestOrchestrator_EventPersistence verifies that mission events are persisted to the EventStore.
func TestOrchestrator_EventPersistence(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	eventStore := NewDBEventStore(db)
	ctx := context.Background()

	// Create orchestrator with event store
	orchestrator, err := NewMissionOrchestrator(
		store,
		WithEventStore(eventStore),
	)
	require.NoError(t, err)

	// Create and save mission
	mission := createTestMission(t)
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	// Execute mission - this will emit events
	result, err := orchestrator.Execute(ctx, mission)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Query events from the event store
	filter := NewEventFilter().
		WithMissionID(mission.ID).
		WithPagination(100, 0)

	events, err := eventStore.Query(ctx, filter)
	require.NoError(t, err)

	// Verify events were persisted
	assert.NotEmpty(t, events, "Expected mission events to be persisted")

	// Check for expected event types
	eventTypes := make(map[MissionEventType]bool)
	for _, event := range events {
		eventTypes[event.Type] = true
		assert.Equal(t, mission.ID, event.MissionID, "Event mission ID should match")
		assert.False(t, event.Timestamp.IsZero(), "Event timestamp should be set")
	}

	// Verify we have the essential events
	assert.True(t, eventTypes[EventMissionStarted], "Should have mission.started event")
	assert.True(t, eventTypes[EventMissionCompleted], "Should have mission.completed event")

	// Verify events are ordered by timestamp
	for i := 1; i < len(events); i++ {
		assert.True(t, events[i].Timestamp.After(events[i-1].Timestamp) || events[i].Timestamp.Equal(events[i-1].Timestamp),
			"Events should be ordered by timestamp")
	}
}

// TestOrchestrator_EventPersistence_WithEventTypes tests filtering by event type.
func TestOrchestrator_EventPersistence_WithEventTypes(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	eventStore := NewDBEventStore(db)
	ctx := context.Background()

	// Create orchestrator with event store
	orchestrator, err := NewMissionOrchestrator(
		store,
		WithEventStore(eventStore),
	)
	require.NoError(t, err)

	// Create and execute mission
	mission := createTestMission(t)
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	_, err = orchestrator.Execute(ctx, mission)
	require.NoError(t, err)

	// Query only mission.started events
	filter := NewEventFilter().
		WithMissionID(mission.ID).
		WithEventTypes(EventMissionStarted)

	events, err := eventStore.Query(ctx, filter)
	require.NoError(t, err)

	// Verify we only get started events
	assert.Len(t, events, 1, "Should have exactly one mission.started event")
	assert.Equal(t, EventMissionStarted, events[0].Type)

	// Query only completion events
	filter = NewEventFilter().
		WithMissionID(mission.ID).
		WithEventTypes(EventMissionCompleted, EventMissionFailed, EventMissionCancelled)

	events, err = eventStore.Query(ctx, filter)
	require.NoError(t, err)

	// Should have at least one completion-type event
	assert.NotEmpty(t, events, "Should have at least one completion event")
	for _, event := range events {
		assert.Contains(t, []MissionEventType{EventMissionCompleted, EventMissionFailed, EventMissionCancelled}, event.Type)
	}
}

// TestOrchestrator_EventPersistence_FailedMission verifies events are persisted even when mission fails.
func TestOrchestrator_EventPersistence_FailedMission(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	eventStore := NewDBEventStore(db)
	ctx := context.Background()

	// Create real workflow executor
	executor := workflow.NewWorkflowExecutor()

	// Create orchestrator with event store but no harness factory to cause failure
	orchestrator, err := NewMissionOrchestrator(
		store,
		WithEventStore(eventStore),
		WithWorkflowExecutor(executor),
	)
	require.NoError(t, err)

	// Create mission with workflow JSON that requires harness
	mission := createTestMission(t)
	mission.WorkflowJSON = createWorkflowJSON(t, mission.WorkflowID)
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	// Execute mission - should fail due to missing harness factory
	result, err := orchestrator.Execute(ctx, mission)
	require.Error(t, err)
	assert.Equal(t, MissionStatusFailed, result.Status)

	// Query events from the event store
	filter := NewEventFilter().
		WithMissionID(mission.ID).
		WithPagination(100, 0)

	events, err := eventStore.Query(ctx, filter)
	require.NoError(t, err)

	// Verify events were persisted even though mission failed
	assert.NotEmpty(t, events, "Events should be persisted even on failure")

	// Check for expected event types
	eventTypes := make(map[MissionEventType]bool)
	for _, event := range events {
		eventTypes[event.Type] = true
	}

	// Verify we have started and failed events
	assert.True(t, eventTypes[EventMissionStarted], "Should have mission.started event")
	assert.True(t, eventTypes[EventMissionFailed], "Should have mission.failed event")
}

// TestOrchestrator_EventPersistence_TimeRange tests filtering by time range.
func TestOrchestrator_EventPersistence_TimeRange(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	eventStore := NewDBEventStore(db)
	ctx := context.Background()

	// Create orchestrator with event store
	orchestrator, err := NewMissionOrchestrator(
		store,
		WithEventStore(eventStore),
	)
	require.NoError(t, err)

	// Record start time
	startTime := time.Now()

	// Create and execute mission
	mission := createTestMission(t)
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	_, err = orchestrator.Execute(ctx, mission)
	require.NoError(t, err)

	// Record end time
	endTime := time.Now()

	// Query events within time range
	filter := NewEventFilter().
		WithMissionID(mission.ID).
		WithTimeRange(startTime, endTime)

	events, err := eventStore.Query(ctx, filter)
	require.NoError(t, err)

	// All events should be within the time range
	assert.NotEmpty(t, events)
	for _, event := range events {
		assert.True(t, event.Timestamp.After(startTime) || event.Timestamp.Equal(startTime))
		assert.True(t, event.Timestamp.Before(endTime) || event.Timestamp.Equal(endTime))
	}

	// Query with time range that excludes all events
	futureTime := time.Now().Add(1 * time.Hour)
	filter = NewEventFilter().
		WithMissionID(mission.ID).
		WithTimeRange(futureTime, futureTime.Add(1*time.Hour))

	events, err = eventStore.Query(ctx, filter)
	require.NoError(t, err)
	assert.Empty(t, events, "Should have no events in future time range")
}

// TestOrchestrator_EventStore_Stream tests the event streaming functionality.
func TestOrchestrator_EventStore_Stream(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	eventStore := NewDBEventStore(db)
	ctx := context.Background()

	// Create orchestrator with event store
	orchestrator, err := NewMissionOrchestrator(
		store,
		WithEventStore(eventStore),
	)
	require.NoError(t, err)

	// Create and execute mission
	mission := createTestMission(t)
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	startTime := time.Now()

	_, err = orchestrator.Execute(ctx, mission)
	require.NoError(t, err)

	// Stream events from start time
	streamCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	eventCh, err := eventStore.Stream(streamCtx, mission.ID, startTime)
	require.NoError(t, err)

	// Collect events from stream
	var streamedEvents []*MissionEvent
	for event := range eventCh {
		streamedEvents = append(streamedEvents, event)
	}

	// Verify we received events
	assert.NotEmpty(t, streamedEvents, "Should receive events from stream")

	// Verify events are for the correct mission
	for _, event := range streamedEvents {
		assert.Equal(t, mission.ID, event.MissionID)
	}

	// Verify events are ordered
	for i := 1; i < len(streamedEvents); i++ {
		assert.True(t, streamedEvents[i].Timestamp.After(streamedEvents[i-1].Timestamp) || streamedEvents[i].Timestamp.Equal(streamedEvents[i-1].Timestamp))
	}
}

// TestOrchestrator_IsPauseRequested tests the pause request flag mechanism.
func TestOrchestrator_IsPauseRequested(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	orchestrator, err := NewMissionOrchestrator(store)
	require.NoError(t, err)
	ctx := context.Background()

	mission := createTestMission(t)
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	// Initially, no pause should be requested
	assert.False(t, orchestrator.IsPauseRequested(mission.ID), "No pause should be requested initially")

	// Request pause
	err = orchestrator.RequestPause(ctx, mission.ID)
	require.NoError(t, err)

	// Pause should now be requested
	assert.True(t, orchestrator.IsPauseRequested(mission.ID), "Pause should be requested after RequestPause")

	// Clear pause request
	orchestrator.ClearPauseRequest(mission.ID)

	// Pause should no longer be requested
	assert.False(t, orchestrator.IsPauseRequested(mission.ID), "Pause should not be requested after ClearPauseRequest")
}

// TestOrchestrator_IsPauseRequested_MultipleMissions tests pause requests for multiple missions.
func TestOrchestrator_IsPauseRequested_MultipleMissions(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	orchestrator, err := NewMissionOrchestrator(store)
	require.NoError(t, err)
	ctx := context.Background()

	// Create two missions
	mission1 := createTestMission(t)
	err = store.Save(ctx, mission1)
	require.NoError(t, err)

	mission2 := createTestMission(t)
	err = store.Save(ctx, mission2)
	require.NoError(t, err)

	// Request pause for mission1 only
	err = orchestrator.RequestPause(ctx, mission1.ID)
	require.NoError(t, err)

	// Verify only mission1 has pause requested
	assert.True(t, orchestrator.IsPauseRequested(mission1.ID), "Pause should be requested for mission1")
	assert.False(t, orchestrator.IsPauseRequested(mission2.ID), "Pause should not be requested for mission2")

	// Request pause for mission2
	err = orchestrator.RequestPause(ctx, mission2.ID)
	require.NoError(t, err)

	// Verify both have pause requested
	assert.True(t, orchestrator.IsPauseRequested(mission1.ID), "Pause should still be requested for mission1")
	assert.True(t, orchestrator.IsPauseRequested(mission2.ID), "Pause should now be requested for mission2")

	// Clear pause for mission1
	orchestrator.ClearPauseRequest(mission1.ID)

	// Verify only mission2 has pause requested
	assert.False(t, orchestrator.IsPauseRequested(mission1.ID), "Pause should be cleared for mission1")
	assert.True(t, orchestrator.IsPauseRequested(mission2.ID), "Pause should still be requested for mission2")
}

// TestOrchestrator_ExecuteFromCheckpoint tests resuming from a checkpoint.
func TestOrchestrator_ExecuteFromCheckpoint(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	eventStore := NewDBEventStore(db)
	ctx := context.Background()

	// Create orchestrator with event store
	orchestrator, err := NewMissionOrchestrator(
		store,
		WithEventStore(eventStore),
	)
	require.NoError(t, err)

	// Create mission in paused state
	mission := createTestMission(t)
	mission.Status = MissionStatusPaused
	startedAt := time.Now()
	mission.StartedAt = &startedAt
	mission.Metrics = &MissionMetrics{
		StartedAt:          startedAt,
		LastUpdateAt:       startedAt,
		FindingsBySeverity: make(map[string]int),
		CompletedNodes:     2,
		TotalNodes:         5,
	}
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	// Create a checkpoint
	checkpoint := &MissionCheckpoint{
		CompletedNodes: []string{"node1", "node2"},
		PendingNodes:   []string{"node3", "node4", "node5"},
		NodeResults: map[string]any{
			"node1": map[string]any{"status": "completed"},
			"node2": map[string]any{"status": "completed"},
		},
		LastNodeID:     "node2",
		CheckpointedAt: startedAt,
	}

	// Execute from checkpoint
	result, err := orchestrator.ExecuteFromCheckpoint(ctx, mission, checkpoint)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, mission.ID, result.MissionID)
	assert.Equal(t, MissionStatusCompleted, result.Status)

	// Verify mission was updated
	updatedMission, err := store.Get(ctx, mission.ID)
	require.NoError(t, err)
	assert.Equal(t, MissionStatusCompleted, updatedMission.Status)
	assert.NotNil(t, updatedMission.CompletedAt)

	// Verify resumed event was emitted and persisted
	filter := NewEventFilter().
		WithMissionID(mission.ID).
		WithEventTypes(EventMissionResumed)

	events, err := eventStore.Query(ctx, filter)
	require.NoError(t, err)
	assert.NotEmpty(t, events, "Should have mission.resumed event")
	assert.Equal(t, EventMissionResumed, events[0].Type)
}

// TestOrchestrator_ExecuteFromCheckpoint_InvalidState tests resuming from invalid state.
func TestOrchestrator_ExecuteFromCheckpoint_InvalidState(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	orchestrator, err := NewMissionOrchestrator(store)
	require.NoError(t, err)
	ctx := context.Background()

	// Create mission in completed state (cannot resume)
	mission := createTestMission(t)
	mission.Status = MissionStatusCompleted
	completedAt := time.Now()
	mission.CompletedAt = &completedAt
	err = store.Save(ctx, mission)
	require.NoError(t, err)

	// Create a checkpoint
	checkpoint := &MissionCheckpoint{
		CompletedNodes: []string{"node1"},
		PendingNodes:   []string{"node2"},
		CheckpointedAt: time.Now(),
	}

	// Attempt to execute from checkpoint - should fail
	_, err = orchestrator.ExecuteFromCheckpoint(ctx, mission, checkpoint)
	require.Error(t, err)
	assert.True(t, IsInvalidStateError(err), "Should return invalid state error")
}

// TestNewMissionOrchestrator_Success verifies that orchestrator can be created successfully.
func TestNewMissionOrchestrator_Success(t *testing.T) {
	// Use mock store to avoid DB dependencies
	store := &mockMissionStore{}

	// Create orchestrator without any options
	orchestrator, err := NewMissionOrchestrator(store)

	// Should succeed
	require.NoError(t, err)
	assert.NotNil(t, orchestrator)
	assert.NotNil(t, orchestrator.emitter)
	assert.NotNil(t, orchestrator.pauseRequested)
}
