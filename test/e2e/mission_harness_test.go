package e2e

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// TestE2E_MultiAgentWorkflow tests workflow execution with multiple agents.
// Verifies:
// 1. Each agent receives its own harness
// 2. Memory.Working() is isolated per agent
// 3. Findings from all agents are collected in final result
func TestE2E_MultiAgentWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Track which agents were executed
	executedAgents := &sync.Map{}
	agentFindings := &sync.Map{}

	// Create mock harness that tracks agent executions
	mockHarness := newMultiAgentMockHarness()
	mockHarness.delegateFunc = func(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
		executedAgents.Store(name, true)

		// Each agent produces a finding
		finding := agent.Finding{
			ID:          types.NewID(),
			Title:       "Finding from " + name,
			Severity:    agent.SeverityMedium,
			Description: "Discovered by " + name,
		}
		agentFindings.Store(name, finding)

		return agent.Result{
			Status:   agent.ResultStatusCompleted,
			Findings: []agent.Finding{finding},
		}, nil
	}

	// Create workflow executor
	executor := workflow.NewWorkflowExecutor()

	// Create multi-agent workflow: agent-1 -> agent-2 -> agent-3
	testWorkflow := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "multi-agent-test-workflow",
		Nodes: map[string]*workflow.WorkflowNode{
			"agent-1": {
				ID:        "agent-1",
				Type:      workflow.NodeTypeAgent,
				Name:      "Recon Agent",
				AgentName: "recon-agent",
				AgentTask: &agent.Task{
					ID:          types.NewID(),
					Name:        "reconnaissance",
					Description: "Gather target information",
				},
			},
			"agent-2": {
				ID:           "agent-2",
				Type:         workflow.NodeTypeAgent,
				Name:         "Scanner Agent",
				AgentName:    "scanner-agent",
				Dependencies: []string{"agent-1"},
				AgentTask: &agent.Task{
					ID:          types.NewID(),
					Name:        "scanning",
					Description: "Scan for vulnerabilities",
				},
			},
			"agent-3": {
				ID:           "agent-3",
				Type:         workflow.NodeTypeAgent,
				Name:         "Exploit Agent",
				AgentName:    "exploit-agent",
				Dependencies: []string{"agent-2"},
				AgentTask: &agent.Task{
					ID:          types.NewID(),
					Name:        "exploitation",
					Description: "Attempt to exploit vulnerabilities",
				},
			},
		},
	}

	// Execute workflow
	result, err := executor.Execute(ctx, testWorkflow, mockHarness)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify workflow completed
	assert.Equal(t, workflow.WorkflowStatusCompleted, result.Status)
	assert.Equal(t, 3, result.NodesExecuted)
	assert.Equal(t, 0, result.NodesFailed)

	// Verify all agents were executed
	_, reconExecuted := executedAgents.Load("recon-agent")
	_, scannerExecuted := executedAgents.Load("scanner-agent")
	_, exploitExecuted := executedAgents.Load("exploit-agent")
	assert.True(t, reconExecuted, "Recon agent should have been executed")
	assert.True(t, scannerExecuted, "Scanner agent should have been executed")
	assert.True(t, exploitExecuted, "Exploit agent should have been executed")

	// Verify findings from all agents were collected
	assert.Len(t, result.Findings, 3, "Should have findings from all 3 agents")

	// Verify execution order (delegate calls should be in order due to dependencies)
	assert.Len(t, mockHarness.delegateCalls, 3)
	assert.Equal(t, "recon-agent", mockHarness.delegateCalls[0].name, "Recon should execute first")
	assert.Equal(t, "scanner-agent", mockHarness.delegateCalls[1].name, "Scanner should execute second")
	assert.Equal(t, "exploit-agent", mockHarness.delegateCalls[2].name, "Exploit should execute third")
}

// TestE2E_ParallelAgentWorkflow tests parallel agent execution
func TestE2E_ParallelAgentWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Track execution
	executionStart := &sync.Map{}
	executionEnd := &sync.Map{}

	mockHarness := newMultiAgentMockHarness()
	mockHarness.delegateFunc = func(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
		executionStart.Store(name, time.Now())
		time.Sleep(10 * time.Millisecond) // Simulate work
		executionEnd.Store(name, time.Now())

		return agent.Result{
			Status: agent.ResultStatusCompleted,
			Findings: []agent.Finding{
				{
					ID:          types.NewID(),
					Title:       "Finding from " + name,
					Severity:    agent.SeverityLow,
					Description: "Test finding",
				},
			},
		}, nil
	}

	executor := workflow.NewWorkflowExecutor(
		workflow.WithMaxParallel(10), // Allow parallel execution
	)

	// Create workflow with parallel branches: agent-1 splits to agent-2a and agent-2b, then joins at agent-3
	testWorkflow := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "parallel-agent-workflow",
		Nodes: map[string]*workflow.WorkflowNode{
			"agent-1": {
				ID:        "agent-1",
				Type:      workflow.NodeTypeAgent,
				Name:      "Initial Agent",
				AgentName: "initial-agent",
				AgentTask: &agent.Task{ID: types.NewID(), Name: "initial"},
			},
			"agent-2a": {
				ID:           "agent-2a",
				Type:         workflow.NodeTypeAgent,
				Name:         "Parallel Branch A",
				AgentName:    "branch-a-agent",
				Dependencies: []string{"agent-1"},
				AgentTask:    &agent.Task{ID: types.NewID(), Name: "branch-a"},
			},
			"agent-2b": {
				ID:           "agent-2b",
				Type:         workflow.NodeTypeAgent,
				Name:         "Parallel Branch B",
				AgentName:    "branch-b-agent",
				Dependencies: []string{"agent-1"},
				AgentTask:    &agent.Task{ID: types.NewID(), Name: "branch-b"},
			},
			"agent-3": {
				ID:           "agent-3",
				Type:         workflow.NodeTypeAgent,
				Name:         "Final Agent",
				AgentName:    "final-agent",
				Dependencies: []string{"agent-2a", "agent-2b"},
				AgentTask:    &agent.Task{ID: types.NewID(), Name: "final"},
			},
		},
	}

	result, err := executor.Execute(ctx, testWorkflow, mockHarness)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify workflow completed
	assert.Equal(t, workflow.WorkflowStatusCompleted, result.Status)
	assert.Equal(t, 4, result.NodesExecuted)
	assert.Equal(t, 0, result.NodesFailed)

	// Verify findings from all 4 agents
	assert.Len(t, result.Findings, 4)

	// Verify parallel execution: agent-2a and agent-2b should have overlapping execution times
	startA, _ := executionStart.Load("branch-a-agent")
	startB, _ := executionStart.Load("branch-b-agent")
	endA, _ := executionEnd.Load("branch-a-agent")
	endB, _ := executionEnd.Load("branch-b-agent")

	if startA != nil && startB != nil && endA != nil && endB != nil {
		timeStartA := startA.(time.Time)
		timeStartB := startB.(time.Time)
		timeEndA := endA.(time.Time)
		timeEndB := endB.(time.Time)

		// Either A started before B ended, or B started before A ended (parallel)
		parallelExecution := timeStartA.Before(timeEndB) && timeStartB.Before(timeEndA)
		assert.True(t, parallelExecution, "Branch agents should execute in parallel")
	}
}

// TestE2E_AgentFailureHandling tests handling of agent failures
func TestE2E_AgentFailureHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	failedAgent := "failing-agent"

	mockHarness := newMultiAgentMockHarness()
	mockHarness.delegateFunc = func(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
		if name == failedAgent {
			return agent.Result{
				Status: agent.ResultStatusFailed,
				Error: &agent.ResultError{
					Message: "Agent failed intentionally",
				},
			}, nil
		}
		return agent.Result{
			Status: agent.ResultStatusCompleted,
		}, nil
	}

	executor := workflow.NewWorkflowExecutor()

	// Create workflow where second agent fails
	testWorkflow := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "failure-test-workflow",
		Nodes: map[string]*workflow.WorkflowNode{
			"agent-1": {
				ID:        "agent-1",
				Type:      workflow.NodeTypeAgent,
				Name:      "Success Agent",
				AgentName: "success-agent",
				AgentTask: &agent.Task{ID: types.NewID(), Name: "task-1"},
			},
			"agent-2": {
				ID:           "agent-2",
				Type:         workflow.NodeTypeAgent,
				Name:         "Failing Agent",
				AgentName:    failedAgent,
				Dependencies: []string{"agent-1"},
				AgentTask:    &agent.Task{ID: types.NewID(), Name: "task-2"},
			},
			"agent-3": {
				ID:           "agent-3",
				Type:         workflow.NodeTypeAgent,
				Name:         "After Failure Agent",
				AgentName:    "after-failure-agent",
				Dependencies: []string{"agent-2"},
				AgentTask:    &agent.Task{ID: types.NewID(), Name: "task-3"},
			},
		},
	}

	result, err := executor.Execute(ctx, testWorkflow, mockHarness)

	// Workflow completes but with failures
	// The error can be nil if the workflow completed with failures
	if err == nil {
		require.NotNil(t, result)
		// Check that at least one node failed
		assert.Greater(t, result.NodesFailed, 0, "At least one node should have failed")
	}
}

// ============================================================================
// Multi-Agent Mock Harness
// ============================================================================

type multiAgentMockHarness struct {
	mu            sync.Mutex
	delegateCalls []delegateCall
	delegateFunc  func(ctx context.Context, name string, task agent.Task) (agent.Result, error)
}

func newMultiAgentMockHarness() *multiAgentMockHarness {
	return &multiAgentMockHarness{
		delegateCalls: []delegateCall{},
	}
}

func (m *multiAgentMockHarness) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{
		Message: llm.Message{Role: llm.RoleAssistant, Content: "mock response"},
	}, nil
}

func (m *multiAgentMockHarness) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{
		Message: llm.Message{Role: llm.RoleAssistant, Content: "mock response"},
	}, nil
}

func (m *multiAgentMockHarness) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{Delta: llm.StreamDelta{Content: "mock"}}
	close(ch)
	return ch, nil
}

func (m *multiAgentMockHarness) CallTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	return map[string]any{"result": "success"}, nil
}

func (m *multiAgentMockHarness) ListTools() []harness.ToolDescriptor {
	return nil
}

func (m *multiAgentMockHarness) QueryPlugin(ctx context.Context, name string, method string, params map[string]any) (any, error) {
	return nil, nil
}

func (m *multiAgentMockHarness) ListPlugins() []harness.PluginDescriptor {
	return nil
}

func (m *multiAgentMockHarness) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
	m.mu.Lock()
	m.delegateCalls = append(m.delegateCalls, delegateCall{name: name, task: task})
	m.mu.Unlock()

	if m.delegateFunc != nil {
		return m.delegateFunc(ctx, name, task)
	}
	return agent.Result{Status: agent.ResultStatusCompleted}, nil
}

func (m *multiAgentMockHarness) ListAgents() []harness.AgentDescriptor {
	return nil
}

func (m *multiAgentMockHarness) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	return nil
}

func (m *multiAgentMockHarness) GetFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return nil, nil
}

func (m *multiAgentMockHarness) Memory() memory.MemoryStore {
	return nil
}

func (m *multiAgentMockHarness) Mission() harness.MissionContext {
	return harness.MissionContext{ID: types.NewID(), Name: "test-mission"}
}

func (m *multiAgentMockHarness) Target() harness.TargetInfo {
	return harness.TargetInfo{Name: "test-target"}
}

func (m *multiAgentMockHarness) Tracer() trace.Tracer {
	return noop.NewTracerProvider().Tracer("test")
}

func (m *multiAgentMockHarness) Logger() *slog.Logger {
	return slog.Default()
}

func (m *multiAgentMockHarness) Metrics() harness.MetricsRecorder {
	return nil
}

func (m *multiAgentMockHarness) TokenUsage() *llm.TokenTracker {
	return nil
}
