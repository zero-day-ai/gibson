package mission

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// MissionStore provides persistence for Mission entities.
type MissionStore interface {
	// Save persists a new mission to the database
	Save(ctx context.Context, mission *Mission) error

	// Get retrieves a mission by ID
	Get(ctx context.Context, id types.ID) (*Mission, error)

	// List retrieves missions with optional filtering
	List(ctx context.Context, filter *MissionFilter) ([]*Mission, error)

	// Update modifies an existing mission
	Update(ctx context.Context, mission *Mission) error

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

// DBMissionStore implements MissionStore using a SQL database.
type DBMissionStore struct {
	db *database.DB
}

// NewDBMissionStore creates a new database-backed mission store.
func NewDBMissionStore(db *database.DB) *DBMissionStore {
	return &DBMissionStore{db: db}
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

	query := `
		INSERT INTO missions (
			id, name, description, status, target_id, workflow_id,
			constraints, metrics, checkpoint, error,
			created_at, started_at, completed_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.ExecContext(ctx, query,
		mission.ID.String(),
		mission.Name,
		mission.Description,
		string(mission.Status),
		mission.TargetID.String(),
		mission.WorkflowID.String(),
		constraintsJSON,
		metricsJSON,
		checkpointJSON,
		mission.Error,
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
			id, name, description, status, target_id, workflow_id,
			constraints, metrics, checkpoint, error,
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
			id, name, description, status, target_id, workflow_id,
			constraints, metrics, checkpoint, error,
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

	query := `
		UPDATE missions SET
			name = ?,
			description = ?,
			status = ?,
			target_id = ?,
			workflow_id = ?,
			constraints = ?,
			metrics = ?,
			checkpoint = ?,
			error = ?,
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
		constraintsJSON,
		metricsJSON,
		checkpointJSON,
		mission.Error,
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
			id, name, description, status, target_id, workflow_id,
			constraints, metrics, checkpoint, error,
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
		idStr          string
		statusStr      string
		targetIDStr    string
		workflowIDStr  string
		constraintsStr sql.NullString
		metricsStr     sql.NullString
		checkpointStr  sql.NullString
		errorStr       sql.NullString
		startedAt      sql.NullTime
		completedAt    sql.NullTime
	)

	err := scanner.Scan(
		&idStr,
		&m.Name,
		&m.Description,
		&statusStr,
		&targetIDStr,
		&workflowIDStr,
		&constraintsStr,
		&metricsStr,
		&checkpointStr,
		&errorStr,
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
	if errorStr.Valid {
		m.Error = errorStr.String
	}

	if startedAt.Valid {
		m.StartedAt = &startedAt.Time
	}

	if completedAt.Valid {
		m.CompletedAt = &completedAt.Time
	}

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

// timePtr converts a *time.Time to an interface{} suitable for SQL null handling.
func timePtr(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}

// Ensure DBMissionStore implements MissionStore at compile time.
var _ MissionStore = (*DBMissionStore)(nil)
