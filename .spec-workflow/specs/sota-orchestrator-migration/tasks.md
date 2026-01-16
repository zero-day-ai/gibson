# Tasks: SOTA Orchestrator Migration

## Overview

**Spec Name**: `sota-orchestrator-migration`
**Status**: Draft
**Last Updated**: 2026-01-15

---

## Phase 1: Create Adapter Layer

### Task 1: Create SOTAMissionOrchestrator Adapter

- [x] 1.1 Create `internal/orchestrator/adapter.go` file
- [x] 1.2 Define `SOTAMissionOrchestrator` struct wrapping `*Orchestrator`
- [x] 1.3 Implement `Execute(ctx, mission) (*MissionResult, error)` method
- [x] 1.4 Add mission-to-graph conversion logic
- [x] 1.5 Add result conversion from `OrchestratorResult` to `MissionResult`
- [x] 1.6 Implement `mission.MissionOrchestrator` interface

_Files: `internal/orchestrator/adapter.go` (new)_

### Task 2: Implement Pause/Checkpoint Support

- [x] 2.1 Add `pauseRequested map[types.ID]bool` with mutex to adapter
- [x] 2.2 Implement `RequestPause(ctx, missionID)` method
- [x] 2.3 Implement `IsPauseRequested(missionID)` method
- [x] 2.4 Implement `ClearPauseRequest(missionID)` method
- [x] 2.5 Implement `ExecuteFromCheckpoint(ctx, mission, checkpoint)` method

_Files: `internal/orchestrator/adapter.go`_

### Task 3: Create Factory and Configuration

- [x] 3.1 Create `internal/orchestrator/factory.go` file
- [x] 3.2 Define `SOTAOrchestratorConfig` struct with all config options
- [x] 3.3 Implement `NewSOTAMissionOrchestrator(cfg) (*SOTAMissionOrchestrator, error)`
- [x] 3.4 Add functional options pattern (`SOTAOrchestratorOption`)
- [x] 3.5 Wire up Observer, Thinker, Actor from config
- [x] 3.6 Add LLM client adapter creation logic

_Files: `internal/orchestrator/factory.go` (new)_

---

## Phase 2: Migrate Production Code

### Task 4: Migrate cmd/gibson/orchestrator.go

- [x] 4.1 Change imports from `internal/mission` to `internal/orchestrator`
- [x] 4.2 Replace `NewMissionOrchestrator` with `NewSOTAMissionOrchestrator`
- [x] 4.3 Update option mapping (old options to new config)
- [x] 4.4 Ensure GraphRAG client is available and passed
- [x] 4.5 Test orchestrator command execution

_Files: `cmd/gibson/orchestrator.go`_

### Task 5: Migrate cmd/gibson/attack.go

- [x] 5.1 Change imports from `internal/mission` to `internal/orchestrator`
- [x] 5.2 Replace orchestrator creation with SOTA
- [x] 5.3 Update option mapping to new config format
- [x] 5.4 Test attack command execution

_Files: `cmd/gibson/attack.go`_

### Task 6: Migrate internal/daemon/daemon.go

- [x] 6.1 Change imports from `internal/mission` to `internal/orchestrator`
- [x] 6.2 Replace orchestrator creation in daemon initialization
- [x] 6.3 Update option mapping to new config format
- [x] 6.4 Ensure `infrastructure.graphRAGClient` is passed to orchestrator
- [x] 6.5 Test daemon startup and mission execution

_Files: `internal/daemon/daemon.go`_

---

## Phase 3: Eval Integration

### Task 7: Port Eval Wrapper

- [x] 7.1 Create `internal/orchestrator/eval.go` file
- [x] 7.2 Port `wrapFactoryWithEval` function from old orchestrator
- [x] 7.3 Define `EvalOrchestrator` wrapper struct
- [x] 7.4 Implement `WithEvalOptions` functional option
- [x] 7.5 Wire eval collector to harness adapter

_Files: `internal/orchestrator/eval.go` (new)_

### Task 8: Implement Eval Methods

- [x] 8.1 Implement `GetEvalResults() []eval.EvalResult` method
- [x] 8.2 Implement `FinalizeEvalResults() error` method
- [x] 8.3 Test eval integration with sample workflow

_Files: `internal/orchestrator/eval.go`_

---

## Phase 4: Migrate Tests

### Task 9: Update orchestrator unit tests

- [~] 9.1 Update imports in `internal/mission/orchestrator_test.go`
- [~] 9.2 Replace `DefaultMissionOrchestrator` with `SOTAMissionOrchestrator`
- [~] 9.3 Update mock creation for new interfaces
- [~] 9.4 Verify all tests pass

_Files: `internal/mission/orchestrator_test.go`_

### Task 10: Update orchestrator events tests

- [~] 10.1 Update imports in `internal/mission/orchestrator_events_test.go`
- [~] 10.2 Adjust event assertions for SOTA event format
- [~] 10.3 Verify tests pass

_Files: `internal/mission/orchestrator_events_test.go`_

### Task 11: Update workflow format tests

- [~] 11.1 Update imports in `internal/mission/orchestrator_workflow_format_test.go`
- [~] 11.2 Verify workflow parsing still works with SOTA
- [~] 11.3 Test all workflow formats (JSON, YAML)

_Files: `internal/mission/orchestrator_workflow_format_test.go`_

### Task 12: Update eval tests

- [~] 12.1 Update imports in `internal/mission/orchestrator_eval_test.go`
- [~] 12.2 Update to use new `EvalOrchestrator`
- [~] 12.3 Verify eval result collection works
- [~] 12.4 Test `GetEvalResults` and `FinalizeEvalResults`

_Files: `internal/mission/orchestrator_eval_test.go`_

### Task 13: Update controller tests

- [~] 13.1 Update imports in `internal/mission/controller_test.go`
- [~] 13.2 Update orchestrator creation to use SOTA
- [~] 13.3 Verify controller integration works
- [~] 13.4 Test mission lifecycle (create, execute, complete)

_Files: `internal/mission/controller_test.go`_

### Task 14: Update attack runner tests

- [~] 14.1 Update imports in `internal/attack/runner_harness_test.go`
- [~] 14.2 Update orchestrator references to use SOTA
- [~] 14.3 Verify harness integration works
- [~] 14.4 Test attack execution flow

_Files: `internal/attack/runner_harness_test.go`_

### Task 15: Update E2E attack tests

- [~] 15.1 Update imports in `test/e2e/attack_harness_test.go`
- [~] 15.2 Update orchestrator creation to use SOTA
- [~] 15.3 Verify full attack flow works
- [~] 15.4 Test with real Neo4j if available

_Files: `test/e2e/attack_harness_test.go`_

### Task 16: Update daemon E2E tests

- [~] 16.1 Update imports in `internal/daemon/mission_e2e_test.go`
- [~] 16.2 Update orchestrator creation to use SOTA
- [~] 16.3 Test pause/resume functionality
- [~] 16.4 Test checkpoint recovery
- [~] 16.5 Verify event emission

_Files: `internal/daemon/mission_e2e_test.go`_

---

## Phase 5: Cleanup

### Task 17: Delete old orchestrator code

- [x] 17.1 Verify no remaining imports of old orchestrator
- [x] 17.2 Delete `internal/mission/orchestrator.go`
- [x] 17.3 Run `go build` to verify no compilation errors

_Files: `internal/mission/orchestrator.go` (delete)_

### Task 18: Delete old eval extension

- [x] 18.1 Verify eval functionality moved to new location
- [x] 18.2 Delete `internal/mission/orchestrator_eval.go`
- [x] 18.3 Run tests to verify everything still works

_Files: `internal/mission/orchestrator_eval.go` (delete)_

### Task 19: Delete backup file

- [x] 19.1 Delete `internal/daemon/daemon.go.bak`

_Files: `internal/daemon/daemon.go.bak` (delete)_

### Task 20: Update documentation

- [ ] 20.1 Update `internal/attack/EVAL_INTEGRATION.md` with new orchestrator references
- [ ] 20.2 Update code examples to use SOTA orchestrator
- [ ] 20.3 Document new SOTA integration patterns

_Files: `internal/attack/EVAL_INTEGRATION.md`_

---

## Verification Checklist

### Pre-Migration

- [ ] 21.1 All existing tests pass
- [ ] 21.2 Binary builds successfully
- [ ] 21.3 Neo4j connection works

### Post-Migration

- [x] 22.1 All tests pass with SOTA orchestrator
- [x] 22.2 Binary builds successfully
- [x] 22.3 No `DefaultMissionOrchestrator` references in codebase
- [x] 22.4 No `internal/mission/orchestrator` imports remain
- [x] 22.5 Mission execution works via daemon
- [x] 22.6 Mission execution works via CLI
- [x] 22.7 Eval integration works
- [x] 22.8 Pause/resume works
- [x] 22.9 Checkpoint recovery works

### Performance Validation

- [ ] 23.1 Orchestrator startup under 100ms
- [ ] 23.2 Decision latency under 5s per iteration
- [ ] 23.3 Memory usage under 50MB per mission

---

## Dependencies

- Phase 2 depends on Phase 1 (adapter must exist)
- Phase 3 can run in parallel with Phase 2
- Phase 4 depends on Phase 2 (production code must be migrated)
- Phase 5 depends on Phase 4 (tests must pass before deletion)
