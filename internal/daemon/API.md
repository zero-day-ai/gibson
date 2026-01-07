# Gibson Daemon API Documentation

This document describes the Gibson daemon's gRPC API for client communication. The daemon exposes mission orchestration, component discovery, and event streaming capabilities via gRPC.

## Table of Contents

1. [Overview](#overview)
2. [Connection and Health](#connection-and-health)
3. [Component Discovery](#component-discovery)
4. [Mission Lifecycle](#mission-lifecycle)
5. [Attack Operations](#attack-operations)
6. [Event Streaming](#event-streaming)
7. [Error Handling](#error-handling)
8. [Client Usage Examples](#client-usage-examples)

## Overview

The Gibson daemon uses a client-server architecture where:
- **Daemon**: Long-running background process that manages:
  - Embedded etcd registry for component discovery
  - Callback server for agent harnesses
  - gRPC API server for client connections
  - Mission orchestration and execution

- **Client**: CLI commands or TUI that connect to daemon via gRPC to:
  - Start and monitor missions
  - List registered agents, tools, and plugins
  - Stream real-time events for TUI updates
  - Execute attacks with live progress

### Connection Information

The daemon writes connection information to `~/.gibson/daemon.json`:

```json
{
  "pid": 12345,
  "start_time": "2024-01-15T10:30:00Z",
  "grpc_address": "localhost:50002",
  "version": "0.1.0"
}
```

Clients read this file to discover the daemon's gRPC endpoint.

## Connection and Health

### Connect

Establishes a client connection and returns session information.

**Request:**
```protobuf
message ConnectRequest {
  string client_version = 1;  // Version of Gibson CLI
  string client_id = 2;       // Optional unique client identifier
}
```

**Response:**
```protobuf
message ConnectResponse {
  string daemon_version = 1;  // Version of running daemon
  string session_id = 2;      // Unique session identifier
  string grpc_address = 3;    // gRPC address daemon is listening on
}
```

**Usage:**
```go
resp, err := client.Connect(ctx, &ConnectRequest{
    ClientVersion: "0.1.0",
    ClientId:      "cli-session-1",
})
```

### Ping

Health check that verifies daemon responsiveness.

**Request:**
```protobuf
message PingRequest {}
```

**Response:**
```protobuf
message PingResponse {
  int64 timestamp = 1;  // Server time when ping received (Unix timestamp)
}
```

**Usage:**
```go
resp, err := client.Ping(ctx, &PingRequest{})
if err == nil {
    fmt.Printf("Daemon responsive at %d\n", resp.Timestamp)
}
```

### Status

Returns comprehensive daemon status and health information.

**Request:**
```protobuf
message StatusRequest {}
```

**Response:**
```protobuf
message StatusResponse {
  bool running = 1;              // Always true if responding
  int32 pid = 2;                 // Process ID
  int64 start_time = 3;          // Start time (Unix timestamp)
  string uptime = 4;             // Human-readable uptime (e.g., "2h 15m")
  string grpc_address = 5;       // gRPC server address
  string registry_type = 6;      // "embedded" or "etcd"
  string registry_addr = 7;      // Registry endpoint
  string callback_addr = 8;      // Callback server endpoint
  int32 agent_count = 9;         // Number of registered agents
  int32 mission_count = 10;      // Total missions (historical)
  int32 active_mission_count = 11;  // Currently running missions
}
```

**Usage:**
```go
status, err := client.Status(ctx, &StatusRequest{})
if err == nil {
    fmt.Printf("Daemon running: %v\n", status.Running)
    fmt.Printf("Uptime: %s\n", status.Uptime)
    fmt.Printf("Active missions: %d\n", status.ActiveMissionCount)
}
```

## Component Discovery

The daemon provides APIs to discover agents, tools, and plugins registered in the etcd registry.

### ListAgents

Returns all registered agents from the component registry.

**Request:**
```protobuf
message ListAgentsRequest {
  string kind = 1;  // Optional filter by component kind
}
```

**Response:**
```protobuf
message ListAgentsResponse {
  repeated AgentInfo agents = 1;
}

message AgentInfo {
  string id = 1;                   // Unique agent identifier
  string name = 2;                 // Agent name
  string kind = 3;                 // Component kind (always "agent")
  string version = 4;              // Agent version
  string endpoint = 5;             // gRPC endpoint
  repeated string capabilities = 6;  // Agent capabilities
  string health = 7;               // Health status ("healthy", "unknown")
  int64 last_seen = 8;             // Last seen timestamp
}
```

**Usage:**
```go
resp, err := client.ListAgents(ctx, &ListAgentsRequest{})
for _, agent := range resp.Agents {
    fmt.Printf("Agent: %s (%s) - %s\n", agent.Name, agent.Version, agent.Health)
}
```

### GetAgentStatus

Returns detailed status for a specific agent.

**Request:**
```protobuf
message GetAgentStatusRequest {
  string agent_id = 1;  // Agent identifier
}
```

**Response:**
```protobuf
message AgentStatusResponse {
  AgentInfo agent = 1;          // Agent information
  bool active = 2;              // Currently executing a task
  string current_task = 3;      // Description of active task
  int64 task_start_time = 4;    // When current task started
}
```

**Usage:**
```go
resp, err := client.GetAgentStatus(ctx, &GetAgentStatusRequest{
    AgentId: "recon-agent",
})
if resp.Active {
    fmt.Printf("Agent executing: %s\n", resp.CurrentTask)
}
```

### ListTools

Returns all registered tools from the component registry.

**Request:**
```protobuf
message ListToolsRequest {}
```

**Response:**
```protobuf
message ListToolsResponse {
  repeated ToolInfo tools = 1;
}

message ToolInfo {
  string id = 1;          // Unique tool identifier
  string name = 2;        // Tool name
  string version = 3;     // Tool version
  string endpoint = 4;    // gRPC endpoint
  string description = 5; // Tool description
  string health = 6;      // Health status
  int64 last_seen = 7;    // Last seen timestamp
}
```

### ListPlugins

Returns all registered plugins from the component registry.

**Request:**
```protobuf
message ListPluginsRequest {}
```

**Response:**
```protobuf
message ListPluginsResponse {
  repeated PluginInfo plugins = 1;
}

message PluginInfo {
  string id = 1;          // Unique plugin identifier
  string name = 2;        // Plugin name
  string version = 3;     // Plugin version
  string endpoint = 4;    // gRPC endpoint
  string description = 5; // Plugin description
  string health = 6;      // Health status
  int64 last_seen = 7;    // Last seen timestamp
}
```

## Mission Lifecycle

APIs for starting, monitoring, and stopping missions.

### RunMission (Server Streaming)

Starts a mission and streams execution events until completion.

**Request:**
```protobuf
message RunMissionRequest {
  string workflow_path = 1;           // Path to workflow YAML file
  string mission_id = 2;              // Optional custom mission identifier
  map<string, string> variables = 3;  // Workflow variables to override
}
```

**Response Stream:**
```protobuf
message MissionEvent {
  string event_type = 1;  // Event type (mission_started, node_started, etc.)
  int64 timestamp = 2;    // When event occurred (Unix timestamp)
  string mission_id = 3;  // Mission identifier
  string node_id = 4;     // Workflow node ID (if applicable)
  string message = 5;     // Human-readable message
  string data = 6;        // Event-specific data (JSON-encoded)
  string error = 7;       // Error information (if applicable)
}
```

**Event Types:**
- `mission_started` - Mission execution began
- `node_started` - Workflow node started executing
- `node_completed` - Workflow node completed successfully
- `node_failed` - Workflow node failed
- `finding_discovered` - Vulnerability finding discovered
- `mission_completed` - Mission finished successfully
- `mission_failed` - Mission failed

**Usage:**
```go
stream, err := client.RunMission(ctx, &RunMissionRequest{
    WorkflowPath: "/path/to/workflow.yaml",
    MissionId:    "recon-mission-1",
    Variables: map[string]string{
        "target": "example.com",
    },
})

for {
    event, err := stream.Recv()
    if err == io.EOF {
        break  // Mission completed
    }
    if err != nil {
        log.Fatalf("Stream error: %v", err)
    }

    fmt.Printf("[%s] %s: %s\n", event.EventType, event.NodeId, event.Message)
}
```

### StopMission

Stops a running mission gracefully or forcefully.

**Request:**
```protobuf
message StopMissionRequest {
  string mission_id = 1;  // Mission identifier to stop
  bool force = 2;         // Force-kill (true) or graceful stop (false)
}
```

**Response:**
```protobuf
message StopMissionResponse {
  bool success = 1;   // Whether stop was successful
  string message = 2; // Additional context
}
```

**Usage:**
```go
resp, err := client.StopMission(ctx, &StopMissionRequest{
    MissionId: "recon-mission-1",
    Force:     false,  // Graceful shutdown
})
```

### ListMissions

Returns list of missions (historical and active).

**Request:**
```protobuf
message ListMissionsRequest {
  bool active_only = 1;  // Filter to only running missions
  int32 limit = 2;       // Max results (pagination)
  int32 offset = 3;      // Pagination offset
}
```

**Response:**
```protobuf
message ListMissionsResponse {
  repeated MissionInfo missions = 1;
  int32 total = 2;  // Total count for pagination
}

message MissionInfo {
  string id = 1;              // Mission identifier
  string workflow_path = 2;   // Path to workflow file
  string status = 3;          // Status (running, completed, failed)
  int64 start_time = 4;       // Start time (Unix timestamp)
  int64 end_time = 5;         // End time (0 if still running)
  int32 finding_count = 6;    // Number of findings discovered
}
```

**Usage:**
```go
resp, err := client.ListMissions(ctx, &ListMissionsRequest{
    ActiveOnly: true,
    Limit:      10,
    Offset:     0,
})

for _, mission := range resp.Missions {
    fmt.Printf("Mission %s: %s (%d findings)\n",
        mission.Id, mission.Status, mission.FindingCount)
}
```

## Attack Operations

APIs for executing attacks with real-time progress.

### RunAttack (Server Streaming)

Executes an attack and streams progress events.

**Request:**
```protobuf
message RunAttackRequest {
  string target = 1;                 // Target URL or identifier
  string attack_type = 2;            // Type of attack to execute
  string agent_id = 3;               // Agent to use (optional, auto-select if empty)
  string payload_filter = 4;         // Filter for payloads to use
  map<string, string> options = 5;   // Attack-specific options
}
```

**Response Stream:**
```protobuf
message AttackEvent {
  string event_type = 1;     // Event type (attack_started, etc.)
  int64 timestamp = 2;       // When event occurred
  string attack_id = 3;      // Attack identifier
  string message = 4;        // Human-readable message
  string data = 5;           // Event-specific data (JSON)
  string error = 6;          // Error information
  FindingInfo finding = 7;   // Finding details (if discovered)
}

message FindingInfo {
  string id = 1;          // Finding identifier
  string title = 2;       // Finding title
  string severity = 3;    // Severity level
  string category = 4;    // Finding category
  string description = 5; // Detailed description
  string technique = 6;   // MITRE ATT&CK/ATLAS technique
  string evidence = 7;    // Supporting evidence
  int64 timestamp = 8;    // Discovery timestamp
}
```

**Usage:**
```go
stream, err := client.RunAttack(ctx, &RunAttackRequest{
    Target:        "https://api.example.com",
    AttackType:    "prompt-injection",
    PayloadFilter: "severity:high",
    Options: map[string]string{
        "max_payloads": "50",
    },
})

for {
    event, err := stream.Recv()
    if err == io.EOF {
        break
    }

    if event.Finding != nil {
        fmt.Printf("FINDING: %s (%s)\n", event.Finding.Title, event.Finding.Severity)
    }
}
```

## Event Streaming

API for subscribing to real-time daemon events (for TUI).

### Subscribe (Server Streaming)

Establishes an event stream for real-time updates.

**Request:**
```protobuf
message SubscribeRequest {
  repeated string event_types = 1;  // Filter by event types (empty = all)
  string mission_id = 2;            // Filter by mission ID (empty = all)
}
```

**Response Stream:**
```protobuf
message Event {
  string event_type = 1;   // Event type
  int64 timestamp = 2;     // When event occurred
  string source = 3;       // Event source (mission, agent, daemon)
  string data = 4;         // Event-specific data (JSON)

  // Specific event types (only one will be set)
  oneof event {
    MissionEvent mission_event = 5;
    AttackEvent attack_event = 6;
    AgentEvent agent_event = 7;
    FindingEvent finding_event = 8;
  }
}

message AgentEvent {
  string event_type = 1;  // agent_registered, agent_unregistered, health_change
  int64 timestamp = 2;
  string agent_id = 3;
  string agent_name = 4;
  string message = 5;
  string data = 6;
}

message FindingEvent {
  string event_type = 1;   // finding_discovered, finding_updated
  int64 timestamp = 2;
  FindingInfo finding = 3;
  string mission_id = 4;
}
```

**Usage:**
```go
stream, err := client.Subscribe(ctx, &SubscribeRequest{
    EventTypes: []string{"mission_started", "finding_discovered"},
    MissionId:  "",  // All missions
})

for {
    event, err := stream.Recv()
    if err != nil {
        break
    }

    switch e := event.Event.(type) {
    case *Event_MissionEvent:
        fmt.Printf("Mission: %s\n", e.MissionEvent.Message)
    case *Event_FindingEvent:
        fmt.Printf("Finding: %s\n", e.FindingEvent.Finding.Title)
    case *Event_AgentEvent:
        fmt.Printf("Agent: %s\n", e.AgentEvent.Message)
    }
}
```

## Error Handling

The daemon API uses standard gRPC status codes for errors:

### Common Error Codes

| Code | Condition | Example |
|------|-----------|---------|
| `OK` | Success | Request completed successfully |
| `INVALID_ARGUMENT` | Invalid request | Missing required field, invalid format |
| `NOT_FOUND` | Resource not found | Mission ID doesn't exist, agent not registered |
| `ALREADY_EXISTS` | Duplicate resource | Mission ID already in use |
| `INTERNAL` | Internal error | Database error, registry unavailable |
| `UNAVAILABLE` | Service unavailable | Daemon shutting down, registry offline |
| `DEADLINE_EXCEEDED` | Timeout | Operation took too long |
| `CANCELLED` | Client cancelled | Context cancelled by client |

### Error Response Format

Errors include descriptive messages:

```go
_, err := client.RunMission(ctx, req)
if err != nil {
    if st, ok := status.FromError(err); ok {
        fmt.Printf("Error code: %s\n", st.Code())
        fmt.Printf("Error message: %s\n", st.Message())
    }
}
```

### Implementation Status Errors

Some operations return "not yet implemented" errors until full implementation:

```
rpc error: code = Internal desc = mission execution not yet implemented
rpc error: code = Internal desc = attack execution not yet implemented
rpc error: code = Internal desc = event subscription not yet implemented
```

These will be replaced with actual functionality in future releases.

## Client Usage Examples

### Complete Mission Execution Example

```go
package main

import (
    "context"
    "fmt"
    "io"
    "log"
    "time"

    "github.com/zero-day-ai/gibson/internal/daemon/client"
)

func main() {
    ctx := context.Background()

    // Connect to daemon
    c, err := client.ConnectFromInfo(ctx, "/home/user/.gibson/daemon.json")
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer c.Close()

    // Check daemon status
    status, err := c.Status(ctx)
    if err != nil {
        log.Fatalf("Failed to get status: %v", err)
    }
    fmt.Printf("Daemon uptime: %s\n", status.Uptime)
    fmt.Printf("Active missions: %d\n", status.ActiveMissionCount)

    // List available agents
    agents, err := c.ListAgents(ctx)
    if err != nil {
        log.Fatalf("Failed to list agents: %v", err)
    }
    fmt.Printf("Found %d agents\n", len(agents))

    // Run mission (when implemented)
    // Note: This will return "not yet implemented" error currently
    stream, err := c.RunMission(ctx, &RunMissionRequest{
        WorkflowPath: "/path/to/workflow.yaml",
        Variables: map[string]string{
            "target": "example.com",
        },
    })
    if err != nil {
        log.Printf("Mission not yet implemented: %v", err)
        return
    }

    // Stream mission events
    for {
        event, err := stream.Recv()
        if err == io.EOF {
            fmt.Println("Mission completed")
            break
        }
        if err != nil {
            log.Fatalf("Stream error: %v", err)
        }

        fmt.Printf("[%s] %s\n", event.EventType, event.Message)

        if event.Error != "" {
            fmt.Printf("Error: %s\n", event.Error)
        }
    }
}
```

### TUI Event Subscription Example

```go
// Subscribe to all events for TUI real-time updates
stream, err := client.Subscribe(ctx, &SubscribeRequest{})
if err != nil {
    log.Fatalf("Failed to subscribe: %v", err)
}

eventChan := make(chan *Event)
go func() {
    for {
        event, err := stream.Recv()
        if err != nil {
            close(eventChan)
            return
        }
        eventChan <- event
    }
}()

// Process events in TUI
for event := range eventChan {
    updateTUI(event)  // Update TUI display
}
```

## Testing

Integration tests are provided in:
- `mission_integration_test.go` - Mission lifecycle testing
- `error_scenarios_integration_test.go` - Error handling testing
- `event_streaming_integration_test.go` - Event streaming testing

Run integration tests:
```bash
cd internal/daemon
go test -tags integration,fts5 -v
```

Test workflow files are in `testdata/`:
- `simple-workflow.yaml` - Basic linear workflow
- `parallel-workflow.yaml` - Parallel execution testing
- `invalid-workflow.yaml` - Error scenario testing

## Future Enhancements

Planned improvements:
1. **Mission persistence** - Store mission state in SQLite for resumption
2. **Event replay** - Allow clients to replay historical events
3. **Mission templates** - Predefined attack workflows
4. **Real-time metrics** - Performance and progress metrics
5. **Webhook support** - External notifications for events
6. **Multi-daemon** - Distributed mission execution across multiple daemons

## See Also

- Proto definitions: `internal/daemon/api/daemon.proto`
- Client implementation: `internal/daemon/client/`
- Server implementation: `internal/daemon/api/server.go`
- Daemon implementation: `internal/daemon/daemon.go`
