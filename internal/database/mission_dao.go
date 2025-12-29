package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// MissionStatus represents the current status of a mission
type MissionStatus string

const (
	// MissionStatusPending indicates the mission is ready but not yet started
	MissionStatusPending MissionStatus = "pending"
	// MissionStatusRunning indicates the mission is currently executing
	MissionStatusRunning MissionStatus = "running"
	// MissionStatusCompleted indicates the mission has completed successfully
	MissionStatusCompleted MissionStatus = "completed"
	// MissionStatusFailed indicates the mission execution has failed
	MissionStatusFailed MissionStatus = "failed"
	// MissionStatusCancelled indicates the mission was cancelled during execution
	MissionStatusCancelled MissionStatus = "cancelled"
)

// Mission represents a persisted mission with workflow
type Mission struct {
	ID               types.ID          `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Status           MissionStatus     `json:"status"`
	WorkflowID       types.ID          `json:"workflow_id"`
	WorkflowJSON     string            `json:"workflow,omitempty"`
	Progress         float64           `json:"progress"`
	FindingsCount    int               `json:"findings_count"`
	AgentAssignments map[string]string `json:"agent_assignments,omitempty"`
	Metadata         map[string]any    `json:"metadata,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	StartedAt        *time.Time        `json:"started_at,omitempty"`
	CompletedAt      *time.Time        `json:"completed_at,omitempty"`
}

// MissionDAO provides database operations for missions
type MissionDAO interface {
	// Create creates a new mission
	Create(ctx context.Context, mission *Mission) error

	// GetByID retrieves a mission by ID
	GetByID(ctx context.Context, id types.ID) (*Mission, error)

	// GetByName retrieves a mission by name
	GetByName(ctx context.Context, name string) (*Mission, error)

	// List lists all missions with optional status filter
	List(ctx context.Context, status MissionStatus) ([]*Mission, error)

	// Update updates a mission
	Update(ctx context.Context, mission *Mission) error

	// UpdateStatus updates just the status of a mission
	UpdateStatus(ctx context.Context, id types.ID, status MissionStatus) error

	// Delete deletes a mission
	Delete(ctx context.Context, id types.ID) error

	// UpdateProgress updates the progress of a mission
	UpdateProgress(ctx context.Context, id types.ID, progress float64) error
}

// missionDAO implements MissionDAO
type missionDAO struct {
	db *DB
}

// NewMissionDAO creates a new mission DAO
func NewMissionDAO(db *DB) MissionDAO {
	return &missionDAO{db: db}
}

// Create creates a new mission
func (d *missionDAO) Create(ctx context.Context, mission *Mission) error {
	if mission.ID == "" {
		mission.ID = types.NewID()
	}

	// Serialize agent assignments to JSON
	var agentAssignmentsJSON []byte
	var err error
	if mission.AgentAssignments != nil {
		agentAssignmentsJSON, err = json.Marshal(mission.AgentAssignments)
		if err != nil {
			return fmt.Errorf("failed to marshal agent assignments: %w", err)
		}
	}

	// Serialize metadata to JSON
	var metadataJSON []byte
	if mission.Metadata != nil {
		metadataJSON, err = json.Marshal(mission.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	query := `
		INSERT INTO missions (
			id, name, description, status, workflow_id, workflow, progress,
			findings_count, agent_assignments, metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`

	_, err = d.db.conn.ExecContext(
		ctx, query,
		mission.ID,
		mission.Name,
		mission.Description,
		mission.Status,
		mission.WorkflowID,
		mission.WorkflowJSON,
		mission.Progress,
		mission.FindingsCount,
		string(agentAssignmentsJSON),
		string(metadataJSON),
	)

	if err != nil {
		return fmt.Errorf("failed to create mission: %w", err)
	}

	return nil
}

// GetByID retrieves a mission by ID
func (d *missionDAO) GetByID(ctx context.Context, id types.ID) (*Mission, error) {
	query := `
		SELECT
			id, name, description, status, workflow_id, workflow, progress,
			findings_count, agent_assignments, metadata, created_at, updated_at,
			started_at, completed_at
		FROM missions
		WHERE id = ?
	`

	var mission Mission
	var workflowJSON, agentAssignmentsJSON, metadataJSON sql.NullString
	var startedAt, completedAt sql.NullTime

	err := d.db.conn.QueryRowContext(ctx, query, id).Scan(
		&mission.ID,
		&mission.Name,
		&mission.Description,
		&mission.Status,
		&mission.WorkflowID,
		&workflowJSON,
		&mission.Progress,
		&mission.FindingsCount,
		&agentAssignmentsJSON,
		&metadataJSON,
		&mission.CreatedAt,
		&mission.UpdatedAt,
		&startedAt,
		&completedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("mission not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get mission: %w", err)
	}

	// Store workflow JSON directly
	if workflowJSON.Valid {
		mission.WorkflowJSON = workflowJSON.String
	}

	// Unmarshal agent assignments
	if agentAssignmentsJSON.Valid && agentAssignmentsJSON.String != "" {
		if err := json.Unmarshal([]byte(agentAssignmentsJSON.String), &mission.AgentAssignments); err != nil {
			return nil, fmt.Errorf("failed to unmarshal agent assignments: %w", err)
		}
	}

	// Unmarshal metadata
	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &mission.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	// Handle nullable timestamps
	if startedAt.Valid {
		mission.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		mission.CompletedAt = &completedAt.Time
	}

	return &mission, nil
}

// GetByName retrieves a mission by name
func (d *missionDAO) GetByName(ctx context.Context, name string) (*Mission, error) {
	query := `
		SELECT
			id, name, description, status, workflow_id, workflow, progress,
			findings_count, agent_assignments, metadata, created_at, updated_at,
			started_at, completed_at
		FROM missions
		WHERE name = ?
	`

	var mission Mission
	var workflowJSON, agentAssignmentsJSON, metadataJSON sql.NullString
	var startedAt, completedAt sql.NullTime

	err := d.db.conn.QueryRowContext(ctx, query, name).Scan(
		&mission.ID,
		&mission.Name,
		&mission.Description,
		&mission.Status,
		&mission.WorkflowID,
		&workflowJSON,
		&mission.Progress,
		&mission.FindingsCount,
		&agentAssignmentsJSON,
		&metadataJSON,
		&mission.CreatedAt,
		&mission.UpdatedAt,
		&startedAt,
		&completedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("mission not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get mission: %w", err)
	}

	// Store workflow JSON directly
	if workflowJSON.Valid {
		mission.WorkflowJSON = workflowJSON.String
	}

	// Unmarshal agent assignments
	if agentAssignmentsJSON.Valid && agentAssignmentsJSON.String != "" {
		if err := json.Unmarshal([]byte(agentAssignmentsJSON.String), &mission.AgentAssignments); err != nil {
			return nil, fmt.Errorf("failed to unmarshal agent assignments: %w", err)
		}
	}

	// Unmarshal metadata
	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &mission.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	// Handle nullable timestamps
	if startedAt.Valid {
		mission.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		mission.CompletedAt = &completedAt.Time
	}

	return &mission, nil
}

// List lists all missions with optional status filter
func (d *missionDAO) List(ctx context.Context, status MissionStatus) ([]*Mission, error) {
	var query string
	var args []interface{}

	if status != "" {
		query = `
			SELECT
				id, name, description, status, workflow_id, workflow, progress,
				findings_count, agent_assignments, metadata, created_at, updated_at,
				started_at, completed_at
			FROM missions
			WHERE status = ?
			ORDER BY created_at DESC
		`
		args = append(args, status)
	} else {
		query = `
			SELECT
				id, name, description, status, workflow_id, workflow, progress,
				findings_count, agent_assignments, metadata, created_at, updated_at,
				started_at, completed_at
			FROM missions
			ORDER BY created_at DESC
		`
	}

	rows, err := d.db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list missions: %w", err)
	}
	defer rows.Close()

	var missions []*Mission
	for rows.Next() {
		var mission Mission
		var workflowJSON, agentAssignmentsJSON, metadataJSON sql.NullString
		var startedAt, completedAt sql.NullTime

		err := rows.Scan(
			&mission.ID,
			&mission.Name,
			&mission.Description,
			&mission.Status,
			&mission.WorkflowID,
			&workflowJSON,
			&mission.Progress,
			&mission.FindingsCount,
			&agentAssignmentsJSON,
			&metadataJSON,
			&mission.CreatedAt,
			&mission.UpdatedAt,
			&startedAt,
			&completedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan mission: %w", err)
		}

		// Store workflow JSON directly
		if workflowJSON.Valid {
			mission.WorkflowJSON = workflowJSON.String
		}

		// Unmarshal agent assignments
		if agentAssignmentsJSON.Valid && agentAssignmentsJSON.String != "" {
			if err := json.Unmarshal([]byte(agentAssignmentsJSON.String), &mission.AgentAssignments); err != nil {
				return nil, fmt.Errorf("failed to unmarshal agent assignments: %w", err)
			}
		}

		// Unmarshal metadata
		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &mission.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		// Handle nullable timestamps
		if startedAt.Valid {
			mission.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			mission.CompletedAt = &completedAt.Time
		}

		missions = append(missions, &mission)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating missions: %w", err)
	}

	return missions, nil
}

// Update updates a mission
func (d *missionDAO) Update(ctx context.Context, mission *Mission) error {
	// Serialize agent assignments to JSON
	var agentAssignmentsJSON []byte
	var err error
	if mission.AgentAssignments != nil {
		agentAssignmentsJSON, err = json.Marshal(mission.AgentAssignments)
		if err != nil {
			return fmt.Errorf("failed to marshal agent assignments: %w", err)
		}
	}

	// Serialize metadata to JSON
	var metadataJSON []byte
	if mission.Metadata != nil {
		metadataJSON, err = json.Marshal(mission.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	query := `
		UPDATE missions
		SET name = ?, description = ?, status = ?, workflow_id = ?, workflow = ?,
		    progress = ?, findings_count = ?, agent_assignments = ?, metadata = ?,
		    updated_at = CURRENT_TIMESTAMP,
		    started_at = ?, completed_at = ?
		WHERE id = ?
	`

	_, err = d.db.conn.ExecContext(
		ctx, query,
		mission.Name,
		mission.Description,
		mission.Status,
		mission.WorkflowID,
		mission.WorkflowJSON,
		mission.Progress,
		mission.FindingsCount,
		string(agentAssignmentsJSON),
		string(metadataJSON),
		mission.StartedAt,
		mission.CompletedAt,
		mission.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update mission: %w", err)
	}

	return nil
}

// UpdateStatus updates just the status of a mission
func (d *missionDAO) UpdateStatus(ctx context.Context, id types.ID, status MissionStatus) error {
	query := `
		UPDATE missions
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	result, err := d.db.conn.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update mission status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("mission not found: %s", id)
	}

	return nil
}

// Delete deletes a mission
func (d *missionDAO) Delete(ctx context.Context, id types.ID) error {
	query := `DELETE FROM missions WHERE id = ?`

	result, err := d.db.conn.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete mission: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("mission not found: %s", id)
	}

	return nil
}

// UpdateProgress updates the progress of a mission
func (d *missionDAO) UpdateProgress(ctx context.Context, id types.ID, progress float64) error {
	query := `
		UPDATE missions
		SET progress = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	result, err := d.db.conn.ExecContext(ctx, query, progress, id)
	if err != nil {
		return fmt.Errorf("failed to update mission progress: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("mission not found: %s", id)
	}

	return nil
}
