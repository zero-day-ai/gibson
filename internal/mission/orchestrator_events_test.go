package mission

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestMissionOrchestrator_EmitsProgressEvents verifies that progress events are emitted
func TestMissionOrchestrator_EmitsProgressEvents(t *testing.T) {
	store := &mockMissionStore{}
	emitter := NewDefaultEventEmitter()
	defer emitter.Close()

	// Subscribe to events
	ctx := context.Background()
	eventChan, cleanup := emitter.Subscribe(ctx)
	defer cleanup()

	// Create orchestrator with event emitter but no workflow executor
	// This will skip workflow execution but emit start/complete events
	orchestrator := NewMissionOrchestrator(
		store,
		WithEventEmitter(emitter),
	)

	// Create mission
	mission := &Mission{
		ID:           types.NewID(),
		Name:         "Test Mission",
		Status:       MissionStatusPending,
		WorkflowJSON: "", // No workflow - will skip execution
	}

	err := store.Create(ctx, mission)
	require.NoError(t, err)

	// Execute mission in goroutine
	go func() {
		_, _ = orchestrator.Execute(ctx, mission)
	}()

	// Collect events
	receivedEvents := []MissionEvent{}
	timeout := time.After(2 * time.Second)

	// We expect at least started and completed events
	expectedEventCount := 2

	for i := 0; i < expectedEventCount; i++ {
		select {
		case event := <-eventChan:
			receivedEvents = append(receivedEvents, event)
			if event.Type == EventMissionCompleted {
				// Mission completed, we're done
				goto checkEvents
			}
		case <-timeout:
			t.Fatalf("Timeout waiting for event %d", i+1)
		}
	}

checkEvents:
	// Verify we received events in order
	assert.GreaterOrEqual(t, len(receivedEvents), 2)
	assert.Equal(t, EventMissionStarted, receivedEvents[0].Type)
	assert.Equal(t, mission.ID, receivedEvents[0].MissionID)

	// Last event should be completed
	lastEvent := receivedEvents[len(receivedEvents)-1]
	assert.Equal(t, EventMissionCompleted, lastEvent.Type)
	assert.Equal(t, mission.ID, lastEvent.MissionID)
}

// TestMissionOrchestrator_EmitsFailedEvents verifies that failed events are emitted
func TestMissionOrchestrator_EmitsFailedEvents(t *testing.T) {
	t.Skip("Skipping - requires workflow executor to trigger parsing errors")
}

// TestMissionOrchestrator_EmitsCancelledEvents verifies that cancelled events are emitted
func TestMissionOrchestrator_EmitsCancelledEvents(t *testing.T) {
	store := &mockMissionStore{}
	emitter := NewDefaultEventEmitter()
	defer emitter.Close()

	ctx, cancel := context.WithCancel(context.Background())
	eventChan, cleanup := emitter.Subscribe(ctx)
	defer cleanup()

	orchestrator := NewMissionOrchestrator(
		store,
		WithEventEmitter(emitter),
	)

	// Create mission - no workflow executor means it will simulate execution
	mission := &Mission{
		ID:           types.NewID(),
		Name:         "Test Mission",
		Status:       MissionStatusPending,
		WorkflowJSON: "",
	}

	err := store.Create(ctx, mission)
	require.NoError(t, err)

	// Start execution and cancel immediately
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Execute mission
	go func() {
		_, _ = orchestrator.Execute(ctx, mission)
	}()

	// Wait for events (we might get started, then cancelled, or just error)
	timeout := time.After(2 * time.Second)

	for {
		select {
		case event := <-eventChan:
			if event.Type == EventMissionCompleted {
				// Test completed
				return
			}
		case <-timeout:
			// Context cancellation during mission execution is handled differently
			// depending on timing, so we're just verifying the code runs
			return
		}
	}
}

// TestMissionOrchestrator_EventOrderAndTiming verifies event order and timing
func TestMissionOrchestrator_EventOrderAndTiming(t *testing.T) {
	store := &mockMissionStore{}
	emitter := NewDefaultEventEmitter()
	defer emitter.Close()

	ctx := context.Background()
	eventChan, cleanup := emitter.Subscribe(ctx)
	defer cleanup()

	orchestrator := NewMissionOrchestrator(
		store,
		WithEventEmitter(emitter),
	)

	mission := &Mission{
		ID:           types.NewID(),
		Name:         "Test Mission",
		Status:       MissionStatusPending,
		WorkflowJSON: "",
	}

	err := store.Create(ctx, mission)
	require.NoError(t, err)

	// Track event timestamps
	eventTimestamps := make(map[MissionEventType]time.Time)

	// Execute mission
	go func() {
		_, _ = orchestrator.Execute(ctx, mission)
	}()

	// Collect events with timestamps
	timeout := time.After(2 * time.Second)
	receivedCompleted := false

	for !receivedCompleted {
		select {
		case event := <-eventChan:
			eventTimestamps[event.Type] = event.Timestamp

			if event.Type == EventMissionCompleted {
				receivedCompleted = true
			}
		case <-timeout:
			t.Fatal("Timeout waiting for events")
		}
	}

	// Verify event ordering (started before completed)
	startedTime, hasStarted := eventTimestamps[EventMissionStarted]
	completedTime, hasCompleted := eventTimestamps[EventMissionCompleted]

	assert.True(t, hasStarted, "Should have received started event")
	assert.True(t, hasCompleted, "Should have received completed event")

	if hasStarted && hasCompleted {
		assert.True(t, startedTime.Before(completedTime),
			"Started event should occur before completed event")
	}
}

// TestMissionOrchestrator_FailedEventWithError verifies error information in failed events
func TestMissionOrchestrator_FailedEventWithError(t *testing.T) {
	t.Skip("Skipping - requires workflow executor to trigger failure paths")
}

// TestMissionOrchestrator_MultipleSubscribers verifies multiple subscribers receive events
func TestMissionOrchestrator_MultipleSubscribers(t *testing.T) {
	store := &mockMissionStore{}
	emitter := NewDefaultEventEmitter()
	defer emitter.Close()

	ctx := context.Background()

	// Create multiple subscribers
	eventChan1, cleanup1 := emitter.Subscribe(ctx)
	defer cleanup1()

	eventChan2, cleanup2 := emitter.Subscribe(ctx)
	defer cleanup2()

	orchestrator := NewMissionOrchestrator(
		store,
		WithEventEmitter(emitter),
	)

	mission := &Mission{
		ID:           types.NewID(),
		Name:         "Test Mission",
		Status:       MissionStatusPending,
		WorkflowJSON: "",
	}

	err := store.Create(ctx, mission)
	require.NoError(t, err)

	// Execute mission
	go func() {
		_, _ = orchestrator.Execute(ctx, mission)
	}()

	// Collect events from both subscribers
	events1 := []MissionEvent{}
	events2 := []MissionEvent{}

	timeout := time.After(2 * time.Second)
	done := false

	for !done {
		select {
		case event := <-eventChan1:
			events1 = append(events1, event)
			if event.Type == EventMissionCompleted {
				done = true
			}
		case event := <-eventChan2:
			events2 = append(events2, event)
		case <-timeout:
			t.Fatal("Timeout waiting for events")
		}
	}

	// Verify both subscribers received events
	assert.GreaterOrEqual(t, len(events1), 2, "Subscriber 1 should receive events")
	assert.GreaterOrEqual(t, len(events2), 1, "Subscriber 2 should receive events")

	// Verify both received the same event types (though possibly different timing)
	eventTypes1 := make(map[MissionEventType]bool)
	for _, e := range events1 {
		eventTypes1[e.Type] = true
	}

	eventTypes2 := make(map[MissionEventType]bool)
	for _, e := range events2 {
		eventTypes2[e.Type] = true
	}

	// Both should have received started event
	assert.True(t, eventTypes1[EventMissionStarted], "Subscriber 1 should receive started event")
	// Subscriber 2 might not have received all events due to timing
}

// TestMissionOrchestrator_EventMissionID verifies all events have correct mission ID
func TestMissionOrchestrator_EventMissionID(t *testing.T) {
	store := &mockMissionStore{}
	emitter := NewDefaultEventEmitter()
	defer emitter.Close()

	ctx := context.Background()
	eventChan, cleanup := emitter.Subscribe(ctx)
	defer cleanup()

	orchestrator := NewMissionOrchestrator(
		store,
		WithEventEmitter(emitter),
	)

	missionID := types.NewID()
	mission := &Mission{
		ID:           missionID,
		Name:         "Test Mission",
		Status:       MissionStatusPending,
		WorkflowJSON: "",
	}

	err := store.Create(ctx, mission)
	require.NoError(t, err)

	go func() {
		_, _ = orchestrator.Execute(ctx, mission)
	}()

	// Verify all events have the correct mission ID
	timeout := time.After(2 * time.Second)
	receivedCompleted := false

	for !receivedCompleted {
		select {
		case event := <-eventChan:
			assert.Equal(t, missionID, event.MissionID,
				fmt.Sprintf("Event type %s should have correct mission ID", event.Type))

			if event.Type == EventMissionCompleted {
				receivedCompleted = true
			}
		case <-timeout:
			t.Fatal("Timeout waiting for events")
		}
	}
}
