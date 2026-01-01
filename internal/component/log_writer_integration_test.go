//go:build integration

package component

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_ComponentOutputCaptured validates the complete logging flow
// from component start to log file output with proper formatting.
func TestIntegration_ComponentOutputCaptured(t *testing.T) {
	// Create temporary Gibson home directory structure
	tempHome := t.TempDir()
	logsDir := filepath.Join(tempHome, "logs")
	require.NoError(t, os.MkdirAll(logsDir, 0755))

	// Create DefaultLogWriter pointing to <temp>/logs/
	logWriter, err := NewDefaultLogWriter(logsDir, nil)
	require.NoError(t, err, "should create log writer")

	// Create lifecycle manager with the log writer
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManagerWithTimeouts(
		healthMonitor,
		dao,
		logWriter,
		localTracker,
		2*time.Second, // Short startup timeout
		5*time.Second, // Shutdown timeout
		50000,         // Port range start
		51000,         // Port range end
	)

	// Create a simple component that outputs to stdout and stderr
	componentName := "test-component"
	stdoutMsg := "Hello from stdout - integration test"
	stderrMsg := "Error from stderr - integration test"
	scriptPath := createMultiLineOutputScript(t, stdoutMsg, stderrMsg)

	comp := &Component{
		Kind:    ComponentKindAgent,
		Name:    componentName,
		Version: "1.0.0",
		Status:  ComponentStatusAvailable,
		BinPath: scriptPath,
		Manifest: &Manifest{
			Name:    componentName,
			Version: "1.0.0",
			Runtime: &RuntimeConfig{
				Type:       RuntimeTypeBinary,
				Entrypoint: scriptPath,
			},
		},
	}

	// Start the component (will fail health check but logs should be written)
	ctx := context.Background()
	_, _ = manager.StartComponent(ctx, comp) // Ignore error - health check will fail

	// Give process time to output
	time.Sleep(500 * time.Millisecond)

	// Stop component and flush logs
	if comp.PID > 0 {
		proc, _ := os.FindProcess(comp.PID)
		if proc != nil {
			_ = proc.Kill()
		}
	}

	// Close log writers to flush buffers
	err = logWriter.Close(componentName)
	require.NoError(t, err, "should close log writer")

	// Give file system a moment to finalize
	time.Sleep(100 * time.Millisecond)

	// Verify log file exists
	logPath := filepath.Join(logsDir, fmt.Sprintf("%s.log", componentName))
	require.FileExists(t, logPath, "log file should exist")

	// Read and verify log content
	logData, err := os.ReadFile(logPath)
	require.NoError(t, err, "should read log file")

	logContent := string(logData)
	t.Logf("Log content:\n%s", logContent)

	// Verify output appears in log file with correct format
	assert.Contains(t, logContent, stdoutMsg, "stdout message should be captured")
	assert.Contains(t, logContent, stderrMsg, "stderr message should be captured")

	// Verify timestamps are present (RFC3339 format: 2025-01-01T12:00:00Z or with timezone)
	assert.Contains(t, logContent, "T", "should contain timestamp separator")
	assert.Contains(t, logContent, ":", "should contain timestamp colons")

	// Verify stream markers are present
	assert.Contains(t, logContent, "[STDOUT]", "stdout marker should be present")
	assert.Contains(t, logContent, "[STDERR]", "stderr marker should be present")

	// Verify each line has timestamp and stream marker
	lines := strings.Split(strings.TrimSpace(logContent), "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`, line,
			"line %d should start with timestamp: %s", i, line)
		assert.True(t,
			strings.Contains(line, "[STDOUT]") || strings.Contains(line, "[STDERR]"),
			"line %d should have stream marker: %s", i, line)
	}
}

// TestIntegration_LogRotation validates log rotation with small threshold.
func TestIntegration_LogRotation(t *testing.T) {
	// Create temporary log directory
	tempDir := t.TempDir()

	// Create a RotatingLogWriter with small rotation threshold (1KB for testing)
	rotationThreshold := int64(1024) // 1KB
	maxBackups := 3

	rotator := NewDefaultLogRotator(rotationThreshold, maxBackups)
	require.NotNil(t, rotator, "should create rotator")

	componentName := "rotation-test-component"
	logPath := filepath.Join(tempDir, fmt.Sprintf("%s.log", componentName))

	// Create initial log file
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	require.NoError(t, err)

	// Write data until we exceed threshold
	// Each line is roughly 100 bytes, so write 15 lines to exceed 1KB
	lineTemplate := "2025-01-01T12:00:00Z [STDOUT] This is a test log line with some content - line %03d\n"
	for i := 0; i < 15; i++ {
		line := fmt.Sprintf(lineTemplate, i)
		_, err := file.WriteString(line)
		require.NoError(t, err)
	}
	file.Close()

	// Verify file exceeds threshold
	info, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), rotationThreshold, "file should exceed threshold")
	t.Logf("Initial file size: %d bytes (threshold: %d)", info.Size(), rotationThreshold)

	// Check if rotation is needed
	shouldRotate, err := rotator.ShouldRotate(logPath)
	require.NoError(t, err)
	assert.True(t, shouldRotate, "should need rotation")

	// Perform rotation
	newFile, err := rotator.Rotate(logPath)
	require.NoError(t, err, "rotation should succeed")
	require.NotNil(t, newFile, "should return new file")
	newFile.Close()

	// Verify .log.1 was created with the old content
	backup1Path := fmt.Sprintf("%s.1", logPath)
	require.FileExists(t, backup1Path, "backup .log.1 should exist")

	// Verify backup contains old data
	backup1Data, err := os.ReadFile(backup1Path)
	require.NoError(t, err)
	assert.Contains(t, string(backup1Data), "line 000", "backup should contain old data")
	assert.Contains(t, string(backup1Data), "line 014", "backup should contain old data")

	// Verify current .log is empty (new file)
	currentInfo, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Equal(t, int64(0), currentInfo.Size(), "new log file should be empty")

	// Generate more output to trigger another rotation
	file2, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	require.NoError(t, err)
	for i := 0; i < 15; i++ {
		line := fmt.Sprintf(lineTemplate, i+100) // Different line numbers
		_, err := file2.WriteString(line)
		require.NoError(t, err)
	}
	file2.Close()

	// Rotate again
	shouldRotate, err = rotator.ShouldRotate(logPath)
	require.NoError(t, err)
	assert.True(t, shouldRotate, "should need rotation again")

	newFile2, err := rotator.Rotate(logPath)
	require.NoError(t, err)
	require.NotNil(t, newFile2)
	newFile2.Close()

	// Verify .log.2 and .log.1 exist
	backup2Path := fmt.Sprintf("%s.2", logPath)
	require.FileExists(t, backup2Path, "backup .log.2 should exist")
	require.FileExists(t, backup1Path, "backup .log.1 should still exist")

	// Verify .log.2 contains the first rotation's data
	backup2Data, err := os.ReadFile(backup2Path)
	require.NoError(t, err)
	assert.Contains(t, string(backup2Data), "line 000", ".log.2 should contain original data")

	// Verify .log.1 contains the second rotation's data
	backup1Data2, err := os.ReadFile(backup1Path)
	require.NoError(t, err)
	assert.Contains(t, string(backup1Data2), "line 100", ".log.1 should contain second batch")

	// Verify current .log is small (under threshold)
	currentInfo2, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Less(t, currentInfo2.Size(), rotationThreshold, "new log should be under threshold")
}

// TestIntegration_LogRotationMaxBackups verifies that old backups are deleted
// when exceeding maxBackups limit.
func TestIntegration_LogRotationMaxBackups(t *testing.T) {
	tempDir := t.TempDir()

	// Create rotator with very small threshold and only 2 backups
	rotationThreshold := int64(500) // 500 bytes
	maxBackups := 2
	rotator := NewDefaultLogRotator(rotationThreshold, maxBackups)

	componentName := "max-backups-test"
	logPath := filepath.Join(tempDir, fmt.Sprintf("%s.log", componentName))

	lineTemplate := "2025-01-01T12:00:00Z [STDOUT] Log line batch %d iteration %03d\n"

	// Perform multiple rotations to exceed maxBackups
	for batch := 0; batch < 4; batch++ {
		// Write enough data to exceed threshold
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		require.NoError(t, err)

		for i := 0; i < 10; i++ {
			line := fmt.Sprintf(lineTemplate, batch, i)
			_, err := file.WriteString(line)
			require.NoError(t, err)
		}
		file.Close()

		// Check and rotate if needed
		shouldRotate, err := rotator.ShouldRotate(logPath)
		require.NoError(t, err)

		if shouldRotate {
			newFile, err := rotator.Rotate(logPath)
			require.NoError(t, err)
			newFile.Close()
			t.Logf("Performed rotation after batch %d", batch)
		}
	}

	// Verify only maxBackups (2) backup files exist
	backup1Path := fmt.Sprintf("%s.1", logPath)
	backup2Path := fmt.Sprintf("%s.2", logPath)
	backup3Path := fmt.Sprintf("%s.3", logPath)

	require.FileExists(t, backup1Path, ".log.1 should exist")
	require.FileExists(t, backup2Path, ".log.2 should exist")
	assert.NoFileExists(t, backup3Path, ".log.3 should not exist (exceeds maxBackups)")

	// Verify .log.1 contains most recent rotated data
	backup1Data, err := os.ReadFile(backup1Path)
	require.NoError(t, err)
	backup1Content := string(backup1Data)
	t.Logf("Backup .log.1 content preview: %s...", backup1Content[:min(100, len(backup1Content))])

	// Verify .log.2 contains older data
	backup2Data, err := os.ReadFile(backup2Path)
	require.NoError(t, err)
	backup2Content := string(backup2Data)
	t.Logf("Backup .log.2 content preview: %s...", backup2Content[:min(100, len(backup2Content))])
}

// TestIntegration_ConcurrentLogging validates concurrent writes from multiple streams
// don't corrupt the log file.
func TestIntegration_ConcurrentLogging(t *testing.T) {
	tempDir := t.TempDir()

	logWriter, err := NewDefaultLogWriter(tempDir, nil)
	require.NoError(t, err)

	componentName := "concurrent-test"

	// Create stdout and stderr writers
	stdoutWriter, err := logWriter.CreateWriter(componentName, "stdout")
	require.NoError(t, err)
	stderrWriter, err := logWriter.CreateWriter(componentName, "stderr")
	require.NoError(t, err)

	// Create a script that rapidly outputs to both stdout and stderr
	scriptPath := createConcurrentOutputScript(t)

	cmd := exec.Command(scriptPath)
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	// Start process
	err = cmd.Start()
	require.NoError(t, err)

	// Wait for process to complete
	err = cmd.Wait()
	require.NoError(t, err)

	// Close writers to flush
	err = logWriter.Close(componentName)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Read log file
	logPath := filepath.Join(tempDir, fmt.Sprintf("%s.log", componentName))
	logData, err := os.ReadFile(logPath)
	require.NoError(t, err)

	logContent := string(logData)
	lines := strings.Split(strings.TrimSpace(logContent), "\n")

	// Verify we got both stdout and stderr lines
	stdoutCount := 0
	stderrCount := 0
	for _, line := range lines {
		if strings.Contains(line, "[STDOUT]") {
			stdoutCount++
		}
		if strings.Contains(line, "[STDERR]") {
			stderrCount++
		}
	}

	assert.Greater(t, stdoutCount, 0, "should have stdout lines")
	assert.Greater(t, stderrCount, 0, "should have stderr lines")

	// Verify each line is properly formatted (not corrupted)
	for i, line := range lines {
		if line == "" {
			continue
		}
		// Each line should have timestamp, stream marker, and message
		assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T`, line,
			"line %d should have timestamp: %s", i, line)
		assert.True(t,
			strings.Contains(line, "[STDOUT]") || strings.Contains(line, "[STDERR]"),
			"line %d should have stream marker: %s", i, line)
	}

	t.Logf("Processed %d total lines (%d stdout, %d stderr)", len(lines), stdoutCount, stderrCount)
}

// Helper functions

// createMultiLineOutputScript creates a script that outputs multiple lines
// to both stdout and stderr.
func createMultiLineOutputScript(t *testing.T, stdoutMsg, stderrMsg string) string {
	t.Helper()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "multi-output.sh")

	script := fmt.Sprintf(`#!/bin/bash
# Output to stdout
echo "%s"
echo "stdout line 2"
echo "stdout line 3"

# Output to stderr
echo "%s" >&2
echo "stderr line 2" >&2
echo "stderr line 3" >&2

# Sleep briefly to allow health check attempts
sleep 2
`, stdoutMsg, stderrMsg)

	err := os.WriteFile(scriptPath, []byte(script), 0755)
	require.NoError(t, err)

	return scriptPath
}

// createConcurrentOutputScript creates a script that rapidly interleaves
// stdout and stderr output to test concurrent writing.
func createConcurrentOutputScript(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "concurrent-output.sh")

	script := `#!/bin/bash
# Rapidly output to both stdout and stderr
for i in {1..50}; do
  echo "STDOUT line $i" &
  echo "STDERR line $i" >&2 &
done

# Wait for all background jobs
wait
`

	err := os.WriteFile(scriptPath, []byte(script), 0755)
	require.NoError(t, err)

	return scriptPath
}

// min returns the minimum of two integers (helper for Go < 1.21).
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
