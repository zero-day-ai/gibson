# Daemon API - gRPC Protocol Buffers

This directory contains the gRPC API definitions for the Gibson daemon.

## Files

- `daemon.proto` - Protocol buffer definitions for the daemon gRPC service
- `daemon.pb.go` - Generated Go types (DO NOT EDIT)
- `daemon_grpc.pb.go` - Generated gRPC service code (DO NOT EDIT)
- `server.go` - Implementation of the gRPC service
- `schema_convert.go` - Utilities for converting between proto and SDK types

## Regenerating Proto Files

After modifying `daemon.proto`, regenerate the Go code:

```bash
# From the gibson root directory:
cd /home/anthony/Code/zero-day.ai/opensource/gibson

# Install protoc plugins (if not already installed):
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Generate Go code from proto file:
protoc --proto_path=internal/daemon/api \
    --go_out=internal/daemon/api --go_opt=paths=source_relative \
    --go-grpc_out=internal/daemon/api --go-grpc_opt=paths=source_relative \
    internal/daemon/api/daemon.proto
```

**Note:** The Makefile `make proto` target expects proto files in `api/proto/` but the daemon proto is in `internal/daemon/api/`. Use the commands above for daemon proto regeneration.

## Recent Changes

### Dependency Resolution (Component Dependency Resolution Spec)

Added gRPC methods for mission dependency resolution and validation:

**New RPC Methods:**
- `ResolveMissionDependencies` - Resolves the complete dependency tree for a mission
- `ValidateMissionDependencies` - Validates that all dependencies are installed and running
- `EnsureDependenciesRunning` - Starts any stopped dependencies in topological order

**New Message Types:**
- `DependencyTree` - Represents the complete dependency graph
- `DependencyNode` - Individual component in the dependency tree
- `DependencySource` - Enum indicating where a dependency was discovered
- `ValidationResult` - Validation status and problem reports
- `VersionMismatchInfo` - Version constraint violation details

These additions support the new `gibson mission validate` and `gibson mission plan` commands.
