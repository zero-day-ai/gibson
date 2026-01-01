//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/database"
	initpkg "github.com/zero-day-ai/gibson/internal/init"
	"golang.org/x/sys/unix"
)

// setupAttackIntegrationTest creates a complete test environment for attack command testing
func setupAttackIntegrationTest(t *testing.T) (homeDir string, cleanup func()) {
	t.Helper()

	// Create temp directory for test home
	tempDir, err := os.MkdirTemp("", "gibson-attack-integration-*")
	require.NoError(t, err, "Failed to create temp directory")

	// Initialize Gibson home directory
	initializer := initpkg.NewDefaultInitializer()
	opts := initpkg.InitOptions{
		HomeDir:        tempDir,
		NonInteractive: true,
		Force:          false,
	}

	_, err = initializer.Initialize(context.Background(), opts)
	require.NoError(t, err, "Failed to initialize Gibson home")

	// Set GIBSON_HOME environment variable for child processes
	oldHome := os.Getenv("GIBSON_HOME")

	cleanup = func() {
		os.RemoveAll(tempDir)
		if oldHome != "" {
			os.Setenv("GIBSON_HOME", oldHome)
		} else {
			os.Unsetenv("GIBSON_HOME")
		}
	}

	return tempDir, cleanup
}

// createMockTargetServer creates an HTTP test server that simulates a target
func createMockTargetServer(t *testing.T) *httptest.Server {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simple mock response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := map[string]string{
			"status":  "ok",
			"message": "Mock target response",
		}
		json.NewEncoder(w).Encode(response)
	})

	return httptest.NewServer(handler)
}

// runGibsonCommand executes the gibson command with the given arguments
func runGibsonCommand(t *testing.T, homeDir string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	// Build the command - use go run to avoid needing to build binary
	cmdArgs := append([]string{"run", "-tags", "fts5", "."}, args...)
	cmd := exec.Command("go", cmdArgs...)

	// Set environment
	cmd.Env = append(os.Environ(), "GIBSON_HOME="+homeDir)
	cmd.Dir = filepath.Join(os.Getenv("PWD"))

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Run with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := cmd.Start()
	require.NoError(t, err, "Failed to start command")

	// Wait for command to complete or timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		} else {
			exitCode = 0
		}
	case <-ctx.Done():
		cmd.Process.Kill()
		require.Fail(t, "Command timed out after 30 seconds")
	}

	return stdoutBuf.String(), stderrBuf.String(), exitCode
}

// TestAttackCLI_DryRun tests the --dry-run flag
func TestAttackCLI_DryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupAttackIntegrationTest(t)
	defer cleanup()

	server := createMockTargetServer(t)
	defer server.Close()

	stdout, stderr, exitCode := runGibsonCommand(t, homeDir,
		"attack", server.URL,
		"--agent", "test-agent",
		"--dry-run",
	)

	// Combine stdout and stderr for checking (cobra may write to both)
	output := stdout + stderr

	// Should succeed with exit code 0
	assert.Equal(t, 0, exitCode, "Dry-run should exit with code 0, stderr: %s", stderr)

	// Verify output contains expected dry-run content
	assert.Contains(t, output, "Dry-run mode", "Output should indicate dry-run mode")
	assert.Contains(t, output, "Attack Configuration", "Output should show configuration")
	assert.Contains(t, output, server.URL, "Output should contain target URL")
	assert.Contains(t, output, "test-agent", "Output should contain agent name")
	assert.Contains(t, output, "Configuration is valid", "Output should confirm valid config")
	assert.Contains(t, output, "Use without --dry-run to execute", "Output should show next steps")

	// Should NOT actually execute the attack
	assert.NotContains(t, output, "Gibson Attack", "Should not show attack execution banner")
}

// TestAttackCLI_DryRunWithAllOptions tests dry-run with comprehensive options
func TestAttackCLI_DryRunWithAllOptions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupAttackIntegrationTest(t)
	defer cleanup()

	server := createMockTargetServer(t)
	defer server.Close()

	stdout, stderr, exitCode := runGibsonCommand(t, homeDir,
		"attack", server.URL,
		"--agent", "test-agent",
		"--goal", "Test goal description",
		"--max-turns", "10",
		"--timeout", "5m",
		"--category", "injection",
		"--max-findings", "5",
		"--severity-threshold", "medium",
		"--rate-limit", "10",
		"--insecure",
		"--proxy", "http://proxy:8080",
		"--output", "text",
		"--verbose",
		"--dry-run",
	)

	output := stdout + stderr
	assert.Equal(t, 0, exitCode, "Dry-run should exit with code 0, stderr: %s", stderr)

	// Verify all options are displayed
	assert.Contains(t, output, "Goal:", "Should show goal")
	assert.Contains(t, output, "Max Turns:", "Should show max turns")
	assert.Contains(t, output, "10", "Should show max turns value")
	assert.Contains(t, output, "Timeout:", "Should show timeout")
	assert.Contains(t, output, "5m", "Should show timeout value")
	assert.Contains(t, output, "Category:", "Should show payload category")
	assert.Contains(t, output, "injection", "Should show category value")
	assert.Contains(t, output, "Max Findings:", "Should show max findings")
	assert.Contains(t, output, "Min Severity:", "Should show severity threshold")
	assert.Contains(t, output, "medium", "Should show severity value")
	assert.Contains(t, output, "Rate Limit:", "Should show rate limit")
	assert.Contains(t, output, "Proxy:", "Should show proxy")
	assert.Contains(t, output, "proxy:8080", "Should show proxy URL")
	assert.Contains(t, output, "INSECURE", "Should show insecure TLS warning")
}

// TestAttackCLI_ListAgents tests the --list-agents flag
func TestAttackCLI_ListAgents(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupAttackIntegrationTest(t)
	defer cleanup()

	stdout, stderr, exitCode := runGibsonCommand(t, homeDir,
		"attack", "--list-agents",
	)

	output := stdout + stderr

	// Should succeed
	assert.Equal(t, 0, exitCode, "List agents should exit with code 0, stderr: %s", stderr)

	// Verify output contains agent listing (note: placeholder until registry is wired up)
	assert.Contains(t, output, "placeholder", "Output should show placeholder content")
}

// TestAttackCLI_ListAgentsJSON tests --list-agents with JSON output
func TestAttackCLI_ListAgentsJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupAttackIntegrationTest(t)
	defer cleanup()

	stdout, stderr, exitCode := runGibsonCommand(t, homeDir,
		"attack", "--list-agents", "--output", "json",
	)

	// Should succeed
	assert.Equal(t, 0, exitCode, "List agents should exit with code 0, stderr: %s", stderr)

	// Verify JSON output is valid
	var result map[string]interface{}
	err := json.Unmarshal([]byte(stdout), &result)
	require.NoError(t, err, "Output should be valid JSON")

	// Verify structure
	agents, ok := result["agents"]
	assert.True(t, ok, "JSON should contain 'agents' field")
	assert.NotNil(t, agents, "Agents field should not be nil")
}

// TestAttackCLI_MissingAgent tests error when no agent is specified
func TestAttackCLI_MissingAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupAttackIntegrationTest(t)
	defer cleanup()

	server := createMockTargetServer(t)
	defer server.Close()

	stdout, stderr, exitCode := runGibsonCommand(t, homeDir,
		"attack", server.URL,
		// Missing --agent flag
	)

	output := stdout + stderr

	// Should fail with config error exit code (10)
	assert.NotEqual(t, 0, exitCode, "Should fail when agent not specified")

	// Verify error message indicates configuration error
	assert.Contains(t, strings.ToLower(output), "configuration", "Error should indicate configuration error")
}

// TestAttackCLI_MissingURL tests error when no URL is provided
func TestAttackCLI_MissingURL(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupAttackIntegrationTest(t)
	defer cleanup()

	stdout, stderr, exitCode := runGibsonCommand(t, homeDir,
		"attack",
		"--agent", "test-agent",
		// Missing URL argument
	)

	output := stdout + stderr

	// Should fail
	assert.NotEqual(t, 0, exitCode, "Should fail when URL not provided")

	// Verify error message mentions URL/target
	assert.True(t,
		strings.Contains(strings.ToLower(output), "url") ||
			strings.Contains(strings.ToLower(output), "target"),
		"Error should mention URL or target requirement, got: %s", output)
}

// TestAttackCLI_InvalidOutputFormat tests error with invalid output format
func TestAttackCLI_InvalidOutputFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupAttackIntegrationTest(t)
	defer cleanup()

	server := createMockTargetServer(t)
	defer server.Close()

	stdout, stderr, exitCode := runGibsonCommand(t, homeDir,
		"attack", server.URL,
		"--agent", "test-agent",
		"--output", "invalid-format",
		"--dry-run", // Use dry-run to avoid actual execution
	)

	output := stdout + stderr

	// Should fail with config error
	assert.NotEqual(t, 0, exitCode, "Should fail with invalid output format")

	// Verify error message indicates configuration error
	assert.Contains(t, strings.ToLower(output), "configuration", "Error should indicate configuration error")
}

// TestAttackCLI_ConflictingFlags tests error with conflicting flags
func TestAttackCLI_ConflictingFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupAttackIntegrationTest(t)
	defer cleanup()

	server := createMockTargetServer(t)
	defer server.Close()

	// Test conflicting persist flags
	t.Run("conflicting persist flags", func(t *testing.T) {
		stdout, stderr, exitCode := runGibsonCommand(t, homeDir,
			"attack", server.URL,
			"--agent", "test-agent",
			"--persist",
			"--no-persist",
			"--dry-run",
		)

		output := stdout + stderr
		assert.NotEqual(t, 0, exitCode, "Should fail with conflicting persist flags")
		assert.Contains(t, strings.ToLower(output), "configuration", "Error should indicate configuration error")
	})

	// Test conflicting verbose/quiet flags
	t.Run("conflicting verbose/quiet flags", func(t *testing.T) {
		stdout, stderr, exitCode := runGibsonCommand(t, homeDir,
			"attack", server.URL,
			"--agent", "test-agent",
			"--verbose",
			"--quiet",
			"--dry-run",
		)

		output := stdout + stderr
		assert.NotEqual(t, 0, exitCode, "Should fail with conflicting verbose/quiet flags")
		assert.Contains(t, strings.ToLower(output), "configuration", "Error should indicate configuration error")
	})
}

// TestAttackCLI_InvalidTimeout tests error with invalid timeout format
func TestAttackCLI_InvalidTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupAttackIntegrationTest(t)
	defer cleanup()

	server := createMockTargetServer(t)
	defer server.Close()

	stdout, stderr, exitCode := runGibsonCommand(t, homeDir,
		"attack", server.URL,
		"--agent", "test-agent",
		"--timeout", "invalid-duration",
		"--dry-run",
	)

	output := stdout + stderr

	// Should fail with config error
	assert.NotEqual(t, 0, exitCode, "Should fail with invalid timeout")

	// Verify error message indicates build/config error
	assert.True(t,
		strings.Contains(strings.ToLower(output), "build") ||
			strings.Contains(strings.ToLower(output), "options") ||
			strings.Contains(strings.ToLower(output), "configuration"),
		"Error should indicate build or configuration error")
}

// TestAttackCLI_InvalidHeaders tests error with malformed JSON headers
func TestAttackCLI_InvalidHeaders(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupAttackIntegrationTest(t)
	defer cleanup()

	server := createMockTargetServer(t)
	defer server.Close()

	stdout, stderr, exitCode := runGibsonCommand(t, homeDir,
		"attack", server.URL,
		"--agent", "test-agent",
		"--headers", "{invalid-json}",
		"--dry-run",
	)

	output := stdout + stderr

	// Should fail with config error
	assert.NotEqual(t, 0, exitCode, "Should fail with invalid JSON headers")

	// Verify error message indicates build/config error
	assert.True(t,
		strings.Contains(strings.ToLower(output), "build") ||
			strings.Contains(strings.ToLower(output), "options") ||
			strings.Contains(strings.ToLower(output), "configuration"),
		"Error should indicate build or configuration error")
}

// TestAttackCLI_DatabaseAccess tests that attack command can access the database
func TestAttackCLI_DatabaseAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupAttackIntegrationTest(t)
	defer cleanup()

	// Verify database exists and is accessible
	dbPath := filepath.Join(homeDir, "gibson.db")
	assert.FileExists(t, dbPath, "Database should exist after initialization")

	// Open database and verify structure
	db, err := database.Open(dbPath)
	require.NoError(t, err, "Should be able to open database")
	defer db.Close()

	// Verify required tables exist
	tables := []string{"missions", "findings", "targets", "credentials"}
	for _, table := range tables {
		var exists bool
		err := db.QueryRow(`
			SELECT COUNT(*) > 0
			FROM sqlite_master
			WHERE type='table' AND name=?
		`, table).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "Table %s should exist", table)
	}
}

// TestAttackCLI_HelpText tests that help text is available
func TestAttackCLI_HelpText(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupAttackIntegrationTest(t)
	defer cleanup()

	stdout, stderr, exitCode := runGibsonCommand(t, homeDir,
		"attack", "--help",
	)

	output := stdout + stderr

	// Help should exit successfully
	assert.Equal(t, 0, exitCode, "Help should exit with code 0, stderr: %s", stderr)

	// Verify help text contains expected content
	assert.Contains(t, output, "attack", "Help should mention attack command")
	assert.Contains(t, output, "--agent", "Help should document --agent flag")
	assert.Contains(t, output, "--dry-run", "Help should document --dry-run flag")
	assert.Contains(t, output, "--list-agents", "Help should document --list-agents flag")
	assert.Contains(t, output, "Examples:", "Help should include examples")
}

// TestAttackCLI_ValidConfiguration tests a complete valid configuration
func TestAttackCLI_ValidConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupAttackIntegrationTest(t)
	defer cleanup()

	server := createMockTargetServer(t)
	defer server.Close()

	// Test a comprehensive valid configuration
	stdout, stderr, exitCode := runGibsonCommand(t, homeDir,
		"attack", server.URL,
		"--agent", "test-agent",
		"--goal", "Comprehensive security test",
		"--max-turns", "15",
		"--timeout", "10m",
		"--payloads", "payload-1,payload-2",
		"--category", "injection",
		"--techniques", "T1059,T1190",
		"--max-findings", "10",
		"--severity-threshold", "low",
		"--rate-limit", "5",
		"--no-follow-redirects",
		"--persist",
		"--output", "text",
		"--verbose",
		"--dry-run",
	)

	output := stdout + stderr

	// Should succeed with valid configuration
	assert.Equal(t, 0, exitCode, "Valid configuration should be accepted, stderr: %s", stderr)
	assert.Contains(t, output, "Configuration is valid", "Should confirm valid configuration")

	// Verify all settings are reflected in output
	assert.Contains(t, output, server.URL)
	assert.Contains(t, output, "test-agent")
	assert.Contains(t, output, "Comprehensive security test")
	assert.Contains(t, output, "15")
	assert.Contains(t, output, "10m")
	assert.Contains(t, output, "payload-1") // Payloads are formatted with spaces
	assert.Contains(t, output, "payload-2")
	assert.Contains(t, output, "injection")
	assert.Contains(t, output, "T1059") // Techniques are formatted with spaces
	assert.Contains(t, output, "T1190")
	assert.Contains(t, output, "10")
	assert.Contains(t, output, "low")
	assert.Contains(t, output, "5")
	assert.Contains(t, output, "false") // Follow Redirects: false
	assert.Contains(t, output, "always persist")
}

// TestAttackRunner_WithLocalAgent tests attack execution with a local agent
// discovered via UnifiedDiscovery from the filesystem.
func TestAttackRunner_WithLocalAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupAttackIntegrationTest(t)
	defer cleanup()

	// Create a mock local agent by setting up PID file and socket
	agentName := "local-test-agent"
	runDir := filepath.Join(homeDir, "run", "agent")
	require.NoError(t, os.MkdirAll(runDir, 0755))

	// Create PID file
	pidFile := filepath.Join(runDir, agentName+".pid")
	pid := os.Getpid() // Use current process PID for testing
	port := 50051
	pidContent := fmt.Sprintf("%d\n%d\n", pid, port)
	require.NoError(t, os.WriteFile(pidFile, []byte(pidContent), 0600))

	// Create socket file (empty file is fine for testing discovery)
	socketFile := filepath.Join(runDir, agentName+".sock")
	require.NoError(t, os.WriteFile(socketFile, []byte{}, 0600))

	// Create lock file and acquire lock
	lockFile := filepath.Join(runDir, agentName+".lock")
	lockFd, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	defer lockFd.Close()
	require.NoError(t, unix.Flock(int(lockFd.Fd()), unix.LOCK_EX|unix.LOCK_NB))

	// Note: In a real scenario, we would need a running gRPC server,
	// but for this integration test we're verifying discovery works.
	// The attack would fail at execution, but discovery should succeed.

	// Test that attack runner can discover the local agent
	t.Run("local agent is discovered", func(t *testing.T) {
		// Create unified discovery with local tracker
		localTracker := createLocalTrackerForTest(homeDir)
		remoteProber := createEmptyRemoteProber()
		discovery := createUnifiedDiscovery(localTracker, remoteProber)

		// Discover agents
		agents, err := discovery.DiscoverAgents(context.Background())
		require.NoError(t, err, "Should discover local agents")

		// Verify local agent was discovered
		foundLocalAgent := false
		for _, agent := range agents {
			if agent.Name == agentName {
				foundLocalAgent = true
				assert.Equal(t, "local", string(agent.Source), "Agent should be from local source")
				assert.True(t, strings.HasSuffix(agent.Address, ".sock"), "Address should be Unix socket path")
				assert.Equal(t, port, agent.Port, "Port should match PID file")
				assert.True(t, agent.Healthy, "Agent should be healthy")
			}
		}
		assert.True(t, foundLocalAgent, "Local agent %s should be discovered", agentName)
	})

	// Cleanup
	unix.Flock(int(lockFd.Fd()), unix.LOCK_UN)
	os.Remove(pidFile)
	os.Remove(socketFile)
	os.Remove(lockFile)
}

// TestAttackRunner_WithRemoteAgent tests attack execution with a remote agent
// discovered via configuration.
func TestAttackRunner_WithRemoteAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupAttackIntegrationTest(t)
	defer cleanup()

	// Create a mock remote gRPC agent server
	agentName := "remote-test-agent"
	remoteAddr := "localhost:50052"

	// In a real test, we would start a gRPC health server here.
	// For this integration test, we'll mock the RemoteProber to return
	// a healthy remote agent.

	t.Run("remote agent is discovered via config", func(t *testing.T) {
		// Create unified discovery with remote prober configured
		localTracker := createLocalTrackerForTest(homeDir)
		remoteProber := createMockRemoteProberWithAgents(map[string]string{
			agentName: remoteAddr,
		})
		discovery := createUnifiedDiscovery(localTracker, remoteProber)

		// Discover agents
		agents, err := discovery.DiscoverAgents(context.Background())
		require.NoError(t, err, "Should discover remote agents")

		// Verify remote agent was discovered
		foundRemoteAgent := false
		for _, agent := range agents {
			if agent.Name == agentName {
				foundRemoteAgent = true
				assert.Equal(t, "remote", string(agent.Source), "Agent should be from remote source")
				assert.Equal(t, remoteAddr, agent.Address, "Address should match config")
				assert.True(t, agent.Healthy, "Agent should be healthy")
			}
		}
		assert.True(t, foundRemoteAgent, "Remote agent %s should be discovered", agentName)
	})
}

// TestAttackRunner_WithMixedAgents tests attack execution with both local and remote agents,
// verifying that local takes precedence when the same agent exists in both.
func TestAttackRunner_WithMixedAgents(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupAttackIntegrationTest(t)
	defer cleanup()

	// Set up local agent
	localAgentName := "mixed-agent"
	runDir := filepath.Join(homeDir, "run", "agent")
	require.NoError(t, os.MkdirAll(runDir, 0755))

	pidFile := filepath.Join(runDir, localAgentName+".pid")
	pid := os.Getpid()
	localPort := 50053
	pidContent := fmt.Sprintf("%d\n%d\n", pid, localPort)
	require.NoError(t, os.WriteFile(pidFile, []byte(pidContent), 0600))

	socketFile := filepath.Join(runDir, localAgentName+".sock")
	require.NoError(t, os.WriteFile(socketFile, []byte{}, 0600))

	lockFile := filepath.Join(runDir, localAgentName+".lock")
	lockFd, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	defer lockFd.Close()
	require.NoError(t, unix.Flock(int(lockFd.Fd()), unix.LOCK_EX|unix.LOCK_NB))

	// Set up remote agents (including duplicate of local agent)
	remoteAgents := map[string]string{
		localAgentName:      "remote-host:50053", // Same name as local agent
		"remote-only-agent": "remote-host:50054",
	}

	t.Run("mixed agents with local precedence", func(t *testing.T) {
		// Create unified discovery
		localTracker := createLocalTrackerForTest(homeDir)
		remoteProber := createMockRemoteProberWithAgents(remoteAgents)
		discovery := createUnifiedDiscovery(localTracker, remoteProber)

		// Discover all agents
		agents, err := discovery.DiscoverAgents(context.Background())
		require.NoError(t, err, "Should discover mixed agents")

		// Verify agents discovered
		foundLocal := false
		foundRemoteOnly := false
		localCount := 0

		for _, agent := range agents {
			if agent.Name == localAgentName {
				// Local agent should be discovered and have local source
				if agent.Source.String() == "local" {
					foundLocal = true
					localCount++
					assert.Equal(t, localPort, agent.Port, "Should use local port")
					assert.True(t, strings.HasSuffix(agent.Address, ".sock"), "Should use Unix socket")
				} else {
					// Remote version should NOT appear because local takes precedence
					t.Errorf("Found remote version of %s, but local should take precedence", localAgentName)
				}
			}
			if agent.Name == "remote-only-agent" {
				foundRemoteOnly = true
				assert.Equal(t, "remote", agent.Source.String(), "Should be remote source")
			}
		}

		assert.True(t, foundLocal, "Local version of mixed-agent should be discovered")
		assert.Equal(t, 1, localCount, "Should only have one instance of mixed-agent (local)")
		assert.True(t, foundRemoteOnly, "Remote-only agent should be discovered")
		assert.GreaterOrEqual(t, len(agents), 2, "Should discover at least 2 agents")
	})

	// Cleanup
	unix.Flock(int(lockFd.Fd()), unix.LOCK_UN)
	os.Remove(pidFile)
	os.Remove(socketFile)
	os.Remove(lockFile)
}

// Helper functions for component discovery testing

func createLocalTrackerForTest(homeDir string) component.LocalTracker {
	return component.NewDefaultLocalTracker()
}

func createEmptyRemoteProber() component.RemoteProber {
	return &mockRemoteProber{
		components: []component.RemoteComponentState{},
	}
}

func createMockRemoteProberWithAgents(agents map[string]string) component.RemoteProber {
	components := make([]component.RemoteComponentState, 0, len(agents))
	for name, addr := range agents {
		components = append(components, component.RemoteComponentState{
			Kind:    component.ComponentKindAgent,
			Name:    name,
			Address: addr,
			Healthy: true,
		})
	}
	return &mockRemoteProber{
		components: components,
	}
}

func createUnifiedDiscovery(local component.LocalTracker, remote component.RemoteProber) component.UnifiedDiscovery {
	return component.NewDefaultUnifiedDiscovery(local, remote, &mockLogger{})
}

// Mock implementations

type mockLogger struct{}

func (l *mockLogger) Infof(format string, args ...interface{})  {}
func (l *mockLogger) Warnf(format string, args ...interface{})  {}
func (l *mockLogger) Errorf(format string, args ...interface{}) {}
func (l *mockLogger) Debugf(format string, args ...interface{}) {}

type mockRemoteProber struct {
	components []component.RemoteComponentState
}

func (p *mockRemoteProber) Probe(ctx context.Context, address string) (component.RemoteComponentState, error) {
	for _, comp := range p.components {
		if comp.Address == address {
			return comp, nil
		}
	}
	return component.RemoteComponentState{}, fmt.Errorf("component not found at address: %s", address)
}

func (p *mockRemoteProber) ProbeAll(ctx context.Context) ([]component.RemoteComponentState, error) {
	return p.components, nil
}

func (p *mockRemoteProber) LoadConfig(remoteAgents, remoteTools, remotePlugins map[string]component.RemoteComponentConfig) error {
	return nil
}

// Mock attack package types for testing (simplified versions)

type mockAttackOrchestrator struct{}

func newMockAttackOrchestrator(withFindings bool) *mockAttackOrchestrator {
	return &mockAttackOrchestrator{}
}

type mockAttackAgentRegistry struct{}

func newMockAttackAgentRegistry() *mockAttackAgentRegistry {
	return &mockAttackAgentRegistry{}
}

type mockAttackPayloadRegistry struct{}

func newMockAttackPayloadRegistry() *mockAttackPayloadRegistry {
	return &mockAttackPayloadRegistry{}
}

type mockAttackMissionStore struct{}

func newMockAttackMissionStore() *mockAttackMissionStore {
	return &mockAttackMissionStore{}
}

type mockAttackFindingStore struct{}

func newMockAttackFindingStore() *mockAttackFindingStore {
	return &mockAttackFindingStore{}
}
