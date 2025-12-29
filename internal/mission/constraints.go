package mission

import (
	"context"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
)

// MissionConstraints defines execution boundaries for a mission.
// Constraints are enforced during mission execution to prevent runaway costs,
// excessive findings, or operations outside acceptable parameters.
type MissionConstraints struct {
	// MaxDuration is the maximum allowed execution time.
	// If exceeded, the mission will be failed with a timeout error.
	MaxDuration time.Duration `json:"max_duration,omitempty" yaml:"max_duration,omitempty"`

	// MaxFindings is the maximum number of findings before stopping.
	// When reached, the mission will complete with findings limit status.
	MaxFindings int `json:"max_findings,omitempty" yaml:"max_findings,omitempty"`

	// SeverityThreshold is the minimum severity level to trigger action.
	// If a finding with this severity or higher is discovered, the configured action is taken.
	SeverityThreshold agent.FindingSeverity `json:"severity_threshold,omitempty" yaml:"severity_threshold,omitempty"`

	// SeverityAction is the action to take when severity threshold is exceeded.
	SeverityAction ConstraintAction `json:"severity_action,omitempty" yaml:"severity_action,omitempty"`

	// RequireEvidence indicates whether findings must include evidence.
	// If true, findings without evidence will be rejected.
	RequireEvidence bool `json:"require_evidence,omitempty" yaml:"require_evidence,omitempty"`

	// MaxTokens is the maximum total token usage allowed.
	// When exceeded, the mission will pause for budget approval.
	MaxTokens int64 `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`

	// MaxCost is the maximum cost in dollars for LLM usage.
	// When exceeded, the mission will pause for budget approval.
	MaxCost float64 `json:"max_cost,omitempty" yaml:"max_cost,omitempty"`
}

// ConstraintAction defines what action to take when a constraint is violated.
type ConstraintAction string

const (
	// ConstraintActionPause suspends mission execution until manual intervention.
	ConstraintActionPause ConstraintAction = "pause"

	// ConstraintActionFail immediately fails the mission with constraint violation error.
	ConstraintActionFail ConstraintAction = "fail"
)

// String returns the string representation of the constraint action.
func (a ConstraintAction) String() string {
	return string(a)
}

// ConstraintViolation describes a violated constraint.
// This is returned when a constraint check fails and contains information
// about what was violated and what action should be taken.
type ConstraintViolation struct {
	// Constraint is the name of the violated constraint.
	Constraint string `json:"constraint"`

	// Message is a human-readable description of the violation.
	Message string `json:"message"`

	// Action is the action to take (pause or fail).
	Action ConstraintAction `json:"action"`

	// CurrentValue is the current value that violated the constraint (optional).
	CurrentValue any `json:"current_value,omitempty"`

	// ThresholdValue is the threshold that was exceeded (optional).
	ThresholdValue any `json:"threshold_value,omitempty"`
}

// Error implements the error interface for ConstraintViolation.
func (v *ConstraintViolation) Error() string {
	return fmt.Sprintf("constraint violation: %s - %s (action: %s)", v.Constraint, v.Message, v.Action)
}

// ConstraintChecker evaluates mission constraints against current metrics.
type ConstraintChecker interface {
	// Check evaluates all constraints and returns a violation if any constraint is violated.
	// Returns nil if all constraints are satisfied.
	Check(ctx context.Context, constraints *MissionConstraints, metrics *MissionMetrics) (*ConstraintViolation, error)
}

// DefaultConstraintChecker implements ConstraintChecker with standard validation logic.
type DefaultConstraintChecker struct{}

// NewDefaultConstraintChecker creates a new DefaultConstraintChecker.
func NewDefaultConstraintChecker() *DefaultConstraintChecker {
	return &DefaultConstraintChecker{}
}

// Check evaluates all constraints and returns the first violation found.
// Constraints are checked in order of severity (cost, duration, findings, severity).
func (c *DefaultConstraintChecker) Check(ctx context.Context, constraints *MissionConstraints, metrics *MissionMetrics) (*ConstraintViolation, error) {
	if constraints == nil || metrics == nil {
		return nil, nil
	}

	// Check max cost constraint (highest priority)
	if constraints.MaxCost > 0 && metrics.TotalCost > constraints.MaxCost {
		return &ConstraintViolation{
			Constraint:     "max_cost",
			Message:        fmt.Sprintf("Mission cost %.2f exceeds maximum allowed cost %.2f", metrics.TotalCost, constraints.MaxCost),
			Action:         ConstraintActionPause, // Cost violations always pause for approval
			CurrentValue:   metrics.TotalCost,
			ThresholdValue: constraints.MaxCost,
		}, nil
	}

	// Check max tokens constraint
	if constraints.MaxTokens > 0 && metrics.TotalTokens > constraints.MaxTokens {
		return &ConstraintViolation{
			Constraint:     "max_tokens",
			Message:        fmt.Sprintf("Mission tokens %d exceeds maximum allowed tokens %d", metrics.TotalTokens, constraints.MaxTokens),
			Action:         ConstraintActionPause, // Token violations always pause for approval
			CurrentValue:   metrics.TotalTokens,
			ThresholdValue: constraints.MaxTokens,
		}, nil
	}

	// Check max duration constraint
	if constraints.MaxDuration > 0 && metrics.Duration > constraints.MaxDuration {
		return &ConstraintViolation{
			Constraint:     "max_duration",
			Message:        fmt.Sprintf("Mission duration %s exceeds maximum allowed duration %s", metrics.Duration, constraints.MaxDuration),
			Action:         ConstraintActionFail, // Duration violations always fail
			CurrentValue:   metrics.Duration.String(),
			ThresholdValue: constraints.MaxDuration.String(),
		}, nil
	}

	// Check max findings constraint
	if constraints.MaxFindings > 0 && metrics.TotalFindings >= constraints.MaxFindings {
		return &ConstraintViolation{
			Constraint:     "max_findings",
			Message:        fmt.Sprintf("Mission findings %d reached maximum allowed findings %d", metrics.TotalFindings, constraints.MaxFindings),
			Action:         ConstraintActionPause, // Findings limit pauses to allow review
			CurrentValue:   metrics.TotalFindings,
			ThresholdValue: constraints.MaxFindings,
		}, nil
	}

	// Check severity threshold constraint
	if constraints.SeverityThreshold != "" && metrics.FindingsBySeverity != nil {
		if c.checkSeverityThreshold(constraints.SeverityThreshold, metrics.FindingsBySeverity) {
			action := constraints.SeverityAction
			if action == "" {
				action = ConstraintActionPause // Default action for severity violations
			}

			return &ConstraintViolation{
				Constraint:     "severity_threshold",
				Message:        fmt.Sprintf("Finding with severity %s or higher discovered", constraints.SeverityThreshold),
				Action:         action,
				CurrentValue:   metrics.FindingsBySeverity,
				ThresholdValue: constraints.SeverityThreshold,
			}, nil
		}
	}

	return nil, nil
}

// checkSeverityThreshold checks if any findings meet or exceed the severity threshold.
func (c *DefaultConstraintChecker) checkSeverityThreshold(threshold agent.FindingSeverity, findingsBySeverity map[string]int) bool {
	// Define severity ordering (higher index = more severe)
	severities := []agent.FindingSeverity{
		agent.SeverityInfo,
		agent.SeverityLow,
		agent.SeverityMedium,
		agent.SeverityHigh,
		agent.SeverityCritical,
	}

	// Find threshold index
	thresholdIdx := -1
	for i, sev := range severities {
		if sev == threshold {
			thresholdIdx = i
			break
		}
	}

	if thresholdIdx == -1 {
		return false // Invalid threshold
	}

	// Check if any findings at or above threshold exist
	for i := thresholdIdx; i < len(severities); i++ {
		if count, ok := findingsBySeverity[string(severities[i])]; ok && count > 0 {
			return true
		}
	}

	return false
}

// DefaultConstraints returns a reasonable set of default constraints.
func DefaultConstraints() *MissionConstraints {
	return &MissionConstraints{
		MaxDuration:       24 * time.Hour,           // 24 hour max runtime
		MaxFindings:       1000,                     // Max 1000 findings
		SeverityThreshold: agent.SeverityCritical,   // Alert on critical findings
		SeverityAction:    ConstraintActionPause,    // Pause for critical findings
		RequireEvidence:   false,                    // Don't require evidence by default
		MaxTokens:         10000000,                 // 10M tokens (generous default)
		MaxCost:           100.0,                    // $100 max cost
	}
}

// Validate checks if the constraints are valid.
func (c *MissionConstraints) Validate() error {
	if c.MaxDuration < 0 {
		return fmt.Errorf("max_duration cannot be negative")
	}
	if c.MaxFindings < 0 {
		return fmt.Errorf("max_findings cannot be negative")
	}
	if c.MaxTokens < 0 {
		return fmt.Errorf("max_tokens cannot be negative")
	}
	if c.MaxCost < 0 {
		return fmt.Errorf("max_cost cannot be negative")
	}

	// Validate severity threshold if set
	if c.SeverityThreshold != "" {
		validSeverities := []agent.FindingSeverity{
			agent.SeverityInfo,
			agent.SeverityLow,
			agent.SeverityMedium,
			agent.SeverityHigh,
			agent.SeverityCritical,
		}
		valid := false
		for _, sev := range validSeverities {
			if c.SeverityThreshold == sev {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid severity_threshold: %s", c.SeverityThreshold)
		}
	}

	// Validate severity action if set
	if c.SeverityAction != "" {
		if c.SeverityAction != ConstraintActionPause && c.SeverityAction != ConstraintActionFail {
			return fmt.Errorf("invalid severity_action: %s (must be 'pause' or 'fail')", c.SeverityAction)
		}
	}

	return nil
}

// Ensure DefaultConstraintChecker implements ConstraintChecker at compile time
var _ ConstraintChecker = (*DefaultConstraintChecker)(nil)
