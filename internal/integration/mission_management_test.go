package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// TestMissionManagementEndToEnd tests mission management lifecycle including:
// - Creating parent and child missions with lineage tracking
// - Spawn limit enforcement (max children per parent)
// - Depth limit enforcement (max hierarchy depth)
func TestMissionManagementEndToEnd(t *testing.T) {
	ctx := context.Background()

	// Create in-memory mission store
	store := newTestMissionStore()

	// Create mock orchestrator
	orchestrator := newTestOrchestrator(store)

	// Create mission client with spawn limits:
	// - Max 3 children per parent
	// - Max 10 concurrent missions system-wide
	// - Max depth of 2 (depth 0, 1 allowed; depth 2 rejected)
	client := mission.NewMissionClient(
		store,
		orchestrator,
		mission.WithSpawnLimits(3, 10, 2),
	)

	// Root mission IDs for testing
	rootMissionID := types.NewID()
	rootTargetID := types.NewID()

	t.Run("CreateChildMissions", func(t *testing.T) {
		// Create first child mission
		wf1 := createTestWorkflow("Child Workflow 1", "scan-1")
		
		req1 := &mission.CreateMissionRequest{
			Workflow:        wf1,
			TargetID:        rootTargetID,
			ParentMissionID: &rootMissionID,
			ParentDepth:     0,
			Name:            "Child Mission 1",
			Tags:            []string{"test", "child"},
		}

		child1, err := client.Create(ctx, req1)
		require.NoError(t, err, "should create first child")
		assert.Equal(t, 1, child1.Depth, "child depth should be 1")
		assert.Equal(t, &rootMissionID, child1.ParentMissionID, "parent ID should match")

		// Create second child mission
		wf2 := createTestWorkflow("Child Workflow 2", "scan-2")
		
		req2 := &mission.CreateMissionRequest{
			Workflow:        wf2,
			TargetID:        rootTargetID,
			ParentMissionID: &rootMissionID,
			ParentDepth:     0,
			Name:            "Child Mission 2",
			Tags:            []string{"test", "child"},
		}

		child2, err := client.Create(ctx, req2)
		require.NoError(t, err, "should create second child")
		assert.Equal(t, 1, child2.Depth, "child depth should be 1")

		// Create third child mission (at limit)
		wf3 := createTestWorkflow("Child Workflow 3", "scan-3")
		
		req3 := &mission.CreateMissionRequest{
			Workflow:        wf3,
			TargetID:        rootTargetID,
			ParentMissionID: &rootMissionID,
			ParentDepth:     0,
			Name:            "Child Mission 3",
			Tags:            []string{"test", "child"},
		}

		child3, err := client.Create(ctx, req3)
		require.NoError(t, err, "should create third child")
		assert.Equal(t, 1, child3.Depth, "child depth should be 1")

		t.Logf("Created 3 child missions: %s, %s, %s", child1.ID, child2.ID, child3.ID)
	})

	t.Run("VerifyLineageTracking", func(t *testing.T) {
		// List all missions
		filter := &mission.MissionFilter{
			Limit: 100,
		}

		missions, err := client.List(ctx, filter)
		require.NoError(t, err, "should list missions")

		// Count children of root mission
		childCount := 0
		for _, m := range missions {
			if m.ParentMissionID != nil && *m.ParentMissionID == rootMissionID {
				childCount++
			}
		}

		assert.Equal(t, 3, childCount, "should have exactly 3 children of root mission")
		t.Logf("Verified lineage: %d children of parent %s", childCount, rootMissionID)
	})

	t.Run("EnforceSpawnLimit", func(t *testing.T) {
		// Try to create 4th child (should fail due to limit of 3)
		wf4 := createTestWorkflow("Child Workflow 4", "scan-4")
		
		req4 := &mission.CreateMissionRequest{
			Workflow:        wf4,
			TargetID:        rootTargetID,
			ParentMissionID: &rootMissionID,
			ParentDepth:     0,
			Name:            "Child Mission 4 - Should Fail",
		}

		_, err := client.Create(ctx, req4)
		require.Error(t, err, "should fail due to child limit")
		assert.Contains(t, err.Error(), "limit", "error should mention limit")

		t.Logf("Spawn limit enforced correctly: %v", err)
	})

	t.Run("EnforceDepthLimit", func(t *testing.T) {
		// Create separate depth-0 mission
		depth0ID := types.NewID()
		depth0Target := types.NewID()

		// Create depth-1 child
		wf1 := createTestWorkflow("Depth 1 Workflow", "depth-1")
		
		req1 := &mission.CreateMissionRequest{
			Workflow:        wf1,
			TargetID:        depth0Target,
			ParentMissionID: &depth0ID,
			ParentDepth:     0,
			Name:            "Depth 1 Mission",
		}

		depth1Mission, err := client.Create(ctx, req1)
		require.NoError(t, err, "should create depth 1 mission")
		assert.Equal(t, 1, depth1Mission.Depth, "depth should be 1")

		// Try to create depth-2 child (should fail, max depth is 2 meaning 0-1 only)
		wf2 := createTestWorkflow("Depth 2 Workflow", "depth-2")
		
		req2 := &mission.CreateMissionRequest{
			Workflow:        wf2,
			TargetID:        depth0Target,
			ParentMissionID: &depth1Mission.ID,
			ParentDepth:     depth1Mission.Depth,
			Name:            "Depth 2 Mission - Should Fail",
		}

		_, err = client.Create(ctx, req2)
		require.Error(t, err, "should fail due to depth limit")
		assert.Contains(t, err.Error(), "depth", "error should mention depth")

		t.Logf("Depth limit enforced correctly: %v", err)
	})

	t.Run("CancelMission", func(t *testing.T) {
		// Create a mission to cancel (new parent to avoid limits)
		cancelParent := types.NewID()
		cancelTarget := types.NewID()

		wf := createTestWorkflow("Cancel Test", "cancel-node")
		
		req := &mission.CreateMissionRequest{
			Workflow:        wf,
			TargetID:        cancelTarget,
			ParentMissionID: &cancelParent,
			ParentDepth:     0,
			Name:            "Mission To Cancel",
		}

		m, err := client.Create(ctx, req)
		require.NoError(t, err, "should create mission to cancel")

		// Cancel it
		err = client.Cancel(ctx, m.ID)
		require.NoError(t, err, "should cancel mission")

		// Verify it's cancelled
		cancelled, err := store.Get(ctx, m.ID)
		require.NoError(t, err, "should get cancelled mission")
		assert.Equal(t, mission.MissionStatusCancelled, cancelled.Status, "status should be cancelled")

		t.Logf("Mission cancelled successfully: %s", m.ID)
	})
}

// createTestWorkflow creates a simple workflow for testing
func createTestWorkflow(name, nodeID string) *workflow.Workflow {
	wf := &workflow.Workflow{
		ID:          types.NewID(),
		Name:        name,
		Description: "Test workflow",
		Nodes: map[string]*workflow.WorkflowNode{
			nodeID: {
				ID:        nodeID,
				Type:      workflow.NodeTypeAgent,
				Name:      "test-agent",
				AgentName: "test",
			},
		},
		EntryPoints: []string{nodeID},
		ExitPoints:  []string{nodeID},
		CreatedAt:   time.Now(),
	}
	return wf
}

// testMissionStore is an in-memory mission store for testing
type testMissionStore struct {
	mu       sync.RWMutex
	missions map[types.ID]*mission.Mission
}

func newTestMissionStore() *testMissionStore {
	return &testMissionStore{
		missions: make(map[types.ID]*mission.Mission),
	}
}

func (s *testMissionStore) Save(ctx context.Context, m *mission.Mission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.missions[m.ID] = m
	return nil
}

func (s *testMissionStore) Get(ctx context.Context, id types.ID) (*mission.Mission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.missions[id]
	if !ok {
		return nil, fmt.Errorf("mission not found: %s", id)
	}
	return m, nil
}

func (s *testMissionStore) GetByName(ctx context.Context, name string) (*mission.Mission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.missions {
		if m.Name == name {
			return m, nil
		}
	}
	return nil, fmt.Errorf("mission not found: %s", name)
}

func (s *testMissionStore) Update(ctx context.Context, m *mission.Mission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.missions[m.ID]; !ok {
		return fmt.Errorf("mission not found: %s", m.ID)
	}
	m.UpdatedAt = time.Now()
	s.missions[m.ID] = m
	return nil
}

func (s *testMissionStore) UpdateStatus(ctx context.Context, id types.ID, status mission.MissionStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.missions[id]
	if !ok {
		return fmt.Errorf("mission not found: %s", id)
	}
	m.Status = status
	m.UpdatedAt = time.Now()
	return nil
}

func (s *testMissionStore) UpdateProgress(ctx context.Context, id types.ID, progress float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.missions[id]
	if !ok {
		return fmt.Errorf("mission not found: %s", id)
	}
	m.Progress = progress
	m.UpdatedAt = time.Now()
	return nil
}

func (s *testMissionStore) Delete(ctx context.Context, id types.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.missions, id)
	return nil
}

func (s *testMissionStore) GetByTarget(ctx context.Context, targetID types.ID) ([]*mission.Mission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*mission.Mission
	for _, m := range s.missions {
		if m.TargetID == targetID {
			result = append(result, m)
		}
	}
	return result, nil
}

func (s *testMissionStore) GetActive(ctx context.Context) ([]*mission.Mission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*mission.Mission
	for _, m := range s.missions {
		if m.Status == mission.MissionStatusRunning || m.Status == mission.MissionStatusPaused {
			result = append(result, m)
		}
	}
	return result, nil
}

func (s *testMissionStore) SaveCheckpoint(ctx context.Context, missionID types.ID, checkpoint *mission.MissionCheckpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.missions[missionID]
	if !ok {
		return fmt.Errorf("mission not found: %s", missionID)
	}
	m.Checkpoint = checkpoint
	return nil
}

func (s *testMissionStore) List(ctx context.Context, filter *mission.MissionFilter) ([]*mission.Mission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*mission.Mission
	for _, m := range s.missions {
		// Apply filters
		if filter != nil {
			if filter.Status != nil && m.Status != *filter.Status {
				continue
			}
			if filter.TargetID != nil && m.TargetID != *filter.TargetID {
				continue
			}
			if filter.WorkflowID != nil && m.WorkflowID != *filter.WorkflowID {
				continue
			}
		}
		result = append(result, m)
	}
	// Apply limits
	if filter != nil {
		if filter.Offset > 0 && filter.Offset < len(result) {
			result = result[filter.Offset:]
		}
		if filter.Limit > 0 && filter.Limit < len(result) {
			result = result[:filter.Limit]
		}
	}
	return result, nil
}

func (s *testMissionStore) Count(ctx context.Context, filter *mission.MissionFilter) (int, error) {
	missions, err := s.List(ctx, filter)
	return len(missions), err
}

func (s *testMissionStore) GetByNameAndStatus(ctx context.Context, name string, status mission.MissionStatus) (*mission.Mission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.missions {
		if m.Name == name && m.Status == status {
			return m, nil
		}
	}
	return nil, fmt.Errorf("mission not found: %s (status=%s)", name, status)
}

func (s *testMissionStore) ListByName(ctx context.Context, name string, limit int) ([]*mission.Mission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*mission.Mission
	for _, m := range s.missions {
		if m.Name == name {
			result = append(result, m)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (s *testMissionStore) GetLatestByName(ctx context.Context, name string) (*mission.Mission, error) {
	missions, err := s.ListByName(ctx, name, 1)
	if err != nil {
		return nil, err
	}
	if len(missions) == 0 {
		return nil, fmt.Errorf("mission not found: %s", name)
	}
	return missions[0], nil
}

func (s *testMissionStore) IncrementRunNumber(ctx context.Context, name string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	maxRun := 0
	for _, m := range s.missions {
		if m.Name == name && m.RunNumber > maxRun {
			maxRun = m.RunNumber
		}
	}
	return maxRun + 1, nil
}

// testOrchestrator is a mock orchestrator for testing
type testOrchestrator struct {
	store *testMissionStore
}

func newTestOrchestrator(store *testMissionStore) *testOrchestrator {
	return &testOrchestrator{store: store}
}

func (o *testOrchestrator) Execute(ctx context.Context, m *mission.Mission) (*mission.MissionResult, error) {
	// Simulate mission execution
	startedAt := time.Now()
	m.Status = mission.MissionStatusRunning
	m.StartedAt = &startedAt
	o.store.Update(ctx, m)

	time.Sleep(10 * time.Millisecond) // Simulate work

	completedAt := time.Now()
	m.Status = mission.MissionStatusCompleted
	m.CompletedAt = &completedAt
	o.store.Update(ctx, m)

	return &mission.MissionResult{
		MissionID:   m.ID,
		Status:      mission.MissionStatusCompleted,
		FindingIDs:  []types.ID{},
		CompletedAt: completedAt,
		Metrics: &mission.MissionMetrics{
			StartedAt:    startedAt,
			LastUpdateAt: completedAt,
			Duration:     completedAt.Sub(startedAt),
		},
	}, nil
}
