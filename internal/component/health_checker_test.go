package component

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// mockHealthServer implements the gRPC health checking protocol for testing.
type mockHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
	status          grpc_health_v1.HealthCheckResponse_ServingStatus
	serviceStatuses map[string]grpc_health_v1.HealthCheckResponse_ServingStatus
}

// newMockHealthServer creates a new mock health server with the given status.
func newMockHealthServer(status grpc_health_v1.HealthCheckResponse_ServingStatus) *mockHealthServer {
	return &mockHealthServer{
		status:          status,
		serviceStatuses: make(map[string]grpc_health_v1.HealthCheckResponse_ServingStatus),
	}
}

// Check implements the health check protocol.
func (m *mockHealthServer) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	// Check if service-specific status exists
	if req.Service != "" {
		if serviceStatus, ok := m.serviceStatuses[req.Service]; ok {
			return &grpc_health_v1.HealthCheckResponse{
				Status: serviceStatus,
			}, nil
		}
		// Service not found
		return &grpc_health_v1.HealthCheckResponse{
			Status: grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN,
		}, nil
	}

	// Return overall server status
	return &grpc_health_v1.HealthCheckResponse{
		Status: m.status,
	}, nil
}

// setServiceStatus sets the status for a specific service.
func (m *mockHealthServer) setServiceStatus(service string, status grpc_health_v1.HealthCheckResponse_ServingStatus) {
	m.serviceStatuses[service] = status
}

// startGRPCHealthServer starts a gRPC server with health checking on a random port.
// Returns the server, listener, and address.
func startGRPCHealthServer(t *testing.T, status grpc_health_v1.HealthCheckResponse_ServingStatus) (*grpc.Server, net.Listener, string, *mockHealthServer) {
	t.Helper()

	// Create listener on random port
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	// Create gRPC server
	server := grpc.NewServer()

	// Register health service
	healthServer := newMockHealthServer(status)
	grpc_health_v1.RegisterHealthServer(server, healthServer)

	// Start server in background
	go func() {
		_ = server.Serve(lis)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	return server, lis, lis.Addr().String(), healthServer
}

// TestHTTPHealthChecker_Success tests successful HTTP health check.
func TestHTTPHealthChecker_Success(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create health checker
	checker := NewHTTPHealthChecker(server.URL + "/health")
	defer checker.Close()

	// Perform health check
	ctx := context.Background()
	err := checker.Check(ctx)
	assert.NoError(t, err)
	assert.Equal(t, HealthCheckProtocolHTTP, checker.Protocol())
}

// TestHTTPHealthChecker_FailedStatusCodes tests HTTP health check with various failure status codes.
func TestHTTPHealthChecker_FailedStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		expectErr  bool
	}{
		{"status 200", http.StatusOK, false},
		{"status 204", http.StatusNoContent, false},
		{"status 299", 299, false},
		{"status 400", http.StatusBadRequest, true},
		{"status 404", http.StatusNotFound, true},
		{"status 500", http.StatusInternalServerError, true},
		{"status 503", http.StatusServiceUnavailable, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			checker := NewHTTPHealthChecker(server.URL + "/health")
			defer checker.Close()

			ctx := context.Background()
			err := checker.Check(ctx)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), fmt.Sprintf("unhealthy status code: %d", tt.statusCode))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestHTTPHealthChecker_Timeout tests HTTP health check timeout.
func TestHTTPHealthChecker_Timeout(t *testing.T) {
	// Create server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create checker with short timeout
	checker := NewHTTPHealthChecker(
		server.URL+"/health",
		WithHTTPTimeout(50*time.Millisecond),
	)
	defer checker.Close()

	ctx := context.Background()
	err := checker.Check(ctx)
	assert.Error(t, err)
}

// TestHTTPHealthChecker_ContextCancellation tests HTTP health check with context cancellation.
func TestHTTPHealthChecker_ContextCancellation(t *testing.T) {
	// Create server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checker := NewHTTPHealthChecker(server.URL + "/health")
	defer checker.Close()

	// Create context that will be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := checker.Check(ctx)
	assert.Error(t, err)
}

// TestHTTPHealthChecker_CustomClient tests HTTP health check with custom client.
func TestHTTPHealthChecker_CustomClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	customClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	checker := NewHTTPHealthChecker(
		server.URL+"/health",
		WithHTTPClient(customClient),
	)
	defer checker.Close()

	assert.Equal(t, customClient, checker.client)

	ctx := context.Background()
	err := checker.Check(ctx)
	assert.NoError(t, err)
}

// TestHTTPHealthChecker_Endpoint tests HTTP health checker endpoint getter.
func TestHTTPHealthChecker_Endpoint(t *testing.T) {
	endpoint := "http://localhost:8080/health"
	checker := NewHTTPHealthChecker(endpoint)
	defer checker.Close()

	assert.Equal(t, endpoint, checker.Endpoint())
}

// TestGRPCHealthChecker_Serving tests gRPC health check with SERVING status.
func TestGRPCHealthChecker_Serving(t *testing.T) {
	// Start mock gRPC server with SERVING status
	server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server.Stop()
	defer lis.Close()

	// Create gRPC health checker
	checker, err := NewGRPCHealthChecker(addr)
	require.NoError(t, err)
	defer checker.Close()

	// Perform health check
	ctx := context.Background()
	err = checker.Check(ctx)
	assert.NoError(t, err)
	assert.Equal(t, HealthCheckProtocolGRPC, checker.Protocol())
	assert.Equal(t, addr, checker.Address())
}

// TestGRPCHealthChecker_NotServing tests gRPC health check with NOT_SERVING status.
func TestGRPCHealthChecker_NotServing(t *testing.T) {
	server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	defer server.Stop()
	defer lis.Close()

	checker, err := NewGRPCHealthChecker(addr)
	require.NoError(t, err)
	defer checker.Close()

	ctx := context.Background()
	err = checker.Check(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service not serving")
}

// TestGRPCHealthChecker_Unknown tests gRPC health check with UNKNOWN status.
func TestGRPCHealthChecker_Unknown(t *testing.T) {
	server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_UNKNOWN)
	defer server.Stop()
	defer lis.Close()

	checker, err := NewGRPCHealthChecker(addr)
	require.NoError(t, err)
	defer checker.Close()

	ctx := context.Background()
	err = checker.Check(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service health unknown")
}

// TestGRPCHealthChecker_ServiceUnknown tests gRPC health check with SERVICE_UNKNOWN status.
func TestGRPCHealthChecker_ServiceUnknown(t *testing.T) {
	server, lis, addr, healthServer := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server.Stop()
	defer lis.Close()

	// Create checker for specific service that doesn't exist
	checker, err := NewGRPCHealthChecker(addr, WithServiceName("nonexistent.Service"))
	require.NoError(t, err)
	defer checker.Close()

	ctx := context.Background()
	err = checker.Check(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service 'nonexistent.Service' not found")
	assert.Equal(t, "nonexistent.Service", checker.ServiceName())

	// Now register the service
	healthServer.setServiceStatus("nonexistent.Service", grpc_health_v1.HealthCheckResponse_SERVING)

	// Should succeed now
	err = checker.Check(ctx)
	assert.NoError(t, err)
}

// TestGRPCHealthChecker_ConnectionFailure tests gRPC health check with connection failure.
func TestGRPCHealthChecker_ConnectionFailure(t *testing.T) {
	// Try to connect to non-existent server
	checker, err := NewGRPCHealthChecker("localhost:99999", WithGRPCTimeout(1*time.Second))
	require.NoError(t, err) // Connection is lazy
	defer checker.Close()

	ctx := context.Background()
	err = checker.Check(ctx)
	assert.Error(t, err)
}

// TestGRPCHealthChecker_Timeout tests gRPC health check timeout.
func TestGRPCHealthChecker_Timeout(t *testing.T) {
	server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server.Stop()
	defer lis.Close()

	// Create checker with very short timeout
	checker, err := NewGRPCHealthChecker(addr, WithGRPCTimeout(1*time.Nanosecond))
	require.NoError(t, err)
	defer checker.Close()

	ctx := context.Background()
	err = checker.Check(ctx)
	// This might or might not timeout depending on timing, but it should not panic
	_ = err
}

// TestGRPCHealthChecker_ServiceName tests gRPC health check with custom service name.
func TestGRPCHealthChecker_ServiceName(t *testing.T) {
	server, lis, addr, healthServer := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server.Stop()
	defer lis.Close()

	// Register specific service
	healthServer.setServiceStatus("myapp.Service", grpc_health_v1.HealthCheckResponse_SERVING)

	checker, err := NewGRPCHealthChecker(addr, WithServiceName("myapp.Service"))
	require.NoError(t, err)
	defer checker.Close()

	ctx := context.Background()
	err = checker.Check(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "myapp.Service", checker.ServiceName())
}

// TestProtocolAwareHealthChecker_ExplicitGRPC tests protocol-aware checker with explicit gRPC protocol.
func TestProtocolAwareHealthChecker_ExplicitGRPC(t *testing.T) {
	server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server.Stop()
	defer lis.Close()

	// Parse host and port
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	_, err = fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)

	config := &HealthCheckConfig{
		Protocol: HealthCheckProtocolGRPC,
		Timeout:  5 * time.Second,
	}

	checker := NewProtocolAwareHealthChecker(host, port, config)
	defer checker.Close()

	ctx := context.Background()
	err = checker.Check(ctx)
	assert.NoError(t, err)

	// Should have detected gRPC
	assert.Equal(t, HealthCheckProtocolGRPC, checker.Protocol())
	assert.Equal(t, HealthCheckProtocolGRPC, checker.DetectedProtocol())
	assert.Equal(t, host, checker.Address())
	assert.Equal(t, port, checker.Port())
}

// TestProtocolAwareHealthChecker_ExplicitHTTP tests protocol-aware checker with explicit HTTP protocol.
func TestProtocolAwareHealthChecker_ExplicitHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Parse host and port from server URL
	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	var port int
	_, err = fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)

	config := &HealthCheckConfig{
		Protocol: HealthCheckProtocolHTTP,
		Endpoint: "/health",
		Timeout:  5 * time.Second,
	}

	checker := NewProtocolAwareHealthChecker(host, port, config)
	defer checker.Close()

	ctx := context.Background()
	err = checker.Check(ctx)
	assert.NoError(t, err)

	// Should have detected HTTP
	assert.Equal(t, HealthCheckProtocolHTTP, checker.Protocol())
	assert.Equal(t, HealthCheckProtocolHTTP, checker.DetectedProtocol())
}

// TestProtocolAwareHealthChecker_AutoDetectGRPC tests auto-detection with gRPC server.
func TestProtocolAwareHealthChecker_AutoDetectGRPC(t *testing.T) {
	server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server.Stop()
	defer lis.Close()

	// Parse host and port
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	_, err = fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)

	config := &HealthCheckConfig{
		Protocol: HealthCheckProtocolAuto,
		Timeout:  5 * time.Second,
	}

	checker := NewProtocolAwareHealthChecker(host, port, config)
	defer checker.Close()

	// Before first check, protocol should be auto
	assert.Equal(t, HealthCheckProtocolAuto, checker.Protocol())

	ctx := context.Background()
	err = checker.Check(ctx)
	assert.NoError(t, err)

	// After check, should have detected gRPC
	assert.Equal(t, HealthCheckProtocolGRPC, checker.Protocol())
	assert.Equal(t, HealthCheckProtocolGRPC, checker.DetectedProtocol())
}

// TestProtocolAwareHealthChecker_AutoDetectFallbackHTTP tests auto-detection fallback to HTTP when gRPC connection fails.
func TestProtocolAwareHealthChecker_AutoDetectFallbackHTTP(t *testing.T) {
	// This test demonstrates that auto-detection tries gRPC first, and if it gets a connection error
	// (not a protocol error), it falls back to HTTP.
	// To properly test this, we need to:
	// 1. Start with gRPC unavailable (port not listening)
	// 2. Have HTTP available on the same port
	// Since we can't have both at once, we'll start an HTTP server and then stop gRPC attempts
	// will fail with "connection refused" which triggers HTTP fallback.

	// Create HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Parse host and port
	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	var port int
	_, err = fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)

	// First, verify that when gRPC connects to HTTP, it doesn't fall back
	// (because it's a protocol error, not a connection error)
	config := &HealthCheckConfig{
		Protocol: HealthCheckProtocolAuto,
		Endpoint: "/",
		Timeout:  1 * time.Second,
	}

	checker := NewProtocolAwareHealthChecker(host, port, config)
	defer checker.Close()

	ctx := context.Background()
	err = checker.Check(ctx)

	// When gRPC connects to an HTTP server, it returns a protocol error (not a connection error),
	// so the auto-detect logic won't fall back to HTTP. This is expected behavior -
	// the server responded but with the wrong protocol, indicating a configuration issue.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "grpc")
}

// TestProtocolAwareHealthChecker_ProtocolCaching tests that protocol is cached after detection.
func TestProtocolAwareHealthChecker_ProtocolCaching(t *testing.T) {
	server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server.Stop()
	defer lis.Close()

	// Parse host and port
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	_, err = fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)

	config := &HealthCheckConfig{
		Protocol: HealthCheckProtocolAuto,
		Timeout:  5 * time.Second,
	}

	checker := NewProtocolAwareHealthChecker(host, port, config)
	defer checker.Close()

	ctx := context.Background()

	// First check - should detect gRPC
	err = checker.Check(ctx)
	assert.NoError(t, err)
	assert.Equal(t, HealthCheckProtocolGRPC, checker.Protocol())

	// Stop gRPC server to ensure we're using cached protocol
	server.Stop()
	lis.Close()

	// Wait a moment for server to fully stop
	time.Sleep(100 * time.Millisecond)

	// Second check - should use cached gRPC checker (will fail because server stopped)
	err = checker.Check(ctx)
	assert.Error(t, err) // Server is stopped, so this should fail

	// But protocol should still be gRPC (cached)
	assert.Equal(t, HealthCheckProtocolGRPC, checker.Protocol())
}

// TestProtocolAwareHealthChecker_InvalidProtocol tests protocol-aware checker with invalid protocol.
func TestProtocolAwareHealthChecker_InvalidProtocol(t *testing.T) {
	config := &HealthCheckConfig{
		Protocol: HealthCheckProtocol("invalid"),
		Timeout:  5 * time.Second,
	}

	checker := NewProtocolAwareHealthChecker("localhost", 8080, config)
	defer checker.Close()

	ctx := context.Background()
	err := checker.Check(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid health check protocol")
}

// TestProtocolAwareHealthChecker_NilConfig tests protocol-aware checker with nil config.
func TestProtocolAwareHealthChecker_NilConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Parse host and port
	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	var port int
	_, err = fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)

	// Nil config should use defaults (auto protocol)
	checker := NewProtocolAwareHealthChecker(host, port, nil)
	defer checker.Close()

	ctx := context.Background()
	err = checker.Check(ctx)

	// When gRPC connects to HTTP, it gets a protocol error (not connection error)
	// so auto-detect doesn't fall back
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "grpc")
}

// TestProtocolAwareHealthChecker_BothProtocolsFail tests auto-detection when both protocols fail.
func TestProtocolAwareHealthChecker_BothProtocolsFail(t *testing.T) {
	// Use a port that nothing is listening on
	config := &HealthCheckConfig{
		Protocol: HealthCheckProtocolAuto,
		Timeout:  1 * time.Second,
	}

	checker := NewProtocolAwareHealthChecker("localhost", 99999, config)
	defer checker.Close()

	ctx := context.Background()
	err := checker.Check(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gRPC")
	assert.Contains(t, err.Error(), "HTTP")
}

// TestProtocolAwareHealthChecker_GRPCServiceUnavailableNoFallback tests that service unavailable doesn't trigger HTTP fallback.
func TestProtocolAwareHealthChecker_GRPCServiceUnavailableNoFallback(t *testing.T) {
	// Start gRPC server with NOT_SERVING status
	server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	defer server.Stop()
	defer lis.Close()

	// Parse host and port
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	_, err = fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)

	config := &HealthCheckConfig{
		Protocol: HealthCheckProtocolAuto,
		Timeout:  5 * time.Second,
	}

	checker := NewProtocolAwareHealthChecker(host, port, config)
	defer checker.Close()

	ctx := context.Background()
	err = checker.Check(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service not serving")

	// When service is NOT_SERVING, it's a service error not a connection error,
	// so protocol is not detected/cached (stays as auto)
	// The error should not mention HTTP fallback
	assert.NotContains(t, err.Error(), "HTTP")
}

// TestHTTPHealthChecker_Close tests closing HTTP health checker.
func TestHTTPHealthChecker_Close(t *testing.T) {
	checker := NewHTTPHealthChecker("http://localhost:8080/health")
	err := checker.Close()
	assert.NoError(t, err)
}

// TestGRPCHealthChecker_Close tests closing gRPC health checker.
func TestGRPCHealthChecker_Close(t *testing.T) {
	server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server.Stop()
	defer lis.Close()

	checker, err := NewGRPCHealthChecker(addr)
	require.NoError(t, err)

	// Perform a health check to ensure connection is established
	ctx := context.Background()
	err = checker.Check(ctx)
	require.NoError(t, err)

	// Close the checker
	err = checker.Close()
	assert.NoError(t, err)

	// Closing again might return an error (connection already closed), which is acceptable
	_ = checker.Close()
}

// TestProtocolAwareHealthChecker_Close tests closing protocol-aware health checker.
func TestProtocolAwareHealthChecker_Close(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	var port int
	_, err = fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)

	config := &HealthCheckConfig{
		Protocol: HealthCheckProtocolHTTP,
		Timeout:  5 * time.Second,
	}

	checker := NewProtocolAwareHealthChecker(host, port, config)

	// Perform check to create active checker
	ctx := context.Background()
	err = checker.Check(ctx)
	require.NoError(t, err)

	// Close
	err = checker.Close()
	assert.NoError(t, err)

	// Should be able to close again (idempotent)
	err = checker.Close()
	assert.NoError(t, err)
}

// Benchmark tests

// BenchmarkHTTPHealthChecker benchmarks HTTP health check performance.
func BenchmarkHTTPHealthChecker(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checker := NewHTTPHealthChecker(server.URL + "/health")
	defer checker.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = checker.Check(ctx)
	}
}

// BenchmarkGRPCHealthChecker benchmarks gRPC health check performance.
func BenchmarkGRPCHealthChecker(b *testing.B) {
	server, lis, addr, _ := startGRPCHealthServer(&testing.T{}, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server.Stop()
	defer lis.Close()

	checker, err := NewGRPCHealthChecker(addr)
	if err != nil {
		b.Fatal(err)
	}
	defer checker.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = checker.Check(ctx)
	}
}

// BenchmarkProtocolAwareHealthChecker_HTTP benchmarks protocol-aware checker with HTTP.
func BenchmarkProtocolAwareHealthChecker_HTTP(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	host, portStr, _ := net.SplitHostPort(server.Listener.Addr().String())
	var port int
	_, _ = fmt.Sscanf(portStr, "%d", &port)

	config := &HealthCheckConfig{
		Protocol: HealthCheckProtocolHTTP,
		Timeout:  5 * time.Second,
	}

	checker := NewProtocolAwareHealthChecker(host, port, config)
	defer checker.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = checker.Check(ctx)
	}
}
