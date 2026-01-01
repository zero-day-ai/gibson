package component

import (
	"context"
	"fmt"
)

// DiscoverySource represents the source of a discovered component.
// This is distinct from ComponentSource which indicates the origin/installation source.
// DiscoverySource indicates where the component was discovered at runtime.
type DiscoverySource string

const (
	// DiscoverySourceLocal indicates the component was discovered via filesystem
	// (PID files, lock files, Unix domain sockets).
	DiscoverySourceLocal DiscoverySource = "local"

	// DiscoverySourceRemote indicates the component was discovered via network
	// (gRPC health probes, configuration-based discovery).
	DiscoverySourceRemote DiscoverySource = "remote"
)

// String returns the string representation of the DiscoverySource.
func (s DiscoverySource) String() string {
	return string(s)
}

// IsValid checks if the DiscoverySource is a valid enum value.
func (s DiscoverySource) IsValid() bool {
	return s == DiscoverySourceLocal || s == DiscoverySourceRemote
}

// DiscoveredComponent represents a component discovered at runtime through
// either local filesystem tracking (PID files, sockets) or remote network
// probing (gRPC health checks). This is the unified view used by the attack
// runner and CLI to get the current state of all available components.
type DiscoveredComponent struct {
	// Kind is the type of component (agent, tool, or plugin).
	Kind ComponentKind `json:"kind"`

	// Name is the unique identifier for the component within its kind.
	Name string `json:"name"`

	// Source indicates where the component was discovered (local or remote).
	Source DiscoverySource `json:"source"`

	// Address is the connection address for the component.
	// For local components: Unix socket path (e.g., "/home/user/.gibson/run/agent/k8skiller.sock")
	// For remote components: TCP address (e.g., "k8skiller-container:50051" or "192.168.1.100:50052")
	Address string `json:"address"`

	// Healthy indicates whether the component passed its health check.
	// For local: socket exists and gRPC health check succeeded
	// For remote: gRPC health probe succeeded within timeout
	Healthy bool `json:"healthy"`

	// Port is the TCP port the component is listening on for gRPC connections.
	// For local components: read from PID file or determined during socket health check
	// For remote components: extracted from the configured address
	Port int `json:"port"`
}

// Validate validates the DiscoveredComponent fields.
func (d *DiscoveredComponent) Validate() error {
	if !d.Kind.IsValid() {
		return fmt.Errorf("invalid component kind: %s", d.Kind)
	}

	if d.Name == "" {
		return fmt.Errorf("component name is required")
	}

	if !d.Source.IsValid() {
		return fmt.Errorf("invalid discovery source: %s", d.Source)
	}

	if d.Address == "" {
		return fmt.Errorf("component address is required")
	}

	// Port should be valid for healthy components (0 is acceptable for unhealthy)
	if d.Healthy && (d.Port < 1 || d.Port > 65535) {
		return fmt.Errorf("port must be between 1 and 65535 for healthy components, got %d", d.Port)
	}

	return nil
}

// IsLocal returns true if the component was discovered locally via filesystem.
func (d *DiscoveredComponent) IsLocal() bool {
	return d.Source == DiscoverySourceLocal
}

// IsRemote returns true if the component was discovered remotely via network.
func (d *DiscoveredComponent) IsRemote() bool {
	return d.Source == DiscoverySourceRemote
}

// UnifiedDiscovery provides a single interface for discovering all components
// regardless of their deployment location (local or remote). It queries both
// LocalTracker (filesystem-based discovery) and RemoteProber (network-based
// discovery) to provide a complete view of available components.
//
// The attack runner and CLI commands use this interface to get the current
// state of all components without needing to know whether they're running
// locally or on remote hosts.
//
// Discovery precedence: When a component exists in both local and remote
// configurations, the local component takes precedence.
type UnifiedDiscovery interface {
	// DiscoverAll returns all discovered components (agents, tools, and plugins)
	// from both local and remote sources. Only healthy components are included.
	//
	// Local components are discovered by scanning ~/.gibson/run/{kind}/*.pid files
	// and validating process liveness and socket health.
	//
	// Remote components are discovered by loading configuration and performing
	// parallel gRPC health probes.
	//
	// If a component with the same kind and name exists in both local and remote,
	// the local instance takes precedence and the remote is excluded from results.
	//
	// Returns an error if both local and remote discovery fail. Returns partial
	// results if only one source fails.
	DiscoverAll(ctx context.Context) ([]DiscoveredComponent, error)

	// DiscoverAgents returns all discovered agents (both local and remote).
	// This is equivalent to DiscoverAll filtered to ComponentKindAgent.
	//
	// Agents are LLM-powered autonomous components that perform security testing.
	// This method is commonly used by the attack runner to find available agents
	// for task execution.
	//
	// Returns an error if both local and remote discovery fail. Returns partial
	// results if only one source fails.
	DiscoverAgents(ctx context.Context) ([]DiscoveredComponent, error)

	// DiscoverTools returns all discovered tools (both local and remote).
	// This is equivalent to DiscoverAll filtered to ComponentKindTool.
	//
	// Tools are stateless, deterministic components that perform specific operations
	// (e.g., nmap, sqlmap, nuclei). Agents call tools during task execution.
	//
	// Returns an error if both local and remote discovery fail. Returns partial
	// results if only one source fails.
	DiscoverTools(ctx context.Context) ([]DiscoveredComponent, error)

	// DiscoverPlugins returns all discovered plugins (both local and remote).
	// This is equivalent to DiscoverAll filtered to ComponentKindPlugin.
	//
	// Plugins are stateful services that provide external data access or specialized
	// functionality (e.g., CVE database, credential vault).
	//
	// Returns an error if both local and remote discovery fail. Returns partial
	// results if only one source fails.
	DiscoverPlugins(ctx context.Context) ([]DiscoveredComponent, error)

	// GetComponent retrieves a specific component by kind and name from either
	// local or remote sources. Returns the first healthy instance found, with
	// local taking precedence over remote.
	//
	// Returns nil if the component is not found or is unhealthy.
	// Returns an error if discovery fails for both sources.
	//
	// Example:
	//   agent, err := discovery.GetComponent(ctx, ComponentKindAgent, "k8skiller")
	GetComponent(ctx context.Context, kind ComponentKind, name string) (*DiscoveredComponent, error)
}

// DefaultUnifiedDiscovery is the default implementation of UnifiedDiscovery.
// It coordinates between LocalTracker for filesystem-based discovery and
// RemoteProber for network-based discovery.
type DefaultUnifiedDiscovery struct {
	localTracker  LocalTracker
	remoteProber  RemoteProber
	logger        Logger // Uses existing Logger interface from config.go
}

// NewDefaultUnifiedDiscovery creates a new DefaultUnifiedDiscovery instance.
// If logger is nil, warnings are not logged (silent mode).
func NewDefaultUnifiedDiscovery(localTracker LocalTracker, remoteProber RemoteProber, logger Logger) *DefaultUnifiedDiscovery {
	return &DefaultUnifiedDiscovery{
		localTracker: localTracker,
		remoteProber: remoteProber,
		logger:       logger,
	}
}

// DiscoverAll returns all discovered components from both local and remote sources.
// Local components take precedence over remote when the same component exists in both.
func (d *DefaultUnifiedDiscovery) DiscoverAll(ctx context.Context) ([]DiscoveredComponent, error) {
	var localErr, remoteErr error
	var localComponents, remoteComponents []DiscoveredComponent

	// Discover local components for all kinds
	localComponents, localErr = d.discoverLocal(ctx)
	if localErr != nil && d.logger != nil {
		d.logger.Warnf("Local discovery failed: %v", localErr)
	}

	// Discover remote components
	remoteComponents, remoteErr = d.discoverRemote(ctx)
	if remoteErr != nil && d.logger != nil {
		d.logger.Warnf("Remote discovery failed: %v", remoteErr)
	}

	// If both failed, return error
	if localErr != nil && remoteErr != nil {
		return nil, fmt.Errorf("both local and remote discovery failed: local=%v, remote=%v", localErr, remoteErr)
	}

	// Merge results with local precedence
	merged := d.mergeComponents(localComponents, remoteComponents)

	return merged, nil
}

// DiscoverAgents returns all discovered agents (both local and remote).
func (d *DefaultUnifiedDiscovery) DiscoverAgents(ctx context.Context) ([]DiscoveredComponent, error) {
	all, err := d.DiscoverAll(ctx)
	if err != nil {
		return nil, err
	}

	// Filter to agents only
	agents := make([]DiscoveredComponent, 0, len(all))
	for _, comp := range all {
		if comp.Kind == ComponentKindAgent {
			agents = append(agents, comp)
		}
	}

	return agents, nil
}

// DiscoverTools returns all discovered tools (both local and remote).
func (d *DefaultUnifiedDiscovery) DiscoverTools(ctx context.Context) ([]DiscoveredComponent, error) {
	all, err := d.DiscoverAll(ctx)
	if err != nil {
		return nil, err
	}

	// Filter to tools only
	tools := make([]DiscoveredComponent, 0, len(all))
	for _, comp := range all {
		if comp.Kind == ComponentKindTool {
			tools = append(tools, comp)
		}
	}

	return tools, nil
}

// DiscoverPlugins returns all discovered plugins (both local and remote).
func (d *DefaultUnifiedDiscovery) DiscoverPlugins(ctx context.Context) ([]DiscoveredComponent, error) {
	all, err := d.DiscoverAll(ctx)
	if err != nil {
		return nil, err
	}

	// Filter to plugins only
	plugins := make([]DiscoveredComponent, 0, len(all))
	for _, comp := range all {
		if comp.Kind == ComponentKindPlugin {
			plugins = append(plugins, comp)
		}
	}

	return plugins, nil
}

// GetComponent retrieves a specific component by kind and name.
// Returns the local instance if available, otherwise the remote instance.
// Returns nil if the component is not found or is unhealthy.
func (d *DefaultUnifiedDiscovery) GetComponent(ctx context.Context, kind ComponentKind, name string) (*DiscoveredComponent, error) {
	// Try local first
	localState, err := d.localTracker.Discover(ctx, kind)
	if err == nil {
		// Search for the component in local results
		for _, state := range localState {
			if state.Name == name && state.Healthy {
				comp := d.localStateToDiscovered(state)
				return &comp, nil
			}
		}
	}

	// Try remote if not found locally
	remoteStates, err := d.remoteProber.ProbeAll(ctx)
	if err == nil {
		// Search for the component in remote results
		for _, state := range remoteStates {
			if state.Kind == kind && state.Name == name && state.Healthy {
				comp := d.remoteStateToDiscovered(state)
				return &comp, nil
			}
		}
	}

	// Not found in either source
	return nil, nil
}

// discoverLocal discovers all local components across all kinds.
func (d *DefaultUnifiedDiscovery) discoverLocal(ctx context.Context) ([]DiscoveredComponent, error) {
	var allComponents []DiscoveredComponent
	var errs []error

	// Discover agents
	agents, err := d.localTracker.Discover(ctx, ComponentKindAgent)
	if err != nil {
		errs = append(errs, fmt.Errorf("agent discovery failed: %w", err))
	} else {
		for _, state := range agents {
			if state.Healthy {
				allComponents = append(allComponents, d.localStateToDiscovered(state))
			}
		}
	}

	// Discover tools
	tools, err := d.localTracker.Discover(ctx, ComponentKindTool)
	if err != nil {
		errs = append(errs, fmt.Errorf("tool discovery failed: %w", err))
	} else {
		for _, state := range tools {
			if state.Healthy {
				allComponents = append(allComponents, d.localStateToDiscovered(state))
			}
		}
	}

	// Discover plugins
	plugins, err := d.localTracker.Discover(ctx, ComponentKindPlugin)
	if err != nil {
		errs = append(errs, fmt.Errorf("plugin discovery failed: %w", err))
	} else {
		for _, state := range plugins {
			if state.Healthy {
				allComponents = append(allComponents, d.localStateToDiscovered(state))
			}
		}
	}

	// If all discoveries failed, return error
	if len(errs) == 3 {
		return nil, fmt.Errorf("all local discoveries failed: %v", errs)
	}

	return allComponents, nil
}

// discoverRemote discovers all remote components.
func (d *DefaultUnifiedDiscovery) discoverRemote(ctx context.Context) ([]DiscoveredComponent, error) {
	states, err := d.remoteProber.ProbeAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("remote probe failed: %w", err)
	}

	components := make([]DiscoveredComponent, 0, len(states))
	for _, state := range states {
		if state.Healthy {
			components = append(components, d.remoteStateToDiscovered(state))
		}
	}

	return components, nil
}

// mergeComponents merges local and remote components, with local taking precedence.
// If the same component (kind+name) exists in both, only the local instance is included.
func (d *DefaultUnifiedDiscovery) mergeComponents(local, remote []DiscoveredComponent) []DiscoveredComponent {
	// Create a map of local components for fast lookup
	localMap := make(map[string]DiscoveredComponent)
	for _, comp := range local {
		key := string(comp.Kind) + "/" + comp.Name
		localMap[key] = comp
	}

	// Start with all local components
	merged := make([]DiscoveredComponent, 0, len(local)+len(remote))
	merged = append(merged, local...)

	// Add remote components that don't conflict with local
	for _, comp := range remote {
		key := string(comp.Kind) + "/" + comp.Name
		if localComp, exists := localMap[key]; exists {
			// Local takes precedence - log the conflict if logger is available
			if d.logger != nil {
				d.logger.Warnf("Local component shadows remote: %s/%s (local address: %s, remote address: %s)",
					comp.Kind, comp.Name, localComp.Address, comp.Address)
			}
		} else {
			merged = append(merged, comp)
		}
	}

	return merged
}

// localStateToDiscovered converts a LocalComponentState to a DiscoveredComponent.
func (d *DefaultUnifiedDiscovery) localStateToDiscovered(state LocalComponentState) DiscoveredComponent {
	return DiscoveredComponent{
		Kind:    state.Kind,
		Name:    state.Name,
		Source:  DiscoverySourceLocal,
		Address: state.Socket, // Unix socket path
		Healthy: state.Healthy,
		Port:    state.Port,
	}
}

// remoteStateToDiscovered converts a RemoteComponentState to a DiscoveredComponent.
func (d *DefaultUnifiedDiscovery) remoteStateToDiscovered(state RemoteComponentState) DiscoveredComponent {
	// Extract port from address if possible
	port := 0
	if addr := state.Address; addr != "" {
		// Try to parse host:port format
		if colonIdx := len(addr) - 1; colonIdx > 0 {
			for i := len(addr) - 1; i >= 0; i-- {
				if addr[i] == ':' {
					colonIdx = i
					break
				}
			}
			if colonIdx > 0 && colonIdx < len(addr)-1 {
				portStr := addr[colonIdx+1:]
				fmt.Sscanf(portStr, "%d", &port)
			}
		}
	}

	return DiscoveredComponent{
		Kind:    state.Kind,
		Name:    state.Name,
		Source:  DiscoverySourceRemote,
		Address: state.Address, // TCP address (host:port)
		Healthy: state.Healthy,
		Port:    port,
	}
}
