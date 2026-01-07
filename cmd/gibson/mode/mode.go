// Package mode provides command mode classification for the Gibson CLI.
// It determines whether a command should run standalone (without daemon),
// as the daemon itself, or as a client connecting to an existing daemon.
package mode

// CommandMode represents the execution mode for a Gibson command.
type CommandMode int

const (
	// Standalone mode runs without requiring a daemon connection.
	// Used for simple commands like version, help, init, and config operations.
	Standalone CommandMode = iota

	// Daemon mode is used for commands that manage the daemon lifecycle itself.
	// These commands start, stop, or check the status of the Gibson daemon.
	Daemon

	// Client mode requires an active daemon connection.
	// Used for commands that need access to the registry, agents, or other daemon services.
	Client
)

// String returns a string representation of the CommandMode.
func (m CommandMode) String() string {
	switch m {
	case Standalone:
		return "Standalone"
	case Daemon:
		return "Daemon"
	case Client:
		return "Client"
	default:
		return "Unknown"
	}
}

// CommandRegistry maps command paths to their execution modes.
// Command paths follow the format "gibson <command> [<subcommand>...]".
var CommandRegistry = make(map[string]CommandMode)

// Register adds a command path and its mode to the registry.
// The path should be the full command path, e.g., "gibson agent list".
func Register(path string, mode CommandMode) {
	CommandRegistry[path] = mode
}

// GetMode returns the execution mode for a given command path.
// If the exact path is not found, it returns Client as the default mode.
// This ensures that new commands requiring daemon access work correctly by default.
func GetMode(path string) CommandMode {
	if mode, ok := CommandRegistry[path]; ok {
		return mode
	}
	return Client // Default to client mode for safety
}
