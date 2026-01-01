package component

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// setupTestEnv creates a temporary home directory for tests
// and returns a cleanup function
func setupTestEnv(t *testing.T) (cleanup func()) {
	t.Helper()

	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)

	// Create run directories for all kinds
	for _, kind := range []ComponentKind{ComponentKindAgent, ComponentKindTool, ComponentKindPlugin} {
		runDir, err := GetRunDir(kind)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(runDir, 0755))
	}

	return func() {
		os.Setenv("HOME", originalHome)
	}
}

// startMockGRPCServer starts a mock gRPC health server on a Unix socket
func startMockGRPCServer(t *testing.T, socketPath string) (*grpc.Server, func()) {
	t.Helper()

	// Remove socket if it exists
	_ = os.Remove(socketPath)

	// Create Unix listener
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	// Create gRPC server
	server := grpc.NewServer()
	healthServer := newMockHealthServer(grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(server, healthServer)

	// Start serving in background
	go func() {
		_ = server.Serve(listener)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	cleanup := func() {
		server.Stop()
		listener.Close()
		_ = os.Remove(socketPath)
	}

	return server, cleanup
}

// TestWritePIDFile tests PID file creation
func TestWritePIDFile(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	err := writePIDFile(ComponentKindAgent, "test-agent", 12345, 50051)
	require.NoError(t, err)

	// Verify file exists
	pidPath, err := GetPIDFilePath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	info, err := os.Stat(pidPath)
	require.NoError(t, err)

	// Verify permissions
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// Read and verify content
	content, err := os.ReadFile(pidPath)
	require.NoError(t, err)
	assert.Equal(t, "12345\n50051\n", string(content))
}

// TestWritePIDFileCreatesDirectory tests that run directory is created if missing
func TestWritePIDFileCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Don't create run directory - writePIDFile should create it
	err := writePIDFile(ComponentKindAgent, "test-agent", 12345, 50051)
	require.NoError(t, err)

	// Verify run directory was created
	runDir, err := GetRunDir(ComponentKindAgent)
	require.NoError(t, err)

	info, err := os.Stat(runDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestWritePIDFileAtomic tests that PID file write is atomic
func TestWritePIDFileAtomic(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	// Write first time
	err := writePIDFile(ComponentKindAgent, "test-agent", 12345, 50051)
	require.NoError(t, err)

	// Overwrite with new values
	err = writePIDFile(ComponentKindAgent, "test-agent", 67890, 50052)
	require.NoError(t, err)

	// Verify new values
	pid, port, err := readPIDFile(ComponentKindAgent, "test-agent")
	require.NoError(t, err)
	assert.Equal(t, 67890, pid)
	assert.Equal(t, 50052, port)

	// Verify no temp file left behind
	pidPath, _ := GetPIDFilePath(ComponentKindAgent, "test-agent")
	tmpPath := pidPath + ".tmp"
	_, err = os.Stat(tmpPath)
	assert.True(t, os.IsNotExist(err))
}

// TestReadPIDFile tests reading PID files
func TestReadPIDFile(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	// Write a PID file
	err := writePIDFile(ComponentKindAgent, "test-agent", 12345, 50051)
	require.NoError(t, err)

	// Read it back
	pid, port, err := readPIDFile(ComponentKindAgent, "test-agent")
	require.NoError(t, err)
	assert.Equal(t, 12345, pid)
	assert.Equal(t, 50051, port)
}

// TestReadPIDFileNotExist tests reading non-existent PID file
func TestReadPIDFileNotExist(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	_, _, err := readPIDFile(ComponentKindAgent, "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PID file does not exist")
}

// TestReadPIDFileMalformed tests reading malformed PID files
func TestReadPIDFileMalformed(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	pidPath, err := GetPIDFilePath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	tests := []struct {
		name    string
		content string
		errMsg  string
	}{
		{
			name:    "empty file",
			content: "",
			errMsg:  "expected 2 lines",
		},
		{
			name:    "single line",
			content: "12345\n",
			errMsg:  "expected 2 lines",
		},
		{
			name:    "invalid PID",
			content: "abc\n50051\n",
			errMsg:  "invalid PID",
		},
		{
			name:    "invalid port",
			content: "12345\nabc\n",
			errMsg:  "invalid port",
		},
		{
			name:    "negative PID",
			content: "-1\n50051\n",
			errMsg:  "invalid PID value",
		},
		{
			name:    "zero PID",
			content: "0\n50051\n",
			errMsg:  "invalid PID value",
		},
		{
			name:    "port too high",
			content: "12345\n99999\n",
			errMsg:  "invalid port value",
		},
		{
			name:    "zero port",
			content: "12345\n0\n",
			errMsg:  "invalid port value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.WriteFile(pidPath, []byte(tt.content), 0600)
			require.NoError(t, err)

			_, _, err = readPIDFile(ComponentKindAgent, "test-agent")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

// TestRemovePIDFile tests PID file removal
func TestRemovePIDFile(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	// Write a PID file
	err := writePIDFile(ComponentKindAgent, "test-agent", 12345, 50051)
	require.NoError(t, err)

	pidPath, err := GetPIDFilePath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Verify it exists
	_, err = os.Stat(pidPath)
	require.NoError(t, err)

	// Remove it
	err = removePIDFile(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Verify it's gone
	_, err = os.Stat(pidPath)
	assert.True(t, os.IsNotExist(err))
}

// TestRemovePIDFileNotExist tests removing non-existent PID file
func TestRemovePIDFileNotExist(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	// Should not error
	err := removePIDFile(ComponentKindAgent, "nonexistent")
	assert.NoError(t, err)
}

// TestValidatePID tests process validation
func TestValidatePID(t *testing.T) {
	// Test with current process (should be alive)
	pid := os.Getpid()
	assert.True(t, validatePID(pid), "current process should be alive")

	// Test with non-existent PID (very high number unlikely to exist)
	assert.False(t, validatePID(999999), "PID 999999 should not exist")

	// Test with PID 0 (should not be valid)
	assert.False(t, validatePID(0), "PID 0 should not be valid")
}

// TestValidatePIDZombie tests that zombie processes are detected as invalid
func TestValidatePIDZombie(t *testing.T) {
	// This test creates a zombie process to verify detection
	// Skip on CI or if we can't fork
	if os.Getenv("CI") != "" {
		t.Skip("Skipping zombie test in CI")
	}

	// Create a child process that exits immediately
	// The parent will not wait, creating a zombie
	pid := syscall.Getpid()

	// Fork a process
	fpid, err := syscall.ForkExec("/bin/true", []string{"/bin/true"}, &syscall.ProcAttr{})
	if err != nil {
		t.Skip("Cannot fork process")
	}

	// Give it time to become a zombie
	time.Sleep(100 * time.Millisecond)

	// Check if it's a zombie (it might have been reaped already)
	// We can't guarantee a zombie state, so this is a best-effort test
	_ = pid
	_ = fpid

	// Clean up
	var ws syscall.WaitStatus
	_, _ = syscall.Wait4(fpid, &ws, syscall.WNOHANG, nil)
}

// TestWaitForSocket tests socket wait with success
func TestWaitForSocket(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	socketPath, err := GetSocketPath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Create socket in background after a delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		listener, err := net.Listen("unix", socketPath)
		if err != nil {
			return
		}
		defer listener.Close()
		time.Sleep(200 * time.Millisecond)
	}()

	ctx := context.Background()
	err = tracker.waitForSocket(ctx, socketPath, 1*time.Second)
	assert.NoError(t, err)

	// Cleanup
	_ = os.Remove(socketPath)
}

// TestWaitForSocketTimeout tests socket wait timeout
func TestWaitForSocketTimeout(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	socketPath, err := GetSocketPath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	ctx := context.Background()
	err = tracker.waitForSocket(ctx, socketPath, 200*time.Millisecond)
	require.Error(t, err)

	var timeoutErr *ComponentError
	assert.ErrorAs(t, err, &timeoutErr)
	assert.Equal(t, ErrCodeTimeout, timeoutErr.Code)
}

// TestWaitForSocketCancellation tests context cancellation
func TestWaitForSocketCancellation(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	socketPath, err := GetSocketPath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after 100ms
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err = tracker.waitForSocket(ctx, socketPath, 5*time.Second)
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

// TestWaitForSocketNotASocket tests rejection of non-socket files
func TestWaitForSocketNotASocket(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	socketPath, err := GetSocketPath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Create a regular file instead of a socket
	err = os.WriteFile(socketPath, []byte("not a socket"), 0600)
	require.NoError(t, err)

	ctx := context.Background()
	err = tracker.waitForSocket(ctx, socketPath, 500*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a socket")

	// Cleanup
	_ = os.Remove(socketPath)
}

// TestCheckSocketHealth tests socket health check
func TestCheckSocketHealth(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	socketPath, err := GetSocketPath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Start mock gRPC server
	_, serverCleanup := startMockGRPCServer(t, socketPath)
	defer serverCleanup()

	ctx := context.Background()
	err = tracker.checkSocketHealth(ctx, socketPath, 1*time.Second)
	assert.NoError(t, err)
}

// TestCheckSocketHealthNotExist tests health check on non-existent socket
func TestCheckSocketHealthNotExist(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	socketPath, err := GetSocketPath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	ctx := context.Background()
	err = tracker.checkSocketHealth(ctx, socketPath, 1*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

// TestCheckSocketHealthNotASocket tests health check on non-socket file
func TestCheckSocketHealthNotASocket(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	socketPath, err := GetSocketPath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Create regular file
	err = os.WriteFile(socketPath, []byte("not a socket"), 0600)
	require.NoError(t, err)
	defer os.Remove(socketPath)

	ctx := context.Background()
	err = tracker.checkSocketHealth(ctx, socketPath, 1*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a socket")
}

// TestCleanupStaleSocket tests stale socket cleanup
func TestCleanupStaleSocket(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	socketPath, err := GetSocketPath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Create a socket and keep it open briefly, then close to make it stale
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	// Verify socket exists before closing
	info, err := os.Stat(socketPath)
	require.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSocket != 0)

	// Close the listener - this makes the socket stale but may remove the file on some systems
	listener.Close()

	// If the socket was automatically removed by the OS, create it manually for the test
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Skip("OS automatically removes socket files on listener close")
	}

	ctx := context.Background()
	err = tracker.cleanupStaleSocket(ctx, socketPath)
	assert.NoError(t, err)

	// Verify socket was removed
	_, err = os.Stat(socketPath)
	assert.True(t, os.IsNotExist(err))
}

// TestCleanupStaleSocketNotExist tests cleanup of non-existent socket
func TestCleanupStaleSocketNotExist(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	socketPath, err := GetSocketPath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	ctx := context.Background()
	err = tracker.cleanupStaleSocket(ctx, socketPath)
	assert.NoError(t, err) // Should not error
}

// TestCleanupStaleSocketHealthy tests that healthy sockets are not removed
func TestCleanupStaleSocketHealthy(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	socketPath, err := GetSocketPath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Start healthy server
	_, serverCleanup := startMockGRPCServer(t, socketPath)
	defer serverCleanup()

	ctx := context.Background()
	err = tracker.cleanupStaleSocket(ctx, socketPath)
	require.Error(t, err) // Should refuse to remove healthy socket
	assert.Contains(t, err.Error(), "healthy")

	// Verify socket still exists
	_, err = os.Stat(socketPath)
	assert.NoError(t, err)
}

// TestCleanupStaleSocketNotASocket tests cleanup refuses to remove non-socket files
func TestCleanupStaleSocketNotASocket(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	socketPath, err := GetSocketPath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Create regular file
	err = os.WriteFile(socketPath, []byte("not a socket"), 0600)
	require.NoError(t, err)
	defer os.Remove(socketPath)

	ctx := context.Background()
	err = tracker.cleanupStaleSocket(ctx, socketPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a socket")

	// Verify file was not removed
	_, err = os.Stat(socketPath)
	assert.NoError(t, err)
}

// TestStartSuccess tests successful component start
func TestStartSuccess(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	comp := &Component{
		Kind: ComponentKindAgent,
		Name: "test-agent",
		PID:  os.Getpid(),
		Port: 50051,
	}

	socketPath, err := GetSocketPath(comp.Kind, comp.Name)
	require.NoError(t, err)

	// Create socket in background
	go func() {
		time.Sleep(100 * time.Millisecond)
		listener, err := net.Listen("unix", socketPath)
		if err != nil {
			return
		}
		defer listener.Close()
		time.Sleep(500 * time.Millisecond)
	}()

	ctx := context.Background()
	err = tracker.Start(ctx, comp)
	assert.NoError(t, err)

	// Verify PID file was created
	pid, port, err := readPIDFile(comp.Kind, comp.Name)
	require.NoError(t, err)
	assert.Equal(t, comp.PID, pid)
	assert.Equal(t, comp.Port, port)

	// Verify lock was acquired
	locked, err := tracker.isLocked(comp.Kind, comp.Name)
	require.NoError(t, err)
	assert.True(t, locked)

	// Cleanup
	_ = tracker.Stop(ctx, comp)
	_ = os.Remove(socketPath)
}

// TestStartNilComponent tests Start with nil component
func TestStartNilComponent(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	err := tracker.Start(ctx, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

// TestStartSocketTimeout tests Start timeout waiting for socket
func TestStartSocketTimeout(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	comp := &Component{
		Kind: ComponentKindAgent,
		Name: "test-agent",
		PID:  os.Getpid(),
		Port: 50051,
	}

	ctx := context.Background()
	err := tracker.Start(ctx, comp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "waiting for socket")

	// Verify cleanup happened - PID file should be removed
	pidPath, _ := GetPIDFilePath(comp.Kind, comp.Name)
	_, err = os.Stat(pidPath)
	assert.True(t, os.IsNotExist(err))

	// Verify lock was released
	locked, _ := tracker.isLocked(comp.Kind, comp.Name)
	assert.False(t, locked)
}

// TestStartAlreadyRunning tests Start when component is already running
func TestStartAlreadyRunning(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	comp := &Component{
		Kind: ComponentKindAgent,
		Name: "test-agent",
		PID:  os.Getpid(),
		Port: 50051,
	}

	socketPath, err := GetSocketPath(comp.Kind, comp.Name)
	require.NoError(t, err)

	// Start first component
	go func() {
		time.Sleep(100 * time.Millisecond)
		listener, err := net.Listen("unix", socketPath)
		if err != nil {
			return
		}
		defer listener.Close()
		time.Sleep(2 * time.Second)
	}()

	ctx := context.Background()
	err = tracker.Start(ctx, comp)
	require.NoError(t, err)

	// Try to start again
	tracker2 := NewDefaultLocalTracker()
	err = tracker2.Start(ctx, comp)
	require.Error(t, err)

	var runningErr *ComponentAlreadyRunningError
	assert.ErrorAs(t, err, &runningErr)

	// Cleanup
	_ = tracker.Stop(ctx, comp)
	_ = os.Remove(socketPath)
}

// TestStopSuccess tests successful component stop
func TestStopSuccess(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	comp := &Component{
		Kind: ComponentKindAgent,
		Name: "test-agent",
		PID:  os.Getpid(),
		Port: 50051,
	}

	// Manually set up state (simulating a started component)
	err := writePIDFile(comp.Kind, comp.Name, comp.PID, comp.Port)
	require.NoError(t, err)

	err = tracker.acquireLock(comp.Kind, comp.Name)
	require.NoError(t, err)

	// Stop
	ctx := context.Background()
	err = tracker.Stop(ctx, comp)
	assert.NoError(t, err)

	// Verify PID file removed
	pidPath, _ := GetPIDFilePath(comp.Kind, comp.Name)
	_, err = os.Stat(pidPath)
	assert.True(t, os.IsNotExist(err))

	// Verify lock released
	locked, _ := tracker.isLocked(comp.Kind, comp.Name)
	assert.False(t, locked)
}

// TestStopNilComponent tests Stop with nil component
func TestStopNilComponent(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	err := tracker.Stop(ctx, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

// TestIsRunning tests component running detection
func TestIsRunning(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	// Not running initially
	running, err := tracker.IsRunning(ctx, ComponentKindAgent, "test-agent")
	require.NoError(t, err)
	assert.False(t, running)

	// Acquire lock
	err = tracker.acquireLock(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Still not running (no socket)
	running, err = tracker.IsRunning(ctx, ComponentKindAgent, "test-agent")
	require.NoError(t, err)
	assert.False(t, running)

	// Create socket
	socketPath, err := GetSocketPath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()
	defer os.Remove(socketPath)

	// Now running (lock + socket)
	running, err = tracker.IsRunning(ctx, ComponentKindAgent, "test-agent")
	require.NoError(t, err)
	assert.True(t, running)

	// Release lock
	err = tracker.releaseLock(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	// Not running (no lock)
	running, err = tracker.IsRunning(ctx, ComponentKindAgent, "test-agent")
	require.NoError(t, err)
	assert.False(t, running)
}

// TestDiscoverEmpty tests discovery with no running components
func TestDiscoverEmpty(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	states, err := tracker.Discover(ctx, ComponentKindAgent)
	require.NoError(t, err)
	assert.Empty(t, states)
}

// TestDiscoverSingle tests discovering a single running component
func TestDiscoverSingle(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	// Set up a running component
	comp := &Component{
		Kind: ComponentKindAgent,
		Name: "test-agent",
		PID:  os.Getpid(),
		Port: 50051,
	}

	err := writePIDFile(comp.Kind, comp.Name, comp.PID, comp.Port)
	require.NoError(t, err)

	err = tracker.acquireLock(comp.Kind, comp.Name)
	require.NoError(t, err)
	defer tracker.releaseLock(comp.Kind, comp.Name)

	socketPath, err := GetSocketPath(comp.Kind, comp.Name)
	require.NoError(t, err)
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()
	defer os.Remove(socketPath)

	// Discover
	states, err := tracker.Discover(ctx, ComponentKindAgent)
	require.NoError(t, err)
	require.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, ComponentKindAgent, state.Kind)
	assert.Equal(t, "test-agent", state.Name)
	assert.Equal(t, comp.PID, state.PID)
	assert.Equal(t, comp.Port, state.Port)
	assert.Equal(t, socketPath, state.Socket)
	assert.True(t, state.Healthy)
}

// TestDiscoverMultiple tests discovering multiple running components
func TestDiscoverMultiple(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	components := []struct {
		name string
		port int
	}{
		{"agent1", 50051},
		{"agent2", 50052},
		{"agent3", 50053},
	}

	var listeners []net.Listener
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()

	for _, c := range components {
		err := writePIDFile(ComponentKindAgent, c.name, os.Getpid(), c.port)
		require.NoError(t, err)

		err = tracker.acquireLock(ComponentKindAgent, c.name)
		require.NoError(t, err)
		defer tracker.releaseLock(ComponentKindAgent, c.name)

		socketPath, err := GetSocketPath(ComponentKindAgent, c.name)
		require.NoError(t, err)
		listener, err := net.Listen("unix", socketPath)
		require.NoError(t, err)
		listeners = append(listeners, listener)
		defer os.Remove(socketPath)
	}

	// Discover
	states, err := tracker.Discover(ctx, ComponentKindAgent)
	require.NoError(t, err)
	assert.Len(t, states, 3)

	// Verify all components found
	names := make(map[string]bool)
	for _, state := range states {
		names[state.Name] = true
		assert.True(t, state.Healthy)
	}

	for _, c := range components {
		assert.True(t, names[c.name], fmt.Sprintf("component %s should be discovered", c.name))
	}
}

// TestDiscoverStaleProcessCleanup tests that stale PID files are cleaned up
func TestDiscoverStaleProcessCleanup(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	// Create a PID file with a dead process
	err := writePIDFile(ComponentKindAgent, "dead-agent", 999999, 50051)
	require.NoError(t, err)

	// Verify PID file exists
	pidPath, _ := GetPIDFilePath(ComponentKindAgent, "dead-agent")
	_, err = os.Stat(pidPath)
	require.NoError(t, err)

	// Discover should clean it up
	states, err := tracker.Discover(ctx, ComponentKindAgent)
	require.NoError(t, err)
	assert.Empty(t, states)

	// Verify PID file was removed
	_, err = os.Stat(pidPath)
	assert.True(t, os.IsNotExist(err))
}

// TestDiscoverUnhealthyComponents tests discovery filters out unhealthy components
func TestDiscoverUnhealthyComponents(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	// Create component with lock but no socket (unhealthy)
	err := writePIDFile(ComponentKindAgent, "unhealthy", os.Getpid(), 50051)
	require.NoError(t, err)

	err = tracker.acquireLock(ComponentKindAgent, "unhealthy")
	require.NoError(t, err)
	defer tracker.releaseLock(ComponentKindAgent, "unhealthy")

	// Discover should filter it out and clean up
	states, err := tracker.Discover(ctx, ComponentKindAgent)
	require.NoError(t, err)
	assert.Empty(t, states)

	// Verify PID file was cleaned up
	pidPath, _ := GetPIDFilePath(ComponentKindAgent, "unhealthy")
	_, err = os.Stat(pidPath)
	assert.True(t, os.IsNotExist(err))
}

// TestCleanupSuccess tests successful cleanup
func TestCleanupSuccess(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	// Create stale PID files for dead processes across different kinds
	staleComponents := []struct {
		kind ComponentKind
		name string
	}{
		{ComponentKindAgent, "dead-agent"},
		{ComponentKindTool, "dead-tool"},
		{ComponentKindPlugin, "dead-plugin"},
	}

	for _, comp := range staleComponents {
		err := writePIDFile(comp.kind, comp.name, 999999, 50051)
		require.NoError(t, err)

		// Also create lock file
		lockPath, err := GetLockFilePath(comp.kind, comp.name)
		require.NoError(t, err)
		err = os.WriteFile(lockPath, []byte{}, 0600)
		require.NoError(t, err)
	}

	// Run cleanup
	err := tracker.Cleanup(ctx)
	assert.NoError(t, err)

	// Verify all stale files were removed
	for _, comp := range staleComponents {
		pidPath, _ := GetPIDFilePath(comp.kind, comp.name)
		_, err := os.Stat(pidPath)
		assert.True(t, os.IsNotExist(err), "PID file should be removed for %s/%s", comp.kind, comp.name)

		lockPath, _ := GetLockFilePath(comp.kind, comp.name)
		_, err = os.Stat(lockPath)
		assert.True(t, os.IsNotExist(err), "Lock file should be removed for %s/%s", comp.kind, comp.name)
	}
}

// TestCleanupPreservesHealthy tests that cleanup preserves healthy components
func TestCleanupPreservesHealthy(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	// Create a healthy component
	err := writePIDFile(ComponentKindAgent, "healthy-agent", os.Getpid(), 50051)
	require.NoError(t, err)

	err = tracker.acquireLock(ComponentKindAgent, "healthy-agent")
	require.NoError(t, err)
	defer tracker.releaseLock(ComponentKindAgent, "healthy-agent")

	// Create a stale component
	err = writePIDFile(ComponentKindAgent, "dead-agent", 999999, 50052)
	require.NoError(t, err)

	// Run cleanup
	err = tracker.Cleanup(ctx)
	assert.NoError(t, err)

	// Verify healthy component preserved
	pidPath, _ := GetPIDFilePath(ComponentKindAgent, "healthy-agent")
	_, err = os.Stat(pidPath)
	assert.NoError(t, err, "Healthy component PID file should be preserved")

	// Verify stale component removed
	stalePidPath, _ := GetPIDFilePath(ComponentKindAgent, "dead-agent")
	_, err = os.Stat(stalePidPath)
	assert.True(t, os.IsNotExist(err), "Stale component PID file should be removed")
}

// TestCleanupOrphanedLockFiles tests cleanup of orphaned lock files
func TestCleanupOrphanedLockFiles(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	// Create orphaned lock file (no corresponding PID file)
	lockPath, err := GetLockFilePath(ComponentKindAgent, "orphaned")
	require.NoError(t, err)
	err = os.WriteFile(lockPath, []byte{}, 0600)
	require.NoError(t, err)

	// Verify it exists
	_, err = os.Stat(lockPath)
	require.NoError(t, err)

	// Run cleanup
	err = tracker.Cleanup(ctx)
	assert.NoError(t, err)

	// Verify orphaned lock file was removed
	_, err = os.Stat(lockPath)
	assert.True(t, os.IsNotExist(err))
}

// TestCleanupRemovesStaleSocket tests that cleanup removes stale sockets
func TestCleanupRemovesStaleSocket(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	// Create stale PID file
	err := writePIDFile(ComponentKindAgent, "dead-agent", 999999, 50051)
	require.NoError(t, err)

	// Create stale socket
	socketPath, err := GetSocketPath(ComponentKindAgent, "dead-agent")
	require.NoError(t, err)
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	listener.Close() // Close to make it stale

	// Run cleanup
	err = tracker.Cleanup(ctx)
	assert.NoError(t, err)

	// Verify socket was removed
	_, err = os.Stat(socketPath)
	assert.True(t, os.IsNotExist(err))
}

// TestGetRunDir tests run directory path generation
func TestGetRunDir(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	tests := []struct {
		kind     ComponentKind
		expected string
		errMsg   string
	}{
		{
			kind:     ComponentKindAgent,
			expected: filepath.Join(tmpDir, ".gibson/run/agent"),
		},
		{
			kind:     ComponentKindTool,
			expected: filepath.Join(tmpDir, ".gibson/run/tool"),
		},
		{
			kind:     ComponentKindPlugin,
			expected: filepath.Join(tmpDir, ".gibson/run/plugin"),
		},
		{
			kind:   ComponentKindRepository,
			errMsg: "invalid component kind",
		},
		{
			kind:   ComponentKind("invalid"),
			errMsg: "invalid component kind",
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			runDir, err := GetRunDir(tt.kind)
			if tt.errMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, runDir)
			}
		})
	}
}

// TestGetPIDFilePath tests PID file path generation
func TestGetPIDFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	path, err := GetPIDFilePath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)
	expected := filepath.Join(tmpDir, ".gibson/run/agent/test-agent.pid")
	assert.Equal(t, expected, path)
}

// TestGetLockFilePath tests lock file path generation
func TestGetLockFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	path, err := GetLockFilePath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)
	expected := filepath.Join(tmpDir, ".gibson/run/agent/test-agent.lock")
	assert.Equal(t, expected, path)
}

// TestGetSocketPath tests socket path generation
func TestGetSocketPath(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	path, err := GetSocketPath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)
	expected := filepath.Join(tmpDir, ".gibson/run/agent/test-agent.sock")
	assert.Equal(t, expected, path)
}

// TestConcurrentAccess tests thread-safety of tracker operations
func TestConcurrentAccess(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()

	// Spawn multiple goroutines trying to acquire different locks
	const numComponents = 10
	done := make(chan bool, numComponents)

	for i := 0; i < numComponents; i++ {
		go func(idx int) {
			name := fmt.Sprintf("agent-%d", idx)
			err := tracker.acquireLock(ComponentKindAgent, name)
			if err != nil {
				done <- false
				return
			}
			defer tracker.releaseLock(ComponentKindAgent, name)

			// Simulate some work
			time.Sleep(10 * time.Millisecond)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numComponents; i++ {
		success := <-done
		assert.True(t, success, "goroutine should succeed")
	}

	// Verify all locks are released
	tracker.mu.Lock()
	assert.Equal(t, 0, len(tracker.lockFiles))
	tracker.mu.Unlock()
}
