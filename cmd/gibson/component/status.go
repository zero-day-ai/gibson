package component

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/database"
)

// StatusOutput represents the JSON output structure for component status.
type StatusOutput struct {
	Name          string              `json:"name"`
	Status        string              `json:"status"`
	PID           int                 `json:"pid,omitempty"`
	Port          int                 `json:"port,omitempty"`
	ProcessState  string              `json:"process_state"`
	Uptime        string              `json:"uptime,omitempty"`
	UptimeSeconds float64             `json:"uptime_seconds,omitempty"`
	StartedAt     *time.Time          `json:"started_at,omitempty"`
	HealthCheck   *HealthCheckOutput  `json:"health_check,omitempty"`
	Config        *HealthConfigOutput `json:"health_config,omitempty"`
	RecentErrors  []ErrorOutput       `json:"recent_errors"`
}

// HealthCheckOutput represents the health check section of JSON output.
type HealthCheckOutput struct {
	Status         string  `json:"status"`
	Protocol       string  `json:"protocol"`
	ResponseTimeMs float64 `json:"response_time_ms"`
	Error          string  `json:"error,omitempty"`
}

// HealthConfigOutput represents the health check configuration section of JSON output.
type HealthConfigOutput struct {
	Protocol string `json:"protocol"`
	Interval string `json:"interval,omitempty"`
	Timeout  string `json:"timeout,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

// ErrorOutput represents an error entry in JSON output.
type ErrorOutput struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

// newStatusCommand creates a status command for the specified component type.
func newStatusCommand(cfg Config, flags *StatusFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status NAME",
		Short: "Show detailed runtime status",
		Long: fmt.Sprintf(`Display runtime status including health check results, uptime, and recent errors.

Shows real-time information about a running %s:
  - Current health status and response time
  - Process uptime and resource usage
  - Recent errors and error count
  - Port and PID information

Use --watch to continuously monitor the %s status with automatic refresh.`,
			cfg.DisplayName, cfg.DisplayName),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, args, cfg, flags)
		},
	}

	// Register flags
	cmd.Flags().BoolVarP(&flags.Watch, "watch", "w", false, "Enable continuous monitoring")
	cmd.Flags().DurationVar(&flags.Interval, "interval", 2*time.Second, "Refresh interval for watch mode (minimum 1s)")
	cmd.Flags().IntVar(&flags.ErrorCount, "errors", 5, "Number of recent errors to display")
	cmd.Flags().BoolVar(&flags.JSON, "json", false, "Output in JSON format")

	return cmd
}

// runStatus executes the status command for a component.
func runStatus(cmd *cobra.Command, args []string, cfg Config, flags *StatusFlags) error {
	ctx := cmd.Context()
	componentName := args[0]

	// Validate interval
	if flags.Interval < time.Second {
		return fmt.Errorf("interval must be at least 1s, got %v", flags.Interval)
	}

	// Watch mode doesn't work with JSON output
	if flags.Watch && flags.JSON {
		return fmt.Errorf("watch mode cannot be used with --json flag")
	}

	// If watch mode is enabled, delegate to runStatusWatch
	if flags.Watch {
		return runStatusWatch(ctx, cmd, cfg, flags, componentName)
	}

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database for metadata
	dbPath := filepath.Join(homeDir, "gibson.db")
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create DAO
	dao := database.NewComponentDAO(db)

	// Get component metadata from database
	comp, err := dao.GetByName(ctx, cfg.Kind, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		return fmt.Errorf("%s '%s' not found", cfg.DisplayName, componentName)
	}

	// Create UnifiedDiscovery to get live status
	discovery := createUnifiedDiscovery(homeDir)

	// Get the live component state
	discovered, err := discovery.GetComponent(ctx, cfg.Kind, componentName)
	if err != nil {
		return fmt.Errorf("failed to discover component: %w", err)
	}

	// Update component state from discovery
	if discovered != nil && discovered.Healthy {
		// Component is running - update runtime fields
		comp.PID = 0 // PID not available from discovery
		comp.Port = discovered.Port
		comp.Status = "running"
	} else {
		// Component is not running or not found
		comp.PID = 0
		comp.Port = 0
		comp.Status = "stopped"
	}

	// Create StatusChecker with logs directory
	logsDir := filepath.Join(homeDir, "logs")
	checker := component.NewStatusChecker(logsDir)

	// Check status
	statusResult, err := checker.CheckStatus(ctx, comp)
	if err != nil {
		return fmt.Errorf("failed to check status: %w", err)
	}

	// Check if JSON output is requested
	if flags.JSON {
		return outputJSON(cmd, statusResult)
	}

	// Display formatted output
	displayStatus(cmd, statusResult, cfg)

	return nil
}

// outputJSON outputs the status result as JSON.
func outputJSON(cmd *cobra.Command, result *component.StatusResult) error {
	comp := result.Component

	// Build StatusOutput from StatusResult
	output := StatusOutput{
		Name:         comp.Name,
		ProcessState: string(result.ProcessState),
		RecentErrors: []ErrorOutput{},
	}

	// Set status based on process state and health check
	if result.IsRunning() {
		if result.HealthCheck != nil && result.HealthCheck.IsHealthy() {
			output.Status = "healthy"
		} else if result.HealthCheck != nil {
			output.Status = "unhealthy"
		} else {
			output.Status = "running"
		}
	} else {
		output.Status = "stopped"
	}

	// Add PID and Port if available
	if comp.PID > 0 {
		output.PID = comp.PID
	}
	if comp.Port > 0 {
		output.Port = comp.Port
	}

	// Add uptime if running
	if result.IsRunning() && result.Uptime > 0 {
		output.Uptime = formatDuration(result.Uptime)
		output.UptimeSeconds = result.Uptime.Seconds()
	}

	// Add started time
	if comp.StartedAt != nil {
		output.StartedAt = comp.StartedAt
	}

	// Add health check results
	if result.HealthCheck != nil {
		output.HealthCheck = &HealthCheckOutput{
			Status:         result.HealthCheck.Status,
			Protocol:       string(result.HealthCheck.Protocol),
			ResponseTimeMs: float64(result.HealthCheck.ResponseTime.Microseconds()) / 1000.0,
		}
		if result.HealthCheck.HasError() {
			output.HealthCheck.Error = result.HealthCheck.Error
		}
	}

	// Add health configuration
	if comp.Manifest != nil && comp.Manifest.Runtime != nil && comp.Manifest.Runtime.HealthCheck != nil {
		hc := comp.Manifest.Runtime.HealthCheck
		output.Config = &HealthConfigOutput{
			Protocol: string(hc.Protocol),
		}
		if hc.Interval > 0 {
			output.Config.Interval = hc.Interval.String()
		}
		if hc.Timeout > 0 {
			output.Config.Timeout = hc.Timeout.String()
		}
		if hc.Endpoint != "" {
			output.Config.Endpoint = hc.Endpoint
		}
	}

	// Add recent errors
	for _, logErr := range result.RecentErrors {
		output.RecentErrors = append(output.RecentErrors, ErrorOutput{
			Timestamp: logErr.Timestamp.Format(time.RFC3339),
			Level:     logErr.Level,
			Message:   logErr.Message,
		})
	}

	// Marshal to JSON with indentation
	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Print JSON to stdout
	cmd.Println(string(jsonBytes))

	// Set exit code based on status
	// Exit 0 for healthy/running, 1 for unhealthy/stopped
	if output.Status == "stopped" || output.Status == "unhealthy" {
		cmd.SilenceUsage = true
		return fmt.Errorf("") // Return empty error to set exit code 1
	}

	return nil
}

// displayStatus formats and displays the status result.
func displayStatus(cmd *cobra.Command, result *component.StatusResult, cfg Config) {
	comp := result.Component

	// Header - component name and status
	cmd.Printf("%s: %s\n", titleCaser.String(cfg.DisplayName), comp.Name)

	// Display status based on process state
	if result.IsRunning() {
		cmd.Printf("Status: running\n")
	} else {
		cmd.Printf("Status: stopped\n")
	}

	// Display PID and Port
	if comp.PID > 0 {
		cmd.Printf("PID: %d\n", comp.PID)
	}
	if comp.Port > 0 {
		cmd.Printf("Port: %d\n", comp.Port)
	}

	// Display uptime if running
	if result.IsRunning() && result.Uptime > 0 {
		cmd.Printf("Uptime: %s\n", formatDuration(result.Uptime))
	}

	// Display started time
	if comp.StartedAt != nil {
		cmd.Printf("Started: %s\n", comp.StartedAt.Format(time.RFC3339))
	}

	// Display health check section (only if component is running)
	if result.IsRunning() && result.HealthCheck != nil {
		cmd.Printf("\nHealth Check:\n")

		// Color-code status if possible
		status := result.HealthCheck.Status
		if status == "SERVING" {
			cmd.Printf("  Status: %s\n", status)
		} else {
			cmd.Printf("  Status: %s\n", status)
		}

		cmd.Printf("  Protocol: %s\n", result.HealthCheck.Protocol)
		cmd.Printf("  Response Time: %s\n", formatResponseTime(result.HealthCheck.ResponseTime))

		// Display error if health check failed
		if result.HealthCheck.HasError() {
			cmd.Printf("  Error: %s\n", result.HealthCheck.Error)
		}
	}

	// Display health configuration if available
	if comp.Manifest != nil && comp.Manifest.Runtime != nil && comp.Manifest.Runtime.HealthCheck != nil {
		hc := comp.Manifest.Runtime.HealthCheck
		cmd.Printf("\nHealth Configuration:\n")
		cmd.Printf("  Protocol: %s\n", hc.Protocol)
		if hc.Interval > 0 {
			cmd.Printf("  Interval: %s\n", hc.Interval)
		}
		if hc.Timeout > 0 {
			cmd.Printf("  Timeout: %s\n", hc.Timeout)
		}
	}

	// Display recent errors
	cmd.Printf("\n")
	if result.HasRecentErrors() {
		cmd.Printf("Recent Errors:\n")
		for _, logErr := range result.RecentErrors {
			cmd.Printf("  [%s] %s: %s\n",
				logErr.Timestamp.Format("2006-01-02 15:04:05"),
				logErr.Level,
				logErr.Message)
		}
	} else {
		cmd.Printf("Recent Errors: (none)\n")
	}
}

// formatDuration formats a duration in a human-readable format (e.g., "2h 15m 30s").
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		if seconds > 0 {
			return fmt.Sprintf("%dm %ds", minutes, seconds)
		}
		return fmt.Sprintf("%dm", minutes)
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if seconds > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dh", hours)
}

// formatResponseTime formats response time in milliseconds with precision.
func formatResponseTime(d time.Duration) string {
	ms := float64(d.Microseconds()) / 1000.0
	return fmt.Sprintf("%.1fms", ms)
}

// runStatusWatch executes the status command in watch mode for continuous monitoring.
// It continuously refreshes the component status at the configured interval until
// interrupted by SIGINT/SIGTERM or context cancellation.
func runStatusWatch(ctx context.Context, cmd *cobra.Command, cfg Config, flags *StatusFlags, componentName string) error {
	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Create ticker for periodic refresh
	ticker := time.NewTicker(flags.Interval)
	defer ticker.Stop()

	// Get Gibson home directory once (doesn't change during watch)
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database once (will be used for all iterations)
	dbPath := filepath.Join(homeDir, "gibson.db")
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create DAO, StatusChecker, and UnifiedDiscovery once
	dao := database.NewComponentDAO(db)
	logsDir := filepath.Join(homeDir, "logs")
	checker := component.NewStatusChecker(logsDir)
	discovery := createUnifiedDiscovery(homeDir)

	// Display initial status
	if err := displayStatusUpdate(ctx, cmd, cfg, flags, dao, checker, componentName, discovery); err != nil {
		return err
	}

	// Watch loop
	for {
		select {
		case <-sigChan:
			cmd.Println("\nExiting watch mode...")
			return nil

		case <-ticker.C:
			// Refresh status on each tick
			if err := displayStatusUpdate(ctx, cmd, cfg, flags, dao, checker, componentName, discovery); err != nil {
				// Don't exit on errors during watch - component might restart
				// Just display the error and continue watching
				cmd.Printf("\nError refreshing status: %v\n", err)
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// displayStatusUpdate fetches the current component status and displays it with a timestamp.
// This is called both for the initial display and on each refresh tick in watch mode.
func displayStatusUpdate(ctx context.Context, cmd *cobra.Command, cfg Config, flags *StatusFlags, dao database.ComponentDAO, checker *component.StatusChecker, componentName string, discovery component.UnifiedDiscovery) error {
	// Get component metadata (re-fetch each time in case it changed)
	comp, err := dao.GetByName(ctx, cfg.Kind, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		return fmt.Errorf("%s '%s' not found", cfg.DisplayName, componentName)
	}

	// Get the live component state from discovery
	discovered, err := discovery.GetComponent(ctx, cfg.Kind, componentName)
	if err != nil {
		// Don't fail on discovery errors in watch mode - just show as stopped
		comp.PID = 0
		comp.Port = 0
		comp.Status = "stopped"
	} else if discovered != nil && discovered.Healthy {
		// Component is running - update runtime fields
		comp.PID = 0 // PID not available from discovery
		comp.Port = discovered.Port
		comp.Status = "running"
	} else {
		// Component is not running
		comp.PID = 0
		comp.Port = 0
		comp.Status = "stopped"
	}

	// Check status
	statusResult, err := checker.CheckStatus(ctx, comp)
	if err != nil {
		return fmt.Errorf("failed to check status: %w", err)
	}

	// Clear screen and display with timestamp
	clearScreen(cmd)
	displayWithTimestamp(cmd, statusResult, cfg)

	return nil
}

// clearScreen clears the terminal screen using ANSI escape codes.
// Moves cursor to home position (top-left) and clears the entire screen.
func clearScreen(cmd *cobra.Command) {
	cmd.Print("\033[H\033[2J")
}

// displayWithTimestamp displays the status result with a timestamp header.
// This is used in watch mode to show when the status was last updated.
func displayWithTimestamp(cmd *cobra.Command, result *component.StatusResult, cfg Config) {
	// Display timestamp header
	cmd.Printf("Last update: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	// Display normal status output
	displayStatus(cmd, result, cfg)
}
