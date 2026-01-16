# Requirements: SOTA Orchestrator Migration

## Overview

**Spec Name**: `sota-orchestrator-migration`
**Created**: 2026-01-15
**Status**: Draft

### Problem Statement

Gibson currently has two orchestrator implementations:
1. **Old Orchestrator** (`internal/mission/orchestrator.go`): A simple workflow executor that runs nodes sequentially without LLM reasoning
2. **New SOTA Orchestrator** (`internal/orchestrator/`): A sophisticated Observe → Think → Act reasoning loop with LLM-driven decision making

The old orchestrator is used throughout the codebase (10+ files) but provides no intelligent decision-making. The new SOTA orchestrator has been partially integrated into `mission_manager.go` but the old code remains, causing confusion and technical debt.

### Business Value

- **Intelligent Execution**: LLM-driven decisions for node execution, retries, dynamic spawning
- **Better Observability**: State tracked in Neo4j GraphRAG with decision history
- **Maintainability**: Single orchestrator implementation to maintain
- **Production Readiness**: Comprehensive resource constraints (budget, concurrency, timeout)

---

## User Stories

### US-1: Complete Migration of Production Code
**As a** Gibson developer
**I want** all production code to use the SOTA orchestrator
**So that** missions execute with intelligent LLM-driven decisions

**Acceptance Criteria (EARS)**:
- WHERE `cmd/gibson/orchestrator.go` creates an orchestrator, THE system SHALL use the SOTA orchestrator from `internal/orchestrator/`
- WHERE `cmd/gibson/attack.go` creates an orchestrator, THE system SHALL use the SOTA orchestrator
- WHERE `internal/daemon/daemon.go` creates an orchestrator, THE system SHALL use the SOTA orchestrator
- WHEN a mission is executed, THE system SHALL use the Observe → Think → Act loop
- THE system SHALL maintain all existing configuration options (harness factory, event emitter, etc.)

### US-2: Eval Integration Migration
**As a** Gibson developer running evaluations
**I want** the eval integration to work with the SOTA orchestrator
**So that** I can measure agent performance during mission execution

**Acceptance Criteria (EARS)**:
- WHERE `orchestrator_eval.go` adds eval capabilities, THE system SHALL port these to the SOTA orchestrator
- WHEN eval is enabled, THE system SHALL collect eval metrics through the SOTA orchestrator
- THE system SHALL support `GetEvalResults()` and `FinalizeEvalResults()` methods

### US-3: Checkpoint/Pause Support
**As a** Gibson operator
**I want** the SOTA orchestrator to support pause and checkpoint functionality
**So that** I can pause long-running missions and resume them later

**Acceptance Criteria (EARS)**:
- WHEN `RequestPause()` is called, THE system SHALL pause at the next safe boundary
- WHERE a checkpoint exists, THE system SHALL support `ExecuteFromCheckpoint()` to resume
- THE system SHALL maintain pause state via `IsPauseRequested()` and `ClearPauseRequest()`

### US-4: Test Migration
**As a** Gibson developer
**I want** all tests to use the SOTA orchestrator
**So that** test coverage validates the new implementation

**Acceptance Criteria (EARS)**:
- WHERE unit tests create orchestrators, THE system SHALL use the SOTA orchestrator
- WHERE E2E tests verify pause/resume, THE system SHALL use SOTA orchestrator methods
- WHEN tests run, THE system SHALL pass with the new orchestrator

### US-5: Old Code Removal
**As a** Gibson maintainer
**I want** the old orchestrator code completely removed
**So that** there is a single source of truth for orchestration

**Acceptance Criteria (EARS)**:
- THE system SHALL NOT contain `DefaultMissionOrchestrator` struct
- THE system SHALL NOT contain the old `NewMissionOrchestrator` function in `internal/mission/`
- THE system SHALL remove or update all references to the old orchestrator
- THE system SHALL delete `internal/daemon/daemon.go.bak`

### US-6: Interface Compatibility
**As a** Gibson developer
**I want** the SOTA orchestrator to implement the `MissionOrchestrator` interface
**So that** existing code using the interface works without modification

**Acceptance Criteria (EARS)**:
- THE SOTA orchestrator SHALL implement `MissionOrchestrator` interface
- WHERE code uses `MissionOrchestrator` interface (e.g., `controller.go`), THE system SHALL work unchanged
- THE system SHALL provide configuration options matching the old orchestrator

---

## Functional Requirements

### FR-1: Core Orchestration
| ID | Requirement | Priority |
|----|-------------|----------|
| FR-1.1 | SOTA orchestrator SHALL execute missions via Observe → Think → Act loop | Must |
| FR-1.2 | SOTA orchestrator SHALL support all 6 decision types (execute_agent, skip_agent, modify_params, retry, spawn_agent, complete) | Must |
| FR-1.3 | SOTA orchestrator SHALL track state in Neo4j GraphRAG | Must |
| FR-1.4 | SOTA orchestrator SHALL enforce resource constraints (budget, concurrency, timeout) | Must |

### FR-2: Configuration Options
| ID | Requirement | Priority |
|----|-------------|----------|
| FR-2.1 | SOTA orchestrator SHALL accept `WithHarnessFactory()` option | Must |
| FR-2.2 | SOTA orchestrator SHALL accept `WithWorkflowExecutor()` option or equivalent | Should |
| FR-2.3 | SOTA orchestrator SHALL accept `WithEventEmitter()` option or equivalent | Should |
| FR-2.4 | SOTA orchestrator SHALL accept `WithEventBus()` option or equivalent | Should |
| FR-2.5 | SOTA orchestrator SHALL accept `WithTargetStore()` option | Should |
| FR-2.6 | SOTA orchestrator SHALL accept `WithEvalOptions()` option | Should |

### FR-3: Pause/Checkpoint
| ID | Requirement | Priority |
|----|-------------|----------|
| FR-3.1 | SOTA orchestrator SHALL support `RequestPause()` method | Should |
| FR-3.2 | SOTA orchestrator SHALL support `ExecuteFromCheckpoint()` method | Should |
| FR-3.3 | SOTA orchestrator SHALL track pause state per mission | Should |

### FR-4: Migration
| ID | Requirement | Priority |
|----|-------------|----------|
| FR-4.1 | All production code SHALL be updated to use SOTA orchestrator | Must |
| FR-4.2 | All test code SHALL be updated to use SOTA orchestrator | Must |
| FR-4.3 | Old orchestrator code SHALL be removed from codebase | Must |
| FR-4.4 | Documentation SHALL be updated to reference new orchestrator | Should |

---

## Non-Functional Requirements

### NFR-1: Performance
| ID | Requirement | Target |
|----|-------------|--------|
| NFR-1.1 | Orchestrator startup time | < 100ms |
| NFR-1.2 | Decision latency (Observe + Think + Act) | < 5s per iteration |
| NFR-1.3 | Memory overhead per mission | < 50MB |

### NFR-2: Reliability
| ID | Requirement | Target |
|----|-------------|--------|
| NFR-2.1 | Test pass rate after migration | 100% of existing tests |
| NFR-2.2 | No regression in mission success rate | Equal or better |
| NFR-2.3 | Graceful handling of LLM failures | Fallback/retry |

### NFR-3: Maintainability
| ID | Requirement | Target |
|----|-------------|--------|
| NFR-3.1 | Single orchestrator implementation | 1 package |
| NFR-3.2 | Code coverage | > 80% |
| NFR-3.3 | No circular dependencies | 0 cycles |

---

## Files to Modify

### Production Code (Must Migrate)
| File | Current Usage | Action |
|------|--------------|--------|
| `cmd/gibson/orchestrator.go` | Creates old orchestrator | Change to SOTA |
| `cmd/gibson/attack.go` | Creates old orchestrator | Change to SOTA |
| `internal/daemon/daemon.go` | Creates old orchestrator | Change to SOTA |

### Extension Code (Must Migrate)
| File | Current Usage | Action |
|------|--------------|--------|
| `internal/mission/orchestrator_eval.go` | Extends old orchestrator | Port eval to SOTA |

### Test Code (Must Migrate)
| File | Current Usage | Action |
|------|--------------|--------|
| `internal/mission/orchestrator_test.go` | Tests old orchestrator | Update tests |
| `internal/mission/orchestrator_events_test.go` | Tests events | Update tests |
| `internal/mission/orchestrator_workflow_format_test.go` | Tests workflow formats | Update tests |
| `internal/mission/orchestrator_eval_test.go` | Tests eval | Update tests |
| `internal/mission/controller_test.go` | Tests controller | Update tests |
| `internal/attack/runner_harness_test.go` | Tests attack runner | Update tests |
| `test/e2e/attack_harness_test.go` | E2E tests | Update tests |
| `internal/daemon/mission_e2e_test.go` | E2E tests | Update tests |

### Files to Delete
| File | Reason |
|------|--------|
| `internal/daemon/daemon.go.bak` | Deprecated backup |
| `internal/mission/orchestrator.go` (after migration) | Old implementation |
| `internal/mission/orchestrator_eval.go` (after migration) | Old eval extension |

### Documentation to Update
| File | Change |
|------|--------|
| `internal/attack/EVAL_INTEGRATION.md` | Reference new orchestrator |

---

## Out of Scope

- Changes to the SOTA orchestrator's core Observe → Think → Act logic
- Changes to Neo4j GraphRAG schema
- Changes to agent implementations
- Performance optimization of LLM calls
- Adding new decision types

---

## Dependencies

| Dependency | Status |
|------------|--------|
| SOTA orchestrator implementation | Complete |
| Neo4j GraphRAG | Complete |
| LLM integration | Complete |
| Harness factory | Complete |

---

## Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Breaking existing workflows | High | Comprehensive test migration |
| Performance regression | Medium | Benchmark before/after |
| Missing eval functionality | Medium | Verify eval tests pass |
| Checkpoint/pause regression | Medium | E2E test coverage |

---

## Success Metrics

1. All production code uses SOTA orchestrator
2. All tests pass (100%)
3. Old orchestrator code deleted
4. No `DefaultMissionOrchestrator` references in codebase
5. Documentation updated
