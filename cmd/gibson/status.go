package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/internal"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/types"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Display system health and status",
	Long: `Display overall system status including:
  - Running agents and plugins with port/PID information
  - Database connectivity status
  - Configured LLM providers with health status
  - Overall system health assessment`,
	RunE: runStatus,
}

// SystemStatus represents the complete system status
type SystemStatus struct {
	OverallHealth types.HealthStatus  `json:"overall_health"`
	Components    ComponentsStatus    `json:"components"`
	Database      DatabaseStatus      `json:"database"`
	LLMProviders  []LLMProviderStatus `json:"llm_providers"`
	CheckedAt     time.Time           `json:"checked_at"`
}

// ComponentsStatus represents the status of all components
type ComponentsStatus struct {
	Agents  []ComponentInfo `json:"agents"`
	Tools   []ComponentInfo `json:"tools"`
	Plugins []ComponentInfo `json:"plugins"`
}

// ComponentInfo represents information about a single component
type ComponentInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Port   int    `json:"port,omitempty"`
	PID    int    `json:"pid,omitempty"`
}

// DatabaseStatus represents database health information
type DatabaseStatus struct {
	Connected bool   `json:"connected"`
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Error     string `json:"error,omitempty"`
}

// LLMProviderStatus represents LLM provider health information
type LLMProviderStatus struct {
	Name         string             `json:"name"`
	Configured   bool               `json:"configured"`
	HealthStatus types.HealthStatus `json:"health_status"`
}

func init() {
	// Add --json flag for structured output
	statusCmd.Flags().Bool("json", false, "Output status in JSON format")
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Parse global flags
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return err
	}

	// Determine home directory
	homeDir := flags.HomeDir
	if homeDir == "" {
		homeDir = os.Getenv("GIBSON_HOME")
	}
	if homeDir == "" {
		homeDir = config.DefaultHomeDir()
	}

	// Check if JSON output is requested
	jsonOutput, _ := cmd.Flags().GetBool("json")
	format := internal.FormatText
	if jsonOutput {
		format = internal.FormatJSON
	}

	// Create formatter
	formatter := internal.NewFormatter(format, cmd.OutOrStdout())

	// Collect system status
	status := collectSystemStatus(ctx, homeDir)

	// Output status
	if format == internal.FormatJSON {
		return formatter.PrintJSON(status)
	}

	// Text output
	return printTextStatus(formatter, status)
}

// collectSystemStatus collects status from all subsystems
func collectSystemStatus(ctx context.Context, homeDir string) SystemStatus {
	status := SystemStatus{
		CheckedAt: time.Now(),
	}

	// Check components
	status.Components = checkComponentsStatus(homeDir)

	// Check database
	status.Database = checkDatabaseStatus(ctx, homeDir)

	// Check LLM providers
	status.LLMProviders = checkLLMProviders(ctx, homeDir)

	// Determine overall health
	status.OverallHealth = determineOverallHealth(status)

	return status
}

// checkComponentsStatus checks the status of all components
func checkComponentsStatus(homeDir string) ComponentsStatus {
	componentStatus := ComponentsStatus{
		Agents:  []ComponentInfo{},
		Tools:   []ComponentInfo{},
		Plugins: []ComponentInfo{},
	}

	// Open database connection
	dbPath := filepath.Join(homeDir, "gibson.db")
	db, err := database.Open(dbPath)
	if err != nil {
		// Database may not exist yet, return empty status
		return componentStatus
	}
	defer db.Close()

	// Create component DAO
	dao := database.NewComponentDAO(db)
	ctx := context.Background()

	// Get all components
	allComponents, err := dao.ListAll(ctx)
	if err != nil {
		// Error listing components, return empty status
		return componentStatus
	}

	// Collect agents
	for _, comp := range allComponents[component.ComponentKindAgent] {
		componentStatus.Agents = append(componentStatus.Agents, ComponentInfo{
			Name:   comp.Name,
			Status: comp.Status.String(),
			Port:   comp.Port,
			PID:    comp.PID,
		})
	}

	// Collect tools
	for _, comp := range allComponents[component.ComponentKindTool] {
		componentStatus.Tools = append(componentStatus.Tools, ComponentInfo{
			Name:   comp.Name,
			Status: comp.Status.String(),
			Port:   comp.Port,
			PID:    comp.PID,
		})
	}

	// Collect plugins
	for _, comp := range allComponents[component.ComponentKindPlugin] {
		componentStatus.Plugins = append(componentStatus.Plugins, ComponentInfo{
			Name:   comp.Name,
			Status: comp.Status.String(),
			Port:   comp.Port,
			PID:    comp.PID,
		})
	}

	return componentStatus
}

// checkDatabaseStatus checks database connectivity and status
func checkDatabaseStatus(ctx context.Context, homeDir string) DatabaseStatus {
	dbStatus := DatabaseStatus{
		Connected: false,
	}

	// Determine database path
	dbPath := filepath.Join(homeDir, "gibson.db")
	dbStatus.Path = dbPath

	// Check if database file exists
	info, err := os.Stat(dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			dbStatus.Error = "database file not found (run 'gibson init' to initialize)"
		} else {
			dbStatus.Error = fmt.Sprintf("failed to stat database: %v", err)
		}
		return dbStatus
	}

	dbStatus.Size = info.Size()

	// Try to connect to database
	db, err := database.Open(dbPath)
	if err != nil {
		dbStatus.Error = fmt.Sprintf("failed to open database: %v", err)
		return dbStatus
	}
	defer db.Close()

	// Test connection with health check
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.Health(healthCtx); err != nil {
		dbStatus.Error = fmt.Sprintf("health check failed: %v", err)
		return dbStatus
	}

	dbStatus.Connected = true
	return dbStatus
}

// checkLLMProviders checks configured LLM providers and their health
func checkLLMProviders(ctx context.Context, homeDir string) []LLMProviderStatus {
	providers := []LLMProviderStatus{}

	// Determine config file path
	configFile := config.DefaultConfigPath(homeDir)

	// Load configuration
	loader := config.NewConfigLoader(config.NewValidator())
	cfg, err := loader.LoadWithDefaults(configFile)
	if err != nil {
		// Config may not exist yet
		return providers
	}

	// Create LLM registry (in real implementation, this would be injected)
	registry := llm.NewLLMRegistry()

	// Check if default provider is configured
	if cfg.LLM.DefaultProvider != "" {
		// Try to get provider from registry
		provider, err := registry.GetProvider(cfg.LLM.DefaultProvider)
		if err != nil {
			// Provider not registered, add as unconfigured
			providers = append(providers, LLMProviderStatus{
				Name:         cfg.LLM.DefaultProvider,
				Configured:   false,
				HealthStatus: types.Unhealthy("provider not registered"),
			})
		} else {
			// Check provider health
			health := provider.Health(ctx)
			providers = append(providers, LLMProviderStatus{
				Name:         cfg.LLM.DefaultProvider,
				Configured:   true,
				HealthStatus: health,
			})
		}
	}

	// If no providers configured, note that
	if len(providers) == 0 {
		providers = append(providers, LLMProviderStatus{
			Name:         "none",
			Configured:   false,
			HealthStatus: types.Unhealthy("no LLM providers configured"),
		})
	}

	return providers
}

// determineOverallHealth determines overall system health based on subsystems
func determineOverallHealth(status SystemStatus) types.HealthStatus {
	// Count health states
	healthyCount := 0
	unhealthyCount := 0
	issues := []string{}

	// Check database
	if !status.Database.Connected {
		unhealthyCount++
		issues = append(issues, "database unavailable")
	} else {
		healthyCount++
	}

	// Check LLM providers
	hasHealthyProvider := false
	for _, provider := range status.LLMProviders {
		if provider.Configured && provider.HealthStatus.IsHealthy() {
			hasHealthyProvider = true
			break
		}
	}
	if !hasHealthyProvider {
		unhealthyCount++
		issues = append(issues, "no healthy LLM providers")
	} else {
		healthyCount++
	}

	// Check for running components
	totalComponents := len(status.Components.Agents) +
		len(status.Components.Tools) +
		len(status.Components.Plugins)

	runningComponents := 0
	for _, agent := range status.Components.Agents {
		if agent.Status == component.ComponentStatusRunning.String() {
			runningComponents++
		}
	}
	for _, tool := range status.Components.Tools {
		if tool.Status == component.ComponentStatusRunning.String() {
			runningComponents++
		}
	}
	for _, plugin := range status.Components.Plugins {
		if plugin.Status == component.ComponentStatusRunning.String() {
			runningComponents++
		}
	}

	// Determine overall status
	if unhealthyCount == 0 {
		if totalComponents == 0 {
			return types.Healthy("system initialized, no components installed")
		}
		if runningComponents == 0 {
			return types.Degraded("system healthy, no components running")
		}
		return types.Healthy(fmt.Sprintf("all systems operational (%d/%d components running)",
			runningComponents, totalComponents))
	} else if healthyCount > 0 {
		return types.Degraded(fmt.Sprintf("system degraded: %v", issues))
	} else {
		return types.Unhealthy(fmt.Sprintf("system unhealthy: %v", issues))
	}
}

// printTextStatus prints status in human-readable text format
func printTextStatus(formatter internal.Formatter, status SystemStatus) error {
	// Print overall health
	healthSymbol := "✓"
	if status.OverallHealth.IsDegraded() {
		healthSymbol = "⚠"
	} else if status.OverallHealth.IsUnhealthy() {
		healthSymbol = "✗"
	}

	fmt.Printf("\n%s Overall Status: %s\n", healthSymbol, status.OverallHealth.State)
	if status.OverallHealth.Message != "" {
		fmt.Printf("  %s\n", status.OverallHealth.Message)
	}
	fmt.Println()

	// Print database status
	fmt.Println("Database:")
	if status.Database.Connected {
		fmt.Printf("  ✓ Connected: %s\n", status.Database.Path)
		fmt.Printf("    Size: %d bytes\n", status.Database.Size)
	} else {
		fmt.Printf("  ✗ Not connected\n")
		if status.Database.Error != "" {
			fmt.Printf("    Error: %s\n", status.Database.Error)
		}
	}
	fmt.Println()

	// Print LLM providers
	fmt.Println("LLM Providers:")
	if len(status.LLMProviders) == 0 {
		fmt.Println("  No providers configured")
	} else {
		for _, provider := range status.LLMProviders {
			symbol := "✓"
			if !provider.Configured {
				symbol = "✗"
			} else if provider.HealthStatus.IsUnhealthy() {
				symbol = "✗"
			} else if provider.HealthStatus.IsDegraded() {
				symbol = "⚠"
			}

			fmt.Printf("  %s %s: %s\n", symbol, provider.Name, provider.HealthStatus.State)
			if provider.HealthStatus.Message != "" {
				fmt.Printf("    %s\n", provider.HealthStatus.Message)
			}
		}
	}
	fmt.Println()

	// Print components
	if len(status.Components.Agents) > 0 {
		fmt.Println("Agents:")
		headers := []string{"Name", "Status", "Port", "PID"}
		rows := [][]string{}
		for _, agent := range status.Components.Agents {
			port := "-"
			pid := "-"
			if agent.Port > 0 {
				port = fmt.Sprintf("%d", agent.Port)
			}
			if agent.PID > 0 {
				pid = fmt.Sprintf("%d", agent.PID)
			}
			rows = append(rows, []string{agent.Name, agent.Status, port, pid})
		}
		formatter.PrintTable(headers, rows)
		fmt.Println()
	}

	if len(status.Components.Tools) > 0 {
		fmt.Println("Tools:")
		headers := []string{"Name", "Status", "Port", "PID"}
		rows := [][]string{}
		for _, tool := range status.Components.Tools {
			port := "-"
			pid := "-"
			if tool.Port > 0 {
				port = fmt.Sprintf("%d", tool.Port)
			}
			if tool.PID > 0 {
				pid = fmt.Sprintf("%d", tool.PID)
			}
			rows = append(rows, []string{tool.Name, tool.Status, port, pid})
		}
		formatter.PrintTable(headers, rows)
		fmt.Println()
	}

	if len(status.Components.Plugins) > 0 {
		fmt.Println("Plugins:")
		headers := []string{"Name", "Status", "Port", "PID"}
		rows := [][]string{}
		for _, plugin := range status.Components.Plugins {
			port := "-"
			pid := "-"
			if plugin.Port > 0 {
				port = fmt.Sprintf("%d", plugin.Port)
			}
			if plugin.PID > 0 {
				pid = fmt.Sprintf("%d", plugin.PID)
			}
			rows = append(rows, []string{plugin.Name, plugin.Status, port, pid})
		}
		formatter.PrintTable(headers, rows)
		fmt.Println()
	}

	return nil
}
