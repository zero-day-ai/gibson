package workflow

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/guardrail"
	"github.com/zero-day-ai/gibson/internal/harness"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// VerboseEventBus is a minimal interface for emitting verbose events.
// This avoids import cycles with the verbose package.
// Events are sent as interface{} (any type) to maximize compatibility.
type VerboseEventBus interface {
	Emit(ctx context.Context, event interface{}) error
}

// WorkflowExecutor orchestrates the execution of workflow DAGs.
// It manages parallel node execution, respects dependencies, enforces
// parallelism limits, and integrates with guardrails and observability.
type WorkflowExecutor struct {
	guardrails  *guardrail.GuardrailPipeline
	logger      *slog.Logger
	tracer      trace.Tracer
	maxParallel int
	verboseBus  VerboseEventBus // Optional event bus for verbose logging
}

// ExecutorOption is a functional option for configuring WorkflowExecutor
type ExecutorOption func(*WorkflowExecutor)

// WithGuardrails configures the executor to use the specified guardrail pipeline
// for input/output validation and filtering during workflow execution.
func WithGuardrails(pipeline *guardrail.GuardrailPipeline) ExecutorOption {
	return func(e *WorkflowExecutor) {
		e.guardrails = pipeline
	}
}

// WithLogger configures the executor to use the specified structured logger
// for workflow execution logging.
func WithLogger(logger *slog.Logger) ExecutorOption {
	return func(e *WorkflowExecutor) {
		e.logger = logger
	}
}

// WithTracer configures the executor to use the specified OpenTelemetry tracer
// for distributed tracing during workflow execution.
func WithTracer(tracer trace.Tracer) ExecutorOption {
	return func(e *WorkflowExecutor) {
		e.tracer = tracer
	}
}

// WithMaxParallel configures the maximum number of nodes that can execute
// concurrently. This limits resource consumption and prevents overwhelming
// downstream systems.
func WithMaxParallel(n int) ExecutorOption {
	return func(e *WorkflowExecutor) {
		if n > 0 {
			e.maxParallel = n
		}
	}
}

// WithVerboseEventBus configures the executor to emit verbose events during
// DAG node execution (start, complete, fail, skip).
func WithVerboseEventBus(bus VerboseEventBus) ExecutorOption {
	return func(e *WorkflowExecutor) {
		e.verboseBus = bus
	}
}

// NewWorkflowExecutor creates a new WorkflowExecutor with the specified options.
// Default configuration:
//   - No guardrails
//   - Default logger (slog.Default())
//   - No tracer
//   - maxParallel: 10
func NewWorkflowExecutor(opts ...ExecutorOption) *WorkflowExecutor {
	executor := &WorkflowExecutor{
		logger:      slog.Default(),
		maxParallel: 10,
	}

	// Apply options
	for _, opt := range opts {
		opt(executor)
	}

	return executor
}

// Execute executes the workflow DAG, respecting dependencies and parallelism limits.
// It uses a semaphore pattern to limit concurrent node execution and ensures all
// dependencies are satisfied before executing each node.
//
// The execution process:
//  1. Creates a parent span for workflow execution
//  2. Initializes WorkflowState with all nodes in pending status
//  3. Main execution loop:
//     a. Get ready nodes (pending nodes with all dependencies completed)
//     b. If no ready nodes and workflow not complete, return DeadlockError
//     c. Dispatch ready nodes in parallel (limited by maxParallel)
//     d. Wait for batch to complete
//     e. Update state with results
//     f. Check for context cancellation
//  4. Aggregate findings from all node results
//  5. Return WorkflowResult with execution summary
//
// Returns:
//   - *WorkflowResult: Complete workflow execution result with node results and findings
//   - error: Non-nil if workflow execution fails (deadlock, cancellation, etc.)
func (e *WorkflowExecutor) Execute(ctx context.Context, workflow *Workflow, harness harness.AgentHarness) (*WorkflowResult, error) {
	// Create parent span for workflow execution
	var span trace.Span
	if e.tracer != nil {
		ctx, span = e.tracer.Start(ctx, "workflow.execute",
			trace.WithAttributes(
				attribute.String("workflow.id", workflow.ID.String()),
				attribute.String("workflow.name", workflow.Name),
				attribute.Int("workflow.node_count", len(workflow.Nodes)),
			),
		)
		defer span.End()
	}

	e.logger.InfoContext(ctx, "Starting workflow execution",
		"workflow_id", workflow.ID,
		"workflow_name", workflow.Name,
		"node_count", len(workflow.Nodes),
	)

	// Initialize workflow state
	state := NewWorkflowState(workflow)
	state.Status = WorkflowStatusRunning

	// Track workflow start time
	startTime := time.Now()

	// Main execution loop
	for {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			e.logger.WarnContext(ctx, "Workflow execution cancelled",
				"workflow_id", workflow.ID,
				"reason", ctx.Err(),
			)
			state.Status = WorkflowStatusCancelled
			now := time.Now()
			state.CompletedAt = &now

			return e.buildWorkflowResult(state, startTime, &WorkflowError{
				Code:    WorkflowErrorWorkflowCancelled,
				Message: "Workflow execution was cancelled",
				Cause:   ctx.Err(),
			}), ctx.Err()
		default:
		}

		// Get ready nodes
		readyNodes := state.GetReadyNodes()

		// Check for completion
		if len(readyNodes) == 0 {
			if state.IsComplete() {
				// Workflow completed successfully
				e.logger.InfoContext(ctx, "Workflow execution completed",
					"workflow_id", workflow.ID,
					"duration", time.Since(startTime),
				)
				state.Status = WorkflowStatusCompleted
				now := time.Now()
				state.CompletedAt = &now

				return e.buildWorkflowResult(state, startTime, nil), nil
			}

			// No ready nodes but workflow not complete - deadlock
			e.logger.ErrorContext(ctx, "Workflow deadlock detected",
				"workflow_id", workflow.ID,
			)
			state.Status = WorkflowStatusFailed
			now := time.Now()
			state.CompletedAt = &now

			deadlockErr := &WorkflowError{
				Code:    WorkflowErrorDeadlock,
				Message: "No ready nodes available but workflow is not complete - potential dependency cycle or failed dependencies",
			}

			return e.buildWorkflowResult(state, startTime, deadlockErr), deadlockErr
		}

		// Execute ready nodes in parallel (limited by semaphore)
		e.executeNodeBatch(ctx, readyNodes, state, harness)
	}
}

// executeNodeBatch executes a batch of ready nodes in parallel, respecting
// the maxParallel limit using a semaphore pattern.
func (e *WorkflowExecutor) executeNodeBatch(
	ctx context.Context,
	nodes []*WorkflowNode,
	state *WorkflowState,
	harness harness.AgentHarness,
) {
	// Create semaphore to limit parallelism
	sem := make(chan struct{}, e.maxParallel)

	// Create wait group for batch synchronization
	var wg sync.WaitGroup

	// Execute each node in a goroutine
	for _, node := range nodes {
		wg.Add(1)

		// Acquire semaphore slot
		sem <- struct{}{}

		// Execute node in goroutine
		go func(n *WorkflowNode) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore slot

			// Mark node as started
			state.MarkNodeStarted(n.ID)

			// Emit node started event
			e.emitNodeEvent(ctx, n, "started", 0, nil)

			// Track node execution time
			nodeStartTime := time.Now()

			// Execute the node (delegates to node_executor.go)
			result, err := e.executeNode(ctx, n, state, harness)

			// Calculate node duration
			nodeDuration := time.Since(nodeStartTime)

			// Update state based on result
			if err != nil || result.Status == NodeStatusFailed {
				var nodeErr error
				if result != nil && result.Error != nil {
					nodeErr = result.Error
				} else if err != nil {
					nodeErr = err
				}
				state.MarkNodeFailed(n.ID, nodeErr)
				// Emit node failed event
				e.emitNodeEvent(ctx, n, "failed", nodeDuration, nodeErr)
			} else if result.Status == NodeStatusSkipped {
				reason := "node was skipped"
				if result.Metadata != nil {
					if skipReason, ok := result.Metadata["skip_reason"].(string); ok {
						reason = skipReason
					}
				}
				state.MarkNodeSkipped(n.ID, reason)
				// Emit node skipped event
				e.emitNodeEvent(ctx, n, "skipped", nodeDuration, nil)
			} else {
				state.MarkNodeCompleted(n.ID, result)
				// Emit node completed event
				e.emitNodeEvent(ctx, n, "completed", nodeDuration, nil)
			}
		}(node)
	}

	// Wait for all nodes in batch to complete
	wg.Wait()
}

// emitNodeEvent emits a verbose event for a node execution event.
// If verboseBus is nil, this is a no-op.
// Events are sent as maps to avoid import cycles with the verbose package.
func (e *WorkflowExecutor) emitNodeEvent(ctx context.Context, node *WorkflowNode, status string, duration time.Duration, err error) {
	if e.verboseBus == nil {
		return
	}

	// Create event as a map to avoid import cycles
	nodeData := map[string]interface{}{
		"node_id":   node.ID,
		"node_type": string(node.Type),
		"status":    status,
		"duration":  duration,
	}

	if err != nil {
		nodeData["error"] = err.Error()
	}

	// Emit to the verbose event bus (adapter will convert to proper format)
	_ = e.verboseBus.Emit(ctx, nodeData)
}

// buildWorkflowResult constructs the final WorkflowResult from the execution state.
func (e *WorkflowExecutor) buildWorkflowResult(
	state *WorkflowState,
	startTime time.Time,
	workflowErr *WorkflowError,
) *WorkflowResult {
	// Calculate summary statistics
	nodesExecuted := 0
	nodesFailed := 0
	nodesSkipped := 0

	// Aggregate all findings from node results
	var allFindings []agent.Finding

	// Build node results map
	nodeResults := make(map[string]*NodeResult)

	state.mu.RLock()
	for nodeID, nodeState := range state.NodeStates {
		// Get result if available
		result := state.Results[nodeID]
		if result != nil {
			nodeResults[nodeID] = result

			// Aggregate findings
			if len(result.Findings) > 0 {
				allFindings = append(allFindings, result.Findings...)
			}
		}

		// Count node statuses
		switch nodeState.Status {
		case NodeStatusCompleted:
			nodesExecuted++
		case NodeStatusFailed:
			nodesFailed++
		case NodeStatusSkipped:
			nodesSkipped++
		}
	}
	state.mu.RUnlock()

	return &WorkflowResult{
		WorkflowID:    state.WorkflowID,
		Status:        state.Status,
		NodeResults:   nodeResults,
		Findings:      allFindings,
		TotalDuration: time.Since(startTime),
		NodesExecuted: nodesExecuted,
		NodesFailed:   nodesFailed,
		NodesSkipped:  nodesSkipped,
		Error:         workflowErr,
	}
}
