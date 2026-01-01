//go:build integration

package component

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// TestLocalLifecycleFullStartStop tests the complete lifecycle of starting
// and stopping a component with proper PID, lock, and socket management.
func TestLocalLifecycleFullStartStop(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	// Start a test server
	socketPath, err := GetSocketPath(ComponentKindAgent, "test-agent")
	require.NoError(t, err)

	server := startTestServer(t, socketPath)
	defer server.Stop()

	comp := &Component{
		Kind: ComponentKindAgent,
		Name: "test-agent",
		PID:  os.Getpid(),
		Port: 50051,
	}

	// Track the component lifecycle
	err = tracker.Start(ctx, comp)
	require.NoError(t, err)

	// Verify PID file was written
	pid, port, err := readPIDFile(comp.Kind, comp.Name)
	require.NoError(t, err)
	assert.Equal(t, comp.PID, pid)
	assert.Equal(t, comp.Port, port)

	// Verify lock is held
	locked, err := tracker.isLocked(comp.Kind, comp.Name)
	require.NoError(t, err)
	assert.True(t, locked, "Lock should be held by tracker")

	// Verify socket exists
	info, err := os.Stat(socketPath)
	require.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSocket != 0, "Socket file should exist")

	// Verify component is running
	running, err := tracker.IsRunning(ctx, comp.Kind, comp.Name)
	require.NoError(t, err)
	assert.True(t, running, "Component should be detected as running")

	// Stop the component
	err = tracker.Stop(ctx, comp)
	require.NoError(t, err)

	// Verify PID file was removed
	pidPath, _ := GetPIDFilePath(comp.Kind, comp.Name)
	_, err = os.Stat(pidPath)
	assert.True(t, os.IsNotExist(err), "PID file should be removed")

	// Verify lock was released
	locked, err = tracker.isLocked(comp.Kind, comp.Name)
	require.NoError(t, err)
	assert.False(t, locked, "Lock should be released")

	// Verify component is no longer running
	running, err = tracker.IsRunning(ctx, comp.Kind, comp.Name)
	require.NoError(t, err)
	assert.False(t, running, "Component should not be detected as running")
}

// TestLocalLifecycleCrashRecovery tests that the system properly cleans up
// stale state when a component's files are left behind.
func TestLocalLifecycleCrashRecovery(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	socketPath, err := GetSocketPath(ComponentKindAgent, "crash-agent")
	require.NoError(t, err)

	// Start a server
	server := startTestServer(t, socketPath)

	comp := &Component{
		Kind: ComponentKindAgent,
		Name: "crash-agent",
		PID:  os.Getpid(),
		Port: 50052,
	}

	// Track the component
	err = tracker.Start(ctx, comp)
	require.NoError(t, err)

	// Verify component is running
	running, err := tracker.IsRunning(ctx, comp.Kind, comp.Name)
	require.NoError(t, err)
	assert.True(t, running)

	// Simulate a crash - stop server and remove socket (as OS would)
	server.Stop()
	time.Sleep(100 * time.Millisecond)
	_ = os.Remove(socketPath)

	// PID file and lock file still exist (stale state from tracker's perspective)
	pidPath, _ := GetPIDFilePath(comp.Kind, comp.Name)
	_, err = os.Stat(pidPath)
	assert.NoError(t, err, "PID file still exists after crash")

	lockPath, _ := GetLockFilePath(comp.Kind, comp.Name)
	_, err = os.Stat(lockPath)
	assert.NoError(t, err, "Lock file still exists after crash")

	// Component should not be detected as running (socket missing)
	running, err = tracker.IsRunning(ctx, comp.Kind, comp.Name)
	require.NoError(t, err)
	assert.False(t, running, "Component without socket should not be running")

	// Manually release the lock for cleanup test
	_ = tracker.releaseLock(comp.Kind, comp.Name)

	// Now create a stale PID file with a dead process
	err = writePIDFile(comp.Kind, comp.Name, 999999, comp.Port)
	require.NoError(t, err)

	// Run cleanup - this should detect the dead process and clean up
	err = tracker.Cleanup(ctx)
	assert.NoError(t, err)

	// Verify cleanup removed stale PID file
	_, err = os.Stat(pidPath)
	assert.True(t, os.IsNotExist(err), "Cleanup should remove stale PID file")
}

// TestLocalLifecycleDuplicateStartPrevention tests that the lock mechanism
// prevents duplicate starts of the same component.
func TestLocalLifecycleDuplicateStartPrevention(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker1 := NewDefaultLocalTracker()
	tracker2 := NewDefaultLocalTracker()
	ctx := context.Background()

	socketPath, err := GetSocketPath(ComponentKindAgent, "duplicate-agent")
	require.NoError(t, err)

	// Start first server
	server := startTestServer(t, socketPath)
	defer server.Stop()

	comp1 := &Component{
		Kind: ComponentKindAgent,
		Name: "duplicate-agent",
		PID:  os.Getpid(),
		Port: 50053,
	}

	// First tracker starts successfully
	err = tracker1.Start(ctx, comp1)
	require.NoError(t, err)

	// Try to start with second tracker (simulating another process)
	comp2 := &Component{
		Kind: ComponentKindAgent,
		Name: "duplicate-agent",
		PID:  os.Getpid(),
		Port: 50054,
	}

	err = tracker2.Start(ctx, comp2)
	require.Error(t, err, "Second start should fail due to lock")

	// Verify it's a ComponentAlreadyRunningError
	var runningErr *ComponentAlreadyRunningError
	assert.ErrorAs(t, err, &runningErr)
	assert.Equal(t, ComponentKindAgent, runningErr.Kind)
	assert.Equal(t, "duplicate-agent", runningErr.Name)

	// First component should still be running
	running, err := tracker1.IsRunning(ctx, ComponentKindAgent, "duplicate-agent")
	require.NoError(t, err)
	assert.True(t, running)

	// Clean up
	_ = tracker1.Stop(ctx, comp1)
}

// TestLocalLifecycleDiscoveryFindsRunningAgents tests that the discovery
// mechanism correctly finds all running components.
func TestLocalLifecycleDiscoveryFindsRunningAgents(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	// Start multiple agents
	agents := []struct {
		name string
		port int
	}{
		{"agent-1", 50055},
		{"agent-2", 50056},
		{"agent-3", 50057},
	}

	var servers []*grpc.Server
	var components []*Component

	for _, agent := range agents {
		socketPath, err := GetSocketPath(ComponentKindAgent, agent.name)
		require.NoError(t, err)

		server := startTestServer(t, socketPath)
		servers = append(servers, server)

		comp := &Component{
			Kind: ComponentKindAgent,
			Name: agent.name,
			PID:  os.Getpid(),
			Port: agent.port,
		}
		components = append(components, comp)

		err = tracker.Start(ctx, comp)
		require.NoError(t, err)
	}

	// Clean up all servers at the end
	defer func() {
		for _, server := range servers {
			server.Stop()
		}
	}()

	// Discover running agents
	states, err := tracker.Discover(ctx, ComponentKindAgent)
	require.NoError(t, err)
	require.Len(t, states, 3, "Should discover all 3 running agents")

	// Verify all agents are discovered and healthy
	discoveredNames := make(map[string]LocalComponentState)
	for _, state := range states {
		discoveredNames[state.Name] = state
		assert.Equal(t, ComponentKindAgent, state.Kind)
		assert.True(t, state.Healthy, "Agent %s should be healthy", state.Name)
	}

	// Verify each expected agent was discovered
	for _, agent := range agents {
		state, found := discoveredNames[agent.name]
		assert.True(t, found, "Agent %s should be discovered", agent.name)
		if found {
			assert.Equal(t, agent.port, state.Port)
		}
	}

	// Stop one agent and verify discovery updates
	err = tracker.Stop(ctx, components[0])
	require.NoError(t, err)
	servers[0].Stop()
	time.Sleep(100 * time.Millisecond)

	// Discover again - should only find 2 agents now
	states, err = tracker.Discover(ctx, ComponentKindAgent)
	require.NoError(t, err)
	require.Len(t, states, 2, "Should discover 2 running agents after stopping one")

	// Verify the stopped agent is not in the list
	for _, state := range states {
		assert.NotEqual(t, "agent-1", state.Name, "Stopped agent should not be discovered")
	}
}

// TestLocalLifecycleDiscoveryMixedHealthStates tests discovery with a mix
// of healthy and unhealthy components.
func TestLocalLifecycleDiscoveryMixedHealthStates(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	// Start healthy agent
	socketPath1, err := GetSocketPath(ComponentKindAgent, "healthy-agent")
	require.NoError(t, err)
	server1 := startTestServer(t, socketPath1)
	defer server1.Stop()

	comp1 := &Component{
		Kind: ComponentKindAgent,
		Name: "healthy-agent",
		PID:  os.Getpid(),
		Port: 50058,
	}
	err = tracker.Start(ctx, comp1)
	require.NoError(t, err)

	// Create unhealthy agent (PID file with dead process)
	err = writePIDFile(ComponentKindAgent, "dead-agent", 999999, 50059)
	require.NoError(t, err)

	// Create unhealthy agent (running but no socket)
	socketPath3, err := GetSocketPath(ComponentKindAgent, "no-socket-agent")
	require.NoError(t, err)
	server3 := startTestServer(t, socketPath3)
	defer server3.Stop()

	// Track without socket (manually create PID and lock, then remove socket)
	comp3 := &Component{
		Kind: ComponentKindAgent,
		Name: "no-socket-agent",
		PID:  os.Getpid(),
		Port: 50060,
	}
	err = writePIDFile(comp3.Kind, comp3.Name, comp3.PID, comp3.Port)
	require.NoError(t, err)
	err = tracker.acquireLock(comp3.Kind, comp3.Name)
	require.NoError(t, err)

	// Remove the socket to make it unhealthy
	server3.Stop()
	_ = os.Remove(socketPath3)

	// Discover - should only find healthy agent
	states, err := tracker.Discover(ctx, ComponentKindAgent)
	require.NoError(t, err)
	require.Len(t, states, 1, "Should only discover healthy agent")
	assert.Equal(t, "healthy-agent", states[0].Name)
	assert.True(t, states[0].Healthy)

	// Verify unhealthy agents were cleaned up
	pidPath2, _ := GetPIDFilePath(ComponentKindAgent, "dead-agent")
	_, err = os.Stat(pidPath2)
	assert.True(t, os.IsNotExist(err), "Dead agent PID file should be cleaned up")

	pidPath3, _ := GetPIDFilePath(ComponentKindAgent, "no-socket-agent")
	_, err = os.Stat(pidPath3)
	assert.True(t, os.IsNotExist(err), "No-socket agent PID file should be cleaned up")
}

// TestLocalLifecycleDiscoveryAcrossKinds tests that discovery correctly
// filters by component kind.
func TestLocalLifecycleDiscoveryAcrossKinds(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	// Start agent
	socketPath1, _ := GetSocketPath(ComponentKindAgent, "test-agent")
	server1 := startTestServer(t, socketPath1)
	defer server1.Stop()

	comp1 := &Component{
		Kind: ComponentKindAgent,
		Name: "test-agent",
		PID:  os.Getpid(),
		Port: 50061,
	}
	err := tracker.Start(ctx, comp1)
	require.NoError(t, err)

	// Start tool
	socketPath2, _ := GetSocketPath(ComponentKindTool, "test-tool")
	server2 := startTestServer(t, socketPath2)
	defer server2.Stop()

	comp2 := &Component{
		Kind: ComponentKindTool,
		Name: "test-tool",
		PID:  os.Getpid(),
		Port: 50062,
	}
	err = tracker.Start(ctx, comp2)
	require.NoError(t, err)

	// Start plugin
	socketPath3, _ := GetSocketPath(ComponentKindPlugin, "test-plugin")
	server3 := startTestServer(t, socketPath3)
	defer server3.Stop()

	comp3 := &Component{
		Kind: ComponentKindPlugin,
		Name: "test-plugin",
		PID:  os.Getpid(),
		Port: 50063,
	}
	err = tracker.Start(ctx, comp3)
	require.NoError(t, err)

	// Discover agents only
	agentStates, err := tracker.Discover(ctx, ComponentKindAgent)
	require.NoError(t, err)
	require.Len(t, agentStates, 1)
	assert.Equal(t, ComponentKindAgent, agentStates[0].Kind)
	assert.Equal(t, "test-agent", agentStates[0].Name)

	// Discover tools only
	toolStates, err := tracker.Discover(ctx, ComponentKindTool)
	require.NoError(t, err)
	require.Len(t, toolStates, 1)
	assert.Equal(t, ComponentKindTool, toolStates[0].Kind)
	assert.Equal(t, "test-tool", toolStates[0].Name)

	// Discover plugins only
	pluginStates, err := tracker.Discover(ctx, ComponentKindPlugin)
	require.NoError(t, err)
	require.Len(t, pluginStates, 1)
	assert.Equal(t, ComponentKindPlugin, pluginStates[0].Kind)
	assert.Equal(t, "test-plugin", pluginStates[0].Name)
}

// TestLocalLifecycleConcurrentStartStop tests concurrent start/stop operations
// to ensure thread safety.
func TestLocalLifecycleConcurrentStartStop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent test in short mode")
	}

	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	const numAgents = 5
	var wg sync.WaitGroup
	errors := make(chan error, numAgents*2)

	// Start and stop multiple agents concurrently
	for i := 0; i < numAgents; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			name := fmt.Sprintf("concurrent-agent-%d", idx)
			socketPath, _ := GetSocketPath(ComponentKindAgent, name)

			server := startTestServer(t, socketPath)
			defer server.Stop()

			comp := &Component{
				Kind: ComponentKindAgent,
				Name: name,
				PID:  os.Getpid(),
				Port: 50070 + idx,
			}

			if err := tracker.Start(ctx, comp); err != nil {
				errors <- fmt.Errorf("failed to start %s: %w", name, err)
				return
			}

			// Give it a moment to be registered
			time.Sleep(50 * time.Millisecond)

			// Stop the agent
			if err := tracker.Stop(ctx, comp); err != nil {
				errors <- fmt.Errorf("failed to stop %s: %w", name, err)
				return
			}
		}(i)
	}

	// Wait for all operations to complete
	wg.Wait()
	close(errors)

	// Check for errors
	var errList []error
	for err := range errors {
		errList = append(errList, err)
	}

	if len(errList) > 0 {
		for _, err := range errList {
			t.Errorf("Concurrent operation error: %v", err)
		}
		t.FailNow()
	}
}

// TestLocalLifecycleStaleSocketCleanup tests that stale sockets are properly
// cleaned up when the server is not responding.
func TestLocalLifecycleStaleSocketCleanup(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	socketPath, err := GetSocketPath(ComponentKindAgent, "stale-socket-agent")
	require.NoError(t, err)

	// Create a socket manually (simulating a leftover socket)
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	// Close the listener immediately - socket file remains but won't accept connections
	listener.Close()

	// Verify socket file exists
	info, err := os.Stat(socketPath)
	require.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSocket != 0)

	// Try to clean up the stale socket
	err = tracker.cleanupStaleSocket(ctx, socketPath)
	assert.NoError(t, err, "Should clean up stale socket")

	// Verify socket was removed
	_, err = os.Stat(socketPath)
	assert.True(t, os.IsNotExist(err), "Stale socket should be removed")
}

// TestLocalLifecycleHealthySocketNotRemoved tests that healthy sockets
// are not removed during cleanup.
func TestLocalLifecycleHealthySocketNotRemoved(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tracker := NewDefaultLocalTracker()
	ctx := context.Background()

	socketPath, err := GetSocketPath(ComponentKindAgent, "healthy-socket-agent")
	require.NoError(t, err)

	// Start a healthy server
	server := startTestServer(t, socketPath)
	defer server.Stop()

	// Verify socket exists
	info, err := os.Stat(socketPath)
	require.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSocket != 0)

	// Try to clean up - should refuse because socket is healthy
	err = tracker.cleanupStaleSocket(ctx, socketPath)
	assert.Error(t, err, "Should refuse to clean up healthy socket")
	assert.Contains(t, err.Error(), "healthy")

	// Verify socket still exists
	_, err = os.Stat(socketPath)
	assert.NoError(t, err, "Healthy socket should not be removed")
}

// Helper Functions

// startTestServer starts a gRPC health server listening on a Unix socket.
func startTestServer(t *testing.T, socketPath string) *grpc.Server {
	t.Helper()

	// Remove socket if it exists
	_ = os.Remove(socketPath)

	// Create listener
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	// Create gRPC server with health check
	server := grpc.NewServer()
	healthServer := newMockHealthServer(grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(server, healthServer)

	// Start serving in background
	go func() {
		_ = server.Serve(listener)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Verify socket was created
	_, err = os.Stat(socketPath)
	require.NoError(t, err, "Socket should be created")

	return server
}

// killProcessByPID sends signals to a process to terminate it.
// This is used in crash recovery tests.
func killProcessByPID(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	// Try graceful shutdown first
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return err
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Force kill if still alive
	_ = proc.Signal(syscall.SIGKILL)

	return nil
}

// ========== Remote Probing Integration Tests ==========

// TestIntegration_RemoteProber_RealGRPCHealthServer tests remote probing against a real gRPC health server.
// This integration test validates that the remote prober can successfully communicate with actual
// gRPC servers implementing the health check protocol.
func TestIntegration_RemoteProber_RealGRPCHealthServer(t *testing.T) {
	tests := []struct {
		name           string
		status         grpc_health_v1.HealthCheckResponse_ServingStatus
		expectHealthy  bool
		expectErrorMsg string
	}{
		{
			name:          "healthy server",
			status:        grpc_health_v1.HealthCheckResponse_SERVING,
			expectHealthy: true,
		},
		{
			name:           "not serving",
			status:         grpc_health_v1.HealthCheckResponse_NOT_SERVING,
			expectHealthy:  false,
			expectErrorMsg: "service not serving",
		},
		{
			name:           "service unknown",
			status:         grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN,
			expectHealthy:  false,
			expectErrorMsg: "service not serving",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start real gRPC server with health service
			server, lis, addr, _ := startGRPCHealthServer(t, tt.status)
			defer server.Stop()
			defer lis.Close()

			// Create prober and perform health check
			prober := NewDefaultRemoteProber()
			ctx := context.Background()

			state, err := prober.Probe(ctx, addr)

			// Verify results
			if tt.expectHealthy {
				require.NoError(t, err)
				assert.True(t, state.Healthy, "server should be healthy")
				assert.Empty(t, state.Error)
			} else {
				require.Error(t, err)
				assert.False(t, state.Healthy, "server should be unhealthy")
				assert.Contains(t, state.Error, tt.expectErrorMsg)
			}

			// Verify state metadata
			assert.Equal(t, addr, state.Address)
			assert.Greater(t, state.ResponseTime, time.Duration(0), "response time should be tracked")
			assert.False(t, state.LastCheck.IsZero(), "last check time should be set")
		})
	}
}

// TestIntegration_RemoteProber_TimeoutHandling tests timeout behavior with slow servers.
// This validates that the prober correctly handles servers that delay responses and
// respects configured timeout values.
func TestIntegration_RemoteProber_TimeoutHandling(t *testing.T) {
	tests := []struct {
		name          string
		serverDelay   time.Duration
		probeTimeout  time.Duration
		expectTimeout bool
	}{
		{
			name:          "fast server within timeout",
			serverDelay:   10 * time.Millisecond,
			probeTimeout:  100 * time.Millisecond,
			expectTimeout: false,
		},
		{
			name:          "slow server exceeds timeout",
			serverDelay:   200 * time.Millisecond,
			probeTimeout:  50 * time.Millisecond,
			expectTimeout: true,
		},
		{
			name:          "server just under timeout",
			serverDelay:   90 * time.Millisecond,
			probeTimeout:  100 * time.Millisecond,
			expectTimeout: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create slow health server
			server := grpc.NewServer()
			grpc_health_v1.RegisterHealthServer(server, &slowHealthServer{delay: tt.serverDelay})

			lis, err := net.Listen("tcp", "localhost:0")
			require.NoError(t, err)
			defer lis.Close()

			go func() {
				_ = server.Serve(lis)
			}()
			defer server.Stop()

			time.Sleep(50 * time.Millisecond) // Let server start

			// Configure prober with custom timeout
			prober := NewDefaultRemoteProber()
			err = prober.LoadConfig(
				map[string]RemoteComponentConfig{
					"slow-server": {
						Address:     lis.Addr().String(),
						HealthCheck: "grpc",
						Timeout:     tt.probeTimeout,
					},
				},
				map[string]RemoteComponentConfig{},
				map[string]RemoteComponentConfig{},
			)
			require.NoError(t, err)

			// Perform probe
			ctx := context.Background()
			startTime := time.Now()
			states, err := prober.ProbeAll(ctx)
			duration := time.Since(startTime)

			require.NoError(t, err)
			require.Len(t, states, 1)

			if tt.expectTimeout {
				assert.False(t, states[0].Healthy, "should timeout and be unhealthy")
				assert.NotEmpty(t, states[0].Error, "should have error message")
				// Should complete around the timeout duration
				assert.Less(t, duration, tt.probeTimeout*2, "should respect timeout")
			} else {
				assert.True(t, states[0].Healthy, "should succeed within timeout")
				assert.Empty(t, states[0].Error)
				// Should complete around the server delay
				assert.GreaterOrEqual(t, duration, tt.serverDelay, "should take at least server delay")
			}
		})
	}
}

// TestIntegration_RemoteProber_ParallelProbing tests parallel probing performance.
// This validates that ProbeAll executes probes concurrently and meets performance targets.
// Target: 500ms for 20 components = 25ms average per component.
func TestIntegration_RemoteProber_ParallelProbing(t *testing.T) {
	tests := []struct {
		name             string
		numServers       int
		delayPerServer   time.Duration
		maxTotalTime     time.Duration
		expectAllHealthy bool
	}{
		{
			name:             "10 fast servers",
			numServers:       10,
			delayPerServer:   20 * time.Millisecond,
			maxTotalTime:     100 * time.Millisecond, // Much less than sequential (200ms)
			expectAllHealthy: true,
		},
		{
			name:             "20 servers with 25ms delay",
			numServers:       20,
			delayPerServer:   25 * time.Millisecond,
			maxTotalTime:     150 * time.Millisecond, // Much less than sequential (500ms)
			expectAllHealthy: true,
		},
		{
			name:             "50 fast servers",
			numServers:       50,
			delayPerServer:   10 * time.Millisecond,
			maxTotalTime:     200 * time.Millisecond, // Much less than sequential (500ms)
			expectAllHealthy: true,
		},
		{
			name:             "100 servers stress test",
			numServers:       100,
			delayPerServer:   5 * time.Millisecond,
			maxTotalTime:     300 * time.Millisecond, // Much less than sequential (500ms)
			expectAllHealthy: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create multiple servers
			var servers []*grpc.Server
			var listeners []net.Listener
			agents := make(map[string]RemoteComponentConfig)

			for i := 0; i < tt.numServers; i++ {
				server := grpc.NewServer()
				grpc_health_v1.RegisterHealthServer(server, &slowHealthServer{delay: tt.delayPerServer})

				lis, err := net.Listen("tcp", "localhost:0")
				require.NoError(t, err)

				go func() {
					_ = server.Serve(lis)
				}()

				servers = append(servers, server)
				listeners = append(listeners, lis)

				// Use numeric names for simplicity
				agentName := generateAgentName(i)

				agents[agentName] = RemoteComponentConfig{
					Address:     lis.Addr().String(),
					HealthCheck: "grpc",
					Timeout:     5 * time.Second, // Generous timeout
				}
			}

			// Cleanup
			defer func() {
				for _, s := range servers {
					s.Stop()
				}
				for _, l := range listeners {
					l.Close()
				}
			}()

			time.Sleep(100 * time.Millisecond) // Let servers start

			// Configure prober
			prober := NewDefaultRemoteProber()
			err := prober.LoadConfig(agents, map[string]RemoteComponentConfig{}, map[string]RemoteComponentConfig{})
			require.NoError(t, err)

			// Perform parallel probing
			ctx := context.Background()
			startTime := time.Now()
			states, err := prober.ProbeAll(ctx)
			duration := time.Since(startTime)

			// Verify results
			require.NoError(t, err)
			require.Len(t, states, tt.numServers)

			// Verify parallelism: total time should be much less than sequential
			sequentialTime := tt.delayPerServer * time.Duration(tt.numServers)
			t.Logf("Parallel execution: %v (sequential would be: %v)", duration, sequentialTime)
			assert.Less(t, duration, tt.maxTotalTime,
				"parallel execution should be much faster than sequential")

			// Verify all components are healthy
			if tt.expectAllHealthy {
				for _, state := range states {
					assert.True(t, state.Healthy, "component %s should be healthy", state.Name)
					assert.Empty(t, state.Error)
					assert.Greater(t, state.ResponseTime, time.Duration(0))
				}
			}
		})
	}
}

// TestIntegration_RemoteProber_ConcurrentProbeAllCalls tests concurrent ProbeAll calls.
// This validates thread-safety when multiple goroutines call ProbeAll simultaneously.
func TestIntegration_RemoteProber_ConcurrentProbeAllCalls(t *testing.T) {
	const (
		numServers       = 20
		numConcurrentOps = 10
	)

	// Create test servers
	var servers []*grpc.Server
	var listeners []net.Listener
	agents := make(map[string]RemoteComponentConfig)

	for i := 0; i < numServers; i++ {
		server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
		servers = append(servers, server)
		listeners = append(listeners, lis)
		agents[generateAgentName(i)] = RemoteComponentConfig{
			Address:     addr,
			HealthCheck: "grpc",
		}
	}

	defer func() {
		for _, s := range servers {
			s.Stop()
		}
		for _, l := range listeners {
			l.Close()
		}
	}()

	// Configure prober
	prober := NewDefaultRemoteProber()
	err := prober.LoadConfig(agents, map[string]RemoteComponentConfig{}, map[string]RemoteComponentConfig{})
	require.NoError(t, err)

	// Execute concurrent ProbeAll calls
	var wg sync.WaitGroup
	var successCount atomic.Int32
	var totalProbes atomic.Int32
	ctx := context.Background()

	for i := 0; i < numConcurrentOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			states, err := prober.ProbeAll(ctx)
			if err == nil && len(states) == numServers {
				successCount.Add(1)
				totalProbes.Add(int32(len(states)))
			}
		}()
	}

	wg.Wait()

	// Verify all operations succeeded
	assert.Equal(t, int32(numConcurrentOps), successCount.Load(),
		"all concurrent ProbeAll calls should succeed")
	assert.Equal(t, int32(numServers*numConcurrentOps), totalProbes.Load(),
		"should have probed all components in all calls")
}

// TestIntegration_RemoteProber_MixedHealthStates tests probing with mixed healthy/unhealthy components.
// This validates that ProbeAll handles partial failures correctly.
func TestIntegration_RemoteProber_MixedHealthStates(t *testing.T) {
	// Create servers with different health states
	healthyServer1, healthyLis1, healthyAddr1, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer healthyServer1.Stop()
	defer healthyLis1.Close()

	healthyServer2, healthyLis2, healthyAddr2, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer healthyServer2.Stop()
	defer healthyLis2.Close()

	unhealthyServer, unhealthyLis, unhealthyAddr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	defer unhealthyServer.Stop()
	defer unhealthyLis.Close()

	// Configure prober with mixed components
	prober := NewDefaultRemoteProber()
	agents := map[string]RemoteComponentConfig{
		"healthy1":  {Address: healthyAddr1, HealthCheck: "grpc"},
		"healthy2":  {Address: healthyAddr2, HealthCheck: "grpc"},
		"unhealthy": {Address: unhealthyAddr, HealthCheck: "grpc"},
		"offline":   {Address: "localhost:99999", HealthCheck: "grpc", Timeout: 100 * time.Millisecond},
	}
	err := prober.LoadConfig(agents, map[string]RemoteComponentConfig{}, map[string]RemoteComponentConfig{})
	require.NoError(t, err)

	// Perform probing
	ctx := context.Background()
	states, err := prober.ProbeAll(ctx)
	require.NoError(t, err)
	require.Len(t, states, 4)

	// Categorize results
	healthyCount := 0
	unhealthyCount := 0
	for _, state := range states {
		if state.Healthy {
			healthyCount++
			assert.Empty(t, state.Error)
		} else {
			unhealthyCount++
			assert.NotEmpty(t, state.Error, "unhealthy component should have error message")
		}
		assert.Greater(t, state.ResponseTime, time.Duration(0))
		assert.False(t, state.LastCheck.IsZero())
	}

	assert.Equal(t, 2, healthyCount, "should have 2 healthy components")
	assert.Equal(t, 2, unhealthyCount, "should have 2 unhealthy components")
}

// TestIntegration_RemoteProber_PerformanceTarget tests that ProbeAll meets performance targets.
// Target: 500ms for 20 components with realistic network conditions.
func TestIntegration_RemoteProber_PerformanceTarget(t *testing.T) {
	const (
		numComponents = 20
		targetTime    = 500 * time.Millisecond
		serverDelay   = 10 * time.Millisecond
	)

	// Create 20 servers with small delay to simulate network latency
	var servers []*grpc.Server
	var listeners []net.Listener
	agents := make(map[string]RemoteComponentConfig)

	for i := 0; i < numComponents; i++ {
		server := grpc.NewServer()
		grpc_health_v1.RegisterHealthServer(server, &slowHealthServer{delay: serverDelay})

		lis, err := net.Listen("tcp", "localhost:0")
		require.NoError(t, err)

		go func() {
			_ = server.Serve(lis)
		}()

		servers = append(servers, server)
		listeners = append(listeners, lis)
		agents[generateAgentName(i)] = RemoteComponentConfig{
			Address:     lis.Addr().String(),
			HealthCheck: "grpc",
			Timeout:     1 * time.Second,
		}
	}

	defer func() {
		for _, s := range servers {
			s.Stop()
		}
		for _, l := range listeners {
			l.Close()
		}
	}()

	time.Sleep(100 * time.Millisecond) // Let servers start

	// Configure prober
	prober := NewDefaultRemoteProber()
	err := prober.LoadConfig(agents, map[string]RemoteComponentConfig{}, map[string]RemoteComponentConfig{})
	require.NoError(t, err)

	// Run multiple times to get consistent measurements
	var totalDuration time.Duration
	const numRuns = 5

	for i := 0; i < numRuns; i++ {
		ctx := context.Background()
		startTime := time.Now()
		states, err := prober.ProbeAll(ctx)
		duration := time.Since(startTime)

		require.NoError(t, err)
		require.Len(t, states, numComponents)

		totalDuration += duration
	}

	avgDuration := totalDuration / numRuns
	t.Logf("Average ProbeAll duration for %d components: %v (target: %v)",
		numComponents, avgDuration, targetTime)

	// Performance assertion
	assert.Less(t, avgDuration, targetTime,
		"ProbeAll should meet performance target of %v for %d components", targetTime, numComponents)
}

// TestIntegration_RemoteProber_ResponseTimeAccuracy tests response time tracking accuracy.
// This validates that reported response times match actual server delays.
func TestIntegration_RemoteProber_ResponseTimeAccuracy(t *testing.T) {
	tests := []struct {
		name        string
		serverDelay time.Duration
	}{
		{"10ms delay", 10 * time.Millisecond},
		{"50ms delay", 50 * time.Millisecond},
		{"100ms delay", 100 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create slow server
			server := grpc.NewServer()
			grpc_health_v1.RegisterHealthServer(server, &slowHealthServer{delay: tt.serverDelay})

			lis, err := net.Listen("tcp", "localhost:0")
			require.NoError(t, err)
			defer lis.Close()

			go func() {
				_ = server.Serve(lis)
			}()
			defer server.Stop()

			time.Sleep(50 * time.Millisecond)

			// Configure prober
			prober := NewDefaultRemoteProber()
			agents := map[string]RemoteComponentConfig{
				"test": {
					Address:     lis.Addr().String(),
					HealthCheck: "grpc",
					Timeout:     5 * time.Second,
				},
			}
			err = prober.LoadConfig(agents, map[string]RemoteComponentConfig{}, map[string]RemoteComponentConfig{})
			require.NoError(t, err)

			// Probe and verify response time
			ctx := context.Background()
			states, err := prober.ProbeAll(ctx)
			require.NoError(t, err)
			require.Len(t, states, 1)

			// Response time should be at least the server delay
			assert.GreaterOrEqual(t, states[0].ResponseTime, tt.serverDelay,
				"response time should be at least server delay")
			// But not too much longer (allow 3x for overhead)
			assert.Less(t, states[0].ResponseTime, tt.serverDelay*3,
				"response time should not be excessively longer than server delay")
		})
	}
}

// TestIntegration_RemoteProber_ContextCancellation tests context cancellation during ProbeAll.
// This validates that in-flight probes are cancelled when the context is cancelled.
func TestIntegration_RemoteProber_ContextCancellation(t *testing.T) {
	const numServers = 10

	// Create slow servers
	var servers []*grpc.Server
	var listeners []net.Listener
	agents := make(map[string]RemoteComponentConfig)

	for i := 0; i < numServers; i++ {
		server := grpc.NewServer()
		grpc_health_v1.RegisterHealthServer(server, &slowHealthServer{delay: 5 * time.Second})

		lis, err := net.Listen("tcp", "localhost:0")
		require.NoError(t, err)

		go func() {
			_ = server.Serve(lis)
		}()

		servers = append(servers, server)
		listeners = append(listeners, lis)
		agents[generateAgentName(i)] = RemoteComponentConfig{
			Address:     lis.Addr().String(),
			HealthCheck: "grpc",
		}
	}

	defer func() {
		for _, s := range servers {
			s.Stop()
		}
		for _, l := range listeners {
			l.Close()
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// Configure prober
	prober := NewDefaultRemoteProber()
	err := prober.LoadConfig(agents, map[string]RemoteComponentConfig{}, map[string]RemoteComponentConfig{})
	require.NoError(t, err)

	// Create context that will be cancelled
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// ProbeAll should return quickly due to context cancellation
	startTime := time.Now()
	states, err := prober.ProbeAll(ctx)
	duration := time.Since(startTime)

	require.NoError(t, err)
	// Should complete quickly (much less than server delay of 5s)
	assert.Less(t, duration, 1*time.Second, "should cancel quickly")

	// All probes should have failed due to timeout
	for _, state := range states {
		assert.False(t, state.Healthy, "all probes should fail due to cancellation")
	}
}

// TestIntegration_RemoteProber_LoadConfigReplacement tests that LoadConfig replaces configuration.
// This validates that calling LoadConfig multiple times correctly replaces the previous configuration.
func TestIntegration_RemoteProber_LoadConfigReplacement(t *testing.T) {
	// Create two sets of servers
	server1, lis1, addr1, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server1.Stop()
	defer lis1.Close()

	server2, lis2, addr2, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server2.Stop()
	defer lis2.Close()

	prober := NewDefaultRemoteProber()

	// Load initial config
	err := prober.LoadConfig(
		map[string]RemoteComponentConfig{
			"agent1": {Address: addr1, HealthCheck: "grpc"},
		},
		map[string]RemoteComponentConfig{},
		map[string]RemoteComponentConfig{},
	)
	require.NoError(t, err)

	// Verify initial config
	states, err := prober.ProbeAll(context.Background())
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "agent1", states[0].Name)

	// Load new config (should replace)
	err = prober.LoadConfig(
		map[string]RemoteComponentConfig{
			"agent2": {Address: addr2, HealthCheck: "grpc"},
		},
		map[string]RemoteComponentConfig{},
		map[string]RemoteComponentConfig{},
	)
	require.NoError(t, err)

	// Verify new config replaced old
	states, err = prober.ProbeAll(context.Background())
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "agent2", states[0].Name)
}

// generateAgentName generates an agent name for the given index.
// Uses alphabetic naming (a-z, aa-az, ba-bz, etc.) to support large numbers of agents.
func generateAgentName(i int) string {
	if i < 26 {
		return string(rune('a' + i))
	}
	// For more than 26 servers, use two-letter names
	return string(rune('a'+(i/26))) + string(rune('a'+(i%26)))
}
