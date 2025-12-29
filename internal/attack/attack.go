/*
Package attack provides an ephemeral, mission-based attack execution framework for Gibson.

The attack package implements a streamlined workflow for executing AI-powered security
assessments against LLM systems, APIs, and AI applications. It creates temporary missions
that are only persisted when vulnerabilities are discovered, making it ideal for ad-hoc
testing and continuous security validation.

# Architecture

The attack package orchestrates multiple components to execute security assessments:

  - Target Resolution: Validates and resolves target URLs, headers, and credentials
  - Agent Selection: Selects appropriate AI agents from the registry
  - Payload Filtering: Filters attack payloads by category, technique, or ID
  - Mission Creation: Creates ephemeral single-node missions for execution
  - Execution: Runs missions through the Gibson orchestrator
  - Result Collection: Aggregates findings and execution metrics
  - Auto-Persistence: Conditionally persists missions with discovered findings

# Key Types

The package exports several key types for attack configuration and results:

AttackOptions configures attack execution parameters including target, agent, payloads,
constraints, and output formatting. Use NewAttackOptions() to create with defaults:

	opts := attack.NewAttackOptions()
	opts.TargetURL = "https://api.example.com/chat"
	opts.AgentName = "prompt-injection"
	opts.MaxTurns = 10
	opts.Timeout = 5 * time.Minute

AttackResult contains the complete execution outcome including findings, metrics, and
final status. It provides methods for checking results and extracting statistics:

	if result.HasFindings() {
	    fmt.Printf("Found %d vulnerabilities\n", result.FindingCount())
	    if result.HasCriticalFindings() {
	        fmt.Println("CRITICAL vulnerabilities discovered!")
	    }
	}

AttackStatus represents the final outcome: success, findings, failed, timeout, or cancelled.

# Basic Usage

Create an AttackRunner with dependencies and execute attacks:

	// Initialize dependencies
	orchestrator := mission.NewOrchestrator(...)
	agentRegistry := agent.NewRegistry()
	payloadRegistry := payload.NewRegistry()
	missionStore := mission.NewStore(...)
	findingStore := finding.NewStore(...)

	// Create attack runner
	runner := attack.NewAttackRunner(
	    orchestrator,
	    agentRegistry,
	    payloadRegistry,
	    missionStore,
	    findingStore,
	    attack.WithLogger(logger),
	    attack.WithTracer(tracer),
	)

	// Configure attack
	opts := attack.NewAttackOptions()
	opts.TargetURL = "https://api.example.com/v1/chat"
	opts.AgentName = "prompt-injection"
	opts.Goal = "Test for prompt injection vulnerabilities"
	opts.MaxTurns = 10
	opts.Timeout = 5 * time.Minute

	// Execute attack
	ctx := context.Background()
	result, err := runner.Run(ctx, opts)
	if err != nil {
	    log.Fatal(err)
	}

	// Check results
	fmt.Printf("Status: %s\n", result.Status)
	fmt.Printf("Duration: %s\n", result.Duration)
	fmt.Printf("Findings: %d\n", result.FindingCount())

# Advanced Configuration

Use functional options for fine-grained control:

	opts := attack.NewAttackOptions()
	opts.Apply(
	    attack.WithTargetURL("https://api.example.com/chat"),
	    attack.WithAgentName("jailbreak"),
	    attack.WithPayloadCategory("adversarial"),
	    attack.WithMaxFindings(10),
	    attack.WithSeverityThreshold("high"),
	    attack.WithTimeout(10 * time.Minute),
	    attack.WithVerbose(true),
	)

# Target Configuration

Specify targets by URL or use saved target lookups:

	// Direct URL
	opts.TargetURL = "https://api.openai.com/v1/chat/completions"
	opts.TargetType = types.TargetTypeLLMAPI
	opts.TargetProvider = "openai"
	opts.Credential = "openai-key"

	// Custom headers
	opts.TargetHeaders = map[string]string{
	    "Authorization": "Bearer token",
	    "X-Custom-Header": "value",
	}

	// Saved target lookup
	opts.TargetName = "production-chatbot"

# Agent Selection

Agents are selected from the AgentRegistry. The attack package provides helpers
for listing and validating available agents:

	// List available agents
	selector := attack.NewAgentSelector(agentRegistry)
	agents, err := selector.ListAvailable(ctx)
	for _, agent := range agents {
	    fmt.Printf("- %s: %s\n", agent.Name, agent.Description)
	}

	// Select specific agent
	agent, err := selector.Select(ctx, "prompt-injection")

# Payload Filtering

Filter attack payloads to target specific vulnerabilities:

	// By category
	opts.PayloadCategory = "injection"

	// By MITRE technique
	opts.Techniques = []string{"T1059", "T1203"}

	// By specific IDs
	opts.PayloadIDs = []string{"payload-001", "payload-002"}

# Execution Constraints

Control attack behavior with constraints:

	// Limit turns and timeout
	opts.MaxTurns = 20
	opts.Timeout = 10 * time.Minute

	// Stop after findings
	opts.MaxFindings = 5
	opts.SeverityThreshold = "critical"

	// Rate limiting
	opts.RateLimit = 10 // requests per second

# Persistence Modes

Control whether missions are persisted to the database:

	// Auto-persist on findings (default behavior)
	// Mission saved only if vulnerabilities discovered

	// Always persist
	opts.Persist = true

	// Never persist (ephemeral only)
	opts.NoPersist = true

When persisted, the result contains the mission ID:

	if result.Persisted {
	    fmt.Printf("Mission ID: %s\n", result.MissionID)
	}

# Output Formats

Support multiple output formats for different consumers:

	// Human-readable text (default)
	opts.OutputFormat = "text"
	opts.Verbose = true

	// JSON for programmatic consumption
	opts.OutputFormat = "json"

	// SARIF 2.1.0 for security tools
	opts.OutputFormat = "sarif"

The TextOutputHandler provides colored terminal output with severity indicators,
while JSONOutputHandler streams NDJSON events and SARIFOutputHandler generates
standards-compliant security reports.

# Result Analysis

AttackResult provides comprehensive methods for analyzing outcomes:

	// Status checks
	if result.IsSuccessful() {
	    // Attack completed (with or without findings)
	}
	if result.IsFailed() {
	    // Attack encountered an error
	}
	if result.IsTimeout() {
	    // Attack exceeded time limit
	}
	if result.IsCancelled() {
	    // Attack was cancelled by user
	}

	// Finding statistics
	total := result.FindingCount()
	critical := result.CriticalFindingCount()
	high := result.HighFindingCount()
	medium := result.MediumFindingCount()
	low := result.LowFindingCount()

	// Iterate findings
	for _, finding := range result.Findings {
	    fmt.Printf("[%s] %s\n", finding.Severity, finding.Title)
	    fmt.Printf("  %s\n", finding.Description)
	    fmt.Printf("  Confidence: %.0f%%\n", finding.Confidence*100)
	}

# Exit Codes

The package defines standard exit codes for CLI integration:

	exitCode := attack.ExitCodeFromResult(result)
	os.Exit(exitCode)

Exit codes follow this convention:
  - 0: Success, no findings
  - 1: Findings discovered
  - 2: Critical findings discovered
  - 3: Error during execution
  - 4: Timeout
  - 5: Cancelled by user
  - 10: Invalid configuration

# Error Handling

The package provides structured errors with context:

	result, err := runner.Run(ctx, opts)
	if err != nil {
	    var attackErr *attack.AttackError
	    if errors.As(err, &attackErr) {
	        switch attackErr.Code {
	        case attack.ErrCodeAgentNotFound:
	            fmt.Println("Agent not found. Available agents:")
	            // List available agents
	        case attack.ErrCodeInvalidTarget:
	            fmt.Printf("Invalid target: %s\n", attackErr.Message)
	        }
	    }
	}

# Cancellation and Timeouts

Attacks respect context cancellation and timeouts:

	// With timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// With cancellation
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
	    // Cancel on signal
	    <-sigChan
	    cancel()
	}()

	result, err := runner.Run(ctx, opts)
	if result.IsCancelled() {
	    fmt.Println("Attack cancelled")
	}

# Dry-Run Mode

Validate configuration without execution:

	opts.DryRun = true
	result, err := runner.Run(ctx, opts)
	// Validation occurs, but no attack executes

# Component Integration

The attack package integrates with existing Gibson infrastructure:

  - internal/mission: Mission and workflow orchestration
  - internal/agent: AI agent registry and execution
  - internal/payload: Attack payload registry and filtering
  - internal/finding: Vulnerability finding storage
  - internal/types: Common types and target definitions
  - internal/workflow: Workflow node and edge definitions

# Testing

The package provides comprehensive test coverage with mocked components:

	// Mock dependencies
	mockOrchestrator := &mockMissionOrchestrator{}
	mockAgentRegistry := &mockAgentRegistry{}
	mockPayloadRegistry := &mockPayloadRegistry{}
	mockMissionStore := &mockMissionStore{}
	mockFindingStore := &mockFindingStore{}

	// Create runner with mocks
	runner := attack.NewAttackRunner(
	    mockOrchestrator,
	    mockAgentRegistry,
	    mockPayloadRegistry,
	    mockMissionStore,
	    mockFindingStore,
	)

	// Test attack execution
	result, err := runner.Run(ctx, opts)

# Performance Considerations

Attacks are designed for efficiency:

  - Ephemeral missions minimize database operations
  - Single-node workflows reduce orchestration overhead
  - Streaming output provides real-time feedback
  - Rate limiting prevents target overload
  - Timeout handling prevents runaway executions

# Security Considerations

The attack package implements several security measures:

  - URL validation prevents malformed targets
  - Header validation prevents injection attacks
  - Credential resolution uses secure storage
  - TLS verification (unless explicitly disabled)
  - Proxy support for isolated environments

# CLI Integration

The attack package is designed for seamless CLI integration:

	cmd := &cobra.Command{
	    Use:   "attack [URL]",
	    Short: "Execute an attack against a target",
	    RunE: func(cmd *cobra.Command, args []string) error {
	        opts := parseFlags(cmd, args)
	        runner := createRunner()
	        result, err := runner.Run(cmd.Context(), opts)
	        if err != nil {
	            return err
	        }
	        os.Exit(attack.ExitCodeFromResult(result))
	        return nil
	    },
	}

# Package Structure

The attack package is organized into focused components:

  - attack.go: Package documentation and overview (this file)
  - options.go: AttackOptions configuration and validation
  - result.go: AttackResult and AttackStatus types
  - exit.go: Exit codes and ExitCodeFromResult
  - errors.go: AttackError and error codes
  - target.go: TargetResolver and target configuration
  - agent.go: AgentSelector and agent validation
  - payload.go: PayloadFilter and payload selection
  - output.go: OutputHandler implementations (Text, JSON, SARIF)
  - runner.go: AttackRunner and execution orchestration

# Example: Full CLI-Style Attack

	package main

	import (
	    "context"
	    "fmt"
	    "os"
	    "time"

	    "github.com/zero-day-ai/gibson/internal/attack"
	)

	func main() {
	    // Initialize dependencies (simplified)
	    runner := attack.NewAttackRunner(
	        orchestrator,
	        agentRegistry,
	        payloadRegistry,
	        missionStore,
	        findingStore,
	    )

	    // Parse configuration
	    opts := attack.NewAttackOptions()
	    opts.TargetURL = os.Args[1]
	    opts.AgentName = "prompt-injection"
	    opts.Goal = "Find injection vulnerabilities"
	    opts.MaxTurns = 10
	    opts.Timeout = 5 * time.Minute
	    opts.OutputFormat = "text"
	    opts.Verbose = true

	    // Validate options
	    if err := opts.Validate(); err != nil {
	        fmt.Fprintf(os.Stderr, "Error: %s\n", err)
	        os.Exit(attack.ExitCodeInvalidConfig)
	    }

	    // Setup output handler
	    handler := attack.NewOutputHandler(
	        opts.OutputFormat,
	        os.Stdout,
	        opts.Verbose,
	        opts.Quiet,
	    )

	    // Execute attack
	    ctx := context.Background()
	    handler.OnStart(opts)

	    result, err := runner.Run(ctx, opts)
	    if err != nil {
	        handler.OnError(err)
	        os.Exit(attack.ExitCodeError)
	    }

	    // Display results
	    for _, finding := range result.Findings {
	        handler.OnFinding(finding)
	    }
	    handler.OnComplete(result)

	    // Exit with appropriate code
	    os.Exit(attack.ExitCodeFromResult(result))
	}

# Example: Programmatic Attack Automation

	// Automated security testing pipeline
	func runSecurityTests(targets []string) error {
	    runner := createAttackRunner()

	    for _, target := range targets {
	        opts := attack.NewAttackOptions()
	        opts.TargetURL = target
	        opts.AgentName = "comprehensive-scan"
	        opts.MaxTurns = 20
	        opts.Timeout = 10 * time.Minute
	        opts.Persist = true // Always save results

	        result, err := runner.Run(context.Background(), opts)
	        if err != nil {
	            return fmt.Errorf("attack failed for %s: %w", target, err)
	        }

	        if result.HasCriticalFindings() {
	            // Alert security team
	            alertSecurityTeam(target, result)
	        }

	        // Log results
	        logAttackResult(target, result)
	    }

	    return nil
	}

# Example: Custom Agent Validation

	// Validate agent exists before attack
	func validateAgentExists(registry agent.AgentRegistry, agentName string) error {
	    selector := attack.NewAgentSelector(registry)

	    if err := attack.ValidateAgentName(
	        context.Background(),
	        selector,
	        agentName,
	    ); err != nil {
	        // List available agents on error
	        agents, _ := selector.ListAvailable(context.Background())
	        fmt.Println("Available agents:")
	        for _, agent := range agents {
	            fmt.Printf("  - %s: %s\n", agent.Name, agent.Description)
	        }
	        return err
	    }

	    return nil
	}
*/
package attack
