# Task 13: gRPC Proto Definitions for Dependency Resolver Service

## Summary

Added Protocol Buffer definitions for the dependency resolver service to `daemon.proto`. The definitions enable gRPC communication between the Gibson CLI and daemon for mission dependency resolution, validation, and automatic startup.

## Changes Made

### 1. New RPC Methods (Added to DaemonService)

```protobuf
// ResolveMissionDependencies resolves the complete dependency tree for a mission.
// Walks all transitive dependencies from agents, tools, and plugins.
rpc ResolveMissionDependencies(ResolveMissionDependenciesRequest) returns (ResolveMissionDependenciesResponse);

// ValidateMissionDependencies validates that all mission dependencies are installed and running.
// Returns detailed validation results with component status and any issues found.
rpc ValidateMissionDependencies(ValidateMissionDependenciesRequest) returns (ValidateMissionDependenciesResponse);

// EnsureDependenciesRunning starts any stopped dependencies in the correct topological order.
// Ensures all components required by a mission are running before execution.
rpc EnsureDependenciesRunning(EnsureDependenciesRunningRequest) returns (EnsureDependenciesRunningResponse);
```

### 2. Request/Response Messages

#### ResolveMissionDependencies
- `ResolveMissionDependenciesRequest` - Takes `workflow_path`
- `ResolveMissionDependenciesResponse` - Returns `DependencyTree` with success status

#### ValidateMissionDependencies
- `ValidateMissionDependenciesRequest` - Takes `workflow_path`
- `ValidateMissionDependenciesResponse` - Returns `ValidationResult` with detailed status

#### EnsureDependenciesRunning
- `EnsureDependenciesRunningRequest` - Takes `workflow_path` and optional `timeout_ms`
- `EnsureDependenciesRunningResponse` - Returns counts of started/skipped components

### 3. Core Data Structures

#### DependencyTree
Represents the complete resolved dependency graph:
- `roots` - Root nodes (direct mission dependencies)
- `nodes` - Map of component keys to dependency nodes
- `agents`, `tools`, `plugins` - Flattened lists by kind
- `resolved_at` - Resolution timestamp
- `mission_ref` - Mission name/path

#### DependencyNode
Represents a single component in the tree:
- Component identification: `kind`, `name`, `version`
- Relationships: `required_by`, `depends_on` (string keys)
- Source tracking: `source` (enum), `source_ref`
- State fields: `installed`, `running`, `healthy`, `actual_version`

#### DependencySource (enum)
Indicates where a dependency was discovered:
- `SOURCE_UNKNOWN` (0) - Default
- `SOURCE_MISSION_EXPLICIT` (1) - Listed in mission dependencies
- `SOURCE_MISSION_NODE` (2) - Referenced by a mission node
- `SOURCE_MANIFEST` (3) - From component's manifest dependencies

#### ValidationResult
Contains validation outcome:
- Overall status: `valid`, `summary`
- Counts: `total_components`, `installed_count`, `running_count`, `healthy_count`
- Problem lists: `not_installed`, `not_running`, `unhealthy`, `version_mismatches`
- Timing: `validated_at`, `duration_ms`

#### VersionMismatchInfo
Describes version constraint violations:
- `node` - Component with mismatch
- `required_version` - Version constraint required
- `actual_version` - Version actually installed

## Design Decisions

### 1. Relationship Storage Format
DependencyNode stores relationships as `repeated string` (component keys like "agent:vuln-scanner") instead of nested DependencyNode objects to avoid:
- Circular reference issues in proto3
- Massive message duplication (same node repeated in multiple places)
- Serialization complexity

Clients can resolve keys using the `DependencyTree.nodes` map.

### 2. Proto3 Enum Naming
Following proto3 conventions:
- Enum values prefixed with type name: `SOURCE_MISSION_EXPLICIT`
- Zero value is always `_UNKNOWN` for safety
- Matches existing enums in daemon.proto

### 3. Timestamp Representation
Using `int64` Unix timestamps (milliseconds) instead of `google.protobuf.Timestamp`:
- Consistent with existing daemon.proto patterns
- Simpler to work with in Go code
- No additional proto dependencies

### 4. Optional vs Required Fields
Proto3 doesn't have required fields, so validation happens in Go code:
- `workflow_path` is logically required for all requests
- Empty/missing values handled in gRPC server implementation
- Error messages guide users to provide required fields

## Files Modified

1. **`internal/daemon/api/daemon.proto`** - Added proto definitions (197 new lines)
   - 3 new RPC methods
   - 6 request/response message pairs
   - 5 core data structure messages
   - 1 enum definition

2. **`internal/daemon/api/README.md`** (NEW) - Documentation for proto regeneration
   - Regeneration instructions
   - File descriptions
   - Recent changes summary

## Proto Regeneration Command

To regenerate Go code after proto changes:

```bash
cd /home/anthony/Code/zero-day.ai/opensource/gibson

protoc --proto_path=internal/daemon/api \
    --go_out=internal/daemon/api --go_opt=paths=source_relative \
    --go-grpc_out=internal/daemon/api --go-grpc_opt=paths=source_relative \
    internal/daemon/api/daemon.proto
```

**Note:** Do NOT run this yet - wait for approval and coordinate with other tasks that depend on these types.

## Integration Points

These proto definitions are designed to match the Go structs defined in:
- `internal/component/resolver/tree.go` (DependencyTree, DependencyNode)
- `internal/component/resolver/validation.go` (ValidationResult, VersionMismatchInfo)

The gRPC service implementation will:
1. Call `DependencyResolver.ResolveFromWorkflow()` for resolve operations
2. Call `DependencyResolver.ValidateState()` for validation operations
3. Call `DependencyResolver.EnsureRunning()` for startup operations
4. Convert between Go structs and proto messages using conversion functions

## Next Steps

1. **Task 12**: Implement gRPC handlers in `internal/daemon/daemon.go` that:
   - Create DependencyResolver instance
   - Wire to ComponentStore and LifecycleManager
   - Implement proto service methods
   - Convert between Go types and proto messages

2. **Task 9-10**: Implement CLI commands that:
   - Connect to daemon gRPC service
   - Call resolver methods via gRPC
   - Format and display results

3. **Proto Generation**: After tasks 1-8 implement the Go types, regenerate proto to ensure compatibility

## Testing Notes

After proto regeneration, verify:
- Generated code compiles without errors
- Message field numbers don't conflict with existing messages
- Enum values are unique within their scope
- gRPC service definition is valid

Integration tests will verify:
- Proto messages serialize/deserialize correctly
- Conversion between Go structs and proto messages preserves data
- gRPC methods can be called from CLI clients
