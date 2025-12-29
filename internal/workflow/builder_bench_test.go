package workflow

import (
	"fmt"
	"testing"

	"github.com/zero-day-ai/gibson/internal/agent"
)

// BenchmarkWorkflowBuilder_Simple benchmarks building a simple linear workflow
func BenchmarkWorkflowBuilder_Simple(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = NewWorkflow("bench").
			AddAgentNode("n1", "a1", nil).
			AddAgentNode("n2", "a2", nil).
			AddAgentNode("n3", "a3", nil).
			AddEdge("n1", "n2").
			AddEdge("n2", "n3").
			Build()
	}
}

// BenchmarkWorkflowBuilder_Complex benchmarks building a complex workflow
func BenchmarkWorkflowBuilder_Complex(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		wb := NewWorkflow("complex-bench").
			WithDescription("Complex benchmark workflow")

		// Add 10 nodes
		for j := 0; j < 10; j++ {
			wb.AddAgentNode(fmt.Sprintf("agent-%d", j), fmt.Sprintf("a%d", j), &agent.Task{
				Name: fmt.Sprintf("task-%d", j),
			})
		}

		// Create a DAG with multiple paths
		wb.AddEdge("agent-0", "agent-1").
			AddEdge("agent-0", "agent-2").
			AddEdge("agent-1", "agent-3").
			AddEdge("agent-2", "agent-3").
			AddEdge("agent-3", "agent-4").
			AddEdge("agent-4", "agent-5").
			AddEdge("agent-4", "agent-6").
			AddEdge("agent-5", "agent-7").
			AddEdge("agent-6", "agent-7").
			AddEdge("agent-7", "agent-8").
			AddEdge("agent-8", "agent-9")

		_, _ = wb.Build()
	}
}

// BenchmarkWorkflowBuilder_ParallelBranches benchmarks building a workflow with many parallel branches
func BenchmarkWorkflowBuilder_ParallelBranches(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		wb := NewWorkflow("parallel-bench").
			AddAgentNode("start", "starter", nil).
			AddAgentNode("end", "finisher", nil)

		// Create 20 parallel branches
		for j := 0; j < 20; j++ {
			nodeID := fmt.Sprintf("parallel-%d", j)
			wb.AddAgentNode(nodeID, fmt.Sprintf("worker-%d", j), nil).
				AddEdge("start", nodeID).
				AddEdge(nodeID, "end")
		}

		_, _ = wb.Build()
	}
}

// BenchmarkWorkflowBuilder_CycleDetection benchmarks cycle detection performance
func BenchmarkWorkflowBuilder_CycleDetection(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Build a workflow with a cycle (should fail validation)
		_, _ = NewWorkflow("cycle-bench").
			AddAgentNode("n1", "a1", nil).
			AddAgentNode("n2", "a2", nil).
			AddAgentNode("n3", "a3", nil).
			AddAgentNode("n4", "a4", nil).
			AddEdge("n1", "n2").
			AddEdge("n2", "n3").
			AddEdge("n3", "n4").
			AddEdge("n4", "n2"). // Creates cycle
			Build()
	}
}

// BenchmarkWorkflowBuilder_BuildOnly benchmarks just the Build() method
func BenchmarkWorkflowBuilder_BuildOnly(b *testing.B) {
	// Pre-build a workflow
	wb := NewWorkflow("bench").
		AddAgentNode("n1", "a1", nil).
		AddAgentNode("n2", "a2", nil).
		AddAgentNode("n3", "a3", nil).
		AddAgentNode("n4", "a4", nil).
		AddAgentNode("n5", "a5", nil).
		AddEdge("n1", "n2").
		AddEdge("n1", "n3").
		AddEdge("n2", "n4").
		AddEdge("n3", "n4").
		AddEdge("n4", "n5")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Reset workflow state
		wb.workflow.EntryPoints = []string{}
		wb.workflow.ExitPoints = []string{}
		wb.errors = []error{}

		_, _ = wb.Build()
	}
}

// BenchmarkWorkflowBuilder_AddNode benchmarks node addition
func BenchmarkWorkflowBuilder_AddNode(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		wb := NewWorkflow("bench")
		for j := 0; j < 100; j++ {
			wb.AddAgentNode(fmt.Sprintf("node-%d", j), fmt.Sprintf("agent-%d", j), nil)
		}
	}
}
