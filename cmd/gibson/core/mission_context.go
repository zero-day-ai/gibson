package core

import (
	"fmt"
	"strings"

	dclient "github.com/zero-day-ai/gibson/internal/daemon/client"
)

// MissionContextResult represents the structured output from MissionContext command.
type MissionContextResult struct {
	MissionName      string
	MissionID        string
	RunNumber        int
	TotalRuns        int
	Status           string
	Resumed          bool
	ResumedFromNode  string
	PreviousRunID    string
	PreviousStatus   string
	TotalFindings    int
	MemoryContinuity string
	WorkflowName     string
}

// MissionContext retrieves and displays comprehensive context for a mission.
// This includes run history, resume status, and cross-run information.
//
// The function queries the daemon for:
// - Current mission details (name, ID, status)
// - Mission history to determine run number (X of Y)
// - Previous run information if this is a resumed mission
// - Checkpoint data to determine if mission was resumed
// - Total findings across all runs
//
// Parameters:
//   - cc: Command context with daemon connection
//   - missionID: The mission ID to get context for
//
// Returns:
//   - *CommandResult: Structured result containing mission context data
//   - error: Non-nil if context retrieval fails
func MissionContext(cc *CommandContext, missionID string) (*CommandResult, error) {
	// Mission context requires daemon for full information
	client, err := dclient.RequireDaemon(cc.Ctx)
	if err != nil {
		return &CommandResult{
			Error: fmt.Errorf("mission context requires daemon: %w", err),
		}, nil
	}
	defer client.Close()

	// Step 1: Get current mission details
	missions, _, err := client.ListMissions(cc.Ctx, false, "", missionID, 10, 0)
	if err != nil {
		return &CommandResult{
			Error: fmt.Errorf("failed to query mission: %w", err),
		}, nil
	}

	// Find the matching mission
	var currentMission *dclient.MissionInfo
	for i := range missions {
		if missions[i].ID == missionID || strings.Contains(missions[i].ID, missionID) {
			currentMission = &missions[i]
			break
		}
	}

	if currentMission == nil {
		return &CommandResult{
			Error: fmt.Errorf("mission not found: %s", missionID),
		}, nil
	}

	// Step 2: Extract mission name from workflow path or ID
	missionName := extractMissionName(currentMission.WorkflowPath, currentMission.ID)

	// Step 3: Get mission history to determine run number
	runs, totalRuns, err := client.GetMissionHistory(cc.Ctx, missionName, 100, 0)
	if err != nil {
		// If history fails, we can still show basic info with run 1 of 1
		runs = []dclient.MissionRun{}
		totalRuns = 1
	}

	// Find current run number
	currentRunNumber := 1
	var previousRun *dclient.MissionRun
	for i, run := range runs {
		if run.MissionID == currentMission.ID {
			currentRunNumber = run.RunNumber
			// Previous run is the one before this (if exists)
			if i+1 < len(runs) {
				previousRun = &runs[i+1]
			}
			break
		}
	}

	// Step 4: Get checkpoint data to determine resume status
	checkpoints, err := client.GetMissionCheckpoints(cc.Ctx, currentMission.ID)
	resumed := false
	resumedFromNode := ""
	if err == nil && len(checkpoints) > 0 {
		// If we have checkpoints and this is not the first run, it's likely resumed
		// We can infer resume status from checkpoint data
		latestCheckpoint := checkpoints[0]
		if latestCheckpoint.CompletedNodes > 0 {
			resumed = true
			// Note: We don't have direct access to which node we resumed from
			// This would require additional metadata in the checkpoint
			resumedFromNode = fmt.Sprintf("checkpoint-%d-nodes", latestCheckpoint.CompletedNodes)
		}
	}

	// Step 5: Calculate total findings across all runs
	totalFindings := 0
	for _, run := range runs {
		totalFindings += run.FindingsCount
	}

	// Step 6: Build result
	result := &MissionContextResult{
		MissionName:      missionName,
		MissionID:        currentMission.ID,
		RunNumber:        currentRunNumber,
		TotalRuns:        totalRuns,
		Status:           currentMission.Status,
		Resumed:          resumed,
		ResumedFromNode:  resumedFromNode,
		TotalFindings:    totalFindings,
		MemoryContinuity: "inherit", // Default, would need metadata to determine
		WorkflowName:     currentMission.WorkflowPath,
	}

	if previousRun != nil {
		result.PreviousRunID = previousRun.MissionID
		result.PreviousStatus = previousRun.Status
	}

	return &CommandResult{
		Data:    result,
		Message: fmt.Sprintf("Mission context for %s", missionID),
	}, nil
}

// extractMissionName extracts a clean mission name from workflow path or ID.
// Examples:
//   - "/path/to/recon-webapp.yaml" -> "recon-webapp"
//   - "recon-webapp-20260107-123456" -> "recon-webapp"
func extractMissionName(workflowPath, missionID string) string {
	// Try to extract from workflow path first
	if workflowPath != "" {
		// Get base filename
		parts := strings.Split(workflowPath, "/")
		filename := parts[len(parts)-1]
		// Remove extension
		name := strings.TrimSuffix(filename, ".yaml")
		name = strings.TrimSuffix(name, ".yml")
		if name != "" {
			return name
		}
	}

	// Fallback to mission ID, strip timestamp suffix if present
	// Mission IDs often have format: name-YYYYMMDD-HHMMSS
	parts := strings.Split(missionID, "-")
	if len(parts) >= 3 {
		// Try to find where the timestamp starts (numeric part)
		for i := len(parts) - 1; i >= 0; i-- {
			if !isNumeric(parts[i]) {
				// Everything up to this point is the name
				return strings.Join(parts[:i+1], "-")
			}
		}
	}

	// If all else fails, return the mission ID
	return missionID
}

// isNumeric checks if a string contains only digits.
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
