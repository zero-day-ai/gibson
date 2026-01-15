package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/harness"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// executeNode executes a single workflow node with timeout and retry support.
// It creates a child span for observability and routes execution to the appropriate
// node type handler. Returns the execution result or an error if execution fails.
func (we *WorkflowExecutor) executeNode(
	ctx context.Context,
	node *WorkflowNode,
	state *WorkflowState,
	harness harness.AgentHarness,
) (*NodeResult, error) {
	// Create a child span for this node execution
	ctx, span := harness.Tracer().Start(ctx, "workflow.execute_node",
		trace.WithAttributes(
			attribute.String("node.id", node.ID),
			attribute.String("node.type", string(node.Type)),
			attribute.String("node.name", node.Name),
		),
	)
	defer span.End()

	harness.Logger().Info("executing workflow node",
		"node_id", node.ID,
		"node_type", node.Type,
		"node_name", node.Name,
	)

	// Create execution function that will be retried if needed
	executeFn := func() (*NodeResult, error) {
		// Apply timeout if specified
		execCtx := ctx
		var cancel context.CancelFunc
		if node.Timeout > 0 {
			execCtx, cancel = context.WithTimeout(ctx, node.Timeout)
			defer cancel()
		}

		// Route to appropriate executor based on node type
		var result *NodeResult
		var err error

		switch node.Type {
		case NodeTypeAgent:
			result, err = we.executeAgentNode(execCtx, node, harness)
		case NodeTypeTool:
			result, err = we.executeToolNode(execCtx, node, harness)
		case NodeTypePlugin:
			result, err = we.executePluginNode(execCtx, node, harness)
		case NodeTypeCondition:
			result, err = we.executeConditionNode(execCtx, node, state)
		case NodeTypeParallel:
			result, err = we.executeParallelNode(execCtx, node, state, harness)
		case NodeTypeJoin:
			result, err = we.executeJoinNode(execCtx, node, state)
		default:
			err = &NodeError{
				Code:    "INVALID_NODE_TYPE",
				Message: fmt.Sprintf("unknown node type: %s", node.Type),
			}
		}

		// Check for timeout
		if execCtx.Err() == context.DeadlineExceeded {
			return nil, &WorkflowError{
				Code:    WorkflowErrorNodeTimeout,
				Message: fmt.Sprintf("node execution timed out after %v", node.Timeout),
				NodeID:  node.ID,
			}
		}

		return result, err
	}

	// Execute with retry if retry policy is configured
	var result *NodeResult
	var err error
	if node.RetryPolicy != nil {
		result, err = we.executeWithRetry(ctx, node, executeFn)
	} else {
		result, err = executeFn()
	}

	// Record span status
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		harness.Logger().Error("node execution failed",
			"node_id", node.ID,
			"error", err,
		)
	} else {
		span.SetStatus(codes.Ok, "node executed successfully")
		harness.Logger().Info("node execution completed",
			"node_id", node.ID,
			"status", result.Status,
		)
	}

	return result, err
}

// executeAgentNode executes an agent node by delegating to the specified agent.
func (we *WorkflowExecutor) executeAgentNode(
	ctx context.Context,
	node *WorkflowNode,
	harness harness.AgentHarness,
) (*NodeResult, error) {
	// Create agent execution span for Neo4j graph tracking
	ctx, agentSpan := harness.Tracer().Start(ctx, "gibson.agent.execute",
		trace.WithAttributes(
			attribute.String("gibson.mission.id", harness.MissionID().String()),
			attribute.String("gibson.agent.name", node.AgentName),
		),
	)
	defer agentSpan.End()

	// Add task ID attribute if available
	var taskID string
	if node.AgentTask != nil {
		taskID = node.AgentTask.ID.String()
		agentSpan.SetAttributes(attribute.String("gibson.task.id", taskID))
	}

	startTime := time.Now()

	// Validate agent node configuration
	if node.AgentName == "" {
		err := &NodeError{
			Code:    "INVALID_AGENT_NODE",
			Message: "agent_name is required for agent nodes",
		}
		agentSpan.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	if node.AgentTask == nil {
		err := &NodeError{
			Code:    "INVALID_AGENT_NODE",
			Message: "agent_task is required for agent nodes",
		}
		agentSpan.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// Extract parent span ID for PART_OF relationship creation
	// The parent span is the workflow execution span, not the agent span
	parentSpanID := ""
	if agentSpan.SpanContext().IsValid() {
		// We want the parent of the current agent span, which is the workflow/node execution span
		// OpenTelemetry doesn't expose parent span ID directly, but we can get it from the context
		// The graph processor derives relationships from trace context automatically
	}

	// Emit agent.started event for graph processing
	we.emitAgentStarted(ctx, node.AgentName, taskID, parentSpanID)

	// Delegate to the agent
	agentResult, err := harness.DelegateToAgent(ctx, node.AgentName, *node.AgentTask)

	// Calculate duration
	duration := time.Since(startTime)

	if err != nil {
		// Emit agent.failed event
		we.emitAgentFailed(ctx, node.AgentName, duration, err.Error())

		nodeErr := &NodeError{
			Code:    "AGENT_EXECUTION_FAILED",
			Message: fmt.Sprintf("failed to execute agent %s: %v", node.AgentName, err),
			Cause:   err,
		}
		agentSpan.SetStatus(codes.Error, nodeErr.Error())
		return nil, nodeErr
	}

	// Convert agent result to node result
	nodeResult := &NodeResult{
		NodeID:      node.ID,
		Status:      NodeStatusCompleted,
		Output:      agentResult.Output,
		Findings:    agentResult.Findings,
		Duration:    duration,
		StartedAt:   startTime,
		CompletedAt: time.Now(),
		Metadata: map[string]any{
			"agent_name":    node.AgentName,
			"agent_metrics": agentResult.Metrics,
		},
	}

	// Check if agent execution failed
	if agentResult.Status == agent.ResultStatusFailed {
		nodeResult.Status = NodeStatusFailed
		errorMsg := "agent execution failed"
		if agentResult.Error != nil {
			nodeResult.Error = &NodeError{
				Code:    agentResult.Error.Code,
				Message: agentResult.Error.Message,
				Details: agentResult.Error.Details,
			}
			errorMsg = nodeResult.Error.Message
			agentSpan.SetStatus(codes.Error, nodeResult.Error.Error())
		} else {
			agentSpan.SetStatus(codes.Error, errorMsg)
		}

		// Emit agent.failed event
		we.emitAgentFailed(ctx, node.AgentName, duration, errorMsg)
	} else {
		agentSpan.SetStatus(codes.Ok, "agent executed successfully")

		// Create output summary for completed event
		outputSummary := fmt.Sprintf("Agent completed successfully with %d findings", len(agentResult.Findings))
		if agentResult.Output != nil {
			// Try to extract a meaningful summary from output
			if summary, ok := agentResult.Output["summary"].(string); ok && summary != "" {
				outputSummary = summary
			}
		}

		// Emit agent.completed event
		we.emitAgentCompleted(ctx, node.AgentName, duration, outputSummary)
	}

	return nodeResult, nil
}

// executeToolNode executes a tool node by calling the specified tool.
func (we *WorkflowExecutor) executeToolNode(
	ctx context.Context,
	node *WorkflowNode,
	harness harness.AgentHarness,
) (*NodeResult, error) {
	startTime := time.Now()

	// Validate tool node configuration
	if node.ToolName == "" {
		return nil, &NodeError{
			Code:    "INVALID_TOOL_NODE",
			Message: "tool_name is required for tool nodes",
		}
	}

	if node.ToolInput == nil {
		node.ToolInput = make(map[string]any)
	}

	// Call the tool
	toolResult, err := harness.CallTool(ctx, node.ToolName, node.ToolInput)
	if err != nil {
		return nil, &NodeError{
			Code:    "TOOL_EXECUTION_FAILED",
			Message: fmt.Sprintf("failed to execute tool %s: %v", node.ToolName, err),
			Cause:   err,
		}
	}

	return &NodeResult{
		NodeID:      node.ID,
		Status:      NodeStatusCompleted,
		Output:      toolResult,
		Duration:    time.Since(startTime),
		StartedAt:   startTime,
		CompletedAt: time.Now(),
		Metadata: map[string]any{
			"tool_name": node.ToolName,
		},
	}, nil
}

// executePluginNode executes a plugin node by calling the specified plugin method.
func (we *WorkflowExecutor) executePluginNode(
	ctx context.Context,
	node *WorkflowNode,
	harness harness.AgentHarness,
) (*NodeResult, error) {
	startTime := time.Now()

	// Validate plugin node configuration
	if node.PluginName == "" {
		return nil, &NodeError{
			Code:    "INVALID_PLUGIN_NODE",
			Message: "plugin_name is required for plugin nodes",
		}
	}

	if node.PluginMethod == "" {
		return nil, &NodeError{
			Code:    "INVALID_PLUGIN_NODE",
			Message: "plugin_method is required for plugin nodes",
		}
	}

	if node.PluginParams == nil {
		node.PluginParams = make(map[string]any)
	}

	// Query the plugin
	pluginResult, err := harness.QueryPlugin(ctx, node.PluginName, node.PluginMethod, node.PluginParams)
	if err != nil {
		return nil, &NodeError{
			Code:    "PLUGIN_EXECUTION_FAILED",
			Message: fmt.Sprintf("failed to execute plugin %s.%s: %v", node.PluginName, node.PluginMethod, err),
			Cause:   err,
		}
	}

	// Convert plugin result to output map
	output := map[string]any{
		"result": pluginResult,
	}

	return &NodeResult{
		NodeID:      node.ID,
		Status:      NodeStatusCompleted,
		Output:      output,
		Duration:    time.Since(startTime),
		StartedAt:   startTime,
		CompletedAt: time.Now(),
		Metadata: map[string]any{
			"plugin_name":   node.PluginName,
			"plugin_method": node.PluginMethod,
		},
	}, nil
}

// executeConditionNode executes a condition node by evaluating the conditional expression.
// Returns a result indicating which branch should be taken (true or false).
func (we *WorkflowExecutor) executeConditionNode(
	ctx context.Context,
	node *WorkflowNode,
	state *WorkflowState,
) (*NodeResult, error) {
	startTime := time.Now()

	// Validate condition node configuration
	if node.Condition == nil {
		return nil, &NodeError{
			Code:    "INVALID_CONDITION_NODE",
			Message: "condition is required for condition nodes",
		}
	}

	if node.Condition.Expression == "" {
		return nil, &NodeError{
			Code:    "INVALID_CONDITION_NODE",
			Message: "condition expression is required",
		}
	}

	// Build evaluation context from workflow state
	evalContext := &EvaluationContext{
		NodeResults: state.Results,
		Variables:   make(map[string]any),
	}

	// Create condition evaluator
	evaluator := NewConditionEvaluator()

	// Evaluate the condition
	result, err := evaluator.Evaluate(node.Condition.Expression, evalContext)
	if err != nil {
		return nil, &NodeError{
			Code:    "CONDITION_EVALUATION_FAILED",
			Message: fmt.Sprintf("failed to evaluate condition: %v", err),
			Cause:   err,
		}
	}

	// Determine which branch to take
	var nextNodes []string
	if result {
		nextNodes = node.Condition.TrueBranch
	} else {
		nextNodes = node.Condition.FalseBranch
	}

	return &NodeResult{
		NodeID: node.ID,
		Status: NodeStatusCompleted,
		Output: map[string]any{
			"condition_result": result,
			"next_nodes":       nextNodes,
		},
		Duration:    time.Since(startTime),
		StartedAt:   startTime,
		CompletedAt: time.Now(),
		Metadata: map[string]any{
			"expression": node.Condition.Expression,
			"result":     result,
		},
	}, nil
}

// executeParallelNode executes a parallel node by running all sub-nodes concurrently.
// Uses a WaitGroup to coordinate concurrent execution and collects all results.
func (we *WorkflowExecutor) executeParallelNode(
	ctx context.Context,
	node *WorkflowNode,
	state *WorkflowState,
	harness harness.AgentHarness,
) (*NodeResult, error) {
	startTime := time.Now()

	// Validate parallel node configuration
	if len(node.SubNodes) == 0 {
		return nil, &NodeError{
			Code:    "INVALID_PARALLEL_NODE",
			Message: "parallel nodes must have at least one sub-node",
		}
	}

	// Execute all sub-nodes concurrently
	var wg sync.WaitGroup
	results := make([]*NodeResult, len(node.SubNodes))
	errors := make([]error, len(node.SubNodes))
	resultsMu := sync.Mutex{}

	for i, subNode := range node.SubNodes {
		wg.Add(1)
		go func(index int, sn *WorkflowNode) {
			defer wg.Done()

			// Execute the sub-node using the detailed executor
			subResult, err := we.executeNode(ctx, sn, state, harness)

			// Store result safely
			resultsMu.Lock()
			results[index] = subResult
			errors[index] = err
			resultsMu.Unlock()
		}(i, subNode)
	}

	// Wait for all sub-nodes to complete
	wg.Wait()

	// Collect results and check for errors
	subResults := make(map[string]*NodeResult)
	var failedNodes []string
	allFindings := []agent.Finding{}

	for i, subNode := range node.SubNodes {
		if errors[i] != nil {
			failedNodes = append(failedNodes, subNode.ID)
		} else if results[i] != nil {
			subResults[subNode.ID] = results[i]
			// Aggregate findings from all sub-nodes
			if results[i].Findings != nil {
				allFindings = append(allFindings, results[i].Findings...)
			}
		}
	}

	// Determine overall status
	status := NodeStatusCompleted
	var nodeError *NodeError
	if len(failedNodes) > 0 {
		status = NodeStatusFailed
		nodeError = &NodeError{
			Code:    "PARALLEL_EXECUTION_FAILED",
			Message: fmt.Sprintf("%d sub-nodes failed", len(failedNodes)),
			Details: map[string]any{
				"failed_nodes": failedNodes,
			},
		}
	}

	return &NodeResult{
		NodeID: node.ID,
		Status: status,
		Output: map[string]any{
			"sub_results": subResults,
		},
		Error:       nodeError,
		Findings:    allFindings,
		Duration:    time.Since(startTime),
		StartedAt:   startTime,
		CompletedAt: time.Now(),
		Metadata: map[string]any{
			"total_sub_nodes":  len(node.SubNodes),
			"failed_sub_nodes": len(failedNodes),
		},
	}, nil
}

// executeJoinNode executes a join node by aggregating results from all predecessor nodes.
// This is useful for combining outputs from parallel branches or collecting data from multiple sources.
func (we *WorkflowExecutor) executeJoinNode(
	ctx context.Context,
	node *WorkflowNode,
	state *WorkflowState,
) (*NodeResult, error) {
	startTime := time.Now()

	// Validate join node configuration
	if len(node.Dependencies) == 0 {
		return nil, &NodeError{
			Code:    "INVALID_JOIN_NODE",
			Message: "join nodes must have at least one dependency",
		}
	}

	// Collect results from all dependencies
	aggregatedResults := make(map[string]*NodeResult)
	aggregatedOutput := make(map[string]any)
	allFindings := []agent.Finding{}
	var failedDeps []string

	for _, depID := range node.Dependencies {
		depResult := state.GetResult(depID)
		if depResult == nil {
			return nil, &NodeError{
				Code:    "MISSING_DEPENDENCY_RESULT",
				Message: fmt.Sprintf("no result found for dependency: %s", depID),
				Details: map[string]any{
					"dependency_id": depID,
				},
			}
		}

		aggregatedResults[depID] = depResult

		// Collect output
		if depResult.Output != nil {
			aggregatedOutput[depID] = depResult.Output
		}

		// Collect findings
		if depResult.Findings != nil {
			allFindings = append(allFindings, depResult.Findings...)
		}

		// Track failed dependencies
		if depResult.Status == NodeStatusFailed {
			failedDeps = append(failedDeps, depID)
		}
	}

	// Determine overall status
	status := NodeStatusCompleted
	var nodeError *NodeError
	if len(failedDeps) > 0 {
		status = NodeStatusFailed
		nodeError = &NodeError{
			Code:    "JOIN_DEPENDENCY_FAILED",
			Message: fmt.Sprintf("%d dependencies failed", len(failedDeps)),
			Details: map[string]any{
				"failed_dependencies": failedDeps,
			},
		}
	}

	return &NodeResult{
		NodeID:   node.ID,
		Status:   status,
		Output:   aggregatedOutput,
		Error:    nodeError,
		Findings: allFindings,
		Metadata: map[string]any{
			"total_dependencies":  len(node.Dependencies),
			"failed_dependencies": len(failedDeps),
			"aggregated_results":  aggregatedResults,
		},
		Duration:    time.Since(startTime),
		StartedAt:   startTime,
		CompletedAt: time.Now(),
	}, nil
}

// executeWithRetry implements retry logic for node execution using the node's retry policy.
// It calculates backoff delays between retry attempts and tracks the retry count.
func (we *WorkflowExecutor) executeWithRetry(
	ctx context.Context,
	node *WorkflowNode,
	fn func() (*NodeResult, error),
) (*NodeResult, error) {
	var lastErr error
	var result *NodeResult

	for attempt := 0; attempt <= node.RetryPolicy.MaxRetries; attempt++ {
		// Execute the function
		result, lastErr = fn()

		// If successful, return immediately
		if lastErr == nil && result != nil && result.Status == NodeStatusCompleted {
			if result != nil {
				result.RetryCount = attempt
			}
			return result, nil
		}

		// Don't retry if this was the last attempt
		if attempt == node.RetryPolicy.MaxRetries {
			break
		}

		// Calculate delay for next retry
		delay := node.RetryPolicy.CalculateDelay(attempt)

		// Log retry attempt
		we.logger.Info("retrying node execution",
			"node_id", node.ID,
			"attempt", attempt+1,
			"max_retries", node.RetryPolicy.MaxRetries,
			"delay", delay,
			"error", lastErr,
		)

		// Wait before retrying (respecting context cancellation)
		select {
		case <-ctx.Done():
			return nil, &WorkflowError{
				Code:    WorkflowErrorWorkflowCancelled,
				Message: "workflow cancelled during retry delay",
				NodeID:  node.ID,
				Cause:   ctx.Err(),
			}
		case <-time.After(delay):
			// Continue to next retry attempt
		}
	}

	// All retries exhausted
	if result != nil {
		result.RetryCount = node.RetryPolicy.MaxRetries
	}

	return result, &NodeError{
		Code:    "MAX_RETRIES_EXCEEDED",
		Message: fmt.Sprintf("node execution failed after %d retries", node.RetryPolicy.MaxRetries),
		Details: map[string]any{
			"max_retries": node.RetryPolicy.MaxRetries,
		},
		Cause: lastErr,
	}
}
