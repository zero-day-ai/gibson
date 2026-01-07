//go:build fts5

package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/types"
)

// mockMissionStore implements mission.MissionStore for testing
type mockMissionStore struct {
	missions         []*mission.Mission
	saveFn           func(ctx context.Context, m *mission.Mission) error
	getFn            func(ctx context.Context, id types.ID) (*mission.Mission, error)
	getByNameFn      func(ctx context.Context, name string) (*mission.Mission, error)
	listFn           func(ctx context.Context, filter *mission.MissionFilter) ([]*mission.Mission, error)
	updateFn         func(ctx context.Context, m *mission.Mission) error
	updateStatusFn   func(ctx context.Context, id types.ID, status mission.MissionStatus) error
	updateProgressFn func(ctx context.Context, id types.ID, progress float64) error
	deleteFn         func(ctx context.Context, id types.ID) error
	getByTargetFn    func(ctx context.Context, targetID types.ID) ([]*mission.Mission, error)
	getActiveFn      func(ctx context.Context) ([]*mission.Mission, error)
	saveCheckpointFn func(ctx context.Context, missionID types.ID, checkpoint *mission.MissionCheckpoint) error
	countFn          func(ctx context.Context, filter *mission.MissionFilter) (int, error)
}

func (m *mockMissionStore) Save(ctx context.Context, mission *mission.Mission) error {
	if m.saveFn != nil {
		return m.saveFn(ctx, mission)
	}
	m.missions = append(m.missions, mission)
	return nil
}

func (m *mockMissionStore) Get(ctx context.Context, id types.ID) (*mission.Mission, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	for _, mis := range m.missions {
		if mis.ID == id {
			return mis, nil
		}
	}
	return nil, mission.NewNotFoundError(id.String())
}

func (m *mockMissionStore) GetByName(ctx context.Context, name string) (*mission.Mission, error) {
	if m.getByNameFn != nil {
		return m.getByNameFn(ctx, name)
	}
	for _, mission := range m.missions {
		if mission.Name == name {
			return mission, nil
		}
	}
	return nil, mission.NewNotFoundError(name)
}

func (m *mockMissionStore) List(ctx context.Context, filter *mission.MissionFilter) ([]*mission.Mission, error) {
	if m.listFn != nil {
		return m.listFn(ctx, filter)
	}
	if filter == nil || filter.Status == nil {
		return m.missions, nil
	}
	var filtered []*mission.Mission
	for _, mission := range m.missions {
		if mission.Status == *filter.Status {
			filtered = append(filtered, mission)
		}
	}
	return filtered, nil
}

func (m *mockMissionStore) Update(ctx context.Context, mis *mission.Mission) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, mis)
	}
	for i, existing := range m.missions {
		if existing.ID == mis.ID {
			m.missions[i] = mis
			return nil
		}
	}
	return mission.NewNotFoundError(mis.ID.String())
}

func (m *mockMissionStore) UpdateStatus(ctx context.Context, id types.ID, status mission.MissionStatus) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, status)
	}
	for _, existing := range m.missions {
		if existing.ID == id {
			existing.Status = status
			return nil
		}
	}
	return mission.NewNotFoundError(id.String())
}

func (m *mockMissionStore) UpdateProgress(ctx context.Context, id types.ID, progress float64) error {
	if m.updateProgressFn != nil {
		return m.updateProgressFn(ctx, id, progress)
	}
	for _, existing := range m.missions {
		if existing.ID == id {
			existing.Progress = progress
			return nil
		}
	}
	return mission.NewNotFoundError(id.String())
}

func (m *mockMissionStore) Delete(ctx context.Context, id types.ID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	for i, existing := range m.missions {
		if existing.ID == id {
			m.missions = append(m.missions[:i], m.missions[i+1:]...)
			return nil
		}
	}
	return mission.NewNotFoundError(id.String())
}

func (m *mockMissionStore) GetByTarget(ctx context.Context, targetID types.ID) ([]*mission.Mission, error) {
	if m.getByTargetFn != nil {
		return m.getByTargetFn(ctx, targetID)
	}
	var filtered []*mission.Mission
	for _, mission := range m.missions {
		if mission.TargetID == targetID {
			filtered = append(filtered, mission)
		}
	}
	return filtered, nil
}

func (m *mockMissionStore) GetActive(ctx context.Context) ([]*mission.Mission, error) {
	if m.getActiveFn != nil {
		return m.getActiveFn(ctx)
	}
	var active []*mission.Mission
	for _, mis := range m.missions {
		if mis.Status == mission.MissionStatusRunning || mis.Status == mission.MissionStatusPaused {
			active = append(active, mis)
		}
	}
	return active, nil
}

func (m *mockMissionStore) SaveCheckpoint(ctx context.Context, missionID types.ID, checkpoint *mission.MissionCheckpoint) error {
	if m.saveCheckpointFn != nil {
		return m.saveCheckpointFn(ctx, missionID, checkpoint)
	}
	for _, existing := range m.missions {
		if existing.ID == missionID {
			existing.Checkpoint = checkpoint
			return nil
		}
	}
	return mission.NewNotFoundError(missionID.String())
}

func (m *mockMissionStore) Count(ctx context.Context, filter *mission.MissionFilter) (int, error) {
	if m.countFn != nil {
		return m.countFn(ctx, filter)
	}
	missions, err := m.List(ctx, filter)
	if err != nil {
		return 0, err
	}
	return len(missions), nil
}

func TestMissionList(t *testing.T) {
	now := time.Now()
	targetID := types.NewID()
	workflowID := types.NewID()

	tests := []struct {
		name         string
		statusFilter string
		setupMock    func(*mockMissionStore)
		wantErr      bool
		wantCount    int
		wantErrMsg   string
	}{
		{
			name:         "list all missions",
			statusFilter: "",
			setupMock: func(m *mockMissionStore) {
				m.missions = []*mission.Mission{
					{
						ID:         types.NewID(),
						Name:       "mission1",
						Status:     mission.MissionStatusRunning,
						TargetID:   targetID,
						WorkflowID: workflowID,
						CreatedAt:  now,
						UpdatedAt:  now,
					},
					{
						ID:         types.NewID(),
						Name:       "mission2",
						Status:     mission.MissionStatusCompleted,
						TargetID:   targetID,
						WorkflowID: workflowID,
						CreatedAt:  now,
						UpdatedAt:  now,
					},
				}
			},
			wantCount: 2,
		},
		{
			name:         "filter by running status",
			statusFilter: "running",
			setupMock: func(m *mockMissionStore) {
				m.missions = []*mission.Mission{
					{
						ID:         types.NewID(),
						Name:       "mission1",
						Status:     mission.MissionStatusRunning,
						TargetID:   targetID,
						WorkflowID: workflowID,
						CreatedAt:  now,
						UpdatedAt:  now,
					},
					{
						ID:         types.NewID(),
						Name:       "mission2",
						Status:     mission.MissionStatusCompleted,
						TargetID:   targetID,
						WorkflowID: workflowID,
						CreatedAt:  now,
						UpdatedAt:  now,
					},
				}
			},
			wantCount: 1,
		},
		{
			name:         "filter by completed status",
			statusFilter: "completed",
			setupMock: func(m *mockMissionStore) {
				m.missions = []*mission.Mission{
					{
						ID:         types.NewID(),
						Name:       "mission1",
						Status:     mission.MissionStatusRunning,
						TargetID:   targetID,
						WorkflowID: workflowID,
						CreatedAt:  now,
						UpdatedAt:  now,
					},
					{
						ID:         types.NewID(),
						Name:       "mission2",
						Status:     mission.MissionStatusCompleted,
						TargetID:   targetID,
						WorkflowID: workflowID,
						CreatedAt:  now,
						UpdatedAt:  now,
					},
				}
			},
			wantCount: 1,
		},
		{
			name:         "invalid status filter",
			statusFilter: "invalid-status",
			setupMock:    func(m *mockMissionStore) {},
			wantErr:      false, // Returns CommandResult with error, not actual error
			wantCount:    0,
			wantErrMsg:   "invalid status filter",
		},
		{
			name:         "empty list",
			statusFilter: "",
			setupMock: func(m *mockMissionStore) {
				m.missions = []*mission.Mission{}
			},
			wantCount: 0,
		},
		{
			name:         "nil mission store",
			statusFilter: "",
			setupMock:    nil,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var store mission.MissionStore
			if tt.setupMock != nil {
				mockStore := &mockMissionStore{}
				tt.setupMock(mockStore)
				store = mockStore
			}

			cc := &CommandContext{
				Ctx:          context.Background(),
				MissionStore: store,
			}

			result, err := MissionList(cc, tt.statusFilter)

			if tt.wantErr {
				if err == nil {
					t.Errorf("MissionList() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("MissionList() unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("MissionList() returned nil result")
			}

			// Check for error in result
			if tt.wantErrMsg != "" {
				if result.Error == nil {
					t.Errorf("MissionList() expected error in result, got nil")
				} else if result.Error.Error() != tt.wantErrMsg && !contains(result.Error.Error(), tt.wantErrMsg) {
					t.Errorf("MissionList() error = %v, wantErrMsg %v", result.Error, tt.wantErrMsg)
				}
				return
			}

			if result.Error != nil {
				t.Errorf("MissionList() unexpected error in result: %v", result.Error)
			}

			data, ok := result.Data.(*MissionListResult)
			if !ok {
				t.Fatalf("MissionList() result.Data is not *MissionListResult")
			}

			if data.Count != tt.wantCount {
				t.Errorf("MissionList() count = %d, want %d", data.Count, tt.wantCount)
			}

			if len(data.Missions) != tt.wantCount {
				t.Errorf("MissionList() missions length = %d, want %d", len(data.Missions), tt.wantCount)
			}
		})
	}
}

func TestMissionShow(t *testing.T) {
	now := time.Now()
	targetID := types.NewID()
	workflowID := types.NewID()

	tests := []struct {
		name        string
		missionName string
		setupMock   func(*mockMissionStore)
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:        "existing mission",
			missionName: "test-mission",
			setupMock: func(m *mockMissionStore) {
				m.missions = []*mission.Mission{
					{
						ID:         types.NewID(),
						Name:       "test-mission",
						Status:     mission.MissionStatusRunning,
						TargetID:   targetID,
						WorkflowID: workflowID,
						CreatedAt:  now,
						UpdatedAt:  now,
					},
				}
			},
		},
		{
			name:        "non-existing mission",
			missionName: "non-existing",
			setupMock: func(m *mockMissionStore) {
				m.missions = []*mission.Mission{}
			},
			wantErrMsg: "failed to get mission",
		},
		{
			name:        "nil mission store",
			missionName: "test",
			setupMock:   nil,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var store mission.MissionStore
			if tt.setupMock != nil {
				mockStore := &mockMissionStore{}
				tt.setupMock(mockStore)
				store = mockStore
			}

			cc := &CommandContext{
				Ctx:          context.Background(),
				MissionStore: store,
			}

			result, err := MissionShow(cc, tt.missionName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("MissionShow() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("MissionShow() unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("MissionShow() returned nil result")
			}

			if tt.wantErrMsg != "" {
				if result.Error == nil {
					t.Errorf("MissionShow() expected error in result, got nil")
				} else if !contains(result.Error.Error(), tt.wantErrMsg) {
					t.Errorf("MissionShow() error = %v, wantErrMsg %v", result.Error, tt.wantErrMsg)
				}
				return
			}

			if result.Error != nil {
				t.Errorf("MissionShow() unexpected error in result: %v", result.Error)
			}

			mission, ok := result.Data.(*mission.Mission)
			if !ok {
				t.Fatalf("MissionShow() result.Data is not *mission.Mission")
			}

			if mission.Name != tt.missionName {
				t.Errorf("MissionShow() mission name = %s, want %s", mission.Name, tt.missionName)
			}
		})
	}
}

func TestMissionRun(t *testing.T) {
	// Create a temporary workflow file for testing
	tempDir := t.TempDir()
	validWorkflowPath := filepath.Join(tempDir, "valid-workflow.yaml")
	validWorkflow := `
name: test-workflow
description: Test workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
entry_points:
  - node1
exit_points:
  - node1
`
	if err := os.WriteFile(validWorkflowPath, []byte(validWorkflow), 0644); err != nil {
		t.Fatalf("Failed to create test workflow file: %v", err)
	}

	tests := []struct {
		name         string
		workflowFile string
		setupMock    func(*mockMissionStore)
		wantErr      bool
		wantErrMsg   string
	}{
		{
			name:         "valid workflow file",
			workflowFile: validWorkflowPath,
			setupMock:    func(m *mockMissionStore) {},
		},
		{
			name:         "non-existing file",
			workflowFile: "/nonexistent/workflow.yaml",
			setupMock:    func(m *mockMissionStore) {},
			wantErrMsg:   "failed to parse workflow file",
		},
		{
			name:         "nil mission store",
			workflowFile: validWorkflowPath,
			setupMock:    nil,
			wantErr:      true,
		},
		{
			name:         "save error",
			workflowFile: validWorkflowPath,
			setupMock: func(m *mockMissionStore) {
				m.saveFn = func(ctx context.Context, mission *mission.Mission) error {
					return errors.New("database error")
				}
			},
			wantErrMsg: "failed to create mission",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var store mission.MissionStore
			if tt.setupMock != nil {
				mockStore := &mockMissionStore{}
				tt.setupMock(mockStore)
				store = mockStore
			}

			cc := &CommandContext{
				Ctx:          context.Background(),
				MissionStore: store,
			}

			result, err := MissionRun(cc, tt.workflowFile)

			if tt.wantErr {
				if err == nil {
					t.Errorf("MissionRun() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("MissionRun() unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("MissionRun() returned nil result")
			}

			if tt.wantErrMsg != "" {
				if result.Error == nil {
					t.Errorf("MissionRun() expected error in result, got nil")
				} else if !contains(result.Error.Error(), tt.wantErrMsg) {
					t.Errorf("MissionRun() error = %v, wantErrMsg %v", result.Error, tt.wantErrMsg)
				}
				return
			}

			if result.Error != nil {
				t.Errorf("MissionRun() unexpected error in result: %v", result.Error)
			}

			data, ok := result.Data.(*MissionRunResult)
			if !ok {
				t.Fatalf("MissionRun() result.Data is not *MissionRunResult")
			}

			if data.Mission.Status != mission.MissionStatusRunning {
				t.Errorf("MissionRun() mission status = %s, want %s", data.Mission.Status, mission.MissionStatusRunning)
			}
		})
	}
}

func TestMissionResume(t *testing.T) {
	now := time.Now()
	targetID := types.NewID()
	workflowID := types.NewID()

	tests := []struct {
		name        string
		missionName string
		setupMock   func(*mockMissionStore)
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:        "resume pending mission",
			missionName: "test-mission",
			setupMock: func(m *mockMissionStore) {
				m.missions = []*mission.Mission{
					{
						ID:         types.NewID(),
						Name:       "test-mission",
						Status:     mission.MissionStatusPending,
						TargetID:   targetID,
						WorkflowID: workflowID,
						CreatedAt:  now,
						UpdatedAt:  now,
					},
				}
			},
		},
		{
			name:        "cannot resume completed mission",
			missionName: "completed-mission",
			setupMock: func(m *mockMissionStore) {
				m.missions = []*mission.Mission{
					{
						ID:         types.NewID(),
						Name:       "completed-mission",
						Status:     mission.MissionStatusCompleted,
						TargetID:   targetID,
						WorkflowID: workflowID,
						CreatedAt:  now,
						UpdatedAt:  now,
					},
				}
			},
			wantErrMsg: "cannot resume completed mission",
		},
		{
			name:        "cannot resume failed mission",
			missionName: "failed-mission",
			setupMock: func(m *mockMissionStore) {
				m.missions = []*mission.Mission{
					{
						ID:         types.NewID(),
						Name:       "failed-mission",
						Status:     mission.MissionStatusFailed,
						TargetID:   targetID,
						WorkflowID: workflowID,
						CreatedAt:  now,
						UpdatedAt:  now,
					},
				}
			},
			wantErrMsg: "cannot resume failed mission",
		},
		{
			name:        "cannot resume cancelled mission",
			missionName: "cancelled-mission",
			setupMock: func(m *mockMissionStore) {
				m.missions = []*mission.Mission{
					{
						ID:         types.NewID(),
						Name:       "cancelled-mission",
						Status:     mission.MissionStatusCancelled,
						TargetID:   targetID,
						WorkflowID: workflowID,
						CreatedAt:  now,
						UpdatedAt:  now,
					},
				}
			},
			wantErrMsg: "cannot resume cancelled mission",
		},
		{
			name:        "mission not found",
			missionName: "non-existing",
			setupMock:   func(m *mockMissionStore) {},
			wantErrMsg:  "failed to get mission",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var store mission.MissionStore
			if tt.setupMock != nil {
				mockStore := &mockMissionStore{}
				tt.setupMock(mockStore)
				store = mockStore
			}

			cc := &CommandContext{
				Ctx:          context.Background(),
				MissionStore: store,
			}

			result, err := MissionResume(cc, tt.missionName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("MissionResume() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("MissionResume() unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("MissionResume() returned nil result")
			}

			if tt.wantErrMsg != "" {
				if result.Error == nil {
					t.Errorf("MissionResume() expected error in result, got nil")
				} else if !contains(result.Error.Error(), tt.wantErrMsg) {
					t.Errorf("MissionResume() error = %v, wantErrMsg %v", result.Error, tt.wantErrMsg)
				}
				return
			}

			if result.Error != nil {
				t.Errorf("MissionResume() unexpected error in result: %v", result.Error)
			}
		})
	}
}

func TestMissionStop(t *testing.T) {
	now := time.Now()
	targetID := types.NewID()
	workflowID := types.NewID()

	tests := []struct {
		name        string
		missionName string
		setupMock   func(*mockMissionStore)
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:        "stop running mission",
			missionName: "running-mission",
			setupMock: func(m *mockMissionStore) {
				m.missions = []*mission.Mission{
					{
						ID:         types.NewID(),
						Name:       "running-mission",
						Status:     mission.MissionStatusRunning,
						TargetID:   targetID,
						WorkflowID: workflowID,
						CreatedAt:  now,
						UpdatedAt:  now,
					},
				}
			},
		},
		{
			name:        "cannot stop non-running mission",
			missionName: "pending-mission",
			setupMock: func(m *mockMissionStore) {
				m.missions = []*mission.Mission{
					{
						ID:         types.NewID(),
						Name:       "pending-mission",
						Status:     mission.MissionStatusPending,
						TargetID:   targetID,
						WorkflowID: workflowID,
						CreatedAt:  now,
						UpdatedAt:  now,
					},
				}
			},
			wantErrMsg: "mission is not running",
		},
		{
			name:        "mission not found",
			missionName: "non-existing",
			setupMock:   func(m *mockMissionStore) {},
			wantErrMsg:  "failed to get mission",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var store mission.MissionStore
			if tt.setupMock != nil {
				mockStore := &mockMissionStore{}
				tt.setupMock(mockStore)
				store = mockStore
			}

			cc := &CommandContext{
				Ctx:          context.Background(),
				MissionStore: store,
			}

			result, err := MissionStop(cc, tt.missionName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("MissionStop() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("MissionStop() unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("MissionStop() returned nil result")
			}

			if tt.wantErrMsg != "" {
				if result.Error == nil {
					t.Errorf("MissionStop() expected error in result, got nil")
				} else if !contains(result.Error.Error(), tt.wantErrMsg) {
					t.Errorf("MissionStop() error = %v, wantErrMsg %v", result.Error, tt.wantErrMsg)
				}
				return
			}

			if result.Error != nil {
				t.Errorf("MissionStop() unexpected error in result: %v", result.Error)
			}
		})
	}
}

func TestMissionDelete(t *testing.T) {
	now := time.Now()
	targetID := types.NewID()
	workflowID := types.NewID()

	tests := []struct {
		name        string
		missionName string
		force       bool
		setupMock   func(*mockMissionStore)
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:        "delete completed mission",
			missionName: "completed-mission",
			force:       false,
			setupMock: func(m *mockMissionStore) {
				missionID := types.NewID()
				m.missions = []*mission.Mission{
					{
						ID:         missionID,
						Name:       "completed-mission",
						Status:     mission.MissionStatusCompleted,
						TargetID:   targetID,
						WorkflowID: workflowID,
						CreatedAt:  now,
						UpdatedAt:  now,
					},
				}
				// Mock Delete to check terminal state
				m.deleteFn = func(ctx context.Context, id types.ID) error {
					for _, m := range m.missions {
						if m.ID == id {
							if !m.Status.IsTerminal() {
								return mission.NewInvalidStateError(m.Status, mission.MissionStatusCancelled)
							}
							return nil
						}
					}
					return mission.NewNotFoundError(id.String())
				}
			},
		},
		{
			name:        "delete with force flag",
			missionName: "test-mission",
			force:       true,
			setupMock: func(m *mockMissionStore) {
				m.missions = []*mission.Mission{
					{
						ID:         types.NewID(),
						Name:       "test-mission",
						Status:     mission.MissionStatusCompleted,
						TargetID:   targetID,
						WorkflowID: workflowID,
						CreatedAt:  now,
						UpdatedAt:  now,
					},
				}
			},
		},
		{
			name:        "mission not found",
			missionName: "non-existing",
			force:       false,
			setupMock:   func(m *mockMissionStore) {},
			wantErrMsg:  "failed to get mission",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var store mission.MissionStore
			if tt.setupMock != nil {
				mockStore := &mockMissionStore{}
				tt.setupMock(mockStore)
				store = mockStore
			}

			cc := &CommandContext{
				Ctx:          context.Background(),
				MissionStore: store,
			}

			result, err := MissionDelete(cc, tt.missionName, tt.force)

			if tt.wantErr {
				if err == nil {
					t.Errorf("MissionDelete() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("MissionDelete() unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("MissionDelete() returned nil result")
			}

			if tt.wantErrMsg != "" {
				if result.Error == nil {
					t.Errorf("MissionDelete() expected error in result, got nil")
				} else if !contains(result.Error.Error(), tt.wantErrMsg) {
					t.Errorf("MissionDelete() error = %v, wantErrMsg %v", result.Error, tt.wantErrMsg)
				}
				return
			}

			if result.Error != nil {
				t.Errorf("MissionDelete() unexpected error in result: %v", result.Error)
			}
		})
	}
}

func TestIsValidMissionStatus(t *testing.T) {
	tests := []struct {
		name   string
		status mission.MissionStatus
		want   bool
	}{
		{
			name:   "valid pending status",
			status: mission.MissionStatusPending,
			want:   true,
		},
		{
			name:   "valid running status",
			status: mission.MissionStatusRunning,
			want:   true,
		},
		{
			name:   "valid completed status",
			status: mission.MissionStatusCompleted,
			want:   true,
		},
		{
			name:   "valid failed status",
			status: mission.MissionStatusFailed,
			want:   true,
		},
		{
			name:   "valid cancelled status",
			status: mission.MissionStatusCancelled,
			want:   true,
		},
		{
			name:   "invalid status",
			status: mission.MissionStatus("invalid"),
			want:   false,
		},
		{
			name:   "empty status",
			status: mission.MissionStatus(""),
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidMissionStatus(tt.status)
			if got != tt.want {
				t.Errorf("IsValidMissionStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && fmt.Sprintf("%s", s) != "" &&
			len(s) >= len(substr) && s[:len(substr)] == substr) ||
		(len(s) > len(substr) && s[len(s)-len(substr):] == substr) ||
		(len(s) > len(substr) && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
