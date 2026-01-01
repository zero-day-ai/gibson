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
	default:
		return "Unknown"
	}
}

// ModeFromKey returns the AppMode corresponding to a key press (F1 through F4)
// Returns the current mode if the key is not recognized
func ModeFromKey(key string, currentMode AppMode) AppMode {
	switch key {
	case "f1":
		return ModeDashboard
	case "f2":
		return ModeConsole
	case "f3":
		return ModeMission
	case "f4":
		return ModeFindings
	default:
		return currentMode
	}
}
