package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DecisionAction represents the type of action the orchestrator decides to take
type DecisionAction string

const (
	// ActionExecuteAgent runs the specified workflow node/agent
	ActionExecuteAgent DecisionAction = "execute_agent"

	// ActionSkipAgent skips execution of a workflow node
	ActionSkipAgent DecisionAction = "skip_agent"

	// ActionModifyParams modifies parameters for a target node before execution
	ActionModifyParams DecisionAction = "modify_params"

	// ActionRetry retries execution of a failed node
	ActionRetry DecisionAction = "retry"

	// ActionSpawnAgent dynamically creates and adds a new node to the workflow
	ActionSpawnAgent DecisionAction = "spawn_agent"

	// ActionComplete marks the workflow as complete and stops orchestration
	ActionComplete DecisionAction = "complete"
)

// String returns the string representation of a DecisionAction
func (d DecisionAction) String() string {
	return string(d)
}

// IsValid checks if the DecisionAction is one of the defined constants
func (d DecisionAction) IsValid() bool {
	switch d {
	case ActionExecuteAgent, ActionSkipAgent, ActionModifyParams,
		ActionRetry, ActionSpawnAgent, ActionComplete:
		return true
	default:
		return false
	}
}

// IsTerminal returns true if this action ends the orchestration loop
func (d DecisionAction) IsTerminal() bool {
	return d == ActionComplete
}

// Decision represents the orchestrator's reasoning output from the LLM.
// This struct is designed to be JSON serializable for structured output.
type Decision struct {
	// Reasoning is the chain-of-thought explanation of why this decision was made
	Reasoning string `json:"reasoning"`

	// Action is what the orchestrator should do
	Action DecisionAction `json:"action"`

	// TargetNodeID is which workflow node to act on (if applicable)
	// Required for: execute_agent, skip_agent, modify_params, retry
	TargetNodeID string `json:"target_node_id,omitempty"`

	// Modifications are parameter overrides for the target node
	// Used with: modify_params action
	Modifications map[string]interface{} `json:"modifications,omitempty"`

	// SpawnConfig is configuration for dynamically creating new nodes
	// Required for: spawn_agent action
	SpawnConfig *SpawnNodeConfig `json:"spawn_config,omitempty"`

	// Confidence is a value between 0.0 and 1.0 indicating the orchestrator's
	// certainty in this decision
	Confidence float64 `json:"confidence"`

	// StopReason explains why the workflow is complete
	// Required for: complete action
	StopReason string `json:"stop_reason,omitempty"`
}

// SpawnNodeConfig contains configuration for dynamically spawning a new workflow node
type SpawnNodeConfig struct {
	// AgentName is the type of agent to spawn (must exist in registry)
	AgentName string `json:"agent_name"`

	// Description explains the purpose of this dynamically spawned node
	Description string `json:"description"`

	// TaskConfig contains parameters specific to this spawned agent
	TaskConfig map[string]interface{} `json:"task_config"`

	// DependsOn lists node IDs that must complete before this spawned node can run
	DependsOn []string `json:"depends_on"`
}

// Validate checks if the Decision is properly formed and all required fields
// are present for the specified action
func (d *Decision) Validate() error {
	if d == nil {
		return fmt.Errorf("decision is nil")
	}

	// Check reasoning is present
	if strings.TrimSpace(d.Reasoning) == "" {
		return fmt.Errorf("reasoning is required")
	}

	// Validate action is known
	if !d.Action.IsValid() {
		return fmt.Errorf("invalid action: %s", d.Action)
	}

	// Validate confidence range
	if d.Confidence < 0.0 || d.Confidence > 1.0 {
		return fmt.Errorf("confidence must be between 0.0 and 1.0, got: %f", d.Confidence)
	}

	// Action-specific validation
	switch d.Action {
	case ActionExecuteAgent, ActionSkipAgent, ActionRetry:
		if strings.TrimSpace(d.TargetNodeID) == "" {
			return fmt.Errorf("target_node_id is required for action: %s", d.Action)
		}

	case ActionModifyParams:
		if strings.TrimSpace(d.TargetNodeID) == "" {
			return fmt.Errorf("target_node_id is required for modify_params action")
		}
		if len(d.Modifications) == 0 {
			return fmt.Errorf("modifications are required for modify_params action")
		}

	case ActionSpawnAgent:
		if d.SpawnConfig == nil {
			return fmt.Errorf("spawn_config is required for spawn_agent action")
		}
		if err := d.SpawnConfig.Validate(); err != nil {
			return fmt.Errorf("invalid spawn_config: %w", err)
		}

	case ActionComplete:
		if strings.TrimSpace(d.StopReason) == "" {
			return fmt.Errorf("stop_reason is required for complete action")
		}
	}

	return nil
}

// Validate checks if the SpawnNodeConfig is properly formed
func (s *SpawnNodeConfig) Validate() error {
	if s == nil {
		return fmt.Errorf("spawn config is nil")
	}

	if strings.TrimSpace(s.AgentName) == "" {
		return fmt.Errorf("agent_name is required")
	}

	if strings.TrimSpace(s.Description) == "" {
		return fmt.Errorf("description is required")
	}

	// TaskConfig can be empty but not nil
	if s.TaskConfig == nil {
		return fmt.Errorf("task_config cannot be nil (use empty map if no config needed)")
	}

	// DependsOn can be empty but not nil
	if s.DependsOn == nil {
		return fmt.Errorf("depends_on cannot be nil (use empty slice if no dependencies)")
	}

	return nil
}

// ParseDecision parses a JSON string (typically from LLM structured output)
// into a Decision struct and validates it
func ParseDecision(jsonStr string) (*Decision, error) {
	if strings.TrimSpace(jsonStr) == "" {
		return nil, fmt.Errorf("empty JSON string")
	}

	var decision Decision
	if err := json.Unmarshal([]byte(jsonStr), &decision); err != nil {
		return nil, fmt.Errorf("failed to parse decision JSON: %w", err)
	}

	if err := decision.Validate(); err != nil {
		return nil, fmt.Errorf("invalid decision: %w", err)
	}

	return &decision, nil
}

// IsTerminal returns true if this decision ends the orchestration loop
func (d *Decision) IsTerminal() bool {
	return d != nil && d.Action.IsTerminal()
}

// String returns a human-readable representation of the decision
func (d *Decision) String() string {
	if d == nil {
		return "<nil decision>"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Decision{Action: %s", d.Action))

	if d.TargetNodeID != "" {
		sb.WriteString(fmt.Sprintf(", Target: %s", d.TargetNodeID))
	}

	if d.SpawnConfig != nil {
		sb.WriteString(fmt.Sprintf(", SpawnAgent: %s", d.SpawnConfig.AgentName))
	}

	sb.WriteString(fmt.Sprintf(", Confidence: %.2f", d.Confidence))

	if d.StopReason != "" {
		sb.WriteString(fmt.Sprintf(", Reason: %s", d.StopReason))
	}

	sb.WriteString("}")
	return sb.String()
}

// ToJSON serializes the Decision to a JSON string
func (d *Decision) ToJSON() (string, error) {
	if d == nil {
		return "", fmt.Errorf("cannot serialize nil decision")
	}

	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal decision: %w", err)
	}

	return string(data), nil
}
