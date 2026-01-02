# Design: TUI Complete Implementation

## Overview

This design connects TUI stub handlers to existing backend services. The key insight is that **all required functionality already exists** - the TUI simply needs to call it. No new backend services are required.

## Architecture Approach

### Design Principle: Reuse, Don't Rebuild

The CLI commands already implement all required functionality. The TUI handlers will:
1. Extract and adapt logic from CLI command implementations
2. Use existing services (installer, exporter, log reader) directly
3. Format output appropriately for console viewport display

### Component Integration Map

```
TUI Console Executor
       │
       ├── Log Viewing ──────────► cmd/gibson/component/logs.go
       │                           (tailLogs, followLogs functions)
       │
       ├── Component Install ───► internal/component/installer.go
       │                           (Installer.Install, InstallAll)
       │
       ├── Component Uninstall ─► internal/component/lifecycle.go
       │                           (LifecycleManager.StopComponent)
       │                           + ComponentDAO.Delete
       │
       ├── Attack Execution ────► internal/attack/runner.go
       │                           (AttackRunner.Run)
       │
       ├── Mission Create ──────► internal/mission/controller.go
       │                           (MissionController.Create)
       │
       └── Findings Export ─────► internal/finding/export/*.go
                                   (JSONExporter, CSVExporter, etc.)
```

## Detailed Design

### 1. Log Viewing Implementation

**Files Modified:** `internal/tui/console/executor.go`

**Approach:** Port `tailLogs` function from `cmd/gibson/component/logs.go` to work within TUI context.

```go
// handleAgentLogs - Updated implementation
func (e *Executor) handleAgentLogs(ctx context.Context, args []string) (*ExecutionResult, error) {
    agentName := args[0]
    lines := 50 // default, parse from args if --lines flag present

    // Build log path (same as CLI)
    logPath := filepath.Join(e.config.HomeDir, "logs", "agent", agentName+".log")

    // Check file exists
    if _, err := os.Stat(logPath); os.IsNotExist(err) {
        return &ExecutionResult{
            Output:  fmt.Sprintf("No logs found for agent '%s'\n", agentName),
            IsError: true,
        }, nil
    }

    // Read last N lines (adapted from tailLogs)
    content, err := readLastNLines(logPath, lines)
    if err != nil {
        return errorResult(err)
    }

    return &ExecutionResult{
        Output: content,
    }, nil
}
```

**Helper Function:**
```go
// readLastNLines reads the last N lines from a file
func readLastNLines(path string, n int) (string, error) {
    file, err := os.Open(path)
    if err != nil {
        return "", err
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    var lines []string
    for scanner.Scan() {
        lines = append(lines, scanner.Text())
    }

    start := 0
    if len(lines) > n {
        start = len(lines) - n
    }

    return strings.Join(lines[start:], "\n"), scanner.Err()
}
```

### 2. Component Installation Implementation

**Files Modified:** `internal/tui/console/executor.go`

**New Dependencies in ExecutorConfig:**
```go
type ExecutorConfig struct {
    // Existing fields...

    // New: Component installation
    Installer component.Installer
}
```

**Implementation:**
```go
func (e *Executor) handleAgentInstall(ctx context.Context, args []string) (*ExecutionResult, error) {
    if len(args) < 1 {
        return usageResult("Usage: /agent install <github-url>")
    }

    repoURL := args[0]

    // Parse flags from args
    opts := component.InstallOptions{
        Force: containsFlag(args, "--force"),
    }

    // Get or create installer
    installer := e.getInstaller()

    result, err := installer.Install(ctx, repoURL, component.ComponentKindAgent, opts)
    if err != nil {
        return errorResult(err)
    }

    return &ExecutionResult{
        Output: fmt.Sprintf("Agent '%s' installed successfully (v%s)\n",
            result.Component.Name, result.Component.Version),
    }, nil
}

func (e *Executor) getInstaller() component.Installer {
    if e.config.Installer != nil {
        return e.config.Installer
    }
    // Create default installer (same as CLI does)
    gitOps := git.NewDefaultGitOperations()
    builder := build.NewDefaultBuildExecutor()
    return component.NewDefaultInstaller(gitOps, builder, e.config.ComponentDAO)
}
```

### 3. Component Uninstallation Implementation

**Files Modified:** `internal/tui/console/executor.go`

**Implementation:**
```go
func (e *Executor) handleAgentUninstall(ctx context.Context, args []string) (*ExecutionResult, error) {
    if len(args) < 1 {
        return usageResult("Usage: /agent uninstall <name> --confirm")
    }

    agentName := args[0]

    // Require --confirm flag
    if !containsFlag(args, "--confirm") {
        return &ExecutionResult{
            Output: "Uninstall requires --confirm flag.\nUsage: /agent uninstall <name> --confirm\n",
            IsError: true,
        }, nil
    }

    // Get component
    agent, err := e.config.ComponentDAO.GetByName(ctx, component.ComponentKindAgent, agentName)
    if err != nil || agent == nil {
        return errorResult(fmt.Errorf("agent '%s' not found", agentName))
    }

    // Stop if running
    if agent.IsRunning() {
        lm := e.getLifecycleManager()
        if err := lm.StopComponent(ctx, agent); err != nil {
            return errorResult(fmt.Errorf("failed to stop agent: %w", err))
        }
    }

    // Delete from database
    if err := e.config.ComponentDAO.Delete(ctx, agent.ID); err != nil {
        return errorResult(err)
    }

    // Optionally remove files (component.InstallPath)
    if agent.InstallPath != "" {
        os.RemoveAll(agent.InstallPath)
    }

    return &ExecutionResult{
        Output: fmt.Sprintf("Agent '%s' uninstalled successfully\n", agentName),
    }, nil
}
```

### 4. Attack Command Implementation

**Files Modified:** `internal/tui/console/executor.go`

**New Dependencies in ExecutorConfig:**
```go
type ExecutorConfig struct {
    // Existing fields...

    // New: Attack execution
    AttackRunner *attack.AttackRunner
}
```

**Implementation:**
```go
func (e *Executor) handleAttack(ctx context.Context, args []string) (*ExecutionResult, error) {
    if len(args) < 1 {
        return usageResult("Usage: /attack <target-url> --agent <name> [--goal <goal>]")
    }

    // Parse arguments
    targetURL := args[0]
    agentName := getFlag(args, "--agent")
    goal := getFlag(args, "--goal")

    if agentName == "" {
        return errorResult(fmt.Errorf("--agent flag is required"))
    }

    // Create attack options
    opts := attack.NewAttackOptions()
    opts.TargetURL = targetURL
    opts.AgentName = agentName
    opts.Goal = goal
    opts.OutputFormat = "text"
    opts.Verbose = true

    // Validate options
    if err := opts.Validate(); err != nil {
        return errorResult(err)
    }

    // Get attack runner (must be injected at TUI startup)
    if e.config.AttackRunner == nil {
        return errorResult(fmt.Errorf("attack runner not configured"))
    }

    // Execute attack
    result, err := e.config.AttackRunner.Run(ctx, opts)
    if err != nil {
        return errorResult(err)
    }

    // Format output
    var output strings.Builder
    output.WriteString(fmt.Sprintf("Attack completed: %s\n", result.Status))
    output.WriteString(fmt.Sprintf("Duration: %s\n", result.Duration))
    output.WriteString(fmt.Sprintf("Findings: %d\n", result.FindingCount()))

    if result.HasFindings() {
        output.WriteString("\nFindings:\n")
        for _, f := range result.Findings {
            output.WriteString(fmt.Sprintf("  [%s] %s\n", f.Severity, f.Title))
        }
    }

    if result.Persisted {
        output.WriteString(fmt.Sprintf("\nMission saved: %s\n", result.MissionID))
    }

    return &ExecutionResult{
        Output: output.String(),
    }, nil
}
```

### 5. Mission Creation Implementation

**Files Modified:** `internal/tui/console/executor.go`

**New Dependencies in ExecutorConfig:**
```go
type ExecutorConfig struct {
    // Existing fields...

    // New: Mission management
    MissionController *mission.MissionController
}
```

**Implementation:**
```go
func (e *Executor) handleMissionCreate(ctx context.Context, args []string) (*ExecutionResult, error) {
    // Parse -f flag for workflow file
    workflowPath := getFlag(args, "-f")
    if workflowPath == "" {
        workflowPath = getFlag(args, "--file")
    }

    if workflowPath == "" {
        return usageResult("Usage: /mission create -f <workflow.yaml>")
    }

    // Read workflow file
    workflowData, err := os.ReadFile(workflowPath)
    if err != nil {
        return errorResult(fmt.Errorf("failed to read workflow file: %w", err))
    }

    // Parse workflow YAML
    var wf workflow.Workflow
    if err := yaml.Unmarshal(workflowData, &wf); err != nil {
        return errorResult(fmt.Errorf("invalid workflow YAML: %w", err))
    }

    // Create mission
    missionStore := mission.NewDBMissionStore(e.config.DB)

    m := &mission.Mission{
        ID:          uuid.New().String(),
        Name:        wf.Name,
        Description: wf.Description,
        Status:      mission.MissionStatusPending,
        CreatedAt:   time.Now(),
    }

    // Serialize workflow to JSON for storage
    workflowJSON, _ := json.Marshal(wf)
    m.WorkflowJSON = string(workflowJSON)

    if err := missionStore.Create(ctx, m); err != nil {
        return errorResult(err)
    }

    return &ExecutionResult{
        Output: fmt.Sprintf("Mission '%s' created successfully\nID: %s\nStart with: /mission start %s\n",
            m.Name, m.ID, m.Name),
    }, nil
}
```

### 6. Findings Export Implementation

**Files Modified:** `internal/tui/views/findings.go`

**Implementation:**
```go
func (v *FindingsView) exportFindings(format string) tea.Cmd {
    return func() tea.Msg {
        ctx := context.Background()

        // Convert view findings to pointers for exporter
        findings := make([]*finding.EnhancedFinding, len(v.findings))
        for i := range v.findings {
            findings[i] = &v.findings[i]
        }

        // Get appropriate exporter
        var exporter export.Exporter
        switch format {
        case "json":
            exporter = export.NewJSONExporter()
        case "csv":
            exporter = export.NewCSVExporter()
        case "sarif":
            exporter = export.NewSARIFExporter()
        case "markdown", "md":
            exporter = export.NewMarkdownExporter()
        case "html":
            exporter = export.NewHTMLExporter()
        default:
            return findingExportedMsg{message: fmt.Sprintf("Unknown format: %s", format)}
        }

        // Export with default options
        opts := export.DefaultExportOptions()
        data, err := exporter.Export(ctx, findings, opts)
        if err != nil {
            return findingExportedMsg{message: fmt.Sprintf("Export failed: %v", err)}
        }

        // Write to file
        filename := fmt.Sprintf("findings_%s.%s",
            time.Now().Format("20060102_150405"), format)

        if err := os.WriteFile(filename, data, 0644); err != nil {
            return findingExportedMsg{message: fmt.Sprintf("Failed to write file: %v", err)}
        }

        return findingExportedMsg{
            message: fmt.Sprintf("Exported %d findings to %s", len(findings), filename),
        }
    }
}
```

### 7. Mission Logs Implementation

**Files Modified:** `internal/tui/views/mission.go`

**Approach:** Mission execution logs can come from:
1. Agent execution events (stored during workflow execution)
2. File-based logs in `~/.gibson/logs/missions/<mission-id>.log`

```go
func (v *MissionView) loadMissionLogs() tea.Cmd {
    return func() tea.Msg {
        if v.selectedMission == nil {
            return missionLogsLoadedMsg{logs: "No mission selected"}
        }

        // Try to load from mission log file
        logPath := filepath.Join(v.homeDir, "logs", "missions",
            v.selectedMission.ID + ".log")

        if _, err := os.Stat(logPath); err == nil {
            content, err := os.ReadFile(logPath)
            if err == nil && len(content) > 0 {
                return missionLogsLoadedMsg{logs: string(content)}
            }
        }

        // Fall back to generating logs from mission events/findings
        var logs strings.Builder
        logs.WriteString(fmt.Sprintf("Mission: %s\n", v.selectedMission.Name))
        logs.WriteString(fmt.Sprintf("Status: %s\n", v.selectedMission.Status))
        logs.WriteString(fmt.Sprintf("Created: %s\n", v.selectedMission.CreatedAt.Format(time.RFC3339)))

        if v.selectedMission.StartedAt != nil {
            logs.WriteString(fmt.Sprintf("Started: %s\n", v.selectedMission.StartedAt.Format(time.RFC3339)))
        }
        if v.selectedMission.CompletedAt != nil {
            logs.WriteString(fmt.Sprintf("Completed: %s\n", v.selectedMission.CompletedAt.Format(time.RFC3339)))
        }

        logs.WriteString(fmt.Sprintf("\nFindings: %d\n", v.selectedMission.FindingsCount))
        logs.WriteString(fmt.Sprintf("Progress: %.0f%%\n", v.selectedMission.Progress*100))

        return missionLogsLoadedMsg{logs: logs.String()}
    }
}
```

## Dependency Injection Updates

### TUI App Initialization

**File:** `cmd/gibson/tui.go`

The TUI app initialization needs to inject the new dependencies:

```go
// In the TUI command setup
func createTUIApp() (*tui.App, error) {
    // ... existing setup ...

    // Create installer for component operations
    gitOps := git.NewDefaultGitOperations()
    builder := build.NewDefaultBuildExecutor()
    installer := component.NewDefaultInstaller(gitOps, builder, componentDAO)

    // Create attack runner (if orchestrator available)
    var attackRunner *attack.AttackRunner
    if orchestrator != nil {
        attackRunner = attack.NewAttackRunner(
            orchestrator,
            registryAdapter,
            payloadRegistry,
            missionStore,
            findingStore,
        )
    }

    executorConfig := console.ExecutorConfig{
        DB:           db,
        ComponentDAO: componentDAO,
        FindingStore: findingStore,
        StreamManager: streamManager,
        HomeDir:      homeDir,
        ConfigFile:   configFile,
        // New dependencies
        Installer:    installer,
        AttackRunner: attackRunner,
    }

    // ... rest of setup ...
}
```

## Flag Parsing Helper Functions

Add utility functions for parsing flags from args:

```go
// containsFlag checks if a flag is present in args
func containsFlag(args []string, flag string) bool {
    for _, arg := range args {
        if arg == flag {
            return true
        }
    }
    return false
}

// getFlag gets the value of a flag from args
// Example: getFlag(args, "--agent") returns the value after --agent
func getFlag(args []string, flag string) string {
    for i, arg := range args {
        if arg == flag && i+1 < len(args) {
            return args[i+1]
        }
        // Handle --flag=value format
        if strings.HasPrefix(arg, flag+"=") {
            return strings.TrimPrefix(arg, flag+"=")
        }
    }
    return ""
}
```

## Error Handling Pattern

Standardize error result creation:

```go
func errorResult(err error) (*ExecutionResult, error) {
    return &ExecutionResult{
        Error:    err.Error(),
        IsError:  true,
        ExitCode: 1,
    }, nil
}

func usageResult(usage string) (*ExecutionResult, error) {
    return &ExecutionResult{
        Output:   usage + "\n",
        IsError:  true,
        ExitCode: 1,
    }, nil
}
```

## File Changes Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `internal/tui/console/executor.go` | Modify | Update 8 handler stubs to use real implementations |
| `internal/tui/console/helpers.go` | Create | Add `readLastNLines`, `containsFlag`, `getFlag`, `errorResult`, `usageResult` |
| `internal/tui/views/findings.go` | Modify | Update `exportFindings` to use real exporters |
| `internal/tui/views/mission.go` | Modify | Update `loadMissionLogs` to read real logs |
| `cmd/gibson/tui.go` | Modify | Inject Installer and AttackRunner dependencies |
| `internal/tui/console/config.go` | Modify | Add Installer, AttackRunner to ExecutorConfig |

## Testing Strategy

1. **Unit Tests:** Test each handler with mocked dependencies
2. **Integration Tests:** Test full command flow with real database
3. **Manual Testing:** Verify each command works identically to CLI

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Attack runner requires many dependencies | Make AttackRunner optional, show helpful error if not configured |
| Log files may not exist | Handle gracefully with informative messages |
| Large log files could slow TUI | Limit to last N lines by default |
| Export may fail on permission errors | Catch and display clear error messages |

## Success Criteria

1. All 8 stub handlers return real results instead of "not implemented"
2. `/agent logs`, `/tool logs`, `/plugin logs` display actual log content
3. `/agent install`, `/tool install`, `/plugin install` perform actual installation
4. `/agent uninstall`, `/tool uninstall`, `/plugin uninstall` perform actual uninstallation
5. `/attack` executes real attacks via AttackRunner
6. `/mission create -f` creates missions from workflow files
7. Findings export writes actual files in all supported formats
8. Mission view shows real execution logs or meaningful status
