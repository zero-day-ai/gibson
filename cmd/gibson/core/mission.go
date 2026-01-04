package core

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// MissionListResult represents the structured output from MissionList
type MissionListResult struct {
	Missions []*mission.Mission
	Count    int
}

// MissionList lists all missions with optional status filter.
// Returns structured data that can be formatted by CLI or TUI.
func MissionList(cc *CommandContext, statusFilter string) (*CommandResult, error) {
	// Validate mission store
	if cc.MissionStore == nil {
		return nil, fmt.Errorf("mission store not initialized")
	}

	// Parse status filter
	var filter *mission.MissionFilter
	if statusFilter != "" {
		status := mission.MissionStatus(statusFilter)
		// Validate status
		if !IsValidMissionStatus(status) {
			return &CommandResult{
				Error: fmt.Errorf("invalid status filter: must be pending, running, completed, failed, or cancelled"),
			}, nil
		}
		filter = mission.NewMissionFilter().WithStatus(status)
	} else {
		filter = mission.NewMissionFilter()
	}

	// List missions
	missions, err := cc.MissionStore.List(cc.Ctx, filter)
	if err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to list missions: %w", err),
		}, nil
	}

	return &CommandResult{
		Data: &MissionListResult{
			Missions: missions,
			Count:    len(missions),
		},
		Message: fmt.Sprintf("Found %d missions", len(missions)),
	}, nil
}

// MissionShow displays detailed information about a specific mission.
func MissionShow(cc *CommandContext, name string) (*CommandResult, error) {
	// Validate mission store
	if cc.MissionStore == nil {
		return nil, fmt.Errorf("mission store not initialized")
	}

	// Get mission
	m, err := cc.MissionStore.GetByName(cc.Ctx, name)
	if err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to get mission: %w", err),
		}, nil
	}

	return &CommandResult{
		Data:    m,
		Message: fmt.Sprintf("Mission '%s' details", m.Name),
	}, nil
}

// MissionRunResult represents the structured output from MissionRun
type MissionRunResult struct {
	Mission      *mission.Mission
	Workflow     *workflow.Workflow
	Status       string
	NodesCount   int
	EntryPoints  int
	ExitPoints   int
}

// MissionRun creates and runs a new mission from a workflow YAML file.
func MissionRun(cc *CommandContext, workflowFile string) (*CommandResult, error) {
	// Validate mission store
	if cc.MissionStore == nil {
		return nil, fmt.Errorf("mission store not initialized")
	}

	// Parse workflow file
	wf, err := workflow.ParseWorkflowFile(workflowFile)
	if err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to parse workflow file: %w", err),
		}, nil
	}

	// Serialize workflow to JSON
	workflowJSON, err := json.Marshal(wf)
	if err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to serialize workflow: %w", err),
		}, nil
	}

	// Create mission
	// Note: TargetID is required in the new Mission type, but we don't have it in this workflow-only context.
	// This is a limitation - missions should probably be created with explicit target specification.
	// For now, we'll use an empty string as a placeholder.
	// TODO: Update mission run command to require a target specification
	now := time.Now()
	m := &mission.Mission{
		ID:               types.NewID(),
		Name:             wf.Name,
		Description:      wf.Description,
		Status:           mission.MissionStatusPending,
		TargetID:         "", // FIXME: This should be specified by the user
		WorkflowID:       wf.ID,
		WorkflowJSON:     string(workflowJSON),
		Progress:         0.0,
		FindingsCount:    0,
		AgentAssignments: make(map[string]string),
		Metadata:         make(map[string]any),
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := cc.MissionStore.Save(cc.Ctx, m); err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to create mission: %w", err),
		}, nil
	}

	// Update status to running
	m.Status = mission.MissionStatusRunning
	m.StartedAt = &now
	if err := cc.MissionStore.Update(cc.Ctx, m); err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to start mission: %w", err),
		}, nil
	}

	return &CommandResult{
		Data: &MissionRunResult{
			Mission:     m,
			Workflow:    wf,
			Status:      "started",
			NodesCount:  len(wf.Nodes),
			EntryPoints: len(wf.EntryPoints),
			ExitPoints:  len(wf.ExitPoints),
		},
		Message: fmt.Sprintf("Mission '%s' started successfully", m.Name),
	}, nil
}

// MissionResume resumes a paused mission.
func MissionResume(cc *CommandContext, name string) (*CommandResult, error) {
	// Validate mission store
	if cc.MissionStore == nil {
		return nil, fmt.Errorf("mission store not initialized")
	}

	// Get mission
	m, err := cc.MissionStore.GetByName(cc.Ctx, name)
	if err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to get mission: %w", err),
		}, nil
	}

	// Check if mission can be resumed (not completed or failed)
	if m.Status == mission.MissionStatusCompleted {
		return &CommandResult{
			Error: fmt.Errorf("cannot resume completed mission"),
		}, nil
	}
	if m.Status == mission.MissionStatusFailed {
		return &CommandResult{
			Error: fmt.Errorf("cannot resume failed mission"),
		}, nil
	}
	if m.Status == mission.MissionStatusCancelled {
		return &CommandResult{
			Error: fmt.Errorf("cannot resume cancelled mission"),
		}, nil
	}

	// Update status to running
	if err := cc.MissionStore.UpdateStatus(cc.Ctx, m.ID, mission.MissionStatusRunning); err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to resume mission: %w", err),
		}, nil
	}

	return &CommandResult{
		Data: map[string]interface{}{
			"mission": m.Name,
			"status":  "resumed",
		},
		Message: fmt.Sprintf("Mission '%s' resumed successfully", m.Name),
	}, nil
}

// MissionStop stops a running mission.
func MissionStop(cc *CommandContext, name string) (*CommandResult, error) {
	// Validate mission store
	if cc.MissionStore == nil {
		return nil, fmt.Errorf("mission store not initialized")
	}

	// Get mission
	m, err := cc.MissionStore.GetByName(cc.Ctx, name)
	if err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to get mission: %w", err),
		}, nil
	}

	// Check if mission is running
	if m.Status != mission.MissionStatusRunning {
		return &CommandResult{
			Error: fmt.Errorf("mission is not running (current status: %s)", m.Status),
		}, nil
	}

	// Update status to cancelled
	if err := cc.MissionStore.UpdateStatus(cc.Ctx, m.ID, mission.MissionStatusCancelled); err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to stop mission: %w", err),
		}, nil
	}

	return &CommandResult{
		Data: map[string]interface{}{
			"mission": m.Name,
			"status":  "stopped",
		},
		Message: fmt.Sprintf("Mission '%s' stopped successfully", m.Name),
	}, nil
}

// MissionDelete deletes a mission.
// The force parameter determines whether to skip confirmation prompts (handled by caller).
// This function only performs the actual deletion logic.
func MissionDelete(cc *CommandContext, name string, force bool) (*CommandResult, error) {
	// Validate mission store
	if cc.MissionStore == nil {
		return nil, fmt.Errorf("mission store not initialized")
	}

	// Get mission to retrieve ID
	m, err := cc.MissionStore.GetByName(cc.Ctx, name)
	if err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to get mission: %w", err),
		}, nil
	}

	// Delete mission
	if err := cc.MissionStore.Delete(cc.Ctx, m.ID); err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to delete mission: %w", err),
		}, nil
	}

	return &CommandResult{
		Data: map[string]interface{}{
			"mission": m.Name,
			"status":  "deleted",
		},
		Message: fmt.Sprintf("Mission '%s' deleted successfully", m.Name),
	}, nil
}

// Helper functions

// IsValidMissionStatus validates that a mission status is one of the valid values.
func IsValidMissionStatus(status mission.MissionStatus) bool {
	switch status {
	case mission.MissionStatusPending,
		mission.MissionStatusRunning,
		mission.MissionStatusCompleted,
		mission.MissionStatusFailed,
		mission.MissionStatusCancelled:
		return true
	default:
		return false
	}
}
