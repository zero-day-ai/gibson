package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/daemon"
)

// TestRequireDaemon_Unavailable tests RequireDaemon when daemon is not running.
func TestRequireDaemon_Unavailable(t *testing.T) {
	// Set GIBSON_HOME to a temporary directory without daemon.json
	tempDir := t.TempDir()
	os.Setenv("GIBSON_HOME", tempDir)
	defer os.Unsetenv("GIBSON_HOME")

	ctx := context.Background()
	client, err := RequireDaemon(ctx)

	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "daemon required for this operation")
	assert.Contains(t, err.Error(), "not running")
	assert.Contains(t, err.Error(), "gibson daemon start")
}

// TestRequireDaemon_Available would require an actual running daemon server.
// This test demonstrates the expected behavior when daemon is available.
func TestRequireDaemon_Available(t *testing.T) {
	t.Skip("Requires running daemon server - tested in integration tests")

	// When daemon is running, the test would look like:
	// ctx := context.Background()
	// client, err := RequireDaemon(ctx)
	// assert.NoError(t, err)
	// assert.NotNil(t, client)
	// defer client.Close()
	//
	// // Verify client is functional
	// err = client.Ping(ctx)
	// assert.NoError(t, err)
}

// TestRequireDaemon_DaemonInfoExistsButNotRunning tests when daemon info file
// exists but daemon process is not responding.
func TestRequireDaemon_DaemonInfoExistsButNotRunning(t *testing.T) {
	// Create temporary directory with daemon.json
	tempDir := t.TempDir()
	os.Setenv("GIBSON_HOME", tempDir)
	defer os.Unsetenv("GIBSON_HOME")

	// Write daemon info for a non-existent server
	infoPath := filepath.Join(tempDir, "daemon.json")
	info := &daemon.DaemonInfo{
		PID:         99997,
		StartTime:   time.Now(),
		GRPCAddress: "localhost:59997",
		Version:     "1.0.0",
	}
	err := daemon.WriteDaemonInfo(infoPath, info)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client, err := RequireDaemon(ctx)

	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "daemon required for this operation")
	assert.Contains(t, err.Error(), "failed to connect")
}

// TestRequireDaemon_ContextCanceled tests RequireDaemon with a canceled context.
func TestRequireDaemon_ContextCanceled(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("GIBSON_HOME", tempDir)
	defer os.Unsetenv("GIBSON_HOME")

	// Write daemon info
	infoPath := filepath.Join(tempDir, "daemon.json")
	info := &daemon.DaemonInfo{
		PID:         99996,
		StartTime:   time.Now(),
		GRPCAddress: "localhost:59996",
		Version:     "1.0.0",
	}
	err := daemon.WriteDaemonInfo(infoPath, info)
	require.NoError(t, err)

	// Create already-canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client, err := RequireDaemon(ctx)

	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "daemon required for this operation")
}

// TestOptionalDaemon_Unavailable tests OptionalDaemon when daemon is not running.
func TestOptionalDaemon_Unavailable(t *testing.T) {
	// Set GIBSON_HOME to a temporary directory without daemon.json
	tempDir := t.TempDir()
	os.Setenv("GIBSON_HOME", tempDir)
	defer os.Unsetenv("GIBSON_HOME")

	ctx := context.Background()
	client := OptionalDaemon(ctx)

	// OptionalDaemon returns nil (not error) when daemon unavailable
	assert.Nil(t, client)
}

// TestOptionalDaemon_Available would require an actual running daemon server.
func TestOptionalDaemon_Available(t *testing.T) {
	t.Skip("Requires running daemon server - tested in integration tests")

	// When daemon is running, the test would look like:
	// ctx := context.Background()
	// client := OptionalDaemon(ctx)
	// assert.NotNil(t, client)
	// defer client.Close()
	//
	// // Verify client is functional
	// err := client.Ping(ctx)
	// assert.NoError(t, err)
}

// TestOptionalDaemon_DaemonInfoExistsButNotRunning tests when daemon info exists
// but daemon process is not responding.
func TestOptionalDaemon_DaemonInfoExistsButNotRunning(t *testing.T) {
	// Create temporary directory with daemon.json
	tempDir := t.TempDir()
	os.Setenv("GIBSON_HOME", tempDir)
	defer os.Unsetenv("GIBSON_HOME")

	// Write daemon info for a non-existent server
	infoPath := filepath.Join(tempDir, "daemon.json")
	info := &daemon.DaemonInfo{
		PID:         99995,
		StartTime:   time.Now(),
		GRPCAddress: "localhost:59995",
		Version:     "1.0.0",
	}
	err := daemon.WriteDaemonInfo(infoPath, info)
	require.NoError(t, err)

	ctx := context.Background()
	client := OptionalDaemon(ctx)

	// Should return nil when connection fails (not error)
	assert.Nil(t, client)
}

// TestOptionalDaemon_ShortTimeout tests that OptionalDaemon uses a short timeout.
func TestOptionalDaemon_ShortTimeout(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("GIBSON_HOME", tempDir)
	defer os.Unsetenv("GIBSON_HOME")

	// Write daemon info for non-existent server
	infoPath := filepath.Join(tempDir, "daemon.json")
	info := &daemon.DaemonInfo{
		PID:         99994,
		StartTime:   time.Now(),
		GRPCAddress: "localhost:59994",
		Version:     "1.0.0",
	}
	err := daemon.WriteDaemonInfo(infoPath, info)
	require.NoError(t, err)

	ctx := context.Background()
	start := time.Now()
	client := OptionalDaemon(ctx)
	elapsed := time.Since(start)

	// Should return quickly (within ~3 seconds including timeout buffer)
	assert.Nil(t, client)
	assert.Less(t, elapsed, 4*time.Second, "OptionalDaemon should timeout quickly")
}

// TestOptionalDaemon_NilContext tests OptionalDaemon behavior with nil-like context.
func TestOptionalDaemon_BackgroundContext(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("GIBSON_HOME", tempDir)
	defer os.Unsetenv("GIBSON_HOME")

	// No daemon.json exists
	ctx := context.Background()
	client := OptionalDaemon(ctx)

	// Should handle gracefully and return nil
	assert.Nil(t, client)
}

// TestIsDaemonRunning_False tests IsDaemonRunning when daemon is not running.
func TestIsDaemonRunning_False(t *testing.T) {
	// Set GIBSON_HOME to a temporary directory without daemon.json
	tempDir := t.TempDir()
	os.Setenv("GIBSON_HOME", tempDir)
	defer os.Unsetenv("GIBSON_HOME")

	running := IsDaemonRunning()

	assert.False(t, running, "IsDaemonRunning should return false when daemon.json doesn't exist")
}

// TestIsDaemonRunning_True would require an actual running daemon server.
func TestIsDaemonRunning_True(t *testing.T) {
	t.Skip("Requires running daemon server - tested in integration tests")

	// When daemon is running, the test would look like:
	// running := IsDaemonRunning()
	// assert.True(t, running, "IsDaemonRunning should return true when daemon is running")
}

// TestIsDaemonRunning_DaemonInfoExistsButNotResponding tests when daemon.json
// exists but daemon is not responding to pings.
func TestIsDaemonRunning_DaemonInfoExistsButNotResponding(t *testing.T) {
	// Create temporary directory with daemon.json
	tempDir := t.TempDir()
	os.Setenv("GIBSON_HOME", tempDir)
	defer os.Unsetenv("GIBSON_HOME")

	// Write daemon info for a non-existent server
	infoPath := filepath.Join(tempDir, "daemon.json")
	info := &daemon.DaemonInfo{
		PID:         99993,
		StartTime:   time.Now(),
		GRPCAddress: "localhost:59993",
		Version:     "1.0.0",
	}
	err := daemon.WriteDaemonInfo(infoPath, info)
	require.NoError(t, err)

	running := IsDaemonRunning()

	// Should return false when daemon doesn't respond to ping
	assert.False(t, running, "IsDaemonRunning should return false when daemon doesn't respond")
}

// TestIsDaemonRunning_InvalidDaemonInfo tests when daemon.json is malformed.
func TestIsDaemonRunning_InvalidDaemonInfo(t *testing.T) {
	// Create temporary directory with invalid daemon.json
	tempDir := t.TempDir()
	os.Setenv("GIBSON_HOME", tempDir)
	defer os.Unsetenv("GIBSON_HOME")

	// Write invalid JSON
	infoPath := filepath.Join(tempDir, "daemon.json")
	err := os.WriteFile(infoPath, []byte("invalid json{"), 0644)
	require.NoError(t, err)

	running := IsDaemonRunning()

	// Should return false when daemon.json is invalid
	assert.False(t, running, "IsDaemonRunning should return false when daemon.json is invalid")
}

// TestIsDaemonRunning_FastExecution tests that IsDaemonRunning returns quickly.
func TestIsDaemonRunning_FastExecution(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("GIBSON_HOME", tempDir)
	defer os.Unsetenv("GIBSON_HOME")

	// Write daemon info for non-existent server
	infoPath := filepath.Join(tempDir, "daemon.json")
	info := &daemon.DaemonInfo{
		PID:         99992,
		StartTime:   time.Now(),
		GRPCAddress: "localhost:59992",
		Version:     "1.0.0",
	}
	err := daemon.WriteDaemonInfo(infoPath, info)
	require.NoError(t, err)

	start := time.Now()
	running := IsDaemonRunning()
	elapsed := time.Since(start)

	// Should return quickly (within ~2 seconds including timeout buffer)
	assert.False(t, running)
	assert.Less(t, elapsed, 3*time.Second, "IsDaemonRunning should complete quickly")
}

// TestIsDaemonRunning_EmptyGibsonHome tests behavior when GIBSON_HOME is not set.
func TestIsDaemonRunning_EmptyGibsonHome(t *testing.T) {
	// Set GIBSON_HOME to empty temp dir to avoid using actual daemon.json
	tempDir := t.TempDir()

	// Temporarily set GIBSON_HOME to temp dir (simulates using default path)
	originalHome := os.Getenv("GIBSON_HOME")
	os.Setenv("GIBSON_HOME", tempDir)
	defer func() {
		if originalHome != "" {
			os.Setenv("GIBSON_HOME", originalHome)
		} else {
			os.Unsetenv("GIBSON_HOME")
		}
	}()

	// IsDaemonRunning should use the configured home directory
	running := IsDaemonRunning()

	// Should return false (daemon.json doesn't exist in temp dir)
	assert.False(t, running, "IsDaemonRunning should handle missing daemon.json gracefully")
}

// TestIsDaemonRunning_PermissionDenied tests when daemon.json exists but is not readable.
func TestIsDaemonRunning_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Test cannot run as root (permissions always granted)")
	}

	tempDir := t.TempDir()
	os.Setenv("GIBSON_HOME", tempDir)
	defer os.Unsetenv("GIBSON_HOME")

	// Create daemon.json with no read permissions
	infoPath := filepath.Join(tempDir, "daemon.json")
	info := &daemon.DaemonInfo{
		PID:         99991,
		StartTime:   time.Now(),
		GRPCAddress: "localhost:59991",
		Version:     "1.0.0",
	}
	err := daemon.WriteDaemonInfo(infoPath, info)
	require.NoError(t, err)

	// Remove read permissions
	err = os.Chmod(infoPath, 0000)
	require.NoError(t, err)
	defer os.Chmod(infoPath, 0644) // Restore for cleanup

	running := IsDaemonRunning()

	// Should return false when file is not readable
	assert.False(t, running, "IsDaemonRunning should return false when daemon.json is not readable")
}
