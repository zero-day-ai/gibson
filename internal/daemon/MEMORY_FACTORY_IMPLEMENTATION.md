# MemoryManagerFactory Implementation

This document describes the implementation of tasks 5.1 and 5.2 for the harness-factory-wiring spec.

## Overview

The MemoryManagerFactory provides mission-scoped memory isolation by creating separate MemoryManager instances for each mission. This ensures agents can use working memory, mission memory, and long-term memory without cross-mission contamination.

## Task 5.1: Create MemoryManagerFactory

**File**: `/home/anthony/Code/zero-day.ai/opensource/gibson/internal/daemon/memory_factory.go`

### Implementation Details

```go
type MemoryManagerFactory struct {
    db     *database.DB
    config *memory.MemoryConfig
}
```

The factory stores:
- **Database connection**: Used for mission-tier persistence (SQLite)
- **Memory configuration**: Applied to all created memory managers

### Key Methods

#### NewMemoryManagerFactory
```go
func NewMemoryManagerFactory(db *database.DB, config *memory.MemoryConfig) (*MemoryManagerFactory, error)
```
- Creates a new factory instance
- Validates database connection (cannot be nil)
- Applies default configuration if nil is passed
- Validates configuration before creating factory

#### CreateForMission
```go
func (f *MemoryManagerFactory) CreateForMission(ctx context.Context, missionID types.ID) (memory.MemoryManager, error)
```
- Creates a mission-scoped MemoryManager
- Validates mission ID
- Returns configured MemoryManager with:
  - Working memory: In-memory, ephemeral key-value store
  - Mission memory: SQLite persistence scoped to mission ID
  - Long-term memory: Vector store for semantic search

## Task 5.2: Integrate into HarnessFactory

### Infrastructure Changes

**File**: `/home/anthony/Code/zero-day.ai/opensource/gibson/internal/daemon/infrastructure.go`

Added `memoryManagerFactory` field to Infrastructure struct:
```go
type Infrastructure struct {
    planExecutor         *plan.PlanExecutor
    findingStore         finding.FindingStore
    llmRegistry          llm.LLMRegistry
    slotManager          llm.SlotManager
    memoryManagerFactory *MemoryManagerFactory  // NEW
    harnessFactory       harness.HarnessFactoryInterface
}
```

Updated `newInfrastructure()` to initialize the factory:
```go
// Create memory manager factory with database and config
var memConfig *memory.MemoryConfig
if d.config != nil {
    memConfig = &d.config.Memory
}
memoryFactory, err := NewMemoryManagerFactory(d.db, memConfig)
if err != nil {
    return nil, fmt.Errorf("failed to create memory manager factory: %w", err)
}
d.logger.Info("initialized memory manager factory")
```

### Harness Integration

**File**: `/home/anthony/Code/zero-day.ai/opensource/gibson/internal/daemon/harness_init.go`

Added documentation explaining how to use the factory:
```go
// The MemoryManagerFactory is available at d.infrastructure.memoryManagerFactory
// and should be used by the mission manager to create mission-scoped memory
// managers when executing missions. Each mission should call:
//   memMgr, err := d.infrastructure.memoryManagerFactory.CreateForMission(ctx, missionID)
// and then pass the memory manager to harness configuration when creating
// harnesses for that mission.
```

## Usage Pattern

When creating a harness for a mission, the mission manager should:

1. Get the factory from infrastructure:
   ```go
   factory := d.infrastructure.memoryManagerFactory
   ```

2. Create a memory manager for the mission:
   ```go
   memMgr, err := factory.CreateForMission(ctx, missionID)
   if err != nil {
       return err
   }
   defer memMgr.Close()
   ```

3. Pass the memory manager to harness creation:
   ```go
   harness, err := harnessFactory.Create(agentName, missionCtx, targetInfo, memMgr)
   ```

## Memory Isolation

The factory ensures memory isolation between missions:

- Each mission gets its own MemoryManager instance
- Working memory is fully isolated (in-memory, per-instance)
- Mission memory is scoped by mission ID in the database
- Long-term memory uses the mission ID for semantic context

## Testing

Comprehensive tests have been implemented:

### Unit Tests
**File**: `/home/anthony/Code/zero-day.ai/opensource/gibson/internal/daemon/memory_factory_test.go`

- Factory initialization with default and custom config
- Error handling for nil database and invalid config
- Memory manager creation for valid mission IDs
- Isolation between multiple missions
- Manager lifecycle (close, idempotent close)
- Config propagation

### Integration Tests
**File**: `/home/anthony/Code/zero-day.ai/opensource/gibson/internal/daemon/memory_factory_integration_test.go`

- Full integration with database
- Functional memory managers with all three tiers
- Cross-mission memory isolation verification

## Architecture Benefits

1. **Centralized Configuration**: All memory managers use consistent config
2. **Mission Scoping**: Each mission has isolated memory storage
3. **Resource Management**: Factories can manage pooling/cleanup in the future
4. **Testability**: Easy to inject mock factories for testing
5. **Separation of Concerns**: Factory handles creation, manager handles operations

## Future Enhancements

Potential improvements for future iterations:

1. **Connection Pooling**: Reuse database connections across managers
2. **Resource Limits**: Enforce memory quotas per mission
3. **Metrics**: Track memory usage and performance per mission
4. **Cleanup**: Automatic cleanup of old mission memory
5. **Caching**: Cache frequently accessed mission data

## Success Criteria Met

✅ Factory creates isolated MemoryManagers per mission
✅ Harnesses have working Memory() method returning mission-scoped store
✅ Memory isolation between different missions
✅ Factory is initialized during daemon startup
✅ Factory configuration is read from daemon config
✅ Comprehensive tests verify functionality
