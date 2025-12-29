package mission

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()

	// Create temporary database file
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Open database
	db, err := database.Open(dbPath)
	require.NoError(t, err)

	// Initialize schema
	err = db.InitSchema()
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
		os.Remove(dbPath)
	})

	return db
}

func createTestMission(t *testing.T) *Mission {
	t.Helper()

	return &Mission{
		ID:          types.NewID(),
		Name:        "test-mission-" + types.NewID().String()[:8],
		Description: "Test mission description",
		Status:      MissionStatusPending,
		TargetID:    types.NewID(),
		WorkflowID:  types.NewID(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func TestDBMissionStore_Save(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	mission := createTestMission(t)

	err := store.Save(ctx, mission)
	require.NoError(t, err)

	// Verify mission was saved
	retrieved, err := store.Get(ctx, mission.ID)
	require.NoError(t, err)
	assert.Equal(t, mission.ID, retrieved.ID)
	assert.Equal(t, mission.Name, retrieved.Name)
	assert.Equal(t, mission.Status, retrieved.Status)
}

func TestDBMissionStore_Get(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	t.Run("existing mission", func(t *testing.T) {
		mission := createTestMission(t)
		err := store.Save(ctx, mission)
		require.NoError(t, err)

		retrieved, err := store.Get(ctx, mission.ID)
		require.NoError(t, err)
		assert.Equal(t, mission.ID, retrieved.ID)
	})

	t.Run("non-existent mission", func(t *testing.T) {
		_, err := store.Get(ctx, types.NewID())
		assert.Error(t, err)
		assert.True(t, IsNotFoundError(err))
	})
}

func TestDBMissionStore_List(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	// Create test missions
	missions := []*Mission{
		createTestMission(t),
		createTestMission(t),
		createTestMission(t),
	}

	for _, m := range missions {
		err := store.Save(ctx, m)
		require.NoError(t, err)
	}

	t.Run("list all", func(t *testing.T) {
		filter := NewMissionFilter()
		results, err := store.List(ctx, filter)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	t.Run("filter by status", func(t *testing.T) {
		filter := NewMissionFilter().WithStatus(MissionStatusPending)
		results, err := store.List(ctx, filter)
		require.NoError(t, err)
		for _, m := range results {
			assert.Equal(t, MissionStatusPending, m.Status)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		filter := NewMissionFilter().WithPagination(2, 0)
		results, err := store.List(ctx, filter)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(results), 2)
	})
}

func TestDBMissionStore_Update(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	mission := createTestMission(t)
	err := store.Save(ctx, mission)
	require.NoError(t, err)

	// Update mission
	mission.Status = MissionStatusRunning
	mission.Description = "Updated description"
	startedAt := time.Now()
	mission.StartedAt = &startedAt

	err = store.Update(ctx, mission)
	require.NoError(t, err)

	// Verify update
	retrieved, err := store.Get(ctx, mission.ID)
	require.NoError(t, err)
	assert.Equal(t, MissionStatusRunning, retrieved.Status)
	assert.Equal(t, "Updated description", retrieved.Description)
	assert.NotNil(t, retrieved.StartedAt)
}

func TestDBMissionStore_Delete(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	t.Run("delete terminal mission", func(t *testing.T) {
		mission := createTestMission(t)
		mission.Status = MissionStatusCompleted
		err := store.Save(ctx, mission)
		require.NoError(t, err)

		err = store.Delete(ctx, mission.ID)
		require.NoError(t, err)

		// Verify deletion
		_, err = store.Get(ctx, mission.ID)
		assert.Error(t, err)
		assert.True(t, IsNotFoundError(err))
	})

	t.Run("cannot delete non-terminal mission", func(t *testing.T) {
		mission := createTestMission(t)
		mission.Status = MissionStatusRunning
		err := store.Save(ctx, mission)
		require.NoError(t, err)

		err = store.Delete(ctx, mission.ID)
		assert.Error(t, err)
		assert.True(t, IsInvalidStateError(err))
	})
}

func TestDBMissionStore_GetByTarget(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	targetID := types.NewID()

	// Create missions with same target
	for i := 0; i < 3; i++ {
		mission := createTestMission(t)
		mission.TargetID = targetID
		err := store.Save(ctx, mission)
		require.NoError(t, err)
	}

	results, err := store.GetByTarget(ctx, targetID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 3)
	for _, m := range results {
		assert.Equal(t, targetID, m.TargetID)
	}
}

func TestDBMissionStore_GetActive(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	// Create running mission
	running := createTestMission(t)
	running.Status = MissionStatusRunning
	err := store.Save(ctx, running)
	require.NoError(t, err)

	// Create paused mission
	paused := createTestMission(t)
	paused.Status = MissionStatusPaused
	err = store.Save(ctx, paused)
	require.NoError(t, err)

	// Create completed mission (should not be returned)
	completed := createTestMission(t)
	completed.Status = MissionStatusCompleted
	err = store.Save(ctx, completed)
	require.NoError(t, err)

	results, err := store.GetActive(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 2)

	for _, m := range results {
		assert.True(t, m.Status == MissionStatusRunning || m.Status == MissionStatusPaused)
	}
}

func TestDBMissionStore_SaveCheckpoint(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	mission := createTestMission(t)
	err := store.Save(ctx, mission)
	require.NoError(t, err)

	checkpoint := &MissionCheckpoint{
		CompletedNodes: []string{"node1", "node2"},
		PendingNodes:   []string{"node3", "node4"},
		LastNodeID:     "node2",
		CheckpointedAt: time.Now(),
		WorkflowState:  map[string]any{"key": "value"},
		NodeResults:    map[string]any{"node1": "result1"},
	}

	err = store.SaveCheckpoint(ctx, mission.ID, checkpoint)
	require.NoError(t, err)

	// Verify checkpoint was saved
	retrieved, err := store.Get(ctx, mission.ID)
	require.NoError(t, err)
	assert.NotNil(t, retrieved.Checkpoint)
	assert.Equal(t, checkpoint.LastNodeID, retrieved.Checkpoint.LastNodeID)
	assert.Equal(t, len(checkpoint.CompletedNodes), len(retrieved.Checkpoint.CompletedNodes))
}

func TestDBMissionStore_Count(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	// Create test missions
	for i := 0; i < 5; i++ {
		mission := createTestMission(t)
		err := store.Save(ctx, mission)
		require.NoError(t, err)
	}

	count, err := store.Count(ctx, NewMissionFilter())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 5)
}

func TestMissionFilter(t *testing.T) {
	filter := NewMissionFilter()
	assert.Equal(t, 100, filter.Limit)
	assert.Equal(t, 0, filter.Offset)

	filter = filter.
		WithStatus(MissionStatusRunning).
		WithPagination(50, 10)

	assert.Equal(t, MissionStatusRunning, *filter.Status)
	assert.Equal(t, 50, filter.Limit)
	assert.Equal(t, 10, filter.Offset)
}
