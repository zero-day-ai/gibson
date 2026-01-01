package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/types"
	proto "github.com/zero-day-ai/sdk/api/gen/proto"
)

// GRPCAgentRegistry implements AgentRegistry using gRPC connections to external agent processes.
//
// This registry manages connections to external agents running as separate processes,
// communicating via gRPC. It provides:
// - Lazy connection establishment (connect on first use)
// - Connection pooling and reuse
// - Thread-safe operations
// - Descriptor caching for performance
// - Agent process lifecycle management (discovery, startup, health checking, shutdown)
//
// The registry discovers agents by scanning configured paths for component.yaml manifests,
// starts agent processes on demand, and stops them during cleanup.
type GRPCAgentRegistry struct {
	// agentPaths contains paths to scan for agent component.yaml manifests
	agentPaths []string

	// timeout for gRPC connection establishment and RPC calls
	timeout time.Duration

	// lifecycleManager handles agent process lifecycle (start, stop, health checks)
	lifecycleManager component.LifecycleManager

	// clients maps agent name to established gRPC connection
	clients map[string]*grpc.ClientConn

	// descriptors caches agent descriptors from GetDescriptor calls
	descriptors map[string]AgentDescriptor

	// addresses maps agent name to gRPC address (host:port)
	// Populated by agent discovery from component.yaml manifests
	addresses map[string]string

	// components maps agent name to Component with manifest and runtime info
	// Populated by discoverAgents()
	components map[string]*component.Component

	// mu protects all maps for concurrent access
	mu sync.RWMutex
}

// GRPCAgentRegistryOption configures the GRPCAgentRegistry
type GRPCAgentRegistryOption func(*GRPCAgentRegistry)

// WithAgentPaths sets the paths to scan for agent manifests
func WithAgentPaths(paths []string) GRPCAgentRegistryOption {
	return func(r *GRPCAgentRegistry) {
		r.agentPaths = paths
	}
}

// WithTimeout sets the gRPC connection and RPC timeout
func WithTimeout(timeout time.Duration) GRPCAgentRegistryOption {
	return func(r *GRPCAgentRegistry) {
		r.timeout = timeout
	}
}

// WithLifecycleManager sets the lifecycle manager for agent process management
func WithLifecycleManager(lm component.LifecycleManager) GRPCAgentRegistryOption {
	return func(r *GRPCAgentRegistry) {
		r.lifecycleManager = lm
	}
}

// NewGRPCAgentRegistry creates a new gRPC-based agent registry.
//
// The registry starts with no active connections. Connections are established
// lazily when agents are first accessed via Get or DelegateToAgent.
// If agentPaths are configured, it discovers agents by scanning for component.yaml manifests.
func NewGRPCAgentRegistry(opts ...GRPCAgentRegistryOption) *GRPCAgentRegistry {
	r := &GRPCAgentRegistry{
		agentPaths:  []string{},
		timeout:     30 * time.Second, // Default timeout
		clients:     make(map[string]*grpc.ClientConn),
		descriptors: make(map[string]AgentDescriptor),
		addresses:   make(map[string]string),
		components:  make(map[string]*component.Component),
	}

	for _, opt := range opts {
		opt(r)
	}

	// Discover agents from configured paths
	if len(r.agentPaths) > 0 {
		_ = r.discoverAgents() // Ignore errors during initialization
	}

	return r
}

// RegisterInternal is not supported by GRPCAgentRegistry.
// This registry only manages external gRPC agents.
func (r *GRPCAgentRegistry) RegisterInternal(name string, factory AgentFactory) error {
	return fmt.Errorf("GRPCAgentRegistry does not support internal agents")
}

// RegisterExternal registers an external gRPC agent at the given address.
//
// The address should be in the form "host:port" (e.g., "localhost:50051").
// The connection is not established immediately; it will be created lazily
// when the agent is first accessed.
func (r *GRPCAgentRegistry) RegisterExternal(name string, client ExternalAgentClient) error {
	if name == "" {
		return fmt.Errorf("agent name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// If the client is a GRPCAgentClient, we can extract its connection
	// For now, we just store it as registered
	// Task 3.2 will add proper agent discovery
	return fmt.Errorf("RegisterExternal not yet fully implemented (task 3.2)")
}

// RegisterAddress registers an agent's gRPC address without establishing a connection.
// This is used by agent discovery to register agents found in component manifests.
func (r *GRPCAgentRegistry) RegisterAddress(name, address string) error {
	if name == "" {
		return fmt.Errorf("agent name cannot be empty")
	}
	if address == "" {
		return fmt.Errorf("agent address cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.addresses[name] = address
	return nil
}

// Unregister removes an agent from the registry and closes its connection.
func (r *GRPCAgentRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Close connection if it exists
	if conn, exists := r.clients[name]; exists {
		if err := conn.Close(); err != nil {
			return fmt.Errorf("failed to close connection for agent %s: %w", name, err)
		}
		delete(r.clients, name)
	}

	// Remove from maps
	delete(r.addresses, name)
	delete(r.descriptors, name)

	return nil
}

// List returns descriptors for all registered agents.
//
// For agents that haven't been accessed yet, this will establish a connection
// and fetch their descriptors. Descriptors are cached for subsequent calls.
func (r *GRPCAgentRegistry) List() []AgentDescriptor {
	r.mu.Lock()
	defer r.mu.Unlock()

	descriptors := make([]AgentDescriptor, 0, len(r.addresses))

	// Return cached descriptors
	for _, desc := range r.descriptors {
		descriptors = append(descriptors, desc)
	}

	// For addresses without cached descriptors, fetch them
	for name, address := range r.addresses {
		if _, cached := r.descriptors[name]; !cached {
			// Try to get descriptor (may fail if agent is not running)
			ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
			defer cancel()

			desc, err := r.getDescriptorLocked(ctx, name, address)
			if err != nil {
				// Skip agents that fail to respond
				continue
			}
			descriptors = append(descriptors, desc)
		}
	}

	return descriptors
}

// GetDescriptor returns the descriptor for a specific agent.
func (r *GRPCAgentRegistry) GetDescriptor(name string) (AgentDescriptor, error) {
	r.mu.RLock()
	// Check cache first
	if desc, exists := r.descriptors[name]; exists {
		r.mu.RUnlock()
		return desc, nil
	}
	r.mu.RUnlock()

	// Get address
	r.mu.RLock()
	address, exists := r.addresses[name]
	r.mu.RUnlock()

	if !exists {
		return AgentDescriptor{}, fmt.Errorf("agent %s not found", name)
	}

	// Fetch descriptor
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.getDescriptorLocked(ctx, name, address)
}

// getDescriptorLocked fetches a descriptor from the agent via gRPC.
// Caller must hold r.mu for writing.
func (r *GRPCAgentRegistry) getDescriptorLocked(ctx context.Context, name, address string) (AgentDescriptor, error) {
	// Ensure connection exists
	conn, err := r.ensureConnectionLocked(ctx, name, address)
	if err != nil {
		return AgentDescriptor{}, fmt.Errorf("failed to connect to agent %s: %w", name, err)
	}

	// Create gRPC client
	client := proto.NewAgentServiceClient(conn)

	// Fetch descriptor
	resp, err := client.GetDescriptor(ctx, &proto.AgentGetDescriptorRequest{})
	if err != nil {
		return AgentDescriptor{}, fmt.Errorf("failed to get descriptor from agent %s: %w", name, err)
	}

	// Convert proto descriptor to internal type
	desc := AgentDescriptor{
		Name:           resp.Name,
		Version:        resp.Version,
		Description:    resp.Description,
		Capabilities:   resp.Capabilities,
		TargetTypes:    convertTargetTypes(resp.TargetTypes),
		TechniqueTypes: convertTechniqueTypes(resp.TechniqueTypes),
		IsExternal:     true,
	}

	// Cache the descriptor
	r.descriptors[name] = desc

	return desc, nil
}

// Create instantiates an agent with the given configuration.
//
// For gRPC agents, this returns a GRPCAgentClient wrapping the connection.
// The agent process must already be running; this method does not start it.
func (r *GRPCAgentRegistry) Create(name string, cfg AgentConfig) (Agent, error) {
	r.mu.RLock()
	address, exists := r.addresses[name]
	// Debug: log registered addresses
	registeredNames := make([]string, 0, len(r.addresses))
	for k := range r.addresses {
		registeredNames = append(registeredNames, k)
	}
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("agent %s not found (registered agents: %v)", name, registeredNames)
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Ensure connection exists
	conn, err := r.ensureConnectionLocked(ctx, name, address)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to agent %s: %w", name, err)
	}

	// Get descriptor for metadata
	desc, err := r.getDescriptorLocked(ctx, name, address)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent descriptor: %w", err)
	}

	// Create GRPCAgentClient
	client := &GRPCAgentClient{
		name:        desc.Name,
		version:     desc.Version,
		description: desc.Description,
		conn:        conn,
		client:      proto.NewAgentServiceClient(conn),
	}

	return client, nil
}

// DelegateToAgent executes a task using a named agent.
//
// This ensures the agent process is running, establishes a connection to the agent
// (if not already connected), sends the task via gRPC, and returns the result.
func (r *GRPCAgentRegistry) DelegateToAgent(ctx context.Context, name string, task Task, harness AgentHarness) (Result, error) {
	// Get agent address
	r.mu.RLock()
	address, exists := r.addresses[name]
	r.mu.RUnlock()

	if !exists {
		return Result{}, fmt.Errorf("agent %s not found", name)
	}

	// Ensure agent process is running (if lifecycle manager is configured)
	if err := r.ensureRunning(ctx, name); err != nil {
		return Result{}, fmt.Errorf("failed to start agent %s: %w", name, err)
	}

	// Ensure connection with timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	r.mu.Lock()
	conn, err := r.ensureConnectionLocked(ctxWithTimeout, name, address)
	r.mu.Unlock()

	if err != nil {
		return Result{}, fmt.Errorf("failed to connect to agent %s: %w", name, err)
	}

	// Create gRPC client
	client := proto.NewAgentServiceClient(conn)

	// Marshal task to JSON
	taskJSON, err := json.Marshal(task)
	if err != nil {
		return Result{}, fmt.Errorf("failed to marshal task: %w", err)
	}

	// Send Execute request
	timeoutMs := int64(task.Timeout.Milliseconds())
	req := &proto.AgentExecuteRequest{
		TaskJson:  string(taskJSON),
		TimeoutMs: timeoutMs,
	}

	resp, err := client.Execute(ctx, req)
	if err != nil {
		result := NewResult(task.ID)
		result.Fail(fmt.Errorf("agent execution failed: %w", err))
		return result, nil
	}

	// Check for errors in response
	if resp.Error != nil {
		result := NewResult(task.ID)
		result.Fail(fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message))
		return result, nil
	}

	// Unmarshal result from JSON
	var result Result
	if err := json.Unmarshal([]byte(resp.ResultJson), &result); err != nil {
		return Result{}, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return result, nil
}

// RunningAgents returns all currently executing agents.
//
// Note: This information is not available from gRPC agents.
// Returns an empty slice for now.
func (r *GRPCAgentRegistry) RunningAgents() []AgentRuntime {
	// gRPC agents don't expose runtime information
	// This would need to be tracked separately if needed
	return []AgentRuntime{}
}

// Health returns the health status of the registry.
//
// Checks connectivity to all registered agents and reports any issues.
func (r *GRPCAgentRegistry) Health(ctx context.Context) types.HealthStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	totalAgents := len(r.addresses)
	if totalAgents == 0 {
		return types.Healthy("No agents registered")
	}

	unhealthyCount := 0
	for name := range r.addresses {
		// Check if connection exists and is healthy
		conn, exists := r.clients[name]
		if !exists {
			// Not connected yet, skip health check
			continue
		}

		// Check connection state
		state := conn.GetState()
		if state.String() != "READY" && state.String() != "IDLE" {
			unhealthyCount++
		}
	}

	if unhealthyCount > 0 {
		return types.Degraded(fmt.Sprintf("%d/%d agents unhealthy", unhealthyCount, totalAgents))
	}

	return types.Healthy(fmt.Sprintf("All %d agents healthy", totalAgents))
}

// Close closes all gRPC connections and stops all managed agent processes.
func (r *GRPCAgentRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var lastErr error

	// Close all gRPC connections
	for name, conn := range r.clients {
		if err := conn.Close(); err != nil {
			lastErr = fmt.Errorf("failed to close connection for agent %s: %w", name, err)
		}
	}

	// Stop all managed agent processes (if lifecycle manager is configured)
	if r.lifecycleManager != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		for name, comp := range r.components {
			if comp.IsRunning() {
				if err := r.lifecycleManager.StopComponent(ctx, comp); err != nil {
					lastErr = fmt.Errorf("failed to stop agent %s: %w", name, err)
				}
			}
		}
	}

	// Clear maps
	r.clients = make(map[string]*grpc.ClientConn)
	r.descriptors = make(map[string]AgentDescriptor)

	return lastErr
}

// ensureConnectionLocked ensures a gRPC connection exists for the given agent.
// Returns the existing connection or creates a new one.
// Caller must hold r.mu for writing.
func (r *GRPCAgentRegistry) ensureConnectionLocked(ctx context.Context, name, address string) (*grpc.ClientConn, error) {
	// Check if connection already exists
	if conn, exists := r.clients[name]; exists {
		// Check if connection is still valid
		state := conn.GetState()
		if state.String() == "READY" || state.String() == "IDLE" {
			return conn, nil
		}
		// Connection is in bad state, close it and create a new one
		_ = conn.Close()
		delete(r.clients, name)
	}

	// Create new connection
	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection to %s: %w", address, err)
	}

	// Wait for connection to be ready (with timeout from context)
	// grpc.NewClient with WithBlock() handles this automatically

	// Store connection
	r.clients[name] = conn

	return conn, nil
}

// Count returns the number of registered agents
func (r *GRPCAgentRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.addresses)
}

// IsRegistered checks if an agent is registered
func (r *GRPCAgentRegistry) IsRegistered(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.addresses[name]
	return exists
}

// convertTargetTypes converts proto target types to internal component.TargetType
func convertTargetTypes(protoTypes []string) []component.TargetType {
	result := make([]component.TargetType, len(protoTypes))
	for i, t := range protoTypes {
		result[i] = component.TargetType(t)
	}
	return result
}

// convertTechniqueTypes converts proto technique types to internal component.TechniqueType
func convertTechniqueTypes(protoTypes []string) []component.TechniqueType {
	result := make([]component.TechniqueType, len(protoTypes))
	for i, t := range protoTypes {
		result[i] = component.TechniqueType(t)
	}
	return result
}

// discoverAgents scans configured agent paths for component.yaml manifests.
// It populates the components, addresses, and descriptors maps with discovered agents.
// This method is called during registry initialization if agentPaths are configured.
func (r *GRPCAgentRegistry) discoverAgents() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, agentPath := range r.agentPaths {
		// Check if path exists
		info, err := os.Stat(agentPath)
		if err != nil {
			// Skip non-existent paths
			continue
		}

		// If it's a directory, scan for component.yaml
		if info.IsDir() {
			manifestPath := filepath.Join(agentPath, "component.yaml")
			if _, err := os.Stat(manifestPath); err != nil {
				// No component.yaml in this directory, skip
				continue
			}

			// Load manifest
			manifest, err := component.LoadManifest(manifestPath)
			if err != nil {
				// Skip invalid manifests
				continue
			}

			// Create component from manifest
			comp := &component.Component{
				Kind:     component.ComponentKindAgent,
				Name:     manifest.Name,
				Version:  manifest.Version,
				RepoPath: agentPath,
				BinPath:  filepath.Join(agentPath, manifest.Runtime.Entrypoint),
				Source:   component.ComponentSourceExternal,
				Status:   component.ComponentStatusAvailable,
				Manifest: manifest,
			}

			// Register component
			r.components[comp.Name] = comp

			// Register address (will be determined when agent starts)
			// For now, we'll use a placeholder and update it when the agent starts
			r.addresses[comp.Name] = ""
		}
	}

	return nil
}

// ensureRunning ensures an agent process is running.
// If the agent is not running and a lifecycle manager is configured,
// it starts the agent process and updates the address with the assigned port.
func (r *GRPCAgentRegistry) ensureRunning(ctx context.Context, name string) error {
	// If no lifecycle manager, assume agent is already running
	if r.lifecycleManager == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Get component
	comp, exists := r.components[name]
	if !exists {
		return fmt.Errorf("agent %s not found in discovered agents", name)
	}

	// Check if already running
	if comp.IsRunning() && comp.PID > 0 {
		// Verify process still exists via health check
		if err := r.healthCheckLocked(ctx, name); err == nil {
			// Process is healthy, no need to restart
			return nil
		}
		// Health check failed, process may have died - continue to restart
	}

	// Start the agent process
	port, err := r.lifecycleManager.StartComponent(ctx, comp)
	if err != nil {
		return fmt.Errorf("failed to start agent process: %w", err)
	}

	// Update address with the assigned port
	r.addresses[name] = fmt.Sprintf("localhost:%d", port)

	return nil
}

// healthCheck verifies that an agent is responding to health checks.
// This is called to verify agent health before delegating tasks.
func (r *GRPCAgentRegistry) healthCheck(ctx context.Context, name string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.healthCheckLocked(ctx, name)
}

// healthCheckLocked performs health check with lock already held.
// Caller must hold r.mu for reading.
func (r *GRPCAgentRegistry) healthCheckLocked(ctx context.Context, name string) error {
	// Get component
	comp, exists := r.components[name]
	if !exists {
		return fmt.Errorf("agent %s not found", name)
	}

	// If no lifecycle manager, skip health check
	if r.lifecycleManager == nil {
		return nil
	}

	// Get status from lifecycle manager
	status, err := r.lifecycleManager.GetStatus(ctx, comp)
	if err != nil {
		return fmt.Errorf("failed to get agent status: %w", err)
	}

	// Check if running
	if status != component.ComponentStatusRunning {
		return fmt.Errorf("agent is not running (status: %s)", status)
	}

	return nil
}
