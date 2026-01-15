package registry

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/grpc"

	"github.com/zero-day-ai/gibson/internal/types"
	proto "github.com/zero-day-ai/sdk/api/gen/proto"
	"github.com/zero-day-ai/sdk/registry"
	"github.com/zero-day-ai/sdk/schema"
)

// GRPCToolClient implements tool.Tool interface for tools discovered via etcd registry.
//
// This client wraps a gRPC connection to a remote tool and translates between
// Gibson's internal tool.Tool interface and the gRPC protocol. It uses ServiceInfo
// from the etcd registry to populate tool metadata (name, version, tags, etc.).
//
// Key features:
// - Implements full tool.Tool interface for remote gRPC tools
// - Parses tags from ServiceInfo metadata
// - Delegates execution to remote tool via gRPC
// - Handles descriptor caching and health checks
// - Marshals/unmarshals JSON for Execute RPC
type GRPCToolClient struct {
	conn   *grpc.ClientConn
	client proto.ToolServiceClient
	info   registry.ServiceInfo

	// Cached descriptor from GetDescriptor RPC
	// This avoids repeated gRPC calls for static metadata
	descriptor *toolDescriptor
}

// toolDescriptor holds cached metadata from the tool's GetDescriptor RPC.
// This struct is internal and used only for caching purposes.
type toolDescriptor struct {
	Name         string
	Description  string
	Version      string
	Tags         []string
	InputSchema  schema.JSON
	OutputSchema schema.JSON
}

// NewGRPCToolClient creates a new GRPCToolClient wrapping an existing gRPC connection.
//
// The connection should already be established and ready to use. The ServiceInfo
// provides metadata about the tool (name, version, endpoint, tags, etc.)
// that was discovered from the etcd registry.
//
// Parameters:
//   - conn: Established gRPC connection to the tool
//   - info: ServiceInfo from etcd registry with tool metadata
//
// Returns a GRPCToolClient that implements the tool.Tool interface.
func NewGRPCToolClient(conn *grpc.ClientConn, info registry.ServiceInfo) *GRPCToolClient {
	return &GRPCToolClient{
		conn:   conn,
		client: proto.NewToolServiceClient(conn),
		info:   info,
	}
}

// Name returns the tool name from ServiceInfo
func (c *GRPCToolClient) Name() string {
	return c.info.Name
}

// Description returns the tool description.
//
// If available, this is retrieved from the cached descriptor (which comes from
// the tool's GetDescriptor RPC). Otherwise, falls back to metadata or empty string.
func (c *GRPCToolClient) Description() string {
	if c.descriptor != nil {
		return c.descriptor.Description
	}

	// Try to get from metadata
	if desc, ok := c.info.Metadata["description"]; ok {
		return desc
	}

	return ""
}

// Version returns the tool version from ServiceInfo
func (c *GRPCToolClient) Version() string {
	return c.info.Version
}

// Tags returns the tool's tags from ServiceInfo metadata.
//
// The metadata should contain a "tags" key with comma-separated values.
// For example: "network,scanner,recon"
func (c *GRPCToolClient) Tags() []string {
	if c.descriptor != nil {
		return c.descriptor.Tags
	}

	return parseCommaSeparated(c.info.Metadata["tags"])
}

// InputSchema returns the JSON schema defining valid input parameters.
//
// This calls fetchDescriptor() to retrieve the schema from the remote tool
// if not already cached. The schema is used for input validation.
func (c *GRPCToolClient) InputSchema() schema.JSON {
	// Ensure descriptor is loaded
	if c.descriptor == nil {
		ctx := context.Background()
		_, _ = c.fetchDescriptor(ctx)
	}

	if c.descriptor != nil {
		return c.descriptor.InputSchema
	}

	// Return empty schema if fetch failed
	return schema.JSON{}
}

// OutputSchema returns the JSON schema defining the output structure.
//
// This calls fetchDescriptor() to retrieve the schema from the remote tool
// if not already cached. The schema documents the expected output format.
func (c *GRPCToolClient) OutputSchema() schema.JSON {
	// Ensure descriptor is loaded
	if c.descriptor == nil {
		ctx := context.Background()
		_, _ = c.fetchDescriptor(ctx)
	}

	if c.descriptor != nil {
		return c.descriptor.OutputSchema
	}

	// Return empty schema if fetch failed
	return schema.JSON{}
}

// Execute runs the tool with the given input and returns the result.
//
// This method:
//  1. Marshals the input to JSON
//  2. Sends an Execute RPC to the remote tool
//  3. Receives the result via gRPC
//  4. Unmarshals and returns the result
//
// The input and output maps must conform to the InputSchema and OutputSchema respectively.
// Returns an error if execution fails or input validation fails.
func (c *GRPCToolClient) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	// Marshal input to JSON
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}

	// Send Execute RPC
	req := &proto.ToolExecuteRequest{
		InputJson: string(inputJSON),
		TimeoutMs: 0, // Use tool's default timeout
	}

	resp, err := c.client.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("tool execution failed: %w", err)
	}

	// Check for errors in response
	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	// Unmarshal output from JSON
	var output map[string]any
	if err := json.Unmarshal([]byte(resp.OutputJson), &output); err != nil {
		return nil, fmt.Errorf("failed to unmarshal output: %w", err)
	}

	return output, nil
}

// Health returns the current health status of the tool.
//
// This sends a Health RPC to the remote tool to check its status.
// If the RPC fails, the tool is considered unhealthy.
func (c *GRPCToolClient) Health(ctx context.Context) types.HealthStatus {
	req := &proto.ToolHealthRequest{}

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

// fetchDescriptor retrieves the tool descriptor from the remote tool via gRPC.
//
// This is called lazily when descriptor information is needed. The result is
// cached in c.descriptor to avoid repeated RPC calls.
func (c *GRPCToolClient) fetchDescriptor(ctx context.Context) (*toolDescriptor, error) {
	if c.descriptor != nil {
		return c.descriptor, nil
	}

	req := &proto.ToolGetDescriptorRequest{}
	resp, err := c.client.GetDescriptor(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get descriptor: %w", err)
	}

	// Convert proto schemas to internal type
	inputSchema, err := protoSchemaToInternal(resp.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to convert input schema: %w", err)
	}

	outputSchema, err := protoSchemaToInternal(resp.OutputSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to convert output schema: %w", err)
	}

	// Build descriptor
	desc := &toolDescriptor{
		Name:         resp.Name,
		Description:  resp.Description,
		Version:      resp.Version,
		Tags:         resp.Tags,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}

	c.descriptor = desc
	return desc, nil
}

// protoSchemaToInternal converts a proto JSONSchema to SDK schema.JSON.
//
// The proto JSONSchema contains a serialized JSON string that needs to be
// unmarshaled into the SDK schema.JSON struct.
func protoSchemaToInternal(protoSchema *proto.JSONSchema) (schema.JSON, error) {
	if protoSchema == nil || protoSchema.Json == "" {
		return schema.JSON{}, nil
	}

	var internalSchema schema.JSON
	if err := json.Unmarshal([]byte(protoSchema.Json), &internalSchema); err != nil {
		return schema.JSON{}, fmt.Errorf("failed to unmarshal schema: %w", err)
	}

	return internalSchema, nil
}
