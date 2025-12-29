package mission

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultMissionOrchestrator_Execute(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	orchestrator := NewMissionOrchestrator(store)
	ctx := context.Background()

	mission := createTestMission(t)
	err := store.Save(ctx, mission)
	require.NoError(t, err)

	result, err := orchestrator.Execute(ctx, mission)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, mission.ID, result.MissionID)
	assert.Equal(t, MissionStatusCompleted, result.Status)
}

func TestDefaultMissionOrchestrator_InvalidState(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	orchestrator := NewMissionOrchestrator(store)
	ctx := context.Background()

	mission := createTestMission(t)
	mission.Status = MissionStatusCompleted // Terminal state
	err := store.Save(ctx, mission)
	require.NoError(t, err)

	_, err = orchestrator.Execute(ctx, mission)
	assert.Error(t, err)
	assert.True(t, IsInvalidStateError(err))
}

func TestDefaultMissionOrchestrator_WithEventEmitter(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	emitter := NewDefaultEventEmitter(WithBufferSize(10))

	// Subscribe to events
	ctx := context.Background()
	events, unsubscribe := emitter.Subscribe(ctx)
	defer unsubscribe()

	orchestrator := NewMissionOrchestrator(store, WithEventEmitter(emitter))

	mission := createTestMission(t)
	err := store.Save(ctx, mission)
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
