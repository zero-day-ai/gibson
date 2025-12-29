package workflow

import (
	"math"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
)

// BackoffStrategy defines the strategy for calculating retry delays
type BackoffStrategy string

const (
	// BackoffConstant returns a constant delay for all retry attempts
	BackoffConstant BackoffStrategy = "constant"
	// BackoffLinear increases the delay linearly with each retry attempt
	BackoffLinear BackoffStrategy = "linear"
	// BackoffExponential increases the delay exponentially with each retry attempt
	BackoffExponential BackoffStrategy = "exponential"
)

// NodeType defines the type of workflow node
type NodeType string

const (
	NodeTypeAgent     NodeType = "agent"
	NodeTypeTool      NodeType = "tool"
	NodeTypePlugin    NodeType = "plugin"
	NodeTypeCondition NodeType = "condition"
	NodeTypeParallel  NodeType = "parallel"
	NodeTypeJoin      NodeType = "join"
)

// NodeStatus represents the execution status of a workflow node
type NodeStatus string

const (
	NodeStatusPending   NodeStatus = "pending"
	NodeStatusRunning   NodeStatus = "running"
	NodeStatusCompleted NodeStatus = "completed"
	NodeStatusFailed    NodeStatus = "failed"
	NodeStatusSkipped   NodeStatus = "skipped"
	NodeStatusCancelled NodeStatus = "cancelled"
)

// WorkflowNode represents a single node in a workflow DAG
type WorkflowNode struct {
	// Core identity fields
	ID          string   `json:"id"`
	Type        NodeType `json:"type"`
	Name        string   `json:"name"`
	Description string   `json:"description"`

	// Agent node fields
	AgentName string      `json:"agent_name,omitempty"`
	AgentTask *agent.Task `json:"agent_task,omitempty"`

	// Tool node fields
	ToolName  string         `json:"tool_name,omitempty"`
	ToolInput map[string]any `json:"tool_input,omitempty"`

	// Plugin node fields
	PluginName   string         `json:"plugin_name,omitempty"`
	PluginMethod string         `json:"plugin_method,omitempty"`
	PluginParams map[string]any `json:"plugin_params,omitempty"`

	// Condition node fields
	Condition *NodeCondition `json:"condition,omitempty"`

	// Parallel node fields
	SubNodes []*WorkflowNode `json:"sub_nodes,omitempty"`

	// Execution control fields
	Dependencies []string      `json:"dependencies,omitempty"`
	Timeout      time.Duration `json:"timeout,omitempty"`
	RetryPolicy  *RetryPolicy  `json:"retry_policy,omitempty"`

	// Additional metadata
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NodeCondition defines conditional branching logic for condition nodes
type NodeCondition struct {
	// Expression to evaluate (e.g., "result.status == 'success'")
	Expression string `json:"expression"`

	// Node IDs to execute if condition is true
	TrueBranch []string `json:"true_branch,omitempty"`

	// Node IDs to execute if condition is false
	FalseBranch []string `json:"false_branch,omitempty"`
}

// RetryPolicy defines the retry behavior for a workflow node
type RetryPolicy struct {
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int `json:"max_retries"`
	// BackoffStrategy determines how delays are calculated between retries
	BackoffStrategy BackoffStrategy `json:"backoff_strategy"`
	// InitialDelay is the delay before the first retry attempt
	InitialDelay time.Duration `json:"initial_delay"`
	// MaxDelay is the maximum delay between retry attempts (used for exponential backoff)
	MaxDelay time.Duration `json:"max_delay"`
	// Multiplier is the factor by which the delay increases (used for exponential backoff)
	Multiplier float64 `json:"multiplier"`
}

// CalculateDelay calculates the delay duration for a given retry attempt
// based on the configured backoff strategy
func (rp *RetryPolicy) CalculateDelay(attempt int) time.Duration {
	switch rp.BackoffStrategy {
	case BackoffConstant:
		return rp.InitialDelay
	case BackoffLinear:
		return rp.InitialDelay + (rp.InitialDelay * time.Duration(attempt))
	case BackoffExponential:
		delay := time.Duration(float64(rp.InitialDelay) * math.Pow(rp.Multiplier, float64(attempt)))
		if delay > rp.MaxDelay {
			return rp.MaxDelay
		}
		return delay
	default:
		return rp.InitialDelay
	}
}
