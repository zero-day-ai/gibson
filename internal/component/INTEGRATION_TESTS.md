# Integration Tests for Local Component Lifecycle

This document describes the integration tests for the local component lifecycle system.

## Overview

The integration tests in `integration_test.go` provide end-to-end validation of the local component lifecycle management, including:

- Component start/stop with PID, lock, and socket tracking
- Crash recovery and stale state cleanup
- Duplicate start prevention via file locks
- Component discovery across different kinds
- Concurrent operations and thread safety

## Running the Tests

### Prerequisites

- Go 1.21+ with CGO enabled
- Linux/Unix system (tests use Unix domain sockets)

### Run All Integration Tests

```bash
cd /home/anthony/Code/zero-day.ai/opensource/gibson
go test -tags integration,fts5 -v ./internal/component/
```

### Run Specific Test

```bash
go test -tags integration,fts5 -v -run TestLocalLifecycleFullStartStop ./internal/component/
```

### Run with Timeout

```bash
go test -tags integration,fts5 -v -timeout 5m ./internal/component/
```

### Skip Long-Running Tests

```bash
go test -tags integration,fts5 -v -short ./internal/component/
```

This skips the concurrent operations test which may take longer.

## Test Coverage

### TestLocalLifecycleFullStartStop
Tests the complete lifecycle of starting and stopping a component with proper PID file, lock file, and Unix socket management.

**Validates:**
- PID file creation and content
- Lock acquisition and release
- Socket file creation
- Component running status detection

### TestLocalLifecycleCrashRecovery
Tests cleanup of stale state when a component's files are left behind after a crash.

**Validates:**
- Stale PID file detection and removal
- Dead process cleanup
- Lock file cleanup
- Socket file cleanup

### TestLocalLifecycleDuplicateStartPrevention
Tests that the file lock mechanism prevents duplicate starts of the same component.

**Validates:**
- Exclusive lock enforcement
- ComponentAlreadyRunningError
- Multi-tracker conflict resolution

### TestLocalLifecycleDiscoveryFindsRunningAgents
Tests that the discovery mechanism correctly finds all running components.

**Validates:**
- Multiple component discovery
- Real-time discovery updates
- Component health validation
- Dynamic state changes

### TestLocalLifecycleDiscoveryMixedHealthStates
Tests discovery with a mix of healthy and unhealthy components.

**Validates:**
- Dead process filtering
- Missing socket detection
- Automatic cleanup of unhealthy components

### TestLocalLifecycleDiscoveryAcrossKinds
Tests that discovery correctly filters by component kind (agent, tool, plugin).

**Validates:**
- Agent-specific discovery
- Tool-specific discovery
- Plugin-specific discovery
- Kind-based filtering

### TestLocalLifecycleConcurrentStartStop
Tests thread safety of concurrent start/stop operations.

**Validates:**
- Concurrent start/stop operations
- Lock contention handling
- Race condition prevention
- Internal state consistency

### TestLocalLifecycleStaleSocketCleanup
Tests cleanup of leftover socket files from crashed components.

**Validates:**
- Stale socket detection
- Socket file removal
- Connection validation

### TestLocalLifecycleHealthySocketNotRemoved
Tests that healthy, active sockets are not removed during cleanup.

**Validates:**
- Health check validation
- Socket protection
- Error handling for healthy components

## Test Helpers

### `startTestServer(t *testing.T, socketPath string) *grpc.Server`

Starts a gRPC server with health check on a Unix socket.

**Parameters:**
- `t`: Testing instance
- `socketPath`: Absolute path to Unix socket

**Returns:**
- `*grpc.Server`: Server instance for cleanup

**Features:**
- Automatic socket cleanup
- Health service registration
- Background serving
- Socket creation validation

### `killProcessByPID(pid int) error`

Sends signals to a process to terminate it (SIGTERM then SIGKILL).

**Parameters:**
- `pid`: Process ID to terminate

**Returns:**
- `error`: Error if process termination fails

**Features:**
- Graceful shutdown attempt (SIGTERM)
- Forced termination (SIGKILL)
- Wait period between signals

## Implementation Notes

### Build Tags

All integration tests use `//go:build integration` to separate them from unit tests.

### Test Isolation

Each test uses `setupTestEnv(t)` which:
- Creates a temporary HOME directory
- Initializes run directories for all component kinds
- Returns a cleanup function
- Ensures tests don't interfere with each other

### Process Simulation

Tests use goroutines with gRPC servers instead of spawning actual subprocesses because:
- Faster execution
- Better portability across platforms
- Easier cleanup and error handling
- Simplified test infrastructure

**Note:** In production, components are actual subprocesses with separate PIDs.

### CI/CD Compatibility

Tests are designed to work in CI environments:
- Use temporary directories (no system-wide pollution)
- Support `-short` flag to skip long-running tests
- Proper cleanup even on test failure
- No external dependencies beyond Go and CGO

## Troubleshooting

### Tests Fail with "Socket already exists"

Cleanup from a previous test run may have failed. Manually remove sockets:

```bash
rm -rf ~/.gibson-test-*/run
```

### Tests Fail with "Cannot acquire lock"

Another process or test may hold the lock. Check for stuck test processes:

```bash
ps aux | grep integration_test
kill -9 <PID>
```

### Tests Timeout

Increase the timeout value:

```bash
go test -tags integration,fts5 -v -timeout 10m ./internal/component/
```

## Future Enhancements

- Add tests with actual subprocess spawning
- Add tests for network failures
- Add tests for filesystem permission errors
- Add benchmarks for discovery performance
- Add tests for very large numbers of components (100+)
