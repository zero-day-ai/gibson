package graphrag

import (
	"context"
	"log/slog"

	"github.com/zero-day-ai/gibson/internal/types"
)

// Mission represents a minimal mission interface for query scoping.
// This avoids circular dependencies with the mission package.
type Mission struct {
	ID   types.ID
	Name string
}

// MissionLister defines the minimal interface needed for mission-based scoping.
// This interface can be implemented by mission.MissionStore without creating
// a circular dependency.
type MissionLister interface {
	// ListByName retrieves all missions with the given name.
	// Returns missions ordered by run number descending.
	ListByName(ctx context.Context, name string, limit int) ([]*Mission, error)
}

// QueryScoper resolves mission scope to actual mission IDs for query filtering.
// Implements mission-aware filtering for GraphRAG queries to scope results
// to the current run, all runs of the same mission, or all missions.
//
// Thread-safety: Implementations must be safe for concurrent use.
type QueryScoper interface {
	// ResolveScope converts a scope to a list of mission IDs to include in query.
	// Returns nil for ScopeAll (no filtering).
	// Returns single ID for ScopeCurrentRun.
	// Returns all IDs with same name for ScopeSameMission.
	//
	// If an error occurs during resolution (e.g., mission not found),
	// implementations should log a warning and fall back to ScopeAll behavior.
	ResolveScope(ctx context.Context, scope MissionScope, missionName string, currentMissionID types.ID) ([]types.ID, error)
}

// DefaultQueryScoper implements QueryScoper with mission store integration.
// Provides graceful degradation on errors, falling back to unrestricted scope.
type DefaultQueryScoper struct {
	missionLister MissionLister
	logger        *slog.Logger
}

// NewQueryScoper creates a new query scoper with the given mission lister and logger.
// The mission lister can be any implementation that provides mission lookup by name,
// such as mission.MissionStore.
// The logger is used for warning messages when scope resolution fails.
func NewQueryScoper(missionLister MissionLister, logger *slog.Logger) *DefaultQueryScoper {
	return &DefaultQueryScoper{
		missionLister: missionLister,
		logger:        logger,
	}
}

// ResolveScope resolves the mission scope to a list of mission IDs for filtering.
//
// Behavior by scope:
// - ScopeAll or empty: Returns nil (no filtering applied)
// - ScopeCurrentRun: Returns []types.ID{currentMissionID}
// - ScopeSameMission: Queries all missions with the same name and returns their IDs
//
// Error handling:
// - If mission lookup fails, logs a warning and returns nil (falls back to ScopeAll)
// - Never returns an error - always provides graceful degradation
func (s *DefaultQueryScoper) ResolveScope(
	ctx context.Context,
	scope MissionScope,
	missionName string,
	currentMissionID types.ID,
) ([]types.ID, error) {
	// Handle empty or "all" scope - no filtering
	if scope == "" || scope == ScopeAll {
		s.logger.Debug("query scope set to all missions", "scope", scope)
		return nil, nil
	}

	// Handle "current_run" scope - single mission ID
	if scope == ScopeCurrentRun {
		s.logger.Debug("query scope set to current run",
			"scope", scope,
			"mission_id", currentMissionID)
		return []types.ID{currentMissionID}, nil
	}

	// Handle "same_mission" scope - all runs with the same name
	if scope == ScopeSameMission {
		if missionName == "" {
			s.logger.Warn("same_mission scope requested but mission name is empty, falling back to all",
				"scope", scope,
				"mission_id", currentMissionID)
			return nil, nil
		}

		// Query all missions with the same name
		// Use a reasonable limit to prevent unbounded queries
		missions, err := s.missionLister.ListByName(ctx, missionName, 1000)
		if err != nil {
			s.logger.Warn("failed to list missions by name, falling back to all",
				"scope", scope,
				"mission_name", missionName,
				"error", err)
			return nil, nil
		}

		// Extract mission IDs
		if len(missions) == 0 {
			s.logger.Warn("no missions found with name, falling back to all",
				"scope", scope,
				"mission_name", missionName)
			return nil, nil
		}

		missionIDs := make([]types.ID, 0, len(missions))
		for _, m := range missions {
			missionIDs = append(missionIDs, m.ID)
		}

		s.logger.Debug("query scope resolved to same mission runs",
			"scope", scope,
			"mission_name", missionName,
			"mission_count", len(missionIDs))

		return missionIDs, nil
	}

	// Unknown scope - log warning and fall back to all
	s.logger.Warn("unknown mission scope, falling back to all",
		"scope", scope,
		"mission_id", currentMissionID)
	return nil, nil
}
