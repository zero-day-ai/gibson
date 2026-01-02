package registry

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkregistry "github.com/zero-day-ai/sdk/registry"
)

// mockRegistry is a simple in-memory registry for testing.
type mockRegistry struct {
	services map[string][]sdkregistry.ServiceInfo
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{
		services: make(map[string][]sdkregistry.ServiceInfo),
	}
}

func (m *mockRegistry) Register(ctx context.Context, info sdkregistry.ServiceInfo) error {
	key := info.Kind + ":" + info.Name
	m.services[key] = append(m.services[key], info)
	return nil
}

func (m *mockRegistry) Deregister(ctx context.Context, info sdkregistry.ServiceInfo) error {
	key := info.Kind + ":" + info.Name
	instances := m.services[key]
	filtered := make([]sdkregistry.ServiceInfo, 0, len(instances))
	for _, instance := range instances {
		if instance.InstanceID != info.InstanceID {
			filtered = append(filtered, instance)
		}
	}
	m.services[key] = filtered
	return nil
}

func (m *mockRegistry) Discover(ctx context.Context, kind, name string) ([]sdkregistry.ServiceInfo, error) {
	key := kind + ":" + name
	instances := m.services[key]
	if instances == nil {
		return []sdkregistry.ServiceInfo{}, nil
	}
	return instances, nil
}

func (m *mockRegistry) DiscoverAll(ctx context.Context, kind string) ([]sdkregistry.ServiceInfo, error) {
	var all []sdkregistry.ServiceInfo
	for key, instances := range m.services {
		if len(key) > 0 && key[:len(kind)] == kind {
			all = append(all, instances...)
		}
	}
	return all, nil
}

func (m *mockRegistry) Watch(ctx context.Context, kind, name string) (<-chan []sdkregistry.ServiceInfo, error) {
	ch := make(chan []sdkregistry.ServiceInfo)
	go func() {
		instances, _ := m.Discover(ctx, kind, name)
		ch <- instances
		close(ch)
	}()
	return ch, nil
}

func (m *mockRegistry) Close() error {
	return nil
}

func TestLoadBalancer_SingleInstance(t *testing.T) {
	reg := newMockRegistry()
	lb := NewLoadBalancer(reg, StrategyRoundRobin)

	ctx := context.Background()

	// Register single instance
	instance := sdkregistry.ServiceInfo{
		Kind:       "agent",
		Name:       "davinci",
		Version:    "1.0.0",
		InstanceID: "instance-1",
		Endpoint:   "localhost:50051",
		Metadata:   map[string]string{"capabilities": "jailbreak"},
		StartedAt:  time.Now(),
	}
	err := reg.Register(ctx, instance)
	require.NoError(t, err)

	// Select should always return the same instance
	for i := 0; i < 5; i++ {
		selected, err := lb.Select(ctx, "agent", "davinci")
		require.NoError(t, err)
		assert.Equal(t, "instance-1", selected.InstanceID)
		assert.Equal(t, "localhost:50051", selected.Endpoint)
	}
}

func TestLoadBalancer_RoundRobin(t *testing.T) {
	reg := newMockRegistry()
	lb := NewLoadBalancer(reg, StrategyRoundRobin)

	ctx := context.Background()

	// Register three instances
	instances := []sdkregistry.ServiceInfo{
		{
			Kind:       "agent",
			Name:       "k8skiller",
			Version:    "1.0.0",
			InstanceID: "instance-1",
			Endpoint:   "localhost:50051",
			StartedAt:  time.Now(),
		},
		{
			Kind:       "agent",
			Name:       "k8skiller",
			Version:    "1.0.0",
			InstanceID: "instance-2",
			Endpoint:   "localhost:50052",
			StartedAt:  time.Now(),
		},
		{
			Kind:       "agent",
			Name:       "k8skiller",
			Version:    "1.0.0",
			InstanceID: "instance-3",
			Endpoint:   "localhost:50053",
			StartedAt:  time.Now(),
		},
	}

	for _, instance := range instances {
		err := reg.Register(ctx, instance)
		require.NoError(t, err)
	}

	// Should cycle through instances in order
	selectedIDs := make([]string, 6)
	for i := 0; i < 6; i++ {
		selected, err := lb.Select(ctx, "agent", "k8skiller")
		require.NoError(t, err)
		selectedIDs[i] = selected.InstanceID
	}

	// Should see pattern: 1, 2, 3, 1, 2, 3
	assert.Equal(t, "instance-1", selectedIDs[0])
	assert.Equal(t, "instance-2", selectedIDs[1])
	assert.Equal(t, "instance-3", selectedIDs[2])
	assert.Equal(t, "instance-1", selectedIDs[3])
	assert.Equal(t, "instance-2", selectedIDs[4])
	assert.Equal(t, "instance-3", selectedIDs[5])
}

func TestLoadBalancer_Random(t *testing.T) {
	reg := newMockRegistry()
	lb := NewLoadBalancer(reg, StrategyRandom)

	ctx := context.Background()

	// Register three instances
	for i := 1; i <= 3; i++ {
		instance := sdkregistry.ServiceInfo{
			Kind:       "tool",
			Name:       "nmap",
			Version:    "1.0.0",
			InstanceID: "instance-" + string(rune('0'+i)),
			Endpoint:   "localhost:5005" + string(rune('0'+i)),
			StartedAt:  time.Now(),
		}
		err := reg.Register(ctx, instance)
		require.NoError(t, err)
	}

	// Make many selections and verify distribution
	counts := make(map[string]int)
	for i := 0; i < 100; i++ {
		selected, err := lb.Select(ctx, "tool", "nmap")
		require.NoError(t, err)
		counts[selected.InstanceID]++
	}

	// Each instance should be selected at least once (very high probability)
	assert.Equal(t, 3, len(counts), "All instances should be selected")

	// With 100 selections across 3 instances, each should get roughly 33
	// Allow wide margin since it's random (at least 10 each)
	for id, count := range counts {
		assert.GreaterOrEqual(t, count, 10, "Instance %s should be selected multiple times", id)
	}
}

func TestLoadBalancer_NoInstances(t *testing.T) {
	reg := newMockRegistry()
	lb := NewLoadBalancer(reg, StrategyRoundRobin)

	ctx := context.Background()

	// Try to select from empty registry
	_, err := lb.Select(ctx, "agent", "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no instances")
}

func TestLoadBalancer_SelectEndpoint(t *testing.T) {
	reg := newMockRegistry()
	lb := NewLoadBalancer(reg, StrategyRoundRobin)

	ctx := context.Background()

	// Register instance
	instance := sdkregistry.ServiceInfo{
		Kind:       "plugin",
		Name:       "cvedb",
		Version:    "1.0.0",
		InstanceID: "instance-1",
		Endpoint:   "localhost:50051",
		StartedAt:  time.Now(),
	}
	err := reg.Register(ctx, instance)
	require.NoError(t, err)

	// SelectEndpoint should return just the endpoint string
	endpoint, err := lb.SelectEndpoint(ctx, "plugin", "cvedb")
	require.NoError(t, err)
	assert.Equal(t, "localhost:50051", endpoint)
}

func TestLoadBalancer_StrategyChange(t *testing.T) {
	reg := newMockRegistry()
	lb := NewLoadBalancer(reg, StrategyRoundRobin)

	ctx := context.Background()

	// Register three instances
	for i := 1; i <= 3; i++ {
		instance := sdkregistry.ServiceInfo{
			Kind:       "agent",
			Name:       "davinci",
			Version:    "1.0.0",
			InstanceID: "instance-" + string(rune('0'+i)),
			Endpoint:   "localhost:5005" + string(rune('0'+i)),
			StartedAt:  time.Now(),
		}
		err := reg.Register(ctx, instance)
		require.NoError(t, err)
	}

	// Verify initial strategy
	assert.Equal(t, StrategyRoundRobin, lb.Strategy())

	// Select a few times with round-robin
	for i := 0; i < 3; i++ {
		_, err := lb.Select(ctx, "agent", "davinci")
		require.NoError(t, err)
	}

	// Change strategy
	lb.SetStrategy(StrategyRandom)
	assert.Equal(t, StrategyRandom, lb.Strategy())

	// Should still work with new strategy
	selected, err := lb.Select(ctx, "agent", "davinci")
	require.NoError(t, err)
	assert.NotEmpty(t, selected.InstanceID)
}

func TestLoadBalancer_ConcurrentAccess(t *testing.T) {
	reg := newMockRegistry()
	lb := NewLoadBalancer(reg, StrategyRoundRobin)

	ctx := context.Background()

	// Register three instances
	for i := 1; i <= 3; i++ {
		instance := sdkregistry.ServiceInfo{
			Kind:       "tool",
			Name:       "sqlmap",
			Version:    "1.0.0",
			InstanceID: "instance-" + string(rune('0'+i)),
			Endpoint:   "localhost:5005" + string(rune('0'+i)),
			StartedAt:  time.Now(),
		}
		err := reg.Register(ctx, instance)
		require.NoError(t, err)
	}

	// Simulate concurrent requests
	const numGoroutines = 10
	const selectionsPerGoroutine = 20

	done := make(chan bool, numGoroutines)
	errors := make(chan error, numGoroutines*selectionsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < selectionsPerGoroutine; j++ {
				_, err := lb.Select(ctx, "tool", "sqlmap")
				if err != nil {
					errors <- err
				}
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
	close(errors)

	// Should have no errors
	errorCount := 0
	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
		errorCount++
	}
	assert.Equal(t, 0, errorCount, "No errors should occur during concurrent access")
}
