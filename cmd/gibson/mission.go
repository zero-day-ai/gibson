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
	"github.com/zero-day-ai/gibson/cmd/gibson/core"
	"github.com/zero-day-ai/gibson/cmd/gibson/internal"
	dclient "github.com/zero-day-ai/gibson/internal/daemon/client"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/mission"
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
	Use:   "resume <mission-id>",
	Short: "Resume a paused mission",
	Long: `Resume execution of a paused mission from its last checkpoint.

The mission will restore its state from the last saved checkpoint and continue
execution from where it left off. All previously discovered findings, metrics,
and completed workflow nodes are preserved.

CHECKPOINT SELECTION:
  By default, the latest checkpoint is used. Use --from-checkpoint to specify
  a particular checkpoint ID to resume from. This is useful if you want to
  restart from an earlier point in the mission execution.

The mission must be in 'paused' status to be resumed. Use 'gibson mission status'
to check the current status and checkpoint availability.`,
	Example: `  # Resume from the latest checkpoint
  gibson mission resume mission-20260107-153045-abc123

  # Resume from a specific checkpoint
  gibson mission resume mission-20260107-153045-abc123 --from-checkpoint chk-20260107-153100-def456

  # Check status before resuming
  gibson mission status mission-20260107-153045-abc123
  gibson mission resume mission-20260107-153045-abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runMissionResume,
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

var missionPauseCmd = &cobra.Command{
	Use:   "pause <mission-id>",
	Short: "Pause a running mission",
	Long: `Pause a running mission at the next clean checkpoint boundary.

GRACEFUL PAUSE (default):
  The mission will complete its current workflow node execution before pausing.
  This ensures a consistent state is saved in the checkpoint, making resume more
  reliable. The checkpoint will include:
    - Current workflow DAG state (completed/pending nodes)
    - Node results from completed nodes
    - All findings discovered so far
    - Token usage and cost metrics

FORCE PAUSE (--force):
  The mission will pause immediately without waiting for the current node to
  complete. This may result in the mission pausing at a less optimal state,
  potentially requiring the current node to be restarted on resume.

A paused mission can be resumed later with 'gibson mission resume'.`,
	Example: `  # Gracefully pause a mission (waits for current node to complete)
  gibson mission pause mission-20260107-153045-abc123

  # Force immediate pause without waiting for clean checkpoint
  gibson mission pause mission-20260107-153045-abc123 --force

  # Pause and see checkpoint ID
  gibson mission pause mission-20260107-153045-abc123
  # Output: Mission paused. Checkpoint: chk-20260107-153100-def456`,
	Args: cobra.ExactArgs(1),
	RunE: runMissionPause,
}

var missionHistoryCmd = &cobra.Command{
	Use:   "history <name>",
	Short: "Show mission execution history",
	Long: `Display the complete execution history for a mission name.

This command shows all execution runs of missions with the given workflow name.
Each run represents a separate execution instance, allowing you to track how
a mission has been executed over time.

DISPLAYED INFORMATION:
  - Run number: Sequential execution number (1, 2, 3, ...)
  - Mission ID: Unique identifier for each run
  - Status: Current or final status (running, paused, completed, failed)
  - Created: When the run was started
  - Completed: When the run finished (if applicable)
  - Findings: Number of security findings discovered in that run

This is useful for:
  - Comparing results across multiple runs
  - Tracking mission execution patterns
  - Auditing security testing activities
  - Finding the mission ID for a specific run to resume or inspect`,
	Example: `  # Show all runs of a mission workflow
  gibson mission history api-security-scan

  # Show limited number of recent runs
  gibson mission history api-security-scan --limit 5

  # Example output:
  # Run  Mission ID                        Status     Created              Findings
  # 3    api-security-scan-20260107-1530  completed  2026-01-07 15:30:45  12
  # 2    api-security-scan-20260106-0915  completed  2026-01-06 09:15:22  8
  # 1    api-security-scan-20260105-1420  failed     2026-01-05 14:20:11  5`,
	Args: cobra.ExactArgs(1),
	RunE: runMissionHistory,
}

var missionCheckpointsCmd = &cobra.Command{
	Use:   "checkpoints <mission-id>",
	Short: "List checkpoints for a mission",
	Long: `Display all saved checkpoints for a specific mission.

Checkpoints represent saved execution states that can be used to resume a mission
from a specific point. They are created:
  - Automatically when pausing a mission (graceful pause)
  - Periodically during long-running missions (auto-checkpoint)
  - At workflow boundaries (node completion)

CHECKPOINT CONTENTS:
  Each checkpoint includes:
    - Complete workflow DAG state
    - Results from completed nodes
    - All findings discovered up to that point
    - Token usage and cost metrics
    - Mission metadata and configuration

USE CASES:
  - View available resume points for a paused mission
  - Select a specific checkpoint to resume from
  - Audit mission execution progress over time
  - Verify checkpoint integrity before resuming`,
	Example: `  # List all checkpoints for a mission
  gibson mission checkpoints mission-20260107-153045-abc123

  # Example output:
  # Checkpoint ID               Created              Nodes      Findings
  # chk-20260107-153100-def456  2026-01-07 15:31:00  5/10       8
  # chk-20260107-153045-abc123  2026-01-07 15:30:45  3/10       5
  # chk-20260107-153030-xyz789  2026-01-07 15:30:30  1/10       2

  # Use a checkpoint to resume
  gibson mission checkpoints mission-20260107-153045-abc123
  gibson mission resume mission-20260107-153045-abc123 --from-checkpoint chk-20260107-153045-abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runMissionCheckpoints,
}

var missionStatusCmd = &cobra.Command{
	Use:   "status <mission-id>",
	Short: "Show mission status",
	Long: `Display the current status of a mission including progress, checkpoint availability, and resume capability.

This command provides a quick overview of mission state, useful for monitoring
running missions and determining if a paused mission can be resumed.`,
	Args: cobra.ExactArgs(1),
	RunE: runMissionStatus,
}

// Flags
var (
	missionStatusFilter   string
	missionWorkflowFile   string
	missionTargetFlag     string
	missionForceDelete    bool
	missionForcePause     bool
	missionFromCheckpoint string
)

// getHomeDirFromFlags returns the Gibson home directory from flags or environment
func getHomeDirFromFlags(flags *GlobalFlags) (string, error) {
	if flags != nil && flags.HomeDir != "" {
		return flags.HomeDir, nil
	}
	return getGibsonHome()
}

// buildMissionCommandContext creates a CommandContext for mission commands.
// It handles database connection, mission store initialization, and context setup.
func buildMissionCommandContext(cmd *cobra.Command) (*core.CommandContext, error) {
	ctx := cmd.Context()

	// Parse global flags
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return nil, internal.WrapError(internal.ExitConfigError, "failed to parse flags", err)
	}

	// Get Gibson home directory
	homeDir, err := getHomeDirFromFlags(flags)
	if err != nil {
		return nil, fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return nil, internal.WrapError(internal.ExitDatabaseError, "failed to open database", err)
	}

	// Create mission store
	missionStore := mission.NewDBMissionStore(db)

	// Create target DAO
	targetDAO := database.NewTargetDAO(db)

	return &core.CommandContext{
		Ctx:          ctx,
		DB:           db,
		HomeDir:      homeDir,
		MissionStore: missionStore,
		TargetDAO:    targetDAO,
	}, nil
}

func init() {
	// Add subcommands
	missionCmd.AddCommand(missionListCmd)
	missionCmd.AddCommand(missionShowCmd)
	missionCmd.AddCommand(missionStatusCmd)
	missionCmd.AddCommand(missionRunCmd)
	missionCmd.AddCommand(missionResumeCmd)
	missionCmd.AddCommand(missionStopCmd)
	missionCmd.AddCommand(missionDeleteCmd)
	missionCmd.AddCommand(missionPauseCmd)
	missionCmd.AddCommand(missionHistoryCmd)
	missionCmd.AddCommand(missionCheckpointsCmd)

	// List flags
	missionListCmd.Flags().StringVar(&missionStatusFilter, "status", "", "Filter by status (pending, running, paused, completed, failed)")

	// Run flags
	missionRunCmd.Flags().StringVarP(&missionWorkflowFile, "file", "f", "", "Workflow YAML file (required)")
	missionRunCmd.MarkFlagRequired("file")
	missionRunCmd.Flags().StringVar(&missionTargetFlag, "target", "", "Target name or ID (overrides YAML target if specified)")

	// Delete flags
	missionDeleteCmd.Flags().BoolVar(&missionForceDelete, "force", false, "Skip confirmation prompt")

	// Pause flags
	missionPauseCmd.Flags().BoolVar(&missionForcePause, "force", false, "Pause immediately without waiting for clean checkpoint boundary")

	// Resume flags
	missionResumeCmd.Flags().StringVar(&missionFromCheckpoint, "from-checkpoint", "", "Resume from specific checkpoint ID (optional)")
}

// runMissionList lists all missions with optional status filter
func runMissionList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Try daemon first (optional - fall back to local if unavailable)
	client := dclient.OptionalDaemon(ctx)
	if client != nil {
		defer client.Close()

		// Query missions from daemon
		missions, total, err := client.ListMissions(ctx, false, missionStatusFilter, "", 100, 0)
		if err == nil {
			// Display missions from daemon
			fmt.Printf("Found %d missions (from daemon)\n\n", total)
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSTATUS\tWORKFLOW\tSTARTED\tFINDINGS")
			for _, m := range missions {
				startTime := m.StartTime.Format("2006-01-02 15:04")
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n",
					m.ID, m.Status, m.WorkflowPath, startTime, m.FindingCount)
			}
			w.Flush()
			return nil
		}
	}

	// Fall back to local data
	fmt.Fprintln(os.Stderr, "[WARN] Daemon not running, showing local data only")

	// Build command context for local query
	cc, err := buildMissionCommandContext(cmd)
	if err != nil {
		return err
	}
	defer internal.CloseWithLog(cc, nil, "gRPC connection")

	// Call core function
	result, err := core.MissionList(cc, missionStatusFilter)
	if err != nil {
		return err
	}

	// Handle errors from core
	if result.Error != nil {
		return internal.WrapError(internal.ExitError, "mission list failed", result.Error)
	}

	// Format output
	return formatMissionListOutput(cmd, result)
}

// runMissionShow shows detailed mission information
func runMissionShow(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	missionName := args[0]

	// Try daemon first (optional - fall back to local if unavailable)
	client := dclient.OptionalDaemon(ctx)
	if client != nil {
		defer client.Close()

		// Query mission from daemon using name pattern filter
		missions, _, err := client.ListMissions(ctx, false, "", missionName, 10, 0)
		if err == nil && len(missions) > 0 {
			// Find exact match by ID or workflow path containing the name
			for _, m := range missions {
				if strings.Contains(m.ID, missionName) || strings.Contains(m.WorkflowPath, missionName) {
					// Display mission details from daemon
					fmt.Printf("Mission: %s\n", m.ID)
					fmt.Printf("Status: %s\n", m.Status)
					fmt.Printf("Workflow: %s\n", m.WorkflowPath)
					fmt.Printf("Started: %s\n", m.StartTime.Format("2006-01-02 15:04:05"))
					if !m.EndTime.IsZero() {
						fmt.Printf("Ended: %s\n", m.EndTime.Format("2006-01-02 15:04:05"))
					}
					fmt.Printf("Findings: %d\n", m.FindingCount)
					return nil
				}
			}
		}
	}

	// Fall back to local data
	fmt.Fprintln(os.Stderr, "[WARN] Daemon not running, showing local data only")

	// Build command context for local query
	cc, err := buildMissionCommandContext(cmd)
	if err != nil {
		return err
	}
	defer internal.CloseWithLog(cc, nil, "gRPC connection")

	// Call core function
	result, err := core.MissionShow(cc, missionName)
	if err != nil {
		return err
	}

	// Handle errors from core
	if result.Error != nil {
		return internal.WrapError(internal.ExitError, "mission show failed", result.Error)
	}

	// Format output
	return formatMissionShowOutput(cmd, result)
}

// runMissionRun creates and runs a new mission from workflow YAML
func runMissionRun(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Parse global flags for verbose logging
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return internal.WrapError(internal.ExitConfigError, "failed to parse flags", err)
	}

	// Setup verbose logging infrastructure
	jsonOutput := flags.OutputFormat == "json"
	_, cleanup := internal.SetupVerbose(cmd, flags.VerbosityLevel(), jsonOutput)
	defer cleanup()

	verbose := flags.IsVerbose()

	// Verbose output
	if verbose {
		fmt.Printf("Loading workflow from %s\n", missionWorkflowFile)
	}

	// Mission execution requires daemon
	client, err := dclient.RequireDaemon(ctx)
	if err != nil {
		return internal.WrapError(internal.ExitError, "mission execution requires daemon", err)
	}
	defer client.Close()

	// Start mission execution via daemon
	eventChan, err := client.RunMission(ctx, missionWorkflowFile)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to start mission", err)
	}

	// Stream and display events as they occur
	var missionID string
	nodeEvents := 0

	for event := range eventChan {
		switch event.Type {
		case "mission.started":
			missionID = getEventData(event.Data, "mission_id")
			fmt.Printf("Mission %s started\n", missionID)

		case "node.started":
			nodeEvents++
			if verbose {
				nodeID := getEventData(event.Data, "node_id")
				fmt.Printf("  [%s] Node %s started\n", event.Timestamp.Format("15:04:05"), nodeID)
			}

		case "node.completed":
			if verbose {
				nodeID := getEventData(event.Data, "node_id")
				fmt.Printf("  [%s] Node %s completed\n", event.Timestamp.Format("15:04:05"), nodeID)
			}

		case "mission.completed":
			fmt.Printf("Mission %s completed successfully\n", missionID)

		case "mission.failed":
			errorMsg := event.Message
			if errorMsg == "" {
				errorMsg = getEventData(event.Data, "error")
			}
			fmt.Printf("Mission %s failed: %s\n", missionID, errorMsg)
			return internal.WrapError(internal.ExitError, "mission failed", fmt.Errorf("%s", errorMsg))

		default:
			if verbose {
				fmt.Printf("  [%s] %s: %s\n", event.Timestamp.Format("15:04:05"), event.Type, event.Message)
			}
		}
	}

	return nil
}

// getEventData extracts a string value from event data map
func getEventData(data map[string]interface{}, key string) string {
	if data == nil {
		return ""
	}
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// runMissionResume resumes a paused mission
func runMissionResume(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	missionID := args[0]

	// Parse global flags for verbose logging
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return internal.WrapError(internal.ExitConfigError, "failed to parse flags", err)
	}

	// Setup verbose logging infrastructure
	jsonOutput := flags.OutputFormat == "json"
	_, cleanup := internal.SetupVerbose(cmd, flags.VerbosityLevel(), jsonOutput)
	defer cleanup()

	verbose := flags.IsVerbose()

	// Resuming a mission requires daemon
	client, err := dclient.RequireDaemon(ctx)
	if err != nil {
		return internal.WrapError(internal.ExitError, "mission resume requires daemon", err)
	}
	defer client.Close()

	// Resume the mission via daemon with optional checkpoint
	if verbose && missionFromCheckpoint != "" {
		fmt.Printf("Resuming mission %s from checkpoint %s\n", missionID, missionFromCheckpoint)
	} else if verbose {
		fmt.Printf("Resuming mission %s from last checkpoint\n", missionID)
	}

	eventChan, err := client.ResumeMission(ctx, missionID, missionFromCheckpoint)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to resume mission", err)
	}

	// Stream and display events as they occur
	nodeEvents := 0
	for event := range eventChan {
		switch event.Type {
		case "mission.resumed":
			fmt.Printf("Mission %s resumed\n", missionID)

		case "node.started":
			nodeEvents++
			if verbose {
				nodeID := getEventData(event.Data, "node_id")
				fmt.Printf("  [%s] Node %s started\n", event.Timestamp.Format("15:04:05"), nodeID)
			}

		case "node.completed":
			if verbose {
				nodeID := getEventData(event.Data, "node_id")
				fmt.Printf("  [%s] Node %s completed\n", event.Timestamp.Format("15:04:05"), nodeID)
			}

		case "mission.completed":
			fmt.Printf("Mission %s completed successfully\n", missionID)

		case "mission.failed":
			errorMsg := event.Message
			if errorMsg == "" {
				errorMsg = getEventData(event.Data, "error")
			}
			fmt.Printf("Mission %s failed: %s\n", missionID, errorMsg)
			return internal.WrapError(internal.ExitError, "mission failed", fmt.Errorf("%s", errorMsg))

		default:
			if verbose {
				fmt.Printf("  [%s] %s: %s\n", event.Timestamp.Format("15:04:05"), event.Type, event.Message)
			}
		}
	}

	return nil
}

// runMissionStop stops a running mission
func runMissionStop(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	missionName := args[0]

	// Stopping a mission requires daemon
	client, err := dclient.RequireDaemon(ctx)
	if err != nil {
		return internal.WrapError(internal.ExitError, "mission stop requires daemon", err)
	}
	defer client.Close()

	// First, find the mission ID from the name
	missions, _, err := client.ListMissions(ctx, true, "", missionName, 10, 0)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to query missions", err)
	}

	// Find matching mission
	var missionID string
	for _, m := range missions {
		if strings.Contains(m.ID, missionName) || strings.Contains(m.WorkflowPath, missionName) {
			missionID = m.ID
			break
		}
	}

	if missionID == "" {
		return internal.WrapError(internal.ExitError, "mission not found or not running", fmt.Errorf("no running mission matches '%s'", missionName))
	}

	// Stop the mission via daemon
	force := false // Could add a --force flag in the future
	err = client.StopMission(ctx, missionID, force)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to stop mission", err)
	}

	fmt.Printf("Mission %s stopped successfully\n", missionID)
	return nil
}

// runMissionDelete deletes a mission
func runMissionDelete(cmd *cobra.Command, args []string) error {
	missionName := args[0]

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

	// Build command context
	cc, err := buildMissionCommandContext(cmd)
	if err != nil {
		return err
	}
	defer internal.CloseWithLog(cc, nil, "gRPC connection")

	// Call core function
	result, err := core.MissionDelete(cc, missionName, missionForceDelete)
	if err != nil {
		return err
	}

	// Handle errors from core
	if result.Error != nil {
		return internal.WrapError(internal.ExitError, "mission delete failed", result.Error)
	}

	// Format output
	return formatMissionActionOutput(cmd, result)
}

// runMissionPause pauses a running mission
func runMissionPause(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	missionID := args[0]

	// Pausing a mission requires daemon
	client, err := dclient.RequireDaemon(ctx)
	if err != nil {
		return internal.WrapError(internal.ExitError, "mission pause requires daemon", err)
	}
	defer client.Close()

	// Pause the mission via daemon
	checkpointID, err := client.PauseMission(ctx, missionID, missionForcePause)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to pause mission", err)
	}

	// Display success message
	if missionForcePause {
		fmt.Printf("Mission %s paused immediately\n", missionID)
	} else {
		fmt.Printf("Mission %s paused gracefully\n", missionID)
	}
	if checkpointID != "" {
		fmt.Printf("Checkpoint: %s\n", checkpointID)
	}

	return nil
}

// runMissionHistory shows execution history for a mission name
func runMissionHistory(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	missionName := args[0]

	// Querying history requires daemon
	client, err := dclient.RequireDaemon(ctx)
	if err != nil {
		return internal.WrapError(internal.ExitError, "mission history requires daemon", err)
	}
	defer client.Close()

	// Get mission history from daemon
	runs, total, err := client.GetMissionHistory(ctx, missionName, 100, 0)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to get mission history", err)
	}

	// Display results
	if total == 0 {
		fmt.Printf("No execution history found for mission '%s'\n", missionName)
		return nil
	}

	fmt.Printf("Mission '%s' execution history (%d runs)\n\n", missionName, total)

	// Create table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RUN#\tMISSION ID\tSTATUS\tCREATED\tCOMPLETED\tFINDINGS")

	for _, run := range runs {
		createdStr := run.CreatedAt.Format("2006-01-02 15:04")
		completedStr := "-"
		if run.CompletedAt != nil {
			completedStr = run.CompletedAt.Format("2006-01-02 15:04")
		}

		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%d\n",
			run.RunNumber, run.MissionID, run.Status,
			createdStr, completedStr, run.FindingsCount)
	}

	w.Flush()
	return nil
}

// runMissionCheckpoints lists checkpoints for a mission
func runMissionCheckpoints(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	missionID := args[0]

	// Querying checkpoints requires daemon
	client, err := dclient.RequireDaemon(ctx)
	if err != nil {
		return internal.WrapError(internal.ExitError, "mission checkpoints requires daemon", err)
	}
	defer client.Close()

	// Get checkpoints from daemon
	checkpoints, err := client.GetMissionCheckpoints(ctx, missionID)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to get mission checkpoints", err)
	}

	// Display results
	if len(checkpoints) == 0 {
		fmt.Printf("No checkpoints found for mission '%s'\n", missionID)
		return nil
	}

	fmt.Printf("Checkpoints for mission '%s' (%d total)\n\n", missionID, len(checkpoints))

	// Create table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CHECKPOINT ID\tCREATED\tCOMPLETED NODES\tTOTAL NODES\tFINDINGS")

	for _, cp := range checkpoints {
		createdStr := cp.CreatedAt.Format("2006-01-02 15:04:05")
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\n",
			cp.CheckpointID, createdStr, cp.CompletedNodes,
			cp.TotalNodes, cp.FindingsCount)
	}

	w.Flush()
	return nil
}

// runMissionStatus displays mission status with checkpoint information
func runMissionStatus(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	missionID := args[0]

	// Querying status requires daemon
	client, err := dclient.RequireDaemon(ctx)
	if err != nil {
		return internal.WrapError(internal.ExitError, "mission status requires daemon", err)
	}
	defer client.Close()

	// Query mission details from daemon
	missions, _, err := client.ListMissions(ctx, false, "", missionID, 10, 0)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to query mission", err)
	}

	// Find the matching mission
	var mission *dclient.MissionInfo
	for _, m := range missions {
		if m.ID == missionID || strings.Contains(m.ID, missionID) {
			mission = &m
			break
		}
	}

	if mission == nil {
		return internal.WrapError(internal.ExitError, "mission not found", fmt.Errorf("no mission matches '%s'", missionID))
	}

	// Query checkpoints for this mission
	checkpoints, err := client.GetMissionCheckpoints(ctx, mission.ID)
	hasCheckpoint := err == nil && len(checkpoints) > 0

	// Determine if mission can be resumed
	canResume := mission.Status == "paused" && hasCheckpoint

	// Display mission status
	fmt.Printf("Mission: %s\n", mission.ID)
	fmt.Printf("Status: %s\n", mission.Status)
	fmt.Printf("Workflow: %s\n", mission.WorkflowPath)
	fmt.Printf("Progress: %d findings\n", mission.FindingCount)

	// Time information
	fmt.Printf("Started: %s\n", mission.StartTime.Format("2006-01-02 15:04:05"))
	if !mission.EndTime.IsZero() {
		fmt.Printf("Ended: %s\n", mission.EndTime.Format("2006-01-02 15:04:05"))
	}

	// Checkpoint information
	if hasCheckpoint {
		lastCheckpoint := checkpoints[0] // Checkpoints are sorted by creation time descending
		fmt.Printf("\nCheckpoint Available: Yes\n")
		fmt.Printf("Last Checkpoint: %s\n", lastCheckpoint.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Checkpoint Progress: %d/%d nodes completed\n",
			lastCheckpoint.CompletedNodes, lastCheckpoint.TotalNodes)
	} else {
		fmt.Printf("\nCheckpoint Available: No\n")
	}

	// Resume capability
	if canResume {
		fmt.Printf("Can Resume: Yes\n")
		fmt.Printf("\nTo resume: gibson mission resume %s\n", mission.ID)
	} else if mission.Status == "paused" {
		fmt.Printf("Can Resume: No (no checkpoint available)\n")
	} else if mission.Status == "running" {
		fmt.Printf("Can Resume: N/A (mission is running)\n")
	} else {
		fmt.Printf("Can Resume: No (mission is %s)\n", mission.Status)
	}

	return nil
}

// Output formatting functions

// formatMissionListOutput formats the mission list result
func formatMissionListOutput(cmd *cobra.Command, result *core.CommandResult) error {
	// Parse global flags
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return internal.WrapError(internal.ExitConfigError, "failed to parse flags", err)
	}

	// Create formatter
	outFormat := internal.FormatText
	if flags.OutputFormat == "json" {
		outFormat = internal.FormatJSON
	}
	formatter := internal.NewFormatter(outFormat, cmd.OutOrStdout())

	// Extract result data
	listResult, ok := result.Data.(*core.MissionListResult)
	if !ok {
		return fmt.Errorf("invalid result type for mission list")
	}

	if outFormat == internal.FormatJSON {
		return formatter.PrintJSON(map[string]interface{}{
			"missions": listResult.Missions,
			"count":    listResult.Count,
		})
	}

	// Text format
	if len(listResult.Missions) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No missions found")
		return nil
	}

	// Print table
	headers := []string{"Name", "Status", "Progress", "Findings", "Created", "Updated"}
	rows := make([][]string, 0, len(listResult.Missions))

	for _, m := range listResult.Missions {
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

// formatMissionShowOutput formats the mission show result
func formatMissionShowOutput(cmd *cobra.Command, result *core.CommandResult) error {
	// Parse global flags
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return internal.WrapError(internal.ExitConfigError, "failed to parse flags", err)
	}

	// Create formatter
	outFormat := internal.FormatText
	if flags.OutputFormat == "json" {
		outFormat = internal.FormatJSON
	}
	formatter := internal.NewFormatter(outFormat, cmd.OutOrStdout())

	// Extract mission data
	m, ok := result.Data.(*mission.Mission)
	if !ok {
		return fmt.Errorf("invalid result type for mission show")
	}

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

// formatMissionRunOutput formats the mission run result
func formatMissionRunOutput(cmd *cobra.Command, result *core.CommandResult) error {
	// Parse global flags
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return internal.WrapError(internal.ExitConfigError, "failed to parse flags", err)
	}

	// Create formatter
	outFormat := internal.FormatText
	if flags.OutputFormat == "json" {
		outFormat = internal.FormatJSON
	}
	formatter := internal.NewFormatter(outFormat, cmd.OutOrStdout())

	// Extract result data
	runResult, ok := result.Data.(*core.MissionRunResult)
	if !ok {
		return fmt.Errorf("invalid result type for mission run")
	}

	if outFormat == internal.FormatJSON {
		return formatter.PrintJSON(map[string]interface{}{
			"mission": runResult.Mission,
			"status":  runResult.Status,
		})
	}

	// Print success message
	fmt.Printf("Mission '%s' started successfully\n", runResult.Mission.Name)
	fmt.Printf("Mission ID: %s\n", runResult.Mission.ID)
	fmt.Printf("Workflow: %s (%d nodes)\n", runResult.Workflow.Name, runResult.NodesCount)

	return nil
}

// formatMissionActionOutput formats the output for mission actions (resume, stop, delete)
func formatMissionActionOutput(cmd *cobra.Command, result *core.CommandResult) error {
	// Parse global flags
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return internal.WrapError(internal.ExitConfigError, "failed to parse flags", err)
	}

	// Create formatter
	outFormat := internal.FormatText
	if flags.OutputFormat == "json" {
		outFormat = internal.FormatJSON
	}
	formatter := internal.NewFormatter(outFormat, cmd.OutOrStdout())

	if outFormat == internal.FormatJSON {
		return formatter.PrintJSON(result.Data)
	}

	// Print success message
	fmt.Println(result.Message)
	return nil
}

// Helper functions

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
