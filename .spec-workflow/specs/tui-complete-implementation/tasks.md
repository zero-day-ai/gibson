# Tasks: TUI Complete Implementation

## Overview
Break down the TUI implementation into atomic tasks that connect stub handlers to existing backend services.

---

## Task 1: Add Helper Functions for Command Parsing

- [ ] 1.1 Create helpers.go with utility functions

**Files:** `internal/tui/console/helpers.go` (create)

**Description:** Create helper functions for reading log files, parsing command flags, and standardizing error results.

_Requirements: TR-2, NFR-2_

_Prompt: Role: Go developer implementing TUI console utilities | Task: Create internal/tui/console/helpers.go with the following functions: readLastNLines(path string, n int) (string, error) - reads last N lines from a file, containsFlag(args []string, flag string) bool - checks if flag exists in args, getFlag(args []string, flag string) string - gets value after flag (supports --flag value and --flag=value), errorResult(err error) (*ExecutionResult, error) - creates standardized error result, usageResult(usage string) (*ExecutionResult, error) - creates usage error result. Context: Read internal/tui/console/executor.go to understand ExecutionResult structure. | Restrictions: Do not modify any existing files in this task. Use standard library only (bufio, os, strings, filepath). Follow existing code style in the console package. | Success: helpers.go compiles without errors. All 5 functions are implemented and exported. After completing, mark task 1.1 as in-progress in tasks.md before starting, use log-implementation tool to record what was done, then mark as complete._

---

## Task 2: Implement Log Viewing Handlers

- [ ] 2.1 Implement handleAgentLogs

**Files:** `internal/tui/console/executor.go`

**Description:** Update handleAgentLogs to read actual log file content instead of returning placeholder.

_Requirements: US-1_

_Prompt: Role: Go developer implementing TUI log viewing | Task: Update handleAgentLogs in internal/tui/console/executor.go to: 1. Parse --lines flag from args (default 50) 2. Build log path: {HomeDir}/logs/agent/{agentName}.log 3. Check if log file exists 4. Call readLastNLines helper to get content 5. Return content in ExecutionResult.Output. Context: Read cmd/gibson/component/logs.go for the CLI implementation pattern. Read internal/tui/console/helpers.go for helper functions. | Restrictions: Keep existing agent validation logic. Use helpers.go functions. Handle missing log file gracefully with informative message. | Success: /agent logs name displays actual log content. /agent logs name --lines 100 shows last 100 lines. Missing log file shows helpful error message. After completing, use log-implementation tool to record what was done, then mark task as complete._

- [ ] 2.2 Implement handleToolLogs

**Files:** `internal/tui/console/executor.go`

**Description:** Update handleToolLogs to read actual log file content.

_Requirements: US-1_

_Prompt: Role: Go developer implementing TUI log viewing | Task: Update handleToolLogs in internal/tui/console/executor.go to: 1. Parse --lines flag from args (default 50) 2. Build log path: {HomeDir}/logs/tool/{toolName}.log 3. Check if log file exists 4. Call readLastNLines helper to get content 5. Return content in ExecutionResult.Output. Context: Use same pattern as handleAgentLogs implementation. | Restrictions: Keep existing tool validation logic. Use helpers.go functions. | Success: /tool logs name displays actual log content. Missing log file shows helpful error message. After completing, use log-implementation tool then mark task complete._

- [ ] 2.3 Add handlePluginLogs handler

**Files:** `internal/tui/console/executor.go`, `internal/tui/console/commands.go`

**Description:** Add plugin logs handler following same pattern as agent/tool logs.

_Requirements: US-1_

_Prompt: Role: Go developer implementing TUI log viewing | Task: 1. Add handlePluginLogs function to executor.go following handleToolLogs pattern 2. Register /plugin logs command in commands.go if not already present 3. Update handlePlugin router to route logs subcommand. Context: Check if plugin logs subcommand routing exists. | Restrictions: Follow existing plugin handler patterns. Use helpers.go functions. | Success: /plugin logs name displays actual log content. Command is properly registered and routed. After completing, use log-implementation tool then mark task complete._

---

## Task 3: Implement Component Installation Handlers

- [ ] 3.1 Update ExecutorConfig with Installer dependency

**Files:** `internal/tui/console/executor.go`

**Description:** Add Installer field to ExecutorConfig and create getter method.

_Requirements: TR-1_

_Prompt: Role: Go developer implementing dependency injection | Task: 1. Add Installer field to ExecutorConfig struct: Installer component.Installer 2. Add getInstaller() method to Executor that returns e.config.Installer if set or creates default installer if not set (using git.NewDefaultGitOperations, build.NewDefaultBuildExecutor, e.config.ComponentDAO) 3. Add necessary imports. Context: Read internal/component/installer.go for Installer interface. Read cmd/gibson/component/install.go getInstaller function. | Restrictions: Do not break existing code. Installer should be optional (create default if nil). | Success: ExecutorConfig has Installer field. getInstaller() returns working Installer instance. Code compiles without errors. After completing, use log-implementation tool then mark task complete._

- [ ] 3.2 Implement handleAgentInstall

**Files:** `internal/tui/console/executor.go`

**Description:** Update handleAgentInstall to perform actual installation.

_Requirements: US-3_

_Prompt: Role: Go developer implementing TUI component installation | Task: Update handleAgentInstall in executor.go to: 1. Parse repoURL from args[0] 2. Parse optional flags: --force, --branch, --tag 3. Create component.InstallOptions from flags 4. Call e.getInstaller().Install(ctx, repoURL, component.ComponentKindAgent, opts) 5. Return success message with component name and version 6. Handle errors with errorResult helper. Context: Read cmd/gibson/component/install.go runInstall function. Read internal/component/installer.go for Install method signature. | Restrictions: Use helpers.go for flag parsing. Follow error handling patterns. | Success: /agent install github-url installs agent. /agent install url --force reinstalls. Error messages are clear and actionable. After completing, use log-implementation tool then mark task complete._

- [ ] 3.3 Implement handleToolInstall

**Files:** `internal/tui/console/executor.go`

**Description:** Update handleToolInstall to perform actual installation.

_Requirements: US-3_

_Prompt: Role: Go developer implementing TUI component installation | Task: Update handleToolInstall following same pattern as handleAgentInstall but with component.ComponentKindTool instead of ComponentKindAgent. Context: Use handleAgentInstall as template. | Restrictions: Follow exact same pattern as agent install. | Success: /tool install github-url installs tool. After completing, use log-implementation tool then mark task complete._

- [ ] 3.4 Implement handlePluginInstall

**Files:** `internal/tui/console/executor.go`

**Description:** Update handlePluginInstall to perform actual installation.

_Requirements: US-3_

_Prompt: Role: Go developer implementing TUI component installation | Task: Update handlePluginInstall following same pattern as handleAgentInstall but with component.ComponentKindPlugin instead of ComponentKindAgent. Context: Use handleAgentInstall as template. | Restrictions: Follow exact same pattern as agent install. | Success: /plugin install github-url installs plugin. After completing, use log-implementation tool then mark task complete._

---

## Task 4: Implement Component Uninstallation Handlers

- [ ] 4.1 Implement handleAgentUninstall

**Files:** `internal/tui/console/executor.go`

**Description:** Update handleAgentUninstall to perform actual uninstallation.

_Requirements: US-4_

_Prompt: Role: Go developer implementing TUI component uninstallation | Task: Update handleAgentUninstall in executor.go to: 1. Require --confirm flag (already partially implemented) 2. Get component via ComponentDAO.GetByName 3. If running, stop via getLifecycleManager().StopComponent 4. Delete from database via ComponentDAO.Delete 5. Remove installation directory if exists (component.InstallPath) 6. Return success message. Context: Read existing handleAgentUninstall for confirm flag logic. Read internal/component/lifecycle.go for StopComponent. Read internal/database/component_dao.go for Delete method. | Restrictions: Keep existing --confirm requirement. Stop component before deleting. Handle errors gracefully. | Success: /agent uninstall name --confirm removes agent. Running agents are stopped first. Clear error if agent not found. After completing, use log-implementation tool then mark task complete._

- [ ] 4.2 Implement handleToolUninstall

**Files:** `internal/tui/console/executor.go`

**Description:** Update handleToolUninstall following agent pattern.

_Requirements: US-4_

_Prompt: Role: Go developer implementing TUI component uninstallation | Task: Update handleToolUninstall following handleAgentUninstall pattern with component.ComponentKindTool. Context: Use handleAgentUninstall as template. | Restrictions: Follow exact same pattern. | Success: /tool uninstall name --confirm removes tool. After completing, use log-implementation tool then mark task complete._

- [ ] 4.3 Implement handlePluginUninstall

**Files:** `internal/tui/console/executor.go`

**Description:** Update handlePluginUninstall following agent pattern.

_Requirements: US-4_

_Prompt: Role: Go developer implementing TUI component uninstallation | Task: Update handlePluginUninstall following handleAgentUninstall pattern with component.ComponentKindPlugin. Context: Use handleAgentUninstall as template. | Restrictions: Follow exact same pattern. | Success: /plugin uninstall name --confirm removes plugin. After completing, use log-implementation tool then mark task complete._

---

## Task 5: Implement Attack Command

- [ ] 5.1 Add AttackRunner to ExecutorConfig

**Files:** `internal/tui/console/executor.go`

**Description:** Add AttackRunner dependency to ExecutorConfig.

_Requirements: TR-1_

_Prompt: Role: Go developer implementing dependency injection | Task: 1. Add AttackRunner field to ExecutorConfig: AttackRunner *attack.AttackRunner 2. Add import for internal/attack package 3. AttackRunner is optional (can be nil). Context: Read internal/attack/runner.go for AttackRunner type. Read internal/attack/attack.go package documentation. | Restrictions: Do not create AttackRunner in executor (injected from TUI startup). Keep field optional. | Success: ExecutorConfig has AttackRunner field. Code compiles. After completing, use log-implementation tool then mark task complete._

- [ ] 5.2 Implement handleAttack

**Files:** `internal/tui/console/executor.go`

**Description:** Update handleAttack to execute real attacks via AttackRunner.

_Requirements: US-2_

_Prompt: Role: Go developer implementing TUI attack execution | Task: Update handleAttack in executor.go to: 1. Parse target URL from args[0] 2. Parse required --agent flag 3. Parse optional --goal, --mode, --max-findings, --timeout flags 4. Check AttackRunner is available (return helpful error if nil) 5. Create attack.AttackOptions with parsed values 6. Validate options 7. Execute attack via AttackRunner.Run 8. Format result with status, duration, finding count 9. List findings if any discovered. Context: Read internal/attack/options.go for AttackOptions. Read internal/attack/runner.go for Run method. Read internal/attack/result.go for AttackResult. | Restrictions: Use helpers.go for flag parsing. Handle nil AttackRunner gracefully. Format output for console readability. | Success: /attack url --agent name executes attack. Attack progress displayed. Findings listed when discovered. Clear error if AttackRunner not configured. After completing, use log-implementation tool then mark task complete._

---

## Task 6: Implement Mission Creation

- [ ] 6.1 Implement handleMissionCreate

**Files:** `internal/tui/console/executor.go`

**Description:** Update handleMissionCreate to create missions from workflow files.

_Requirements: US-7_

_Prompt: Role: Go developer implementing TUI mission creation | Task: Update handleMissionCreate in executor.go to: 1. Parse -f or --file flag for workflow file path 2. Read workflow file content 3. Parse YAML into workflow.Workflow struct 4. Create mission.Mission with ID from uuid.New().String(), Name from workflow, Description from workflow, Status MissionStatusPending, WorkflowJSON serialized workflow 5. Save via mission.NewDBMissionStore(e.config.DB).Create 6. Return success with mission ID and start command hint. Context: Read internal/workflow/workflow.go for Workflow struct. Read internal/mission/mission.go for Mission struct. Read internal/mission/store.go for MissionStore interface. | Restrictions: Require workflow file path. Validate YAML parsing. Handle file read errors. | Success: /mission create -f workflow.yaml creates mission. Mission appears in /mission list. Clear error on invalid YAML. After completing, use log-implementation tool then mark task complete._

---

## Task 7: Implement Findings Export

- [ ] 7.1 Update exportFindings in FindingsView

**Files:** `internal/tui/views/findings.go`

**Description:** Update exportFindings to write actual files using existing exporters.

_Requirements: US-6_

_Prompt: Role: Go developer implementing TUI findings export | Task: Update exportFindings in internal/tui/views/findings.go to: 1. Convert v.findings slice to []*finding.EnhancedFinding pointers 2. Select appropriate exporter based on format: json uses export.NewJSONExporter(), csv uses export.NewCSVExporter(), sarif uses export.NewSARIFExporter(), markdown/md uses export.NewMarkdownExporter(), html uses export.NewHTMLExporter() 3. Call exporter.Export with DefaultExportOptions 4. Generate filename: findings_{timestamp}.{format} 5. Write to file with os.WriteFile 6. Return success message with filename and count. Context: Read internal/finding/export/exporter.go for interface. Read internal/finding/export/json_exporter.go for example. Check existing exportFindings stub implementation. | Restrictions: Handle all supported formats. Handle export/write errors gracefully. Use current working directory for output. | Success: Export dialog creates actual files. All 5 formats work (json, csv, sarif, markdown, html). Files contain valid formatted data. After completing, use log-implementation tool then mark task complete._

---

## Task 8: Implement Mission Logs

- [ ] 8.1 Update loadMissionLogs in MissionView

**Files:** `internal/tui/views/mission.go`

**Description:** Update mission logs loading to show real data instead of placeholders.

_Requirements: US-5_

_Prompt: Role: Go developer implementing TUI mission logs | Task: Update mission log loading in internal/tui/views/mission.go to: 1. Find where placeholder logs are generated (search for TODO or placeholder) 2. Try loading from file: {homeDir}/logs/missions/{missionID}.log 3. If file exists, read and display content 4. If no log file, generate summary from mission state including Mission name, ID, status, Created/started/completed timestamps, Findings count, Progress percentage, Agent assignments if available. Context: Read existing mission.go to find log loading code. Check MissionView struct for homeDir field (may need to add). | Restrictions: Do not break existing functionality. Handle missing log files gracefully. Keep output readable. | Success: Mission view shows real logs or meaningful status. No more placeholder messages. Logs update when mission is selected. After completing, use log-implementation tool then mark task complete._

---

## Task 9: Wire Up TUI Dependencies

- [ ] 9.1 Update TUI initialization to inject dependencies

**Files:** `cmd/gibson/tui.go`

**Description:** Update TUI command to create and inject Installer and AttackRunner.

_Requirements: TR-1_

_Prompt: Role: Go developer implementing TUI initialization | Task: Update cmd/gibson/tui.go to: 1. Create Installer using git.NewDefaultGitOperations, build.NewDefaultBuildExecutor, componentDAO 2. Create AttackRunner if all dependencies available (orchestrator, registryAdapter, payloadRegistry, missionStore, findingStore) 3. Pass Installer and AttackRunner to ExecutorConfig 4. AttackRunner can be nil if dependencies unavailable. Context: Read existing tui.go for current initialization. Read cmd/gibson/attack.go for how AttackRunner is created in CLI. Read internal/tui/console/executor.go for ExecutorConfig. | Restrictions: Do not break existing TUI startup. Make AttackRunner optional. Log warning if AttackRunner unavailable. | Success: TUI starts without errors. Installer is always available. AttackRunner available when dependencies exist. Clear warning logged if AttackRunner unavailable. After completing, use log-implementation tool then mark task complete._

---

## Task 10: Testing and Verification

- [ ] 10.1 Manual testing of all implemented handlers

**Files:** None (testing task)

**Description:** Test all implemented functionality to ensure it works correctly.

_Requirements: NFR-2, NFR-3_

_Prompt: Role: QA engineer verifying TUI functionality | Task: Manually test all implemented handlers: 1. Start TUI: gibson tui 2. Test log viewing with /agent logs, /tool logs, /plugin logs 3. Test component installation with /agent install if test repo available 4. Test component uninstallation with /agent uninstall name --confirm 5. Test findings export in each format from Findings view 6. Test mission creation with /mission create -f workflow.yaml if workflow file available 7. Test mission logs by selecting mission in Mission view. Document any failures or issues found. | Restrictions: Do not modify code in this task. Document issues for follow-up tasks. | Success: All handlers return real data instead of not implemented. No crashes or panics. Error messages are helpful. After completing, use log-implementation tool to record results then mark task complete._

---

## Summary

| Task | Description | Files | Depends On |
|------|-------------|-------|------------|
| 1.1 | Create helpers.go | console/helpers.go | - |
| 2.1 | Agent logs | executor.go | 1.1 |
| 2.2 | Tool logs | executor.go | 1.1 |
| 2.3 | Plugin logs | executor.go | 1.1 |
| 3.1 | Add Installer to config | executor.go | - |
| 3.2 | Agent install | executor.go | 1.1, 3.1 |
| 3.3 | Tool install | executor.go | 3.2 |
| 3.4 | Plugin install | executor.go | 3.2 |
| 4.1 | Agent uninstall | executor.go | 1.1 |
| 4.2 | Tool uninstall | executor.go | 4.1 |
| 4.3 | Plugin uninstall | executor.go | 4.1 |
| 5.1 | Add AttackRunner to config | executor.go | - |
| 5.2 | Attack command | executor.go | 1.1, 5.1 |
| 6.1 | Mission create | executor.go | 1.1 |
| 7.1 | Findings export | views/findings.go | - |
| 8.1 | Mission logs | views/mission.go | - |
| 9.1 | Wire up TUI deps | cmd/gibson/tui.go | 3.1, 5.1 |
| 10.1 | Manual testing | - | All |
