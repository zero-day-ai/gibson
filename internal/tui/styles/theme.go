package styles

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme defines the color palette and styles for the TUI.
type Theme struct {
	// Color palette
	Primary lipgloss.Color
	Success lipgloss.Color
	Warning lipgloss.Color
	Danger  lipgloss.Color
	Muted   lipgloss.Color
	Info    lipgloss.Color

	// Panel styles
	PanelStyle        lipgloss.Style
	FocusedPanelStyle lipgloss.Style
	TitleStyle        lipgloss.Style

	// Severity styles
	SeverityCritical lipgloss.Style
	SeverityHigh     lipgloss.Style
	SeverityMedium   lipgloss.Style
	SeverityLow      lipgloss.Style
	SeverityInfo     lipgloss.Style

	// Status styles
	StatusRunning  lipgloss.Style
	StatusPaused   lipgloss.Style
	StatusStopped  lipgloss.Style
	StatusError    lipgloss.Style
	StatusComplete lipgloss.Style
	StatusPending  lipgloss.Style
}

// DefaultTheme returns a theme with default colors and styles.
func DefaultTheme() *Theme {
	theme := &Theme{
		// Define color palette
		Primary: lipgloss.Color("#00A6FF"),
		Success: lipgloss.Color("#00FF00"),
		Warning: lipgloss.Color("#FFA500"),
		Danger:  lipgloss.Color("#FF0000"),
		Muted:   lipgloss.Color("#888888"),
		Info:    lipgloss.Color("#00FFFF"),
	}

	// Create panel styles
	theme.PanelStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Muted).
		Padding(0, 1)

	theme.FocusedPanelStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Primary).
		Padding(0, 1)

	theme.TitleStyle = lipgloss.NewStyle().
		Foreground(theme.Primary).
		Bold(true)

	// Create severity styles
	theme.SeverityCritical = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(theme.Danger).
		Bold(true)

	theme.SeverityHigh = lipgloss.NewStyle().
		Foreground(theme.Danger).
		Bold(true)

	theme.SeverityMedium = lipgloss.NewStyle().
		Foreground(theme.Warning)

	theme.SeverityLow = lipgloss.NewStyle().
		Foreground(theme.Info)

	theme.SeverityInfo = lipgloss.NewStyle().
		Foreground(theme.Muted)

	// Create status styles
	theme.StatusRunning = lipgloss.NewStyle().
		Foreground(theme.Success).
		Bold(true)

	theme.StatusPaused = lipgloss.NewStyle().
		Foreground(theme.Warning)

	theme.StatusStopped = lipgloss.NewStyle().
		Foreground(theme.Muted)

	theme.StatusError = lipgloss.NewStyle().
		Foreground(theme.Danger).
		Bold(true)

	theme.StatusComplete = lipgloss.NewStyle().
		Foreground(theme.Success)

	theme.StatusPending = lipgloss.NewStyle().
		Foreground(theme.Muted)

	return theme
}

// SeverityStyle returns the appropriate style for a given severity level.
func (t *Theme) SeverityStyle(severity string) lipgloss.Style {
	switch severity {
	case "critical":
		return t.SeverityCritical
	case "high":
		return t.SeverityHigh
	case "medium":
		return t.SeverityMedium
	case "low":
		return t.SeverityLow
	case "info":
		return t.SeverityInfo
	default:
		return lipgloss.NewStyle()
	}
}

// StatusStyle returns the appropriate style for a given status.
func (t *Theme) StatusStyle(status string) lipgloss.Style {
	switch status {
	case "running":
		return t.StatusRunning
	case "paused":
		return t.StatusPaused
	case "stopped":
		return t.StatusStopped
	case "error", "failed":
		return t.StatusError
	case "completed", "complete":
		return t.StatusComplete
	case "pending":
		return t.StatusPending
	default:
		return lipgloss.NewStyle()
	}
}
