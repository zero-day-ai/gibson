// Package client provides a client library for connecting to the Gibson daemon.
//
// The client package implements the client-side of the daemon-client architecture,
// allowing CLI commands to connect to the running daemon and invoke operations via gRPC.
// It handles connection management, streaming RPCs, and provides high-level convenience
// methods for common daemon operations.
package client

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/zero-day-ai/gibson/internal/daemon"
	"github.com/zero-day-ai/gibson/internal/daemon/api"
)

// Client represents a connection to the Gibson daemon.
//
// The client wraps a gRPC connection and provides high-level methods for
// interacting with the daemon. It supports both Unix socket and TCP connections,
// automatically handling the appropriate connection setup based on address format.
//
// Example usage:
//
//	// Connect directly to an address
//	client, err := Connect(ctx, "unix:///home/user/.gibson/daemon.sock")
//	if err != nil {
//	    return err
//	}
//	defer client.Close()
//
//	// Or connect using daemon info file
//	client, err := ConnectFromInfo(ctx, "/home/user/.gibson/daemon.json")
//	if err != nil {
//	    return err
//	}
//	defer client.Close()
//
//	// Use the client
//	status, err := client.Status(ctx)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Daemon running: %v\n", status.Running)
type Client struct {
	// conn is the underlying gRPC connection
	conn *grpc.ClientConn

	// daemon is the gRPC service client for daemon operations
	daemon api.DaemonServiceClient
}

// Connect establishes a connection to the Gibson daemon at the specified address.
//
// The address can be either:
//   - Unix socket: "unix:///path/to/socket" (recommended for local connections)
//   - TCP: "localhost:50002" or "127.0.0.1:50002"
//
// Unix socket connections are preferred for security and performance when connecting
// to a local daemon. TCP connections are useful for remote daemon connections or
// container environments where Unix sockets are not available.
//
// Parameters:
//   - ctx: Context with timeout for connection establishment
//   - address: Daemon address (unix:// or TCP host:port)
//
// Returns:
//   - *Client: Connected client instance
//   - error: Non-nil if connection fails
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//
//	client, err := Connect(ctx, "unix:///home/user/.gibson/daemon.sock")
//	if err != nil {
//	    return fmt.Errorf("failed to connect to daemon: %w", err)
//	}
//	defer client.Close()
func Connect(ctx context.Context, address string) (*Client, error) {
	if address == "" {
		return nil, fmt.Errorf("daemon address cannot be empty")
	}

	// Add default timeout if context has none
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	// Determine connection type and format address correctly
	var target string
	if strings.HasPrefix(address, "unix://") {
		// Unix socket - use the path directly
		target = address
	} else if strings.HasPrefix(address, "/") {
		// Unix socket path without scheme
		target = "unix://" + address
	} else {
		// TCP address (host:port)
		target = address
	}

	// Establish gRPC connection
	// Note: grpc.NewClient doesn't actually connect until the first RPC.
	// We use DialContext for immediate connection validation.
	conn, err := grpc.DialContext(
		ctx,
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), // Block until connection is ready or context timeout
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon at %s: %w", address, err)
	}

	// Create daemon service client
	daemonClient := api.NewDaemonServiceClient(conn)

	return &Client{
		conn:   conn,
		daemon: daemonClient,
	}, nil
}

// ConnectFromInfo reads daemon connection information from a JSON file and connects.
//
// This function reads the daemon.json file (created by WriteDaemonInfo) to discover
// the daemon's gRPC address, then establishes a connection using that address.
// This is the recommended way for CLI commands to connect to the daemon, as it
// automatically handles address discovery.
//
// Parameters:
//   - ctx: Context with timeout for connection
//   - infoPath: Path to daemon.json file (typically ~/.gibson/daemon.json)
//
// Returns:
//   - *Client: Connected client instance
//   - error: Non-nil if file read or connection fails
//
// Example:
//
//	ctx := context.Background()
//	client, err := ConnectFromInfo(ctx, "/home/user/.gibson/daemon.json")
//	if err != nil {
//	    if os.IsNotExist(err) {
//	        return fmt.Errorf("daemon not running")
//	    }
//	    return err
//	}
//	defer client.Close()
func ConnectFromInfo(ctx context.Context, infoPath string) (*Client, error) {
	if infoPath == "" {
		return nil, fmt.Errorf("daemon info path cannot be empty")
	}

	// Read daemon connection info from file
	info, err := daemon.ReadDaemonInfo(infoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read daemon info: %w", err)
	}

	// Prefer Unix socket if available, otherwise use gRPC address
	address := info.GRPCAddress
	if info.SocketPath != "" {
		address = info.SocketPath
	}

	// Connect using the discovered address
	client, err := Connect(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon (PID %d): %w", info.PID, err)
	}

	return client, nil
}

// Close closes the connection to the daemon.
//
// This method should be called when the client is no longer needed, typically
// using defer immediately after successful connection:
//
//	client, err := Connect(ctx, address)
//	if err != nil {
//	    return err
//	}
//	defer client.Close()
//
// Returns:
//   - error: Non-nil if connection close fails
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Ping checks if the daemon is responsive.
//
// This is a lightweight health check that can be used to verify the daemon
// is running and responding to requests. Useful for status commands and
// connection validation.
//
// Parameters:
//   - ctx: Context for the RPC call
//
// Returns:
//   - error: Non-nil if daemon doesn't respond or responds with an error
//
// Example:
//
//	if err := client.Ping(ctx); err != nil {
//	    return fmt.Errorf("daemon not responding: %w", err)
//	}
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.daemon.Ping(ctx, &api.PingRequest{})
	if err != nil {
		// Wrap error with user-friendly message
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.Unavailable:
				return fmt.Errorf("daemon not responding (connection unavailable)")
			case codes.DeadlineExceeded:
				return fmt.Errorf("daemon ping timeout")
			default:
				return fmt.Errorf("daemon ping failed: %s", st.Message())
			}
		}
		return fmt.Errorf("daemon ping failed: %w", err)
	}
	return nil
}

// Status retrieves the daemon's current status and health information.
//
// This method queries the daemon for comprehensive status including:
//   - Process state (running, PID, uptime)
//   - Service endpoints (registry, callback, gRPC)
//   - Component counts (agents, missions, active missions)
//
// Parameters:
//   - ctx: Context for the RPC call
//
// Returns:
//   - *daemon.DaemonStatus: Complete status information
//   - error: Non-nil if RPC fails or daemon is unhealthy
//
// Example:
//
//	status, err := client.Status(ctx)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Daemon uptime: %s\n", status.Uptime)
//	fmt.Printf("Registered agents: %d\n", status.AgentCount)
func (c *Client) Status(ctx context.Context) (*daemon.DaemonStatus, error) {
	resp, err := c.daemon.Status(ctx, &api.StatusRequest{})
	if err != nil {
		// Wrap error with user-friendly message
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.Unavailable:
				return nil, fmt.Errorf("daemon not responding (is it running?)")
			case codes.DeadlineExceeded:
				return nil, fmt.Errorf("daemon status request timeout")
			default:
				return nil, fmt.Errorf("failed to get daemon status: %s", st.Message())
			}
		}
		return nil, fmt.Errorf("failed to get daemon status: %w", err)
	}

	return convertProtoStatus(resp), nil
}

// AgentInfo represents information about a registered agent.
type AgentInfo struct {
	Name        string
	Version     string
	Description string
	Address     string
	Status      string
}

// ToolInfo represents information about a registered tool.
type ToolInfo struct {
	Name        string
	Version     string
	Description string
	Address     string
	Status      string
}

// PluginInfo represents information about a registered plugin.
type PluginInfo struct {
	Name        string
	Version     string
	Description string
	Address     string
	Status      string
}

// ListAgents retrieves a list of all registered agents from the daemon.
//
// This method queries the daemon's agent registry to get information about
// all agents that are currently registered and available for mission execution.
//
// Parameters:
//   - ctx: Context for the RPC call
//
// Returns:
//   - []AgentInfo: List of agent information
//   - error: Non-nil if RPC fails
//
// Example:
//
//	agents, err := client.ListAgents(ctx)
//	if err != nil {
//	    return err
//	}
//	for _, agent := range agents {
//	    fmt.Printf("Agent: %s (v%s) - %s\n", agent.Name, agent.Version, agent.Status)
//	}
func (c *Client) ListAgents(ctx context.Context) ([]AgentInfo, error) {
	resp, err := c.daemon.ListAgents(ctx, &api.ListAgentsRequest{})
	if err != nil {
		// Wrap error with user-friendly message
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.Unavailable:
				return nil, fmt.Errorf("daemon not responding (is it running?)")
			case codes.DeadlineExceeded:
				return nil, fmt.Errorf("daemon request timeout while listing agents")
			default:
				return nil, fmt.Errorf("failed to list agents: %s", st.Message())
			}
		}
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	// Convert proto agents to domain types
	agents := convertProtoAgents(resp.Agents)
	return agents, nil
}

// ListTools retrieves a list of all registered tools from the daemon.
//
// This method queries the daemon's tool registry to get information about
// all tools that are currently registered and available for use.
//
// Parameters:
//   - ctx: Context for the RPC call
//
// Returns:
//   - []ToolInfo: List of tool information
//   - error: Non-nil if RPC fails
//
// Example:
//
//	tools, err := client.ListTools(ctx)
//	if err != nil {
//	    return err
//	}
//	for _, tool := range tools {
//	    fmt.Printf("Tool: %s (v%s) - %s\n", tool.Name, tool.Version, tool.Status)
//	}
func (c *Client) ListTools(ctx context.Context) ([]ToolInfo, error) {
	resp, err := c.daemon.ListTools(ctx, &api.ListToolsRequest{})
	if err != nil {
		// Wrap error with user-friendly message
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.Unavailable:
				return nil, fmt.Errorf("daemon not responding (is it running?)")
			case codes.DeadlineExceeded:
				return nil, fmt.Errorf("daemon request timeout while listing tools")
			default:
				return nil, fmt.Errorf("failed to list tools: %s", st.Message())
			}
		}
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	// Convert proto tools to domain types
	tools := convertProtoTools(resp.Tools)
	return tools, nil
}

// ListPlugins retrieves a list of all registered plugins from the daemon.
//
// This method queries the daemon's plugin registry to get information about
// all plugins that are currently registered and available for use.
//
// Parameters:
//   - ctx: Context for the RPC call
//
// Returns:
//   - []PluginInfo: List of plugin information
//   - error: Non-nil if RPC fails
//
// Example:
//
//	plugins, err := client.ListPlugins(ctx)
//	if err != nil {
//	    return err
//	}
//	for _, plugin := range plugins {
//	    fmt.Printf("Plugin: %s (v%s) - %s\n", plugin.Name, plugin.Version, plugin.Status)
//	}
func (c *Client) ListPlugins(ctx context.Context) ([]PluginInfo, error) {
	resp, err := c.daemon.ListPlugins(ctx, &api.ListPluginsRequest{})
	if err != nil {
		// Wrap error with user-friendly message
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.Unavailable:
				return nil, fmt.Errorf("daemon not responding (is it running?)")
			case codes.DeadlineExceeded:
				return nil, fmt.Errorf("daemon request timeout while listing plugins")
			default:
				return nil, fmt.Errorf("failed to list plugins: %s", st.Message())
			}
		}
		return nil, fmt.Errorf("failed to list plugins: %w", err)
	}

	// Convert proto plugins to domain types
	plugins := convertProtoPlugins(resp.Plugins)
	return plugins, nil
}

// MissionEvent represents an event from a running mission.
type MissionEvent struct {
	Type      string
	Timestamp time.Time
	Message   string
	Data      map[string]interface{}
}

// RunMission executes a mission workflow via the daemon and streams events.
//
// This method starts a mission execution on the daemon and returns a channel
// that receives mission events as they occur. Events include mission start,
// agent execution, tool invocations, findings, and mission completion.
//
// The returned channel is closed when the mission completes or encounters an error.
// Callers should read from the channel until it closes to get all mission events.
//
// Parameters:
//   - ctx: Context for the mission execution (cancellation stops the mission)
//   - workflowPath: Path to the workflow YAML file
//
// Returns:
//   - <-chan MissionEvent: Channel receiving mission events
//   - error: Non-nil if mission start fails (not for mission execution errors)
//
// Example:
//
//	events, err := client.RunMission(ctx, "/path/to/workflow.yaml")
//	if err != nil {
//	    return err
//	}
//
//	for event := range events {
//	    fmt.Printf("[%s] %s: %s\n", event.Timestamp, event.Type, event.Message)
//	}
func (c *Client) RunMission(ctx context.Context, workflowPath string) (<-chan MissionEvent, error) {
	// Start streaming RPC
	stream, err := c.daemon.RunMission(ctx, &api.RunMissionRequest{
		WorkflowPath: workflowPath,
	})
	if err != nil {
		// Wrap error with user-friendly message
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.Unavailable:
				return nil, fmt.Errorf("daemon not responding (is it running?)")
			case codes.NotFound:
				return nil, fmt.Errorf("workflow file not found: %s", workflowPath)
			case codes.InvalidArgument:
				return nil, fmt.Errorf("invalid workflow: %s", st.Message())
			default:
				return nil, fmt.Errorf("failed to start mission: %s", st.Message())
			}
		}
		return nil, fmt.Errorf("failed to start mission: %w", err)
	}

	// Create event channel and spawn goroutine to read from stream
	eventChan := make(chan MissionEvent, 10) // Buffer to avoid blocking daemon
	go func() {
		defer close(eventChan)
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				// Stream completed normally
				return
			}
			if err != nil {
				// Check if context was cancelled
				if ctx.Err() != nil {
					return
				}
				// Stream error - log and exit
				// TODO: Consider sending error event to channel
				return
			}

			// Convert and send event
			select {
			case eventChan <- convertProtoMissionEvent(event):
			case <-ctx.Done():
				return
			}
		}
	}()

	return eventChan, nil
}

// AttackOptions contains options for running an attack.
type AttackOptions struct {
	Target      string
	AttackType  string
	Credentials []string
	Payloads    []string
	MaxDepth    int
	Timeout     time.Duration
}

// AttackEvent represents an event from a running attack.
type AttackEvent struct {
	Type      string
	Timestamp time.Time
	Message   string
	Severity  string
	Data      map[string]interface{}
}

// RunAttack executes an attack via the daemon and streams events.
//
// This method starts an attack execution on the daemon and returns a channel
// that receives attack events as they occur. Events include attack start,
// payload execution, vulnerability discovery, and attack completion.
//
// The returned channel is closed when the attack completes or encounters an error.
// Callers should read from the channel until it closes to get all attack events.
//
// Parameters:
//   - ctx: Context for the attack execution (cancellation stops the attack)
//   - opts: Attack configuration options
//
// Returns:
//   - <-chan AttackEvent: Channel receiving attack events
//   - error: Non-nil if attack start fails (not for attack execution errors)
//
// Example:
//
//	opts := AttackOptions{
//	    Target:     "http://target.example.com",
//	    AttackType: "prompt-injection",
//	    MaxDepth:   3,
//	    Timeout:    30 * time.Minute,
//	}
//	events, err := client.RunAttack(ctx, opts)
//	if err != nil {
//	    return err
//	}
//
//	for event := range events {
//	    if event.Severity == "high" {
//	        fmt.Printf("[!] %s: %s\n", event.Type, event.Message)
//	    }
//	}
func (c *Client) RunAttack(ctx context.Context, opts AttackOptions) (<-chan AttackEvent, error) {
	// Start streaming RPC
	stream, err := c.daemon.RunAttack(ctx, &api.RunAttackRequest{
		Target:     opts.Target,
		AttackType: opts.AttackType,
		AgentId:    opts.AttackType, // Use attack type as agent ID
	})
	if err != nil {
		// Wrap error with user-friendly message
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.Unavailable:
				return nil, fmt.Errorf("daemon not responding (is it running?)")
			case codes.NotFound:
				return nil, fmt.Errorf("attack agent not found: %s", opts.AttackType)
			case codes.InvalidArgument:
				return nil, fmt.Errorf("invalid attack configuration: %s", st.Message())
			default:
				return nil, fmt.Errorf("failed to start attack: %s", st.Message())
			}
		}
		return nil, fmt.Errorf("failed to start attack: %w", err)
	}

	// Create event channel and spawn goroutine to read from stream
	eventChan := make(chan AttackEvent, 10) // Buffer to avoid blocking daemon
	go func() {
		defer close(eventChan)
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				// Stream completed normally
				return
			}
			if err != nil {
				// Check if context was cancelled
				if ctx.Err() != nil {
					return
				}
				// Stream error - log and exit
				// TODO: Consider sending error event to channel
				return
			}

			// Convert and send event
			select {
			case eventChan <- convertProtoAttackEvent(event):
			case <-ctx.Done():
				return
			}
		}
	}()

	return eventChan, nil
}

// Event represents a generic daemon event for TUI subscription.
type Event struct {
	Type      string
	Source    string
	Timestamp time.Time
	Data      map[string]interface{}
}

// Subscribe subscribes to all daemon events for real-time updates.
//
// This method establishes a streaming connection to the daemon that receives
// all significant events including:
//   - Agent registration/deregistration
//   - Mission start/stop
//   - Finding discoveries
//   - System health changes
//
// The returned channel is closed when the subscription ends (context cancellation,
// daemon shutdown, or connection loss). This is primarily used by the TUI for
// real-time dashboard updates.
//
// Parameters:
//   - ctx: Context for the subscription (cancellation stops the stream)
//
// Returns:
//   - <-chan Event: Channel receiving all daemon events
//   - error: Non-nil if subscription setup fails
//
// Example:
//
//	events, err := client.Subscribe(ctx)
//	if err != nil {
//	    return err
//	}
//
//	for event := range events {
//	    switch event.Type {
//	    case "agent_registered":
//	        updateAgentList(event.Data)
//	    case "mission_started":
//	        updateMissionView(event.Data)
//	    case "finding_discovered":
//	        showFindingNotification(event.Data)
//	    }
//	}
func (c *Client) Subscribe(ctx context.Context) (<-chan Event, error) {
	// Start streaming RPC
	stream, err := c.daemon.Subscribe(ctx, &api.SubscribeRequest{})
	if err != nil {
		// Wrap error with user-friendly message
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.Unavailable:
				return nil, fmt.Errorf("daemon not responding (is it running?)")
			case codes.PermissionDenied:
				return nil, fmt.Errorf("permission denied for event subscription")
			default:
				return nil, fmt.Errorf("failed to subscribe to events: %s", st.Message())
			}
		}
		return nil, fmt.Errorf("failed to subscribe to events: %w", err)
	}

	// Create event channel and spawn goroutine to read from stream
	eventChan := make(chan Event, 50) // Larger buffer for high-frequency events
	go func() {
		defer close(eventChan)
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				// Stream completed normally
				return
			}
			if err != nil {
				// Check if context was cancelled
				if ctx.Err() != nil {
					return
				}
				// Stream error - log and exit
				// TODO: Consider sending error event to channel
				return
			}

			// Convert and send event
			select {
			case eventChan <- convertProtoEvent(event):
			case <-ctx.Done():
				return
			}
		}
	}()

	return eventChan, nil
}

// StartResult represents the result of starting a component.
type StartResult struct {
	PID     int
	Port    int
	LogPath string
}

// StopResult represents the result of stopping a component.
type StopResult struct {
	StoppedCount int
	TotalCount   int
}

// StartAgent starts an agent by name.
func (c *Client) StartAgent(ctx context.Context, name string) (*StartResult, error) {
	return c.startComponent(ctx, "agent", name)
}

// StopAgent stops an agent by name.
func (c *Client) StopAgent(ctx context.Context, name string) (*StopResult, error) {
	return c.stopComponent(ctx, "agent", name, false)
}

// StartTool starts a tool by name.
func (c *Client) StartTool(ctx context.Context, name string) (*StartResult, error) {
	return c.startComponent(ctx, "tool", name)
}

// StopTool stops a tool by name.
func (c *Client) StopTool(ctx context.Context, name string) (*StopResult, error) {
	return c.stopComponent(ctx, "tool", name, false)
}

// StartPlugin starts a plugin by name.
func (c *Client) StartPlugin(ctx context.Context, name string) (*StartResult, error) {
	return c.startComponent(ctx, "plugin", name)
}

// StopPlugin stops a plugin by name.
func (c *Client) StopPlugin(ctx context.Context, name string) (*StopResult, error) {
	return c.stopComponent(ctx, "plugin", name, false)
}

// startComponent is the internal method that starts a component via the daemon.
func (c *Client) startComponent(ctx context.Context, kind, name string) (*StartResult, error) {
	resp, err := c.daemon.StartComponent(ctx, &api.StartComponentRequest{
		Kind: kind,
		Name: name,
	})
	if err != nil {
		// Wrap error with user-friendly message
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.Unavailable:
				return nil, fmt.Errorf("daemon not responding (is it running?)")
			case codes.NotFound:
				return nil, fmt.Errorf("component '%s' not found", name)
			case codes.AlreadyExists:
				return nil, fmt.Errorf("component '%s' is already running", name)
			case codes.InvalidArgument:
				return nil, fmt.Errorf("invalid component kind or name: %s", st.Message())
			default:
				return nil, fmt.Errorf("failed to start component: %s", st.Message())
			}
		}
		return nil, fmt.Errorf("failed to start component: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("failed to start component: %s", resp.Message)
	}

	return &StartResult{
		PID:     int(resp.Pid),
		Port:    int(resp.Port),
		LogPath: resp.LogPath,
	}, nil
}

// stopComponent is the internal method that stops a component via the daemon.
func (c *Client) stopComponent(ctx context.Context, kind, name string, force bool) (*StopResult, error) {
	resp, err := c.daemon.StopComponent(ctx, &api.StopComponentRequest{
		Kind:  kind,
		Name:  name,
		Force: force,
	})
	if err != nil {
		// Wrap error with user-friendly message
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.Unavailable:
				return nil, fmt.Errorf("daemon not responding (is it running?)")
			case codes.NotFound:
				return nil, fmt.Errorf("component '%s' is not running", name)
			case codes.InvalidArgument:
				return nil, fmt.Errorf("invalid component kind or name: %s", st.Message())
			default:
				return nil, fmt.Errorf("failed to stop component: %s", st.Message())
			}
		}
		return nil, fmt.Errorf("failed to stop component: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("failed to stop component: %s", resp.Message)
	}

	return &StopResult{
		StoppedCount: int(resp.StoppedCount),
		TotalCount:   int(resp.TotalCount),
	}, nil
}
