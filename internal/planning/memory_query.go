package planning

import (
	"context"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/types"
)

// MemoryQueryResult contains the results of a memory query for planning purposes.
// It aggregates similar missions, successful techniques, failed approaches, and relevant findings
// to inform the strategic planner about what has worked in the past.
type MemoryQueryResult struct {
	// QueryID uniquely identifies this query for tracing and debugging
	QueryID string `json:"query_id"`

	// TargetProfile is a summary of the target characteristics used in the query
	TargetProfile string `json:"target_profile"`

	// SimilarMissions contains summaries of past missions against similar targets
	SimilarMissions []MissionSummary `json:"similar_missions"`

	// SuccessfulTechniques lists techniques that have historically succeeded on similar targets
	SuccessfulTechniques []TechniqueRecord `json:"successful_techniques"`

	// FailedApproaches lists approaches that consistently failed on similar targets
	FailedApproaches []ApproachRecord `json:"failed_approaches"`

	// RelevantFindings contains summaries of past findings that may inform current strategy
	RelevantFindings []FindingSummary `json:"relevant_findings"`

	// QueryLatency tracks how long the memory query took to execute
	QueryLatency time.Duration `json:"query_latency"`

	// ResultCount is the total number of results returned across all categories
	ResultCount int `json:"result_count"`
}

// TechniqueRecord captures historical data about a security testing technique.
type TechniqueRecord struct {
	// Technique is the name or identifier of the technique (e.g., "prompt-injection", "sql-injection")
	Technique string `json:"technique"`

	// SuccessRate is the historical success rate (0.0-1.0) for this technique on similar targets
	SuccessRate float64 `json:"success_rate"`

	// AvgTokenCost is the average number of tokens required to execute this technique
	AvgTokenCost int `json:"avg_token_cost"`

	// LastUsed is when this technique was last successfully used
	LastUsed time.Time `json:"last_used"`
}

// ApproachRecord captures information about an approach that failed.
type ApproachRecord struct {
	// Approach is a description of the failed approach
	Approach string `json:"approach"`

	// FailureReason explains why this approach failed
	FailureReason string `json:"failure_reason"`

	// TargetType is the type of target this approach failed against
	TargetType string `json:"target_type"`

	// LastAttempted is when this approach was last tried
	LastAttempted time.Time `json:"last_attempted"`
}

// MissionSummary provides a high-level summary of a past mission.
type MissionSummary struct {
	// ID is the unique identifier of the mission
	ID types.ID `json:"id"`

	// TargetType is the type of target this mission tested (e.g., "llm", "api", "web-app")
	TargetType string `json:"target_type"`

	// Techniques lists the techniques that were employed in this mission
	Techniques []string `json:"techniques"`

	// Success indicates whether the mission was successful overall
	Success bool `json:"success"`

	// FindingsCount is the number of findings discovered in this mission
	FindingsCount int `json:"findings_count"`
}

// FindingSummary provides a compact summary of a finding.
type FindingSummary struct {
	// ID is the unique identifier of the finding
	ID types.ID `json:"id"`

	// Title is the finding's title or summary
	Title string `json:"title"`

	// Category is the finding category (e.g., "injection", "authentication")
	Category string `json:"category"`

	// Severity is the severity level (e.g., "high", "medium", "low")
	Severity string `json:"severity"`

	// Technique is the technique that discovered this finding
	Technique string `json:"technique"`
}

// QueryMemoryForPlanning queries the harness's long-term memory to gather historical data
// that can inform planning decisions. This function is designed to be fast and resilient,
// timing out within 500ms and gracefully handling memory unavailability.
//
// The function queries for:
//   - Similar past missions against the same target type
//   - Techniques that historically succeeded on similar targets
//   - Approaches that consistently failed
//   - Relevant past findings that may inform strategy
//
// Parameters:
//   - ctx: Context for cancellation and timeout (will be wrapped with 500ms timeout)
//   - harness: The agent harness providing access to memory
//   - target: Information about the target being tested
//
// Returns:
//   - *MemoryQueryResult: Query results (may be empty if memory unavailable)
//   - error: Only returns error for invalid parameters; memory unavailability returns empty result
func QueryMemoryForPlanning(ctx context.Context, h harness.AgentHarness, target harness.TargetInfo) (*MemoryQueryResult, error) {
	// Validate inputs
	if h == nil {
		return nil, NewInvalidParameterError("harness cannot be nil")
	}

	startTime := time.Now()

	// Generate unique query ID for tracing
	queryID := fmt.Sprintf("memquery-%d", time.Now().UnixNano())

	// Create target profile for metadata
	targetProfile := fmt.Sprintf("type=%s,name=%s", target.Type, target.Name)
	if target.Provider != "" {
		targetProfile += fmt.Sprintf(",provider=%s", target.Provider)
	}

	// Create timeout context (500ms max)
	queryCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	result := &MemoryQueryResult{
		QueryID:              queryID,
		TargetProfile:        targetProfile,
		SimilarMissions:      []MissionSummary{},
		SuccessfulTechniques: []TechniqueRecord{},
		FailedApproaches:     []ApproachRecord{},
		RelevantFindings:     []FindingSummary{},
		ResultCount:          0,
	}

	// Access long-term memory through the harness
	memory := h.Memory()
	if memory == nil {
		// Memory unavailable - return empty result gracefully
		result.QueryLatency = time.Since(startTime)
		return result, nil
	}

	longTerm := memory.LongTerm()
	if longTerm == nil {
		// Long-term memory unavailable - return empty result gracefully
		result.QueryLatency = time.Since(startTime)
		return result, nil
	}

	// Check memory health before querying
	healthStatus := longTerm.Health(queryCtx)
	if healthStatus.IsUnhealthy() {
		// Memory is unhealthy - return empty result gracefully
		result.QueryLatency = time.Since(startTime)
		return result, nil
	}

	// Query for similar missions
	// We search for missions with the same target type
	missionQuery := fmt.Sprintf("mission target_type:%s", target.Type)
	missionResults, err := longTerm.Search(queryCtx, missionQuery, 5, map[string]any{
		"type": "mission_summary",
	})
	if err == nil {
		// Convert memory results to mission summaries
		for _, mr := range missionResults {
			ms := parseMissionSummary(mr)
			if ms != nil {
				result.SimilarMissions = append(result.SimilarMissions, *ms)
				result.ResultCount++
			}
		}
	}
	// Ignore errors - we continue with other queries

	// Query for successful techniques
	techniqueQuery := fmt.Sprintf("successful technique target_type:%s", target.Type)
	techniqueResults, err := longTerm.Search(queryCtx, techniqueQuery, 10, map[string]any{
		"type": "technique_record",
	})
	if err == nil {
		for _, mr := range techniqueResults {
			tr := parseTechniqueRecord(mr)
			if tr != nil {
				result.SuccessfulTechniques = append(result.SuccessfulTechniques, *tr)
				result.ResultCount++
			}
		}
	}

	// Query for failed approaches
	failureQuery := fmt.Sprintf("failed approach target_type:%s", target.Type)
	failureResults, err := longTerm.Search(queryCtx, failureQuery, 10, map[string]any{
		"type": "failure_record",
	})
	if err == nil {
		for _, mr := range failureResults {
			ar := parseApproachRecord(mr)
			if ar != nil {
				result.FailedApproaches = append(result.FailedApproaches, *ar)
				result.ResultCount++
			}
		}
	}

	// Query for relevant findings using semantic search on target characteristics
	findingQuery := fmt.Sprintf("%s %s findings", target.Type, target.Name)
	findingResults, err := longTerm.SimilarFindings(queryCtx, findingQuery, 5)
	if err == nil {
		for _, mr := range findingResults {
			fs := parseFindingSummary(mr)
			if fs != nil {
				result.RelevantFindings = append(result.RelevantFindings, *fs)
				result.ResultCount++
			}
		}
	}

	// Record query latency
	result.QueryLatency = time.Since(startTime)

	return result, nil
}

// parseMissionSummary extracts a MissionSummary from a memory result
func parseMissionSummary(mr interface{}) *MissionSummary {
	// This is a placeholder implementation - in production, this would parse
	// structured metadata from the memory result
	// For now, return nil to indicate we can't parse it yet
	return nil
}

// parseTechniqueRecord extracts a TechniqueRecord from a memory result
func parseTechniqueRecord(mr interface{}) *TechniqueRecord {
	// This is a placeholder implementation - in production, this would parse
	// structured metadata from the memory result
	// For now, return nil to indicate we can't parse it yet
	return nil
}

// parseApproachRecord extracts an ApproachRecord from a memory result
func parseApproachRecord(mr interface{}) *ApproachRecord {
	// This is a placeholder implementation - in production, this would parse
	// structured metadata from the memory result
	// For now, return nil to indicate we can't parse it yet
	return nil
}

// parseFindingSummary extracts a FindingSummary from a memory result
func parseFindingSummary(mr interface{}) *FindingSummary {
	// This is a placeholder implementation - in production, this would parse
	// structured metadata from the memory result
	// For now, return nil to indicate we can't parse it yet
	return nil
}
