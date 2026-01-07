package mission

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

func TestCheckpointManager_Capture(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	manager := NewCheckpointManager(store)

	// Create a test mission
	mission := createTestMission(t)
	mission.Status = MissionStatusRunning
	mission.Metrics = &MissionMetrics{
		TotalNodes:     5,
		CompletedNodes: 2,
		TotalFindings:  3,
		StartedAt:      time.Now(),
	}
	require.NoError(t, store.Save(ctx, mission))

	// Create test workflow and state
	wf := &workflow.Workflow{
		ID:    mission.WorkflowID,
		Name:  "test-workflow",
		Nodes: make(map[string]*workflow.WorkflowNode),
	}

	// Add some nodes
	wf.Nodes["node1"] = &workflow.WorkflowNode{ID: "node1", Name: "Node 1", Type: workflow.NodeTypeAgent}
	wf.Nodes["node2"] = &workflow.WorkflowNode{ID: "node2", Name: "Node 2", Type: workflow.NodeTypeAgent}
	wf.Nodes["node3"] = &workflow.WorkflowNode{ID: "node3", Name: "Node 3", Type: workflow.NodeTypeAgent}

	state := workflow.NewWorkflowState(wf)
	state.Status = workflow.WorkflowStatusRunning

	// Mark some nodes as completed
	now := time.Now()
	state.MarkNodeStarted("node1")
	state.MarkNodeCompleted("node1", &workflow.NodeResult{
		NodeID:      "node1",
		Status:      workflow.NodeStatusCompleted,
		Output:      map[string]any{"result": "success"},
		CompletedAt: now,
	})

	state.MarkNodeStarted("node2")
	state.MarkNodeCompleted("node2", &workflow.NodeResult{
		NodeID:      "node2",
		Status:      workflow.NodeStatusCompleted,
		Output:      map[string]any{"result": "success"},
		CompletedAt: now,
	})

	// Capture checkpoint
	checkpoint, err := manager.Capture(ctx, mission.ID, state)
	require.NoError(t, err)
	require.NotNil(t, checkpoint)

	// Verify checkpoint fields
	assert.False(t, checkpoint.ID.IsZero(), "Checkpoint ID should not be zero")
	assert.Equal(t, 1, checkpoint.Version, "Checkpoint version should be 1")
	assert.NotEmpty(t, checkpoint.Checksum, "Checkpoint should have a checksum")
	assert.NotEmpty(t, checkpoint.CompletedNodes, "Checkpoint should have completed nodes")
	assert.Contains(t, checkpoint.CompletedNodes, "node1")
	assert.Contains(t, checkpoint.CompletedNodes, "node2")
	assert.Contains(t, checkpoint.PendingNodes, "node3")
	assert.NotNil(t, checkpoint.MetricsSnapshot, "Checkpoint should have metrics snapshot")
	assert.Equal(t, 2, checkpoint.MetricsSnapshot.CompletedNodes)
}

func TestCheckpointManager_Restore(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	manager := NewCheckpointManager(store)

	// Create a test mission
	mission := createTestMission(t)
	mission.Status = MissionStatusPaused
	mission.Metrics = &MissionMetrics{
		TotalNodes:     3,
		CompletedNodes: 1,
	}
	require.NoError(t, store.Save(ctx, mission))

	// Create and save a checkpoint
	originalCheckpoint := &MissionCheckpoint{
		ID:             types.NewID(),
		Version:        1,
		WorkflowState:  map[string]any{"status": "running"},
		CompletedNodes: []string{"node1"},
		PendingNodes:   []string{"node2", "node3"},
		NodeResults: map[string]any{
			"node1": map[string]any{"status": "completed"},
		},
		LastNodeID:     "node1",
		CheckpointedAt: time.Now(),
	}

	// Compute checksum for the original checkpoint
	tempManager := manager.(*DefaultCheckpointManager)
	checksum, err := tempManager.computeChecksum(originalCheckpoint)
	require.NoError(t, err)
	originalCheckpoint.Checksum = checksum

	// Save checkpoint
	require.NoError(t, store.SaveCheckpoint(ctx, mission.ID, originalCheckpoint))

	// Restore checkpoint
	restoredCheckpoint, err := manager.Restore(ctx, mission.ID)
	require.NoError(t, err)
	require.NotNil(t, restoredCheckpoint)

	// Verify restored checkpoint matches original
	assert.Equal(t, originalCheckpoint.ID, restoredCheckpoint.ID)
	assert.Equal(t, originalCheckpoint.Version, restoredCheckpoint.Version)
	assert.Equal(t, originalCheckpoint.Checksum, restoredCheckpoint.Checksum)
	assert.Equal(t, originalCheckpoint.CompletedNodes, restoredCheckpoint.CompletedNodes)
	assert.Equal(t, originalCheckpoint.PendingNodes, restoredCheckpoint.PendingNodes)
	assert.Equal(t, originalCheckpoint.LastNodeID, restoredCheckpoint.LastNodeID)
}

func TestCheckpointManager_Restore_NoCheckpoint(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	manager := NewCheckpointManager(store)

	// Create a test mission without a checkpoint
	mission := createTestMission(t)
	mission.Status = MissionStatusRunning
	require.NoError(t, store.Save(ctx, mission))

	// Restore should return nil when no checkpoint exists
	checkpoint, err := manager.Restore(ctx, mission.ID)
	require.NoError(t, err)
	assert.Nil(t, checkpoint, "Should return nil when no checkpoint exists")
}

func TestCheckpointManager_Restore_CorruptedCheckpoint(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	manager := NewCheckpointManager(store)

	// Create a test mission
	mission := createTestMission(t)
	mission.Status = MissionStatusPaused
	require.NoError(t, store.Save(ctx, mission))

	// Create a checkpoint with invalid checksum
	corruptedCheckpoint := &MissionCheckpoint{
		ID:             types.NewID(),
		Version:        1,
		WorkflowState:  map[string]any{"status": "running"},
		CompletedNodes: []string{"node1"},
		PendingNodes:   []string{"node2"},
		CheckpointedAt: time.Now(),
		Checksum:       "invalid_checksum_12345", // Invalid checksum
	}

	// Save corrupted checkpoint
	require.NoError(t, store.SaveCheckpoint(ctx, mission.ID, corruptedCheckpoint))

	// Restore should return error for corrupted checkpoint
	_, err := manager.Restore(ctx, mission.ID)
	require.Error(t, err)
	// The error should be a checkpoint error
	var missionErr *MissionError
	require.ErrorAs(t, err, &missionErr)
	assert.Equal(t, ErrMissionCheckpoint, missionErr.Code)
}

func TestCheckpointManager_List(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	manager := NewCheckpointManager(store)

	// Create a test mission
	mission := createTestMission(t)
	mission.Status = MissionStatusPaused
	require.NoError(t, store.Save(ctx, mission))

	// List should return empty when no checkpoints exist
	checkpoints, err := manager.List(ctx, mission.ID)
	require.NoError(t, err)
	assert.Empty(t, checkpoints)

	// Create and save a checkpoint
	checkpoint := &MissionCheckpoint{
		ID:             types.NewID(),
		Version:        1,
		WorkflowState:  map[string]any{"status": "running"},
		CompletedNodes: []string{"node1"},
		PendingNodes:   []string{"node2"},
		CheckpointedAt: time.Now(),
	}

	tempManager := manager.(*DefaultCheckpointManager)
	checksum, err := tempManager.computeChecksum(checkpoint)
	require.NoError(t, err)
	checkpoint.Checksum = checksum

	require.NoError(t, store.SaveCheckpoint(ctx, mission.ID, checkpoint))

	// List should return the checkpoint
	checkpoints, err = manager.List(ctx, mission.ID)
	require.NoError(t, err)
	assert.Len(t, checkpoints, 1)
	assert.Equal(t, checkpoint.ID, checkpoints[0].ID)
}

func TestCheckpointManager_AutoCheckpointInterval(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	manager := NewCheckpointManager(store).(*DefaultCheckpointManager)

	// Default interval should be 0 (disabled)
	assert.Equal(t, time.Duration(0), manager.GetAutoCheckpointInterval())

	// Set auto-checkpoint interval
	interval := 5 * time.Minute
	manager.SetAutoCheckpointInterval(interval)
	assert.Equal(t, interval, manager.GetAutoCheckpointInterval())

	// Disable auto-checkpoint
	manager.SetAutoCheckpointInterval(0)
	assert.Equal(t, time.Duration(0), manager.GetAutoCheckpointInterval())
}

func TestCheckpointManager_ChecksumValidation(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	manager := NewCheckpointManager(store).(*DefaultCheckpointManager)

	checkpoint := &MissionCheckpoint{
		ID:             types.NewID(),
		Version:        1,
		WorkflowState:  map[string]any{"status": "running", "node_count": 5},
		CompletedNodes: []string{"node1", "node2"},
		PendingNodes:   []string{"node3", "node4", "node5"},
		NodeResults: map[string]any{
			"node1": map[string]any{"status": "completed"},
		},
		LastNodeID:     "node2",
		CheckpointedAt: time.Now(),
	}

	// Compute checksum
	checksum1, err := manager.computeChecksum(checkpoint)
	require.NoError(t, err)
	assert.NotEmpty(t, checksum1)

	// Compute checksum again - should be identical
	checksum2, err := manager.computeChecksum(checkpoint)
	require.NoError(t, err)
	assert.Equal(t, checksum1, checksum2, "Checksum should be deterministic")

	// Modify checkpoint data
	checkpoint.CompletedNodes = append(checkpoint.CompletedNodes, "node3")

	// Checksum should be different after modification
	checksum3, err := manager.computeChecksum(checkpoint)
	require.NoError(t, err)
	assert.NotEqual(t, checksum1, checksum3, "Checksum should change after data modification")
}

func TestCheckpointManager_SerializeWorkflowState(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	manager := NewCheckpointManager(store).(*DefaultCheckpointManager)

	// Create test workflow and state
	wf := &workflow.Workflow{
		ID:    types.NewID(),
		Name:  "test-workflow",
		Nodes: make(map[string]*workflow.WorkflowNode),
	}
	wf.Nodes["node1"] = &workflow.WorkflowNode{ID: "node1", Name: "Node 1", Type: workflow.NodeTypeAgent}
	wf.Nodes["node2"] = &workflow.WorkflowNode{ID: "node2", Name: "Node 2", Type: workflow.NodeTypeAgent}

	state := workflow.NewWorkflowState(wf)
	state.Status = workflow.WorkflowStatusRunning
	state.MarkNodeCompleted("node1", nil)

	// Serialize state
	serialized, err := manager.serializeWorkflowState(state)
	require.NoError(t, err)
	require.NotNil(t, serialized)

	// Verify serialized data contains expected fields
	assert.Contains(t, serialized, "workflow_id")
	assert.Contains(t, serialized, "status")
	assert.Contains(t, serialized, "started_at")
	assert.Contains(t, serialized, "node_states")

	// Verify workflow ID is serialized correctly
	assert.Equal(t, wf.ID.String(), serialized["workflow_id"])
	assert.Equal(t, string(workflow.WorkflowStatusRunning), serialized["status"])

	// Verify node states are serialized
	nodeStates, ok := serialized["node_states"].(map[string]map[string]any)
	require.True(t, ok)
	assert.Contains(t, nodeStates, "node1")
	assert.Contains(t, nodeStates, "node2")
	assert.Equal(t, string(workflow.NodeStatusCompleted), nodeStates["node1"]["status"])
	assert.Equal(t, string(workflow.NodeStatusPending), nodeStates["node2"]["status"])
}
