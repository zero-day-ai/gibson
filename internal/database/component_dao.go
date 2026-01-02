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

	// Update updates a component's metadata
	Update(ctx context.Context, comp *component.Component) error

	// UpdateStatus is deprecated and does nothing - runtime state is tracked via process checking
	// This method is kept for backward compatibility and will be removed when StatusUpdater interface is removed
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
			kind, name, version, repo_path, bin_path, source, manifest,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`

	result, err := d.db.conn.ExecContext(
		ctx, query,
		comp.Kind,
		comp.Name,
		comp.Version,
		comp.RepoPath,
		comp.BinPath,
		comp.Source,
		string(manifestJSON),
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

	comp, err := d.scanComponent(d.db.conn.QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	return comp, nil
}

// scanComponent scans a single row into a Component struct
func (d *componentDAO) scanComponent(row *sql.Row) (*component.Component, error) {
	var comp component.Component
	var manifestJSON sql.NullString
	var pid, port sql.NullInt64
	var startedAt, stoppedAt sql.NullTime

	err := row.Scan(
		&comp.ID,
		&comp.Kind,
		&comp.Name,
		&comp.Version,
		&comp.RepoPath,
		&comp.BinPath,
		&comp.Source,
		&comp.Status,
		&manifestJSON,
		&pid,
		&port,
		&comp.CreatedAt,
		&comp.UpdatedAt,
		&startedAt,
		&stoppedAt,
	)
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if pid.Valid {
		comp.PID = int(pid.Int64)
	}
	if port.Valid {
		comp.Port = int(port.Int64)
	}
	if startedAt.Valid {
		comp.StartedAt = &startedAt.Time
	}
	if stoppedAt.Valid {
		comp.StoppedAt = &stoppedAt.Time
	}

	// Unmarshal manifest if present
	if manifestJSON.Valid && manifestJSON.String != "" {
		var manifest component.Manifest
		if err := json.Unmarshal([]byte(manifestJSON.String), &manifest); err != nil {
			return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
		}
		comp.Manifest = &manifest
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

	comp, err := d.scanComponent(d.db.conn.QueryRowContext(ctx, query, kind, name))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	return comp, nil
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

	return d.scanComponents(rows)
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

	components, err := d.scanComponents(rows)
	if err != nil {
		return nil, err
	}

	result := make(map[component.ComponentKind][]*component.Component)
	for _, comp := range components {
		result[comp.Kind] = append(result[comp.Kind], comp)
	}

	return result, nil
}

// scanComponents scans multiple rows into Component slices
func (d *componentDAO) scanComponents(rows *sql.Rows) ([]*component.Component, error) {
	var components []*component.Component

	for rows.Next() {
		var comp component.Component
		var manifestJSON sql.NullString
		var pid, port sql.NullInt64
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
			&pid,
			&port,
			&comp.CreatedAt,
			&comp.UpdatedAt,
			&startedAt,
			&stoppedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan component: %w", err)
		}

		// Handle nullable fields
		if pid.Valid {
			comp.PID = int(pid.Int64)
		}
		if port.Valid {
			comp.Port = int(port.Int64)
		}
		if startedAt.Valid {
			comp.StartedAt = &startedAt.Time
		}
		if stoppedAt.Valid {
			comp.StoppedAt = &stoppedAt.Time
		}

		// Unmarshal manifest if present
		if manifestJSON.Valid && manifestJSON.String != "" {
			var manifest component.Manifest
			if err := json.Unmarshal([]byte(manifestJSON.String), &manifest); err != nil {
				return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
			}
			comp.Manifest = &manifest
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
		    manifest = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	result, err := d.db.conn.ExecContext(
		ctx, query,
		comp.Name,
		comp.Version,
		comp.RepoPath,
		comp.BinPath,
		comp.Source,
		string(manifestJSON),
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

// UpdateStatus updates the component's runtime status in the database.
func (d *componentDAO) UpdateStatus(ctx context.Context, id int64, status component.ComponentStatus, pid, port int) error {
	now := time.Now()
	var query string
	var args []interface{}

	if status == component.ComponentStatusRunning {
		query = `UPDATE components SET status = ?, pid = ?, port = ?, started_at = ?, updated_at = ? WHERE id = ?`
		args = []interface{}{status, pid, port, now, now, id}
	} else if status == component.ComponentStatusStopped {
		query = `UPDATE components SET status = ?, pid = NULL, port = NULL, stopped_at = ?, updated_at = ? WHERE id = ?`
		args = []interface{}{status, now, now, id}
	} else {
		query = `UPDATE components SET status = ?, pid = ?, port = ?, updated_at = ? WHERE id = ?`
		args = []interface{}{status, pid, port, now, id}
	}

	_, err := d.db.conn.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update component status: %w", err)
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
