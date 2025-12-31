package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	proto "github.com/zero-day-ai/sdk/api/gen/proto"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// GRPCAgentClient implements ExternalAgentClient for gRPC-based agents.
// This is a placeholder for Stage 7 (gRPC integration).
//
// External agents allow extending Gibson with agents written in any language
// that implements the Gibson agent gRPC protocol. This enables:
// - Language-specific security tools (e.g., Python-based ML models)
// - Legacy tool integration
// - Third-party agent development
// - Distributed agent execution
type GRPCAgentClient struct {
	name        string
	version     string
	description string
	conn        *grpc.ClientConn
	client      proto.AgentServiceClient
}

// NewGRPCAgentClient creates a new gRPC agent client
// Full implementation in Stage 7
func NewGRPCAgentClient(address string) (*GRPCAgentClient, error) {
	// Establish gRPC connection
	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	client := proto.NewAgentServiceClient(conn)

	return &GRPCAgentClient{
		name:        "grpc-agent",
		version:     "0.1.0",
		description: "External gRPC agent",
		conn:        conn,
		client:      client,
	}, nil
}

// Name returns the unique identifier for this agent
func (c *GRPCAgentClient) Name() string {
	return c.name
}

// Version returns the semantic version of this agent
func (c *GRPCAgentClient) Version() string {
	return c.version
}

// Description returns a human-readable description
func (c *GRPCAgentClient) Description() string {
	return c.description
}

// Capabilities returns the list of capabilities
func (c *GRPCAgentClient) Capabilities() []string {
	// Will be populated from gRPC metadata in Stage 7
	return []string{}
}

// TargetTypes returns the types of targets this agent supports
func (c *GRPCAgentClient) TargetTypes() []component.TargetType {
	// Will be populated from gRPC metadata in Stage 7
	return []component.TargetType{}
}

// TechniqueTypes returns the types of techniques this agent supports
func (c *GRPCAgentClient) TechniqueTypes() []component.TechniqueType {
	// Will be populated from gRPC metadata in Stage 7
	return []component.TechniqueType{}
}

// LLMSlots returns the LLM slot requirements
func (c *GRPCAgentClient) LLMSlots() []SlotDefinition {
	// Will be populated from gRPC metadata in Stage 7
	return []SlotDefinition{}
}

// Execute runs the agent via gRPC
func (c *GRPCAgentClient) Execute(ctx context.Context, task Task, harness AgentHarness) (Result, error) {
	// Full implementation in Stage 7
	// This will:
	// 1. Serialize task to protobuf
	// 2. Send gRPC ExecuteTask request
	// 3. Stream responses
	// 4. Handle harness callbacks via bidirectional streaming
	// 5. Return final result

	result := NewResult(task.ID)
	result.Fail(fmt.Errorf("gRPC agent execution not yet implemented (Stage 7)"))
	return result, nil
}

// Initialize initializes the gRPC agent
func (c *GRPCAgentClient) Initialize(ctx context.Context, cfg AgentConfig) error {
	// Full implementation in Stage 7
	// This will send an Initialize gRPC request
	return nil
}

// Shutdown cleanly shuts down the gRPC connection
func (c *GRPCAgentClient) Shutdown(ctx context.Context) error {
	// Full implementation in Stage 7
	// This will close the gRPC connection
	return nil
}

// Health checks the health of the gRPC agent
func (c *GRPCAgentClient) Health(ctx context.Context) types.HealthStatus {
	// Full implementation in Stage 7
	// This will send a Health gRPC request
	return types.Unhealthy("gRPC health check not yet implemented (Stage 7)")
}

// Close closes the gRPC connection
func (c *GRPCAgentClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// StreamExecute starts a bidirectional streaming execution.
// Returns a StreamClient for sending steering messages and receiving events.
// Falls back to regular Execute if the agent doesn't support streaming.
func (c *GRPCAgentClient) StreamExecute(ctx context.Context, task Task, sessionID types.ID) (*StreamClient, error) {
	// Check if we have a valid connection
	if c.conn == nil {
		return nil, fmt.Errorf("gRPC connection not established")
	}

	// Create the StreamClient, which internally establishes the bidirectional stream
	streamClient := NewStreamClient(ctx, c.conn, c.name, sessionID)

	// Marshal the task to JSON
	taskJSON, err := json.Marshal(task)
	if err != nil {
		streamClient.Close()
		return nil, fmt.Errorf("failed to marshal task: %w", err)
	}

	// Send the initial task with autonomous mode as default
	if err := streamClient.Start(string(taskJSON), database.AgentModeAutonomous); err != nil {
		streamClient.Close()
		return nil, fmt.Errorf("failed to start execution: %w", err)
	}

	return streamClient, nil
}

// SupportsStreaming checks if the agent supports bidirectional streaming.
// This can be called before StreamExecute to determine if fallback is needed.
func (c *GRPCAgentClient) SupportsStreaming() bool {
	// For now, assume all gRPC agents support streaming
	// In the future, this could check agent capabilities via GetDescriptor
	return c.conn != nil
}
