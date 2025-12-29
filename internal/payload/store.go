package payload

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// PayloadStore provides database access for Payload entities
type PayloadStore interface {
	// Save inserts a new payload with version tracking
	Save(ctx context.Context, payload *Payload) error

	// Get retrieves a payload by ID
	Get(ctx context.Context, id types.ID) (*Payload, error)

	// List retrieves payloads with optional filtering
	List(ctx context.Context, filter *PayloadFilter) ([]*Payload, error)

	// Search performs full-text search on payloads using FTS5
	Search(ctx context.Context, query string, filter *PayloadFilter) ([]*Payload, error)

	// Update modifies an existing payload and increments version
	Update(ctx context.Context, payload *Payload) error

	// Delete soft-deletes a payload by disabling it
	Delete(ctx context.Context, id types.ID) error

	// GetVersionHistory retrieves all versions of a payload
	GetVersionHistory(ctx context.Context, id types.ID) ([]*PayloadVersion, error)

	// Exists checks if a payload exists by ID
	Exists(ctx context.Context, id types.ID) (bool, error)

	// ExistsByName checks if a payload exists by name
	ExistsByName(ctx context.Context, name string) (bool, error)

	// Count returns the total number of payloads matching the filter
	Count(ctx context.Context, filter *PayloadFilter) (int, error)
}

// PayloadVersion represents a historical version of a payload
type PayloadVersion struct {
	ID            types.ID  `json:"id"`
	PayloadID     types.ID  `json:"payload_id"`
	Version       string    `json:"version"`
	Payload       Payload   `json:"payload"`
	ChangeType    string    `json:"change_type"` // 'created', 'updated', 'disabled', 'enabled'
	ChangeSummary string    `json:"change_summary,omitempty"`
	ChangedBy     string    `json:"changed_by,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// payloadStore implements PayloadStore interface
type payloadStore struct {
	db *database.DB
}

// NewPayloadStore creates a new PayloadStore instance
func NewPayloadStore(db *database.DB) PayloadStore {
	return &payloadStore{db: db}
}

// Save inserts a new payload into the database with version tracking
func (s *payloadStore) Save(ctx context.Context, payload *Payload) error {
	if payload == nil {
		return fmt.Errorf("payload cannot be nil")
	}

	if err := payload.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		// Marshal JSON fields
		categoriesJSON, err := json.Marshal(payload.Categories)
		if err != nil {
			return fmt.Errorf("failed to marshal categories: %w", err)
		}

		tagsJSON, err := json.Marshal(payload.Tags)
		if err != nil {
			return fmt.Errorf("failed to marshal tags: %w", err)
		}

		parametersJSON, err := json.Marshal(payload.Parameters)
		if err != nil {
			return fmt.Errorf("failed to marshal parameters: %w", err)
		}

		indicatorsJSON, err := json.Marshal(payload.SuccessIndicators)
		if err != nil {
			return fmt.Errorf("failed to marshal success indicators: %w", err)
		}

		targetTypesJSON, err := json.Marshal(payload.TargetTypes)
		if err != nil {
			return fmt.Errorf("failed to marshal target types: %w", err)
		}

		mitreTechniquesJSON, err := json.Marshal(payload.MitreTechniques)
		if err != nil {
			return fmt.Errorf("failed to marshal mitre techniques: %w", err)
		}

		metadataJSON, err := json.Marshal(payload.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}

		// Insert payload
		query := `
			INSERT INTO payloads (
				id, name, version, description,
				categories, tags, template,
				parameters, success_indicators,
				target_types, severity, mitre_techniques,
				metadata, built_in, enabled,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`

		_, err = tx.ExecContext(ctx, query,
			payload.ID.String(),
			payload.Name,
			payload.Version,
			payload.Description,
			string(categoriesJSON),
			string(tagsJSON),
			payload.Template,
			string(parametersJSON),
			string(indicatorsJSON),
			string(targetTypesJSON),
			string(payload.Severity),
			string(mitreTechniquesJSON),
			string(metadataJSON),
			payload.BuiltIn,
			payload.Enabled,
			payload.CreatedAt,
			payload.UpdatedAt,
		)

		if err != nil {
			return fmt.Errorf("failed to insert payload: %w", err)
		}

		// Create initial version record
		if err := s.saveVersionTx(ctx, tx, payload, "created", ""); err != nil {
			return fmt.Errorf("failed to save version: %w", err)
		}

		return nil
	})
}

// Get retrieves a payload by ID
func (s *payloadStore) Get(ctx context.Context, id types.ID) (*Payload, error) {
	query := `
		SELECT
			id, name, version, description,
			categories, tags, template,
			parameters, success_indicators,
			target_types, severity, mitre_techniques,
			metadata, built_in, enabled,
			created_at, updated_at
		FROM payloads
		WHERE id = ?
	`

	row := s.db.QueryRowContext(ctx, query, id.String())
	return s.scanPayload(row)
}

// List retrieves payloads with optional filtering
func (s *payloadStore) List(ctx context.Context, filter *PayloadFilter) ([]*Payload, error) {
	if filter == nil {
		filter = &PayloadFilter{
			Limit:  100,
			Offset: 0,
		}
	}

	// Set default limit if not specified
	if filter.Limit == 0 {
		filter.Limit = 100
	}

	// Build query with filters
	query := `
		SELECT
			id, name, version, description,
			categories, tags, template,
			parameters, success_indicators,
			target_types, severity, mitre_techniques,
			metadata, built_in, enabled,
			created_at, updated_at
		FROM payloads
		WHERE 1=1
	`

	var args []interface{}
	var conditions []string

	// Apply filters
	if len(filter.IDs) > 0 {
		placeholders := make([]string, len(filter.IDs))
		for i, id := range filter.IDs {
			placeholders[i] = "?"
			args = append(args, id.String())
		}
		conditions = append(conditions, fmt.Sprintf("id IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(filter.Categories) > 0 {
		// Categories is a JSON array, so we need to check if any category matches
		categoryConditions := make([]string, len(filter.Categories))
		for i, cat := range filter.Categories {
			categoryConditions[i] = "categories LIKE ?"
			args = append(args, fmt.Sprintf("%%\"%s\"%%", cat.String()))
		}
		conditions = append(conditions, fmt.Sprintf("(%s)", strings.Join(categoryConditions, " OR ")))
	}

	if len(filter.Tags) > 0 {
		// Tags is a JSON array, check if any tag matches
		tagConditions := make([]string, len(filter.Tags))
		for i, tag := range filter.Tags {
			tagConditions[i] = "tags LIKE ?"
			args = append(args, fmt.Sprintf("%%\"%s\"%%", tag))
		}
		conditions = append(conditions, fmt.Sprintf("(%s)", strings.Join(tagConditions, " OR ")))
	}

	if len(filter.TargetTypes) > 0 {
		// TargetTypes is a JSON array, check if any target type matches
		targetConditions := make([]string, len(filter.TargetTypes))
		for i, tt := range filter.TargetTypes {
			targetConditions[i] = "target_types LIKE ?"
			args = append(args, fmt.Sprintf("%%\"%s\"%%", tt))
		}
		conditions = append(conditions, fmt.Sprintf("(%s)", strings.Join(targetConditions, " OR ")))
	}

	if len(filter.Severities) > 0 {
		placeholders := make([]string, len(filter.Severities))
		for i, severity := range filter.Severities {
			placeholders[i] = "?"
			args = append(args, string(severity))
		}
		conditions = append(conditions, fmt.Sprintf("severity IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(filter.MitreTechniques) > 0 {
		// MitreTechniques is a JSON array, check if any technique matches
		techniqueConditions := make([]string, len(filter.MitreTechniques))
		for i, tech := range filter.MitreTechniques {
			techniqueConditions[i] = "mitre_techniques LIKE ?"
			args = append(args, fmt.Sprintf("%%\"%s\"%%", tech))
		}
		conditions = append(conditions, fmt.Sprintf("(%s)", strings.Join(techniqueConditions, " OR ")))
	}

	if filter.BuiltIn != nil {
		conditions = append(conditions, "built_in = ?")
		args = append(args, *filter.BuiltIn)
	}

	if filter.Enabled != nil {
		conditions = append(conditions, "enabled = ?")
		args = append(args, *filter.Enabled)
	}

	// Add conditions to query
	if len(conditions) > 0 {
		query += " AND " + strings.Join(conditions, " AND ")
	}

	// Add ordering and pagination
	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, filter.Limit, filter.Offset)

	// Execute query
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query payloads: %w", err)
	}
	defer rows.Close()

	var payloads []*Payload
	for rows.Next() {
		payload, err := s.scanPayloadFromRows(rows)
		if err != nil {
			return nil, err
		}
		payloads = append(payloads, payload)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating payloads: %w", err)
	}

	return payloads, nil
}

// Search performs full-text search on payloads using FTS5
func (s *payloadStore) Search(ctx context.Context, searchQuery string, filter *PayloadFilter) ([]*Payload, error) {
	if searchQuery == "" {
		return s.List(ctx, filter)
	}

	if filter == nil {
		filter = &PayloadFilter{
			Limit:  100,
			Offset: 0,
		}
	}

	// Set default limit if not specified
	if filter.Limit == 0 {
		filter.Limit = 100
	}

	// Build FTS query
	query := `
		SELECT
			p.id, p.name, p.version, p.description,
			p.categories, p.tags, p.template,
			p.parameters, p.success_indicators,
			p.target_types, p.severity, p.mitre_techniques,
			p.metadata, p.built_in, p.enabled,
			p.created_at, p.updated_at
		FROM payloads p
		JOIN payloads_fts fts ON p.rowid = fts.rowid
		WHERE payloads_fts MATCH ?
	`

	var args []interface{}
	args = append(args, searchQuery)

	var conditions []string

	// Apply additional filters
	if len(filter.IDs) > 0 {
		placeholders := make([]string, len(filter.IDs))
		for i, id := range filter.IDs {
			placeholders[i] = "?"
			args = append(args, id.String())
		}
		conditions = append(conditions, fmt.Sprintf("p.id IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(filter.Categories) > 0 {
		categoryConditions := make([]string, len(filter.Categories))
		for i, cat := range filter.Categories {
			categoryConditions[i] = "p.categories LIKE ?"
			args = append(args, fmt.Sprintf("%%\"%s\"%%", cat.String()))
		}
		conditions = append(conditions, fmt.Sprintf("(%s)", strings.Join(categoryConditions, " OR ")))
	}

	if len(filter.Severities) > 0 {
		placeholders := make([]string, len(filter.Severities))
		for i, severity := range filter.Severities {
			placeholders[i] = "?"
			args = append(args, string(severity))
		}
		conditions = append(conditions, fmt.Sprintf("p.severity IN (%s)", strings.Join(placeholders, ",")))
	}

	if filter.BuiltIn != nil {
		conditions = append(conditions, "p.built_in = ?")
		args = append(args, *filter.BuiltIn)
	}

	if filter.Enabled != nil {
		conditions = append(conditions, "p.enabled = ?")
		args = append(args, *filter.Enabled)
	}

	// Add conditions to query
	if len(conditions) > 0 {
		query += " AND " + strings.Join(conditions, " AND ")
	}

	// Add ordering by rank and pagination
	query += " ORDER BY rank LIMIT ? OFFSET ?"
	args = append(args, filter.Limit, filter.Offset)

	// Execute query
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute FTS query: %w", err)
	}
	defer rows.Close()

	var payloads []*Payload
	for rows.Next() {
		payload, err := s.scanPayloadFromRows(rows)
		if err != nil {
			return nil, err
		}
		payloads = append(payloads, payload)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating search results: %w", err)
	}

	return payloads, nil
}

// Update modifies an existing payload and increments version
func (s *payloadStore) Update(ctx context.Context, payload *Payload) error {
	if err := payload.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		// Get current version to save to history
		var currentPayload Payload
		var categoriesJSON, tagsJSON, parametersJSON, indicatorsJSON string
		var targetTypesJSON, mitreTechniquesJSON, metadataJSON string

		query := `
			SELECT
				id, name, version, description,
				categories, tags, template,
				parameters, success_indicators,
				target_types, severity, mitre_techniques,
				metadata, built_in, enabled,
				created_at, updated_at
			FROM payloads
			WHERE id = ?
		`

		err := tx.QueryRowContext(ctx, query, payload.ID.String()).Scan(
			&currentPayload.ID,
			&currentPayload.Name,
			&currentPayload.Version,
			&currentPayload.Description,
			&categoriesJSON,
			&tagsJSON,
			&currentPayload.Template,
			&parametersJSON,
			&indicatorsJSON,
			&targetTypesJSON,
			&currentPayload.Severity,
			&mitreTechniquesJSON,
			&metadataJSON,
			&currentPayload.BuiltIn,
			&currentPayload.Enabled,
			&currentPayload.CreatedAt,
			&currentPayload.UpdatedAt,
		)

		if err == sql.ErrNoRows {
			return fmt.Errorf("payload not found: %s", payload.ID)
		}
		if err != nil {
			return fmt.Errorf("failed to get current payload: %w", err)
		}

		// Unmarshal current payload for version history
		if err := json.Unmarshal([]byte(categoriesJSON), &currentPayload.Categories); err != nil {
			return fmt.Errorf("failed to unmarshal categories: %w", err)
		}
		if err := json.Unmarshal([]byte(tagsJSON), &currentPayload.Tags); err != nil {
			return fmt.Errorf("failed to unmarshal tags: %w", err)
		}
		if err := json.Unmarshal([]byte(parametersJSON), &currentPayload.Parameters); err != nil {
			return fmt.Errorf("failed to unmarshal parameters: %w", err)
		}
		if err := json.Unmarshal([]byte(indicatorsJSON), &currentPayload.SuccessIndicators); err != nil {
			return fmt.Errorf("failed to unmarshal indicators: %w", err)
		}
		if err := json.Unmarshal([]byte(targetTypesJSON), &currentPayload.TargetTypes); err != nil {
			return fmt.Errorf("failed to unmarshal target types: %w", err)
		}
		if err := json.Unmarshal([]byte(mitreTechniquesJSON), &currentPayload.MitreTechniques); err != nil {
			return fmt.Errorf("failed to unmarshal mitre techniques: %w", err)
		}
		if err := json.Unmarshal([]byte(metadataJSON), &currentPayload.Metadata); err != nil {
			return fmt.Errorf("failed to unmarshal metadata: %w", err)
		}

		// Save current version to history before update
		if err := s.saveVersionTx(ctx, tx, &currentPayload, "updated", ""); err != nil {
			return fmt.Errorf("failed to save version to history: %w", err)
		}

		// Update timestamp
		payload.UpdatedAt = time.Now()

		// Marshal JSON fields for new version
		newCategoriesJSON, err := json.Marshal(payload.Categories)
		if err != nil {
			return fmt.Errorf("failed to marshal categories: %w", err)
		}

		newTagsJSON, err := json.Marshal(payload.Tags)
		if err != nil {
			return fmt.Errorf("failed to marshal tags: %w", err)
		}

		newParametersJSON, err := json.Marshal(payload.Parameters)
		if err != nil {
			return fmt.Errorf("failed to marshal parameters: %w", err)
		}

		newIndicatorsJSON, err := json.Marshal(payload.SuccessIndicators)
		if err != nil {
			return fmt.Errorf("failed to marshal success indicators: %w", err)
		}

		newTargetTypesJSON, err := json.Marshal(payload.TargetTypes)
		if err != nil {
			return fmt.Errorf("failed to marshal target types: %w", err)
		}

		newMitreTechniquesJSON, err := json.Marshal(payload.MitreTechniques)
		if err != nil {
			return fmt.Errorf("failed to marshal mitre techniques: %w", err)
		}

		newMetadataJSON, err := json.Marshal(payload.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}

		// Update payload
		updateQuery := `
			UPDATE payloads SET
				name = ?,
				version = ?,
				description = ?,
				categories = ?,
				tags = ?,
				template = ?,
				parameters = ?,
				success_indicators = ?,
				target_types = ?,
				severity = ?,
				mitre_techniques = ?,
				metadata = ?,
				built_in = ?,
				enabled = ?,
				updated_at = ?
			WHERE id = ?
		`

		result, err := tx.ExecContext(ctx, updateQuery,
			payload.Name,
			payload.Version,
			payload.Description,
			string(newCategoriesJSON),
			string(newTagsJSON),
			payload.Template,
			string(newParametersJSON),
			string(newIndicatorsJSON),
			string(newTargetTypesJSON),
			string(payload.Severity),
			string(newMitreTechniquesJSON),
			string(newMetadataJSON),
			payload.BuiltIn,
			payload.Enabled,
			payload.UpdatedAt,
			payload.ID.String(),
		)

		if err != nil {
			return fmt.Errorf("failed to update payload: %w", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected: %w", err)
		}

		if rowsAffected == 0 {
			return fmt.Errorf("payload not found: %s", payload.ID)
		}

		return nil
	})
}

// Delete soft-deletes a payload by disabling it
func (s *payloadStore) Delete(ctx context.Context, id types.ID) error {
	query := `
		UPDATE payloads
		SET enabled = 0, updated_at = ?
		WHERE id = ?
	`

	result, err := s.db.ExecContext(ctx, query, time.Now(), id.String())
	if err != nil {
		return fmt.Errorf("failed to disable payload: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("payload not found: %s", id)
	}

	return nil
}

// GetVersionHistory retrieves all versions of a payload
func (s *payloadStore) GetVersionHistory(ctx context.Context, id types.ID) ([]*PayloadVersion, error) {
	query := `
		SELECT
			id, payload_id, version, payload_data,
			change_type, change_summary, changed_by, created_at
		FROM payload_versions
		WHERE payload_id = ?
		ORDER BY created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query, id.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query version history: %w", err)
	}
	defer rows.Close()

	var versions []*PayloadVersion
	for rows.Next() {
		var version PayloadVersion
		var payloadIDStr, payloadData string
		var changeSummary, changedBy sql.NullString

		err := rows.Scan(
			&version.ID,
			&payloadIDStr,
			&version.Version,
			&payloadData,
			&version.ChangeType,
			&changeSummary,
			&changedBy,
			&version.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan version: %w", err)
		}

		// Parse payload ID
		version.PayloadID, err = types.ParseID(payloadIDStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse payload ID: %w", err)
		}

		// Unmarshal payload data
		if err := json.Unmarshal([]byte(payloadData), &version.Payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payload data: %w", err)
		}

		if changeSummary.Valid {
			version.ChangeSummary = changeSummary.String
		}
		if changedBy.Valid {
			version.ChangedBy = changedBy.String
		}

		versions = append(versions, &version)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating versions: %w", err)
	}

	return versions, nil
}

// Exists checks if a payload exists by ID
func (s *payloadStore) Exists(ctx context.Context, id types.ID) (bool, error) {
	query := "SELECT COUNT(*) FROM payloads WHERE id = ?"
	var count int
	err := s.db.QueryRowContext(ctx, query, id.String()).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check payload existence: %w", err)
	}
	return count > 0, nil
}

// ExistsByName checks if a payload exists by name
func (s *payloadStore) ExistsByName(ctx context.Context, name string) (bool, error) {
	query := "SELECT COUNT(*) FROM payloads WHERE name = ?"
	var count int
	err := s.db.QueryRowContext(ctx, query, name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check payload name existence: %w", err)
	}
	return count > 0, nil
}

// Count returns the total number of payloads matching the filter
func (s *payloadStore) Count(ctx context.Context, filter *PayloadFilter) (int, error) {
	if filter == nil {
		filter = &PayloadFilter{}
	}

	query := "SELECT COUNT(*) FROM payloads WHERE 1=1"
	var args []interface{}
	var conditions []string

	// Apply same filters as List
	if len(filter.IDs) > 0 {
		placeholders := make([]string, len(filter.IDs))
		for i, id := range filter.IDs {
			placeholders[i] = "?"
			args = append(args, id.String())
		}
		conditions = append(conditions, fmt.Sprintf("id IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(filter.Categories) > 0 {
		categoryConditions := make([]string, len(filter.Categories))
		for i, cat := range filter.Categories {
			categoryConditions[i] = "categories LIKE ?"
			args = append(args, fmt.Sprintf("%%\"%s\"%%", cat.String()))
		}
		conditions = append(conditions, fmt.Sprintf("(%s)", strings.Join(categoryConditions, " OR ")))
	}

	if filter.BuiltIn != nil {
		conditions = append(conditions, "built_in = ?")
		args = append(args, *filter.BuiltIn)
	}

	if filter.Enabled != nil {
		conditions = append(conditions, "enabled = ?")
		args = append(args, *filter.Enabled)
	}

	if len(conditions) > 0 {
		query += " AND " + strings.Join(conditions, " AND ")
	}

	var count int
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count payloads: %w", err)
	}

	return count, nil
}

// saveVersionTx saves a payload version within a transaction
func (s *payloadStore) saveVersionTx(ctx context.Context, tx *sql.Tx, payload *Payload, changeType string, changeSummary string) error {
	// Marshal entire payload to JSON for version storage
	payloadData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload data: %w", err)
	}

	// Get the next version number for this payload
	var versionCount int
	countQuery := "SELECT COUNT(*) FROM payload_versions WHERE payload_id = ?"
	err = tx.QueryRowContext(ctx, countQuery, payload.ID.String()).Scan(&versionCount)
	if err != nil {
		return fmt.Errorf("failed to get version count: %w", err)
	}

	// Generate sequential version number (v1, v2, v3, etc.)
	versionNumber := fmt.Sprintf("v%d", versionCount+1)

	versionID := types.NewID()
	query := `
		INSERT INTO payload_versions (
			id, payload_id, version, payload_data,
			change_type, change_summary, changed_by, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = tx.ExecContext(ctx, query,
		versionID.String(),
		payload.ID.String(),
		versionNumber,
		string(payloadData),
		changeType,
		nullString(changeSummary),
		nil, // changed_by - could be added later
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to insert version: %w", err)
	}

	return nil
}

// scanPayload scans a single payload from a query row
func (s *payloadStore) scanPayload(row *sql.Row) (*Payload, error) {
	var payload Payload
	var id, severity string
	var categoriesJSON, tagsJSON, parametersJSON, indicatorsJSON string
	var targetTypesJSON, mitreTechniquesJSON, metadataJSON string

	err := row.Scan(
		&id,
		&payload.Name,
		&payload.Version,
		&payload.Description,
		&categoriesJSON,
		&tagsJSON,
		&payload.Template,
		&parametersJSON,
		&indicatorsJSON,
		&targetTypesJSON,
		&severity,
		&mitreTechniquesJSON,
		&metadataJSON,
		&payload.BuiltIn,
		&payload.Enabled,
		&payload.CreatedAt,
		&payload.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("payload not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan payload: %w", err)
	}

	// Parse ID
	payload.ID, err = types.ParseID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse payload ID: %w", err)
	}

	// Parse severity enum
	payload.Severity = agent.FindingSeverity(severity)

	// Unmarshal JSON fields
	if err := json.Unmarshal([]byte(categoriesJSON), &payload.Categories); err != nil {
		return nil, fmt.Errorf("failed to unmarshal categories: %w", err)
	}

	if err := json.Unmarshal([]byte(tagsJSON), &payload.Tags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	if err := json.Unmarshal([]byte(parametersJSON), &payload.Parameters); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	if err := json.Unmarshal([]byte(indicatorsJSON), &payload.SuccessIndicators); err != nil {
		return nil, fmt.Errorf("failed to unmarshal success indicators: %w", err)
	}

	if err := json.Unmarshal([]byte(targetTypesJSON), &payload.TargetTypes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal target types: %w", err)
	}

	if err := json.Unmarshal([]byte(mitreTechniquesJSON), &payload.MitreTechniques); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mitre techniques: %w", err)
	}

	if err := json.Unmarshal([]byte(metadataJSON), &payload.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &payload, nil
}

// scanPayloadFromRows scans a payload from sql.Rows
func (s *payloadStore) scanPayloadFromRows(rows *sql.Rows) (*Payload, error) {
	var payload Payload
	var id, severity string
	var categoriesJSON, tagsJSON, parametersJSON, indicatorsJSON string
	var targetTypesJSON, mitreTechniquesJSON, metadataJSON string

	err := rows.Scan(
		&id,
		&payload.Name,
		&payload.Version,
		&payload.Description,
		&categoriesJSON,
		&tagsJSON,
		&payload.Template,
		&parametersJSON,
		&indicatorsJSON,
		&targetTypesJSON,
		&severity,
		&mitreTechniquesJSON,
		&metadataJSON,
		&payload.BuiltIn,
		&payload.Enabled,
		&payload.CreatedAt,
		&payload.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scan payload: %w", err)
	}

	// Parse ID
	payload.ID, err = types.ParseID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse payload ID: %w", err)
	}

	// Parse severity enum
	payload.Severity = agent.FindingSeverity(severity)

	// Unmarshal JSON fields
	if err := json.Unmarshal([]byte(categoriesJSON), &payload.Categories); err != nil {
		return nil, fmt.Errorf("failed to unmarshal categories: %w", err)
	}

	if err := json.Unmarshal([]byte(tagsJSON), &payload.Tags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	if err := json.Unmarshal([]byte(parametersJSON), &payload.Parameters); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	if err := json.Unmarshal([]byte(indicatorsJSON), &payload.SuccessIndicators); err != nil {
		return nil, fmt.Errorf("failed to unmarshal success indicators: %w", err)
	}

	if err := json.Unmarshal([]byte(targetTypesJSON), &payload.TargetTypes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal target types: %w", err)
	}

	if err := json.Unmarshal([]byte(mitreTechniquesJSON), &payload.MitreTechniques); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mitre techniques: %w", err)
	}

	if err := json.Unmarshal([]byte(metadataJSON), &payload.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &payload, nil
}

// nullString converts a string to sql.NullString
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}
