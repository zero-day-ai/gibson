// Package registry provides load balancing capabilities for service discovery.
//
// This file implements simple load balancing strategies without external dependencies.
// The load balancer wraps a Registry and provides intelligent instance selection.
package registry

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"

	sdkregistry "github.com/zero-day-ai/sdk/registry"
)

// LoadBalanceStrategy defines load balancing algorithms.
type LoadBalanceStrategy string

const (
	// StrategyRoundRobin distributes requests evenly across instances in rotation
	StrategyRoundRobin LoadBalanceStrategy = "round_robin"

	// StrategyRandom selects instances randomly with uniform distribution
	StrategyRandom LoadBalanceStrategy = "random"

	// StrategyLeastConnection selects the instance with fewest active connections
	// Note: Currently unimplemented, falls back to RoundRobin
	StrategyLeastConnection LoadBalanceStrategy = "least_connection"
)

// LoadBalancer provides load-balanced service discovery.
//
// It wraps a Registry and adds instance selection strategies for distributing
// requests across multiple instances of the same service. This enables horizontal
// scaling and fault tolerance.
//
// Example usage:
//
//	reg, _ := NewExternalRegistry(cfg)
//	lb := NewLoadBalancer(reg, StrategyRoundRobin)
//
//	// Select an instance for each request
//	for i := 0; i < 10; i++ {
//	    endpoint, _ := lb.SelectEndpoint(ctx, "agent", "k8skiller")
//	    // Make request to endpoint
//	}
//
// Thread-safe: All methods can be called concurrently.
type LoadBalancer struct {
	registry sdkregistry.Registry
	strategy LoadBalanceStrategy

	// State for round-robin strategy (per kind:name key)
	rrMutex    sync.RWMutex
	rrCounters map[string]*uint64

	// State for least-connection strategy (future use)
	connMutex  sync.RWMutex
	connCounts map[string]int // endpoint -> connection count
}

// NewLoadBalancer creates a load balancer wrapping a registry.
//
// The strategy parameter determines how instances are selected:
//   - StrategyRoundRobin: Cycles through instances sequentially
//   - StrategyRandom: Selects instances randomly
//   - StrategyLeastConnection: Selects instance with fewest connections (future)
//
// The load balancer does not take ownership of the registry. The caller is
// responsible for closing the registry when done.
func NewLoadBalancer(reg sdkregistry.Registry, strategy LoadBalanceStrategy) *LoadBalancer {
	return &LoadBalancer{
		registry:   reg,
		strategy:   strategy,
		rrCounters: make(map[string]*uint64),
		connCounts: make(map[string]int),
	}
}

// Select returns a single service instance based on the configured strategy.
//
// If multiple instances are registered, the strategy determines which one is returned:
//   - RoundRobin: Returns the next instance in sequence
//   - Random: Returns a random instance
//   - LeastConnection: Returns instance with fewest connections (currently same as RoundRobin)
//
// If no instances are registered, returns ErrNoInstances.
// If only one instance exists, it is always returned regardless of strategy.
//
// Example:
//
//	instance, err := lb.Select(ctx, "agent", "davinci")
//	if err != nil {
//	    return err
//	}
//	// Connect to instance.Endpoint
func (lb *LoadBalancer) Select(ctx context.Context, kind, name string) (*sdkregistry.ServiceInfo, error) {
	instances, err := lb.registry.Discover(ctx, kind, name)
	if err != nil {
		return nil, fmt.Errorf("discover failed: %w", err)
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("no instances of %s:%s available", kind, name)
	}

	// Single instance - no load balancing needed
	if len(instances) == 1 {
		return &instances[0], nil
	}

	// Apply strategy
	var selected *sdkregistry.ServiceInfo
	switch lb.strategy {
	case StrategyRoundRobin:
		selected = lb.selectRoundRobin(kind, name, instances)
	case StrategyRandom:
		selected = lb.selectRandom(instances)
	case StrategyLeastConnection:
		// TODO: Implement least-connection tracking
		// For now, fall back to round-robin
		selected = lb.selectRoundRobin(kind, name, instances)
	default:
		// Unknown strategy - default to round-robin
		selected = lb.selectRoundRobin(kind, name, instances)
	}

	return selected, nil
}

// SelectEndpoint returns just the endpoint string for convenience.
//
// This is a shorthand for Select() that extracts only the endpoint field.
// Useful when you just need the network address and don't care about metadata.
//
// Example:
//
//	endpoint, err := lb.SelectEndpoint(ctx, "tool", "nmap")
//	if err != nil {
//	    return err
//	}
//	conn, err := grpc.Dial(endpoint, grpc.WithInsecure())
func (lb *LoadBalancer) SelectEndpoint(ctx context.Context, kind, name string) (string, error) {
	instance, err := lb.Select(ctx, kind, name)
	if err != nil {
		return "", err
	}
	return instance.Endpoint, nil
}

// selectRoundRobin implements round-robin selection.
//
// Maintains a per-service counter that increments with each call.
// Uses atomic operations for thread safety without locks during selection.
func (lb *LoadBalancer) selectRoundRobin(kind, name string, instances []sdkregistry.ServiceInfo) *sdkregistry.ServiceInfo {
	key := fmt.Sprintf("%s:%s", kind, name)

	// Get or create counter for this service
	lb.rrMutex.Lock()
	counter, exists := lb.rrCounters[key]
	if !exists {
		var zero uint64
		counter = &zero
		lb.rrCounters[key] = counter
	}
	lb.rrMutex.Unlock()

	// Atomically increment and select
	count := atomic.AddUint64(counter, 1)
	index := (count - 1) % uint64(len(instances))
	return &instances[index]
}

// selectRandom implements random selection.
//
// Uses uniform random distribution across all instances.
// Each instance has equal probability of being selected.
func (lb *LoadBalancer) selectRandom(instances []sdkregistry.ServiceInfo) *sdkregistry.ServiceInfo {
	index := rand.Intn(len(instances))
	return &instances[index]
}

// Strategy returns the current load balancing strategy.
func (lb *LoadBalancer) Strategy() LoadBalanceStrategy {
	return lb.strategy
}

// SetStrategy changes the load balancing strategy at runtime.
//
// Note: Changing strategies resets internal state (e.g., round-robin counters).
func (lb *LoadBalancer) SetStrategy(strategy LoadBalanceStrategy) {
	lb.strategy = strategy

	// Reset round-robin state when changing strategies
	lb.rrMutex.Lock()
	lb.rrCounters = make(map[string]*uint64)
	lb.rrMutex.Unlock()
}
