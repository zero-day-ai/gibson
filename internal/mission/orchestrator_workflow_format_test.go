package mission

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// TestOrchestrator_JSONWorkflowFormat tests that the orchestrator can handle JSON workflow format
func TestOrchestrator_JSONWorkflowFormat(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)

	// Create real workflow executor but no harness factory
	executor := workflow.NewWorkflowExecutor()
	orchestrator, err := NewMissionOrchestrator(
		store,
		WithWorkflowExecutor(executor),
	)
	require.NoError(t, err)

	// Create a workflow and serialize it to JSON
	wf := &workflow.Workflow{
		ID:          types.NewID(),
		Name:        "test-workflow",
		Description: "Test workflow in JSON format",
		Nodes: map[string]*workflow.WorkflowNode{
			"node1": {
				ID:        "node1",
				Type:      workflow.NodeTypeAgent,
				AgentName: "test-agent",
			},
		},
		Edges:       []workflow.WorkflowEdge{},
		EntryPoints: []string{"node1"},
		ExitPoints:  []string{"node1"},
		CreatedAt:   time.Now(),
	}

	workflowJSON, err := json.Marshal(wf)
	require.NoError(t, err, "Failed to marshal workflow to JSON")

	// Create a mission with JSON workflow
	mission := &Mission{
		ID:           types.NewID(),
		Name:         "test-mission-json",
		Description:  "Test mission with JSON workflow",
		TargetID:     types.NewID(),
		WorkflowID:   wf.ID,
		Status:       MissionStatusPending,
		WorkflowJSON: string(workflowJSON),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err = store.Save(context.Background(), mission)
	require.NoError(t, err, "Failed to save mission")

	// Execute the mission (this should parse the JSON workflow)
	ctx := context.Background()
	_, err = orchestrator.Execute(ctx, mission)

	// We expect an error because harness factory is not configured,
	// but the workflow should be parsed successfully (no parse error)
	require.Error(t, err, "Expected error due to missing harness factory")

	// Check that the error is NOT a workflow parse error
	require.Equal(t, "harness factory not configured", mission.Error,
		"Expected harness factory error, not parse error")
}

// TestOrchestrator_YAMLWorkflowFormat tests that the orchestrator can still handle YAML workflow format
func TestOrchestrator_YAMLWorkflowFormat(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)

	// Create real workflow executor but no harness factory
	executor := workflow.NewWorkflowExecutor()
	orchestrator, err := NewMissionOrchestrator(
		store,
		WithWorkflowExecutor(executor),
	)
	require.NoError(t, err)

	// Create a YAML workflow (using the same format as createWorkflowJSON helper)
	yamlWorkflow := `
name: test-workflow
description: Test workflow for mission orchestrator
nodes:
  - id: node1
    type: agent
    name: Test Agent Node
    agent: test-agent
    task:
      action: test
      target: localhost
`

	// Create a mission with YAML workflow
	mission := &Mission{
		ID:           types.NewID(),
		Name:         "test-mission-yaml",
		Description:  "Test mission with YAML workflow",
		TargetID:     types.NewID(),
		WorkflowID:   types.NewID(),
		Status:       MissionStatusPending,
		WorkflowJSON: yamlWorkflow,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err = store.Save(context.Background(), mission)
	require.NoError(t, err, "Failed to save mission")

	// Execute the mission (this should parse the YAML workflow)
	ctx := context.Background()
	_, err = orchestrator.Execute(ctx, mission)

	// We expect an error because harness factory is not configured,
	// but the workflow should be parsed successfully (no parse error)
	require.Error(t, err, "Expected error due to missing harness factory")

	// Check that the error is NOT a workflow parse error
	require.Equal(t, "harness factory not configured", mission.Error,
		"Expected harness factory error, not parse error")
}

// TestOrchestrator_InvalidJSONWorkflow tests error handling for invalid JSON
func TestOrchestrator_InvalidJSONWorkflow(t *testing.T) {
	db := setupTestDB(t)
	store := NewDBMissionStore(db)

	// Create real workflow executor but no harness factory
	executor := workflow.NewWorkflowExecutor()
	orchestrator, err := NewMissionOrchestrator(
		store,
		WithWorkflowExecutor(executor),
	)
	require.NoError(t, err)

	// Create a mission with invalid JSON workflow
	mission := &Mission{
		ID:           types.NewID(),
		Name:         "test-mission-invalid-json",
		Description:  "Test mission with invalid JSON workflow",
		TargetID:     types.NewID(),
		WorkflowID:   types.NewID(),
		Status:       MissionStatusPending,
		WorkflowJSON: `{"name": "invalid", "nodes": [}`, // Invalid JSON
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err = store.Save(context.Background(), mission)
	require.NoError(t, err, "Failed to save mission")

	// Execute the mission
	ctx := context.Background()
	_, err = orchestrator.Execute(ctx, mission)

	// We expect a workflow parse error
	require.Error(t, err, "Expected error for invalid JSON workflow")

	// Check that the error message indicates a parse failure
	require.NotEmpty(t, mission.Error, "Expected mission.Error to be set")
	require.Contains(t, mission.Error, "failed to parse workflow",
		"Expected parse error message")

	require.Equal(t, MissionStatusFailed, mission.Status,
		"Expected mission status to be Failed")
}
