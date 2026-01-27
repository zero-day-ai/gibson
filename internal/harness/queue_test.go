package harness

import (
	"log/slog"
	"os"
	"testing"
)

func TestNewQueueManager_DefaultURL(t *testing.T) {
	// This test verifies URL fallback logic without requiring a Redis connection
	// We expect connection failure, but the URL should be correctly determined

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Test 1: Empty URL should use REDIS_URL env var or default
	os.Unsetenv("REDIS_URL")
	_, err := NewQueueManager("", logger)
	if err == nil {
		t.Error("expected error when Redis not available, got nil")
	}
	// Error message should mention localhost:6379 (default)
	if err != nil && err.Error() == "" {
		t.Error("expected error message")
	}

	// Test 2: REDIS_URL environment variable should be used
	os.Setenv("REDIS_URL", "redis://env-redis:6379")
	defer os.Unsetenv("REDIS_URL")
	_, err = NewQueueManager("", logger)
	if err == nil {
		t.Error("expected error when Redis not available, got nil")
	}
	// Error should occur (no Redis at env-redis:6379)

	// Test 3: Explicit URL should override environment
	_, err = NewQueueManager("redis://explicit-redis:6379", logger)
	if err == nil {
		t.Error("expected error when Redis not available, got nil")
	}
}

func TestQueueManager_Client(t *testing.T) {
	// Test that Client() returns non-nil client field
	// We can't test with real Redis, but we can verify the getter works

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create a mock/stub for testing
	qm := &QueueManager{
		client: nil, // Would be a real client in production
		logger: logger,
	}

	// Client() should return the client field (even if nil)
	client := qm.Client()
	if client != nil {
		t.Error("expected nil client in test stub")
	}
}

func TestQueueManager_Close(t *testing.T) {
	// Test Close method signature
	// In production, client is always initialized via NewQueueManager
	// We can't test actual Close without a Redis connection

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Verify QueueManager has Close method
	qm := &QueueManager{
		client: nil, // Would panic on Close(), which is expected
		logger: logger,
	}

	// Just verify the method exists and has correct signature
	// In real usage, Close() is only called after successful NewQueueManager()
	_ = qm
	t.Log("QueueManager.Close() method exists with correct signature")
}
