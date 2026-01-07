package mission

import (
	"context"

	"github.com/zero-day-ai/gibson/internal/types"
)

// mockMissionStore is a minimal mock implementation of MissionStore for testing
type mockMissionStore struct{}

func (m *mockMissionStore) Save(ctx context.Context, mission *Mission) error {
	return nil
}

func (m *mockMissionStore) Create(ctx context.Context, mission *Mission) error {
	return nil
}

func (m *mockMissionStore) Get(ctx context.Context, id types.ID) (*Mission, error) {
	return nil, nil
}

func (m *mockMissionStore) GetByName(ctx context.Context, name string) (*Mission, error) {
	return nil, nil
}

func (m *mockMissionStore) List(ctx context.Context, filter *MissionFilter) ([]*Mission, error) {
	return []*Mission{}, nil
}

func (m *mockMissionStore) Update(ctx context.Context, mission *Mission) error {
	return nil
}

func (m *mockMissionStore) UpdateStatus(ctx context.Context, id types.ID, status MissionStatus) error {
	return nil
}

func (m *mockMissionStore) UpdateProgress(ctx context.Context, id types.ID, progress float64) error {
	return nil
}

func (m *mockMissionStore) Delete(ctx context.Context, id types.ID) error {
	return nil
}

func (m *mockMissionStore) GetByTargetID(ctx context.Context, targetID types.ID) ([]*Mission, error) {
	return []*Mission{}, nil
}

func (m *mockMissionStore) GetByTarget(ctx context.Context, targetID types.ID) ([]*Mission, error) {
	return []*Mission{}, nil
}

func (m *mockMissionStore) GetActive(ctx context.Context) ([]*Mission, error) {
	return []*Mission{}, nil
}

func (m *mockMissionStore) SaveCheckpoint(ctx context.Context, missionID types.ID, checkpoint *MissionCheckpoint) error {
	return nil
}

func (m *mockMissionStore) GetCheckpoint(ctx context.Context, missionID types.ID) (*MissionCheckpoint, error) {
	return nil, nil
}

func (m *mockMissionStore) GetCheckpoints(ctx context.Context, missionID types.ID) ([]*MissionCheckpoint, error) {
	return []*MissionCheckpoint{}, nil
}

func (m *mockMissionStore) ListCheckpoints(ctx context.Context, missionID types.ID) ([]*MissionCheckpoint, error) {
	return []*MissionCheckpoint{}, nil
}

func (m *mockMissionStore) DeleteCheckpoint(ctx context.Context, missionID types.ID, checkpointID types.ID) error {
	return nil
}

func (m *mockMissionStore) Count(ctx context.Context, filter *MissionFilter) (int, error) {
	return 0, nil
}
