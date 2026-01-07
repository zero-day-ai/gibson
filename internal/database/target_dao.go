package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// TargetDAO provides database access for Target entities
type TargetDAO struct {
	db *DB
}

// NewTargetDAO creates a new TargetDAO instance
func NewTargetDAO(db *DB) *TargetDAO {
	return &TargetDAO{db: db}
}

// Create inserts a new target into the database
func (dao *TargetDAO) Create(ctx context.Context, target *types.Target) error {
	if err := target.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Marshal JSON fields
	headersJSON, err := json.Marshal(target.Headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}

	configJSON, err := json.Marshal(target.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	capabilitiesJSON, err := json.Marshal(target.Capabilities)
	if err != nil {
		return fmt.Errorf("failed to marshal capabilities: %w", err)
	}

	tagsJSON, err := json.Marshal(target.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	query := `
		INSERT INTO targets (
			id, name, type, provider, url, model,
			headers, config, capabilities,
			auth_type, credential_id,
			status, description, tags, timeout,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = dao.db.ExecContext(ctx, query,
		target.ID.String(),
		target.Name,
		target.Type, // Type is now string, not enum
		target.Provider.String(),
		target.URL,
		target.Model,
		string(headersJSON),
		string(configJSON),
		string(capabilitiesJSON),
		target.AuthType.String(),
		nullableID(target.CredentialID),
		target.Status.String(),
		target.Description,
		string(tagsJSON),
		target.Timeout,
		target.CreatedAt,
		target.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to insert target: %w", err)
	}

	return nil
}

// Get retrieves a target by ID
func (dao *TargetDAO) Get(ctx context.Context, id types.ID) (*types.Target, error) {
	query := `
		SELECT
			id, name, type, provider, url, model,
			headers, config, capabilities,
			auth_type, credential_id,
			status, description, tags, timeout,
			created_at, updated_at
		FROM targets
		WHERE id = ?
	`

	row := dao.db.QueryRowContext(ctx, query, id.String())
	return dao.scanTarget(row)
}

// GetByName retrieves a target by name
func (dao *TargetDAO) GetByName(ctx context.Context, name string) (*types.Target, error) {
	query := `
		SELECT
			id, name, type, provider, url, model,
			headers, config, capabilities,
			auth_type, credential_id,
			status, description, tags, timeout,
			created_at, updated_at
		FROM targets
		WHERE name = ?
	`

	row := dao.db.QueryRowContext(ctx, query, name)
	return dao.scanTarget(row)
}

// List retrieves targets with optional filtering
func (dao *TargetDAO) List(ctx context.Context, filter *types.TargetFilter) ([]*types.Target, error) {
	if filter == nil {
		filter = types.NewTargetFilter()
	}

	// Build query with filters
	query := `
		SELECT
			id, name, type, provider, url, model,
			headers, config, capabilities,
			auth_type, credential_id,
			status, description, tags, timeout,
			created_at, updated_at
		FROM targets
		WHERE 1=1
	`

	var args []interface{}
	var conditions []string

	if filter.Provider != nil {
		conditions = append(conditions, "provider = ?")
		args = append(args, filter.Provider.String())
	}

	if filter.Type != nil {
		conditions = append(conditions, "type = ?")
		args = append(args, *filter.Type) // Type is now *string, not *TargetType
	}

	if filter.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, filter.Status.String())
	}

	if len(conditions) > 0 {
		query += " AND " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, filter.Limit, filter.Offset)

	rows, err := dao.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query targets: %w", err)
	}
	defer rows.Close()

	var targets []*types.Target
	for rows.Next() {
		target, err := dao.scanTargetFromRows(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating targets: %w", err)
	}

	return targets, nil
}

// Update updates an existing target
func (dao *TargetDAO) Update(ctx context.Context, target *types.Target) error {
	if err := target.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Update the timestamp
	target.UpdatedAt = time.Now()

	// Marshal JSON fields
	headersJSON, err := json.Marshal(target.Headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}

	configJSON, err := json.Marshal(target.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	capabilitiesJSON, err := json.Marshal(target.Capabilities)
	if err != nil {
		return fmt.Errorf("failed to marshal capabilities: %w", err)
	}

	tagsJSON, err := json.Marshal(target.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	query := `
		UPDATE targets SET
			name = ?,
			type = ?,
			provider = ?,
			url = ?,
			model = ?,
			headers = ?,
			config = ?,
			capabilities = ?,
			auth_type = ?,
			credential_id = ?,
			status = ?,
			description = ?,
			tags = ?,
			timeout = ?,
			updated_at = ?
		WHERE id = ?
	`

	result, err := dao.db.ExecContext(ctx, query,
		target.Name,
		target.Type, // Type is now string, not enum
		target.Provider.String(),
		target.URL,
		target.Model,
		string(headersJSON),
		string(configJSON),
		string(capabilitiesJSON),
		target.AuthType.String(),
		nullableID(target.CredentialID),
		target.Status.String(),
		target.Description,
		string(tagsJSON),
		target.Timeout,
		target.UpdatedAt,
		target.ID.String(),
	)

	if err != nil {
		return fmt.Errorf("failed to update target: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("target not found: %s", target.ID)
	}

	return nil
}

// Delete removes a target from the database
func (dao *TargetDAO) Delete(ctx context.Context, id types.ID) error {
	query := "DELETE FROM targets WHERE id = ?"
	result, err := dao.db.ExecContext(ctx, query, id.String())
	if err != nil {
		return fmt.Errorf("failed to delete target: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("target not found: %s", id)
	}

	return nil
}

// Exists checks if a target exists by ID
func (dao *TargetDAO) Exists(ctx context.Context, id types.ID) (bool, error) {
	query := "SELECT COUNT(*) FROM targets WHERE id = ?"
	var count int
	err := dao.db.QueryRowContext(ctx, query, id.String()).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check target existence: %w", err)
	}
	return count > 0, nil
}

// ExistsByName checks if a target exists by name
func (dao *TargetDAO) ExistsByName(ctx context.Context, name string) (bool, error) {
	query := "SELECT COUNT(*) FROM targets WHERE name = ?"
	var count int
	err := dao.db.QueryRowContext(ctx, query, name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check target name existence: %w", err)
	}
	return count > 0, nil
}

// scanTarget scans a single target from a query row
func (dao *TargetDAO) scanTarget(row *sql.Row) (*types.Target, error) {
	var target types.Target
	var id, provider, authType, status, targetType string
	var credentialID sql.NullString
	var headersJSON, configJSON, capabilitiesJSON, tagsJSON string

	err := row.Scan(
		&id,
		&target.Name,
		&targetType,
		&provider,
		&target.URL,
		&target.Model,
		&headersJSON,
		&configJSON,
		&capabilitiesJSON,
		&authType,
		&credentialID,
		&status,
		&target.Description,
		&tagsJSON,
		&target.Timeout,
		&target.CreatedAt,
		&target.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("target not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan target: %w", err)
	}

	// Parse ID
	target.ID, err = types.ParseID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target ID: %w", err)
	}

	// Parse enums (Type is now just a string, not an enum)
	target.Type = targetType
	target.Provider = types.Provider(provider)
	target.AuthType = types.AuthType(authType)
	target.Status = types.TargetStatus(status)

	// Parse credential ID
	if credentialID.Valid {
		cid, err := types.ParseID(credentialID.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse credential ID: %w", err)
		}
		target.CredentialID = &cid
	}

	// Unmarshal JSON fields
	if err := json.Unmarshal([]byte(headersJSON), &target.Headers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal headers: %w", err)
	}

	if err := json.Unmarshal([]byte(configJSON), &target.Config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := json.Unmarshal([]byte(capabilitiesJSON), &target.Capabilities); err != nil {
		return nil, fmt.Errorf("failed to unmarshal capabilities: %w", err)
	}

	if err := json.Unmarshal([]byte(tagsJSON), &target.Tags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	return &target, nil
}

// scanTargetFromRows scans a target from sql.Rows
func (dao *TargetDAO) scanTargetFromRows(rows *sql.Rows) (*types.Target, error) {
	var target types.Target
	var id, provider, authType, status, targetType string
	var credentialID sql.NullString
	var headersJSON, configJSON, capabilitiesJSON, tagsJSON string

	err := rows.Scan(
		&id,
		&target.Name,
		&targetType,
		&provider,
		&target.URL,
		&target.Model,
		&headersJSON,
		&configJSON,
		&capabilitiesJSON,
		&authType,
		&credentialID,
		&status,
		&target.Description,
		&tagsJSON,
		&target.Timeout,
		&target.CreatedAt,
		&target.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scan target: %w", err)
	}

	// Parse ID
	target.ID, err = types.ParseID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target ID: %w", err)
	}

	// Parse enums (Type is now just a string, not an enum)
	target.Type = targetType
	target.Provider = types.Provider(provider)
	target.AuthType = types.AuthType(authType)
	target.Status = types.TargetStatus(status)

	// Parse credential ID
	if credentialID.Valid {
		cid, err := types.ParseID(credentialID.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse credential ID: %w", err)
		}
		target.CredentialID = &cid
	}

	// Unmarshal JSON fields
	if err := json.Unmarshal([]byte(headersJSON), &target.Headers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal headers: %w", err)
	}

	if err := json.Unmarshal([]byte(configJSON), &target.Config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := json.Unmarshal([]byte(capabilitiesJSON), &target.Capabilities); err != nil {
		return nil, fmt.Errorf("failed to unmarshal capabilities: %w", err)
	}

	if err := json.Unmarshal([]byte(tagsJSON), &target.Tags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	return &target, nil
}

// nullableID converts a pointer to ID to sql.NullString
func nullableID(id *types.ID) sql.NullString {
	if id == nil {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: id.String(), Valid: true}
}
