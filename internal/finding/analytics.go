package finding

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
)

// FindingAnalytics provides analytics and statistics for findings
type FindingAnalytics struct {
	store *DBFindingStore
}

// FindingStats represents aggregated statistics for findings
type FindingStats struct {
	Total              int                              `json:"total"`
	BySeverity         map[agent.FindingSeverity]int    `json:"by_severity"`
	ByCategory         map[FindingCategory]int          `json:"by_category"`
	ByStatus           map[FindingStatus]int            `json:"by_status"`
	AverageRiskScore   float64                          `json:"average_risk_score"`
	TopMitreTechniques []TechniqueCount                 `json:"top_mitre_techniques"`
}

// TechniqueCount represents a MITRE technique with its occurrence count
type TechniqueCount struct {
	TechniqueID   string `json:"technique_id"`
	TechniqueName string `json:"technique_name"`
	Count         int    `json:"count"`
}

// TrendPoint represents a point in time-series data
type TrendPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Count     int       `json:"count"`
	RiskScore float64   `json:"risk_score"`
}

// VulnerabilityPattern represents a common vulnerability pattern
type VulnerabilityPattern struct {
	Category    FindingCategory `json:"category"`
	Subcategory string          `json:"subcategory"`
	Count       int             `json:"count"`
	AvgSeverity float64         `json:"avg_severity"`
}

// NewFindingAnalytics creates a new analytics instance
func NewFindingAnalytics(store *DBFindingStore) *FindingAnalytics {
	return &FindingAnalytics{store: store}
}

// GetStatistics returns aggregated statistics for a mission's findings
func (a *FindingAnalytics) GetStatistics(ctx context.Context, missionID types.ID) (*FindingStats, error) {
	stats := &FindingStats{
		BySeverity:         make(map[agent.FindingSeverity]int),
		ByCategory:         make(map[FindingCategory]int),
		ByStatus:           make(map[FindingStatus]int),
		TopMitreTechniques: []TechniqueCount{},
	}

	// Collect MITRE techniques
	techniqueMap := make(map[string]*TechniqueCount)

	// Get counts by severity, category, and status
	severityQuery := `
		SELECT severity, COUNT(*) as count
		FROM findings
		WHERE mission_id = ?
		GROUP BY severity
	`
	rows, err := a.store.db.QueryContext(ctx, severityQuery, missionID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query severity stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var severity agent.FindingSeverity
		var count int
		if err := rows.Scan(&severity, &count); err != nil {
			return nil, fmt.Errorf("failed to scan severity: %w", err)
		}
		stats.BySeverity[severity] = count
		stats.Total += count
	}

	// Get counts by category
	categoryQuery := `
		SELECT category, COUNT(*) as count
		FROM findings
		WHERE mission_id = ?
		GROUP BY category
	`
	rows2, err := a.store.db.QueryContext(ctx, categoryQuery, missionID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query category stats: %w", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var category FindingCategory
		var count int
		if err := rows2.Scan(&category, &count); err != nil {
			return nil, fmt.Errorf("failed to scan category: %w", err)
		}
		stats.ByCategory[category] = count
	}

	// Get counts by status
	statusQuery := `
		SELECT status, COUNT(*) as count
		FROM findings
		WHERE mission_id = ?
		GROUP BY status
	`
	rows3, err := a.store.db.QueryContext(ctx, statusQuery, missionID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query status stats: %w", err)
	}
	defer rows3.Close()

	for rows3.Next() {
		var status FindingStatus
		var count int
		if err := rows3.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan status: %w", err)
		}
		stats.ByStatus[status] = count
	}

	// Get average risk score
	riskQuery := `
		SELECT COALESCE(AVG(risk_score), 0.0) as avg_risk
		FROM findings
		WHERE mission_id = ?
	`
	err = a.store.db.QueryRowContext(ctx, riskQuery, missionID.String()).Scan(&stats.AverageRiskScore)
	if err != nil {
		return nil, fmt.Errorf("failed to query average risk: %w", err)
	}

	// Extract and count MITRE techniques
	techniqueQuery := `
		SELECT mitre_attack, mitre_atlas
		FROM findings
		WHERE mission_id = ?
	`
	rows4, err := a.store.db.QueryContext(ctx, techniqueQuery, missionID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query MITRE techniques: %w", err)
	}
	defer rows4.Close()

	for rows4.Next() {
		var mitreAttackJSON, mitreAtlasJSON string
		if err := rows4.Scan(&mitreAttackJSON, &mitreAtlasJSON); err != nil {
			return nil, fmt.Errorf("failed to scan MITRE data: %w", err)
		}

		// Parse MITRE ATT&CK techniques
		if mitreAttackJSON != "" && mitreAttackJSON != "null" {
			var techniques []MitreMapping
			if err := json.Unmarshal([]byte(mitreAttackJSON), &techniques); err == nil {
				for _, tech := range techniques {
					if existing, ok := techniqueMap[tech.TechniqueID]; ok {
						existing.Count++
					} else {
						techniqueMap[tech.TechniqueID] = &TechniqueCount{
							TechniqueID:   tech.TechniqueID,
							TechniqueName: tech.TechniqueName,
							Count:         1,
						}
					}
				}
			}
		}

		// Parse MITRE ATLAS techniques
		if mitreAtlasJSON != "" && mitreAtlasJSON != "null" {
			var techniques []MitreMapping
			if err := json.Unmarshal([]byte(mitreAtlasJSON), &techniques); err == nil {
				for _, tech := range techniques {
					if existing, ok := techniqueMap[tech.TechniqueID]; ok {
						existing.Count++
					} else {
						techniqueMap[tech.TechniqueID] = &TechniqueCount{
							TechniqueID:   tech.TechniqueID,
							TechniqueName: tech.TechniqueName,
							Count:         1,
						}
					}
				}
			}
		}
	}

	// Convert map to sorted slice (top techniques)
	for _, tc := range techniqueMap {
		stats.TopMitreTechniques = append(stats.TopMitreTechniques, *tc)
	}

	// Sort by count (descending)
	for i := 0; i < len(stats.TopMitreTechniques); i++ {
		for j := i + 1; j < len(stats.TopMitreTechniques); j++ {
			if stats.TopMitreTechniques[j].Count > stats.TopMitreTechniques[i].Count {
				stats.TopMitreTechniques[i], stats.TopMitreTechniques[j] = stats.TopMitreTechniques[j], stats.TopMitreTechniques[i]
			}
		}
	}

	// Limit to top 10
	if len(stats.TopMitreTechniques) > 10 {
		stats.TopMitreTechniques = stats.TopMitreTechniques[:10]
	}

	return stats, nil
}

// GetTrends returns time-series data showing finding trends over a period
func (a *FindingAnalytics) GetTrends(ctx context.Context, missionID types.ID, period time.Duration) ([]TrendPoint, error) {
	// Calculate the start time based on period
	startTime := time.Now().Add(-period)

	// Query findings grouped by time buckets
	query := `
		SELECT
			strftime('%Y-%m-%d %H:00:00', created_at) as bucket,
			COUNT(*) as count,
			AVG(risk_score) as avg_risk
		FROM findings
		WHERE mission_id = ? AND created_at >= ?
		GROUP BY bucket
		ORDER BY bucket ASC
	`

	rows, err := a.store.db.QueryContext(ctx, query, missionID.String(), startTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query trends: %w", err)
	}
	defer rows.Close()

	var trends []TrendPoint
	for rows.Next() {
		var (
			bucketStr string
			count     int
			avgRisk   sql.NullFloat64
		)
		if err := rows.Scan(&bucketStr, &count, &avgRisk); err != nil {
			return nil, fmt.Errorf("failed to scan trend: %w", err)
		}

		timestamp, err := time.Parse("2006-01-02 15:04:05", bucketStr)
		if err != nil {
			// Skip invalid timestamps
			continue
		}

		riskScore := 0.0
		if avgRisk.Valid {
			riskScore = avgRisk.Float64
		}

		trends = append(trends, TrendPoint{
			Timestamp: timestamp,
			Count:     count,
			RiskScore: riskScore,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating trends: %w", err)
	}

	return trends, nil
}

// GetRiskScore calculates a weighted aggregate risk score for a mission
// Uses severity-based weighting: critical=10, high=7, medium=4, low=1
func (a *FindingAnalytics) GetRiskScore(ctx context.Context, missionID types.ID) (float64, error) {
	query := `
		SELECT severity, COUNT(*) as count
		FROM findings
		WHERE mission_id = ? AND status != ?
		GROUP BY severity
	`

	rows, err := a.store.db.QueryContext(ctx, query, missionID.String(), StatusResolved)
	if err != nil {
		return 0, fmt.Errorf("failed to query risk score: %w", err)
	}
	defer rows.Close()

	var totalScore float64
	var totalCount int

	for rows.Next() {
		var severity agent.FindingSeverity
		var count int
		if err := rows.Scan(&severity, &count); err != nil {
			return 0, fmt.Errorf("failed to scan severity: %w", err)
		}

		weight := getSeverityWeight(severity)
		totalScore += weight * float64(count)
		totalCount += count
	}

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("error iterating risk scores: %w", err)
	}

	if totalCount == 0 {
		return 0, nil
	}

	// Return average weighted score
	return totalScore / float64(totalCount), nil
}

// GetTopVulnerabilities returns the most common vulnerability patterns
func (a *FindingAnalytics) GetTopVulnerabilities(ctx context.Context, limit int) ([]VulnerabilityPattern, error) {
	query := `
		SELECT
			category,
			subcategory,
			COUNT(*) as count,
			AVG(CASE severity
				WHEN 'critical' THEN 5
				WHEN 'high' THEN 4
				WHEN 'medium' THEN 3
				WHEN 'low' THEN 2
				WHEN 'info' THEN 1
				ELSE 0
			END) as avg_severity
		FROM findings
		GROUP BY category, subcategory
		ORDER BY count DESC
		LIMIT ?
	`

	rows, err := a.store.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top vulnerabilities: %w", err)
	}
	defer rows.Close()

	var patterns []VulnerabilityPattern
	for rows.Next() {
		var pattern VulnerabilityPattern
		var subcategory sql.NullString
		if err := rows.Scan(&pattern.Category, &subcategory, &pattern.Count, &pattern.AvgSeverity); err != nil {
			return nil, fmt.Errorf("failed to scan vulnerability pattern: %w", err)
		}
		if subcategory.Valid {
			pattern.Subcategory = subcategory.String
		}
		patterns = append(patterns, pattern)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating patterns: %w", err)
	}

	return patterns, nil
}

// GetRemediationProgress returns the count of open vs resolved findings
func (a *FindingAnalytics) GetRemediationProgress(ctx context.Context, missionID types.ID) (open, resolved int, err error) {
	query := `
		SELECT status, COUNT(*) as count
		FROM findings
		WHERE mission_id = ?
		GROUP BY status
	`

	rows, err := a.store.db.QueryContext(ctx, query, missionID.String())
	if err != nil {
		return 0, 0, fmt.Errorf("failed to query remediation progress: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status FindingStatus
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return 0, 0, fmt.Errorf("failed to scan status: %w", err)
		}

		switch status {
		case StatusResolved:
			resolved += count
		case StatusOpen, StatusConfirmed:
			open += count
		// StatusFalsePositive not counted in either
		}
	}

	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("error iterating remediation progress: %w", err)
	}

	return open, resolved, nil
}

// getSeverityWeight returns the numeric weight for a severity level
func getSeverityWeight(severity agent.FindingSeverity) float64 {
	switch severity {
	case agent.SeverityCritical:
		return 10.0
	case agent.SeverityHigh:
		return 7.0
	case agent.SeverityMedium:
		return 4.0
	case agent.SeverityLow:
		return 1.0
	case agent.SeverityInfo:
		return 0.5
	default:
		return 0.0
	}
}
