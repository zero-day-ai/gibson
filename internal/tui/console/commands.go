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
		// Agent commands (full CLI parity)
		{
			Name:        "agent",
			Aliases:     []string{"agents"},
			Description: "Manage agents",
			Usage:       "/agent [start|stop|install|uninstall|logs|list|show|build|update] [name] [args]",
			Handler:     nil,
		},
		{
			Name:        "agent-start",
			Description: "Start an agent",
			Usage:       "/agent-start <name> [--target <url>] [--mode <mode>]",
			Handler:     nil,
		},
		{
			Name:        "agent-execute",
			Aliases:     []string{"agent-exec", "agent-run"},
			Description: "Execute a task on a running agent with streaming output",
			Usage:       "/agent-execute <name> <goal>",
			Handler:     nil,
		},
		{
			Name:        "agent-stop",
			Description: "Stop a running agent",
			Usage:       "/agent-stop <name>",
			Handler:     nil,
		},
		{
			Name:        "agent-install",
			Description: "Install agent from source",
			Usage:       "/agent-install <source> [--branch <branch>] [--tag <tag>] [--force]",
			Handler:     nil,
		},
		{
			Name:        "agent-uninstall",
			Description: "Uninstall an agent",
			Usage:       "/agent-uninstall <name> [--force]",
			Handler:     nil,
		},
		{
			Name:        "agent-logs",
			Description: "Show agent logs",
			Usage:       "/agent-logs <name> [--follow] [--lines <n>]",
			Handler:     nil,
		},

		// Tool commands (full CLI parity)
		{
			Name:        "tool",
			Aliases:     []string{"tools"},
			Description: "Manage tools",
			Usage:       "/tool [start|stop|install|uninstall|logs|list|show|build|update|invoke|test] [name] [args]",
			Handler:     nil,
		},
		{
			Name:        "tool-start",
			Description: "Start a tool server",
			Usage:       "/tool-start <name>",
			Handler:     nil,
		},
		{
			Name:        "tool-stop",
			Description: "Stop a tool server",
			Usage:       "/tool-stop <name>",
			Handler:     nil,
		},
		{
			Name:        "tool-install",
			Description: "Install tool from source",
			Usage:       "/tool-install <source> [--branch <branch>] [--tag <tag>] [--force]",
			Handler:     nil,
		},
		{
			Name:        "tool-uninstall",
			Description: "Uninstall a tool",
			Usage:       "/tool-uninstall <name> [--force]",
			Handler:     nil,
		},
		{
			Name:        "tool-logs",
			Description: "Show tool logs",
			Usage:       "/tool-logs <name> [--follow] [--lines <n>]",
			Handler:     nil,
		},

		// Plugin commands (full CLI parity)
		{
			Name:        "plugin",
			Aliases:     []string{"plugins"},
			Description: "Manage plugins",
			Usage:       "/plugin [start|stop|install|uninstall|list|show|build|update|query] [name] [args]",
			Handler:     nil,
		},
		{
			Name:        "plugin-start",
			Description: "Start a plugin",
			Usage:       "/plugin-start <name>",
			Handler:     nil,
		},
		{
			Name:        "plugin-stop",
			Description: "Stop a plugin",
			Usage:       "/plugin-stop <name>",
			Handler:     nil,
		},
		{
			Name:        "plugin-install",
			Description: "Install plugin from source",
			Usage:       "/plugin-install <source> [--branch <branch>] [--tag <tag>] [--force]",
			Handler:     nil,
		},
		{
			Name:        "plugin-uninstall",
			Description: "Uninstall a plugin",
			Usage:       "/plugin-uninstall <name> [--force]",
			Handler:     nil,
		},

		// Attack command (quick single-agent attack)
		{
			Name:        "attack",
			Description: "Launch a quick single-agent attack",
			Usage:       "/attack <agent> --target <url> [--mode <mode>] [--goal <goal>] [--timeout <duration>]",
			Handler:     nil,
		},

		// Mission commands (full CLI parity)
		{
			Name:        "mission",
			Aliases:     []string{"missions"},
			Description: "Manage missions",
			Usage:       "/mission [start|stop|create|delete|list|show|resume] [name] [args]",
			Handler:     nil,
		},
		{
			Name:        "mission-start",
			Aliases:     []string{"mission-run"},
			Description: "Start a mission",
			Usage:       "/mission-start <name> [-f <workflow-file>]",
			Handler:     nil,
		},
		{
			Name:        "mission-stop",
			Description: "Stop a running mission",
			Usage:       "/mission-stop <name>",
			Handler:     nil,
		},
		{
			Name:        "mission-create",
			Description: "Create a new mission",
			Usage:       "/mission-create <name> [-f <workflow-file>] [--description <desc>]",
			Handler:     nil,
		},
		{
			Name:        "mission-delete",
			Description: "Delete a mission",
			Usage:       "/mission-delete <name> [--force]",
			Handler:     nil,
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
