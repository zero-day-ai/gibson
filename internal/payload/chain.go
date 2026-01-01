package payload

import (
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// FailureAction defines how to handle stage failures in an attack chain.
type FailureAction string

const (
	// FailureActionStop halts the entire chain execution on stage failure.
	FailureActionStop FailureAction = "stop"

	// FailureActionContinue proceeds to the next stage despite the failure.
	FailureActionContinue FailureAction = "continue"

	// FailureActionSkip skips dependent stages but continues with independent stages.
	FailureActionSkip FailureAction = "skip"
)

// String returns the string representation of the failure action.
func (fa FailureAction) String() string {
	return string(fa)
}

// IsValid checks if the failure action is a valid value.
func (fa FailureAction) IsValid() bool {
	switch fa {
	case FailureActionStop, FailureActionContinue, FailureActionSkip:
		return true
	default:
		return false
	}
}

// ConditionType defines the type of condition to evaluate for stage execution.
type ConditionType string

const (
	// ConditionTypeExpression evaluates a boolean expression against stage context.
	ConditionTypeExpression ConditionType = "expression"

	// ConditionTypeSuccessRate checks if previous stages meet a success rate threshold.
	ConditionTypeSuccessRate ConditionType = "success_rate"

	// ConditionTypeFindingSeverity checks if findings meet a severity threshold.
	ConditionTypeFindingSeverity ConditionType = "finding_severity"

	// ConditionTypeCustom allows custom condition evaluation via plugins.
	ConditionTypeCustom ConditionType = "custom"
)

// String returns the string representation of the condition type.
func (ct ConditionType) String() string {
	return string(ct)
}

// IsValid checks if the condition type is a valid value.
func (ct ConditionType) IsValid() bool {
	switch ct {
	case ConditionTypeExpression, ConditionTypeSuccessRate,
		ConditionTypeFindingSeverity, ConditionTypeCustom:
		return true
	default:
		return false
	}
}

// StageCondition defines a conditional check for stage execution.
// Conditions can evaluate previous stage results, context variables, or custom logic.
type StageCondition struct {
	// Type specifies the kind of condition to evaluate.
	Type ConditionType `json:"type" yaml:"type"`

	// Expression is the condition expression to evaluate (for ConditionTypeExpression).
	// Supports syntax like: "stages.recon.success == true && stages.recon.findings_count > 0"
	Expression string `json:"expression,omitempty" yaml:"expression,omitempty"`

	// Threshold is the numeric threshold for rate-based conditions (0.0 - 1.0).
	Threshold float64 `json:"threshold,omitempty" yaml:"threshold,omitempty"`

	// CustomFunc is the name of a custom condition function (for ConditionTypeCustom).
	CustomFunc string `json:"custom_func,omitempty" yaml:"custom_func,omitempty"`

	// Parameters provides additional parameters for condition evaluation.
	Parameters map[string]any `json:"parameters,omitempty" yaml:"parameters,omitempty"`
}

// StageAction defines an action to take based on stage results.
// This enables conditional branching in attack chains.
type StageAction struct {
	// OnSuccess defines the next stage IDs to execute if this stage succeeds.
	OnSuccess []string `json:"on_success,omitempty" yaml:"on_success,omitempty"`

	// OnFailure defines the next stage IDs to execute if this stage fails.
	OnFailure []string `json:"on_failure,omitempty" yaml:"on_failure,omitempty"`

	// OnCondition defines conditional branches based on stage results.
	// Each condition is evaluated in order; the first matching condition's actions execute.
	OnCondition []ConditionalBranch `json:"on_condition,omitempty" yaml:"on_condition,omitempty"`
}

// ConditionalBranch represents a conditional branch in stage execution.
type ConditionalBranch struct {
	// Condition defines the condition to evaluate.
	Condition StageCondition `json:"condition" yaml:"condition"`

	// NextStages defines which stages to execute if the condition is true.
	NextStages []string `json:"next_stages" yaml:"next_stages"`
}

// ChainStage represents a single stage in an attack chain.
// Each stage executes a payload with specific parameters and can branch based on results.
type ChainStage struct {
	// ID is the unique identifier for this stage within the chain.
	ID string `json:"id" yaml:"id"`

	// Name is a human-readable name for this stage.
	Name string `json:"name" yaml:"name"`

	// Description provides context about what this stage does.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// PayloadID references the payload to execute in this stage.
	PayloadID types.ID `json:"payload_id" yaml:"payload_id"`

	// Parameters provides parameter overrides for the payload.
	// These merge with and override the payload's default parameters.
	Parameters map[string]any `json:"parameters,omitempty" yaml:"parameters,omitempty"`

	// Condition defines an optional pre-execution condition.
	// If specified, the stage only executes if the condition evaluates to true.
	Condition *StageCondition `json:"condition,omitempty" yaml:"condition,omitempty"`

	// Action defines branching logic based on stage results.
	Action *StageAction `json:"action,omitempty" yaml:"action,omitempty"`

	// OnFailure specifies the failure handling strategy for this stage.
	OnFailure FailureAction `json:"on_failure" yaml:"on_failure"`

	// Dependencies lists stage IDs that must complete before this stage can execute.
	// Supports both sequential and parallel execution patterns.
	Dependencies []string `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`

	// Parallel indicates whether this stage can run in parallel with other stages.
	// True if this stage has no dependencies on other parallel stages.
	Parallel bool `json:"parallel,omitempty" yaml:"parallel,omitempty"`

	// Timeout specifies the maximum execution time for this stage.
	// Zero value means no timeout (uses default from payload or executor).
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`

	// Metadata contains additional custom metadata for this stage.
	Metadata map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// ChainMetadata contains descriptive metadata about an attack chain.
type ChainMetadata struct {
	// Author is the creator of this attack chain.
	Author string `json:"author,omitempty" yaml:"author,omitempty"`

	// Version is the version identifier for this chain.
	Version string `json:"version" yaml:"version"`

	// Tags are searchable keywords for categorizing the chain.
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// MitreAtlas contains MITRE ATLAS technique mappings for this chain.
	// Format: ["AML.T0043", "AML.T0051"]
	MitreAtlas []string `json:"mitre_atlas,omitempty" yaml:"mitre_atlas,omitempty"`

	// TargetTypes specifies which target types this chain is designed for.
	// Examples: "chat", "completion", "rag", "agent"
	TargetTypes []string `json:"target_types,omitempty" yaml:"target_types,omitempty"`

	// Difficulty indicates the complexity level of this attack chain.
	// Values: "beginner", "intermediate", "advanced", "expert"
	Difficulty string `json:"difficulty,omitempty" yaml:"difficulty,omitempty"`

	// EstimatedDuration is the expected execution time for this chain.
	EstimatedDuration time.Duration `json:"estimated_duration,omitempty" yaml:"estimated_duration,omitempty"`

	// References contains URLs to documentation, research, or related materials.
	References []string `json:"references,omitempty" yaml:"references,omitempty"`

	// Notes contains additional freeform notes about the chain.
	Notes string `json:"notes,omitempty" yaml:"notes,omitempty"`
}

// AttackChain represents a multi-stage attack sequence.
// Chains orchestrate multiple payloads with conditional branching and parallel execution.
type AttackChain struct {
	// ID is the unique identifier for this attack chain.
	ID types.ID `json:"id" yaml:"id"`

	// Name is a human-readable name for the chain.
	Name string `json:"name" yaml:"name"`

	// Description provides context about the attack chain's purpose and strategy.
	Description string `json:"description" yaml:"description"`

	// Stages contains all stages in this chain, indexed by stage ID.
	Stages map[string]*ChainStage `json:"stages" yaml:"stages"`

	// EntryStages lists the stage IDs that serve as entry points.
	// These are stages with no dependencies and execute first.
	EntryStages []string `json:"entry_stages" yaml:"entry_stages"`

	// Metadata contains descriptive metadata about this chain.
	Metadata ChainMetadata `json:"metadata" yaml:"metadata"`

	// Enabled indicates whether this chain is active and available for execution.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// BuiltIn indicates whether this is a built-in chain (read-only).
	BuiltIn bool `json:"built_in" yaml:"built_in"`

	// CreatedAt is the timestamp when the chain was created.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// UpdatedAt is the timestamp when the chain was last modified.
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`
}

// GetStage retrieves a stage by its ID from the chain.
// Returns nil if the stage is not found.
func (ac *AttackChain) GetStage(id string) *ChainStage {
	if ac.Stages == nil {
		return nil
	}
	return ac.Stages[id]
}

// GetEntryStages returns all stages designated as entry points.
// Returns an empty slice if there are no entry points or if stages cannot be found.
func (ac *AttackChain) GetEntryStages() []*ChainStage {
	if ac.EntryStages == nil || ac.Stages == nil {
		return []*ChainStage{}
	}

	stages := make([]*ChainStage, 0, len(ac.EntryStages))
	for _, id := range ac.EntryStages {
		if stage := ac.Stages[id]; stage != nil {
			stages = append(stages, stage)
		}
	}
	return stages
}

// StageCount returns the total number of stages in the chain.
func (ac *AttackChain) StageCount() int {
	return len(ac.Stages)
}

// HasParallelStages returns true if any stages in the chain are marked for parallel execution.
func (ac *AttackChain) HasParallelStages() bool {
	for _, stage := range ac.Stages {
		if stage.Parallel {
			return true
		}
	}
	return false
}
