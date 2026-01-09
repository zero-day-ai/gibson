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

// TestGetHarness_EmptyMissionId_FallsBackToTaskId tests backward compatibility.
func TestGetHarness_EmptyMissionId_FallsBackToTaskId(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create a mock harness
	mockHarn := newMockHarness()

	// Create service without registry
	service := NewHarnessCallbackService(logger)

	// Register harness by task ID (legacy mode)
	taskID := "task-789"
	service.RegisterHarness(taskID, mockHarn)

	// Create ContextInfo with empty MissionId
	contextInfo := &pb.ContextInfo{
		TaskId:    taskID,
		AgentName: "test-agent",
		MissionId: "", // Empty - should fall back to task-based lookup
	}

	// Test getHarness
	harness, err := service.getHarness(context.Background(), contextInfo)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, mockHarn, harness)
}

// TestGetHarness_LegacyTaskIdWithColon tests legacy missionID:taskID format.
func TestGetHarness_LegacyTaskIdWithColon(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create a mock harness
	mockHarn := newMockHarness()

	// Create callback harness registry and register the harness
	registry := NewCallbackHarnessRegistry()
	registry.Register("mission-abc", "test-agent", mockHarn)

	// Create service with registry
	service := NewHarnessCallbackServiceWithRegistry(logger, registry)

	// Create ContextInfo with legacy format (mission in TaskId)
	contextInfo := &pb.ContextInfo{
		TaskId:    "mission-abc:task-xyz", // Legacy format
		AgentName: "test-agent",
		MissionId: "", // Empty - should parse from TaskId
	}

	// Test getHarness
	harness, err := service.getHarness(context.Background(), contextInfo)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, mockHarn, harness)
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
	assert.Contains(t, err.Error(), "no active harness for task")
}
