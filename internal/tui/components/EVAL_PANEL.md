# EvalMetricsPanel Component

The `EvalMetricsPanel` is a TUI component for displaying real-time evaluation metrics in the Gibson framework. It provides a visual dashboard for monitoring agent performance during execution.

## Features

### 1. Overall Score Display
- Color-coded score indicators based on performance thresholds:
  - **Green (●)**: Score ≥ 0.7 (Good performance)
  - **Yellow (◐)**: Score 0.5-0.7 (Acceptable performance)
  - **Red (○)**: Score < 0.5 (Poor performance)
- Three decimal precision for accurate score tracking

### 2. Per-Scorer Breakdown
- Individual scores from each configured scorer
- Easy identification of which scorers are contributing to overall performance
- Organized list format for readability

### 3. Alert Summary
- Total alert count
- Warning alerts (⚠) with count
- Critical alerts (✖) with count and highlighting
- Success indicator (✓) when no issues are present

### 4. Score Trend Visualization
- Sparkline-style visualization using Unicode block characters (▁▂▃▄▅▆▇█)
- Color-coded bars matching score thresholds
- Min/max range display
- Tracks last 100 scores automatically
- Visual display limited to last 30 scores for readability

## Usage

### Basic Usage

```go
import (
    "github.com/zero-day-ai/gibson/internal/eval"
    "github.com/zero-day-ai/gibson/internal/tui/components"
    "github.com/zero-day-ai/gibson/internal/types"
)

// Create panel
panel := components.NewEvalMetricsPanel()
panel.SetSize(70, 30)

// Create evaluation summary
summary := eval.NewEvalSummary(types.NewID())
summary.OverallScore = 0.85
summary.ScorerScores = map[string]float64{
    "accuracy": 0.90,
    "safety":   0.80,
}
summary.TotalAlerts = 0

// Update panel
panel.SetSummary(summary)

// Render
output := panel.Render()
fmt.Println(output)
```

### As a Bubbletea Model

The panel implements `tea.Model` interface and can be used standalone:

```go
import tea "github.com/charmbracelet/bubbletea"

// Create panel
panel := components.NewEvalMetricsPanel()

// Run as bubbletea program
p := tea.NewProgram(panel)
if err := p.Start(); err != nil {
    log.Fatal(err)
}
```

### Handling Updates

```go
// Update via custom messages
msg := components.EvalSummaryMsg{
    Summary: summary,
}
updatedModel, cmd := panel.Update(msg)

// Add alerts
alertMsg := components.EvalAlertMsg{
    Alert: alert,
}
panel.Update(alertMsg)

// Handle window resize
resizeMsg := tea.WindowSizeMsg{Width: 100, Height: 40}
panel.Update(resizeMsg)
```

### Visibility Control

```go
// Hide panel
panel.SetVisible(false)

// Show panel
panel.SetVisible(true)

// Check visibility
if panel.Render() == "" {
    // Panel is hidden
}
```

## API Reference

### Constructor

```go
func NewEvalMetricsPanel() *EvalMetricsPanel
```

Creates a new evaluation metrics panel with default settings:
- Width: 40
- Height: 20
- Visible: true
- Uses default Gibson theme

### Methods

#### SetSummary
```go
func (p *EvalMetricsPanel) SetSummary(summary *eval.EvalSummary)
```
Updates the evaluation summary and adds the overall score to the trend history.

#### AddAlert
```go
func (p *EvalMetricsPanel) AddAlert(alert sdkeval.Alert)
```
Adds an alert to the panel's alert list. Keeps last 100 alerts.

#### SetSize
```go
func (p *EvalMetricsPanel) SetSize(width, height int)
```
Sets the dimensions of the panel. Ignores invalid (≤0) values.

#### SetVisible
```go
func (p *EvalMetricsPanel) SetVisible(visible bool)
```
Controls panel visibility. Hidden panels render as empty string.

#### SetTheme
```go
func (p *EvalMetricsPanel) SetTheme(theme *styles.Theme)
```
Sets the theme for styling. Accepts nil-safe theme updates.

#### Render
```go
func (p *EvalMetricsPanel) Render() string
```
Renders the panel to a string. Can be called directly or via `View()`.

### Bubbletea Model Interface

```go
func (p *EvalMetricsPanel) Init() tea.Cmd
func (p *EvalMetricsPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (p *EvalMetricsPanel) View() string
```

Implements the `tea.Model` interface for use in bubbletea applications.

### Custom Messages

```go
type EvalSummaryMsg struct {
    Summary *eval.EvalSummary
}

type EvalAlertMsg struct {
    Alert sdkeval.Alert
}
```

Use these message types to update the panel in bubbletea applications.

## Design Patterns

### Component Pattern
The panel follows Gibson's component pattern:
- Simple render-only components have a `Render()` method
- Interactive components implement `tea.Model` interface
- Components use lipgloss for styling
- Components accept theme customization

### Integration Pattern
The panel integrates with:
- **EvalSummary** (Task 3): Core data structure for evaluation results
- **EvalEventHandler** (Task 8): Receives real-time updates during execution
- **Gibson Theme**: Uses consistent color scheme and styling
- **Panel Component**: Wraps content in bordered container

## Examples

See `eval_panel_example_test.go` for complete working examples, including:
- Basic usage
- Adding alerts
- Score trend tracking
- Visibility control
- Bubbletea integration

Run the demo:
```bash
cd internal/tui/components
go run eval_panel_demo.go
```

## Testing

Run tests:
```bash
go test ./internal/tui/components -run TestEvalMetricsPanel -v
```

Run examples:
```bash
go test ./internal/tui/components -run Example
```

## Dependencies

- **internal/eval**: EvalSummary data structure
- **sdk/eval**: Alert types and feedback structures
- **internal/tui/styles**: Theme and styling
- **github.com/charmbracelet/bubbletea**: TUI framework
- **github.com/charmbracelet/lipgloss**: Styling library

## Notes

- Score trend automatically caps at 100 scores to prevent unbounded memory growth
- Alert list caps at 100 alerts for the same reason
- Panel uses zero values safely - nil summary shows "No data available"
- Color coding uses Gibson's amber CRT theme for consistency
- Unicode sparkline works in most modern terminals
