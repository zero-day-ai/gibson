// Package migrations provides database and GraphRAG migration utilities.
//
// This file contains the migration for adding run metadata to existing GraphRAG nodes.
// The migration is idempotent and can be run multiple times safely.
package migrations

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/types"
)

// WritableGraphClient extends GraphClient with write operations for migrations.
// The Neo4jClient implements this interface.
type WritableGraphClient interface {
	graph.GraphClient

	// ExecuteWrite executes a Cypher write query (MERGE, CREATE, SET, DELETE, etc.)
	ExecuteWrite(ctx context.Context, cypher string, params map[string]any) (graph.QueryResult, error)
}

// MigrationStatus represents the status of a migration.
type MigrationStatus struct {
	Name       string    `json:"name"`
	Applied    bool      `json:"applied"`
	AppliedAt  time.Time `json:"applied_at,omitempty"`
	NodesFound int       `json:"nodes_found"`
	NodesFixed int       `json:"nodes_fixed"`
	Duration   string    `json:"duration"`
	Error      string    `json:"error,omitempty"`
}

// MissionLister provides access to missions for backfilling mission names.
type MissionLister interface {
	GetByID(ctx context.Context, id types.ID) (Mission, error)
}

// Mission is a minimal interface for mission data needed by the migration.
type Mission struct {
	ID   types.ID
	Name string
}

// RunMetadataMigration migrates existing GraphRAG nodes to include run metadata.
// This migration:
// - Backfills mission_name from mission store lookup
// - Sets run_number = 1 for existing nodes without run_number
// - Adds migrated_at timestamp to track when nodes were migrated
//
// The migration is idempotent - running it multiple times is safe as it only
// modifies nodes that don't already have the run metadata.
type RunMetadataMigration struct {
	graphClient   WritableGraphClient
	missionLister MissionLister
	logger        *slog.Logger
}

// NewRunMetadataMigration creates a new run metadata migration.
func NewRunMetadataMigration(
	graphClient WritableGraphClient,
	missionLister MissionLister,
	logger *slog.Logger,
) *RunMetadataMigration {
	if logger == nil {
		logger = slog.Default()
	}
	return &RunMetadataMigration{
		graphClient:   graphClient,
		missionLister: missionLister,
		logger:        logger,
	}
}

// Name returns the migration name.
func (m *RunMetadataMigration) Name() string {
	return "012_add_run_metadata"
}

// Run executes the migration.
// Returns a MigrationStatus with details about what was migrated.
func (m *RunMetadataMigration) Run(ctx context.Context) (*MigrationStatus, error) {
	startTime := time.Now()
	status := &MigrationStatus{
		Name: m.Name(),
	}

	m.logger.Info("starting run metadata migration",
		"migration", m.Name())

	// Step 1: Find all nodes that don't have run_number set
	nodesWithoutRunNumber, err := m.findNodesWithoutRunMetadata(ctx)
	if err != nil {
		status.Error = fmt.Sprintf("failed to find nodes: %v", err)
		status.Duration = time.Since(startTime).String()
		return status, err
	}

	status.NodesFound = nodesWithoutRunNumber
	m.logger.Info("found nodes without run metadata",
		"count", nodesWithoutRunNumber)

	if nodesWithoutRunNumber == 0 {
		status.Applied = true
		status.AppliedAt = time.Now()
		status.Duration = time.Since(startTime).String()
		m.logger.Info("no nodes to migrate, migration complete")
		return status, nil
	}

	// Step 2: Backfill run metadata on all nodes
	nodesFixed, err := m.backfillRunMetadata(ctx)
	if err != nil {
		status.Error = fmt.Sprintf("failed to backfill run metadata: %v", err)
		status.Duration = time.Since(startTime).String()
		return status, err
	}

	status.NodesFixed = nodesFixed
	status.Applied = true
	status.AppliedAt = time.Now()
	status.Duration = time.Since(startTime).String()

	m.logger.Info("migration complete",
		"nodes_found", nodesWithoutRunNumber,
		"nodes_fixed", nodesFixed,
		"duration", status.Duration)

	return status, nil
}

// findNodesWithoutRunMetadata counts nodes that are missing run_number property.
func (m *RunMetadataMigration) findNodesWithoutRunMetadata(ctx context.Context) (int, error) {
	// Query to count nodes without run_number property
	cypher := `
		MATCH (n)
		WHERE n.run_number IS NULL
		RETURN count(n) as count
	`

	result, err := m.graphClient.Query(ctx, cypher, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to query nodes without run metadata: %w", err)
	}

	if len(result.Records) == 0 {
		return 0, nil
	}

	countVal, ok := result.Records[0]["count"]
	if !ok {
		return 0, nil
	}

	// Handle different numeric types that Neo4j might return
	switch v := countVal.(type) {
	case int64:
		return int(v), nil
	case int:
		return v, nil
	case float64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("unexpected count type: %T", countVal)
	}
}

// backfillRunMetadata sets run_number = 1 and migrated_at for nodes without run metadata.
// For nodes with a mission_id, it also attempts to backfill mission_name from the mission store.
func (m *RunMetadataMigration) backfillRunMetadata(ctx context.Context) (int, error) {
	// Step 1: Set run_number = 1 and migrated_at for all nodes without run_number
	// This is the primary migration that ensures all nodes have run metadata
	cypher := `
		MATCH (n)
		WHERE n.run_number IS NULL
		SET n.run_number = 1,
		    n.created_in_run = 1,
		    n.migrated_at = datetime()
		RETURN count(n) as updated
	`

	result, err := m.graphClient.ExecuteWrite(ctx, cypher, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to backfill run_number: %w", err)
	}

	nodesUpdated := 0
	if len(result.Records) > 0 {
		if updatedVal, ok := result.Records[0]["updated"]; ok {
			switch v := updatedVal.(type) {
			case int64:
				nodesUpdated = int(v)
			case int:
				nodesUpdated = v
			case float64:
				nodesUpdated = int(v)
			}
		}
	}

	m.logger.Info("set run_number on nodes",
		"nodes_updated", nodesUpdated)

	// Step 2: Backfill mission_name for nodes that have mission_id but not mission_name
	// This requires looking up missions from the mission store
	if m.missionLister != nil {
		missionNamesBackfilled, err := m.backfillMissionNames(ctx)
		if err != nil {
			// Log warning but don't fail the migration
			m.logger.Warn("failed to backfill mission names, continuing",
				"error", err)
		} else {
			m.logger.Info("backfilled mission names",
				"nodes_updated", missionNamesBackfilled)
		}
	}

	return nodesUpdated, nil
}

// backfillMissionNames looks up mission names for nodes with mission_id but no mission_name.
func (m *RunMetadataMigration) backfillMissionNames(ctx context.Context) (int, error) {
	// Get distinct mission IDs that need name backfill
	cypher := `
		MATCH (n)
		WHERE n.mission_id IS NOT NULL AND n.mission_name IS NULL
		RETURN DISTINCT n.mission_id as mission_id
		LIMIT 1000
	`

	result, err := m.graphClient.Query(ctx, cypher, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to query distinct mission IDs: %w", err)
	}

	if len(result.Records) == 0 {
		return 0, nil
	}

	totalUpdated := 0

	// For each mission ID, look up the name and update nodes
	for _, record := range result.Records {
		missionIDStr, ok := record["mission_id"].(string)
		if !ok {
			continue
		}

		missionID, err := types.ParseID(missionIDStr)
		if err != nil {
			m.logger.Warn("invalid mission ID in node",
				"mission_id", missionIDStr,
				"error", err)
			continue
		}

		// Look up mission name
		mission, err := m.missionLister.GetByID(ctx, missionID)
		if err != nil {
			// Mission might have been deleted, skip
			m.logger.Debug("mission not found, skipping name backfill",
				"mission_id", missionIDStr)
			continue
		}

		if mission.Name == "" {
			continue
		}

		// Update all nodes with this mission_id
		updateCypher := `
			MATCH (n)
			WHERE n.mission_id = $mission_id AND n.mission_name IS NULL
			SET n.mission_name = $mission_name
			RETURN count(n) as updated
		`

		updateResult, err := m.graphClient.ExecuteWrite(ctx, updateCypher, map[string]any{
			"mission_id":   missionIDStr,
			"mission_name": mission.Name,
		})
		if err != nil {
			m.logger.Warn("failed to update mission name for nodes",
				"mission_id", missionIDStr,
				"error", err)
			continue
		}

		if len(updateResult.Records) > 0 {
			if updatedVal, ok := updateResult.Records[0]["updated"]; ok {
				switch v := updatedVal.(type) {
				case int64:
					totalUpdated += int(v)
				case int:
					totalUpdated += v
				case float64:
					totalUpdated += int(v)
				}
			}
		}
	}

	return totalUpdated, nil
}

// Verify checks if the migration has been applied by checking for nodes without run metadata.
func (m *RunMetadataMigration) Verify(ctx context.Context) (bool, error) {
	count, err := m.findNodesWithoutRunMetadata(ctx)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

// Rollback removes run metadata added by this migration.
// Only affects nodes that have the migrated_at property set.
func (m *RunMetadataMigration) Rollback(ctx context.Context) error {
	m.logger.Info("rolling back run metadata migration",
		"migration", m.Name())

	cypher := `
		MATCH (n)
		WHERE n.migrated_at IS NOT NULL
		REMOVE n.run_number, n.created_in_run, n.mission_name, n.migrated_at
		RETURN count(n) as reverted
	`

	result, err := m.graphClient.ExecuteWrite(ctx, cypher, nil)
	if err != nil {
		return fmt.Errorf("failed to rollback migration: %w", err)
	}

	if len(result.Records) > 0 {
		if revertedVal, ok := result.Records[0]["reverted"]; ok {
			m.logger.Info("rollback complete",
				"nodes_reverted", revertedVal)
		}
	}

	return nil
}
