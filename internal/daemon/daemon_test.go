package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestWritePIDFile tests PID file creation with proper permissions.
func TestWritePIDFile(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test.pid")

	// Write PID file
	pid := 12345
	err := WritePIDFile(pidFile, pid)
	if err != nil {
		t.Fatalf("WritePIDFile failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		t.Fatalf("PID file was not created")
	}

	// Verify file permissions (0600)
	info, err := os.Stat(pidFile)
	if err != nil {
		t.Fatalf("Failed to stat PID file: %v", err)
	}
	mode := info.Mode()
	if mode.Perm() != 0600 {
		t.Errorf("PID file has wrong permissions: got %o, want 0600", mode.Perm())
	}

	// Verify file content
	readPID, err := ReadPIDFile(pidFile)
	if err != nil {
		t.Fatalf("ReadPIDFile failed: %v", err)
	}
	if readPID != pid {
		t.Errorf("PID mismatch: got %d, want %d", readPID, pid)
	}
}

// TestWritePIDFileCreatesDirectory tests that parent directory is created.
func TestWritePIDFileCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "subdir", "nested", "test.pid")

	// Write PID file (should create parent directories)
	err := WritePIDFile(pidFile, 99999)
	if err != nil {
		t.Fatalf("WritePIDFile failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		t.Fatalf("PID file was not created")
	}
}

// TestReadPIDFileNotExists tests reading non-existent PID file.
func TestReadPIDFileNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "nonexistent.pid")

	pid, err := ReadPIDFile(pidFile)
	if err != nil {
		t.Errorf("ReadPIDFile should not error for missing file: %v", err)
	}
	if pid != 0 {
		t.Errorf("ReadPIDFile should return 0 for missing file, got %d", pid)
	}
}

// TestReadPIDFileInvalid tests reading PID file with invalid content.
func TestReadPIDFileInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "invalid.pid")

	// Write invalid content
	err := os.WriteFile(pidFile, []byte("not-a-number\n"), 0600)
	if err != nil {
		t.Fatalf("Failed to write invalid PID file: %v", err)
	}

	_, err = ReadPIDFile(pidFile)
	if err == nil {
		t.Errorf("ReadPIDFile should error for invalid content")
	}
}

// TestReadPIDFileNegative tests reading PID file with negative PID.
func TestReadPIDFileNegative(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "negative.pid")

	// Write negative PID
	err := os.WriteFile(pidFile, []byte("-123\n"), 0600)
	if err != nil {
		t.Fatalf("Failed to write negative PID file: %v", err)
	}

	_, err = ReadPIDFile(pidFile)
	if err == nil {
		t.Errorf("ReadPIDFile should error for negative PID")
	}
}

// TestRemovePIDFile tests PID file removal.
func TestRemovePIDFile(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test.pid")

	// Create PID file
	err := WritePIDFile(pidFile, 12345)
	if err != nil {
		t.Fatalf("WritePIDFile failed: %v", err)
	}

	// Remove PID file
	err = RemovePIDFile(pidFile)
	if err != nil {
		t.Fatalf("RemovePIDFile failed: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Errorf("PID file still exists after removal")
	}
}

// TestRemovePIDFileNotExists tests removing non-existent PID file (should be no-op).
func TestRemovePIDFileNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "nonexistent.pid")

	// Remove non-existent file (should not error)
	err := RemovePIDFile(pidFile)
	if err != nil {
		t.Errorf("RemovePIDFile should not error for missing file: %v", err)
	}
}

// TestCheckPIDFileNotExists tests checking non-existent PID file.
func TestCheckPIDFileNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "nonexistent.pid")

	running, pid, err := CheckPIDFile(pidFile)
	if err != nil {
		t.Errorf("CheckPIDFile should not error for missing file: %v", err)
	}
	if running {
		t.Errorf("CheckPIDFile should return running=false for missing file")
	}
	if pid != 0 {
		t.Errorf("CheckPIDFile should return pid=0 for missing file, got %d", pid)
	}
}

// TestCheckPIDFileCurrentProcess tests checking PID file for current process.
func TestCheckPIDFileCurrentProcess(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "current.pid")

	// Write current process PID
	currentPID := os.Getpid()
	err := WritePIDFile(pidFile, currentPID)
	if err != nil {
		t.Fatalf("WritePIDFile failed: %v", err)
	}

	// Check PID file (should be running)
	running, pid, err := CheckPIDFile(pidFile)
	if err != nil {
		t.Fatalf("CheckPIDFile failed: %v", err)
	}
	if !running {
		t.Errorf("CheckPIDFile should return running=true for current process")
	}
	if pid != currentPID {
		t.Errorf("CheckPIDFile returned wrong PID: got %d, want %d", pid, currentPID)
	}
}

// TestCheckPIDFileStalePID tests checking PID file for non-existent process.
func TestCheckPIDFileStalePID(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "stale.pid")

	// Write a PID that's very unlikely to exist
	// We'll use a high PID that's unlikely to be allocated
	stalePID := 999999
	err := WritePIDFile(pidFile, stalePID)
	if err != nil {
		t.Fatalf("WritePIDFile failed: %v", err)
	}

	// Check PID file (should be not running due to stale PID)
	running, pid, err := CheckPIDFile(pidFile)
	if err != nil {
		t.Fatalf("CheckPIDFile failed: %v", err)
	}

	// We expect the process not to exist, but if it does exist (unlikely),
	// we can't reliably test this without potentially interfering with a real process
	if running {
		// If the process exists, verify we got the right PID back
		if pid != stalePID {
			t.Errorf("CheckPIDFile returned wrong PID: got %d, want %d", pid, stalePID)
		}
		t.Skipf("Process %d exists (unexpected but valid), skipping stale PID test", stalePID)
	} else {
		// Process doesn't exist (expected)
		if pid != stalePID {
			t.Errorf("CheckPIDFile returned wrong PID: got %d, want %d", pid, stalePID)
		}
	}
}

// TestCheckPIDFilePermissionDenied tests checking PID for a process we can't signal.
// This is tricky to test portably, so we'll skip if we can't set up the test.
func TestCheckPIDFilePermissionDenied(t *testing.T) {
	// Get PID 1 (init/systemd) which we typically can't signal unless root
	if os.Getuid() == 0 {
		t.Skip("Running as root, can signal all processes")
	}

	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "pid1.pid")

	// Write PID 1
	err := WritePIDFile(pidFile, 1)
	if err != nil {
		t.Fatalf("WritePIDFile failed: %v", err)
	}

	// Check PID file
	running, pid, err := CheckPIDFile(pidFile)
	if err != nil {
		// On some systems we might get an error
		t.Logf("CheckPIDFile returned error for PID 1: %v (this is acceptable)", err)
	}

	// Should still identify the PID
	if pid != 1 {
		t.Errorf("CheckPIDFile returned wrong PID: got %d, want 1", pid)
	}

	// Process should be detected as running (even if we got EPERM)
	// because the error handling treats EPERM as "process exists"
	if !running {
		t.Errorf("CheckPIDFile should detect PID 1 as running")
	}
}

// TestWriteDaemonInfo tests daemon info file creation.
func TestWriteDaemonInfo(t *testing.T) {
	tmpDir := t.TempDir()
	infoFile := filepath.Join(tmpDir, "daemon.json")

	// Create daemon info
	info := &DaemonInfo{
		PID:         12345,
		StartTime:   time.Now(),
		GRPCAddress: "localhost:50002",
		Version:     "1.0.0",
	}

	// Write daemon info
	err := WriteDaemonInfo(infoFile, info)
	if err != nil {
		t.Fatalf("WriteDaemonInfo failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(infoFile); os.IsNotExist(err) {
		t.Fatalf("Daemon info file was not created")
	}

	// Verify file permissions (0600)
	fileInfo, err := os.Stat(infoFile)
	if err != nil {
		t.Fatalf("Failed to stat daemon info file: %v", err)
	}
	mode := fileInfo.Mode()
	if mode.Perm() != 0600 {
		t.Errorf("Daemon info file has wrong permissions: got %o, want 0600", mode.Perm())
	}

	// Read back and verify
	readInfo, err := ReadDaemonInfo(infoFile)
	if err != nil {
		t.Fatalf("ReadDaemonInfo failed: %v", err)
	}

	if readInfo.PID != info.PID {
		t.Errorf("PID mismatch: got %d, want %d", readInfo.PID, info.PID)
	}
	if readInfo.GRPCAddress != info.GRPCAddress {
		t.Errorf("GRPCAddress mismatch: got %s, want %s", readInfo.GRPCAddress, info.GRPCAddress)
	}
	if readInfo.Version != info.Version {
		t.Errorf("Version mismatch: got %s, want %s", readInfo.Version, info.Version)
	}

	// Verify timestamps are close (within 1 second)
	timeDiff := readInfo.StartTime.Sub(info.StartTime)
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}
	if timeDiff > time.Second {
		t.Errorf("StartTime difference too large: %v", timeDiff)
	}
}

// TestWriteDaemonInfoNil tests writing nil daemon info (should error).
func TestWriteDaemonInfoNil(t *testing.T) {
	tmpDir := t.TempDir()
	infoFile := filepath.Join(tmpDir, "daemon.json")

	err := WriteDaemonInfo(infoFile, nil)
	if err == nil {
		t.Errorf("WriteDaemonInfo should error for nil info")
	}
}

// TestWriteDaemonInfoCreatesDirectory tests that parent directory is created.
func TestWriteDaemonInfoCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	infoFile := filepath.Join(tmpDir, "subdir", "nested", "daemon.json")

	info := &DaemonInfo{
		PID:         12345,
		StartTime:   time.Now(),
		GRPCAddress: "localhost:50002",
		Version:     "1.0.0",
	}

	// Write daemon info (should create parent directories)
	err := WriteDaemonInfo(infoFile, info)
	if err != nil {
		t.Fatalf("WriteDaemonInfo failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(infoFile); os.IsNotExist(err) {
		t.Fatalf("Daemon info file was not created")
	}
}

// TestReadDaemonInfoNotExists tests reading non-existent daemon info file.
func TestReadDaemonInfoNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	infoFile := filepath.Join(tmpDir, "nonexistent.json")

	_, err := ReadDaemonInfo(infoFile)
	if err == nil {
		t.Errorf("ReadDaemonInfo should error for missing file")
	}
	// Use errors.Is to check wrapped errors
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ReadDaemonInfo should return NotExist error, got: %v", err)
	}
}

// TestReadDaemonInfoInvalid tests reading daemon info file with invalid JSON.
func TestReadDaemonInfoInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	infoFile := filepath.Join(tmpDir, "invalid.json")

	// Write invalid JSON
	err := os.WriteFile(infoFile, []byte("not valid json"), 0600)
	if err != nil {
		t.Fatalf("Failed to write invalid JSON: %v", err)
	}

	_, err = ReadDaemonInfo(infoFile)
	if err == nil {
		t.Errorf("ReadDaemonInfo should error for invalid JSON")
	}
}

// TestReadDaemonInfoInvalidPID tests reading daemon info with invalid PID.
func TestReadDaemonInfoInvalidPID(t *testing.T) {
	tmpDir := t.TempDir()
	infoFile := filepath.Join(tmpDir, "invalid_pid.json")

	// Write JSON with invalid PID
	err := os.WriteFile(infoFile, []byte(`{"pid":0,"start_time":"2024-01-01T00:00:00Z","grpc_address":"localhost:50002","version":"1.0.0"}`), 0600)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err = ReadDaemonInfo(infoFile)
	if err == nil {
		t.Errorf("ReadDaemonInfo should error for PID=0")
	}
}

// TestReadDaemonInfoMissingGRPCAddress tests reading daemon info without gRPC address.
func TestReadDaemonInfoMissingGRPCAddress(t *testing.T) {
	tmpDir := t.TempDir()
	infoFile := filepath.Join(tmpDir, "no_grpc.json")

	// Write JSON without gRPC address
	err := os.WriteFile(infoFile, []byte(`{"pid":12345,"start_time":"2024-01-01T00:00:00Z","version":"1.0.0"}`), 0600)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err = ReadDaemonInfo(infoFile)
	if err == nil {
		t.Errorf("ReadDaemonInfo should error for missing gRPC address")
	}
}

// TestRemoveDaemonInfo tests daemon info file removal.
func TestRemoveDaemonInfo(t *testing.T) {
	tmpDir := t.TempDir()
	infoFile := filepath.Join(tmpDir, "daemon.json")

	// Create daemon info
	info := &DaemonInfo{
		PID:         12345,
		StartTime:   time.Now(),
		GRPCAddress: "localhost:50002",
		Version:     "1.0.0",
	}
	err := WriteDaemonInfo(infoFile, info)
	if err != nil {
		t.Fatalf("WriteDaemonInfo failed: %v", err)
	}

	// Remove daemon info
	err = RemoveDaemonInfo(infoFile)
	if err != nil {
		t.Fatalf("RemoveDaemonInfo failed: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(infoFile); !os.IsNotExist(err) {
		t.Errorf("Daemon info file still exists after removal")
	}
}

// TestRemoveDaemonInfoNotExists tests removing non-existent daemon info file (should be no-op).
func TestRemoveDaemonInfoNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	infoFile := filepath.Join(tmpDir, "nonexistent.json")

	// Remove non-existent file (should not error)
	err := RemoveDaemonInfo(infoFile)
	if err != nil {
		t.Errorf("RemoveDaemonInfo should not error for missing file: %v", err)
	}
}

// TestFormatDuration tests duration formatting.
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"seconds only", 45 * time.Second, "45s"},
		{"minutes and seconds", 2*time.Minute + 15*time.Second, "2m 15s"},
		{"hours minutes seconds", 1*time.Hour + 30*time.Minute + 45*time.Second, "1h 30m 45s"},
		{"exactly 1 hour", 1 * time.Hour, "1h 0m 0s"},
		{"sub-second rounds", 500 * time.Millisecond, "1s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

// TestPIDFileRace tests concurrent PID file operations.
func TestPIDFileRace(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "race.pid")

	// Multiple goroutines trying to write/read/check PID file
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			// Write
			WritePIDFile(pidFile, os.Getpid())
			// Read
			ReadPIDFile(pidFile)
			// Check
			CheckPIDFile(pidFile)
			// Remove
			if id%2 == 0 {
				RemovePIDFile(pidFile)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// No assertion - just checking we don't panic or deadlock
}

// TestDaemonInfoRace tests concurrent daemon info operations.
func TestDaemonInfoRace(t *testing.T) {
	tmpDir := t.TempDir()
	infoFile := filepath.Join(tmpDir, "race.json")

	info := &DaemonInfo{
		PID:         12345,
		StartTime:   time.Now(),
		GRPCAddress: "localhost:50002",
		Version:     "1.0.0",
	}

	// Multiple goroutines trying to write/read daemon info
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			// Write
			WriteDaemonInfo(infoFile, info)
			// Read
			ReadDaemonInfo(infoFile)
			// Remove
			if id%2 == 0 {
				RemoveDaemonInfo(infoFile)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// No assertion - just checking we don't panic or deadlock
}

// TestCheckProcessExists is a helper to verify our syscall.Kill logic.
func TestCheckProcessExists(t *testing.T) {
	// Test with current process (should exist)
	currentPID := os.Getpid()
	err := syscall.Kill(currentPID, 0)
	if err != nil {
		t.Errorf("Current process should exist but got error: %v", err)
	}

	// Test with PID 1 (init/systemd, should exist)
	err = syscall.Kill(1, 0)
	if err != nil && err != syscall.EPERM {
		t.Errorf("PID 1 should exist but got unexpected error: %v", err)
	}

	// Test with very high PID (should not exist)
	err = syscall.Kill(999999, 0)
	if err != syscall.ESRCH {
		t.Logf("High PID check returned: %v (expected ESRCH)", err)
	}
}
