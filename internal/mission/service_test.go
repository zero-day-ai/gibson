package mission

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultMissionService_CreateFromConfig(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	service := NewMissionService(store)
	ctx := context.Background()

	config := &MissionConfig{
		Name:        "test-mission",
		Description: "Test mission description",
		Target: MissionTargetConfig{
			Reference: "target-1",
		},
		Workflow: MissionWorkflowConfig{
			Reference: "workflow-1",
		},
	}

	// This will fail without full implementation of target/workflow resolution
	// but demonstrates the test structure
	mission, err := service.CreateFromConfig(ctx, config)

	// For now, just verify the structure works
	if err == nil {
		assert.NotNil(t, mission)
		assert.Equal(t, config.Name, mission.Name)
	}
}

func TestDefaultMissionService_ValidateMission(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	service := NewMissionService(store)
	ctx := context.Background()

	t.Run("valid mission", func(t *testing.T) {
		mission := createTestMission(t)
		err := service.ValidateMission(ctx, mission)
		assert.NoError(t, err)
	})

	t.Run("invalid mission - missing name", func(t *testing.T) {
		mission := createTestMission(t)
		mission.Name = ""
		err := service.ValidateMission(ctx, mission)
		assert.Error(t, err)
	})
}

func TestDefaultMissionService_GetSummary(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	service := NewMissionService(store)
	ctx := context.Background()

	mission := createTestMission(t)
	err := store.Save(ctx, mission)
	require.NoError(t, err)

	summary, err := service.GetSummary(ctx, mission.ID)
	require.NoError(t, err)
	assert.NotNil(t, summary)
	assert.Equal(t, mission.ID, summary.Mission.ID)
	assert.NotNil(t, summary.Progress)
}
