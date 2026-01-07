package mission

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
)

// mockWorkflowStore is a mock implementation of WorkflowStore for testing
type mockWorkflowStore struct {
	workflows map[types.ID]interface{}
	getError  error
}

func (m *mockWorkflowStore) Get(ctx context.Context, id types.ID) (interface{}, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	if wf, ok := m.workflows[id]; ok {
		return wf, nil
	}
	return nil, NewNotFoundError(id.String())
}

// mockFindingStore is a mock implementation of FindingStore for testing
type mockFindingStore struct {
	findings       map[types.ID][]interface{}
	severityCounts map[types.ID]map[string]int
	getError       error
	countError     error
}

func (m *mockFindingStore) GetByMission(ctx context.Context, missionID types.ID) ([]interface{}, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	if findings, ok := m.findings[missionID]; ok {
		return findings, nil
	}
	return []interface{}{}, nil
}

func (m *mockFindingStore) CountBySeverity(ctx context.Context, missionID types.ID) (map[string]int, error) {
	if m.countError != nil {
		return nil, m.countError
	}
	if counts, ok := m.severityCounts[missionID]; ok {
		return counts, nil
	}
	return make(map[string]int), nil
}

func TestDefaultMissionService_LoadWorkflow(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)

	t.Run("load from inline definition", func(t *testing.T) {
		service := NewMissionService(store, nil, nil)
		ctx := context.Background()

		inlineWorkflow := &InlineWorkflowConfig{
			Agents: []string{"agent1", "agent2"},
		}

		config := &MissionWorkflowConfig{
			Inline: inlineWorkflow,
		}

		result, err := service.LoadWorkflow(ctx, config)
		require.NoError(t, err)
		assert.Equal(t, inlineWorkflow, result)
	})

	t.Run("load from workflow store by reference", func(t *testing.T) {
		workflowID := types.NewID()
		expectedWorkflow := map[string]interface{}{"name": "test-workflow"}

		mockStore := &mockWorkflowStore{
			workflows: map[types.ID]interface{}{
				workflowID: expectedWorkflow,
			},
		}

		service := NewMissionService(store, mockStore, nil)
		ctx := context.Background()

		config := &MissionWorkflowConfig{
			Reference: workflowID.String(),
		}

		result, err := service.LoadWorkflow(ctx, config)
		require.NoError(t, err)
		assert.Equal(t, expectedWorkflow, result)
	})

	t.Run("error when workflow store not configured", func(t *testing.T) {
		service := NewMissionService(store, nil, nil)
		ctx := context.Background()

		config := &MissionWorkflowConfig{
			Reference: types.NewID().String(),
		}

		_, err := service.LoadWorkflow(ctx, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "workflow store not configured")
	})

	t.Run("error when workflow not found", func(t *testing.T) {
		mockStore := &mockWorkflowStore{
			workflows: make(map[types.ID]interface{}),
		}

		service := NewMissionService(store, mockStore, nil)
		ctx := context.Background()

		config := &MissionWorkflowConfig{
			Reference: types.NewID().String(),
		}

		_, err := service.LoadWorkflow(ctx, config)
		assert.Error(t, err)
	})

	t.Run("error when config is nil", func(t *testing.T) {
		service := NewMissionService(store, nil, nil)
		ctx := context.Background()

		_, err := service.LoadWorkflow(ctx, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "workflow config is required")
	})

	t.Run("error when neither reference nor inline provided", func(t *testing.T) {
		service := NewMissionService(store, nil, nil)
		ctx := context.Background()

		config := &MissionWorkflowConfig{}

		_, err := service.LoadWorkflow(ctx, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must specify either 'reference' or 'inline'")
	})
}

func TestDefaultMissionService_AggregateFindings(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)

	t.Run("successfully aggregate findings", func(t *testing.T) {
		missionID := types.NewID()
		expectedFindings := []interface{}{
			map[string]string{"title": "finding1"},
			map[string]string{"title": "finding2"},
		}

		mockStore := &mockFindingStore{
			findings: map[types.ID][]interface{}{
				missionID: expectedFindings,
			},
		}

		service := NewMissionService(store, nil, mockStore)
		ctx := context.Background()

		findings, err := service.AggregateFindings(ctx, missionID)
		require.NoError(t, err)
		assert.Len(t, findings, 2)
		assert.Equal(t, expectedFindings, findings)
	})

	t.Run("return empty list when no findings", func(t *testing.T) {
		mockStore := &mockFindingStore{
			findings: make(map[types.ID][]interface{}),
		}

		service := NewMissionService(store, nil, mockStore)
		ctx := context.Background()

		findings, err := service.AggregateFindings(ctx, types.NewID())
		require.NoError(t, err)
		assert.Len(t, findings, 0)
	})

	t.Run("error when finding store not configured", func(t *testing.T) {
		service := NewMissionService(store, nil, nil)
		ctx := context.Background()

		_, err := service.AggregateFindings(ctx, types.NewID())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "finding store not configured")
	})

	t.Run("error when mission ID is zero", func(t *testing.T) {
		mockStore := &mockFindingStore{
			findings: make(map[types.ID][]interface{}),
		}

		service := NewMissionService(store, nil, mockStore)
		ctx := context.Background()

		_, err := service.AggregateFindings(ctx, types.ID(""))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mission ID is required")
	})
}

func TestDefaultMissionService_GetSummary(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	t.Run("get summary with finding counts", func(t *testing.T) {
		mission := createTestMission(t)
		err := store.Save(ctx, mission)
		require.NoError(t, err)

		severityCounts := map[string]int{
			string(agent.SeverityCritical): 2,
			string(agent.SeverityHigh):     5,
			string(agent.SeverityMedium):   10,
		}

		mockStore := &mockFindingStore{
			severityCounts: map[types.ID]map[string]int{
				mission.ID: severityCounts,
			},
		}

		service := NewMissionService(store, nil, mockStore)

		summary, err := service.GetSummary(ctx, mission.ID)
		require.NoError(t, err)
		assert.NotNil(t, summary)
		assert.Equal(t, mission.ID, summary.Mission.ID)
		assert.Equal(t, 17, summary.FindingsCount) // 2 + 5 + 10
		assert.Equal(t, severityCounts, summary.FindingsByLevel)
		assert.NotNil(t, summary.Progress)
	})

	t.Run("get summary without finding store", func(t *testing.T) {
		mission := createTestMission(t)
		err := store.Save(ctx, mission)
		require.NoError(t, err)

		service := NewMissionService(store, nil, nil)

		summary, err := service.GetSummary(ctx, mission.ID)
		require.NoError(t, err)
		assert.NotNil(t, summary)
		assert.Equal(t, 0, summary.FindingsCount)
		assert.NotNil(t, summary.FindingsByLevel)
		assert.Len(t, summary.FindingsByLevel, 0)
	})

	t.Run("error when mission not found", func(t *testing.T) {
		service := NewMissionService(store, nil, nil)

		_, err := service.GetSummary(ctx, types.NewID())
		assert.Error(t, err)
	})

	t.Run("error when mission ID is zero", func(t *testing.T) {
		service := NewMissionService(store, nil, nil)

		_, err := service.GetSummary(ctx, types.ID(""))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mission ID is required")
	})
}

func TestDefaultMissionService_ValidateMission_Constraints(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	t.Run("valid constraints", func(t *testing.T) {
		service := NewMissionService(store, nil, nil)

		mission := createTestMission(t)
		mission.Constraints = &MissionConstraints{
			MaxDuration:       1 * time.Hour,
			MaxFindings:       100,
			MaxCost:           10.0,
			MaxTokens:         100000,
			SeverityThreshold: agent.SeverityHigh,
		}

		err := service.ValidateMission(ctx, mission)
		assert.NoError(t, err)
	})

	t.Run("error when max_duration too short", func(t *testing.T) {
		service := NewMissionService(store, nil, nil)

		mission := createTestMission(t)
		mission.Constraints = &MissionConstraints{
			MaxDuration: 30 * time.Second,
		}

		err := service.ValidateMission(ctx, mission)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max_duration too short")
	})

	t.Run("max_findings zero is valid (means unlimited)", func(t *testing.T) {
		service := NewMissionService(store, nil, nil)

		mission := createTestMission(t)
		mission.Constraints = &MissionConstraints{
			MaxFindings: 0, // Zero means unlimited/not set
		}

		err := service.ValidateMission(ctx, mission)
		// MaxFindings: 0 is valid - it means "unlimited/not set"
		assert.NoError(t, err)
	})

	t.Run("error when max_cost too low", func(t *testing.T) {
		service := NewMissionService(store, nil, nil)

		mission := createTestMission(t)
		mission.Constraints = &MissionConstraints{
			MaxCost: 0.001,
		}

		err := service.ValidateMission(ctx, mission)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max_cost too low")
	})

	t.Run("error when max_tokens too low", func(t *testing.T) {
		service := NewMissionService(store, nil, nil)

		mission := createTestMission(t)
		mission.Constraints = &MissionConstraints{
			MaxTokens: 500,
		}

		err := service.ValidateMission(ctx, mission)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max_tokens too low")
	})
}

func TestDefaultMissionService_ValidateMission_Workflow(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)
	ctx := context.Background()

	t.Run("workflow exists in store", func(t *testing.T) {
		workflowID := types.NewID()
		mockStore := &mockWorkflowStore{
			workflows: map[types.ID]interface{}{
				workflowID: map[string]interface{}{"name": "test"},
			},
		}

		service := NewMissionService(store, mockStore, nil)

		mission := createTestMission(t)
		mission.WorkflowID = workflowID

		err := service.ValidateMission(ctx, mission)
		assert.NoError(t, err)
	})

	t.Run("error when workflow not found", func(t *testing.T) {
		mockStore := &mockWorkflowStore{
			workflows: make(map[types.ID]interface{}),
		}

		service := NewMissionService(store, mockStore, nil)

		mission := createTestMission(t)
		mission.WorkflowID = types.NewID()

		err := service.ValidateMission(ctx, mission)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "workflow validation failed")
	})
}
