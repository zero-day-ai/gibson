package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/component"
	"github.com/zero-day-ai/gibson/internal/daemon/client"
	internalcomp "github.com/zero-day-ai/gibson/internal/component"
)

var toolCmd = &cobra.Command{
	Use:   "tool",
	Short: "Manage Gibson tools",
	Long:  `Install, manage, and invoke tools for agent capabilities`,
}

var toolTestCmd = &cobra.Command{
	Use:   "test NAME",
	Short: "Run a tool's test suite",
	Long:  `Execute the test suite for a tool`,
	Args:  cobra.ExactArgs(1),
	RunE:  runToolTest,
}

var toolInvokeCmd = &cobra.Command{
	Use:   "invoke NAME",
	Short: "Invoke a tool with JSON input",
	Long:  `Execute a tool via the daemon's Tool Executor Service`,
	Args:  cobra.ExactArgs(1),
	RunE:  runToolInvoke,
}

var toolAvailableCmd = &cobra.Command{
	Use:   "available",
	Short: "List available tools from the Tool Executor Service",
	Long: `List all tools discovered by the daemon's Tool Executor Service.
These are tool binaries installed in ~/.gibson/tools/bin/ that can be
executed as subprocesses by the daemon.`,
	RunE: runToolAvailable,
}

// Flags for tool invoke
var (
	toolInvokeInput   string
	toolInvokeTimeout int64
)

func init() {
	// Register common component commands but exclude start/stop
	// Since tools are now executed via the daemon's Tool Executor Service,
	// they don't need individual start/stop lifecycle management.
	commands := component.NewCommands(component.Config{
		Kind:          internalcomp.ComponentKindTool,
		DisplayName:   "tool",
		DisplayPlural: "tools",
	})

	// Register all commands EXCEPT start and stop
	// Tools are executed via daemon subprocess model, not individual gRPC servers
	toolCmd.AddCommand(commands.List)
	toolCmd.AddCommand(commands.Install)
	toolCmd.AddCommand(commands.InstallAll)
	toolCmd.AddCommand(commands.Uninstall)
	toolCmd.AddCommand(commands.Update)
	toolCmd.AddCommand(commands.Show)
	toolCmd.AddCommand(commands.Build)
	// commands.Start - REMOVED: tools are now subprocess-based via daemon
	// commands.Stop  - REMOVED: tools are now subprocess-based via daemon
	toolCmd.AddCommand(commands.Status)
	toolCmd.AddCommand(commands.Logs)

	// Invoke command flags
	toolInvokeCmd.Flags().StringVar(&toolInvokeInput, "input", "{}", "JSON input for the tool")
	toolInvokeCmd.Flags().Int64Var(&toolInvokeTimeout, "timeout", 0, "Execution timeout in milliseconds (0 = daemon default)")

	// Add tool-specific subcommands
	toolCmd.AddCommand(toolTestCmd)
	toolCmd.AddCommand(toolInvokeCmd)
	toolCmd.AddCommand(toolAvailableCmd)
}

// runToolTest executes the tool test command
// NOTE: This command is deprecated as component data is now stored in etcd
func runToolTest(cmd *cobra.Command, args []string) error {
	toolName := args[0]

	cmd.Printf("Running tests for tool '%s'...\n", toolName)

	// Component data is now stored in etcd, not SQLite
	// This legacy command is no longer supported
	_, _, err := getComponentDAO()
	if err != nil {
		return fmt.Errorf("tool test command is no longer supported: %w", err)
	}

	// Unreachable
	return nil
}

// runToolInvoke executes the tool invoke command via the daemon's Tool Executor Service
func runToolInvoke(cmd *cobra.Command, args []string) error {
	toolName := args[0]

	// Connect to daemon
	daemonClient, err := connectToDaemon(cmd.Context())
	if err != nil {
		return err
	}
	defer daemonClient.Close()

	cmd.Printf("Invoking tool '%s' via daemon...\n", toolName)

	// Execute the tool
	start := time.Now()
	result, err := daemonClient.ExecuteTool(cmd.Context(), toolName, toolInvokeInput, toolInvokeTimeout)
	if err != nil {
		return fmt.Errorf("tool execution failed: %w", err)
	}

	duration := time.Since(start)

	// Check for execution error
	if !result.Success {
		cmd.Printf("Tool execution failed: %s\n", result.Error)
		return fmt.Errorf("tool execution failed: %s", result.Error)
	}

	cmd.Printf("Tool executed successfully in %v (daemon reported: %dms)\n", duration, result.DurationMs)

	// Display output as formatted JSON
	cmd.Printf("\nOutput:\n")
	var output map[string]any
	if err := json.Unmarshal([]byte(result.OutputJSON), &output); err != nil {
		// If we can't parse as JSON, just show the raw output
		cmd.Printf("%s\n", result.OutputJSON)
	} else {
		outputJSON, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to format output: %w", err)
		}
		cmd.Printf("%s\n", string(outputJSON))
	}

	return nil
}

// runToolAvailable lists all tools available via the daemon's Tool Executor Service
func runToolAvailable(cmd *cobra.Command, args []string) error {
	// Connect to daemon
	daemonClient, err := connectToDaemon(cmd.Context())
	if err != nil {
		return err
	}
	defer daemonClient.Close()

	// Get available tools
	tools, err := daemonClient.GetAvailableTools(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get available tools: %w", err)
	}

	if len(tools) == 0 {
		cmd.Printf("No tools available. Install tools to ~/.gibson/tools/bin/\n")
		return nil
	}

	cmd.Printf("Available tools (%d):\n\n", len(tools))

	// Display tools in a formatted way
	for _, t := range tools {
		statusIcon := "✓"
		if t.Status == "error" {
			statusIcon = "✗"
		} else if t.Status == "schema-unknown" {
			statusIcon = "?"
		}

		cmd.Printf("  %s %s (v%s)\n", statusIcon, t.Name, t.Version)
		if t.Description != "" {
			cmd.Printf("    Description: %s\n", t.Description)
		}
		if len(t.Tags) > 0 {
			cmd.Printf("    Tags: %s\n", strings.Join(t.Tags, ", "))
		}
		cmd.Printf("    Status: %s\n", t.Status)
		if t.ErrorMessage != "" {
			cmd.Printf("    Error: %s\n", t.ErrorMessage)
		}
		cmd.Printf("\n")
	}

	return nil
}

// connectToDaemon establishes a connection to the running daemon
func connectToDaemon(ctx context.Context) (*client.Client, error) {
	// Get Gibson home directory
	homeDir := os.Getenv("GIBSON_HOME")
	if homeDir == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		homeDir = userHome + "/.gibson"
	}

	// Connect to daemon using daemon.json info
	infoPath := homeDir + "/daemon.json"
	daemonClient, err := client.ConnectFromInfo(ctx, infoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon (is it running?): %w", err)
	}

	return daemonClient, nil
}
