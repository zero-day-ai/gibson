package mission

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// MissionRunStore provides persistence for MissionRun entities.
type MissionRunStore interface {
	// Save persists a new mission run to the database
	Save(ctx context.Context, run *MissionRun) error

	// Get retrieves a mission run by ID
	Get(ctx context.Context, id types.ID) (*MissionRun, error)

	// GetByMissionAndNumber retrieves a run by mission ID and run number
	GetByMissionAndNumber(ctx context.Context, missionID types.ID, runNumber int) (*MissionRun, error)

	// ListByMission retrieves all runs for a mission, ordered by run number descending
	ListByMission(ctx context.Context, missionID types.ID) ([]*MissionRun, error)

	// GetLatestByMission retrieves the most recent run for a mission
	GetLatestByMission(ctx context.Context, missionID types.ID) (*MissionRun, error)

	// GetNextRunNumber returns the next run number for a mission
	GetNextRunNumber(ctx context.Context, missionID types.ID) (int, error)

	// Update modifies an existing mission run
	Update(ctx context.Context, run *MissionRun) error

	// UpdateStatus updates only the status field
	UpdateStatus(ctx context.Context, id types.ID, status MissionRunStatus) error

	// UpdateProgress updates only the progress field
	UpdateProgress(ctx context.Context, id types.ID, progress float64) error

	// GetActive retrieves all active runs (running or paused)
	GetActive(ctx context.Context) ([]*MissionRun, error)

	// Delete removes a mission run (only terminal states)
	Delete(ctx context.Context, id types.ID) error

	// CountByMission returns the number of runs for a mission
	CountByMission(ctx context.Context, missionID types.ID) (int, error)
}

// DBMissionRunStore implements MissionRunStore using SQLite.
type DBMissionRunStore struct {
	db *database.DB
}

// NewDBMissionRunStore creates a new database-backed mission run store.
func NewDBMissionRunStore(db *database.DB) *DBMissionRunStore {
	return &DBMissionRunStore{db: db}
}

// Save persists a new mission run to the database.
func (s *DBMissionRunStore) Save(ctx context.Context, run *MissionRun) error {
	if run == nil {
		return fmt.Errorf("mission run cannot be nil")
	}

	if err := run.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	checkpointJSON, err := s.marshalCheckpoint(run.Checkpoint)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	query := `
		INSERT INTO mission_runs (
			id, mission_id, run_number, status,
			progress, findings_count, checkpoint, error,
			created_at, started_at, completed_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.ExecContext(ctx, query,
		run.ID.String(),
		run.MissionID.String(),
		run.RunNumber,
		string(run.Status),
		run.Progress,
		run.FindingsCount,
		checkpointJSON,
		run.Error,
		run.CreatedAt,
		timePtr(run.StartedAt),
		timePtr(run.CompletedAt),
		run.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to insert mission run: %w", err)
	}

	return nil
}

// Get retrieves a mission run by ID.
func (s *DBMissionRunStore) Get(ctx context.Context, id types.ID) (*MissionRun, error) {
	query := `
		SELECT
			id, mission_id, run_number, status,
			progress, findings_count, checkpoint, error,
			created_at, started_at, completed_at, updated_at
		FROM mission_runs
		WHERE id = ?
	`

	row := s.db.QueryRowContext(ctx, query, id.String())
	run, err := s.scanRun(row)
	if err == sql.ErrNoRows {
		return nil, NewNotFoundError(id.String())
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get mission run: %w", err)
	}

	return run, nil
}

// GetByMissionAndNumber retrieves a run by mission ID and run number.
func (s *DBMissionRunStore) GetByMissionAndNumber(ctx context.Context, missionID types.ID, runNumber int) (*MissionRun, error) {
	query := `
		SELECT
			id, mission_id, run_number, status,
			progress, findings_count, checkpoint, error,
			created_at, started_at, completed_at, updated_at
		FROM mission_runs
		WHERE mission_id = ? AND run_number = ?
	`

	row := s.db.QueryRowContext(ctx, query, missionID.String(), runNumber)
	run, err := s.scanRun(row)
	if err == sql.ErrNoRows {
		return nil, NewNotFoundError(fmt.Sprintf("mission %s run %d", missionID, runNumber))
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get mission run: %w", err)
	}

	return run, nil
}

// ListByMission retrieves all runs for a mission, ordered by run number descending.
func (s *DBMissionRunStore) ListByMission(ctx context.Context, missionID types.ID) ([]*MissionRun, error) {
	query := `
		SELECT
			id, mission_id, run_number, status,
			progress, findings_count, checkpoint, error,
			created_at, started_at, completed_at, updated_at
		FROM mission_runs
		WHERE mission_id = ?
		ORDER BY run_number DESC
	`

	rows, err := s.db.QueryContext(ctx, query, missionID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to list mission runs: %w", err)
	}
	defer rows.Close()

	return s.scanRuns(rows)
}

// GetLatestByMission retrieves the most recent run for a mission.
func (s *DBMissionRunStore) GetLatestByMission(ctx context.Context, missionID types.ID) (*MissionRun, error) {
	query := `
		SELECT
			id, mission_id, run_number, status,
			progress, findings_count, checkpoint, error,
			created_at, started_at, completed_at, updated_at
		FROM mission_runs
		WHERE mission_id = ?
		ORDER BY run_number DESC
		LIMIT 1
	`

	row := s.db.QueryRowContext(ctx, query, missionID.String())
	run, err := s.scanRun(row)
	if err == sql.ErrNoRows {
		return nil, nil // No runs yet is not an error
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest mission run: %w", err)
	}

	return run, nil
}

// GetNextRunNumber returns the next run number for a mission.
func (s *DBMissionRunStore) GetNextRunNumber(ctx context.Context, missionID types.ID) (int, error) {
	query := `
		SELECT COALESCE(MAX(run_number), 0)
		FROM mission_runs
		WHERE mission_id = ?
	`

	var maxRunNumber int
	err := s.db.QueryRowContext(ctx, query, missionID.String()).Scan(&maxRunNumber)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to get max run number: %w", err)
	}

	return maxRunNumber + 1, nil
}

// Update modifies an existing mission run.
func (s *DBMissionRunStore) Update(ctx context.Context, run *MissionRun) error {
	if run == nil {
		return fmt.Errorf("mission run cannot be nil")
	}

	if err := run.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	run.UpdatedAt = time.Now()

	checkpointJSON, err := s.marshalCheckpoint(run.Checkpoint)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	query := `
		UPDATE mission_runs SET
			status = ?,
			progress = ?,
			findings_count = ?,
			checkpoint = ?,
			error = ?,
			started_at = ?,
			completed_at = ?,
			updated_at = ?
		WHERE id = ?
	`

	result, err := s.db.ExecContext(ctx, query,
		string(run.Status),
		run.Progress,
		run.FindingsCount,
		checkpointJSON,
		run.Error,
		timePtr(run.StartedAt),
		timePtr(run.CompletedAt),
		run.UpdatedAt,
		run.ID.String(),
	)

	if err != nil {
		return fmt.Errorf("failed to update mission run: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return NewNotFoundError(run.ID.String())
	}

	return nil
}

// UpdateStatus updates only the status field.
func (s *DBMissionRunStore) UpdateStatus(ctx context.Context, id types.ID, status MissionRunStatus) error {
	query := "UPDATE mission_runs SET status = ?, updated_at = ? WHERE id = ?"
	result, err := s.db.ExecContext(ctx, query, string(status), time.Now(), id.String())
	if err != nil {
		return fmt.Errorf("failed to update mission run status: %w", err)
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

// UpdateProgress updates only the progress field.
func (s *DBMissionRunStore) UpdateProgress(ctx context.Context, id types.ID, progress float64) error {
	if progress < 0 || progress > 1 {
		return fmt.Errorf("progress must be between 0.0 and 1.0, got %f", progress)
	}

	query := "UPDATE mission_runs SET progress = ?, updated_at = ? WHERE id = ?"
	result, err := s.db.ExecContext(ctx, query, progress, time.Now(), id.String())
	if err != nil {
		return fmt.Errorf("failed to update mission run progress: %w", err)
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

// GetActive retrieves all active runs (running or paused).
func (s *DBMissionRunStore) GetActive(ctx context.Context) ([]*MissionRun, error) {
	query := `
		SELECT
			id, mission_id, run_number, status,
			progress, findings_count, checkpoint, error,
			created_at, started_at, completed_at, updated_at
		FROM mission_runs
		WHERE status IN (?, ?)
		ORDER BY created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query,
		string(MissionRunStatusRunning),
		string(MissionRunStatusPaused),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get active mission runs: %w", err)
	}
	defer rows.Close()

	return s.scanRuns(rows)
}

// Delete removes a mission run (only terminal states).
func (s *DBMissionRunStore) Delete(ctx context.Context, id types.ID) error {
	// First check if run is in terminal state
	run, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	if !run.Status.IsTerminal() {
		return fmt.Errorf("cannot delete run in non-terminal status: %s", run.Status)
	}

	query := "DELETE FROM mission_runs WHERE id = ?"
	result, err := s.db.ExecContext(ctx, query, id.String())
	if err != nil {
		return fmt.Errorf("failed to delete mission run: %w", err)
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

// CountByMission returns the number of runs for a mission.
func (s *DBMissionRunStore) CountByMission(ctx context.Context, missionID types.ID) (int, error) {
	query := "SELECT COUNT(*) FROM mission_runs WHERE mission_id = ?"

	var count int
	err := s.db.QueryRowContext(ctx, query, missionID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count mission runs: %w", err)
	}

	return count, nil
}

// scanRun scans a single mission run from a query row.
func (s *DBMissionRunStore) scanRun(scanner interface {
	Scan(dest ...interface{}) error
}) (*MissionRun, error) {
	var r MissionRun
	var (
		idStr         string
		missionIDStr  string
		statusStr     string
		checkpointStr sql.NullString
		errorStr      sql.NullString
		startedAt     sql.NullTime
		completedAt   sql.NullTime
	)

	err := scanner.Scan(
		&idStr,
		&missionIDStr,
		&r.RunNumber,
		&statusStr,
		&r.Progress,
		&r.FindingsCount,
		&checkpointStr,
		&errorStr,
		&r.CreatedAt,
		&startedAt,
		&completedAt,
		&r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Parse IDs
	r.ID, err = types.ParseID(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse run ID: %w", err)
	}

	r.MissionID, err = types.ParseID(missionIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mission ID: %w", err)
	}

	// Set status
	r.Status = MissionRunStatus(statusStr)

	// Parse nullable fields
	if errorStr.Valid {
		r.Error = errorStr.String
	}

	if startedAt.Valid {
		r.StartedAt = &startedAt.Time
	}

	if completedAt.Valid {
		r.CompletedAt = &completedAt.Time
	}

	// Unmarshal checkpoint
	if checkpointStr.Valid && checkpointStr.String != "" {
		if err := json.Unmarshal([]byte(checkpointStr.String), &r.Checkpoint); err != nil {
			return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
		}
	}

	return &r, nil
}

// scanRuns scans multiple mission runs from query rows.
func (s *DBMissionRunStore) scanRuns(rows *sql.Rows) ([]*MissionRun, error) {
	runs := make([]*MissionRun, 0)

	for rows.Next() {
		run, err := s.scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan mission run: %w", err)
		}
		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return runs, nil
}

// marshalCheckpoint marshals checkpoint to JSON.
func (s *DBMissionRunStore) marshalCheckpoint(checkpoint *MissionCheckpoint) (string, error) {
	if checkpoint == nil {
		return "", nil
	}
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Ensure DBMissionRunStore implements MissionRunStore at compile time.
var _ MissionRunStore = (*DBMissionRunStore)(nil)
