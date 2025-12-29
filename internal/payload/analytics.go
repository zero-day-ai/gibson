package payload

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
)

// EffectivenessTracker tracks and analyzes payload effectiveness over time
type EffectivenessTracker interface {
	// RecordExecution records an execution for analytics tracking
	RecordExecution(ctx context.Context, execution *Execution) error

	// GetPayloadStats returns statistics for a specific payload
	GetPayloadStats(ctx context.Context, payloadID types.ID) (*PayloadStats, error)

	// GetCategoryStats returns aggregated statistics by category
	GetCategoryStats(ctx context.Context, category PayloadCategory) (*CategoryStats, error)

	// GetTargetTypeStats returns statistics by target type
	GetTargetTypeStats(ctx context.Context, targetType types.TargetType) (*TargetTypeStats, error)

	// GetRecommendations returns recommended payloads based on effectiveness
	GetRecommendations(ctx context.Context, filter RecommendationFilter) ([]*PayloadRecommendation, error)

	// ExportStats exports statistics in various formats
	ExportStats(ctx context.Context, format ExportFormat, writer io.Writer) error
}

// PayloadStats contains effectiveness statistics for a single payload
type PayloadStats struct {
	PayloadID   types.ID `json:"payload_id"`
	PayloadName string   `json:"payload_name"`

	// Execution counts
	TotalExecutions    int `json:"total_executions"`
	SuccessfulAttacks  int `json:"successful_attacks"`
	FailedExecutions   int `json:"failed_executions"`
	TimeoutCount       int `json:"timeout_count"`

	// Success metrics
	SuccessRate       float64 `json:"success_rate"`        // 0.0 - 1.0
	ConfidenceLevel   float64 `json:"confidence_level"`    // Statistical confidence based on sample size
	AverageConfidence float64 `json:"average_confidence"`  // Average confidence score of successful attacks

	// Performance metrics
	AverageDuration     time.Duration `json:"average_duration"`
	MedianDuration      time.Duration `json:"median_duration"`
	AverageTokensUsed   float64       `json:"average_tokens_used"`
	AverageCost         float64       `json:"average_cost"`
	TotalCost           float64       `json:"total_cost"`

	// Finding metrics
	FindingsCreated     int     `json:"findings_created"`
	FindingCreationRate float64 `json:"finding_creation_rate"` // Percentage of successful attacks that created findings

	// Target breakdown
	TargetTypeBreakdown map[types.TargetType]*TargetTypeBreakdown `json:"target_type_breakdown"`

	// Temporal data
	FirstExecution time.Time `json:"first_execution"`
	LastExecution  time.Time `json:"last_execution"`
	LastSuccess    *time.Time `json:"last_success,omitempty"`

	// Trend data (last 30 days)
	RecentSuccessRate float64 `json:"recent_success_rate"` // Success rate in last 30 days
	Trending          string  `json:"trending"`            // "up", "down", "stable"
}

// TargetTypeBreakdown contains statistics for a specific target type
type TargetTypeBreakdown struct {
	TargetType        types.TargetType `json:"target_type"`
	Executions        int              `json:"executions"`
	Successes         int              `json:"successes"`
	SuccessRate       float64          `json:"success_rate"`
	AverageDuration   time.Duration    `json:"average_duration"`
	AverageConfidence float64          `json:"average_confidence"`
}

// CategoryStats contains aggregated statistics for a payload category
type CategoryStats struct {
	Category PayloadCategory `json:"category"`

	// Payload counts
	TotalPayloads   int `json:"total_payloads"`
	EnabledPayloads int `json:"enabled_payloads"`

	// Execution counts
	TotalExecutions   int `json:"total_executions"`
	SuccessfulAttacks int `json:"successful_attacks"`
	FailedExecutions  int `json:"failed_executions"`

	// Aggregate metrics
	SuccessRate       float64       `json:"success_rate"`
	AverageDuration   time.Duration `json:"average_duration"`
	AverageConfidence float64       `json:"average_confidence"`
	TotalCost         float64       `json:"total_cost"`

	// Finding metrics
	FindingsCreated int     `json:"findings_created"`
	FindingRate     float64 `json:"finding_rate"`

	// Top performing payloads in this category
	TopPayloads []*PayloadStats `json:"top_payloads,omitempty"`
}

// TargetTypeStats contains statistics for executions against a specific target type
type TargetTypeStats struct {
	TargetType types.TargetType `json:"target_type"`

	// Execution counts
	TotalExecutions   int `json:"total_executions"`
	SuccessfulAttacks int `json:"successful_attacks"`
	FailedExecutions  int `json:"failed_executions"`

	// Aggregate metrics
	SuccessRate       float64       `json:"success_rate"`
	AverageDuration   time.Duration `json:"average_duration"`
	AverageConfidence float64       `json:"average_confidence"`

	// Category breakdown
	CategoryBreakdown map[PayloadCategory]*CategoryBreakdown `json:"category_breakdown"`

	// Most effective payloads for this target type
	MostEffective []*PayloadStats `json:"most_effective,omitempty"`
}

// CategoryBreakdown contains category-specific statistics within a target type
type CategoryBreakdown struct {
	Category    PayloadCategory `json:"category"`
	Executions  int             `json:"executions"`
	Successes   int             `json:"successes"`
	SuccessRate float64         `json:"success_rate"`
}

// RecommendationFilter defines filters for payload recommendations
type RecommendationFilter struct {
	TargetType       *types.TargetType  `json:"target_type,omitempty"`
	Category         *PayloadCategory   `json:"category,omitempty"`
	MinSuccessRate   float64            `json:"min_success_rate"`   // Minimum success rate (0.0 - 1.0)
	MinExecutions    int                `json:"min_executions"`     // Minimum execution count for confidence
	Severity         *agent.FindingSeverity `json:"severity,omitempty"`
	MaxDuration      *time.Duration     `json:"max_duration,omitempty"`
	MaxCost          *float64           `json:"max_cost,omitempty"`
	Limit            int                `json:"limit"` // Maximum number of recommendations
}

// PayloadRecommendation represents a recommended payload with context
type PayloadRecommendation struct {
	Payload     *Payload      `json:"payload"`
	Stats       *PayloadStats `json:"stats"`
	Score       float64       `json:"score"`        // Recommendation score (0.0 - 1.0)
	Reason      string        `json:"reason"`       // Why this payload is recommended
	Confidence  float64       `json:"confidence"`   // Confidence in the recommendation
}

// ExportFormat defines the format for exporting statistics
type ExportFormat string

const (
	ExportFormatJSON ExportFormat = "json"
	ExportFormatCSV  ExportFormat = "csv"
)

// effectivenessTracker implements EffectivenessTracker
type effectivenessTracker struct {
	executionStore ExecutionStore
	payloadStore   PayloadStore
}

// NewEffectivenessTracker creates a new effectiveness tracker
func NewEffectivenessTracker(executionStore ExecutionStore, payloadStore PayloadStore) EffectivenessTracker {
	return &effectivenessTracker{
		executionStore: executionStore,
		payloadStore:   payloadStore,
	}
}

// RecordExecution records an execution for analytics tracking
func (et *effectivenessTracker) RecordExecution(ctx context.Context, execution *Execution) error {
	// The execution is already stored by the executor
	// This method is for any additional analytics processing
	// For now, we just verify it's stored
	if et.executionStore == nil {
		return fmt.Errorf("execution store not initialized")
	}

	// Could add additional processing here like:
	// - Real-time trend calculation
	// - Anomaly detection
	// - Alert triggering
	return nil
}

// GetPayloadStats returns statistics for a specific payload
func (et *effectivenessTracker) GetPayloadStats(ctx context.Context, payloadID types.ID) (*PayloadStats, error) {
	// Get payload info
	payload, err := et.payloadStore.Get(ctx, payloadID)
	if err != nil {
		return nil, fmt.Errorf("failed to get payload: %w", err)
	}

	// Get all executions for this payload (no limit)
	executions, err := et.executionStore.GetByPayload(ctx, payloadID, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get executions: %w", err)
	}

	if len(executions) == 0 {
		// Return empty stats
		return &PayloadStats{
			PayloadID:           payloadID,
			PayloadName:         payload.Name,
			TargetTypeBreakdown: make(map[types.TargetType]*TargetTypeBreakdown),
		}, nil
	}

	stats := &PayloadStats{
		PayloadID:           payloadID,
		PayloadName:         payload.Name,
		TargetTypeBreakdown: make(map[types.TargetType]*TargetTypeBreakdown),
	}

	// Calculate basic counts and metrics
	var totalDuration time.Duration
	var totalTokens int
	var totalCost float64
	var totalConfidence float64
	var successCount int
	durations := []time.Duration{}
	targetTypeMap := make(map[types.TargetType][]*Execution)

	recentCutoff := time.Now().Add(-30 * 24 * time.Hour) // 30 days ago
	recentSuccesses := 0
	recentTotal := 0

	for _, exec := range executions {
		stats.TotalExecutions++

		// Update temporal data
		if stats.FirstExecution.IsZero() || exec.CreatedAt.Before(stats.FirstExecution) {
			stats.FirstExecution = exec.CreatedAt
		}
		if exec.CreatedAt.After(stats.LastExecution) {
			stats.LastExecution = exec.CreatedAt
		}

		// Count by status
		switch exec.Status {
		case ExecutionStatusCompleted:
			if exec.Success {
				stats.SuccessfulAttacks++
				successCount++
				if stats.LastSuccess == nil || exec.CreatedAt.After(*stats.LastSuccess) {
					stats.LastSuccess = &exec.CreatedAt
				}
				totalConfidence += exec.ConfidenceScore

				// Recent trend tracking
				if exec.CreatedAt.After(recentCutoff) {
					recentSuccesses++
				}
			} else {
				stats.FailedExecutions++
			}
		case ExecutionStatusFailed:
			stats.FailedExecutions++
		case ExecutionStatusTimeout:
			stats.TimeoutCount++
			stats.FailedExecutions++
		case ExecutionStatusCancelled:
			stats.FailedExecutions++
		}

		// Aggregate metrics
		if exec.CompletedAt != nil && exec.StartedAt != nil {
			duration := exec.CompletedAt.Sub(*exec.StartedAt)
			totalDuration += duration
			durations = append(durations, duration)
		}

		totalTokens += exec.TokensUsed
		totalCost += exec.Cost

		if exec.FindingCreated {
			stats.FindingsCreated++
		}

		// Track by target type
		targetTypeMap[exec.TargetType] = append(targetTypeMap[exec.TargetType], exec)

		// Recent tracking
		if exec.CreatedAt.After(recentCutoff) {
			recentTotal++
		}
	}

	// Calculate rates
	if stats.TotalExecutions > 0 {
		stats.SuccessRate = float64(stats.SuccessfulAttacks) / float64(stats.TotalExecutions)
		stats.AverageDuration = totalDuration / time.Duration(stats.TotalExecutions)
		stats.AverageTokensUsed = float64(totalTokens) / float64(stats.TotalExecutions)
		stats.AverageCost = totalCost / float64(stats.TotalExecutions)
	}

	if successCount > 0 {
		stats.AverageConfidence = totalConfidence / float64(successCount)
		stats.FindingCreationRate = float64(stats.FindingsCreated) / float64(successCount)
	}

	stats.TotalCost = totalCost

	// Calculate median duration
	if len(durations) > 0 {
		stats.MedianDuration = calculateMedianDuration(durations)
	}

	// Calculate confidence level based on sample size
	stats.ConfidenceLevel = calculateConfidenceLevel(stats.TotalExecutions)

	// Calculate recent success rate and trend
	if recentTotal > 0 {
		stats.RecentSuccessRate = float64(recentSuccesses) / float64(recentTotal)
		stats.Trending = calculateTrend(stats.SuccessRate, stats.RecentSuccessRate)
	} else {
		stats.RecentSuccessRate = stats.SuccessRate
		stats.Trending = "stable"
	}

	// Build target type breakdown
	for targetType, execs := range targetTypeMap {
		breakdown := calculateTargetTypeBreakdown(targetType, execs)
		stats.TargetTypeBreakdown[targetType] = breakdown
	}

	return stats, nil
}

// GetCategoryStats returns aggregated statistics by category
func (et *effectivenessTracker) GetCategoryStats(ctx context.Context, category PayloadCategory) (*CategoryStats, error) {
	// Get all payloads in this category
	filter := &PayloadFilter{
		Categories: []PayloadCategory{category},
	}
	payloads, err := et.payloadStore.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list payloads: %w", err)
	}

	stats := &CategoryStats{
		Category:    category,
		TopPayloads: []*PayloadStats{},
	}

	stats.TotalPayloads = len(payloads)

	// Aggregate statistics across all payloads
	var totalDuration time.Duration
	var totalConfidence float64
	var successCount int
	var executionCount int

	payloadStats := []*PayloadStats{}

	for _, payload := range payloads {
		if payload.Enabled {
			stats.EnabledPayloads++
		}

		// Get stats for this payload
		pStats, err := et.GetPayloadStats(ctx, payload.ID)
		if err != nil {
			continue // Skip on error
		}

		if pStats.TotalExecutions == 0 {
			continue // Skip payloads with no executions
		}

		payloadStats = append(payloadStats, pStats)

		stats.TotalExecutions += pStats.TotalExecutions
		stats.SuccessfulAttacks += pStats.SuccessfulAttacks
		stats.FailedExecutions += pStats.FailedExecutions
		stats.FindingsCreated += pStats.FindingsCreated
		stats.TotalCost += pStats.TotalCost

		totalDuration += pStats.AverageDuration * time.Duration(pStats.TotalExecutions)
		totalConfidence += pStats.AverageConfidence * float64(pStats.SuccessfulAttacks)
		successCount += pStats.SuccessfulAttacks
		executionCount += pStats.TotalExecutions
	}

	// Calculate aggregate rates
	if stats.TotalExecutions > 0 {
		stats.SuccessRate = float64(stats.SuccessfulAttacks) / float64(stats.TotalExecutions)
		stats.AverageDuration = totalDuration / time.Duration(executionCount)
	}

	if successCount > 0 {
		stats.AverageConfidence = totalConfidence / float64(successCount)
		stats.FindingRate = float64(stats.FindingsCreated) / float64(successCount)
	}

	// Sort payloads by success rate and take top 5
	stats.TopPayloads = getTopPayloads(payloadStats, 5)

	return stats, nil
}

// GetTargetTypeStats returns statistics by target type
func (et *effectivenessTracker) GetTargetTypeStats(ctx context.Context, targetType types.TargetType) (*TargetTypeStats, error) {
	stats := &TargetTypeStats{
		TargetType:        targetType,
		CategoryBreakdown: make(map[PayloadCategory]*CategoryBreakdown),
		MostEffective:     []*PayloadStats{},
	}

	// Get all payloads
	payloads, err := et.payloadStore.List(ctx, &PayloadFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list payloads: %w", err)
	}

	var totalDuration time.Duration
	var totalConfidence float64
	var successCount int
	var executionCount int

	categoryMap := make(map[PayloadCategory]struct {
		executions int
		successes  int
	})

	payloadStats := []*PayloadStats{}

	for _, payload := range payloads {
		// Get stats for this payload
		pStats, err := et.GetPayloadStats(ctx, payload.ID)
		if err != nil {
			continue
		}

		// Check if this payload has executions against this target type
		breakdown, exists := pStats.TargetTypeBreakdown[targetType]
		if !exists || breakdown.Executions == 0 {
			continue
		}

		payloadStats = append(payloadStats, pStats)

		stats.TotalExecutions += breakdown.Executions
		stats.SuccessfulAttacks += breakdown.Successes
		stats.FailedExecutions += breakdown.Executions - breakdown.Successes

		totalDuration += breakdown.AverageDuration * time.Duration(breakdown.Executions)
		totalConfidence += breakdown.AverageConfidence * float64(breakdown.Successes)
		successCount += breakdown.Successes
		executionCount += breakdown.Executions

		// Track category breakdown
		for _, category := range payload.Categories {
			cb := categoryMap[category]
			cb.executions += breakdown.Executions
			cb.successes += breakdown.Successes
			categoryMap[category] = cb
		}
	}

	// Calculate aggregate rates
	if stats.TotalExecutions > 0 {
		stats.SuccessRate = float64(stats.SuccessfulAttacks) / float64(stats.TotalExecutions)
		stats.AverageDuration = totalDuration / time.Duration(executionCount)
	}

	if successCount > 0 {
		stats.AverageConfidence = totalConfidence / float64(successCount)
	}

	// Build category breakdown
	for category, data := range categoryMap {
		breakdown := &CategoryBreakdown{
			Category:   category,
			Executions: data.executions,
			Successes:  data.successes,
		}
		if data.executions > 0 {
			breakdown.SuccessRate = float64(data.successes) / float64(data.executions)
		}
		stats.CategoryBreakdown[category] = breakdown
	}

	// Get most effective payloads
	stats.MostEffective = getTopPayloads(payloadStats, 10)

	return stats, nil
}

// GetRecommendations returns recommended payloads based on effectiveness
func (et *effectivenessTracker) GetRecommendations(ctx context.Context, filter RecommendationFilter) ([]*PayloadRecommendation, error) {
	// Get all payloads matching the filter
	payloadFilter := &PayloadFilter{}
	if filter.Category != nil {
		payloadFilter.Categories = []PayloadCategory{*filter.Category}
	}
	if filter.Severity != nil {
		payloadFilter.Severities = []agent.FindingSeverity{*filter.Severity}
	}
	if filter.TargetType != nil {
		payloadFilter.TargetTypes = []string{string(*filter.TargetType)}
	}

	payloads, err := et.payloadStore.List(ctx, payloadFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to list payloads: %w", err)
	}

	recommendations := []*PayloadRecommendation{}

	for _, payload := range payloads {
		if !payload.Enabled {
			continue
		}

		// Get stats
		stats, err := et.GetPayloadStats(ctx, payload.ID)
		if err != nil {
			continue
		}

		// Apply filters
		if stats.TotalExecutions < filter.MinExecutions {
			continue
		}

		if stats.SuccessRate < filter.MinSuccessRate {
			continue
		}

		if filter.MaxDuration != nil && stats.AverageDuration > *filter.MaxDuration {
			continue
		}

		if filter.MaxCost != nil && stats.AverageCost > *filter.MaxCost {
			continue
		}

		// Calculate recommendation score
		score := calculateRecommendationScore(stats, filter)
		reason := generateRecommendationReason(stats, filter)

		recommendations = append(recommendations, &PayloadRecommendation{
			Payload:    payload,
			Stats:      stats,
			Score:      score,
			Reason:     reason,
			Confidence: stats.ConfidenceLevel,
		})
	}

	// Sort by score
	sortRecommendationsByScore(recommendations)

	// Apply limit
	if filter.Limit > 0 && len(recommendations) > filter.Limit {
		recommendations = recommendations[:filter.Limit]
	}

	return recommendations, nil
}

// ExportStats exports statistics in various formats
func (et *effectivenessTracker) ExportStats(ctx context.Context, format ExportFormat, writer io.Writer) error {
	// Get all payloads
	payloads, err := et.payloadStore.List(ctx, &PayloadFilter{})
	if err != nil {
		return fmt.Errorf("failed to list payloads: %w", err)
	}

	// Collect stats for all payloads
	allStats := []*PayloadStats{}
	for _, payload := range payloads {
		stats, err := et.GetPayloadStats(ctx, payload.ID)
		if err != nil {
			continue
		}
		if stats.TotalExecutions > 0 {
			allStats = append(allStats, stats)
		}
	}

	// Export in requested format
	switch format {
	case ExportFormatJSON:
		return exportJSON(allStats, writer)
	case ExportFormatCSV:
		return exportCSV(allStats, writer)
	default:
		return fmt.Errorf("unsupported export format: %s", format)
	}
}

// Helper functions

func calculateMedianDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	// Simple bubble sort for small arrays
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)

	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

func calculateConfidenceLevel(sampleSize int) float64 {
	// Calculate statistical confidence based on sample size
	// Using a simplified formula: confidence increases with sample size
	// Asymptotically approaches 1.0

	if sampleSize == 0 {
		return 0.0
	}

	// Using formula: 1 - (1 / sqrt(n))
	// This gives reasonable confidence scaling:
	// n=1: 0.0, n=4: 0.5, n=9: 0.67, n=25: 0.8, n=100: 0.9
	confidence := 1.0 - (1.0 / math.Sqrt(float64(sampleSize)))
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	return confidence
}

func calculateTrend(overallRate, recentRate float64) string {
	diff := recentRate - overallRate

	// Consider significant if difference is more than 5%
	if diff > 0.05 {
		return "up"
	} else if diff < -0.05 {
		return "down"
	}
	return "stable"
}

func calculateTargetTypeBreakdown(targetType types.TargetType, executions []*Execution) *TargetTypeBreakdown {
	breakdown := &TargetTypeBreakdown{
		TargetType: targetType,
	}

	var totalDuration time.Duration
	var totalConfidence float64
	var successCount int

	for _, exec := range executions {
		breakdown.Executions++

		if exec.Success {
			breakdown.Successes++
			successCount++
			totalConfidence += exec.ConfidenceScore
		}

		if exec.CompletedAt != nil && exec.StartedAt != nil {
			totalDuration += exec.CompletedAt.Sub(*exec.StartedAt)
		}
	}

	if breakdown.Executions > 0 {
		breakdown.SuccessRate = float64(breakdown.Successes) / float64(breakdown.Executions)
		breakdown.AverageDuration = totalDuration / time.Duration(breakdown.Executions)
	}

	if successCount > 0 {
		breakdown.AverageConfidence = totalConfidence / float64(successCount)
	}

	return breakdown
}

func getTopPayloads(payloads []*PayloadStats, limit int) []*PayloadStats {
	// Sort by success rate (considering confidence)
	sorted := make([]*PayloadStats, len(payloads))
	copy(sorted, payloads)

	// Bubble sort by weighted score
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			scoreI := sorted[i].SuccessRate * sorted[i].ConfidenceLevel
			scoreJ := sorted[j].SuccessRate * sorted[j].ConfidenceLevel

			if scoreI < scoreJ {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	if len(sorted) > limit {
		sorted = sorted[:limit]
	}

	return sorted
}

func calculateRecommendationScore(stats *PayloadStats, filter RecommendationFilter) float64 {
	// Calculate a weighted score based on multiple factors
	score := 0.0

	// Success rate (40% weight)
	score += stats.SuccessRate * 0.4

	// Confidence level (30% weight)
	score += stats.ConfidenceLevel * 0.3

	// Finding creation rate (20% weight)
	score += stats.FindingCreationRate * 0.2

	// Recent trend bonus (10% weight)
	trendBonus := 0.0
	if stats.Trending == "up" {
		trendBonus = 0.1
	} else if stats.Trending == "down" {
		trendBonus = -0.1
	}
	score += trendBonus * 0.1

	// Normalize to 0-1 range
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score
}

func generateRecommendationReason(stats *PayloadStats, filter RecommendationFilter) string {
	reasons := []string{}

	if stats.SuccessRate >= 0.8 {
		reasons = append(reasons, fmt.Sprintf("High success rate (%.1f%%)", stats.SuccessRate*100))
	}

	if stats.ConfidenceLevel >= 0.8 {
		reasons = append(reasons, fmt.Sprintf("High confidence (%.1f%%, %d executions)", stats.ConfidenceLevel*100, stats.TotalExecutions))
	}

	if stats.FindingCreationRate >= 0.8 {
		reasons = append(reasons, "Reliably creates findings")
	}

	if stats.Trending == "up" {
		reasons = append(reasons, "Trending up recently")
	}

	if len(reasons) == 0 {
		return "Meets specified criteria"
	}

	result := reasons[0]
	for i := 1; i < len(reasons); i++ {
		result += "; " + reasons[i]
	}

	return result
}

func sortRecommendationsByScore(recommendations []*PayloadRecommendation) {
	// Bubble sort by score (descending)
	for i := 0; i < len(recommendations); i++ {
		for j := i + 1; j < len(recommendations); j++ {
			if recommendations[i].Score < recommendations[j].Score {
				recommendations[i], recommendations[j] = recommendations[j], recommendations[i]
			}
		}
	}
}

func exportJSON(stats []*PayloadStats, writer io.Writer) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(stats)
}

func exportCSV(stats []*PayloadStats, writer io.Writer) error {
	csvWriter := csv.NewWriter(writer)
	defer csvWriter.Flush()

	// Write header
	header := []string{
		"Payload ID",
		"Payload Name",
		"Total Executions",
		"Successful Attacks",
		"Failed Executions",
		"Success Rate",
		"Confidence Level",
		"Average Confidence",
		"Average Duration (ms)",
		"Average Tokens",
		"Average Cost",
		"Total Cost",
		"Findings Created",
		"Finding Rate",
		"Trending",
	}
	if err := csvWriter.Write(header); err != nil {
		return err
	}

	// Write data rows
	for _, stat := range stats {
		row := []string{
			string(stat.PayloadID),
			stat.PayloadName,
			fmt.Sprintf("%d", stat.TotalExecutions),
			fmt.Sprintf("%d", stat.SuccessfulAttacks),
			fmt.Sprintf("%d", stat.FailedExecutions),
			fmt.Sprintf("%.4f", stat.SuccessRate),
			fmt.Sprintf("%.4f", stat.ConfidenceLevel),
			fmt.Sprintf("%.4f", stat.AverageConfidence),
			fmt.Sprintf("%d", stat.AverageDuration.Milliseconds()),
			fmt.Sprintf("%.2f", stat.AverageTokensUsed),
			fmt.Sprintf("%.6f", stat.AverageCost),
			fmt.Sprintf("%.6f", stat.TotalCost),
			fmt.Sprintf("%d", stat.FindingsCreated),
			fmt.Sprintf("%.4f", stat.FindingCreationRate),
			stat.Trending,
		}
		if err := csvWriter.Write(row); err != nil {
			return err
		}
	}

	return nil
}
