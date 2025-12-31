package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/component"
)

// ComponentDAO provides database operations for components
type ComponentDAO interface {
	// Create inserts a new component
	Create(ctx context.Context, comp *component.Component) error

	// GetByID retrieves a component by its database ID
	GetByID(ctx context.Context, id int64) (*component.Component, error)

	// GetByName retrieves a component by kind and name (unique key)
	GetByName(ctx context.Context, kind component.ComponentKind, name string) (*component.Component, error)

	// List returns all components of a specific kind
	List(ctx context.Context, kind component.ComponentKind) ([]*component.Component, error)

	// ListAll returns all components grouped by kind
	ListAll(ctx context.Context) (map[component.ComponentKind][]*component.Component, error)

	// ListByStatus returns components filtered by status
	ListByStatus(ctx context.Context, kind component.ComponentKind, status component.ComponentStatus) ([]*component.Component, error)

	// Update updates a component's metadata
	Update(ctx context.Context, comp *component.Component) error

	// UpdateStatus updates status, pid, port, and timestamps
	UpdateStatus(ctx context.Context, id int64, status component.ComponentStatus, pid, port int) error

	// Delete removes a component by kind and name
	Delete(ctx context.Context, kind component.ComponentKind, name string) error
}

// componentDAO implements ComponentDAO
type componentDAO struct {
	db *DB
}

// NewComponentDAO creates a new component DAO
func NewComponentDAO(db *DB) ComponentDAO {
	return &componentDAO{db: db}
}

// Create inserts a new component
func (d *componentDAO) Create(ctx context.Context, comp *component.Component) error {
	// Serialize manifest to JSON if present
	var manifestJSON []byte
	var err error
	if comp.Manifest != nil {
		manifestJSON, err = json.Marshal(comp.Manifest)
		if err != nil {
			return fmt.Errorf("failed to marshal manifest: %w", err)
		}
	}

	query := `
		INSERT INTO components (
			kind, name, version, repo_path, bin_path, source, status, manifest,
			pid, port, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`

	result, err := d.db.conn.ExecContext(
		ctx, query,
		comp.Kind,
		comp.Name,
		comp.Version,
		comp.RepoPath,
		comp.BinPath,
		comp.Source,
		comp.Status,
		string(manifestJSON),
		comp.PID,
		comp.Port,
	)

	if err != nil {
		return fmt.Errorf("failed to create component: %w", err)
	}

	// Set the ID on the component
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}
	comp.ID = id

	return nil
}

// GetByID retrieves a component by its database ID
func (d *componentDAO) GetByID(ctx context.Context, id int64) (*component.Component, error) {
	query := `
		SELECT
			id, kind, name, version, repo_path, bin_path, source, status, manifest,
			pid, port, created_at, updated_at, started_at, stopped_at
		FROM components
		WHERE id = ?
	`

	var comp component.Component
	var manifestJSON sql.NullString
	var startedAt, stoppedAt sql.NullTime

	err := d.db.conn.QueryRowContext(ctx, query, id).Scan(
		&comp.ID,
		&comp.Kind,
		&comp.Name,
		&comp.Version,
		&comp.RepoPath,
		&comp.BinPath,
		&comp.Source,
		&comp.Status,
		&manifestJSON,
		&comp.PID,
		&comp.Port,
		&comp.CreatedAt,
		&comp.UpdatedAt,
		&startedAt,
		&stoppedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Unmarshal manifest if present
	if manifestJSON.Valid && manifestJSON.String != "" {
		var manifest component.Manifest
		if err := json.Unmarshal([]byte(manifestJSON.String), &manifest); err != nil {
			return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
		}
		comp.Manifest = &manifest
	}

	// Handle nullable timestamps
	if startedAt.Valid {
		comp.StartedAt = &startedAt.Time
	}
	if stoppedAt.Valid {
		comp.StoppedAt = &stoppedAt.Time
	}

	return &comp, nil
}

// GetByName retrieves a component by kind and name (unique key)
func (d *componentDAO) GetByName(ctx context.Context, kind component.ComponentKind, name string) (*component.Component, error) {
	query := `
		SELECT
			id, kind, name, version, repo_path, bin_path, source, status, manifest,
			pid, port, created_at, updated_at, started_at, stopped_at
		FROM components
		WHERE kind = ? AND name = ?
	`

	var comp component.Component
	var manifestJSON sql.NullString
	var startedAt, stoppedAt sql.NullTime

	err := d.db.conn.QueryRowContext(ctx, query, kind, name).Scan(
		&comp.ID,
		&comp.Kind,
		&comp.Name,
		&comp.Version,
		&comp.RepoPath,
		&comp.BinPath,
		&comp.Source,
		&comp.Status,
		&manifestJSON,
		&comp.PID,
		&comp.Port,
		&comp.CreatedAt,
		&comp.UpdatedAt,
		&startedAt,
		&stoppedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Unmarshal manifest if present
	if manifestJSON.Valid && manifestJSON.String != "" {
		var manifest component.Manifest
		if err := json.Unmarshal([]byte(manifestJSON.String), &manifest); err != nil {
			return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
		}
		comp.Manifest = &manifest
	}

	// Handle nullable timestamps
	if startedAt.Valid {
		comp.StartedAt = &startedAt.Time
	}
	if stoppedAt.Valid {
		comp.StoppedAt = &stoppedAt.Time
	}

	return &comp, nil
}

// List returns all components of a specific kind
func (d *componentDAO) List(ctx context.Context, kind component.ComponentKind) ([]*component.Component, error) {
	query := `
		SELECT
			id, kind, name, version, repo_path, bin_path, source, status, manifest,
			pid, port, created_at, updated_at, started_at, stopped_at
		FROM components
		WHERE kind = ?
		ORDER BY name ASC
	`

	rows, err := d.db.conn.QueryContext(ctx, query, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to list components: %w", err)
	}
	defer rows.Close()

	var components []*component.Component
	for rows.Next() {
		var comp component.Component
		var manifestJSON sql.NullString
		var startedAt, stoppedAt sql.NullTime

		err := rows.Scan(
			&comp.ID,
			&comp.Kind,
			&comp.Name,
			&comp.Version,
			&comp.RepoPath,
			&comp.BinPath,
			&comp.Source,
			&comp.Status,
			&manifestJSON,
			&comp.PID,
			&comp.Port,
			&comp.CreatedAt,
			&comp.UpdatedAt,
			&startedAt,
			&stoppedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan component: %w", err)
		}

		// Unmarshal manifest if present
		if manifestJSON.Valid && manifestJSON.String != "" {
			var manifest component.Manifest
			if err := json.Unmarshal([]byte(manifestJSON.String), &manifest); err != nil {
				return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
			}
			comp.Manifest = &manifest
		}

		// Handle nullable timestamps
		if startedAt.Valid {
			comp.StartedAt = &startedAt.Time
		}
		if stoppedAt.Valid {
			comp.StoppedAt = &stoppedAt.Time
		}

		components = append(components, &comp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating components: %w", err)
	}

	return components, nil
}

// ListAll returns all components grouped by kind
func (d *componentDAO) ListAll(ctx context.Context) (map[component.ComponentKind][]*component.Component, error) {
	query := `
		SELECT
			id, kind, name, version, repo_path, bin_path, source, status, manifest,
			pid, port, created_at, updated_at, started_at, stopped_at
		FROM components
		ORDER BY kind ASC, name ASC
	`

	rows, err := d.db.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list all components: %w", err)
	}
	defer rows.Close()

	result := make(map[component.ComponentKind][]*component.Component)

	for rows.Next() {
		var comp component.Component
		var manifestJSON sql.NullString
		var startedAt, stoppedAt sql.NullTime

		err := rows.Scan(
			&comp.ID,
			&comp.Kind,
			&comp.Name,
			&comp.Version,
			&comp.RepoPath,
			&comp.BinPath,
			&comp.Source,
			&comp.Status,
			&manifestJSON,
			&comp.PID,
			&comp.Port,
			&comp.CreatedAt,
			&comp.UpdatedAt,
			&startedAt,
			&stoppedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan component: %w", err)
		}

		// Unmarshal manifest if present
		if manifestJSON.Valid && manifestJSON.String != "" {
			var manifest component.Manifest
			if err := json.Unmarshal([]byte(manifestJSON.String), &manifest); err != nil {
				return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
			}
			comp.Manifest = &manifest
		}

		// Handle nullable timestamps
		if startedAt.Valid {
			comp.StartedAt = &startedAt.Time
		}
		if stoppedAt.Valid {
			comp.StoppedAt = &stoppedAt.Time
		}

		result[comp.Kind] = append(result[comp.Kind], &comp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating components: %w", err)
	}

	return result, nil
}

// ListByStatus returns components filtered by status
func (d *componentDAO) ListByStatus(ctx context.Context, kind component.ComponentKind, status component.ComponentStatus) ([]*component.Component, error) {
	query := `
		SELECT
			id, kind, name, version, repo_path, bin_path, source, status, manifest,
			pid, port, created_at, updated_at, started_at, stopped_at
		FROM components
		WHERE kind = ? AND status = ?
		ORDER BY name ASC
	`

	rows, err := d.db.conn.QueryContext(ctx, query, kind, status)
	if err != nil {
		return nil, fmt.Errorf("failed to list components by status: %w", err)
	}
	defer rows.Close()

	var components []*component.Component
	for rows.Next() {
		var comp component.Component
		var manifestJSON sql.NullString
		var startedAt, stoppedAt sql.NullTime

		err := rows.Scan(
			&comp.ID,
			&comp.Kind,
			&comp.Name,
			&comp.Version,
			&comp.RepoPath,
			&comp.BinPath,
			&comp.Source,
			&comp.Status,
			&manifestJSON,
			&comp.PID,
			&comp.Port,
			&comp.CreatedAt,
			&comp.UpdatedAt,
			&startedAt,
			&stoppedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan component: %w", err)
		}

		// Unmarshal manifest if present
		if manifestJSON.Valid && manifestJSON.String != "" {
			var manifest component.Manifest
			if err := json.Unmarshal([]byte(manifestJSON.String), &manifest); err != nil {
				return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
			}
			comp.Manifest = &manifest
		}

		// Handle nullable timestamps
		if startedAt.Valid {
			comp.StartedAt = &startedAt.Time
		}
		if stoppedAt.Valid {
			comp.StoppedAt = &stoppedAt.Time
		}

		components = append(components, &comp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating components: %w", err)
	}

	return components, nil
}

// Update updates a component's metadata
func (d *componentDAO) Update(ctx context.Context, comp *component.Component) error {
	// Serialize manifest to JSON if present
	var manifestJSON []byte
	var err error
	if comp.Manifest != nil {
		manifestJSON, err = json.Marshal(comp.Manifest)
		if err != nil {
			return fmt.Errorf("failed to marshal manifest: %w", err)
		}
	}

	query := `
		UPDATE components
		SET name = ?, version = ?, repo_path = ?, bin_path = ?, source = ?,
		    status = ?, manifest = ?, pid = ?, port = ?,
		    updated_at = CURRENT_TIMESTAMP,
		    started_at = ?, stopped_at = ?
		WHERE id = ?
	`

	result, err := d.db.conn.ExecContext(
		ctx, query,
		comp.Name,
		comp.Version,
		comp.RepoPath,
		comp.BinPath,
		comp.Source,
		comp.Status,
		string(manifestJSON),
		comp.PID,
		comp.Port,
		comp.StartedAt,
		comp.StoppedAt,
		comp.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update component: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("component not found: %d", comp.ID)
	}

	return nil
}

// UpdateStatus updates status, pid, port, and timestamps
func (d *componentDAO) UpdateStatus(ctx context.Context, id int64, status component.ComponentStatus, pid, port int) error {
	now := time.Now()
	var startedAt, stoppedAt interface{}

	// Set started_at when transitioning to running
	if status == component.ComponentStatusRunning {
		startedAt = now
		stoppedAt = nil
	}

	// Set stopped_at when transitioning to stopped
	if status == component.ComponentStatusStopped {
		startedAt = nil
		stoppedAt = now
	}

	query := `
		UPDATE components
		SET status = ?, pid = ?, port = ?,
		    updated_at = CURRENT_TIMESTAMP,
		    started_at = COALESCE(?, started_at),
		    stopped_at = COALESCE(?, stopped_at)
		WHERE id = ?
	`

	result, err := d.db.conn.ExecContext(
		ctx, query,
		status,
		pid,
		port,
		startedAt,
		stoppedAt,
		id,
	)

	if err != nil {
		return fmt.Errorf("failed to update component status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("component not found: %d", id)
	}

	return nil
}

// Delete removes a component by kind and name
func (d *componentDAO) Delete(ctx context.Context, kind component.ComponentKind, name string) error {
	query := `DELETE FROM components WHERE kind = ? AND name = ?`

	result, err := d.db.conn.ExecContext(ctx, query, kind, name)
	if err != nil {
		return fmt.Errorf("failed to delete component: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("component not found: %s %s", kind, name)
	}

	return nil
}
