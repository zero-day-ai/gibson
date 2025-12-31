package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	internalcomp "github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/plugin"

	"github.com/zero-day-ai/gibson/cmd/gibson/component"
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

	// Get component DAO to verify plugin exists and is running
	dao, db, err := getComponentDAO()
	if err != nil {
		return fmt.Errorf("failed to get component DAO: %w", err)
	}
	defer db.Close()

	pluginComp, err := dao.GetByName(ctx, internalcomp.ComponentKindPlugin, pluginName)
	if err != nil {
		return fmt.Errorf("failed to get plugin: %w", err)
	}
	if pluginComp == nil {
		return fmt.Errorf("plugin '%s' not found", pluginName)
	}

	if !pluginComp.IsRunning() {
		return fmt.Errorf("plugin '%s' is not running. Start it first with: gibson plugin start %s", pluginName, pluginName)
	}

	cmd.Printf("Querying plugin '%s' method '%s'...\n", pluginName, pluginQueryMethod)

	// Get plugin registry
	pluginRegistry := getPluginRegistry()

	// Verify method exists
	methods, err := pluginRegistry.Methods(pluginName)
	if err != nil {
		return fmt.Errorf("failed to get plugin methods: %w", err)
	}

	methodExists := false
	for _, m := range methods {
		if m.Name == pluginQueryMethod {
			methodExists = true
			break
		}
	}

	if !methodExists {
		cmd.Printf("Error: Method '%s' not found in plugin '%s'\n\n", pluginQueryMethod, pluginName)
		cmd.Printf("Available methods:\n")
		for _, m := range methods {
			cmd.Printf("  - %s", m.Name)
			if m.Description != "" {
				cmd.Printf(": %s", m.Description)
			}
			cmd.Println()
		}
		return fmt.Errorf("method '%s' not found", pluginQueryMethod)
	}

	// Execute query
	start := time.Now()
	result, err := pluginRegistry.Query(ctx, pluginName, pluginQueryMethod, params)
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	duration := time.Since(start)

	cmd.Printf("Query executed successfully in %v\n", duration)

	// Display result as formatted JSON
	cmd.Printf("\nResult:\n")
	resultJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format result: %w", err)
	}

	cmd.Printf("%s\n", string(resultJSON))

	return nil
}

// getPluginRegistry creates or retrieves the plugin registry
func getPluginRegistry() plugin.PluginRegistry {
	// In a real implementation, this would be a singleton or retrieved from a global context
	// For now, create a new one
	return plugin.NewPluginRegistry()
}
