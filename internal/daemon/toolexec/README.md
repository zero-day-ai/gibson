# Tool Executor Service

The Tool Executor Service provides a centralized orchestration layer for discovering, managing, and executing external tool binaries as short-lived subprocesses. It eliminates the need for individual gRPC servers per tool, simplifying deployment and reducing resource overhead.

## Architecture

The service consists of three main components:

1. **BinaryScanner** (`scanner.go`) - Discovers tool binaries and retrieves their schemas
2. **SubprocessExecutor** (`executor.go`) - Executes tool binaries as subprocesses with JSON I/O
3. **ToolExecutorService** (`service.go`) - Orchestrates scanning, caching, and execution

## Features

- **Automatic Tool Discovery**: Scans a tools directory and automatically discovers executable binaries
- **Schema Introspection**: Fetches input/output schemas from tools via `--schema` flag
- **Subprocess Execution**: Runs tools as short-lived processes with JSON communication over stdin/stdout
- **Hot Reload**: Supports refreshing the tool registry without restarting the daemon
- **Metrics Tracking**: Records execution statistics (count, duration, success/failure) per tool
- **Thread-Safe**: Concurrent access to tool registry is protected with RWMutex
- **Timeout Handling**: Configurable timeouts with automatic subprocess termination
- **Error Handling**: Comprehensive error types for different failure scenarios

## Usage

### Basic Setup

```go
import (
    "context"
    "log/slog"
    "time"

    "github.com/zero-day-ai/gibson/internal/daemon/toolexec"
)

// Create the service
logger := slog.Default()
service := toolexec.NewToolExecutorService(
    "~/.gibson/tools/bin",  // Tools directory
    5*time.Minute,           // Default timeout
    logger,
)

// Start the service (scans for tools)
ctx := context.Background()
if err := service.Start(ctx); err != nil {
    log.Fatal(err)
}
defer service.Stop(ctx)
```

### List Available Tools

```go
tools := service.ListTools()
for _, tool := range tools {
    fmt.Printf("Tool: %s (status: %s)\n", tool.Name, tool.Status)
    fmt.Printf("  Description: %s\n", tool.Description)
    fmt.Printf("  Version: %s\n", tool.Version)
    fmt.Printf("  Path: %s\n", tool.BinaryPath)

    if tool.Metrics != nil {
        fmt.Printf("  Executions: %d (success: %d, failed: %d)\n",
            tool.Metrics.TotalExecutions,
            tool.Metrics.SuccessfulExecutions,
            tool.Metrics.FailedExecutions)
    }
}
```

### Execute a Tool

```go
// Prepare input
input := map[string]any{
    "message": "Hello, world!",
    "count": 5,
}

// Execute with 30-second timeout
output, err := service.Execute(ctx, "example-tool", input, 30*time.Second)
if err != nil {
    log.Printf("Execution failed: %v", err)
    return
}

// Process output
fmt.Printf("Result: %v\n", output)
```

### Get Tool Schema

```go
schema, err := service.GetToolSchema("example-tool")
if err != nil {
    log.Printf("Tool not found: %v", err)
    return
}

fmt.Printf("Input schema: %+v\n", schema.InputSchema)
fmt.Printf("Output schema: %+v\n", schema.OutputSchema)
```

### Hot Reload Tools

```go
// Rescan the tools directory
if err := service.RefreshTools(ctx); err != nil {
    log.Printf("Failed to refresh tools: %v", err)
}

// Tool registry is now updated with any new/removed tools
// Existing tool metrics are preserved
```

## Tool Binary Protocol

Tools must follow this protocol to work with the executor service:

### Schema Request (`--schema` flag)

When invoked with `--schema`, the tool must output a JSON schema to stdout:

```json
{
  "name": "example-tool",
  "version": "1.0.0",
  "description": "An example tool",
  "tags": ["example", "demo"],
  "input_schema": {
    "type": "object",
    "properties": {
      "message": {
        "type": "string",
        "description": "Message to process"
      }
    },
    "required": ["message"]
  },
  "output_schema": {
    "type": "object",
    "properties": {
      "result": {
        "type": "string",
        "description": "Processed result"
      }
    }
  }
}
```

### Execution Mode (stdin/stdout JSON)

When invoked without flags, the tool:
1. Reads JSON input from stdin
2. Processes the input
3. Writes JSON output to stdout
4. Exits with code 0 on success, non-zero on failure

Example:

```bash
# Input (stdin)
{"message": "Hello"}

# Output (stdout)
{"result": "Processed: Hello"}

# Exit code
0
```

### Environment Variables

The executor sets `GIBSON_TOOL_MODE=subprocess` to distinguish subprocess execution from gRPC mode.

## Error Handling

The service uses typed error codes for different failure scenarios:

- `ErrToolNotFound` - Tool not found in registry
- `ErrToolTimeout` - Execution exceeded timeout
- `ErrToolExecutionFailed` - Tool exited with non-zero code
- `ErrInvalidToolOutput` - Tool output is not valid JSON
- `ErrToolSpawnFailed` - Failed to spawn the subprocess
- `ErrToolSchemaFetchError` - Failed to fetch tool schema

## Tool Status

Tools can have three status values:

- **ready** - Tool has valid schemas and is ready for execution
- **schema-unknown** - Tool is executable but schemas couldn't be fetched
- **error** - Tool has an error (see `ErrorMessage` field)

## Metrics

The service tracks the following metrics per tool:

- `TotalExecutions` - Total number of execution attempts
- `SuccessfulExecutions` - Number of successful executions (exit code 0)
- `FailedExecutions` - Number of failed executions
- `TotalDuration` - Cumulative execution time
- `AverageDuration` - Mean execution time
- `LastExecutedAt` - Timestamp of most recent execution

## Thread Safety

The service is thread-safe and can handle concurrent:
- Tool execution requests
- Tool listing operations
- Schema retrieval
- Registry refreshes

All access to the internal tool registry is protected with `sync.RWMutex`.

## Configuration

### Default Tools Directory

The default tools directory is `~/.gibson/tools/bin/`. The service supports `~` expansion.

### Default Timeout

The default execution timeout is 5 minutes. This can be overridden per execution.

### Schema Fetch Timeout

Schema fetching has a fixed 5-second timeout to prevent blocking during tool discovery.

## Testing

The package includes comprehensive tests for all components:

```bash
# Run all tests
go test ./internal/daemon/toolexec/

# Run with verbose output
go test -v ./internal/daemon/toolexec/

# Run specific test
go test -v ./internal/daemon/toolexec/ -run TestToolExecutorService_Execute
```

## Design Decisions

### Why Subprocess Execution?

Subprocess execution offers several advantages:
- **Simple Deployment**: No need to manage separate gRPC servers per tool
- **Resource Efficiency**: Tools only consume resources during execution
- **Isolation**: Each execution is isolated in its own process
- **Language Agnostic**: Tools can be written in any language

### Why JSON for I/O?

JSON provides:
- Universal support across all languages
- Human-readable format for debugging
- Built-in support in Go's standard library
- Compatibility with existing tool ecosystems

### Why Schema Introspection?

Schema introspection enables:
- Automatic validation of tool inputs
- Dynamic tool discovery and registration
- Self-documenting tool interfaces
- IDE autocomplete and type safety

## Future Enhancements

Potential future improvements:

- Schema validation before execution
- Output schema validation
- Tool execution pooling for high-throughput scenarios
- Streaming I/O for large datasets
- Tool versioning and compatibility checks
- Circuit breaker pattern for failing tools
- Distributed tracing integration
