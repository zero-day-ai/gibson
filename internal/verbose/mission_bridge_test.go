package verbose

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/types"
)

func TestMissionEventBridge_StartStop(t *testing.T) {
	// Create mission emitter and verbose bus
	missionEmitter := mission.NewDefaultEventEmitter()
	defer missionEmitter.Close()

	verboseBus := NewDefaultVerboseEventBus()
	defer verboseBus.Close()

	// Create bridge
	bridge := NewMissionEventBridge(missionEmitter, verboseBus)

	// Start bridge
	ctx := context.Background()
	err := bridge.Start(ctx)
	require.NoError(t, err)

	// Verify started
	assert.True(t, bridge.started)

	// Stop bridge
	bridge.Stop()

	// Verify stopped
	assert.False(t, bridge.started)
}

func TestMissionEventBridge_StartIdempotent(t *testing.T) {
	missionEmitter := mission.NewDefaultEventEmitter()
	defer missionEmitter.Close()

	verboseBus := NewDefaultVerboseEventBus()
	defer verboseBus.Close()

	bridge := NewMissionEventBridge(missionEmitter, verboseBus)

	ctx := context.Background()

	// Start twice
	err1 := bridge.Start(ctx)
	err2 := bridge.Start(ctx)

	require.NoError(t, err1)
	require.NoError(t, err2)

	bridge.Stop()
}

func TestMissionEventBridge_TranslateMissionStarted(t *testing.T) {
	missionEmitter := mission.NewDefaultEventEmitter()
	defer missionEmitter.Close()

	verboseBus := NewDefaultVerboseEventBus()
	defer verboseBus.Close()

	bridge := NewMissionEventBridge(missionEmitter, verboseBus)

	// Subscribe to verbose events
	ctx := context.Background()
	verboseChan, cleanup := verboseBus.Subscribe(ctx)
	defer cleanup()

	// Start bridge
	err := bridge.Start(ctx)
	require.NoError(t, err)
	defer bridge.Stop()

	// Emit mission started event
	missionID := types.NewID()
	missionEvent := mission.NewStartedEvent(missionID)

	err = missionEmitter.Emit(ctx, missionEvent)
	require.NoError(t, err)

	// Wait for verbose event
	select {
	case verboseEvent := <-verboseChan:
		assert.Equal(t, EventMissionStarted, verboseEvent.Type)
		assert.Equal(t, LevelVerbose, verboseEvent.Level)
		assert.Equal(t, missionID, verboseEvent.MissionID)

		payload, ok := verboseEvent.Payload.(*MissionStartedData)
		require.True(t, ok, "Payload should be MissionStartedData")
		assert.Equal(t, missionID, payload.MissionID)

	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for verbose event")
	}
}

func TestMissionEventBridge_TranslateMissionProgress(t *testing.T) {
	missionEmitter := mission.NewDefaultEventEmitter()
	defer missionEmitter.Close()

	verboseBus := NewDefaultVerboseEventBus()
	defer verboseBus.Close()

	bridge := NewMissionEventBridge(missionEmitter, verboseBus)

	ctx := context.Background()
	verboseChan, cleanup := verboseBus.Subscribe(ctx)
	defer cleanup()

	err := bridge.Start(ctx)
	require.NoError(t, err)
	defer bridge.Stop()

	// Emit mission progress event
	missionID := types.NewID()
	progress := &mission.MissionProgress{
		MissionID:       missionID,
		CompletedNodes:  5,
		TotalNodes:      10,
		PercentComplete: 50.0,
	}
	missionEvent := mission.NewProgressEvent(missionID, progress, "Halfway done")

	err = missionEmitter.Emit(ctx, missionEvent)
	require.NoError(t, err)

	// Wait for verbose event
	select {
	case verboseEvent := <-verboseChan:
		assert.Equal(t, EventMissionProgress, verboseEvent.Type)
		assert.Equal(t, LevelVerbose, verboseEvent.Level)
		assert.Equal(t, missionID, verboseEvent.MissionID)

		payload, ok := verboseEvent.Payload.(*MissionProgressData)
		require.True(t, ok, "Payload should be MissionProgressData")
		assert.Equal(t, missionID, payload.MissionID)
		assert.Equal(t, 5, payload.CompletedNodes)
		assert.Equal(t, 10, payload.TotalNodes)
		assert.Equal(t, "Halfway done", payload.Message)

	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for verbose event")
	}
}

func TestMissionEventBridge_TranslateMissionCompleted(t *testing.T) {
	missionEmitter := mission.NewDefaultEventEmitter()
	defer missionEmitter.Close()

	verboseBus := NewDefaultVerboseEventBus()
	defer verboseBus.Close()

	bridge := NewMissionEventBridge(missionEmitter, verboseBus)

	ctx := context.Background()
	verboseChan, cleanup := verboseBus.Subscribe(ctx)
	defer cleanup()

	err := bridge.Start(ctx)
	require.NoError(t, err)
	defer bridge.Stop()

	// Emit mission completed event
	missionID := types.NewID()
	result := &mission.MissionResult{
		MissionID:  missionID,
		Status:     mission.MissionStatusCompleted,
		FindingIDs: []types.ID{types.NewID(), types.NewID()},
		Metrics: &mission.MissionMetrics{
			Duration: 5 * time.Minute,
		},
	}
	missionEvent := mission.NewCompletedEvent(missionID, result)

	err = missionEmitter.Emit(ctx, missionEvent)
	require.NoError(t, err)

	// Wait for verbose event
	select {
	case verboseEvent := <-verboseChan:
		assert.Equal(t, EventMissionCompleted, verboseEvent.Type)
		assert.Equal(t, LevelVerbose, verboseEvent.Level)
		assert.Equal(t, missionID, verboseEvent.MissionID)

		payload, ok := verboseEvent.Payload.(*MissionCompletedData)
		require.True(t, ok, "Payload should be MissionCompletedData")
		assert.Equal(t, missionID, payload.MissionID)
		assert.True(t, payload.Success)
		assert.Equal(t, 2, payload.FindingCount)
		assert.Equal(t, 5*time.Minute, payload.Duration)

	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for verbose event")
	}
}

func TestMissionEventBridge_TranslateMissionFailed(t *testing.T) {
	missionEmitter := mission.NewDefaultEventEmitter()
	defer missionEmitter.Close()

	verboseBus := NewDefaultVerboseEventBus()
	defer verboseBus.Close()

	bridge := NewMissionEventBridge(missionEmitter, verboseBus)

	ctx := context.Background()
	verboseChan, cleanup := verboseBus.Subscribe(ctx)
	defer cleanup()

	err := bridge.Start(ctx)
	require.NoError(t, err)
	defer bridge.Stop()

	// Emit mission failed event
	missionID := types.NewID()
	missionEvent := mission.NewFailedEvent(missionID, assert.AnError)

	err = missionEmitter.Emit(ctx, missionEvent)
	require.NoError(t, err)

	// Wait for verbose event
	select {
	case verboseEvent := <-verboseChan:
		assert.Equal(t, EventMissionFailed, verboseEvent.Type)
		assert.Equal(t, LevelVerbose, verboseEvent.Level)
		assert.Equal(t, missionID, verboseEvent.MissionID)

		payload, ok := verboseEvent.Payload.(*MissionFailedData)
		require.True(t, ok, "Payload should be MissionFailedData")
		assert.Equal(t, missionID, payload.MissionID)
		assert.Equal(t, assert.AnError.Error(), payload.Error)

	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for verbose event")
	}
}

func TestMissionEventBridge_TranslateFindingSubmitted(t *testing.T) {
	missionEmitter := mission.NewDefaultEventEmitter()
	defer missionEmitter.Close()

	verboseBus := NewDefaultVerboseEventBus()
	defer verboseBus.Close()

	bridge := NewMissionEventBridge(missionEmitter, verboseBus)

	ctx := context.Background()
	verboseChan, cleanup := verboseBus.Subscribe(ctx)
	defer cleanup()

	err := bridge.Start(ctx)
	require.NoError(t, err)
	defer bridge.Stop()

	// Emit finding event
	missionID := types.NewID()
	findingID := types.NewID()
	missionEvent := mission.NewFindingEvent(
		missionID,
		findingID,
		"SQL Injection Found",
		"high",
		"sqlmap-agent",
	)

	err = missionEmitter.Emit(ctx, missionEvent)
	require.NoError(t, err)

	// Wait for verbose event
	select {
	case verboseEvent := <-verboseChan:
		assert.Equal(t, EventFindingSubmitted, verboseEvent.Type)
		assert.Equal(t, LevelVerbose, verboseEvent.Level)
		assert.Equal(t, missionID, verboseEvent.MissionID)

		payload, ok := verboseEvent.Payload.(*FindingSubmittedData)
		require.True(t, ok, "Payload should be FindingSubmittedData")
		assert.Equal(t, findingID, payload.FindingID)
		assert.Equal(t, "SQL Injection Found", payload.Title)
		assert.Equal(t, "high", payload.Severity)
		assert.Equal(t, "sqlmap-agent", payload.AgentName)

	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for verbose event")
	}
}

func TestMissionEventBridge_TranslateMissionCheckpoint(t *testing.T) {
	missionEmitter := mission.NewDefaultEventEmitter()
	defer missionEmitter.Close()

	verboseBus := NewDefaultVerboseEventBus()
	defer verboseBus.Close()

	bridge := NewMissionEventBridge(missionEmitter, verboseBus)

	ctx := context.Background()
	verboseChan, cleanup := verboseBus.Subscribe(ctx)
	defer cleanup()

	err := bridge.Start(ctx)
	require.NoError(t, err)
	defer bridge.Stop()

	// Emit checkpoint event
	missionID := types.NewID()
	missionEvent := mission.NewCheckpointEvent(
		missionID,
		"checkpoint-1",
		3,
		10,
	)

	err = missionEmitter.Emit(ctx, missionEvent)
	require.NoError(t, err)

	// Wait for verbose event
	select {
	case verboseEvent := <-verboseChan:
		assert.Equal(t, EventMissionNode, verboseEvent.Type)
		assert.Equal(t, LevelVeryVerbose, verboseEvent.Level)
		assert.Equal(t, missionID, verboseEvent.MissionID)

		payload, ok := verboseEvent.Payload.(*MissionNodeData)
		require.True(t, ok, "Payload should be MissionNodeData")
		assert.Equal(t, "checkpoint-1", payload.NodeID)
		assert.Equal(t, "checkpoint", payload.NodeType)
		assert.Equal(t, "completed", payload.Status)

	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for verbose event")
	}
}

func TestMissionEventBridge_MultipleEvents(t *testing.T) {
	missionEmitter := mission.NewDefaultEventEmitter()
	defer missionEmitter.Close()

	verboseBus := NewDefaultVerboseEventBus()
	defer verboseBus.Close()

	bridge := NewMissionEventBridge(missionEmitter, verboseBus)

	ctx := context.Background()
	verboseChan, cleanup := verboseBus.Subscribe(ctx)
	defer cleanup()

	err := bridge.Start(ctx)
	require.NoError(t, err)
	defer bridge.Stop()

	// Emit multiple events
	missionID := types.NewID()

	events := []mission.MissionEvent{
		mission.NewStartedEvent(missionID),
		mission.NewProgressEvent(missionID, &mission.MissionProgress{
			MissionID:      missionID,
			CompletedNodes: 1,
			TotalNodes:     3,
		}, "Node 1 completed"),
		mission.NewCompletedEvent(missionID, &mission.MissionResult{
			MissionID: missionID,
			Status:    mission.MissionStatusCompleted,
		}),
	}

	for _, evt := range events {
		err = missionEmitter.Emit(ctx, evt)
		require.NoError(t, err)
	}

	// Collect verbose events
	receivedEvents := []VerboseEvent{}
	timeout := time.After(2 * time.Second)

	for i := 0; i < len(events); i++ {
		select {
		case verboseEvent := <-verboseChan:
			receivedEvents = append(receivedEvents, verboseEvent)
		case <-timeout:
			t.Fatalf("Timeout waiting for event %d", i+1)
		}
	}

	// Verify we received all events in order
	assert.Len(t, receivedEvents, 3)
	assert.Equal(t, EventMissionStarted, receivedEvents[0].Type)
	assert.Equal(t, EventMissionProgress, receivedEvents[1].Type)
	assert.Equal(t, EventMissionCompleted, receivedEvents[2].Type)
}
