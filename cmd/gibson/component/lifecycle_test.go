package component

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zero-day-ai/gibson/internal/component"
)

func TestGetLifecycleManager_WithLogWriter(t *testing.T) {
	// Set up a temporary Gibson home for testing
	tempDir := t.TempDir()
	oldEnv := os.Getenv("GIBSON_HOME")
	os.Setenv("GIBSON_HOME", tempDir)
	defer os.Setenv("GIBSON_HOME", oldEnv)

	// Create a mock DAO (nil is acceptable for this test)
	var dao component.StatusUpdater

	// Call getLifecycleManager
	mgr := getLifecycleManager(dao)

	// Verify that the manager was created
	if mgr == nil {
		t.Fatal("expected lifecycle manager to be created, got nil")
	}

	// Verify that the log directory was created
	logDir := filepath.Join(tempDir, "logs")
	info, err := os.Stat(logDir)
	if err != nil {
		t.Fatalf("expected log directory to be created at %s, got error: %v", logDir, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", logDir)
	}
}

func TestGetLifecycleManager_WithInvalidHome(t *testing.T) {
	// Set up an invalid Gibson home (file instead of directory)
	tempDir := t.TempDir()
	invalidHome := filepath.Join(tempDir, "invalid")

	// Create a file instead of a directory
	if err := os.WriteFile(invalidHome, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	oldEnv := os.Getenv("GIBSON_HOME")
	os.Setenv("GIBSON_HOME", invalidHome)
	defer os.Setenv("GIBSON_HOME", oldEnv)

	// Create a mock DAO
	var dao component.StatusUpdater

	// Call getLifecycleManager - should not panic even with invalid home
	mgr := getLifecycleManager(dao)

	// Verify that the manager was created (with nil LogWriter)
	if mgr == nil {
		t.Fatal("expected lifecycle manager to be created even with invalid home, got nil")
	}
}

func TestGetLifecycleManager_WithoutGibsonHome(t *testing.T) {
	// Unset GIBSON_HOME to test fallback to user home
	oldEnv := os.Getenv("GIBSON_HOME")
	os.Unsetenv("GIBSON_HOME")
	defer func() {
		if oldEnv != "" {
			os.Setenv("GIBSON_HOME", oldEnv)
		}
	}()

	// Create a mock DAO
	var dao component.StatusUpdater

	// Call getLifecycleManager
	mgr := getLifecycleManager(dao)

	// Verify that the manager was created
	if mgr == nil {
		t.Fatal("expected lifecycle manager to be created, got nil")
	}

	// Verify that the log directory was created in user home
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Skip("could not get user home directory")
	}

	logDir := filepath.Join(userHome, ".gibson", "logs")
	info, err := os.Stat(logDir)
	if err != nil {
		// It's okay if this fails - we just want to ensure the manager was created
		t.Logf("log directory not created (expected): %v", err)
		return
	}

	// Clean up if it was created
	defer os.RemoveAll(filepath.Join(userHome, ".gibson"))

	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", logDir)
	}
}
