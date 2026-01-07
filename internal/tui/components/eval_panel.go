package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/eval"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
	sdkeval "github.com/zero-day-ai/sdk/eval"
)

// EvalMetricsPanel is a TUI component for displaying real-time evaluation metrics.
// It shows overall scores, per-scorer scores, alerts, and score trends using lipgloss styling.
type EvalMetricsPanel struct {
	width      int
	height     int
	summary    *eval.EvalSummary
	alerts     []sdkeval.Alert
	scoreTrend []float64 // History of overall scores for trend visualization
	theme      *styles.Theme
	visible    bool
}

// NewEvalMetricsPanel creates a new evaluation metrics panel with default dimensions.
// The panel is visible by default and uses the default Gibson theme.
func NewEvalMetricsPanel() *EvalMetricsPanel {
	return &EvalMetricsPanel{
		width:      40,
		height:     20,
		summary:    nil,
		alerts:     []sdkeval.Alert{},
		scoreTrend: []float64{},
		theme:      styles.DefaultTheme(),
		visible:    true,
	}
}

// Init implements tea.Model interface.
// Returns nil as the panel has no initialization commands.
func (p *EvalMetricsPanel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model interface.
// Handles window resize and custom messages for updating eval data.
func (p *EvalMetricsPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.SetSize(msg.Width, msg.Height)
		return p, nil

	case EvalSummaryMsg:
		p.SetSummary(msg.Summary)
		return p, nil

	case EvalAlertMsg:
		p.AddAlert(msg.Alert)
		return p, nil
	}

	return p, nil
}

// View implements tea.Model interface.
// Renders the panel using the Render method.
func (p *EvalMetricsPanel) View() string {
	return p.Render()
}

// Render renders the evaluation metrics panel to a string.
// This is the main rendering method that can be called directly or via View().
func (p *EvalMetricsPanel) Render() string {
	if !p.visible {
		return ""
	}

	// Build panel content
	var content strings.Builder

	// Title
	titleStyle := p.theme.TitleStyle
	content.WriteString(titleStyle.Render("EVALUATION METRICS"))
	content.WriteString("\n\n")

	if p.summary == nil {
		// No data yet
		emptyStyle := lipgloss.NewStyle().Foreground(p.theme.Muted).Italic(true)
		content.WriteString(emptyStyle.Render("No evaluation data available"))
	} else {
		// Overall Score with color coding
		content.WriteString(p.renderOverallScore())
		content.WriteString("\n\n")

		// Per-scorer scores
		if len(p.summary.ScorerScores) > 0 {
			content.WriteString(p.renderScorerScores())
			content.WriteString("\n\n")
		}

		// Alerts summary
		content.WriteString(p.renderAlertsSummary())
		content.WriteString("\n\n")

		// Score trend
		if len(p.scoreTrend) > 0 {
			content.WriteString(p.renderScoreTrend())
		}
	}

	// Wrap in a panel
	panel := NewPanel("Evaluation")
	panel.SetContent(content.String())
	panel.SetSize(p.width, p.height)
	panel.SetTheme(p.theme)

	return panel.Render()
}

// renderOverallScore renders the overall evaluation score with color coding.
// Green > 0.7, yellow 0.5-0.7, red < 0.5
func (p *EvalMetricsPanel) renderOverallScore() string {
	score := p.summary.OverallScore
	scoreStr := fmt.Sprintf("%.3f", score)

	// Determine color based on score thresholds
	var scoreStyle lipgloss.Style
	var indicator string

	if score >= 0.7 {
		// Good score - use success style (bright amber)
		scoreStyle = lipgloss.NewStyle().
			Foreground(p.theme.Success).
			Bold(true)
		indicator = "●" // Filled circle
	} else if score >= 0.5 {
		// Warning score - use warning style (medium amber)
		scoreStyle = lipgloss.NewStyle().
			Foreground(p.theme.Warning).
			Bold(true)
		indicator = "◐" // Half-filled circle
	} else {
		// Low score - use danger style (dim amber)
		scoreStyle = lipgloss.NewStyle().
			Foreground(p.theme.Danger).
			Bold(true)
		indicator = "○" // Empty circle
	}

	labelStyle := lipgloss.NewStyle().Foreground(p.theme.Primary)
	label := labelStyle.Render("Overall Score: ")
	scoreDisplay := scoreStyle.Render(indicator + " " + scoreStr)

	return label + scoreDisplay
}

// renderScorerScores renders individual scores from each scorer.
func (p *EvalMetricsPanel) renderScorerScores() string {
	var lines []string

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(p.theme.Primary).
		Underline(true)
	lines = append(lines, titleStyle.Render("Per-Scorer Breakdown:"))

	// Sort scorers by name for consistent display (optional - could sort by score)
	scorerNames := make([]string, 0, len(p.summary.ScorerScores))
	for name := range p.summary.ScorerScores {
		scorerNames = append(scorerNames, name)
	}

	// Render each scorer
	nameStyle := lipgloss.NewStyle().Foreground(p.theme.Muted)
	scoreStyle := lipgloss.NewStyle().Foreground(p.theme.Primary)

	for _, name := range scorerNames {
		score := p.summary.ScorerScores[name]
		scoreLine := fmt.Sprintf("  %s: %s",
			nameStyle.Render(name),
			scoreStyle.Render(fmt.Sprintf("%.3f", score)))
		lines = append(lines, scoreLine)
	}

	return strings.Join(lines, "\n")
}

// renderAlertsSummary renders the alert count with warning/critical indicators.
func (p *EvalMetricsPanel) renderAlertsSummary() string {
	var parts []string

	labelStyle := lipgloss.NewStyle().Foreground(p.theme.Primary)
	parts = append(parts, labelStyle.Render("Alerts: "))

	// Total alerts
	totalStyle := lipgloss.NewStyle().Foreground(p.theme.Muted)
	parts = append(parts, totalStyle.Render(fmt.Sprintf("%d total", p.summary.TotalAlerts)))

	// Warning count with yellow indicator
	if p.summary.WarningCount > 0 {
		warningStyle := lipgloss.NewStyle().
			Foreground(p.theme.Warning).
			Bold(true)
		parts = append(parts, " | ")
		parts = append(parts, warningStyle.Render(fmt.Sprintf("⚠ %d warning", p.summary.WarningCount)))
		if p.summary.WarningCount != 1 {
			parts[len(parts)-1] = warningStyle.Render(fmt.Sprintf("⚠ %d warnings", p.summary.WarningCount))
		}
	}

	// Critical count with red indicator
	if p.summary.CriticalCount > 0 {
		criticalStyle := lipgloss.NewStyle().
			Foreground(p.theme.Danger).
			Bold(true).
			Background(lipgloss.Color("#000000"))
		parts = append(parts, " | ")
		parts = append(parts, criticalStyle.Render(fmt.Sprintf("✖ %d critical", p.summary.CriticalCount)))
		if p.summary.CriticalCount != 1 {
			parts[len(parts)-1] = criticalStyle.Render(fmt.Sprintf("✖ %d critical", p.summary.CriticalCount))
		}
	}

	// If no alerts, show success indicator
	if p.summary.TotalAlerts == 0 {
		successStyle := lipgloss.NewStyle().
			Foreground(p.theme.Success)
		parts = append(parts, " ")
		parts = append(parts, successStyle.Render("✓ No issues"))
	}

	return strings.Join(parts, "")
}

// renderScoreTrend renders a simple score trend visualization.
// Uses sparkline-style bars to show score history.
func (p *EvalMetricsPanel) renderScoreTrend() string {
	var lines []string

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(p.theme.Primary).
		Underline(true)
	lines = append(lines, titleStyle.Render("Score Trend:"))

	// Limit trend to last N scores to fit in panel width
	maxBars := 30
	trendData := p.scoreTrend
	if len(trendData) > maxBars {
		trendData = trendData[len(trendData)-maxBars:]
	}

	// Build sparkline using vertical bars
	// Unicode block characters: ▁▂▃▄▅▆▇█
	blocks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	var sparkline strings.Builder
	for _, score := range trendData {
		// Map score (0.0-1.0) to block index (0-7)
		blockIdx := int(score * float64(len(blocks)-1))
		if blockIdx < 0 {
			blockIdx = 0
		}
		if blockIdx >= len(blocks) {
			blockIdx = len(blocks) - 1
		}

		// Color based on score
		var blockStyle lipgloss.Style
		if score >= 0.7 {
			blockStyle = lipgloss.NewStyle().Foreground(p.theme.Success)
		} else if score >= 0.5 {
			blockStyle = lipgloss.NewStyle().Foreground(p.theme.Warning)
		} else {
			blockStyle = lipgloss.NewStyle().Foreground(p.theme.Danger)
		}

		sparkline.WriteString(blockStyle.Render(string(blocks[blockIdx])))
	}

	lines = append(lines, "  "+sparkline.String())

	// Add min/max labels
	if len(trendData) > 0 {
		minScore := trendData[0]
		maxScore := trendData[0]
		for _, s := range trendData {
			if s < minScore {
				minScore = s
			}
			if s > maxScore {
				maxScore = s
			}
		}

		labelStyle := lipgloss.NewStyle().Foreground(p.theme.Muted)
		statsLine := fmt.Sprintf("  %s min: %.3f  max: %.3f",
			labelStyle.Render("Range:"),
			minScore,
			maxScore)
		lines = append(lines, labelStyle.Render(statsLine))
	}

	return strings.Join(lines, "\n")
}

// SetSummary updates the evaluation summary and adds the overall score to the trend history.
func (p *EvalMetricsPanel) SetSummary(summary *eval.EvalSummary) {
	p.summary = summary

	// Add overall score to trend
	if summary != nil {
		p.scoreTrend = append(p.scoreTrend, summary.OverallScore)

		// Keep only last 100 scores for trend
		if len(p.scoreTrend) > 100 {
			p.scoreTrend = p.scoreTrend[len(p.scoreTrend)-100:]
		}
	}
}

// AddAlert adds an alert to the panel's alert list.
// This method is provided for compatibility but alerts are also tracked in the summary.
func (p *EvalMetricsPanel) AddAlert(alert sdkeval.Alert) {
	p.alerts = append(p.alerts, alert)

	// Keep only last 100 alerts
	if len(p.alerts) > 100 {
		p.alerts = p.alerts[len(p.alerts)-100:]
	}
}

// SetSize sets the dimensions of the panel.
func (p *EvalMetricsPanel) SetSize(width, height int) {
	if width > 0 {
		p.width = width
	}
	if height > 0 {
		p.height = height
	}
}

// SetVisible sets the visibility state of the panel.
// When invisible, Render() returns an empty string.
func (p *EvalMetricsPanel) SetVisible(visible bool) {
	p.visible = visible
}

// IsVisible returns the current visibility state of the panel.
func (p *EvalMetricsPanel) IsVisible() bool {
	return p.visible
}

// SetTheme sets the theme for the panel.
func (p *EvalMetricsPanel) SetTheme(theme *styles.Theme) {
	if theme != nil {
		p.theme = theme
	}
}

// EvalSummaryMsg is a custom message type for updating the eval summary.
type EvalSummaryMsg struct {
	Summary *eval.EvalSummary
}

// EvalAlertMsg is a custom message type for adding an alert.
type EvalAlertMsg struct {
	Alert sdkeval.Alert
}
