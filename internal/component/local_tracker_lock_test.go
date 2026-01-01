package component

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAcquireLock verifies that lock acquisition creates the lock file
// and successfully acquires an exclusive flock.
func TestAcquireLock(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir := t.TempDir()

	// Override the home directory for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create the run directory
	runDir := filepath.Join(tmpDir, runDirBase, runDirAgent)
	require.NoError(t, os.MkdirAll(runDir, 0755))

	tracker := NewDefaultLocalTracker()

	// Acquire lock for test agent
	err := tracker.acquireLock(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Verify lock file was created
	lockPath, err := GetLockFilePath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	_, err = os.Stat(lockPath)
	assert.NoError(t, err, "lock file should exist")

	// Verify lock is held (tracked internally)
	tracker.mu.Lock()
	_, exists := tracker.lockFiles[lockPath]
	tracker.mu.Unlock()
	assert.True(t, exists, "lock file handle should be tracked")

	// Clean up
	require.NoError(t, tracker.releaseLock(ComponentKindAgent, "test-agent"))
}

// TestAcquireLockTwice verifies that acquiring the same lock twice fails.
func TestAcquireLockTwice(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	runDir := filepath.Join(tmpDir, runDirBase, runDirAgent)
	require.NoError(t, os.MkdirAll(runDir, 0755))

	tracker := NewDefaultLocalTracker()

	// Acquire lock first time
	err := tracker.acquireLock(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Try to acquire the same lock again (should fail)
	tracker2 := NewDefaultLocalTracker()
	err = tracker2.acquireLock(ComponentKindAgent, "test-agent")

	// Should get ComponentAlreadyRunningError
	assert.Error(t, err)
	var runningErr *ComponentAlreadyRunningError
	assert.ErrorAs(t, err, &runningErr)
	assert.Equal(t, ComponentKindAgent, runningErr.Kind)
	assert.Equal(t, "test-agent", runningErr.Name)

	// Clean up
	require.NoError(t, tracker.releaseLock(ComponentKindAgent, "test-agent"))
}

// TestReleaseLock verifies that releasing a lock closes the file descriptor
// and removes it from the internal map.
func TestReleaseLock(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	runDir := filepath.Join(tmpDir, runDirBase, runDirAgent)
	require.NoError(t, os.MkdirAll(runDir, 0755))

	tracker := NewDefaultLocalTracker()

	// Acquire lock
	err := tracker.acquireLock(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	lockPath, err := GetLockFilePath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Release lock
	err = tracker.releaseLock(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Verify lock file handle is no longer tracked
	tracker.mu.Lock()
	_, exists := tracker.lockFiles[lockPath]
	tracker.mu.Unlock()
	assert.False(t, exists, "lock file handle should be removed from map")

	// Verify lock can be acquired again after release
	err = tracker.acquireLock(ComponentKindAgent, "test-agent")
	assert.NoError(t, err, "should be able to acquire lock after release")

	// Clean up
	require.NoError(t, tracker.releaseLock(ComponentKindAgent, "test-agent"))
}

// TestReleaseLockNeverAcquired verifies that releasing a lock that was
// never acquired is a safe no-op.
func TestReleaseLockNeverAcquired(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	runDir := filepath.Join(tmpDir, runDirBase, runDirAgent)
	require.NoError(t, os.MkdirAll(runDir, 0755))

	tracker := NewDefaultLocalTracker()

	// Release lock that was never acquired (should be safe)
	err := tracker.releaseLock(ComponentKindAgent, "test-agent")
	assert.NoError(t, err, "releasing non-existent lock should be safe")
}

// TestIsLocked verifies the non-blocking lock check functionality.
func TestIsLocked(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	runDir := filepath.Join(tmpDir, runDirBase, runDirAgent)
	require.NoError(t, os.MkdirAll(runDir, 0755))

	tracker := NewDefaultLocalTracker()

	// Check lock before it's acquired (should be false)
	locked, err := tracker.isLocked(ComponentKindAgent, "test-agent")
	require.NoError(t, err)
	assert.False(t, locked, "lock should not be held initially")

	// Acquire lock
	err = tracker.acquireLock(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Check lock after acquisition (should be true)
	tracker2 := NewDefaultLocalTracker()
	locked, err = tracker2.isLocked(ComponentKindAgent, "test-agent")
	require.NoError(t, err)
	assert.True(t, locked, "lock should be held after acquisition")

	// Release lock
	err = tracker.releaseLock(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Check lock after release (should be false)
	locked, err = tracker.isLocked(ComponentKindAgent, "test-agent")
	require.NoError(t, err)
	assert.False(t, locked, "lock should not be held after release")
}

// TestLockFilePermissions verifies that lock files are created with 0600 permissions.
func TestLockFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	runDir := filepath.Join(tmpDir, runDirBase, runDirAgent)
	require.NoError(t, os.MkdirAll(runDir, 0755))

	tracker := NewDefaultLocalTracker()

	// Acquire lock
	err := tracker.acquireLock(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Check file permissions
	lockPath, err := GetLockFilePath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	info, err := os.Stat(lockPath)
	require.NoError(t, err)

	// Verify permissions are 0600 (owner read/write only)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "lock file should have 0600 permissions")

	// Clean up
	require.NoError(t, tracker.releaseLock(ComponentKindAgent, "test-agent"))
}

// TestMultipleComponentsLocks verifies that multiple components can have
// independent locks.
func TestMultipleComponentsLocks(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create run directories for different component kinds
	for _, kind := range []ComponentKind{ComponentKindAgent, ComponentKindTool, ComponentKindPlugin} {
		runDir, err := GetRunDir(kind)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(runDir, 0755))
	}

	tracker := NewDefaultLocalTracker()

	// Acquire locks for multiple components
	err := tracker.acquireLock(ComponentKindAgent, "agent1")
	require.NoError(t, err)

	err = tracker.acquireLock(ComponentKindAgent, "agent2")
	require.NoError(t, err)

	err = tracker.acquireLock(ComponentKindTool, "tool1")
	require.NoError(t, err)

	err = tracker.acquireLock(ComponentKindPlugin, "plugin1")
	require.NoError(t, err)

	// Verify all locks are tracked
	tracker.mu.Lock()
	assert.Equal(t, 4, len(tracker.lockFiles), "should track 4 lock files")
	tracker.mu.Unlock()

	// Release all locks
	require.NoError(t, tracker.releaseLock(ComponentKindAgent, "agent1"))
	require.NoError(t, tracker.releaseLock(ComponentKindAgent, "agent2"))
	require.NoError(t, tracker.releaseLock(ComponentKindTool, "tool1"))
	require.NoError(t, tracker.releaseLock(ComponentKindPlugin, "plugin1"))

	// Verify all locks are released
	tracker.mu.Lock()
	assert.Equal(t, 0, len(tracker.lockFiles), "all locks should be released")
	tracker.mu.Unlock()
}
