package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
)

// Panel represents a bordered container with a title and content.
// It supports focus state with different border styles.
type Panel struct {
	title   string
	content string
	width   int
	height  int
	focused bool
	theme   *styles.Theme
}

// NewPanel creates a new Panel with the given title.
// The panel uses the default theme for styling.
func NewPanel(title string) *Panel {
	return &Panel{
		title:   title,
		content: "",
		width:   40,
		height:  10,
		focused: false,
		theme:   styles.DefaultTheme(),
	}
}

// SetContent sets the content of the panel.
// Content will be truncated and scrolling indicators added if it exceeds panel dimensions.
func (p *Panel) SetContent(content string) {
	p.content = content
}

// SetSize sets the dimensions of the panel.
// Width and height should include the border.
func (p *Panel) SetSize(width, height int) {
	if width > 0 {
		p.width = width
	}
	if height > 0 {
		p.height = height
	}
}

// SetFocused sets the focus state of the panel.
// Focused panels use a highlighted border color.
func (p *Panel) SetFocused(focused bool) {
	p.focused = focused
}

// Focused returns whether the panel is currently focused.
func (p *Panel) Focused() bool {
	return p.focused
}

// Title returns the panel's title.
func (p *Panel) Title() string {
	return p.title
}

// SetTitle sets the panel's title.
func (p *Panel) SetTitle(title string) {
	p.title = title
}

// Render renders the panel to a string with borders, title, and content.
// It handles content truncation and adds scrolling indicators when needed.
func (p *Panel) Render() string {
	// Select the appropriate style based on focus state
	var panelStyle lipgloss.Style
	if p.focused {
		panelStyle = p.theme.FocusedPanelStyle
	} else {
		panelStyle = p.theme.PanelStyle
	}

	// Calculate available content dimensions
	// Account for borders (2 chars width, 2 chars height) and padding (2 chars width per side)
	contentWidth := p.width - 6
	contentHeight := p.height - 2

	if contentWidth < 1 {
		contentWidth = 1
	}
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Process content: split into lines and truncate/pad as needed
	lines := strings.Split(p.content, "\n")
	visibleLines := make([]string, 0, contentHeight)

	// Determine if we need scrolling indicators
	hasMoreContent := len(lines) > contentHeight

	// Take only the lines that fit in the available height
	displayLines := lines
	if hasMoreContent {
		// Show the first contentHeight-1 lines and add an indicator
		if contentHeight > 1 {
			displayLines = lines[:contentHeight-1]
		} else {
			displayLines = lines[:1]
		}
	} else {
		displayLines = lines
	}

	// Truncate each line to fit the width
	for _, line := range displayLines {
		if len(line) > contentWidth {
			// Truncate and add ellipsis
			if contentWidth > 3 {
				visibleLines = append(visibleLines, line[:contentWidth-3]+"...")
			} else {
				visibleLines = append(visibleLines, line[:contentWidth])
			}
		} else {
			// Pad line to content width for consistent appearance
			visibleLines = append(visibleLines, line+strings.Repeat(" ", contentWidth-len(line)))
		}
	}

	// Add scrolling indicator if there's more content
	if hasMoreContent && contentHeight > 0 {
		indicator := "..."
		if contentWidth > 3 {
			indicator = strings.Repeat(" ", (contentWidth-3)/2) + "..." + strings.Repeat(" ", (contentWidth-3)/2)
			if len(indicator) < contentWidth {
				indicator += " "
			}
		}
		visibleLines = append(visibleLines, indicator[:contentWidth])
	}

	// Pad with empty lines if we don't have enough content
	for len(visibleLines) < contentHeight {
		visibleLines = append(visibleLines, strings.Repeat(" ", contentWidth))
	}

	// Join lines into content string
	contentStr := strings.Join(visibleLines, "\n")

	// Apply the panel style with title
	styledContent := panelStyle.
		Width(contentWidth).
		Height(contentHeight).
		Render(contentStr)

	// Add title if present
	if p.title != "" {
		titleStr := " " + p.theme.TitleStyle.Render(p.title) + " "

		// Create the final panel with title in the top border
		// We need to manually construct this because lipgloss doesn't have built-in title support
		renderedLines := strings.Split(styledContent, "\n")
		if len(renderedLines) > 0 {
			// Insert title into the top border
			topBorder := renderedLines[0]
			// Find a good position for the title (after the first border character)
			if len(topBorder) > 2 {
				// Simple approach: replace part of the top border with the title
				titleLen := lipgloss.Width(titleStr)
				if titleLen < len(topBorder)-4 {
					// Center the title in the top border
					leftPad := (len(topBorder) - titleLen - 2) / 2
					if leftPad < 1 {
						leftPad = 1
					}
					renderedLines[0] = topBorder[:leftPad] + titleStr + topBorder[leftPad+titleLen:]
				}
			}
		}
		styledContent = strings.Join(renderedLines, "\n")
	}

	return styledContent
}

// SetTheme sets the theme for the panel.
// This allows customization of panel appearance.
func (p *Panel) SetTheme(theme *styles.Theme) {
	if theme != nil {
		p.theme = theme
	}
}
