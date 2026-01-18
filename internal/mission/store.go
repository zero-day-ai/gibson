package mission

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// MissionStore provides persistence for Mission entities.
type MissionStore interface {
	// Mission instance methods (SQLite-backed)

	// Save persists a new mission to the database
	Save(ctx context.Context, mission *Mission) error

	// Get retrieves a mission by ID
	Get(ctx context.Context, id types.ID) (*Mission, error)

	// GetByName retrieves a mission by name
	GetByName(ctx context.Context, name string) (*Mission, error)

	// List retrieves missions with optional filtering
	List(ctx context.Context, filter *MissionFilter) ([]*Mission, error)

	// Update modifies an existing mission
	Update(ctx context.Context, mission *Mission) error

	// UpdateStatus updates only the status field of a mission
	UpdateStatus(ctx context.Context, id types.ID, status MissionStatus) error

	// UpdateProgress updates only the progress field of a mission
	UpdateProgress(ctx context.Context, id types.ID, progress float64) error

	// Delete soft-deletes a mission (only terminal states)
	Delete(ctx context.Context, id types.ID) error

	// GetByTarget retrieves all missions for a specific target
	GetByTarget(ctx context.Context, targetID types.ID) ([]*Mission, error)

	// GetActive retrieves all active missions (running or paused)
	GetActive(ctx context.Context) ([]*Mission, error)

	// SaveCheckpoint persists a mission checkpoint for resume capability
	SaveCheckpoint(ctx context.Context, missionID types.ID, checkpoint *MissionCheckpoint) error

	// Count returns the total number of missions matching the filter
	Count(ctx context.Context, filter *MissionFilter) (int, error)

	// GetByNameAndStatus retrieves a mission by name and status
	GetByNameAndStatus(ctx context.Context, name string, status MissionStatus) (*Mission, error)

	// ListByName retrieves all missions with the given name, ordered by run number descending
	ListByName(ctx context.Context, name string, limit int) ([]*Mission, error)

	// GetLatestByName retrieves the most recent mission with the given name
	GetLatestByName(ctx context.Context, name string) (*Mission, error)

	// IncrementRunNumber atomically increments and returns the next run number for a mission name
	IncrementRunNumber(ctx context.Context, name string) (int, error)

	// Mission definition methods (etcd-backed)

	// CreateDefinition stores a new mission definition in etcd.
	// Returns error if a definition with the same name already exists.
	CreateDefinition(ctx context.Context, def *MissionDefinition) error

	// GetDefinition retrieves a mission definition by name from etcd.
	// Returns nil, nil if not found.
	GetDefinition(ctx context.Context, name string) (*MissionDefinition, error)

	// ListDefinitions returns all installed mission definitions from etcd.
	ListDefinitions(ctx context.Context) ([]*MissionDefinition, error)

	// UpdateDefinition updates an existing mission definition in etcd.
	// Returns error if the definition does not exist.
	UpdateDefinition(ctx context.Context, def *MissionDefinition) error

	// DeleteDefinition removes a mission definition from etcd.
	// Returns error if the definition does not exist.
	DeleteDefinition(ctx context.Context, name string) error
}

// MissionFilter provides filtering options for mission queries.
type MissionFilter struct {
	// Status filters by mission status
	Status *MissionStatus

	// TargetID filters by target
	TargetID *types.ID

	// WorkflowID filters by workflow
	WorkflowID *types.ID

	// CreatedAfter filters missions created after this time
	CreatedAfter *time.Time

	// CreatedBefore filters missions created before this time
	CreatedBefore *time.Time

	// Limit limits the number of results
	Limit int

	// Offset skips the first N results
	Offset int

	// SearchText performs full-text search on name and description
	SearchText *string
}

// NewMissionFilter creates a new empty filter with default pagination.
func NewMissionFilter() *MissionFilter {
	return &MissionFilter{
		Limit:  100,
		Offset: 0,
	}
}

// WithStatus filters by mission status.
func (f *MissionFilter) WithStatus(status MissionStatus) *MissionFilter {
	f.Status = &status
	return f
}

// WithTarget filters by target ID.
func (f *MissionFilter) WithTarget(targetID types.ID) *MissionFilter {
	f.TargetID = &targetID
	return f
}

// WithWorkflow filters by workflow ID.
func (f *MissionFilter) WithWorkflow(workflowID types.ID) *MissionFilter {
	f.WorkflowID = &workflowID
	return f
}

// WithDateRange filters by creation date range.
func (f *MissionFilter) WithDateRange(after, before time.Time) *MissionFilter {
	f.CreatedAfter = &after
	f.CreatedBefore = &before
	return f
}

// WithPagination sets pagination parameters.
func (f *MissionFilter) WithPagination(limit, offset int) *MissionFilter {
	f.Limit = limit
	f.Offset = offset
	return f
}

// DBMissionStore implements MissionStore using a SQL database for mission instances
// and etcd for mission definitions.
type DBMissionStore struct {
	db        *database.DB
	etcdClient *clientv3.Client
	namespace  string
}

// NewDBMissionStore creates a new database-backed mission store.
// The etcdClient parameter can be nil if mission definition storage is not needed.
func NewDBMissionStore(db *database.DB) *DBMissionStore {
	return &DBMissionStore{
		db:        db,
		namespace: "gibson", // default namespace
	}
}

// WithEtcd configures the etcd client for mission definition storage.
// This must be called to enable CreateDefinition, GetDefinition, etc.
func (s *DBMissionStore) WithEtcd(client *clientv3.Client, namespace string) *DBMissionStore {
	s.etcdClient = client
	if namespace != "" {
		s.namespace = namespace
	}
	return s
}

// Save persists a new mission to the database.
func (s *DBMissionStore) Save(ctx context.Context, mission *Mission) error {
	if mission == nil {
		return fmt.Errorf("mission cannot be nil")
	}

	if err := mission.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Marshal JSON fields
	constraintsJSON, err := s.marshalConstraints(mission.Constraints)
	if err != nil {
		return fmt.Errorf("failed to marshal constraints: %w", err)
	}

	metricsJSON, err := s.marshalMetrics(mission.Metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	checkpointJSON, err := s.marshalCheckpoint(mission.Checkpoint)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	agentAssignmentsJSON, err := s.marshalAgentAssignments(mission.AgentAssignments)
	if err != nil {
		return fmt.Errorf("failed to marshal agent assignments: %w", err)
	}

	metadataJSON, err := s.marshalMetadata(mission.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Convert PreviousRunID to string pointer for SQL
	var previousRunIDStr *string
	if mission.PreviousRunID != nil {
		s := mission.PreviousRunID.String()
		previousRunIDStr = &s
	}

	// Convert ParentMissionID to string pointer for SQL (handle null parent gracefully)
	var parentMissionIDStr *string
	if mission.ParentMissionID != nil {
		s := mission.ParentMissionID.String()
		parentMissionIDStr = &s
	}

	query := `
		INSERT INTO missions (
			id, name, description, status, target_id, workflow_id, workflow_json,
			constraints, metrics, checkpoint, error,
			progress, findings_count, agent_assignments, metadata,
			run_number, previous_run_id, checkpoint_at,
			parent_mission_id, depth,
			created_at, started_at, completed_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.ExecContext(ctx, query,
		mission.ID.String(),
		mission.Name,
		mission.Description,
		string(mission.Status),
		mission.TargetID.String(),
		mission.WorkflowID.String(),
		mission.WorkflowJSON,
		constraintsJSON,
		metricsJSON,
		checkpointJSON,
		mission.Error,
		mission.Progress,
		mission.FindingsCount,
		agentAssignmentsJSON,
		metadataJSON,
		mission.RunNumber,
		previousRunIDStr,
		timePtr(mission.CheckpointAt),
		parentMissionIDStr,
		mission.Depth,
		mission.CreatedAt,
		timePtr(mission.StartedAt),
		timePtr(mission.CompletedAt),
		mission.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to insert mission: %w", err)
	}

	return nil
}

// Get retrieves a mission by ID.
func (s *DBMissionStore) Get(ctx context.Context, id types.ID) (*Mission, error) {
	query := `
		SELECT
			id, name, description, status, target_id, workflow_id, workflow_json,
			constraints, metrics, checkpoint, error,
			progress, findings_count, agent_assignments, metadata,
			run_number, previous_run_id, checkpoint_at,
			parent_mission_id, depth,
			created_at, started_at, completed_at, updated_at
		FROM missions
		WHERE id = ?
	`

	row := s.db.QueryRowContext(ctx, query, id.String())
	mission, err := s.scanMission(row)
	if err == sql.ErrNoRows {
		return nil, NewNotFoundError(id.String())
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get mission: %w", err)
	}

	return mission, nil
}

// GetByName retrieves a mission by name.
func (s *DBMissionStore) GetByName(ctx context.Context, name string) (*Mission, error) {
	query := `
		SELECT
			id, name, description, status, target_id, workflow_id, workflow_json,
			constraints, metrics, checkpoint, error,
			progress, findings_count, agent_assignments, metadata,
			run_number, previous_run_id, checkpoint_at,
			parent_mission_id, depth,
			created_at, started_at, completed_at, updated_at
		FROM missions
		WHERE name = ?
	`

	row := s.db.QueryRowContext(ctx, query, name)
	mission, err := s.scanMission(row)
	if err == sql.ErrNoRows {
		return nil, NewNotFoundError(name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get mission by name: %w", err)
	}

	return mission, nil
}

// List retrieves missions with optional filtering.
func (s *DBMissionStore) List(ctx context.Context, filter *MissionFilter) ([]*Mission, error) {
	if filter == nil {
		filter = NewMissionFilter()
	}

	// Set default limit if not specified
	if filter.Limit == 0 {
		filter.Limit = 100
	}

	// Build WHERE clause dynamically
	var conditions []string
	var args []interface{}

	if filter.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, string(*filter.Status))
	}

	if filter.TargetID != nil {
		conditions = append(conditions, "target_id = ?")
		args = append(args, filter.TargetID.String())
	}

	if filter.WorkflowID != nil {
		conditions = append(conditions, "workflow_id = ?")
		args = append(args, filter.WorkflowID.String())
	}

	if filter.CreatedAfter != nil {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, *filter.CreatedAfter)
	}

	if filter.CreatedBefore != nil {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, *filter.CreatedBefore)
	}

	if filter.SearchText != nil && *filter.SearchText != "" {
		conditions = append(conditions, "(name LIKE ? OR description LIKE ?)")
		searchPattern := "%" + *filter.SearchText + "%"
		args = append(args, searchPattern, searchPattern)
	}

	// Build query
	query := `
		SELECT
			id, name, description, status, target_id, workflow_id, workflow_json,
			constraints, metrics, checkpoint, error,
			progress, findings_count, agent_assignments, metadata,
			run_number, previous_run_id, checkpoint_at,
			parent_mission_id, depth,
			created_at, started_at, completed_at, updated_at
		FROM missions
	`

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list missions: %w", err)
	}
	defer rows.Close()

	return s.scanMissions(rows)
}

// ListByParent retrieves all child missions for a given parent mission ID.
// This enables querying the mission hierarchy and tracking mission lineage.
// Returns an empty slice if no child missions exist.
func (s *DBMissionStore) ListByParent(ctx context.Context, parentID types.ID) ([]*Mission, error) {
	query := `
		SELECT
			id, name, description, status, target_id, workflow_id, workflow_json,
			constraints, metrics, checkpoint, error,
			progress, findings_count, agent_assignments, metadata,
			run_number, previous_run_id, checkpoint_at,
			parent_mission_id, depth,
			created_at, started_at, completed_at, updated_at
		FROM missions
		WHERE parent_mission_id = ?
		ORDER BY created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, parentID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to list child missions: %w", err)
	}
	defer rows.Close()

	return s.scanMissions(rows)
}

// Update modifies an existing mission.
func (s *DBMissionStore) Update(ctx context.Context, mission *Mission) error {
	if mission == nil {
		return fmt.Errorf("mission cannot be nil")
	}

	if err := mission.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Update timestamp
	mission.UpdatedAt = time.Now()

	// Marshal JSON fields
	constraintsJSON, err := s.marshalConstraints(mission.Constraints)
	if err != nil {
		return fmt.Errorf("failed to marshal constraints: %w", err)
	}

	metricsJSON, err := s.marshalMetrics(mission.Metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	checkpointJSON, err := s.marshalCheckpoint(mission.Checkpoint)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	agentAssignmentsJSON, err := s.marshalAgentAssignments(mission.AgentAssignments)
	if err != nil {
		return fmt.Errorf("failed to marshal agent assignments: %w", err)
	}

	metadataJSON, err := s.marshalMetadata(mission.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Convert ParentMissionID to string pointer for SQL (handle null parent gracefully)
	var parentMissionIDStr *string
	if mission.ParentMissionID != nil {
		s := mission.ParentMissionID.String()
		parentMissionIDStr = &s
	}

	query := `
		UPDATE missions SET
			name = ?,
			description = ?,
			status = ?,
			target_id = ?,
			workflow_id = ?,
			workflow_json = ?,
			constraints = ?,
			metrics = ?,
			checkpoint = ?,
			error = ?,
			progress = ?,
			findings_count = ?,
			agent_assignments = ?,
			metadata = ?,
			parent_mission_id = ?,
			depth = ?,
			started_at = ?,
			completed_at = ?,
			updated_at = ?
		WHERE id = ?
	`

	result, err := s.db.ExecContext(ctx, query,
		mission.Name,
		mission.Description,
		string(mission.Status),
		mission.TargetID.String(),
		mission.WorkflowID.String(),
		mission.WorkflowJSON,
		constraintsJSON,
		metricsJSON,
		checkpointJSON,
		mission.Error,
		mission.Progress,
		mission.FindingsCount,
		agentAssignmentsJSON,
		metadataJSON,
		parentMissionIDStr,
		mission.Depth,
		timePtr(mission.StartedAt),
		timePtr(mission.CompletedAt),
		mission.UpdatedAt,
		mission.ID.String(),
	)

	if err != nil {
		return fmt.Errorf("failed to update mission: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return NewNotFoundError(mission.ID.String())
	}

	return nil
}

// UpdateStatus updates only the status field of a mission.
func (s *DBMissionStore) UpdateStatus(ctx context.Context, id types.ID, status MissionStatus) error {
	query := "UPDATE missions SET status = ?, updated_at = ? WHERE id = ?"
	result, err := s.db.ExecContext(ctx, query, string(status), time.Now(), id.String())
	if err != nil {
		return fmt.Errorf("failed to update mission status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return NewNotFoundError(id.String())
	}

	return nil
}

// UpdateProgress updates only the progress field of a mission.
func (s *DBMissionStore) UpdateProgress(ctx context.Context, id types.ID, progress float64) error {
	// Validate progress is in valid range (0.0 to 1.0)
	if progress < 0.0 || progress > 1.0 {
		return fmt.Errorf("progress must be between 0.0 and 1.0, got %f", progress)
	}

	query := "UPDATE missions SET progress = ?, updated_at = ? WHERE id = ?"
	result, err := s.db.ExecContext(ctx, query, progress, time.Now(), id.String())
	if err != nil {
		return fmt.Errorf("failed to update mission progress: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return NewNotFoundError(id.String())
	}

	return nil
}

// Delete soft-deletes a mission by setting status to cancelled.
// Only missions in terminal states can be deleted.
func (s *DBMissionStore) Delete(ctx context.Context, id types.ID) error {
	// First, check if mission exists and is in a terminal state
	mission, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	if !mission.Status.IsTerminal() {
		return NewInvalidStateError(mission.Status, MissionStatusCancelled)
	}

	// For terminal states, we can actually delete the record
	query := "DELETE FROM missions WHERE id = ?"
	result, err := s.db.ExecContext(ctx, query, id.String())
	if err != nil {
		return fmt.Errorf("failed to delete mission: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return NewNotFoundError(id.String())
	}

	return nil
}

// GetByTarget retrieves all missions for a specific target.
func (s *DBMissionStore) GetByTarget(ctx context.Context, targetID types.ID) ([]*Mission, error) {
	filter := NewMissionFilter().WithTarget(targetID)
	return s.List(ctx, filter)
}

// GetActive retrieves all active missions (running or paused).
func (s *DBMissionStore) GetActive(ctx context.Context) ([]*Mission, error) {
	query := `
		SELECT
			id, name, description, status, target_id, workflow_id, workflow_json,
			constraints, metrics, checkpoint, error,
			progress, findings_count, agent_assignments, metadata,
			run_number, previous_run_id, checkpoint_at,
			parent_mission_id, depth,
			created_at, started_at, completed_at, updated_at
		FROM missions
		WHERE status IN (?, ?)
		ORDER BY created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query,
		string(MissionStatusRunning),
		string(MissionStatusPaused),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get active missions: %w", err)
	}
	defer rows.Close()

	return s.scanMissions(rows)
}

// SaveCheckpoint persists a mission checkpoint for resume capability.
func (s *DBMissionStore) SaveCheckpoint(ctx context.Context, missionID types.ID, checkpoint *MissionCheckpoint) error {
	if checkpoint == nil {
		return fmt.Errorf("checkpoint cannot be nil")
	}

	checkpointJSON, err := s.marshalCheckpoint(checkpoint)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	query := "UPDATE missions SET checkpoint = ?, updated_at = ? WHERE id = ?"
	result, err := s.db.ExecContext(ctx, query, checkpointJSON, time.Now(), missionID.String())
	if err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return NewNotFoundError(missionID.String())
	}

	return nil
}

// Count returns the total number of missions matching the filter.
func (s *DBMissionStore) Count(ctx context.Context, filter *MissionFilter) (int, error) {
	if filter == nil {
		filter = NewMissionFilter()
	}

	// Build WHERE clause dynamically
	var conditions []string
	var args []interface{}

	if filter.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, string(*filter.Status))
	}

	if filter.TargetID != nil {
		conditions = append(conditions, "target_id = ?")
		args = append(args, filter.TargetID.String())
	}

	if filter.WorkflowID != nil {
		conditions = append(conditions, "workflow_id = ?")
		args = append(args, filter.WorkflowID.String())
	}

	if filter.CreatedAfter != nil {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, *filter.CreatedAfter)
	}

	if filter.CreatedBefore != nil {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, *filter.CreatedBefore)
	}

	query := "SELECT COUNT(*) FROM missions"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	var count int
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count missions: %w", err)
	}

	return count, nil
}

// scanMission scans a single mission from a query row.
func (s *DBMissionStore) scanMission(scanner interface {
	Scan(dest ...interface{}) error
}) (*Mission, error) {
	var m Mission
	var (
		idStr               string
		statusStr           string
		targetIDStr         string
		workflowIDStr       string
		workflowJSON        sql.NullString
		constraintsStr      sql.NullString
		metricsStr          sql.NullString
		checkpointStr       sql.NullString
		errorStr            sql.NullString
		agentAssignmentsStr sql.NullString
		metadataStr         sql.NullString
		runNumber           sql.NullInt64
		previousRunIDStr    sql.NullString
		checkpointAt        sql.NullTime
		parentMissionIDStr  sql.NullString
		depth               int
		startedAt           sql.NullTime
		completedAt         sql.NullTime
	)

	err := scanner.Scan(
		&idStr,
		&m.Name,
		&m.Description,
		&statusStr,
		&targetIDStr,
		&workflowIDStr,
		&workflowJSON,
		&constraintsStr,
		&metricsStr,
		&checkpointStr,
		&errorStr,
		&m.Progress,
		&m.FindingsCount,
		&agentAssignmentsStr,
		&metadataStr,
		&runNumber,
		&previousRunIDStr,
		&checkpointAt,
		&parentMissionIDStr,
		&depth,
		&m.CreatedAt,
		&startedAt,
		&completedAt,
		&m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Parse IDs
	m.ID, err = types.ParseID(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mission ID: %w", err)
	}

	m.TargetID, err = types.ParseID(targetIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target ID: %w", err)
	}

	m.WorkflowID, err = types.ParseID(workflowIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse workflow ID: %w", err)
	}

	// Set status
	m.Status = MissionStatus(statusStr)

	// Parse nullable fields
	if workflowJSON.Valid {
		m.WorkflowJSON = workflowJSON.String
	}

	if errorStr.Valid {
		m.Error = errorStr.String
	}

	if startedAt.Valid {
		m.StartedAt = &startedAt.Time
	}

	if completedAt.Valid {
		m.CompletedAt = &completedAt.Time
	}

	if checkpointAt.Valid {
		m.CheckpointAt = &checkpointAt.Time
	}

	// Parse run number
	if runNumber.Valid {
		m.RunNumber = int(runNumber.Int64)
	}

	// Parse previous run ID
	if previousRunIDStr.Valid && previousRunIDStr.String != "" {
		prevID, err := types.ParseID(previousRunIDStr.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse previous run ID: %w", err)
		}
		m.PreviousRunID = &prevID
	}

	// Parse parent mission ID - handle null gracefully for root missions
	if parentMissionIDStr.Valid && parentMissionIDStr.String != "" {
		parentID, err := types.ParseID(parentMissionIDStr.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse parent mission ID: %w", err)
		}
		m.ParentMissionID = &parentID
	}

	// Set depth
	m.Depth = depth

	// Unmarshal JSON fields
	if constraintsStr.Valid && constraintsStr.String != "" {
		if err := json.Unmarshal([]byte(constraintsStr.String), &m.Constraints); err != nil {
			return nil, fmt.Errorf("failed to unmarshal constraints: %w", err)
		}
	}

	if metricsStr.Valid && metricsStr.String != "" {
		if err := json.Unmarshal([]byte(metricsStr.String), &m.Metrics); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metrics: %w", err)
		}
	}

	if checkpointStr.Valid && checkpointStr.String != "" {
		if err := json.Unmarshal([]byte(checkpointStr.String), &m.Checkpoint); err != nil {
			return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
		}
	}

	if agentAssignmentsStr.Valid && agentAssignmentsStr.String != "" {
		if err := json.Unmarshal([]byte(agentAssignmentsStr.String), &m.AgentAssignments); err != nil {
			return nil, fmt.Errorf("failed to unmarshal agent assignments: %w", err)
		}
	}

	if metadataStr.Valid && metadataStr.String != "" {
		if err := json.Unmarshal([]byte(metadataStr.String), &m.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &m, nil
}

// scanMissions scans multiple missions from query rows.
func (s *DBMissionStore) scanMissions(rows *sql.Rows) ([]*Mission, error) {
	missions := make([]*Mission, 0)

	for rows.Next() {
		mission, err := s.scanMission(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan mission: %w", err)
		}
		missions = append(missions, mission)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return missions, nil
}

// marshalConstraints marshals mission constraints to JSON.
func (s *DBMissionStore) marshalConstraints(constraints *MissionConstraints) (string, error) {
	if constraints == nil {
		return "", nil
	}
	data, err := json.Marshal(constraints)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// marshalMetrics marshals mission metrics to JSON.
func (s *DBMissionStore) marshalMetrics(metrics *MissionMetrics) (string, error) {
	if metrics == nil {
		return "", nil
	}
	data, err := json.Marshal(metrics)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// marshalCheckpoint marshals mission checkpoint to JSON.
func (s *DBMissionStore) marshalCheckpoint(checkpoint *MissionCheckpoint) (string, error) {
	if checkpoint == nil {
		return "", nil
	}
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// marshalAgentAssignments marshals agent assignments to JSON.
func (s *DBMissionStore) marshalAgentAssignments(assignments map[string]string) (string, error) {
	if assignments == nil || len(assignments) == 0 {
		return "", nil
	}
	data, err := json.Marshal(assignments)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// marshalMetadata marshals metadata to JSON.
func (s *DBMissionStore) marshalMetadata(metadata map[string]any) (string, error) {
	if metadata == nil || len(metadata) == 0 {
		return "", nil
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// timePtr converts a *time.Time to an interface{} suitable for SQL null handling.
func timePtr(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}

// GetByNameAndStatus retrieves a mission by name and status.
func (s *DBMissionStore) GetByNameAndStatus(ctx context.Context, name string, status MissionStatus) (*Mission, error) {
	query := `
		SELECT
			id, name, description, status, target_id, workflow_id, workflow_json,
			constraints, metrics, checkpoint, error,
			progress, findings_count, agent_assignments, metadata,
			run_number, previous_run_id, checkpoint_at,
			parent_mission_id, depth,
			created_at, started_at, completed_at, updated_at
		FROM missions
		WHERE name = ? AND status = ?
		ORDER BY created_at DESC
		LIMIT 1
	`

	row := s.db.QueryRowContext(ctx, query, name, string(status))
	mission, err := s.scanMission(row)
	if err == sql.ErrNoRows {
		return nil, NewNotFoundError(fmt.Sprintf("%s (status=%s)", name, status))
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get mission by name and status: %w", err)
	}

	return mission, nil
}

// ListByName retrieves all missions with the given name, ordered by run number descending.
func (s *DBMissionStore) ListByName(ctx context.Context, name string, limit int) ([]*Mission, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT
			id, name, description, status, target_id, workflow_id, workflow_json,
			constraints, metrics, checkpoint, error,
			progress, findings_count, agent_assignments, metadata,
			run_number, previous_run_id, checkpoint_at,
			parent_mission_id, depth,
			created_at, started_at, completed_at, updated_at
		FROM missions
		WHERE name = ?
		ORDER BY run_number DESC
		LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, query, name, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list missions by name: %w", err)
	}
	defer rows.Close()

	return s.scanMissions(rows)
}

// GetLatestByName retrieves the most recent mission with the given name.
func (s *DBMissionStore) GetLatestByName(ctx context.Context, name string) (*Mission, error) {
	query := `
		SELECT
			id, name, description, status, target_id, workflow_id, workflow_json,
			constraints, metrics, checkpoint, error,
			progress, findings_count, agent_assignments, metadata,
			run_number, previous_run_id, checkpoint_at,
			parent_mission_id, depth,
			created_at, started_at, completed_at, updated_at
		FROM missions
		WHERE name = ?
		ORDER BY run_number DESC
		LIMIT 1
	`

	row := s.db.QueryRowContext(ctx, query, name)
	mission, err := s.scanMission(row)
	if err == sql.ErrNoRows {
		return nil, NewNotFoundError(name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest mission by name: %w", err)
	}

	return mission, nil
}

// IncrementRunNumber atomically increments and returns the next run number for a mission name.
// This method uses the run_number column added in migration 11.
func (s *DBMissionStore) IncrementRunNumber(ctx context.Context, name string) (int, error) {
	// Query the maximum run number for this mission name
	// Using the run_number column directly (not metadata JSON)
	query := `
		SELECT COALESCE(MAX(run_number), 0)
		FROM missions
		WHERE name = ?
	`

	var maxRunNumber int
	err := s.db.QueryRowContext(ctx, query, name).Scan(&maxRunNumber)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to get max run number: %w", err)
	}

	// Next run number is max + 1
	return maxRunNumber + 1, nil
}

// Mission Definition Storage (etcd-backed)
// These methods implement storage for installable mission definitions (templates)
// using etcd with the key pattern: /gibson/missions/definitions/{name}

var (
	// ErrEtcdNotConfigured is returned when etcd operations are attempted without configuration
	ErrEtcdNotConfigured = fmt.Errorf("etcd client not configured")

	// ErrDefinitionExists is returned when attempting to create a definition that already exists
	ErrDefinitionExists = fmt.Errorf("mission definition already exists")

	// ErrDefinitionNotFound is returned when a definition cannot be found
	ErrDefinitionNotFound = fmt.Errorf("mission definition not found")
)

// definitionKey constructs the etcd key for a mission definition.
// Format: /{namespace}/missions/definitions/{name}
func (s *DBMissionStore) definitionKey(name string) string {
	return filepath.Join("/", s.namespace, "missions", "definitions", name)
}

// definitionsPrefix constructs the etcd key prefix for all mission definitions.
// Format: /{namespace}/missions/definitions/
func (s *DBMissionStore) definitionsPrefix() string {
	return filepath.Join("/", s.namespace, "missions", "definitions") + "/"
}

// CreateDefinition stores a new mission definition in etcd.
// Returns ErrDefinitionExists if a definition with the same name already exists.
// Returns ErrEtcdNotConfigured if etcd client is not configured.
func (s *DBMissionStore) CreateDefinition(ctx context.Context, def *MissionDefinition) error {
	if s.etcdClient == nil {
		return ErrEtcdNotConfigured
	}

	if def == nil {
		return fmt.Errorf("mission definition cannot be nil")
	}

	if def.Name == "" {
		return fmt.Errorf("mission definition name is required")
	}

	key := s.definitionKey(def.Name)

	// Set timestamps if not already set
	now := time.Now()
	if def.InstalledAt.IsZero() {
		def.InstalledAt = now
	}
	if def.CreatedAt.IsZero() {
		def.CreatedAt = now
	}

	// Marshal to JSON
	data, err := json.Marshal(def)
	if err != nil {
		return fmt.Errorf("failed to marshal mission definition: %w", err)
	}

	// Use transaction to create only if key doesn't exist
	txn := s.etcdClient.Txn(ctx)
	resp, err := txn.
		If(clientv3.Compare(clientv3.Version(key), "=", 0)).
		Then(clientv3.OpPut(key, string(data))).
		Commit()

	if err != nil {
		return fmt.Errorf("failed to create mission definition: %w", err)
	}

	if !resp.Succeeded {
		return ErrDefinitionExists
	}

	return nil
}

// GetDefinition retrieves a mission definition by name from etcd.
// Returns nil, nil if not found.
// Returns ErrEtcdNotConfigured if etcd client is not configured.
func (s *DBMissionStore) GetDefinition(ctx context.Context, name string) (*MissionDefinition, error) {
	if s.etcdClient == nil {
		return nil, ErrEtcdNotConfigured
	}

	if name == "" {
		return nil, fmt.Errorf("mission definition name is required")
	}

	key := s.definitionKey(name)

	resp, err := s.etcdClient.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get mission definition: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, nil
	}

	var def MissionDefinition
	if err := json.Unmarshal(resp.Kvs[0].Value, &def); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mission definition: %w", err)
	}

	return &def, nil
}

// ListDefinitions returns all installed mission definitions from etcd.
// Returns an empty slice if no definitions are found.
// Returns ErrEtcdNotConfigured if etcd client is not configured.
func (s *DBMissionStore) ListDefinitions(ctx context.Context) ([]*MissionDefinition, error) {
	if s.etcdClient == nil {
		return nil, ErrEtcdNotConfigured
	}

	prefix := s.definitionsPrefix()

	resp, err := s.etcdClient.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list mission definitions: %w", err)
	}

	definitions := make([]*MissionDefinition, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var def MissionDefinition
		if err := json.Unmarshal(kv.Value, &def); err != nil {
			// Log error but continue with other definitions
			continue
		}
		definitions = append(definitions, &def)
	}

	return definitions, nil
}

// UpdateDefinition updates an existing mission definition in etcd.
// Returns ErrDefinitionNotFound if the definition does not exist.
// Returns ErrEtcdNotConfigured if etcd client is not configured.
func (s *DBMissionStore) UpdateDefinition(ctx context.Context, def *MissionDefinition) error {
	if s.etcdClient == nil {
		return ErrEtcdNotConfigured
	}

	if def == nil {
		return fmt.Errorf("mission definition cannot be nil")
	}

	if def.Name == "" {
		return fmt.Errorf("mission definition name is required")
	}

	key := s.definitionKey(def.Name)

	// Check if definition exists
	resp, err := s.etcdClient.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check mission definition: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return ErrDefinitionNotFound
	}

	// Preserve InstalledAt timestamp from existing record
	var existing MissionDefinition
	if err := json.Unmarshal(resp.Kvs[0].Value, &existing); err != nil {
		return fmt.Errorf("failed to unmarshal existing definition: %w", err)
	}

	// Preserve original install time, update CreatedAt to reflect modification
	if !existing.InstalledAt.IsZero() {
		def.InstalledAt = existing.InstalledAt
	}
	def.CreatedAt = time.Now()

	// Marshal to JSON
	data, err := json.Marshal(def)
	if err != nil {
		return fmt.Errorf("failed to marshal mission definition: %w", err)
	}

	// Update the value
	_, err = s.etcdClient.Put(ctx, key, string(data))
	if err != nil {
		return fmt.Errorf("failed to update mission definition: %w", err)
	}

	return nil
}

// DeleteDefinition removes a mission definition from etcd.
// Returns ErrDefinitionNotFound if the definition does not exist.
// Returns ErrEtcdNotConfigured if etcd client is not configured.
func (s *DBMissionStore) DeleteDefinition(ctx context.Context, name string) error {
	if s.etcdClient == nil {
		return ErrEtcdNotConfigured
	}

	if name == "" {
		return fmt.Errorf("mission definition name is required")
	}

	key := s.definitionKey(name)

	// Use transaction to delete only if key exists
	txn := s.etcdClient.Txn(ctx)
	resp, err := txn.
		If(clientv3.Compare(clientv3.Version(key), ">", 0)).
		Then(clientv3.OpDelete(key)).
		Commit()

	if err != nil {
		return fmt.Errorf("failed to delete mission definition: %w", err)
	}

	if !resp.Succeeded {
		return ErrDefinitionNotFound
	}

	return nil
}

// Ensure DBMissionStore implements MissionStore at compile time.
var _ MissionStore = (*DBMissionStore)(nil)
