package observability

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/orchestrator"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/trace"
)

// DecisionLogWriterAdapter adapts the orchestrator's DecisionLogWriter interface
// to the existing MissionTracer implementation. It bridges orchestrator decisions
// and actions to Langfuse traces without creating new API calls.
//
// Thread Safety: All methods are thread-safe and use fire-and-forget error handling.
// Tracing errors are logged but never propagate to the orchestrator to ensure
// mission execution is never blocked by observability failures.
//
// Usage:
//
//	tracer, _ := NewMissionTracer(config)
//	adapter, _ := NewDecisionLogWriterAdapter(ctx, tracer, mission)
//	defer adapter.Close(ctx, summary)
//
//	// Pass to orchestrator
//	orch := orchestrator.NewOrchestrator(...,
//		orchestrator.WithDecisionLogWriter(adapter))
type DecisionLogWriterAdapter struct {
	tracer      *MissionTracer
	trace       *MissionTrace
	agentLogs   map[string]*AgentExecutionLog // Maps agent execution ID to log
	mu          sync.RWMutex                  // Protects agentLogs and statistics
	statistics  *adapterStatistics
	missionID   string
	missionName string
}

// adapterStatistics tracks metrics for the mission trace summary.
type adapterStatistics struct {
	totalDecisions  int
	totalExecutions int
	totalTools      int
	totalTokens     int
	startTime       time.Time
}

// NewDecisionLogWriterAdapter creates a new adapter that wraps a MissionTracer.
// It immediately starts a mission trace in Langfuse.
//
// Parameters:
//   - ctx: Context for the trace creation
//   - tracer: The MissionTracer to delegate to (must not be nil)
//   - mission: The mission being traced (must not be nil)
//
// Returns:
//   - *DecisionLogWriterAdapter: The initialized adapter
//   - error: Any error encountered during trace creation
//
// The adapter uses fire-and-forget error handling internally, but returns errors
// during construction so callers can decide whether to proceed without tracing.
func NewDecisionLogWriterAdapter(ctx context.Context, tracer *MissionTracer, mission *schema.Mission) (*DecisionLogWriterAdapter, error) {
	if tracer == nil {
		return nil, fmt.Errorf("tracer cannot be nil")
	}
	if mission == nil {
		return nil, fmt.Errorf("mission cannot be nil")
	}

	// Start the mission trace
	trace, err := tracer.StartMissionTrace(ctx, mission)
	if err != nil {
		return nil, fmt.Errorf("failed to start mission trace: %w", err)
	}

	adapter := &DecisionLogWriterAdapter{
		tracer:      tracer,
		trace:       trace,
		agentLogs:   make(map[string]*AgentExecutionLog),
		missionID:   mission.ID.String(),
		missionName: mission.Name,
		statistics: &adapterStatistics{
			startTime: time.Now(),
		},
	}

	slog.Info("created decision log writer adapter",
		"mission_id", mission.ID.String(),
		"trace_id", trace.TraceID,
	)

	return adapter, nil
}

// LogDecision logs an orchestrator decision and its result to Langfuse as a Generation.
// This method converts orchestrator types to Langfuse types and delegates to MissionTracer.
//
// Parameters:
//   - ctx: Context for the logging operation
//   - decision: The orchestrator decision made
//   - result: The think result containing LLM metadata
//   - iteration: The orchestration iteration number
//   - missionID: The mission ID (used for validation)
//
// Returns:
//   - error: Always returns nil (fire-and-forget pattern)
//
// The method uses fire-and-forget error handling: errors are logged but not propagated
// to ensure orchestration is never blocked by tracing failures.
func (a *DecisionLogWriterAdapter) LogDecision(ctx context.Context, decision *orchestrator.Decision, result *orchestrator.ThinkResult, iteration int, missionID string) error {
	if decision == nil || result == nil {
		slog.Warn("skipping decision log: nil decision or result",
			"mission_id", a.missionID,
			"iteration", iteration,
		)
		return nil
	}

	// Validate mission ID matches
	if missionID != a.missionID {
		slog.Warn("decision log mission ID mismatch",
			"expected", a.missionID,
			"got", missionID,
			"iteration", iteration,
		)
		return nil
	}

	// Convert orchestrator types to schema types for Langfuse
	schemaDecision := a.convertDecision(decision, result, iteration)

	// Extract OTEL trace ID from context for correlation with infrastructure traces
	otelTraceID := extractOTELTraceID(ctx)

	// Build DecisionLog for MissionTracer
	decisionLog := &DecisionLog{
		Decision:      schemaDecision,
		Prompt:        a.buildPromptSummary(decision, iteration),
		Response:      result.RawResponse,
		Model:         result.Model,
		GraphSnapshot: a.buildGraphSnapshot(decision),
		Neo4jNodeID:   "", // Not stored in Neo4j at this level
		OTELTraceID:   otelTraceID,
	}

	// Log to Langfuse (fire-and-forget)
	if err := a.tracer.LogDecision(ctx, a.trace, decisionLog); err != nil {
		slog.Warn("failed to log decision to langfuse",
			"mission_id", a.missionID,
			"iteration", iteration,
			"error", err,
		)
		// Don't return the error - continue execution
	}

	// Update statistics
	a.mu.Lock()
	a.statistics.totalDecisions++
	a.statistics.totalTokens += result.TotalTokens
	a.mu.Unlock()

	return nil
}

// LogAction logs an action result to Langfuse as agent execution and tool execution spans.
// This method routes to the appropriate Langfuse span type based on the action.
//
// Parameters:
//   - ctx: Context for the logging operation
//   - action: The action result from the actor
//   - iteration: The orchestration iteration number
//   - missionID: The mission ID (used for validation)
//
// Returns:
//   - error: Always returns nil (fire-and-forget pattern)
//
// The method uses fire-and-forget error handling: errors are logged but not propagated.
func (a *DecisionLogWriterAdapter) LogAction(ctx context.Context, action *orchestrator.ActionResult, iteration int, missionID string) error {
	if action == nil {
		slog.Warn("skipping action log: nil action",
			"mission_id", a.missionID,
			"iteration", iteration,
		)
		return nil
	}

	// Validate mission ID matches
	if missionID != a.missionID {
		slog.Warn("action log mission ID mismatch",
			"expected", a.missionID,
			"got", missionID,
			"iteration", iteration,
		)
		return nil
	}

	// Route based on action type
	switch action.Action {
	case orchestrator.ActionExecuteAgent:
		a.logAgentExecution(ctx, action, iteration)
	case orchestrator.ActionSpawnAgent:
		a.logSpawnAgent(ctx, action, iteration)
	case orchestrator.ActionComplete:
		// Complete is logged in Close()
	default:
		// Other actions (skip, modify, retry) don't create separate spans
		slog.Debug("action logged without span creation",
			"action", action.Action,
			"mission_id", a.missionID,
			"iteration", iteration,
		)
	}

	return nil
}

// logAgentExecution logs an agent execution to Langfuse as a span.
func (a *DecisionLogWriterAdapter) logAgentExecution(ctx context.Context, action *orchestrator.ActionResult, iteration int) {
	if action.AgentExecution == nil {
		return
	}

	exec := action.AgentExecution

	// Extract agent name from metadata
	agentName := "unknown"
	if name, ok := action.Metadata["agent_name"].(string); ok {
		agentName = name
	}

	// Extract OTEL trace ID from context for correlation with infrastructure traces
	otelTraceID := extractOTELTraceID(ctx)

	// Create AgentExecutionLog
	agentLog := &AgentExecutionLog{
		Execution:   exec,
		AgentName:   agentName,
		Config:      make(map[string]any),
		Neo4jNodeID: "", // Not stored at this level
		OTELTraceID: otelTraceID,
	}

	// Store in map for tool execution parent linking
	a.mu.Lock()
	a.agentLogs[exec.ID.String()] = agentLog
	a.statistics.totalExecutions++
	a.mu.Unlock()

	// Log to Langfuse (fire-and-forget)
	if err := a.tracer.LogAgentExecution(ctx, a.trace, agentLog); err != nil {
		slog.Warn("failed to log agent execution to langfuse",
			"mission_id", a.missionID,
			"execution_id", exec.ID.String(),
			"error", err,
		)
	}
}

// logSpawnAgent logs a dynamically spawned agent to Langfuse metadata.
func (a *DecisionLogWriterAdapter) logSpawnAgent(ctx context.Context, action *orchestrator.ActionResult, iteration int) {
	if action.NewNode == nil {
		return
	}

	slog.Debug("spawned agent logged",
		"mission_id", a.missionID,
		"node_id", action.NewNode.ID.String(),
		"agent_name", action.NewNode.AgentName,
		"iteration", iteration,
	)

	// Spawn actions don't create separate Langfuse spans
	// They're captured in decision metadata
}

// Close finalizes the mission trace and sends the summary to Langfuse.
// This method should be called when the mission completes (success or failure).
//
// Parameters:
//   - ctx: Context for the finalization
//   - summary: Optional mission summary with statistics (can be nil)
//
// Returns:
//   - error: Always returns nil (fire-and-forget pattern)
//
// The method uses fire-and-forget error handling: errors are logged but not propagated.
func (a *DecisionLogWriterAdapter) Close(ctx context.Context, summary *MissionTraceSummary) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Build summary if not provided
	if summary == nil {
		summary = a.buildDefaultSummary()
	} else {
		// Merge our statistics with provided summary
		summary.TotalDecisions = a.statistics.totalDecisions
		summary.TotalExecutions = a.statistics.totalExecutions
		summary.TotalTools = a.statistics.totalTools
		if summary.TotalTokens == 0 {
			summary.TotalTokens = a.statistics.totalTokens
		}
		if summary.Duration == 0 {
			summary.Duration = time.Since(a.statistics.startTime)
		}
	}

	// End the trace (fire-and-forget)
	if err := a.tracer.EndMissionTrace(ctx, a.trace, summary); err != nil {
		slog.Warn("failed to end mission trace in langfuse",
			"mission_id", a.missionID,
			"error", err,
		)
	}

	slog.Info("closed decision log writer adapter",
		"mission_id", a.missionID,
		"trace_id", a.trace.TraceID,
		"decisions", summary.TotalDecisions,
		"executions", summary.TotalExecutions,
		"duration", summary.Duration,
	)

	return nil
}

// convertDecision converts orchestrator.Decision to schema.Decision.
func (a *DecisionLogWriterAdapter) convertDecision(decision *orchestrator.Decision, result *orchestrator.ThinkResult, iteration int) *schema.Decision {
	now := time.Now()

	// Convert orchestrator.DecisionAction to schema.DecisionAction
	schemaAction := schema.DecisionAction(decision.Action.String())

	// Parse missionID from string to types.ID
	missionID := types.ID(a.missionID)

	schemaDecision := schema.NewDecision(
		missionID,
		iteration,
		schemaAction,
	)
	schemaDecision.TargetNodeID = decision.TargetNodeID
	schemaDecision.Reasoning = decision.Reasoning
	schemaDecision.WithConfidence(decision.Confidence)
	schemaDecision.WithTokenUsage(result.PromptTokens, result.CompletionTokens)
	schemaDecision.WithLatency(int(result.Latency.Milliseconds()))
	schemaDecision.Timestamp = now

	// Add modifications if present
	if len(decision.Modifications) > 0 {
		schemaDecision.Modifications = decision.Modifications
	}

	return schemaDecision
}

// buildPromptSummary creates a summary of the prompt for logging.
func (a *DecisionLogWriterAdapter) buildPromptSummary(decision *orchestrator.Decision, iteration int) string {
	return fmt.Sprintf("Orchestrator decision iteration %d: %s (confidence: %.2f)",
		iteration, decision.Action, decision.Confidence)
}

// buildGraphSnapshot creates a snapshot description of the graph state.
func (a *DecisionLogWriterAdapter) buildGraphSnapshot(decision *orchestrator.Decision) string {
	if decision.StopReason != "" {
		return fmt.Sprintf("Mission complete: %s", decision.StopReason)
	}
	if decision.TargetNodeID != "" {
		return fmt.Sprintf("Target node: %s", decision.TargetNodeID)
	}
	return fmt.Sprintf("Action: %s", decision.Action)
}

// buildDefaultSummary creates a default summary from adapter statistics.
func (a *DecisionLogWriterAdapter) buildDefaultSummary() *MissionTraceSummary {
	return &MissionTraceSummary{
		Status:          string(schema.MissionStatusCompleted),
		TotalDecisions:  a.statistics.totalDecisions,
		TotalExecutions: a.statistics.totalExecutions,
		TotalTools:      a.statistics.totalTools,
		TotalTokens:     a.statistics.totalTokens,
		TotalCost:       0.0, // Cost calculation not implemented
		Duration:        time.Since(a.statistics.startTime),
		Outcome:         "Mission completed",
		GraphStats:      make(map[string]int),
	}
}

// extractOTELTraceID extracts the OpenTelemetry trace ID from context.
// This enables correlation between Langfuse traces and infrastructure traces in Grafana/Jaeger.
// Returns empty string if no valid OTEL span is found in the context.
func extractOTELTraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return ""
	}
	return span.SpanContext().TraceID().String()
}
