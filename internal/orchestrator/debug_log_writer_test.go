package orchestrator

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/types"
)

func TestDebugLogWriter_LogObservation(t *testing.T) {
	buf := &bytes.Buffer{}
	w := NewDebugLogWriterWithOutput(buf)

	state := &ObservationState{
		MissionInfo: MissionInfo{
			ID:        "mission-123",
			Name:      "Test Mission",
			Objective: "Test the debug log writer",
			Status:    "running",
		},
		GraphSummary: GraphSummary{
			TotalNodes:      5,
			CompletedNodes:  2,
			FailedNodes:     0,
			PendingNodes:    3,
			TotalDecisions:  1,
			TotalExecutions: 2,
		},
		ReadyNodes: []NodeSummary{
			{ID: "node-1", AgentName: "recon", Status: "ready"},
		},
		RunningNodes:   []NodeSummary{},
		CompletedNodes: []CompletedNodeSummary{},
		FailedNodes:    []NodeSummary{},
		ResourceConstraints: ResourceConstraints{
			MaxConcurrent:   10,
			CurrentRunning:  0,
			TotalIterations: 1,
			TimeElapsed:     5 * time.Second,
		},
		ObservedAt: time.Now(),
	}

	w.LogObservation(1, "mission-123", state)

	output := buf.String()
	assert.Contains(t, output, "ITERATION 1")
	assert.Contains(t, output, "mission-123")
	assert.Contains(t, output, "=== OBSERVE ===")
	assert.Contains(t, output, "Test Mission")
	assert.Contains(t, output, "Total nodes: 5")
	assert.Contains(t, output, "Ready nodes:")
	assert.Contains(t, output, "node-1")
}

func TestDebugLogWriter_LogDecision(t *testing.T) {
	buf := &bytes.Buffer{}
	w := NewDebugLogWriterWithOutput(buf)

	decision := &Decision{
		Reasoning:    "Node-1 is ready and has no dependencies",
		Action:       ActionExecuteAgent,
		TargetNodeID: "node-1",
		Confidence:   0.95,
	}

	result := &ThinkResult{
		Decision:         decision,
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		Latency:          500 * time.Millisecond,
		RawResponse:      `{"reasoning": "test", "action": "execute_agent"}`,
		Model:            "claude-sonnet-4-20250514",
	}

	err := w.LogDecision(context.Background(), decision, result, 1, "mission-123")
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "=== THINK ===")
	assert.Contains(t, output, "claude-sonnet-4-20250514")
	assert.Contains(t, output, "100 prompt + 50 completion = 150 total")
	assert.Contains(t, output, "--- RAW RESPONSE ---")
	assert.Contains(t, output, "=== DECIDE ===")
	assert.Contains(t, output, "execute_agent")
	assert.Contains(t, output, "node-1")
	assert.Contains(t, output, "0.95")
}

func TestDebugLogWriter_LogAction(t *testing.T) {
	buf := &bytes.Buffer{}
	w := NewDebugLogWriterWithOutput(buf)

	now := time.Now()
	action := &ActionResult{
		Action:       ActionExecuteAgent,
		TargetNodeID: "node-1",
		IsTerminal:   false,
		AgentExecution: &schema.AgentExecution{
			ID:             types.NewID(),
			WorkflowNodeID: "node-1",
			Status:         schema.ExecutionStatusCompleted,
			StartedAt:      now,
			Attempt:        1,
		},
	}

	err := w.LogAction(context.Background(), action, 1, "mission-123")
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "=== ACT ===")
	assert.Contains(t, output, "execute_agent")
	assert.Contains(t, output, "node-1")
	assert.Contains(t, output, "SUCCESS")
	assert.Contains(t, output, "Agent Execution:")
	assert.Contains(t, output, "Workflow Node: node-1")
}

func TestDebugLogWriter_LogActionWithError(t *testing.T) {
	buf := &bytes.Buffer{}
	w := NewDebugLogWriterWithOutput(buf)

	action := &ActionResult{
		Action:       ActionExecuteAgent,
		TargetNodeID: "node-1",
		IsTerminal:   false,
		Error:        assert.AnError,
	}

	err := w.LogAction(context.Background(), action, 1, "mission-123")
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "=== ACT ===")
	assert.Contains(t, output, "FAILED")
	assert.Contains(t, output, "assert.AnError")
}

func TestDebugLogWriter_NilSafety(t *testing.T) {
	buf := &bytes.Buffer{}
	w := NewDebugLogWriterWithOutput(buf)

	// Should not panic on nil inputs
	w.LogObservation(1, "mission-123", nil)
	assert.Empty(t, buf.String())

	err := w.LogDecision(context.Background(), nil, nil, 1, "mission-123")
	assert.NoError(t, err)

	err = w.LogAction(context.Background(), nil, 1, "mission-123")
	assert.NoError(t, err)
}
