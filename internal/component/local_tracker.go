package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// LocalTracker manages the filesystem-based lifecycle for local components.
// It uses PID files, exclusive lock files, and Unix domain sockets to track
// component state without relying on database records that can become stale.
//
// Local components are tracked using three filesystem mechanisms:
//  1. PID files (~/.gibson/run/{kind}/{name}.pid) - Contains process ID and port
//  2. Lock files (~/.gibson/run/{kind}/{name}.lock) - Exclusive flock prevents duplicate starts
//  3. Unix sockets (~/.gibson/run/{kind}/{name}.sock) - Primary liveness indicator, auto-cleaned by OS
//
// This approach ensures that component state accurately reflects actual process state,
// with the OS automatically cleaning up stale state when processes die.
type LocalTracker interface {
	// Start initiates the local component lifecycle by:
	//  1. Creating the run directory if it doesn't exist
	//  2. Writing the PID file with process ID and port
	//  3. Acquiring an exclusive lock on the lock file
	//  4. Waiting for the component to create its Unix socket
	//
	// If any step fails, previously created resources are cleaned up.
	// Returns an error if the component is already running (lock held)
	// or if the socket doesn't appear within the startup timeout.
	Start(ctx context.Context, comp *Component) error

	// Stop terminates the local component lifecycle by:
	//  1. Releasing the exclusive lock on the lock file
	//  2. Removing the PID file
	//
	// The Unix socket is removed by the component itself or automatically
	// by the OS when the process terminates.
	Stop(ctx context.Context, comp *Component) error

	// IsRunning checks if a local component is currently running by:
	//  1. Attempting to acquire the lock file (non-blocking)
	//  2. Checking if the Unix socket exists
	//
	// Returns true only if both the lock is held (by another process)
	// and the socket file exists, indicating an active component.
	IsRunning(ctx context.Context, kind ComponentKind, name string) (bool, error)

	// Discover scans the run directory for a given component kind,
	// validates each PID file, and returns the state of all running components.
	//
	// This method:
	//  1. Scans ~/.gibson/run/{kind}/*.pid files
	//  2. Validates each process is alive
	//  3. Checks socket existence
	//  4. Removes stale PID/lock files for dead processes
	//
	// Returns a slice of LocalComponentState for all healthy components.
	Discover(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error)

	// Cleanup removes stale PID and lock files for dead processes
	// across all component kinds. This should be called periodically
	// or at Gibson startup to ensure clean state.
	//
	// Cleanup is safe to run concurrently with other operations.
	Cleanup(ctx context.Context) error
}

// LocalComponentState represents the runtime state of a local component
// as discovered from the filesystem (PID files, sockets, locks).
type LocalComponentState struct {
	// Kind is the component type (agent, tool, plugin)
	Kind ComponentKind `json:"kind"`

	// Name is the component name
	Name string `json:"name"`

	// PID is the process ID from the PID file
	PID int `json:"pid"`

	// Port is the TCP port for gRPC connections (read from PID file)
	Port int `json:"port"`

	// Socket is the path to the Unix domain socket
	Socket string `json:"socket"`

	// Healthy indicates if the component passed health checks
	// (process alive, socket exists, lock held)
	Healthy bool `json:"healthy"`
}

// Runtime directory constants for local component tracking.
// These directories store PID files, lock files, and Unix sockets
// for each component kind.
const (
	// runDirBase is the base directory for all runtime state
	runDirBase = ".gibson/run"

	// runDirAgent is the directory for agent runtime files
	runDirAgent = "agent"

	// runDirTool is the directory for tool runtime files
	runDirTool = "tool"

	// runDirPlugin is the directory for plugin runtime files
	runDirPlugin = "plugin"
)

// GetRunDir returns the absolute path to the run directory for a given component kind.
// The run directory is where PID files, lock files, and Unix sockets are stored.
//
// Returns ~/.gibson/run/{kind}/ where kind is agent, tool, or plugin.
// For repository kinds, returns an error as they are not runtime components.
func GetRunDir(kind ComponentKind) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	var kindDir string
	switch kind {
	case ComponentKindAgent:
		kindDir = runDirAgent
	case ComponentKindTool:
		kindDir = runDirTool
	case ComponentKindPlugin:
		kindDir = runDirPlugin
	case ComponentKindRepository:
		return "", &InvalidComponentKindError{Kind: kind, Operation: "GetRunDir"}
	default:
		return "", &InvalidComponentKindError{Kind: kind, Operation: "GetRunDir"}
	}

	return filepath.Join(homeDir, runDirBase, kindDir), nil
}

// GetPIDFilePath returns the absolute path to the PID file for a component.
// PID files are stored as ~/.gibson/run/{kind}/{name}.pid
func GetPIDFilePath(kind ComponentKind, name string) (string, error) {
	runDir, err := GetRunDir(kind)
	if err != nil {
		return "", err
	}
	return filepath.Join(runDir, name+".pid"), nil
}

// GetLockFilePath returns the absolute path to the lock file for a component.
// Lock files are stored as ~/.gibson/run/{kind}/{name}.lock
func GetLockFilePath(kind ComponentKind, name string) (string, error) {
	runDir, err := GetRunDir(kind)
	if err != nil {
		return "", err
	}
	return filepath.Join(runDir, name+".lock"), nil
}

// GetSocketPath returns the absolute path to the Unix socket for a component.
// Sockets are stored as ~/.gibson/run/{kind}/{name}.sock
func GetSocketPath(kind ComponentKind, name string) (string, error) {
	runDir, err := GetRunDir(kind)
	if err != nil {
		return "", err
	}
	return filepath.Join(runDir, name+".sock"), nil
}

// InvalidComponentKindError is returned when an operation is attempted
// on an invalid or unsupported component kind.
type InvalidComponentKindError struct {
	Kind      ComponentKind
	Operation string
}

func (e *InvalidComponentKindError) Error() string {
	return "invalid component kind '" + e.Kind.String() + "' for operation: " + e.Operation
}

// PID file management functions
//
// PID files store process ID and port in a simple two-line format:
// Line 1: Process ID (integer)
// Line 2: TCP port (integer)
//
// These functions handle reading, writing, validating, and removing PID files
// for local component tracking.

// writePIDFile atomically writes a PID file containing the process ID and port.
// The file is written with 0600 permissions using atomic rename to prevent corruption.
//
// Format:
//
//	PID\n
//	PORT\n
//
// The write is atomic: we write to a temporary file and then rename it.
// This prevents partial writes in case of crashes.
func writePIDFile(kind ComponentKind, name string, pid, port int) error {
	pidPath, err := GetPIDFilePath(kind, name)
	if err != nil {
		return err
	}

	// Ensure run directory exists
	runDir := filepath.Dir(pidPath)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("failed to create run directory: %w", err)
	}

	// Write to temporary file first for atomic operation
	tmpPath := pidPath + ".tmp"
	content := fmt.Sprintf("%d\n%d\n", pid, port)

	// Write with 0600 permissions (owner read/write only)
	if err := os.WriteFile(tmpPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write temporary PID file: %w", err)
	}

	// Atomically rename to final location
	if err := os.Rename(tmpPath, pidPath); err != nil {
		// Clean up temp file on failure
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename PID file: %w", err)
	}

	return nil
}

// readPIDFile reads a PID file and returns the process ID and port.
// Returns an error if the file doesn't exist, is malformed, or contains invalid data.
func readPIDFile(kind ComponentKind, name string) (pid, port int, err error) {
	pidPath, err := GetPIDFilePath(kind, name)
	if err != nil {
		return 0, 0, err
	}

	// Read file contents
	data, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, fmt.Errorf("PID file does not exist: %w", err)
		}
		return 0, 0, fmt.Errorf("failed to read PID file: %w", err)
	}

	// Parse content (two lines: PID and PORT)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		return 0, 0, fmt.Errorf("invalid PID file format: expected 2 lines, got %d", len(lines))
	}

	// Parse PID
	pid, err = strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid PID in PID file: %w", err)
	}

	// Parse port
	port, err = strconv.Atoi(strings.TrimSpace(lines[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid port in PID file: %w", err)
	}

	// Validate ranges
	if pid <= 0 {
		return 0, 0, fmt.Errorf("invalid PID value: %d", pid)
	}
	if port <= 0 || port > 65535 {
		return 0, 0, fmt.Errorf("invalid port value: %d", port)
	}

	return pid, port, nil
}

// removePIDFile deletes a PID file.
// Returns nil if the file doesn't exist (already removed).
func removePIDFile(kind ComponentKind, name string) error {
	pidPath, err := GetPIDFilePath(kind, name)
	if err != nil {
		return err
	}

	err = os.Remove(pidPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}

	return nil
}

// validatePID checks if a process with the given PID is alive.
// This implementation uses the /proc filesystem to validate process state,
// reusing the logic from lifecycle.go's isProcessAlive.
//
// Returns true if:
//   - Process exists (can send signal 0)
//   - Process is not a zombie (state != Z)
//   - Process is not dead (state != X)
//
// This handles edge cases like zombie processes that technically exist
// but are not functional.
func validatePID(pid int) bool {
	// Try to find the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix systems, FindProcess always succeeds, so we need to send signal 0
	// to check if the process actually exists
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		return false
	}

	// Check if process is a zombie by reading /proc/<pid>/stat
	// This is critical because zombie processes respond to signal 0
	// but are not actually running
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		// If we can't read stat, process may not exist or be accessible
		return false
	}

	// The stat file format has the process state as the third field
	// Format: pid (comm) state ...
	// State can be: R (running), S (sleeping), D (disk sleep), Z (zombie), T (stopped), etc.
	stat := string(data)

	// Find the closing paren of comm field (to handle comm with spaces/special chars)
	closeParen := -1
	for i := len(stat) - 1; i >= 0; i-- {
		if stat[i] == ')' {
			closeParen = i
			break
		}
	}
	if closeParen == -1 || closeParen+2 >= len(stat) {
		return false
	}

	// State is after ") "
	state := stat[closeParen+2]
	// Z = zombie, X = dead
	if state == 'Z' || state == 'X' {
		return false
	}

	return true
}

// DefaultLocalTracker is the default implementation of LocalTracker.
// It manages local component lifecycle using PID files, lock files, and Unix sockets.
//
// The tracker maintains a map of open lock file handles for cleanup and
// ensures thread-safe access to the lock file map using a mutex.
type DefaultLocalTracker struct {
	// mu protects access to lockFiles map
	mu sync.Mutex

	// lockFiles stores open lock file descriptors keyed by lock file path
	// These file descriptors hold exclusive flocks and must be kept open
	// until the lock is released.
	lockFiles map[string]*os.File
}

// NewDefaultLocalTracker creates a new DefaultLocalTracker instance.
func NewDefaultLocalTracker() *DefaultLocalTracker {
	return &DefaultLocalTracker{
		lockFiles: make(map[string]*os.File),
	}
}

// acquireLock acquires an exclusive lock on the lock file for the given component.
// It uses flock(2) with LOCK_EX to ensure only one process can start a component.
//
// The lock file is created with 0600 permissions (owner read/write only).
// The file descriptor is stored in the lockFiles map and must remain open
// until releaseLock is called.
//
// Returns an error if:
//   - The lock file path cannot be determined
//   - The lock file cannot be created
//   - The lock is already held by another process (EWOULDBLOCK)
//   - flock syscall fails
func (t *DefaultLocalTracker) acquireLock(kind ComponentKind, name string) error {
	lockPath, err := GetLockFilePath(kind, name)
	if err != nil {
		return fmt.Errorf("failed to get lock file path: %w", err)
	}

	// Create lock file with 0600 permissions (owner read/write only)
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("failed to create lock file %s: %w", lockPath, err)
	}

	// Attempt to acquire exclusive lock (non-blocking)
	// LOCK_EX = exclusive lock, LOCK_NB = non-blocking
	err = unix.Flock(int(lockFile.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err != nil {
		lockFile.Close()
		if err == unix.EWOULDBLOCK {
			// Lock is held by another process - component is already running
			return &ComponentAlreadyRunningError{
				Kind: kind,
				Name: name,
			}
		}
		return fmt.Errorf("failed to acquire lock on %s: %w", lockPath, err)
	}

	// Store file descriptor for later release
	t.mu.Lock()
	t.lockFiles[lockPath] = lockFile
	t.mu.Unlock()

	return nil
}

// releaseLock releases the exclusive lock on the lock file for the given component.
// It closes the file descriptor and removes it from the lockFiles map.
//
// The flock is automatically released when the file descriptor is closed.
// This function is safe to call even if the lock was never acquired.
//
// Returns an error if the lock file path cannot be determined.
func (t *DefaultLocalTracker) releaseLock(kind ComponentKind, name string) error {
	lockPath, err := GetLockFilePath(kind, name)
	if err != nil {
		return fmt.Errorf("failed to get lock file path: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Get the lock file descriptor
	lockFile, exists := t.lockFiles[lockPath]
	if !exists {
		// Lock was never acquired or already released
		return nil
	}

	// Close the file descriptor (this releases the flock automatically)
	if err := lockFile.Close(); err != nil {
		return fmt.Errorf("failed to close lock file %s: %w", lockPath, err)
	}

	// Remove from map
	delete(t.lockFiles, lockPath)

	return nil
}

// isLocked checks if the lock file is currently held by another process.
// It performs a non-blocking lock attempt to detect if the lock is held.
//
// This function does NOT acquire the lock - it only checks if the lock
// is available. If the lock is not held, the file descriptor is immediately
// closed.
//
// Returns:
//   - (true, nil) if the lock is held by another process
//   - (false, nil) if the lock is available
//   - (false, error) if the lock file path cannot be determined or check fails
func (t *DefaultLocalTracker) isLocked(kind ComponentKind, name string) (bool, error) {
	lockPath, err := GetLockFilePath(kind, name)
	if err != nil {
		return false, fmt.Errorf("failed to get lock file path: %w", err)
	}

	// Check if lock file exists
	_, err = os.Stat(lockPath)
	if os.IsNotExist(err) {
		// No lock file means not locked
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to stat lock file %s: %w", lockPath, err)
	}

	// Try to open the lock file
	lockFile, err := os.OpenFile(lockPath, os.O_RDONLY, 0600)
	if err != nil {
		// If we can't open it, assume it's not locked (or there's a permission issue)
		return false, fmt.Errorf("failed to open lock file %s: %w", lockPath, err)
	}
	defer lockFile.Close()

	// Try to acquire lock non-blocking
	err = unix.Flock(int(lockFile.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == unix.EWOULDBLOCK {
		// Lock is held by another process
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check lock on %s: %w", lockPath, err)
	}

	// Lock was acquired - release it immediately and return false
	unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)
	return false, nil
}

// ComponentAlreadyRunningError is returned when attempting to start a component
// that is already running (lock file is held).
type ComponentAlreadyRunningError struct {
	Kind ComponentKind
	Name string
}

func (e *ComponentAlreadyRunningError) Error() string {
	return fmt.Sprintf("component %s/%s is already running", e.Kind, e.Name)
}

// Socket health check constants
const (
	// DefaultSocketStartupTimeout is the default timeout for waiting for a socket to appear
	DefaultSocketStartupTimeout = 30 * time.Second

	// DefaultSocketHealthTimeout is the default timeout for socket health checks
	DefaultSocketHealthTimeout = 5 * time.Second

	// DefaultSocketPollInterval is the default interval for polling socket existence
	DefaultSocketPollInterval = 100 * time.Millisecond
)

// waitForSocket polls for the existence of a Unix socket file with a timeout.
// It checks for socket existence at regular intervals until either the socket
// appears or the timeout is reached.
//
// This function is used during component startup to wait for the component to
// create its Unix socket after the process has been started.
//
// Parameters:
//   - ctx: Context for cancellation and deadline propagation
//   - socketPath: Absolute path to the Unix socket file
//   - timeout: Maximum time to wait for socket to appear (0 uses default)
//
// Returns:
//   - error: nil if socket appears within timeout, error otherwise
func (t *DefaultLocalTracker) waitForSocket(ctx context.Context, socketPath string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = DefaultSocketStartupTimeout
	}

	// Create a timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(DefaultSocketPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			if ctx.Err() != nil {
				// Parent context was cancelled
				return ctx.Err()
			}
			// Timeout waiting for socket
			return NewTimeoutError(socketPath, "waiting for socket to appear")

		case <-ticker.C:
			// Check if socket exists
			info, err := os.Stat(socketPath)
			if err == nil {
				// Socket file exists, verify it's actually a socket
				if info.Mode()&os.ModeSocket != 0 {
					return nil
				}
				// File exists but is not a socket
				return NewConnectionFailedError(socketPath, fmt.Errorf("file exists but is not a socket"))
			}
			// If file doesn't exist, continue polling
			if !os.IsNotExist(err) {
				// Some other error occurred (permission denied, etc.)
				return NewConnectionFailedError(socketPath, fmt.Errorf("failed to stat socket: %w", err))
			}
		}
	}
}

// checkSocketHealth connects to a Unix socket and performs a gRPC health check.
// This verifies that the component is not only running, but also responding
// correctly to health check requests.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - socketPath: Absolute path to the Unix socket file
//   - timeout: Timeout for the health check operation (0 uses default)
//
// Returns:
//   - error: nil if health check passes, error otherwise
func (t *DefaultLocalTracker) checkSocketHealth(ctx context.Context, socketPath string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = DefaultSocketHealthTimeout
	}

	// Create a timeout context for the health check
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Check if socket exists first
	info, err := os.Stat(socketPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewConnectionFailedError(socketPath, fmt.Errorf("socket does not exist"))
		}
		return NewConnectionFailedError(socketPath, fmt.Errorf("failed to stat socket: %w", err))
	}

	// Verify it's actually a socket
	if info.Mode()&os.ModeSocket == 0 {
		return NewConnectionFailedError(socketPath, fmt.Errorf("file is not a socket"))
	}

	// Create gRPC health checker using Unix socket
	// Format: unix:///path/to/socket (absolute path needs three slashes)
	socketAddr := "unix://" + socketPath
	checker, err := NewGRPCHealthChecker(socketAddr, WithGRPCTimeout(timeout))
	if err != nil {
		return NewHealthCheckError(socketPath, "grpc", fmt.Errorf("failed to create health checker: %w", err))
	}
	defer checker.Close()

	// Perform the health check
	if err := checker.Check(checkCtx); err != nil {
		return err
	}

	return nil
}

// cleanupStaleSocket removes a Unix socket file if it exists but the connection fails.
// This is used to clean up sockets left behind by crashed or improperly terminated processes.
//
// The cleanup is performed atomically to prevent race conditions with other processes.
// If the socket is actively being used by another process, the cleanup will fail safely.
//
// Parameters:
//   - ctx: Context for cancellation (not used for timeout as cleanup should be fast)
//   - socketPath: Absolute path to the Unix socket file
//
// Returns:
//   - error: nil if socket was cleaned up or didn't exist, error otherwise
func (t *DefaultLocalTracker) cleanupStaleSocket(ctx context.Context, socketPath string) error {
	// Check if socket exists
	info, err := os.Stat(socketPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Socket doesn't exist, nothing to clean up
			return nil
		}
		return fmt.Errorf("failed to stat socket: %w", err)
	}

	// Verify it's actually a socket
	if info.Mode()&os.ModeSocket == 0 {
		// Not a socket, don't remove it
		return fmt.Errorf("file exists but is not a socket, refusing to remove")
	}

	// Try to connect to verify it's really stale
	// Use a very short timeout since we're just checking if it's alive
	quickCheckCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	err = t.checkSocketHealth(quickCheckCtx, socketPath, 500*time.Millisecond)
	if err == nil {
		// Socket is healthy, don't remove it
		return fmt.Errorf("socket is healthy and in use, refusing to remove")
	}

	// Socket exists but connection failed, safe to remove
	if err := os.Remove(socketPath); err != nil {
		return fmt.Errorf("failed to remove stale socket: %w", err)
	}

	return nil
}

// Start initiates the local component lifecycle by:
//  1. Creating the run directory if it doesn't exist
//  2. Writing the PID file with process ID and port
//  3. Acquiring an exclusive lock on the lock file
//  4. Waiting for the component to create its Unix socket
//
// If any step fails, previously created resources are cleaned up.
// Returns an error if the component is already running (lock held)
// or if the socket doesn't appear within the startup timeout.
func (t *DefaultLocalTracker) Start(ctx context.Context, comp *Component) error {
	if comp == nil {
		return fmt.Errorf("component cannot be nil")
	}

	// Ensure run directory exists
	runDir, err := GetRunDir(comp.Kind)
	if err != nil {
		return fmt.Errorf("failed to get run directory: %w", err)
	}
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("failed to create run directory: %w", err)
	}

	// Track what we've done for cleanup on failure
	pidWritten := false
	lockAcquired := false

	// Cleanup helper
	cleanup := func() {
		if lockAcquired {
			_ = t.releaseLock(comp.Kind, comp.Name)
		}
		if pidWritten {
			_ = removePIDFile(comp.Kind, comp.Name)
		}
	}

	// Write PID file
	// Note: comp.PID and comp.Port should be set by the caller before calling Start
	if err := writePIDFile(comp.Kind, comp.Name, comp.PID, comp.Port); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}
	pidWritten = true

	// Acquire exclusive lock
	if err := t.acquireLock(comp.Kind, comp.Name); err != nil {
		cleanup()
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	lockAcquired = true

	// Wait for component to create Unix socket
	socketPath, err := GetSocketPath(comp.Kind, comp.Name)
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to get socket path: %w", err)
	}

	if err := t.waitForSocket(ctx, socketPath, DefaultSocketStartupTimeout); err != nil {
		cleanup()
		return fmt.Errorf("failed waiting for socket: %w", err)
	}

	return nil
}

// Stop terminates the local component lifecycle by:
//  1. Releasing the exclusive lock on the lock file
//  2. Removing the PID file
//
// The Unix socket is removed by the component itself or automatically
// by the OS when the process terminates.
func (t *DefaultLocalTracker) Stop(ctx context.Context, comp *Component) error {
	if comp == nil {
		return fmt.Errorf("component cannot be nil")
	}

	// Release the lock first
	if err := t.releaseLock(comp.Kind, comp.Name); err != nil {
		// Log but continue - we still want to remove PID file
		// In production, use proper logging instead of fmt
		fmt.Fprintf(os.Stderr, "warning: failed to release lock for %s/%s: %v\n", comp.Kind, comp.Name, err)
	}

	// Remove PID file
	if err := removePIDFile(comp.Kind, comp.Name); err != nil {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}

	// Note: Socket is removed by component itself or OS on process exit
	// We don't remove it here to avoid race conditions

	return nil
}

// IsRunning checks if a local component is currently running by:
//  1. Checking if the PID file exists and contains a valid, alive process
//  2. Checking if the Unix socket exists
//
// Returns true if the process is alive AND the socket exists.
// Note: We don't require the lock to be held because the lock is only
// used during startup to prevent race conditions. After Gibson (the starter)
// exits, the lock is released but the component continues running.
func (t *DefaultLocalTracker) IsRunning(ctx context.Context, kind ComponentKind, name string) (bool, error) {
	// Check if PID file exists and process is alive
	pid, _, err := readPIDFile(kind, name)
	if err != nil {
		// No PID file or invalid - not running
		return false, nil
	}

	// Validate process is alive
	if !validatePID(pid) {
		// Process is dead
		return false, nil
	}

	// Check if socket exists
	socketPath, err := GetSocketPath(kind, name)
	if err != nil {
		return false, fmt.Errorf("failed to get socket path: %w", err)
	}

	info, err := os.Stat(socketPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Socket doesn't exist, component is not healthy
			return false, nil
		}
		return false, fmt.Errorf("failed to stat socket: %w", err)
	}

	// Verify it's actually a socket
	if info.Mode()&os.ModeSocket == 0 {
		// File exists but is not a socket
		return false, nil
	}

	// Process is alive and socket exists
	return true, nil
}

// Discover scans the run directory for a given component kind,
// validates each PID file, and returns the state of all running components.
//
// This method:
//  1. Scans ~/.gibson/run/{kind}/*.pid files
//  2. Validates each process is alive
//  3. Checks socket existence
//  4. Removes stale PID/lock files for dead processes
//
// Returns a slice of LocalComponentState for all healthy components.
func (t *DefaultLocalTracker) Discover(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
	runDir, err := GetRunDir(kind)
	if err != nil {
		return nil, fmt.Errorf("failed to get run directory: %w", err)
	}

	// Ensure run directory exists
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create run directory: %w", err)
	}

	// Read all .pid files in the directory
	pidPattern := filepath.Join(runDir, "*.pid")
	pidFiles, err := filepath.Glob(pidPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob PID files: %w", err)
	}

	var states []LocalComponentState

	// Process each PID file
	for _, pidPath := range pidFiles {
		// Extract component name from filename
		filename := filepath.Base(pidPath)
		name := strings.TrimSuffix(filename, ".pid")

		// Read PID file
		pid, port, err := readPIDFile(kind, name)
		if err != nil {
			// Stale or corrupted PID file - clean it up
			_ = removePIDFile(kind, name)
			_ = t.releaseLock(kind, name)
			continue
		}

		// Validate process is alive
		if !validatePID(pid) {
			// Process is dead - clean up stale files
			_ = removePIDFile(kind, name)
			_ = t.releaseLock(kind, name)

			// Clean up stale socket if it exists
			socketPath, _ := GetSocketPath(kind, name)
			_ = t.cleanupStaleSocket(ctx, socketPath)
			continue
		}

		// Check socket existence
		socketPath, err := GetSocketPath(kind, name)
		if err != nil {
			continue
		}

		info, err := os.Stat(socketPath)
		socketExists := err == nil && info.Mode()&os.ModeSocket != 0

		// Component is healthy if process is alive AND socket exists.
		// Note: We don't require the lock to be held because:
		// 1. The lock is acquired by Gibson during startup to prevent races
		// 2. The lock is released when Gibson exits (the starter process)
		// 3. The component process (k8skiller) continues running without holding the lock
		// The lock's purpose is startup race prevention, not ongoing health indication.
		healthy := socketExists

		// If process is alive but socket missing, clean up
		if !healthy {
			_ = removePIDFile(kind, name)
			continue
		}

		// Add to results
		states = append(states, LocalComponentState{
			Kind:    kind,
			Name:    name,
			PID:     pid,
			Port:    port,
			Socket:  socketPath,
			Healthy: healthy,
		})
	}

	return states, nil
}

// Cleanup removes stale PID and lock files for dead processes
// across all component kinds. This should be called periodically
// or at Gibson startup to ensure clean state.
//
// Cleanup is safe to run concurrently with other operations.
func (t *DefaultLocalTracker) Cleanup(ctx context.Context) error {
	// Define all component kinds to clean up
	kinds := []ComponentKind{
		ComponentKindAgent,
		ComponentKindTool,
		ComponentKindPlugin,
	}

	// Track errors but don't fail on individual failures
	var cleanupErrors []error

	for _, kind := range kinds {
		runDir, err := GetRunDir(kind)
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to get run dir for %s: %w", kind, err))
			continue
		}

		// Ensure run directory exists
		if err := os.MkdirAll(runDir, 0755); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to create run dir for %s: %w", kind, err))
			continue
		}

		// Read all .pid files
		pidPattern := filepath.Join(runDir, "*.pid")
		pidFiles, err := filepath.Glob(pidPattern)
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to glob PID files for %s: %w", kind, err))
			continue
		}

		// Process each PID file
		for _, pidPath := range pidFiles {
			filename := filepath.Base(pidPath)
			name := strings.TrimSuffix(filename, ".pid")

			// Read PID file
			pid, _, err := readPIDFile(kind, name)
			if err != nil {
				// Corrupted PID file - remove it
				_ = removePIDFile(kind, name)
				_ = t.releaseLock(kind, name)

				// Clean up lock file if it exists
				lockPath, _ := GetLockFilePath(kind, name)
				_ = os.Remove(lockPath)
				continue
			}

			// Validate process is alive
			if !validatePID(pid) {
				// Process is dead - clean up all files
				_ = removePIDFile(kind, name)
				_ = t.releaseLock(kind, name)

				// Remove lock file
				lockPath, _ := GetLockFilePath(kind, name)
				_ = os.Remove(lockPath)

				// Clean up stale socket
				socketPath, _ := GetSocketPath(kind, name)
				_ = t.cleanupStaleSocket(ctx, socketPath)
			}
		}

		// Also clean up orphaned lock files (lock without PID file)
		lockPattern := filepath.Join(runDir, "*.lock")
		lockFiles, err := filepath.Glob(lockPattern)
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to glob lock files for %s: %w", kind, err))
			continue
		}

		for _, lockPath := range lockFiles {
			filename := filepath.Base(lockPath)
			name := strings.TrimSuffix(filename, ".lock")

			// Check if corresponding PID file exists
			pidPath, _ := GetPIDFilePath(kind, name)
			if _, err := os.Stat(pidPath); os.IsNotExist(err) {
				// PID file doesn't exist - remove orphaned lock
				_ = os.Remove(lockPath)
			}
		}
	}

	// If we had any errors, combine them
	if len(cleanupErrors) > 0 {
		// Create a combined error message
		errMsg := "cleanup encountered errors:"
		for _, err := range cleanupErrors {
			errMsg += "\n  " + err.Error()
		}
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}
