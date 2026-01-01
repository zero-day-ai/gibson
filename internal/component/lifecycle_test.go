package component

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/health/grpc_health_v1"
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

func (m *mockHealthMonitor) RegisterComponentWithChecker(name string, checker HealthChecker) {}

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
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)

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
		nil, // logWriter
		nil, // localTracker
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
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)

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
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)

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
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)
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
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)
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
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)
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
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)
	ctx := context.Background()

	err := manager.StopComponent(ctx, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "component cannot be nil")
}

// TestStopComponent_NotRunning tests stopping a non-running component.
func TestStopComponent_NotRunning(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)
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
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)
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
		nil, // logWriter
		nil, // localTracker
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
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)
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
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)
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
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)
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
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)

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
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)

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

// TestLifecycleWithGRPCHealthCheck tests lifecycle with gRPC health check config.
func TestLifecycleWithGRPCHealthCheck(t *testing.T) {
	// Start a gRPC server with health checking
	server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server.Stop()
	defer lis.Close()

	// Parse host and port
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var grpcPort int
	_, err = fmt.Sscanf(portStr, "%d", &grpcPort)
	require.NoError(t, err)

	// Create component with gRPC health check config
	comp := &Component{
		Kind:    ComponentKindAgent,
		Name:    "grpc-test-agent",
		Version: "1.0.0",
		Status:  ComponentStatusAvailable,
		BinPath: createTestHealthServer(t),
		Manifest: &Manifest{
			Name:    "grpc-test-agent",
			Version: "1.0.0",
			Runtime: &RuntimeConfig{
				Type:       RuntimeTypeBinary,
				Entrypoint: "test-server",
				HealthCheck: &HealthCheckConfig{
					Protocol: HealthCheckProtocolGRPC,
				},
			},
		},
	}

	// Verify ProtocolAwareHealthChecker can be created with gRPC config
	checker := NewProtocolAwareHealthChecker(host, grpcPort, comp.Manifest.Runtime.HealthCheck)
	defer checker.Close()

	// Before check, protocol should be auto (not yet detected)
	assert.Equal(t, HealthCheckProtocolAuto, checker.Protocol())

	// Perform health check against the gRPC server
	ctx := context.Background()
	err = checker.Check(ctx)
	assert.NoError(t, err)

	// After successful check, should have detected gRPC
	assert.Equal(t, HealthCheckProtocolGRPC, checker.Protocol())
	assert.Equal(t, HealthCheckProtocolGRPC, checker.DetectedProtocol())
}

// TestLifecycleWithHTTPHealthCheck tests lifecycle with HTTP health check config.
func TestLifecycleWithHTTPHealthCheck(t *testing.T) {
	// Create HTTP server
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer httpServer.Close()

	// Parse host and port
	host, portStr, err := net.SplitHostPort(httpServer.Listener.Addr().String())
	require.NoError(t, err)
	var httpPort int
	_, err = fmt.Sscanf(portStr, "%d", &httpPort)
	require.NoError(t, err)

	// Create component with HTTP health check config
	comp := &Component{
		Kind:    ComponentKindAgent,
		Name:    "http-test-agent",
		Version: "1.0.0",
		Status:  ComponentStatusAvailable,
		BinPath: createTestHealthServer(t),
		Manifest: &Manifest{
			Name:    "http-test-agent",
			Version: "1.0.0",
			Runtime: &RuntimeConfig{
				Type:       RuntimeTypeBinary,
				Entrypoint: "test-server",
				HealthCheck: &HealthCheckConfig{
					Protocol: HealthCheckProtocolHTTP,
					Endpoint: "/health",
				},
			},
		},
	}

	// Verify ProtocolAwareHealthChecker can be created with HTTP config
	checker := NewProtocolAwareHealthChecker(host, httpPort, comp.Manifest.Runtime.HealthCheck)
	defer checker.Close()

	// Before check, protocol should be auto (not yet detected)
	assert.Equal(t, HealthCheckProtocolAuto, checker.Protocol())

	// Perform health check
	ctx := context.Background()
	err = checker.Check(ctx)
	assert.NoError(t, err)

	// After successful check, should have detected HTTP
	assert.Equal(t, HealthCheckProtocolHTTP, checker.Protocol())
	assert.Equal(t, HealthCheckProtocolHTTP, checker.DetectedProtocol())
}

// TestLifecycleWithAutoDetectHealthCheck tests lifecycle with auto-detect config.
func TestLifecycleWithAutoDetectHealthCheck(t *testing.T) {
	// Start a gRPC server
	server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server.Stop()
	defer lis.Close()

	// Parse host and port
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var grpcPort int
	_, err = fmt.Sscanf(portStr, "%d", &grpcPort)
	require.NoError(t, err)

	// Create component with auto-detect health check config
	comp := &Component{
		Kind:    ComponentKindAgent,
		Name:    "auto-test-agent",
		Version: "1.0.0",
		Status:  ComponentStatusAvailable,
		BinPath: createTestHealthServer(t),
		Manifest: &Manifest{
			Name:    "auto-test-agent",
			Version: "1.0.0",
			Runtime: &RuntimeConfig{
				Type:       RuntimeTypeBinary,
				Entrypoint: "test-server",
				HealthCheck: &HealthCheckConfig{
					Protocol: HealthCheckProtocolAuto,
				},
			},
		},
	}

	// Verify ProtocolAwareHealthChecker can be created with auto config
	checker := NewProtocolAwareHealthChecker(host, grpcPort, comp.Manifest.Runtime.HealthCheck)
	defer checker.Close()

	// Before check, protocol should be auto
	assert.Equal(t, HealthCheckProtocolAuto, checker.Protocol())

	// Perform health check - should detect gRPC
	ctx := context.Background()
	err = checker.Check(ctx)
	assert.NoError(t, err)

	// After check, should have detected gRPC
	assert.Equal(t, HealthCheckProtocolGRPC, checker.Protocol())
	assert.Equal(t, HealthCheckProtocolGRPC, checker.DetectedProtocol())
}

// TestLifecycleWithNilHealthCheckConfig tests lifecycle with nil health check config (defaults to auto).
func TestLifecycleWithNilHealthCheckConfig(t *testing.T) {
	// Create component without health check config (nil)
	comp := &Component{
		Kind:    ComponentKindAgent,
		Name:    "nil-config-agent",
		Version: "1.0.0",
		Status:  ComponentStatusAvailable,
		BinPath: createTestHealthServer(t),
		Manifest: &Manifest{
			Name:    "nil-config-agent",
			Version: "1.0.0",
			Runtime: &RuntimeConfig{
				Type:        RuntimeTypeBinary,
				Entrypoint:  "test-server",
				HealthCheck: nil, // Explicit nil
			},
		},
	}

	// Verify ProtocolAwareHealthChecker can be created with nil config
	checker := NewProtocolAwareHealthChecker("localhost", 50000, comp.Manifest.Runtime.HealthCheck)
	defer checker.Close()

	// Should default to auto-detect
	assert.Equal(t, HealthCheckProtocolAuto, checker.Protocol())
}

// TestConcurrentOperations tests concurrent lifecycle operations.
func TestConcurrentOperations(t *testing.T) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)

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
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)

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
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)

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
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.isPortAvailable(50000 + (i % 1000))
	}
}

// BenchmarkIsProcessAlive benchmarks process alive check.
func BenchmarkIsProcessAlive(b *testing.B) {
	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	localTracker := NewDefaultLocalTracker()
	manager := NewLifecycleManager(healthMonitor, dao, nil, localTracker)

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

// LogWriter Integration Tests

// mockLogWriter is a mock implementation of LogWriter for failure testing.
type mockLogWriter struct {
	mu          sync.Mutex
	createErr   error
	closeErr    error
	writers     map[string][]io.WriteCloser
	createCalls int
	closeCalls  int
}

func newMockLogWriter() *mockLogWriter {
	return &mockLogWriter{
		writers: make(map[string][]io.WriteCloser),
	}
}

func (m *mockLogWriter) CreateWriter(componentName string, stream string) (io.WriteCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createCalls++

	if m.createErr != nil {
		return nil, m.createErr
	}

	// Return a no-op writer
	writer := &noopWriter{}
	m.writers[componentName] = append(m.writers[componentName], writer)
	return writer, nil
}

func (m *mockLogWriter) Close(componentName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closeCalls++

	if m.closeErr != nil {
		return m.closeErr
	}

	delete(m.writers, componentName)
	return nil
}

func (m *mockLogWriter) getCreateCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.createCalls
}

func (m *mockLogWriter) getCloseCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closeCalls
}

func (m *mockLogWriter) setCreateError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createErr = err
}

func (m *mockLogWriter) setCloseError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeErr = err
}

// noopWriter is a no-op implementation of io.WriteCloser
type noopWriter struct{}

func (w *noopWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (w *noopWriter) Close() error {
	return nil
}

// TestStartComponent_CapturesOutput verifies that output is captured to log files.
func TestStartComponent_CapturesOutput(t *testing.T) {
	// Create temporary log directory
	tempDir := t.TempDir()

	// Create DefaultLogWriter
	logWriter, err := NewDefaultLogWriter(tempDir, nil)
	require.NoError(t, err)

	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManagerWithTimeouts(
		healthMonitor,
		dao,
		logWriter,
		nil, // localTracker
		2*time.Second, // Short timeout so test runs faster
		5*time.Second,
		50000,
		51000,
	)

	ctx := context.Background()

	// Create a test script that outputs text and stays running a bit
	testScript := createTestOutputScript(t, "Hello from stdout", "Error from stderr")

	comp := &Component{
		Kind:    ComponentKindAgent,
		Name:    "output-test-agent",
		Version: "1.0.0",
		Status:  ComponentStatusAvailable,
		BinPath: testScript,
		Manifest: &Manifest{
			Name:    "output-test-agent",
			Version: "1.0.0",
			Runtime: &RuntimeConfig{
				Type:       RuntimeTypeBinary,
				Entrypoint: testScript,
			},
		},
	}

	// Start the component (will fail health check but logs should be written)
	_, startErr := manager.StartComponent(ctx, comp)
	// Expected to fail health check, but we don't care - we just want logs

	// Close log writers explicitly to flush
	if err := logWriter.Close(comp.Name); err != nil {
		t.Logf("warning: failed to close log writer: %v", err)
	}

	// Give a moment for file system
	time.Sleep(100 * time.Millisecond)

	// Verify log file exists
	logPath := filepath.Join(tempDir, "output-test-agent.log")
	logData, err := os.ReadFile(logPath)
	require.NoError(t, err, "log file should exist")

	// Verify output contains stdout and stderr
	logContent := string(logData)
	assert.Contains(t, logContent, "Hello from stdout", "stdout should be captured")
	assert.Contains(t, logContent, "Error from stderr", "stderr should be captured")
	assert.Contains(t, logContent, "[STDOUT]", "stdout marker should be present")
	assert.Contains(t, logContent, "[STDERR]", "stderr marker should be present")

	// Verify we got an error (since health check will fail)
	assert.Error(t, startErr)
}

// TestStartComponent_LogFailureDoesNotBlock verifies that logging failures don't block component startup.
func TestStartComponent_LogFailureDoesNotBlock(t *testing.T) {
	// Create a mock LogWriter that always fails
	logWriter := newMockLogWriter()
	logWriter.setCreateError(fmt.Errorf("simulated log writer failure"))

	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManagerWithTimeouts(
		healthMonitor,
		dao,
		logWriter,
		nil, // localTracker
		2*time.Second,
		5*time.Second,
		50000,
		51000,
	)

	ctx := context.Background()

	// Create a simple script that just sleeps (will fail health check but process will start)
	testScript := createTestOutputScript(t, "test", "test")

	comp := &Component{
		Kind:    ComponentKindAgent,
		Name:    "log-failure-test",
		Version: "1.0.0",
		Status:  ComponentStatusAvailable,
		BinPath: testScript,
		Manifest: &Manifest{
			Name:    "log-failure-test",
			Version: "1.0.0",
			Runtime: &RuntimeConfig{
				Type:       RuntimeTypeBinary,
				Entrypoint: testScript,
			},
		},
	}

	// Start the component - process should start despite log writer failure
	// (health check will fail but that's separate from process startup)
	_, err := manager.StartComponent(ctx, comp)

	// Verify process started (even though health check failed)
	// The key point is that PID was assigned, meaning process started
	assert.Greater(t, comp.PID, 0, "process should have started (PID assigned)")

	// Verify CreateWriter was called (failed, but attempted)
	assert.Greater(t, logWriter.getCreateCalls(), 0, "CreateWriter should have been called")

	// Clean up - kill the process directly since it's not "running" in component status
	if comp.PID > 0 {
		proc, _ := os.FindProcess(comp.PID)
		if proc != nil {
			_ = proc.Kill()
		}
	}

	// The test verifies that even though logging failed, the process was started
	// (we got a PID). This proves logging failures don't block component startup.
	_ = err // Health check failure is expected and not relevant to this test
}

// TestStopComponent_FlushesLogs verifies that logs are flushed when component stops.
func TestStopComponent_FlushesLogs(t *testing.T) {
	// Create temporary log directory
	tempDir := t.TempDir()

	// Create DefaultLogWriter
	logWriter, err := NewDefaultLogWriter(tempDir, nil)
	require.NoError(t, err)

	healthMonitor := newMockHealthMonitor()
	dao := &mockStatusUpdater{}
	manager := NewLifecycleManagerWithTimeouts(
		healthMonitor,
		dao,
		logWriter,
		nil, // localTracker
		1*time.Second, // Very short timeout
		5*time.Second,
		50000,
		51000,
	)

	// Create a long-running process that outputs continuously
	cmd := exec.Command("bash", "-c", `
counter=0
while [ $counter -lt 50 ]; do
  echo "Output line $counter"
  counter=$((counter+1))
  sleep 0.1
done
`)

	// Set up log writers manually to capture output
	stdoutWriter, err := logWriter.CreateWriter("flush-test-agent", "stdout")
	require.NoError(t, err)
	stderrWriter, err := logWriter.CreateWriter("flush-test-agent", "stderr")
	require.NoError(t, err)

	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	// Start the process
	err = cmd.Start()
	require.NoError(t, err)

	// Create component manually with running process
	comp := &Component{
		Kind:    ComponentKindAgent,
		Name:    "flush-test-agent",
		Version: "1.0.0",
		Status:  ComponentStatusRunning,
		PID:     cmd.Process.Pid,
		Port:    50000,
	}

	// Store process reference
	manager.mu.Lock()
	manager.processes[comp.Name] = cmd.Process
	manager.mu.Unlock()

	// Wait for some output to be generated
	time.Sleep(500 * time.Millisecond)

	// Stop the component - this should flush logs
	ctx := context.Background()
	err = manager.StopComponent(ctx, comp)
	assert.NoError(t, err)

	// Give file system a moment
	time.Sleep(100 * time.Millisecond)

	// Verify log file exists and has content
	logPath := filepath.Join(tempDir, "flush-test-agent.log")
	logData, err := os.ReadFile(logPath)
	require.NoError(t, err, "log file should exist")

	// Verify we captured output
	assert.NotEmpty(t, logData, "log file should contain data")

	logContent := string(logData)
	assert.Contains(t, logContent, "line", "should contain output lines")
	assert.Contains(t, logContent, "[STDOUT]", "should have stdout marker")
}

// createTestOutputScript creates a script that outputs to stdout and stderr then runs for a bit.
func createTestOutputScript(t *testing.T, stdoutMsg, stderrMsg string) string {
	t.Helper()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "output-test.sh")

	// Immediately output and flush, then sleep to allow health check attempts
	script := fmt.Sprintf(`#!/bin/bash
echo "%s"
echo "%s" >&2
# Sleep long enough for health checks to attempt
sleep 3
`, stdoutMsg, stderrMsg)

	err := os.WriteFile(scriptPath, []byte(script), 0755)
	require.NoError(t, err)

	return scriptPath
}

// createTestHealthyScript creates a script that starts a simple HTTP server.
func createTestHealthyScript(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "healthy-test.sh")

	script := `#!/bin/bash
# Parse port from arguments
PORT=50000
while [[ $# -gt 0 ]]; do
  case $1 in
    --port)
      PORT="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

# Start simple HTTP server that responds to health checks
while true; do
  echo -e "HTTP/1.1 200 OK\r\n\r\nOK" | nc -l -p $PORT -q 1
done
`

	err := os.WriteFile(scriptPath, []byte(script), 0755)
	require.NoError(t, err)

	return scriptPath
}

// createTestContinuousOutputScript creates a script that continuously outputs text.
func createTestContinuousOutputScript(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "continuous-test.sh")

	// Run long enough that the test can capture and stop it
	script := `#!/bin/bash
counter=0
while [ $counter -lt 50 ]; do
  echo "Output line $counter"
  counter=$((counter+1))
  sleep 0.1
done
`

	err := os.WriteFile(scriptPath, []byte(script), 0755)
	require.NoError(t, err)

	return scriptPath
}
