package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/component"
	"github.com/zero-day-ai/gibson/cmd/gibson/internal"
	"github.com/zero-day-ai/gibson/internal/attack"
	"github.com/zero-day-ai/gibson/internal/daemon/client"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/llm/providers"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/payload"
	"github.com/zero-day-ai/gibson/internal/plugin"
	"github.com/zero-day-ai/gibson/internal/registry"
	"github.com/zero-day-ai/gibson/internal/tool"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/verbose"
	"github.com/zero-day-ai/gibson/internal/workflow"
	"go.opentelemetry.io/otel/trace"
)

// attackCmd represents the attack command
var attackCmd = &cobra.Command{
	Use:   "attack",
	Short: "Launch a quick single-agent attack against a target",
	Long: `Launch an attack against a target using a specified agent.
This command creates an ephemeral mission that is automatically persisted
if findings are discovered (unless --no-persist is set).

The attack command provides a quick way to test a target without creating
a full mission workflow. It runs a single agent with optional payload
filtering, execution constraints, and output formatting.

Examples:
  # Attack using a stored target
  gibson attack --target my-api --agent web-scanner

  # Attack with inline target definition
  gibson attack --type http_api --connection '{"url":"https://example.com"}' --agent web-scanner

  # Attack with specific goal and timeout
  gibson attack --target api-prod --agent prompt-injector \
    --goal "Find prompt injection vulnerabilities" --timeout 30m

  # Attack with payload filtering
  gibson attack --target test-app --agent sql-injector \
    --category injection --techniques T1059,T1190

  # Attack with output options
  gibson attack --target my-target --agent xss-scanner \
    --output json --verbose

  # Dry-run to validate configuration
  gibson attack --target my-api --agent web-scanner --dry-run

  # List available agents
  gibson attack --list-agents`,
	Args:              cobra.NoArgs,
	RunE:              runAttackCommand,
	ValidArgsFunction: nil,
}

// Attack command flags
var (
	// Target configuration
	attackTargetName     string // --target flag for stored target name/ID
	attackTargetType     string // --type flag for inline target type
	attackConnection     string // --connection flag for inline target connection JSON
	attackTargetProvider string
	attackHeaders        string
	attackCredential     string

	// Agent configuration
	attackAgent    string
	attackGoal     string
	attackMaxTurns int
	attackTimeout  string

	// Payload filtering
	attackPayloads   []string
	attackCategory   string
	attackTechniques []string

	// Execution constraints
	attackMaxFindings       int
	attackSeverityThreshold string
	attackRateLimit         int

	// Network options
	attackNoFollowRedirects bool
	attackInsecure          bool
	attackProxy             string

	// Persistence options
	attackPersist   bool
	attackNoPersist bool

	// Output options
	attackOutput  string
	attackVerbose bool
	attackQuiet   bool
	attackDryRun  bool

	// List agents flag
	attackListAgents bool
)

// Note: globalCallbackManager is declared in root.go and initialized during loadConfig.
// The attack command relies on the workflow engine to use this manager
// for harness registration and callback coordination during agent execution.

func init() {
	// Target configuration flags
	attackCmd.Flags().StringVar(&attackTargetName, "target", "", "Stored target name or ID (required unless using --type and --connection)")
	attackCmd.Flags().StringVar(&attackTargetType, "type", "", "Inline target type (http_api, kubernetes, smart_contract, etc.) - requires --connection")
	attackCmd.Flags().StringVar(&attackConnection, "connection", "", "Inline target connection parameters as JSON - requires --type")
	attackCmd.Flags().StringVar(&attackTargetProvider, "provider", "", "Target provider (openai, anthropic, custom, etc.)")
	attackCmd.Flags().StringVar(&attackHeaders, "headers", "", "Custom HTTP headers as JSON object (e.g., '{\"X-API-Key\":\"value\"}')")
	attackCmd.Flags().StringVar(&attackCredential, "credential", "", "Credential name or ID for authentication")

	// Agent configuration flags (--agent is required)
	attackCmd.Flags().StringVar(&attackAgent, "agent", "", "Agent name to execute (REQUIRED)")
	attackCmd.Flags().StringVar(&attackGoal, "goal", "", "Attack goal or objective description")
	attackCmd.Flags().IntVar(&attackMaxTurns, "max-turns", 20, "Maximum number of agent turns")
	attackCmd.Flags().StringVar(&attackTimeout, "timeout", "10m", "Attack timeout duration (e.g., 10m, 1h, 30s)")

	// Payload filtering flags
	attackCmd.Flags().StringSliceVar(&attackPayloads, "payloads", []string{}, "Filter to specific payload IDs (comma-separated)")
	attackCmd.Flags().StringVar(&attackCategory, "category", "", "Filter payloads by category (e.g., injection, evasion)")
	attackCmd.Flags().StringSliceVar(&attackTechniques, "techniques", []string{}, "Filter by MITRE technique IDs (comma-separated, e.g., T1059,T1190)")

	// Execution constraint flags
	attackCmd.Flags().IntVar(&attackMaxFindings, "max-findings", 0, "Stop after discovering N findings (0 = unlimited)")
	attackCmd.Flags().StringVar(&attackSeverityThreshold, "severity-threshold", "", "Minimum severity to report (low, medium, high, critical)")
	attackCmd.Flags().IntVar(&attackRateLimit, "rate-limit", 0, "Maximum requests per second (0 = unlimited)")

	// Network option flags
	attackCmd.Flags().BoolVar(&attackNoFollowRedirects, "no-follow-redirects", false, "Don't follow HTTP redirects")
	attackCmd.Flags().BoolVar(&attackInsecure, "insecure", false, "Skip TLS certificate verification (insecure)")
	attackCmd.Flags().StringVar(&attackProxy, "proxy", "", "HTTP/HTTPS proxy URL")

	// Persistence option flags
	attackCmd.Flags().BoolVar(&attackPersist, "persist", false, "Always persist mission and findings")
	attackCmd.Flags().BoolVar(&attackNoPersist, "no-persist", false, "Never persist, even if findings are discovered")

	// Output option flags
	attackCmd.Flags().StringVar(&attackOutput, "output", "text", "Output format (text, json, sarif)")
	attackCmd.Flags().BoolVarP(&attackVerbose, "verbose", "v", false, "Enable verbose output")
	attackCmd.Flags().BoolVarP(&attackQuiet, "quiet", "q", false, "Suppress non-essential output (show only findings)")
	attackCmd.Flags().BoolVar(&attackDryRun, "dry-run", false, "Validate configuration without executing attack")

	// List agents flag
	attackCmd.Flags().BoolVar(&attackListAgents, "list-agents", false, "List available agents and exit")
}

// runAttackCommand is the main entry point for the attack command
func runAttackCommand(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Setup verbose logging (Phase 5, Task 13)
	vw, cleanup := internal.SetupVerbose(cmd, globalFlags.VerbosityLevel(), globalFlags.OutputFormat == "json")
	defer cleanup()

	// Handle --list-agents subcommand (Task 8.2, Task 14)
	if attackListAgents {
		return runListAgents(cmd, ctx)
	}

	// Reject positional URL arguments with clear migration error
	if len(args) > 0 {
		return internal.NewCLIError(attack.ExitConfigError,
			"URL argument no longer supported. Use --target <name> or --type <type> --connection <json>\n\n"+
				"Examples:\n"+
				"  gibson attack --target my-api --agent web-scanner\n"+
				"  gibson attack --type http_api --connection '{\"url\":\"https://example.com\"}' --agent web-scanner")
	}

	// Validate target specification: require --target OR (--type AND --connection)
	hasStoredTarget := attackTargetName != ""
	hasInlineTarget := attackTargetType != "" && attackConnection != ""

	if !hasStoredTarget && !hasInlineTarget {
		return internal.NewCLIError(attack.ExitConfigError,
			"target specification required\n\n"+
				"Either:\n"+
				"  - Use --target <name> to reference a stored target, or\n"+
				"  - Use --type <type> --connection <json> to define an inline target\n\n"+
				"Examples:\n"+
				"  gibson attack --target my-api --agent web-scanner\n"+
				"  gibson attack --type http_api --connection '{\"url\":\"https://example.com\"}' --agent web-scanner")
	}

	if hasStoredTarget && hasInlineTarget {
		return internal.NewCLIError(attack.ExitConfigError,
			"cannot specify both --target and --type/--connection flags\n\n"+
				"Use either:\n"+
				"  --target <name> for stored target, or\n"+
				"  --type <type> --connection <json> for inline target")
	}

	// Validate inline target has both flags
	if (attackTargetType != "" && attackConnection == "") || (attackTargetType == "" && attackConnection != "") {
		return internal.NewCLIError(attack.ExitConfigError,
			"inline target requires both --type and --connection flags\n\n"+
				"Example:\n"+
				"  gibson attack --type http_api --connection '{\"url\":\"https://example.com\"}' --agent web-scanner")
	}

	// Task 15: Check for daemon client in context
	// If daemon is available, use it for attack execution
	if daemonClient := component.GetDaemonClient(ctx); daemonClient != nil {
		// Use daemon client for attack execution
		return runAttackViaDaemon(cmd, daemonClient)
	}

	// Fall back to local execution for standalone mode (when daemon not running)

	// Parse flags into AttackOptions (Task 8.3)
	opts, err := buildAttackOptions()
	if err != nil {
		return internal.WrapError(attack.ExitConfigError, "failed to build attack options", err)
	}

	// Validate options
	if err := opts.Validate(); err != nil {
		return internal.WrapError(attack.ExitConfigError, "invalid attack configuration", err)
	}

	// Handle dry-run mode (Task 8.4)
	if opts.DryRun {
		return runDryRun(cmd, opts)
	}

	// Set up signal handling for graceful cancellation (Task 8.5)
	ctx, cancel := setupSignalHandler(ctx)
	defer cancel()

	// Create output handler
	outputHandler := attack.NewOutputHandler(opts.OutputFormat, cmd.OutOrStdout(), opts.Verbose, opts.Quiet)

	// Create attack runner with dependencies
	runner, err := createAttackRunner(ctx, vw)
	if err != nil {
		return internal.WrapError(attack.ExitError, "failed to create attack runner", err)
	}

	// Notify start
	outputHandler.OnStart(opts)

	// Execute attack (Task 8.3)
	result, err := runner.Run(ctx, opts)
	if err != nil {
		outputHandler.OnError(err)
		return internal.WrapError(attack.ExitError, "attack execution failed", err)
	}

	// Stream findings as they are discovered
	for _, finding := range result.Findings {
		outputHandler.OnFinding(finding)
	}

	// Notify completion
	outputHandler.OnComplete(result)

	// Return with appropriate exit code (Task 8.3)
	exitCode := attack.ExitCodeFromResult(result)
	if exitCode != attack.ExitSuccess {
		os.Exit(exitCode)
	}

	return nil
}

// runAttackViaDaemon executes an attack using the daemon client (Task 15)
func runAttackViaDaemon(cmd *cobra.Command, daemonClient interface{}) error {
	ctx := cmd.Context()

	// Parse flags into AttackOptions
	opts, err := buildAttackOptions()
	if err != nil {
		return internal.WrapError(attack.ExitConfigError, "failed to build attack options", err)
	}

	// Validate options
	if err := opts.Validate(); err != nil {
		return internal.WrapError(attack.ExitConfigError, "invalid attack configuration", err)
	}

	// Handle dry-run mode
	if opts.DryRun {
		return runDryRun(cmd, opts)
	}

	// Type assert to the daemon client
	dclient, ok := daemonClient.(*client.Client)
	if !ok {
		return fmt.Errorf("invalid daemon client type")
	}

	// Set up signal handling for graceful cancellation
	ctx, cancel := setupSignalHandler(ctx)
	defer cancel()

	// Create output handler
	outputHandler := attack.NewOutputHandler(opts.OutputFormat, cmd.OutOrStdout(), opts.Verbose, opts.Quiet)

	// Notify start
	outputHandler.OnStart(opts)

	// Convert AttackOptions to client.AttackOptions
	clientOpts := client.AttackOptions{
		Target:     opts.TargetURL,
		AttackType: opts.AgentName,
		MaxDepth:   opts.MaxTurns,
		Timeout:    opts.Timeout,
	}

	// Execute attack via daemon and stream events
	eventChan, err := dclient.RunAttack(ctx, clientOpts)
	if err != nil {
		// Check if it's a "not implemented" error from daemon
		if strings.Contains(err.Error(), "not yet implemented") || strings.Contains(err.Error(), "Unimplemented") {
			outputHandler.OnError(fmt.Errorf("daemon-based attack execution not yet implemented, use local mode"))
			return internal.WrapError(attack.ExitError, "attack via daemon not yet implemented", err)
		}
		outputHandler.OnError(err)
		return internal.WrapError(attack.ExitError, "failed to start attack via daemon", err)
	}

	// Stream events from daemon
	findingCount := 0
	for event := range eventChan {
		if opts.Verbose {
			cmd.Printf("[%s] %s: %s\n", event.Timestamp.Format("15:04:05"), event.Type, event.Message)
		}

		// Check for finding events
		if event.Type == "finding" || event.Severity != "" {
			findingCount++
			// Note: In the future, we'll convert event.Data to a proper Finding struct
			// For now, just track counts
		}
	}

	// Create a result summary (simplified for now)
	result := &attack.AttackResult{
		// Note: We'll need to enhance the event stream to provide full result data
		// For now, just show a completion message
	}

	outputHandler.OnComplete(result)

	// Return with appropriate exit code
	exitCode := attack.ExitSuccess
	if findingCount > 0 {
		exitCode = attack.ExitWithFindings
	}

	if exitCode != attack.ExitSuccess {
		os.Exit(exitCode)
	}

	return nil
}

// buildAttackOptions constructs AttackOptions from command-line flags (Task 8.1, 8.3)
func buildAttackOptions() (*attack.AttackOptions, error) {
	opts := attack.NewAttackOptions()

	// Target configuration
	// For stored target: use TargetName
	if attackTargetName != "" {
		opts.TargetName = attackTargetName
	}

	// For inline target: store type and connection JSON (will be validated later)
	if attackTargetType != "" {
		opts.TargetType = types.TargetType(attackTargetType)
	}
	if attackConnection != "" {
		// Store connection JSON in TargetURL temporarily for backward compatibility
		// TODO: Add TargetConnection field to AttackOptions in future refactor
		opts.TargetURL = attackConnection
	}

	opts.TargetProvider = attackTargetProvider
	opts.Credential = attackCredential

	// Parse headers JSON if provided
	if attackHeaders != "" {
		headers := make(map[string]string)
		if err := json.Unmarshal([]byte(attackHeaders), &headers); err != nil {
			return nil, fmt.Errorf("invalid headers JSON: %w", err)
		}
		opts.TargetHeaders = headers
	}

	// Agent configuration
	opts.AgentName = attackAgent
	opts.Goal = attackGoal
	opts.MaxTurns = attackMaxTurns

	// Parse timeout
	if attackTimeout != "" {
		duration, err := time.ParseDuration(attackTimeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout duration: %w", err)
		}
		opts.Timeout = duration
	}

	// Payload filtering
	opts.PayloadIDs = attackPayloads
	opts.PayloadCategory = attackCategory
	opts.Techniques = attackTechniques

	// Execution constraints
	opts.MaxFindings = attackMaxFindings
	opts.SeverityThreshold = attackSeverityThreshold
	opts.RateLimit = attackRateLimit

	// Network options
	opts.FollowRedirects = !attackNoFollowRedirects
	opts.InsecureTLS = attackInsecure
	opts.ProxyURL = attackProxy

	// Persistence options
	opts.Persist = attackPersist
	opts.NoPersist = attackNoPersist

	// Output options
	opts.OutputFormat = attackOutput
	opts.Verbose = attackVerbose
	opts.Quiet = attackQuiet
	opts.DryRun = attackDryRun

	return opts, nil
}

// createAttackRunner creates an AttackRunner with all dependencies (Task 4.1, 4.2)
func createAttackRunner(ctx context.Context, vw *verbose.VerboseWriter) (attack.AttackRunner, error) {
	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return nil, fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Step 1: Create stores
	missionStore := mission.NewDBMissionStore(db)
	findingStore := finding.NewDBFindingStore(db)

	// Step 2: Get registry manager from context and create adapter
	regManager := component.GetRegistryManager(ctx)
	if regManager == nil {
		return nil, fmt.Errorf("registry not available (run 'gibson init' first)")
	}

	// Create registry adapter for component discovery
	registryAdapter := registry.NewRegistryAdapter(regManager.Registry())

	// Step 2.5: Create registries (tools and plugins still use legacy registries for now)
	toolRegistry := tool.NewToolRegistry()
	pluginRegistry := plugin.NewPluginRegistry()
	payloadRegistry := payload.NewPayloadRegistryWithDefaults(db)

	// Step 3: Create LLM components
	llmRegistry, slotManager, err := initializeLLMComponents()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LLM components: %w", err)
	}

	// Step 4: Create harness factory
	harnessConfig := harness.HarnessConfig{
		LLMRegistry:     llmRegistry,
		SlotManager:     slotManager,
		ToolRegistry:    toolRegistry,
		PluginRegistry:  pluginRegistry,
		RegistryAdapter: registryAdapter, // Use new etcd-based registry adapter
		FindingStore:    nil,             // Will be created per-harness if needed
		Logger:          slog.Default(),
		Tracer:          trace.NewNoopTracerProvider().Tracer("attack-runner"),
	}

	baseHarnessFactory, err := harness.NewDefaultHarnessFactory(harnessConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create harness factory: %w", err)
	}

	// Wrap harness factory with verbose events if verbose is enabled (Phase 5, Task 13)
	var harnessFactory harness.HarnessFactoryInterface = baseHarnessFactory
	if vw != nil {
		harnessFactory = verbose.WrapHarnessFactory(harnessFactory, vw.Bus(), globalFlags.VerbosityLevel())
	}

	// Step 5: Create workflow executor
	// NOTE (Task 7): When CallbackManager is available (Task 5), pass it to the workflow executor
	// via a WithCallbackManager option (to be implemented in Task 6).
	// This enables the workflow engine to register harnesses and provide callback endpoints
	// when executing external gRPC agents.
	workflowExecutor := workflow.NewWorkflowExecutor(
		workflow.WithLogger(slog.Default()),
		workflow.WithTracer(trace.NewNoopTracerProvider().Tracer("workflow")),
		// TODO (Task 6): workflow.WithCallbackManager(globalCallbackManager),
	)

	// Step 6: Create mission orchestrator
	orchestrator := mission.NewMissionOrchestrator(
		missionStore,
		mission.WithWorkflowExecutor(workflowExecutor),
		mission.WithHarnessFactory(harnessFactory),
	)

	// Step 7: Create attack runner options
	runnerOpts := []attack.RunnerOption{
		attack.WithLogger(slog.Default()),
		attack.WithTracer(trace.NewNoopTracerProvider().Tracer("attack-runner")),
	}

	// Add verbose bus if enabled (Phase 5, Task 14)
	if vw != nil {
		runnerOpts = append(runnerOpts, attack.WithVerboseBus(vw.Bus()))
	}

	// Step 8: Create and return attack runner
	runner := attack.NewAttackRunner(
		orchestrator,
		registryAdapter,
		payloadRegistry,
		missionStore,
		findingStore,
		runnerOpts...,
	)

	return runner, nil
}

// runListAgents lists all available agents (Task 8.2)
func runListAgents(cmd *cobra.Command, ctx context.Context) error {
	// Task 14: Check for daemon client first
	if daemonClient := component.GetDaemonClient(ctx); daemonClient != nil {
		return runListAgentsViaDaemon(cmd, ctx, daemonClient)
	}

	// Fall back to registry adapter for standalone mode (when daemon not running)
	regManager := component.GetRegistryManager(ctx)
	if regManager == nil {
		return fmt.Errorf("registry not available (run 'gibson init' first)")
	}

	// Create registry adapter for component discovery
	registryAdapter := registry.NewRegistryAdapter(regManager.Registry())
	defer registryAdapter.Close()

	// Query registry for all agents
	agentInfos, err := registryAdapter.ListAgents(ctx)
	if err != nil {
		return fmt.Errorf("failed to list agents: %w", err)
	}

	if attackOutput == "json" {
		// JSON output format
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(map[string]interface{}{
			"agents": agentInfos,
		})
	}

	// Text output format
	formatter := internal.NewTextFormatter(cmd.OutOrStdout())

	cmd.Println("Available Agents:")
	cmd.Println()

	if len(agentInfos) == 0 {
		cmd.Println("No agents registered.")
		cmd.Println()
		cmd.Println("To register an agent:")
		cmd.Println("  1. Build your agent using the SDK")
		cmd.Println("  2. Start it with 'agent serve --port <PORT>'")
		cmd.Println("  3. It will auto-register with the embedded etcd")
		return nil
	}

	// Build table rows from agent info
	headers := []string{"Name", "Version", "Instances", "Capabilities"}
	rows := make([][]string, 0, len(agentInfos))
	for _, info := range agentInfos {
		capabilities := strings.Join(info.Capabilities, ", ")
		if capabilities == "" {
			capabilities = "N/A"
		}
		rows = append(rows, []string{
			info.Name,
			info.Version,
			fmt.Sprintf("%d", info.Instances),
			capabilities,
		})
	}

	if err := formatter.PrintTable(headers, rows); err != nil {
		return err
	}

	cmd.Println()
	cmd.Printf("Total: %d agent(s) with %d instance(s)\n", len(agentInfos), sumInstances(agentInfos))

	return nil
}

// runListAgentsViaDaemon lists agents using the daemon client (Task 14)
func runListAgentsViaDaemon(cmd *cobra.Command, ctx context.Context, daemonClient interface{}) error {
	// Type assert to the daemon client interface
	// We use interface{} in the context to avoid circular dependencies
	type daemonClientInterface interface {
		ListAgents(ctx context.Context) ([]interface{}, error)
	}

	// Try to use the client's ListAgents method via reflection or type assertion
	// For now, we'll need to import the actual client type
	dclient, ok := daemonClient.(*client.Client)
	if !ok {
		// Fall back to local registry if type assertion fails
		return fmt.Errorf("invalid daemon client type")
	}

	// Query daemon for all agents
	agentInfos, err := dclient.ListAgents(ctx)
	if err != nil {
		return fmt.Errorf("failed to list agents from daemon: %w", err)
	}

	if attackOutput == "json" {
		// JSON output format - convert client.AgentInfo to map for JSON encoding
		type jsonAgent struct {
			Name        string `json:"name"`
			Version     string `json:"version"`
			Description string `json:"description"`
			Address     string `json:"address"`
			Status      string `json:"status"`
		}

		agents := make([]jsonAgent, len(agentInfos))
		for i, info := range agentInfos {
			agents[i] = jsonAgent{
				Name:        info.Name,
				Version:     info.Version,
				Description: info.Description,
				Address:     info.Address,
				Status:      info.Status,
			}
		}

		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(map[string]interface{}{
			"agents": agents,
		})
	}

	// Text output format
	formatter := internal.NewTextFormatter(cmd.OutOrStdout())

	cmd.Println("Available Agents:")
	cmd.Println()

	if len(agentInfos) == 0 {
		cmd.Println("No agents registered.")
		cmd.Println()
		cmd.Println("To register an agent:")
		cmd.Println("  1. Build your agent using the SDK")
		cmd.Println("  2. Start it with 'agent serve --port <PORT>'")
		cmd.Println("  3. It will auto-register with the embedded etcd")
		return nil
	}

	// Build table rows from agent info - convert client.AgentInfo format
	headers := []string{"Name", "Version", "Status", "Address"}
	rows := make([][]string, 0, len(agentInfos))
	for _, info := range agentInfos {
		status := info.Status
		if status == "" {
			status = "unknown"
		}
		rows = append(rows, []string{
			info.Name,
			info.Version,
			status,
			info.Address,
		})
	}

	if err := formatter.PrintTable(headers, rows); err != nil {
		return err
	}

	cmd.Println()
	cmd.Printf("Total: %d agent(s)\n", len(agentInfos))

	return nil
}

// sumInstances calculates total number of instances across all agents
func sumInstances(agents []registry.AgentInfo) int {
	total := 0
	for _, agent := range agents {
		total += agent.Instances
	}
	return total
}

// runDryRun validates configuration and displays what would be executed (Task 8.4)
func runDryRun(cmd *cobra.Command, opts *attack.AttackOptions) error {
	cmd.Println("Dry-run mode: Validating configuration...")
	cmd.Println()

	// Display resolved configuration
	cmd.Println("Attack Configuration:")
	cmd.Println(strings.Repeat("=", 60))
	cmd.Println()

	// Target configuration
	cmd.Printf("Target URL:      %s\n", opts.TargetURL)
	if opts.TargetType != "" {
		cmd.Printf("Target Type:     %s\n", opts.TargetType)
	}
	if opts.TargetProvider != "" {
		cmd.Printf("Provider:        %s\n", opts.TargetProvider)
	}
	if len(opts.TargetHeaders) > 0 {
		cmd.Printf("Custom Headers:  %d headers\n", len(opts.TargetHeaders))
	}
	if opts.Credential != "" {
		cmd.Printf("Credential:      %s\n", opts.Credential)
	}
	cmd.Println()

	// Agent configuration
	cmd.Printf("Agent:           %s\n", opts.AgentName)
	if opts.Goal != "" {
		cmd.Printf("Goal:            %s\n", opts.Goal)
	}
	cmd.Printf("Max Turns:       %d\n", opts.MaxTurns)
	if opts.Timeout > 0 {
		cmd.Printf("Timeout:         %s\n", opts.Timeout)
	}
	cmd.Println()

	// Payload filtering
	if len(opts.PayloadIDs) > 0 {
		cmd.Printf("Payload IDs:     %s\n", strings.Join(opts.PayloadIDs, ", "))
	}
	if opts.PayloadCategory != "" {
		cmd.Printf("Category:        %s\n", opts.PayloadCategory)
	}
	if len(opts.Techniques) > 0 {
		cmd.Printf("Techniques:      %s\n", strings.Join(opts.Techniques, ", "))
	}
	if len(opts.PayloadIDs) > 0 || opts.PayloadCategory != "" || len(opts.Techniques) > 0 {
		cmd.Println()
	}

	// Execution constraints
	if opts.MaxFindings > 0 {
		cmd.Printf("Max Findings:    %d\n", opts.MaxFindings)
	}
	if opts.SeverityThreshold != "" {
		cmd.Printf("Min Severity:    %s\n", opts.SeverityThreshold)
	}
	if opts.RateLimit > 0 {
		cmd.Printf("Rate Limit:      %d req/s\n", opts.RateLimit)
	}
	if opts.MaxFindings > 0 || opts.SeverityThreshold != "" || opts.RateLimit > 0 {
		cmd.Println()
	}

	// Network options
	cmd.Printf("Follow Redirects: %t\n", opts.FollowRedirects)
	if opts.InsecureTLS {
		cmd.Printf("TLS Verification: disabled (INSECURE)\n")
	}
	if opts.ProxyURL != "" {
		cmd.Printf("Proxy:           %s\n", opts.ProxyURL)
	}
	cmd.Println()

	// Persistence options
	if opts.Persist {
		cmd.Printf("Persistence:     always persist\n")
	} else if opts.NoPersist {
		cmd.Printf("Persistence:     never persist\n")
	} else {
		cmd.Printf("Persistence:     auto-persist on findings\n")
	}
	cmd.Println()

	// Output options
	cmd.Printf("Output Format:   %s\n", opts.OutputFormat)
	cmd.Printf("Verbose:         %t\n", opts.Verbose)
	cmd.Printf("Quiet:           %t\n", opts.Quiet)
	cmd.Println()

	cmd.Println(strings.Repeat("=", 60))
	cmd.Println()
	cmd.Println("Configuration is valid. Attack would execute with the above settings.")
	cmd.Println("(Use without --dry-run to execute)")

	return nil
}

// setupSignalHandler sets up signal handling for graceful cancellation (Task 8.5)
func setupSignalHandler(ctx context.Context) (context.Context, context.CancelFunc) {
	// Create a context that cancels on SIGINT or SIGTERM
	ctx, cancel := context.WithCancel(ctx)

	// Create signal channel
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start goroutine to handle signals
	go func() {
		select {
		case <-sigChan:
			// Signal received - cancel context for graceful shutdown
			fmt.Fprintln(os.Stderr, "\nReceived interrupt signal. Cancelling attack...")
			cancel()
		case <-ctx.Done():
			// Context already cancelled
		}
	}()

	return ctx, cancel
}

// initializeLLMComponents creates and configures LLM registry and slot manager (Task 4.2)
func initializeLLMComponents() (llm.LLMRegistry, llm.SlotManager, error) {
	// Create registry
	registry := llm.NewLLMRegistry()

	// Track number of providers successfully registered
	providersRegistered := 0

	// Check for Anthropic
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		cfg := llm.ProviderConfig{
			Type:         llm.ProviderAnthropic,
			APIKey:       apiKey,
			DefaultModel: os.Getenv("ANTHROPIC_MODEL"), // Use env var, provider will use its default if empty
		}

		provider, err := providers.NewAnthropicProvider(cfg)
		if err != nil {
			slog.Warn("failed to create Anthropic provider", "error", err)
		} else {
			if err := registry.RegisterProvider(provider); err != nil {
				slog.Warn("failed to register Anthropic provider", "error", err)
			} else {
				slog.Info("registered Anthropic LLM provider")
				providersRegistered++
			}
		}
	}

	// Check for OpenAI
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		cfg := llm.ProviderConfig{
			Type:         llm.ProviderOpenAI,
			APIKey:       apiKey,
			DefaultModel: os.Getenv("OPENAI_MODEL"), // Use env var, provider will use its default if empty
		}

		provider, err := providers.NewOpenAIProvider(cfg)
		if err != nil {
			slog.Warn("failed to create OpenAI provider", "error", err)
		} else {
			if err := registry.RegisterProvider(provider); err != nil {
				slog.Warn("failed to register OpenAI provider", "error", err)
			} else {
				slog.Info("registered OpenAI LLM provider")
				providersRegistered++
			}
		}
	}

	// Check for Google
	if apiKey := os.Getenv("GOOGLE_API_KEY"); apiKey != "" {
		cfg := llm.ProviderConfig{
			Type:         llm.ProviderGoogle,
			APIKey:       apiKey,
			DefaultModel: os.Getenv("GOOGLE_MODEL"), // Use env var, provider will use its default if empty
		}

		provider, err := providers.NewGoogleProvider(cfg)
		if err != nil {
			slog.Warn("failed to create Google provider", "error", err)
		} else {
			if err := registry.RegisterProvider(provider); err != nil {
				slog.Warn("failed to register Google provider", "error", err)
			} else {
				slog.Info("registered Google LLM provider")
				providersRegistered++
			}
		}
	}

	// Check for Ollama (local, no API key required)
	if ollamaURL := os.Getenv("OLLAMA_URL"); ollamaURL != "" {
		cfg := llm.ProviderConfig{
			Type:         "ollama",
			BaseURL:      ollamaURL,
			DefaultModel: os.Getenv("OLLAMA_MODEL"), // Use env var, provider will use its default if empty
		}

		provider, err := providers.NewOllamaProvider(cfg)
		if err != nil {
			slog.Warn("failed to create Ollama provider", "error", err)
		} else {
			if err := registry.RegisterProvider(provider); err != nil {
				slog.Warn("failed to register Ollama provider", "error", err)
			} else {
				slog.Info("registered Ollama LLM provider", "url", ollamaURL)
				providersRegistered++
			}
		}
	} else {
		// Try default Ollama URL (localhost:11434)
		cfg := llm.ProviderConfig{
			Type:         "ollama",
			BaseURL:      "http://localhost:11434",
			DefaultModel: os.Getenv("OLLAMA_MODEL"), // Use env var, provider will use its default if empty
		}

		provider, err := providers.NewOllamaProvider(cfg)
		if err != nil {
			// Don't warn for default Ollama - it's optional
			slog.Debug("Ollama not available at default URL", "error", err)
		} else {
			if err := registry.RegisterProvider(provider); err != nil {
				slog.Debug("failed to register default Ollama provider", "error", err)
			} else {
				slog.Info("registered Ollama LLM provider at default URL")
				providersRegistered++
			}
		}
	}

	// Log warning if no providers are available
	if providersRegistered == 0 {
		slog.Warn("no LLM providers available - set ANTHROPIC_API_KEY, OPENAI_API_KEY, GOOGLE_API_KEY, or configure Ollama")
	} else {
		slog.Info("LLM initialization complete", "providers", providersRegistered)
	}

	// Create slot manager
	slotManager := llm.NewSlotManager(registry)

	return registry, slotManager, nil
}

// slogAdapter adapts slog.Logger to the component.Logger interface
type slogAdapter struct{}

func (s *slogAdapter) Infof(format string, args ...interface{}) {
	slog.Info(fmt.Sprintf(format, args...))
}

func (s *slogAdapter) Warnf(format string, args ...interface{}) {
	slog.Warn(fmt.Sprintf(format, args...))
}

func (s *slogAdapter) Errorf(format string, args ...interface{}) {
	slog.Error(fmt.Sprintf(format, args...))
}

func (s *slogAdapter) Debugf(format string, args ...interface{}) {
	slog.Debug(fmt.Sprintf(format, args...))
}
