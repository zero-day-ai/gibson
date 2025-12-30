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
		// Define color palette - Amber CRT Theme
		Primary: lipgloss.Color("#FFD966"),
		Success: lipgloss.Color("#FFB000"),
		Warning: lipgloss.Color("#FFB000"),
		Danger:  lipgloss.Color("#FFD966"),
		Muted:   lipgloss.Color("#805800"),
		Info:    lipgloss.Color("#805800"),
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

	// Create severity styles - Brightness-based differentiation
	theme.SeverityCritical = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#FFB000")).
		Bold(true)

	theme.SeverityHigh = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFD966")).
		Bold(true)

	theme.SeverityMedium = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFB000"))

	theme.SeverityLow = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#805800"))

	theme.SeverityInfo = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#805800"))

	// Create status styles - Brightness-based differentiation
	theme.StatusRunning = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFD966")).
		Bold(true)

	theme.StatusPaused = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFB000")).
		Italic(true)

	theme.StatusStopped = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#805800"))

	theme.StatusError = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#FFD966")).
		Bold(true)

	theme.StatusComplete = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFB000"))

	theme.StatusPending = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#805800"))

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
