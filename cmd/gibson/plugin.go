package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/component"
	internalcomp "github.com/zero-day-ai/gibson/internal/component"
	daemonclient "github.com/zero-day-ai/gibson/internal/daemon/client"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage Gibson plugins",
	Long:  `Install, manage, and query plugins for external data access`,
}

var pluginQueryCmd = &cobra.Command{
	Use:   "query NAME",
	Short: "Query a plugin method",
	Long: `Execute a plugin method with the provided parameters

Example:
  gibson plugin query mydata --method GetData --params '{"key": "value"}'
  gibson plugin query weather --method GetForecast --params '{"location": "NYC"}'`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginQuery,
}

// Flags for plugin query
var (
	pluginQueryMethod string
	pluginQueryParams string
)

func init() {
	// Register common component commands (list, install, install-all, uninstall, update, show, build, start, stop)
	component.RegisterCommands(pluginCmd, component.Config{
		Kind:          internalcomp.ComponentKindPlugin,
		DisplayName:   "plugin",
		DisplayPlural: "plugins",
	})

	// Query command flags
	pluginQueryCmd.Flags().StringVar(&pluginQueryMethod, "method", "", "Method name to execute")
	pluginQueryCmd.Flags().StringVar(&pluginQueryParams, "params", "{}", "JSON parameters for the method")
	pluginQueryCmd.MarkFlagRequired("method")

	// Add plugin-specific query command
	pluginCmd.AddCommand(pluginQueryCmd)
}

// runPluginQuery executes the plugin query command
func runPluginQuery(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	pluginName := args[0]

	// Parse JSON parameters
	var params map[string]any
	if err := json.Unmarshal([]byte(pluginQueryParams), &params); err != nil {
		return fmt.Errorf("invalid JSON parameters: %w", err)
	}

	// Get daemon client from context (set by root.go in Client mode)
	client := component.GetDaemonClient(ctx)
	if client == nil {
		return fmt.Errorf("daemon not connected (ensure daemon is running)")
	}

	// Type assert to daemon client
	dc, ok := client.(*daemonclient.Client)
	if !ok {
		return fmt.Errorf("invalid daemon client type")
	}

	cmd.Printf("Querying plugin '%s' method '%s'...\n", pluginName, pluginQueryMethod)

	// Execute query via daemon
	start := time.Now()
	result, err := dc.QueryPlugin(ctx, pluginName, pluginQueryMethod, params)
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	duration := time.Since(start)

	cmd.Printf("Query executed successfully in %v\n", duration)

	// Display result as formatted JSON
	cmd.Printf("\nResult:\n")
	resultJSON, err := json.MarshalIndent(result.Result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format result: %w", err)
	}

	cmd.Printf("%s\n", string(resultJSON))

	return nil
}
