package views

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/types"
)

// MockMissionStore is a simple mock for testing
type MockMissionStore struct {
	missions []*mission.Mission
	err      error
}

func (m *MockMissionStore) Save(ctx context.Context, mission *mission.Mission) error {
	return m.err
}

func (m *MockMissionStore) Get(ctx context.Context, id types.ID) (*mission.Mission, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.missions) > 0 {
		return m.missions[0], nil
	}
	return nil, nil
}

func (m *MockMissionStore) GetByName(ctx context.Context, name string) (*mission.Mission, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.missions) > 0 {
		return m.missions[0], nil
	}
	return nil, nil
}

func (m *MockMissionStore) List(ctx context.Context, filter *mission.MissionFilter) ([]*mission.Mission, error) {
	return m.missions, m.err
}

func (m *MockMissionStore) Update(ctx context.Context, mission *mission.Mission) error {
	return m.err
}

func (m *MockMissionStore) UpdateStatus(ctx context.Context, id types.ID, status mission.MissionStatus) error {
	return m.err
}

func (m *MockMissionStore) UpdateProgress(ctx context.Context, id types.ID, progress float64) error {
	return m.err
}

func (m *MockMissionStore) Delete(ctx context.Context, id types.ID) error {
	return m.err
}

func (m *MockMissionStore) GetByTarget(ctx context.Context, targetID types.ID) ([]*mission.Mission, error) {
	return m.missions, m.err
}

func (m *MockMissionStore) GetActive(ctx context.Context) ([]*mission.Mission, error) {
	return m.missions, m.err
}

func (m *MockMissionStore) SaveCheckpoint(ctx context.Context, missionID types.ID, checkpoint *mission.MissionCheckpoint) error {
	return m.err
}

func (m *MockMissionStore) Count(ctx context.Context, filter *mission.MissionFilter) (int, error) {
	if m.err != nil {
		return 0, m.err
	}
	return len(m.missions), nil
}

func (m *MockMissionStore) GetByNameAndStatus(ctx context.Context, name string, status mission.MissionStatus) (*mission.Mission, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.missions) > 0 {
		return m.missions[0], nil
	}
	return nil, nil
}

func (m *MockMissionStore) ListByName(ctx context.Context, name string, limit int) ([]*mission.Mission, error) {
	return m.missions, m.err
}

func (m *MockMissionStore) GetLatestByName(ctx context.Context, name string) (*mission.Mission, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.missions) > 0 {
		return m.missions[0], nil
	}
	return nil, nil
}

func (m *MockMissionStore) IncrementRunNumber(ctx context.Context, name string) (int, error) {
	if m.err != nil {
		return 0, m.err
	}
	return 1, nil
}

func TestNewMissionView(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockMissionStore{}
	view := NewMissionView(ctx, mockStore, "/tmp/gibson-test")

	assert.NotNil(t, view)
	assert.NotNil(t, view.theme)
	assert.NotNil(t, view.list)
	assert.NotNil(t, view.logViewport)
	assert.False(t, view.detailsExpanded)
}

func TestMissionView_Init(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockMissionStore{
		missions: []*mission.Mission{
			{ID: "1", Name: "Test Mission", Status: mission.MissionStatusRunning},
		},
	}
	view := NewMissionView(ctx, mockStore, "/tmp/gibson-test")

	cmd := view.Init()

	// Init should return a command to load missions
	assert.NotNil(t, cmd)
}

func TestMissionView_Update_WindowSize(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockMissionStore{}
	view := NewMissionView(ctx, mockStore, "/tmp/gibson-test")

	msg := tea.WindowSizeMsg{Width: 150, Height: 50}
	_, _ = view.Update(msg)

	assert.Equal(t, 150, view.width)
	assert.Equal(t, 50, view.height)
}

func TestMissionView_Update_ToggleDetails(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockMissionStore{}
	view := NewMissionView(ctx, mockStore, "/tmp/gibson-test")
	view.width = 100
	view.height = 30

	assert.False(t, view.detailsExpanded)

	// Press Enter to expand
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, _ = view.Update(msg)

	assert.True(t, view.detailsExpanded)

	// Press Enter again to collapse
	_, _ = view.Update(msg)

	assert.False(t, view.detailsExpanded)
}

func TestMissionView_View_Empty(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockMissionStore{}
	view := NewMissionView(ctx, mockStore, "/tmp/gibson-test")

	// Before setting size
	output := view.View()
	assert.Equal(t, "Loading...", output)

	// After setting size
	view.width = 100
	view.height = 30
	output = view.View()
	assert.NotEmpty(t, output)
}

func TestMissionView_RenderProgressBar(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockMissionStore{}
	view := NewMissionView(ctx, mockStore, "/tmp/gibson-test")

	tests := []struct {
		progress float64
		contains string
	}{
		{0.0, "0%"},
		{0.5, "50%"},
		{1.0, "100%"},
	}

	for _, tt := range tests {
		t.Run(tt.contains, func(t *testing.T) {
			result := view.renderProgressBar(tt.progress)
			assert.Contains(t, result, tt.contains)
			assert.Contains(t, result, "[")
			assert.Contains(t, result, "]")
		})
	}
}

func TestMissionView_RenderStatusBar(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockMissionStore{}
	view := NewMissionView(ctx, mockStore, "/tmp/gibson-test")
	view.width = 100

	output := view.renderStatusBar()

	assert.NotEmpty(t, output)
	assert.Contains(t, output, "j/k")
	assert.Contains(t, output, "enter")
}

func TestMissionView_RenderStatusBar_WithError(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockMissionStore{}
	view := NewMissionView(ctx, mockStore, "/tmp/gibson-test")
	view.width = 100
	view.err = assert.AnError

	output := view.renderStatusBar()

	assert.NotEmpty(t, output)
	assert.Contains(t, output, "Error")
}

func TestMissionSummary_Interfaces(t *testing.T) {
	now := time.Now()
	summary := MissionSummary{
		ID:           "test-id",
		Name:         "Test Mission",
		Status:       mission.MissionStatusRunning,
		Progress:     0.5,
		FindingCount: 10,
		StartedAt:    &now,
		Duration:     time.Hour,
	}

	// Test list.Item interface
	assert.Equal(t, "Test Mission", summary.FilterValue())
	assert.Equal(t, "Test Mission", summary.Title())

	desc := summary.Description()
	assert.Contains(t, desc, "running")
	assert.Contains(t, desc, "50%")
	assert.Contains(t, desc, "10")
}

func TestMissionsLoadedMsg(t *testing.T) {
	missions := []MissionSummary{
		{ID: "1", Name: "Mission 1"},
		{ID: "2", Name: "Mission 2"},
	}
	msg := missionsLoadedMsg{missions: missions}

	assert.Len(t, msg.missions, 2)
}

func TestMissionDetailsLoadedMsg(t *testing.T) {
	m := &mission.Mission{ID: "1", Name: "Test"}
	msg := missionDetailsLoadedMsg{mission: m, workflow: nil}

	assert.Equal(t, m, msg.mission)
	assert.Nil(t, msg.workflow)
}

func TestErrMsg(t *testing.T) {
	msg := errMsg{error: assert.AnError}
	assert.Error(t, msg.error)
}
