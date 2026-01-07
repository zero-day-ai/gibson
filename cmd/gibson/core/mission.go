package core

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/verbose"
	"github.com/zero-day-ai/gibson/internal/workflow"
	"gopkg.in/yaml.v3"
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
	Mission     *mission.Mission
	Workflow    *workflow.Workflow
	Status      string
	NodesCount  int
	EntryPoints int
	ExitPoints  int
}

// MissionRun creates and runs a new mission from a workflow YAML file.
// This is the non-verbose version that calls MissionRunWithVerbose with nil verbose writer.
func MissionRun(cc *CommandContext, workflowFile string, targetFlag string) (*CommandResult, error) {
	return MissionRunWithVerbose(cc, workflowFile, targetFlag, nil, verbose.LevelNone)
}

// MissionRunWithVerbose creates and runs a new mission from a workflow YAML file with verbose logging support.
// If vw is non-nil, it integrates verbose event logging for mission events and DAG node execution.
func MissionRunWithVerbose(cc *CommandContext, workflowFile string, targetFlag string, vw *verbose.VerboseWriter, level verbose.VerboseLevel) (*CommandResult, error) {
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

	// Parse YAML to get target field (ParseWorkflowFile only returns Workflow, not YAMLWorkflow)
	data, err := os.ReadFile(workflowFile)
	if err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to read workflow file: %w", err),
		}, nil
	}

	var yamlWf workflow.YAMLWorkflow
	if err := yaml.Unmarshal(data, &yamlWf); err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to parse workflow YAML: %w", err),
		}, nil
	}

	// Resolve target
	targetID, err := resolveTarget(cc, targetFlag, yamlWf.Target)
	if err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to resolve target: %w", err),
		}, nil
	}

	// Serialize workflow to JSON
	workflowJSON, err := json.Marshal(wf)
	if err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to serialize workflow: %w", err),
		}, nil
	}

	// Create mission with resolved target
	now := time.Now()
	m := &mission.Mission{
		ID:               types.NewID(),
		Name:             wf.Name,
		Description:      wf.Description,
		Status:           mission.MissionStatusPending,
		TargetID:         targetID, // Use resolved target ID from CLI flag or YAML
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

	// If verbose is enabled, set up mission event bridge
	// Note: This would require having access to the orchestrator and its event emitter.
	// Since the current implementation doesn't actually execute the mission (just creates it),
	// we'll add a TODO for when mission execution is integrated.
	// TODO: When mission execution is integrated:
	//   if vw != nil {
	//       bridge := verbose.NewMissionEventBridge(orchestrator.EventEmitter(), vw.Bus())
	//       bridge.Start(cc.Ctx)
	//       defer bridge.Stop()
	//
	//       // Wrap harness factory for verbose events
	//       harnessFactory = verbose.WrapHarnessFactory(harnessFactory, vw.Bus(), level)
	//   }

	// TODO: Actually execute the mission with orchestrator
	// For now, this just creates and saves the mission without executing it

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

// resolveTarget resolves target from CLI flag or YAML to a types.ID.
// Priority: CLI flag > YAML reference > YAML inline > error
func resolveTarget(cc *CommandContext, flagTarget string, yamlTarget *workflow.YAMLTarget) (types.ID, error) {
	// Priority 1: CLI flag
	if flagTarget != "" {
		return lookupTarget(cc, flagTarget)
	}

	// Priority 2: YAML target
	if yamlTarget == nil {
		return "", fmt.Errorf("target required: specify in YAML or use --target flag")
	}

	// String reference in YAML
	if yamlTarget.IsReference() {
		return lookupTarget(cc, yamlTarget.Reference)
	}

	// Inline definition in YAML
	if yamlTarget.IsInline() {
		return createInlineTarget(cc, yamlTarget)
	}

	return "", fmt.Errorf("invalid target specification")
}

// lookupTarget finds a target by name or ID in the database.
// It tries name lookup first (more common), then falls back to UUID parsing.
func lookupTarget(cc *CommandContext, nameOrID string) (types.ID, error) {
	if cc.TargetDAO == nil {
		return "", fmt.Errorf("target DAO not initialized")
	}

	// Try name first (more common)
	target, err := cc.TargetDAO.GetByName(cc.Ctx, nameOrID)
	if err == nil {
		return target.ID, nil
	}

	// Try as UUID
	id, err := types.ParseID(nameOrID)
	if err != nil {
		return "", fmt.Errorf("target not found: %s", nameOrID)
	}

	exists, err := cc.TargetDAO.Exists(cc.Ctx, id)
	if err != nil || !exists {
		return "", fmt.Errorf("target not found: %s", nameOrID)
	}

	return id, nil
}

// createInlineTarget creates a new target from inline YAML definition.
// If no name is provided, generates one like "inline-<short-id>".
func createInlineTarget(cc *CommandContext, yt *workflow.YAMLTarget) (types.ID, error) {
	if cc.TargetDAO == nil {
		return "", fmt.Errorf("target DAO not initialized")
	}

	name := yt.Name
	if name == "" {
		// Generate a short name using first 8 chars of UUID
		id := types.NewID()
		name = fmt.Sprintf("inline-%s", string(id)[:8])
	}

	// Check for name collision
	if exists, _ := cc.TargetDAO.ExistsByName(cc.Ctx, name); exists {
		return "", fmt.Errorf("target already exists: %s", name)
	}

	target := types.NewTargetWithConnection(name, yt.Type, yt.Connection)
	if yt.Provider != "" {
		target.Provider = types.Provider(yt.Provider)
	}
	target.Tags = yt.Tags

	if err := cc.TargetDAO.Create(cc.Ctx, target); err != nil {
		return "", fmt.Errorf("failed to create inline target: %w", err)
	}

	return target.ID, nil
}
