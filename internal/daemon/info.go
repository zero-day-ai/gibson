package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteDaemonInfo atomically writes daemon connection information to a JSON file.
//
// This function serializes the DaemonInfo struct to JSON and writes it to
// ~/.gibson/daemon.json. Clients read this file to discover how to connect
// to the running daemon (gRPC address, socket path, version, etc.).
//
// The write operation is atomic using a temp file and rename strategy to
// prevent corruption if the process crashes mid-write.
//
// Parameters:
//   - path: Full path to the daemon info file (typically ~/.gibson/daemon.json)
//   - info: Daemon connection information to persist
//
// Returns:
//   - error: Non-nil if serialization or file operations fail
//
// Example usage:
//
//	info := &DaemonInfo{
//	    PID:         os.Getpid(),
//	    StartTime:   time.Now(),
//	    GRPCAddress: "localhost:50002",
//	    Version:     "1.0.0",
//	}
//	if err := WriteDaemonInfo("/home/user/.gibson/daemon.json", info); err != nil {
//	    return fmt.Errorf("failed to write daemon info: %w", err)
//	}
func WriteDaemonInfo(path string, info *DaemonInfo) error {
	if info == nil {
		return fmt.Errorf("daemon info cannot be nil")
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create daemon info directory: %w", err)
	}

	// Create temp file in same directory (required for atomic rename)
	tempFile, err := os.CreateTemp(dir, ".daemon.json.tmp.*")
	if err != nil {
		return fmt.Errorf("failed to create temp daemon info file: %w", err)
	}
	tempPath := tempFile.Name()

	// Serialize to JSON with indentation for readability
	encoder := json.NewEncoder(tempFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(info); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return fmt.Errorf("failed to encode daemon info to JSON: %w", err)
	}

	// Set restrictive permissions (owner read/write only)
	if err := tempFile.Chmod(0600); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return fmt.Errorf("failed to set daemon info file permissions: %w", err)
	}

	// Close temp file before rename
	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to close temp daemon info file: %w", err)
	}

	// Atomic rename (overwrites existing file)
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename daemon info file: %w", err)
	}

	return nil
}

// ReadDaemonInfo reads daemon connection information from the JSON file.
//
// This function is used by clients to discover how to connect to the running
// daemon. It reads and deserializes the daemon.json file created by WriteDaemonInfo.
//
// Parameters:
//   - path: Full path to the daemon info file
//
// Returns:
//   - *DaemonInfo: The daemon connection information
//   - error: Non-nil if file read or deserialization fails
//
// Example usage:
//
//	info, err := ReadDaemonInfo("/home/user/.gibson/daemon.json")
//	if err != nil {
//	    if os.IsNotExist(err) {
//	        return fmt.Errorf("daemon not running: %w", err)
//	    }
//	    return fmt.Errorf("failed to read daemon info: %w", err)
//	}
//	fmt.Printf("Connecting to daemon at %s (PID %d)\n", info.GRPCAddress, info.PID)
func ReadDaemonInfo(path string) (*DaemonInfo, error) {
	// Read file contents
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("daemon info file not found (daemon not running?): %w", err)
		}
		return nil, fmt.Errorf("failed to read daemon info file: %w", err)
	}

	// Deserialize JSON
	var info DaemonInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("failed to parse daemon info JSON: %w", err)
	}

	// Validate required fields
	if info.PID <= 0 {
		return nil, fmt.Errorf("invalid daemon info: PID must be positive (got %d)", info.PID)
	}
	if info.GRPCAddress == "" {
		return nil, fmt.Errorf("invalid daemon info: gRPC address is required")
	}

	return &info, nil
}

// RemoveDaemonInfo deletes the daemon info file.
//
// This function is called during daemon shutdown to clean up state files.
// It is idempotent - removing a non-existent file is not an error.
//
// Parameters:
//   - path: Full path to the daemon info file
//
// Returns:
//   - error: Non-nil if removal fails (excluding "file not found")
//
// Example usage:
//
//	defer RemoveDaemonInfo("/home/user/.gibson/daemon.json")
func RemoveDaemonInfo(path string) error {
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			// File already gone - not an error
			return nil
		}
		return fmt.Errorf("failed to remove daemon info file: %w", err)
	}
	return nil
}
