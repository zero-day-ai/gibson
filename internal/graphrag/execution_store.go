// Package graphrag provides the GraphRAG store implementation.
// This file defines the ExecutionGraphStore interface for recording execution traces
// and performance metrics in the graph database for observability and analysis.
package graphrag

import (
	"context"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// AgentStartEvent captures the start of an agent's execution.
// Contains trace correlation fields for distributed tracing integration.
type AgentStartEvent struct {
	AgentName string    // Name of the agent starting execution
	TaskID    string    // Task identifier being executed
	MissionID types.ID  // Mission this agent belongs to
	TraceID   string    // OpenTelemetry trace ID for correlation
	SpanID    string    // OpenTelemetry span ID for this agent execution
	StartedAt time.Time // Timestamp when agent started
}

// AgentCompleteEvent captures the completion of an agent's execution.
// Records final status, metrics, and trace correlation data.
type AgentCompleteEvent struct {
	AgentName     string    // Name of the agent completing execution
	TaskID        string    // Task identifier that was executed
	MissionID     types.ID  // Mission this agent belongs to
	Status        string    // Final status: "success", "failure", "timeout", etc.
	FindingsCount int       // Number of findings produced by this agent
	TraceID       string    // OpenTelemetry trace ID for correlation
	SpanID        string    // OpenTelemetry span ID for this agent execution
	CompletedAt   time.Time // Timestamp when agent completed
	DurationMs    int64     // Total execution duration in milliseconds
}

// DelegationEvent captures when one agent delegates work to another agent.
// Tracks the delegation graph for workflow analysis.
type DelegationEvent struct {
	FromAgent string    // Agent delegating the task
	ToAgent   string    // Agent receiving the delegated task
	TaskID    string    // Task being delegated
	MissionID types.ID  // Mission context
	TraceID   string    // OpenTelemetry trace ID for correlation
	SpanID    string    // OpenTelemetry span ID for the delegation operation
	Timestamp time.Time // When the delegation occurred
}

// LLMCallEvent captures a single LLM API call for cost and performance tracking.
// Records model usage, token counts, cost, and latency metrics.
type LLMCallEvent struct {
	AgentName string    // Agent making the LLM call
	MissionID types.ID  // Mission context
	Model     string    // LLM model used (e.g., "gpt-4", "claude-3-opus")
	Provider  string    // LLM provider (e.g., "openai", "anthropic")
	Slot      string    // Slot name used for this call
	TokensIn  int       // Input tokens consumed
	TokensOut int       // Output tokens generated
	CostUSD   float64   // Estimated cost in USD
	LatencyMs int64     // API call latency in milliseconds
	TraceID   string    // OpenTelemetry trace ID for correlation
	SpanID    string    // OpenTelemetry span ID for this LLM call
	Timestamp time.Time // When the LLM call was made
}

// ToolCallEvent captures a single tool execution for performance analysis.
// Records tool usage patterns, success rates, and error conditions.
type ToolCallEvent struct {
	AgentName  string    // Agent executing the tool
	MissionID  types.ID  // Mission context
	ToolName   string    // Name of the tool called
	DurationMs int64     // Tool execution duration in milliseconds
	Success    bool      // Whether the tool call succeeded
	Error      string    // Error message if the call failed (empty if successful)
	TraceID    string    // OpenTelemetry trace ID for correlation
	SpanID     string    // OpenTelemetry span ID for this tool call
	Timestamp  time.Time // When the tool call was made
}

// MissionStartEvent captures the start of a mission execution.
// Top-level event for mission lifecycle tracking.
type MissionStartEvent struct {
	MissionID   types.ID  // Unique mission identifier
	MissionName string    // Human-readable mission name
	TraceID     string    // OpenTelemetry trace ID for the entire mission
	StartedAt   time.Time // Timestamp when mission started
}

// MissionCompleteEvent captures the completion of a mission.
// Records final mission status and aggregate metrics.
type MissionCompleteEvent struct {
	MissionID     types.ID  // Unique mission identifier
	Status        string    // Final mission status: "success", "failure", "partial", etc.
	FindingsCount int       // Total findings discovered during the mission
	TraceID       string    // OpenTelemetry trace ID for the entire mission
	CompletedAt   time.Time // Timestamp when mission completed
	DurationMs    int64     // Total mission duration in milliseconds
}

// ExecutionGraphStore provides an interface for recording execution traces and metrics
// in the graph database. This enables observability, performance analysis, and
// cross-mission correlation of execution patterns.
//
// The store captures:
// - Agent lifecycle events (start, complete, delegation)
// - LLM API calls (cost, performance, model usage)
// - Tool executions (success/failure rates, performance)
// - Mission lifecycle events (start, complete)
//
// All events include OpenTelemetry trace/span IDs for distributed tracing integration.
//
// Thread-safety: All implementations must be safe for concurrent access.
type ExecutionGraphStore interface {
	// RecordAgentStart records the start of an agent's execution.
	// Creates an Agent node if it doesn't exist and tracks execution start.
	RecordAgentStart(ctx context.Context, event AgentStartEvent) error

	// RecordAgentComplete records the completion of an agent's execution.
	// Updates the agent execution with final status, metrics, and duration.
	RecordAgentComplete(ctx context.Context, event AgentCompleteEvent) error

	// RecordDelegation records a delegation from one agent to another.
	// Creates a DELEGATES_TO relationship between agents for workflow analysis.
	RecordDelegation(ctx context.Context, event DelegationEvent) error

	// RecordLLMCall records a single LLM API call.
	// Tracks cost, token usage, model selection, and performance metrics.
	RecordLLMCall(ctx context.Context, event LLMCallEvent) error

	// RecordToolCall records a single tool execution.
	// Tracks tool usage patterns, success rates, and performance.
	RecordToolCall(ctx context.Context, event ToolCallEvent) error

	// RecordMissionStart records the start of a mission.
	// Creates a Mission node and initializes mission-level tracking.
	RecordMissionStart(ctx context.Context, event MissionStartEvent) error

	// RecordMissionComplete records the completion of a mission.
	// Updates the mission with final status, aggregate metrics, and duration.
	RecordMissionComplete(ctx context.Context, event MissionCompleteEvent) error

	// Close releases all resources and closes connections.
	// Should be called during graceful shutdown.
	Close() error
}
