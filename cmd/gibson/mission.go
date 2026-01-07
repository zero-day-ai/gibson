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

	return &core.CommandContext{
		Ctx:          ctx,
		DB:           db,
		HomeDir:      homeDir,
		MissionStore: missionStore,
	}, nil
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
	// Build command context
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
	missionName := args[0]

	// Build command context
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
	// Parse global flags for verbose logging
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return internal.WrapError(internal.ExitConfigError, "failed to parse flags", err)
	}

	// Setup verbose logging infrastructure
	jsonOutput := flags.OutputFormat == "json"
	_, cleanup := internal.SetupVerbose(cmd, flags.VerbosityLevel(), jsonOutput)
	defer cleanup()

	// Verbose output
	if flags.IsVerbose() {
		fmt.Printf("Loading workflow from %s\n", missionWorkflowFile)
	}

	// Build command context
	cc, err := buildMissionCommandContext(cmd)
	if err != nil {
		return err
	}
	defer cc.Close()

	// TODO (Phase 6): When mission execution is fully implemented, integrate verbose logging here:
	// - Create orchestrator with verbose.WorkflowVerboseAdapter
	// - Set up MissionEventBridge for mission events
	// - Wrap harness factory with verbose.WrapHarnessFactory
	//
	// Example:
	//   ctx := cmd.Context()
	//   var opts *OrchestratorOptions
	//   if vw != nil {
	//       opts = &OrchestratorOptions{
	//           VerboseBus:   verbose.NewWorkflowVerboseAdapter(vw.Bus()),
	//           VerboseLevel: flags.VerbosityLevel(),
	//       }
	//   }
	//   bundle, err := createOrchestratorWithOptions(ctx, opts)
	//   ...
	//   bridge := verbose.NewMissionEventBridge(bundle.EventEmitter, vw.Bus())
	//   bridge.Start(ctx)
	//   defer bridge.Stop()

	// Call core function
	result, err := core.MissionRun(cc, missionWorkflowFile)
	if err != nil {
		return err
	}

	// Handle errors from core
	if result.Error != nil {
		return internal.WrapError(internal.ExitError, "mission run failed", result.Error)
	}

	// Verbose output
	if flags.IsVerbose() {
		if runResult, ok := result.Data.(*core.MissionRunResult); ok {
			fmt.Printf("Workflow loaded: %s (%d nodes)\n", runResult.Workflow.Name, runResult.NodesCount)
		}
	}

	// Format output
	return formatMissionRunOutput(cmd, result)
}

// runMissionResume resumes a paused mission
func runMissionResume(cmd *cobra.Command, args []string) error {
	missionName := args[0]

	// Build command context
	cc, err := buildMissionCommandContext(cmd)
	if err != nil {
		return err
	}
	defer internal.CloseWithLog(cc, nil, "gRPC connection")

	// Call core function
	result, err := core.MissionResume(cc, missionName)
	if err != nil {
		return err
	}

	// Handle errors from core
	if result.Error != nil {
		return internal.WrapError(internal.ExitError, "mission resume failed", result.Error)
	}

	// Format output
	return formatMissionActionOutput(cmd, result)
}

// runMissionStop stops a running mission
func runMissionStop(cmd *cobra.Command, args []string) error {
	missionName := args[0]

	// Build command context
	cc, err := buildMissionCommandContext(cmd)
	if err != nil {
		return err
	}
	defer internal.CloseWithLog(cc, nil, "gRPC connection")

	// Call core function
	result, err := core.MissionStop(cc, missionName)
	if err != nil {
		return err
	}

	// Handle errors from core
	if result.Error != nil {
		return internal.WrapError(internal.ExitError, "mission stop failed", result.Error)
	}

	// Format output
	return formatMissionActionOutput(cmd, result)
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
