package console

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/finding"
)

// ExecutorConfig holds all dependencies and configuration needed for command execution.
type ExecutorConfig struct {
	// DB is the database connection for data persistence operations.
	DB *database.DB
	// ComponentRegistry manages agents, tools, and plugins.
	ComponentRegistry component.ComponentRegistry
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
	registry *CommandRegistry
	config   ExecutorConfig
	ctx      context.Context
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

	// Execute the command handler
	result, err := cmd.Handler(e.ctx, parsed.Args)
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
// provided dependencies (DB, ComponentRegistry, FindingStore).
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

	// /agents - List agents
	if cmd, ok := e.registry.Get("agents"); ok {
		cmd.Handler = e.handleAgents
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

	// /tools - List tools
	if cmd, ok := e.registry.Get("tools"); ok {
		cmd.Handler = e.handleTools
	}

	// /plugins - List plugins
	if cmd, ok := e.registry.Get("plugins"); ok {
		cmd.Handler = e.handlePlugins
	}

	// /config - Show configuration
	if cmd, ok := e.registry.Get("config"); ok {
		cmd.Handler = e.handleConfig
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

	// Component registry status
	if e.config.ComponentRegistry != nil {
		agents := e.config.ComponentRegistry.List(component.ComponentKindAgent)
		tools := e.config.ComponentRegistry.List(component.ComponentKindTool)
		plugins := e.config.ComponentRegistry.List(component.ComponentKindPlugin)

		output.WriteString("Components:\n")
		output.WriteString(fmt.Sprintf("  Agents:  %d registered\n", len(agents)))
		output.WriteString(fmt.Sprintf("  Tools:   %d registered\n", len(tools)))
		output.WriteString(fmt.Sprintf("  Plugins: %d registered\n", len(plugins)))
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
	default:
		// Treat as agent name for backward compatibility
		return e.handleAgentsList(ctx, args)
	}
}

// handleAgentsList lists all registered agents from the component registry.
func (e *Executor) handleAgentsList(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.config.ComponentRegistry == nil {
		return &ExecutionResult{
			Output:   "Component registry not available\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	agents := e.config.ComponentRegistry.List(component.ComponentKindAgent)

	if len(agents) == 0 {
		return &ExecutionResult{
			Output:   "No agents registered\n",
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
		Output:   fmt.Sprintf("Focusing on agent: %s\nUse key '5' to switch to Agent Focus view.\n", args[0]),
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

	missionDAO := database.NewMissionDAO(e.config.DB)
	missions, err := missionDAO.List(ctx, "")
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

// handleTools lists all registered tools from the component registry.
func (e *Executor) handleTools(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.config.ComponentRegistry == nil {
		return &ExecutionResult{
			Output:   "Component registry not available\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	tools := e.config.ComponentRegistry.List(component.ComponentKindTool)

	if len(tools) == 0 {
		return &ExecutionResult{
			Output:   "No tools registered\n",
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

// handlePlugins lists all registered plugins from the component registry.
func (e *Executor) handlePlugins(ctx context.Context, args []string) (*ExecutionResult, error) {
	if e.config.ComponentRegistry == nil {
		return &ExecutionResult{
			Output:   "Component registry not available\n",
			IsError:  false,
			ExitCode: 0,
		}, nil
	}

	plugins := e.config.ComponentRegistry.List(component.ComponentKindPlugin)

	if len(plugins) == 0 {
		return &ExecutionResult{
			Output:   "No plugins registered\n",
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
	if e.config.ComponentRegistry != nil {
		output.WriteString("  Registry:       Available\n")
		allComponents := e.config.ComponentRegistry.ListAll()
		totalCount := 0
		for _, components := range allComponents {
			totalCount += len(components)
		}
		output.WriteString(fmt.Sprintf("  Total:          %d components\n", totalCount))
	} else {
		output.WriteString("  Registry:       Not available\n")
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
