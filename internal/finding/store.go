package finding

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/trace"
)

// FindingStore provides persistence for EnhancedFindings
type FindingStore interface {
	// Store persists an enhanced finding
	Store(ctx context.Context, finding EnhancedFinding) error

	// Get retrieves a finding by ID
	Get(ctx context.Context, id types.ID) (*EnhancedFinding, error)

	// List retrieves findings for a mission with optional filtering
	List(ctx context.Context, missionID types.ID, filter *FindingFilter) ([]EnhancedFinding, error)

	// Update updates an existing finding
	Update(ctx context.Context, finding EnhancedFinding) error

	// Delete removes a finding
	Delete(ctx context.Context, id types.ID) error

	// Count returns the total number of findings for a mission
	Count(ctx context.Context, missionID types.ID) (int, error)
}

// FindingFilter provides filtering options for finding queries
type FindingFilter struct {
	Severity   *agent.FindingSeverity
	Category   *FindingCategory
	Status     *FindingStatus
	MinRisk    *float64
	MaxRisk    *float64
	AgentName  *string
	SearchText *string
}

// NewFindingFilter creates a new empty filter
func NewFindingFilter() *FindingFilter {
	return &FindingFilter{}
}

// WithSeverity filters by severity
func (f *FindingFilter) WithSeverity(severity agent.FindingSeverity) *FindingFilter {
	f.Severity = &severity
	return f
}

// WithCategory filters by category
func (f *FindingFilter) WithCategory(category FindingCategory) *FindingFilter {
	f.Category = &category
	return f
}

// WithStatus filters by status
func (f *FindingFilter) WithStatus(status FindingStatus) *FindingFilter {
	f.Status = &status
	return f
}

// WithRiskRange filters by risk score range
func (f *FindingFilter) WithRiskRange(min, max float64) *FindingFilter {
	f.MinRisk = &min
	f.MaxRisk = &max
	return f
}

// DBFindingStore implements FindingStore using a SQL database with caching and tracing
type DBFindingStore struct {
	db     *database.DB
	cache  *sync.Map
	tracer trace.Tracer
}

// StoreOption is a functional option for configuring DBFindingStore
type StoreOption func(*DBFindingStore)

// WithTracer sets the OpenTelemetry tracer for the store
func WithTracer(tracer trace.Tracer) StoreOption {
	return func(s *DBFindingStore) {
		s.tracer = tracer
	}
}

// WithCacheSize enables caching for the store
// Note: Currently using sync.Map which doesn't have a size limit
// For production use, consider using groupcache or an LRU cache
func WithCacheSize(size int) StoreOption {
	return func(s *DBFindingStore) {
		s.cache = &sync.Map{}
	}
}

// NewDBFindingStore creates a new database-backed finding store with functional options
func NewDBFindingStore(db *database.DB, opts ...StoreOption) *DBFindingStore {
	store := &DBFindingStore{
		db:     db,
		cache:  nil,
		tracer: trace.NewNoopTracerProvider().Tracer("finding-store"),
	}

	for _, opt := range opts {
		opt(store)
	}

	return store
}

// StoreEnhanced persists an enhanced finding to the database (implements FindingStore interface)
func (s *DBFindingStore) Store(ctx context.Context, finding EnhancedFinding) error {
	return s.StoreWithMission(ctx, finding.MissionID, &finding)
}

// StoreWithMission saves an enhanced finding to the database with explicit mission ID
// This is the implementation required by the task specification
func (s *DBFindingStore) StoreWithMission(ctx context.Context, missionID types.ID, finding *EnhancedFinding) error {
	ctx, span := s.tracer.Start(ctx, "DBFindingStore.Store")
	defer span.End()

	// Ensure mission ID is set
	finding.MissionID = missionID
	finding.UpdatedAt = time.Now()

	conn := s.db.Conn()

	// Marshal JSON fields
	// Note: Evidence is stored in a separate table (findings_evidence) - not implemented yet
	mitreAttackJSON, err := json.Marshal(finding.MitreAttack)
	if err != nil {
		return fmt.Errorf("failed to marshal mitre_attack: %w", err)
	}

	mitreAtlasJSON, err := json.Marshal(finding.MitreAtlas)
	if err != nil {
		return fmt.Errorf("failed to marshal mitre_atlas: %w", err)
	}

	referencesJSON, err := json.Marshal(finding.References)
	if err != nil {
		return fmt.Errorf("failed to marshal references: %w", err)
	}

	relatedIDsJSON, err := json.Marshal(finding.RelatedIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal related_ids: %w", err)
	}

	reproStepsJSON, err := json.Marshal(finding.ReproSteps)
	if err != nil {
		return fmt.Errorf("failed to marshal repro_steps: %w", err)
	}

	cweJSON, err := json.Marshal(finding.CWE)
	if err != nil {
		return fmt.Errorf("failed to marshal cwe_ids: %w", err)
	}

	// Handle CVSS score
	var cvssVector *string
	var cvssScore *float64
	if finding.CVSS != nil {
		cvssVector = &finding.CVSS.Vector
		cvssScore = &finding.CVSS.Score
	}

	query := `
		INSERT INTO findings (
			id, title, description, severity, status, remediation,
			mission_id, agent_name, delegated_from, category, subcategory,
			confidence, risk_score, mitre_attack, mitre_atlas, references_json,
			repro_steps, related_ids, occurrence_count, cvss_vector, cvss_score,
			cwe_ids, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			description = excluded.description,
			severity = excluded.severity,
			status = excluded.status,
			remediation = excluded.remediation,
			agent_name = excluded.agent_name,
			delegated_from = excluded.delegated_from,
			category = excluded.category,
			subcategory = excluded.subcategory,
			confidence = excluded.confidence,
			risk_score = excluded.risk_score,
			mitre_attack = excluded.mitre_attack,
			mitre_atlas = excluded.mitre_atlas,
			references_json = excluded.references_json,
			repro_steps = excluded.repro_steps,
			related_ids = excluded.related_ids,
			occurrence_count = excluded.occurrence_count,
			cvss_vector = excluded.cvss_vector,
			cvss_score = excluded.cvss_score,
			cwe_ids = excluded.cwe_ids,
			updated_at = excluded.updated_at
	`

	_, err = conn.ExecContext(ctx, query,
		finding.ID.String(),
		finding.Title,
		finding.Description,
		string(finding.Severity),
		string(finding.Status),
		finding.Remediation,
		finding.MissionID.String(),
		finding.AgentName,
		finding.DelegatedFrom,
		finding.Category,
		finding.Subcategory,
		finding.Confidence,
		finding.RiskScore,
		string(mitreAttackJSON),
		string(mitreAtlasJSON),
		string(referencesJSON),
		string(reproStepsJSON),
		string(relatedIDsJSON),
		finding.OccurrenceCount,
		cvssVector,
		cvssScore,
		string(cweJSON),
		finding.CreatedAt,
		finding.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert finding: %w", err)
	}

	// Update cache if enabled
	if s.cache != nil {
		s.cache.Store(finding.ID.String(), finding)
	}

	return nil
}

// GetByID retrieves a finding by ID (implements FindingStore interface)
func (s *DBFindingStore) Get(ctx context.Context, id types.ID) (*EnhancedFinding, error) {
	ctx, span := s.tracer.Start(ctx, "DBFindingStore.GetByID")
	defer span.End()

	// Check cache first
	if s.cache != nil {
		if cached, ok := s.cache.Load(id.String()); ok {
			return cached.(*EnhancedFinding), nil
		}
	}

	conn := s.db.Conn()
	query := `
		SELECT
			id, title, description, severity, status, remediation,
			mission_id, agent_name, delegated_from, category, subcategory,
			confidence, risk_score, mitre_attack, mitre_atlas, references_json,
			repro_steps, related_ids, occurrence_count, cvss_vector, cvss_score,
			cwe_ids, target_id, created_at, updated_at
		FROM findings
		WHERE id = ?
	`

	row := conn.QueryRowContext(ctx, query, id.String())
	finding, err := s.scanFinding(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("finding not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get finding: %w", err)
	}

	// Update cache
	if s.cache != nil {
		s.cache.Store(id.String(), finding)
	}

	return finding, nil
}

// GetWithFilter retrieves findings from the database based on harness.FindingFilter
// This is the method required by the task specification
func (s *DBFindingStore) GetWithFilter(ctx context.Context, missionID types.ID, filter harness.FindingFilter) ([]*EnhancedFinding, error) {
	ctx, span := s.tracer.Start(ctx, "DBFindingStore.Get")
	defer span.End()

	conn := s.db.Conn()

	// Build WHERE clause dynamically
	var conditions []string
	var args []interface{}

	// Always filter by mission ID
	conditions = append(conditions, "mission_id = ?")
	args = append(args, missionID.String())

	// Apply filter criteria
	if filter.Severity != nil {
		conditions = append(conditions, "severity = ?")
		args = append(args, string(*filter.Severity))
	}

	if filter.MinConfidence != nil {
		conditions = append(conditions, "confidence >= ?")
		args = append(args, *filter.MinConfidence)
	}

	if filter.MaxConfidence != nil {
		conditions = append(conditions, "confidence <= ?")
		args = append(args, *filter.MaxConfidence)
	}

	if filter.Category != nil {
		conditions = append(conditions, "LOWER(category) LIKE ?")
		args = append(args, "%"+strings.ToLower(*filter.Category)+"%")
	}

	if filter.TargetID != nil {
		// Note: target_id is not in the base findings table per schema.sql
		// It will be added in migration 3
		conditions = append(conditions, "target_id = ?")
		args = append(args, filter.TargetID.String())
	}

	if filter.TitleContains != nil {
		conditions = append(conditions, "LOWER(title) LIKE ?")
		args = append(args, "%"+strings.ToLower(*filter.TitleContains)+"%")
	}

	if filter.DescriptionContains != nil {
		conditions = append(conditions, "LOWER(description) LIKE ?")
		args = append(args, "%"+strings.ToLower(*filter.DescriptionContains)+"%")
	}

	if filter.HasCVSS != nil {
		if *filter.HasCVSS {
			conditions = append(conditions, "cvss_score IS NOT NULL")
		} else {
			conditions = append(conditions, "cvss_score IS NULL")
		}
	}

	if filter.MinCVSS != nil {
		conditions = append(conditions, "cvss_score >= ?")
		args = append(args, *filter.MinCVSS)
	}

	if filter.MaxCVSS != nil {
		conditions = append(conditions, "cvss_score <= ?")
		args = append(args, *filter.MaxCVSS)
	}

	// Build query with explicit column list matching scan order
	columns := `
		id, title, description, severity, status, remediation,
		mission_id, agent_name, delegated_from, category, subcategory,
		confidence, risk_score, mitre_attack, mitre_atlas, references_json,
		repro_steps, related_ids, occurrence_count, cvss_vector, cvss_score,
		cwe_ids, target_id, created_at, updated_at
	`
	query := "SELECT " + columns + " FROM findings WHERE " + strings.Join(conditions, " AND ") + " ORDER BY created_at DESC"

	rows, err := conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query findings: %w", err)
	}
	defer rows.Close()

	findings, err := s.scanFindings(rows)
	if err != nil {
		return nil, err
	}

	// Apply CWE filter (done in-memory since it requires JSON parsing)
	if len(filter.CWE) > 0 {
		filtered := make([]*EnhancedFinding, 0)
		for _, f := range findings {
			if hasCWEMatch(f.CWE, filter.CWE) {
				filtered = append(filtered, f)
			}
		}
		findings = filtered
	}

	return findings, nil
}

// ListWithPagination retrieves findings with pagination (task specification)
func (s *DBFindingStore) ListWithPagination(ctx context.Context, missionID types.ID, offset, limit int) ([]*EnhancedFinding, error) {
	ctx, span := s.tracer.Start(ctx, "DBFindingStore.List")
	defer span.End()

	conn := s.db.Conn()
	query := `
		SELECT
			id, title, description, severity, status, remediation,
			mission_id, agent_name, delegated_from, category, subcategory,
			confidence, risk_score, mitre_attack, mitre_atlas, references_json,
			repro_steps, related_ids, occurrence_count, cvss_vector, cvss_score,
			cwe_ids, target_id, created_at, updated_at
		FROM findings
		WHERE mission_id = ? AND status != 'deleted'
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := conn.QueryContext(ctx, query, missionID.String(), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list findings: %w", err)
	}
	defer rows.Close()

	return s.scanFindings(rows)
}

// List retrieves findings for a mission with optional filtering (implements FindingStore interface)
func (s *DBFindingStore) List(ctx context.Context, missionID types.ID, filter *FindingFilter) ([]EnhancedFinding, error) {
	query := `
		SELECT
			id, title, description, severity, status, remediation,
			mission_id, agent_name, delegated_from, category, subcategory,
			confidence, risk_score, mitre_attack, mitre_atlas, references_json,
			repro_steps, related_ids, occurrence_count, cvss_vector, cvss_score,
			cwe_ids, target_id, created_at, updated_at
		FROM findings
		WHERE 1=1
	`

	args := []interface{}{}

	// Add mission ID filter if provided
	if missionID.String() != "" {
		query += " AND mission_id = ?"
		args = append(args, missionID.String())
	}

	// Apply filters
	if filter != nil {
		if filter.Severity != nil {
			query += " AND severity = ?"
			args = append(args, *filter.Severity)
		}
		if filter.Category != nil {
			query += " AND category = ?"
			args = append(args, *filter.Category)
		}
		if filter.Status != nil {
			query += " AND status = ?"
			args = append(args, *filter.Status)
		}
		if filter.MinRisk != nil {
			query += " AND risk_score >= ?"
			args = append(args, *filter.MinRisk)
		}
		if filter.MaxRisk != nil {
			query += " AND risk_score <= ?"
			args = append(args, *filter.MaxRisk)
		}
		if filter.AgentName != nil {
			query += " AND agent_name = ?"
			args = append(args, *filter.AgentName)
		}
		if filter.SearchText != nil {
			query += " AND (title LIKE ? OR description LIKE ?)"
			searchPattern := "%" + *filter.SearchText + "%"
			args = append(args, searchPattern, searchPattern)
		}
	}

	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list findings: %w", err)
	}
	defer rows.Close()

	var findings []EnhancedFinding
	for rows.Next() {
		finding, err := s.scanFinding(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan finding: %w", err)
		}
		findings = append(findings, *finding)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating findings: %w", err)
	}

	return findings, nil
}

// UpdateFinding updates an existing finding (implements FindingStore interface)
func (s *DBFindingStore) Update(ctx context.Context, finding EnhancedFinding) error {
	return s.UpdateEnhanced(ctx, &finding)
}

// UpdateEnhanced modifies an existing finding in the database (task specification)
func (s *DBFindingStore) UpdateEnhanced(ctx context.Context, finding *EnhancedFinding) error {
	ctx, span := s.tracer.Start(ctx, "DBFindingStore.Update")
	defer span.End()

	finding.UpdatedAt = time.Now()

	conn := s.db.Conn()

	// Marshal JSON fields
	// Note: Evidence is stored in a separate table (findings_evidence) - not implemented yet
	mitreAttackJSON, err := json.Marshal(finding.MitreAttack)
	if err != nil {
		return fmt.Errorf("failed to marshal mitre_attack: %w", err)
	}

	mitreAtlasJSON, err := json.Marshal(finding.MitreAtlas)
	if err != nil {
		return fmt.Errorf("failed to marshal mitre_atlas: %w", err)
	}

	referencesJSON, err := json.Marshal(finding.References)
	if err != nil {
		return fmt.Errorf("failed to marshal references: %w", err)
	}

	relatedIDsJSON, err := json.Marshal(finding.RelatedIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal related_ids: %w", err)
	}

	reproStepsJSON, err := json.Marshal(finding.ReproSteps)
	if err != nil {
		return fmt.Errorf("failed to marshal repro_steps: %w", err)
	}

	cweJSON, err := json.Marshal(finding.CWE)
	if err != nil {
		return fmt.Errorf("failed to marshal cwe_ids: %w", err)
	}

	// Handle CVSS score
	var cvssVector *string
	var cvssScore *float64
	if finding.CVSS != nil {
		cvssVector = &finding.CVSS.Vector
		cvssScore = &finding.CVSS.Score
	}

	query := `
		UPDATE findings SET
			title = ?,
			description = ?,
			severity = ?,
			status = ?,
			remediation = ?,
			category = ?,
			subcategory = ?,
			confidence = ?,
			risk_score = ?,
			mitre_attack = ?,
			mitre_atlas = ?,
			references_json = ?,
			repro_steps = ?,
			related_ids = ?,
			occurrence_count = ?,
			cvss_vector = ?,
			cvss_score = ?,
			cwe_ids = ?,
			updated_at = ?
		WHERE id = ?
	`

	result, err := conn.ExecContext(ctx, query,
		finding.Title,
		finding.Description,
		string(finding.Severity),
		string(finding.Status),
		finding.Remediation,
		finding.Category,
		finding.Subcategory,
		finding.Confidence,
		finding.RiskScore,
		string(mitreAttackJSON),
		string(mitreAtlasJSON),
		string(referencesJSON),
		string(reproStepsJSON),
		string(relatedIDsJSON),
		finding.OccurrenceCount,
		cvssVector,
		cvssScore,
		string(cweJSON),
		finding.UpdatedAt,
		finding.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to update finding: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("finding not found: %s", finding.ID)
	}

	// Update cache if enabled
	if s.cache != nil {
		s.cache.Store(finding.ID.String(), finding)
	}

	return nil
}

// Delete soft-deletes a finding by setting its status to 'deleted' (task specification)
func (s *DBFindingStore) Delete(ctx context.Context, findingID types.ID) error {
	ctx, span := s.tracer.Start(ctx, "DBFindingStore.Delete")
	defer span.End()

	conn := s.db.Conn()
	query := "UPDATE findings SET status = ?, updated_at = ? WHERE id = ?"
	result, err := conn.ExecContext(ctx, query, "deleted", time.Now(), findingID.String())
	if err != nil {
		return fmt.Errorf("failed to delete finding: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("finding not found: %s", findingID)
	}

	// Remove from cache if enabled
	if s.cache != nil {
		s.cache.Delete(findingID.String())
	}

	return nil
}

// Search performs full-text search on findings using FTS5 (task specification)
func (s *DBFindingStore) Search(ctx context.Context, query string, limit int) ([]*EnhancedFinding, error) {
	ctx, span := s.tracer.Start(ctx, "DBFindingStore.Search")
	defer span.End()

	conn := s.db.Conn()

	// Column list matching scan order
	columns := `
		id, title, description, severity, status, remediation,
		mission_id, agent_name, delegated_from, category, subcategory,
		confidence, risk_score, mitre_attack, mitre_atlas, references_json,
		repro_steps, related_ids, occurrence_count, cvss_vector, cvss_score,
		cwe_ids, target_id, created_at, updated_at
	`

	// Try FTS5 virtual table first
	ftsQuery := `
		SELECT ` + columns + ` FROM findings f
		INNER JOIN findings_fts fts ON f.rowid = fts.rowid
		WHERE findings_fts MATCH ?
		AND f.status != 'deleted'
		ORDER BY rank
		LIMIT ?
	`

	rows, err := conn.QueryContext(ctx, ftsQuery, query, limit)
	if err != nil {
		// Fallback to simple LIKE search if FTS5 not available
		fallbackQuery := `
			SELECT ` + columns + ` FROM findings
			WHERE (title LIKE ? OR description LIKE ? OR remediation LIKE ?)
			AND status != 'deleted'
			ORDER BY created_at DESC
			LIMIT ?
		`
		likePattern := "%" + query + "%"
		rows, err = conn.QueryContext(ctx, fallbackQuery, likePattern, likePattern, likePattern, limit)
		if err != nil {
			return nil, fmt.Errorf("failed to search findings: %w", err)
		}
	}
	defer rows.Close()

	return s.scanFindings(rows)
}

// CountBySeverity returns a count of findings grouped by severity (task specification)
func (s *DBFindingStore) CountBySeverity(ctx context.Context, missionID types.ID) (map[agent.FindingSeverity]int, error) {
	ctx, span := s.tracer.Start(ctx, "DBFindingStore.CountBySeverity")
	defer span.End()

	conn := s.db.Conn()
	query := `
		SELECT severity, COUNT(*) as count
		FROM findings
		WHERE mission_id = ? AND status != 'deleted'
		GROUP BY severity
	`

	rows, err := conn.QueryContext(ctx, query, missionID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to count by severity: %w", err)
	}
	defer rows.Close()

	counts := make(map[agent.FindingSeverity]int)
	for rows.Next() {
		var severity string
		var count int
		if err := rows.Scan(&severity, &count); err != nil {
			return nil, fmt.Errorf("failed to scan count: %w", err)
		}
		counts[agent.FindingSeverity(severity)] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return counts, nil
}

// Count returns the total number of findings for a mission (implements FindingStore interface)
func (s *DBFindingStore) Count(ctx context.Context, missionID types.ID) (int, error) {
	conn := s.db.Conn()
	query := "SELECT COUNT(*) FROM findings WHERE mission_id = ? AND status != 'deleted'"
	var count int
	err := conn.QueryRowContext(ctx, query, missionID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count findings: %w", err)
	}
	return count, nil
}

// scanFinding scans a single database row into an EnhancedFinding
func (s *DBFindingStore) scanFinding(scanner interface {
	Scan(dest ...interface{}) error
}) (*EnhancedFinding, error) {
	var f EnhancedFinding
	var (
		idStr           string
		missionIDStr    string
		severityStr     string
		statusStr       string
		delegatedFrom   sql.NullString
		subcategory     sql.NullString
		remediation     sql.NullString
		cvssVector      sql.NullString
		cvssScore       sql.NullFloat64
		mitreAttackJSON string
		mitreAtlasJSON  string
		referencesJSON  string
		reproStepsJSON  string
		relatedIDsJSON  string
		cweJSON         string
		targetIDStr     sql.NullString
	)

	err := scanner.Scan(
		&idStr,
		&f.Title,
		&f.Description,
		&severityStr,
		&statusStr,
		&remediation,
		&missionIDStr,
		&f.AgentName,
		&delegatedFrom,
		&f.Category,
		&subcategory,
		&f.Confidence,
		&f.RiskScore,
		&mitreAttackJSON,
		&mitreAtlasJSON,
		&referencesJSON,
		&reproStepsJSON,
		&relatedIDsJSON,
		&f.OccurrenceCount,
		&cvssVector,
		&cvssScore,
		&cweJSON,
		&targetIDStr,
		&f.CreatedAt,
		&f.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Parse IDs
	id, err := types.ParseID(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse finding ID: %w", err)
	}
	f.ID = id

	missionID, err := types.ParseID(missionIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mission ID: %w", err)
	}
	f.MissionID = missionID

	// Set string fields
	f.Severity = agent.FindingSeverity(severityStr)
	f.Status = FindingStatus(statusStr)

	if delegatedFrom.Valid {
		f.DelegatedFrom = &delegatedFrom.String
	}

	if subcategory.Valid {
		f.Subcategory = subcategory.String
	}

	if remediation.Valid {
		f.Remediation = remediation.String
	}

	// Handle target ID
	if targetIDStr.Valid && targetIDStr.String != "" {
		targetID, err := types.ParseID(targetIDStr.String)
		if err == nil {
			f.Finding.TargetID = &targetID
		}
	}

	// Handle CVSS score
	if cvssVector.Valid && cvssScore.Valid {
		f.CVSS = &agent.CVSSScore{
			Vector:  cvssVector.String,
			Score:   cvssScore.Float64,
			Version: "3.1", // Default version
		}
	}

	// Unmarshal JSON fields (ignoring errors for optional fields)
	json.Unmarshal([]byte(mitreAttackJSON), &f.MitreAttack)
	json.Unmarshal([]byte(mitreAtlasJSON), &f.MitreAtlas)
	json.Unmarshal([]byte(referencesJSON), &f.References)
	json.Unmarshal([]byte(reproStepsJSON), &f.ReproSteps)
	json.Unmarshal([]byte(relatedIDsJSON), &f.RelatedIDs)
	json.Unmarshal([]byte(cweJSON), &f.CWE)

	return &f, nil
}

// scanFindings scans SQL rows into EnhancedFinding structs
func (s *DBFindingStore) scanFindings(rows *sql.Rows) ([]*EnhancedFinding, error) {
	findings := make([]*EnhancedFinding, 0)

	for rows.Next() {
		finding, err := s.scanFinding(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan finding: %w", err)
		}
		findings = append(findings, finding)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return findings, nil
}

// hasCWEMatch checks if the finding's CWE list contains any of the filter CWE IDs
func hasCWEMatch(findingCWEs, filterCWEs []string) bool {
	if len(findingCWEs) == 0 {
		return false
	}

	for _, filterCWE := range filterCWEs {
		for _, findingCWE := range findingCWEs {
			if findingCWE == filterCWE {
				return true
			}
		}
	}

	return false
}

// Ensure DBFindingStore implements FindingStore at compile time
var _ FindingStore = (*DBFindingStore)(nil)
