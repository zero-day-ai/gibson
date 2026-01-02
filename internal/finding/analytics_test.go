package finding

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// setupTestAnalytics creates a test database and analytics instance
func setupTestAnalytics(t *testing.T) (*FindingAnalytics, *DBFindingStore, types.ID) {
	t.Helper()

	// Create temporary file for database
	tmpFile, err := os.CreateTemp("", "analytics_test_*.db")
	require.NoError(t, err)
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Cleanup on test completion
	t.Cleanup(func() {
		os.Remove(tmpPath)
		os.Remove(tmpPath + "-wal")
		os.Remove(tmpPath + "-shm")
	})

	// Create database
	db, err := database.Open(tmpPath)
	require.NoError(t, err)

	// Run migrations - skip test if FTS5 not available
	migrator := database.NewMigrator(db)
	err = migrator.Migrate(context.Background())
	if err != nil && strings.Contains(err.Error(), "no such module: fts5") {
		t.Skip("Skipping test: FTS5 module not available in SQLite")
	}
	require.NoError(t, err)

	// Create store and analytics
	store := NewDBFindingStore(db)
	analytics := NewFindingAnalytics(store)

	// Create test mission ID
	missionID := types.NewID()

	return analytics, store, missionID
}

// createTestFinding creates a test finding with specified attributes
func createTestFinding(missionID types.ID, severity agent.FindingSeverity, category FindingCategory, status FindingStatus, riskScore float64) EnhancedFinding {
	baseFinding := agent.NewFinding(
		"Test Finding",
		"Test Description",
		severity,
	)

	finding := NewEnhancedFinding(baseFinding, missionID, "test-agent")
	finding.Category = string(category)
	finding.Status = status
	finding.RiskScore = riskScore
	finding.Subcategory = "test_subcategory"

	// Add MITRE mappings
	finding.MitreAttack = []SimpleMitreMapping{
		{
			TechniqueID:   "T1059",
			TechniqueName: "Command and Scripting Interpreter",
			Tactic:        "Execution",
		},
	}

	return finding
}

func TestNewFindingAnalytics(t *testing.T) {
	_, store, _ := setupTestAnalytics(t)
	analytics := NewFindingAnalytics(store)

	assert.NotNil(t, analytics)
	assert.NotNil(t, analytics.store)
}

func TestGetStatistics_Empty(t *testing.T) {
	analytics, _, missionID := setupTestAnalytics(t)
	ctx := context.Background()

	stats, err := analytics.GetStatistics(ctx, missionID)
	require.NoError(t, err)
	require.NotNil(t, stats)

	assert.Equal(t, 0, stats.Total)
	assert.Equal(t, 0, len(stats.BySeverity))
	assert.Equal(t, 0, len(stats.ByCategory))
	assert.Equal(t, 0, len(stats.ByStatus))
	assert.Equal(t, 0.0, stats.AverageRiskScore)
	assert.Equal(t, 0, len(stats.TopMitreTechniques))
}

func TestGetStatistics_WithFindings(t *testing.T) {
	analytics, store, missionID := setupTestAnalytics(t)
	ctx := context.Background()

	// Create test findings with various attributes
	findings := []EnhancedFinding{
		createTestFinding(missionID, agent.SeverityCritical, CategoryJailbreak, StatusOpen, 9.5),
		createTestFinding(missionID, agent.SeverityHigh, CategoryPromptInjection, StatusOpen, 7.5),
		createTestFinding(missionID, agent.SeverityHigh, CategoryPromptInjection, StatusConfirmed, 7.0),
		createTestFinding(missionID, agent.SeverityMedium, CategoryDataExtraction, StatusResolved, 4.5),
		createTestFinding(missionID, agent.SeverityLow, CategoryInformationDisclosure, StatusFalsePositive, 2.0),
	}

	// Store findings
	for _, f := range findings {
		err := store.Store(ctx, f)
		require.NoError(t, err)
	}

	// Get statistics
	stats, err := analytics.GetStatistics(ctx, missionID)
	require.NoError(t, err)
	require.NotNil(t, stats)

	// Verify total
	assert.Equal(t, 5, stats.Total)

	// Verify by severity
	assert.Equal(t, 1, stats.BySeverity[agent.SeverityCritical])
	assert.Equal(t, 2, stats.BySeverity[agent.SeverityHigh])
	assert.Equal(t, 1, stats.BySeverity[agent.SeverityMedium])
	assert.Equal(t, 1, stats.BySeverity[agent.SeverityLow])

	// Verify by category
	assert.Equal(t, 1, stats.ByCategory[CategoryJailbreak])
	assert.Equal(t, 2, stats.ByCategory[CategoryPromptInjection])
	assert.Equal(t, 1, stats.ByCategory[CategoryDataExtraction])
	assert.Equal(t, 1, stats.ByCategory[CategoryInformationDisclosure])

	// Verify by status
	assert.Equal(t, 2, stats.ByStatus[StatusOpen])
	assert.Equal(t, 1, stats.ByStatus[StatusConfirmed])
	assert.Equal(t, 1, stats.ByStatus[StatusResolved])
	assert.Equal(t, 1, stats.ByStatus[StatusFalsePositive])

	// Verify average risk score (9.5 + 7.5 + 7.0 + 4.5 + 2.0) / 5 = 6.1
	assert.InDelta(t, 6.1, stats.AverageRiskScore, 0.01)

	// Verify MITRE techniques
	assert.Greater(t, len(stats.TopMitreTechniques), 0)
	if len(stats.TopMitreTechniques) > 0 {
		assert.Equal(t, "T1059", stats.TopMitreTechniques[0].TechniqueID)
		assert.Equal(t, 5, stats.TopMitreTechniques[0].Count) // All findings have this technique
	}
}

func TestGetStatistics_TopMitreTechniques(t *testing.T) {
	analytics, store, missionID := setupTestAnalytics(t)
	ctx := context.Background()

	// Create findings with different MITRE techniques
	for i := 0; i < 15; i++ {
		finding := createTestFinding(missionID, agent.SeverityHigh, CategoryJailbreak, StatusOpen, 7.0)

		// Add multiple different techniques to test top 10 limit
		finding.MitreAttack = []SimpleMitreMapping{
			{
				TechniqueID:   "T1059",
				TechniqueName: "Command and Scripting Interpreter",
				Tactic:        "Execution",
			},
		}

		// Add unique technique for some findings
		if i < 12 {
			finding.MitreAtlas = []SimpleMitreMapping{
				{
					TechniqueID:   "AML.T0043",
					TechniqueName: "Craft Adversarial Data",
					Tactic:        "ML Model Access",
				},
			}
		}

		err := store.Store(ctx, finding)
		require.NoError(t, err)
	}

	stats, err := analytics.GetStatistics(ctx, missionID)
	require.NoError(t, err)

	// Should have techniques sorted by count
	assert.Greater(t, len(stats.TopMitreTechniques), 0)
	assert.LessOrEqual(t, len(stats.TopMitreTechniques), 10) // Max 10 techniques

	// T1059 should be first (15 occurrences)
	if len(stats.TopMitreTechniques) > 0 {
		assert.Equal(t, "T1059", stats.TopMitreTechniques[0].TechniqueID)
		assert.Equal(t, 15, stats.TopMitreTechniques[0].Count)
	}

	// AML.T0043 should be second (12 occurrences)
	if len(stats.TopMitreTechniques) > 1 {
		assert.Equal(t, "AML.T0043", stats.TopMitreTechniques[1].TechniqueID)
		assert.Equal(t, 12, stats.TopMitreTechniques[1].Count)
	}
}

func TestGetTrends_Empty(t *testing.T) {
	analytics, _, missionID := setupTestAnalytics(t)
	ctx := context.Background()

	trends, err := analytics.GetTrends(ctx, missionID, 24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, len(trends))
}

func TestGetTrends_WithFindings(t *testing.T) {
	analytics, store, missionID := setupTestAnalytics(t)
	ctx := context.Background()

	// Create findings at different times
	now := time.Now()
	findings := []EnhancedFinding{
		createTestFinding(missionID, agent.SeverityHigh, CategoryJailbreak, StatusOpen, 8.0),
		createTestFinding(missionID, agent.SeverityMedium, CategoryPromptInjection, StatusOpen, 5.0),
		createTestFinding(missionID, agent.SeverityLow, CategoryDataExtraction, StatusOpen, 2.0),
	}

	// Set different creation times
	findings[0].CreatedAt = now.Add(-3 * time.Hour)
	findings[1].CreatedAt = now.Add(-2 * time.Hour)
	findings[2].CreatedAt = now.Add(-1 * time.Hour)

	for _, f := range findings {
		err := store.Store(ctx, f)
		require.NoError(t, err)
	}

	// Get trends for last 24 hours
	trends, err := analytics.GetTrends(ctx, missionID, 24*time.Hour)
	require.NoError(t, err)

	// Should have trend points
	assert.Greater(t, len(trends), 0)

	// Verify trend points are ordered by time
	for i := 1; i < len(trends); i++ {
		assert.True(t, trends[i].Timestamp.After(trends[i-1].Timestamp) || trends[i].Timestamp.Equal(trends[i-1].Timestamp))
	}
}

func TestGetRiskScore_Empty(t *testing.T) {
	analytics, _, missionID := setupTestAnalytics(t)
	ctx := context.Background()

	score, err := analytics.GetRiskScore(ctx, missionID)
	require.NoError(t, err)
	assert.Equal(t, 0.0, score)
}

func TestGetRiskScore_WithFindings(t *testing.T) {
	analytics, store, missionID := setupTestAnalytics(t)
	ctx := context.Background()

	// Create findings with known severities
	// Critical = 10, High = 7, Medium = 4, Low = 1
	findings := []EnhancedFinding{
		createTestFinding(missionID, agent.SeverityCritical, CategoryJailbreak, StatusOpen, 9.0),        // weight: 10
		createTestFinding(missionID, agent.SeverityHigh, CategoryPromptInjection, StatusOpen, 7.0),      // weight: 7
		createTestFinding(missionID, agent.SeverityMedium, CategoryDataExtraction, StatusOpen, 4.0),     // weight: 4
		createTestFinding(missionID, agent.SeverityLow, CategoryInformationDisclosure, StatusOpen, 1.0), // weight: 1
	}

	for _, f := range findings {
		err := store.Store(ctx, f)
		require.NoError(t, err)
	}

	score, err := analytics.GetRiskScore(ctx, missionID)
	require.NoError(t, err)

	// Expected: (10 + 7 + 4 + 1) / 4 = 5.5
	assert.InDelta(t, 5.5, score, 0.01)
}

func TestGetRiskScore_ExcludesResolved(t *testing.T) {
	analytics, store, missionID := setupTestAnalytics(t)
	ctx := context.Background()

	// Create findings with some resolved
	findings := []EnhancedFinding{
		createTestFinding(missionID, agent.SeverityCritical, CategoryJailbreak, StatusOpen, 9.0),       // weight: 10
		createTestFinding(missionID, agent.SeverityHigh, CategoryPromptInjection, StatusResolved, 7.0), // excluded
		createTestFinding(missionID, agent.SeverityMedium, CategoryDataExtraction, StatusOpen, 4.0),    // weight: 4
	}

	for _, f := range findings {
		err := store.Store(ctx, f)
		require.NoError(t, err)
	}

	score, err := analytics.GetRiskScore(ctx, missionID)
	require.NoError(t, err)

	// Expected: (10 + 4) / 2 = 7.0 (resolved not counted)
	assert.InDelta(t, 7.0, score, 0.01)
}

func TestGetTopVulnerabilities_Empty(t *testing.T) {
	analytics, _, _ := setupTestAnalytics(t)
	ctx := context.Background()

	patterns, err := analytics.GetTopVulnerabilities(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, len(patterns))
}

func TestGetTopVulnerabilities_WithFindings(t *testing.T) {
	analytics, store, _ := setupTestAnalytics(t)
	ctx := context.Background()

	// Create findings with different category/subcategory combinations
	// Note: GetTopVulnerabilities doesn't filter by mission, so we use dummy IDs
	findings := []EnhancedFinding{
		createTestFinding(types.NewID(), agent.SeverityHigh, CategoryJailbreak, StatusOpen, 8.0),
		createTestFinding(types.NewID(), agent.SeverityHigh, CategoryJailbreak, StatusOpen, 8.5),
		createTestFinding(types.NewID(), agent.SeverityMedium, CategoryPromptInjection, StatusOpen, 5.0),
		createTestFinding(types.NewID(), agent.SeverityLow, CategoryDataExtraction, StatusOpen, 2.0),
	}

	// Set subcategories
	findings[0].Subcategory = "dan_bypass"
	findings[1].Subcategory = "dan_bypass"
	findings[2].Subcategory = "indirect_injection"
	findings[3].Subcategory = "pii_extraction"

	for _, f := range findings {
		err := store.Store(ctx, f)
		require.NoError(t, err)
	}

	patterns, err := analytics.GetTopVulnerabilities(ctx, 10)
	require.NoError(t, err)

	// Should have patterns sorted by count
	assert.Greater(t, len(patterns), 0)

	// First pattern should be jailbreak/dan_bypass (2 occurrences)
	assert.Equal(t, CategoryJailbreak, patterns[0].Category)
	assert.Equal(t, "dan_bypass", patterns[0].Subcategory)
	assert.Equal(t, 2, patterns[0].Count)
}

func TestGetTopVulnerabilities_Limit(t *testing.T) {
	analytics, store, _ := setupTestAnalytics(t)
	ctx := context.Background()

	// Create many different vulnerability patterns
	categories := []FindingCategory{
		CategoryJailbreak,
		CategoryPromptInjection,
		CategoryDataExtraction,
		CategoryInformationDisclosure,
	}

	for i := 0; i < 20; i++ {
		finding := createTestFinding(
			types.NewID(),
			agent.SeverityMedium,
			categories[i%len(categories)],
			StatusOpen,
			5.0,
		)
		finding.Subcategory = "test_" + string(rune(i))
		err := store.Store(ctx, finding)
		require.NoError(t, err)
	}

	// Request top 5
	patterns, err := analytics.GetTopVulnerabilities(ctx, 5)
	require.NoError(t, err)

	// Should be limited to 5
	assert.LessOrEqual(t, len(patterns), 5)
}

func TestGetRemediationProgress_Empty(t *testing.T) {
	analytics, _, missionID := setupTestAnalytics(t)
	ctx := context.Background()

	open, resolved, err := analytics.GetRemediationProgress(ctx, missionID)
	require.NoError(t, err)
	assert.Equal(t, 0, open)
	assert.Equal(t, 0, resolved)
}

func TestGetRemediationProgress_WithFindings(t *testing.T) {
	analytics, store, missionID := setupTestAnalytics(t)
	ctx := context.Background()

	// Create findings with different statuses
	findings := []EnhancedFinding{
		createTestFinding(missionID, agent.SeverityHigh, CategoryJailbreak, StatusOpen, 8.0),
		createTestFinding(missionID, agent.SeverityHigh, CategoryPromptInjection, StatusOpen, 7.0),
		createTestFinding(missionID, agent.SeverityMedium, CategoryDataExtraction, StatusConfirmed, 5.0),
		createTestFinding(missionID, agent.SeverityMedium, CategoryInformationDisclosure, StatusResolved, 4.0),
		createTestFinding(missionID, agent.SeverityLow, CategoryJailbreak, StatusResolved, 2.0),
		createTestFinding(missionID, agent.SeverityLow, CategoryPromptInjection, StatusFalsePositive, 1.0),
	}

	for _, f := range findings {
		err := store.Store(ctx, f)
		require.NoError(t, err)
	}

	open, resolved, err := analytics.GetRemediationProgress(ctx, missionID)
	require.NoError(t, err)

	// Open = 2 (open) + 1 (confirmed) = 3
	assert.Equal(t, 3, open)
	// Resolved = 2
	assert.Equal(t, 2, resolved)
	// False positive not counted in either
}

func TestGetSeverityWeight(t *testing.T) {
	tests := []struct {
		name     string
		severity agent.FindingSeverity
		expected float64
	}{
		{"Critical", agent.SeverityCritical, 10.0},
		{"High", agent.SeverityHigh, 7.0},
		{"Medium", agent.SeverityMedium, 4.0},
		{"Low", agent.SeverityLow, 1.0},
		{"Info", agent.SeverityInfo, 0.5},
		{"Unknown", "unknown", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weight := getSeverityWeight(tt.severity)
			assert.Equal(t, tt.expected, weight)
		})
	}
}
