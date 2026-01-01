package console

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/component/build"
	"github.com/zero-day-ai/gibson/internal/component/git"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/mission"
)

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

	// Build args list: prepend subcommand if present
	args := parsed.Args
	if parsed.Subcommand != "" {
		args = append([]string{parsed.Subcommand}, parsed.Args...)
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
	// /help - Show available commands
	if cmd, ok := e.registry.Get("help"); ok {
		cmd.Handler = e.handleHelp
	}

	// /clear - Clear console output
	if cmd, ok := e.registry.Get("clear"); ok {
		cmd.Handler = e.handleClear
	}

	// /status - Show system status
	if cmd, ok := e.registry.Get("status"); ok {
		cmd.Handler = e.handleStatus
	}

	// /agents - List agents (alias)
	if cmd, ok := e.registry.Get("agents"); ok {
		cmd.Handler = e.handleAgents
	}

	// /agent - Agent commands (with subcommand routing)
	if cmd, ok := e.registry.Get("agent"); ok {
		cmd.Handler = e.handleAgents
	}

	// Standalone agent commands
	if cmd, ok := e.registry.Get("agent-start"); ok {
		cmd.Handler = e.handleAgentStart
	}
	if cmd, ok := e.registry.Get("agent-execute"); ok {
		cmd.Handler = e.handleAgentExecute
	}
	if cmd, ok := e.registry.Get("agent-stop"); ok {
		cmd.Handler = e.handleAgentStop
	}
	if cmd, ok := e.registry.Get("agent-install"); ok {
		cmd.Handler = e.handleAgentInstall
	}
	if cmd, ok := e.registry.Get("agent-uninstall"); ok {
		cmd.Handler = e.handleAgentUninstall
	}
	if cmd, ok := e.registry.Get("agent-logs"); ok {
		cmd.Handler = e.handleAgentLogs
	}

	// /missions - List missions
	if cmd, ok := e.registry.Get("missions"); ok {
		cmd.Handler = e.handleMissions
	}

	// /credentials - List credentials
	if cmd, ok := e.registry.Get("credentials"); ok {
		cmd.Handler = e.handleCredentials
	}

	// /findings - List findings
	if cmd, ok := e.registry.Get("findings"); ok {
		cmd.Handler = e.handleFindings
	}

	// /targets - List targets
	if cmd, ok := e.registry.Get("targets"); ok {
		cmd.Handler = e.handleTargets
	}

	if cmd, ok := e.registry.Get("tool"); ok {
		cmd.Handler = e.handleTool
	}
	// /tool - Tool commands (with subcommand routing)

	// /tools - List tools
	if cmd, ok := e.registry.Get("tools"); ok {
		cmd.Handler = e.handleTools
	}

	// Standalone tool commands
	if cmd, ok := e.registry.Get("tool-start"); ok {
		cmd.Handler = e.handleToolStart
	}
	if cmd, ok := e.registry.Get("tool-stop"); ok {
		cmd.Handler = e.handleToolStop
	}
	if cmd, ok := e.registry.Get("tool-install"); ok {
		cmd.Handler = e.handleToolInstall
	}
	if cmd, ok := e.registry.Get("tool-uninstall"); ok {
		cmd.Handler = e.handleToolUninstall
	}
	if cmd, ok := e.registry.Get("tool-logs"); ok {
		cmd.Handler = e.handleToolLogs
	}

	// /plugin - Plugin commands (with subcommand routing)
	if cmd, ok := e.registry.Get("plugin"); ok {
		cmd.Handler = e.handlePlugin
	}

	// /plugins - List plugins
	if cmd, ok := e.registry.Get("plugins"); ok {
		cmd.Handler = e.handlePlugins
	}

	// Standalone plugin commands
	if cmd, ok := e.registry.Get("plugin-start"); ok {
		cmd.Handler = e.handlePluginStart
	}
	if cmd, ok := e.registry.Get("plugin-stop"); ok {
		cmd.Handler = e.handlePluginStop
	}
	if cmd, ok := e.registry.Get("plugin-install"); ok {
		cmd.Handler = e.handlePluginInstall
	}
	if cmd, ok := e.registry.Get("plugin-uninstall"); ok {
		cmd.Handler = e.handlePluginUninstall
	}

	// /config - Show configuration
	if cmd, ok := e.registry.Get("config"); ok {
		cmd.Handler = e.handleConfig
	}

	// /attack - Launch single-agent attack
	if cmd, ok := e.registry.Get("attack"); ok {
		cmd.Handler = e.handleAttack
	}

	// /mission - Mission management (router)
	if cmd, ok := e.registry.Get("mission"); ok {
		cmd.Handler = e.handleMission
	}

	// /mission-start - Start a mission
	if cmd, ok := e.registry.Get("mission-start"); ok {
		cmd.Handler = e.handleMissionStart
	}

	// /mission-stop - Stop a mission
	if cmd, ok := e.registry.Get("mission-stop"); ok {
		cmd.Handler = e.handleMissionStop
	}

	// /mission-create - Create a mission
	if cmd, ok := e.registry.Get("mission-create"); ok {
		cmd.Handler = e.handleMissionCreate
	}

	// /mission-delete - Delete a mission
	if cmd, ok := e.registry.Get("mission-delete"); ok {
		cmd.Handler = e.handleMissionDelete
	}

	// Steering commands
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
}

// handleHelp displays a list of available commands with their descriptions.
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

// handleClear returns an empty result, allowing the view to handle clearing.
func (e *Executor) handleClear(ctx context.Context, args []string) (*ExecutionResult, error) {
	return &ExecutionResult{
		Output:   "", // Empty output signals the view to clear
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleStatus displays system status information.
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

// handleAgents routes to appropriate subcommand handler or lists agents.
func (e *Executor) handleAgents(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) == 0 {
		return e.handleAgentsList(ctx, args)
	}

	switch args[0] {
	case "list":
		return e.handleAgentsList(ctx, args[1:])
	case "focus":
		return e.handleAgentsFocus(ctx, args[1:])
	case "mode":
		return e.handleAgentsMode(ctx, args[1:])
	case "interrupt":
		return e.handleAgentsInterrupt(ctx, args[1:])
	case "status":
		return e.handleAgentsStatus(ctx, args[1:])
	case "start":
		return e.handleAgentStart(ctx, args[1:])
	case "stop":
		return e.handleAgentStop(ctx, args[1:])
	case "install":
		return e.handleAgentInstall(ctx, args[1:])
	case "uninstall":
		return e.handleAgentUninstall(ctx, args[1:])
	case "logs":
		return e.handleAgentLogs(ctx, args[1:])
	default:
		// Treat as agent name for backward compatibility
		return e.handleAgentsList(ctx, args)
	}
}

// handleAgentsList lists all registered agents from the component DAO.
func (e *Executor) handleAgentsList(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.config.ComponentDAO == nil {
		return &ExecutionResult{
			Output:   "Database not available. Run 'gibson init' to initialize.\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	agents, err := e.config.ComponentDAO.List(ctx, component.ComponentKindAgent)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to list agents: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	if len(agents) == 0 {
		return &ExecutionResult{
			Output:   "No agents installed\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("%-20s %-12s %-10s %-8s %s\n",
		"NAME", "VERSION", "STATUS", "PORT", "SOURCE"))
	output.WriteString(strings.Repeat("-", 80) + "\n")

	for _, agent := range agents {
		port := "-"
		if agent.Port > 0 {
			port = fmt.Sprintf("%d", agent.Port)
		}

		output.WriteString(fmt.Sprintf("%-20s %-12s %-10s %-8s %s\n",
			agent.Name,
			agent.Version,
			agent.Status,
			port,
			agent.Source,
		))
	}

	return &ExecutionResult{
		Output:   output.String(),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleAgentsFocus switches TUI focus to specific agent.
func (e *Executor) handleAgentsFocus(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /agents focus <agent-name>\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Return message indicating focus should switch (TUI handles actual focus)
	return &ExecutionResult{
		Output:   fmt.Sprintf("Focusing on agent: %s\nAgent output will appear in the console view.\n", args[0]),
		ExitCode: 0,
	}, nil
}

// handleAgentsMode changes agent mode (autonomous/interactive).
func (e *Executor) handleAgentsMode(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 2 {
		return &ExecutionResult{
			Output:   "Usage: /agents mode <agent-name> <autonomous|interactive>\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	agentName := args[0]
	mode := args[1]

	if e.config.StreamManager == nil {
		return &ExecutionResult{
			Output:   "Stream manager not available\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	var agentMode database.AgentMode
	switch mode {
	case "autonomous", "auto":
		agentMode = database.AgentModeAutonomous
	case "interactive":
		agentMode = database.AgentModeInteractive
	default:
		return &ExecutionResult{
			Output:   fmt.Sprintf("Invalid mode: %s. Use 'autonomous' or 'interactive'\n", mode),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	if err := e.config.StreamManager.SetMode(agentName, agentMode); err != nil {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Failed to set mode: %v\n", err),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Agent %s mode set to %s\n", agentName, mode),
		ExitCode: 0,
	}, nil
}

// handleAgentsInterrupt sends interrupt to agent.
func (e *Executor) handleAgentsInterrupt(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /agents interrupt <agent-name> [reason]\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	agentName := args[0]
	reason := "User requested interrupt"
	if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}

	if e.config.StreamManager == nil {
		return &ExecutionResult{
			Output:   "Stream manager not available\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	if err := e.config.StreamManager.SendInterrupt(agentName, reason); err != nil {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Failed to interrupt agent: %v\n", err),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Interrupt sent to agent %s\n", agentName),
		ExitCode: 0,
	}, nil
}

// handleAgentsStatus shows detailed status of an agent.
func (e *Executor) handleAgentsStatus(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /agents status <agent-name>\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	agentName := args[0]

	if e.config.StreamManager == nil {
		return &ExecutionResult{
			Output:   "Stream manager not available\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	session, err := e.config.StreamManager.GetSession(agentName)
	if err != nil {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Agent not found: %s\n", agentName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Agent: %s\n", session.AgentName))
	output.WriteString(fmt.Sprintf("Status: %s\n", session.Status))
	output.WriteString(fmt.Sprintf("Mode: %s\n", session.Mode))
	output.WriteString(fmt.Sprintf("Session ID: %s\n", session.ID))
	output.WriteString(fmt.Sprintf("Started: %s\n", session.StartedAt.Format(time.RFC3339)))
	if session.EndedAt != nil {
		output.WriteString(fmt.Sprintf("Ended: %s\n", session.EndedAt.Format(time.RFC3339)))
	}

	return &ExecutionResult{
		Output:   output.String(),
		ExitCode: 0,
	}, nil
}

// handleMissions lists all missions from the database.
func (e *Executor) handleMissions(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.config.DB == nil {
		return &ExecutionResult{
			Output:   "Database not available\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	missionStore := mission.NewDBMissionStore(e.config.DB)
	missions, err := missionStore.List(ctx, nil)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to list missions: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	if len(missions) == 0 {
		return &ExecutionResult{
			Output:   "No missions found\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("%-30s %-12s %-10s %-10s %s\n",
		"NAME", "STATUS", "PROGRESS", "FINDINGS", "CREATED"))
	output.WriteString(strings.Repeat("-", 80) + "\n")

	for _, mission := range missions {
		progress := fmt.Sprintf("%.1f%%", mission.Progress*100)
		created := mission.CreatedAt.Format("2006-01-02")

		output.WriteString(fmt.Sprintf("%-30s %-12s %-10s %-10d %s\n",
			mission.Name,
			mission.Status,
			progress,
			mission.FindingsCount,
			created,
		))
	}

	return &ExecutionResult{
		Output:   output.String(),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleCredentials lists all credentials from the database.
func (e *Executor) handleCredentials(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.config.DB == nil {
		return &ExecutionResult{
			Output:   "Database not available\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	credentialDAO := database.NewCredentialDAO(e.config.DB)
	credentials, err := credentialDAO.List(ctx, nil)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to list credentials: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	if len(credentials) == 0 {
		return &ExecutionResult{
			Output:   "No credentials found\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("%-25s %-15s %-15s %-10s %s\n",
		"NAME", "TYPE", "PROVIDER", "STATUS", "DESCRIPTION"))
	output.WriteString(strings.Repeat("-", 90) + "\n")

	for _, cred := range credentials {
		description := cred.Description
		if len(description) > 30 {
			description = description[:27] + "..."
		}

		output.WriteString(fmt.Sprintf("%-25s %-15s %-15s %-10s %s\n",
			cred.Name,
			cred.Type,
			cred.Provider,
			cred.Status,
			description,
		))
	}

	return &ExecutionResult{
		Output:   output.String(),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleFindings lists security findings from the finding store.
func (e *Executor) handleFindings(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.config.FindingStore == nil {
		return &ExecutionResult{
			Output:   "Finding store not available\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	// Note: This is a simplified implementation that would need to be enhanced
	// with mission filtering once we have a way to get the current mission context
	return &ExecutionResult{
		Output:   "Finding list functionality requires mission context\nUse the Findings view for full functionality\n",
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleTargets lists all targets from the database.
func (e *Executor) handleTargets(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.config.DB == nil {
		return &ExecutionResult{
			Output:   "Database not available\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	targetDAO := database.NewTargetDAO(e.config.DB)
	targets, err := targetDAO.List(ctx, nil)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to list targets: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	if len(targets) == 0 {
		return &ExecutionResult{
			Output:   "No targets found\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("%-25s %-15s %-15s %-10s %s\n",
		"NAME", "TYPE", "PROVIDER", "STATUS", "URL"))
	output.WriteString(strings.Repeat("-", 90) + "\n")

	for _, target := range targets {
		url := target.URL
		if len(url) > 35 {
			url = url[:32] + "..."
		}

		output.WriteString(fmt.Sprintf("%-25s %-15s %-15s %-10s %s\n",
			target.Name,
			target.Type,
			target.Provider,
			target.Status,
			url,
		))
	}

	return &ExecutionResult{
		Output:   output.String(),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleTools lists all registered tools from the component DAO.
func (e *Executor) handleTools(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.config.ComponentDAO == nil {
		return &ExecutionResult{
			Output:   "Database not available. Run 'gibson init' to initialize.\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	tools, err := e.config.ComponentDAO.List(ctx, component.ComponentKindTool)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to list tools: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	if len(tools) == 0 {
		return &ExecutionResult{
			Output:   "No tools installed\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("%-20s %-12s %-10s %s\n",
		"NAME", "VERSION", "STATUS", "SOURCE"))
	output.WriteString(strings.Repeat("-", 70) + "\n")

	for _, tool := range tools {
		output.WriteString(fmt.Sprintf("%-20s %-12s %-10s %s\n",
			tool.Name,
			tool.Version,
			tool.Status,
			tool.Source,
		))
	}

	return &ExecutionResult{
		Output:   output.String(),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handlePlugins lists all registered plugins from the component DAO.
func (e *Executor) handlePlugins(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.config.ComponentDAO == nil {
		return &ExecutionResult{
			Output:   "Database not available. Run 'gibson init' to initialize.\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	plugins, err := e.config.ComponentDAO.List(ctx, component.ComponentKindPlugin)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to list plugins: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	if len(plugins) == 0 {
		return &ExecutionResult{
			Output:   "No plugins installed\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("%-20s %-12s %-10s %s\n",
		"NAME", "VERSION", "STATUS", "SOURCE"))
	output.WriteString(strings.Repeat("-", 70) + "\n")

	for _, plugin := range plugins {
		output.WriteString(fmt.Sprintf("%-20s %-12s %-10s %s\n",
			plugin.Name,
			plugin.Version,
			plugin.Status,
			plugin.Source,
		))
	}

	return &ExecutionResult{
		Output:   output.String(),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleConfig displays configuration information.
func (e *Executor) handleConfig(ctx context.Context, args []string) (*ExecutionResult, error) {
	var output strings.Builder

	output.WriteString("Gibson Configuration\n")
	output.WriteString(strings.Repeat("=", 60) + "\n\n")

	output.WriteString("Paths:\n")
	output.WriteString(fmt.Sprintf("  Home Directory: %s\n", e.config.HomeDir))
	output.WriteString(fmt.Sprintf("  Config File:    %s\n", e.config.ConfigFile))

	output.WriteString("\n")
	output.WriteString("Database:\n")
	if e.config.DB != nil {
		output.WriteString(fmt.Sprintf("  Path:           %s\n", e.config.DB.Path()))
		output.WriteString("  Status:         Connected\n")
	} else {
		output.WriteString("  Status:         Not available\n")
	}

	output.WriteString("\n")
	output.WriteString("Components:\n")
	if e.config.ComponentDAO != nil {
		output.WriteString("  DAO:            Available\n")
		allComponents, _ := e.config.ComponentDAO.ListAll(ctx)
		totalCount := 0
		for _, components := range allComponents {
			totalCount += len(components)
		}
		output.WriteString(fmt.Sprintf("  Total:          %d components\n", totalCount))
	} else {
		output.WriteString("  DAO:            Not available\n")
	}

	output.WriteString("\n")
	output.WriteString("Findings:\n")
	if e.config.FindingStore != nil {
		output.WriteString("  Store:          Available\n")
	} else {
		output.WriteString("  Store:          Not available\n")
	}

	return &ExecutionResult{
		Output:   output.String(),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleAgentStart starts an agent component.
func (e *Executor) handleAgentStart(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Error:    "Usage: /agents start <agent-name>",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	agentName := args[0]

	if e.config.ComponentDAO == nil {
		return &ExecutionResult{
			Error:    "Database not available. Run 'gibson init' to initialize.",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Get the agent component
	agent, err := e.config.ComponentDAO.GetByName(ctx, component.ComponentKindAgent, agentName)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to get agent: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}
	if agent == nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Agent '%s' not found", agentName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Check if already running
	if agent.IsRunning() {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Agent '%s' is already running (PID: %d)", agentName, agent.PID),
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	// Create lifecycle manager
	healthMonitor := component.NewHealthMonitor()
	localTracker := component.NewDefaultLocalTracker()
	lifecycleManager := component.NewLifecycleManager(healthMonitor, e.config.ComponentDAO, nil, localTracker)

	// Start the agent
	port, err := lifecycleManager.StartComponent(ctx, agent)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to start agent: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Agent '%s' started successfully\n", agentName))
	output.WriteString(fmt.Sprintf("PID: %d\n", agent.PID))
	output.WriteString(fmt.Sprintf("Port: %d\n", port))

	return &ExecutionResult{
		Output:   output.String(),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleAgentExecute executes a task on a running agent with streaming output.
func (e *Executor) handleAgentExecute(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 2 {
		return &ExecutionResult{
			Error:    "Usage: /agent-execute <agent-name> <goal>",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	agentName := args[0]
	goal := strings.Join(args[1:], " ")

	if e.config.ComponentDAO == nil {
		return &ExecutionResult{
			Error:    "Database not available. Run 'gibson init' to initialize.",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	if e.config.StreamManager == nil {
		return &ExecutionResult{
			Error:    "Stream manager not available. Agent streaming is disabled.",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Get the agent component
	agentComp, err := e.config.ComponentDAO.GetByName(ctx, component.ComponentKindAgent, agentName)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to get agent: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}
	if agentComp == nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Agent '%s' not found. Use /agent-install to install it first.", agentName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Check if agent is running
	if !agentComp.IsRunning() {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Agent '%s' is not running. Use /agent-start %s to start it first.", agentName, agentName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Create gRPC connection to the agent
	address := fmt.Sprintf("localhost:%d", agentComp.Port)
	grpcClient, err := agent.NewGRPCAgentClient(address)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to connect to agent: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	// Build task using NewTask helper
	task := agent.NewTask("tui-task", goal, map[string]any{
		"source": "tui",
	})

	// Serialize task to JSON
	taskJSON, err := json.Marshal(task)
	if err != nil {
		grpcClient.Close()
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to serialize task: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	// Connect to streaming
	sessionID, err := e.config.StreamManager.Connect(ctx, agentName, grpcClient.Connection(), string(taskJSON), database.AgentModeAutonomous)
	if err != nil {
		grpcClient.Close()
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to start streaming session: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	// Set focused agent and subscribe to events
	e.focusedAgent = agentName
	if err := e.subscribeFocusedAgent(agentName); err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to subscribe to agent events: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Started streaming execution on agent '%s'\n", agentName))
	output.WriteString(fmt.Sprintf("Session ID: %s\n", sessionID))
	output.WriteString(fmt.Sprintf("Goal: %s\n", goal))
	output.WriteString("\nStreaming output will appear below...")

	return &ExecutionResult{
		Output:   output.String(),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleAgentStop stops a running agent component.
func (e *Executor) handleAgentStop(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Error:    "Usage: /agents stop <agent-name>",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	agentName := args[0]

	if e.config.ComponentDAO == nil {
		return &ExecutionResult{
			Error:    "Database not available. Run 'gibson init' to initialize.",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Get the agent component
	agent, err := e.config.ComponentDAO.GetByName(ctx, component.ComponentKindAgent, agentName)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to get agent: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}
	if agent == nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Agent '%s' not found", agentName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Check if running
	if !agent.IsRunning() {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Agent '%s' is not running", agentName),
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	// Create lifecycle manager
	healthMonitor := component.NewHealthMonitor()
	localTracker := component.NewDefaultLocalTracker()
	lifecycleManager := component.NewLifecycleManager(healthMonitor, e.config.ComponentDAO, nil, localTracker)

	// Stop the agent
	if err := lifecycleManager.StopComponent(ctx, agent); err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to stop agent: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Agent '%s' stopped successfully", agentName),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleAgentInstall installs an agent from a git repository.
func (e *Executor) handleAgentInstall(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Error:    "Usage: /agents install <repo-url>",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	repoURL := args[0]

	if e.config.ComponentDAO == nil || e.config.DB == nil {
		return &ExecutionResult{
			Error:    "Component DAO or database not available",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Note: For the TUI, we'll use default install options
	// In a full implementation, you might parse flags from args
	opts := component.InstallOptions{
		Force:        false,
		SkipBuild:    false,
		SkipRegister: false,
	}

	// Create installer with dependencies
	gitOps := git.NewDefaultGitOperations()
	builder := build.NewDefaultBuildExecutor()
	installer := component.NewDefaultInstaller(gitOps, builder, e.config.ComponentDAO)

	// Perform the installation
	result, err := installer.Install(ctx, repoURL, component.ComponentKindAgent, opts)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Installation failed: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Agent '%s' (v%s) installed successfully in %v\n",
		result.Component.Name,
		result.Component.Version,
		result.Duration))

	if result.BuildOutput != "" {
		output.WriteString("\nBuild output:\n")
		output.WriteString(result.BuildOutput)
	}

	return &ExecutionResult{
		Output:   output.String(),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleAgentUninstall uninstalls an agent component.
func (e *Executor) handleAgentUninstall(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Error:    "Usage: /agents uninstall <agent-name> --confirm",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	agentName := args[0]

	// Check for --confirm flag (we need to parse from args manually here)
	// In a full implementation, this would use ParsedCommand.Flags
	hasConfirm := false
	for _, arg := range args {
		if arg == "--confirm" {
			hasConfirm = true
			break
		}
	}

	if !hasConfirm {
		return &ExecutionResult{
			Error:    "Uninstall requires --confirm flag for safety",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	if e.config.ComponentDAO == nil {
		return &ExecutionResult{
			Error:    "Database not available. Run 'gibson init' to initialize.",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Check if component exists
	existing, err := e.config.ComponentDAO.GetByName(ctx, component.ComponentKindAgent, agentName)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to get agent: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}
	if existing == nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Agent '%s' not found", agentName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Create installer
	gitOps := git.NewDefaultGitOperations()
	builder := build.NewDefaultBuildExecutor()
	installer := component.NewDefaultInstaller(gitOps, builder, e.config.ComponentDAO)

	// Uninstall the agent
	result, err := installer.Uninstall(ctx, component.ComponentKindAgent, agentName)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Uninstall failed: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Agent '%s' uninstalled successfully in %v", agentName, result.Duration),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleAgentLogs retrieves and displays recent logs for an agent.
func (e *Executor) handleAgentLogs(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Error:    "Usage: /agents logs <agent-name>",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	agentName := args[0]

	if e.config.ComponentDAO == nil {
		return &ExecutionResult{
			Error:    "Database not available. Run 'gibson init' to initialize.",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Get the agent component
	agent, err := e.config.ComponentDAO.GetByName(ctx, component.ComponentKindAgent, agentName)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to get agent: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}
	if agent == nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Agent '%s' not found", agentName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// For now, return a placeholder message
	// In a full implementation, this would read from ~/.gibson/logs/<agent-name>.log
	// or integrate with the actual logging system
	logPath := fmt.Sprintf("%s/logs/%s.log", e.config.HomeDir, agentName)

	return &ExecutionResult{
		Output:   fmt.Sprintf("Log viewer not yet implemented.\nLogs would be displayed from: %s\n\nUse the CLI command 'gibson agent logs %s' for full log viewing.", logPath, agentName),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleAttack launches a single-agent attack against a target.
// Usage: /attack <agent-name> --target <url> [--mode <mode>] [--goal <goal>]
func (e *Executor) handleAttack(ctx context.Context, args []string) (*ExecutionResult, error) {
	// Note: The Execute method doesn't pass ParsedCommand to handlers,
	// so we cannot access flags directly. This is a design limitation.
	// For TUI integration, this will need enhancement.

	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /attack <agent-name> --target <url> [--mode <mode>]\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	agentName := args[0]

	// For now, return a placeholder since full attack integration requires:
	// 1. Parsing flags from args (--target, --mode, --goal, etc.)
	// 2. Target URL validation
	// 3. Integration with attack orchestrator
	// 4. Agent session management

	return &ExecutionResult{
		Output:   fmt.Sprintf("Attack command for agent '%s' requires full attack orchestrator integration\nThis will be implemented in the TUI integration phase\n", agentName),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleMission routes mission subcommands to their specific handlers.
func (e *Executor) handleMission(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) == 0 {
		// Default to list
		return e.handleMissions(ctx, args)
	}

	switch args[0] {
	case "list":
		return e.handleMissions(ctx, args[1:])
	case "start", "run":
		return e.handleMissionStart(ctx, args[1:])
	case "stop":
		return e.handleMissionStop(ctx, args[1:])
	case "create":
		return e.handleMissionCreate(ctx, args[1:])
	case "delete":
		return e.handleMissionDelete(ctx, args[1:])
	default:
		return &ExecutionResult{
			Output:   fmt.Sprintf("Unknown mission subcommand: %s\nAvailable: list, start, stop, create, delete\n", args[0]),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}
}

// handleMissionStart starts a new or existing mission.
// Usage: /mission-start <name>
func (e *Executor) handleMissionStart(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /mission-start <name>\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	missionName := args[0]

	if e.config.DB == nil {
		return &ExecutionResult{
			Output:   "Database not available\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	missionStore := mission.NewDBMissionStore(e.config.DB)

	// Check if mission exists
	m, err := missionStore.GetByName(ctx, missionName)
	if err != nil {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Mission not found: %s\nCreate it first with /mission-create\n", missionName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Update status to running
	now := time.Now()
	m.Status = mission.MissionStatusRunning
	m.StartedAt = &now
	if err := missionStore.Update(ctx, m); err != nil {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Failed to start mission: %v\n", err),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Mission '%s' started successfully\nID: %s\n", m.Name, m.ID),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleMissionStop stops a running mission.
// Usage: /mission-stop <name>
func (e *Executor) handleMissionStop(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /mission-stop <name>\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	missionName := args[0]

	if e.config.DB == nil {
		return &ExecutionResult{
			Output:   "Database not available\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	missionStore := mission.NewDBMissionStore(e.config.DB)

	// Get mission
	m, err := missionStore.GetByName(ctx, missionName)
	if err != nil {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Mission not found: %s\n", missionName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Check if mission is running
	if m.Status != mission.MissionStatusRunning {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Mission is not running (current status: %s)\n", m.Status),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Update status to cancelled
	if err := missionStore.UpdateStatus(ctx, m.ID, mission.MissionStatusCancelled); err != nil {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Failed to stop mission: %v\n", err),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Mission '%s' stopped successfully\n", m.Name),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleMissionCreate creates a new mission.
// Usage: /mission-create <name>
func (e *Executor) handleMissionCreate(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /mission-create <name>\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	missionName := args[0]

	if e.config.DB == nil {
		return &ExecutionResult{
			Output:   "Database not available\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	missionStore := mission.NewDBMissionStore(e.config.DB)

	// Check if mission already exists
	if _, err := missionStore.GetByName(ctx, missionName); err == nil {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Mission '%s' already exists\n", missionName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Create placeholder mission
	// Note: Full workflow parsing and creation requires workflow file path from flags

	return &ExecutionResult{
		Output:   "Mission creation requires workflow file specification\nThis will be fully implemented in the TUI integration phase\n",
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleMissionDelete deletes a mission.
// Usage: /mission-delete <name> [--confirm]
func (e *Executor) handleMissionDelete(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /mission-delete <name> [--confirm]\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	missionName := args[0]

	// Note: The --confirm flag should be checked via parsed.Flags.
	// This is a limitation since CommandHandler doesn't receive ParsedCommand.
	// For now, we delete without requiring confirmation (not ideal).

	if e.config.DB == nil {
		return &ExecutionResult{
			Output:   "Database not available\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	missionStore := mission.NewDBMissionStore(e.config.DB)

	// Get mission to retrieve ID
	m, err := missionStore.GetByName(ctx, missionName)
	if err != nil {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Mission not found: %s\n", missionName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Delete mission
	if err := missionStore.Delete(ctx, m.ID); err != nil {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Failed to delete mission: %v\n", err),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Mission '%s' deleted successfully\n", m.Name),
		IsError:  false,
		ExitCode: 0,
	}, nil
}

// handleTool routes to appropriate tool subcommand handler or lists tools.
func (e *Executor) handleTool(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) == 0 {
		return e.handleToolsList(ctx, args)
	}

	switch args[0] {
	case "list":
		return e.handleToolsList(ctx, args[1:])
	case "start":
		return e.handleToolStart(ctx, args[1:])
	case "stop":
		return e.handleToolStop(ctx, args[1:])
	case "install":
		return e.handleToolInstall(ctx, args[1:])
	case "uninstall":
		return e.handleToolUninstall(ctx, args[1:])
	case "logs":
		return e.handleToolLogs(ctx, args[1:])
	default:
		return &ExecutionResult{
			Output:   fmt.Sprintf("Unknown tool subcommand: %s\nAvailable: list, start, stop, install, uninstall, logs\n", args[0]),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}
}

// handleToolsList lists all tools (delegates to handleTools).
func (e *Executor) handleToolsList(ctx context.Context, args []string) (*ExecutionResult, error) {
	return e.handleTools(ctx, args)
}

// handleToolStart starts a tool component.
func (e *Executor) handleToolStart(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /tool start <name>\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	toolName := args[0]

	if e.config.ComponentDAO == nil {
		return &ExecutionResult{
			Output:   "Database not available. Run 'gibson init' to initialize.\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Get tool
	tool, err := e.config.ComponentDAO.GetByName(ctx, component.ComponentKindTool, toolName)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to get tool: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}
	if tool == nil {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Tool '%s' not found\n", toolName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Check if already running
	if tool.IsRunning() {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Tool '%s' is already running (PID: %d)\n", toolName, tool.PID),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Start the tool
	lifecycleManager := e.getLifecycleManager()
	port, err := lifecycleManager.StartComponent(ctx, tool)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to start tool: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Tool '%s' started successfully\nPID: %d\nPort: %d\n", toolName, tool.PID, port),
		ExitCode: 0,
	}, nil
}

// handleToolStop stops a running tool component.
func (e *Executor) handleToolStop(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /tool stop <name>\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	toolName := args[0]

	if e.config.ComponentDAO == nil {
		return &ExecutionResult{
			Output:   "Database not available. Run 'gibson init' to initialize.\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Get tool
	tool, err := e.config.ComponentDAO.GetByName(ctx, component.ComponentKindTool, toolName)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to get tool: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}
	if tool == nil {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Tool '%s' not found\n", toolName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Check if running
	if !tool.IsRunning() {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Tool '%s' is not running\n", toolName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Stop the tool
	lifecycleManager := e.getLifecycleManager()
	if err := lifecycleManager.StopComponent(ctx, tool); err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to stop tool: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Tool '%s' stopped successfully\n", toolName),
		ExitCode: 0,
	}, nil
}

// handleToolInstall installs a new tool component.
func (e *Executor) handleToolInstall(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /tool install <source>\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	return &ExecutionResult{
		Output:   "Tool installation from TUI is not yet implemented.\nUse: gibson tool install <source>\n",
		IsError:  true,
		ExitCode: 1,
	}, nil
}

// handleToolUninstall uninstalls a tool component.
func (e *Executor) handleToolUninstall(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /tool uninstall <name> --confirm\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Check for --confirm flag
	confirmed := false
	for _, arg := range args {
		if arg == "--confirm" {
			confirmed = true
			break
		}
	}

	if !confirmed {
		return &ExecutionResult{
			Output:   "Uninstall requires --confirm flag to proceed.\nUsage: /tool uninstall <name> --confirm\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	return &ExecutionResult{
		Output:   "Tool uninstallation from TUI is not yet implemented.\nUse: gibson tool uninstall <name> --force\n",
		IsError:  true,
		ExitCode: 1,
	}, nil
}

// handleToolLogs displays logs for a tool component.
func (e *Executor) handleToolLogs(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /tool logs <name>\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	toolName := args[0]

	if e.config.ComponentDAO == nil {
		return &ExecutionResult{
			Output:   "Database not available. Run 'gibson init' to initialize.\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Get tool
	tool, err := e.config.ComponentDAO.GetByName(ctx, component.ComponentKindTool, toolName)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to get tool: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}
	if tool == nil {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Tool '%s' not found\n", toolName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Construct log file path
	logPath := fmt.Sprintf("%s/logs/%s.log", e.config.HomeDir, toolName)

	return &ExecutionResult{
		Output:   fmt.Sprintf("Log file path: %s\n\nLog viewing in TUI not yet implemented.\nUse: gibson tool logs %s\n", logPath, toolName),
		ExitCode: 0,
	}, nil
}

// handlePlugin routes to appropriate plugin subcommand handler or lists plugins.
func (e *Executor) handlePlugin(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) == 0 {
		return e.handlePluginsList(ctx, args)
	}

	switch args[0] {
	case "list":
		return e.handlePluginsList(ctx, args[1:])
	case "start":
		return e.handlePluginStart(ctx, args[1:])
	case "stop":
		return e.handlePluginStop(ctx, args[1:])
	case "install":
		return e.handlePluginInstall(ctx, args[1:])
	case "uninstall":
		return e.handlePluginUninstall(ctx, args[1:])
	default:
		return &ExecutionResult{
			Output:   fmt.Sprintf("Unknown plugin subcommand: %s\nAvailable: list, start, stop, install, uninstall\n", args[0]),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}
}

// handlePluginsList lists all plugins (delegates to handlePlugins).
func (e *Executor) handlePluginsList(ctx context.Context, args []string) (*ExecutionResult, error) {
	return e.handlePlugins(ctx, args)
}

// handlePluginStart starts a plugin component.
func (e *Executor) handlePluginStart(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /plugin start <name>\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	pluginName := args[0]

	if e.config.ComponentDAO == nil {
		return &ExecutionResult{
			Output:   "Database not available. Run 'gibson init' to initialize.\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Get plugin
	plugin, err := e.config.ComponentDAO.GetByName(ctx, component.ComponentKindPlugin, pluginName)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to get plugin: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}
	if plugin == nil {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Plugin '%s' not found\n", pluginName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Check if already running
	if plugin.IsRunning() {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Plugin '%s' is already running (PID: %d)\n", pluginName, plugin.PID),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Start the plugin
	lifecycleManager := e.getLifecycleManager()
	port, err := lifecycleManager.StartComponent(ctx, plugin)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to start plugin: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Plugin '%s' started successfully\nPID: %d\nPort: %d\n", pluginName, plugin.PID, port),
		ExitCode: 0,
	}, nil
}

// handlePluginStop stops a running plugin component.
func (e *Executor) handlePluginStop(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /plugin stop <name>\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	pluginName := args[0]

	if e.config.ComponentDAO == nil {
		return &ExecutionResult{
			Output:   "Database not available. Run 'gibson init' to initialize.\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Get plugin
	plugin, err := e.config.ComponentDAO.GetByName(ctx, component.ComponentKindPlugin, pluginName)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to get plugin: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}
	if plugin == nil {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Plugin '%s' not found\n", pluginName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Check if running
	if !plugin.IsRunning() {
		return &ExecutionResult{
			Output:   fmt.Sprintf("Plugin '%s' is not running\n", pluginName),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Stop the plugin
	lifecycleManager := e.getLifecycleManager()
	if err := lifecycleManager.StopComponent(ctx, plugin); err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to stop plugin: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, err
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Plugin '%s' stopped successfully\n", pluginName),
		ExitCode: 0,
	}, nil
}

// handlePluginInstall installs a new plugin component.
func (e *Executor) handlePluginInstall(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /plugin install <source>\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	return &ExecutionResult{
		Output:   "Plugin installation from TUI is not yet implemented.\nUse: gibson plugin install <source>\n",
		IsError:  true,
		ExitCode: 1,
	}, nil
}

// handlePluginUninstall uninstalls a plugin component.
func (e *Executor) handlePluginUninstall(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Output:   "Usage: /plugin uninstall <name> --confirm\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Check for --confirm flag
	confirmed := false
	for _, arg := range args {
		if arg == "--confirm" {
			confirmed = true
			break
		}
	}

	if !confirmed {
		return &ExecutionResult{
			Output:   "Uninstall requires --confirm flag to proceed.\nUsage: /plugin uninstall <name> --confirm\n",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	return &ExecutionResult{
		Output:   "Plugin uninstallation from TUI is not yet implemented.\nUse: gibson plugin uninstall <name> --force\n",
		IsError:  true,
		ExitCode: 1,
	}, nil
}

// getLifecycleManager creates a lifecycle manager for component operations.
func (e *Executor) getLifecycleManager() component.LifecycleManager {
	healthMonitor := component.NewHealthMonitor()
	localTracker := component.NewDefaultLocalTracker()
	return component.NewLifecycleManager(healthMonitor, e.config.ComponentDAO, nil, localTracker)
}

// handleFocus switches TUI focus to a specific agent and subscribes to its event stream.
// Usage: /focus <agent-name>
func (e *Executor) handleFocus(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Error:    "Usage: /focus <agent-name>",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	agentName := args[0]

	// Verify the agent is running and has an active streaming session
	if err := e.verifyAgentRunning(ctx, agentName); err != nil {
		return &ExecutionResult{
			Error:    err.Error(),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// If already focused on a different agent, unsubscribe first
	if e.focusedAgent != "" && e.focusedAgent != agentName {
		if err := e.unsubscribeFocusedAgent(); err != nil {
			// Log but don't fail - we still want to switch focus
			_ = err
		}
	}

	// Set the focused agent
	e.SetFocusedAgent(agentName)

	// Subscribe to the agent's event stream
	if err := e.subscribeFocusedAgent(agentName); err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Focused on agent '%s' but failed to subscribe to events: %v", agentName, err),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Focused on agent: %s\nStreaming agent events to console.\n", agentName),
		ExitCode: 0,
	}, nil
}

// handleInterrupt sends an interrupt to the focused agent.
// Usage: /interrupt [reason]
func (e *Executor) handleInterrupt(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.config.StreamManager == nil {
		return &ExecutionResult{
			Error:    "Stream manager not available",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	focusedAgent := e.GetFocusedAgent()
	if focusedAgent == "" {
		return &ExecutionResult{
			Error:    "No agent is currently focused. Use /focus <agent-name> first.",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	reason := "User interrupt"
	if len(args) > 0 {
		reason = strings.Join(args, " ")
	}

	if err := e.config.StreamManager.SendInterrupt(focusedAgent, reason); err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to interrupt agent: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Interrupt sent to agent '%s'\n", focusedAgent),
		ExitCode: 0,
	}, nil
}

// handleMode sets the execution mode for the focused agent.
// Usage: /mode <autonomous|interactive>
func (e *Executor) handleMode(ctx context.Context, args []string) (*ExecutionResult, error) {
	if len(args) < 1 {
		return &ExecutionResult{
			Error:    "Usage: /mode <autonomous|interactive>",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	if e.config.StreamManager == nil {
		return &ExecutionResult{
			Error:    "Stream manager not available",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	focusedAgent := e.GetFocusedAgent()
	if focusedAgent == "" {
		return &ExecutionResult{
			Error:    "No agent is currently focused. Use /focus <agent-name> first.",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	mode := args[0]
	var agentMode database.AgentMode
	switch mode {
	case "autonomous", "auto":
		agentMode = database.AgentModeAutonomous
	case "interactive":
		agentMode = database.AgentModeInteractive
	default:
		return &ExecutionResult{
			Error:    fmt.Sprintf("Invalid mode: %s. Use 'autonomous' or 'interactive'", mode),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	if err := e.config.StreamManager.SetMode(focusedAgent, agentMode); err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to set mode: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Agent '%s' mode set to %s\n", focusedAgent, mode),
		ExitCode: 0,
	}, nil
}

// handlePause pauses the focused agent's execution.
// Usage: /pause
func (e *Executor) handlePause(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.config.StreamManager == nil {
		return &ExecutionResult{
			Error:    "Stream manager not available",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	focusedAgent := e.GetFocusedAgent()
	if focusedAgent == "" {
		return &ExecutionResult{
			Error:    "No agent is currently focused. Use /focus <agent-name> first.",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	// Use SendInterrupt with "pause" reason as workaround since PauseAgent doesn't exist
	if err := e.config.StreamManager.SendInterrupt(focusedAgent, "pause requested"); err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to pause agent: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	return &ExecutionResult{
		Output:   fmt.Sprintf("Pause signal sent to agent '%s'\n", focusedAgent),
		ExitCode: 0,
	}, nil
}

// handleResume resumes a paused agent with optional guidance.
// Usage: /resume [message]
func (e *Executor) handleResume(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.config.StreamManager == nil {
		return &ExecutionResult{
			Error:    "Stream manager not available",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	focusedAgent := e.GetFocusedAgent()
	if focusedAgent == "" {
		return &ExecutionResult{
			Error:    "No agent is currently focused. Use /focus <agent-name> first.",
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	guidance := ""
	if len(args) > 0 {
		guidance = strings.Join(args, " ")
	}

	if err := e.config.StreamManager.Resume(focusedAgent, guidance); err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("Failed to resume agent: %v", err),
			IsError:  true,
			ExitCode: 1,
		}, nil
	}

	output := fmt.Sprintf("Agent '%s' resumed", focusedAgent)
	if guidance != "" {
		output += fmt.Sprintf(" with guidance: %s", guidance)
	}
	output += "\n"

	return &ExecutionResult{
		Output:   output,
		ExitCode: 0,
	}, nil
}

// verifyAgentRunning checks if an agent is actually running with an active streaming session.
// Returns nil if the agent is running and has an active session, or an error describing the problem.
func (e *Executor) verifyAgentRunning(ctx context.Context, agentName string) error {
	// Check if ComponentDAO is available
	if e.config.ComponentDAO == nil {
		return fmt.Errorf("component DAO not available")
	}

	// Check if agent exists
	comp, err := e.config.ComponentDAO.GetByName(ctx, component.ComponentKindAgent, agentName)
	if err != nil {
		return fmt.Errorf("agent '%s' not found", agentName)
	}
	if comp == nil {
		return fmt.Errorf("agent '%s' not found", agentName)
	}

	// Check if agent process is running
	if !comp.IsRunning() {
		return fmt.Errorf("agent '%s' is not running. Use /agent-start %s to start it", agentName, agentName)
	}

	// Check if StreamManager is available
	if e.config.StreamManager == nil {
		return fmt.Errorf("stream manager not available. Agent streaming is disabled")
	}

	// Check if agent has an active streaming session
	session, err := e.config.StreamManager.GetSession(agentName)
	if err != nil || session == nil {
		return fmt.Errorf("agent '%s' has no active streaming session. It may be running in non-streaming mode", agentName)
	}

	return nil
}

// subscribeFocusedAgent subscribes to the agent's event stream and starts the event processor.
// Returns an error if subscription fails.
func (e *Executor) subscribeFocusedAgent(agentName string) error {
	if e.config.StreamManager == nil {
		return fmt.Errorf("stream manager not available")
	}

	if e.program == nil {
		return fmt.Errorf("program not set - cannot deliver events to TUI")
	}

	// Subscribe to the agent's event stream
	eventChan := e.config.StreamManager.Subscribe(agentName)
	if eventChan == nil {
		return fmt.Errorf("failed to subscribe to agent stream: nil channel returned")
	}

	// Store the subscription channel
	e.subscriptionCh = eventChan

	// Create and start the event processor
	e.eventProcessor = NewEventProcessor(eventChan, e.program, agentName)
	e.eventProcessor.Start()

	return nil
}

// unsubscribeFocusedAgent stops the event processor and unsubscribes from the current agent.
// Safe to call even if no agent is focused.
func (e *Executor) unsubscribeFocusedAgent() error {
	// Stop the event processor if running
	if e.eventProcessor != nil {
		e.eventProcessor.Stop()
		e.eventProcessor = nil
	}

	// Unsubscribe from the stream if we have a subscription
	if e.subscriptionCh != nil && e.config.StreamManager != nil && e.focusedAgent != "" {
		e.config.StreamManager.Unsubscribe(e.focusedAgent, e.subscriptionCh)
		e.subscriptionCh = nil
	}

	return nil
}
