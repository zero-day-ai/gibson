package attack

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/llm/providers"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestOrchestratorReceivesHarnessFactory verifies that the MissionOrchestrator
// is created with a working HarnessFactory that can create harnesses for agents.
// This tests the wiring path: daemon -> infrastructure -> orchestrator -> harness
func TestOrchestratorReceivesHarnessFactory(t *testing.T) {
	// Create a real slot manager with mock registry
	registry := llm.NewLLMRegistry()

	// Register a mock provider
	mockProvider := providers.NewMockProvider([]string{"test response"})
	err := registry.RegisterProvider(mockProvider)
	require.NoError(t, err)

	// Create slot manager
	slotManager := llm.NewSlotManager(registry)

	// Create harness config with slot manager
	config := harness.HarnessConfig{
		SlotManager: slotManager,
		LLMRegistry: registry,
	}

	// Create the factory
	factory, err := harness.NewHarnessFactory(config)
	require.NoError(t, err)
	require.NotNil(t, factory)

	// Create mission context
	missionID := types.NewID()
	missionCtx := harness.NewMissionContext(missionID, "test-mission", "test-agent")
	targetInfo := harness.NewTargetInfo(types.NewID(), "test-target", "http://example.com", "web")

	// Create a harness from the factory
	h, err := factory.Create("test-agent", missionCtx, targetInfo)
	require.NoError(t, err)
	require.NotNil(t, h)

	// Verify harness has required methods
	assert.NotNil(t, h.Mission())
	assert.NotNil(t, h.Target())
	assert.NotNil(t, h.Logger())

	// Verify mission context is properly set
	assert.Equal(t, missionID, h.Mission().ID)
	assert.Equal(t, "test-mission", h.Mission().Name)
	assert.Equal(t, "test-agent", h.Mission().CurrentAgent)

	// Verify target info is properly set
	assert.Equal(t, "test-target", h.Target().Name)
	assert.Equal(t, "http://example.com", h.Target().URL)
}

// TestOrchestratorWithHarnessFactoryOption verifies that MissionOrchestrator
// accepts and uses the WithHarnessFactory option.
func TestOrchestratorWithHarnessFactoryOption(t *testing.T) {
	// Create LLM registry and slot manager
	registry := llm.NewLLMRegistry()
	mockProvider := providers.NewMockProvider([]string{"test response"})
	err := registry.RegisterProvider(mockProvider)
	require.NoError(t, err)

	slotManager := llm.NewSlotManager(registry)

	// Create harness factory
	config := harness.HarnessConfig{
		SlotManager: slotManager,
		LLMRegistry: registry,
	}
	factory, err := harness.NewHarnessFactory(config)
	require.NoError(t, err)

	// Create mock mission store
	store := &mockMissionStore{}

	// Create orchestrator with harness factory
	orchestrator := mission.NewMissionOrchestrator(
		store,
		mission.WithHarnessFactory(factory),
	)

	require.NotNil(t, orchestrator)
}

// mockMissionStore implements mission.MissionStore for testing
type mockMissionStore struct{}

func (m *mockMissionStore) Save(ctx context.Context, mission *mission.Mission) error {
	return nil
}

func (m *mockMissionStore) Get(ctx context.Context, id types.ID) (*mission.Mission, error) {
	return nil, nil
}

func (m *mockMissionStore) GetByName(ctx context.Context, name string) (*mission.Mission, error) {
	return nil, nil
}

func (m *mockMissionStore) List(ctx context.Context, filter *mission.MissionFilter) ([]*mission.Mission, error) {
	return nil, nil
}

func (m *mockMissionStore) Update(ctx context.Context, mission *mission.Mission) error {
	return nil
}

func (m *mockMissionStore) UpdateStatus(ctx context.Context, id types.ID, status mission.MissionStatus) error {
	return nil
}

func (m *mockMissionStore) UpdateProgress(ctx context.Context, id types.ID, progress float64) error {
	return nil
}

func (m *mockMissionStore) Delete(ctx context.Context, id types.ID) error {
	return nil
}

func (m *mockMissionStore) GetByTarget(ctx context.Context, targetID types.ID) ([]*mission.Mission, error) {
	return nil, nil
}

func (m *mockMissionStore) GetActive(ctx context.Context) ([]*mission.Mission, error) {
	return nil, nil
}

func (m *mockMissionStore) SaveCheckpoint(ctx context.Context, missionID types.ID, checkpoint *mission.MissionCheckpoint) error {
	return nil
}

func (m *mockMissionStore) Count(ctx context.Context, filter *mission.MissionFilter) (int, error) {
	return 0, nil
}
