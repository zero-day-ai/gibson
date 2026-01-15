package observability

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/types"
)

// LangfuseSpan represents a generic Langfuse span observation.
// This is the base type for tracking operations and executions.
type LangfuseSpan struct {
	// ID is the unique identifier for this span
	ID string `json:"id"`

	// TraceID is the ID of the parent trace
	TraceID string `json:"traceId"`

	// ParentObservationID is the ID of the parent observation (optional)
	ParentObservationID string `json:"parentObservationId,omitempty"`

	// Name is the human-readable name of the span
	Name string `json:"name"`

	// StartTime is when the span started
	StartTime time.Time `json:"startTime"`

	// EndTime is when the span ended (nil if still running)
	EndTime *time.Time `json:"endTime,omitempty"`

	// Input contains the input data for this operation
	Input map[string]any `json:"input,omitempty"`

	// Output contains the output/result data
	Output map[string]any `json:"output,omitempty"`

	// Metadata contains additional context and correlation IDs
	Metadata map[string]any `json:"metadata,omitempty"`

	// Level indicates the status level (DEFAULT, WARNING, ERROR)
	Level string `json:"level"`

	// StatusMessage contains additional status information
	StatusMessage string `json:"statusMessage,omitempty"`
}

// LangfuseGeneration represents a Langfuse generation observation.
// This is specifically for LLM completions and includes token usage metrics.
type LangfuseGeneration struct {
	// ID is the unique identifier for this generation
	ID string `json:"id"`

	// TraceID is the ID of the parent trace
	TraceID string `json:"traceId"`

	// ParentObservationID is the ID of the parent observation (optional)
	ParentObservationID string `json:"parentObservationId,omitempty"`

	// Name is the human-readable name of the generation
	Name string `json:"name"`

	// StartTime is when the generation started
	StartTime time.Time `json:"startTime"`

	// EndTime is when the generation ended (nil if still running)
	EndTime *time.Time `json:"endTime,omitempty"`

	// Model is the LLM model used
	Model string `json:"model"`

	// Input contains the prompt sent to the LLM
	Input any `json:"input,omitempty"`

	// Output contains the LLM response
	Output any `json:"output,omitempty"`

	// Metadata contains additional context and correlation IDs
	Metadata map[string]any `json:"metadata,omitempty"`

	// PromptTokens is the number of tokens in the prompt
	PromptTokens int `json:"promptTokens,omitempty"`

	// CompletionTokens is the number of tokens in the completion
	CompletionTokens int `json:"completionTokens,omitempty"`

	// Level indicates the status level (DEFAULT, WARNING, ERROR)
	Level string `json:"level"`

	// StatusMessage contains additional status information
	StatusMessage string `json:"statusMessage,omitempty"`
}

// SpanMissionSummary represents a summary of a mission execution for Langfuse.
// This is used to create a final summary span with aggregate metrics.
type SpanMissionSummary struct {
	// MissionID is the unique identifier for the mission
	MissionID types.ID

	// Status is the final mission status
	Status string

	// TotalDecisions is the count of orchestrator decisions made
	TotalDecisions int

	// TotalTokens is the aggregate token usage across all LLM calls
	TotalTokens int

	// Duration is the total mission execution time
	Duration time.Duration

	// CompletedNodes is the count of successfully completed workflow nodes
	CompletedNodes int

	// FailedNodes is the count of failed workflow nodes
	FailedNodes int

	// Error contains any fatal error message
	Error string

	// StopReason explains why the mission ended
	StopReason string
}

// BuildDecisionSpan creates a Langfuse generation span from an orchestrator decision.
// This represents an LLM decision-making operation with full prompt/response tracking.
//
// The span includes:
//   - Full reasoning prompt as input
//   - Decision action and reasoning as output
//   - Token usage and latency metrics
//   - Neo4j decision node ID for correlation
//
// Parameters:
//   - decision: The Decision node from the GraphRAG schema
//   - prompt: The full prompt sent to the LLM (optional, for observability)
//   - traceID: The mission trace ID
//   - parentSpanID: The parent span ID (optional)
//
// Returns:
//   - *LangfuseGeneration: The built generation span ready for export
func BuildDecisionSpan(decision *schema.Decision, prompt string, traceID string, parentSpanID string) *LangfuseGeneration {
	if decision == nil {
		return nil
	}

	// Calculate end time and duration
	var endTime *time.Time
	if decision.LatencyMs > 0 {
		// Reconstruct end time from start + latency
		end := decision.Timestamp.Add(time.Duration(decision.LatencyMs) * time.Millisecond)
		endTime = &end
	}

	// Build input structure with the prompt
	input := map[string]any{
		"iteration":        decision.Iteration,
		"graph_state":      decision.GraphStateSummary,
		"prompt":           prompt,
		"requested_action": decision.Action.String(),
		"target_node_id":   decision.TargetNodeID,
	}

	// Build output structure with the decision details
	output := map[string]any{
		"action":      decision.Action.String(),
		"target_node": decision.TargetNodeID,
		"reasoning":   decision.Reasoning,
		"confidence":  decision.Confidence,
	}

	// Include modifications if present
	if len(decision.Modifications) > 0 {
		output["modifications"] = decision.Modifications
	}

	// Build metadata with correlation IDs
	metadata := map[string]any{
		"decision_id":        decision.ID.String(),
		"mission_id":         decision.MissionID.String(),
		"iteration":          decision.Iteration,
		"latency_ms":         decision.LatencyMs,
		"neo4j_node_id":      decision.ID.String(), // For Neo4j correlation
		"decision_timestamp": decision.Timestamp.Format(time.RFC3339Nano),
	}

	// Add langfuse span ID if present (for bidirectional linking)
	if decision.LangfuseSpanID != "" {
		metadata["langfuse_correlation_id"] = decision.LangfuseSpanID
	}

	// Determine level based on confidence
	level := "DEFAULT"
	if decision.Confidence < 0.5 {
		level = "WARNING"
	}

	return &LangfuseGeneration{
		ID:                  decision.ID.String(),
		TraceID:             traceID,
		ParentObservationID: parentSpanID,
		Name:                fmt.Sprintf("orchestrator.decision[%d]: %s", decision.Iteration, decision.Action),
		StartTime:           decision.Timestamp,
		EndTime:             endTime,
		Model:               decision.Model,
		Input:               input,
		Output:              output,
		Metadata:            metadata,
		PromptTokens:        decision.PromptTokens,
		CompletionTokens:    decision.CompletionTokens,
		Level:               level,
	}
}

// BuildAgentSpan creates a Langfuse span from an agent execution.
// This represents an agent's execution lifecycle including configuration and results.
//
// The span includes:
//   - Agent name and task configuration as input
//   - Execution result and status as output
//   - Duration and attempt metrics
//   - Neo4j execution node ID for correlation
//
// Parameters:
//   - exec: The AgentExecution node from the GraphRAG schema
//   - traceID: The mission trace ID
//   - parentSpanID: The parent span ID (optional)
//
// Returns:
//   - *LangfuseSpan: The built span ready for export
func BuildAgentSpan(exec *schema.AgentExecution, traceID string, parentSpanID string) *LangfuseSpan {
	if exec == nil {
		return nil
	}

	// Build input structure with agent configuration
	input := map[string]any{
		"workflow_node_id": exec.WorkflowNodeID,
		"attempt":          exec.Attempt,
		"config":           exec.ConfigUsed,
	}

	// Build output structure with execution results
	output := map[string]any{
		"status": exec.Status.String(),
		"result": exec.Result,
	}

	// Add error to output if present
	if exec.Error != "" {
		output["error"] = exec.Error
	}

	// Build metadata with correlation IDs
	metadata := map[string]any{
		"execution_id":     exec.ID.String(),
		"mission_id":       exec.MissionID.String(),
		"workflow_node_id": exec.WorkflowNodeID,
		"attempt":          exec.Attempt,
		"neo4j_node_id":    exec.ID.String(), // For Neo4j correlation
	}

	// Add duration if completed
	if exec.CompletedAt != nil {
		duration := exec.Duration()
		metadata["duration_ms"] = duration.Milliseconds()
	}

	// Add langfuse span ID if present
	if exec.LangfuseSpanID != "" {
		metadata["langfuse_correlation_id"] = exec.LangfuseSpanID
	}

	// Determine level based on status
	level := "DEFAULT"
	statusMessage := ""
	if exec.Status == schema.ExecutionStatusFailed {
		level = "ERROR"
		statusMessage = exec.Error
	}

	return &LangfuseSpan{
		ID:                  exec.ID.String(),
		TraceID:             traceID,
		ParentObservationID: parentSpanID,
		Name:                fmt.Sprintf("agent.execute: %s", exec.WorkflowNodeID),
		StartTime:           exec.StartedAt,
		EndTime:             exec.CompletedAt,
		Input:               input,
		Output:              output,
		Metadata:            metadata,
		Level:               level,
		StatusMessage:       statusMessage,
	}
}

// BuildToolSpan creates a Langfuse span from a tool execution.
// This represents a tool invocation within an agent execution.
//
// The span includes:
//   - Tool name and input parameters
//   - Tool output and duration
//   - Execution status and errors
//   - Neo4j tool execution node ID for correlation
//
// Parameters:
//   - tool: The ToolExecution node from the GraphRAG schema
//   - traceID: The mission trace ID
//   - parentSpanID: The parent span ID (should be the agent execution span)
//
// Returns:
//   - *LangfuseSpan: The built span ready for export
func BuildToolSpan(tool *schema.ToolExecution, traceID string, parentSpanID string) *LangfuseSpan {
	if tool == nil {
		return nil
	}

	// Build input structure with tool parameters
	input := map[string]any{
		"tool_name": tool.ToolName,
		"params":    tool.Input,
	}

	// Build output structure with tool results
	output := map[string]any{
		"status": tool.Status.String(),
		"result": tool.Output,
	}

	// Add error to output if present
	if tool.Error != "" {
		output["error"] = tool.Error
	}

	// Build metadata with correlation IDs
	metadata := map[string]any{
		"tool_execution_id":  tool.ID.String(),
		"agent_execution_id": tool.AgentExecutionID.String(),
		"tool_name":          tool.ToolName,
		"neo4j_node_id":      tool.ID.String(), // For Neo4j correlation
	}

	// Add duration if completed
	if tool.CompletedAt != nil {
		duration := tool.Duration()
		metadata["duration_ms"] = duration.Milliseconds()
	}

	// Add langfuse span ID if present
	if tool.LangfuseSpanID != "" {
		metadata["langfuse_correlation_id"] = tool.LangfuseSpanID
	}

	// Determine level based on status
	level := "DEFAULT"
	statusMessage := ""
	if tool.Status == schema.ExecutionStatusFailed {
		level = "ERROR"
		statusMessage = tool.Error
	}

	return &LangfuseSpan{
		ID:                  tool.ID.String(),
		TraceID:             traceID,
		ParentObservationID: parentSpanID,
		Name:                fmt.Sprintf("tool.%s", tool.ToolName),
		StartTime:           tool.StartedAt,
		EndTime:             tool.CompletedAt,
		Input:               input,
		Output:              output,
		Metadata:            metadata,
		Level:               level,
		StatusMessage:       statusMessage,
	}
}

// BuildMissionSummarySpan creates a final summary span for the mission.
// This provides aggregate metrics and final status for the entire mission execution.
//
// The span includes:
//   - Total decisions, tokens, and duration
//   - Completed and failed node counts
//   - Final mission status and stop reason
//
// Parameters:
//   - summary: The SpanMissionSummary with aggregate metrics
//   - startTime: When the mission started
//   - endTime: When the mission ended
//   - traceID: The mission trace ID
//
// Returns:
//   - *LangfuseSpan: The built span ready for export
func BuildMissionSummarySpan(summary *SpanMissionSummary, startTime time.Time, endTime time.Time, traceID string) *LangfuseSpan {
	if summary == nil {
		return nil
	}

	// Build input structure with mission overview
	input := map[string]any{
		"mission_id": summary.MissionID.String(),
	}

	// Build output structure with summary metrics
	output := map[string]any{
		"status":          summary.Status,
		"total_decisions": summary.TotalDecisions,
		"total_tokens":    summary.TotalTokens,
		"duration_ms":     summary.Duration.Milliseconds(),
		"completed_nodes": summary.CompletedNodes,
		"failed_nodes":    summary.FailedNodes,
		"stop_reason":     summary.StopReason,
	}

	// Add error if present
	if summary.Error != "" {
		output["error"] = summary.Error
	}

	// Build metadata
	metadata := map[string]any{
		"mission_id":      summary.MissionID.String(),
		"total_decisions": summary.TotalDecisions,
		"total_tokens":    summary.TotalTokens,
		"duration_ms":     summary.Duration.Milliseconds(),
		"completed_nodes": summary.CompletedNodes,
		"failed_nodes":    summary.FailedNodes,
	}

	// Determine level based on status
	level := "DEFAULT"
	statusMessage := ""
	if summary.Error != "" {
		level = "ERROR"
		statusMessage = summary.Error
	} else if summary.Status == "completed" {
		level = "DEFAULT"
		statusMessage = "Mission completed successfully"
	}

	return &LangfuseSpan{
		ID:                  summary.MissionID.String() + "-summary",
		TraceID:             traceID,
		ParentObservationID: "", // Root span for summary
		Name:                "mission.summary",
		StartTime:           startTime,
		EndTime:             &endTime,
		Input:               input,
		Output:              output,
		Metadata:            metadata,
		Level:               level,
		StatusMessage:       statusMessage,
	}
}

// ToJSON serializes the span to JSON for debugging and logging.
func (s *LangfuseSpan) ToJSON() (string, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("failed to marshal span: %w", err)
	}
	return string(data), nil
}

// ToJSON serializes the generation to JSON for debugging and logging.
func (g *LangfuseGeneration) ToJSON() (string, error) {
	data, err := json.Marshal(g)
	if err != nil {
		return "", fmt.Errorf("failed to marshal generation: %w", err)
	}
	return string(data), nil
}

// Duration calculates the duration of the span.
// Returns 0 if the span hasn't completed yet.
func (s *LangfuseSpan) Duration() time.Duration {
	if s.EndTime == nil {
		return 0
	}
	return s.EndTime.Sub(s.StartTime)
}

// Duration calculates the duration of the generation.
// Returns 0 if the generation hasn't completed yet.
func (g *LangfuseGeneration) Duration() time.Duration {
	if g.EndTime == nil {
		return 0
	}
	return g.EndTime.Sub(g.StartTime)
}

// IsComplete returns true if the span has completed.
func (s *LangfuseSpan) IsComplete() bool {
	return s.EndTime != nil
}

// IsComplete returns true if the generation has completed.
func (g *LangfuseGeneration) IsComplete() bool {
	return g.EndTime != nil
}

// TotalTokens returns the sum of prompt and completion tokens.
func (g *LangfuseGeneration) TotalTokens() int {
	return g.PromptTokens + g.CompletionTokens
}
