package payload

import (
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
)

// ExecutionStatus represents the status of a payload execution
type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusCancelled ExecutionStatus = "cancelled"
	ExecutionStatusTimeout   ExecutionStatus = "timeout"
)

// String returns the string representation of ExecutionStatus
func (s ExecutionStatus) String() string {
	return string(s)
}

// IsValid checks if the status is a valid value
func (s ExecutionStatus) IsValid() bool {
	switch s {
	case ExecutionStatusPending, ExecutionStatusRunning, ExecutionStatusCompleted,
		ExecutionStatusFailed, ExecutionStatusCancelled, ExecutionStatusTimeout:
		return true
	default:
		return false
	}
}

// Execution represents a single payload execution with complete tracking data
type Execution struct {
	ID        types.ID        `json:"id"`
	PayloadID types.ID        `json:"payload_id"`
	MissionID *types.ID       `json:"mission_id,omitempty"`
	TargetID  types.ID        `json:"target_id"`
	AgentID   types.ID        `json:"agent_id"`
	Status    ExecutionStatus `json:"status"`

	// Execution parameters
	Parameters       map[string]interface{} `json:"parameters,omitempty"`
	InstantiatedText string                 `json:"instantiated_text"` // Final payload text after parameter substitution

	// Response data
	Response       string  `json:"response,omitempty"`
	ResponseTime   int64   `json:"response_time_ms"` // milliseconds
	TokensUsed     int     `json:"tokens_used"`
	Cost           float64 `json:"cost"` // USD

	// Success evaluation
	Success          bool                   `json:"success"`
	IndicatorsMatched []string              `json:"indicators_matched,omitempty"` // Which indicators triggered
	ConfidenceScore  float64               `json:"confidence_score"`             // 0.0 - 1.0
	MatchDetails     map[string]interface{} `json:"match_details,omitempty"`      // Details about indicator matches

	// Finding attribution
	FindingID      *types.ID `json:"finding_id,omitempty"`       // Created finding if successful
	FindingCreated bool      `json:"finding_created"`

	// Error information
	ErrorMessage string                 `json:"error_message,omitempty"`
	ErrorDetails map[string]interface{} `json:"error_details,omitempty"`

	// Analytics metadata
	TargetType     types.TargetType `json:"target_type"`
	TargetProvider types.Provider   `json:"target_provider,omitempty"`
	TargetModel    string           `json:"target_model,omitempty"`

	// Timestamps
	CreatedAt   time.Time `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Additional metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Tags     []string               `json:"tags,omitempty"`
}

// NewExecution creates a new execution record
func NewExecution(payloadID, targetID, agentID types.ID) *Execution {
	now := time.Now()
	return &Execution{
		ID:               types.NewID(),
		PayloadID:        payloadID,
		TargetID:         targetID,
		AgentID:          agentID,
		Status:           ExecutionStatusPending,
		Parameters:       make(map[string]interface{}),
		IndicatorsMatched: []string{},
		MatchDetails:     make(map[string]interface{}),
		ErrorDetails:     make(map[string]interface{}),
		Metadata:         make(map[string]interface{}),
		Tags:             []string{},
		CreatedAt:        now,
	}
}

// Start marks the execution as started
func (e *Execution) Start() {
	e.Status = ExecutionStatusRunning
	now := time.Now()
	e.StartedAt = &now
}

// Complete marks the execution as completed
func (e *Execution) Complete() {
	e.Status = ExecutionStatusCompleted
	now := time.Now()
	e.CompletedAt = &now
}

// Fail marks the execution as failed with an error
func (e *Execution) Fail(err error) {
	e.Status = ExecutionStatusFailed
	e.ErrorMessage = err.Error()
	now := time.Now()
	e.CompletedAt = &now
}

// Cancel marks the execution as cancelled
func (e *Execution) Cancel() {
	e.Status = ExecutionStatusCancelled
	now := time.Now()
	e.CompletedAt = &now
}

// Timeout marks the execution as timed out
func (e *Execution) Timeout() {
	e.Status = ExecutionStatusTimeout
	e.ErrorMessage = "execution timed out"
	now := time.Now()
	e.CompletedAt = &now
}

// Duration returns the execution duration if completed
func (e *Execution) Duration() time.Duration {
	if e.StartedAt == nil || e.CompletedAt == nil {
		return 0
	}
	return e.CompletedAt.Sub(*e.StartedAt)
}

// IsCompleted returns true if execution has finished (any terminal state)
func (e *Execution) IsCompleted() bool {
	return e.Status == ExecutionStatusCompleted ||
		e.Status == ExecutionStatusFailed ||
		e.Status == ExecutionStatusCancelled ||
		e.Status == ExecutionStatusTimeout
}

// ExecutionRequest represents a request to execute a payload
type ExecutionRequest struct {
	PayloadID  types.ID               `json:"payload_id"`
	TargetID   types.ID               `json:"target_id"`
	AgentID    types.ID               `json:"agent_id"`
	MissionID  *types.ID              `json:"mission_id,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	Timeout    time.Duration          `json:"timeout,omitempty"`    // Execution timeout
	DryRun     bool                   `json:"dry_run"`              // Don't execute, just validate
	Tags       []string               `json:"tags,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// NewExecutionRequest creates a new execution request
func NewExecutionRequest(payloadID, targetID, agentID types.ID) *ExecutionRequest {
	return &ExecutionRequest{
		PayloadID:  payloadID,
		TargetID:   targetID,
		AgentID:    agentID,
		Parameters: make(map[string]interface{}),
		Timeout:    30 * time.Second, // Default 30 second timeout
		Tags:       []string{},
		Metadata:   make(map[string]interface{}),
	}
}

// WithMission sets the mission ID
func (r *ExecutionRequest) WithMission(missionID types.ID) *ExecutionRequest {
	r.MissionID = &missionID
	return r
}

// WithParameters sets the parameters
func (r *ExecutionRequest) WithParameters(params map[string]interface{}) *ExecutionRequest {
	r.Parameters = params
	return r
}

// WithTimeout sets the execution timeout
func (r *ExecutionRequest) WithTimeout(timeout time.Duration) *ExecutionRequest {
	r.Timeout = timeout
	return r
}

// WithDryRun enables dry run mode
func (r *ExecutionRequest) WithDryRun() *ExecutionRequest {
	r.DryRun = true
	return r
}

// WithTags sets the tags
func (r *ExecutionRequest) WithTags(tags ...string) *ExecutionRequest {
	r.Tags = tags
	return r
}

// WithMetadata sets custom metadata
func (r *ExecutionRequest) WithMetadata(metadata map[string]interface{}) *ExecutionRequest {
	r.Metadata = metadata
	return r
}

// ExecutionResult represents the result of a payload execution
type ExecutionResult struct {
	ExecutionID types.ID        `json:"execution_id"`
	Status      ExecutionStatus `json:"status"`

	// Outcome
	Success         bool    `json:"success"`
	ConfidenceScore float64 `json:"confidence_score"` // 0.0 - 1.0

	// Response data
	Response       string                 `json:"response,omitempty"`
	ResponseTime   time.Duration          `json:"response_time"`
	InstantiatedText string               `json:"instantiated_text"`

	// Indicators
	IndicatorsMatched []string              `json:"indicators_matched,omitempty"`
	MatchDetails     map[string]interface{} `json:"match_details,omitempty"`

	// Finding
	Finding        *agent.Finding `json:"finding,omitempty"`
	FindingCreated bool           `json:"finding_created"`

	// Metrics
	TokensUsed int     `json:"tokens_used"`
	Cost       float64 `json:"cost"`

	// Error information
	Error        error                  `json:"-"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	ErrorDetails map[string]interface{} `json:"error_details,omitempty"`

	// Timing
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt time.Time  `json:"completed_at"`
	Duration    time.Duration `json:"duration"`
}

// NewExecutionResult creates a new execution result
func NewExecutionResult(executionID types.ID) *ExecutionResult {
	now := time.Now()
	return &ExecutionResult{
		ExecutionID:       executionID,
		Status:            ExecutionStatusCompleted,
		IndicatorsMatched: []string{},
		MatchDetails:     make(map[string]interface{}),
		ErrorDetails:     make(map[string]interface{}),
		StartedAt:        now,
		CompletedAt:      now,
	}
}

// WithSuccess marks the execution as successful
func (r *ExecutionResult) WithSuccess(confidence float64) *ExecutionResult {
	r.Success = true
	r.ConfidenceScore = confidence
	return r
}

// WithFinding attaches a finding to the result
func (r *ExecutionResult) WithFinding(finding *agent.Finding) *ExecutionResult {
	r.Finding = finding
	r.FindingCreated = finding != nil
	return r
}

// WithError sets the error information
func (r *ExecutionResult) WithError(err error) *ExecutionResult {
	r.Status = ExecutionStatusFailed
	r.Error = err
	if err != nil {
		r.ErrorMessage = err.Error()
	}
	return r
}

// DryRunResult represents the result of a dry run (validation without execution)
type DryRunResult struct {
	Valid              bool                   `json:"valid"`
	InstantiatedText   string                 `json:"instantiated_text"`
	ParameterErrors    map[string]string      `json:"parameter_errors,omitempty"`
	ValidationErrors   []string               `json:"validation_errors,omitempty"`
	EstimatedTokens    int                    `json:"estimated_tokens"`
	EstimatedCost      float64                `json:"estimated_cost"`
	Warnings           []string               `json:"warnings,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
}

// NewDryRunResult creates a new dry run result
func NewDryRunResult() *DryRunResult {
	return &DryRunResult{
		Valid:            true,
		ParameterErrors:  make(map[string]string),
		ValidationErrors: []string{},
		Warnings:         []string{},
		Metadata:         make(map[string]interface{}),
	}
}

// AddParameterError adds a parameter validation error
func (d *DryRunResult) AddParameterError(param, message string) {
	d.Valid = false
	d.ParameterErrors[param] = message
}

// AddValidationError adds a general validation error
func (d *DryRunResult) AddValidationError(message string) {
	d.Valid = false
	d.ValidationErrors = append(d.ValidationErrors, message)
}

// AddWarning adds a warning (doesn't invalidate)
func (d *DryRunResult) AddWarning(message string) {
	d.Warnings = append(d.Warnings, message)
}

// ChainExecutionStatus represents the status of a chain execution
type ChainExecutionStatus string

const (
	ChainStatusPending   ChainExecutionStatus = "pending"
	ChainStatusRunning   ChainExecutionStatus = "running"
	ChainStatusPaused    ChainExecutionStatus = "paused"
	ChainStatusCompleted ChainExecutionStatus = "completed"
	ChainStatusFailed    ChainExecutionStatus = "failed"
	ChainStatusCancelled ChainExecutionStatus = "cancelled"
)

// String returns the string representation of ChainExecutionStatus
func (s ChainExecutionStatus) String() string {
	return string(s)
}

// IsValid checks if the status is a valid value
func (s ChainExecutionStatus) IsValid() bool {
	switch s {
	case ChainStatusPending, ChainStatusRunning, ChainStatusPaused,
		ChainStatusCompleted, ChainStatusFailed, ChainStatusCancelled:
		return true
	default:
		return false
	}
}

// ChainExecutionRequest represents a request to execute an attack chain
type ChainExecutionRequest struct {
	ChainID    types.ID               `json:"chain_id"`
	TargetID   types.ID               `json:"target_id"`
	AgentID    types.ID               `json:"agent_id"`
	MissionID  *types.ID              `json:"mission_id,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"` // Chain-level parameters
	Timeout    time.Duration          `json:"timeout,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`

	// Execution control
	StopOnFailure    bool `json:"stop_on_failure"`     // Stop entire chain on any stage failure
	ContinueOnError  bool `json:"continue_on_error"`   // Continue to next stage on error
	MaxRetries       int  `json:"max_retries"`         // Max retries per stage
	ParallelStages   bool `json:"parallel_stages"`     // Allow parallel stage execution
}

// NewChainExecutionRequest creates a new chain execution request
func NewChainExecutionRequest(chainID, targetID, agentID types.ID) *ChainExecutionRequest {
	return &ChainExecutionRequest{
		ChainID:          chainID,
		TargetID:         targetID,
		AgentID:          agentID,
		Parameters:       make(map[string]interface{}),
		Timeout:          5 * time.Minute, // Default 5 minute timeout for chains
		Tags:             []string{},
		Metadata:         make(map[string]interface{}),
		StopOnFailure:    true,
		ContinueOnError:  false,
		MaxRetries:       0,
		ParallelStages:   false,
	}
}

// ChainProgress represents the progress of a chain execution
type ChainProgress struct {
	ChainExecutionID types.ID             `json:"chain_execution_id"`
	ChainID          types.ID             `json:"chain_id"`
	Status           ChainExecutionStatus `json:"status"`

	// Progress tracking
	TotalStages      int            `json:"total_stages"`
	CompletedStages  int            `json:"completed_stages"`
	CurrentStageIndex int           `json:"current_stage_index,omitempty"`
	CurrentStageID   *types.ID      `json:"current_stage_id,omitempty"`

	// Stage results
	StageResults []StageResult `json:"stage_results"`

	// Aggregated metrics
	TotalExecutions    int           `json:"total_executions"`
	SuccessfulAttacks  int           `json:"successful_attacks"`
	FailedExecutions   int           `json:"failed_executions"`
	TotalFindings      int           `json:"total_findings"`
	TotalDuration      time.Duration `json:"total_duration"`
	TotalTokensUsed    int           `json:"total_tokens_used"`
	TotalCost          float64       `json:"total_cost"`

	// Error tracking
	ErrorMessage string                 `json:"error_message,omitempty"`
	ErrorDetails map[string]interface{} `json:"error_details,omitempty"`

	// Timestamps
	StartedAt   time.Time  `json:"started_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Context for next stages
	ChainContext map[string]interface{} `json:"chain_context,omitempty"` // Passed between stages
}

// NewChainProgress creates a new chain progress tracker
func NewChainProgress(chainExecutionID, chainID types.ID, totalStages int) *ChainProgress {
	now := time.Now()
	return &ChainProgress{
		ChainExecutionID: chainExecutionID,
		ChainID:          chainID,
		Status:           ChainStatusPending,
		TotalStages:      totalStages,
		StageResults:     make([]StageResult, 0, totalStages),
		ErrorDetails:     make(map[string]interface{}),
		ChainContext:     make(map[string]interface{}),
		StartedAt:        now,
		UpdatedAt:        now,
	}
}

// Start marks the chain execution as started
func (p *ChainProgress) Start() {
	p.Status = ChainStatusRunning
	now := time.Now()
	p.StartedAt = now
	p.UpdatedAt = now
}

// Complete marks the chain execution as completed
func (p *ChainProgress) Complete() {
	p.Status = ChainStatusCompleted
	now := time.Now()
	p.CompletedAt = &now
	p.UpdatedAt = now
}

// Fail marks the chain execution as failed
func (p *ChainProgress) Fail(err error) {
	p.Status = ChainStatusFailed
	if err != nil {
		p.ErrorMessage = err.Error()
	}
	now := time.Now()
	p.CompletedAt = &now
	p.UpdatedAt = now
}

// Pause marks the chain execution as paused
func (p *ChainProgress) Pause() {
	p.Status = ChainStatusPaused
	p.UpdatedAt = time.Now()
}

// Resume marks the chain execution as running again
func (p *ChainProgress) Resume() {
	p.Status = ChainStatusRunning
	p.UpdatedAt = time.Now()
}

// AddStageResult adds a completed stage result
func (p *ChainProgress) AddStageResult(result StageResult) {
	p.StageResults = append(p.StageResults, result)
	p.CompletedStages++
	p.UpdatedAt = time.Now()

	// Update aggregated metrics
	p.TotalExecutions++
	if result.Success {
		p.SuccessfulAttacks++
	} else {
		p.FailedExecutions++
	}
	if result.FindingCreated {
		p.TotalFindings++
	}
	p.TotalDuration += result.Duration
	p.TotalTokensUsed += result.TokensUsed
	p.TotalCost += result.Cost
}

// SetCurrentStage sets the currently executing stage
func (p *ChainProgress) SetCurrentStage(stageIndex int, stageID types.ID) {
	p.CurrentStageIndex = stageIndex
	p.CurrentStageID = &stageID
	p.UpdatedAt = time.Now()
}

// PercentComplete returns the percentage of stages completed
func (p *ChainProgress) PercentComplete() float64 {
	if p.TotalStages == 0 {
		return 0.0
	}
	return float64(p.CompletedStages) / float64(p.TotalStages) * 100.0
}

// StageResult represents the result of a single stage in a chain
type StageResult struct {
	StageID      types.ID        `json:"stage_id"`
	StageName    string          `json:"stage_name"`
	StageIndex   int             `json:"stage_index"`
	PayloadID    types.ID        `json:"payload_id"`
	ExecutionID  types.ID        `json:"execution_id"`
	Status       ExecutionStatus `json:"status"`

	// Outcome
	Success         bool    `json:"success"`
	ConfidenceScore float64 `json:"confidence_score"`

	// Finding
	FindingID      *types.ID `json:"finding_id,omitempty"`
	FindingCreated bool      `json:"finding_created"`

	// Response data
	Response         string                 `json:"response,omitempty"`
	InstantiatedText string                 `json:"instantiated_text"`
	IndicatorsMatched []string              `json:"indicators_matched,omitempty"`

	// Metrics
	Duration   time.Duration `json:"duration"`
	TokensUsed int           `json:"tokens_used"`
	Cost       float64       `json:"cost"`

	// Error information
	ErrorMessage string                 `json:"error_message,omitempty"`
	ErrorDetails map[string]interface{} `json:"error_details,omitempty"`

	// Timing
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`

	// Output context for next stages
	OutputContext map[string]interface{} `json:"output_context,omitempty"`
}

// NewStageResult creates a new stage result
func NewStageResult(stageID types.ID, stageName string, stageIndex int, payloadID, executionID types.ID) *StageResult {
	now := time.Now()
	return &StageResult{
		StageID:           stageID,
		StageName:         stageName,
		StageIndex:        stageIndex,
		PayloadID:         payloadID,
		ExecutionID:       executionID,
		Status:            ExecutionStatusCompleted,
		IndicatorsMatched: []string{},
		ErrorDetails:      make(map[string]interface{}),
		OutputContext:     make(map[string]interface{}),
		StartedAt:         now,
		CompletedAt:       now,
	}
}

// ChainResult represents the complete result of a chain execution
type ChainResult struct {
	ChainExecutionID types.ID             `json:"chain_execution_id"`
	ChainID          types.ID             `json:"chain_id"`
	Status           ChainExecutionStatus `json:"status"`

	// Outcome
	Success        bool          `json:"success"`        // True if all stages successful
	StagesExecuted int           `json:"stages_executed"`
	StageResults   []StageResult `json:"stage_results"`

	// Aggregated findings
	Findings       []*agent.Finding `json:"findings,omitempty"`
	TotalFindings  int              `json:"total_findings"`

	// Aggregated metrics
	SuccessfulStages int           `json:"successful_stages"`
	FailedStages     int           `json:"failed_stages"`
	TotalDuration    time.Duration `json:"total_duration"`
	TotalTokensUsed  int           `json:"total_tokens_used"`
	TotalCost        float64       `json:"total_cost"`

	// Error information
	ErrorMessage string                 `json:"error_message,omitempty"`
	ErrorDetails map[string]interface{} `json:"error_details,omitempty"`

	// Timing
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`

	// Final context
	FinalContext map[string]interface{} `json:"final_context,omitempty"`
}

// NewChainResult creates a new chain result
func NewChainResult(chainExecutionID, chainID types.ID) *ChainResult {
	now := time.Now()
	return &ChainResult{
		ChainExecutionID: chainExecutionID,
		ChainID:          chainID,
		Status:           ChainStatusCompleted,
		StageResults:     []StageResult{},
		Findings:         []*agent.Finding{},
		ErrorDetails:     make(map[string]interface{}),
		FinalContext:     make(map[string]interface{}),
		StartedAt:        now,
		CompletedAt:      now,
	}
}

// AddStageResult adds a stage result to the chain result
func (r *ChainResult) AddStageResult(result StageResult) {
	r.StageResults = append(r.StageResults, result)
	r.StagesExecuted++

	// Update aggregated metrics
	if result.Success {
		r.SuccessfulStages++
	} else {
		r.FailedStages++
	}
	if result.FindingCreated {
		r.TotalFindings++
	}
	r.TotalDuration += result.Duration
	r.TotalTokensUsed += result.TokensUsed
	r.TotalCost += result.Cost
}

// IsSuccess determines if the chain was successful overall
func (r *ChainResult) IsSuccess() bool {
	return r.Status == ChainStatusCompleted && r.FailedStages == 0
}
