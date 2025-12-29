package component

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewHealthMonitor tests creating a new health monitor.
func TestNewHealthMonitor(t *testing.T) {
	monitor := NewHealthMonitor()

	assert.NotNil(t, monitor)
	assert.NotNil(t, monitor.client)
	assert.Equal(t, DefaultHealthCheckInterval, monitor.interval)
	assert.Equal(t, DefaultHealthCheckTimeout, monitor.timeout)
	assert.Equal(t, DefaultHealthCheckRetries, monitor.retries)
	assert.NotNil(t, monitor.components)
	assert.NotNil(t, monitor.callbacks)
	assert.False(t, monitor.running)
}

// TestNewHealthMonitorWithConfig tests creating a health monitor with custom config.
func TestNewHealthMonitorWithConfig(t *testing.T) {
	interval := 5 * time.Second
	timeout := 3 * time.Second
	retries := 5

	monitor := NewHealthMonitorWithConfig(interval, timeout, retries)

	assert.NotNil(t, monitor)
	assert.Equal(t, interval, monitor.interval)
	assert.Equal(t, timeout, monitor.timeout)
	assert.Equal(t, retries, monitor.retries)
}

// TestHealthMonitor_Start tests starting the health monitor.
func TestHealthMonitor_Start(t *testing.T) {
	monitor := NewHealthMonitor()
	ctx := context.Background()

	err := monitor.Start(ctx)
	require.NoError(t, err)
	assert.True(t, monitor.running)

	// Starting again should fail
	err = monitor.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	// Clean up
	err = monitor.Stop()
	assert.NoError(t, err)
}

// TestHealthMonitor_Stop tests stopping the health monitor.
func TestHealthMonitor_Stop(t *testing.T) {
	monitor := NewHealthMonitor()
	ctx := context.Background()

	// Stopping before starting should fail
	err := monitor.Stop()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")

	// Start and then stop
	err = monitor.Start(ctx)
	require.NoError(t, err)

	err = monitor.Stop()
	assert.NoError(t, err)
	assert.False(t, monitor.running)

	// Stopping again should fail
	err = monitor.Stop()
	assert.Error(t, err)
}

// TestHealthMonitor_CheckComponent_Success tests successful health check.
func TestHealthMonitor_CheckComponent_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	monitor := NewHealthMonitor()
	ctx := context.Background()

	err := monitor.CheckComponent(ctx, server.URL+"/health")
	assert.NoError(t, err)
}

// TestHealthMonitor_CheckComponent_Failure tests failed health check.
func TestHealthMonitor_CheckComponent_Failure(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		serverFunc     func(http.ResponseWriter, *http.Request)
		expectError    bool
		errorContains  string
	}{
		{
			name:       "status 500",
			statusCode: http.StatusInternalServerError,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectError:   true,
			errorContains: "status code: 500",
		},
		{
			name:       "status 404",
			statusCode: http.StatusNotFound,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			expectError:   true,
			errorContains: "status code: 404",
		},
		{
			name:       "status 200",
			statusCode: http.StatusOK,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			expectError: false,
		},
		{
			name:       "status 204",
			statusCode: http.StatusNoContent,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
			defer server.Close()

			monitor := NewHealthMonitor()
			ctx := context.Background()

			err := monitor.CheckComponent(ctx, server.URL+"/health")
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestHealthMonitor_CheckComponent_Timeout tests health check timeout.
func TestHealthMonitor_CheckComponent_Timeout(t *testing.T) {
	// Create server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	monitor := NewHealthMonitorWithConfig(
		1*time.Second,
		100*time.Millisecond, // Short timeout
		3,
	)

	ctx := context.Background()
	err := monitor.CheckComponent(ctx, server.URL+"/health")
	assert.Error(t, err)
}

// TestHealthMonitor_CheckComponent_InvalidURL tests health check with invalid URL.
func TestHealthMonitor_CheckComponent_InvalidURL(t *testing.T) {
	monitor := NewHealthMonitor()
	ctx := context.Background()

	err := monitor.CheckComponent(ctx, "http://invalid-host-that-does-not-exist:99999/health")
	assert.Error(t, err)
}

// TestHealthMonitor_CheckComponent_ContextCancellation tests context cancellation.
func TestHealthMonitor_CheckComponent_ContextCancellation(t *testing.T) {
	// Create server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	monitor := NewHealthMonitor()
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context immediately
	cancel()

	err := monitor.CheckComponent(ctx, server.URL+"/health")
	assert.Error(t, err)
}

// TestHealthMonitor_RegisterComponent tests registering a component.
func TestHealthMonitor_RegisterComponent(t *testing.T) {
	monitor := NewHealthMonitor()

	monitor.RegisterComponent("test-component", "http://localhost:8080/health")

	monitor.mu.RLock()
	comp, exists := monitor.components["test-component"]
	monitor.mu.RUnlock()

	assert.True(t, exists)
	assert.NotNil(t, comp)
	assert.Equal(t, "test-component", comp.name)
	assert.Equal(t, "http://localhost:8080/health", comp.healthEndpoint)
	assert.Equal(t, HealthStatusUnknown, comp.currentStatus)
	assert.Equal(t, 0, comp.failureCount)
}

// TestHealthMonitor_UnregisterComponent tests unregistering a component.
func TestHealthMonitor_UnregisterComponent(t *testing.T) {
	monitor := NewHealthMonitor()

	monitor.RegisterComponent("test-component", "http://localhost:8080/health")
	monitor.UnregisterComponent("test-component")

	monitor.mu.RLock()
	_, exists := monitor.components["test-component"]
	monitor.mu.RUnlock()

	assert.False(t, exists)
}

// TestHealthMonitor_GetHealth tests getting component health status.
func TestHealthMonitor_GetHealth(t *testing.T) {
	monitor := NewHealthMonitor()

	// Unknown component
	status := monitor.GetHealth("unknown-component")
	assert.Equal(t, HealthStatusUnknown, status)

	// Registered component
	monitor.RegisterComponent("test-component", "http://localhost:8080/health")
	status = monitor.GetHealth("test-component")
	assert.Equal(t, HealthStatusUnknown, status)

	// Update status
	monitor.mu.Lock()
	monitor.components["test-component"].currentStatus = HealthStatusHealthy
	monitor.mu.Unlock()

	status = monitor.GetHealth("test-component")
	assert.Equal(t, HealthStatusHealthy, status)
}

// TestHealthMonitor_OnStatusChange tests registering status change callbacks.
func TestHealthMonitor_OnStatusChange(t *testing.T) {
	monitor := NewHealthMonitor()

	var called bool
	var receivedName string
	var receivedOld, receivedNew HealthStatus

	callback := func(name string, oldStatus, newStatus HealthStatus) {
		called = true
		receivedName = name
		receivedOld = oldStatus
		receivedNew = newStatus
	}

	monitor.OnStatusChange(callback)

	// Trigger status change
	monitor.mu.Lock()
	monitor.notifyStatusChange("test-component", HealthStatusUnknown, HealthStatusHealthy)
	monitor.mu.Unlock()

	// Give callback goroutine time to execute
	time.Sleep(100 * time.Millisecond)

	assert.True(t, called)
	assert.Equal(t, "test-component", receivedName)
	assert.Equal(t, HealthStatusUnknown, receivedOld)
	assert.Equal(t, HealthStatusHealthy, receivedNew)
}

// TestHealthMonitor_OnStatusChange_MultipleCallbacks tests multiple callbacks.
func TestHealthMonitor_OnStatusChange_MultipleCallbacks(t *testing.T) {
	monitor := NewHealthMonitor()

	var wg sync.WaitGroup
	wg.Add(3)

	callCount := 0
	var mu sync.Mutex

	for i := 0; i < 3; i++ {
		monitor.OnStatusChange(func(name string, oldStatus, newStatus HealthStatus) {
			mu.Lock()
			callCount++
			mu.Unlock()
			wg.Done()
		})
	}

	// Trigger status change
	monitor.mu.Lock()
	monitor.notifyStatusChange("test-component", HealthStatusUnknown, HealthStatusHealthy)
	monitor.mu.Unlock()

	// Wait for all callbacks
	wg.Wait()

	assert.Equal(t, 3, callCount)
}

// TestHealthMonitor_CheckComponentHealth tests checking component health.
func TestHealthMonitor_CheckComponentHealth(t *testing.T) {
	tests := []struct {
		name              string
		serverStatusCode  int
		retries           int
		expectedStatus    HealthStatus
		expectedFailCount int
	}{
		{
			name:              "healthy component",
			serverStatusCode:  http.StatusOK,
			retries:           3,
			expectedStatus:    HealthStatusHealthy,
			expectedFailCount: 0,
		},
		{
			name:              "unhealthy after retries",
			serverStatusCode:  http.StatusInternalServerError,
			retries:           3,
			expectedStatus:    HealthStatusUnhealthy,
			expectedFailCount: 3,
		},
		{
			name:              "single retry not enough",
			serverStatusCode:  http.StatusInternalServerError,
			retries:           5,
			expectedStatus:    HealthStatusUnknown,
			expectedFailCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatusCode)
			}))
			defer server.Close()

			monitor := NewHealthMonitorWithConfig(
				100*time.Millisecond,
				1*time.Second,
				tt.retries,
			)

			comp := &monitoredComponent{
				name:           "test-component",
				healthEndpoint: server.URL + "/health",
				currentStatus:  HealthStatusUnknown,
				failureCount:   0,
			}

			ctx := context.Background()

			// Check health the appropriate number of times
			checksNeeded := tt.expectedFailCount
			if tt.expectedStatus == HealthStatusHealthy {
				checksNeeded = 1
			}

			for i := 0; i < checksNeeded; i++ {
				monitor.checkComponentHealth(ctx, comp)
			}

			assert.Equal(t, tt.expectedStatus, comp.currentStatus)
			assert.Equal(t, tt.expectedFailCount, comp.failureCount)
		})
	}
}

// TestHealthMonitor_CheckComponentHealth_Transition tests health status transitions.
func TestHealthMonitor_CheckComponentHealth_Transition(t *testing.T) {
	callbackCalled := false
	var callbackOldStatus, callbackNewStatus HealthStatus
	var wg sync.WaitGroup

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	monitor := NewHealthMonitorWithConfig(
		100*time.Millisecond,
		1*time.Second,
		3,
	)

	wg.Add(1)
	monitor.OnStatusChange(func(name string, oldStatus, newStatus HealthStatus) {
		callbackCalled = true
		callbackOldStatus = oldStatus
		callbackNewStatus = newStatus
		wg.Done()
	})

	comp := &monitoredComponent{
		name:           "test-component",
		healthEndpoint: server.URL + "/health",
		currentStatus:  HealthStatusUnknown,
		failureCount:   0,
	}

	ctx := context.Background()
	monitor.checkComponentHealth(ctx, comp)

	// Wait for callback
	wg.Wait()

	assert.True(t, callbackCalled)
	assert.Equal(t, HealthStatusUnknown, callbackOldStatus)
	assert.Equal(t, HealthStatusHealthy, callbackNewStatus)
	assert.Equal(t, HealthStatusHealthy, comp.currentStatus)
	assert.Equal(t, 0, comp.failureCount)
}

// TestHealthMonitor_MonitorLoop tests the monitoring loop.
func TestHealthMonitor_MonitorLoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	monitor := NewHealthMonitorWithConfig(
		200*time.Millisecond, // Fast interval for testing
		1*time.Second,
		3,
	)

	monitor.RegisterComponent("test-component", server.URL+"/health")

	ctx := context.Background()
	err := monitor.Start(ctx)
	require.NoError(t, err)

	// Wait for a few checks
	time.Sleep(600 * time.Millisecond)

	// Component should be healthy
	status := monitor.GetHealth("test-component")
	assert.Equal(t, HealthStatusHealthy, status)

	// Stop monitor
	err = monitor.Stop()
	assert.NoError(t, err)
}

// TestHealthMonitor_MonitorLoop_StatusChanges tests status changes during monitoring.
func TestHealthMonitor_MonitorLoop_StatusChanges(t *testing.T) {
	healthyCount := 0
	mu := sync.Mutex{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if healthyCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		healthyCount++
	}))
	defer server.Close()

	monitor := NewHealthMonitorWithConfig(
		100*time.Millisecond,
		1*time.Second,
		3,
	)

	statusChanges := make([]HealthStatus, 0)
	var statusMu sync.Mutex

	monitor.OnStatusChange(func(name string, oldStatus, newStatus HealthStatus) {
		statusMu.Lock()
		statusChanges = append(statusChanges, newStatus)
		statusMu.Unlock()
	})

	monitor.RegisterComponent("test-component", server.URL+"/health")

	ctx := context.Background()
	err := monitor.Start(ctx)
	require.NoError(t, err)

	// Wait for monitoring to detect changes
	time.Sleep(800 * time.Millisecond)

	err = monitor.Stop()
	assert.NoError(t, err)

	statusMu.Lock()
	defer statusMu.Unlock()

	// Should have at least one status change
	assert.Greater(t, len(statusChanges), 0)
}

// TestHealthMonitor_ContextCancellation tests context cancellation during monitoring.
func TestHealthMonitor_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	monitor := NewHealthMonitor()
	monitor.RegisterComponent("test-component", server.URL+"/health")

	ctx, cancel := context.WithCancel(context.Background())

	err := monitor.Start(ctx)
	require.NoError(t, err)

	// Cancel context
	cancel()

	// Monitor should stop
	time.Sleep(100 * time.Millisecond)

	// Cleanup
	_ = monitor.Stop()
}

// TestHealthMonitor_ConcurrentAccess tests concurrent access to health monitor.
func TestHealthMonitor_ConcurrentAccess(t *testing.T) {
	monitor := NewHealthMonitor()

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent registrations
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := fmt.Sprintf("component-%d", id)
			monitor.RegisterComponent(name, fmt.Sprintf("http://localhost:%d/health", 8000+id))
		}(i)
	}

	wg.Wait()

	// Verify all components registered
	monitor.mu.RLock()
	count := len(monitor.components)
	monitor.mu.RUnlock()
	assert.Equal(t, iterations, count)

	// Concurrent reads
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := fmt.Sprintf("component-%d", id)
			status := monitor.GetHealth(name)
			assert.Equal(t, HealthStatusUnknown, status)
		}(i)
	}

	wg.Wait()

	// Concurrent unregistrations
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := fmt.Sprintf("component-%d", id)
			monitor.UnregisterComponent(name)
		}(i)
	}

	wg.Wait()

	// Verify all components unregistered
	monitor.mu.RLock()
	count = len(monitor.components)
	monitor.mu.RUnlock()
	assert.Equal(t, 0, count)
}

// Benchmark tests

// BenchmarkCheckComponent benchmarks health check performance.
func BenchmarkCheckComponent(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	monitor := NewHealthMonitor()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = monitor.CheckComponent(ctx, server.URL+"/health")
	}
}

// BenchmarkRegisterComponent benchmarks component registration.
func BenchmarkRegisterComponent(b *testing.B) {
	monitor := NewHealthMonitor()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("component-%d", i)
		monitor.RegisterComponent(name, "http://localhost:8080/health")
	}
}

// BenchmarkGetHealth benchmarks getting health status.
func BenchmarkGetHealth(b *testing.B) {
	monitor := NewHealthMonitor()
	monitor.RegisterComponent("test-component", "http://localhost:8080/health")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = monitor.GetHealth("test-component")
	}
}

// BenchmarkConcurrentHealthChecks benchmarks concurrent health checks.
func BenchmarkConcurrentHealthChecks(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	monitor := NewHealthMonitor()
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = monitor.CheckComponent(ctx, server.URL+"/health")
		}
	})
}
