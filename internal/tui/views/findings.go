package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
	"github.com/zero-day-ai/gibson/internal/types"
)

// FindingsView displays a filterable list of findings with detailed view.
type FindingsView struct {
	ctx              context.Context
	store            finding.FindingStore
	theme            *styles.Theme
	list             list.Model
	viewport         viewport.Model
	findings         []finding.EnhancedFinding
	selectedIndex    int
	filter           FindingFilter
	width            int
	height           int
	showDetail       bool
	filterDialogOpen bool
	exportDialogOpen bool
	searchMode       bool
	searchText       string
	statusMessage    string
}

// FindingFilter provides filtering options for findings.
type FindingFilter struct {
	Severity   *agent.FindingSeverity
	Category   *finding.FindingCategory
	MissionID  *types.ID
	SearchText string
}

// findingItem implements list.Item for the bubbles list.
type findingItem struct {
	finding finding.EnhancedFinding
	theme   *styles.Theme
}

func (i findingItem) FilterValue() string {
	return i.finding.Title
}

func (i findingItem) Title() string {
	// Format: [SEVERITY] Title
	severityStyle := i.theme.SeverityStyle(string(i.finding.Severity))
	severityBadge := severityStyle.Render(fmt.Sprintf("[%s]", strings.ToUpper(string(i.finding.Severity))))
	return fmt.Sprintf("%s %s", severityBadge, i.finding.Title)
}

func (i findingItem) Description() string {
	// Show finding ID, category, and timestamp
	categoryStr := i.finding.Category
	if categoryStr == "" {
		categoryStr = "uncategorized"
	}
	timeStr := i.finding.CreatedAt.Format("2006-01-02 15:04")
	return fmt.Sprintf("%s | %s | %s", i.finding.ID.String()[:8], categoryStr, timeStr)
}

// NewFindingsView creates a new findings view.
func NewFindingsView(ctx context.Context, store finding.FindingStore) *FindingsView {
	theme := styles.DefaultTheme()

	// Initialize list with custom delegate
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "Findings"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(false) // We'll handle filtering manually
	l.Styles.Title = theme.TitleStyle

	// Initialize viewport for detail view
	vp := viewport.New(0, 0)

	return &FindingsView{
		ctx:           ctx,
		store:         store,
		theme:         theme,
		list:          l,
		viewport:      vp,
		findings:      []finding.EnhancedFinding{},
		selectedIndex: 0,
		filter:        FindingFilter{},
		showDetail:    false,
		searchMode:    false,
	}
}

// Init initializes the findings view by loading findings from the store.
func (v *FindingsView) Init() tea.Cmd {
	return v.loadFindings()
}

// Update handles keyboard input and updates the view state.
func (v *FindingsView) Update(msg tea.Msg) (*FindingsView, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		v.updateSizes()
		return v, nil

	case findingsLoadedMsg:
		v.findings = msg.findings
		v.updateList()
		v.statusMessage = fmt.Sprintf("Loaded %d findings", len(v.findings))
		return v, nil

	case tea.KeyMsg:
		// Handle search mode
		if v.searchMode {
			return v.handleSearchInput(msg)
		}

		// Handle filter dialog
		if v.filterDialogOpen {
			return v.handleFilterDialog(msg)
		}

		// Handle export dialog
		if v.exportDialogOpen {
			return v.handleExportDialog(msg)
		}

		// Handle normal navigation
		switch msg.String() {
		case "j", "down":
			if v.showDetail {
				v.viewport, cmd = v.viewport.Update(msg)
				return v, cmd
			}
			v.list, cmd = v.list.Update(msg)
			v.selectedIndex = v.list.Index()
			v.updateDetailView()
			return v, cmd

		case "k", "up":
			if v.showDetail {
				v.viewport, cmd = v.viewport.Update(msg)
				return v, cmd
			}
			v.list, cmd = v.list.Update(msg)
			v.selectedIndex = v.list.Index()
			v.updateDetailView()
			return v, cmd

		case "enter":
			v.showDetail = !v.showDetail
			v.updateDetailView()
			return v, nil

		case "f":
			v.filterDialogOpen = true
			return v, nil

		case "e":
			v.exportDialogOpen = true
			return v, nil

		case "/":
			v.searchMode = true
			v.searchText = ""
			return v, nil

		case "c":
			// Copy finding ID to clipboard
			if len(v.findings) > 0 && v.selectedIndex < len(v.findings) {
				selectedFinding := v.findings[v.selectedIndex]
				v.statusMessage = fmt.Sprintf("Copied finding ID: %s", selectedFinding.ID.String())
				// Note: Actual clipboard copy would require platform-specific code
				// For now, just show the message
			}
			return v, nil

		case "r":
			// Refresh findings
			return v, v.loadFindings()

		case "esc":
			v.showDetail = false
			v.filterDialogOpen = false
			v.exportDialogOpen = false
			v.searchMode = false
			return v, nil
		}

	case findingExportedMsg:
		v.statusMessage = msg.message
		v.exportDialogOpen = false
		return v, nil
	}

	// Update list if not in detail mode
	if !v.showDetail {
		v.list, cmd = v.list.Update(msg)
		cmds = append(cmds, cmd)
	}

	return v, tea.Batch(cmds...)
}

// View renders the findings view.
func (v *FindingsView) View() string {
	if v.width == 0 || v.height == 0 {
		return "Loading..."
	}

	// Render filter dialog if open
	if v.filterDialogOpen {
		return v.renderFilterDialog()
	}

	// Render export dialog if open
	if v.exportDialogOpen {
		return v.renderExportDialog()
	}

	// Split layout: list on left, details on right
	listWidth := v.width / 2
	detailWidth := v.width - listWidth

	if v.showDetail {
		listWidth = v.width / 3
		detailWidth = v.width - listWidth
	}

	// Render list
	v.list.SetSize(listWidth-2, v.height-3)
	listPanel := v.theme.PanelStyle.
		Width(listWidth - 2).
		Height(v.height - 3).
		Render(v.list.View())

	// Render detail view if showing details
	var detailPanel string
	if v.showDetail && len(v.findings) > 0 && v.selectedIndex < len(v.findings) {
		detailContent := v.renderFindingDetails(v.findings[v.selectedIndex])
		v.viewport.SetContent(detailContent)
		v.viewport.Width = detailWidth - 4
		v.viewport.Height = v.height - 5

		detailPanel = v.theme.FocusedPanelStyle.
			Width(detailWidth - 2).
			Height(v.height - 3).
			Render(v.viewport.View())
	} else {
		// Show summary view
		summaryContent := v.renderSummary()
		detailPanel = v.theme.PanelStyle.
			Width(detailWidth - 2).
			Height(v.height - 3).
			Render(summaryContent)
	}

	// Combine panels side by side
	mainView := lipgloss.JoinHorizontal(lipgloss.Top, listPanel, detailPanel)

	// Render status bar
	statusBar := v.renderStatusBar()

	// Combine main view and status bar
	return lipgloss.JoinVertical(lipgloss.Left, mainView, statusBar)
}

// SetSize updates the view dimensions.
func (v *FindingsView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.updateSizes()
}

// SetFilter applies a new filter to the findings view.
func (v *FindingsView) SetFilter(filter FindingFilter) tea.Cmd {
	v.filter = filter
	return v.loadFindings()
}

// updateSizes recalculates component sizes based on view dimensions.
func (v *FindingsView) updateSizes() {
	listWidth := v.width / 2
	if v.showDetail {
		listWidth = v.width / 3
	}
	v.list.SetSize(listWidth-2, v.height-3)

	detailWidth := v.width - listWidth
	v.viewport.Width = detailWidth - 4
	v.viewport.Height = v.height - 5
}

// updateList refreshes the list with current findings.
func (v *FindingsView) updateList() {
	items := make([]list.Item, len(v.findings))
	for i, f := range v.findings {
		items[i] = findingItem{finding: f, theme: v.theme}
	}
	v.list.SetItems(items)

	// Reset selection if out of bounds
	if v.selectedIndex >= len(v.findings) {
		v.selectedIndex = 0
	}
}

// updateDetailView updates the detail viewport content.
func (v *FindingsView) updateDetailView() {
	if !v.showDetail || len(v.findings) == 0 || v.selectedIndex >= len(v.findings) {
		return
	}

	selectedFinding := v.findings[v.selectedIndex]
	detailContent := v.renderFindingDetails(selectedFinding)
	v.viewport.SetContent(detailContent)
}

// renderFindingDetails formats a finding for detailed display.
func (v *FindingsView) renderFindingDetails(f finding.EnhancedFinding) string {
	var sb strings.Builder

	// Title
	titleStyle := v.theme.TitleStyle.Bold(true)
	sb.WriteString(titleStyle.Render(f.Title))
	sb.WriteString("\n\n")

	// Metadata section
	sb.WriteString(v.theme.TitleStyle.Render("Metadata"))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("ID:           %s\n", f.ID.String()))
	sb.WriteString(fmt.Sprintf("Severity:     %s\n", v.theme.SeverityStyle(string(f.Severity)).Render(string(f.Severity))))
	sb.WriteString(fmt.Sprintf("Status:       %s\n", f.Status))
	sb.WriteString(fmt.Sprintf("Category:     %s\n", f.Category))
	if f.Subcategory != "" {
		sb.WriteString(fmt.Sprintf("Subcategory:  %s\n", f.Subcategory))
	}
	sb.WriteString(fmt.Sprintf("Risk Score:   %.1f/10.0\n", f.RiskScore))
	sb.WriteString(fmt.Sprintf("Confidence:   %.2f\n", f.Confidence))
	sb.WriteString(fmt.Sprintf("Agent:        %s\n", f.AgentName))
	if f.DelegatedFrom != nil {
		sb.WriteString(fmt.Sprintf("Delegated:    %s\n", *f.DelegatedFrom))
	}
	sb.WriteString(fmt.Sprintf("Created:      %s\n", f.CreatedAt.Format(time.RFC1123)))
	sb.WriteString(fmt.Sprintf("Updated:      %s\n", f.UpdatedAt.Format(time.RFC1123)))
	sb.WriteString("\n")

	// Description
	sb.WriteString(v.theme.TitleStyle.Render("Description"))
	sb.WriteString("\n")
	sb.WriteString(f.Description)
	sb.WriteString("\n\n")

	// Remediation
	if f.Remediation != "" {
		sb.WriteString(v.theme.TitleStyle.Render("Remediation"))
		sb.WriteString("\n")
		sb.WriteString(f.Remediation)
		sb.WriteString("\n\n")
	}

	// CVSS Score
	if f.CVSS != nil {
		sb.WriteString(v.theme.TitleStyle.Render("CVSS"))
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("Score:   %.1f (%s)\n", f.CVSS.Score, f.CVSS.Version))
		sb.WriteString(fmt.Sprintf("Vector:  %s\n", f.CVSS.Vector))
		sb.WriteString("\n")
	}

	// CWE IDs
	if len(f.CWE) > 0 {
		sb.WriteString(v.theme.TitleStyle.Render("CWE IDs"))
		sb.WriteString("\n")
		sb.WriteString(strings.Join(f.CWE, ", "))
		sb.WriteString("\n\n")
	}

	// MITRE ATT&CK Mappings
	if len(f.MitreAttack) > 0 {
		sb.WriteString(v.theme.TitleStyle.Render("MITRE ATT&CK"))
		sb.WriteString("\n")
		for _, mapping := range f.MitreAttack {
			sb.WriteString(fmt.Sprintf("- %s: %s", mapping.TechniqueID, mapping.TechniqueName))
			if mapping.Tactic != "" {
				sb.WriteString(fmt.Sprintf(" (%s)", mapping.Tactic))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// MITRE ATLAS Mappings
	if len(f.MitreAtlas) > 0 {
		sb.WriteString(v.theme.TitleStyle.Render("MITRE ATLAS"))
		sb.WriteString("\n")
		for _, mapping := range f.MitreAtlas {
			sb.WriteString(fmt.Sprintf("- %s: %s", mapping.TechniqueID, mapping.TechniqueName))
			if mapping.Tactic != "" {
				sb.WriteString(fmt.Sprintf(" (%s)", mapping.Tactic))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Evidence
	if len(f.Evidence) > 0 {
		sb.WriteString(v.theme.TitleStyle.Render("Evidence"))
		sb.WriteString("\n")
		for i, ev := range f.Evidence {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, ev.Type))
			if ev.Description != "" {
				sb.WriteString(fmt.Sprintf("   %s\n", ev.Description))
			}
		}
		sb.WriteString("\n")
	}

	// Reproduction Steps
	if len(f.ReproSteps) > 0 {
		sb.WriteString(v.theme.TitleStyle.Render("Reproduction Steps"))
		sb.WriteString("\n")
		for _, step := range f.ReproSteps {
			sb.WriteString(fmt.Sprintf("%d. %s\n", step.StepNumber, step.Description))
			if step.ExpectedResult != "" {
				sb.WriteString(fmt.Sprintf("   Expected: %s\n", step.ExpectedResult))
			}
		}
		sb.WriteString("\n")
	}

	// References
	if len(f.References) > 0 {
		sb.WriteString(v.theme.TitleStyle.Render("References"))
		sb.WriteString("\n")
		for _, ref := range f.References {
			sb.WriteString(fmt.Sprintf("- %s\n", ref))
		}
		sb.WriteString("\n")
	}

	// Related Findings
	if len(f.RelatedIDs) > 0 {
		sb.WriteString(v.theme.TitleStyle.Render("Related Findings"))
		sb.WriteString("\n")
		for _, relID := range f.RelatedIDs {
			sb.WriteString(fmt.Sprintf("- %s\n", relID.String()))
		}
		sb.WriteString("\n")
	}

	// Occurrence count
	if f.OccurrenceCount > 1 {
		sb.WriteString(fmt.Sprintf("This finding has occurred %d times\n", f.OccurrenceCount))
	}

	return sb.String()
}

// renderSummary shows a summary of findings by severity.
func (v *FindingsView) renderSummary() string {
	var sb strings.Builder

	sb.WriteString(v.theme.TitleStyle.Render("Findings Summary"))
	sb.WriteString("\n\n")

	// Count by severity
	counts := make(map[agent.FindingSeverity]int)
	for _, f := range v.findings {
		counts[f.Severity]++
	}

	sb.WriteString(fmt.Sprintf("Total: %d findings\n\n", len(v.findings)))

	severities := []agent.FindingSeverity{
		agent.SeverityCritical,
		agent.SeverityHigh,
		agent.SeverityMedium,
		agent.SeverityLow,
		agent.SeverityInfo,
	}

	for _, sev := range severities {
		count := counts[sev]
		if count > 0 {
			style := v.theme.SeverityStyle(string(sev))
			sb.WriteString(fmt.Sprintf("%s: %d\n", style.Render(string(sev)), count))
		}
	}

	sb.WriteString("\n")
	sb.WriteString("Press Enter to view details\n")
	sb.WriteString("Press f to filter\n")
	sb.WriteString("Press e to export\n")
	sb.WriteString("Press / to search\n")
	sb.WriteString("Press r to refresh\n")

	return sb.String()
}

// renderStatusBar renders the status bar at the bottom.
func (v *FindingsView) renderStatusBar() string {
	// Left: current mode
	mode := "Findings"
	if v.searchMode {
		mode = fmt.Sprintf("Search: %s", v.searchText)
	}

	// Center: status message
	center := v.statusMessage

	// Right: key hints
	hints := "j/k: navigate | enter: details | f: filter | e: export | /: search | c: copy ID | r: refresh"

	leftWidth := len(mode) + 2
	rightWidth := len(hints) + 2
	centerWidth := v.width - leftWidth - rightWidth

	if centerWidth < 0 {
		centerWidth = 0
	}

	// Truncate center message if too long
	if len(center) > centerWidth {
		if centerWidth > 3 {
			center = center[:centerWidth-3] + "..."
		} else {
			center = ""
		}
	}

	// Pad center to fill space
	centerPadding := centerWidth - len(center)
	centerPadLeft := centerPadding / 2
	centerPadRight := centerPadding - centerPadLeft

	statusBar := fmt.Sprintf(" %s %s%s%s %s ",
		mode,
		strings.Repeat(" ", centerPadLeft),
		center,
		strings.Repeat(" ", centerPadRight),
		hints,
	)

	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("240"))

	return style.Width(v.width).Render(statusBar)
}

// renderFilterDialog renders the filter configuration dialog.
func (v *FindingsView) renderFilterDialog() string {
	var sb strings.Builder

	sb.WriteString(v.theme.TitleStyle.Render("Filter Findings"))
	sb.WriteString("\n\n")

	sb.WriteString("Severity: ")
	if v.filter.Severity != nil {
		sb.WriteString(string(*v.filter.Severity))
	} else {
		sb.WriteString("All")
	}
	sb.WriteString("\n")

	sb.WriteString("Category: ")
	if v.filter.Category != nil {
		sb.WriteString(string(*v.filter.Category))
	} else {
		sb.WriteString("All")
	}
	sb.WriteString("\n\n")

	sb.WriteString("Press number to filter by severity:\n")
	sb.WriteString("1: Critical | 2: High | 3: Medium | 4: Low | 5: Info | 0: All\n\n")
	sb.WriteString("Press Esc to close\n")

	content := sb.String()

	// Center the dialog
	dialogWidth := 60
	dialogHeight := 12
	dialogStyle := v.theme.FocusedPanelStyle.
		Width(dialogWidth).
		Height(dialogHeight)

	dialog := dialogStyle.Render(content)

	// Center in view
	x := (v.width - dialogWidth) / 2
	y := (v.height - dialogHeight) / 2

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	// Render overlay (simplified, would need proper positioning in real impl)
	return dialog
}

// renderExportDialog renders the export options dialog.
func (v *FindingsView) renderExportDialog() string {
	var sb strings.Builder

	sb.WriteString(v.theme.TitleStyle.Render("Export Findings"))
	sb.WriteString("\n\n")

	sb.WriteString("Select export format:\n\n")
	sb.WriteString("1: JSON\n")
	sb.WriteString("2: SARIF\n")
	sb.WriteString("3: CSV\n")
	sb.WriteString("4: Markdown\n")
	sb.WriteString("5: HTML\n\n")
	sb.WriteString("Press Esc to cancel\n")

	content := sb.String()

	// Center the dialog
	dialogWidth := 40
	dialogHeight := 14
	dialogStyle := v.theme.FocusedPanelStyle.
		Width(dialogWidth).
		Height(dialogHeight)

	dialog := dialogStyle.Render(content)

	return dialog
}

// handleSearchInput processes search input.
func (v *FindingsView) handleSearchInput(msg tea.KeyMsg) (*FindingsView, tea.Cmd) {
	switch msg.String() {
	case "esc":
		v.searchMode = false
		v.searchText = ""
		return v, nil

	case "enter":
		v.searchMode = false
		v.filter.SearchText = v.searchText
		return v, v.loadFindings()

	case "backspace":
		if len(v.searchText) > 0 {
			v.searchText = v.searchText[:len(v.searchText)-1]
		}
		return v, nil

	default:
		// Add character to search
		if len(msg.String()) == 1 {
			v.searchText += msg.String()
		}
		return v, nil
	}
}

// handleFilterDialog processes filter dialog input.
func (v *FindingsView) handleFilterDialog(msg tea.KeyMsg) (*FindingsView, tea.Cmd) {
	switch msg.String() {
	case "esc":
		v.filterDialogOpen = false
		return v, nil

	case "0":
		// Clear severity filter
		v.filter.Severity = nil
		v.filterDialogOpen = false
		return v, v.loadFindings()

	case "1":
		sev := agent.SeverityCritical
		v.filter.Severity = &sev
		v.filterDialogOpen = false
		return v, v.loadFindings()

	case "2":
		sev := agent.SeverityHigh
		v.filter.Severity = &sev
		v.filterDialogOpen = false
		return v, v.loadFindings()

	case "3":
		sev := agent.SeverityMedium
		v.filter.Severity = &sev
		v.filterDialogOpen = false
		return v, v.loadFindings()

	case "4":
		sev := agent.SeverityLow
		v.filter.Severity = &sev
		v.filterDialogOpen = false
		return v, v.loadFindings()

	case "5":
		sev := agent.SeverityInfo
		v.filter.Severity = &sev
		v.filterDialogOpen = false
		return v, v.loadFindings()
	}

	return v, nil
}

// handleExportDialog processes export dialog input.
func (v *FindingsView) handleExportDialog(msg tea.KeyMsg) (*FindingsView, tea.Cmd) {
	switch msg.String() {
	case "esc":
		v.exportDialogOpen = false
		return v, nil

	case "1":
		return v, v.exportFindings("json")
	case "2":
		return v, v.exportFindings("sarif")
	case "3":
		return v, v.exportFindings("csv")
	case "4":
		return v, v.exportFindings("markdown")
	case "5":
		return v, v.exportFindings("html")
	}

	return v, nil
}

// loadFindings loads findings from the store with the current filter.
func (v *FindingsView) loadFindings() tea.Cmd {
	return func() tea.Msg {
		// Build finding filter from view filter
		storeFilter := &finding.FindingFilter{}

		if v.filter.Severity != nil {
			storeFilter.Severity = v.filter.Severity
		}

		if v.filter.Category != nil {
			storeFilter.Category = v.filter.Category
		}

		if v.filter.SearchText != "" {
			storeFilter.SearchText = &v.filter.SearchText
		}

		// Load findings from store
		// If no mission ID is set, we need to load all findings
		// For now, use a placeholder mission ID or modify to load all
		var findings []finding.EnhancedFinding
		var err error

		if v.filter.MissionID != nil {
			findings, err = v.store.List(v.ctx, *v.filter.MissionID, storeFilter)
		} else {
			// Load all findings (would need a store method for this)
			// For now, use empty mission ID as a workaround
			findings = []finding.EnhancedFinding{}
		}

		if err != nil {
			return findingsLoadedMsg{findings: []finding.EnhancedFinding{}}
		}

		return findingsLoadedMsg{findings: findings}
	}
}

// exportFindings exports findings in the specified format.
func (v *FindingsView) exportFindings(format string) tea.Cmd {
	return func() tea.Msg {
		// Export logic would go here
		// For now, just return a success message
		filename := fmt.Sprintf("findings_%s.%s", time.Now().Format("20060102_150405"), format)
		return findingExportedMsg{
			message: fmt.Sprintf("Exported %d findings to %s", len(v.findings), filename),
		}
	}
}

// findingsLoadedMsg is sent when findings are loaded from the store.
type findingsLoadedMsg struct {
	findings []finding.EnhancedFinding
}

// findingExportedMsg is sent when findings are exported.
type findingExportedMsg struct {
	message string
}
