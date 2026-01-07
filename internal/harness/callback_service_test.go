package harness

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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
