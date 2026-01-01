package component

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// TestNewDefaultRemoteProber tests creation of a new DefaultRemoteProber.
func TestNewDefaultRemoteProber(t *testing.T) {
	prober := NewDefaultRemoteProber()
	require.NotNil(t, prober)
	assert.NotNil(t, prober.components)
	assert.Len(t, prober.components, 0)
}

// TestRemoteProber_LoadConfig_ValidConfig tests loading valid remote component configurations.
func TestRemoteProber_LoadConfig_ValidConfig(t *testing.T) {
	prober := NewDefaultRemoteProber()

	agents := map[string]RemoteComponentConfig{
		"agent1": {
			Address:     "localhost:50051",
			HealthCheck: "grpc",
			Timeout:     5 * time.Second,
		},
		"agent2": {
			Address:     "agent2:50052",
			HealthCheck: "http",
			Timeout:     3 * time.Second,
		},
	}

	tools := map[string]RemoteComponentConfig{
		"tool1": {
			Address:     "tool1:50053",
			HealthCheck: "grpc",
		},
	}

	plugins := map[string]RemoteComponentConfig{
		"plugin1": {
			Address: "plugin1:50054",
			// Should default to grpc
		},
	}

	err := prober.LoadConfig(agents, tools, plugins)
	require.NoError(t, err)

	prober.mu.RLock()
	defer prober.mu.RUnlock()

	// Verify agents
	assert.Len(t, prober.components[ComponentKindAgent], 2)
	assert.Equal(t, "localhost:50051", prober.components[ComponentKindAgent]["agent1"].Address)
	assert.Equal(t, "grpc", prober.components[ComponentKindAgent]["agent1"].HealthCheck)
	assert.Equal(t, "http", prober.components[ComponentKindAgent]["agent2"].HealthCheck)

	// Verify tools
	assert.Len(t, prober.components[ComponentKindTool], 1)
	assert.Equal(t, "tool1:50053", prober.components[ComponentKindTool]["tool1"].Address)

	// Verify plugins
	assert.Len(t, prober.components[ComponentKindPlugin], 1)
	assert.Equal(t, "plugin1:50054", prober.components[ComponentKindPlugin]["plugin1"].Address)
	// Should default to grpc
	assert.Equal(t, "grpc", prober.components[ComponentKindPlugin]["plugin1"].HealthCheck)
	// Should default timeout
	assert.Equal(t, DefaultHealthCheckTimeout, prober.components[ComponentKindPlugin]["plugin1"].Timeout)
}

// TestRemoteProber_LoadConfig_EmptyConfig tests loading empty configurations.
func TestRemoteProber_LoadConfig_EmptyConfig(t *testing.T) {
	prober := NewDefaultRemoteProber()

	err := prober.LoadConfig(
		map[string]RemoteComponentConfig{},
		map[string]RemoteComponentConfig{},
		map[string]RemoteComponentConfig{},
	)
	require.NoError(t, err)

	prober.mu.RLock()
	defer prober.mu.RUnlock()

	assert.Len(t, prober.components[ComponentKindAgent], 0)
	assert.Len(t, prober.components[ComponentKindTool], 0)
	assert.Len(t, prober.components[ComponentKindPlugin], 0)
}

// TestRemoteProber_LoadConfig_MissingAddress tests validation of missing address.
func TestRemoteProber_LoadConfig_MissingAddress(t *testing.T) {
	prober := NewDefaultRemoteProber()

	agents := map[string]RemoteComponentConfig{
		"invalid": {
			HealthCheck: "grpc",
		},
	}

	err := prober.LoadConfig(agents, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "address is required")
}

// TestRemoteProber_LoadConfig_InvalidHealthCheck tests validation of invalid health check protocol.
func TestRemoteProber_LoadConfig_InvalidHealthCheck(t *testing.T) {
	prober := NewDefaultRemoteProber()

	agents := map[string]RemoteComponentConfig{
		"invalid": {
			Address:     "localhost:50051",
			HealthCheck: "invalid-protocol",
		},
	}

	err := prober.LoadConfig(agents, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid health check protocol")
}

// TestRemoteProber_LoadConfig_TLSValidation tests TLS configuration validation.
func TestRemoteProber_LoadConfig_TLSValidation(t *testing.T) {
	tests := []struct {
		name      string
		tlsConfig *TLSConfig
		wantErr   bool
		errMsg    string
	}{
		{
			name: "valid TLS with cert and key",
			tlsConfig: &TLSConfig{
				Enabled:  true,
				CertFile: "/path/to/cert.pem",
				KeyFile:  "/path/to/key.pem",
			},
			wantErr: false,
		},
		{
			name: "valid TLS with only CA",
			tlsConfig: &TLSConfig{
				Enabled: true,
				CAFile:  "/path/to/ca.pem",
			},
			wantErr: false,
		},
		{
			name: "invalid TLS missing key",
			tlsConfig: &TLSConfig{
				Enabled:  true,
				CertFile: "/path/to/cert.pem",
			},
			wantErr: true,
			errMsg:  "both cert_file and key_file are required",
		},
		{
			name: "invalid TLS missing cert",
			tlsConfig: &TLSConfig{
				Enabled: true,
				KeyFile: "/path/to/key.pem",
			},
			wantErr: true,
			errMsg:  "both cert_file and key_file are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prober := NewDefaultRemoteProber()

			agents := map[string]RemoteComponentConfig{
				"test": {
					Address: "localhost:50051",
					TLS:     tt.tlsConfig,
				},
			}

			err := prober.LoadConfig(agents, nil, nil)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestRemoteProber_LoadConfig_ReplacesPreviousConfig tests that LoadConfig replaces previous configuration.
func TestRemoteProber_LoadConfig_ReplacesPreviousConfig(t *testing.T) {
	prober := NewDefaultRemoteProber()

	// Load initial config
	agents1 := map[string]RemoteComponentConfig{
		"agent1": {Address: "localhost:50051"},
	}
	err := prober.LoadConfig(agents1, nil, nil)
	require.NoError(t, err)

	prober.mu.RLock()
	assert.Len(t, prober.components[ComponentKindAgent], 1)
	prober.mu.RUnlock()

	// Load new config (should replace)
	agents2 := map[string]RemoteComponentConfig{
		"agent2": {Address: "localhost:50052"},
		"agent3": {Address: "localhost:50053"},
	}
	err = prober.LoadConfig(agents2, nil, nil)
	require.NoError(t, err)

	prober.mu.RLock()
	defer prober.mu.RUnlock()

	assert.Len(t, prober.components[ComponentKindAgent], 2)
	_, hasAgent1 := prober.components[ComponentKindAgent]["agent1"]
	assert.False(t, hasAgent1, "agent1 should be removed")
	_, hasAgent2 := prober.components[ComponentKindAgent]["agent2"]
	assert.True(t, hasAgent2, "agent2 should be present")
}

// TestRemoteProber_Probe_GRPCSuccess tests successful gRPC health probe.
func TestRemoteProber_Probe_GRPCSuccess(t *testing.T) {
	// Start mock gRPC server
	server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server.Stop()
	defer lis.Close()

	prober := NewDefaultRemoteProber()
	ctx := context.Background()

	state, err := prober.Probe(ctx, addr)
	require.NoError(t, err)
	assert.True(t, state.Healthy)
	assert.Equal(t, addr, state.Address)
	assert.Empty(t, state.Error)
	assert.Greater(t, state.ResponseTime, time.Duration(0))
	assert.False(t, state.LastCheck.IsZero())
}

// TestRemoteProber_Probe_GRPCFailure tests gRPC health probe failure.
func TestRemoteProber_Probe_GRPCFailure(t *testing.T) {
	// Start mock gRPC server with NOT_SERVING status
	server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	defer server.Stop()
	defer lis.Close()

	prober := NewDefaultRemoteProber()
	ctx := context.Background()

	state, err := prober.Probe(ctx, addr)
	require.Error(t, err)
	assert.False(t, state.Healthy)
	assert.Equal(t, addr, state.Address)
	assert.NotEmpty(t, state.Error)
	assert.Contains(t, state.Error, "service not serving")
	assert.Greater(t, state.ResponseTime, time.Duration(0))
}

// TestRemoteProber_Probe_ConnectionFailure tests probe when connection fails.
func TestRemoteProber_Probe_ConnectionFailure(t *testing.T) {
	prober := NewDefaultRemoteProber()
	ctx := context.Background()

	// Try to probe non-existent server
	state, err := prober.Probe(ctx, "localhost:99999")
	require.Error(t, err)
	assert.False(t, state.Healthy)
	assert.NotEmpty(t, state.Error)
	assert.Contains(t, state.Error, "connection refused")
}

// TestRemoteProber_Probe_Timeout tests probe timeout behavior.
func TestRemoteProber_Probe_Timeout(t *testing.T) {
	// Create a server that delays response
	server := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(server, &slowHealthServer{delay: 2 * time.Second})

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer lis.Close()

	go func() {
		_ = server.Serve(lis)
	}()
	defer server.Stop()

	time.Sleep(50 * time.Millisecond) // Let server start

	prober := NewDefaultRemoteProber()

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	state, err := prober.Probe(ctx, lis.Addr().String())
	require.Error(t, err)
	assert.False(t, state.Healthy)
	assert.NotEmpty(t, state.Error)
}

// TestRemoteProber_ProbeAll_Empty tests ProbeAll with no components.
func TestRemoteProber_ProbeAll_Empty(t *testing.T) {
	prober := NewDefaultRemoteProber()
	ctx := context.Background()

	states, err := prober.ProbeAll(ctx)
	require.NoError(t, err)
	assert.Empty(t, states)
}

// TestRemoteProber_ProbeAll_SingleComponent tests ProbeAll with single component.
func TestRemoteProber_ProbeAll_SingleComponent(t *testing.T) {
	// Start mock gRPC server
	server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server.Stop()
	defer lis.Close()

	prober := NewDefaultRemoteProber()
	agents := map[string]RemoteComponentConfig{
		"agent1": {
			Address:     addr,
			HealthCheck: "grpc",
		},
	}
	err := prober.LoadConfig(agents, nil, nil)
	require.NoError(t, err)

	ctx := context.Background()
	states, err := prober.ProbeAll(ctx)
	require.NoError(t, err)
	require.Len(t, states, 1)

	assert.Equal(t, ComponentKindAgent, states[0].Kind)
	assert.Equal(t, "agent1", states[0].Name)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, addr, states[0].Address)
	assert.Empty(t, states[0].Error)
}

// TestRemoteProber_ProbeAll_MultipleComponents tests ProbeAll with multiple components.
func TestRemoteProber_ProbeAll_MultipleComponents(t *testing.T) {
	// Start 3 mock gRPC servers
	server1, lis1, addr1, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server1.Stop()
	defer lis1.Close()

	server2, lis2, addr2, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server2.Stop()
	defer lis2.Close()

	server3, lis3, addr3, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server3.Stop()
	defer lis3.Close()

	prober := NewDefaultRemoteProber()
	agents := map[string]RemoteComponentConfig{
		"agent1": {Address: addr1, HealthCheck: "grpc"},
		"agent2": {Address: addr2, HealthCheck: "grpc"},
	}
	tools := map[string]RemoteComponentConfig{
		"tool1": {Address: addr3, HealthCheck: "grpc"},
	}
	err := prober.LoadConfig(agents, tools, nil)
	require.NoError(t, err)

	ctx := context.Background()
	states, err := prober.ProbeAll(ctx)
	require.NoError(t, err)
	require.Len(t, states, 3)

	// All should be healthy
	for _, state := range states {
		assert.True(t, state.Healthy, "component %s should be healthy", state.Name)
		assert.Empty(t, state.Error)
		assert.Greater(t, state.ResponseTime, time.Duration(0))
	}

	// Verify we have both agents and tool
	kinds := make(map[ComponentKind]int)
	for _, state := range states {
		kinds[state.Kind]++
	}
	assert.Equal(t, 2, kinds[ComponentKindAgent])
	assert.Equal(t, 1, kinds[ComponentKindTool])
}

// TestRemoteProber_ProbeAll_MixedResults tests ProbeAll with healthy and unhealthy components.
func TestRemoteProber_ProbeAll_MixedResults(t *testing.T) {
	// Start 2 servers: one healthy, one unhealthy
	healthyServer, healthyLis, healthyAddr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer healthyServer.Stop()
	defer healthyLis.Close()

	unhealthyServer, unhealthyLis, unhealthyAddr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	defer unhealthyServer.Stop()
	defer unhealthyLis.Close()

	prober := NewDefaultRemoteProber()
	agents := map[string]RemoteComponentConfig{
		"healthy":   {Address: healthyAddr, HealthCheck: "grpc"},
		"unhealthy": {Address: unhealthyAddr, HealthCheck: "grpc"},
		"offline":   {Address: "localhost:99999", HealthCheck: "grpc"},
	}
	err := prober.LoadConfig(agents, nil, nil)
	require.NoError(t, err)

	ctx := context.Background()
	states, err := prober.ProbeAll(ctx)
	require.NoError(t, err)
	require.Len(t, states, 3)

	// Count healthy and unhealthy
	healthyCount := 0
	unhealthyCount := 0
	for _, state := range states {
		if state.Healthy {
			healthyCount++
		} else {
			unhealthyCount++
			assert.NotEmpty(t, state.Error, "unhealthy component should have error message")
		}
	}

	assert.Equal(t, 1, healthyCount, "should have 1 healthy component")
	assert.Equal(t, 2, unhealthyCount, "should have 2 unhealthy components")
}

// TestRemoteProber_ProbeAll_Parallel tests that ProbeAll probes in parallel.
func TestRemoteProber_ProbeAll_Parallel(t *testing.T) {
	// Create 10 servers with 50ms delay each
	const numServers = 10
	const delayPerServer = 50 * time.Millisecond

	var servers []*grpc.Server
	var listeners []net.Listener
	agents := make(map[string]RemoteComponentConfig)

	for i := 0; i < numServers; i++ {
		server := grpc.NewServer()
		grpc_health_v1.RegisterHealthServer(server, &slowHealthServer{delay: delayPerServer})

		lis, err := net.Listen("tcp", "localhost:0")
		require.NoError(t, err)

		go func() {
			_ = server.Serve(lis)
		}()

		servers = append(servers, server)
		listeners = append(listeners, lis)
		agents[string(rune('a'+i))] = RemoteComponentConfig{
			Address:     lis.Addr().String(),
			HealthCheck: "grpc",
			Timeout:     5 * time.Second,
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

	time.Sleep(50 * time.Millisecond) // Let servers start

	prober := NewDefaultRemoteProber()
	err := prober.LoadConfig(agents, nil, nil)
	require.NoError(t, err)

	ctx := context.Background()
	start := time.Now()
	states, err := prober.ProbeAll(ctx)
	duration := time.Since(start)

	require.NoError(t, err)
	require.Len(t, states, numServers)

	// If parallel, should complete much faster than sequential
	// Sequential would be: numServers * delayPerServer = 500ms
	// Parallel should be: ~delayPerServer + overhead = ~100ms
	assert.Less(t, duration, 3*delayPerServer, "should complete in parallel, not sequential")
}

// TestRemoteProber_ProbeAll_CustomTimeout tests ProbeAll respects per-component timeout.
func TestRemoteProber_ProbeAll_CustomTimeout(t *testing.T) {
	// Create a slow server
	server := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(server, &slowHealthServer{delay: 2 * time.Second})

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer lis.Close()

	go func() {
		_ = server.Serve(lis)
	}()
	defer server.Stop()

	time.Sleep(50 * time.Millisecond)

	prober := NewDefaultRemoteProber()
	agents := map[string]RemoteComponentConfig{
		"slow": {
			Address:     lis.Addr().String(),
			HealthCheck: "grpc",
			Timeout:     100 * time.Millisecond, // Short timeout
		},
	}
	err = prober.LoadConfig(agents, nil, nil)
	require.NoError(t, err)

	ctx := context.Background()
	states, err := prober.ProbeAll(ctx)
	require.NoError(t, err)
	require.Len(t, states, 1)

	// Should have failed due to timeout
	assert.False(t, states[0].Healthy)
	assert.NotEmpty(t, states[0].Error)
}

// TestRemoteProber_ProbeHTTP_Success tests HTTP health probe success.
func TestRemoteProber_ProbeHTTP_Success(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Parse address from server URL
	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)

	addr := host + ":" + portStr

	prober := NewDefaultRemoteProber()
	agents := map[string]RemoteComponentConfig{
		"http-agent": {
			Address:     addr,
			HealthCheck: "http",
			Timeout:     5 * time.Second,
		},
	}
	err = prober.LoadConfig(agents, nil, nil)
	require.NoError(t, err)

	// Test via ProbeAll
	ctx := context.Background()
	states, err := prober.ProbeAll(ctx)
	require.NoError(t, err)
	require.Len(t, states, 1)

	assert.True(t, states[0].Healthy)
	assert.Empty(t, states[0].Error)
}

// TestRemoteProber_ProbeHTTP_Failure tests HTTP health probe failure.
func TestRemoteProber_ProbeHTTP_Failure(t *testing.T) {
	// Create test HTTP server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)

	addr := host + ":" + portStr

	prober := NewDefaultRemoteProber()
	agents := map[string]RemoteComponentConfig{
		"http-agent": {
			Address:     addr,
			HealthCheck: "http",
		},
	}
	err = prober.LoadConfig(agents, nil, nil)
	require.NoError(t, err)

	ctx := context.Background()
	states, err := prober.ProbeAll(ctx)
	require.NoError(t, err)
	require.Len(t, states, 1)

	assert.False(t, states[0].Healthy)
	assert.Contains(t, states[0].Error, "500")
}

// TestRemoteProber_TLSConfig_ValidCertificates tests TLS configuration with valid certificates.
func TestRemoteProber_TLSConfig_ValidCertificates(t *testing.T) {
	// Create temp directory for test certificates
	tmpDir := t.TempDir()

	// Note: This test validates the TLS config structure, not actual TLS connections
	// Full TLS testing would require generating valid certificates

	prober := NewDefaultRemoteProber()

	// Create dummy files (empty, just for validation)
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")
	caFile := filepath.Join(tmpDir, "ca.pem")

	for _, f := range []string{certFile, keyFile, caFile} {
		err := os.WriteFile(f, []byte("dummy"), 0600)
		require.NoError(t, err)
	}

	agents := map[string]RemoteComponentConfig{
		"tls-agent": {
			Address:     "localhost:50051",
			HealthCheck: "grpc",
			TLS: &TLSConfig{
				Enabled:  true,
				CertFile: certFile,
				KeyFile:  keyFile,
				CAFile:   caFile,
			},
		},
	}

	// Should accept valid TLS config
	err := prober.LoadConfig(agents, nil, nil)
	require.NoError(t, err)

	prober.mu.RLock()
	defer prober.mu.RUnlock()

	cfg := prober.components[ComponentKindAgent]["tls-agent"]
	assert.True(t, cfg.TLS.Enabled)
	assert.Equal(t, certFile, cfg.TLS.CertFile)
	assert.Equal(t, keyFile, cfg.TLS.KeyFile)
	assert.Equal(t, caFile, cfg.TLS.CAFile)
}

// TestRemoteProber_TLSConfig_InsecureSkipVerify tests TLS with skip verify option.
func TestRemoteProber_TLSConfig_InsecureSkipVerify(t *testing.T) {
	prober := NewDefaultRemoteProber()

	agents := map[string]RemoteComponentConfig{
		"insecure-agent": {
			Address:     "localhost:50051",
			HealthCheck: "grpc",
			TLS: &TLSConfig{
				Enabled:            true,
				InsecureSkipVerify: true,
			},
		},
	}

	err := prober.LoadConfig(agents, nil, nil)
	require.NoError(t, err)

	prober.mu.RLock()
	defer prober.mu.RUnlock()

	cfg := prober.components[ComponentKindAgent]["insecure-agent"]
	assert.True(t, cfg.TLS.InsecureSkipVerify)
}

// TestRemoteProber_ConcurrentAccess tests thread-safety of concurrent operations.
func TestRemoteProber_ConcurrentAccess(t *testing.T) {
	// Start a test server
	server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server.Stop()
	defer lis.Close()

	prober := NewDefaultRemoteProber()

	// Initial config
	agents := map[string]RemoteComponentConfig{
		"agent1": {Address: addr, HealthCheck: "grpc"},
	}
	err := prober.LoadConfig(agents, nil, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	ctx := context.Background()

	// Concurrent ProbeAll calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = prober.ProbeAll(ctx)
		}()
	}

	// Concurrent LoadConfig calls
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			newAgents := map[string]RemoteComponentConfig{
				"agent1": {Address: addr, HealthCheck: "grpc"},
			}
			_ = prober.LoadConfig(newAgents, nil, nil)
		}(i)
	}

	wg.Wait()
	// Should not panic or race
}

// TestRemoteProber_ContextCancellation tests ProbeAll respects context cancellation.
func TestRemoteProber_ContextCancellation(t *testing.T) {
	// Create slow servers
	var servers []*grpc.Server
	var listeners []net.Listener
	agents := make(map[string]RemoteComponentConfig)

	for i := 0; i < 5; i++ {
		server := grpc.NewServer()
		grpc_health_v1.RegisterHealthServer(server, &slowHealthServer{delay: 5 * time.Second})

		lis, err := net.Listen("tcp", "localhost:0")
		require.NoError(t, err)

		go func() {
			_ = server.Serve(lis)
		}()

		servers = append(servers, server)
		listeners = append(listeners, lis)
		agents[string(rune('a'+i))] = RemoteComponentConfig{
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

	time.Sleep(50 * time.Millisecond)

	prober := NewDefaultRemoteProber()
	err := prober.LoadConfig(agents, nil, nil)
	require.NoError(t, err)

	// Create context that will be cancelled quickly
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	states, err := prober.ProbeAll(ctx)
	duration := time.Since(start)

	require.NoError(t, err)
	// Should complete quickly due to context timeout
	assert.Less(t, duration, 1*time.Second)

	// All probes should have timed out
	for _, state := range states {
		assert.False(t, state.Healthy)
	}
}

// TestRemoteProber_ResponseTimeTracking tests that response times are tracked correctly.
func TestRemoteProber_ResponseTimeTracking(t *testing.T) {
	// Start a server with known delay
	const serverDelay = 100 * time.Millisecond
	server := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(server, &slowHealthServer{delay: serverDelay})

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer lis.Close()

	go func() {
		_ = server.Serve(lis)
	}()
	defer server.Stop()

	time.Sleep(50 * time.Millisecond)

	prober := NewDefaultRemoteProber()
	agents := map[string]RemoteComponentConfig{
		"slow": {
			Address:     lis.Addr().String(),
			HealthCheck: "grpc",
			Timeout:     5 * time.Second,
		},
	}
	err = prober.LoadConfig(agents, nil, nil)
	require.NoError(t, err)

	ctx := context.Background()
	states, err := prober.ProbeAll(ctx)
	require.NoError(t, err)
	require.Len(t, states, 1)

	// Response time should be at least the server delay
	assert.GreaterOrEqual(t, states[0].ResponseTime, serverDelay)
	// But not too much longer (allow 2x for overhead)
	assert.Less(t, states[0].ResponseTime, 3*serverDelay)
}

// slowHealthServer is a mock health server that delays responses for testing.
type slowHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
	delay time.Duration
}

func (s *slowHealthServer) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	select {
	case <-time.After(s.delay):
		return &grpc_health_v1.HealthCheckResponse{
			Status: grpc_health_v1.HealthCheckResponse_SERVING,
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// BenchmarkRemoteProber_Probe benchmarks single probe performance.
func BenchmarkRemoteProber_Probe(b *testing.B) {
	server, lis, addr, _ := startGRPCHealthServer(&testing.T{}, grpc_health_v1.HealthCheckResponse_SERVING)
	defer server.Stop()
	defer lis.Close()

	prober := NewDefaultRemoteProber()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = prober.Probe(ctx, addr)
	}
}

// BenchmarkRemoteProber_ProbeAll benchmarks ProbeAll with multiple components.
func BenchmarkRemoteProber_ProbeAll(b *testing.B) {
	// Start 10 test servers
	const numServers = 10
	var servers []*grpc.Server
	var listeners []net.Listener
	agents := make(map[string]RemoteComponentConfig)

	for i := 0; i < numServers; i++ {
		server, lis, addr, _ := startGRPCHealthServer(&testing.T{}, grpc_health_v1.HealthCheckResponse_SERVING)
		servers = append(servers, server)
		listeners = append(listeners, lis)
		agents[string(rune('a'+i))] = RemoteComponentConfig{
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

	prober := NewDefaultRemoteProber()
	_ = prober.LoadConfig(agents, nil, nil)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = prober.ProbeAll(ctx)
	}
}

// BenchmarkRemoteProber_ProbeAll_Parallel benchmarks ProbeAll parallelism.
func BenchmarkRemoteProber_ProbeAll_Parallel(b *testing.B) {
	// Create servers with delays to measure parallel speedup
	const numServers = 20
	const delay = 10 * time.Millisecond

	var servers []*grpc.Server
	var listeners []net.Listener
	agents := make(map[string]RemoteComponentConfig)

	for i := 0; i < numServers; i++ {
		server := grpc.NewServer()
		grpc_health_v1.RegisterHealthServer(server, &slowHealthServer{delay: delay})

		lis, err := net.Listen("tcp", "localhost:0")
		if err != nil {
			b.Fatal(err)
		}

		go func() {
			_ = server.Serve(lis)
		}()

		servers = append(servers, server)
		listeners = append(listeners, lis)
		agents[string(rune('a'+i))] = RemoteComponentConfig{
			Address:     lis.Addr().String(),
			HealthCheck: "grpc",
			Timeout:     5 * time.Second,
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

	time.Sleep(50 * time.Millisecond)

	prober := NewDefaultRemoteProber()
	_ = prober.LoadConfig(agents, nil, nil)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = prober.ProbeAll(ctx)
	}
}

// TestRemoteProber_ProbeAll_StressTest is a stress test with many concurrent probes.
func TestRemoteProber_ProbeAll_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	// Create 50 servers
	const numServers = 50
	var servers []*grpc.Server
	var listeners []net.Listener
	agents := make(map[string]RemoteComponentConfig)

	for i := 0; i < numServers; i++ {
		server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
		servers = append(servers, server)
		listeners = append(listeners, lis)
		agents[string(rune('a'+i))] = RemoteComponentConfig{
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

	prober := NewDefaultRemoteProber()
	err := prober.LoadConfig(agents, nil, nil)
	require.NoError(t, err)

	ctx := context.Background()

	// Run multiple times
	for i := 0; i < 10; i++ {
		states, err := prober.ProbeAll(ctx)
		require.NoError(t, err)
		assert.Len(t, states, numServers)

		// All should be healthy
		for _, state := range states {
			assert.True(t, state.Healthy)
		}
	}
}

// TestRemoteProber_ProbeAll_PartialTimeout tests mixed timeout scenarios.
func TestRemoteProber_ProbeAll_PartialTimeout(t *testing.T) {
	// Create 1 fast server and 1 slow server
	fastServer, fastLis, fastAddr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer fastServer.Stop()
	defer fastLis.Close()

	slowServer := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(slowServer, &slowHealthServer{delay: 2 * time.Second})
	slowLis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer slowLis.Close()

	go func() {
		_ = slowServer.Serve(slowLis)
	}()
	defer slowServer.Stop()

	time.Sleep(50 * time.Millisecond)

	prober := NewDefaultRemoteProber()
	agents := map[string]RemoteComponentConfig{
		"fast": {
			Address:     fastAddr,
			HealthCheck: "grpc",
			Timeout:     5 * time.Second,
		},
		"slow": {
			Address:     slowLis.Addr().String(),
			HealthCheck: "grpc",
			Timeout:     100 * time.Millisecond, // Will timeout
		},
	}
	err = prober.LoadConfig(agents, nil, nil)
	require.NoError(t, err)

	ctx := context.Background()
	states, err := prober.ProbeAll(ctx)
	require.NoError(t, err)
	require.Len(t, states, 2)

	// Find each state
	var fastState, slowState *RemoteComponentState
	for i := range states {
		if states[i].Name == "fast" {
			fastState = &states[i]
		} else if states[i].Name == "slow" {
			slowState = &states[i]
		}
	}

	require.NotNil(t, fastState)
	require.NotNil(t, slowState)

	// Fast should succeed, slow should fail
	assert.True(t, fastState.Healthy, "fast server should be healthy")
	assert.False(t, slowState.Healthy, "slow server should timeout")
}

// TestRemoteProber_ErrorPropagation tests that individual probe errors are captured correctly.
func TestRemoteProber_ErrorPropagation(t *testing.T) {
	prober := NewDefaultRemoteProber()
	agents := map[string]RemoteComponentConfig{
		"connection-refused": {
			Address:     "localhost:99999",
			HealthCheck: "grpc",
		},
	}
	err := prober.LoadConfig(agents, nil, nil)
	require.NoError(t, err)

	ctx := context.Background()
	states, err := prober.ProbeAll(ctx)
	require.NoError(t, err)
	require.Len(t, states, 1)

	assert.False(t, states[0].Healthy)
	assert.NotEmpty(t, states[0].Error)
	// Error should contain useful information
	assert.Contains(t, states[0].Error, "connection refused")
}

// TestRemoteProber_AtomicProbeCount tests that all probes complete even under high concurrency.
func TestRemoteProber_AtomicProbeCount(t *testing.T) {
	// Create 20 servers
	const numServers = 20
	var servers []*grpc.Server
	var listeners []net.Listener
	agents := make(map[string]RemoteComponentConfig)

	for i := 0; i < numServers; i++ {
		server, lis, addr, _ := startGRPCHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
		servers = append(servers, server)
		listeners = append(listeners, lis)
		agents[string(rune('a'+i))] = RemoteComponentConfig{
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

	prober := NewDefaultRemoteProber()
	err := prober.LoadConfig(agents, nil, nil)
	require.NoError(t, err)

	ctx := context.Background()

	// Run multiple concurrent ProbeAll calls
	var wg sync.WaitGroup
	var totalProbes atomic.Int32

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			states, err := prober.ProbeAll(ctx)
			require.NoError(t, err)
			totalProbes.Add(int32(len(states)))
		}()
	}

	wg.Wait()

	// Should have probed exactly numServers * 10
	assert.Equal(t, int32(numServers*10), totalProbes.Load())
}
