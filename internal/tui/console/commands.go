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
		{
			Name:        "agents",
			Aliases:     []string{"agent"},
			Description: "List and manage agents",
			Usage:       "/agents [list|focus|mode|interrupt|status] [name] [args]",
			Handler:     nil,
		},
		{
			Name:        "missions",
			Description: "List and manage missions",
			Usage:       "/missions [list|create|start|stop|delete] [args]",
			Handler:     nil,
		},
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
			Name:        "tools",
			Description: "List available tools",
			Usage:       "/tools [list|info] [name]",
			Handler:     nil,
		},
		{
			Name:        "plugins",
			Description: "List available plugins",
			Usage:       "/plugins [list|info] [name]",
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
