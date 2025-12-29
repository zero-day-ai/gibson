package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

var targetCmd = &cobra.Command{
	Use:   "target",
	Short: "Manage attack targets",
	Long:  `Manage LLM targets for red-team testing operations`,
}

var targetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all targets",
	Long:  `List all configured targets with optional filtering`,
	RunE:  runTargetList,
}

var targetAddCmd = &cobra.Command{
	Use:   "add URL",
	Short: "Add a new target",
	Long:  `Add a new target with auto-detection of settings from URL`,
	Args:  cobra.ExactArgs(1),
	RunE:  runTargetAdd,
}

var targetShowCmd = &cobra.Command{
	Use:   "show NAME",
	Short: "Show detailed target information",
	Long:  `Display detailed information about a specific target`,
	Args:  cobra.ExactArgs(1),
	RunE:  runTargetShow,
}

var targetTestCmd = &cobra.Command{
	Use:   "test NAME",
	Short: "Test target connectivity",
	Long:  `Test connectivity and authentication to a target`,
	Args:  cobra.ExactArgs(1),
	RunE:  runTargetTest,
}

var targetDeleteCmd = &cobra.Command{
	Use:   "delete NAME",
	Short: "Delete a target",
	Long:  `Delete a target from the database`,
	Args:  cobra.ExactArgs(1),
	RunE:  runTargetDelete,
}

// Flags for target list
var (
	listStatusFilter   string
	listProviderFilter string
)

// Flags for target add
var (
	addName       string
	addType       string
	addProvider   string
	addCredential string
	addModel      string
	addTimeout    int
)

// Flags for target delete
var (
	deleteForce bool
)

func init() {
	// List command flags
	targetListCmd.Flags().StringVar(&listStatusFilter, "status", "", "Filter by status (active, inactive, error)")
	targetListCmd.Flags().StringVar(&listProviderFilter, "provider", "", "Filter by provider (openai, anthropic, google, azure, ollama, custom)")

	// Add command flags
	targetAddCmd.Flags().StringVar(&addName, "name", "", "Human-readable name for the target (required)")
	targetAddCmd.Flags().StringVar(&addType, "type", "llm_api", "Target type (llm_chat, llm_api, rag, agent, embedding, multimodal)")
	targetAddCmd.Flags().StringVar(&addProvider, "provider", "", "Provider (openai, anthropic, google, azure, ollama, custom)")
	targetAddCmd.Flags().StringVar(&addCredential, "credential", "", "Credential name to use for authentication")
	targetAddCmd.Flags().StringVar(&addModel, "model", "", "Model identifier (e.g., gpt-4, claude-3)")
	targetAddCmd.Flags().IntVar(&addTimeout, "timeout", 30, "Request timeout in seconds")
	targetAddCmd.MarkFlagRequired("name")

	// Delete command flags
	targetDeleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Skip confirmation prompt")

	// Add subcommands
	targetCmd.AddCommand(targetListCmd)
	targetCmd.AddCommand(targetAddCmd)
	targetCmd.AddCommand(targetShowCmd)
	targetCmd.AddCommand(targetTestCmd)
	targetCmd.AddCommand(targetDeleteCmd)
}

// runTargetList executes the target list command
func runTargetList(cmd *cobra.Command, args []string) error {
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

	// Create filter
	filter := types.NewTargetFilter()

	if listStatusFilter != "" {
		status := types.TargetStatus(listStatusFilter)
		if !status.IsValid() {
			return fmt.Errorf("invalid status: %s", listStatusFilter)
		}
		filter.WithStatus(status)
	}

	if listProviderFilter != "" {
		provider := types.Provider(listProviderFilter)
		if !provider.IsValid() {
			return fmt.Errorf("invalid provider: %s", listProviderFilter)
		}
		filter.WithProvider(provider)
	}

	// Query targets
	dao := database.NewTargetDAO(db)
	targets, err := dao.List(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to list targets: %w", err)
	}

	if len(targets) == 0 {
		cmd.Println("No targets found.")
		return nil
	}

	// Display results in table format
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tPROVIDER\tSTATUS\tURL")
	fmt.Fprintln(w, "----\t----\t--------\t------\t---")

	for _, target := range targets {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			target.Name,
			target.Type,
			target.Provider,
			target.Status,
			target.URL,
		)
	}

	w.Flush()
	return nil
}

// runTargetAdd executes the target add command
func runTargetAdd(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	targetURL := args[0]

	// Validate URL format
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if parsedURL.Scheme == "" {
		return fmt.Errorf("URL must include scheme (http:// or https://)")
	}

	if parsedURL.Host == "" {
		return fmt.Errorf("URL must include host")
	}

	// Auto-detect provider if not specified
	if addProvider == "" {
		addProvider = detectProvider(parsedURL)
	}

	// Auto-detect model if not specified
	if addModel == "" {
		addModel = detectModel(parsedURL, addProvider)
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

	dao := database.NewTargetDAO(db)

	// Check if target name already exists
	exists, err := dao.ExistsByName(ctx, addName)
	if err != nil {
		return fmt.Errorf("failed to check target name: %w", err)
	}
	if exists {
		return fmt.Errorf("target with name '%s' already exists", addName)
	}

	// Create target
	target := types.NewTarget(addName, targetURL, types.TargetType(addType))
	target.Provider = types.Provider(addProvider)
	target.Model = addModel
	target.Timeout = addTimeout

	// Validate target type
	if !target.Type.IsValid() {
		return fmt.Errorf("invalid target type: %s", addType)
	}

	// Validate provider
	if !target.Provider.IsValid() {
		return fmt.Errorf("invalid provider: %s", addProvider)
	}

	// Handle credential if specified
	if addCredential != "" {
		// Look up credential by name
		credDAO := database.NewCredentialDAO(db)
		cred, err := credDAO.GetByName(ctx, addCredential)
		if err != nil {
			return fmt.Errorf("failed to find credential '%s': %w", addCredential, err)
		}
		target.CredentialID = &cred.ID

		// Set auth type based on credential type
		switch cred.Type {
		case types.CredentialTypeAPIKey:
			target.AuthType = types.AuthTypeAPIKey
		case types.CredentialTypeBearer:
			target.AuthType = types.AuthTypeBearer
		case types.CredentialTypeBasic:
			target.AuthType = types.AuthTypeBasic
		case types.CredentialTypeOAuth:
			target.AuthType = types.AuthTypeOAuth
		default:
			target.AuthType = types.AuthTypeNone
		}
	} else {
		target.AuthType = types.AuthTypeNone
	}

	// Save to database
	if err := dao.Create(ctx, target); err != nil {
		return fmt.Errorf("failed to create target: %w", err)
	}

	cmd.Printf("Target '%s' created successfully (ID: %s)\n", target.Name, target.ID)
	return nil
}

// runTargetShow executes the target show command
func runTargetShow(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	targetName := args[0]

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

	// Get target
	dao := database.NewTargetDAO(db)
	target, err := dao.GetByName(ctx, targetName)
	if err != nil {
		return fmt.Errorf("failed to get target: %w", err)
	}

	// Display target details
	cmd.Printf("Target: %s\n", target.Name)
	cmd.Printf("ID: %s\n", target.ID)
	cmd.Printf("Type: %s\n", target.Type)
	cmd.Printf("Provider: %s\n", target.Provider)
	cmd.Printf("URL: %s\n", target.URL)
	cmd.Printf("Model: %s\n", target.Model)
	cmd.Printf("Status: %s\n", target.Status)
	cmd.Printf("Auth Type: %s\n", target.AuthType)

	if target.CredentialID != nil {
		cmd.Printf("Credential ID: %s\n", target.CredentialID)
	}

	if target.Description != "" {
		cmd.Printf("Description: %s\n", target.Description)
	}

	cmd.Printf("Timeout: %d seconds\n", target.Timeout)

	if len(target.Tags) > 0 {
		cmd.Printf("Tags: %s\n", strings.Join(target.Tags, ", "))
	}

	if len(target.Headers) > 0 {
		cmd.Println("\nCustom Headers:")
		for k, v := range target.Headers {
			cmd.Printf("  %s: %s\n", k, v)
		}
	}

	if len(target.Config) > 0 {
		cmd.Println("\nConfiguration:")
		configJSON, _ := json.MarshalIndent(target.Config, "  ", "  ")
		cmd.Printf("  %s\n", string(configJSON))
	}

	if len(target.Capabilities) > 0 {
		cmd.Printf("\nCapabilities: %s\n", strings.Join(target.Capabilities, ", "))
	}

	cmd.Printf("\nCreated: %s\n", target.CreatedAt.Format(time.RFC3339))
	cmd.Printf("Updated: %s\n", target.UpdatedAt.Format(time.RFC3339))

	return nil
}

// runTargetTest executes the target test command
func runTargetTest(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	targetName := args[0]

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

	// Get target
	dao := database.NewTargetDAO(db)
	target, err := dao.GetByName(ctx, targetName)
	if err != nil {
		return fmt.Errorf("failed to get target: %w", err)
	}

	cmd.Printf("Testing connectivity to %s...\n", target.Name)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: time.Duration(target.Timeout) * time.Second,
	}

	// Create a simple OPTIONS request to test connectivity
	req, err := http.NewRequestWithContext(ctx, "OPTIONS", target.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add custom headers
	for k, v := range target.Headers {
		req.Header.Set(k, v)
	}

	// Perform request
	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)

	if err != nil {
		cmd.Printf("Failed: %v\n", err)

		// Update target status to error
		target.Status = types.TargetStatusError
		dao.Update(ctx, target)

		return fmt.Errorf("connectivity test failed")
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode >= 200 && resp.StatusCode < 500 {
		cmd.Printf("Success: Connected in %v (Status: %d)\n", duration, resp.StatusCode)

		// Update target status to active
		if target.Status != types.TargetStatusActive {
			target.Status = types.TargetStatusActive
			dao.Update(ctx, target)
		}

		return nil
	}

	cmd.Printf("Warning: Unexpected status code %d (took %v)\n", resp.StatusCode, duration)
	return nil
}

// runTargetDelete executes the target delete command
func runTargetDelete(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	targetName := args[0]

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

	// Get target
	dao := database.NewTargetDAO(db)
	target, err := dao.GetByName(ctx, targetName)
	if err != nil {
		return fmt.Errorf("failed to get target: %w", err)
	}

	// Confirm deletion unless --force is set
	if !deleteForce {
		cmd.Printf("Are you sure you want to delete target '%s'? (y/N): ", targetName)
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			cmd.Println("Deletion cancelled.")
			return nil
		}
	}

	// Delete target
	if err := dao.Delete(ctx, target.ID); err != nil {
		return fmt.Errorf("failed to delete target: %w", err)
	}

	cmd.Printf("Target '%s' deleted successfully.\n", targetName)
	return nil
}

// detectProvider attempts to auto-detect the provider from the URL
func detectProvider(u *url.URL) string {
	host := strings.ToLower(u.Host)

	if strings.Contains(host, "openai.com") || strings.Contains(host, "api.openai.com") {
		return "openai"
	}
	if strings.Contains(host, "anthropic.com") || strings.Contains(host, "api.anthropic.com") {
		return "anthropic"
	}
	if strings.Contains(host, "googleapis.com") || strings.Contains(host, "generativelanguage.googleapis.com") {
		return "google"
	}
	if strings.Contains(host, "azure.com") || strings.Contains(host, "openai.azure.com") {
		return "azure"
	}
	if strings.Contains(host, "localhost") || strings.Contains(host, "127.0.0.1") {
		return "ollama"
	}

	return "custom"
}

// detectModel attempts to auto-detect the model from the URL and provider
func detectModel(u *url.URL, provider string) string {
	path := strings.ToLower(u.Path)

	// Try to extract model from path
	if strings.Contains(path, "gpt-4") {
		return "gpt-4"
	}
	if strings.Contains(path, "gpt-3.5") {
		return "gpt-3.5-turbo"
	}
	if strings.Contains(path, "claude-3") {
		return "claude-3-opus"
	}
	if strings.Contains(path, "gemini") {
		return "gemini-pro"
	}

	// Default models by provider
	switch provider {
	case "openai":
		return "gpt-4"
	case "anthropic":
		return "claude-3-opus"
	case "google":
		return "gemini-pro"
	case "ollama":
		return "llama2"
	default:
		return ""
	}
}

// getGibsonHome returns the Gibson home directory
func getGibsonHome() (string, error) {
	homeDir := os.Getenv("GIBSON_HOME")
	if homeDir == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		homeDir = userHome + "/.gibson"
	}
	return homeDir, nil
}
