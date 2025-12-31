package tui

// AppMode represents the current view mode of the TUI
type AppMode int

const (
	// ModeDashboard is the main dashboard view showing system overview
	ModeDashboard AppMode = iota
	// ModeConsole is the console view for command execution
	ModeConsole
	// ModeMission is the mission management view
	ModeMission
	// ModeFindings is the findings/vulnerabilities view
	ModeFindings
	// ModeAgentFocus is the agent observation and steering view
	ModeAgentFocus
)

// String returns the string representation of the AppMode
func (m AppMode) String() string {
	switch m {
	case ModeDashboard:
		return "Dashboard"
	case ModeConsole:
		return "Console"
	case ModeMission:
		return "Mission"
	case ModeFindings:
		return "Findings"
	case ModeAgentFocus:
		return "Agent Focus"
	default:
		return "Unknown"
	}
}

// ModeFromKey returns the AppMode corresponding to a key press (1-5)
// Returns the current mode if the key is not recognized
func ModeFromKey(key string, currentMode AppMode) AppMode {
	switch key {
	case "1":
		return ModeDashboard
	case "2":
		return ModeConsole
	case "3":
		return ModeMission
	case "4":
		return ModeFindings
	case "5":
		return ModeAgentFocus
	default:
		return currentMode
	}
}
