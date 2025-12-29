package views

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/types"
)

// MockFindingStore is a simple mock for testing
type MockFindingStore struct {
	findings []finding.EnhancedFinding
	err      error
}

func (m *MockFindingStore) Store(ctx context.Context, f finding.EnhancedFinding) error {
	return m.err
}

func (m *MockFindingStore) Get(ctx context.Context, id types.ID) (*finding.EnhancedFinding, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.findings) > 0 {
		return &m.findings[0], nil
	}
	return nil, nil
}

func (m *MockFindingStore) List(ctx context.Context, missionID types.ID, filter *finding.FindingFilter) ([]finding.EnhancedFinding, error) {
	return m.findings, m.err
}

func (m *MockFindingStore) Update(ctx context.Context, f finding.EnhancedFinding) error {
	return m.err
}

func (m *MockFindingStore) Delete(ctx context.Context, id types.ID) error {
	return m.err
}

func (m *MockFindingStore) Count(ctx context.Context, missionID types.ID) (int, error) {
	return len(m.findings), m.err
}

func TestNewFindingsView(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)

	assert.NotNil(t, view)
	assert.NotNil(t, view.theme)
	assert.NotNil(t, view.list)
	assert.NotNil(t, view.viewport)
	assert.False(t, view.showDetail)
	assert.False(t, view.filterDialogOpen)
	assert.False(t, view.exportDialogOpen)
	assert.False(t, view.searchMode)
}

func TestFindingsView_Init(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)

	cmd := view.Init()

	// Init should return a command to load findings
	assert.NotNil(t, cmd)
}

func TestFindingsView_Update_WindowSize(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)

	msg := tea.WindowSizeMsg{Width: 150, Height: 50}
	_, _ = view.Update(msg)

	assert.Equal(t, 150, view.width)
	assert.Equal(t, 50, view.height)
}

func TestFindingsView_Update_ToggleDetails(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)
	view.width = 100
	view.height = 30

	assert.False(t, view.showDetail)

	// Press Enter to toggle
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, _ = view.Update(msg)

	assert.True(t, view.showDetail)
}

func TestFindingsView_Update_OpenFilterDialog(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)

	assert.False(t, view.filterDialogOpen)

	// Press 'f' to open filter
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")}
	_, _ = view.Update(msg)

	assert.True(t, view.filterDialogOpen)
}

func TestFindingsView_Update_OpenExportDialog(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)

	assert.False(t, view.exportDialogOpen)

	// Press 'e' to open export
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")}
	_, _ = view.Update(msg)

	assert.True(t, view.exportDialogOpen)
}

func TestFindingsView_Update_EnterSearchMode(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)

	assert.False(t, view.searchMode)

	// Press '/' to enter search mode
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")}
	_, _ = view.Update(msg)

	assert.True(t, view.searchMode)
}

func TestFindingsView_Update_Escape(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)

	// Open filter dialog
	view.filterDialogOpen = true

	// Press Escape
	msg := tea.KeyMsg{Type: tea.KeyEsc}
	_, _ = view.Update(msg)

	assert.False(t, view.filterDialogOpen)
}

func TestFindingsView_View_Empty(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)

	// Before setting size
	output := view.View()
	assert.Equal(t, "Loading...", output)

	// After setting size
	view.width = 100
	view.height = 30
	output = view.View()
	assert.NotEmpty(t, output)
}

func TestFindingsView_SetSize(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)

	view.SetSize(150, 50)

	assert.Equal(t, 150, view.width)
	assert.Equal(t, 50, view.height)
}

func TestFindingsView_SetFilter(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)

	severity := agent.SeverityHigh
	filter := FindingFilter{
		Severity: &severity,
	}

	cmd := view.SetFilter(filter)

	assert.Equal(t, filter, view.filter)
	assert.NotNil(t, cmd)
}

func TestFindingsView_RenderSummary(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)

	output := view.renderSummary()

	assert.Contains(t, output, "Findings Summary")
	assert.Contains(t, output, "Total: 0 findings")
}

func TestFindingsView_RenderStatusBar(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)
	view.width = 100

	output := view.renderStatusBar()

	assert.NotEmpty(t, output)
	assert.Contains(t, output, "Findings")
}

func TestFindingsView_RenderFilterDialog(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)
	view.width = 100
	view.height = 30
	view.filterDialogOpen = true

	output := view.renderFilterDialog()

	assert.Contains(t, output, "Filter Findings")
	assert.Contains(t, output, "Severity")
}

func TestFindingsView_RenderExportDialog(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)
	view.width = 100
	view.height = 30
	view.exportDialogOpen = true

	output := view.renderExportDialog()

	assert.Contains(t, output, "Export Findings")
	assert.Contains(t, output, "JSON")
	assert.Contains(t, output, "SARIF")
}

func TestFindingsView_HandleSearchInput(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)
	view.searchMode = true

	// Type a character
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")}
	_, _ = view.handleSearchInput(msg)

	assert.Equal(t, "t", view.searchText)

	// Type another
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")}
	_, _ = view.handleSearchInput(msg)

	assert.Equal(t, "te", view.searchText)

	// Backspace
	msg = tea.KeyMsg{Type: tea.KeyBackspace}
	_, _ = view.handleSearchInput(msg)

	assert.Equal(t, "t", view.searchText)

	// Escape
	msg = tea.KeyMsg{Type: tea.KeyEsc}
	_, _ = view.handleSearchInput(msg)

	assert.False(t, view.searchMode)
	assert.Empty(t, view.searchText)
}

func TestFindingsView_HandleFilterDialog(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)
	view.filterDialogOpen = true

	// Press '1' for Critical severity
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")}
	_, _ = view.handleFilterDialog(msg)

	assert.False(t, view.filterDialogOpen)
	assert.NotNil(t, view.filter.Severity)
	assert.Equal(t, agent.SeverityCritical, *view.filter.Severity)
}

func TestFindingsView_HandleExportDialog(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockFindingStore{}
	view := NewFindingsView(ctx, mockStore)
	view.exportDialogOpen = true

	// Press '1' for JSON export
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")}
	_, cmd := view.handleExportDialog(msg)

	// Should return an export command
	assert.NotNil(t, cmd)
}

func TestFindingItem_Interfaces(t *testing.T) {
	theme := findingItem{}.theme // Will be nil, but that's ok for this test

	item := findingItem{
		finding: finding.EnhancedFinding{
			Finding: agent.Finding{
				Title:    "Test Finding",
				Severity: agent.SeverityHigh,
				Category: string(finding.CategoryPromptInjection),
			},
		},
		theme: theme,
	}

	// Test list.Item interface
	assert.Equal(t, "Test Finding", item.FilterValue())
}

func TestFindingsLoadedMsg(t *testing.T) {
	findings := []finding.EnhancedFinding{
		{Finding: agent.Finding{Title: "Finding 1"}},
		{Finding: agent.Finding{Title: "Finding 2"}},
	}
	msg := findingsLoadedMsg{findings: findings}

	assert.Len(t, msg.findings, 2)
}

func TestFindingExportedMsg(t *testing.T) {
	msg := findingExportedMsg{message: "Exported successfully"}
	assert.Equal(t, "Exported successfully", msg.message)
}

func TestFindingFilter(t *testing.T) {
	severity := agent.SeverityHigh
	category := finding.CategoryPromptInjection
	missionID := types.NewID()

	filter := FindingFilter{
		Severity:   &severity,
		Category:   &category,
		MissionID:  &missionID,
		SearchText: "test",
	}

	assert.Equal(t, agent.SeverityHigh, *filter.Severity)
	assert.Equal(t, finding.CategoryPromptInjection, *filter.Category)
	assert.Equal(t, missionID, *filter.MissionID)
	assert.Equal(t, "test", filter.SearchText)
}
