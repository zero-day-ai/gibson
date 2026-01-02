# Tasks: Component Installer Fix

## Overview
Implementation tasks for fixing the component installer to properly handle `build.workdir`, implement transactional rollback, and detect orphaned entries.

---

## Task 1: Add Artifact Verification Function

- [ ] 1.1 Add verifyArtifactsExist function to installer.go

**Files:** `internal/component/installer.go`

**Description:** Create a new function that verifies all specified artifacts exist at the expected source directory before attempting to copy them.

_Requirements: US-1_

_Prompt: Role: Go backend developer with expertise in file system operations and error handling | Task: Implement the task for spec component-installer-fix. First run spec-workflow-guide to get the workflow guide. Add a new method `verifyArtifactsExist(artifacts []string, sourceDir string) error` on *DefaultInstaller to internal/component/installer.go that: 1. Iterates through each artifact in the artifacts slice 2. Constructs the full path using filepath.Join(sourceDir, artifact) 3. Checks if each artifact exists using os.Stat 4. Collects all missing artifacts into a slice 5. If any are missing, returns a ComponentError using WrapComponentError with ErrCodeLoadFailed, message "artifacts not found after build", and WithContext for missing_artifacts (comma-separated), searched_paths (comma-separated), source_dir 6. Returns nil if all artifacts exist. Context: Read internal/component/installer.go for existing patterns, especially copyArtifactsToBin and error handling. Read internal/component/errors.go for WrapComponentError usage. | Restrictions: Do not modify any existing functions yet. Follow existing error wrapping patterns. Use strings.Join for formatting lists. Add function near copyArtifactsToBin for logical grouping. | Success: Function compiles without errors. Returns nil when all artifacts exist. Returns detailed ComponentError when any artifact is missing. Error includes all searched paths for debugging. After completing, mark this task as in-progress in tasks.md by changing [ ] to [-] before starting, implement the code, use log-implementation tool to record what was done, then mark task complete by changing [-] to [x]._

---

## Task 2: Add Orphan Detection Functions

- [ ] 2.1 Add isOrphanedComponent function

**Files:** `internal/component/installer.go`

**Description:** Create function to detect orphaned database entries (DB record exists but binary is missing).

_Requirements: US-3_

_Prompt: Role: Go backend developer with expertise in database operations and file system checks | Task: Implement the task for spec component-installer-fix. First run spec-workflow-guide to get the workflow guide. Add method `isOrphanedComponent(comp *Component) bool` on *DefaultInstaller to internal/component/installer.go that: 1. Returns false if comp is nil 2. Returns false if comp.BinPath is empty (script-based components don't have binaries) 3. Uses os.Stat to check if comp.BinPath exists 4. Returns true if os.IsNotExist(err) is true 5. Returns false otherwise (file exists or other error). Context: Read internal/component/types.go for Component struct and BinPath field. | Restrictions: Do not modify any existing functions. Keep logic simple and focused. No tracing needed for this helper. | Success: Function compiles. Returns true only when BinPath is set and file doesn't exist. After completing, mark task in-progress, implement, use log-implementation, then mark complete._

- [ ] 2.2 Add cleanupOrphanedComponent function

**Files:** `internal/component/installer.go`

**Description:** Create function to clean up orphaned DB entries with proper tracing.

_Requirements: US-3_

_Prompt: Role: Go backend developer with expertise in database operations and OpenTelemetry | Task: Implement the task for spec component-installer-fix. First run spec-workflow-guide to get the workflow guide. Add method `cleanupOrphanedComponent(ctx context.Context, kind ComponentKind, name string, existing *Component) error` on *DefaultInstaller that: 1. Calls i.isOrphanedComponent(existing) - returns nil if not orphaned 2. Gets span from context using trace.SpanFromContext(ctx) 3. Adds span event "cleaning up orphaned component" with attributes component.name and component.bin_path using trace.WithAttributes 4. If i.dao is not nil, calls i.dao.Delete(ctx, kind, name) 5. Returns WrapComponentError if delete fails with ErrCodeLoadFailed and WithComponent(name) 6. Returns nil on success. Context: Read existing OpenTelemetry usage in installer.go, especially span.AddEvent patterns in Uninstall function. | Restrictions: Follow existing tracing patterns exactly. Only clean if actually orphaned. | Success: Function compiles. Adds proper span event before deletion. Returns nil for non-orphaned components. After completing, mark task in-progress, implement, use log-implementation, then mark complete._

---

## Task 3: Add Install Context for Rollback

- [x] 3.1 Add installContext struct and rollback method

**Files:** `internal/component/installer.go`

**Description:** Create a struct to track resources created during installation for rollback on failure.

_Requirements: US-2_

_Prompt: Role: Go backend developer with expertise in transaction patterns and cleanup logic | Task: Implement the task for spec component-installer-fix. First run spec-workflow-guide to get the workflow guide. Add to internal/component/installer.go: 1. A new struct `installContext` with fields: copiedArtifacts []string (paths of artifacts copied to bin/), dbRegistered bool (whether component was registered in DB), componentKind ComponentKind, componentName string 2. A method `rollback(dao ComponentDAO, ctx context.Context)` on *installContext that: iterates through copiedArtifacts and removes each with os.Remove (ignore errors with _ =), then if dbRegistered is true and dao is not nil calls dao.Delete(ctx, ic.componentKind, ic.componentName) (ignore errors). Context: This is cleanup code during error handling, so errors are intentionally ignored. | Restrictions: Keep the struct simple. Errors during rollback should be silently ignored using _ = assignment. Place struct definition near top of file with other types. | Success: Struct and method compile. rollback removes all tracked artifacts. rollback removes DB entry if registered. No panics on nil dao or empty slices. After completing, mark task in-progress, implement, use log-implementation, then mark complete._

---

## Task 4: Fix installRepository Function

- [ ] 4.1 Add orphan detection to installRepository

**Files:** `internal/component/installer.go`

**Description:** Update the existing component check in installRepository to detect and clean orphaned entries.

_Requirements: US-3_

_Prompt: Role: Senior Go developer with expertise in installation systems | Task: Implement the task for spec component-installer-fix. First run spec-workflow-guide to get the workflow guide. Modify the component existence check in installRepository() function (around lines 711-719) to: 1. Keep existing check for !opts.Force && i.dao != nil 2. When existing component is found, first check if i.isOrphanedComponent(existing) 3. If orphaned, call i.cleanupOrphanedComponent and continue with install if successful, or add to Failed and continue loop if cleanup fails 4. If not orphaned, add to Failed with NewComponentExistsError as before. Context: Read the current implementation at lines 711-719. The existing code just fails if component exists - we need to add orphan detection first. | Restrictions: Maintain all existing logic for non-orphaned components. Keep span/tracing logic intact. Only modify the existence check block. | Success: Orphaned entries (DB but no binary) are cleaned and install proceeds. Valid existing components still fail with "already exists". After completing, mark task in-progress, implement, use log-implementation, then mark complete._

- [ ] 4.2 Fix artifact source path and add verification in installRepository

**Files:** `internal/component/installer.go`

**Description:** Ensure buildWorkDir is tracked correctly and used for artifact source, and add verification before copy.

_Requirements: US-1, US-4_

_Prompt: Role: Senior Go developer with expertise in installation systems | Task: Implement the task for spec component-installer-fix. First run spec-workflow-guide to get the workflow guide. Modify installRepository() (around lines 727-774) to: 1. Declare `var buildWorkDir string` at the start of component processing 2. When building (!opts.SkipBuild && compManifest.Build != nil), set buildWorkDir = componentDir, then if compManifest.Build.WorkDir != "" set buildWorkDir = filepath.Join(componentDir, compManifest.Build.WorkDir) 3. When not building, set buildWorkDir = componentDir 4. Before copyArtifactsToBin (around line 760), add call to i.verifyArtifactsExist(compManifest.Build.Artifacts, buildWorkDir) and handle error by adding to Failed and continue 5. Change the copyArtifactsToBin call to use buildWorkDir instead of artifactSourceDir. Context: Read current implementation - artifactSourceDir is calculated but may not match where build actually ran. buildWorkDir should be the source of truth. | Restrictions: Keep all existing build and tracing logic. Only fix path calculation and add verification. | Success: Artifacts are found at buildWorkDir when workdir is specified. Missing artifacts produce clear error before copy attempt. After completing, mark task in-progress, implement, use log-implementation, then mark complete._

- [ ] 4.3 Add rollback context to installRepository

**Files:** `internal/component/installer.go`

**Description:** Track copied artifacts and implement rollback on failure.

_Requirements: US-2_

_Prompt: Role: Senior Go developer with expertise in transaction patterns | Task: Implement the task for spec component-installer-fix. First run spec-workflow-guide to get the workflow guide. Modify installRepository() to use installContext for rollback: 1. At start of component processing loop, create installCtx := &installContext{componentKind: kind} 2. After loading manifest, set installCtx.componentName = compManifest.Name 3. After successful copyArtifactsToBin, add binPath to installCtx.copiedArtifacts 4. Before adding to result.Failed after artifact copy or validation failure, call installCtx.rollback(i.dao, ctx) 5. After successful dao.Create, set installCtx.dbRegistered = true (though not needed since we're done). Context: Read Task 3 for installContext definition. Rollback should happen before continue statements in error paths. | Restrictions: Only add rollback calls after resources have been created. Don't rollback before artifact copy. Keep success path unchanged. | Success: Failed installations have copied artifacts removed. No partial state left after failures. After completing, mark task in-progress, implement, use log-implementation, then mark complete._

---

## Task 5: Fix Install Function

- [ ] 5.1 Apply fixes to Install function

**Files:** `internal/component/installer.go`

**Description:** Apply the same pattern of fixes (orphan detection, artifact verification, rollback) to the Install() function.

_Requirements: US-1, US-2, US-3_

_Prompt: Role: Senior Go developer with expertise in installation systems | Task: Implement the task for spec component-installer-fix. First run spec-workflow-guide to get the workflow guide. Modify Install() function (lines 256-504) with same fixes as installRepository: 1. Add orphan detection when checking existing component (around line 370-376) - if orphaned, clean and proceed, if valid return exists error 2. Verify buildWorkDir is used correctly (check around line 385-417 - current code already calculates it) 3. Add call to verifyArtifactsExist before copyArtifactsToBin (before line 425) 4. Add rollback: create installContext, track copied artifacts, call rollback on validation failure (line 465-474) and DB registration failure (line 477-491). Context: Follow exact same pattern established in Task 4. Read current Install() implementation. | Restrictions: Maintain existing tracing logic. Keep backwards compatibility. Do not change function signature. | Success: Single component install with workdir works correctly. Orphaned entries are cleaned up. Failed installations are fully rolled back. After completing, mark task in-progress, implement, use log-implementation, then mark complete._

---

## Task 6: Fix installSingleComponent Function

- [ ] 6.1 Apply fixes to installSingleComponent function

**Files:** `internal/component/installer.go`

**Description:** Apply artifact verification and rollback to installSingleComponent() for consistency.

_Requirements: US-1, US-2, US-4_

_Prompt: Role: Senior Go developer with expertise in installation systems | Task: Implement the task for spec component-installer-fix. First run spec-workflow-guide to get the workflow guide. Modify installSingleComponent() (lines 870-1060) with: 1. Verify artifactSourceDir calculation is correct (lines 944-948 look correct) 2. Add call to verifyArtifactsExist before copyArtifactsToBin (before line 951) 3. Add rollback: create installContext, track copied artifacts, call rollback on validation failure (line 990-1008) and DB registration failure (line 1011-1030). Context: Follow pattern from Tasks 4 and 5. This function doesn't check for existing components so no orphan detection needed. | Restrictions: Maintain existing tracing logic. Keep error handling consistent with other functions. | Success: Single component repos with workdir install correctly. Failed installations are rolled back. After completing, mark task in-progress, implement, use log-implementation, then mark complete._

---

## Task 7: Enhance Error Messages

- [ ] 7.1 Improve copyArtifactsToBin error messages

**Files:** `internal/component/installer.go`

**Description:** Add source directory and remediation hints to error messages in copyArtifactsToBin.

_Requirements: TR-5_

_Prompt: Role: Go developer focused on developer experience | Task: Implement the task for spec component-installer-fix. First run spec-workflow-guide to get the workflow guide. Modify copyArtifactsToBin() error at lines 1727-1733: Change from WithContext("from", srcPath).WithContext("to", dstPath) to WithContext("artifact", artifact).WithContext("source", srcPath).WithContext("destination", dstPath).WithContext("source_dir", sourceDir).WithContext("remediation", "verify build completed successfully and artifact exists at source path"). Context: sourceDir is already a parameter to the function. | Restrictions: Only modify the error creation code. Keep existing logic unchanged. | Success: Error includes source_dir for debugging. Error includes actionable remediation hint. After completing, mark task in-progress, implement, use log-implementation, then mark complete._

---

## Task 8: Add Unit Tests

- [ ] 8.1 Add tests for new helper functions

**Files:** `internal/component/installer_test.go`

**Description:** Add unit tests for verifyArtifactsExist, isOrphanedComponent, cleanupOrphanedComponent, and installContext.

_Requirements: All requirements validation_

_Prompt: Role: Go developer with testing expertise using testify | Task: Implement the task for spec component-installer-fix. First run spec-workflow-guide to get the workflow guide. Add test functions to internal/component/installer_test.go: 1. TestVerifyArtifactsExist_AllPresent - create temp dir with test files, call verifyArtifactsExist, assert returns nil 2. TestVerifyArtifactsExist_Missing - create empty temp dir, call with ["missing"], assert error contains "artifacts not found" 3. TestIsOrphanedComponent_NilComponent - assert returns false 4. TestIsOrphanedComponent_EmptyBinPath - component with empty BinPath, assert returns false 5. TestIsOrphanedComponent_MissingBinary - component with BinPath to nonexistent file, assert returns true 6. TestIsOrphanedComponent_ValidBinary - create temp file, component pointing to it, assert returns false 7. TestInstallContext_Rollback - create temp files, create installContext tracking them, call rollback, assert files removed. Context: Follow existing test patterns in the file. Use testify assert/require. Use t.TempDir() for temp directories. | Restrictions: Use existing mock infrastructure. Keep tests isolated. | Success: All new tests pass. go test ./internal/component/... passes. After completing, mark task in-progress, implement, use log-implementation, then mark complete._

- [ ] 8.2 Add integration test for workdir scenario

**Files:** `internal/component/installer_test.go`

**Description:** Add test that replicates the k8skiller mono-repo scenario.

_Requirements: TC-1_

_Prompt: Role: Go developer with testing expertise | Task: Implement the task for spec component-installer-fix. First run spec-workflow-guide to get the workflow guide. Add TestInstallRepository_WithWorkdir to internal/component/installer_test.go: 1. Setup: Create temp directory structure mimicking mono-repo: component.yaml (kind: repository, discover: true), k8skiller/component.yaml (kind: agent, build.workdir: ./k8skiller, artifacts: [k8skiller]), k8skiller/k8skiller (mock binary file) 2. Create mocks that return success for clone, build 3. Execute installRepository 4. Assert: No errors, component registered in mock DAO, binPath points to agents/bin/k8skiller. Context: Use existing test helpers setupTestInstaller, createTestManifest. May need to extend createTestManifest to support more fields. | Restrictions: Follow existing test patterns. Use mocks for git and build operations. | Success: Test passes and validates the exact scenario from bug report. After completing, mark task in-progress, implement, use log-implementation, then mark complete._

---

## Task 9: Verify Fix End-to-End

- [ ] 9.1 Run tests and verify k8skiller installation

**Files:** None (verification only)

**Description:** Run the test suite to ensure no regressions, then verify the actual k8skiller installation works.

_Requirements: Success Metrics validation_

_Prompt: Role: QA engineer verifying the fix | Task: Implement the task for spec component-installer-fix. First run spec-workflow-guide to get the workflow guide. Execute verification: 1. Run unit tests: go test ./internal/component/... -v (assert all pass) 2. Build Gibson: make build (assert succeeds) 3. Clean stale state: sqlite3 ~/.gibson/gibson.db "DELETE FROM components WHERE name='k8skiller';" and rm -f ~/.gibson/agents/bin/k8skiller 4. Test fix: ./bin/gibson agent install-all git@github.com:zero-day-ai/gibson-agents-enterprise.git (assert no errors) 5. Verify binary: ls -la ~/.gibson/agents/bin/k8skiller (assert exists) 6. Test start/stop: ./bin/gibson agent start k8skiller then ./bin/gibson agent stop k8skiller (assert works) 7. Test idempotency: run install-all again, verify clean error or success. | Restrictions: Do not modify any code. Only run verification commands. Document any failures. | Success: All tests pass. k8skiller installs successfully. Binary exists. Agent starts/stops. No orphaned entries. After completing, mark task in-progress, run verification, use log-implementation to record results, then mark complete._

---

## Summary

| Task | Description | Complexity | Depends On |
|------|-------------|------------|------------|
| 1.1 | Add verifyArtifactsExist | Low | - |
| 2.1 | Add isOrphanedComponent | Low | - |
| 2.2 | Add cleanupOrphanedComponent | Low | 2.1 |
| 3.1 | Add installContext and rollback | Low | - |
| 4.1 | Add orphan detection to installRepository | Medium | 2.1, 2.2 |
| 4.2 | Fix artifact path in installRepository | Medium | 1.1 |
| 4.3 | Add rollback to installRepository | Medium | 3.1, 4.2 |
| 5.1 | Fix Install function | Medium | 1.1, 2.1, 2.2, 3.1 |
| 6.1 | Fix installSingleComponent | Medium | 1.1, 3.1 |
| 7.1 | Enhance error messages | Low | - |
| 8.1 | Add unit tests | Medium | 1.1, 2.1, 2.2, 3.1 |
| 8.2 | Add integration test | Medium | 4.1, 4.2, 4.3 |
| 9.1 | Verify fix end-to-end | Low | All |

