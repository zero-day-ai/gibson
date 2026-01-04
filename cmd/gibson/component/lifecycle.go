package component

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/core"
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
	componentName := args[0]

	cmd.Printf("Starting %s '%s'...\n", cfg.DisplayName, componentName)

	// Build command context
	cc, err := buildCommandContext(cmd)
	if err != nil {
		return err
	}
	defer cc.Close()

	// Call core function
	result, err := core.ComponentStart(cc, cfg.Kind, componentName)
	if err != nil {
		return err
	}

	// Extract result data
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected result type")
	}

	pid, _ := data["pid"].(int)
	port, _ := data["port"].(int)

	cmd.Printf("%s '%s' started successfully\n", capitalizeFirst(cfg.DisplayName), componentName)
	cmd.Printf("PID: %d\n", pid)
	cmd.Printf("Port: %d\n", port)

	// Show log path
	logPath := filepath.Join(cc.HomeDir, "logs", string(cfg.Kind), fmt.Sprintf("%s.log", componentName))
	cmd.Printf("Logs: %s\n", logPath)

	return nil
}

// runStop executes the stop command for a component.
func runStop(cmd *cobra.Command, args []string, cfg Config) error {
	componentName := args[0]

	// Build command context
	cc, err := buildCommandContext(cmd)
	if err != nil {
		return err
	}
	defer cc.Close()

	// Call core function
	result, err := core.ComponentStop(cc, cfg.Kind, componentName)
	if err != nil {
		return err
	}

	// Extract result data
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected result type")
	}

	stoppedCount, _ := data["stopped_count"].(int)
	totalCount, _ := data["total_count"].(int)

	cmd.Printf("Stopping %s '%s' (%d instance(s))...\n",
		cfg.DisplayName, componentName, totalCount)

	cmd.Printf("%s '%s' stopped successfully (%d/%d instances)\n",
		capitalizeFirst(cfg.DisplayName), componentName, stoppedCount, totalCount)

	return nil
}


// capitalizeFirst capitalizes the first letter of a string.
func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}
