package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/zero-day-ai/gibson/internal/events"
	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/trace"
)

// OrchestratorStatus represents the final status of an orchestration run.
type OrchestratorStatus string

const (
	// StatusCompleted indicates the mission completed successfully
	StatusCompleted OrchestratorStatus = "completed"

	// StatusFailed indicates the mission failed with an error
	StatusFailed OrchestratorStatus = "failed"

	// StatusMaxIterations indicates max iterations were reached
	StatusMaxIterations OrchestratorStatus = "max_iterations"

	// StatusTimeout indicates the orchestrator timed out
	StatusTimeout OrchestratorStatus = "timeout"

	// StatusCancelled indicates the context was cancelled
	StatusCancelled OrchestratorStatus = "cancelled"

	// StatusBudgetExceeded indicates token budget was exhausted
	StatusBudgetExceeded OrchestratorStatus = "budget_exceeded"

	// StatusConcurrencyLimit indicates max concurrent executions reached
	StatusConcurrencyLimit OrchestratorStatus = "concurrency_limit"
)

// String returns the string representation of OrchestratorStatus
func (s OrchestratorStatus) String() string {
	return string(s)
}

// Orchestrator implements the main Observe → Think → Act control loop.
// It coordinates the observer, thinker, and actor components to autonomously
// execute mission workflows based on LLM reasoning.
type Orchestrator struct {
	observer  *Observer
	thinker   *Thinker
	actor     *Actor
	eventBus  EventBus
	logger    *slog.Logger
	tracer    trace.Tracer
	logWriter DecisionLogWriter

	// Configuration options
	maxIterations int
	budget        int // Total token budget (0 = unlimited)
	maxConcurrent int // Max concurrent node executions
	timeout       time.Duration
}

// EventBus defines the interface for emitting orchestrator events.
type EventBus interface {
	Publish(event events.Event)
}

// DecisionLogWriter defines the interface for logging decisions to external systems.
// This is typically implemented by Langfuse or similar observability platforms.
type DecisionLogWriter interface {
	// LogDecision writes a decision and its outcome to the external log
	LogDecision(ctx context.Context, decision *Decision, result *ThinkResult, iteration int, missionID string) error

	// LogAction writes an action result to the external log
	LogAction(ctx context.Context, action *ActionResult, iteration int, missionID string) error
}

// OrchestratorOption is a functional option for configuring the Orchestrator.
type OrchestratorOption func(*Orchestrator)

// WithMaxIterations sets the maximum number of orchestration iterations.
// Default: 100
func WithMaxIterations(n int) OrchestratorOption {
	return func(o *Orchestrator) {
		if n > 0 {
			o.maxIterations = n
		}
	}
}

// WithBudget sets the total token budget for the orchestration run.
// When the budget is exceeded, orchestration stops.
// Default: 0 (unlimited)
func WithBudget(tokens int) OrchestratorOption {
	return func(o *Orchestrator) {
		if tokens >= 0 {
			o.budget = tokens
		}
	}
}

// WithMaxConcurrent sets the maximum number of concurrent node executions.
// Default: 10
func WithMaxConcurrent(n int) OrchestratorOption {
	return func(o *Orchestrator) {
		if n > 0 {
			o.maxConcurrent = n
		}
	}
}

// WithTimeout sets the overall timeout for the orchestration run.
// Default: 0 (no timeout)
func WithTimeout(d time.Duration) OrchestratorOption {
	return func(o *Orchestrator) {
		if d > 0 {
			o.timeout = d
		}
	}
}

// WithLogger sets the logger for orchestrator operations.
func WithLogger(logger *slog.Logger) OrchestratorOption {
	return func(o *Orchestrator) {
		if logger != nil {
			o.logger = logger
		}
	}
}

// WithTracer sets the OpenTelemetry tracer for distributed tracing.
func WithTracer(tracer trace.Tracer) OrchestratorOption {
	return func(o *Orchestrator) {
		if tracer != nil {
			o.tracer = tracer
		}
	}
}

// WithEventBus sets the event bus for emitting orchestration events.
func WithEventBus(bus EventBus) OrchestratorOption {
	return func(o *Orchestrator) {
		if bus != nil {
			o.eventBus = bus
		}
	}
}

// WithDecisionLogWriter sets the decision log writer for external observability.
func WithDecisionLogWriter(writer DecisionLogWriter) OrchestratorOption {
	return func(o *Orchestrator) {
		if writer != nil {
			o.logWriter = writer
		}
	}
}

// NewOrchestrator creates a new Orchestrator with the specified components and options.
//
// Required components:
//   - observer: Gathers execution state from the graph
//   - thinker: Makes decisions using LLM reasoning
//   - actor: Executes decisions and updates graph state
//
// The orchestrator coordinates these components in a loop until the mission
// completes, fails, or hits resource limits.
func NewOrchestrator(observer *Observer, thinker *Thinker, actor *Actor, options ...OrchestratorOption) *Orchestrator {
	o := &Orchestrator{
		observer:      observer,
		thinker:       thinker,
		actor:         actor,
		maxIterations: 100,        // Reasonable default to prevent infinite loops
		maxConcurrent: 10,         // Default concurrency limit
		budget:        0,          // Unlimited by default
		timeout:       0,          // No timeout by default
		logger:        slog.Default(),
		tracer:        trace.NewNoopTracerProvider().Tracer("orchestrator"),
	}

	for _, opt := range options {
		opt(o)
	}

	return o
}

// OrchestratorResult contains the complete result of an orchestration run.
type OrchestratorResult struct {
	// MissionID is the ID of the mission that was orchestrated
	MissionID string

	// Status is the final status of the orchestration run
	Status OrchestratorStatus

	// TotalIterations is the number of observe-think-act cycles performed
	TotalIterations int

	// TotalDecisions is the number of LLM decisions made
	TotalDecisions int

	// TotalTokensUsed is the total tokens consumed by LLM calls
	TotalTokensUsed int

	// Duration is the total time spent in orchestration
	Duration time.Duration

	// CompletedNodes is the number of workflow nodes that completed
	CompletedNodes int

	// FailedNodes is the number of workflow nodes that failed
	FailedNodes int

	// Error contains any fatal error that occurred
	Error error

	// StopReason explains why orchestration stopped (for completed missions)
	StopReason string

	// FinalState is the last observed state before stopping
	FinalState *ObservationState
}

// Run executes the orchestration loop for the specified mission.
//
// The orchestration loop repeats until:
//   - The mission completes (all nodes executed)
//   - A terminal decision is made (complete action)
//   - Max iterations are reached
//   - Token budget is exhausted
//   - Timeout occurs
//   - Context is cancelled
//   - A fatal error occurs
//
// Each iteration follows the pattern:
//  1. OBSERVE - Gather current execution state from graph
//  2. CHECK - Verify termination conditions and constraints
//  3. THINK - Use LLM to make a decision about what to do next
//  4. LOG - Record the decision for observability
//  5. ACT - Execute the decision (run agent, skip node, etc.)
//  6. VERIFY - Check if the action was terminal
//
// Returns an OrchestratorResult summarizing the execution.
func (o *Orchestrator) Run(ctx context.Context, missionID string) (*OrchestratorResult, error) {
	startTime := time.Now()

	// Validate mission ID
	parsedMissionID, err := types.ParseID(missionID)
	if err != nil {
		return nil, fmt.Errorf("invalid mission ID: %w", err)
	}

	// Apply timeout if configured
	if o.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.timeout)
		defer cancel()
	}

	// Start tracing span
	ctx, span := o.tracer.Start(ctx, "orchestrator.Run")
	defer span.End()

	// Emit mission started event
	if o.eventBus != nil {
		o.eventBus.Publish(events.Event{
			Type:      events.EventMissionStarted,
			Timestamp: time.Now(),
			MissionID: parsedMissionID,
			TraceID:   span.SpanContext().TraceID().String(),
			SpanID:    span.SpanContext().SpanID().String(),
			Payload: events.MissionStartedPayload{
				MissionID: parsedMissionID,
			},
		})
	}

	o.logger.Info("orchestrator starting",
		"mission_id", missionID,
		"max_iterations", o.maxIterations,
		"max_concurrent", o.maxConcurrent,
		"budget", o.budget,
	)

	// Initialize result
	result := &OrchestratorResult{
		MissionID: missionID,
		Status:    StatusCompleted, // Optimistic default
	}

	// Track token usage
	totalTokens := 0

	// Main orchestration loop
	for iteration := 0; iteration < o.maxIterations; iteration++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			o.logger.Warn("orchestrator cancelled", "iteration", iteration, "error", ctx.Err())
			result.Status = StatusCancelled
			result.Error = ctx.Err()
			result.Duration = time.Since(startTime)
			return result, nil
		default:
		}

		o.logger.Debug("orchestration iteration starting", "iteration", iteration)

		// 1. OBSERVE - Gather current state
		state, err := o.observer.Observe(ctx, missionID)
		if err != nil {
			o.logger.Error("observation failed", "iteration", iteration, "error", err)
			result.Status = StatusFailed
			result.Error = fmt.Errorf("observation failed: %w", err)
			result.TotalIterations = iteration
			result.Duration = time.Since(startTime)
			return result, err
		}

		result.FinalState = state

		o.logger.Debug("observation complete",
			"iteration", iteration,
			"ready_nodes", len(state.ReadyNodes),
			"running_nodes", len(state.RunningNodes),
			"completed_nodes", len(state.CompletedNodes),
			"failed_nodes", len(state.FailedNodes),
		)

		// 2. CHECK TERMINATION CONDITIONS

		// Check if workflow is naturally complete (no work left)
		if len(state.ReadyNodes) == 0 && len(state.RunningNodes) == 0 {
			o.logger.Info("workflow naturally complete", "iteration", iteration)
			result.Status = StatusCompleted
			result.TotalIterations = iteration + 1
			result.CompletedNodes = len(state.CompletedNodes)
			result.FailedNodes = len(state.FailedNodes)
			result.Duration = time.Since(startTime)
			result.TotalTokensUsed = totalTokens
			result.StopReason = "all workflow nodes completed or no more work available"
			return result, nil
		}

		// Check concurrency limit
		if len(state.RunningNodes) >= o.maxConcurrent {
			o.logger.Debug("concurrency limit reached, skipping iteration",
				"iteration", iteration,
				"running", len(state.RunningNodes),
				"limit", o.maxConcurrent,
			)
			// Sleep briefly and retry in next iteration
			time.Sleep(1 * time.Second)
			continue
		}

		// Check token budget
		if o.budget > 0 && totalTokens >= o.budget {
			o.logger.Warn("token budget exceeded",
				"iteration", iteration,
				"used", totalTokens,
				"budget", o.budget,
			)
			result.Status = StatusBudgetExceeded
			result.TotalIterations = iteration + 1
			result.TotalTokensUsed = totalTokens
			result.Duration = time.Since(startTime)
			return result, nil
		}

		// 3. THINK - LLM decides what to do next
		thinkResult, err := o.thinker.Think(ctx, state)
		if err != nil {
			o.logger.Error("thinking failed", "iteration", iteration, "error", err)
			result.Status = StatusFailed
			result.Error = fmt.Errorf("thinking failed: %w", err)
			result.TotalIterations = iteration + 1
			result.TotalTokensUsed = totalTokens
			result.Duration = time.Since(startTime)
			return result, err
		}

		// Update token usage
		totalTokens += thinkResult.TotalTokens
		result.TotalDecisions++

		o.logger.Info("decision made",
			"iteration", iteration,
			"action", thinkResult.Decision.Action,
			"target", thinkResult.Decision.TargetNodeID,
			"confidence", thinkResult.Decision.Confidence,
			"tokens", thinkResult.TotalTokens,
			"latency_ms", thinkResult.Latency.Milliseconds(),
		)

		// 4. LOG DECISION - Write to graph and external systems
		if err := o.logDecision(ctx, thinkResult, iteration, missionID); err != nil {
			o.logger.Warn("failed to log decision", "iteration", iteration, "error", err)
			// Non-fatal error, continue
		}

		// 5. ACT - Execute the decision
		actionResult, err := o.actor.Act(ctx, thinkResult.Decision, missionID)
		if err != nil {
			o.logger.Error("action failed", "iteration", iteration, "error", err)
			result.Status = StatusFailed
			result.Error = fmt.Errorf("action failed: %w", err)
			result.TotalIterations = iteration + 1
			result.TotalTokensUsed = totalTokens
			result.Duration = time.Since(startTime)
			return result, err
		}

		// Log action result
		if err := o.logAction(ctx, actionResult, iteration, missionID); err != nil {
			o.logger.Warn("failed to log action", "iteration", iteration, "error", err)
			// Non-fatal error, continue
		}

		o.logger.Debug("action completed",
			"iteration", iteration,
			"action", actionResult.Action,
			"terminal", actionResult.IsTerminal,
			"error", actionResult.Error,
		)

		// Emit progress event
		if o.eventBus != nil {
			o.eventBus.Publish(events.Event{
				Type:      events.EventMissionProgress,
				Timestamp: time.Now(),
				MissionID: parsedMissionID,
				Payload: events.MissionProgressPayload{
					MissionID:      parsedMissionID,
					CompletedNodes: len(state.CompletedNodes),
					TotalNodes:     state.GraphSummary.TotalNodes,
				},
			})
		}

		// 6. CHECK TERMINAL - Did this action end the workflow?
		if actionResult.IsTerminal {
			o.logger.Info("terminal action executed", "iteration", iteration+1)
			result.Status = StatusCompleted
			result.TotalIterations = iteration + 1
			result.CompletedNodes = len(state.CompletedNodes)
			result.FailedNodes = len(state.FailedNodes)
			result.TotalTokensUsed = totalTokens
			result.Duration = time.Since(startTime)
			result.StopReason = thinkResult.Decision.StopReason
			return result, nil
		}

		// Check if action resulted in an error
		if actionResult.Error != nil {
			// Non-terminal error - log and continue
			// The failed node is tracked in the graph
			o.logger.Warn("action error (non-terminal)",
				"iteration", iteration,
				"error", actionResult.Error,
			)
		}
	}

	// Max iterations reached
	o.logger.Warn("max iterations reached", "iterations", o.maxIterations)
	result.Status = StatusMaxIterations
	result.TotalIterations = o.maxIterations
	result.TotalTokensUsed = totalTokens
	result.Duration = time.Since(startTime)

	if result.FinalState != nil {
		result.CompletedNodes = len(result.FinalState.CompletedNodes)
		result.FailedNodes = len(result.FinalState.FailedNodes)
	}

	return result, nil
}

// logDecision writes the decision to the graph and external log systems.
func (o *Orchestrator) logDecision(ctx context.Context, result *ThinkResult, iteration int, missionID string) error {
	// Log to external system (Langfuse, etc.) if configured
	if o.logWriter != nil {
		if err := o.logWriter.LogDecision(ctx, result.Decision, result, iteration, missionID); err != nil {
			return fmt.Errorf("failed to write decision log: %w", err)
		}
	}

	// Emit decision event
	if o.eventBus != nil {
		parsedMissionID, _ := types.ParseID(missionID)
		o.eventBus.Publish(events.Event{
			Type:      events.EventMissionProgress,
			Timestamp: time.Now(),
			MissionID: parsedMissionID,
			Attrs: map[string]any{
				"iteration":   iteration,
				"action":      result.Decision.Action.String(),
				"target":      result.Decision.TargetNodeID,
				"confidence":  result.Decision.Confidence,
				"tokens":      result.TotalTokens,
				"latency_ms":  result.Latency.Milliseconds(),
				"retry_count": result.RetryCount,
			},
		})
	}

	return nil
}

// logAction writes the action result to external log systems.
func (o *Orchestrator) logAction(ctx context.Context, action *ActionResult, iteration int, missionID string) error {
	// Log to external system (Langfuse, etc.) if configured
	if o.logWriter != nil {
		if err := o.logWriter.LogAction(ctx, action, iteration, missionID); err != nil {
			return fmt.Errorf("failed to write action log: %w", err)
		}
	}

	// Emit action-specific events
	if o.eventBus != nil {
		parsedMissionID, _ := types.ParseID(missionID)

		// If an agent was executed, emit node events
		if action.AgentExecution != nil {
			exec := action.AgentExecution

			switch exec.Status {
			case schema.ExecutionStatusCompleted:
				o.eventBus.Publish(events.Event{
					Type:      events.EventNodeCompleted,
					Timestamp: time.Now(),
					MissionID: parsedMissionID,
					Payload: events.NodeCompletedPayload{
						MissionID: parsedMissionID,
						NodeID:    exec.WorkflowNodeID,
						Duration:  exec.Duration(),
					},
				})

			case schema.ExecutionStatusFailed:
				o.eventBus.Publish(events.Event{
					Type:      events.EventNodeFailed,
					Timestamp: time.Now(),
					MissionID: parsedMissionID,
					Payload: events.NodeFailedPayload{
						MissionID: parsedMissionID,
						NodeID:    exec.WorkflowNodeID,
						Error:     exec.Error,
						Duration:  exec.Duration(),
					},
				})
			}
		}
	}

	return nil
}

// String returns a human-readable representation of the orchestrator result.
func (r *OrchestratorResult) String() string {
	return fmt.Sprintf(
		"OrchestratorResult{Status: %s, Iterations: %d, Decisions: %d, Tokens: %d, Duration: %s, Completed: %d, Failed: %d}",
		r.Status,
		r.TotalIterations,
		r.TotalDecisions,
		r.TotalTokensUsed,
		r.Duration,
		r.CompletedNodes,
		r.FailedNodes,
	)
}
