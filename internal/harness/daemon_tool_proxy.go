// Package harness provides the agent harness implementation including tool proxying.
package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/daemon/api"
	"github.com/zero-day-ai/gibson/internal/daemon/toolexec"
	"github.com/zero-day-ai/gibson/internal/tool"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/sdk/schema"
)

// Ensure DaemonToolProxy implements tool.Tool at compile time
var _ tool.Tool = (*DaemonToolProxy)(nil)

// DaemonToolProxy implements the tool.Tool interface by proxying tool execution
// to the daemon's Tool Executor Service via gRPC.
//
// This proxy allows agents to execute tools without direct access to tool binaries.
// Tool execution is delegated to the daemon, which manages subprocess spawning,
// timeout enforcement, and metrics collection.
type DaemonToolProxy struct {
	// client is the gRPC client for the daemon service.
	client api.DaemonServiceClient

	// name is the tool's unique identifier.
	name string

	// description is a human-readable description of the tool.
	description string

	// version is the tool's version string.
	version string

	// tags are labels for categorizing the tool.
	tags []string

	// inputSchema defines the expected input structure.
	inputSchema schema.JSON

	// outputSchema defines the output structure.
	outputSchema schema.JSON

	// defaultTimeout is the default execution timeout if not specified in context.
	defaultTimeout time.Duration
}

// DaemonToolProxyConfig contains configuration for creating a DaemonToolProxy.
type DaemonToolProxyConfig struct {
	// Client is the gRPC client for the daemon service.
	Client api.DaemonServiceClient

	// Name is the tool's unique identifier.
	Name string

	// Description is a human-readable description of the tool.
	Description string

	// Version is the tool's version string.
	Version string

	// Tags are labels for categorizing the tool.
	Tags []string

	// InputSchema defines the expected input structure.
	InputSchema schema.JSON

	// OutputSchema defines the output structure.
	OutputSchema schema.JSON

	// DefaultTimeout is the default execution timeout (default: 5 minutes).
	DefaultTimeout time.Duration
}

// NewDaemonToolProxy creates a new DaemonToolProxy with the given configuration.
//
// Example:
//
//	proxy := NewDaemonToolProxy(DaemonToolProxyConfig{
//	    Client:      daemonClient,
//	    Name:        "ping",
//	    Description: "Network ping tool",
//	    Version:     "1.0.0",
//	    Tags:        []string{"network", "recon"},
//	    InputSchema: schema.NewObjectSchema(...),
//	    OutputSchema: schema.NewObjectSchema(...),
//	})
func NewDaemonToolProxy(cfg DaemonToolProxyConfig) *DaemonToolProxy {
	timeout := cfg.DefaultTimeout
	if timeout == 0 {
		timeout = 5 * time.Minute // Default timeout
	}

	return &DaemonToolProxy{
		client:         cfg.Client,
		name:           cfg.Name,
		description:    cfg.Description,
		version:        cfg.Version,
		tags:           cfg.Tags,
		inputSchema:    cfg.InputSchema,
		outputSchema:   cfg.OutputSchema,
		defaultTimeout: timeout,
	}
}

// Name returns the tool's unique identifier.
func (p *DaemonToolProxy) Name() string {
	return p.name
}

// Description returns a human-readable description of what this tool does.
func (p *DaemonToolProxy) Description() string {
	return p.description
}

// Version returns the semantic version of this tool.
func (p *DaemonToolProxy) Version() string {
	return p.version
}

// Tags returns a list of tags for categorization and discovery.
func (p *DaemonToolProxy) Tags() []string {
	return p.tags
}

// InputSchema returns the JSON schema defining valid input parameters.
func (p *DaemonToolProxy) InputSchema() schema.JSON {
	return p.inputSchema
}

// OutputSchema returns the JSON schema defining the output structure.
func (p *DaemonToolProxy) OutputSchema() schema.JSON {
	return p.outputSchema
}

// Tool execution error codes for DaemonToolProxy
const (
	ErrProxyInputSerialization  types.ErrorCode = "TOOL_PROXY_INPUT_SERIALIZATION"
	ErrProxyExecutionFailed     types.ErrorCode = "TOOL_PROXY_EXECUTION_FAILED"
	ErrProxyOutputDeserialization types.ErrorCode = "TOOL_PROXY_OUTPUT_DESERIALIZATION"
)

// Execute runs the tool by proxying to the daemon's Tool Executor Service.
//
// The input map is serialized to JSON and sent to the daemon, which spawns
// the tool as a subprocess. The tool's output is deserialized and returned.
//
// Timeout handling:
//   - If the context has a deadline, that deadline is used
//   - Otherwise, the defaultTimeout is used
//   - The daemon enforces the timeout and kills the subprocess if exceeded
func (p *DaemonToolProxy) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	// Serialize input to JSON
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, types.NewError(
			ErrProxyInputSerialization,
			fmt.Sprintf("failed to serialize tool input: %v", err),
		)
	}

	// Determine timeout
	var timeoutMs int64
	if deadline, ok := ctx.Deadline(); ok {
		// Use context deadline
		remaining := time.Until(deadline)
		if remaining > 0 {
			timeoutMs = remaining.Milliseconds()
		}
	} else {
		// Use default timeout
		timeoutMs = p.defaultTimeout.Milliseconds()
	}

	// Execute tool via daemon
	resp, err := p.client.ExecuteTool(ctx, &api.ExecuteToolRequest{
		Name:      p.name,
		InputJson: string(inputJSON),
		TimeoutMs: timeoutMs,
	})
	if err != nil {
		return nil, types.NewError(
			ErrProxyExecutionFailed,
			fmt.Sprintf("daemon tool execution failed: %v", err),
		)
	}

	// Check for execution error
	if !resp.Success {
		return nil, types.NewError(
			ErrProxyExecutionFailed,
			resp.Error,
		)
	}

	// Deserialize output
	var output map[string]any
	if err := json.Unmarshal([]byte(resp.OutputJson), &output); err != nil {
		return nil, types.NewError(
			ErrProxyOutputDeserialization,
			fmt.Sprintf("failed to deserialize tool output: %v", err),
		)
	}

	return output, nil
}

// Health returns the current health status of this tool.
//
// For daemon-proxied tools, health is determined by:
// 1. Whether the daemon is reachable
// 2. Whether the tool is registered in the Tool Executor Service
//
// Currently returns healthy if the proxy was successfully created.
// Future enhancement: query daemon for tool health status.
func (p *DaemonToolProxy) Health(ctx context.Context) types.HealthStatus {
	// For now, assume healthy if proxy exists
	// The actual health is managed by the daemon's Tool Executor Service
	return types.Healthy(fmt.Sprintf("Tool %s available via daemon", p.name))
}

// DaemonToolProxyFactory creates DaemonToolProxy instances from daemon tool info.
// This is used by the harness factory to populate the tool registry.
type DaemonToolProxyFactory struct {
	client api.DaemonServiceClient
}

// NewDaemonToolProxyFactory creates a new factory for creating tool proxies.
func NewDaemonToolProxyFactory(client api.DaemonServiceClient) *DaemonToolProxyFactory {
	return &DaemonToolProxyFactory{client: client}
}

// CreateFromAvailableToolInfo creates a DaemonToolProxy from daemon tool info.
func (f *DaemonToolProxyFactory) CreateFromAvailableToolInfo(info *api.AvailableToolInfo) (*DaemonToolProxy, error) {
	// Parse input schema
	// Prefer structured schema (with taxonomy support) over JSON string
	var inputSchema schema.JSON
	if info.InputSchema != nil {
		// Convert proto structured schema to SDK schema (with taxonomy preserved)
		inputSchema = api.ProtoToSchema(info.InputSchema)
	} else if info.InputSchemaJson != "" {
		// Fallback to JSON parsing (backward compatibility, but taxonomy is lost)
		if err := json.Unmarshal([]byte(info.InputSchemaJson), &inputSchema); err != nil {
			return nil, fmt.Errorf("failed to parse input schema: %w", err)
		}
	}

	// Parse output schema
	// Prefer structured schema (with taxonomy support) over JSON string
	var outputSchema schema.JSON
	if info.OutputSchema != nil {
		// Convert proto structured schema to SDK schema (with taxonomy preserved)
		outputSchema = api.ProtoToSchema(info.OutputSchema)
	} else if info.OutputSchemaJson != "" {
		// Fallback to JSON parsing (backward compatibility, but taxonomy is lost)
		if err := json.Unmarshal([]byte(info.OutputSchemaJson), &outputSchema); err != nil {
			return nil, fmt.Errorf("failed to parse output schema: %w", err)
		}
	}

	return NewDaemonToolProxy(DaemonToolProxyConfig{
		Client:       f.client,
		Name:         info.Name,
		Description:  info.Description,
		Version:      info.Version,
		Tags:         info.Tags,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}), nil
}

// FetchAndCreateProxies fetches all available tools from the daemon and creates proxies.
func (f *DaemonToolProxyFactory) FetchAndCreateProxies(ctx context.Context) ([]*DaemonToolProxy, error) {
	// Get available tools from daemon
	resp, err := f.client.GetAvailableTools(ctx, &api.GetAvailableToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get available tools from daemon: %w", err)
	}

	// Create proxies for each tool
	proxies := make([]*DaemonToolProxy, 0, len(resp.Tools))
	for _, toolInfo := range resp.Tools {
		// Skip tools with errors
		if toolInfo.Status == "error" {
			continue
		}

		proxy, err := f.CreateFromAvailableToolInfo(toolInfo)
		if err != nil {
			// Log warning but continue with other tools
			continue
		}
		proxies = append(proxies, proxy)
	}

	return proxies, nil
}

// PopulateToolRegistry fetches all available tools from the daemon and registers them
// with the provided tool registry. This is the main integration point for wiring
// the daemon's Tool Executor Service into the harness.
//
// Parameters:
//   - ctx: Context for the RPC call
//   - registry: The tool registry to populate with daemon tools
//
// Returns:
//   - int: Number of tools successfully registered
//   - error: Non-nil if fetching tools fails (individual registration errors are logged but not fatal)
func (f *DaemonToolProxyFactory) PopulateToolRegistry(ctx context.Context, registry tool.ToolRegistry) (int, error) {
	proxies, err := f.FetchAndCreateProxies(ctx)
	if err != nil {
		return 0, err
	}

	registered := 0
	for _, proxy := range proxies {
		if err := registry.RegisterInternal(proxy); err != nil {
			// Tool may already be registered, skip but continue
			continue
		}
		registered++
	}

	return registered, nil
}

// ────────────────────────────────────────────────────────────────────────────
// Direct ToolExecutorService Integration (no gRPC needed)
// ────────────────────────────────────────────────────────────────────────────

// DirectToolProxy implements tool.Tool by directly calling the ToolExecutorService.
// This is used when running inside the daemon to avoid gRPC round-trips.
type DirectToolProxy struct {
	service        toolexec.ToolExecutorService
	name           string
	description    string
	version        string
	tags           []string
	inputSchema    schema.JSON
	outputSchema   schema.JSON
	defaultTimeout time.Duration
}

// Ensure DirectToolProxy implements tool.Tool at compile time
var _ tool.Tool = (*DirectToolProxy)(nil)

// NewDirectToolProxy creates a tool proxy that directly uses the ToolExecutorService.
func NewDirectToolProxy(
	service toolexec.ToolExecutorService,
	descriptor toolexec.ToolDescriptor,
	toolSchema *toolexec.ToolSchema,
	defaultTimeout time.Duration,
) *DirectToolProxy {
	if defaultTimeout == 0 {
		defaultTimeout = 5 * time.Minute
	}

	var inputSchema, outputSchema schema.JSON
	if toolSchema != nil {
		inputSchema = toolSchema.InputSchema
		outputSchema = toolSchema.OutputSchema
	}

	return &DirectToolProxy{
		service:        service,
		name:           descriptor.Name,
		description:    descriptor.Description,
		version:        descriptor.Version,
		tags:           descriptor.Tags,
		inputSchema:    inputSchema,
		outputSchema:   outputSchema,
		defaultTimeout: defaultTimeout,
	}
}

func (p *DirectToolProxy) Name() string                   { return p.name }
func (p *DirectToolProxy) Description() string            { return p.description }
func (p *DirectToolProxy) Version() string                { return p.version }
func (p *DirectToolProxy) Tags() []string                 { return p.tags }
func (p *DirectToolProxy) InputSchema() schema.JSON { return p.inputSchema }
func (p *DirectToolProxy) OutputSchema() schema.JSON { return p.outputSchema }

func (p *DirectToolProxy) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	// Use default timeout - don't derive from context deadline or tool input
	// The context deadline may be for the entire operation (e.g., module timeout)
	// which shouldn't limit individual tool execution.
	// Tool-specific timeout parameters (like ping's "timeout") are handled by the tool
	// internally and should not affect the execution timeout.
	timeout := p.defaultTimeout

	// Execute directly via service
	output, err := p.service.Execute(ctx, p.name, input, timeout)
	if err != nil {
		return nil, types.NewError(
			ErrProxyExecutionFailed,
			fmt.Sprintf("tool execution failed: %v", err),
		)
	}

	return output, nil
}

func (p *DirectToolProxy) Health(ctx context.Context) types.HealthStatus {
	return types.Healthy(fmt.Sprintf("Tool %s available via daemon", p.name))
}

// PopulateToolRegistryFromService populates a tool registry directly from a ToolExecutorService.
// This is the preferred method when running inside the daemon as it avoids gRPC overhead.
func PopulateToolRegistryFromService(
	service toolexec.ToolExecutorService,
	registry tool.ToolRegistry,
) (int, error) {
	descriptors := service.ListTools()

	registered := 0
	for _, desc := range descriptors {
		// Skip tools that aren't ready
		if desc.Status != "ready" {
			continue
		}

		// Get schema for the tool
		toolSchema, err := service.GetToolSchema(desc.Name)
		if err != nil {
			// Skip tools without valid schemas
			continue
		}

		proxy := NewDirectToolProxy(service, desc, toolSchema, 5*time.Minute)
		if err := registry.RegisterInternal(proxy); err != nil {
			// Tool may already be registered, skip but continue
			continue
		}
		registered++
	}

	return registered, nil
}
