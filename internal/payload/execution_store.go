package payload

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

// ExecutionStore provides database access for Execution entities
type ExecutionStore interface {
	// Save inserts a new execution record
	Save(ctx context.Context, execution *Execution) error

	// Get retrieves an execution by ID
	Get(ctx context.Context, id types.ID) (*Execution, error)

	// List retrieves executions with optional filtering
	List(ctx context.Context, filter *ExecutionFilter) ([]*Execution, error)

	// GetByPayload retrieves executions for a specific payload
	GetByPayload(ctx context.Context, payloadID types.ID, limit int) ([]*Execution, error)

	// GetByMission retrieves executions for a specific mission
	GetByMission(ctx context.Context, missionID types.ID, limit int) ([]*Execution, error)

	// GetStats retrieves aggregate statistics for analytics
	GetStats(ctx context.Context, filter *ExecutionFilter) (*ExecutionStats, error)

	// Update updates an existing execution record
	Update(ctx context.Context, execution *Execution) error

	// Count returns the total number of executions matching the filter
	Count(ctx context.Context, filter *ExecutionFilter) (int, error)
}

// ExecutionFilter defines filter criteria for querying executions
type ExecutionFilter struct {
	PayloadIDs      []types.ID         `json:"payload_ids,omitempty"`
	TargetIDs       []types.ID         `json:"target_ids,omitempty"`
	AgentIDs        []types.ID         `json:"agent_ids,omitempty"`
	MissionIDs      []types.ID         `json:"mission_ids,omitempty"`
	Statuses        []ExecutionStatus  `json:"statuses,omitempty"`
	Success         *bool              `json:"success,omitempty"`
	TargetTypes     []types.TargetType `json:"target_types,omitempty"`
	TargetProviders []types.Provider   `json:"target_providers,omitempty"`
	MinConfidence   *float64           `json:"min_confidence,omitempty"`
	After           *time.Time         `json:"after,omitempty"`  // CreatedAt after this time
	Before          *time.Time         `json:"before,omitempty"` // CreatedAt before this time
	Limit           int                `json:"limit,omitempty"`
	Offset          int                `json:"offset,omitempty"`
}

// ExecutionStats contains aggregate statistics for executions
type ExecutionStats struct {
	TotalExecutions   int           `json:"total_executions"`
	SuccessfulAttacks int           `json:"successful_attacks"`
	FailedExecutions  int           `json:"failed_executions"`
	SuccessRate       float64       `json:"success_rate"`
	AverageConfidence float64       `json:"average_confidence"`
	AverageDuration   time.Duration `json:"average_duration"`
	TotalTokensUsed   int           `json:"total_tokens_used"`
	TotalCost         float64       `json:"total_cost"`
	FindingsCreated   int           `json:"findings_created"`
	UniquePayloads    int           `json:"unique_payloads"`
	UniqueTargets     int           `json:"unique_targets"`
}

// executionStore implements ExecutionStore interface
type executionStore struct {
	db *database.DB
}

// NewExecutionStore creates a new ExecutionStore instance
func NewExecutionStore(db *database.DB) ExecutionStore {
	return &executionStore{db: db}
}

// Save inserts a new execution record
func (s *executionStore) Save(ctx context.Context, execution *Execution) error {
	// Marshal JSON fields
	paramsJSON, err := json.Marshal(execution.Parameters)
	if err != nil {
		return fmt.Errorf("failed to marshal parameters: %w", err)
	}

	indicatorsJSON, err := json.Marshal(execution.IndicatorsMatched)
	if err != nil {
		return fmt.Errorf("failed to marshal indicators: %w", err)
	}

	matchDetailsJSON, err := json.Marshal(execution.MatchDetails)
	if err != nil {
		return fmt.Errorf("failed to marshal match details: %w", err)
	}

	errorDetailsJSON, err := json.Marshal(execution.ErrorDetails)
	if err != nil {
		return fmt.Errorf("failed to marshal error details: %w", err)
	}

	metadataJSON, err := json.Marshal(execution.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	tagsJSON, err := json.Marshal(execution.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	query := `
		INSERT INTO payload_executions (
			id, payload_id, mission_id, target_id, agent_id, status,
			parameters, instantiated_text, response, response_time_ms,
			tokens_used, cost, success, indicators_matched, confidence_score,
			match_details, finding_id, finding_created, error_message,
			error_details, target_type, target_provider, target_model,
			created_at, started_at, completed_at, metadata, tags
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.ExecContext(ctx, query,
		execution.ID.String(),
		execution.PayloadID.String(),
		nullableID(execution.MissionID),
		execution.TargetID.String(),
		execution.AgentID.String(),
		string(execution.Status),
		string(paramsJSON),
		execution.InstantiatedText,
		execution.Response,
		execution.ResponseTime,
		execution.TokensUsed,
		execution.Cost,
		execution.Success,
		string(indicatorsJSON),
		execution.ConfidenceScore,
		string(matchDetailsJSON),
		nullableID(execution.FindingID),
		execution.FindingCreated,
		execution.ErrorMessage,
		string(errorDetailsJSON),
		string(execution.TargetType),
		string(execution.TargetProvider),
		execution.TargetModel,
		execution.CreatedAt,
		nullableTime(execution.StartedAt),
		nullableTime(execution.CompletedAt),
		string(metadataJSON),
		string(tagsJSON),
	)

	if err != nil {
		return fmt.Errorf("failed to insert execution: %w", err)
	}

	return nil
}

// Get retrieves an execution by ID
func (s *executionStore) Get(ctx context.Context, id types.ID) (*Execution, error) {
	query := `
		SELECT
			id, payload_id, mission_id, target_id, agent_id, status,
			parameters, instantiated_text, response, response_time_ms,
			tokens_used, cost, success, indicators_matched, confidence_score,
			match_details, finding_id, finding_created, error_message,
			error_details, target_type, target_provider, target_model,
			created_at, started_at, completed_at, metadata, tags
		FROM payload_executions
		WHERE id = ?
	`

	row := s.db.QueryRowContext(ctx, query, id.String())
	return s.scanExecution(row)
}

// List retrieves executions with optional filtering
func (s *executionStore) List(ctx context.Context, filter *ExecutionFilter) ([]*Execution, error) {
	if filter == nil {
		filter = &ExecutionFilter{
			Limit:  100,
			Offset: 0,
		}
	}

	if filter.Limit == 0 {
		filter.Limit = 100
	}

	query := `
		SELECT
			id, payload_id, mission_id, target_id, agent_id, status,
			parameters, instantiated_text, response, response_time_ms,
			tokens_used, cost, success, indicators_matched, confidence_score,
			match_details, finding_id, finding_created, error_message,
			error_details, target_type, target_provider, target_model,
			created_at, started_at, completed_at, metadata, tags
		FROM payload_executions
		WHERE 1=1
	`

	var args []interface{}
	conditions := s.buildFilterConditions(filter, &args)

	if len(conditions) > 0 {
		query += " AND " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query executions: %w", err)
	}
	defer rows.Close()

	var executions []*Execution
	for rows.Next() {
		execution, err := s.scanExecutionFromRows(rows)
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating executions: %w", err)
	}

	return executions, nil
}

// GetByPayload retrieves executions for a specific payload
func (s *executionStore) GetByPayload(ctx context.Context, payloadID types.ID, limit int) ([]*Execution, error) {
	if limit == 0 {
		limit = 100
	}

	filter := &ExecutionFilter{
		PayloadIDs: []types.ID{payloadID},
		Limit:      limit,
		Offset:     0,
	}

	return s.List(ctx, filter)
}

// GetByMission retrieves executions for a specific mission
func (s *executionStore) GetByMission(ctx context.Context, missionID types.ID, limit int) ([]*Execution, error) {
	if limit == 0 {
		limit = 100
	}

	filter := &ExecutionFilter{
		MissionIDs: []types.ID{missionID},
		Limit:      limit,
		Offset:     0,
	}

	return s.List(ctx, filter)
}

// GetStats retrieves aggregate statistics for analytics
func (s *executionStore) GetStats(ctx context.Context, filter *ExecutionFilter) (*ExecutionStats, error) {
	if filter == nil {
		filter = &ExecutionFilter{}
	}

	query := `
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as successful,
			SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END) as failed,
			AVG(CASE WHEN success = 1 THEN confidence_score ELSE NULL END) as avg_confidence,
			AVG(response_time_ms) as avg_duration,
			SUM(tokens_used) as total_tokens,
			SUM(cost) as total_cost,
			SUM(CASE WHEN finding_created = 1 THEN 1 ELSE 0 END) as findings,
			COUNT(DISTINCT payload_id) as unique_payloads,
			COUNT(DISTINCT target_id) as unique_targets
		FROM payload_executions
		WHERE 1=1
	`

	var args []interface{}
	conditions := s.buildFilterConditions(filter, &args)

	if len(conditions) > 0 {
		query += " AND " + strings.Join(conditions, " AND ")
	}

	var stats ExecutionStats
	var avgDurationMS sql.NullFloat64
	var avgConfidence sql.NullFloat64

	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&stats.TotalExecutions,
		&stats.SuccessfulAttacks,
		&stats.FailedExecutions,
		&avgConfidence,
		&avgDurationMS,
		&stats.TotalTokensUsed,
		&stats.TotalCost,
		&stats.FindingsCreated,
		&stats.UniquePayloads,
		&stats.UniqueTargets,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to query stats: %w", err)
	}

	// Calculate success rate
	if stats.TotalExecutions > 0 {
		stats.SuccessRate = float64(stats.SuccessfulAttacks) / float64(stats.TotalExecutions)
	}

	// Set average confidence
	if avgConfidence.Valid {
		stats.AverageConfidence = avgConfidence.Float64
	}

	// Set average duration
	if avgDurationMS.Valid {
		stats.AverageDuration = time.Duration(avgDurationMS.Float64) * time.Millisecond
	}

	return &stats, nil
}

// Update updates an existing execution record
func (s *executionStore) Update(ctx context.Context, execution *Execution) error {
	// Marshal JSON fields
	paramsJSON, err := json.Marshal(execution.Parameters)
	if err != nil {
		return fmt.Errorf("failed to marshal parameters: %w", err)
	}

	indicatorsJSON, err := json.Marshal(execution.IndicatorsMatched)
	if err != nil {
		return fmt.Errorf("failed to marshal indicators: %w", err)
	}

	matchDetailsJSON, err := json.Marshal(execution.MatchDetails)
	if err != nil {
		return fmt.Errorf("failed to marshal match details: %w", err)
	}

	errorDetailsJSON, err := json.Marshal(execution.ErrorDetails)
	if err != nil {
		return fmt.Errorf("failed to marshal error details: %w", err)
	}

	metadataJSON, err := json.Marshal(execution.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	tagsJSON, err := json.Marshal(execution.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	query := `
		UPDATE payload_executions SET
			status = ?,
			parameters = ?,
			instantiated_text = ?,
			response = ?,
			response_time_ms = ?,
			tokens_used = ?,
			cost = ?,
			success = ?,
			indicators_matched = ?,
			confidence_score = ?,
			match_details = ?,
			finding_id = ?,
			finding_created = ?,
			error_message = ?,
			error_details = ?,
			target_type = ?,
			target_provider = ?,
			target_model = ?,
			started_at = ?,
			completed_at = ?,
			metadata = ?,
			tags = ?
		WHERE id = ?
	`

	result, err := s.db.ExecContext(ctx, query,
		string(execution.Status),
		string(paramsJSON),
		execution.InstantiatedText,
		execution.Response,
		execution.ResponseTime,
		execution.TokensUsed,
		execution.Cost,
		execution.Success,
		string(indicatorsJSON),
		execution.ConfidenceScore,
		string(matchDetailsJSON),
		nullableID(execution.FindingID),
		execution.FindingCreated,
		execution.ErrorMessage,
		string(errorDetailsJSON),
		string(execution.TargetType),
		string(execution.TargetProvider),
		execution.TargetModel,
		nullableTime(execution.StartedAt),
		nullableTime(execution.CompletedAt),
		string(metadataJSON),
		string(tagsJSON),
		execution.ID.String(),
	)

	if err != nil {
		return fmt.Errorf("failed to update execution: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("execution not found: %s", execution.ID)
	}

	return nil
}

// Count returns the total number of executions matching the filter
func (s *executionStore) Count(ctx context.Context, filter *ExecutionFilter) (int, error) {
	if filter == nil {
		filter = &ExecutionFilter{}
	}

	query := "SELECT COUNT(*) FROM payload_executions WHERE 1=1"
	var args []interface{}
	conditions := s.buildFilterConditions(filter, &args)

	if len(conditions) > 0 {
		query += " AND " + strings.Join(conditions, " AND ")
	}

	var count int
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count executions: %w", err)
	}

	return count, nil
}

// buildFilterConditions builds SQL WHERE conditions from filter
func (s *executionStore) buildFilterConditions(filter *ExecutionFilter, args *[]interface{}) []string {
	var conditions []string

	if len(filter.PayloadIDs) > 0 {
		placeholders := make([]string, len(filter.PayloadIDs))
		for i, id := range filter.PayloadIDs {
			placeholders[i] = "?"
			*args = append(*args, id.String())
		}
		conditions = append(conditions, fmt.Sprintf("payload_id IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(filter.TargetIDs) > 0 {
		placeholders := make([]string, len(filter.TargetIDs))
		for i, id := range filter.TargetIDs {
			placeholders[i] = "?"
			*args = append(*args, id.String())
		}
		conditions = append(conditions, fmt.Sprintf("target_id IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(filter.AgentIDs) > 0 {
		placeholders := make([]string, len(filter.AgentIDs))
		for i, id := range filter.AgentIDs {
			placeholders[i] = "?"
			*args = append(*args, id.String())
		}
		conditions = append(conditions, fmt.Sprintf("agent_id IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(filter.MissionIDs) > 0 {
		placeholders := make([]string, len(filter.MissionIDs))
		for i, id := range filter.MissionIDs {
			placeholders[i] = "?"
			*args = append(*args, id.String())
		}
		conditions = append(conditions, fmt.Sprintf("mission_id IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(filter.Statuses) > 0 {
		placeholders := make([]string, len(filter.Statuses))
		for i, status := range filter.Statuses {
			placeholders[i] = "?"
			*args = append(*args, string(status))
		}
		conditions = append(conditions, fmt.Sprintf("status IN (%s)", strings.Join(placeholders, ",")))
	}

	if filter.Success != nil {
		conditions = append(conditions, "success = ?")
		*args = append(*args, *filter.Success)
	}

	if len(filter.TargetTypes) > 0 {
		placeholders := make([]string, len(filter.TargetTypes))
		for i, tt := range filter.TargetTypes {
			placeholders[i] = "?"
			*args = append(*args, string(tt))
		}
		conditions = append(conditions, fmt.Sprintf("target_type IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(filter.TargetProviders) > 0 {
		placeholders := make([]string, len(filter.TargetProviders))
		for i, provider := range filter.TargetProviders {
			placeholders[i] = "?"
			*args = append(*args, string(provider))
		}
		conditions = append(conditions, fmt.Sprintf("target_provider IN (%s)", strings.Join(placeholders, ",")))
	}

	if filter.MinConfidence != nil {
		conditions = append(conditions, "confidence_score >= ?")
		*args = append(*args, *filter.MinConfidence)
	}

	if filter.After != nil {
		conditions = append(conditions, "created_at > ?")
		*args = append(*args, *filter.After)
	}

	if filter.Before != nil {
		conditions = append(conditions, "created_at < ?")
		*args = append(*args, *filter.Before)
	}

	return conditions
}

// scanExecution scans a single execution from a query row
func (s *executionStore) scanExecution(row *sql.Row) (*Execution, error) {
	var execution Execution
	var id, payloadID, targetID, agentID, status string
	var missionID, findingID sql.NullString
	var paramsJSON, indicatorsJSON, matchDetailsJSON, errorDetailsJSON string
	var metadataJSON, tagsJSON string
	var startedAt, completedAt sql.NullTime
	var targetType, targetProvider, targetModel string

	err := row.Scan(
		&id, &payloadID, &missionID, &targetID, &agentID, &status,
		&paramsJSON, &execution.InstantiatedText, &execution.Response, &execution.ResponseTime,
		&execution.TokensUsed, &execution.Cost, &execution.Success, &indicatorsJSON, &execution.ConfidenceScore,
		&matchDetailsJSON, &findingID, &execution.FindingCreated, &execution.ErrorMessage,
		&errorDetailsJSON, &targetType, &targetProvider, &targetModel,
		&execution.CreatedAt, &startedAt, &completedAt, &metadataJSON, &tagsJSON,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("execution not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan execution: %w", err)
	}

	// Parse IDs
	execution.ID, _ = types.ParseID(id)
	execution.PayloadID, _ = types.ParseID(payloadID)
	execution.TargetID, _ = types.ParseID(targetID)
	execution.AgentID, _ = types.ParseID(agentID)

	if missionID.Valid {
		mid, _ := types.ParseID(missionID.String)
		execution.MissionID = &mid
	}

	if findingID.Valid {
		fid, _ := types.ParseID(findingID.String)
		execution.FindingID = &fid
	}

	execution.Status = ExecutionStatus(status)
	execution.TargetType = types.TargetType(targetType)
	execution.TargetProvider = types.Provider(targetProvider)
	execution.TargetModel = targetModel

	if startedAt.Valid {
		execution.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		execution.CompletedAt = &completedAt.Time
	}

	// Unmarshal JSON fields
	json.Unmarshal([]byte(paramsJSON), &execution.Parameters)
	json.Unmarshal([]byte(indicatorsJSON), &execution.IndicatorsMatched)
	json.Unmarshal([]byte(matchDetailsJSON), &execution.MatchDetails)
	json.Unmarshal([]byte(errorDetailsJSON), &execution.ErrorDetails)
	json.Unmarshal([]byte(metadataJSON), &execution.Metadata)
	json.Unmarshal([]byte(tagsJSON), &execution.Tags)

	return &execution, nil
}

// scanExecutionFromRows scans an execution from sql.Rows
func (s *executionStore) scanExecutionFromRows(rows *sql.Rows) (*Execution, error) {
	var execution Execution
	var id, payloadID, targetID, agentID, status string
	var missionID, findingID sql.NullString
	var paramsJSON, indicatorsJSON, matchDetailsJSON, errorDetailsJSON string
	var metadataJSON, tagsJSON string
	var startedAt, completedAt sql.NullTime
	var targetType, targetProvider, targetModel string

	err := rows.Scan(
		&id, &payloadID, &missionID, &targetID, &agentID, &status,
		&paramsJSON, &execution.InstantiatedText, &execution.Response, &execution.ResponseTime,
		&execution.TokensUsed, &execution.Cost, &execution.Success, &indicatorsJSON, &execution.ConfidenceScore,
		&matchDetailsJSON, &findingID, &execution.FindingCreated, &execution.ErrorMessage,
		&errorDetailsJSON, &targetType, &targetProvider, &targetModel,
		&execution.CreatedAt, &startedAt, &completedAt, &metadataJSON, &tagsJSON,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scan execution: %w", err)
	}

	// Parse IDs
	execution.ID, _ = types.ParseID(id)
	execution.PayloadID, _ = types.ParseID(payloadID)
	execution.TargetID, _ = types.ParseID(targetID)
	execution.AgentID, _ = types.ParseID(agentID)

	if missionID.Valid {
		mid, _ := types.ParseID(missionID.String)
		execution.MissionID = &mid
	}

	if findingID.Valid {
		fid, _ := types.ParseID(findingID.String)
		execution.FindingID = &fid
	}

	execution.Status = ExecutionStatus(status)
	execution.TargetType = types.TargetType(targetType)
	execution.TargetProvider = types.Provider(targetProvider)
	execution.TargetModel = targetModel

	if startedAt.Valid {
		execution.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		execution.CompletedAt = &completedAt.Time
	}

	// Unmarshal JSON fields
	json.Unmarshal([]byte(paramsJSON), &execution.Parameters)
	json.Unmarshal([]byte(indicatorsJSON), &execution.IndicatorsMatched)
	json.Unmarshal([]byte(matchDetailsJSON), &execution.MatchDetails)
	json.Unmarshal([]byte(errorDetailsJSON), &execution.ErrorDetails)
	json.Unmarshal([]byte(metadataJSON), &execution.Metadata)
	json.Unmarshal([]byte(tagsJSON), &execution.Tags)

	return &execution, nil
}

// Helper functions

func nullableID(id *types.ID) interface{} {
	if id == nil {
		return nil
	}
	return id.String()
}

func nullableTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}
