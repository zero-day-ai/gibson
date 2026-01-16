package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/internal"
	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/graphrag/queries"
	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
	"gopkg.in/yaml.v3"
)

// workflowCmd is the root command for workflow operations
var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage and monitor workflow executions",
	Long: `Parse YAML workflows, monitor mission status, export snapshots, and compare
workflow evolution during execution.

The workflow commands enable you to:
  - Parse and validate YAML workflow definitions
  - Load workflows into the graph database
  - Monitor mission execution status in real-time
  - Export workflow snapshots with execution history
  - Compare original YAML to evolved graph state`,
}

// parseCmd parses a YAML workflow file
var parseCmd = &cobra.Command{
	Use:   "parse <file.yaml>",
	Short: "Parse workflow YAML into graph",
	Long: `Parse a workflow YAML file, validate its structure, and optionally load it into
the graph database as a mission.

The parser performs comprehensive validation including:
  - YAML syntax validation
  - Required field checks
  - Node type validation
  - Dependency graph validation
  - DAG acyclicity verification

With --dry-run, the workflow is parsed and validated without loading into the database.
This is useful for testing workflow definitions before execution.`,
	Example: `  # Parse and validate workflow without loading
  gibson workflow parse recon.yaml --dry-run

  # Parse and load workflow into graph
  gibson workflow parse recon.yaml --graph-url bolt://localhost:7687

  # Parse with custom graph URL
  gibson workflow parse attack.yaml --graph-url bolt://neo4j.example.com:7687`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkflowParse,
}

// workflowStatusCmd shows mission execution status
var workflowStatusCmd = &cobra.Command{
	Use:   "status <mission-id>",
	Short: "Show mission status",
	Long: `Display the current execution status of a mission including:
  - Node counts by status (pending, ready, running, completed, failed, skipped)
  - Overall mission progress percentage
  - Recent orchestrator decisions with reasoning
  - Execution timeline

Use --watch to continuously monitor mission status with automatic refresh every 2 seconds.
This is useful for tracking long-running missions in real-time.`,
	Example: `  # Show mission status once
  gibson workflow status mission-20260115-143045-abc123

  # Watch mission status with auto-refresh
  gibson workflow status mission-20260115-143045-abc123 --watch

  # Query custom graph instance
  gibson workflow status mission-20260115-143045-abc123 --graph-url bolt://custom:7687`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkflowStatus,
}

// snapshotCmd exports a workflow snapshot
var snapshotCmd = &cobra.Command{
	Use:   "snapshot <mission-id>",
	Short: "Export workflow snapshot",
	Long: `Export a complete snapshot of the workflow state including:
  - Original workflow definition
  - Current node statuses
  - Execution history (with --include-history)
  - Orchestrator decisions
  - Task spawning details

The snapshot can be exported as YAML or JSON format and is useful for:
  - Workflow debugging and analysis
  - Preserving mission state for auditing
  - Understanding workflow evolution
  - Sharing execution results`,
	Example: `  # Export snapshot to stdout as YAML
  gibson workflow snapshot mission-20260115-143045-abc123

  # Export snapshot to file as JSON
  gibson workflow snapshot mission-20260115-143045-abc123 --output snapshot.json --format json

  # Include full execution history
  gibson workflow snapshot mission-20260115-143045-abc123 --include-history`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkflowSnapshot,
}

// diffCmd compares original YAML to graph state
var diffCmd = &cobra.Command{
	Use:   "diff <original.yaml> <mission-id>",
	Short: "Compare original to evolved workflow",
	Long: `Compare the original workflow YAML definition to the current graph state, showing:
  - Additions: Dynamically spawned nodes created during execution
  - Modifications: Status changes and execution results
  - Skips: Nodes that were skipped based on conditions
  - Reasoning: Orchestrator decisions explaining changes

This command is essential for understanding how the autonomous orchestrator
evolved the workflow during execution, including which nodes were dynamically
added and why certain paths were taken.`,
	Example: `  # Compare original workflow to current graph state
  gibson workflow diff recon.yaml mission-20260115-143045-abc123

  # Compare with custom graph URL
  gibson workflow diff recon.yaml mission-20260115-143045-abc123 --graph-url bolt://custom:7687`,
	Args: cobra.ExactArgs(2),
	RunE: runWorkflowDiff,
}

// Flags
var (
	workflowDryRun        bool
	workflowGraphURL      string
	workflowWatch         bool
	workflowOutput        string
	workflowIncludeHist   bool
	workflowFormat        string
	workflowWatchInterval int
)

func init() {
	// Register workflow command
	rootCmd.AddCommand(workflowCmd)

	// Add subcommands
	workflowCmd.AddCommand(parseCmd)
	workflowCmd.AddCommand(workflowStatusCmd)
	workflowCmd.AddCommand(snapshotCmd)
	workflowCmd.AddCommand(diffCmd)

	// Parse command flags
	parseCmd.Flags().BoolVar(&workflowDryRun, "dry-run", false, "Parse and validate without loading into graph")
	parseCmd.Flags().StringVar(&workflowGraphURL, "graph-url", "bolt://localhost:7687", "Neo4j graph database URL")

	// Status command flags
	workflowStatusCmd.Flags().BoolVar(&workflowWatch, "watch", false, "Watch mode: refresh status every 2 seconds")
	workflowStatusCmd.Flags().IntVar(&workflowWatchInterval, "interval", 2, "Watch mode refresh interval in seconds")
	workflowStatusCmd.Flags().StringVar(&workflowGraphURL, "graph-url", "bolt://localhost:7687", "Neo4j graph database URL")

	// Snapshot command flags
	snapshotCmd.Flags().StringVarP(&workflowOutput, "output", "o", "", "Output file (default: stdout)")
	snapshotCmd.Flags().BoolVar(&workflowIncludeHist, "include-history", false, "Include full execution history")
	snapshotCmd.Flags().StringVar(&workflowFormat, "format", "yaml", "Output format: yaml or json")
	snapshotCmd.Flags().StringVar(&workflowGraphURL, "graph-url", "bolt://localhost:7687", "Neo4j graph database URL")

	// Diff command flags
	diffCmd.Flags().StringVar(&workflowGraphURL, "graph-url", "bolt://localhost:7687", "Neo4j graph database URL")
}

// runWorkflowParse implements the parse command
func runWorkflowParse(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	filePath := args[0]

	// Parse YAML file
	cmd.Printf("Parsing workflow: %s\n", filePath)
	parsed, err := workflow.ParseWorkflowYAML(filePath)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to parse workflow", err)
	}

	// Validate workflow structure
	cmd.Printf("Validating workflow structure...\n")
	validator := workflow.NewValidator(nil) // No registry validation for CLI
	validationErrors := validator.ValidateWorkflow(parsed.ToWorkflow())
	if len(validationErrors) > 0 {
		cmd.Printf("\nValidation errors found:\n")
		for i, err := range validationErrors {
			cmd.Printf("  %d. %s\n", i+1, err.Error())
		}
		return internal.WrapError(internal.ExitError, "workflow validation failed", validationErrors)
	}

	cmd.Printf("✓ Workflow validation successful\n")
	cmd.Printf("  Name: %s\n", parsed.Name)
	cmd.Printf("  Nodes: %d\n", len(parsed.Nodes))
	cmd.Printf("  Entry points: %d\n", len(parsed.EntryPoints))
	cmd.Printf("  Exit points: %d\n", len(parsed.ExitPoints))

	// Stop here if dry-run
	if workflowDryRun {
		cmd.Printf("\nDry-run complete (not loaded into graph)\n")
		return nil
	}

	// Load into graph
	cmd.Printf("\nLoading workflow into graph...\n")
	graphClient, err := connectToGraph(ctx, workflowGraphURL)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to connect to graph", err)
	}
	defer graphClient.Close(ctx)

	loader := workflow.NewGraphLoader(graphClient)
	missionID, err := loader.LoadParsedWorkflow(ctx, parsed)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to load workflow into graph", err)
	}

	cmd.Printf("✓ Workflow loaded successfully\n")
	cmd.Printf("\nMission ID: %s\n", missionID)
	cmd.Printf("\nUse 'gibson workflow status %s' to monitor execution\n", missionID)

	return nil
}

// runWorkflowStatus implements the status command
func runWorkflowStatus(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	missionIDStr := args[0]

	// Parse mission ID
	missionID, err := types.ParseID(missionIDStr)
	if err != nil {
		return internal.WrapError(internal.ExitError, "invalid mission ID", err)
	}

	// Connect to graph
	graphClient, err := connectToGraph(ctx, workflowGraphURL)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to connect to graph", err)
	}
	defer graphClient.Close(ctx)

	mq := queries.NewMissionQueries(graphClient)

	// Watch mode or single display
	if workflowWatch {
		return watchMissionStatus(ctx, cmd, mq, missionID)
	}

	return displayMissionStatus(ctx, cmd, mq, missionID)
}

// displayMissionStatus shows mission status once
func displayMissionStatus(ctx context.Context, cmd *cobra.Command, mq *queries.MissionQueries, missionID types.ID) error {
	// Get mission details
	mission, err := mq.GetMission(ctx, missionID)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to get mission", err)
	}

	// Get nodes grouped by status
	nodes, err := mq.GetMissionNodes(ctx, missionID)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to get mission nodes", err)
	}

	// Get recent decisions
	decisions, err := mq.GetMissionDecisions(ctx, missionID)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to get decisions", err)
	}

	// Get mission stats
	stats, err := mq.GetMissionStats(ctx, missionID)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to get mission stats", err)
	}

	// Display status
	cmd.Printf("\nMission Status: %s\n", mission.Name)
	cmd.Printf("ID: %s\n", mission.ID)
	cmd.Printf("Status: %s\n", mission.Status)
	cmd.Printf("\n")

	// Node status summary
	statusCounts := countNodesByStatus(nodes)
	total := len(nodes)
	completed := statusCounts[schema.WorkflowNodeStatusCompleted]
	progress := 0.0
	if total > 0 {
		progress = float64(completed) / float64(total) * 100
	}

	cmd.Printf("Progress: %d/%d nodes (%.1f%%)\n\n", completed, total, progress)

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STATUS\tCOUNT")
	fmt.Fprintf(tw, "Pending\t%d\n", statusCounts[schema.WorkflowNodeStatusPending])
	fmt.Fprintf(tw, "Ready\t%d\n", statusCounts[schema.WorkflowNodeStatusReady])
	fmt.Fprintf(tw, "Running\t%d\n", statusCounts[schema.WorkflowNodeStatusRunning])
	fmt.Fprintf(tw, "Completed\t%d\n", statusCounts[schema.WorkflowNodeStatusCompleted])
	fmt.Fprintf(tw, "Failed\t%d\n", statusCounts[schema.WorkflowNodeStatusFailed])
	fmt.Fprintf(tw, "Skipped\t%d\n", statusCounts[schema.WorkflowNodeStatusSkipped])
	tw.Flush()

	// Recent decisions (last 5)
	if len(decisions) > 0 {
		cmd.Printf("\nRecent Decisions:\n\n")
		displayCount := 5
		if len(decisions) < displayCount {
			displayCount = len(decisions)
		}

		for i := len(decisions) - displayCount; i < len(decisions); i++ {
			d := decisions[i]
			cmd.Printf("Iteration %d: %s\n", d.Iteration, d.Action)
			cmd.Printf("  Reasoning: %s\n", truncateString(d.Reasoning, 80))
			cmd.Printf("  Confidence: %.2f\n", d.Confidence)
			if d.TargetNodeID != "" {
				cmd.Printf("  Target: %s\n", d.TargetNodeID)
			}
			cmd.Printf("\n")
		}
	}

	// Execution stats
	cmd.Printf("Execution Statistics:\n")
	cmd.Printf("  Total Decisions: %d\n", stats.TotalDecisions)
	cmd.Printf("  Total Executions: %d\n", stats.TotalExecutions)

	if !stats.StartTime.IsZero() {
		cmd.Printf("  Started: %s\n", stats.StartTime.Format("2006-01-02 15:04:05"))
		if !stats.EndTime.IsZero() {
			duration := stats.EndTime.Sub(stats.StartTime)
			cmd.Printf("  Completed: %s (duration: %s)\n", stats.EndTime.Format("2006-01-02 15:04:05"), duration)
		} else {
			duration := time.Since(stats.StartTime)
			cmd.Printf("  Running for: %s\n", duration)
		}
	}

	return nil
}

// watchMissionStatus continuously displays mission status
func watchMissionStatus(ctx context.Context, cmd *cobra.Command, mq *queries.MissionQueries, missionID types.ID) error {
	ticker := time.NewTicker(time.Duration(workflowWatchInterval) * time.Second)
	defer ticker.Stop()

	for {
		// Clear screen (ANSI escape code)
		cmd.Print("\033[2J\033[H")

		// Display status
		if err := displayMissionStatus(ctx, cmd, mq, missionID); err != nil {
			return err
		}

		cmd.Printf("\n[Refreshing every %d seconds. Press Ctrl+C to exit]\n", workflowWatchInterval)

		// Wait for next tick or context cancellation
		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			return nil
		}
	}
}

// runWorkflowSnapshot implements the snapshot command
func runWorkflowSnapshot(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	missionIDStr := args[0]

	// Parse mission ID
	missionID, err := types.ParseID(missionIDStr)
	if err != nil {
		return internal.WrapError(internal.ExitError, "invalid mission ID", err)
	}

	// Connect to graph
	graphClient, err := connectToGraph(ctx, workflowGraphURL)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to connect to graph", err)
	}
	defer graphClient.Close(ctx)

	mq := queries.NewMissionQueries(graphClient)

	// Build snapshot
	snapshot, err := buildWorkflowSnapshot(ctx, mq, missionID, workflowIncludeHist)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to build snapshot", err)
	}

	// Serialize snapshot
	var output []byte
	if workflowFormat == "json" {
		output, err = json.MarshalIndent(snapshot, "", "  ")
	} else {
		output, err = yaml.Marshal(snapshot)
	}

	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to serialize snapshot", err)
	}

	// Write output
	if workflowOutput == "" {
		cmd.Print(string(output))
	} else {
		if err := os.WriteFile(workflowOutput, output, 0644); err != nil {
			return internal.WrapError(internal.ExitError, "failed to write output file", err)
		}
		cmd.Printf("Snapshot written to: %s\n", workflowOutput)
	}

	return nil
}

// runWorkflowDiff implements the diff command
func runWorkflowDiff(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	originalFile := args[0]
	missionIDStr := args[1]

	// Parse mission ID
	missionID, err := types.ParseID(missionIDStr)
	if err != nil {
		return internal.WrapError(internal.ExitError, "invalid mission ID", err)
	}

	// Parse original workflow
	cmd.Printf("Loading original workflow: %s\n", originalFile)
	original, err := workflow.ParseWorkflowYAML(originalFile)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to parse original workflow", err)
	}

	// Connect to graph
	graphClient, err := connectToGraph(ctx, workflowGraphURL)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to connect to graph", err)
	}
	defer graphClient.Close(ctx)

	mq := queries.NewMissionQueries(graphClient)

	// Get current graph state
	cmd.Printf("Loading graph state for mission: %s\n\n", missionIDStr)
	mission, err := mq.GetMission(ctx, missionID)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to get mission", err)
	}

	nodes, err := mq.GetMissionNodes(ctx, missionID)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to get mission nodes", err)
	}

	decisions, err := mq.GetMissionDecisions(ctx, missionID)
	if err != nil {
		return internal.WrapError(internal.ExitError, "failed to get decisions", err)
	}

	// Build diff
	diff := buildWorkflowDiff(original, mission, nodes, decisions)

	// Display diff
	displayWorkflowDiff(cmd, diff)

	return nil
}

// Helper functions

// connectToGraph creates a Neo4j graph client
func connectToGraph(ctx context.Context, graphURL string) (graph.GraphClient, error) {
	// Start with defaults and override URI
	config := graph.DefaultConfig()
	config.URI = graphURL

	// Check for environment variable overrides
	if user := os.Getenv("NEO4J_USER"); user != "" {
		config.Username = user
	}
	if pass := os.Getenv("NEO4J_PASSWORD"); pass != "" {
		config.Password = pass
	}

	client, err := graph.NewNeo4jClient(config)
	if err != nil {
		return nil, err
	}

	if err := client.Connect(ctx); err != nil {
		return nil, err
	}
	return client, nil
}

// countNodesByStatus counts nodes grouped by status
func countNodesByStatus(nodes []*schema.WorkflowNode) map[schema.WorkflowNodeStatus]int {
	counts := make(map[schema.WorkflowNodeStatus]int)
	for _, node := range nodes {
		counts[node.Status]++
	}
	return counts
}

// WorkflowSnapshot represents a complete workflow snapshot
type WorkflowSnapshot struct {
	Mission   *MissionSnapshot   `json:"mission" yaml:"mission"`
	Nodes     []*NodeSnapshot    `json:"nodes" yaml:"nodes"`
	Decisions []*DecisionSummary `json:"decisions,omitempty" yaml:"decisions,omitempty"`
	Stats     *SnapshotStats     `json:"stats" yaml:"stats"`
}

// MissionSnapshot contains mission metadata
type MissionSnapshot struct {
	ID          string    `json:"id" yaml:"id"`
	Name        string    `json:"name" yaml:"name"`
	Description string    `json:"description" yaml:"description"`
	Status      string    `json:"status" yaml:"status"`
	CreatedAt   time.Time `json:"created_at" yaml:"created_at"`
	StartedAt   time.Time `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
}

// NodeSnapshot contains workflow node state
type NodeSnapshot struct {
	ID          string                 `json:"id" yaml:"id"`
	Name        string                 `json:"name" yaml:"name"`
	Type        string                 `json:"type" yaml:"type"`
	Status      string                 `json:"status" yaml:"status"`
	IsDynamic   bool                   `json:"is_dynamic" yaml:"is_dynamic"`
	SpawnedBy   string                 `json:"spawned_by,omitempty" yaml:"spawned_by,omitempty"`
	AgentName   string                 `json:"agent_name,omitempty" yaml:"agent_name,omitempty"`
	ToolName    string                 `json:"tool_name,omitempty" yaml:"tool_name,omitempty"`
	TaskConfig  map[string]interface{} `json:"task_config,omitempty" yaml:"task_config,omitempty"`
	Executions  int                    `json:"executions,omitempty" yaml:"executions,omitempty"`
	CreatedAt   time.Time              `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" yaml:"updated_at"`
}

// DecisionSummary contains orchestrator decision details
type DecisionSummary struct {
	Iteration    int       `json:"iteration" yaml:"iteration"`
	Action       string    `json:"action" yaml:"action"`
	Reasoning    string    `json:"reasoning" yaml:"reasoning"`
	Confidence   float64   `json:"confidence" yaml:"confidence"`
	TargetNodeID string    `json:"target_node_id,omitempty" yaml:"target_node_id,omitempty"`
	Timestamp    time.Time `json:"timestamp" yaml:"timestamp"`
}

// SnapshotStats contains execution statistics
type SnapshotStats struct {
	TotalNodes      int           `json:"total_nodes" yaml:"total_nodes"`
	CompletedNodes  int           `json:"completed_nodes" yaml:"completed_nodes"`
	FailedNodes     int           `json:"failed_nodes" yaml:"failed_nodes"`
	DynamicNodes    int           `json:"dynamic_nodes" yaml:"dynamic_nodes"`
	TotalDecisions  int           `json:"total_decisions" yaml:"total_decisions"`
	TotalExecutions int           `json:"total_executions" yaml:"total_executions"`
	Duration        time.Duration `json:"duration,omitempty" yaml:"duration,omitempty"`
}

// buildWorkflowSnapshot constructs a complete workflow snapshot
func buildWorkflowSnapshot(ctx context.Context, mq *queries.MissionQueries, missionID types.ID, includeHistory bool) (*WorkflowSnapshot, error) {
	// Get mission
	mission, err := mq.GetMission(ctx, missionID)
	if err != nil {
		return nil, err
	}

	// Get nodes
	nodes, err := mq.GetMissionNodes(ctx, missionID)
	if err != nil {
		return nil, err
	}

	// Get stats
	stats, err := mq.GetMissionStats(ctx, missionID)
	if err != nil {
		return nil, err
	}

	// Build snapshot
	snapshot := &WorkflowSnapshot{
		Mission: &MissionSnapshot{
			ID:          mission.ID.String(),
			Name:        mission.Name,
			Description: mission.Description,
			Status:      mission.Status.String(),
			CreatedAt:   mission.CreatedAt,
		},
		Nodes: make([]*NodeSnapshot, 0, len(nodes)),
		Stats: &SnapshotStats{
			TotalNodes:      stats.TotalNodes,
			CompletedNodes:  stats.CompletedNodes,
			FailedNodes:     stats.FailedNodes,
			TotalDecisions:  stats.TotalDecisions,
			TotalExecutions: stats.TotalExecutions,
		},
	}

	if mission.StartedAt != nil {
		snapshot.Mission.StartedAt = *mission.StartedAt
	}
	if mission.CompletedAt != nil {
		snapshot.Mission.CompletedAt = *mission.CompletedAt
		snapshot.Stats.Duration = mission.CompletedAt.Sub(*mission.StartedAt)
	}

	// Add nodes
	dynamicCount := 0
	for _, node := range nodes {
		if node.IsDynamic {
			dynamicCount++
		}

		nodeSnapshot := &NodeSnapshot{
			ID:         node.ID.String(),
			Name:       node.Name,
			Type:       node.Type.String(),
			Status:     node.Status.String(),
			IsDynamic:  node.IsDynamic,
			SpawnedBy:  node.SpawnedBy,
			AgentName:  node.AgentName,
			ToolName:   node.ToolName,
			TaskConfig: node.TaskConfig,
			CreatedAt:  node.CreatedAt,
			UpdatedAt:  node.UpdatedAt,
		}

		// Get execution count if including history
		if includeHistory {
			executions, err := mq.GetNodeExecutions(ctx, node.ID)
			if err == nil {
				nodeSnapshot.Executions = len(executions)
			}
		}

		snapshot.Nodes = append(snapshot.Nodes, nodeSnapshot)
	}

	snapshot.Stats.DynamicNodes = dynamicCount

	// Add decisions if including history
	if includeHistory {
		decisions, err := mq.GetMissionDecisions(ctx, missionID)
		if err == nil {
			snapshot.Decisions = make([]*DecisionSummary, 0, len(decisions))
			for _, d := range decisions {
				snapshot.Decisions = append(snapshot.Decisions, &DecisionSummary{
					Iteration:    d.Iteration,
					Action:       d.Action.String(),
					Reasoning:    d.Reasoning,
					Confidence:   d.Confidence,
					TargetNodeID: d.TargetNodeID,
					Timestamp:    d.Timestamp,
				})
			}
		}
	}

	return snapshot, nil
}

// WorkflowDiff represents differences between original and evolved workflow
type WorkflowDiff struct {
	Summary      *DiffSummary      `json:"summary"`
	Additions    []*NodeDiff       `json:"additions"`
	Modifications []*NodeDiff       `json:"modifications"`
	Skips        []*NodeDiff       `json:"skips"`
	Decisions    []*DecisionSummary `json:"decisions"`
}

// DiffSummary contains high-level diff statistics
type DiffSummary struct {
	OriginalNodes int `json:"original_nodes"`
	CurrentNodes  int `json:"current_nodes"`
	Added         int `json:"added"`
	Modified      int `json:"modified"`
	Skipped       int `json:"skipped"`
}

// NodeDiff represents a difference in a single node
type NodeDiff struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Status    string                 `json:"status"`
	IsDynamic bool                   `json:"is_dynamic"`
	SpawnedBy string                 `json:"spawned_by,omitempty"`
	Reasoning string                 `json:"reasoning,omitempty"`
	Changes   map[string]interface{} `json:"changes,omitempty"`
}

// buildWorkflowDiff compares original workflow to graph state
func buildWorkflowDiff(original *workflow.ParsedWorkflow, mission *schema.Mission, nodes []*schema.WorkflowNode, decisions []*schema.Decision) *WorkflowDiff {
	diff := &WorkflowDiff{
		Summary: &DiffSummary{
			OriginalNodes: len(original.Nodes),
			CurrentNodes:  len(nodes),
		},
		Additions:     []*NodeDiff{},
		Modifications: []*NodeDiff{},
		Skips:         []*NodeDiff{},
		Decisions:     []*DecisionSummary{},
	}

	// Build map of original node IDs
	originalIDs := make(map[string]bool)
	for id := range original.Nodes {
		originalIDs[id] = true
	}

	// Find additions, modifications, and skips
	for _, node := range nodes {
		nodeID := node.ID.String()

		// Check if this is a dynamically spawned node (addition)
		if node.IsDynamic {
			diff.Additions = append(diff.Additions, &NodeDiff{
				ID:        nodeID,
				Name:      node.Name,
				Type:      node.Type.String(),
				Status:    node.Status.String(),
				IsDynamic: true,
				SpawnedBy: node.SpawnedBy,
			})
			diff.Summary.Added++
			continue
		}

		// Check if node was skipped
		if node.Status == schema.WorkflowNodeStatusSkipped {
			diff.Skips = append(diff.Skips, &NodeDiff{
				ID:     nodeID,
				Name:   node.Name,
				Type:   node.Type.String(),
				Status: node.Status.String(),
			})
			diff.Summary.Skipped++
			continue
		}

		// Check if node status changed (modification)
		if node.Status != schema.WorkflowNodeStatusPending {
			diff.Modifications = append(diff.Modifications, &NodeDiff{
				ID:     nodeID,
				Name:   node.Name,
				Type:   node.Type.String(),
				Status: node.Status.String(),
				Changes: map[string]interface{}{
					"status": node.Status.String(),
				},
			})
			diff.Summary.Modified++
		}
	}

	// Add decision summaries
	for _, d := range decisions {
		diff.Decisions = append(diff.Decisions, &DecisionSummary{
			Iteration:    d.Iteration,
			Action:       d.Action.String(),
			Reasoning:    d.Reasoning,
			Confidence:   d.Confidence,
			TargetNodeID: d.TargetNodeID,
			Timestamp:    d.Timestamp,
		})
	}

	return diff
}

// displayWorkflowDiff displays the workflow diff in a readable format
func displayWorkflowDiff(cmd *cobra.Command, diff *WorkflowDiff) {
	cmd.Printf("\nWorkflow Diff Summary\n")
	cmd.Printf("====================\n\n")

	cmd.Printf("Original nodes: %d\n", diff.Summary.OriginalNodes)
	cmd.Printf("Current nodes:  %d\n", diff.Summary.CurrentNodes)
	cmd.Printf("Added:          %d (dynamically spawned)\n", diff.Summary.Added)
	cmd.Printf("Modified:       %d (status changes)\n", diff.Summary.Modified)
	cmd.Printf("Skipped:        %d (skipped by orchestrator)\n\n", diff.Summary.Skipped)

	// Show additions
	if len(diff.Additions) > 0 {
		cmd.Printf("Additions (Dynamically Spawned Nodes)\n")
		cmd.Printf("====================================\n\n")
		for _, node := range diff.Additions {
			cmd.Printf("+ %s (%s)\n", node.Name, node.ID)
			cmd.Printf("  Type: %s\n", node.Type)
			cmd.Printf("  Status: %s\n", node.Status)
			if node.SpawnedBy != "" {
				cmd.Printf("  Spawned by: %s\n", node.SpawnedBy)
			}
			cmd.Printf("\n")
		}
	}

	// Show modifications
	if len(diff.Modifications) > 0 {
		cmd.Printf("Modifications (Status Changes)\n")
		cmd.Printf("==============================\n\n")
		tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "NODE\tTYPE\tSTATUS")
		for _, node := range diff.Modifications {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", node.Name, node.Type, node.Status)
		}
		tw.Flush()
		cmd.Printf("\n")
	}

	// Show skips
	if len(diff.Skips) > 0 {
		cmd.Printf("Skipped Nodes\n")
		cmd.Printf("=============\n\n")
		for _, node := range diff.Skips {
			cmd.Printf("- %s (%s)\n", node.Name, node.ID)
			cmd.Printf("  Type: %s\n", node.Type)
			cmd.Printf("\n")
		}
	}

	// Show key decisions
	if len(diff.Decisions) > 0 {
		cmd.Printf("Key Orchestrator Decisions\n")
		cmd.Printf("=========================\n\n")
		displayCount := 10
		if len(diff.Decisions) < displayCount {
			displayCount = len(diff.Decisions)
		}

		for i := len(diff.Decisions) - displayCount; i < len(diff.Decisions); i++ {
			d := diff.Decisions[i]
			cmd.Printf("Iteration %d: %s\n", d.Iteration, d.Action)
			cmd.Printf("  Reasoning: %s\n", truncateString(d.Reasoning, 100))
			cmd.Printf("  Confidence: %.2f\n", d.Confidence)
			if d.TargetNodeID != "" {
				cmd.Printf("  Target: %s\n", d.TargetNodeID)
			}
			cmd.Printf("\n")
		}
	}

	// No changes message
	if diff.Summary.Added == 0 && diff.Summary.Modified == 0 && diff.Summary.Skipped == 0 {
		cmd.Printf("No differences found. Workflow executed as originally defined.\n")
	}
}
