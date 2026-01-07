package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/schema"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/sdk/api/gen/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GRPCToolClient wraps a gRPC connection to implement the Tool interface.
// It communicates with external gRPC tools using the ToolService proto definition.
//
// The client handles:
// - gRPC connection management
// - Protocol buffer marshaling/unmarshaling
// - Schema conversion between JSON Schema and proto definitions
// - Health check integration with gRPC health protocol
type GRPCToolClient struct {
	name         string
	description  string
	version      string
	tags         []string
	conn         *grpc.ClientConn
	client       proto.ToolServiceClient
	inputSchema  schema.JSONSchema
	outputSchema schema.JSONSchema
}

// NewGRPCToolClient creates a new GRPCToolClient by connecting to a gRPC tool service.
// It dials the endpoint, creates the client, and fetches the tool descriptor to populate
// metadata and schemas.
//
// Parameters:
//   - endpoint: gRPC server address (e.g., "localhost:50051")
//   - opts: Optional gRPC dial options (uses insecure credentials if none provided)
//
// Returns error if connection fails or GetDescriptor RPC fails.
func NewGRPCToolClient(endpoint string, opts ...grpc.DialOption) (*GRPCToolClient, error) {
	// Use insecure credentials if no options provided
	if len(opts) == 0 {
		opts = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	}

	// Dial the gRPC server
	// Note: Using deprecated Dial for better compatibility with testing (bufconn)
	//nolint:staticcheck // Dial provides better blocking behavior for connection establishment
	conn, err := grpc.Dial(endpoint, opts...)
	if err != nil {
		return nil, types.WrapError(ErrToolExecutionFailed, fmt.Sprintf("failed to dial gRPC endpoint %q", endpoint), err)
	}

	// Create the ToolService client
	client := proto.NewToolServiceClient(conn)

	// Fetch the tool descriptor
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	descriptor, err := client.GetDescriptor(ctx, &proto.ToolGetDescriptorRequest{})
	if err != nil {
		conn.Close()
		return nil, types.WrapError(ErrToolExecutionFailed, "failed to get tool descriptor", err)
	}

	// Convert proto schemas to internal schema types
	inputSchema, err := protoSchemaToInternal(descriptor.GetInputSchema())
	if err != nil {
		conn.Close()
		return nil, types.WrapError(ErrToolExecutionFailed, "failed to parse input schema", err)
	}

	outputSchema, err := protoSchemaToInternal(descriptor.GetOutputSchema())
	if err != nil {
		conn.Close()
		return nil, types.WrapError(ErrToolExecutionFailed, "failed to parse output schema", err)
	}

	return &GRPCToolClient{
		name:         descriptor.GetName(),
		description:  descriptor.GetDescription(),
		version:      descriptor.GetVersion(),
		tags:         descriptor.GetTags(),
		conn:         conn,
		client:       client,
		inputSchema:  inputSchema,
		outputSchema: outputSchema,
	}, nil
}

// protoSchemaToInternal converts a proto JSONSchema to internal schema.JSONSchema.
// The proto type contains a serialized JSON string that we unmarshal.
func protoSchemaToInternal(protoSchema *proto.JSONSchema) (schema.JSONSchema, error) {
	if protoSchema == nil {
		return schema.NewObjectSchema(nil, nil), nil
	}

	var result schema.JSONSchema
	if err := json.Unmarshal([]byte(protoSchema.GetJson()), &result); err != nil {
		return schema.JSONSchema{}, fmt.Errorf("failed to unmarshal JSON schema: %w", err)
	}

	return result, nil
}

// Name returns the unique identifier for this tool
func (c *GRPCToolClient) Name() string {
	return c.name
}

// Description returns a human-readable description of what this tool does
func (c *GRPCToolClient) Description() string {
	return c.description
}

// Version returns the semantic version of this tool
func (c *GRPCToolClient) Version() string {
	return c.version
}

// Tags returns a list of tags for categorization and discovery
func (c *GRPCToolClient) Tags() []string {
	return c.tags
}

// InputSchema returns the JSON schema defining valid input parameters.
func (c *GRPCToolClient) InputSchema() schema.JSONSchema {
	return c.inputSchema
}

// OutputSchema returns the JSON schema defining the output structure.
func (c *GRPCToolClient) OutputSchema() schema.JSONSchema {
	return c.outputSchema
}

// Execute runs the tool via gRPC with the given input.
// It marshals the input to JSON, makes the gRPC call, and unmarshals the output.
func (c *GRPCToolClient) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	// Marshal input to JSON
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, types.WrapError(ErrToolInvalidInput, "failed to marshal input to JSON", err)
	}

	// Create the gRPC request
	req := &proto.ToolExecuteRequest{
		InputJson: string(inputJSON),
	}

	// Make the gRPC call
	resp, err := c.client.Execute(ctx, req)
	if err != nil {
		return nil, types.WrapError(ErrToolExecutionFailed, fmt.Sprintf("gRPC tool %q execution failed", c.name), err)
	}

	// Check for proto-level error in response
	if protoErr := resp.GetError(); protoErr != nil {
		gibsonErr := &types.GibsonError{
			Code:      types.ErrorCode(protoErr.GetCode()),
			Message:   protoErr.GetMessage(),
			Retryable: protoErr.GetRetryable(),
		}
		return nil, gibsonErr
	}

	// Unmarshal output JSON to map
	var output map[string]any
	if err := json.Unmarshal([]byte(resp.GetOutputJson()), &output); err != nil {
		return nil, types.WrapError(ErrToolInvalidOutput, "failed to unmarshal output JSON", err)
	}

	return output, nil
}

// Health returns the current health status of this tool by calling the gRPC Health endpoint.
func (c *GRPCToolClient) Health(ctx context.Context) types.HealthStatus {
	// Make health check call with timeout
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.client.Health(healthCtx, &proto.ToolHealthRequest{})
	if err != nil {
		return types.Unhealthy(fmt.Sprintf("gRPC health check failed: %v", err))
	}

	// Convert proto HealthStatus to internal type
	return types.HealthStatus{
		State:     types.HealthState(resp.GetState()),
		Message:   resp.GetMessage(),
		CheckedAt: time.UnixMilli(resp.GetCheckedAt()),
	}
}

// Close closes the underlying gRPC connection.
func (c *GRPCToolClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
