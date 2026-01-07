package console

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zero-day-ai/gibson/cmd/gibson/core"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/attack"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/registry"
)

// deprecatedCommands maps old hyphenated command names to their migration messages.
// These commands have been removed in favor of the new slash command syntax.
var deprecatedCommands = map[string]string{
	"mission-create":   "Use '/mission run -f <file>' instead",
	"mission-start":    "Use '/mission run -f <file>' instead",
	"mission-stop":     "Use '/mission stop <name>' instead",
	"mission-delete":   "Use '/mission delete <name>' instead",
	"agent-start":      "Use '/agent start <name>' instead",
	"agent-stop":       "Use '/agent stop <name>' instead",
	"agent-install":    "Use '/agent install <source>' instead",
	"agent-uninstall":  "Use '/agent uninstall <name>' instead",
	"agent-execute":    "Use '/agent start <name>' then interact via steering commands",
	"agent-exec":       "Use '/agent start <name>' then interact via steering commands",
	"agent-run":        "Use '/agent start <name>' then interact via steering commands",
	"agent-logs":       "Use '/agent logs <name>' instead",
	"tool-start":       "Use '/tool start <name>' instead",
	"tool-stop":        "Use '/tool stop <name>' instead",
	"tool-install":     "Use '/tool install <source>' instead",
	"tool-uninstall":   "Use '/tool uninstall <name>' instead",
	"tool-logs":        "Use '/tool logs <name>' instead",
	"plugin-start":     "Use '/plugin start <name>' instead",
	"plugin-stop":      "Use '/plugin stop <name>' instead",
	"plugin-install":   "Use '/plugin install <source>' instead",
	"plugin-uninstall": "Use '/plugin uninstall <name>' instead",
}

// ExecutorConfig holds all dependencies and configuration needed for command execution.
type ExecutorConfig struct {
	// DB is the database connection for data persistence operations.
	DB *database.DB
	// ComponentDAO provides access to component data from SQLite.
	ComponentDAO database.ComponentDAO
	// FindingStore provides access to security findings.
	FindingStore finding.FindingStore
	// StreamManager manages bidirectional streams to agents.
	StreamManager *agent.StreamManager
	// Installer provides component installation functionality.
	Installer component.Installer
	// AttackRunner provides attack execution functionality.
	AttackRunner attack.AttackRunner
	// MissionStore provides access to mission data.
	MissionStore mission.MissionStore
	// RegManager provides access to the component registry.
	RegManager *registry.Manager
	// HomeDir is the Gibson home directory path (e.g., ~/.gibson).
	HomeDir string
	// ConfigFile is the path to the Gibson configuration file.
	ConfigFile string
}

// Executor handles command execution for the TUI console.
// It coordinates between the command registry and the Gibson CLI functionality.
type Executor struct {
	registry       *CommandRegistry
	config         ExecutorConfig
	ctx            context.Context
	focusedAgent   string // Currently focused agent for steering commands
	program        *tea.Program
	eventProcessor *EventProcessor
	subscriptionCh <-chan *database.StreamEvent
}

// GetFocusedAgent returns the name of the currently focused agent.
// Returns empty string if no agent is focused.
func (e *Executor) GetFocusedAgent() string {
	return e.focusedAgent
}

// SetFocusedAgent sets the currently focused agent for steering commands.
func (e *Executor) SetFocusedAgent(agentName string) {
	e.focusedAgent = agentName
}

// SetProgram sets the Bubble Tea program reference for sending messages.
// This must be called after the program is created to enable event streaming.
func (e *Executor) SetProgram(p *tea.Program) {
	e.program = p
}

// GetProgram returns the Bubble Tea program reference.
func (e *Executor) GetProgram() *tea.Program {
	return e.program
}

// NewExecutor creates a new command executor with the given configuration.
// The executor is responsible for parsing input, looking up commands in the registry,
// and invoking the appropriate handlers with the necessary dependencies.
func NewExecutor(ctx context.Context, registry *CommandRegistry, config ExecutorConfig) *Executor {
	return &Executor{
		registry: registry,
		config:   config,
		ctx:      ctx,
	}
}

// Execute parses and executes a slash command input string.
// It returns an ExecutionResult containing the output, errors, and timing information.
// If the command is not found, it returns an error result.
func (e *Executor) Execute(input string) (*ExecutionResult, error) {
	startTime := time.Now()

	// Parse the command input
	parsed, err := Parse(input)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to parse command: %v", err),
			IsError:  true,
			Duration: time.Since(startTime),
			ExitCode: 1,
		}, nil
	}

	// Look up the command in the registry
	cmd, found := e.registry.Get(parsed.Name)
	if !found {
		// Check if this is a deprecated command
		if suggestion, ok := deprecatedCommands[parsed.Name]; ok {
			return &ExecutionResult{
				Error:    fmt.Sprintf("Command '/%s' is deprecated. %s", parsed.Name, suggestion),
				IsError:  true,
				Duration: time.Since(startTime),
				ExitCode: 1,
			}, nil
		}

		return &ExecutionResult{
			Error:    fmt.Sprintf("Unknown command: /%s", parsed.Name),
			IsError:  true,
			Duration: time.Since(startTime),
			ExitCode: 1,
		}, nil
	}

	// Check if handler is registered
	if cmd.Handler == nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Command /%s has no handler registered", parsed.Name),
			IsError:  true,
			Duration: time.Since(startTime),
			ExitCode: 1,
		}, nil
	}

	// Build args list: prepend subcommand if present, then add flags
	args := parsed.Args
	if parsed.Subcommand != "" {
		args = append([]string{parsed.Subcommand}, parsed.Args...)
	}

	// Append flags to args so handlers can use GetFlag()
	for key, value := range parsed.Flags {
		if len(key) == 1 {
			// Short flag: -f value
			args = append(args, "-"+key, value)
		} else {
			// Long flag: --flag value
			args = append(args, "--"+key, value)
		}
	}

	// Execute the command handler
	result, err := cmd.Handler(e.ctx, args)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Command execution failed: %v", err),
			IsError:  true,
			Duration: time.Since(startTime),
			ExitCode: 1,
		}, err
	}

	// Update timing if not already set
	if result.Duration == 0 {
		result.Duration = time.Since(startTime)
	}

	return result, nil
}

// SetupHandlers registers all command handlers with their implementations.
// This wires up the slash commands to their actual functionality using the
// provided dependencies (DB, ComponentDAO, FindingStore).
func (e *Executor) SetupHandlers() {
	// Utility commands (TUI-only)
	if cmd, ok := e.registry.Get("help"); ok {
		cmd.Handler = e.handleHelp
	}
	if cmd, ok := e.registry.Get("clear"); ok {
		cmd.Handler = e.handleClear
	}
	if cmd, ok := e.registry.Get("status"); ok {
		cmd.Handler = e.handleStatus
	}
	if cmd, ok := e.registry.Get("config"); ok {
		cmd.Handler = e.handleConfig
	}

	// Mission commands - routed to core
	if cmd, ok := e.registry.Get("mission"); ok {
		cmd.Handler = e.handleMissionRouter
	}
	if cmd, ok := e.registry.Get("missions"); ok {
		cmd.Handler = e.handleMissionsList
	}

	// Agent commands - routed to core
	if cmd, ok := e.registry.Get("agent"); ok {
		cmd.Handler = e.handleComponentRouter(component.ComponentKindAgent)
	}
	if cmd, ok := e.registry.Get("agents"); ok {
		cmd.Handler = e.handleAgentsList
	}

	// Tool commands - routed to core
	if cmd, ok := e.registry.Get("tool"); ok {
		cmd.Handler = e.handleComponentRouter(component.ComponentKindTool)
	}
	if cmd, ok := e.registry.Get("tools"); ok {
		cmd.Handler = e.handleToolsList
	}

	// Plugin commands - routed to core
	if cmd, ok := e.registry.Get("plugin"); ok {
		cmd.Handler = e.handleComponentRouter(component.ComponentKindPlugin)
	}
	if cmd, ok := e.registry.Get("plugins"); ok {
		cmd.Handler = e.handlePluginsList
	}

	// TUI-specific steering commands
	if cmd, ok := e.registry.Get("focus"); ok {
		cmd.Handler = e.handleFocus
	}
	if cmd, ok := e.registry.Get("interrupt"); ok {
		cmd.Handler = e.handleInterrupt
	}
	if cmd, ok := e.registry.Get("mode"); ok {
		cmd.Handler = e.handleMode
	}
	if cmd, ok := e.registry.Get("pause"); ok {
		cmd.Handler = e.handlePause
	}
	if cmd, ok := e.registry.Get("resume"); ok {
		cmd.Handler = e.handleResume
	}

	// Attack command (TUI-specific)
	if cmd, ok := e.registry.Get("attack"); ok {
		cmd.Handler = e.handleAttack
	}

	// Legacy list commands (aliases)
	if cmd, ok := e.registry.Get("credentials"); ok {
		cmd.Handler = e.handleCredentials
	}
	if cmd, ok := e.registry.Get("findings"); ok {
		cmd.Handler = e.handleFindings
	}
	if cmd, ok := e.registry.Get("targets"); ok {
		cmd.Handler = e.handleTargets
	}
}

// buildCommandContext creates a core.CommandContext from the executor's configuration.
func (e *Executor) buildCommandContext() *core.CommandContext {
	// Create mission store if not provided (backward compatibility)
	missionStore := e.config.MissionStore
	if missionStore == nil && e.config.DB != nil {
		missionStore = mission.NewDBMissionStore(e.config.DB)
	}

	return &core.CommandContext{
		Ctx:          e.ctx,
		DB:           e.config.DB,
		DAO:          e.config.ComponentDAO,
		HomeDir:      e.config.HomeDir,
		Installer:    e.config.Installer,
		MissionStore: missionStore,
		RegManager:   e.config.RegManager,
		ConfigFile:   e.config.ConfigFile,
	}
}

// formatCoreResult converts a core.CommandResult to an ExecutionResult for the TUI.
func formatCoreResult(result *core.CommandResult) *ExecutionResult {
	if result.Error != nil {
		return &ExecutionResult{
			Error:    result.Error.Error(),
			IsError:  true,
			Duration: result.Duration,
			ExitCode: 1,
		}
	}

	// Format the data based on type
	output := result.Message
	if output == "" && result.Data != nil {
		output = fmt.Sprintf("%+v", result.Data)
	}

	return &ExecutionResult{
		Output:   output,
		IsError:  false,
		Duration: result.Duration,
		ExitCode: 0,
	}
}

// errorResult creates an error ExecutionResult with the given message.
func errorResult(message string) *ExecutionResult {
	return &ExecutionResult{
		Error:    message,
		IsError:  true,
		ExitCode: 1,
	}
}

// ============================================================================
// ROUTER HANDLERS - Route to core functions
// ============================================================================

// handleMissionRouter routes /mission subcommands to core functions.
func (e *Executor) handleMissionRouter(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) == 0 {
		return e.showMissionSubcommands()
	}

	subcommand := args[0]
	subArgs := args[1:]
	cc := e.buildCommandContext()
	defer cc.Close()

	var result *core.CommandResult
	var err error

	switch subcommand {
	case "list":
		statusFilter := GetFlag(subArgs, "--status")
		result, err = core.MissionList(cc, statusFilter)
		if err == nil {
			return e.formatMissionListResult(result)
		}

	case "show":
		if len(subArgs) == 0 {
			return errorResult("usage: /mission show <name>"), nil
		}
		result, err = core.MissionShow(cc, subArgs[0])
		if err == nil {
			return e.formatMissionShowResult(result)
		}

	case "run":
		file := GetFlag(subArgs, "--file")
		if file == "" {
			file = GetFlag(subArgs, "-f")
		}
		if file == "" {
			return errorResult("usage: /mission run --file <file> [--target <name>]"), nil
		}
		// Get optional target flag
		target := GetFlag(subArgs, "--target")
		result, err = core.MissionRun(cc, file, target)
		if err == nil {
			return e.formatMissionRunResult(result)
		}

	case "resume":
		if len(subArgs) == 0 {
			return errorResult("usage: /mission resume <name>"), nil
		}
		result, err = core.MissionResume(cc, subArgs[0])
		if err == nil {
			return formatCoreResult(result), nil
		}

	case "stop":
		if len(subArgs) == 0 {
			return errorResult("usage: /mission stop <name>"), nil
		}
		result, err = core.MissionStop(cc, subArgs[0])
		if err == nil {
			return formatCoreResult(result), nil
		}

	case "delete":
		if len(subArgs) == 0 {
			return errorResult("usage: /mission delete <name>"), nil
		}
		force := GetFlag(subArgs, "--force") == "true"
		result, err = core.MissionDelete(cc, subArgs[0], force)
		if err == nil {
			return formatCoreResult(result), nil
		}

	default:
		return errorResult(fmt.Sprintf("Unknown subcommand: %s", subcommand)), nil
	}

	if err != nil {
		return errorResult(err.Error()), err
	}

	return formatCoreResult(result), nil
}

// handleComponentRouter returns a handler that routes component subcommands to core functions.
func (e *Executor) handleComponentRouter(kind component.ComponentKind) func(context.Context, []string) (*ExecutionResult, error) {
	return func(ctx context.Context, args []string) (*ExecutionResult, error) {
		if len(args) == 0 {
			return e.showComponentSubcommands(kind)
		}

		subcommand := args[0]
		subArgs := args[1:]
		cc := e.buildCommandContext()
		defer cc.Close()

		var result *core.CommandResult
		var err error

		switch subcommand {
		case "list":
			opts := core.ListOptions{
				Local:  GetFlag(subArgs, "--local") == "true",
				Remote: GetFlag(subArgs, "--remote") == "true",
			}
			result, err = core.ComponentList(cc, kind, opts)
			if err == nil {
				return e.formatComponentListResult(kind, result)
			}

		case "show":
			if len(subArgs) == 0 {
				return errorResult(fmt.Sprintf("usage: /%s show <name>", kind)), nil
			}
			result, err = core.ComponentShow(cc, kind, subArgs[0])
			if err == nil {
				return e.formatComponentShowResult(result)
			}

		case "install":
			if len(subArgs) == 0 {
				return errorResult(fmt.Sprintf("usage: /%s install <source>", kind)), nil
			}
			source := subArgs[0]
			opts := core.InstallOptions{
				Branch:       GetFlag(subArgs, "--branch"),
				Tag:          GetFlag(subArgs, "--tag"),
				Force:        GetFlag(subArgs, "--force") == "true",
				SkipBuild:    GetFlag(subArgs, "--skip-build") == "true",
				SkipRegister: GetFlag(subArgs, "--skip-register") == "true",
			}
			result, err = core.ComponentInstall(cc, kind, source, opts)
			if err == nil {
				return formatCoreResult(result), nil
			}

		case "uninstall":
			if len(subArgs) == 0 {
				return errorResult(fmt.Sprintf("usage: /%s uninstall <name>", kind)), nil
			}
			opts := core.UninstallOptions{
				Force: GetFlag(subArgs, "--force") == "true",
			}
			result, err = core.ComponentUninstall(cc, kind, subArgs[0], opts)
			if err == nil {
				return formatCoreResult(result), nil
			}

		case "update":
			if len(subArgs) == 0 {
				return errorResult(fmt.Sprintf("usage: /%s update <name>", kind)), nil
			}
			opts := core.UpdateOptions{
				Restart:   GetFlag(subArgs, "--restart") == "true",
				SkipBuild: GetFlag(subArgs, "--skip-build") == "true",
			}
			result, err = core.ComponentUpdate(cc, kind, subArgs[0], opts)
			if err == nil {
				return formatCoreResult(result), nil
			}

		case "build":
			if len(subArgs) == 0 {
				return errorResult(fmt.Sprintf("usage: /%s build <name>", kind)), nil
			}
			result, err = core.ComponentBuild(cc, kind, subArgs[0])
			if err == nil {
				return formatCoreResult(result), nil
			}

		case "start":
			if len(subArgs) == 0 {
				return errorResult(fmt.Sprintf("usage: /%s start <name>", kind)), nil
			}
			// Component start now requires daemon
			return errorResult("Component start/stop now requires daemon. Use 'gibson daemon start' then 'gibson agent start <name>' from CLI."), nil

		case "stop":
			if len(subArgs) == 0 {
				return errorResult(fmt.Sprintf("usage: /%s stop <name>", kind)), nil
			}
			// Component stop now requires daemon
			return errorResult("Component start/stop now requires daemon. Use 'gibson daemon start' then 'gibson agent stop <name>' from CLI."), nil

		case "status":
			name := ""
			if len(subArgs) > 0 {
				name = subArgs[0]
			}
			opts := core.StatusOptions{
				JSON: GetFlag(subArgs, "--json") == "true",
			}
			result, err = core.ComponentStatus(cc, kind, name, opts)
			if err == nil {
				return e.formatComponentStatusResult(result)
			}

		case "logs":
			if len(subArgs) == 0 {
				return errorResult(fmt.Sprintf("usage: /%s logs <name>", kind)), nil
			}
			name := subArgs[0]
			lines := 50
			if linesStr := GetFlag(subArgs, "--lines"); linesStr != "" {
				fmt.Sscanf(linesStr, "%d", &lines)
			}
			opts := core.LogsOptions{
				Follow: GetFlag(subArgs, "--follow") == "true",
				Lines:  lines,
			}
			result, err = core.ComponentLogs(cc, kind, name, opts)
			if err == nil {
				return e.formatComponentLogsResult(result)
			}

		default:
			return errorResult(fmt.Sprintf("Unknown subcommand: %s", subcommand)), nil
		}

		if err != nil {
			return errorResult(err.Error()), err
		}

		return formatCoreResult(result), nil
	}
}

// ============================================================================
// FORMATTING HELPERS - Convert core results to TUI output
// ============================================================================

func (e *Executor) showMissionSubcommands() (*ExecutionResult, error) {
	output := `Usage: /mission <subcommand> [args]

Available subcommands:
  list [--status <status>]    List all missions
  show <name>                 Show mission details
  run --file <file>          Run a mission from workflow file
  resume <name>              Resume a paused mission
  stop <name>                Stop a running mission
  delete <name> [--force]    Delete a mission
`
	return &ExecutionResult{Output: output, ExitCode: 0}, nil
}

func (e *Executor) showComponentSubcommands(kind component.ComponentKind) (*ExecutionResult, error) {
	output := fmt.Sprintf(`Usage: /%s <subcommand> [args]

Available subcommands:
  list [--local|--remote]    List all %ss
  show <name>                Show %s details
  install <source>           Install a %s
  uninstall <name>           Uninstall a %s
  update <name>              Update a %s
  build <name>               Build a %s
  start <name>               Start a %s
  stop <name>                Stop a %s
  status [name]              Show %s status
  logs <name>                Show %s logs
`, kind, kind, kind, kind, kind, kind, kind, kind, kind, kind, kind)
	return &ExecutionResult{Output: output, ExitCode: 0}, nil
}

func (e *Executor) formatMissionListResult(result *core.CommandResult) (*ExecutionResult, error) {
	data, ok := result.Data.(*core.MissionListResult)
	if !ok {
		return formatCoreResult(result), nil
	}

	if len(data.Missions) == 0 {
		return &ExecutionResult{Output: "No missions found\n", ExitCode: 0}, nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("%-20s %-12s %-10s %-8s %s\n",
		"NAME", "STATUS", "PROGRESS", "FINDINGS", "TARGET"))
	output.WriteString(strings.Repeat("-", 80) + "\n")

	for _, m := range data.Missions {
		progress := fmt.Sprintf("%.1f%%", m.Progress*100)
		targetID := m.TargetID
		if targetID == "" {
			targetID = "-"
		}
		output.WriteString(fmt.Sprintf("%-20s %-12s %-10s %-8d %s\n",
			m.Name, m.Status, progress, m.FindingsCount, targetID))
	}

	return &ExecutionResult{Output: output.String(), ExitCode: 0}, nil
}

func (e *Executor) formatMissionShowResult(result *core.CommandResult) (*ExecutionResult, error) {
	m, ok := result.Data.(*mission.Mission)
	if !ok {
		return formatCoreResult(result), nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Mission: %s\n", m.Name))
	output.WriteString(strings.Repeat("=", 60) + "\n")
	output.WriteString(fmt.Sprintf("ID:          %s\n", m.ID))
	output.WriteString(fmt.Sprintf("Status:      %s\n", m.Status))
	output.WriteString(fmt.Sprintf("Progress:    %.1f%%\n", m.Progress*100))
	output.WriteString(fmt.Sprintf("Findings:    %d\n", m.FindingsCount))
	output.WriteString(fmt.Sprintf("Target:      %s\n", m.TargetID))
	output.WriteString(fmt.Sprintf("Workflow ID: %s\n", m.WorkflowID))
	output.WriteString(fmt.Sprintf("Created:     %s\n", m.CreatedAt.Format(time.RFC3339)))
	if m.StartedAt != nil {
		output.WriteString(fmt.Sprintf("Started:     %s\n", m.StartedAt.Format(time.RFC3339)))
	}
	if m.CompletedAt != nil {
		output.WriteString(fmt.Sprintf("Completed:   %s\n", m.CompletedAt.Format(time.RFC3339)))
	}
	if m.Description != "" {
		output.WriteString(fmt.Sprintf("\nDescription:\n%s\n", m.Description))
	}

	return &ExecutionResult{Output: output.String(), ExitCode: 0}, nil
}

func (e *Executor) formatMissionRunResult(result *core.CommandResult) (*ExecutionResult, error) {
	data, ok := result.Data.(*core.MissionRunResult)
	if !ok {
		return formatCoreResult(result), nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Mission '%s' started successfully\n", data.Mission.Name))
	output.WriteString(fmt.Sprintf("  Workflow: %d nodes, %d entry points, %d exit points\n",
		data.NodesCount, data.EntryPoints, data.ExitPoints))
	output.WriteString(fmt.Sprintf("  Status: %s\n", data.Status))

	return &ExecutionResult{Output: output.String(), ExitCode: 0}, nil
}

func (e *Executor) formatComponentListResult(kind component.ComponentKind, result *core.CommandResult) (*ExecutionResult, error) {
	components, ok := result.Data.([]*component.Component)
	if !ok {
		return formatCoreResult(result), nil
	}

	if len(components) == 0 {
		return &ExecutionResult{Output: fmt.Sprintf("No %ss installed\n", kind), ExitCode: 0}, nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("%-20s %-12s %-10s %-8s %s\n",
		"NAME", "VERSION", "STATUS", "PORT", "SOURCE"))
	output.WriteString(strings.Repeat("-", 80) + "\n")

	for _, comp := range components {
		port := "-"
		if comp.Port > 0 {
			port = fmt.Sprintf("%d", comp.Port)
		}
		output.WriteString(fmt.Sprintf("%-20s %-12s %-10s %-8s %s\n",
			comp.Name, comp.Version, comp.Status, port, comp.Source))
	}

	return &ExecutionResult{Output: output.String(), ExitCode: 0}, nil
}

func (e *Executor) formatComponentShowResult(result *core.CommandResult) (*ExecutionResult, error) {
	comp, ok := result.Data.(*component.Component)
	if !ok {
		return formatCoreResult(result), nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Component: %s\n", comp.Name))
	output.WriteString(strings.Repeat("=", 60) + "\n")
	output.WriteString(fmt.Sprintf("Kind:        %s\n", comp.Kind))
	output.WriteString(fmt.Sprintf("Version:     %s\n", comp.Version))
	output.WriteString(fmt.Sprintf("Status:      %s\n", comp.Status))
	output.WriteString(fmt.Sprintf("Source:      %s\n", comp.Source))
	if comp.Port > 0 {
		output.WriteString(fmt.Sprintf("Port:        %d\n", comp.Port))
	}
	if comp.PID > 0 {
		output.WriteString(fmt.Sprintf("PID:         %d\n", comp.PID))
	}
	output.WriteString(fmt.Sprintf("Repo Path:   %s\n", comp.RepoPath))
	output.WriteString(fmt.Sprintf("Binary Path: %s\n", comp.BinPath))

	return &ExecutionResult{Output: output.String(), ExitCode: 0}, nil
}

func (e *Executor) formatComponentStartResult(result *core.CommandResult) (*ExecutionResult, error) {
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		return formatCoreResult(result), nil
	}

	name := data["name"]
	pid := data["pid"]
	port := data["port"]

	output := fmt.Sprintf("Component '%s' started successfully (PID: %v, Port: %v)\n", name, pid, port)
	return &ExecutionResult{Output: output, ExitCode: 0}, nil
}

func (e *Executor) formatComponentStatusResult(result *core.CommandResult) (*ExecutionResult, error) {
	// This will be formatted based on whether it's single instance or multiple
	_, ok := result.Data.(map[string]interface{})
	if !ok {
		return formatCoreResult(result), nil
	}

	// For now, just use generic formatting
	// TODO: Add proper registry instance formatting
	return formatCoreResult(result), nil
}

func (e *Executor) formatComponentLogsResult(result *core.CommandResult) (*ExecutionResult, error) {
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		return formatCoreResult(result), nil
	}

	if follow, ok := data["follow"].(bool); ok && follow {
		// Follow mode - return special result
		return &ExecutionResult{
			Output:   fmt.Sprintf("Following logs from %s\n", data["log_path"]),
			ExitCode: 0,
		}, nil
	}

	if lines, ok := data["lines"].([]string); ok {
		output := strings.Join(lines, "\n") + "\n"
		return &ExecutionResult{Output: output, ExitCode: 0}, nil
	}

	return formatCoreResult(result), nil
}

// ============================================================================
// LIST COMMAND HANDLERS - Legacy aliases
// ============================================================================

func (e *Executor) handleMissionsList(ctx context.Context, args []string) (*ExecutionResult, error) {
	cc := e.buildCommandContext()
	defer cc.Close()

	statusFilter := GetFlag(args, "--status")
	result, err := core.MissionList(cc, statusFilter)
	if err != nil {
		return errorResult(err.Error()), err
	}

	return e.formatMissionListResult(result)
}

func (e *Executor) handleAgentsList(ctx context.Context, args []string) (*ExecutionResult, error) {
	cc := e.buildCommandContext()
	defer cc.Close()

	opts := core.ListOptions{
		Local:  GetFlag(args, "--local") == "true",
		Remote: GetFlag(args, "--remote") == "true",
	}
	result, err := core.ComponentList(cc, component.ComponentKindAgent, opts)
	if err != nil {
		return errorResult(err.Error()), err
	}

	return e.formatComponentListResult(component.ComponentKindAgent, result)
}

func (e *Executor) handleToolsList(ctx context.Context, args []string) (*ExecutionResult, error) {
	cc := e.buildCommandContext()
	defer cc.Close()

	opts := core.ListOptions{
		Local:  GetFlag(args, "--local") == "true",
		Remote: GetFlag(args, "--remote") == "true",
	}
	result, err := core.ComponentList(cc, component.ComponentKindTool, opts)
	if err != nil {
		return errorResult(err.Error()), err
	}

	return e.formatComponentListResult(component.ComponentKindTool, result)
}

func (e *Executor) handlePluginsList(ctx context.Context, args []string) (*ExecutionResult, error) {
	cc := e.buildCommandContext()
	defer cc.Close()

	opts := core.ListOptions{
		Local:  GetFlag(args, "--local") == "true",
		Remote: GetFlag(args, "--remote") == "true",
	}
	result, err := core.ComponentList(cc, component.ComponentKindPlugin, opts)
	if err != nil {
		return errorResult(err.Error()), err
	}

	return e.formatComponentListResult(component.ComponentKindPlugin, result)
}

// ============================================================================
// UTILITY HANDLERS - TUI-only commands
// ============================================================================

func (e *Executor) handleHelp(ctx context.Context, args []string) (*ExecutionResult, error) {
	var output strings.Builder

	output.WriteString("Available Commands:\n\n")

	commands := e.registry.List()
	for _, cmd := range commands {
		output.WriteString(fmt.Sprintf("  /%s\n", cmd.Name))
		output.WriteString(fmt.Sprintf("    %s\n", cmd.Description))
		if cmd.Usage != "" {
			output.WriteString(fmt.Sprintf("    Usage: %s\n", cmd.Usage))
		}
		output.WriteString("\n")
	}

	return &ExecutionResult{
		Output:   output.String(),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

func (e *Executor) handleClear(ctx context.Context, args []string) (*ExecutionResult, error) {
	return &ExecutionResult{
		Output:   "", // Empty output signals the view to clear
		IsError:  false,
		ExitCode: 0,
	}, nil
}

func (e *Executor) handleStatus(ctx context.Context, args []string) (*ExecutionResult, error) {
	var output strings.Builder

	output.WriteString("Gibson System Status\n")
	output.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Database status
	if e.config.DB != nil {
		if err := e.config.DB.Health(ctx); err != nil {
			output.WriteString(fmt.Sprintf("Database: ERROR (%v)\n", err))
		} else {
			stats := e.config.DB.Stats()
			output.WriteString("Database: OK\n")
			output.WriteString(fmt.Sprintf("  Connections: %d open, %d in use, %d idle\n",
				stats.OpenConnections, stats.InUse, stats.Idle))
		}
	} else {
		output.WriteString("Database: Not available\n")
	}

	output.WriteString("\n")

	// Component DAO status
	if e.config.ComponentDAO != nil {
		agents, _ := e.config.ComponentDAO.List(ctx, component.ComponentKindAgent)
		tools, _ := e.config.ComponentDAO.List(ctx, component.ComponentKindTool)
		plugins, _ := e.config.ComponentDAO.List(ctx, component.ComponentKindPlugin)

		output.WriteString("Components:\n")
		output.WriteString(fmt.Sprintf("  Agents:  %d installed\n", len(agents)))
		output.WriteString(fmt.Sprintf("  Tools:   %d installed\n", len(tools)))
		output.WriteString(fmt.Sprintf("  Plugins: %d installed\n", len(plugins)))
	} else {
		output.WriteString("Components: Not available\n")
	}

	output.WriteString("\n")

	// Configuration
	output.WriteString("Configuration:\n")
	output.WriteString(fmt.Sprintf("  Home Directory: %s\n", e.config.HomeDir))
	output.WriteString(fmt.Sprintf("  Config File:    %s\n", e.config.ConfigFile))

	return &ExecutionResult{
		Output:   output.String(),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

func (e *Executor) handleConfig(ctx context.Context, args []string) (*ExecutionResult, error) {
	var output strings.Builder

	output.WriteString("Gibson Configuration\n")
	output.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Read config file
	if e.config.ConfigFile != "" {
		if content, err := os.ReadFile(e.config.ConfigFile); err == nil {
			output.WriteString(string(content))
		} else {
			output.WriteString(fmt.Sprintf("Error reading config file: %v\n", err))
		}
	} else {
		output.WriteString("No config file specified\n")
	}

	return &ExecutionResult{
		Output:   output.String(),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

func (e *Executor) handleCredentials(ctx context.Context, args []string) (*ExecutionResult, error) {
	return &ExecutionResult{
		Output:   "Credentials feature not yet implemented\n",
		IsError:  false,
		ExitCode: 0,
	}, nil
}

func (e *Executor) handleFindings(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.config.FindingStore == nil {
		return &ExecutionResult{
			Output:   "Finding store not available\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	// List all findings - pass empty mission ID to get all findings across all missions
	// Note: This may need to be updated if we want mission-specific filtering
	findings, err := e.config.FindingStore.List(ctx, "", nil)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to list findings: %v", err)), err
	}

	if len(findings) == 0 {
		return &ExecutionResult{
			Output:   "No findings\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("%-40s %-10s %-15s %s\n",
		"ID", "SEVERITY", "CATEGORY", "TITLE"))
	output.WriteString(strings.Repeat("-", 80) + "\n")

	for _, f := range findings {
		output.WriteString(fmt.Sprintf("%-40s %-10s %-15s %s\n",
			f.ID, f.Severity, f.Category, f.Title))
	}

	return &ExecutionResult{
		Output:   output.String(),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

func (e *Executor) handleTargets(ctx context.Context, args []string) (*ExecutionResult, error) {
	return &ExecutionResult{
		Output:   "Targets feature not yet implemented\n",
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// ============================================================================
// TUI-SPECIFIC HANDLERS - Steering and attack
// ============================================================================

func (e *Executor) handleFocus(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) == 0 {
		if e.focusedAgent == "" {
			return &ExecutionResult{
				Output:   "No agent currently focused\n",
				ExitCode: 0,
			}, nil
		}
		return &ExecutionResult{
			Output:   fmt.Sprintf("Currently focused on agent: %s\n", e.focusedAgent),
			ExitCode: 0,
		}, nil
	}

	agentName := args[0]
	e.focusedAgent = agentName

	return &ExecutionResult{
		Output:   fmt.Sprintf("Focused on agent: %s\n", agentName),
		ExitCode: 0,
	}, nil
}

func (e *Executor) handleInterrupt(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.focusedAgent == "" {
		return errorResult("No agent focused. Use /focus <agent> first"), nil
	}

	message := "interrupt requested"
	if len(args) > 0 {
		message = strings.Join(args, " ")
	}

	if e.config.StreamManager == nil {
		return errorResult("Stream manager not available"), nil
	}

	// Send interrupt to focused agent
	if err := e.config.StreamManager.SendInterrupt(e.focusedAgent, message); err != nil {
		return errorResult(fmt.Sprintf("Failed to send interrupt: %v", err)), err
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Sent interrupt to %s: %s\n", e.focusedAgent, message),
		ExitCode: 0,
	}, nil
}

func (e *Executor) handleMode(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.focusedAgent == "" {
		return errorResult("No agent focused. Use /focus <agent> first"), nil
	}

	if len(args) == 0 {
		return errorResult("Usage: /mode <mode>"), nil
	}

	mode := args[0]

	if e.config.StreamManager == nil {
		return errorResult("Stream manager not available"), nil
	}

	// Send mode change to focused agent
	// Convert string mode to database.AgentMode type
	agentMode := database.AgentMode(mode)
	if err := e.config.StreamManager.SetMode(e.focusedAgent, agentMode); err != nil {
		return errorResult(fmt.Sprintf("Failed to change mode: %v", err)), err
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Changed mode for %s to: %s\n", e.focusedAgent, mode),
		ExitCode: 0,
	}, nil
}

func (e *Executor) handlePause(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.focusedAgent == "" {
		return errorResult("No agent focused. Use /focus <agent> first"), nil
	}

	if e.config.StreamManager == nil {
		return errorResult("Stream manager not available"), nil
	}

	// Use SendInterrupt with "pause" reason as workaround
	if err := e.config.StreamManager.SendInterrupt(e.focusedAgent, "pause requested"); err != nil {
		return errorResult(fmt.Sprintf("Failed to pause agent: %v", err)), err
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Paused agent: %s\n", e.focusedAgent),
		ExitCode: 0,
	}, nil
}

func (e *Executor) handleResume(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.focusedAgent == "" {
		return errorResult("No agent focused. Use /focus <agent> first"), nil
	}

	if e.config.StreamManager == nil {
		return errorResult("Stream manager not available"), nil
	}

	guidance := ""
	if len(args) > 0 {
		guidance = strings.Join(args, " ")
	}

	if err := e.config.StreamManager.Resume(e.focusedAgent, guidance); err != nil {
		return errorResult(fmt.Sprintf("Failed to resume agent: %v", err)), err
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Resumed agent: %s\n", e.focusedAgent),
		ExitCode: 0,
	}, nil
}

func (e *Executor) handleAttack(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.config.AttackRunner == nil {
		return errorResult("Attack runner not available"), nil
	}

	if len(args) == 0 {
		return errorResult("Usage: /attack <attack-name> [args...]"), nil
	}

	attackName := args[0]
	attackArgs := args[1:]

	// This is a simplified version - actual implementation would need to:
	// 1. Parse attack configuration
	// 2. Start attack execution
	// 3. Stream results back to TUI
	_ = attackName
	_ = attackArgs

	return &ExecutionResult{
		Output:   "Attack execution not yet fully implemented in TUI\n",
		ExitCode: 0,
	}, nil
}
