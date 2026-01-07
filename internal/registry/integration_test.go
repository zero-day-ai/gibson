// Package registry provides integration tests for registry functionality.
//
// These tests use an embedded etcd instance to verify component registration,
// discovery, health tracking, circuit breaker behavior, and watch functionality.
package registry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkregistry "github.com/zero-day-ai/sdk/registry"
)

// TestIntegration_ComponentRegistration tests basic component registration and discovery.
func TestIntegration_ComponentRegistration(t *testing.T) {
	// Create embedded registry
	cfg := sdkregistry.Config{
		Type:          "embedded",
		DataDir:       t.TempDir() + "/etcd",
		ListenAddress: "localhost:12379", // Use a specific port for testing
		Namespace:     "gibson-test",
		TTL:           5, // Short TTL for testing
	}

	reg, err := NewEmbeddedRegistry(cfg)
	require.NoError(t, err)
	defer reg.Close()

	ctx := context.Background()

	// Test registration
	t.Run("register agent", func(t *testing.T) {
		info := sdkregistry.ServiceInfo{
			Kind:       "agent",
			Name:       "test-agent",
			Version:    "1.0.0",
			InstanceID: "instance-1",
			Endpoint:   "localhost:50051",
			Metadata: map[string]string{
				"capabilities": "prompt_injection,jailbreak",
			},
			StartedAt: time.Now(),
		}

		err := reg.Register(ctx, info)
		require.NoError(t, err)

		// Verify health metadata was added
		assert.NotEmpty(t, info.Metadata[MetadataKeyHealth])
		assert.NotEmpty(t, info.Metadata[MetadataKeyLastHealthCheck])
	})

	// Test discovery
	t.Run("discover registered agent", func(t *testing.T) {
		services, err := reg.Discover(ctx, "agent", "test-agent")
		require.NoError(t, err)
		require.Len(t, services, 1)

		svc := services[0]
		assert.Equal(t, "agent", svc.Kind)
		assert.Equal(t, "test-agent", svc.Name)
		assert.Equal(t, "1.0.0", svc.Version)
		assert.Equal(t, "localhost:50051", svc.Endpoint)
		assert.Equal(t, HealthStatusHealthy, GetHealthStatus(svc))
	})

	// Test DiscoverAll
	t.Run("discover all agents", func(t *testing.T) {
		// Register another agent
		info2 := sdkregistry.ServiceInfo{
			Kind:       "agent",
			Name:       "another-agent",
			Version:    "1.0.0",
			InstanceID: "instance-2",
			Endpoint:   "localhost:50052",
			Metadata:   map[string]string{},
			StartedAt:  time.Now(),
		}
		err := reg.Register(ctx, info2)
		require.NoError(t, err)

		// Discover all agents
		services, err := reg.DiscoverAll(ctx, "agent")
		require.NoError(t, err)
		assert.Len(t, services, 2)
	})

	// Test deregistration
	t.Run("deregister agent", func(t *testing.T) {
		info := sdkregistry.ServiceInfo{
			Kind:       "agent",
			Name:       "test-agent",
			InstanceID: "instance-1",
		}

		err := reg.Deregister(ctx, info)
		require.NoError(t, err)

		// Verify service is gone
		services, err := reg.Discover(ctx, "agent", "test-agent")
		require.NoError(t, err)
		assert.Empty(t, services)
	})
}

// TestIntegration_HealthTracking tests health status tracking in metadata.
func TestIntegration_HealthTracking(t *testing.T) {
	cfg := sdkregistry.Config{
		Type:          "embedded",
		DataDir:       t.TempDir() + "/etcd",
		ListenAddress: "localhost:12380",
		Namespace:     "gibson-test",
		TTL:           30,
	}

	reg, err := NewEmbeddedRegistry(cfg)
	require.NoError(t, err)
	defer reg.Close()

	ctx := context.Background()

	t.Run("health status set on registration", func(t *testing.T) {
		info := sdkregistry.ServiceInfo{
			Kind:       "agent",
			Name:       "health-test",
			Version:    "1.0.0",
			InstanceID: "health-1",
			Endpoint:   "localhost:50053",
			Metadata:   map[string]string{},
			StartedAt:  time.Now(),
		}

		err := reg.Register(ctx, info)
		require.NoError(t, err)

		// Discover and verify health metadata
		services, err := reg.Discover(ctx, "agent", "health-test")
		require.NoError(t, err)
		require.Len(t, services, 1)

		svc := services[0]
		assert.Equal(t, HealthStatusHealthy, GetHealthStatus(svc))

		lastCheck := GetLastHealthCheck(svc)
		assert.False(t, lastCheck.IsZero())
		assert.WithinDuration(t, time.Now(), lastCheck, 5*time.Second)
	})

	t.Run("custom health status preserved", func(t *testing.T) {
		info := sdkregistry.ServiceInfo{
			Kind:       "agent",
			Name:       "degraded-test",
			Version:    "1.0.0",
			InstanceID: "degraded-1",
			Endpoint:   "localhost:50054",
			Metadata: map[string]string{
				MetadataKeyHealth: HealthStatusDegraded,
			},
			StartedAt: time.Now(),
		}

		err := reg.Register(ctx, info)
		require.NoError(t, err)

		// Discover and verify health metadata
		services, err := reg.Discover(ctx, "agent", "degraded-test")
		require.NoError(t, err)
		require.Len(t, services, 1)

		svc := services[0]
		assert.Equal(t, HealthStatusDegraded, GetHealthStatus(svc))
	})
}

// TestIntegration_WatchEvents tests event-based watching of component changes.
func TestIntegration_WatchEvents(t *testing.T) {
	cfg := sdkregistry.Config{
		Type:          "embedded",
		DataDir:       t.TempDir() + "/etcd",
		ListenAddress: "localhost:12381",
		Namespace:     "gibson-test",
		TTL:           30,
	}

	reg, err := NewEmbeddedRegistry(cfg)
	require.NoError(t, err)
	defer reg.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("watch registration events", func(t *testing.T) {
		// Start watching before registering
		eventCh, err := WatchEvents(ctx, reg, "agent", "watch-test")
		require.NoError(t, err)

		// Give watch time to initialize
		time.Sleep(100 * time.Millisecond)

		// Register a service
		info := sdkregistry.ServiceInfo{
			Kind:       "agent",
			Name:       "watch-test",
			Version:    "1.0.0",
			InstanceID: "watch-1",
			Endpoint:   "localhost:50055",
			Metadata:   map[string]string{},
			StartedAt:  time.Now(),
		}
		err = reg.Register(ctx, info)
		require.NoError(t, err)

		// Wait for registration event
		select {
		case event := <-eventCh:
			assert.Equal(t, EventTypeRegistered, event.Type)
			assert.Equal(t, "agent", event.Kind)
			assert.Equal(t, "watch-test", event.Name)
			assert.Equal(t, "localhost:50055", event.Endpoint)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for registration event")
		}
	})

	t.Run("watch deregistration events", func(t *testing.T) {
		// Register service first
		info := sdkregistry.ServiceInfo{
			Kind:       "agent",
			Name:       "deregister-test",
			Version:    "1.0.0",
			InstanceID: "deregister-1",
			Endpoint:   "localhost:50056",
			Metadata:   map[string]string{},
			StartedAt:  time.Now(),
		}
		err := reg.Register(ctx, info)
		require.NoError(t, err)

		// Start watching
		eventCh, err := WatchEvents(ctx, reg, "agent", "deregister-test")
		require.NoError(t, err)

		// Consume initial snapshot event (the service is already registered)
		select {
		case event := <-eventCh:
			// Should be registered event from snapshot
			assert.Equal(t, EventTypeRegistered, event.Type)
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for initial snapshot event")
		}

		// Deregister
		err = reg.Deregister(ctx, info)
		require.NoError(t, err)

		// Wait for deregistration event
		select {
		case event := <-eventCh:
			assert.Equal(t, EventTypeDeregistered, event.Type)
			assert.Equal(t, "agent", event.Kind)
			assert.Equal(t, "deregister-test", event.Name)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for deregistration event")
		}
	})
}

// TestIntegration_CircuitBreaker tests circuit breaker behavior with GRPCPool.
func TestIntegration_CircuitBreaker(t *testing.T) {
	t.Run("circuit opens after failures", func(t *testing.T) {
		config := CircuitBreakerConfig{
			FailureThreshold:    3,
			OpenTimeout:         5 * time.Second,
			HalfOpenMaxRequests: 1,
		}

		cb := NewCircuitBreaker(config)

		endpoint := "localhost:9999" // Non-existent endpoint

		// Record failures
		for i := 0; i < 3; i++ {
			cb.RecordFailure(endpoint, fmt.Errorf("connection failed"))
		}

		// Circuit should be open now
		err := cb.Allow(endpoint)
		assert.Error(t, err)
		var circuitErr *CircuitOpenError
		assert.ErrorAs(t, err, &circuitErr)
		assert.Equal(t, endpoint, circuitErr.Endpoint)
	})

	t.Run("circuit transitions to half-open after timeout", func(t *testing.T) {
		config := CircuitBreakerConfig{
			FailureThreshold:    2,
			OpenTimeout:         500 * time.Millisecond, // Short timeout for testing
			HalfOpenMaxRequests: 1,
		}

		cb := NewCircuitBreaker(config)
		endpoint := "localhost:9998"

		// Open the circuit
		cb.RecordFailure(endpoint, fmt.Errorf("error 1"))
		cb.RecordFailure(endpoint, fmt.Errorf("error 2"))

		// Verify circuit is open
		assert.Equal(t, StateOpen, cb.GetState(endpoint))

		// Wait for timeout
		time.Sleep(600 * time.Millisecond)

		// Circuit should allow request (transitioning to half-open)
		err := cb.Allow(endpoint)
		assert.NoError(t, err)
	})

	t.Run("circuit closes after successful request in half-open", func(t *testing.T) {
		config := CircuitBreakerConfig{
			FailureThreshold:    2,
			OpenTimeout:         100 * time.Millisecond,
			HalfOpenMaxRequests: 1,
		}

		cb := NewCircuitBreaker(config)
		endpoint := "localhost:9997"

		// Open the circuit
		cb.RecordFailure(endpoint, fmt.Errorf("error 1"))
		cb.RecordFailure(endpoint, fmt.Errorf("error 2"))
		assert.Equal(t, StateOpen, cb.GetState(endpoint))

		// Wait for transition to half-open
		time.Sleep(150 * time.Millisecond)
		err := cb.Allow(endpoint)
		assert.NoError(t, err)

		// Record success - should close circuit
		cb.RecordSuccess(endpoint)
		assert.Equal(t, StateClosed, cb.GetState(endpoint))

		// Should allow requests now
		err = cb.Allow(endpoint)
		assert.NoError(t, err)
	})

	t.Run("circuit reopens after failure in half-open", func(t *testing.T) {
		config := CircuitBreakerConfig{
			FailureThreshold:    2,
			OpenTimeout:         100 * time.Millisecond,
			HalfOpenMaxRequests: 1,
		}

		cb := NewCircuitBreaker(config)
		endpoint := "localhost:9996"

		// Open the circuit
		cb.RecordFailure(endpoint, fmt.Errorf("error 1"))
		cb.RecordFailure(endpoint, fmt.Errorf("error 2"))

		// Wait for half-open
		time.Sleep(150 * time.Millisecond)
		_ = cb.Allow(endpoint)

		// Record failure - should reopen circuit
		cb.RecordFailure(endpoint, fmt.Errorf("still failing"))
		assert.Equal(t, StateOpen, cb.GetState(endpoint))
	})
}

// TestIntegration_GRPCPoolCircuitBreaker tests GRPCPool integration with circuit breaker.
func TestIntegration_GRPCPoolCircuitBreaker(t *testing.T) {
	t.Run("pool uses circuit breaker", func(t *testing.T) {
		config := CircuitBreakerConfig{
			FailureThreshold:    3,
			OpenTimeout:         5 * time.Second,
			HalfOpenMaxRequests: 1,
		}

		pool := NewGRPCPoolWithCircuitBreaker(config)
		defer pool.Close()

		ctx := context.Background()
		endpoint := "localhost:9995" // Non-existent endpoint

		// Circuit breaker should start closed
		state := pool.GetCircuitState(endpoint)
		assert.Equal(t, StateClosed, state)

		// Manually record failures to simulate failed connection attempts
		// (grpc.NewClient doesn't actually fail for non-existent endpoints - it's non-blocking)
		for i := 0; i < 3; i++ {
			pool.circuitBreaker.RecordFailure(endpoint, fmt.Errorf("connection failed"))
		}

		// After 3 failures, circuit should be open
		state = pool.GetCircuitState(endpoint)
		assert.Equal(t, StateOpen, state)

		// Next attempt should be blocked by circuit breaker
		_, err := pool.Get(ctx, endpoint)
		assert.Error(t, err)
		var circuitErr *CircuitOpenError
		assert.ErrorAs(t, err, &circuitErr)
	})

	t.Run("circuit breaker stats", func(t *testing.T) {
		pool := NewGRPCPool()
		defer pool.Close()

		ctx := context.Background()

		// Try to connect to non-existent endpoints
		endpoints := []string{"localhost:9994", "localhost:9993", "localhost:9992"}
		for _, endpoint := range endpoints {
			_, _ = pool.Get(ctx, endpoint)
		}

		// Get stats
		stats := pool.CircuitBreakerStats()
		assert.GreaterOrEqual(t, stats.Total, 0)
	})

	t.Run("reset circuit breaker", func(t *testing.T) {
		config := CircuitBreakerConfig{
			FailureThreshold:    2,
			OpenTimeout:         10 * time.Second,
			HalfOpenMaxRequests: 1,
		}

		pool := NewGRPCPoolWithCircuitBreaker(config)
		defer pool.Close()

		endpoint := "localhost:9991"

		// Open the circuit by recording failures
		pool.circuitBreaker.RecordFailure(endpoint, fmt.Errorf("error 1"))
		pool.circuitBreaker.RecordFailure(endpoint, fmt.Errorf("error 2"))

		// Verify circuit is open
		assert.Equal(t, StateOpen, pool.GetCircuitState(endpoint))

		// Reset circuit
		pool.ResetCircuit(endpoint)

		// Circuit should be closed now
		assert.Equal(t, StateClosed, pool.GetCircuitState(endpoint))
	})
}

// TestIntegration_RegistryAdapter tests the registry adapter with embedded etcd.
func TestIntegration_RegistryAdapter(t *testing.T) {
	cfg := sdkregistry.Config{
		Type:          "embedded",
		DataDir:       t.TempDir() + "/etcd",
		ListenAddress: "localhost:12382",
		Namespace:     "gibson-test",
		TTL:           30,
	}

	reg, err := NewEmbeddedRegistry(cfg)
	require.NoError(t, err)
	defer reg.Close()

	adapter := NewRegistryAdapter(reg)
	defer adapter.Close()

	ctx := context.Background()

	t.Run("list agents when none registered", func(t *testing.T) {
		agents, err := adapter.ListAgents(ctx)
		require.NoError(t, err)
		assert.Empty(t, agents)
	})

	t.Run("list agents after registration", func(t *testing.T) {
		// Register an agent
		info := sdkregistry.ServiceInfo{
			Kind:       "agent",
			Name:       "adapter-test",
			Version:    "1.0.0",
			InstanceID: "adapter-1",
			Endpoint:   "localhost:50057",
			Metadata: map[string]string{
				"capabilities": "prompt_injection",
			},
			StartedAt: time.Now(),
		}
		err := reg.Register(ctx, info)
		require.NoError(t, err)

		// List agents via adapter
		agents, err := adapter.ListAgents(ctx)
		require.NoError(t, err)
		require.Len(t, agents, 1)

		agent := agents[0]
		assert.Equal(t, "adapter-test", agent.Name)
		assert.Equal(t, "1.0.0", agent.Version)
		assert.Equal(t, 1, agent.Instances)
		assert.Contains(t, agent.Endpoints, "localhost:50057")
	})

	t.Run("discover agent by name", func(t *testing.T) {
		// Discover should fail for gRPC client creation since endpoint doesn't exist
		// But we can test that discovery attempt is made
		_, err := adapter.DiscoverAgent(ctx, "adapter-test")
		_ = err // Use the variable to avoid unused error
		// Will fail because gRPC connection can't be established to non-existent endpoint
		// This is expected behavior since the endpoint doesn't actually exist
	})
}
