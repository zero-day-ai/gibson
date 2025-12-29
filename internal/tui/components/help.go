package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
)

// HelpOverlay displays a centered overlay with keyboard shortcuts.
// It shows all available key bindings grouped by category.
type HelpOverlay struct {
	keymap   map[string][]key.Binding
	theme    *styles.Theme
	visible  bool
	width    int
	height   int
	helpText string
}

// NewHelpOverlay creates a new help overlay with the given keymap.
func NewHelpOverlay(keymap map[string][]key.Binding) *HelpOverlay {
	h := &HelpOverlay{
		keymap:  keymap,
		theme:   styles.DefaultTheme(),
		visible: false,
		width:   80,
		height:  30,
	}
	h.buildHelpText()
	return h
}

// SetSize sets the dimensions for the overlay.
func (h *HelpOverlay) SetSize(width, height int) {
	if width > 0 {
		h.width = width
	}
	if height > 0 {
		h.height = height
	}
}

// Show makes the overlay visible.
func (h *HelpOverlay) Show() {
	h.visible = true
}

// Hide makes the overlay invisible.
func (h *HelpOverlay) Hide() {
	h.visible = false
}

// Toggle toggles the overlay visibility.
func (h *HelpOverlay) Toggle() {
	h.visible = !h.visible
}

// IsVisible returns whether the overlay is currently visible.
func (h *HelpOverlay) IsVisible() bool {
	return h.visible
}

// SetTheme sets the theme for the overlay.
func (h *HelpOverlay) SetTheme(theme *styles.Theme) {
	if theme != nil {
		h.theme = theme
	}
}

// buildHelpText constructs the help text from the keymap.
func (h *HelpOverlay) buildHelpText() {
	var builder strings.Builder

	// Title
	title := h.theme.TitleStyle.Render("Keyboard Shortcuts")
	builder.WriteString(title)
	builder.WriteString("\n\n")

	// Sort categories for consistent display
	categories := []string{"Global", "Views", "Navigation", "Actions"}

	for _, category := range categories {
		bindings, ok := h.keymap[category]
		if !ok {
			continue
		}

		// Category header
		categoryHeader := h.theme.TitleStyle.Copy().
			Underline(true).
			Render(category)
		builder.WriteString(categoryHeader)
		builder.WriteString("\n")

		// List bindings in this category
		for _, binding := range bindings {
			keyHelp := binding.Help()
			if keyHelp.Key != "" {
				// Format: "  key    description"
				keyStyle := h.theme.Primary
				line := fmt.Sprintf("  %-12s %s\n",
					lipgloss.NewStyle().Foreground(keyStyle).Bold(true).Render(keyHelp.Key),
					keyHelp.Desc)
				builder.WriteString(line)
			}
		}
		builder.WriteString("\n")
	}

	// Footer with instructions
	builder.WriteString(strings.Repeat("─", 40))
	builder.WriteString("\n")
	footerStyle := h.theme.SeverityInfo
	footer := footerStyle.Render("Press ? or Esc to close this help overlay")
	builder.WriteString(footer)

	h.helpText = builder.String()
}

// Render renders the help overlay as a centered modal.
// Returns an empty string if the overlay is not visible.
func (h *HelpOverlay) Render() string {
	if !h.visible {
		return ""
	}

	// Calculate overlay dimensions
	// Help overlay should be about 60% of screen width and height
	overlayWidth := h.width * 6 / 10
	overlayHeight := h.height * 6 / 10

	if overlayWidth < 40 {
		overlayWidth = 40
	}
	if overlayHeight < 20 {
		overlayHeight = 20
	}

	// Create the help content box
	contentStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(h.theme.Primary).
		Padding(1, 2).
		Width(overlayWidth - 6). // Account for border and padding
		Height(overlayHeight - 4)

	helpBox := contentStyle.Render(h.helpText)

	// Create a semi-transparent background overlay effect
	// We'll center the help box on the screen
	overlayStyle := lipgloss.NewStyle().
		Width(h.width).
		Height(h.height).
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Center)

	centeredHelp := overlayStyle.Render(helpBox)

	return centeredHelp
}

// Update updates the help overlay based on the keymap.
// Call this if the keymap changes.
func (h *HelpOverlay) Update(keymap map[string][]key.Binding) {
	h.keymap = keymap
	h.buildHelpText()
}
