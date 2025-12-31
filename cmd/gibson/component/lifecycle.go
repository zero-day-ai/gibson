package component

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/database"
)

// newStartCommand creates a start command for the specified component type.
func newStartCommand(cfg Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <name>",
		Short: fmt.Sprintf("Start a %s", cfg.DisplayName),
		Long:  fmt.Sprintf("Start a %s component by name.", cfg.DisplayName),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(cmd, args, cfg)
		},
	}
	return cmd
}

// newStopCommand creates a stop command for the specified component type.
func newStopCommand(cfg Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <name>",
		Short: fmt.Sprintf("Stop a %s", cfg.DisplayName),
		Long:  fmt.Sprintf("Stop a running %s component by name.", cfg.DisplayName),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStop(cmd, args, cfg)
		},
	}
	return cmd
}

// runStart executes the start command for a component.
func runStart(cmd *cobra.Command, args []string, cfg Config) error {
	ctx := cmd.Context()
	componentName := args[0]

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

	// Create DAO
	dao := database.NewComponentDAO(db)

	// Get component using DAO
	comp, err := dao.GetByName(ctx, cfg.Kind, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		return fmt.Errorf("%s '%s' not found", cfg.DisplayName, componentName)
	}

	// Check if already running
	if comp.IsRunning() {
		return fmt.Errorf("%s '%s' is already running (PID: %d)", cfg.DisplayName, componentName, comp.PID)
	}

	cmd.Printf("Starting %s '%s'...\n", cfg.DisplayName, componentName)

	// Get lifecycle manager with DAO
	lifecycleManager := getLifecycleManager(dao)

	// Start component
	port, err := lifecycleManager.StartComponent(ctx, comp)
	if err != nil {
		return fmt.Errorf("failed to start %s: %w", cfg.DisplayName, err)
	}

	cmd.Printf("%s '%s' started successfully\n", capitalizeFirst(cfg.DisplayName), componentName)
	cmd.Printf("PID: %d\n", comp.PID)
	cmd.Printf("Port: %d\n", port)

	return nil
}

// runStop executes the stop command for a component.
func runStop(cmd *cobra.Command, args []string, cfg Config) error {
	ctx := cmd.Context()
	componentName := args[0]

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

	// Create DAO
	dao := database.NewComponentDAO(db)

	// Get component using DAO
	comp, err := dao.GetByName(ctx, cfg.Kind, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		return fmt.Errorf("%s '%s' not found", cfg.DisplayName, componentName)
	}

	// Check if running
	if !comp.IsRunning() {
		return fmt.Errorf("%s '%s' is not running", cfg.DisplayName, componentName)
	}

	cmd.Printf("Stopping %s '%s' (PID: %d)...\n", cfg.DisplayName, componentName, comp.PID)

	// Get lifecycle manager with DAO
	lifecycleManager := getLifecycleManager(dao)

	// Stop component
	if err := lifecycleManager.StopComponent(ctx, comp); err != nil {
		return fmt.Errorf("failed to stop %s: %w", cfg.DisplayName, err)
	}

	cmd.Printf("%s '%s' stopped successfully\n", capitalizeFirst(cfg.DisplayName), componentName)

	return nil
}

// getLifecycleManager creates a lifecycle manager with DAO for status updates.
func getLifecycleManager(dao component.StatusUpdater) component.LifecycleManager {
	healthMonitor := component.NewHealthMonitor()
	return component.NewLifecycleManager(healthMonitor, dao)
}

// capitalizeFirst capitalizes the first letter of a string.
func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}
