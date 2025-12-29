package attack

import (
	"time"

	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/types"
)

// AttackStatus represents the final status of an attack mission.
type AttackStatus string

const (
	// AttackStatusSuccess indicates the attack completed successfully without findings
	AttackStatusSuccess AttackStatus = "success"

	// AttackStatusFindings indicates the attack completed and found vulnerabilities
	AttackStatusFindings AttackStatus = "findings"

	// AttackStatusFailed indicates the attack failed due to an error
	AttackStatusFailed AttackStatus = "failed"

	// AttackStatusTimeout indicates the attack exceeded time limits
	AttackStatusTimeout AttackStatus = "timeout"

	// AttackStatusCancelled indicates the attack was cancelled by the user
	AttackStatusCancelled AttackStatus = "cancelled"
)

// String returns the string representation of AttackStatus
func (s AttackStatus) String() string {
	return string(s)
}

// IsValid checks if the status is a valid value
func (s AttackStatus) IsValid() bool {
	switch s {
	case AttackStatusSuccess, AttackStatusFindings, AttackStatusFailed,
		AttackStatusTimeout, AttackStatusCancelled:
		return true
	default:
		return false
	}
}

// IsSuccessful returns true if the attack completed successfully (with or without findings)
func (s AttackStatus) IsSuccessful() bool {
	return s == AttackStatusSuccess || s == AttackStatusFindings
}

// AttackResult represents the complete result of an attack mission execution.
// It contains mission metadata, execution metrics, discovered findings, and final status.
type AttackResult struct {
	// Mission info (if persisted to database)
	MissionID *types.ID `json:"mission_id,omitempty"`
	Persisted bool      `json:"persisted"`

	// Execution metrics
	Duration   time.Duration `json:"duration"`
	TurnsUsed  int           `json:"turns_used"`
	TokensUsed int64         `json:"tokens_used"`

	// Findings discovered during the attack
	Findings           []finding.EnhancedFinding `json:"findings"`
	FindingsBySeverity map[string]int            `json:"findings_by_severity"`

	// Final status and error information
	Status   AttackStatus `json:"status"`
	Error    error        `json:"error,omitempty"`
	ExitCode int          `json:"exit_code"`
}

// NewAttackResult creates a new AttackResult with default values
func NewAttackResult() *AttackResult {
	return &AttackResult{
		Findings:           []finding.EnhancedFinding{},
		FindingsBySeverity: make(map[string]int),
		Status:             AttackStatusSuccess,
		ExitCode:           0,
	}
}

// WithMissionID sets the mission ID and marks the result as persisted
func (r *AttackResult) WithMissionID(id types.ID) *AttackResult {
	r.MissionID = &id
	r.Persisted = true
	return r
}

// WithDuration sets the execution duration
func (r *AttackResult) WithDuration(d time.Duration) *AttackResult {
	r.Duration = d
	return r
}

// WithTurnsUsed sets the number of turns used
func (r *AttackResult) WithTurnsUsed(turns int) *AttackResult {
	r.TurnsUsed = turns
	return r
}

// WithTokensUsed sets the number of tokens consumed
func (r *AttackResult) WithTokensUsed(tokens int64) *AttackResult {
	r.TokensUsed = tokens
	return r
}

// WithStatus sets the final status
func (r *AttackResult) WithStatus(status AttackStatus) *AttackResult {
	r.Status = status
	return r
}

// WithError sets the error and marks the result as failed
func (r *AttackResult) WithError(err error) *AttackResult {
	r.Error = err
	r.Status = AttackStatusFailed
	if r.ExitCode == 0 {
		r.ExitCode = 1
	}
	return r
}

// WithExitCode sets the exit code
func (r *AttackResult) WithExitCode(code int) *AttackResult {
	r.ExitCode = code
	return r
}

// AddFinding adds a finding to the result and updates severity statistics
func (r *AttackResult) AddFinding(f finding.EnhancedFinding) {
	r.Findings = append(r.Findings, f)

	// Update severity count
	severityKey := string(f.Severity)
	r.FindingsBySeverity[severityKey]++

	// Update status to indicate findings were discovered
	if r.Status == AttackStatusSuccess {
		r.Status = AttackStatusFindings
	}
}

// AddFindings adds multiple findings and updates severity statistics
func (r *AttackResult) AddFindings(findings []finding.EnhancedFinding) {
	for _, f := range findings {
		r.AddFinding(f)
	}
}

// HasFindings returns true if any findings were discovered
func (r *AttackResult) HasFindings() bool {
	return len(r.Findings) > 0
}

// FindingCount returns the total number of findings
func (r *AttackResult) FindingCount() int {
	return len(r.Findings)
}

// CriticalFindingCount returns the number of critical findings
func (r *AttackResult) CriticalFindingCount() int {
	return r.FindingsBySeverity["critical"]
}

// HighFindingCount returns the number of high severity findings
func (r *AttackResult) HighFindingCount() int {
	return r.FindingsBySeverity["high"]
}

// MediumFindingCount returns the number of medium severity findings
func (r *AttackResult) MediumFindingCount() int {
	return r.FindingsBySeverity["medium"]
}

// LowFindingCount returns the number of low severity findings
func (r *AttackResult) LowFindingCount() int {
	return r.FindingsBySeverity["low"]
}

// InfoFindingCount returns the number of informational findings
func (r *AttackResult) InfoFindingCount() int {
	return r.FindingsBySeverity["info"]
}

// IsSuccessful returns true if the attack completed successfully
func (r *AttackResult) IsSuccessful() bool {
	return r.Status.IsSuccessful()
}

// IsFailed returns true if the attack failed
func (r *AttackResult) IsFailed() bool {
	return r.Status == AttackStatusFailed
}

// IsTimeout returns true if the attack timed out
func (r *AttackResult) IsTimeout() bool {
	return r.Status == AttackStatusTimeout
}

// IsCancelled returns true if the attack was cancelled
func (r *AttackResult) IsCancelled() bool {
	return r.Status == AttackStatusCancelled
}

// HasCriticalFindings returns true if any critical findings were discovered
func (r *AttackResult) HasCriticalFindings() bool {
	return r.CriticalFindingCount() > 0
}

// HasHighFindings returns true if any high severity findings were discovered
func (r *AttackResult) HasHighFindings() bool {
	return r.HighFindingCount() > 0
}

// ShouldExitNonZero returns true if the result should cause a non-zero exit code
func (r *AttackResult) ShouldExitNonZero() bool {
	return r.IsFailed() || r.IsTimeout() || r.IsCancelled() || r.HasCriticalFindings()
}
