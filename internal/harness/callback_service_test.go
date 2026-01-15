package harness

import (
	"context"
	"log/slog"
	"os"
	"testing"

	pb "github.com/zero-day-ai/sdk/api/gen/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewHarnessCallbackService tests the service constructor.
func TestNewHarnessCallbackService(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewHarnessCallbackService(logger)

	assert.NotNil(t, service)
	assert.NotNil(t, service.logger)
}

// TestHarnessCallbackServiceRegisterUnregister tests harness registration.
func TestHarnessCallbackServiceRegisterUnregister(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewHarnessCallbackService(logger)

	// Register a harness
	taskID := "task-123"
	service.RegisterHarness(taskID, nil)

	// Check it's registered (we can try to get it)
	_, ok := service.activeHarnesses.Load(taskID)
	assert.True(t, ok)

	// Unregister
	service.UnregisterHarness(taskID)

	// Check it's gone
	_, ok = service.activeHarnesses.Load(taskID)
	assert.False(t, ok)
}

// newMockHarness creates a minimal harness for testing.
func newMockHarness() AgentHarness {
	// Create a minimal harness with nil dependencies - we only need it for storage/retrieval
	return &DefaultAgentHarness{}
}

// TestGetHarness_WithExplicitMissionId tests getHarness with explicit MissionId field.
func TestGetHarness_WithExplicitMissionId(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create a mock harness
	mockHarn := newMockHarness()

	// Create callback harness registry and register the harness
	registry := NewCallbackHarnessRegistry()
	registry.Register("mission-123", "test-agent", mockHarn)

	// Create service with registry
	service := NewHarnessCallbackServiceWithRegistry(logger, registry)

	// Create ContextInfo with explicit MissionId
	contextInfo := &pb.ContextInfo{
		TaskId:    "task-456",
		AgentName: "test-agent",
		MissionId: "mission-123",
	}

	// Test getHarness
	harness, err := service.getHarness(context.Background(), contextInfo)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, mockHarn, harness)
}

// TestGetHarness_EmptyMissionId_ReturnsError tests that empty mission_id returns an error.
// Legacy task-based fallback is no longer supported.
func TestGetHarness_EmptyMissionId_ReturnsError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create callback harness registry
	registry := NewCallbackHarnessRegistry()

	// Create service with registry
	service := NewHarnessCallbackServiceWithRegistry(logger, registry)

	// Create ContextInfo with empty MissionId
	contextInfo := &pb.ContextInfo{
		TaskId:    "task-789",
		AgentName: "test-agent",
		MissionId: "", // Empty - should return error (legacy fallback removed)
	}

	// Test getHarness
	harness, err := service.getHarness(context.Background(), contextInfo)

	// Verify error is returned (legacy fallback no longer supported)
	require.Error(t, err)
	assert.Nil(t, harness)
	assert.Contains(t, err.Error(), "missing mission_id")
}

// TestGetHarness_LegacyTaskIdWithColon tests that legacy missionID:taskID format no longer works.
// The new implementation requires explicit MissionId field.
func TestGetHarness_LegacyTaskIdWithColon_NoLongerSupported(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create a mock harness
	mockHarn := newMockHarness()

	// Create callback harness registry and register the harness
	registry := NewCallbackHarnessRegistry()
	registry.Register("mission-abc", "test-agent", mockHarn)

	// Create service with registry
	service := NewHarnessCallbackServiceWithRegistry(logger, registry)

	// Create ContextInfo with legacy format (mission in TaskId) but no explicit MissionId
	contextInfo := &pb.ContextInfo{
		TaskId:    "mission-abc:task-xyz", // Legacy format
		AgentName: "test-agent",
		MissionId: "", // Empty - legacy parsing no longer supported
	}

	// Test getHarness
	harness, err := service.getHarness(context.Background(), contextInfo)

	// Verify error is returned (legacy format no longer supported)
	require.Error(t, err)
	assert.Nil(t, harness)
	assert.Contains(t, err.Error(), "missing mission_id")
}

// TestGetHarness_MissionIdNotFound tests error handling when mission not found.
func TestGetHarness_MissionIdNotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create empty callback harness registry
	registry := NewCallbackHarnessRegistry()

	// Create service with registry
	service := NewHarnessCallbackServiceWithRegistry(logger, registry)

	// Create ContextInfo with non-existent mission
	contextInfo := &pb.ContextInfo{
		TaskId:    "task-999",
		AgentName: "test-agent",
		MissionId: "nonexistent-mission",
	}

	// Test getHarness
	harness, err := service.getHarness(context.Background(), contextInfo)

	// Verify error is returned
	require.Error(t, err)
	assert.Nil(t, harness)
	assert.Contains(t, err.Error(), "no active harness for mission")
}
