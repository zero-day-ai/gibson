package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/daemon/api"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/types"
)

// ListMissions returns mission list with optional filtering and pagination.
//
// This method queries the mission store to retrieve missions based on the provided
// filters. When activeOnly is true, it returns only missions with status "running"
// or "paused". Pagination is supported via limit and offset parameters.
//
// Parameters:
//   - ctx: Context for the database query
//   - activeOnly: If true, return only running/paused missions
//   - limit: Maximum number of missions to return (0 = use default)
//   - offset: Number of missions to skip for pagination
//
// Returns:
//   - []api.MissionData: List of missions matching the filter
//   - int: Total count of missions (for pagination, not affected by limit/offset)
//   - error: Non-nil if query fails
func (d *daemonImpl) ListMissions(ctx context.Context, activeOnly bool, limit, offset int) ([]api.MissionData, int, error) {
	d.logger.Debug("ListMissions called", "active_only", activeOnly, "limit", limit, "offset", offset)

	// Set default limit if not specified
	if limit == 0 {
		limit = 100
	}

	// Build mission filter
	var missions []*mission.Mission
	var total int
	var err error

	if activeOnly {
		// Query only running or paused missions
		missions, err = d.missionStore.GetActive(ctx)
		if err != nil {
			d.logger.Error("failed to get active missions", "error", err)
			return nil, 0, fmt.Errorf("failed to get active missions: %w", err)
		}

		// Total is the number of active missions
		total = len(missions)

		// Apply pagination manually for active missions
		start := offset
		if start > total {
			start = total
		}
		end := start + limit
		if end > total {
			end = total
		}

		// Slice the results for pagination
		missions = missions[start:end]
	} else {
		// Query all missions with pagination
		filter := mission.NewMissionFilter()
		filter.WithPagination(limit, offset)

		missions, err = d.missionStore.List(ctx, filter)
		if err != nil {
			d.logger.Error("failed to list missions", "error", err)
			return nil, 0, fmt.Errorf("failed to list missions: %w", err)
		}

		// Get total count for pagination (count all missions, not just the page)
		totalFilter := mission.NewMissionFilter()
		total, err = d.missionStore.Count(ctx, totalFilter)
		if err != nil {
			d.logger.Error("failed to count missions", "error", err)
			return nil, 0, fmt.Errorf("failed to count missions: %w", err)
		}
	}

	// Convert to API format
	result := make([]api.MissionData, len(missions))
	for i, m := range missions {
		result[i] = convertMissionToData(m)
	}

	d.logger.Debug("listed missions", "count", len(result), "total", total, "active_only", activeOnly)
	return result, total, nil
}

// convertMissionToData converts a mission.Mission to api.MissionData.
//
// This helper function transforms the internal mission representation to the
// API data structure used in gRPC responses. It handles time pointer conversions
// and extracts relevant fields for the API response.
func convertMissionToData(m *mission.Mission) api.MissionData {
	var startTime, endTime time.Time

	// Convert time pointers to values
	if m.StartedAt != nil {
		startTime = *m.StartedAt
	}
	if m.CompletedAt != nil {
		endTime = *m.CompletedAt
	}

	// Determine workflow path - use WorkflowID as a string for now
	// In future, this could be resolved to actual workflow file path
	workflowPath := m.WorkflowID.String()
	if m.WorkflowJSON != "" {
		// If inline workflow, indicate it's inline
		workflowPath = "<inline>"
	}

	return api.MissionData{
		ID:           m.ID.String(),
		WorkflowPath: workflowPath,
		Status:       string(m.Status),
		StartTime:    startTime,
		EndTime:      endTime,
		FindingCount: int32(m.FindingsCount),
	}
}

// parseMissionID is a helper to parse and validate mission IDs.
func parseMissionID(missionIDStr string) (types.ID, error) {
	if missionIDStr == "" {
		return types.ID(""), fmt.Errorf("mission ID cannot be empty")
	}

	missionID, err := types.ParseID(missionIDStr)
	if err != nil {
		return types.ID(""), fmt.Errorf("invalid mission ID format: %w", err)
	}

	return missionID, nil
}
