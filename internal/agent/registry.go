package agent

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/zero-day-ai/gibson/internal/types"
)

// AgentFactory creates agent instances.
// Factories allow lazy instantiation and configuration of agents.
type AgentFactory func(cfg AgentConfig) (Agent, error)

// AgentRegistry manages agent registration and instantiation.
// The registry is the central point for discovering and creating agents.
type AgentRegistry interface {
	// RegisterInternal registers a native Go agent with a factory function
	RegisterInternal(name string, factory AgentFactory) error

	// RegisterExternal registers an external gRPC-based agent
	RegisterExternal(name string, client ExternalAgentClient) error

	// Unregister removes an agent from the registry
	Unregister(name string) error

	// List returns descriptors for all registered agents
	List() []AgentDescriptor

	// GetDescriptor returns the descriptor for a specific agent
	GetDescriptor(name string) (AgentDescriptor, error)

	// Create instantiates an agent with the given configuration
	Create(name string, cfg AgentConfig) (Agent, error)

	// DelegateToAgent executes a task using a named agent
	DelegateToAgent(ctx context.Context, name string, task Task, harness AgentHarness) (Result, error)

	// RunningAgents returns all currently executing agents
	RunningAgents() []AgentRuntime

	// Health returns the health status of the registry
	Health(ctx context.Context) types.HealthStatus
}

// ExternalAgentClient represents a gRPC client for external agents.
// Full implementation will be added in Stage 7 (gRPC integration).
type ExternalAgentClient interface {
	Agent
}

// DefaultAgentRegistry implements AgentRegistry using in-memory storage
type DefaultAgentRegistry struct {
	mu        sync.RWMutex
	factories map[string]AgentFactory
	external  map[string]ExternalAgentClient
	running   map[types.ID]*AgentRuntime
}

// NewAgentRegistry creates a new agent registry
func NewAgentRegistry() *DefaultAgentRegistry {
	return &DefaultAgentRegistry{
		factories: make(map[string]AgentFactory),
		external:  make(map[string]ExternalAgentClient),
		running:   make(map[types.ID]*AgentRuntime),
	}
}

// RegisterInternal registers a native Go agent with a factory function
func (r *DefaultAgentRegistry) RegisterInternal(name string, factory AgentFactory) error {
	if name == "" {
		return fmt.Errorf("agent name cannot be empty")
	}
	if factory == nil {
		return fmt.Errorf("agent factory cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("agent %s is already registered", name)
	}
	if _, exists := r.external[name]; exists {
		return fmt.Errorf("agent %s is already registered as external", name)
	}

	r.factories[name] = factory
	return nil
}

// RegisterExternal registers an external gRPC-based agent
func (r *DefaultAgentRegistry) RegisterExternal(name string, client ExternalAgentClient) error {
	if name == "" {
		return fmt.Errorf("agent name cannot be empty")
	}
	if client == nil {
		return fmt.Errorf("agent client cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.external[name]; exists {
		return fmt.Errorf("external agent %s is already registered", name)
	}
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("agent %s is already registered as internal", name)
	}

	r.external[name] = client
	return nil
}

// Unregister removes an agent from the registry
func (r *DefaultAgentRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[name]; exists {
		delete(r.factories, name)
		return nil
	}
	if _, exists := r.external[name]; exists {
		delete(r.external, name)
		return nil
	}

	return fmt.Errorf("agent %s not found", name)
}

// List returns descriptors for all registered agents
func (r *DefaultAgentRegistry) List() []AgentDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	descriptors := []AgentDescriptor{}

	// Get descriptors for internal agents
	for name, factory := range r.factories {
		// Create a temporary instance to get metadata
		cfg := NewAgentConfig(name)
		agent, err := factory(cfg)
		if err != nil {
			// Skip agents that fail to instantiate
			continue
		}
		desc := NewAgentDescriptor(agent)
		descriptors = append(descriptors, desc)
		// Clean up if the agent has a shutdown method
		_ = agent.Shutdown(context.Background())
	}

	// Get descriptors for external agents
	for _, client := range r.external {
		desc := NewAgentDescriptor(client)
		desc.IsExternal = true
		descriptors = append(descriptors, desc)
	}

	// Sort by name for consistent ordering
	sort.Slice(descriptors, func(i, j int) bool {
		return descriptors[i].Name < descriptors[j].Name
	})

	return descriptors
}

// GetDescriptor returns the descriptor for a specific agent
func (r *DefaultAgentRegistry) GetDescriptor(name string) (AgentDescriptor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check internal agents
	if factory, exists := r.factories[name]; exists {
		cfg := NewAgentConfig(name)
		agent, err := factory(cfg)
		if err != nil {
			return AgentDescriptor{}, fmt.Errorf("failed to instantiate agent %s: %w", name, err)
		}
		desc := NewAgentDescriptor(agent)
		_ = agent.Shutdown(context.Background())
		return desc, nil
	}

	// Check external agents
	if client, exists := r.external[name]; exists {
		desc := NewAgentDescriptor(client)
		desc.IsExternal = true
		return desc, nil
	}

	return AgentDescriptor{}, fmt.Errorf("agent %s not found", name)
}

// Create instantiates an agent with the given configuration
func (r *DefaultAgentRegistry) Create(name string, cfg AgentConfig) (Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try internal agents first
	if factory, exists := r.factories[name]; exists {
		agent, err := factory(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create agent %s: %w", name, err)
		}
		return agent, nil
	}

	// Try external agents
	if client, exists := r.external[name]; exists {
		// External agents don't need creation, return the client
		return client, nil
	}

	return nil, fmt.Errorf("agent %s not found", name)
}

// DelegateToAgent executes a task using a named agent.
// This is the implementation of the harness delegation method.
func (r *DefaultAgentRegistry) DelegateToAgent(ctx context.Context, name string, task Task, harness AgentHarness) (Result, error) {
	// Create agent instance
	cfg := NewAgentConfig(name)
	agent, err := r.Create(name, cfg)
	if err != nil {
		return Result{}, fmt.Errorf("failed to create agent %s: %w", name, err)
	}

	// Initialize agent
	if err := agent.Initialize(ctx, cfg); err != nil {
		return Result{}, fmt.Errorf("failed to initialize agent %s: %w", name, err)
	}

	// Track running agent
	runtime := NewAgentRuntime(name, task.ID)
	r.trackRuntime(runtime)
	defer r.untrackRuntime(runtime.ID)

	// Execute task
	result, err := agent.Execute(ctx, task, harness)
	if err != nil {
		runtime.Fail()
		return Result{}, fmt.Errorf("agent %s execution failed: %w", name, err)
	}

	// Update runtime status
	if result.Status == ResultStatusCompleted {
		runtime.Complete()
	} else if result.Status == ResultStatusFailed {
		runtime.Fail()
	} else if result.Status == ResultStatusCancelled {
		runtime.Cancel()
	}

	// Shutdown agent
	if shutdownErr := agent.Shutdown(ctx); shutdownErr != nil {
		// Log shutdown error but don't fail the task
		if harness != nil {
			harness.Log("warn", "agent shutdown failed", map[string]any{
				"agent": name,
				"error": shutdownErr.Error(),
			})
		}
	}

	return result, nil
}

// RunningAgents returns all currently executing agents
func (r *DefaultAgentRegistry) RunningAgents() []AgentRuntime {
	r.mu.RLock()
	defer r.mu.RUnlock()

	runtimes := make([]AgentRuntime, 0, len(r.running))
	for _, runtime := range r.running {
		runtimes = append(runtimes, *runtime)
	}

	// Sort by start time (newest first)
	sort.Slice(runtimes, func(i, j int) bool {
		return runtimes[i].StartedAt.After(runtimes[j].StartedAt)
	})

	return runtimes
}

// Health returns the health status of the registry
func (r *DefaultAgentRegistry) Health(ctx context.Context) types.HealthStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check health of external agents
	unhealthyExternal := 0
	for _, client := range r.external {
		if health := client.Health(ctx); !health.IsHealthy() {
			unhealthyExternal++
		}
	}

	if unhealthyExternal > 0 {
		return types.Degraded(fmt.Sprintf("%d external agents are unhealthy", unhealthyExternal))
	}

	return types.Healthy(fmt.Sprintf("Registry healthy: %d internal, %d external, %d running",
		len(r.factories), len(r.external), len(r.running)))
}

// trackRuntime adds a runtime to the tracking map
func (r *DefaultAgentRegistry) trackRuntime(runtime *AgentRuntime) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running[runtime.ID] = runtime
}

// untrackRuntime removes a runtime from the tracking map
func (r *DefaultAgentRegistry) untrackRuntime(id types.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.running, id)
}

// Count returns the number of registered agents
func (r *DefaultAgentRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.factories) + len(r.external)
}

// IsRegistered checks if an agent is registered
func (r *DefaultAgentRegistry) IsRegistered(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, internal := r.factories[name]
	_, external := r.external[name]
	return internal || external
}
