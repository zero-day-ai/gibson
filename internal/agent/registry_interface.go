package agent

import (
	"context"

	"github.com/zero-day-ai/gibson/internal/types"
)

// AgentFactory creates agent instances.
// Factories allow lazy instantiation and configuration of agents.
//
// Deprecated: This type is part of the legacy registry system.
// New code should use registry.ComponentDiscovery for agent discovery.
type AgentFactory func(cfg AgentConfig) (Agent, error)

// AgentRegistry manages agent registration and instantiation.
// The registry is the central point for discovering and creating agents.
//
// Deprecated: This interface is part of the legacy registry system and will be removed.
// New code should use registry.ComponentDiscovery instead, which provides unified
// component discovery via etcd.
//
// This interface is temporarily preserved for backward compatibility during migration:
// - delegation.go still uses this interface
// - harness/config.go has deprecated AgentRegistry field
// - tui/app.go has deprecated AgentRegistry field
//
// These will be removed in Task 5.2 after all consumers migrate to RegistryAdapter.
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
//
// Deprecated: Part of the legacy registry system. Use registry.ComponentDiscovery instead.
type ExternalAgentClient interface {
	Agent
}
