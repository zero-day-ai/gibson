package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/grpc"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/types"
	proto "github.com/zero-day-ai/sdk/api/gen/proto"
	"github.com/zero-day-ai/sdk/registry"
)

// GRPCAgentClient implements agent.Agent interface for agents discovered via etcd registry.
//
// This client wraps a gRPC connection to a remote agent and translates between
// Gibson's internal agent.Agent interface and the gRPC protocol. It uses ServiceInfo
// from the etcd registry to populate agent metadata (name, version, capabilities, etc.).
//
// Key features:
// - Implements full agent.Agent interface for remote gRPC agents
// - Parses capabilities, target types, and technique types from ServiceInfo metadata
// - Delegates execution to remote agent via gRPC
// - Handles descriptor caching and health checks
type GRPCAgentClient struct {
	conn   *grpc.ClientConn
	client proto.AgentServiceClient
	info   registry.ServiceInfo

	// Cached descriptor from GetDescriptor RPC
	// This avoids repeated gRPC calls for static metadata
	descriptor *agent.AgentDescriptor
}

// NewGRPCAgentClient creates a new GRPCAgentClient wrapping an existing gRPC connection.
//
// The connection should already be established and ready to use. The ServiceInfo
// provides metadata about the agent (name, version, endpoint, capabilities, etc.)
// that was discovered from the etcd registry.
//
// Parameters:
//   - conn: Established gRPC connection to the agent
//   - info: ServiceInfo from etcd registry with agent metadata
//
// Returns a GRPCAgentClient that implements the agent.Agent interface.
func NewGRPCAgentClient(conn *grpc.ClientConn, info registry.ServiceInfo) *GRPCAgentClient {
	return &GRPCAgentClient{
		conn:   conn,
		client: proto.NewAgentServiceClient(conn),
		info:   info,
	}
}

// Name returns the agent name from ServiceInfo
func (c *GRPCAgentClient) Name() string {
	return c.info.Name
}

// Version returns the agent version from ServiceInfo
func (c *GRPCAgentClient) Version() string {
	return c.info.Version
}

// Description returns the agent description.
//
// If available, this is retrieved from the cached descriptor (which comes from
// the agent's GetDescriptor RPC). Otherwise, falls back to metadata or empty string.
func (c *GRPCAgentClient) Description() string {
	if c.descriptor != nil {
		return c.descriptor.Description
	}

	// Try to get from metadata
	if desc, ok := c.info.Metadata["description"]; ok {
		return desc
	}

	return ""
}

// Capabilities returns the agent's capabilities from ServiceInfo metadata.
//
// The metadata should contain a "capabilities" key with comma-separated values.
// For example: "prompt_injection,jailbreak,data_extraction"
func (c *GRPCAgentClient) Capabilities() []string {
	if c.descriptor != nil {
		return c.descriptor.Capabilities
	}

	return parseCommaSeparated(c.info.Metadata["capabilities"])
}

// TargetTypes returns the types of targets this agent can operate against.
//
// The metadata should contain a "target_types" key with comma-separated values.
// For example: "llm_chat,llm_api,rag_system"
func (c *GRPCAgentClient) TargetTypes() []component.TargetType {
	if c.descriptor != nil {
		return c.descriptor.TargetTypes
	}

	targetStrs := parseCommaSeparated(c.info.Metadata["target_types"])
	result := make([]component.TargetType, len(targetStrs))
	for i, t := range targetStrs {
		result[i] = component.TargetType(t)
	}
	return result
}

// TechniqueTypes returns the types of techniques this agent can execute.
//
// The metadata should contain a "technique_types" key with comma-separated values.
// For example: "prompt_injection,model_extraction,jailbreak"
func (c *GRPCAgentClient) TechniqueTypes() []component.TechniqueType {
	if c.descriptor != nil {
		return c.descriptor.TechniqueTypes
	}

	techniqueStrs := parseCommaSeparated(c.info.Metadata["technique_types"])
	result := make([]component.TechniqueType, len(techniqueStrs))
	for i, t := range techniqueStrs {
		result[i] = component.TechniqueType(t)
	}
	return result
}

// LLMSlots returns the LLM slot definitions required by this agent.
//
// This calls the GetSlotSchema RPC to fetch detailed slot information.
// The result is cached in the descriptor to avoid repeated RPC calls.
func (c *GRPCAgentClient) LLMSlots() []agent.SlotDefinition {
	// Ensure descriptor is loaded
	if c.descriptor == nil {
		ctx := context.Background()
		_, _ = c.fetchDescriptor(ctx)
	}

	// Try to fetch slots if not already loaded
	if c.descriptor != nil && c.descriptor.Slots == nil {
		ctx := context.Background()
		_ = c.fetchSlots(ctx)
	}

	if c.descriptor != nil && c.descriptor.Slots != nil {
		return c.descriptor.Slots
	}

	return []agent.SlotDefinition{}
}

// Initialize prepares the agent for execution with the given configuration.
//
// For gRPC agents, this is typically a no-op since the remote agent manages
// its own initialization. This method is provided for interface compatibility.
func (c *GRPCAgentClient) Initialize(ctx context.Context, cfg agent.AgentConfig) error {
	// gRPC agents manage their own initialization
	// This could be extended to send an Initialize RPC if needed
	return nil
}

// Execute runs the agent against a task using the provided harness.
//
// This method:
//  1. Marshals the task to JSON
//  2. Sends an Execute RPC to the remote agent
//  3. Receives the result via gRPC
//  4. Unmarshals and returns the result
//
// Note: The harness parameter is currently not used for gRPC agents.
// Future implementations may use bidirectional streaming to support
// harness callbacks (tool execution, plugin queries, sub-agent delegation).
func (c *GRPCAgentClient) Execute(ctx context.Context, task agent.Task, harness agent.AgentHarness) (agent.Result, error) {
	// Marshal task to JSON
	taskJSON, err := json.Marshal(task)
	if err != nil {
		result := agent.NewResult(task.ID)
		result.Fail(fmt.Errorf("failed to marshal task: %w", err))
		return result, nil
	}

	// Send Execute RPC
	timeoutMs := int64(task.Timeout.Milliseconds())
	req := &proto.AgentExecuteRequest{
		TaskJson:  string(taskJSON),
		TimeoutMs: timeoutMs,
	}

	resp, err := c.client.Execute(ctx, req)
	if err != nil {
		result := agent.NewResult(task.ID)
		result.Fail(fmt.Errorf("agent execution failed: %w", err))
		return result, nil
	}

	// Check for errors in response
	if resp.Error != nil {
		result := agent.NewResult(task.ID)
		result.Fail(fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message))
		return result, nil
	}

	// Unmarshal result from JSON
	var result agent.Result
	if err := json.Unmarshal([]byte(resp.ResultJson), &result); err != nil {
		return agent.Result{}, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return result, nil
}

// Shutdown cleanly terminates the agent and releases resources.
//
// This closes the underlying gRPC connection. After shutdown, the client
// should not be used for further operations.
func (c *GRPCAgentClient) Shutdown(ctx context.Context) error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Health returns the current health status of the agent.
//
// This sends a Health RPC to the remote agent to check its status.
// If the RPC fails, the agent is considered unhealthy.
func (c *GRPCAgentClient) Health(ctx context.Context) types.HealthStatus {
	req := &proto.AgentHealthRequest{}

	resp, err := c.client.Health(ctx, req)
	if err != nil {
		return types.Unhealthy(fmt.Sprintf("health check failed: %v", err))
	}

	// Convert proto health status to internal type
	// The proto HealthStatus has a "state" field with values: "healthy", "degraded", "unhealthy"
	switch resp.State {
	case "healthy":
		return types.Healthy(resp.Message)
	case "degraded":
		return types.Degraded(resp.Message)
	case "unhealthy":
		return types.Unhealthy(resp.Message)
	default:
		return types.Unhealthy("unknown health status")
	}
}

// fetchDescriptor retrieves the agent descriptor from the remote agent via gRPC.
//
// This is called lazily when descriptor information is needed. The result is
// cached in c.descriptor to avoid repeated RPC calls.
// Note: Slots are fetched separately via fetchSlots()
func (c *GRPCAgentClient) fetchDescriptor(ctx context.Context) (*agent.AgentDescriptor, error) {
	if c.descriptor != nil {
		return c.descriptor, nil
	}

	req := &proto.AgentGetDescriptorRequest{}
	resp, err := c.client.GetDescriptor(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get descriptor: %w", err)
	}

	// Convert proto descriptor to internal type
	// Note: Slots field is nil initially and populated by fetchSlots()
	desc := &agent.AgentDescriptor{
		Name:           resp.Name,
		Version:        resp.Version,
		Description:    resp.Description,
		Capabilities:   resp.Capabilities,
		TargetTypes:    convertTargetTypes(resp.TargetTypes),
		TechniqueTypes: convertTechniqueTypes(resp.TechniqueTypes),
		Slots:          nil, // Populated by fetchSlots()
		IsExternal:     true,
	}

	c.descriptor = desc
	return desc, nil
}

// fetchSlots retrieves the agent's slot definitions from the remote agent via gRPC.
//
// This is called lazily when slot information is needed. The result is
// cached in c.descriptor.Slots to avoid repeated RPC calls.
func (c *GRPCAgentClient) fetchSlots(ctx context.Context) error {
	if c.descriptor == nil {
		return fmt.Errorf("descriptor not loaded")
	}

	if c.descriptor.Slots != nil {
		return nil // Already fetched
	}

	req := &proto.AgentGetSlotSchemaRequest{}
	resp, err := c.client.GetSlotSchema(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get slot schema: %w", err)
	}

	// Convert proto slots to internal type
	c.descriptor.Slots = convertSlots(resp.Slots)
	return nil
}

// parseCommaSeparated parses a comma-separated string into a slice of trimmed strings.
//
// Empty strings and whitespace-only entries are filtered out.
// For example: "a, b, c" -> ["a", "b", "c"]
func parseCommaSeparated(value string) []string {
	if value == "" {
		return []string{}
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
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

// convertSlots converts proto slot definitions to internal agent.SlotDefinition
func convertSlots(protoSlots []*proto.AgentSlotDefinition) []agent.SlotDefinition {
	if protoSlots == nil {
		return []agent.SlotDefinition{}
	}

	result := make([]agent.SlotDefinition, len(protoSlots))
	for i, ps := range protoSlots {
		result[i] = agent.SlotDefinition{
			Name:        ps.Name,
			Description: ps.Description,
			Required:    ps.Required,
			Default: agent.SlotConfig{
				Provider:    ps.DefaultConfig.Provider,
				Model:       ps.DefaultConfig.Model,
				Temperature: ps.DefaultConfig.Temperature,
				MaxTokens:   int(ps.DefaultConfig.MaxTokens),
			},
			Constraints: agent.SlotConstraints{
				MinContextWindow: int(ps.Constraints.MinContextWindow),
				RequiredFeatures: ps.Constraints.RequiredFeatures,
			},
		}
	}
	return result
}
