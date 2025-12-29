package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// LayoutDirection specifies the direction of the DAG layout.
type LayoutDirection int

const (
	// LayoutHorizontal renders the DAG from left to right.
	LayoutHorizontal LayoutDirection = iota
	// LayoutVertical renders the DAG from top to bottom.
	LayoutVertical
)

// DAGRenderer renders a workflow DAG using ASCII box drawing characters.
// It displays nodes with status indicators and edges with arrows.
type DAGRenderer struct {
	workflow   *workflow.Workflow
	activeNode string
	width      int
	height     int
	layout     LayoutDirection
	theme      *styles.Theme
}

// NewDAGRenderer creates a new DAG renderer for the given workflow.
func NewDAGRenderer(wf *workflow.Workflow) *DAGRenderer {
	return &DAGRenderer{
		workflow:   wf,
		activeNode: "",
		width:      80,
		height:     20,
		layout:     LayoutHorizontal,
		theme:      styles.DefaultTheme(),
	}
}

// SetActiveNode sets the currently active/highlighted node.
func (d *DAGRenderer) SetActiveNode(nodeID string) {
	d.activeNode = nodeID
}

// SetSize sets the dimensions for rendering.
func (d *DAGRenderer) SetSize(width, height int) {
	if width > 0 {
		d.width = width
	}
	if height > 0 {
		d.height = height
	}
}

// SetLayout sets the layout direction.
func (d *DAGRenderer) SetLayout(layout LayoutDirection) {
	d.layout = layout
}

// SetTheme sets the theme for the renderer.
func (d *DAGRenderer) SetTheme(theme *styles.Theme) {
	if theme != nil {
		d.theme = theme
	}
}

// Render renders the DAG to a string.
// It uses ASCII box drawing characters and colors nodes by status.
func (d *DAGRenderer) Render() string {
	if d.workflow == nil || len(d.workflow.Nodes) == 0 {
		return d.renderEmpty()
	}

	// Determine layout based on dimensions
	if d.width < d.height {
		d.layout = LayoutVertical
	}

	// Render based on layout direction
	if d.layout == LayoutVertical {
		return d.renderVertical()
	}
	return d.renderHorizontal()
}

// renderEmpty renders a placeholder when there's no workflow.
func (d *DAGRenderer) renderEmpty() string {
	emptyStyle := lipgloss.NewStyle().
		Foreground(d.theme.Muted).
		Italic(true).
		Width(d.width).
		Height(d.height).
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Center)

	return emptyStyle.Render("No workflow to display")
}

// renderHorizontal renders the DAG in a horizontal layout (left to right).
func (d *DAGRenderer) renderHorizontal() string {
	var lines []string

	// Build a simple linear representation for horizontal layout
	// In a real implementation, we'd do topological sort and level assignment
	entryNodes := d.workflow.GetEntryNodes()
	if len(entryNodes) == 0 {
		return d.renderEmpty()
	}

	// Track nodes we've already rendered
	rendered := make(map[string]bool)
	queue := make([]*workflow.WorkflowNode, len(entryNodes))
	copy(queue, entryNodes)

	var nodeLine strings.Builder
	var arrowLine strings.Builder

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		if rendered[node.ID] {
			continue
		}
		rendered[node.ID] = true

		// Render node box
		nodeStr := d.renderNode(node)
		nodeLine.WriteString(nodeStr)
		arrowLine.WriteString(strings.Repeat(" ", lipgloss.Width(nodeStr)))

		// Add arrow if there are outgoing edges
		hasOutgoing := false
		for _, edge := range d.workflow.Edges {
			if edge.From == node.ID {
				hasOutgoing = true
				// Add target node to queue if not already rendered
				if targetNode := d.workflow.GetNode(edge.To); targetNode != nil && !rendered[edge.To] {
					queue = append(queue, targetNode)
				}
			}
		}

		if hasOutgoing && len(queue) > 0 {
			arrow := " ─→ "
			nodeLine.WriteString(arrow)
			arrowLine.WriteString(strings.Repeat(" ", len(arrow)))
		}
	}

	lines = append(lines, nodeLine.String())

	// Truncate if too wide
	result := strings.Join(lines, "\n")
	if lipgloss.Width(result) > d.width {
		// Simple truncation with ellipsis
		resultLines := strings.Split(result, "\n")
		for i, line := range resultLines {
			if len(line) > d.width-3 {
				resultLines[i] = line[:d.width-3] + "..."
			}
		}
		result = strings.Join(resultLines, "\n")
	}

	return result
}

// renderVertical renders the DAG in a vertical layout (top to bottom).
func (d *DAGRenderer) renderVertical() string {
	var lines []string

	// Build a simple linear representation for vertical layout
	entryNodes := d.workflow.GetEntryNodes()
	if len(entryNodes) == 0 {
		return d.renderEmpty()
	}

	// Track nodes we've already rendered
	rendered := make(map[string]bool)
	queue := make([]*workflow.WorkflowNode, len(entryNodes))
	copy(queue, entryNodes)

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		if rendered[node.ID] {
			continue
		}
		rendered[node.ID] = true

		// Render node box
		nodeStr := d.renderNode(node)
		lines = append(lines, nodeStr)

		// Add arrow if there are outgoing edges
		hasOutgoing := false
		for _, edge := range d.workflow.Edges {
			if edge.From == node.ID {
				hasOutgoing = true
				// Add target node to queue if not already rendered
				if targetNode := d.workflow.GetNode(edge.To); targetNode != nil && !rendered[edge.To] {
					queue = append(queue, targetNode)
				}
			}
		}

		if hasOutgoing && len(queue) > 0 {
			lines = append(lines, "  ↓")
		}
	}

	return strings.Join(lines, "\n")
}

// renderNode renders a single node with its status indicator.
func (d *DAGRenderer) renderNode(node *workflow.WorkflowNode) string {
	// Determine node status (we'll use metadata for now, in real impl would query execution state)
	status := "pending"
	if statusVal, ok := node.Metadata["status"]; ok {
		if statusStr, ok := statusVal.(string); ok {
			status = statusStr
		}
	}

	// Get status indicator and style
	indicator := d.getStatusIndicator(status)
	nodeStyle := d.getNodeStyle(status, node.ID == d.activeNode)

	// Format node name (truncate if needed)
	nodeName := node.Name
	if len(nodeName) > 20 {
		nodeName = nodeName[:17] + "..."
	}

	// Create node representation
	nodeContent := fmt.Sprintf("%s %s", indicator, nodeName)

	// Apply style
	return nodeStyle.Render(nodeContent)
}

// getStatusIndicator returns an emoji/symbol for the given status.
func (d *DAGRenderer) getStatusIndicator(status string) string {
	switch status {
	case "running":
		return "▶"
	case "completed":
		return "✓"
	case "failed":
		return "✗"
	case "pending":
		return "○"
	case "skipped":
		return "⊝"
	case "cancelled":
		return "⊗"
	default:
		return "○"
	}
}

// getNodeStyle returns the style for a node based on status and active state.
func (d *DAGRenderer) getNodeStyle(status string, isActive bool) lipgloss.Style {
	var style lipgloss.Style

	// Base style with border
	if isActive {
		style = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(d.theme.Primary).
			Padding(0, 1).
			Bold(true)
	} else {
		style = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(d.theme.Muted).
			Padding(0, 1)
	}

	// Apply status color from theme
	statusStyle := d.theme.StatusStyle(status)
	style = style.Foreground(statusStyle.GetForeground())

	return style
}

// GetNodeStatus returns the status of a specific node.
// This is a helper method for external code to check node status.
func (d *DAGRenderer) GetNodeStatus(nodeID string) string {
	if d.workflow == nil {
		return "unknown"
	}

	node := d.workflow.GetNode(nodeID)
	if node == nil {
		return "unknown"
	}

	// Get status from metadata
	if statusVal, ok := node.Metadata["status"]; ok {
		if statusStr, ok := statusVal.(string); ok {
			return statusStr
		}
	}

	return "pending"
}
