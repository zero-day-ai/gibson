# Integration Tests for Component Lifecycle

This document describes the integration tests for the component lifecycle system with etcd registry.

## Overview

The integration tests in `integration_test.go` provide end-to-end validation of component lifecycle management with etcd-based discovery, including:

- Component start/stop with registry registration
- Health monitoring and automatic deregistration
- Multi-instance support and load balancing
- Component discovery via etcd registry
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

### TestRegistryLifecycleFullStartStop
Tests the complete lifecycle of starting and stopping a component with registry registration.

**Validates:**
- Component registration with etcd
- Metadata storage (name, version, endpoint, capabilities)
- Health check endpoints
- Graceful deregistration on stop

### TestRegistryLifecycleCrashRecovery
Tests automatic cleanup when components crash without deregistering.

**Validates:**
- TTL-based expiration of stale entries
- Automatic removal of unhealthy components
- Registry consistency after crashes

### TestRegistryLifecycleMultiInstance
Tests running multiple instances of the same component.

**Validates:**
- Multiple instance registration
- Instance-specific metadata
- Load balancing across instances
- Independent lifecycle management

### TestRegistryLifecycleDiscoveryFindsRunningAgents
Tests that the discovery mechanism correctly finds all registered components via etcd.

**Validates:**
- Registry-based component discovery
- Real-time discovery updates
- Health status validation
- Dynamic state changes

### TestRegistryLifecycleDiscoveryAcrossKinds
Tests that discovery correctly filters by component kind (agent, tool, plugin).

**Validates:**
- Agent-specific discovery
- Tool-specific discovery
- Plugin-specific discovery
- Namespace-based filtering

### TestRegistryLifecycleConcurrentStartStop
Tests thread safety of concurrent start/stop operations with registry.

**Validates:**
- Concurrent registration/deregistration
- etcd consistency under load
- Race condition prevention
- Registry state consistency

### TestRegistryLifecycleHealthMonitoring
Tests continuous health monitoring via TTL keepalive.

**Validates:**
- TTL-based keepalive mechanism
- Automatic expiration detection
- Health status tracking
- Reconnection handling

## Test Helpers

### `startTestRegistry(t *testing.T) (*registry.Client, func())`

Starts an embedded etcd instance for testing.

**Parameters:**
- `t`: Testing instance

**Returns:**
- `*registry.Client`: Registry client for test operations
- `func()`: Cleanup function to stop the registry

**Features:**
- Temporary data directory
- Automatic cleanup on test completion
- Isolated test environment

### `startComponentServer(t *testing.T, endpoint string, reg *registry.Client) *grpc.Server`

Starts a gRPC server with health check and registry registration.

**Parameters:**
- `t`: Testing instance
- `endpoint`: gRPC endpoint (e.g., "localhost:50051")
- `reg`: Registry client for self-registration

**Returns:**
- `*grpc.Server`: Server instance for cleanup

**Features:**
- Automatic registration on startup
- Health service implementation
- Graceful deregistration on shutdown
- Background serving

## Implementation Notes

### Build Tags

All integration tests use `//go:build integration` to separate them from unit tests.

### Test Isolation

Each test uses `setupTestEnv(t)` which:
- Creates a temporary HOME directory
- Initializes run directories for all component kinds
- Returns a cleanup function
- Ensures tests don't interfere with each other

### Component Simulation

Tests use in-process gRPC servers with registry integration instead of spawning actual subprocesses because:
- Faster execution
- Better portability across platforms
- Easier cleanup and error handling
- Simplified test infrastructure
- Full registry integration testing

**Note:** In production, components are actual processes that self-register with the etcd registry via the SDK.

### CI/CD Compatibility

Tests are designed to work in CI environments:
- Use temporary directories (no system-wide pollution)
- Support `-short` flag to skip long-running tests
- Proper cleanup even on test failure
- No external dependencies beyond Go and CGO

## Troubleshooting

### Tests Fail with "etcd: failed to start"

The embedded etcd instance may not have cleaned up properly. Manually remove data:

```bash
rm -rf ~/.gibson-test-*/etcd-data
```

### Tests Fail with "registry connection refused"

The test registry may not have started. Check for port conflicts:

```bash
lsof -i :2379
```

### Tests Timeout

Increase the timeout value (etcd startup can take a few seconds):

```bash
go test -tags integration,fts5 -v -timeout 10m ./internal/component/
```

## Future Enhancements

- Add tests with actual subprocess spawning and registry integration
- Add tests for etcd cluster failures and failover
- Add tests for network partition scenarios
- Add benchmarks for registry discovery performance at scale
- Add tests for very large numbers of components (1000+)
- Add tests for TLS-enabled registry connections
