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

	// ImportBatch imports multiple payloads with validation
	ImportBatch(ctx context.Context, payloads []*Payload) (*ImportResult, error)

	// GetSummaryForTargetType returns payload summary for orchestrator context
	GetSummaryForTargetType(ctx context.Context, targetType string) (*PayloadSummary, error)

	// CreateChain creates a new payload chain
	CreateChain(ctx context.Context, chain *PayloadChain) error

	// GetChain retrieves a chain by ID
	GetChain(ctx context.Context, id types.ID) (*PayloadChain, error)

	// ListChains retrieves all chains
	ListChains(ctx context.Context) ([]*PayloadChain, error)

	// UpdateChain updates an existing chain
	UpdateChain(ctx context.Context, chain *PayloadChain) error

	// DeleteChain deletes a chain by ID
	DeleteChain(ctx context.Context, id types.ID) error
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

// PayloadSummary provides aggregate statistics for payloads
type PayloadSummary struct {
	Total        int                           `json:"total"`
	ByCategory   map[PayloadCategory]int       `json:"by_category"`
	ByTargetType map[string]int                `json:"by_target_type"`
	BySeverity   map[agent.FindingSeverity]int `json:"by_severity"`
	EnabledCount int                           `json:"enabled_count"`
	BuiltInCount int                           `json:"built_in_count"`
}

// ImportResult contains the results of a batch import operation
type ImportResult struct {
	Total    int      `json:"total"`
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Failed   int      `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
}

// PayloadChain represents a sequence of payloads to execute in order
type PayloadChain struct {
	ID          types.ID        `json:"id" yaml:"id"`
	Name        string          `json:"name" yaml:"name"`
	Description string          `json:"description" yaml:"description"`
	Steps       []ChainStep     `json:"steps" yaml:"steps"`
	Metadata    PayloadMetadata `json:"metadata" yaml:"metadata"`
	CreatedAt   time.Time       `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" yaml:"updated_at"`
}

// ChainStep represents a single step in a payload chain
type ChainStep struct {
	ID        string         `json:"id" yaml:"id"`
	PayloadID types.ID       `json:"payload_id" yaml:"payload_id"`
	Params    map[string]any `json:"params,omitempty" yaml:"params,omitempty"`
	OnSuccess StepAction     `json:"on_success" yaml:"on_success"`
	OnFailure StepAction     `json:"on_failure" yaml:"on_failure"`
	Requires  []string       `json:"requires,omitempty" yaml:"requires,omitempty"` // step IDs
}

// StepAction defines what to do after a step completes
type StepAction string

const (
	StepActionContinue StepAction = "continue" // Continue to next step
	StepActionAbort    StepAction = "abort"    // Stop chain execution
	StepActionTryNext  StepAction = "try_next" // Try next alternative step
	StepActionSkipTo   StepAction = "skip_to"  // Skip to specific step ID
)

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

// ImportBatch imports multiple payloads with validation
func (s *payloadStore) ImportBatch(ctx context.Context, payloads []*Payload) (*ImportResult, error) {
	if payloads == nil {
		return nil, fmt.Errorf("payloads slice cannot be nil")
	}

	result := &ImportResult{
		Total:  len(payloads),
		Errors: []string{},
	}

	for i, payload := range payloads {
		if payload == nil {
			result.Skipped++
			result.Errors = append(result.Errors, fmt.Sprintf("payload at index %d is nil", i))
			continue
		}

		// Validate payload
		if err := payload.Validate(); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("payload %s: %v", payload.Name, err))
			continue
		}

		// Check if payload already exists by name
		exists, err := s.ExistsByName(ctx, payload.Name)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("payload %s: failed to check existence: %v", payload.Name, err))
			continue
		}

		if exists {
			result.Skipped++
			result.Errors = append(result.Errors, fmt.Sprintf("payload %s: already exists", payload.Name))
			continue
		}

		// Save payload
		if err := s.Save(ctx, payload); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("payload %s: save failed: %v", payload.Name, err))
			continue
		}

		result.Imported++
	}

	return result, nil
}

// GetSummaryForTargetType returns payload summary for orchestrator context
func (s *payloadStore) GetSummaryForTargetType(ctx context.Context, targetType string) (*PayloadSummary, error) {
	summary := &PayloadSummary{
		ByCategory:   make(map[PayloadCategory]int),
		ByTargetType: make(map[string]int),
		BySeverity:   make(map[agent.FindingSeverity]int),
	}

	// Build query with target type filter
	query := `
		SELECT
			categories, target_types, severity, enabled, built_in
		FROM payloads
		WHERE enabled = 1
	`

	var args []interface{}

	// Add target type filter if specified
	if targetType != "" {
		// Empty target_types means supports all types
		query += ` AND (target_types = '[]' OR target_types LIKE ?)`
		args = append(args, fmt.Sprintf("%%\"%s\"%%", targetType))
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query payloads for summary: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var categoriesJSON, targetTypesJSON, severity string
		var enabled, builtIn bool

		if err := rows.Scan(&categoriesJSON, &targetTypesJSON, &severity, &enabled, &builtIn); err != nil {
			return nil, fmt.Errorf("failed to scan payload row: %w", err)
		}

		summary.Total++

		if enabled {
			summary.EnabledCount++
		}

		if builtIn {
			summary.BuiltInCount++
		}

		// Unmarshal and count categories
		var categories []PayloadCategory
		if err := json.Unmarshal([]byte(categoriesJSON), &categories); err != nil {
			return nil, fmt.Errorf("failed to unmarshal categories: %w", err)
		}
		for _, cat := range categories {
			summary.ByCategory[cat]++
		}

		// Unmarshal and count target types
		var targetTypes []string
		if err := json.Unmarshal([]byte(targetTypesJSON), &targetTypes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal target types: %w", err)
		}
		for _, tt := range targetTypes {
			summary.ByTargetType[tt]++
		}

		// Count severity
		sev := agent.FindingSeverity(severity)
		summary.BySeverity[sev]++
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating payload rows: %w", err)
	}

	return summary, nil
}

// CreateChain creates a new payload chain
func (s *payloadStore) CreateChain(ctx context.Context, chain *PayloadChain) error {
	if chain == nil {
		return fmt.Errorf("chain cannot be nil")
	}

	if chain.Name == "" {
		return fmt.Errorf("chain name is required")
	}

	if len(chain.Steps) == 0 {
		return fmt.Errorf("chain must have at least one step")
	}

	// Validate steps
	for i, step := range chain.Steps {
		if step.ID == "" {
			return fmt.Errorf("step at index %d: ID is required", i)
		}
		if err := step.PayloadID.Validate(); err != nil {
			return fmt.Errorf("step at index %d: invalid payload ID: %w", i, err)
		}
	}

	// Marshal JSON fields
	stepsJSON, err := json.Marshal(chain.Steps)
	if err != nil {
		return fmt.Errorf("failed to marshal steps: %w", err)
	}

	metadataJSON, err := json.Marshal(chain.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO attack_chains (
			id, name, description, stages, metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	if chain.CreatedAt.IsZero() {
		chain.CreatedAt = now
	}
	if chain.UpdatedAt.IsZero() {
		chain.UpdatedAt = now
	}

	_, err = s.db.ExecContext(ctx, query,
		chain.ID.String(),
		chain.Name,
		chain.Description,
		string(stepsJSON),
		string(metadataJSON),
		chain.CreatedAt,
		chain.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to insert chain: %w", err)
	}

	return nil
}

// GetChain retrieves a chain by ID
func (s *payloadStore) GetChain(ctx context.Context, id types.ID) (*PayloadChain, error) {
	query := `
		SELECT
			id, name, description, stages, metadata, created_at, updated_at
		FROM attack_chains
		WHERE id = ?
	`

	var chain PayloadChain
	var chainID, stepsJSON, metadataJSON string

	err := s.db.QueryRowContext(ctx, query, id.String()).Scan(
		&chainID,
		&chain.Name,
		&chain.Description,
		&stepsJSON,
		&metadataJSON,
		&chain.CreatedAt,
		&chain.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("chain not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query chain: %w", err)
	}

	// Parse ID
	chain.ID, err = types.ParseID(chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse chain ID: %w", err)
	}

	// Unmarshal JSON fields
	if err := json.Unmarshal([]byte(stepsJSON), &chain.Steps); err != nil {
		return nil, fmt.Errorf("failed to unmarshal steps: %w", err)
	}

	if err := json.Unmarshal([]byte(metadataJSON), &chain.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &chain, nil
}

// ListChains retrieves all chains
func (s *payloadStore) ListChains(ctx context.Context) ([]*PayloadChain, error) {
	query := `
		SELECT
			id, name, description, stages, metadata, created_at, updated_at
		FROM attack_chains
		ORDER BY created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query chains: %w", err)
	}
	defer rows.Close()

	var chains []*PayloadChain
	for rows.Next() {
		var chain PayloadChain
		var chainID, stepsJSON, metadataJSON string

		err := rows.Scan(
			&chainID,
			&chain.Name,
			&chain.Description,
			&stepsJSON,
			&metadataJSON,
			&chain.CreatedAt,
			&chain.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan chain: %w", err)
		}

		// Parse ID
		chain.ID, err = types.ParseID(chainID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse chain ID: %w", err)
		}

		// Unmarshal JSON fields
		if err := json.Unmarshal([]byte(stepsJSON), &chain.Steps); err != nil {
			return nil, fmt.Errorf("failed to unmarshal steps: %w", err)
		}

		if err := json.Unmarshal([]byte(metadataJSON), &chain.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}

		chains = append(chains, &chain)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating chains: %w", err)
	}

	return chains, nil
}

// UpdateChain updates an existing chain
func (s *payloadStore) UpdateChain(ctx context.Context, chain *PayloadChain) error {
	if chain == nil {
		return fmt.Errorf("chain cannot be nil")
	}

	if chain.Name == "" {
		return fmt.Errorf("chain name is required")
	}

	// Marshal JSON fields
	stepsJSON, err := json.Marshal(chain.Steps)
	if err != nil {
		return fmt.Errorf("failed to marshal steps: %w", err)
	}

	metadataJSON, err := json.Marshal(chain.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		UPDATE attack_chains SET
			name = ?,
			description = ?,
			stages = ?,
			metadata = ?,
			updated_at = ?
		WHERE id = ?
	`

	chain.UpdatedAt = time.Now()

	result, err := s.db.ExecContext(ctx, query,
		chain.Name,
		chain.Description,
		string(stepsJSON),
		string(metadataJSON),
		chain.UpdatedAt,
		chain.ID.String(),
	)

	if err != nil {
		return fmt.Errorf("failed to update chain: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("chain not found: %s", chain.ID)
	}

	return nil
}

// DeleteChain deletes a chain by ID
func (s *payloadStore) DeleteChain(ctx context.Context, id types.ID) error {
	query := `DELETE FROM attack_chains WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query, id.String())
	if err != nil {
		return fmt.Errorf("failed to delete chain: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("chain not found: %s", id)
	}

	return nil
}
