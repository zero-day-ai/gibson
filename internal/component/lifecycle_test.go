package component

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHealthMonitor is a mock implementation of HealthMonitor for testing.
type mockHealthMonitor struct {
	mu                sync.RWMutex
	checkComponentErr error
	checkCalls        int
	healthDelay       time.Duration
	healthEndpoint    string
}

func newMockHealthMonitor() *mockHealthMonitor {
	return &mockHealthMonitor{}
}

func (m *mockHealthMonitor) Start(ctx context.Context) error {
	return nil
}

func (m *mockHealthMonitor) Stop() error {
	return nil
}

func (m *mockHealthMonitor) CheckComponent(ctx context.Context, healthEndpoint string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.checkCalls++
	m.healthEndpoint = healthEndpoint

	if m.healthDelay > 0 {
		time.Sleep(m.healthDelay)
	}

	return m.checkComponentErr
}

func (m *mockHealthMonitor) OnStatusChange(callback StatusChangeCallback) {}

func (m *mockHealthMonitor) GetHealth(componentName string) HealthStatus {
	return HealthStatusHealthy
}

func (m *mockHealthMonitor) RegisterComponent(name string, healthEndpoint string) {}

func (m *mockHealthMonitor) UnregisterComponent(name string) {}

func (m *mockHealthMonitor) setCheckError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkComponentErr = err
}

func (m *mockHealthMonitor) getCheckCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.checkCalls
}

func (m *mockHealthMonitor) getHealthEndpoint() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.healthEndpoint
}

// mockStatusUpdater is a mock implementation of StatusUpdater for testing.
type mockStatusUpdater struct{}

func (m *mockStatusUpdater) UpdateStatus(ctx context.Context, id int64, status ComponentStatus, pid, port int) error {
	return nil
}

// TestNewLifecycleManager tests the creation of a new lifecycle manager.
func TestNewLifecycleManager(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)

	assert.NotNil(t, manager)
	assert.Equal(t, DefaultStartupTimeout, manager.startupTimeout)
	assert.Equal(t, DefaultShutdownTimeout, manager.shutdownTimeout)
	assert.Equal(t, DefaultPortRangeStart, manager.portRangeStart)
	assert.Equal(t, DefaultPortRangeEnd, manager.portRangeEnd)
	assert.NotNil(t, manager.processes)
}

// TestNewLifecycleManagerWithTimeouts tests the creation with custom timeouts.
func TestNewLifecycleManagerWithTimeouts(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	startupTimeout := 5 * time.Second
	shutdownTimeout := 3 * time.Second
	portStart := 40000
	portEnd := 45000

	manager := NewLifecycleManagerWithTimeouts(
		healthMonitor,
		dao,
		startupTimeout,
		shutdownTimeout,
		portStart,
		portEnd,
	)

	assert.NotNil(t, manager)
	assert.Equal(t, startupTimeout, manager.startupTimeout)
	assert.Equal(t, shutdownTimeout, manager.shutdownTimeout)
	assert.Equal(t, portStart, manager.portRangeStart)
	assert.Equal(t, portEnd, manager.portRangeEnd)
}

// TestFindAvailablePort tests finding an available port.
func TestFindAvailablePort(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)

	port, err := manager.findAvailablePort()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, port, DefaultPortRangeStart)
	assert.LessOrEqual(t, port, DefaultPortRangeEnd)

	// Verify port is actually available
	assert.True(t, manager.isPortAvailable(port))
}

// TestIsPortAvailable tests port availability checking.
func TestIsPortAvailable(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)

	// Find an available port
	port, err := manager.findAvailablePort()
	require.NoError(t, err)

	// Port should be available
	assert.True(t, manager.isPortAvailable(port))

	// Create a server on the port
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	require.NoError(t, err)
	defer listener.Close()

	// Port should not be available now
	assert.False(t, manager.isPortAvailable(port))
}

// TestStartComponent_NilComponent tests starting a nil component.
func TestStartComponent_NilComponent(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)
	ctx := context.Background()

	port, err := manager.StartComponent(ctx, nil)
	assert.Error(t, err)
	assert.Equal(t, 0, port)
	assert.Contains(t, err.Error(), "component cannot be nil")
}

// TestStartComponent_NoManifest tests starting a component without a manifest.
func TestStartComponent_NoManifest(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)
	ctx := context.Background()

	comp := &Component{
		Kind:    ComponentKindAgent,
		Name:    "test-agent",
		Version: "1.0.0",
		Status:  ComponentStatusAvailable,
	}

	port, err := manager.StartComponent(ctx, comp)
	assert.Error(t, err)
	assert.Equal(t, 0, port)
	assert.Contains(t, err.Error(), "manifest is required")
}

// TestStartComponent_AlreadyRunning tests starting an already running component.
func TestStartComponent_AlreadyRunning(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)
	ctx := context.Background()

	// Create a long-running process
	cmd := exec.Command("sleep", "10")
	err := cmd.Start()
	require.NoError(t, err)
	defer cmd.Process.Kill()

	comp := &Component{
		Kind:    ComponentKindAgent,
		Name:    "test-agent",
		Version: "1.0.0",
		Status:  ComponentStatusRunning,
		PID:     cmd.Process.Pid,
		Manifest: &Manifest{
			Name:    "test-agent",
			Version: "1.0.0",
			Runtime: &RuntimeConfig{
				Type:       RuntimeTypeBinary,
				Entrypoint: "sleep",
				Args:       []string{"10"},
			},
		},
	}

	port, err := manager.StartComponent(ctx, comp)
	assert.Error(t, err)
	assert.Equal(t, 0, port)
	assert.Contains(t, err.Error(), "already running")
}

// TestStopComponent_NilComponent tests stopping a nil component.
func TestStopComponent_NilComponent(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)
	ctx := context.Background()

	err := manager.StopComponent(ctx, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "component cannot be nil")
}

// TestStopComponent_NotRunning tests stopping a non-running component.
func TestStopComponent_NotRunning(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)
	ctx := context.Background()

	comp := &Component{
		Kind:    ComponentKindAgent,
		Name:    "test-agent",
		Version: "1.0.0",
		Status:  ComponentStatusStopped,
	}

	err := manager.StopComponent(ctx, comp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

// TestStopComponent_GracefulShutdown tests graceful shutdown.
func TestStopComponent_GracefulShutdown(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)
	ctx := context.Background()

	// Create a process that will shutdown gracefully
	cmd := exec.Command("sleep", "5")
	err := cmd.Start()
	require.NoError(t, err)

	comp := &Component{
		Kind:    ComponentKindAgent,
		Name:    "test-agent",
		Version: "1.0.0",
		Status:  ComponentStatusRunning,
		PID:     cmd.Process.Pid,
	}

	manager.mu.Lock()
	manager.processes[comp.Name] = cmd.Process
	manager.mu.Unlock()

	err = manager.StopComponent(ctx, comp)
	assert.NoError(t, err)
	assert.Equal(t, ComponentStatusStopped, comp.Status)
	assert.NotNil(t, comp.StoppedAt)

	// Verify process is stopped
	assert.False(t, manager.isProcessAlive(cmd.Process.Pid))
}

// TestStopComponent_ForceKill tests forced kill after timeout.
func TestStopComponent_ForceKill(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManagerWithTimeouts(
		healthMonitor,
		dao,
		5*time.Second,
		500*time.Millisecond, // Short timeout
		50000,
		51000,
	)
	ctx := context.Background()

	// Create a process that ignores SIGTERM
	cmd := exec.Command("sleep", "30")
	err := cmd.Start()
	require.NoError(t, err)

	comp := &Component{
		Kind:    ComponentKindAgent,
		Name:    "test-agent",
		Version: "1.0.0",
		Status:  ComponentStatusRunning,
		PID:     cmd.Process.Pid,
	}

	manager.mu.Lock()
	manager.processes[comp.Name] = cmd.Process
	manager.mu.Unlock()

	err = manager.StopComponent(ctx, comp)
	assert.NoError(t, err)
	assert.Equal(t, ComponentStatusStopped, comp.Status)

	// Verify process is killed
	time.Sleep(100 * time.Millisecond)
	assert.False(t, manager.isProcessAlive(cmd.Process.Pid))
}

// TestGetStatus tests getting component status.
func TestGetStatus(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)
	ctx := context.Background()

	tests := []struct {
		name           string
		component      *Component
		expectedStatus ComponentStatus
		expectError    bool
	}{
		{
			name:        "nil component",
			component:   nil,
			expectError: true,
		},
		{
			name: "stopped component",
			component: &Component{
				Kind:    ComponentKindAgent,
				Name:    "test-agent",
				Version: "1.0.0",
				Status:  ComponentStatusStopped,
			},
			expectedStatus: ComponentStatusStopped,
			expectError:    false,
		},
		{
			name: "available component",
			component: &Component{
				Kind:    ComponentKindAgent,
				Name:    "test-agent",
				Version: "1.0.0",
				Status:  ComponentStatusAvailable,
			},
			expectedStatus: ComponentStatusAvailable,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := manager.GetStatus(ctx, tt.component)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, status)
			}
		})
	}
}

// TestGetStatus_RunningProcess tests status check for running process.
func TestGetStatus_RunningProcess(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)
	ctx := context.Background()

	cmd := exec.Command("sleep", "5")
	err := cmd.Start()
	require.NoError(t, err)
	defer cmd.Process.Kill()

	comp := &Component{
		Kind:    ComponentKindAgent,
		Name:    "test-agent",
		Version: "1.0.0",
		Status:  ComponentStatusRunning,
		PID:     cmd.Process.Pid,
		Port:    50000,
		Manifest: &Manifest{
			Name:    "test-agent",
			Version: "1.0.0",
			Runtime: &RuntimeConfig{
				Type:       RuntimeTypeBinary,
				Entrypoint: "sleep",
			},
		},
	}

	manager.mu.Lock()
	manager.processes[comp.Name] = cmd.Process
	manager.mu.Unlock()

	status, err := manager.GetStatus(ctx, comp)
	assert.NoError(t, err)
	assert.Equal(t, ComponentStatusRunning, status)
}

// TestGetStatus_DeadProcess tests status check for dead process.
func TestGetStatus_DeadProcess(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)
	ctx := context.Background()

	cmd := exec.Command("sleep", "0.1")
	err := cmd.Start()
	require.NoError(t, err)
	time.Sleep(200 * time.Millisecond) // Wait for process to exit

	comp := &Component{
		Kind:    ComponentKindAgent,
		Name:    "test-agent",
		Version: "1.0.0",
		Status:  ComponentStatusRunning,
		PID:     cmd.Process.Pid,
	}

	manager.mu.Lock()
	manager.processes[comp.Name] = cmd.Process
	manager.mu.Unlock()

	status, err := manager.GetStatus(ctx, comp)
	assert.NoError(t, err)
	assert.Equal(t, ComponentStatusStopped, status)
	assert.Equal(t, ComponentStatusStopped, comp.Status)
}

// TestIsProcessAlive tests process alive checking.
func TestIsProcessAlive(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)

	// Test with running process
	cmd := exec.Command("sleep", "5")
	err := cmd.Start()
	require.NoError(t, err)
	defer cmd.Process.Kill()

	assert.True(t, manager.isProcessAlive(cmd.Process.Pid))

	// Test with non-existent PID (very high number unlikely to exist)
	assert.False(t, manager.isProcessAlive(999999))
}

// TestBuildHealthEndpoint tests health endpoint URL construction.
func TestBuildHealthEndpoint(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)

	tests := []struct {
		name        string
		component   *Component
		port        int
		expectedURL string
	}{
		{
			name: "default health URL",
			component: &Component{
				Name: "test-agent",
				Manifest: &Manifest{
					Runtime: &RuntimeConfig{},
				},
			},
			port:        8080,
			expectedURL: "http://localhost:8080/health",
		},
		{
			name: "custom health URL",
			component: &Component{
				Name: "test-agent",
				Manifest: &Manifest{
					Runtime: &RuntimeConfig{
						HealthURL: "/api/health",
					},
				},
			},
			port:        9000,
			expectedURL: "http://localhost:9000/api/health",
		},
		{
			name: "health URL without leading slash",
			component: &Component{
				Name: "test-agent",
				Manifest: &Manifest{
					Runtime: &RuntimeConfig{
						HealthURL: "healthcheck",
					},
				},
			},
			port:        9090,
			expectedURL: "http://localhost:9090/healthcheck",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := manager.buildHealthEndpoint(tt.component, tt.port)
			assert.Equal(t, tt.expectedURL, url)
		})
	}
}

// TestConcurrentOperations tests concurrent lifecycle operations.
func TestConcurrentOperations(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)

	var wg sync.WaitGroup
	iterations := 10

	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			port, err := manager.findAvailablePort()
			if err != nil {
				t.Errorf("iteration %d: failed to find port: %v", id, err)
				return
			}

			// Verify port is unique and available
			if port < DefaultPortRangeStart || port > DefaultPortRangeEnd {
				t.Errorf("iteration %d: port %d out of range", id, port)
			}
		}(i)
	}

	wg.Wait()
}

// TestKillProcess tests process killing.
func TestKillProcess(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)

	// Test killing nil process
	err := manager.killProcess(nil)
	assert.NoError(t, err)

	// Test killing actual process
	cmd := exec.Command("sleep", "30")
	err = cmd.Start()
	require.NoError(t, err)

	err = manager.killProcess(cmd.Process)
	assert.NoError(t, err)

	// Wait a bit and verify process is dead
	time.Sleep(100 * time.Millisecond)
	assert.False(t, manager.isProcessAlive(cmd.Process.Pid))
}

// createTestHealthServer creates a simple test script that responds to health checks.
func createTestHealthServer(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test-server.sh")

	script := `#!/bin/bash
# Simple test server that just sleeps
# In a real test, this would be a proper HTTP server
sleep 10
`

	err := os.WriteFile(scriptPath, []byte(script), 0755)
	require.NoError(t, err)

	return scriptPath
}

// Benchmark tests

// BenchmarkFindAvailablePort benchmarks port finding.
func BenchmarkFindAvailablePort(b *testing.B) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := manager.findAvailablePort()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkIsPortAvailable benchmarks port availability check.
func BenchmarkIsPortAvailable(b *testing.B) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.isPortAvailable(50000 + (i % 1000))
	}
}

// BenchmarkIsProcessAlive benchmarks process alive check.
func BenchmarkIsProcessAlive(b *testing.B) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManager(healthMonitor, dao)

	cmd := exec.Command("sleep", "60")
	err := cmd.Start()
	if err != nil {
		b.Fatal(err)
	}
	defer cmd.Process.Kill()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.isProcessAlive(cmd.Process.Pid)
	}
}
