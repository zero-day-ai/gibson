package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/component"
	internalcomp "github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/component/build"
	"github.com/zero-day-ai/gibson/internal/tool"
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
	Long:  `Execute a tool with the provided JSON input`,
	Args:  cobra.ExactArgs(1),
	RunE:  runToolInvoke,
}

// Flags for tool invoke
var (
	toolInvokeInput string
)

func init() {
	// Register common component commands
	component.RegisterCommands(toolCmd, component.Config{
		Kind:          internalcomp.ComponentKindTool,
		DisplayName:   "tool",
		DisplayPlural: "tools",
	})

	// Invoke command flags
	toolInvokeCmd.Flags().StringVar(&toolInvokeInput, "input", "{}", "JSON input for the tool")
	toolInvokeCmd.MarkFlagRequired("input")

	// Add tool-specific subcommands
	toolCmd.AddCommand(toolTestCmd)
	toolCmd.AddCommand(toolInvokeCmd)
}

// runToolTest executes the tool test command
func runToolTest(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	toolName := args[0]

	cmd.Printf("Running tests for tool '%s'...\n", toolName)

	// Get component DAO
	dao, db, err := getComponentDAO()
	if err != nil {
		return fmt.Errorf("failed to get component DAO: %w", err)
	}
	defer db.Close()

	// Get tool
	toolComp, err := dao.GetByName(ctx, internalcomp.ComponentKindTool, toolName)
	if err != nil {
		return fmt.Errorf("failed to get tool: %w", err)
	}
	if toolComp == nil {
		return fmt.Errorf("tool '%s' not found", toolName)
	}

	// Create build executor for running tests
	builder := build.NewDefaultBuildExecutor()

	// Prepare test configuration
	workDir := toolComp.RepoPath
	if workDir == "" {
		return fmt.Errorf("tool '%s' has no repository path configured", toolName)
	}
	testConfig := build.BuildConfig{
		WorkDir: workDir,
		Command: "make",
		Args:    []string{"test"},
		Env:     make(map[string]string),
	}

	// Check if manifest has test configuration
	if toolComp.Manifest != nil && toolComp.Manifest.Build != nil {
		if toolComp.Manifest.Build.GetEnv() != nil {
			testConfig.Env = toolComp.Manifest.Build.GetEnv()
		}
	}

	// Run tests
	start := time.Now()
	result, err := builder.Build(ctx, testConfig, toolComp.Name, toolComp.Version, "test")
	if err != nil {
		cmd.Printf("Tests failed in %v\n", time.Since(start))
		if result != nil {
			if result.Stdout != "" {
				cmd.Printf("\nStdout:\n%s\n", result.Stdout)
			}
			if result.Stderr != "" {
				cmd.Printf("\nStderr:\n%s\n", result.Stderr)
			}
		}
		return fmt.Errorf("tests failed: %w", err)
	}

	duration := time.Since(start)

	cmd.Printf("Tests passed in %v\n", duration)

	if result.Stdout != "" {
		cmd.Printf("\nStdout:\n%s\n", result.Stdout)
	}

	if result.Stderr != "" {
		cmd.Printf("\nStderr:\n%s\n", result.Stderr)
	}

	return nil
}

// runToolInvoke executes the tool invoke command
func runToolInvoke(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	toolName := args[0]

	// Parse JSON input
	var input map[string]any
	if err := json.Unmarshal([]byte(toolInvokeInput), &input); err != nil {
		return fmt.Errorf("invalid JSON input: %w", err)
	}

	cmd.Printf("Invoking tool '%s' with input...\n", toolName)

	// Create tool registry
	toolRegistry := tool.NewToolRegistry()

	// Note: In a real implementation, we would need to:
	// 1. Load the tool from the component registry
	// 2. Initialize the tool (possibly via gRPC if external)
	// 3. Register it in the tool registry
	// For now, we'll provide a helpful error message

	// Try to get the tool from the registry
	toolInstance, err := toolRegistry.Get(toolName)
	if err != nil {
		return fmt.Errorf("tool '%s' not loaded in registry. You may need to start the tool first", toolName)
	}

	// Validate input against schema if available
	inputSchema := toolInstance.InputSchema()
	if inputSchema.Type != "" {
		// Note: Actual validation would require the schema package implementation
		cmd.Printf("Input schema available for validation\n")
	}

	// Execute the tool
	start := time.Now()
	output, err := toolRegistry.Execute(ctx, toolName, input)
	if err != nil {
		return fmt.Errorf("tool execution failed: %w", err)
	}

	duration := time.Since(start)

	cmd.Printf("Tool executed successfully in %v\n", duration)

	// Display output as formatted JSON
	cmd.Printf("\nOutput:\n")
	outputJSON, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}

	cmd.Printf("%s\n", string(outputJSON))

	return nil
}
