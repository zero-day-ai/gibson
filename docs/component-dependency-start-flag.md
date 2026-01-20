# Component Dependency `--start-dependencies` Flag Implementation

## Overview

Implemented Task 11 from the component-dependency-resolution spec: Added `--start-dependencies` flag to the `gibson mission run` command to automatically start stopped component dependencies before mission execution.

## Changes Made

### 1. Mission Command Flag (`cmd/gibson/mission.go`)

Added new flag to the `mission run` command:
- `--start-dependencies`: Boolean flag (default: false) to automatically start stopped component dependencies

### 2. Dependency Checking Logic

Implemented `checkAndStartDependencies()` function that:
- Parses mission definition to extract all component dependencies (agents, tools, plugins)
- Extracts dependencies from mission nodes (workflow steps)
- Extracts explicit dependencies from mission definition
- Queries component status from daemon via gRPC:
  - `ListAgents()` - get all registered agents
  - `ListTools()` - get all registered tools
  - `ListPlugins()` - get all registered plugins
- Categorizes components into:
  - **Not installed** - fatal error, mission cannot run
  - **Not running** - handled based on flag
- Auto-starts stopped dependencies if `--start-dependencies` is set
- Prints warning and suggested commands if flag is not set

### 3. Mission Definition Adapter (`internal/mission/resolver_adapter.go`)

Created adapter to bridge mission and resolver packages without circular dependencies:
- `missionDefinitionAdapter` - implements `resolver.MissionDefinition` interface
- `missionNodeAdapter` - implements `resolver.MissionNode` interface
- `missionDependencyAdapter` - implements `resolver.MissionDependency` interface

The adapter provides methods:
- `Nodes()` - returns all mission nodes
- `Dependencies()` - returns explicit dependencies from YAML
- `ID()`, `Type()`, `ComponentRef()` - node information

### 4. Schema Taxonomy Helper (`internal/harness/callback_service.go`)

Added missing helper function `hasTaxonomyMappings()` that recursively checks if a schema contains taxonomy mappings. This was causing build errors due to incomplete implementation from a previous change.

## Usage Examples

### Without `--start-dependencies` (Warning Only)

```bash
$ gibson mission run my-mission.yaml

Warning: The following components are installed but not running:
  - nmap-tool (tool)
  - vuln-scanner (agent)

Use --start-dependencies to start them automatically, or run:
  gibson tool start nmap-tool
  gibson agent start vuln-scanner

Proceeding with mission execution anyway...
```

### With `--start-dependencies` (Auto-start)

```bash
$ gibson mission run my-mission.yaml --start-dependencies

Starting stopped dependencies:
  - nmap-tool (tool): started
  - vuln-scanner (agent): started
All dependencies running. Executing mission...
```

### Missing Dependencies (Fatal Error)

```bash
$ gibson mission run my-mission.yaml

Error: The following required components are not installed:
  - missing-agent (agent)

Please install missing components before running the mission.
```

## Implementation Details

### Dependency Resolution Flow

1. **Parse Mission Definition**
   - Read workflow YAML
   - Extract component references from nodes
   - Extract explicit dependencies

2. **Query Component Status**
   - Call daemon gRPC methods: `ListAgents()`, `ListTools()`, `ListPlugins()`
   - Build maps of component status

3. **Categorize Components**
   - Not installed → Fatal error
   - Installed but not running → Needs attention

4. **Handle Stopped Components**
   - If `--start-dependencies`:
     - Start each stopped component
     - Abort on start failure
   - If not set:
     - Print warning
     - Show command suggestions
     - Continue anyway

### Component Start Methods

Uses daemon client methods to start components:
- `client.StartAgent(ctx, name)` - start agent component
- `client.StartTool(ctx, name)` - start tool component
- `client.StartPlugin(ctx, name)` - start plugin component

### Error Handling

- **Not installed components**: Immediate failure with error message
- **Start failures**: Abort mission with specific error
- **Daemon unavailable**: Propagate connection error
- **Parse errors**: Return parsing error

## Testing

Created comprehensive tests for the mission definition adapter:
- `TestMissionDefinitionAdapter` - verifies adapter methods
- `TestNodeAdapterComponentRef` - tests component reference extraction
- `TestDependencyAdapter` - validates dependency adapter

All tests pass:
```
=== RUN   TestMissionDefinitionAdapter
--- PASS: TestMissionDefinitionAdapter (0.00s)
=== RUN   TestNodeAdapterComponentRef
--- PASS: TestNodeAdapterComponentRef (0.00s)
=== RUN   TestDependencyAdapter
--- PASS: TestDependencyAdapter (0.00s)
PASS
```

## Future Enhancements

While the current implementation works with the daemon client, future improvements could include:

1. **Topological Ordering**: Start dependencies in dependency order (requires full resolver integration)
2. **Health Checking**: Wait for components to pass health checks after starting
3. **Timeout Configuration**: Add configurable timeout for component startup
4. **Dry-run Mode**: Add `--dry-run` to show what would be started without actually starting
5. **Parallel Starting**: Start independent components in parallel for faster startup

## Files Modified

- `cmd/gibson/mission.go` - added flag and dependency checking logic
- `internal/mission/resolver_adapter.go` - created adapter for resolver integration (NEW)
- `internal/mission/resolver_adapter_test.go` - adapter tests (NEW)
- `internal/harness/callback_service.go` - added missing `hasTaxonomyMappings()` helper

## Related Specs

- Component Dependency Resolution Spec - Task 11
- Located at: `~/Code/zero-day.ai/.spec-workflow/component-dependency-resolution/`
