package console

import (
	"sort"
	"strings"
)

// CommandRegistry manages the collection of available slash commands.
// It provides registration, lookup, and completion capabilities for
// commands and their aliases.
type CommandRegistry struct {
	commands map[string]*SlashCommand
}

// NewCommandRegistry creates a new command registry with an empty command map.
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		commands: make(map[string]*SlashCommand),
	}
}

// Register adds a command to the registry and registers all of its aliases.
// If a command or alias with the same name already exists, it will be overwritten.
func (r *CommandRegistry) Register(cmd *SlashCommand) {
	if cmd == nil {
		return
	}

	// Register the primary command name
	r.commands[cmd.Name] = cmd

	// Register all aliases pointing to the same command
	for _, alias := range cmd.Aliases {
		r.commands[alias] = cmd
	}
}

// Get retrieves a command by name or alias.
// Returns the command and true if found, nil and false otherwise.
func (r *CommandRegistry) Get(name string) (*SlashCommand, bool) {
	cmd, ok := r.commands[name]
	return cmd, ok
}

// List returns all registered commands sorted by name.
// Commands are deduplicated (aliases point to the same command instance).
func (r *CommandRegistry) List() []*SlashCommand {
	// Use a map to deduplicate commands (aliases point to same instance)
	seen := make(map[*SlashCommand]bool)
	var unique []*SlashCommand

	for _, cmd := range r.commands {
		if !seen[cmd] {
			seen[cmd] = true
			unique = append(unique, cmd)
		}
	}

	// Sort by command name
	sort.Slice(unique, func(i, j int) bool {
		return unique[i].Name < unique[j].Name
	})

	return unique
}

// Complete returns a list of command names (including aliases) that match
// the given prefix. Results are sorted alphabetically.
func (r *CommandRegistry) Complete(prefix string) []string {
	var matches []string

	for name := range r.commands {
		if strings.HasPrefix(name, prefix) {
			matches = append(matches, name)
		}
	}

	sort.Strings(matches)
	return matches
}

// RegisterDefaultCommands registers all built-in Gibson slash commands.
// Command handlers are initially nil and should be set by the executor.
func RegisterDefaultCommands(registry *CommandRegistry) {
	commands := []*SlashCommand{
		// Agent commands with subcommands
		{
			Name:        "agent",
			Aliases:     []string{"agents"},
			Description: "Manage agents",
			Usage:       "/agent <subcommand> [args]",
			Handler:     nil,
			Subcommands: map[string]*SlashCommand{
				"list": {
					Name:        "list",
					Description: "List all agents",
					Usage:       "/agent list [--status <status>]",
					Handler:     nil,
				},
				"show": {
					Name:        "show",
					Description: "Show agent details",
					Usage:       "/agent show <name>",
					Handler:     nil,
				},
				"install": {
					Name:        "install",
					Description: "Install agent from source",
					Usage:       "/agent install <source> [--branch <branch>] [--tag <tag>] [--force]",
					Handler:     nil,
				},
				"install-all": {
					Name:        "install-all",
					Description: "Install all available agents",
					Usage:       "/agent install-all [--force]",
					Handler:     nil,
				},
				"uninstall": {
					Name:        "uninstall",
					Description: "Uninstall an agent",
					Usage:       "/agent uninstall <name> [--force]",
					Handler:     nil,
				},
				"update": {
					Name:        "update",
					Description: "Update an agent",
					Usage:       "/agent update <name> [--branch <branch>] [--tag <tag>]",
					Handler:     nil,
				},
				"build": {
					Name:        "build",
					Description: "Build an agent from source",
					Usage:       "/agent build <path>",
					Handler:     nil,
				},
				"start": {
					Name:        "start",
					Description: "Start an agent",
					Usage:       "/agent start <name> [--target <url>] [--mode <mode>]",
					Handler:     nil,
				},
				"stop": {
					Name:        "stop",
					Description: "Stop a running agent",
					Usage:       "/agent stop <name>",
					Handler:     nil,
				},
				"status": {
					Name:        "status",
					Description: "Show agent status",
					Usage:       "/agent status <name>",
					Handler:     nil,
				},
				"logs": {
					Name:        "logs",
					Description: "Show agent logs",
					Usage:       "/agent logs <name> [--follow] [--lines <n>]",
					Handler:     nil,
				},
			},
		},

		// Tool commands with subcommands
		{
			Name:        "tool",
			Aliases:     []string{"tools"},
			Description: "Manage tools",
			Usage:       "/tool <subcommand> [args]",
			Handler:     nil,
			Subcommands: map[string]*SlashCommand{
				"list": {
					Name:        "list",
					Description: "List all tools",
					Usage:       "/tool list [--status <status>]",
					Handler:     nil,
				},
				"show": {
					Name:        "show",
					Description: "Show tool details",
					Usage:       "/tool show <name>",
					Handler:     nil,
				},
				"install": {
					Name:        "install",
					Description: "Install tool from source",
					Usage:       "/tool install <source> [--branch <branch>] [--tag <tag>] [--force]",
					Handler:     nil,
				},
				"install-all": {
					Name:        "install-all",
					Description: "Install all available tools",
					Usage:       "/tool install-all [--force]",
					Handler:     nil,
				},
				"uninstall": {
					Name:        "uninstall",
					Description: "Uninstall a tool",
					Usage:       "/tool uninstall <name> [--force]",
					Handler:     nil,
				},
				"update": {
					Name:        "update",
					Description: "Update a tool",
					Usage:       "/tool update <name> [--branch <branch>] [--tag <tag>]",
					Handler:     nil,
				},
				"build": {
					Name:        "build",
					Description: "Build a tool from source",
					Usage:       "/tool build <path>",
					Handler:     nil,
				},
				"start": {
					Name:        "start",
					Description: "Start a tool server",
					Usage:       "/tool start <name>",
					Handler:     nil,
				},
				"stop": {
					Name:        "stop",
					Description: "Stop a tool server",
					Usage:       "/tool stop <name>",
					Handler:     nil,
				},
				"status": {
					Name:        "status",
					Description: "Show tool status",
					Usage:       "/tool status <name>",
					Handler:     nil,
				},
				"logs": {
					Name:        "logs",
					Description: "Show tool logs",
					Usage:       "/tool logs <name> [--follow] [--lines <n>]",
					Handler:     nil,
				},
			},
		},

		// Plugin commands with subcommands
		{
			Name:        "plugin",
			Aliases:     []string{"plugins"},
			Description: "Manage plugins",
			Usage:       "/plugin <subcommand> [args]",
			Handler:     nil,
			Subcommands: map[string]*SlashCommand{
				"list": {
					Name:        "list",
					Description: "List all plugins",
					Usage:       "/plugin list [--status <status>]",
					Handler:     nil,
				},
				"show": {
					Name:        "show",
					Description: "Show plugin details",
					Usage:       "/plugin show <name>",
					Handler:     nil,
				},
				"install": {
					Name:        "install",
					Description: "Install plugin from source",
					Usage:       "/plugin install <source> [--branch <branch>] [--tag <tag>] [--force]",
					Handler:     nil,
				},
				"install-all": {
					Name:        "install-all",
					Description: "Install all available plugins",
					Usage:       "/plugin install-all [--force]",
					Handler:     nil,
				},
				"uninstall": {
					Name:        "uninstall",
					Description: "Uninstall a plugin",
					Usage:       "/plugin uninstall <name> [--force]",
					Handler:     nil,
				},
				"update": {
					Name:        "update",
					Description: "Update a plugin",
					Usage:       "/plugin update <name> [--branch <branch>] [--tag <tag>]",
					Handler:     nil,
				},
				"build": {
					Name:        "build",
					Description: "Build a plugin from source",
					Usage:       "/plugin build <path>",
					Handler:     nil,
				},
				"start": {
					Name:        "start",
					Description: "Start a plugin",
					Usage:       "/plugin start <name>",
					Handler:     nil,
				},
				"stop": {
					Name:        "stop",
					Description: "Stop a plugin",
					Usage:       "/plugin stop <name>",
					Handler:     nil,
				},
				"status": {
					Name:        "status",
					Description: "Show plugin status",
					Usage:       "/plugin status <name>",
					Handler:     nil,
				},
				"logs": {
					Name:        "logs",
					Description: "Show plugin logs",
					Usage:       "/plugin logs <name> [--follow] [--lines <n>]",
					Handler:     nil,
				},
			},
		},

		// Attack command (quick single-agent attack)
		{
			Name:        "attack",
			Description: "Launch a quick single-agent attack",
			Usage:       "/attack <agent> --target <url> [--mode <mode>] [--goal <goal>] [--timeout <duration>]",
			Handler:     nil,
		},

		// Mission commands with subcommands
		{
			Name:        "mission",
			Aliases:     []string{"missions"},
			Description: "Manage missions",
			Usage:       "/mission <subcommand> [args]",
			Handler:     nil,
			Subcommands: map[string]*SlashCommand{
				"list": {
					Name:        "list",
					Description: "List all missions",
					Usage:       "/mission list [--status <status>]",
					Handler:     nil,
				},
				"show": {
					Name:        "show",
					Description: "Show mission details",
					Usage:       "/mission show <name>",
					Handler:     nil,
				},
				"run": {
					Name:        "run",
					Aliases:     []string{"start"},
					Description: "Run mission from workflow",
					Usage:       "/mission run -f <file> [--name <name>]",
					Handler:     nil,
				},
				"resume": {
					Name:        "resume",
					Description: "Resume paused mission",
					Usage:       "/mission resume <name>",
					Handler:     nil,
				},
				"stop": {
					Name:        "stop",
					Description: "Stop running mission",
					Usage:       "/mission stop <name>",
					Handler:     nil,
				},
				"delete": {
					Name:        "delete",
					Description: "Delete mission",
					Usage:       "/mission delete <name> [--force]",
					Handler:     nil,
				},
			},
		},

		// Steering commands (agent interaction in TUI)
		{
			Name:        "focus",
			Description: "Focus on a specific agent",
			Usage:       "/focus <agent-name>",
			Handler:     nil,
		},
		{
			Name:        "interrupt",
			Description: "Interrupt the focused agent",
			Usage:       "/interrupt",
			Handler:     nil,
		},
		{
			Name:        "mode",
			Description: "Set agent execution mode",
			Usage:       "/mode <autonomous|interactive>",
			Handler:     nil,
		},
		{
			Name:        "pause",
			Description: "Pause the focused agent",
			Usage:       "/pause",
			Handler:     nil,
		},
		{
			Name:        "resume",
			Description: "Resume paused agent with optional guidance",
			Usage:       "/resume [message]",
			Handler:     nil,
		},

		// Legacy/existing commands (retain for compatibility)
		{
			Name:        "credentials",
			Description: "List and manage credentials",
			Usage:       "/credentials [list|add|remove] [args]",
			Handler:     nil,
		},
		{
			Name:        "findings",
			Description: "List security findings",
			Usage:       "/findings [list|show|export] [args]",
			Handler:     nil,
		},
		{
			Name:        "targets",
			Description: "List and manage targets",
			Usage:       "/targets [list|add|remove] [args]",
			Handler:     nil,
		},
		{
			Name:        "status",
			Description: "Show system status",
			Usage:       "/status",
			Handler:     nil,
		},
		{
			Name:        "config",
			Description: "Show configuration",
			Usage:       "/config [show|set] [key] [value]",
			Handler:     nil,
		},
		{
			Name:        "help",
			Description: "Show available commands",
			Usage:       "/help [command]",
			Handler:     nil,
		},
		{
			Name:        "clear",
			Description: "Clear console output",
			Usage:       "/clear",
			Handler:     nil,
		},
	}

	for _, cmd := range commands {
		registry.Register(cmd)
	}
}
