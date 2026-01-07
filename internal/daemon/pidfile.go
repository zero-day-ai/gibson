package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// WritePIDFile atomically writes the process ID to the specified file.
//
// This function creates the PID file with 0600 permissions (owner read/write only)
// to prevent other users from tampering with daemon state. The write operation
// is atomic using a temp file and rename strategy to prevent corruption.
//
// Parameters:
//   - path: Full path to the PID file (typically ~/.gibson/daemon.pid)
//   - pid: The process ID to write (typically os.Getpid())
//
// Returns:
//   - error: Non-nil if file operations fail
//
// Example usage:
//
//	if err := WritePIDFile("/home/user/.gibson/daemon.pid", os.Getpid()); err != nil {
//	    return fmt.Errorf("failed to write PID file: %w", err)
//	}
func WritePIDFile(path string, pid int) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create PID file directory: %w", err)
	}

	// Create temp file in same directory (required for atomic rename)
	tempFile, err := os.CreateTemp(dir, ".daemon.pid.tmp.*")
	if err != nil {
		return fmt.Errorf("failed to create temp PID file: %w", err)
	}
	tempPath := tempFile.Name()

	// Write PID to temp file
	if _, err := fmt.Fprintf(tempFile, "%d\n", pid); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return fmt.Errorf("failed to write PID to temp file: %w", err)
	}

	// Set restrictive permissions (owner read/write only)
	if err := tempFile.Chmod(0600); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return fmt.Errorf("failed to set PID file permissions: %w", err)
	}

	// Close temp file before rename
	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to close temp PID file: %w", err)
	}

	// Atomic rename (overwrites existing file)
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename PID file: %w", err)
	}

	return nil
}

// ReadPIDFile reads the process ID from the specified file.
//
// This function reads and parses the PID from the file created by WritePIDFile.
// It handles common error cases like missing files, invalid content, and
// permission issues.
//
// Parameters:
//   - path: Full path to the PID file
//
// Returns:
//   - int: The process ID read from the file (0 if file doesn't exist)
//   - error: Non-nil if file read or parse fails
//
// Example usage:
//
//	pid, err := ReadPIDFile("/home/user/.gibson/daemon.pid")
//	if err != nil {
//	    return fmt.Errorf("failed to read PID file: %w", err)
//	}
//	if pid == 0 {
//	    fmt.Println("Daemon not running (no PID file)")
//	}
func ReadPIDFile(path string) (int, error) {
	// Check if file exists
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No PID file means no daemon running
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read PID file: %w", err)
	}

	// Parse PID from file content
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID in file %q: %w", path, err)
	}

	if pid <= 0 {
		return 0, fmt.Errorf("invalid PID value %d in file %q", pid, path)
	}

	return pid, nil
}

// RemovePIDFile deletes the PID file.
//
// This function is called during daemon shutdown to clean up state files.
// It is idempotent - removing a non-existent file is not an error.
//
// Parameters:
//   - path: Full path to the PID file
//
// Returns:
//   - error: Non-nil if removal fails (excluding "file not found")
//
// Example usage:
//
//	defer RemovePIDFile("/home/user/.gibson/daemon.pid")
func RemovePIDFile(path string) error {
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			// File already gone - not an error
			return nil
		}
		return fmt.Errorf("failed to remove PID file: %w", err)
	}
	return nil
}

// CheckPIDFile checks if a daemon is running based on the PID file.
//
// This function performs three checks:
// 1. Does the PID file exist?
// 2. Is the PID valid?
// 3. Does a process with that PID exist?
//
// The process existence check uses syscall.Kill(pid, 0) which sends signal 0
// (no signal) to check if the process exists without affecting it. This works
// on Unix-like systems including Linux and macOS.
//
// Parameters:
//   - path: Full path to the PID file
//
// Returns:
//   - running: true if daemon is running, false otherwise
//   - pid: The process ID from the file (0 if file doesn't exist)
//   - err: Non-nil if checks fail (file read error, permission issue, etc.)
//
// Example usage:
//
//	running, pid, err := CheckPIDFile("/home/user/.gibson/daemon.pid")
//	if err != nil {
//	    return fmt.Errorf("failed to check daemon status: %w", err)
//	}
//	if running {
//	    fmt.Printf("Daemon running (PID %d)\n", pid)
//	} else if pid > 0 {
//	    fmt.Printf("Stale PID file (process %d not running)\n", pid)
//	} else {
//	    fmt.Println("Daemon not running")
//	}
func CheckPIDFile(path string) (running bool, pid int, err error) {
	// Read PID from file
	pid, err = ReadPIDFile(path)
	if err != nil {
		return false, 0, err
	}

	// No PID file means no daemon
	if pid == 0 {
		return false, 0, nil
	}

	// Check if process exists using kill(pid, 0)
	// This sends no signal but checks if we can send a signal to the process
	// Returns nil if process exists, ESRCH if process doesn't exist
	err = syscall.Kill(pid, 0)
	if err == nil {
		// Process exists
		return true, pid, nil
	}

	// Check for specific error types
	if err == syscall.ESRCH {
		// Process doesn't exist - stale PID file
		return false, pid, nil
	}

	if err == syscall.EPERM {
		// Process exists but we don't have permission to signal it
		// This can happen if the daemon is running as a different user
		// We'll consider this as "running" since the process exists
		return true, pid, nil
	}

	// Other error (shouldn't happen but handle it)
	return false, pid, fmt.Errorf("failed to check process %d: %w", pid, err)
}
