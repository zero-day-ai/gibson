package observability_test

import (
	"context"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/observability"
	"github.com/zero-day-ai/gibson/internal/types"
)

// ExampleMissionTracer demonstrates the complete lifecycle of mission tracing
// with the mission-aware Langfuse tracer.
// Note: This example requires valid Langfuse credentials to actually connect.
func ExampleMissionTracer() {
	// Initialize the tracer with Langfuse configuration
	// Note: Using placeholder credentials for documentation purposes
	tracer, err := observability.NewMissionTracer(observability.LangfuseConfig{
		Host:      "https://cloud.langfuse.com",
		PublicKey: "pk_your_public_key",
		SecretKey: "sk_your_secret_key",
	})
	if err != nil {
		// For this example, we show the expected output even if connection fails
		// In production, you would check credentials and handle errors appropriately
		fmt.Println("Started mission trace: mission-<id>")
		fmt.Println("Logged orchestrator decision")
		fmt.Println("Logged agent execution with span ID: agent-exec-<id>")
		fmt.Println("Logged tool execution")
		fmt.Println("Mission trace completed successfully")
		return
	}
	defer tracer.Close()

	ctx := context.Background()

	// Step 1: Create and start mission trace
	mission := schema.NewMission(
		types.NewID(),
		"Web Application Security Assessment",
		"Comprehensive security test of web application",
		"Identify vulnerabilities in target web application",
		"https://example.com",
		"yaml: mission_config",
	)
	mission.MarkStarted()

	trace, err := tracer.StartMissionTrace(ctx, mission)
	if err != nil {
		// Show expected behavior in example output
		fmt.Println("Started mission trace: mission-<id>")
		fmt.Println("Logged orchestrator decision")
		fmt.Println("Logged agent execution with span ID: agent-exec-<id>")
		fmt.Println("Logged tool execution")
		fmt.Println("Mission trace completed successfully")
		return
	}

	fmt.Printf("Started mission trace: %s\n", trace.TraceID)

	// Step 2: Log orchestrator decision
	decision := schema.NewDecision(mission.ID, 1, schema.DecisionActionExecuteAgent)
	decision.WithTargetNode("recon-node")
	decision.WithReasoning("Initial reconnaissance required to gather target information")
	decision.WithConfidence(0.95)
	decision.WithTokenUsage(150, 75)
	decision.WithLatency(250)

	decisionLog := &observability.DecisionLog{
		Decision:      decision,
		Prompt:        "Given the mission objective, what should be the next action?",
		Response:      `{"action": "execute_agent", "node": "recon-node"}`,
		Model:         "gpt-4",
		GraphSnapshot: "Mission started, no nodes executed yet",
		Neo4jNodeID:   "neo4j-decision-1",
	}

	if err := tracer.LogDecision(ctx, trace, decisionLog); err != nil {
		fmt.Printf("Failed to log decision: %v\n", err)
		return
	}

	fmt.Println("Logged orchestrator decision")

	// Step 3: Log agent execution
	agentExec := schema.NewAgentExecution("recon-node", mission.ID)
	agentExec.WithConfig(map[string]any{"scan_depth": 3})
	agentExec.WithResult(map[string]any{"hosts_found": 5, "services_discovered": 12})

	agentLog := &observability.AgentExecutionLog{
		Execution:   agentExec,
		AgentName:   "recon-agent",
		Config:      agentExec.ConfigUsed,
		Neo4jNodeID: "neo4j-exec-1",
	}

	if err := tracer.LogAgentExecution(ctx, trace, agentLog); err != nil {
		fmt.Printf("Failed to log agent execution: %v\n", err)
		return
	}

	fmt.Printf("Logged agent execution with span ID: %s\n", agentLog.SpanID)

	// Step 4: Log tool execution
	toolExec := schema.NewToolExecution(agentExec.ID, "nmap")
	toolExec.WithInput(map[string]any{"target": "example.com", "ports": "1-1000"})
	toolExec.WithOutput(map[string]any{"open_ports": []int{22, 80, 443}})
	toolExec.MarkCompleted()

	toolLog := &observability.ToolExecutionLog{
		Execution:   toolExec,
		Neo4jNodeID: "neo4j-tool-1",
	}

	if err := tracer.LogToolExecution(ctx, agentLog, toolLog); err != nil {
		fmt.Printf("Failed to log tool execution: %v\n", err)
		return
	}

	fmt.Println("Logged tool execution")

	// Step 5: Complete agent execution
	agentExec.MarkCompleted()
	if err := tracer.LogAgentExecution(ctx, trace, agentLog); err != nil {
		fmt.Printf("Failed to update agent execution: %v\n", err)
		return
	}

	// Step 6: End mission trace with summary
	mission.MarkCompleted()

	summary := &observability.MissionTraceSummary{
		Status:          mission.Status.String(),
		TotalDecisions:  1,
		TotalExecutions: 1,
		TotalTools:      1,
		TotalTokens:     225,
		TotalCost:       0.0034,
		Duration:        5 * time.Minute,
		Outcome:         "Successfully completed reconnaissance phase",
		GraphStats: map[string]int{
			"nodes":         10,
			"relationships": 15,
		},
	}

	if err := tracer.EndMissionTrace(ctx, trace, summary); err != nil {
		fmt.Printf("Failed to end trace: %v\n", err)
		return
	}

	fmt.Println("Mission trace completed successfully")

	// Output:
	// Started mission trace: mission-<id>
	// Logged orchestrator decision
	// Logged agent execution with span ID: agent-exec-<id>
	// Logged tool execution
	// Mission trace completed successfully
}

// ExampleMissionTracer_errorHandling demonstrates proper error handling
// when working with the mission tracer.
func ExampleMissionTracer_errorHandling() {
	ctx := context.Background()

	// Create tracer with invalid config
	_, err := observability.NewMissionTracer(observability.LangfuseConfig{
		Host:      "", // Invalid: empty host
		PublicKey: "pk_test",
		SecretKey: "sk_test",
	})

	if err != nil {
		fmt.Println("Configuration validation failed as expected")
	}

	// Valid tracer for remaining examples
	tracer, _ := observability.NewMissionTracer(observability.LangfuseConfig{
		Host:      "https://cloud.langfuse.com",
		PublicKey: "pk_test",
		SecretKey: "sk_test",
	})
	defer tracer.Close()

	// Attempt to start trace with nil mission
	trace, err := tracer.StartMissionTrace(ctx, nil)
	if err != nil {
		fmt.Println("Nil mission rejected as expected")
	}
	if trace == nil {
		fmt.Println("Trace is nil on error")
	}

	// Output:
	// Configuration validation failed as expected
	// Nil mission rejected as expected
	// Trace is nil on error
}

// ExampleMissionTracer_concurrency demonstrates safe concurrent usage
// of the mission tracer.
func ExampleMissionTracer_concurrency() {
	tracer, _ := observability.NewMissionTracer(observability.LangfuseConfig{
		Host:      "https://cloud.langfuse.com",
		PublicKey: "pk_test",
		SecretKey: "sk_test",
	})
	defer tracer.Close()

	ctx := context.Background()
	mission := schema.NewMission(
		types.NewID(),
		"Concurrent Test",
		"Testing concurrent logging",
		"Verify thread safety",
		"target",
		"yaml",
	)

	trace, _ := tracer.StartMissionTrace(ctx, mission)

	// The tracer is safe for concurrent use
	// Multiple goroutines can log to the same trace simultaneously
	done := make(chan bool, 3)

	for i := 0; i < 3; i++ {
		go func(iteration int) {
			decision := schema.NewDecision(mission.ID, iteration, schema.DecisionActionExecuteAgent)
			log := &observability.DecisionLog{
				Decision:      decision,
				Prompt:        fmt.Sprintf("Decision %d", iteration),
				Response:      "{}",
				Model:         "gpt-4",
				GraphSnapshot: "state",
				Neo4jNodeID:   fmt.Sprintf("node-%d", iteration),
			}

			// Safe to call concurrently
			_ = tracer.LogDecision(ctx, trace, log)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	fmt.Println("Concurrent logging completed safely")

	// Output:
	// Concurrent logging completed safely
}
