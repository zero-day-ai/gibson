// Package workflow provides a DAG-based workflow engine for mission orchestration.
//
// The workflow engine enables the Gibson framework to coordinate complex, multi-step
// missions through directed acyclic graphs (DAGs) of tasks. It supports parallel
// execution, conditional branching, retry policies, and seamless integration with
// the agent harness, guardrails, and observability infrastructure.
//
// # Core Architecture
//
// The workflow engine is built around several key components:
//
//   - Workflow: A DAG definition containing nodes, edges, and metadata
//   - WorkflowNode: Individual tasks (agent, tool, plugin, condition, parallel, join)
//   - WorkflowBuilder: Fluent API for programmatically constructing workflows
//   - WorkflowExecutor: Orchestrates parallel execution with dependency resolution
//   - WorkflowState: Thread-safe tracking of execution state and results
//   - ConditionEvaluator: Evaluates conditional expressions for dynamic branching
//
// # Workflow Definition
//
// A Workflow is a directed acyclic graph (DAG) that defines the execution plan
// for a mission. It contains:
//
//   - Nodes: Individual tasks to execute (agents, tools, plugins, conditions)
//   - Edges: Dependencies between nodes that define execution order
//   - Entry Points: Nodes with no incoming edges (where execution begins)
//   - Exit Points: Nodes with no outgoing edges (where execution ends)
//
// Workflows ensure that:
//   - No circular dependencies exist (validated during build)
//   - Dependencies are satisfied before node execution
//   - Parallelism is exploited when nodes have no dependencies
//
// # Node Types
//
// The workflow engine supports six types of nodes:
//
// ## Agent Node (NodeTypeAgent)
//
// Executes an agent task using the AgentHarness. Agent nodes delegate work to
// specialized agents (e.g., reconnaissance, exploitation, analysis).
//
//	node := &WorkflowNode{
//	    ID:        "recon",
//	    Type:      NodeTypeAgent,
//	    AgentName: "ReconAgent",
//	    AgentTask: &agent.Task{
//	        Objective: "Scan target network",
//	        Constraints: map[string]any{
//	            "max_ports": 1024,
//	        },
//	    },
//	}
//
// ## Tool Node (NodeTypeTool)
//
// Executes a registered tool (e.g., nmap, sqlmap, burp).
//
//	node := &WorkflowNode{
//	    ID:       "scan",
//	    Type:     NodeTypeTool,
//	    ToolName: "nmap",
//	    ToolInput: map[string]any{
//	        "target": "192.168.1.0/24",
//	        "ports":  "1-1024",
//	    },
//	}
//
// ## Plugin Node (NodeTypePlugin)
//
// Calls a plugin method (e.g., database query, external API).
//
//	node := &WorkflowNode{
//	    ID:           "check_vuln",
//	    Type:         NodeTypePlugin,
//	    PluginName:   "VulnDB",
//	    PluginMethod: "QueryCVE",
//	    PluginParams: map[string]any{
//	        "cve_id": "CVE-2024-1234",
//	    },
//	}
//
// ## Condition Node (NodeTypeCondition)
//
// Evaluates a conditional expression to determine execution path.
//
//	node := &WorkflowNode{
//	    ID:   "check_results",
//	    Type: NodeTypeCondition,
//	    Condition: &NodeCondition{
//	        Expression:  "nodes.recon.status == 'completed' && len(nodes.recon.output.endpoints) > 0",
//	        TrueBranch:  []string{"exploit"},
//	        FalseBranch: []string{"report_no_targets"},
//	    },
//	}
//
// ## Parallel Node (NodeTypeParallel)
//
// Executes multiple sub-nodes concurrently.
//
//	node := &WorkflowNode{
//	    ID:   "parallel_scan",
//	    Type: NodeTypeParallel,
//	    SubNodes: []*WorkflowNode{
//	        scanWebNode,
//	        scanDatabaseNode,
//	        scanAPINode,
//	    },
//	}
//
// ## Join Node (NodeTypeJoin)
//
// Waits for multiple dependencies to complete before proceeding.
//
//	node := &WorkflowNode{
//	    ID:           "aggregate",
//	    Type:         NodeTypeJoin,
//	    Dependencies: []string{"scan1", "scan2", "scan3"},
//	}
//
// # Building Workflows
//
// ## Programmatic Construction
//
// Use WorkflowBuilder's fluent API to construct workflows programmatically:
//
//	workflow, err := NewWorkflow("penetration_test").
//	    WithDescription("Automated penetration test workflow").
//	    AddAgentNode("recon", "ReconAgent", &agent.Task{
//	        Objective: "Identify target endpoints",
//	    }).
//	    AddToolNode("scan", "nmap", map[string]any{
//	        "target": "{{recon.output.target}}",
//	        "ports":  "1-65535",
//	    }).
//	    AddConditionNode("check_vulns", &NodeCondition{
//	        Expression:  "len(nodes.scan.output.open_ports) > 0",
//	        TrueBranch:  []string{"exploit"},
//	        FalseBranch: []string{"report_safe"},
//	    }).
//	    AddAgentNode("exploit", "ExploitAgent", &agent.Task{
//	        Objective: "Exploit discovered vulnerabilities",
//	    }).
//	    AddEdge("recon", "scan").
//	    AddEdge("scan", "check_vulns").
//	    AddConditionalEdge("check_vulns", "exploit", "true").
//	    Build()
//
//	if err != nil {
//	    log.Fatal("Failed to build workflow:", err)
//	}
//
// ## Validation
//
// The Build() method automatically validates the workflow:
//   - Ensures at least one node exists
//   - Verifies all edges reference existing nodes
//   - Validates dependency references
//   - Detects cycles using depth-first search
//   - Computes entry and exit points
//
// If validation fails, Build() returns an error containing all validation issues.
//
// # Executing Workflows
//
// ## Basic Execution
//
// Create a WorkflowExecutor and execute the workflow:
//
//	executor := NewWorkflowExecutor(
//	    WithLogger(logger),
//	    WithTracer(tracer),
//	    WithMaxParallel(5),
//	    WithGuardrails(guardrailPipeline),
//	)
//
//	result, err := executor.Execute(ctx, workflow, harness)
//	if err != nil {
//	    log.Fatal("Workflow execution failed:", err)
//	}
//
//	fmt.Printf("Workflow completed with %d findings\n", len(result.Findings))
//	fmt.Printf("Executed: %d, Failed: %d, Skipped: %d\n",
//	    result.NodesExecuted, result.NodesFailed, result.NodesSkipped)
//
// ## Execution Model
//
// The executor uses a sophisticated parallel execution model:
//
//  1. Initialize all nodes to pending status
//  2. Main execution loop:
//     a. Identify ready nodes (pending with all dependencies completed)
//     b. If no ready nodes and workflow incomplete → deadlock error
//     c. Execute ready nodes in parallel (limited by maxParallel semaphore)
//     d. Update state with results
//     e. Check for context cancellation
//     f. Repeat until all nodes reach terminal status
//  3. Aggregate findings from all node results
//  4. Return comprehensive WorkflowResult
//
// ## Parallel Execution
//
// The executor exploits parallelism automatically:
//   - Nodes with no dependencies execute immediately in parallel
//   - Nodes with satisfied dependencies execute as soon as possible
//   - Semaphore limits concurrent execution (configurable via WithMaxParallel)
//   - Thread-safe state management ensures consistency
//
// Example execution timeline:
//
//	Time 0: [recon1, recon2, recon3] start in parallel
//	Time 5: [recon1, recon2] complete → [scan1, scan2] start
//	Time 8: [recon3] completes → [scan3] starts
//	Time 10: [scan1, scan2, scan3] complete → [exploit] starts
//	Time 15: [exploit] completes → workflow done
//
// ## Retry Policies
//
// Nodes can specify retry behavior for transient failures:
//
//	node := &WorkflowNode{
//	    ID:   "unreliable_scan",
//	    Type: NodeTypeTool,
//	    RetryPolicy: &RetryPolicy{
//	        MaxRetries:      3,
//	        BackoffStrategy: BackoffExponential,
//	        InitialDelay:    1 * time.Second,
//	        MaxDelay:        30 * time.Second,
//	        Multiplier:      2.0,
//	    },
//	}
//
// Backoff strategies:
//   - BackoffConstant: Fixed delay between retries
//   - BackoffLinear: Linearly increasing delay
//   - BackoffExponential: Exponentially increasing delay with max cap
//
// # Conditional Expressions
//
// ## Expression Syntax
//
// The ConditionEvaluator supports a rich expression language for dynamic branching:
//
//	// Path resolution
//	nodes.recon.status                              // Access node result status
//	nodes.recon.output.found_endpoint               // Access nested output fields
//	nodes.scan.output.vulnerabilities[0].severity   // Array indexing
//
//	// Comparison operators
//	nodes.recon.status == "completed"
//	nodes.scan.output.port_count > 10
//	nodes.exploit.duration <= 300
//
//	// Boolean operators
//	nodes.recon.status == "completed" && nodes.scan.status == "completed"
//	nodes.exploit.status == "failed" || skip_exploit
//	!empty(nodes.scan.output.endpoints)
//
//	// Built-in functions
//	len(nodes.scan.output.endpoints) > 0
//	empty(nodes.recon.output.vulnerabilities)
//	exists(nodes.scan.output.critical_vuln)
//
//	// Grouping
//	(nodes.scan.status == "completed" && critical_found) || force_exploit
//
// ## Custom Functions
//
// Register custom functions for domain-specific logic:
//
//	evaluator := NewConditionEvaluator()
//
//	evaluator.RegisterFunction("has_critical", func(args []any) (any, error) {
//	    if len(args) != 1 {
//	        return nil, fmt.Errorf("has_critical requires 1 argument")
//	    }
//	    vulns, ok := args[0].([]any)
//	    if !ok {
//	        return false, nil
//	    }
//	    for _, v := range vulns {
//	        vuln, ok := v.(map[string]any)
//	        if ok && vuln["severity"] == "critical" {
//	            return true, nil
//	        }
//	    }
//	    return false, nil
//	})
//
//	// Use in expressions
//	expr := "has_critical(nodes.scan.output.vulnerabilities)"
//
// ## Evaluation Context
//
// Expressions are evaluated in an EvaluationContext:
//
//	context := &EvaluationContext{
//	    NodeResults: map[string]*NodeResult{
//	        "recon": {
//	            Status: NodeStatusCompleted,
//	            Output: map[string]any{
//	                "found_endpoint": true,
//	                "target":         "192.168.1.100",
//	            },
//	        },
//	    },
//	    Variables: map[string]any{
//	        "threshold":     5,
//	        "skip_exploit":  false,
//	        "force_exploit": true,
//	    },
//	}
//
//	result, err := evaluator.Evaluate("nodes.recon.output.found_endpoint && !skip_exploit", context)
//
// # Integration with AgentHarness
//
// The workflow executor integrates seamlessly with the agent harness:
//
// ## Agent Execution
//
// When executing agent nodes, the executor:
//
//  1. Retrieves the agent from the harness's agent registry
//
//  2. Creates an isolated execution context with timeout
//
//  3. Calls agent.Execute(ctx, task, harness)
//
//  4. Stores agent.Result in the workflow state
//
//  5. Extracts findings for aggregation
//
//     // In node_executor.go
//     func (e *WorkflowExecutor) executeAgentNode(ctx context.Context, node *WorkflowNode, harness harness.AgentHarness) (*NodeResult, error) {
//     agent := harness.GetAgent(node.AgentName)
//     result, err := agent.Execute(ctx, *node.AgentTask, harness)
//     return &NodeResult{
//     NodeID:   node.ID,
//     Status:   NodeStatusCompleted,
//     Output:   result.Output,
//     Findings: result.Findings,
//     }, err
//     }
//
// ## Tool Execution
//
// Tool nodes delegate to the harness:
//
//	result, err := harness.CallTool(ctx, node.ToolName, node.ToolInput)
//
// ## Plugin Execution
//
// Plugin nodes query registered plugins:
//
//	result, err := harness.QueryPlugin(ctx, node.PluginName, node.PluginMethod, node.PluginParams)
//
// ## Memory Access
//
// Nodes can access memory tiers through the harness:
//
//	// Store intermediate results
//	harness.Memory().Working().Set(ctx, "scan_results", scanData)
//
//	// Retrieve mission context
//	target, _ := harness.Memory().Mission().Get(ctx, "target")
//
// # Guardrails Integration
//
// Workflows can enforce guardrails on inputs and outputs:
//
//	guardrails := guardrail.NewPipeline(
//	    guardrail.NewPromptInjectionGuardrail(),
//	    guardrail.NewPIIGuardrail(),
//	    guardrail.NewToxicityGuardrail(),
//	)
//
//	executor := NewWorkflowExecutor(WithGuardrails(guardrails))
//
// The executor applies guardrails:
//   - Before executing agent/tool/plugin nodes (input validation)
//   - After node completion (output filtering)
//   - On conditional expression evaluation (prevent injection)
//
// If a guardrail violation occurs:
//   - The node is marked as failed
//   - A WorkflowError with code WorkflowErrorGuardrailViolation is stored
//   - The workflow may continue (depending on dependency configuration)
//
// # Observability
//
// The workflow engine provides comprehensive observability:
//
// ## Distributed Tracing
//
// OpenTelemetry spans are created for:
//
//   - Overall workflow execution (parent span)
//
//   - Individual node execution (child spans)
//
//   - Retry attempts (nested spans)
//
//   - Guardrail evaluation (nested spans)
//
//     executor := NewWorkflowExecutor(WithTracer(otel.Tracer("gibson/workflow")))
//
// Trace attributes include:
//   - workflow.id, workflow.name, workflow.node_count
//   - node.id, node.type, node.status
//   - node.retry_count, node.duration
//   - guardrail.violations
//
// ## Structured Logging
//
// The executor uses structured logging (slog) for operational visibility:
//
//	executor := NewWorkflowExecutor(WithLogger(slog.New(handler)))
//
// Log events:
//   - workflow.start: Workflow execution started
//   - node.executing: Node execution started
//   - node.completed: Node execution succeeded
//   - node.failed: Node execution failed
//   - node.skipped: Node was skipped (condition false)
//   - node.retry: Retry attempt for failed node
//   - workflow.deadlock: Deadlock detected in workflow
//   - workflow.completed: Workflow execution completed
//   - workflow.cancelled: Workflow execution cancelled
//
// ## Metrics
//
// Workflow execution produces metrics for monitoring:
//   - workflow_duration_seconds: Total workflow execution time
//   - workflow_nodes_executed: Number of successfully executed nodes
//   - workflow_nodes_failed: Number of failed nodes
//   - workflow_nodes_skipped: Number of skipped nodes
//   - node_execution_duration_seconds: Per-node execution time
//   - node_retry_count: Number of retry attempts per node
//
// # Error Handling
//
// The workflow engine uses typed errors for precise error handling:
//
//	type WorkflowError struct {
//	    Code    WorkflowErrorCode
//	    Message string
//	    Cause   error
//	}
//
// Error codes:
//   - WorkflowErrorDeadlock: No ready nodes but workflow incomplete
//   - WorkflowErrorNodeFailed: Node execution failed
//   - WorkflowErrorExpressionInvalid: Conditional expression syntax error
//   - WorkflowErrorTimeout: Node execution exceeded timeout
//   - WorkflowErrorGuardrailViolation: Guardrail check failed
//   - WorkflowErrorWorkflowCancelled: Workflow cancelled via context
//
// Example error handling:
//
//	result, err := executor.Execute(ctx, workflow, harness)
//	if err != nil {
//	    if wfErr, ok := err.(*WorkflowError); ok {
//	        switch wfErr.Code {
//	        case WorkflowErrorDeadlock:
//	            log.Error("Workflow deadlock - check dependencies")
//	        case WorkflowErrorNodeFailed:
//	            log.Error("Node failed:", wfErr.Message)
//	        case WorkflowErrorTimeout:
//	            log.Error("Workflow timed out")
//	        }
//	    }
//	    return err
//	}
//
// # Thread Safety
//
// All workflow components are safe for concurrent use:
//
//   - WorkflowState uses sync.RWMutex for state access
//   - WorkflowExecutor uses semaphores for parallel execution
//   - ConditionEvaluator is stateless (safe for concurrent evaluation)
//   - NodeResult is immutable after creation
//
// # Performance Considerations
//
// ## Parallelism Tuning
//
// Adjust maxParallel based on resource constraints:
//
//	// CPU-bound workloads
//	executor := NewWorkflowExecutor(WithMaxParallel(runtime.NumCPU()))
//
//	// I/O-bound workloads
//	executor := NewWorkflowExecutor(WithMaxParallel(50))
//
//	// Memory-constrained environments
//	executor := NewWorkflowExecutor(WithMaxParallel(2))
//
// ## Memory Management
//
// For long-running workflows with large outputs:
//   - Store large results in mission memory (not workflow state)
//   - Use references instead of copying data
//   - Clear intermediate results when no longer needed
//
// ## Deadlock Prevention
//
// Avoid deadlocks by ensuring:
//   - No circular dependencies (enforced by DAG validation)
//   - Failed nodes don't block critical paths (use conditional edges)
//   - Timeouts are configured for long-running nodes
//
// # Best Practices
//
//  1. Workflow Design
//     - Keep workflows focused on a single mission objective
//     - Use descriptive node IDs and names
//     - Add metadata for documentation and debugging
//     - Design for idempotency (retries don't cause side effects)
//
//  2. Error Handling
//     - Set appropriate retry policies for transient failures
//     - Use conditional edges for expected failure paths
//     - Log node failures with context for debugging
//     - Aggregate findings even on partial workflow failure
//
//  3. Performance
//     - Maximize parallelism by minimizing dependencies
//     - Use timeouts to prevent hanging nodes
//     - Monitor workflow duration and node execution times
//     - Profile memory usage for large workflows
//
//  4. Security
//     - Apply guardrails to all user-provided inputs
//     - Validate conditional expressions for injection attacks
//     - Limit workflow complexity to prevent resource exhaustion
//     - Audit workflow definitions for security implications
//
//  5. Observability
//     - Enable distributed tracing for production workflows
//     - Use structured logging with context
//     - Collect metrics for workflow health monitoring
//     - Create alerts for workflow failures and deadlocks
//
// # Complete Example
//
// Here's a complete example of a penetration testing workflow:
//
//	package main
//
//	import (
//	    "context"
//	    "log"
//	    "time"
//
//	    "github.com/zero-day-ai/gibson/internal/agent"
//	    "github.com/zero-day-ai/gibson/internal/workflow"
//	)
//
//	func main() {
//	    // Build workflow
//	    wf, err := workflow.NewWorkflow("automated_pentest").
//	        WithDescription("Automated penetration testing workflow").
//	        // Reconnaissance phase
//	        AddAgentNode("recon", "ReconAgent", &agent.Task{
//	            Objective: "Identify target endpoints and services",
//	            Constraints: map[string]any{"max_duration": 300},
//	        }).
//	        // Parallel scanning phase
//	        AddToolNode("port_scan", "nmap", map[string]any{
//	            "target": "{{recon.output.target}}",
//	            "ports":  "1-65535",
//	        }).
//	        AddToolNode("web_scan", "nikto", map[string]any{
//	            "url": "{{recon.output.web_url}}",
//	        }).
//	        AddToolNode("vuln_scan", "openvas", map[string]any{
//	            "target": "{{recon.output.target}}",
//	        }).
//	        // Join results
//	        AddAgentNode("analyze", "AnalysisAgent", &agent.Task{
//	            Objective: "Analyze scan results and prioritize vulnerabilities",
//	        }).
//	        // Conditional exploitation
//	        AddConditionNode("check_critical", &workflow.NodeCondition{
//	            Expression:  "len(nodes.analyze.output.critical_vulns) > 0",
//	            TrueBranch:  []string{"exploit"},
//	            FalseBranch: []string{"report"},
//	        }).
//	        AddAgentNode("exploit", "ExploitAgent", &agent.Task{
//	            Objective: "Exploit critical vulnerabilities safely",
//	            Constraints: map[string]any{"safe_mode": true},
//	        }).
//	        // Final report
//	        AddAgentNode("report", "ReportAgent", &agent.Task{
//	            Objective: "Generate comprehensive security report",
//	        }).
//	        // Define execution order
//	        AddEdge("recon", "port_scan").
//	        AddEdge("recon", "web_scan").
//	        AddEdge("recon", "vuln_scan").
//	        WithDependency("analyze", "port_scan", "web_scan", "vuln_scan").
//	        AddEdge("analyze", "check_critical").
//	        AddConditionalEdge("check_critical", "exploit", "true").
//	        AddConditionalEdge("check_critical", "report", "false").
//	        AddEdge("exploit", "report").
//	        Build()
//
//	    if err != nil {
//	        log.Fatal("Failed to build workflow:", err)
//	    }
//
//	    // Configure executor
//	    executor := workflow.NewWorkflowExecutor(
//	        workflow.WithMaxParallel(5),
//	        workflow.WithLogger(logger),
//	        workflow.WithTracer(tracer),
//	        workflow.WithGuardrails(guardrails),
//	    )
//
//	    // Execute workflow
//	    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
//	    defer cancel()
//
//	    result, err := executor.Execute(ctx, wf, harness)
//	    if err != nil {
//	        log.Fatal("Workflow execution failed:", err)
//	    }
//
//	    // Process results
//	    log.Printf("Workflow completed in %v", result.TotalDuration)
//	    log.Printf("Executed: %d, Failed: %d, Skipped: %d",
//	        result.NodesExecuted, result.NodesFailed, result.NodesSkipped)
//	    log.Printf("Findings: %d", len(result.Findings))
//
//	    // Extract findings by severity
//	    for _, finding := range result.Findings {
//	        if finding.Severity == agent.SeverityCritical {
//	            log.Printf("CRITICAL: %s - %s", finding.Title, finding.Description)
//	        }
//	    }
//	}
package workflow
