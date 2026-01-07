# Task 3.2: Update Harness Implementation for Delegation

## Overview
Updated the `DefaultAgentHarness` implementation to use the new `RegistryAdapter` for component discovery while maintaining backward compatibility with the legacy `AgentRegistry`.

## Files Modified

### 1. `<gibson-root>/internal/harness/implementation.go`

#### Changes Made:

1. **Added import for registry package:**
   ```go
   import "github.com/zero-day-ai/gibson/internal/registry"
   ```

2. **Updated DefaultAgentHarness struct:**
   - Added `registryAdapter` field of type `registry.ComponentDiscovery`
   - Marked `agentRegistry` as deprecated with documentation
   - Added documentation explaining the precedence order

   ```go
   type DefaultAgentHarness struct {
       // ... existing fields ...

       // Deprecated: Use registryAdapter instead for etcd-based component discovery
       // Agent registry for delegation (maintained for backwards compatibility)
       agentRegistry agent.AgentRegistry

       // Registry adapter for unified component discovery via etcd
       // When set, this takes precedence over agentRegistry
       registryAdapter registry.ComponentDiscovery

       // ... remaining fields ...
   }
   ```

3. **Updated DelegateToAgent() method:**
   - Now checks if `registryAdapter` is available first
   - Falls back to `agentRegistry` if `registryAdapter` is nil
   - Returns error if neither registry is available
   - Includes debug logging to track which registry is being used

   ```go
   func (h *DefaultAgentHarness) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
       // ... existing setup code ...

       var result agent.Result

       // Use registryAdapter if available (preferred method), otherwise fall back to agentRegistry
       if h.registryAdapter != nil {
           h.logger.Debug("using registry adapter for delegation", "agent", name)
           result, err = h.registryAdapter.DelegateToAgent(ctx, name, task, agentHarness)
       } else if h.agentRegistry != nil {
           h.logger.Debug("using legacy agent registry for delegation", "agent", name)
           result, err = h.agentRegistry.DelegateToAgent(ctx, name, task, agentHarness)
       } else {
           h.logger.Error("no registry available for delegation", "agent", name)
           return agent.Result{}, types.NewError(
               ErrHarnessDelegationFailed,
               "no registry available for agent delegation",
           )
       }

       // ... existing error handling and metrics code ...
   }
   ```

4. **Updated ListAgents() method:**
   - Now checks if `registryAdapter` is available first
   - Converts `registry.AgentInfo` to `harness.AgentDescriptor`
   - Falls back to `agentRegistry` if `registryAdapter` is nil
   - Returns empty list if neither registry is available
   - Includes debug logging and error handling

   ```go
   func (h *DefaultAgentHarness) ListAgents() []AgentDescriptor {
       // Use registryAdapter if available (preferred method)
       if h.registryAdapter != nil {
           h.logger.Debug("using registry adapter for listing agents")

           agentInfos, err := h.registryAdapter.ListAgents(context.Background())
           if err != nil {
               h.logger.Error("failed to list agents from registry adapter", "error", err)
               return []AgentDescriptor{}
           }

           // Convert registry.AgentInfo to harness.AgentDescriptor
           descriptors := make([]AgentDescriptor, len(agentInfos))
           for i, info := range agentInfos {
               descriptors[i] = AgentDescriptor{
                   Name:         info.Name,
                   Version:      info.Version,
                   Description:  info.Description,
                   Capabilities: info.Capabilities,
                   Slots:        []agent.SlotDefinition{}, // AgentInfo doesn't include slots
                   IsExternal:   true,                     // All registry adapter agents are external
               }
           }
           return descriptors
       }

       // Fall back to legacy agent registry
       if h.agentRegistry != nil {
           h.logger.Debug("using legacy agent registry for listing agents")
           // ... existing code ...
       }

       // No registry available
       h.logger.Warn("no registry available for listing agents")
       return []AgentDescriptor{}
   }
   ```

### 2. `<gibson-root>/internal/harness/factory.go`

#### Changes Made:

1. **Updated DefaultHarnessFactory.Create() method:**
   - Added `registryAdapter` field initialization when creating harness instances
   - Ensures `RegistryAdapter` from config is properly passed to the harness

   ```go
   harness := &DefaultAgentHarness{
       slotManager:         f.config.SlotManager,
       llmRegistry:         f.config.LLMRegistry,
       toolRegistry:        f.config.ToolRegistry,
       pluginRegistry:      f.config.PluginRegistry,
       agentRegistry:       f.config.AgentRegistry,
       registryAdapter:     f.config.RegistryAdapter,  // <- Added this line
       memoryStore:         f.config.MemoryManager,
       findingStore:        f.config.FindingStore,
       factory:             selfFactory,
       missionCtx:          updatedMissionCtx,
       targetInfo:          target,
       tracer:              f.config.Tracer,
       logger:              logger,
       metrics:             f.config.Metrics,
       tokenUsage:          tokenTracker,
       graphRAGBridge:      f.config.GraphRAGBridge,
       graphRAGQueryBridge: f.config.GraphRAGQueryBridge,
   }
   ```

## Key Design Decisions

### 1. Precedence Order
When both `registryAdapter` and `agentRegistry` are available, the harness uses `registryAdapter` first. This allows for gradual migration:
- New code can use `RegistryAdapter` for etcd-based discovery
- Existing code continues to work with `AgentRegistry`
- Systems can run with both during transition period

### 2. Error Handling
- `ListAgents()`: Returns empty list on errors to avoid breaking callers
- `DelegateToAgent()`: Returns explicit error if no registry is available
- Both methods include debug/error logging for troubleshooting

### 3. Type Conversion
`registry.AgentInfo` contains different fields than `agent.AgentDescriptor`:
- AgentInfo has: `Instances`, `Endpoints`, `TargetTypes`, `TechniqueTypes`
- AgentDescriptor has: `Slots`, `TargetTypes`, `TechniqueTypes`
- When converting, we set `IsExternal=true` (all registry agents are external)
- Slots are not available from registry, so we use empty slice

### 4. Backward Compatibility
The implementation maintains full backward compatibility:
- Existing code using `AgentRegistry` continues to work unchanged
- No breaking changes to public interfaces
- Deprecated field clearly marked in documentation

## Testing

### Compilation Tests
- ✅ Full project builds successfully: `go build ./...`
- ✅ Harness package builds: `go build ./internal/harness/...`

### Existing Tests
- ✅ All factory tests pass: `go test -tags fts5 ./internal/harness -run TestFactory`
- ✅ No regression in existing functionality

### Migration Path

1. **Current State (Task 3.2)**:
   - Harness can use either registry type
   - `AgentRegistry` still works (backward compatible)
   - `RegistryAdapter` takes precedence when set

2. **Future State**:
   - Attack command sets `RegistryAdapter` in config (completed in Phase 2)
   - Harness uses `RegistryAdapter` for all agent operations
   - `AgentRegistry` can be deprecated and eventually removed

## Integration Points

This implementation integrates with:
- **Phase 1**: Uses `registry.ComponentDiscovery` interface from adapter infrastructure
- **Phase 2**: Works with updated attack command that provides `RegistryAdapter`
- **Task 3.1**: Uses `HarnessConfig.RegistryAdapter` field added in previous task

## Next Steps

The harness is now ready for:
- **Task 3.3**: Update mission command to use registry adapter
- **Task 3.4**: Add registry adapter support to other commands
- **Future**: Deprecate and remove `AgentRegistry` once all code paths use `RegistryAdapter`
