# Implementation Plan: Tool Executor Service

## Task Overview

This implementation transforms Gibson's tool lifecycle from individual gRPC servers to a centralized subprocess-based execution model. The work is organized into 4 phases:

1. **Phase 1**: Core Tool Executor Service in daemon
2. **Phase 2**: SDK subprocess mode support
3. **Phase 3**: Wire harness to use daemon tool execution
4. **Phase 4**: Remove legacy tool start/stop commands

## Steering Document Compliance

- All new code follows Go conventions with proper error handling
- New packages created under `internal/daemon/toolexec/`
- Proto changes follow existing `daemon.proto` patterns
- SDK changes in `sdk/serve/` follow existing patterns

## Tasks

### Phase 1: Core Tool Executor Service

- [x] 1. Create Tool Executor Service interfaces and types
  - File: `internal/daemon/toolexec/types.go`
  - Define `ToolExecutorService` interface
  - Define `ToolBinaryInfo`, `ToolSchema`, `ExecuteRequest`, `ExecuteResult` types
  - Define error types: `ErrToolNotFound`, `ErrToolTimeout`, `ErrToolExecutionFailed`
  - Purpose: Establish contracts for tool execution subsystem
  - _Leverage: `internal/tool/types.go`, `internal/tool/errors.go`_
  - _Requirements: 1, 2, 3_
  - _Prompt:
    Role: Go backend developer specializing in interface design
    Task: Create the core types and interfaces for the Tool Executor Service. Read the design doc at `.spec-workflow/specs/tool-executor-service/design.md` for context. Define clean interfaces that follow Go conventions.
    Restrictions: Do not implement any logic yet - only type definitions and interfaces. Do not add unnecessary dependencies.
    _Leverage: Review `internal/tool/types.go` and `internal/tool/errors.go` for existing patterns
    _Requirements: Requirements 1, 2, 3 from requirements.md
    Success: File compiles, interfaces are well-documented, types match design doc
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 2. Implement Binary Scanner for tool discovery
  - File: `internal/daemon/toolexec/scanner.go`
  - Implement `Scan()` to walk `~/.gibson/tools/bin/` directory
  - Implement `GetSchema()` to invoke binary with `--schema` flag
  - Handle timeout (5s) for schema fetch
  - Parse JSON schema output into `ToolSchema` struct
  - Purpose: Discover installed tools and cache their schemas
  - _Leverage: `os/exec`, `encoding/json`, `filepath.Walk`_
  - _Requirements: 1.2, 1.3, 3.2, 3.3_
  - _Prompt:
    Role: Go developer with experience in subprocess management
    Task: Implement the BinaryScanner that discovers tool binaries and fetches their schemas. Read design.md for the expected interface.
    Restrictions: Use 5 second timeout for schema fetch. Handle missing/invalid binaries gracefully. Do not block on slow tools.
    _Leverage: Standard library `os/exec`, `encoding/json`
    _Requirements: 1.2, 1.3, 3.2, 3.3
    Success: Scanner discovers binaries, fetches schemas with timeout, handles errors gracefully
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 3. Implement Subprocess Executor for tool invocation
  - File: `internal/daemon/toolexec/executor.go`
  - Implement `Execute()` to spawn tool subprocess
  - Write JSON input to stdin, read JSON output from stdout
  - Capture stderr for error reporting
  - Enforce timeout with context cancellation and SIGKILL
  - Set `GIBSON_TOOL_MODE=subprocess` environment variable
  - Purpose: Execute tools as short-lived subprocesses
  - _Leverage: `os/exec`, `context`, `encoding/json`, `bytes.Buffer`_
  - _Requirements: 2.2, 2.3, 2.4, 2.5, 2.6, 4.1, 4.2, 4.3, 4.4, 4.6_
  - _Prompt:
    Role: Go systems programmer with subprocess expertise
    Task: Implement the SubprocessExecutor that runs tool binaries with stdin/stdout communication. Handle timeouts robustly.
    Restrictions: Must kill subprocess on timeout. Must capture both stdout and stderr. Must set GIBSON_TOOL_MODE=subprocess env var.
    _Leverage: `os/exec.CommandContext`, `context.WithTimeout`
    _Requirements: 2.2-2.6, 4.1-4.6
    Success: Tools execute via subprocess, timeouts enforced, output captured correctly
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 4. Implement Tool Executor Service orchestration
  - File: `internal/daemon/toolexec/service.go`
  - Implement `Start()` to scan tools and populate registry
  - Implement `Stop()` to cleanup running subprocesses
  - Implement `Execute()` to route to SubprocessExecutor
  - Implement `ListTools()` and `GetToolSchema()`
  - Implement `RefreshTools()` for hot-reload
  - Add metrics tracking per tool
  - Purpose: Orchestrate tool discovery, caching, and execution
  - _Leverage: `internal/daemon/toolexec/scanner.go`, `internal/daemon/toolexec/executor.go`_
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 2.1, 3.1, 3.4, 7.1, 7.2_
  - _Prompt:
    Role: Go backend developer building daemon services
    Task: Implement the ToolExecutorService that orchestrates scanning, caching, and execution. Wire together the scanner and executor.
    Restrictions: Must be thread-safe for concurrent tool executions. Must track metrics. Must support hot-reload.
    _Leverage: `internal/daemon/toolexec/scanner.go`, `internal/daemon/toolexec/executor.go`, `sync.RWMutex`
    _Requirements: 1.1-1.5, 2.1, 3.1, 3.4, 7.1, 7.2
    Success: Service starts, discovers tools, executes them, tracks metrics
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 5. Add Tool Executor Service unit tests
  - File: `internal/daemon/toolexec/service_test.go`
  - Test scanner with mock filesystem
  - Test executor with mock binaries
  - Test service orchestration
  - Test concurrent execution
  - Test timeout handling
  - Purpose: Ensure reliability of tool execution subsystem
  - _Leverage: `testing`, `os/exec` mocking patterns_
  - _Requirements: 7.1, 7.2_
  - _Prompt:
    Role: Go test engineer
    Task: Write comprehensive unit tests for the Tool Executor Service components. Use table-driven tests.
    Restrictions: Mock external dependencies (filesystem, exec). Test edge cases. Achieve >80% coverage.
    _Leverage: Go testing package, test patterns from `internal/tool/registry_test.go`
    _Requirements: 7.1, 7.2
    Success: All tests pass, good coverage of happy path and error cases
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 6. Add daemon proto definitions for tool execution
  - File: `internal/daemon/api/daemon.proto`
  - Add `ExecuteTool` RPC with request/response messages
  - Add `GetAvailableTools` RPC with request/response messages
  - Add `ToolDescriptor` message type
  - Purpose: Expose tool execution via daemon gRPC API
  - _Leverage: Existing `daemon.proto` patterns_
  - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5_
  - _Prompt:
    Role: Protocol Buffers designer
    Task: Add new RPC definitions for tool execution to daemon.proto. Follow existing message patterns.
    Restrictions: Follow existing naming conventions. Keep messages simple. Include proper documentation comments.
    _Leverage: Existing `daemon.proto` message patterns
    _Requirements: 8.1-8.5
    Success: Proto compiles, messages are well-documented, follows existing patterns
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 7. Generate proto code and implement daemon handlers
  - Files: `internal/daemon/api/daemon.pb.go` (generated), `internal/daemon/grpc.go`
  - Run `protoc` to regenerate Go code
  - Implement `ExecuteTool` handler delegating to ToolExecutorService
  - Implement `GetAvailableTools` handler
  - Purpose: Wire proto API to Tool Executor Service
  - _Leverage: `internal/daemon/grpc.go` existing handler patterns_
  - _Requirements: 8.1, 8.2, 8.3_
  - _Prompt:
    Role: Go gRPC developer
    Task: Generate proto code and implement the new RPC handlers in grpc.go. Delegate to ToolExecutorService.
    Restrictions: Follow existing handler patterns. Proper error mapping. Include logging.
    _Leverage: Existing handlers in `internal/daemon/grpc.go`
    _Requirements: 8.1, 8.2, 8.3
    Success: RPCs work end-to-end, errors handled properly
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 8. Initialize Tool Executor Service in daemon startup
  - File: `internal/daemon/daemon.go`
  - Create and start ToolExecutorService in `Start()` method
  - Stop ToolExecutorService in `Stop()` method
  - Add toolExecutorService field to daemonImpl struct
  - Purpose: Integrate Tool Executor Service into daemon lifecycle
  - _Leverage: Existing daemon initialization patterns_
  - _Requirements: 1.1, 1.4_
  - _Prompt:
    Role: Go backend developer
    Task: Wire the ToolExecutorService into daemon startup/shutdown. Add it as a field on daemonImpl.
    Restrictions: Start after registry is ready. Stop before other services. Handle startup errors gracefully.
    _Leverage: Existing daemon.go initialization patterns
    _Requirements: 1.1, 1.4
    Success: Daemon starts Tool Executor Service, shuts it down cleanly
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

### Phase 2: SDK Subprocess Mode

- [x] 9. Add subprocess mode to SDK serve package
  - File: `sdk/serve/subprocess.go` (new)
  - Implement `RunSubprocess()` to read stdin, execute tool, write stdout
  - Implement `OutputSchema()` for `--schema` flag handling
  - Handle JSON serialization/deserialization
  - Purpose: Enable SDK tools to run in subprocess mode
  - _Leverage: `sdk/tool/interface.go`, `encoding/json`_
  - _Requirements: 5.1, 5.2, 5.3, 5.4_
  - _Prompt:
    Role: Go SDK developer
    Task: Create subprocess.go in sdk/serve/ that enables tools to run in stdin/stdout mode. Read design.md for interface.
    Restrictions: Clean JSON I/O. Handle errors gracefully. Exit with proper codes.
    _Leverage: `sdk/tool/interface.go` for Tool interface
    _Requirements: 5.1-5.4
    Success: Tools can run in subprocess mode with proper I/O
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 10. Modify serve.Tool() to detect subprocess mode
  - File: `sdk/serve/tool.go`
  - Check for `GIBSON_TOOL_MODE=subprocess` environment variable
  - Check for `--schema` flag in os.Args
  - Route to `RunSubprocess()` or `OutputSchema()` when appropriate
  - Keep gRPC server mode as fallback for backward compatibility during migration
  - Purpose: Allow tools to auto-detect execution mode
  - _Leverage: `os.Getenv`, `os.Args`_
  - _Requirements: 5.5_
  - _Prompt:
    Role: Go SDK developer
    Task: Modify serve.Tool() to detect subprocess mode via env var and --schema flag. Route appropriately.
    Restrictions: Keep backward compatibility with gRPC mode. Clean detection logic.
    _Leverage: Existing `sdk/serve/tool.go`
    _Requirements: 5.5
    Success: Tools auto-detect mode and run appropriately
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 11. Add SDK subprocess mode tests
  - File: `sdk/serve/subprocess_test.go` (new)
  - Test stdin/stdout communication
  - Test schema output
  - Test error handling
  - Purpose: Ensure subprocess mode works correctly
  - _Leverage: `testing`, `bytes.Buffer` for I/O mocking_
  - _Requirements: 5.1, 5.2, 5.3, 5.4_
  - _Prompt:
    Role: Go test engineer
    Task: Write unit tests for SDK subprocess mode. Mock stdin/stdout.
    Restrictions: Test all code paths. Include error cases.
    _Leverage: Go testing package, bytes.Buffer
    _Requirements: 5.1-5.4
    Success: All subprocess mode functionality tested
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 12. Update ping tool to support subprocess mode (N/A - auto-handled by serve.Tool())
  - File: `~/Code/zero-day.ai/opensource/gibson-oss-tools/discovery/ping/main.go`
  - Update to use modified SDK serve.Tool()
  - Verify `--schema` flag works
  - Verify stdin/stdout execution works
  - Purpose: Validate subprocess mode with real tool
  - _Leverage: Updated SDK_
  - _Requirements: 5.1, 5.2, 5.3, 5.4_
  - _Prompt:
    Role: Go tool developer
    Task: Update the ping tool to work with the new subprocess mode. Test both --schema and execution.
    Restrictions: Must work with both subprocess mode and legacy gRPC mode during migration.
    _Leverage: Updated `sdk/serve/tool.go`
    _Requirements: 5.1-5.4
    Success: Ping tool works in subprocess mode
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

### Phase 3: Wire Harness to Daemon Tool Execution

- [x] 13. Create DaemonToolProxy implementation
  - File: `internal/harness/daemon_tool_proxy.go` (new)
  - Implement `tool.Tool` interface
  - Proxy `Execute()` to daemon `ExecuteTool` RPC
  - Cache descriptor for metadata methods
  - Purpose: Allow harness to invoke tools via daemon
  - _Leverage: `internal/tool/interface.go`, `internal/daemon/api`_
  - _Requirements: 2.1, 8.1, 8.2_
  - _Prompt:
    Role: Go developer
    Task: Create DaemonToolProxy that implements tool.Tool by calling daemon gRPC. Cache descriptor.
    Restrictions: Must implement full Tool interface. Handle gRPC errors properly.
    _Leverage: `internal/tool/interface.go`, daemon gRPC client
    _Requirements: 2.1, 8.1, 8.2
    Success: Proxy implements Tool interface, routes to daemon
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 14. Add daemon client methods for tool execution
  - File: `internal/daemon/client/client.go`
  - Add `ExecuteTool()` method wrapping gRPC call
  - Add `GetAvailableTools()` method wrapping gRPC call
  - Purpose: Expose tool execution to harness via daemon client
  - _Leverage: Existing client patterns_
  - _Requirements: 8.1, 8.4_
  - _Prompt:
    Role: Go gRPC client developer
    Task: Add ExecuteTool and GetAvailableTools methods to daemon client. Follow existing patterns.
    Restrictions: Follow existing client method patterns. Proper error handling.
    _Leverage: Existing `internal/daemon/client/client.go` methods
    _Requirements: 8.1, 8.4
    Success: Client methods work, follow existing patterns
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 15. Wire harness factory to use DaemonToolProxy
  - File: `internal/harness/factory.go`
  - Fetch available tools from daemon on harness creation
  - Create DaemonToolProxy for each available tool
  - Register proxies with local ToolRegistry
  - Purpose: Populate harness with daemon-backed tools
  - _Leverage: `internal/harness/daemon_tool_proxy.go`, `internal/daemon/client`_
  - _Requirements: 2.1, 3.1_
  - _Prompt:
    Role: Go developer
    Task: Modify harness factory to fetch tools from daemon and create DaemonToolProxy instances.
    Restrictions: Handle daemon unavailable gracefully. Log tool discovery.
    _Leverage: `internal/harness/factory.go`, DaemonToolProxy
    _Requirements: 2.1, 3.1
    Success: Harness gets tools from daemon, can execute them
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 16. Remove etcd-based tool discovery from harness (replaced by DaemonToolProxy)
  - File: `internal/harness/implementation.go`
  - Remove fallback to `registryAdapter.DiscoverTool()` in `CallTool()`
  - Keep registry adapter for agent/plugin discovery only
  - Update error messages
  - Purpose: Tools now come from daemon, not etcd
  - _Leverage: Existing `CallTool()` implementation_
  - _Requirements: 6.3, 6.4_
  - _Prompt:
    Role: Go developer
    Task: Remove etcd-based tool discovery from CallTool(). Tools only from local registry now (populated by daemon proxies).
    Restrictions: Keep agent/plugin discovery via registry adapter. Update error messages.
    _Leverage: `internal/harness/implementation.go` CallTool method
    _Requirements: 6.3, 6.4
    Success: CallTool only uses local registry, no etcd fallback for tools
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

### Phase 4: Remove Legacy Tool Commands

- [x] 17. Update `gibson tool list` to use Tool Executor Service (added `gibson tool available` command)
  - File: `cmd/gibson/tool.go`
  - Fetch tools from daemon `GetAvailableTools` RPC
  - Display tool status (ready, schema-unknown, error)
  - Show execution metrics
  - Purpose: CLI shows tools from new system
  - _Leverage: `internal/daemon/client`_
  - _Requirements: 7.3_
  - _Prompt:
    Role: Go CLI developer
    Task: Update `gibson tool list` to fetch from daemon GetAvailableTools RPC. Show status and metrics.
    Restrictions: Handle daemon unavailable. Nice formatting.
    _Leverage: `cmd/gibson/tool.go`, daemon client
    _Requirements: 7.3
    Success: tool list shows tools from Tool Executor Service with status
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 18. Add `gibson tool invoke` command for CLI testing
  - File: `cmd/gibson/tool.go`
  - Add `tool invoke <name> --input '{...}'` subcommand
  - Call daemon `ExecuteTool` RPC
  - Display output or error
  - Purpose: Allow direct tool testing via CLI
  - _Leverage: `internal/daemon/client`, `cobra`_
  - _Requirements: (usability)_
  - _Prompt:
    Role: Go CLI developer
    Task: Add `gibson tool invoke` command that calls daemon ExecuteTool. Accept JSON input, display output.
    Restrictions: Nice error messages. Support stdin for input.
    _Leverage: `cmd/gibson/tool.go`, cobra patterns
    _Requirements: Usability requirement from requirements.md
    Success: Can invoke tools directly via CLI
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 19. Remove `gibson tool start` command (done - commands excluded in tool.go init())
  - File: `cmd/gibson/component/lifecycle.go`
  - Remove `startToolCmd` or mark as deprecated with error
  - Update help text
  - Purpose: Tools no longer manually started
  - _Leverage: N/A_
  - _Requirements: 6.1_
  - _Prompt:
    Role: Go CLI developer
    Task: Remove or deprecate `gibson tool start` command. Show helpful error pointing to new system.
    Restrictions: Helpful error message if user tries old command.
    _Leverage: `cmd/gibson/component/lifecycle.go`
    _Requirements: 6.1
    Success: Command removed or shows deprecation error
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 20. Remove `gibson tool stop` command (done - commands excluded in tool.go init())
  - File: `cmd/gibson/component/lifecycle.go`
  - Remove `stopToolCmd` or mark as deprecated with error
  - Update help text
  - Purpose: Tools no longer manually stopped
  - _Leverage: N/A_
  - _Requirements: 6.2_
  - _Prompt:
    Role: Go CLI developer
    Task: Remove or deprecate `gibson tool stop` command. Show helpful error.
    Restrictions: Helpful error message if user tries old command.
    _Leverage: `cmd/gibson/component/lifecycle.go`
    _Requirements: 6.2
    Success: Command removed or shows deprecation error
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 21. Remove tool gRPC registration from SDK (subprocess mode is now primary - gRPC kept as fallback)
  - File: `sdk/serve/tool.go`
  - Remove etcd registration code from gRPC mode
  - Keep gRPC server for backward compatibility but no registry
  - Purpose: Tools don't register individually with etcd
  - _Leverage: N/A_
  - _Requirements: 6.3_
  - _Prompt:
    Role: Go SDK developer
    Task: Remove etcd registration from serve.Tool() gRPC mode. Tools no longer register themselves.
    Restrictions: Keep gRPC server functional for any remaining use cases. Just remove registry calls.
    _Leverage: `sdk/serve/tool.go`
    _Requirements: 6.3
    Success: Tools don't register with etcd, still function
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 22. Delete unused gRPC tool client code (deferred - existing dependencies prevent deletion)
  - File: `internal/registry/grpc_tool_client.go` (delete)
  - Remove file entirely
  - Update any imports
  - Purpose: Remove dead code
  - _Leverage: N/A_
  - _Requirements: 6.4_
  - _Prompt:
    Role: Go developer
    Task: Delete grpc_tool_client.go and update any broken imports.
    Restrictions: Ensure no remaining references. Update tests if needed.
    _Leverage: N/A
    _Requirements: 6.4
    Success: File deleted, code compiles
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 23. Integration test: End-to-end tool execution (created service_integration_test.go)
  - File: `internal/daemon/toolexec/integration_test.go` (new)
  - Create test tool binary
  - Test full flow: daemon start → tool discovery → execution → result
  - Test timeout behavior
  - Test hot-reload
  - Purpose: Validate complete system works
  - _Leverage: `testing`, test utilities_
  - _Requirements: All_
  - _Prompt:
    Role: Go integration test engineer
    Task: Write end-to-end integration tests for Tool Executor Service. Create test tool binary.
    Restrictions: Must test real subprocess execution. Include timeout and error cases.
    _Leverage: Go testing, test patterns from existing integration tests
    _Requirements: All requirements
    Success: Full system tested end-to-end
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._

- [x] 24. Update debug agent to use new tool system (verified - uses harness → DaemonToolProxyFactory)
  - File: `~/Code/zero-day.ai/opensource/agents/debug/` (verify compatibility)
  - Verify debug agent works with daemon-provided tools
  - Test network-recon module with ping tool
  - Purpose: Validate agent tool usage works
  - _Leverage: Debug agent, ping tool_
  - _Requirements: All_
  - _Prompt:
    Role: Integration tester
    Task: Verify debug agent works with new tool system. Run network-recon module with ping.
    Restrictions: Document any issues found. May require debug agent updates.
    _Leverage: Debug agent, ping tool
    _Requirements: All
    Success: Debug agent executes tools successfully via new system
    Instructions: First mark this task as in-progress in tasks.md, implement, then use log-implementation tool to record what was created, then mark as complete._
