package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/internal"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

var missionCmd = &cobra.Command{
	Use:   "mission",
	Short: "Manage missions",
	Long:  `Manage Gibson missions - create, run, monitor, and control mission execution`,
}

var missionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all missions",
	Long:  `List all missions with optional status filter`,
	RunE:  runMissionList,
}

var missionShowCmd = &cobra.Command{
	Use:   "show NAME",
	Short: "Show mission details",
	Long:  `Display detailed information about a specific mission including workflow, status, and progress`,
	Args:  cobra.ExactArgs(1),
	RunE:  runMissionShow,
}

var missionRunCmd = &cobra.Command{
	Use:   "run -f WORKFLOW_FILE",
	Short: "Run a new mission from workflow YAML",
	Long:  `Create and start a new mission from a workflow YAML definition file`,
	RunE:  runMissionRun,
}

var missionResumeCmd = &cobra.Command{
	Use:   "resume NAME",
	Short: "Resume a paused mission",
	Long:  `Resume execution of a paused mission`,
	Args:  cobra.ExactArgs(1),
	RunE:  runMissionResume,
}

var missionStopCmd = &cobra.Command{
	Use:   "stop NAME",
	Short: "Stop a running mission",
	Long:  `Stop a currently running mission (can be resumed later)`,
	Args:  cobra.ExactArgs(1),
	RunE:  runMissionStop,
}

var missionDeleteCmd = &cobra.Command{
	Use:   "delete NAME",
	Short: "Delete a mission",
	Long:  `Delete a mission and all associated data`,
	Args:  cobra.ExactArgs(1),
	RunE:  runMissionDelete,
}

// Flags
var (
	missionStatusFilter string
	missionWorkflowFile string
	missionForceDelete  bool
)

// getHomeDirFromFlags returns the Gibson home directory from flags or environment
func getHomeDirFromFlags(flags *GlobalFlags) (string, error) {
	if flags != nil && flags.HomeDir != "" {
		return flags.HomeDir, nil
	}
	return getGibsonHome()
}

func init() {
	// Add subcommands
	missionCmd.AddCommand(missionListCmd)
	missionCmd.AddCommand(missionShowCmd)
	missionCmd.AddCommand(missionRunCmd)
	missionCmd.AddCommand(missionResumeCmd)
	missionCmd.AddCommand(missionStopCmd)
	missionCmd.AddCommand(missionDeleteCmd)

	// List flags
	missionListCmd.Flags().StringVar(&missionStatusFilter, "status", "", "Filter by status (pending, running, paused, completed, failed)")

	// Run flags
	missionRunCmd.Flags().StringVarP(&missionWorkflowFile, "file", "f", "", "Workflow YAML file (required)")
	missionRunCmd.MarkFlagRequired("file")

	// Delete flags
	missionDeleteCmd.Flags().BoolVar(&missionForceDelete, "force", false, "Skip confirmation prompt")
}

// runMissionList lists all missions with optional status filter
func runMissionList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Parse global flags
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return internal.WrapError(internal.ExitConfigError, "failed to parse flags", err)
	}

	// Get Gibson home directory
	homeDir, err := getHomeDirFromFlags(flags)
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return internal.WrapError(internal.ExitDatabaseError, "failed to open database", err)
	}
	defer db.Close()

	// Create mission store
	missionStore := mission.NewDBMissionStore(db)

	// Parse status filter
	var filter *mission.MissionFilter
	if missionStatusFilter != "" {
		status := mission.MissionStatus(missionStatusFilter)
		// Validate status
		if !isValidMissionStatus(status) {
			return internal.NewCLIError(internal.ExitError, "invalid status filter: must be pending, running, completed, failed, or cancelled")
		}
		filter = mission.NewMissionFilter().WithStatus(status)
	} else {
		filter = mission.NewMissionFilter()
	}

	// List missions
	missions, err := missionStore.List(ctx, filter)
	if err != nil {
		return internal.WrapError(internal.ExitDatabaseError, "failed to list missions", err)
	}

	// Create formatter
	outFormat := internal.FormatText
	if flags.OutputFormat == "json" {
		outFormat = internal.FormatJSON
	}
	formatter := internal.NewFormatter(outFormat, cmd.OutOrStdout())

	if outFormat == internal.FormatJSON {
		return formatter.PrintJSON(map[string]interface{}{
			"missions": missions,
			"count":    len(missions),
		})
	}

	// Text format
	if len(missions) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No missions found")
		return nil
	}

	// Print table
	headers := []string{"Name", "Status", "Progress", "Findings", "Created", "Updated"}
	rows := make([][]string, 0, len(missions))

	for _, m := range missions {
		progressPct := fmt.Sprintf("%.1f%%", m.Progress*100)

		rows = append(rows, []string{
			m.Name,
			string(m.Status),
			progressPct,
			fmt.Sprintf("%d", m.FindingsCount),
			formatTime(m.CreatedAt),
			formatTime(m.UpdatedAt),
		})
	}

	return formatter.PrintTable(headers, rows)
}

// runMissionShow shows detailed mission information
func runMissionShow(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	missionName := args[0]

	// Parse global flags
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return internal.WrapError(internal.ExitConfigError, "failed to parse flags", err)
	}

	// Get Gibson home directory
	homeDir, err := getHomeDirFromFlags(flags)
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return internal.WrapError(internal.ExitDatabaseError, "failed to open database", err)
	}
	defer db.Close()

	// Create mission store
	missionStore := mission.NewDBMissionStore(db)

	// Get mission
	m, err := missionStore.GetByName(ctx, missionName)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to get mission", err)
	}

	// Create formatter
	outFormat := internal.FormatText
	if flags.OutputFormat == "json" {
		outFormat = internal.FormatJSON
	}
	formatter := internal.NewFormatter(outFormat, cmd.OutOrStdout())

	if outFormat == internal.FormatJSON {
		return formatter.PrintJSON(m)
	}

	// Text format - detailed view
	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	defer tw.Flush()

	fmt.Fprintf(tw, "NAME:\t%s\n", m.Name)
	fmt.Fprintf(tw, "ID:\t%s\n", m.ID)
	fmt.Fprintf(tw, "STATUS:\t%s\n", m.Status)
	fmt.Fprintf(tw, "DESCRIPTION:\t%s\n", m.Description)
	fmt.Fprintf(tw, "PROGRESS:\t%.1f%%\n", m.Progress*100)
	fmt.Fprintf(tw, "FINDINGS:\t%d\n", m.FindingsCount)
	fmt.Fprintf(tw, "CREATED:\t%s\n", m.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(tw, "UPDATED:\t%s\n", m.UpdatedAt.Format(time.RFC3339))

	if m.StartedAt != nil {
		fmt.Fprintf(tw, "STARTED:\t%s\n", m.StartedAt.Format(time.RFC3339))
	}
	if m.CompletedAt != nil {
		fmt.Fprintf(tw, "COMPLETED:\t%s\n", m.CompletedAt.Format(time.RFC3339))
	}

	// Show workflow details
	if m.WorkflowJSON != "" {
		var wf workflow.Workflow
		if err := json.Unmarshal([]byte(m.WorkflowJSON), &wf); err == nil {
			fmt.Fprintln(tw, "")
			fmt.Fprintf(tw, "WORKFLOW:\t%s\n", wf.Name)
			fmt.Fprintf(tw, "WORKFLOW ID:\t%s\n", m.WorkflowID)
			fmt.Fprintf(tw, "NODES:\t%d\n", len(wf.Nodes))
			fmt.Fprintf(tw, "ENTRY POINTS:\t%d\n", len(wf.EntryPoints))
			fmt.Fprintf(tw, "EXIT POINTS:\t%d\n", len(wf.ExitPoints))
		}
	}

	// Show agent assignments
	if len(m.AgentAssignments) > 0 {
		fmt.Fprintln(tw, "")
		fmt.Fprintln(tw, "AGENT ASSIGNMENTS:")
		for nodeID, agentName := range m.AgentAssignments {
			fmt.Fprintf(tw, "  %s:\t%s\n", nodeID, agentName)
		}
	}

	return nil
}

// runMissionRun creates and runs a new mission from workflow YAML
func runMissionRun(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Parse global flags
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return internal.WrapError(internal.ExitConfigError, "failed to parse flags", err)
	}

	// Get Gibson home directory
	homeDir, err := getHomeDirFromFlags(flags)
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Parse workflow file
	if flags.IsVerbose() {
		fmt.Printf("Loading workflow from %s\n", missionWorkflowFile)
	}

	wf, err := workflow.ParseWorkflowFile(missionWorkflowFile)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to parse workflow file", err)
	}

	if flags.IsVerbose() {
		fmt.Printf("Workflow loaded: %s (%d nodes)\n", wf.Name, len(wf.Nodes))
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return internal.WrapError(internal.ExitDatabaseError, "failed to open database", err)
	}
	defer db.Close()

	// Create mission store
	missionStore := mission.NewDBMissionStore(db)

	// Serialize workflow to JSON
	workflowJSON, err := json.Marshal(wf)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to serialize workflow", err)
	}

	// Create mission
	// Note: TargetID is required in the new Mission type, but we don't have it in this workflow-only context.
	// This is a limitation - missions should probably be created with explicit target specification.
	// For now, we'll use a zero ID as a placeholder, which will fail validation.
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

	if err := missionStore.Save(ctx, m); err != nil {
		return internal.WrapError(internal.ExitDatabaseError, "failed to create mission", err)
	}

	// Update status to running
	m.Status = mission.MissionStatusRunning
	m.StartedAt = &now
	if err := missionStore.Update(ctx, m); err != nil {
		return internal.WrapError(internal.ExitDatabaseError, "failed to start mission", err)
	}

	// Create formatter
	outFormat := internal.FormatText
	if flags.OutputFormat == "json" {
		outFormat = internal.FormatJSON
	}
	formatter := internal.NewFormatter(outFormat, cmd.OutOrStdout())

	if outFormat == internal.FormatJSON {
		return formatter.PrintJSON(map[string]interface{}{
			"mission": m,
			"status":  "started",
		})
	}

	// Print success message
	fmt.Printf("Mission '%s' started successfully\n", m.Name)
	fmt.Printf("Mission ID: %s\n", m.ID)
	fmt.Printf("Workflow: %s (%d nodes)\n", wf.Name, len(wf.Nodes))

	return nil
}

// runMissionResume resumes a paused mission
func runMissionResume(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	missionName := args[0]

	// Parse global flags
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return internal.WrapError(internal.ExitConfigError, "failed to parse flags", err)
	}

	// Get Gibson home directory
	homeDir, err := getHomeDirFromFlags(flags)
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return internal.WrapError(internal.ExitDatabaseError, "failed to open database", err)
	}
	defer db.Close()

	// Create mission store
	missionStore := mission.NewDBMissionStore(db)

	// Get mission
	m, err := missionStore.GetByName(ctx, missionName)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to get mission", err)
	}

	// Check if mission can be resumed (not completed or failed)
	if m.Status == mission.MissionStatusCompleted {
		return internal.NewCLIError(internal.ExitError, "cannot resume completed mission")
	}
	if m.Status == mission.MissionStatusFailed {
		return internal.NewCLIError(internal.ExitError, "cannot resume failed mission")
	}
	if m.Status == mission.MissionStatusCancelled {
		return internal.NewCLIError(internal.ExitError, "cannot resume cancelled mission")
	}

	// Update status to running
	if err := missionStore.UpdateStatus(ctx, m.ID, mission.MissionStatusRunning); err != nil {
		return internal.WrapError(internal.ExitDatabaseError, "failed to resume mission", err)
	}

	// Create formatter
	outFormat := internal.FormatText
	if flags.OutputFormat == "json" {
		outFormat = internal.FormatJSON
	}
	formatter := internal.NewFormatter(outFormat, cmd.OutOrStdout())

	if outFormat == internal.FormatJSON {
		return formatter.PrintJSON(map[string]interface{}{
			"mission": m.Name,
			"status":  "resumed",
		})
	}

	fmt.Printf("Mission '%s' resumed successfully\n", m.Name)
	return nil
}

// runMissionStop stops a running mission
func runMissionStop(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	missionName := args[0]

	// Parse global flags
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return internal.WrapError(internal.ExitConfigError, "failed to parse flags", err)
	}

	// Get Gibson home directory
	homeDir, err := getHomeDirFromFlags(flags)
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return internal.WrapError(internal.ExitDatabaseError, "failed to open database", err)
	}
	defer db.Close()

	// Create mission store
	missionStore := mission.NewDBMissionStore(db)

	// Get mission
	m, err := missionStore.GetByName(ctx, missionName)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to get mission", err)
	}

	// Check if mission is running
	if m.Status != mission.MissionStatusRunning {
		return internal.NewCLIError(internal.ExitError, fmt.Sprintf("mission is not running (current status: %s)", m.Status))
	}

	// Update status to cancelled
	if err := missionStore.UpdateStatus(ctx, m.ID, mission.MissionStatusCancelled); err != nil {
		return internal.WrapError(internal.ExitDatabaseError, "failed to stop mission", err)
	}

	// Create formatter
	outFormat := internal.FormatText
	if flags.OutputFormat == "json" {
		outFormat = internal.FormatJSON
	}
	formatter := internal.NewFormatter(outFormat, cmd.OutOrStdout())

	if outFormat == internal.FormatJSON {
		return formatter.PrintJSON(map[string]interface{}{
			"mission": m.Name,
			"status":  "stopped",
		})
	}

	fmt.Printf("Mission '%s' stopped successfully\n", m.Name)
	return nil
}

// runMissionDelete deletes a mission
func runMissionDelete(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	missionName := args[0]

	// Parse global flags
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return internal.WrapError(internal.ExitConfigError, "failed to parse flags", err)
	}

	// Confirmation prompt unless --force is set
	if !missionForceDelete {
		fmt.Printf("Are you sure you want to delete mission '%s'? This action cannot be undone.\n", missionName)
		fmt.Print("Type 'yes' to confirm: ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return internal.WrapError(internal.ExitError, "failed to read confirmation", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "yes" {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}

	// Get Gibson home directory
	homeDir, err := getHomeDirFromFlags(flags)
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return internal.WrapError(internal.ExitDatabaseError, "failed to open database", err)
	}
	defer db.Close()

	// Create mission store
	missionStore := mission.NewDBMissionStore(db)

	// Get mission to retrieve ID
	m, err := missionStore.GetByName(ctx, missionName)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to get mission", err)
	}

	// Delete mission
	if err := missionStore.Delete(ctx, m.ID); err != nil {
		return internal.WrapError(internal.ExitDatabaseError, "failed to delete mission", err)
	}

	// Create formatter
	outFormat := internal.FormatText
	if flags.OutputFormat == "json" {
		outFormat = internal.FormatJSON
	}
	formatter := internal.NewFormatter(outFormat, cmd.OutOrStdout())

	if outFormat == internal.FormatJSON {
		return formatter.PrintJSON(map[string]interface{}{
			"mission": m.Name,
			"status":  "deleted",
		})
	}

	fmt.Printf("Mission '%s' deleted successfully\n", m.Name)
	return nil
}

// Helper functions

func isValidMissionStatus(status mission.MissionStatus) bool {
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

func formatTime(t time.Time) string {
	// Format relative time for recent dates
	now := time.Now()
	diff := now.Sub(t)

	if diff < time.Minute {
		return "just now"
	}
	if diff < time.Hour {
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	}
	if diff < 24*time.Hour {
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	if diff < 7*24*time.Hour {
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}

	// For older dates, show absolute date
	return t.Format("2006-01-02")
}
