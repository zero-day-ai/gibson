package mission

import (
	"context"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// Mock workflow types for testing (workflow package was removed)
// These are shared across all test files in the mission package.

type mockWorkflowNodeType string
type mockWorkflowStatus string
type mockNodeStatus string

const (
	mockNodeTypeAgent         mockWorkflowNodeType = "agent"
	mockNodeTypeTool          mockWorkflowNodeType = "tool"
	mockWorkflowStatusRunning mockWorkflowStatus   = "running"
	mockNodeStatusCompleted   mockNodeStatus       = "completed"
	mockNodeStatusPending     mockNodeStatus       = "pending"
)

type mockWorkflowNode struct {
	ID        string
	Name      string
	Type      mockWorkflowNodeType
	AgentName string
}

type mockWorkflowEdge struct {
	From string
	To   string
}

type mockWorkflow struct {
	ID          types.ID
	Name        string
	Description string
	Nodes       map[string]*mockWorkflowNode
	Edges       []mockWorkflowEdge
	EntryPoints []string
	ExitPoints  []string
	Metadata    map[string]any
	CreatedAt   time.Time
}

type mockNodeResult struct {
	NodeID      string
	Status      mockNodeStatus
	Output      map[string]any
	CompletedAt time.Time
}

type mockNodeState struct {
	Status mockNodeStatus
	Result *mockNodeResult
}

type mockWorkflowState struct {
	WorkflowID types.ID
	Status     mockWorkflowStatus
	StartedAt  time.Time
	NodeStates map[string]*mockNodeState
}

func newMockWorkflowState(wf *mockWorkflow) *mockWorkflowState {
	states := make(map[string]*mockNodeState)
	for id := range wf.Nodes {
		states[id] = &mockNodeState{
			Status: mockNodeStatusPending,
		}
	}
	return &mockWorkflowState{
		WorkflowID: wf.ID,
		Status:     mockWorkflowStatusRunning,
		StartedAt:  time.Now(),
		NodeStates: states,
	}
}

func (s *mockWorkflowState) MarkNodeStarted(nodeID string) {
	// No-op for tests
}

func (s *mockWorkflowState) MarkNodeCompleted(nodeID string, result *mockNodeResult) {
	if state, ok := s.NodeStates[nodeID]; ok {
		state.Status = mockNodeStatusCompleted
		state.Result = result
	}
}

// mockMissionOrchestrator is a simple mock orchestrator for testing
type mockMissionOrchestrator struct {
	executeResult *MissionResult
	executeError  error
}

func (m *mockMissionOrchestrator) Execute(ctx context.Context, mission *Mission) (*MissionResult, error) {
	if m.executeError != nil {
		return nil, m.executeError
	}
	if m.executeResult != nil {
		return m.executeResult, nil
	}
	return &MissionResult{
		MissionID: mission.ID,
		Status:    MissionStatusCompleted,
	}, nil
}
