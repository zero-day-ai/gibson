# Requirements Document: Tool Executor Service

## Introduction

The Tool Executor Service is a fundamental architectural refactor of Gibson's tool lifecycle management. Currently, tools are individual gRPC servers that must be manually started (`gibson tool start <name>`), each consuming a port and requiring explicit daemon management. This creates friction, port conflicts, and makes tools unavailable to agents unless manually configured.

The new architecture introduces a single **Tool Executor Service** managed by the daemon that:
- Automatically discovers all installed tool binaries
- Executes tools on-demand as subprocesses
- Eliminates port-per-tool chaos
- Enables dynamic tool availability without agent restarts
- Decouples agents from tool lifecycle management

**Value:** Agents can call any installed tool immediately without manual startup. Tools become "batteries included" - install once, available everywhere, hot-addable during missions.

## Alignment with Product Vision

This feature supports Gibson's core mission of autonomous security testing by:
- Reducing operational friction for tool management
- Enabling agents to dynamically adapt based on available tools
- Supporting hot-deployment of new capabilities during active missions
- Simplifying the developer experience for tool creation

## Requirements

### Requirement 1: Tool Executor Service Lifecycle

**User Story:** As the Gibson daemon, I want to manage a single Tool Executor Service that handles all tool invocations, so that tools don't need individual port management and manual startup.

#### Acceptance Criteria

1. WHEN the daemon starts THEN the daemon SHALL initialize the Tool Executor Service as part of its startup sequence
2. WHEN the Tool Executor Service starts THEN it SHALL scan `~/.gibson/tools/bin/` for installed tool binaries
3. WHEN the Tool Executor Service starts THEN it SHALL build a registry of available tools (name → binary path + cached schema)
4. WHEN the daemon stops THEN it SHALL gracefully shutdown the Tool Executor Service and any running tool subprocesses
5. IF a tool binary is added to `~/.gibson/tools/bin/` while the service is running THEN the Tool Executor Service SHALL detect and register the new tool within 30 seconds

### Requirement 2: Tool Invocation via Executor

**User Story:** As an agent harness, I want to invoke tools through the Tool Executor Service, so that I don't need to manage gRPC connections to individual tool servers.

#### Acceptance Criteria

1. WHEN `harness.CallTool(name, input)` is invoked THEN the harness SHALL route the request to the Tool Executor Service via the daemon
2. WHEN the Tool Executor Service receives an Execute request THEN it SHALL spawn the tool binary as a subprocess with the input provided via stdin (JSON)
3. WHEN the tool subprocess completes THEN the Tool Executor Service SHALL capture stdout as JSON output and return it to the caller
4. IF the tool subprocess exits with non-zero status THEN the Tool Executor Service SHALL return an error with stderr contents
5. WHEN executing a tool THEN the Tool Executor Service SHALL enforce a configurable timeout (default: 5 minutes)
6. IF the tool subprocess exceeds the timeout THEN the Tool Executor Service SHALL kill the subprocess and return a timeout error

### Requirement 3: Tool Schema Discovery

**User Story:** As an agent, I want to discover available tools and their schemas, so that I can determine which tools to use for a given task.

#### Acceptance Criteria

1. WHEN `harness.ListTools()` is called THEN the harness SHALL return descriptors for all tools registered with the Tool Executor Service
2. WHEN a tool is registered THEN the Tool Executor Service SHALL cache its input/output schemas by invoking the tool with `--schema` flag
3. IF a tool does not support `--schema` flag THEN the Tool Executor Service SHALL mark the tool as "schema-unknown" but still allow execution
4. WHEN listing tools THEN each tool descriptor SHALL include: name, version, description, tags, input_schema, output_schema

### Requirement 4: Subprocess Execution Model

**User Story:** As the Tool Executor Service, I want to execute tools as short-lived subprocesses, so that tool crashes don't affect the daemon and resources are released after execution.

#### Acceptance Criteria

1. WHEN executing a tool THEN the Tool Executor Service SHALL spawn a new subprocess for each invocation
2. WHEN spawning a subprocess THEN the Tool Executor Service SHALL pass input as JSON via stdin
3. WHEN spawning a subprocess THEN the Tool Executor Service SHALL capture stdout (JSON output) and stderr (error messages) separately
4. WHEN a tool subprocess completes THEN all subprocess resources SHALL be released immediately
5. IF multiple agents invoke the same tool concurrently THEN the Tool Executor Service SHALL spawn separate subprocesses for each invocation
6. WHEN spawning a subprocess THEN the Tool Executor Service SHALL set `GIBSON_TOOL_MODE=subprocess` environment variable to indicate execution mode

### Requirement 5: SDK Tool Binary Contract

**User Story:** As a tool developer using the SDK, I want a simple contract for creating tool binaries, so that my tools work with the Tool Executor Service.

#### Acceptance Criteria

1. WHEN a tool binary is invoked with `--schema` flag THEN it SHALL output its input/output schemas as JSON to stdout and exit
2. WHEN a tool binary is invoked without flags THEN it SHALL read JSON input from stdin
3. WHEN a tool binary completes successfully THEN it SHALL write JSON output to stdout and exit with code 0
4. IF a tool binary encounters an error THEN it SHALL write error details to stderr and exit with non-zero code
5. WHEN `GIBSON_TOOL_MODE=subprocess` is set THEN the SDK's `serve.Tool()` SHALL execute in subprocess mode instead of starting a gRPC server

### Requirement 6: Backward Compatibility Removal

**User Story:** As a Gibson maintainer, I want to remove the legacy tool gRPC server pattern, so that the codebase is simpler and there's one canonical way to run tools.

#### Acceptance Criteria

1. WHEN refactoring is complete THEN the `gibson tool start` command SHALL be removed
2. WHEN refactoring is complete THEN the `gibson tool stop` command SHALL be removed
3. WHEN refactoring is complete THEN tools SHALL NOT register individually with etcd
4. WHEN refactoring is complete THEN the Tool Executor Service SHALL be the only way to invoke tools
5. WHEN refactoring is complete THEN the SDK's `serve.Tool()` SHALL default to subprocess mode

### Requirement 7: Tool Execution Metrics and Observability

**User Story:** As an operator, I want visibility into tool execution metrics, so that I can monitor tool performance and troubleshoot issues.

#### Acceptance Criteria

1. WHEN a tool is executed THEN the Tool Executor Service SHALL record: execution count, success/failure count, duration
2. WHEN a tool execution fails THEN the Tool Executor Service SHALL log the error with tool name, input summary, and stderr
3. WHEN `gibson tool status` is invoked THEN it SHALL display metrics for all registered tools
4. WHEN a tool is executed THEN the Tool Executor Service SHALL emit an OpenTelemetry span for the execution

### Requirement 8: Daemon gRPC API for Tool Execution

**User Story:** As the agent harness, I want a gRPC API to invoke tools through the daemon, so that tool execution is centralized.

#### Acceptance Criteria

1. WHEN the daemon starts THEN it SHALL expose `ExecuteTool(name, input_json, timeout_ms)` RPC on the daemon gRPC service
2. WHEN `ExecuteTool` is called THEN the daemon SHALL delegate to the Tool Executor Service
3. WHEN `ExecuteTool` completes THEN it SHALL return the tool output or error
4. WHEN the daemon starts THEN it SHALL expose `ListAvailableTools()` RPC that returns all tool descriptors
5. WHEN the daemon starts THEN it SHALL expose `GetToolSchema(name)` RPC that returns a specific tool's schemas

## Non-Functional Requirements

### Performance

- Tool schema caching SHALL reduce overhead for repeated `ListTools()` calls
- Subprocess spawn time SHALL be under 100ms for typical tools
- Concurrent tool executions SHALL be supported (minimum 10 parallel invocations)
- Tool binary scanning SHALL complete within 5 seconds for up to 100 installed tools

### Security

- Tool subprocess SHALL run with the same privileges as the daemon (no privilege escalation)
- Tool input/output SHALL be validated against schemas when available
- Tool execution timeout SHALL prevent runaway processes from consuming resources
- Tool subprocess SHALL be killed if daemon receives shutdown signal

### Reliability

- Tool Executor Service crash SHALL NOT crash the daemon
- Individual tool subprocess crash SHALL NOT affect other tool executions
- Tool binary corruption/removal SHALL be detected and reported gracefully
- Hot-reload of new tools SHALL NOT interrupt in-flight tool executions

### Usability

- `gibson tool list` SHALL show all available tools with their status (ready, schema-unknown, missing-binary)
- `gibson tool invoke <name> --input '{...}'` SHALL allow direct tool testing via CLI
- Tool execution errors SHALL include actionable error messages with stderr contents
- Tool installation (`gibson tool install`) SHALL remain unchanged
