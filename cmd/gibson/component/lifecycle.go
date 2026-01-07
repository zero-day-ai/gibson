package component

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	daemonclient "github.com/zero-day-ai/gibson/internal/daemon/client"
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
	ctx := cmd.Context()

	cmd.Printf("Starting %s '%s'...\n", cfg.DisplayName, componentName)

	// Check for daemon client in context
	clientIface := GetDaemonClient(ctx)
	if clientIface == nil {
		return fmt.Errorf("daemon not running. Start with: gibson daemon start --foreground")
	}

	// Type assert to daemon client
	client, ok := clientIface.(*daemonclient.Client)
	if !ok {
		return fmt.Errorf("invalid daemon client type")
	}

	// Call appropriate start method based on component kind
	var result *daemonclient.StartResult
	var err error

	switch cfg.Kind.String() {
	case "agent":
		result, err = client.StartAgent(ctx, componentName)
	case "tool":
		result, err = client.StartTool(ctx, componentName)
	case "plugin":
		result, err = client.StartPlugin(ctx, componentName)
	default:
		return fmt.Errorf("unsupported component kind: %s", cfg.Kind)
	}

	if err != nil {
		// Check if it's a connection error
		if strings.Contains(err.Error(), "daemon not responding") {
			return fmt.Errorf("daemon not running. Start with: gibson daemon start --foreground")
		}
		return err
	}

	cmd.Printf("%s '%s' started successfully\n", capitalizeFirst(cfg.DisplayName), componentName)
	cmd.Printf("PID: %d\n", result.PID)
	cmd.Printf("Port: %d\n", result.Port)
	cmd.Printf("Logs: %s\n", result.LogPath)

	return nil
}

// runStop executes the stop command for a component.
func runStop(cmd *cobra.Command, args []string, cfg Config) error {
	componentName := args[0]
	ctx := cmd.Context()

	// Check for daemon client in context
	clientIface := GetDaemonClient(ctx)
	if clientIface == nil {
		return fmt.Errorf("daemon not running. Start with: gibson daemon start --foreground")
	}

	// Type assert to daemon client
	client, ok := clientIface.(*daemonclient.Client)
	if !ok {
		return fmt.Errorf("invalid daemon client type")
	}

	// Call appropriate stop method based on component kind
	var result *daemonclient.StopResult
	var err error

	switch cfg.Kind.String() {
	case "agent":
		result, err = client.StopAgent(ctx, componentName)
	case "tool":
		result, err = client.StopTool(ctx, componentName)
	case "plugin":
		result, err = client.StopPlugin(ctx, componentName)
	default:
		return fmt.Errorf("unsupported component kind: %s", cfg.Kind)
	}

	if err != nil {
		// Check if it's a connection error
		if strings.Contains(err.Error(), "daemon not responding") {
			return fmt.Errorf("daemon not running. Start with: gibson daemon start --foreground")
		}
		return err
	}

	cmd.Printf("Stopping %s '%s' (%d instance(s))...\n",
		cfg.DisplayName, componentName, result.TotalCount)

	cmd.Printf("%s '%s' stopped successfully (%d/%d instances)\n",
		capitalizeFirst(cfg.DisplayName), componentName, result.StoppedCount, result.TotalCount)

	return nil
}

// capitalizeFirst capitalizes the first letter of a string.
func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}
