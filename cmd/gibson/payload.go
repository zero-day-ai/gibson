package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/internal"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/payload"
	"github.com/zero-day-ai/gibson/internal/types"
	"gopkg.in/yaml.v3"
)

var payloadCmd = &cobra.Command{
	Use:   "payload",
	Short: "Manage attack payloads",
	Long:  `Create, list, execute, and manage attack payloads for LLM red-teaming`,
}

var payloadListCmd = &cobra.Command{
	Use:   "list",
	Short: "List attack payloads with optional filtering",
	Long: `List attack payloads with optional filtering by category, severity, target type, or MITRE technique.

Examples:
  # List all payloads
  gibson payload list

  # List only jailbreak payloads
  gibson payload list --category jailbreak

  # List critical severity payloads
  gibson payload list --severity critical

  # List payloads for specific target type
  gibson payload list --target-type openai

  # List payloads mapped to MITRE technique
  gibson payload list --mitre AML.T0051

  # Combine multiple filters
  gibson payload list --category prompt_injection --severity high

  # Output as JSON
  gibson payload list --output json`,
	RunE: runPayloadList,
}

var payloadShowCmd = &cobra.Command{
	Use:   "show ID",
	Short: "Show detailed payload information",
	Long: `Display full details for a specific attack payload including all fields,
parameters, success indicators, and metadata.

Examples:
  # Show payload details
  gibson payload show payload_abc123

  # Show payload details as JSON
  gibson payload show payload_abc123 --output json`,
	Args: cobra.ExactArgs(1),
	RunE: runPayloadShow,
}

var payloadCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new attack payload",
	Long: `Create a new attack payload either interactively or from a file.

Examples:
  # Create payload from YAML file
  gibson payload create --from-file payload.yaml

  # Create payload from JSON file
  gibson payload create --from-file payload.json

  # Interactive creation (not yet implemented)
  gibson payload create`,
	RunE: runPayloadCreate,
}

var payloadExecuteCmd = &cobra.Command{
	Use:   "execute ID",
	Short: "Execute an attack payload against a target",
	Long: `Execute an attack payload against a specified target. You can override
parameters, specify execution agent, or run in dry-run mode for testing.

Examples:
  # Execute payload against a target
  gibson payload execute payload_abc123 --target target-api

  # Execute with parameter overrides
  gibson payload execute payload_abc123 --target target-api --params instruction="ignore all rules" --params style=urgent

  # Execute with specific agent
  gibson payload execute payload_abc123 --target target-api --agent agent_xyz789

  # Dry run to preview execution without running
  gibson payload execute payload_abc123 --target target-api --dry-run

  # Execute with custom timeout
  gibson payload execute payload_abc123 --target target-api --timeout 120`,
	Args: cobra.ExactArgs(1),
	RunE: runPayloadExecute,
}

// Flags for payload list command
var (
	listPayloadCategory   string
	listPayloadSeverity   string
	listPayloadTargetType string
	listPayloadMitre      string
	listPayloadOutput     string
)

// Flags for payload show command
var (
	showPayloadOutput string
)

// Flags for payload create command
var (
	createPayloadFromFile string
)

// Flags for payload execute command
var (
	executePayloadTarget  string
	executePayloadAgent   string
	executePayloadDryRun  bool
	executePayloadTimeout int
	executePayloadParams  []string
)

func init() {
	// List command flags
	payloadListCmd.Flags().StringVar(&listPayloadCategory, "category", "", "Filter by category (jailbreak, prompt_injection, data_extraction, etc.)")
	payloadListCmd.Flags().StringVar(&listPayloadSeverity, "severity", "", "Filter by severity (critical, high, medium, low, info)")
	payloadListCmd.Flags().StringVar(&listPayloadTargetType, "target-type", "", "Filter by target type (e.g., openai, anthropic, rag)")
	payloadListCmd.Flags().StringVar(&listPayloadMitre, "mitre", "", "Filter by MITRE ATT&CK technique (e.g., AML.T0051)")
	payloadListCmd.Flags().StringVar(&listPayloadOutput, "output", "text", "Output format (text, json)")

	// Show command flags
	payloadShowCmd.Flags().StringVar(&showPayloadOutput, "output", "text", "Output format (text, json)")

	// Create command flags
	payloadCreateCmd.Flags().StringVar(&createPayloadFromFile, "from-file", "", "Create payload from YAML or JSON file")

	// Execute command flags
	payloadExecuteCmd.Flags().StringVar(&executePayloadTarget, "target", "", "Target name or ID (required)")
	payloadExecuteCmd.Flags().StringVar(&executePayloadAgent, "agent", "", "Agent ID to use for execution")
	payloadExecuteCmd.Flags().BoolVar(&executePayloadDryRun, "dry-run", false, "Validate and preview without executing")
	payloadExecuteCmd.Flags().IntVar(&executePayloadTimeout, "timeout", 0, "Execution timeout in seconds (0 = use default)")
	payloadExecuteCmd.Flags().StringSliceVar(&executePayloadParams, "params", []string{}, "Parameter overrides in key=value format")
	payloadExecuteCmd.MarkFlagRequired("target")

	// Add subcommands
	payloadCmd.AddCommand(payloadListCmd)
	payloadCmd.AddCommand(payloadShowCmd)
	payloadCmd.AddCommand(payloadCreateCmd)
	payloadCmd.AddCommand(payloadExecuteCmd)
	payloadCmd.AddCommand(payloadChainCmd)
	payloadCmd.AddCommand(payloadStatsCmd)
	payloadCmd.AddCommand(payloadImportCmd)
	payloadCmd.AddCommand(payloadExportCmd)
	payloadCmd.AddCommand(payloadSearchCmd)
}

// runPayloadList executes the payload list command
func runPayloadList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create payload store
	store := payload.NewPayloadStore(db)

	// Build filter
	filter := &payload.PayloadFilter{}

	if listPayloadCategory != "" {
		category := payload.PayloadCategory(listPayloadCategory)
		if !category.IsValid() {
			return fmt.Errorf("invalid category: %s (valid: %s)", listPayloadCategory, getValidCategories())
		}
		filter.Categories = []payload.PayloadCategory{category}
	}

	if listPayloadSeverity != "" {
		severity := agent.FindingSeverity(listPayloadSeverity)
		if !isValidSeverity(severity) {
			return fmt.Errorf("invalid severity: %s (valid: critical, high, medium, low, info)", listPayloadSeverity)
		}
		filter.Severities = []agent.FindingSeverity{severity}
	}

	if listPayloadTargetType != "" {
		filter.TargetTypes = []string{listPayloadTargetType}
	}

	if listPayloadMitre != "" {
		filter.MitreTechniques = []string{listPayloadMitre}
	}

	// Query payloads
	payloads, err := store.List(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to list payloads: %w", err)
	}

	// Determine output format
	outputFormat := internal.FormatText
	if listPayloadOutput == "json" {
		outputFormat = internal.FormatJSON
	}

	formatter := internal.NewFormatter(outputFormat, cmd.OutOrStdout())

	// Handle JSON output
	if outputFormat == internal.FormatJSON {
		output := map[string]interface{}{
			"count":    len(payloads),
			"payloads": payloads,
		}
		return formatter.PrintJSON(output)
	}

	// Text output
	if len(payloads) == 0 {
		return formatter.PrintError("No payloads found matching the specified filters")
	}

	// Print summary
	fmt.Fprintf(cmd.OutOrStdout(), "Found %d payload(s)\n\n", len(payloads))

	// Build table
	headers := []string{"ID", "Name", "Categories", "Severity", "Version", "Built-In"}
	rows := make([][]string, 0, len(payloads))

	for _, p := range payloads {
		// Format categories
		categories := make([]string, len(p.Categories))
		for i, cat := range p.Categories {
			categories[i] = string(cat)
		}
		categoryStr := strings.Join(categories, ", ")
		if len(categoryStr) > 30 {
			categoryStr = categoryStr[:27] + "..."
		}

		// Format built-in status
		builtInStr := ""
		if p.BuiltIn {
			builtInStr = "yes"
		}

		rows = append(rows, []string{
			p.ID.String(),
			truncateString(p.Name, 30),
			categoryStr,
			string(p.Severity),
			p.Version,
			builtInStr,
		})
	}

	// Print table
	if err := formatter.PrintTable(headers, rows); err != nil {
		return fmt.Errorf("failed to print table: %w", err)
	}

	// Print filter summary
	if hasFilters() {
		fmt.Fprintf(cmd.OutOrStdout(), "\nFilters applied:")
		if listPayloadCategory != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " category=%s", listPayloadCategory)
		}
		if listPayloadSeverity != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " severity=%s", listPayloadSeverity)
		}
		if listPayloadTargetType != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " target-type=%s", listPayloadTargetType)
		}
		if listPayloadMitre != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " mitre=%s", listPayloadMitre)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	return nil
}

// hasFilters checks if any filters are applied
func hasFilters() bool {
	return listPayloadCategory != "" ||
		listPayloadSeverity != "" ||
		listPayloadTargetType != "" ||
		listPayloadMitre != ""
}

// getValidCategories returns a comma-separated list of valid categories
func getValidCategories() string {
	categories := payload.AllCategories()
	strs := make([]string, len(categories))
	for i, cat := range categories {
		strs[i] = string(cat)
	}
	return strings.Join(strs, ", ")
}

// isValidSeverity checks if the severity is valid
func isValidSeverity(severity agent.FindingSeverity) bool {
	switch severity {
	case agent.SeverityCritical, agent.SeverityHigh, agent.SeverityMedium,
		agent.SeverityLow, agent.SeverityInfo:
		return true
	default:
		return false
	}
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// runPayloadShow executes the payload show command
func runPayloadShow(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	payloadID := args[0]

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create payload store
	store := payload.NewPayloadStore(db)

	// Parse payload ID
	id, err := types.ParseID(payloadID)
	if err != nil {
		return fmt.Errorf("invalid payload ID: %w", err)
	}

	// Get payload by ID
	p, err := store.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get payload: %w", err)
	}

	if p == nil {
		return fmt.Errorf("payload not found: %s", payloadID)
	}

	// Determine output format
	outputFormat := internal.FormatText
	if showPayloadOutput == "json" {
		outputFormat = internal.FormatJSON
	}

	formatter := internal.NewFormatter(outputFormat, cmd.OutOrStdout())

	// Handle JSON output
	if outputFormat == internal.FormatJSON {
		return formatter.PrintJSON(p)
	}

	// Text output - display all payload fields
	out := cmd.OutOrStdout()

	// Header
	fmt.Fprintf(out, "Payload: %s\n", p.Name)
	fmt.Fprintf(out, "ID: %s\n", p.ID.String())
	fmt.Fprintf(out, "Version: %s\n", p.Version)
	fmt.Fprintf(out, "\n")

	// Description
	if p.Description != "" {
		fmt.Fprintf(out, "Description:\n  %s\n\n", p.Description)
	}

	// Categorization
	fmt.Fprintf(out, "Categories:\n")
	if len(p.Categories) > 0 {
		for _, cat := range p.Categories {
			fmt.Fprintf(out, "  - %s\n", cat)
		}
	} else {
		fmt.Fprintf(out, "  (none)\n")
	}
	fmt.Fprintf(out, "\n")

	if len(p.Tags) > 0 {
		fmt.Fprintf(out, "Tags:\n")
		fmt.Fprintf(out, "  %s\n\n", strings.Join(p.Tags, ", "))
	}

	// Severity
	fmt.Fprintf(out, "Severity: %s\n\n", p.Severity)

	// Template
	fmt.Fprintf(out, "Template:\n")
	fmt.Fprintf(out, "  %s\n\n", formatMultilineField(p.Template, 2))

	// Parameters
	if len(p.Parameters) > 0 {
		fmt.Fprintf(out, "Parameters:\n")
		for _, param := range p.Parameters {
			fmt.Fprintf(out, "  - %s (%s)", param.Name, param.Type)
			if param.Required {
				fmt.Fprintf(out, " [required]")
			}
			fmt.Fprintf(out, "\n")
			if param.Description != "" {
				fmt.Fprintf(out, "    Description: %s\n", param.Description)
			}
			if param.Default != nil {
				fmt.Fprintf(out, "    Default: %v\n", param.Default)
			}
			if param.Generator != nil {
				fmt.Fprintf(out, "    Generator: %s\n", param.Generator.Type)
			}
		}
		fmt.Fprintf(out, "\n")
	}

	// Success Indicators
	if len(p.SuccessIndicators) > 0 {
		fmt.Fprintf(out, "Success Indicators:\n")
		for i, indicator := range p.SuccessIndicators {
			fmt.Fprintf(out, "  %d. Type: %s\n", i+1, indicator.Type)
			fmt.Fprintf(out, "     Value: %s\n", indicator.Value)
			if indicator.Description != "" {
				fmt.Fprintf(out, "     Description: %s\n", indicator.Description)
			}
			if indicator.Weight > 0 {
				fmt.Fprintf(out, "     Weight: %.2f\n", indicator.Weight)
			}
			if indicator.Negate {
				fmt.Fprintf(out, "     Negate: true\n")
			}
		}
		fmt.Fprintf(out, "\n")
	}

	// Target Types
	if len(p.TargetTypes) > 0 {
		fmt.Fprintf(out, "Target Types:\n")
		for _, targetType := range p.TargetTypes {
			fmt.Fprintf(out, "  - %s\n", targetType)
		}
		fmt.Fprintf(out, "\n")
	}

	// MITRE Techniques
	if len(p.MitreTechniques) > 0 {
		fmt.Fprintf(out, "MITRE ATT&CK Techniques:\n")
		for _, technique := range p.MitreTechniques {
			fmt.Fprintf(out, "  - %s\n", technique)
		}
		fmt.Fprintf(out, "\n")
	}

	// Metadata
	if p.Metadata.Author != "" || p.Metadata.Source != "" || len(p.Metadata.References) > 0 ||
		p.Metadata.Notes != "" || p.Metadata.Difficulty != "" || p.Metadata.Reliability > 0 {
		fmt.Fprintf(out, "Metadata:\n")
		if p.Metadata.Author != "" {
			fmt.Fprintf(out, "  Author: %s\n", p.Metadata.Author)
		}
		if p.Metadata.Source != "" {
			fmt.Fprintf(out, "  Source: %s\n", p.Metadata.Source)
		}
		if p.Metadata.Difficulty != "" {
			fmt.Fprintf(out, "  Difficulty: %s\n", p.Metadata.Difficulty)
		}
		if p.Metadata.Reliability > 0 {
			fmt.Fprintf(out, "  Reliability: %.2f\n", p.Metadata.Reliability)
		}
		if len(p.Metadata.References) > 0 {
			fmt.Fprintf(out, "  References:\n")
			for _, ref := range p.Metadata.References {
				fmt.Fprintf(out, "    - %s\n", ref)
			}
		}
		if p.Metadata.Notes != "" {
			fmt.Fprintf(out, "  Notes:\n    %s\n", formatMultilineField(p.Metadata.Notes, 4))
		}
		if len(p.Metadata.Examples) > 0 {
			fmt.Fprintf(out, "  Examples:\n")
			for i, example := range p.Metadata.Examples {
				fmt.Fprintf(out, "    %d. %s\n", i+1, formatMultilineField(example, 7))
			}
		}
		fmt.Fprintf(out, "\n")
	}

	// Status
	fmt.Fprintf(out, "Status:\n")
	fmt.Fprintf(out, "  Built-in: %v\n", p.BuiltIn)
	fmt.Fprintf(out, "  Enabled: %v\n", p.Enabled)
	fmt.Fprintf(out, "  Created: %s\n", p.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(out, "  Updated: %s\n", p.UpdatedAt.Format("2006-01-02 15:04:05"))

	return nil
}

// formatMultilineField formats a multiline field with proper indentation
func formatMultilineField(text string, indent int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= 1 {
		return text
	}

	indentStr := strings.Repeat(" ", indent)
	result := lines[0]
	for i := 1; i < len(lines); i++ {
		result += "\n" + indentStr + lines[i]
	}
	return result
}

// runPayloadCreate executes the payload create command
func runPayloadCreate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Check if file-based creation
	if createPayloadFromFile == "" {
		// Interactive payload creation wizard
		return runInteractivePayloadCreation(ctx, cmd)
	}

	// Read payload from file
	p, err := loadPayloadFromFile(createPayloadFromFile)
	if err != nil {
		return fmt.Errorf("failed to load payload from file: %w", err)
	}

	// Validate payload
	if err := validatePayload(p); err != nil {
		return fmt.Errorf("payload validation failed: %w", err)
	}

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create payload store
	store := payload.NewPayloadStore(db)

	// Generate ID if not provided
	if p.ID.IsZero() {
		p.ID = types.NewID()
	}

	// Set timestamps
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now

	// Default values
	if p.Version == "" {
		p.Version = "1.0.0"
	}
	p.BuiltIn = false
	if !p.Enabled {
		p.Enabled = true // Enable by default
	}

	// Save payload
	if err := store.Save(ctx, p); err != nil {
		return fmt.Errorf("failed to save payload: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Successfully created payload: %s (ID: %s)\n", p.Name, p.ID.String())

	return nil
}

// runInteractivePayloadCreation creates a payload interactively via prompts
func runInteractivePayloadCreation(ctx context.Context, cmd *cobra.Command) error {
	var p payload.Payload
	var err error

	fmt.Fprintf(cmd.OutOrStdout(), "=== Interactive Payload Creation ===\n\n")

	// Prompt for name
	fmt.Fprintf(cmd.OutOrStdout(), "Payload Name: ")
	p.Name, err = readLine()
	if err != nil {
		return fmt.Errorf("failed to read name: %w", err)
	}
	if p.Name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	// Prompt for description
	fmt.Fprintf(cmd.OutOrStdout(), "Description: ")
	p.Description, err = readLine()
	if err != nil {
		return fmt.Errorf("failed to read description: %w", err)
	}

	// Prompt for category
	fmt.Fprintf(cmd.OutOrStdout(), "\nAvailable categories:\n")
	categories := payload.AllCategories()
	for i, cat := range categories {
		fmt.Fprintf(cmd.OutOrStdout(), "  %d. %s\n", i+1, cat)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Select category (1-%d): ", len(categories))
	categoryIdx, err := readInt()
	if err != nil || categoryIdx < 1 || categoryIdx > len(categories) {
		return fmt.Errorf("invalid category selection")
	}
	p.Categories = []payload.PayloadCategory{categories[categoryIdx-1]}

	// Prompt for severity
	fmt.Fprintf(cmd.OutOrStdout(), "\nSeverity levels: critical, high, medium, low, info\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Severity: ")
	severityStr, err := readLine()
	if err != nil {
		return fmt.Errorf("failed to read severity: %w", err)
	}
	p.Severity = agent.FindingSeverity(severityStr)
	if !isValidSeverity(p.Severity) {
		p.Severity = agent.SeverityMedium // Default
		fmt.Fprintf(cmd.OutOrStdout(), "Invalid severity, using default: medium\n")
	}

	// Prompt for template
	fmt.Fprintf(cmd.OutOrStdout(), "\nPayload Template (press Enter then Ctrl+D when done):\n")
	p.Template, err = readMultiLine()
	if err != nil {
		return fmt.Errorf("failed to read template: %w", err)
	}
	if p.Template == "" {
		return fmt.Errorf("template cannot be empty")
	}

	// Add a basic success indicator
	p.SuccessIndicators = []payload.SuccessIndicator{
		{
			Type:        payload.IndicatorContains,
			Value:       "success",
			Description: "Response contains success indicator",
			Weight:      1.0,
		},
	}

	// Save the payload
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	store := payload.NewPayloadStore(db)

	// Generate ID and timestamps
	p.ID = types.NewID()
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	p.Version = "1.0.0"
	p.BuiltIn = false
	p.Enabled = true

	// Validate and save
	if err := validatePayload(&p); err != nil {
		return fmt.Errorf("payload validation failed: %w", err)
	}

	if err := store.Save(ctx, &p); err != nil {
		return fmt.Errorf("failed to save payload: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nSuccessfully created payload: %s (ID: %s)\n", p.Name, p.ID.String())

	return nil
}

// readLine reads a single line from stdin
func readLine() (string, error) {
	var line string
	_, err := fmt.Scanln(&line)
	if err != nil && err.Error() != "unexpected newline" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// readInt reads an integer from stdin
func readInt() (int, error) {
	var num int
	_, err := fmt.Scanf("%d\n", &num)
	return num, err
}

// readMultiLine reads multiple lines from stdin until EOF
func readMultiLine() (string, error) {
	var lines []string
	var line string
	for {
		n, err := fmt.Scanln(&line)
		if err != nil || n == 0 {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

// loadPayloadFromFile loads a payload from a YAML or JSON file
func loadPayloadFromFile(filePath string) (*payload.Payload, error) {
	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	p := &payload.Payload{}

	// Try YAML first, then JSON
	if strings.HasSuffix(filePath, ".yaml") || strings.HasSuffix(filePath, ".yml") {
		if err := yaml.Unmarshal(data, p); err != nil {
			return nil, fmt.Errorf("failed to parse YAML: %w", err)
		}
	} else if strings.HasSuffix(filePath, ".json") {
		if err := json.Unmarshal(data, p); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
	} else {
		// Try JSON first, then JSON
		if err := json.Unmarshal(data, p); err != nil {
			if err := yaml.Unmarshal(data, p); err != nil {
				return nil, fmt.Errorf("failed to parse file as JSON or YAML")
			}
		}
	}

	return p, nil
}

// validatePayload validates a payload before saving
func validatePayload(p *payload.Payload) error {
	if p == nil {
		return fmt.Errorf("payload is nil")
	}

	if p.Name == "" {
		return fmt.Errorf("payload name is required")
	}

	if p.Template == "" {
		return fmt.Errorf("payload template is required")
	}

	if len(p.Categories) == 0 {
		return fmt.Errorf("at least one category is required")
	}

	// Validate categories
	for _, cat := range p.Categories {
		if !cat.IsValid() {
			return fmt.Errorf("invalid category: %s", cat)
		}
	}

	// Validate severity
	if p.Severity != "" && !isValidSeverity(p.Severity) {
		return fmt.Errorf("invalid severity: %s", p.Severity)
	}

	// Validate parameters
	for i, param := range p.Parameters {
		if param.Name == "" {
			return fmt.Errorf("parameter %d: name is required", i)
		}
		if param.Type == "" {
			return fmt.Errorf("parameter %d (%s): type is required", i, param.Name)
		}
		if !param.Type.IsValid() {
			return fmt.Errorf("parameter %d (%s): invalid type %s", i, param.Name, param.Type)
		}
	}

	// Validate success indicators
	if len(p.SuccessIndicators) == 0 {
		return fmt.Errorf("at least one success indicator is required")
	}

	for i, indicator := range p.SuccessIndicators {
		if !indicator.Type.IsValid() {
			return fmt.Errorf("success indicator %d: invalid type %s", i, indicator.Type)
		}
		if indicator.Value == "" {
			return fmt.Errorf("success indicator %d: value is required", i)
		}
	}

	return nil
}

// runPayloadExecute executes the payload execute command
func runPayloadExecute(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	payloadID := args[0]

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Parse payload ID
	id, err := types.ParseID(payloadID)
	if err != nil {
		return fmt.Errorf("invalid payload ID: %w", err)
	}

	// Get payload
	payloadStore := payload.NewPayloadStore(db)
	p, err := payloadStore.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get payload: %w", err)
	}
	if p == nil {
		return fmt.Errorf("payload not found: %s", payloadID)
	}

	// Parse parameters from flags
	params, err := parseParameters(executePayloadParams)
	if err != nil {
		return fmt.Errorf("failed to parse parameters: %w", err)
	}

	// Get target
	targetDAO := database.NewTargetDAO(db)
	target, err := getTargetByNameOrID(ctx, targetDAO, executePayloadTarget)
	if err != nil {
		return fmt.Errorf("failed to get target: %w", err)
	}
	if target == nil {
		return fmt.Errorf("target not found: %s", executePayloadTarget)
	}

	// Create execution request
	req := &payload.ExecutionRequest{
		PayloadID:  p.ID,
		TargetID:   target.ID,
		Parameters: params,
	}

	// Set agent ID if specified
	if executePayloadAgent != "" {
		agentID, err := types.ParseID(executePayloadAgent)
		if err != nil {
			return fmt.Errorf("invalid agent ID: %w", err)
		}
		req.AgentID = agentID
	}

	// Set timeout if specified
	if executePayloadTimeout > 0 {
		req.Timeout = time.Duration(executePayloadTimeout) * time.Second
	}

	// Create executor
	executionStore := payload.NewExecutionStore(db)

	// Create payload registry
	registry := payload.NewPayloadRegistry(db, payload.DefaultRegistryConfig())

	// For now, we'll use nil for finding store and agent registry
	// In production, these would be properly initialized
	executor := payload.NewPayloadExecutorWithDefaults(
		registry,
		executionStore,
		nil, // finding store - would need to be initialized
		nil, // agent registry - would need to be initialized
	)

	// Handle dry-run mode
	if executePayloadDryRun {
		return runPayloadExecuteDryRun(cmd, executor, req, p, target)
	}

	// Execute payload
	fmt.Fprintf(cmd.OutOrStdout(), "Executing payload: %s\n", p.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "Target: %s (%s)\n", target.Name, target.URL)
	fmt.Fprintf(cmd.OutOrStdout(), "\n")

	result, err := executor.Execute(ctx, req)
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	// Display results
	return displayExecutionResult(cmd, result, p)
}

// runPayloadExecuteDryRun handles dry-run execution
func runPayloadExecuteDryRun(
	cmd *cobra.Command,
	executor payload.PayloadExecutor,
	req *payload.ExecutionRequest,
	p *payload.Payload,
	target *types.Target,
) error {
	ctx := cmd.Context()

	fmt.Fprintf(cmd.OutOrStdout(), "DRY RUN MODE - Validation Only\n")
	fmt.Fprintf(cmd.OutOrStdout(), "=====================================\n\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Payload: %s\n", p.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "Target: %s (%s)\n", target.Name, target.URL)
	fmt.Fprintf(cmd.OutOrStdout(), "\n")

	// Execute dry run
	dryRunResult, err := executor.ExecuteDryRun(ctx, req)
	if err != nil {
		return fmt.Errorf("dry run failed: %w", err)
	}

	// Display validation results
	if len(dryRunResult.ValidationErrors) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "VALIDATION ERRORS:\n")
		for _, errMsg := range dryRunResult.ValidationErrors {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", errMsg)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\n")
		return fmt.Errorf("validation failed")
	}

	// Display warnings
	if len(dryRunResult.Warnings) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "WARNINGS:\n")
		for _, warning := range dryRunResult.Warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", warning)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\n")
	}

	// Display instantiated text
	fmt.Fprintf(cmd.OutOrStdout(), "INSTANTIATED PAYLOAD:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "-------------------------------------\n")
	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", dryRunResult.InstantiatedText)
	fmt.Fprintf(cmd.OutOrStdout(), "-------------------------------------\n\n")

	// Display estimates
	fmt.Fprintf(cmd.OutOrStdout(), "ESTIMATES:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Estimated tokens: ~%d\n", dryRunResult.EstimatedTokens)
	fmt.Fprintf(cmd.OutOrStdout(), "  Payload length: %d characters\n\n", len(dryRunResult.InstantiatedText))

	fmt.Fprintf(cmd.OutOrStdout(), "Validation passed! Ready for execution.\n")

	return nil
}

// displayExecutionResult displays the execution result
func displayExecutionResult(
	cmd *cobra.Command,
	result *payload.ExecutionResult,
	p *payload.Payload,
) error {
	out := cmd.OutOrStdout()

	// Display status
	fmt.Fprintf(out, "EXECUTION RESULT\n")
	fmt.Fprintf(out, "=====================================\n\n")

	// Status
	fmt.Fprintf(out, "Status: %s\n", result.Status)

	// Success indicator
	if result.Success {
		fmt.Fprintf(out, "Success: YES (Confidence: %.2f%%)\n", result.ConfidenceScore*100)
	} else {
		fmt.Fprintf(out, "Success: NO\n")
	}

	// Error message if any
	if result.ErrorMessage != "" {
		fmt.Fprintf(out, "Error: %s\n", result.ErrorMessage)
	}

	fmt.Fprintf(out, "\n")

	// Execution details
	fmt.Fprintf(out, "EXECUTION DETAILS\n")
	fmt.Fprintf(out, "-------------------------------------\n")
	if !result.StartedAt.IsZero() {
		fmt.Fprintf(out, "Started: %s\n", result.StartedAt.Format("2006-01-02 15:04:05"))
	}
	if !result.CompletedAt.IsZero() {
		fmt.Fprintf(out, "Completed: %s\n", result.CompletedAt.Format("2006-01-02 15:04:05"))
	}
	if result.Duration > 0 {
		fmt.Fprintf(out, "Duration: %s\n", result.Duration)
	}
	if result.ResponseTime > 0 {
		fmt.Fprintf(out, "Response Time: %s\n", result.ResponseTime)
	}
	if result.TokensUsed > 0 {
		fmt.Fprintf(out, "Tokens Used: %d\n", result.TokensUsed)
	}
	if result.Cost > 0 {
		fmt.Fprintf(out, "Cost: $%.4f\n", result.Cost)
	}
	fmt.Fprintf(out, "\n")

	// Display instantiated text
	if result.InstantiatedText != "" {
		fmt.Fprintf(out, "PAYLOAD SENT\n")
		fmt.Fprintf(out, "-------------------------------------\n")
		fmt.Fprintf(out, "%s\n", truncateForDisplay(result.InstantiatedText, 500))
		fmt.Fprintf(out, "-------------------------------------\n\n")
	}

	// Display response
	if result.Response != "" {
		fmt.Fprintf(out, "RESPONSE RECEIVED\n")
		fmt.Fprintf(out, "-------------------------------------\n")
		fmt.Fprintf(out, "%s\n", truncateForDisplay(result.Response, 1000))
		fmt.Fprintf(out, "-------------------------------------\n\n")
	}

	// Display matched indicators
	if result.Success && len(result.IndicatorsMatched) > 0 {
		fmt.Fprintf(out, "MATCHED INDICATORS\n")
		fmt.Fprintf(out, "-------------------------------------\n")
		for i := range result.IndicatorsMatched {
			// Find the indicator in the payload
			var indicatorDesc string
			if i < len(p.SuccessIndicators) {
				indicatorDesc = p.SuccessIndicators[i].Description
				if indicatorDesc == "" {
					indicatorDesc = fmt.Sprintf("%s: %s", p.SuccessIndicators[i].Type, p.SuccessIndicators[i].Value)
				}
			}
			fmt.Fprintf(out, "  %d. %s\n", i+1, indicatorDesc)
		}
		fmt.Fprintf(out, "\n")
	}

	// Display match details
	if len(result.MatchDetails) > 0 {
		fmt.Fprintf(out, "MATCH DETAILS\n")
		fmt.Fprintf(out, "-------------------------------------\n")
		for key, value := range result.MatchDetails {
			fmt.Fprintf(out, "  %s: %v\n", key, value)
		}
		fmt.Fprintf(out, "\n")
	}

	// Final summary
	if result.Success {
		fmt.Fprintf(out, "\nATTACK SUCCESSFUL!\n")
		fmt.Fprintf(out, "The payload successfully exploited the target.\n")
		if result.FindingCreated {
			fmt.Fprintf(out, "A finding has been created for this successful attack.\n")
		}
	} else if result.Status == payload.ExecutionStatusCompleted {
		fmt.Fprintf(out, "\nAttack did not succeed.\n")
		fmt.Fprintf(out, "The target's response did not match the success indicators.\n")
	}

	return nil
}

// parseParameters parses parameter key=value pairs
func parseParameters(params []string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, param := range params {
		parts := strings.SplitN(param, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid parameter format: %s (expected key=value)", param)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "" {
			return nil, fmt.Errorf("parameter key cannot be empty")
		}

		result[key] = value
	}

	return result, nil
}

// getTargetByNameOrID retrieves a target by name or ID
func getTargetByNameOrID(ctx context.Context, dao *database.TargetDAO, nameOrID string) (*types.Target, error) {
	// Try to parse as ID first
	id, err := types.ParseID(nameOrID)
	if err == nil {
		// Valid ID, try to get by ID
		target, err := dao.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if target != nil {
			return target, nil
		}
	}

	// Not a valid ID or not found, try to get by name
	targets, err := dao.List(ctx, types.NewTargetFilter())
	if err != nil {
		return nil, err
	}

	for _, target := range targets {
		if target.Name == nameOrID {
			return target, nil
		}
	}

	return nil, nil
}

// truncateForDisplay truncates text for display
func truncateForDisplay(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "\n... (truncated, " + fmt.Sprintf("%d", len(text)-maxLen) + " more characters)"
}

// Chain commands

var payloadChainCmd = &cobra.Command{
	Use:   "chain",
	Short: "Manage attack chains",
	Long:  `Manage multi-stage attack chains for orchestrated testing`,
}

var payloadChainListCmd = &cobra.Command{
	Use:   "list",
	Short: "List attack chains",
	Long: `List all attack chains with optional filtering.

Examples:
  # List all chains
  gibson payload chain list

  # Output as JSON
  gibson payload chain list --output json`,
	RunE: runPayloadChainList,
}

var payloadChainShowCmd = &cobra.Command{
	Use:   "show ID",
	Short: "Show detailed attack chain information",
	Long: `Display full details for a specific attack chain including all stages.

Examples:
  # Show chain details
  gibson payload chain show chain_abc123

  # Show chain details as JSON
  gibson payload chain show chain_abc123 --output json`,
	Args: cobra.ExactArgs(1),
	RunE: runPayloadChainShow,
}

var payloadChainCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new attack chain",
	Long: `Create a new attack chain from a file.

Examples:
  # Create chain from YAML file
  gibson payload chain create --from-file chain.yaml

  # Create chain from JSON file
  gibson payload chain create --from-file chain.json`,
	RunE: runPayloadChainCreate,
}

var payloadChainExecuteCmd = &cobra.Command{
	Use:   "execute ID",
	Short: "Execute an attack chain against a target",
	Long: `Execute a multi-stage attack chain against a specified target.

Examples:
  # Execute chain against a target
  gibson payload chain execute chain_abc123 --target target-api

  # Execute with parameter overrides
  gibson payload chain execute chain_abc123 --target target-api --params key=value

  # Execute with custom timeout
  gibson payload chain execute chain_abc123 --target target-api --timeout 600`,
	Args: cobra.ExactArgs(1),
	RunE: runPayloadChainExecute,
}

// Flags for chain commands
var (
	chainListOutput     string
	chainShowOutput     string
	chainCreateFromFile string
	chainExecuteTarget  string
	chainExecuteParams  []string
	chainExecuteTimeout int
)

func init() {
	// Chain list flags
	payloadChainListCmd.Flags().StringVar(&chainListOutput, "output", "text", "Output format (text, json)")

	// Chain show flags
	payloadChainShowCmd.Flags().StringVar(&chainShowOutput, "output", "text", "Output format (text, json)")

	// Chain create flags
	payloadChainCreateCmd.Flags().StringVar(&chainCreateFromFile, "from-file", "", "Create chain from YAML or JSON file (required)")
	payloadChainCreateCmd.MarkFlagRequired("from-file")

	// Chain execute flags
	payloadChainExecuteCmd.Flags().StringVar(&chainExecuteTarget, "target", "", "Target name or ID (required)")
	payloadChainExecuteCmd.Flags().StringSliceVar(&chainExecuteParams, "params", []string{}, "Parameter overrides in key=value format")
	payloadChainExecuteCmd.Flags().IntVar(&chainExecuteTimeout, "timeout", 0, "Execution timeout in seconds (0 = use default)")
	payloadChainExecuteCmd.MarkFlagRequired("target")

	// Add chain subcommands to chain command
	payloadChainCmd.AddCommand(payloadChainListCmd)
	payloadChainCmd.AddCommand(payloadChainShowCmd)
	payloadChainCmd.AddCommand(payloadChainCreateCmd)
	payloadChainCmd.AddCommand(payloadChainExecuteCmd)
}

// runPayloadChainList executes the payload chain list command
func runPayloadChainList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Query chains from database
	query := "SELECT id, name, description, version, enabled, built_in, created_at FROM attack_chains ORDER BY created_at DESC"
	queryRows, err := db.Conn().QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query chains: %w", err)
	}
	defer queryRows.Close()

	// Collect chains
	type chainSummary struct {
		ID          string    `json:"id"`
		Name        string    `json:"name"`
		Description string    `json:"description"`
		Version     string    `json:"version"`
		Enabled     bool      `json:"enabled"`
		BuiltIn     bool      `json:"built_in"`
		CreatedAt   time.Time `json:"created_at"`
	}

	chains := []chainSummary{}
	for queryRows.Next() {
		var chain chainSummary
		if err := queryRows.Scan(&chain.ID, &chain.Name, &chain.Description, &chain.Version, &chain.Enabled, &chain.BuiltIn, &chain.CreatedAt); err != nil {
			return fmt.Errorf("failed to scan chain: %w", err)
		}
		chains = append(chains, chain)
	}

	if err := queryRows.Err(); err != nil {
		return fmt.Errorf("error iterating chains: %w", err)
	}

	// Determine output format
	outputFormat := internal.FormatText
	if chainListOutput == "json" {
		outputFormat = internal.FormatJSON
	}

	formatter := internal.NewFormatter(outputFormat, cmd.OutOrStdout())

	// Handle JSON output
	if outputFormat == internal.FormatJSON {
		output := map[string]interface{}{
			"count":  len(chains),
			"chains": chains,
		}
		return formatter.PrintJSON(output)
	}

	// Text output
	if len(chains) == 0 {
		return formatter.PrintError("No attack chains found")
	}

	// Print summary
	fmt.Fprintf(cmd.OutOrStdout(), "Found %d attack chain(s)\n\n", len(chains))

	// Build table
	headers := []string{"ID", "Name", "Description", "Version", "Built-In", "Enabled"}
	tableRows := make([][]string, 0, len(chains))

	for _, chain := range chains {
		builtInStr := ""
		if chain.BuiltIn {
			builtInStr = "yes"
		}
		enabledStr := ""
		if chain.Enabled {
			enabledStr = "yes"
		}

		tableRows = append(tableRows, []string{
			chain.ID,
			truncateString(chain.Name, 30),
			truncateString(chain.Description, 40),
			chain.Version,
			builtInStr,
			enabledStr,
		})
	}

	// Print table
	if err := formatter.PrintTable(headers, tableRows); err != nil {
		return fmt.Errorf("failed to print table: %w", err)
	}

	return nil
}

// runPayloadChainShow executes the payload chain show command
func runPayloadChainShow(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	chainID := args[0]

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Query chain from database
	query := `SELECT id, name, description, version, stages, metadata, enabled, built_in, created_at, updated_at
	          FROM attack_chains WHERE id = ?`

	var chain payload.AttackChain
	var stagesJSON, metadataJSON, version string

	err = db.Conn().QueryRowContext(ctx, query, chainID).Scan(
		&chain.ID, &chain.Name, &chain.Description, &version,
		&stagesJSON, &metadataJSON, &chain.Enabled, &chain.BuiltIn,
		&chain.CreatedAt, &chain.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to get chain: %w", err)
	}

	// Parse stages JSON
	if err := json.Unmarshal([]byte(stagesJSON), &chain.Stages); err != nil {
		return fmt.Errorf("failed to parse stages: %w", err)
	}

	// Parse metadata JSON
	if metadataJSON != "" {
		if err := json.Unmarshal([]byte(metadataJSON), &chain.Metadata); err != nil {
			return fmt.Errorf("failed to parse metadata: %w", err)
		}
	}

	// Set version from the scanned value
	chain.Metadata.Version = version

	// Determine output format
	outputFormat := internal.FormatText
	if chainShowOutput == "json" {
		outputFormat = internal.FormatJSON
	}

	formatter := internal.NewFormatter(outputFormat, cmd.OutOrStdout())

	// Handle JSON output
	if outputFormat == internal.FormatJSON {
		return formatter.PrintJSON(chain)
	}

	// Text output
	out := cmd.OutOrStdout()

	// Header
	fmt.Fprintf(out, "Attack Chain: %s\n", chain.Name)
	fmt.Fprintf(out, "ID: %s\n", chain.ID)
	fmt.Fprintf(out, "Version: %s\n", version)
	fmt.Fprintf(out, "\n")

	// Description
	if chain.Description != "" {
		fmt.Fprintf(out, "Description:\n  %s\n\n", chain.Description)
	}

	// Stage count
	fmt.Fprintf(out, "Stages: %d\n", len(chain.Stages))
	if len(chain.Stages) > 0 {
		fmt.Fprintf(out, "\n")
		for stageID, stage := range chain.Stages {
			fmt.Fprintf(out, "  Stage: %s\n", stageID)
			fmt.Fprintf(out, "    Name: %s\n", stage.Name)
			if stage.Description != "" {
				fmt.Fprintf(out, "    Description: %s\n", stage.Description)
			}
			fmt.Fprintf(out, "    Payload ID: %s\n", stage.PayloadID)
			if len(stage.Dependencies) > 0 {
				fmt.Fprintf(out, "    Dependencies: %s\n", strings.Join(stage.Dependencies, ", "))
			}
			if stage.Parallel {
				fmt.Fprintf(out, "    Parallel: yes\n")
			}
			fmt.Fprintf(out, "\n")
		}
	}

	// Metadata
	if chain.Metadata.Author != "" || chain.Metadata.Difficulty != "" || len(chain.Metadata.Tags) > 0 {
		fmt.Fprintf(out, "Metadata:\n")
		if chain.Metadata.Author != "" {
			fmt.Fprintf(out, "  Author: %s\n", chain.Metadata.Author)
		}
		if chain.Metadata.Difficulty != "" {
			fmt.Fprintf(out, "  Difficulty: %s\n", chain.Metadata.Difficulty)
		}
		if len(chain.Metadata.Tags) > 0 {
			fmt.Fprintf(out, "  Tags: %s\n", strings.Join(chain.Metadata.Tags, ", "))
		}
		if chain.Metadata.EstimatedDuration > 0 {
			fmt.Fprintf(out, "  Estimated Duration: %s\n", chain.Metadata.EstimatedDuration)
		}
		fmt.Fprintf(out, "\n")
	}

	// Status
	fmt.Fprintf(out, "Status:\n")
	fmt.Fprintf(out, "  Built-in: %v\n", chain.BuiltIn)
	fmt.Fprintf(out, "  Enabled: %v\n", chain.Enabled)
	fmt.Fprintf(out, "  Created: %s\n", chain.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(out, "  Updated: %s\n", chain.UpdatedAt.Format("2006-01-02 15:04:05"))

	return nil
}

// runPayloadChainCreate executes the payload chain create command
func runPayloadChainCreate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Load chain from file
	chain, err := loadChainFromFile(chainCreateFromFile)
	if err != nil {
		return fmt.Errorf("failed to load chain from file: %w", err)
	}

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Generate ID if not provided
	if chain.ID.IsZero() {
		chain.ID = types.NewID()
	}

	// Set timestamps
	now := time.Now()
	chain.CreatedAt = now
	chain.UpdatedAt = now
	chain.BuiltIn = false
	if !chain.Enabled {
		chain.Enabled = true
	}

	// Marshal stages and metadata
	stagesJSON, err := json.Marshal(chain.Stages)
	if err != nil {
		return fmt.Errorf("failed to marshal stages: %w", err)
	}

	metadataJSON, err := json.Marshal(chain.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Insert into database
	query := `INSERT INTO attack_chains (id, name, description, version, stages, metadata, enabled, built_in, created_at, updated_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = db.Conn().ExecContext(ctx, query,
		chain.ID, chain.Name, chain.Description, chain.Metadata.Version,
		string(stagesJSON), string(metadataJSON),
		chain.Enabled, chain.BuiltIn, chain.CreatedAt, chain.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save chain: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Successfully created attack chain: %s (ID: %s)\n", chain.Name, chain.ID.String())

	return nil
}

// runPayloadChainExecute executes the payload chain execute command
func runPayloadChainExecute(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	chainID := args[0]

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Parse chain ID
	id, err := types.ParseID(chainID)
	if err != nil {
		return fmt.Errorf("invalid chain ID: %w", err)
	}

	// Get chain from database
	query := `SELECT id, name, description, version, stages, metadata, enabled, built_in, created_at, updated_at
	          FROM attack_chains WHERE id = ?`

	var chain payload.AttackChain
	var stagesJSON, metadataJSON, version string

	err = db.Conn().QueryRowContext(ctx, query, id).Scan(
		&chain.ID, &chain.Name, &chain.Description, &version,
		&stagesJSON, &metadataJSON, &chain.Enabled, &chain.BuiltIn,
		&chain.CreatedAt, &chain.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to get chain: %w", err)
	}

	// Set version
	chain.Metadata.Version = version

	// Parse stages JSON
	if err := json.Unmarshal([]byte(stagesJSON), &chain.Stages); err != nil {
		return fmt.Errorf("failed to parse stages: %w", err)
	}

	// Get target
	targetDAO := database.NewTargetDAO(db)
	target, err := getTargetByNameOrID(ctx, targetDAO, chainExecuteTarget)
	if err != nil {
		return fmt.Errorf("failed to get target: %w", err)
	}
	if target == nil {
		return fmt.Errorf("target not found: %s", chainExecuteTarget)
	}

	// Parse parameters
	params, err := parseParameters(chainExecuteParams)
	if err != nil {
		return fmt.Errorf("failed to parse parameters: %w", err)
	}

	// Create execution request
	req := &payload.ChainExecutionRequest{
		ChainID:    chain.ID,
		TargetID:   target.ID,
		Parameters: params,
	}

	// Set timeout if specified
	if chainExecuteTimeout > 0 {
		req.Timeout = time.Duration(chainExecuteTimeout) * time.Second
	}

	// Create executor and chain runner
	executionStore := payload.NewExecutionStore(db)
	registry := payload.NewPayloadRegistry(db, payload.DefaultRegistryConfig())
	executor := payload.NewPayloadExecutorWithDefaults(registry, executionStore, nil, nil)
	chainRunner := payload.NewChainRunnerWithDefaults(executor, registry, executionStore)

	// Execute chain
	fmt.Fprintf(cmd.OutOrStdout(), "Executing attack chain: %s\n", chain.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "Target: %s (%s)\n", target.Name, target.URL)
	fmt.Fprintf(cmd.OutOrStdout(), "Stages: %d\n\n", len(chain.Stages))

	result, err := chainRunner.Execute(ctx, req)
	if err != nil {
		return fmt.Errorf("chain execution failed: %w", err)
	}

	// Display results
	return displayChainExecutionResult(cmd, result, &chain)
}

// displayChainExecutionResult displays the chain execution result
func displayChainExecutionResult(
	cmd *cobra.Command,
	result *payload.ChainResult,
	chain *payload.AttackChain,
) error {
	out := cmd.OutOrStdout()

	// Display status
	fmt.Fprintf(out, "CHAIN EXECUTION RESULT\n")
	fmt.Fprintf(out, "=====================================\n\n")

	// Status
	fmt.Fprintf(out, "Status: %s\n", result.Status)
	fmt.Fprintf(out, "Success: %v\n", result.Success)
	fmt.Fprintf(out, "Stages Executed: %d\n", result.StagesExecuted)
	fmt.Fprintf(out, "Successful Stages: %d\n", result.SuccessfulStages)
	fmt.Fprintf(out, "Failed Stages: %d\n", result.FailedStages)
	fmt.Fprintf(out, "\n")

	// Execution details
	if !result.StartedAt.IsZero() {
		fmt.Fprintf(out, "Started: %s\n", result.StartedAt.Format("2006-01-02 15:04:05"))
	}
	if !result.CompletedAt.IsZero() {
		fmt.Fprintf(out, "Completed: %s\n", result.CompletedAt.Format("2006-01-02 15:04:05"))
	}
	if result.TotalDuration > 0 {
		fmt.Fprintf(out, "Duration: %s\n", result.TotalDuration)
	}
	if result.TotalTokensUsed > 0 {
		fmt.Fprintf(out, "Total Tokens Used: %d\n", result.TotalTokensUsed)
	}
	if result.TotalCost > 0 {
		fmt.Fprintf(out, "Total Cost: $%.4f\n", result.TotalCost)
	}
	fmt.Fprintf(out, "\n")

	// Stage results
	if len(result.StageResults) > 0 {
		fmt.Fprintf(out, "STAGE RESULTS\n")
		fmt.Fprintf(out, "-------------------------------------\n")
		for _, stageResult := range result.StageResults {
			fmt.Fprintf(out, "Stage: %s\n", stageResult.StageID)
			fmt.Fprintf(out, "  Status: %s\n", stageResult.Status)
			fmt.Fprintf(out, "  Success: %v\n", stageResult.Success)
			if stageResult.ErrorMessage != "" {
				fmt.Fprintf(out, "  Error: %s\n", stageResult.ErrorMessage)
			}
			if stageResult.Duration > 0 {
				fmt.Fprintf(out, "  Duration: %s\n", stageResult.Duration)
			}
			fmt.Fprintf(out, "\n")
		}
	}

	// Final summary
	if result.Success {
		fmt.Fprintf(out, "CHAIN EXECUTION SUCCESSFUL!\n")
		fmt.Fprintf(out, "All stages completed successfully.\n")
	} else {
		fmt.Fprintf(out, "Chain execution completed with failures.\n")
		if result.ErrorMessage != "" {
			fmt.Fprintf(out, "Error: %s\n", result.ErrorMessage)
		}
	}

	return nil
}

// loadChainFromFile loads a chain from a YAML or JSON file
func loadChainFromFile(filePath string) (*payload.AttackChain, error) {
	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	chain := &payload.AttackChain{}

	// Try YAML first, then JSON
	if strings.HasSuffix(filePath, ".yaml") || strings.HasSuffix(filePath, ".yml") {
		if err := yaml.Unmarshal(data, chain); err != nil {
			return nil, fmt.Errorf("failed to parse YAML: %w", err)
		}
	} else if strings.HasSuffix(filePath, ".json") {
		if err := json.Unmarshal(data, chain); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
	} else {
		// Try JSON first, then YAML
		if err := json.Unmarshal(data, chain); err != nil {
			if err := yaml.Unmarshal(data, chain); err != nil {
				return nil, fmt.Errorf("failed to parse file as JSON or YAML")
			}
		}
	}

	return chain, nil
}

// Stats command

var payloadStatsCmd = &cobra.Command{
	Use:   "stats [ID]",
	Short: "Show payload execution statistics",
	Long: `Display execution statistics for a specific payload or category.

Examples:
  # Show stats for a specific payload
  gibson payload stats payload_abc123

  # Show stats for a category
  gibson payload stats --category jailbreak

  # Output as JSON
  gibson payload stats payload_abc123 --output json

  # Show category stats as JSON
  gibson payload stats --category prompt_injection --output json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPayloadStats,
}

// Flags for stats command
var (
	statsPayloadCategory string
	statsPayloadOutput   string
)

func init() {
	// Stats command flags
	payloadStatsCmd.Flags().StringVar(&statsPayloadCategory, "category", "", "Show statistics for a category instead of a specific payload")
	payloadStatsCmd.Flags().StringVar(&statsPayloadOutput, "output", "text", "Output format (text, json)")
}

// runPayloadStats executes the payload stats command
func runPayloadStats(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create analytics tracker
	payloadStore := payload.NewPayloadStore(db)
	executionStore := payload.NewExecutionStore(db)
	tracker := payload.NewEffectivenessTracker(executionStore, payloadStore)

	// Determine output format
	outputFormat := internal.FormatText
	if statsPayloadOutput == "json" {
		outputFormat = internal.FormatJSON
	}

	formatter := internal.NewFormatter(outputFormat, cmd.OutOrStdout())

	// Check if we're showing category stats or payload stats
	if statsPayloadCategory != "" {
		// Show category stats
		return runCategoryStats(cmd, ctx, tracker, formatter, outputFormat)
	}

	// Show payload stats
	if len(args) == 0 {
		return fmt.Errorf("payload ID is required unless --category is specified")
	}

	return runSinglePayloadStats(cmd, ctx, tracker, formatter, outputFormat, args[0])
}

// runSinglePayloadStats shows statistics for a single payload
func runSinglePayloadStats(
	cmd *cobra.Command,
	ctx context.Context,
	tracker payload.EffectivenessTracker,
	formatter internal.Formatter,
	outputFormat internal.OutputFormat,
	payloadID string,
) error {
	// Parse payload ID
	id, err := types.ParseID(payloadID)
	if err != nil {
		return fmt.Errorf("invalid payload ID: %w", err)
	}

	// Get stats
	stats, err := tracker.GetPayloadStats(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get payload stats: %w", err)
	}

	// Handle JSON output
	if outputFormat == internal.FormatJSON {
		return formatter.PrintJSON(stats)
	}

	// Text output
	out := cmd.OutOrStdout()

	fmt.Fprintf(out, "Payload Statistics: %s\n", stats.PayloadName)
	fmt.Fprintf(out, "ID: %s\n", stats.PayloadID.String())
	fmt.Fprintf(out, "\n")

	// Execution counts
	fmt.Fprintf(out, "EXECUTION SUMMARY\n")
	fmt.Fprintf(out, "-------------------------------------\n")
	fmt.Fprintf(out, "Total Executions:     %d\n", stats.TotalExecutions)
	fmt.Fprintf(out, "Successful Attacks:   %d\n", stats.SuccessfulAttacks)
	fmt.Fprintf(out, "Failed Executions:    %d\n", stats.FailedExecutions)
	fmt.Fprintf(out, "Timeouts:             %d\n", stats.TimeoutCount)
	fmt.Fprintf(out, "\n")

	// Success metrics
	if stats.TotalExecutions > 0 {
		fmt.Fprintf(out, "SUCCESS METRICS\n")
		fmt.Fprintf(out, "-------------------------------------\n")
		fmt.Fprintf(out, "Success Rate:         %.2f%%\n", stats.SuccessRate*100)
		fmt.Fprintf(out, "Confidence Level:     %.2f%% (%d executions)\n", stats.ConfidenceLevel*100, stats.TotalExecutions)
		if stats.SuccessfulAttacks > 0 {
			fmt.Fprintf(out, "Average Confidence:   %.2f%%\n", stats.AverageConfidence*100)
		}
		fmt.Fprintf(out, "\n")
	}

	// Performance metrics
	if stats.TotalExecutions > 0 {
		fmt.Fprintf(out, "PERFORMANCE METRICS\n")
		fmt.Fprintf(out, "-------------------------------------\n")
		fmt.Fprintf(out, "Average Duration:     %s\n", stats.AverageDuration)
		fmt.Fprintf(out, "Median Duration:      %s\n", stats.MedianDuration)
		if stats.AverageTokensUsed > 0 {
			fmt.Fprintf(out, "Average Tokens Used:  %.0f\n", stats.AverageTokensUsed)
		}
		if stats.TotalCost > 0 {
			fmt.Fprintf(out, "Average Cost:         $%.4f\n", stats.AverageCost)
			fmt.Fprintf(out, "Total Cost:           $%.4f\n", stats.TotalCost)
		}
		fmt.Fprintf(out, "\n")
	}

	// Finding metrics
	if stats.FindingsCreated > 0 {
		fmt.Fprintf(out, "FINDING METRICS\n")
		fmt.Fprintf(out, "-------------------------------------\n")
		fmt.Fprintf(out, "Findings Created:     %d\n", stats.FindingsCreated)
		fmt.Fprintf(out, "Finding Rate:         %.2f%%\n", stats.FindingCreationRate*100)
		fmt.Fprintf(out, "\n")
	}

	// Temporal data
	if !stats.FirstExecution.IsZero() {
		fmt.Fprintf(out, "TEMPORAL DATA\n")
		fmt.Fprintf(out, "-------------------------------------\n")
		fmt.Fprintf(out, "First Execution:      %s\n", stats.FirstExecution.Format("2006-01-02 15:04:05"))
		fmt.Fprintf(out, "Last Execution:       %s\n", stats.LastExecution.Format("2006-01-02 15:04:05"))
		if stats.LastSuccess != nil {
			fmt.Fprintf(out, "Last Success:         %s\n", stats.LastSuccess.Format("2006-01-02 15:04:05"))
		}
		fmt.Fprintf(out, "\n")
	}

	// Trend data
	if stats.TotalExecutions > 0 {
		fmt.Fprintf(out, "TREND ANALYSIS\n")
		fmt.Fprintf(out, "-------------------------------------\n")
		fmt.Fprintf(out, "Recent Success Rate:  %.2f%% (last 30 days)\n", stats.RecentSuccessRate*100)
		fmt.Fprintf(out, "Trending:             %s\n", stats.Trending)
		fmt.Fprintf(out, "\n")
	}

	// Target type breakdown
	if len(stats.TargetTypeBreakdown) > 0 {
		fmt.Fprintf(out, "TARGET TYPE BREAKDOWN\n")
		fmt.Fprintf(out, "-------------------------------------\n")
		for targetType, breakdown := range stats.TargetTypeBreakdown {
			fmt.Fprintf(out, "\n%s:\n", targetType)
			fmt.Fprintf(out, "  Executions:         %d\n", breakdown.Executions)
			fmt.Fprintf(out, "  Successes:          %d\n", breakdown.Successes)
			fmt.Fprintf(out, "  Success Rate:       %.2f%%\n", breakdown.SuccessRate*100)
			if breakdown.AverageDuration > 0 {
				fmt.Fprintf(out, "  Average Duration:   %s\n", breakdown.AverageDuration)
			}
			if breakdown.AverageConfidence > 0 {
				fmt.Fprintf(out, "  Average Confidence: %.2f%%\n", breakdown.AverageConfidence*100)
			}
		}
		fmt.Fprintf(out, "\n")
	}

	if stats.TotalExecutions == 0 {
		fmt.Fprintf(out, "No execution data available for this payload.\n")
	}

	return nil
}

// runCategoryStats shows statistics for a category
func runCategoryStats(
	cmd *cobra.Command,
	ctx context.Context,
	tracker payload.EffectivenessTracker,
	formatter internal.Formatter,
	outputFormat internal.OutputFormat,
) error {
	// Parse category
	category := payload.PayloadCategory(statsPayloadCategory)
	if !category.IsValid() {
		return fmt.Errorf("invalid category: %s (valid: %s)", statsPayloadCategory, getValidCategories())
	}

	// Get category stats
	stats, err := tracker.GetCategoryStats(ctx, category)
	if err != nil {
		return fmt.Errorf("failed to get category stats: %w", err)
	}

	// Handle JSON output
	if outputFormat == internal.FormatJSON {
		return formatter.PrintJSON(stats)
	}

	// Text output
	out := cmd.OutOrStdout()

	fmt.Fprintf(out, "Category Statistics: %s\n", stats.Category)
	fmt.Fprintf(out, "\n")

	// Payload counts
	fmt.Fprintf(out, "PAYLOAD SUMMARY\n")
	fmt.Fprintf(out, "-------------------------------------\n")
	fmt.Fprintf(out, "Total Payloads:       %d\n", stats.TotalPayloads)
	fmt.Fprintf(out, "Enabled Payloads:     %d\n", stats.EnabledPayloads)
	fmt.Fprintf(out, "\n")

	// Execution counts
	fmt.Fprintf(out, "EXECUTION SUMMARY\n")
	fmt.Fprintf(out, "-------------------------------------\n")
	fmt.Fprintf(out, "Total Executions:     %d\n", stats.TotalExecutions)
	fmt.Fprintf(out, "Successful Attacks:   %d\n", stats.SuccessfulAttacks)
	fmt.Fprintf(out, "Failed Executions:    %d\n", stats.FailedExecutions)
	fmt.Fprintf(out, "\n")

	// Aggregate metrics
	if stats.TotalExecutions > 0 {
		fmt.Fprintf(out, "AGGREGATE METRICS\n")
		fmt.Fprintf(out, "-------------------------------------\n")
		fmt.Fprintf(out, "Success Rate:         %.2f%%\n", stats.SuccessRate*100)
		fmt.Fprintf(out, "Average Duration:     %s\n", stats.AverageDuration)
		if stats.AverageConfidence > 0 {
			fmt.Fprintf(out, "Average Confidence:   %.2f%%\n", stats.AverageConfidence*100)
		}
		if stats.TotalCost > 0 {
			fmt.Fprintf(out, "Total Cost:           $%.4f\n", stats.TotalCost)
		}
		fmt.Fprintf(out, "\n")
	}

	// Finding metrics
	if stats.FindingsCreated > 0 {
		fmt.Fprintf(out, "FINDING METRICS\n")
		fmt.Fprintf(out, "-------------------------------------\n")
		fmt.Fprintf(out, "Findings Created:     %d\n", stats.FindingsCreated)
		fmt.Fprintf(out, "Finding Rate:         %.2f%%\n", stats.FindingRate*100)
		fmt.Fprintf(out, "\n")
	}

	// Top performing payloads
	if len(stats.TopPayloads) > 0 {
		fmt.Fprintf(out, "TOP PERFORMING PAYLOADS\n")
		fmt.Fprintf(out, "-------------------------------------\n")
		for i, ps := range stats.TopPayloads {
			fmt.Fprintf(out, "%d. %s\n", i+1, ps.PayloadName)
			fmt.Fprintf(out, "   Success Rate: %.2f%% (%d executions)\n", ps.SuccessRate*100, ps.TotalExecutions)
			fmt.Fprintf(out, "   Confidence:   %.2f%%\n", ps.ConfidenceLevel*100)
		}
		fmt.Fprintf(out, "\n")
	}

	if stats.TotalExecutions == 0 {
		fmt.Fprintf(out, "No execution data available for this category.\n")
	}

	return nil
}

// Import/Export commands

var payloadImportCmd = &cobra.Command{
	Use:   "import FILE",
	Short: "Import payloads from a file",
	Long: `Import one or more payloads from a YAML or JSON file.

The file can contain a single payload or an array of payloads.

Examples:
  # Import from YAML file
  gibson payload import payloads.yaml

  # Import from JSON file
  gibson payload import payloads.json`,
	Args: cobra.ExactArgs(1),
	RunE: runPayloadImport,
}

var payloadExportCmd = &cobra.Command{
	Use:   "export ID",
	Short: "Export a payload to a file",
	Long: `Export a specific payload to a YAML or JSON file.

Examples:
  # Export to YAML file
  gibson payload export payload_abc123 --output payload.yaml

  # Export to JSON file
  gibson payload export payload_abc123 --output payload.json`,
	Args: cobra.ExactArgs(1),
	RunE: runPayloadExport,
}

// Flags for import/export commands
var (
	exportPayloadOutput string
)

func init() {
	// Export command flags
	payloadExportCmd.Flags().StringVar(&exportPayloadOutput, "output", "", "Output file path (required)")
	payloadExportCmd.MarkFlagRequired("output")
}

// runPayloadImport executes the payload import command
func runPayloadImport(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	filePath := args[0]

	// Read and parse file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create payload store
	store := payload.NewPayloadStore(db)

	// Try to parse as array first, then as single payload
	var payloads []*payload.Payload

	// Determine format from file extension
	isYAML := strings.HasSuffix(filePath, ".yaml") || strings.HasSuffix(filePath, ".yml")
	isJSON := strings.HasSuffix(filePath, ".json")

	if isYAML {
		// Try array first
		if err := yaml.Unmarshal(data, &payloads); err != nil {
			// Try single payload
			var singlePayload payload.Payload
			if err := yaml.Unmarshal(data, &singlePayload); err != nil {
				return fmt.Errorf("failed to parse YAML: %w", err)
			}
			payloads = []*payload.Payload{&singlePayload}
		}
	} else if isJSON {
		// Try array first
		if err := json.Unmarshal(data, &payloads); err != nil {
			// Try single payload
			var singlePayload payload.Payload
			if err := json.Unmarshal(data, &singlePayload); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			payloads = []*payload.Payload{&singlePayload}
		}
	} else {
		// Try JSON first, then YAML
		if err := json.Unmarshal(data, &payloads); err != nil {
			if err := yaml.Unmarshal(data, &payloads); err != nil {
				// Try single payload JSON
				var singlePayload payload.Payload
				if err := json.Unmarshal(data, &singlePayload); err != nil {
					// Try single payload YAML
					if err := yaml.Unmarshal(data, &singlePayload); err != nil {
						return fmt.Errorf("failed to parse file as JSON or YAML")
					}
					payloads = []*payload.Payload{&singlePayload}
				} else {
					payloads = []*payload.Payload{&singlePayload}
				}
			}
		}
	}

	if len(payloads) == 0 {
		return fmt.Errorf("no payloads found in file")
	}

	// Import each payload
	imported := 0
	skipped := 0
	updated := 0
	var errors []string

	for _, p := range payloads {
		// Validate payload
		if err := validatePayload(p); err != nil {
			errors = append(errors, fmt.Sprintf("Validation failed for %s: %v", p.Name, err))
			skipped++
			continue
		}

		// Check if payload already exists
		var existingPayload *payload.Payload
		if !p.ID.IsZero() {
			existingPayload, _ = store.Get(ctx, p.ID)
		}

		if existingPayload != nil {
			// Payload exists - ask user or skip
			// For now, we'll update it
			p.UpdatedAt = time.Now()
			if err := store.Update(ctx, p); err != nil {
				errors = append(errors, fmt.Sprintf("Failed to update %s: %v", p.Name, err))
				skipped++
				continue
			}
			updated++
		} else {
			// New payload
			if p.ID.IsZero() {
				p.ID = types.NewID()
			}
			now := time.Now()
			p.CreatedAt = now
			p.UpdatedAt = now
			p.BuiltIn = false
			p.Enabled = true

			if err := store.Save(ctx, p); err != nil {
				errors = append(errors, fmt.Sprintf("Failed to save %s: %v", p.Name, err))
				skipped++
				continue
			}
			imported++
		}
	}

	// Print summary
	fmt.Fprintf(cmd.OutOrStdout(), "Import complete:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Imported: %d new payloads\n", imported)
	if updated > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Updated:  %d existing payloads\n", updated)
	}
	if skipped > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Skipped:  %d payloads (errors)\n", skipped)
	}

	if len(errors) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nErrors:\n")
		for _, errMsg := range errors {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", errMsg)
		}
	}

	return nil
}

// runPayloadExport executes the payload export command
func runPayloadExport(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	payloadID := args[0]

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create payload store
	store := payload.NewPayloadStore(db)

	// Parse payload ID
	id, err := types.ParseID(payloadID)
	if err != nil {
		return fmt.Errorf("invalid payload ID: %w", err)
	}

	// Get payload
	p, err := store.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get payload: %w", err)
	}
	if p == nil {
		return fmt.Errorf("payload not found: %s", payloadID)
	}

	// Determine format from output file extension
	isYAML := strings.HasSuffix(exportPayloadOutput, ".yaml") || strings.HasSuffix(exportPayloadOutput, ".yml")
	isJSON := strings.HasSuffix(exportPayloadOutput, ".json")

	var data []byte

	if isYAML {
		// Export as YAML
		data, err = yaml.Marshal(p)
		if err != nil {
			return fmt.Errorf("failed to marshal YAML: %w", err)
		}
	} else if isJSON {
		// Export as JSON
		data, err = json.MarshalIndent(p, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
	} else {
		// Default to JSON
		data, err = json.MarshalIndent(p, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
	}

	// Write to file
	if err := os.WriteFile(exportPayloadOutput, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Successfully exported payload to: %s\n", exportPayloadOutput)

	return nil
}

// Search command

var payloadSearchCmd = &cobra.Command{
	Use:   "search QUERY",
	Short: "Search payloads using full-text search",
	Long: `Search payloads using full-text search across name, description, and template.

Examples:
  # Search for jailbreak payloads
  gibson payload search jailbreak

  # Search with category filter
  gibson payload search "system prompt" --category data_extraction

  # Search with severity filter
  gibson payload search bypass --severity high

  # Limit results
  gibson payload search injection --limit 10`,
	Args: cobra.ExactArgs(1),
	RunE: runPayloadSearch,
}

// Flags for search command
var (
	searchPayloadCategory string
	searchPayloadSeverity string
	searchPayloadLimit    int
	searchPayloadOutput   string
)

func init() {
	// Search command flags
	payloadSearchCmd.Flags().StringVar(&searchPayloadCategory, "category", "", "Filter by category")
	payloadSearchCmd.Flags().StringVar(&searchPayloadSeverity, "severity", "", "Filter by severity")
	payloadSearchCmd.Flags().IntVar(&searchPayloadLimit, "limit", 50, "Maximum number of results")
	payloadSearchCmd.Flags().StringVar(&searchPayloadOutput, "output", "text", "Output format (text, json)")
}

// runPayloadSearch executes the payload search command
func runPayloadSearch(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	query := args[0]

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create payload registry
	registry := payload.NewPayloadRegistry(db, payload.DefaultRegistryConfig())

	// Build filter
	filter := &payload.PayloadFilter{}

	if searchPayloadCategory != "" {
		category := payload.PayloadCategory(searchPayloadCategory)
		if !category.IsValid() {
			return fmt.Errorf("invalid category: %s (valid: %s)", searchPayloadCategory, getValidCategories())
		}
		filter.Categories = []payload.PayloadCategory{category}
	}

	if searchPayloadSeverity != "" {
		severity := agent.FindingSeverity(searchPayloadSeverity)
		if !isValidSeverity(severity) {
			return fmt.Errorf("invalid severity: %s (valid: critical, high, medium, low, info)", searchPayloadSeverity)
		}
		filter.Severities = []agent.FindingSeverity{severity}
	}

	// Perform search
	payloads, err := registry.Search(ctx, query, filter)
	if err != nil {
		return fmt.Errorf("failed to search payloads: %w", err)
	}

	// Apply limit
	if searchPayloadLimit > 0 && len(payloads) > searchPayloadLimit {
		payloads = payloads[:searchPayloadLimit]
	}

	// Determine output format
	outputFormat := internal.FormatText
	if searchPayloadOutput == "json" {
		outputFormat = internal.FormatJSON
	}

	formatter := internal.NewFormatter(outputFormat, cmd.OutOrStdout())

	// Handle JSON output
	if outputFormat == internal.FormatJSON {
		output := map[string]interface{}{
			"query":    query,
			"count":    len(payloads),
			"payloads": payloads,
		}
		return formatter.PrintJSON(output)
	}

	// Text output
	if len(payloads) == 0 {
		return formatter.PrintError(fmt.Sprintf("No payloads found matching query: %s", query))
	}

	// Print summary
	fmt.Fprintf(cmd.OutOrStdout(), "Found %d payload(s) matching: %s\n\n", len(payloads), query)

	// Build table
	headers := []string{"ID", "Name", "Categories", "Severity", "Built-In"}
	rows := make([][]string, 0, len(payloads))

	for _, p := range payloads {
		// Format categories
		categories := make([]string, len(p.Categories))
		for i, cat := range p.Categories {
			categories[i] = string(cat)
		}
		categoryStr := strings.Join(categories, ", ")
		if len(categoryStr) > 30 {
			categoryStr = categoryStr[:27] + "..."
		}

		// Format built-in status
		builtInStr := ""
		if p.BuiltIn {
			builtInStr = "yes"
		}

		rows = append(rows, []string{
			p.ID.String(),
			truncateString(p.Name, 40),
			categoryStr,
			string(p.Severity),
			builtInStr,
		})
	}

	// Print table
	if err := formatter.PrintTable(headers, rows); err != nil {
		return fmt.Errorf("failed to print table: %w", err)
	}

	return nil
}
