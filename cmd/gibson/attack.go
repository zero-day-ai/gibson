package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/internal"
	"github.com/zero-day-ai/gibson/internal/attack"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// attackCmd represents the attack command
var attackCmd = &cobra.Command{
	Use:   "attack URL",
	Short: "Launch a quick single-agent attack against a target",
	Long: `Launch an attack against a target URL using a specified agent.
This command creates an ephemeral mission that is automatically persisted
if findings are discovered (unless --no-persist is set).

The attack command provides a quick way to test a target without creating
a full mission workflow. It runs a single agent with optional payload
filtering, execution constraints, and output formatting.

Examples:
  # Basic attack with required agent flag
  gibson attack https://example.com --agent web-scanner

  # Attack with specific goal and timeout
  gibson attack https://api.example.com --agent prompt-injector \
    --goal "Find prompt injection vulnerabilities" --timeout 30m

  # Attack with payload filtering
  gibson attack https://target.com --agent sql-injector \
    --category injection --techniques T1059,T1190

  # Attack with output options
  gibson attack https://example.com --agent xss-scanner \
    --output json --verbose

  # Dry-run to validate configuration
  gibson attack https://example.com --agent web-scanner --dry-run

  # List available agents
  gibson attack --list-agents`,
	Args:             cobra.MaximumNArgs(1),
	RunE:             runAttackCommand,
	ValidArgsFunction: nil,
}

// Attack command flags
var (
	// Target configuration
	attackTargetType     string
	attackTargetProvider string
	attackHeaders        string
	attackCredential     string

	// Agent configuration
	attackAgent    string
	attackGoal     string
	attackMaxTurns int
	attackTimeout  string

	// Payload filtering
	attackPayloads []string
	attackCategory string
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

func init() {
	// Target configuration flags
	attackCmd.Flags().StringVar(&attackTargetType, "type", "", "Target type (llm_chat, llm_api, rag, etc.)")
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

	// Handle --list-agents subcommand (Task 8.2)
	if attackListAgents {
		return runListAgents(cmd, ctx)
	}

	// Validate that URL is provided
	if len(args) == 0 {
		return internal.NewCLIError(attack.ExitConfigError, "target URL is required\n\nUsage: gibson attack URL --agent AGENT")
	}

	targetURL := args[0]

	// Parse flags into AttackOptions (Task 8.3)
	opts, err := buildAttackOptions(targetURL)
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
	runner, err := createAttackRunner()
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

// buildAttackOptions constructs AttackOptions from command-line flags (Task 8.1, 8.3)
func buildAttackOptions(targetURL string) (*attack.AttackOptions, error) {
	opts := attack.NewAttackOptions()

	// Target configuration
	opts.TargetURL = targetURL
	if attackTargetType != "" {
		// Parse target type
		opts.TargetType = types.TargetType(attackTargetType)
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

// createAttackRunner creates an AttackRunner with all dependencies (Task 8.3)
func createAttackRunner() (attack.AttackRunner, error) {
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

	// TODO: Initialize all required dependencies
	// For now, return a placeholder error since the full infrastructure
	// will be wired up in integration stages
	_ = db // Use db to avoid unused variable error

	return nil, fmt.Errorf("attack runner initialization not yet implemented (requires mission orchestrator, registries, etc.)")
}

// runListAgents lists all available agents (Task 8.2)
func runListAgents(cmd *cobra.Command, ctx context.Context) error {
	// Create agent selector
	// TODO: Wire up real agent registry
	// For now, show a placeholder message

	if attackOutput == "json" {
		// JSON output format
		agents := []map[string]interface{}{
			{
				"name":         "placeholder-agent",
				"description":  "Placeholder agent (registry integration pending)",
				"capabilities": []string{"placeholder"},
				"version":      "0.1.0",
			},
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

	// TODO: Replace with real agent listing from registry
	headers := []string{"Name", "Version", "Description", "Capabilities"}
	rows := [][]string{
		{"placeholder-agent", "0.1.0", "Placeholder agent", "placeholder"},
	}

	if err := formatter.PrintTable(headers, rows); err != nil {
		return err
	}

	cmd.Println()
	cmd.Println("Note: Agent registry integration is pending. This is a placeholder output.")

	return nil
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
