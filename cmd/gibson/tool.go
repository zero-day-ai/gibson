package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/component/build"
	"github.com/zero-day-ai/gibson/internal/component/git"
	"github.com/zero-day-ai/gibson/internal/tool"
)

var toolCmd = &cobra.Command{
	Use:   "tool",
	Short: "Manage Gibson tools",
	Long:  `Install, manage, and invoke tools for agent capabilities`,
}

var toolListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all installed tools",
	Long:  `List all installed tools with their status and metadata`,
	RunE:  runToolList,
}

var toolInstallCmd = &cobra.Command{
	Use:   "install REPO_URL",
	Short: "Install a tool from a git repository",
	Long: `Install a tool from a git repository URL.

For single-tool repositories:
  gibson tool install https://github.com/user/gibson-tool-nmap

For mono-repos with multiple tools, use the # fragment to specify the subdirectory:
  gibson tool install https://github.com/user/gibson-tools#discovery/nmap
  gibson tool install git@github.com:user/gibson-tools.git#reconnaissance/nuclei

The component.yaml manifest must be in the root of the specified directory.`,
	Args: cobra.ExactArgs(1),
	RunE: runToolInstall,
}

var toolUninstallCmd = &cobra.Command{
	Use:   "uninstall NAME",
	Short: "Uninstall a tool",
	Long:  `Remove an installed tool from the system`,
	Args:  cobra.ExactArgs(1),
	RunE:  runToolUninstall,
}

var toolUpdateCmd = &cobra.Command{
	Use:   "update NAME",
	Short: "Update a tool to the latest version",
	Long:  `Pull latest changes and rebuild a tool`,
	Args:  cobra.ExactArgs(1),
	RunE:  runToolUpdate,
}

var toolShowCmd = &cobra.Command{
	Use:   "show NAME",
	Short: "Show detailed tool information",
	Long:  `Display detailed information about a tool including its JSON schema`,
	Args:  cobra.ExactArgs(1),
	RunE:  runToolShow,
}

var toolBuildCmd = &cobra.Command{
	Use:   "build NAME",
	Short: "Build a tool locally",
	Long:  `Build or rebuild a tool for local development`,
	Args:  cobra.ExactArgs(1),
	RunE:  runToolBuild,
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

var toolInstallAllCmd = &cobra.Command{
	Use:   "install-all REPO_URL",
	Short: "Install all tools from a mono-repo",
	Long: `Clone a mono-repo and install all tools found within it.

This command recursively walks the repository looking for component.yaml files
and installs each component as a tool.

Examples:
  gibson tool install-all https://github.com/zero-day-ai/gibson-tools-official
  gibson tool install-all git@github.com:zero-day-ai/gibson-tools-official.git

Note: Repositories should contain only tools. Use separate repos for different
component types (tools, agents, plugins).`,
	Args: cobra.ExactArgs(1),
	RunE: runToolInstallAll,
}

// Flags for tool install
var (
	toolInstallBranch     string
	toolInstallTag        string
	toolInstallForce      bool
	toolInstallSkipBuild  bool
	toolInstallSkipRegister bool
)

// Flags for tool update
var (
	toolUpdateRestart  bool
	toolUpdateSkipBuild bool
)

// Flags for tool uninstall
var (
	toolUninstallForce bool
)

// Flags for tool invoke
var (
	toolInvokeInput string
)

// Flags for tool install-all
var (
	toolInstallAllBranch      string
	toolInstallAllTag         string
	toolInstallAllForce       bool
	toolInstallAllSkipBuild   bool
	toolInstallAllSkipRegister bool
)

func init() {
	// Install command flags
	toolInstallCmd.Flags().StringVar(&toolInstallBranch, "branch", "", "Git branch to install")
	toolInstallCmd.Flags().StringVar(&toolInstallTag, "tag", "", "Git tag to install")
	toolInstallCmd.Flags().BoolVar(&toolInstallForce, "force", false, "Force reinstall if tool exists")
	toolInstallCmd.Flags().BoolVar(&toolInstallSkipBuild, "skip-build", false, "Skip building the tool")
	toolInstallCmd.Flags().BoolVar(&toolInstallSkipRegister, "skip-register", false, "Skip registering in component registry")

	// Install-all command flags
	toolInstallAllCmd.Flags().StringVar(&toolInstallAllBranch, "branch", "", "Git branch to install from")
	toolInstallAllCmd.Flags().StringVar(&toolInstallAllTag, "tag", "", "Git tag to install from")
	toolInstallAllCmd.Flags().BoolVar(&toolInstallAllForce, "force", false, "Force reinstall if tools exist")
	toolInstallAllCmd.Flags().BoolVar(&toolInstallAllSkipBuild, "skip-build", false, "Skip building the tools")
	toolInstallAllCmd.Flags().BoolVar(&toolInstallAllSkipRegister, "skip-register", false, "Skip registering in component registry")

	// Update command flags
	toolUpdateCmd.Flags().BoolVar(&toolUpdateRestart, "restart", false, "Restart tool after update if it was running")
	toolUpdateCmd.Flags().BoolVar(&toolUpdateSkipBuild, "skip-build", false, "Skip rebuilding the tool")

	// Uninstall command flags
	toolUninstallCmd.Flags().BoolVar(&toolUninstallForce, "force", false, "Skip confirmation prompt")

	// Invoke command flags
	toolInvokeCmd.Flags().StringVar(&toolInvokeInput, "input", "{}", "JSON input for the tool")
	toolInvokeCmd.MarkFlagRequired("input")

	// Add subcommands
	toolCmd.AddCommand(toolListCmd)
	toolCmd.AddCommand(toolInstallCmd)
	toolCmd.AddCommand(toolInstallAllCmd)
	toolCmd.AddCommand(toolUninstallCmd)
	toolCmd.AddCommand(toolUpdateCmd)
	toolCmd.AddCommand(toolShowCmd)
	toolCmd.AddCommand(toolBuildCmd)
	toolCmd.AddCommand(toolTestCmd)
	toolCmd.AddCommand(toolInvokeCmd)
}

// runToolList executes the tool list command
func runToolList(cmd *cobra.Command, args []string) error {
	// Create component registry
	registry := component.NewDefaultComponentRegistry()

	// Load registry from disk
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	registryPath := homeDir + "/registry.yaml"
	if _, err := os.Stat(registryPath); err == nil {
		if err := registry.LoadFromConfig(registryPath); err != nil {
			return fmt.Errorf("failed to load registry: %w", err)
		}
	}

	// Get all tools
	tools := registry.List(component.ComponentKindTool)

	if len(tools) == 0 {
		cmd.Println("No tools installed.")
		return nil
	}

	// Display results in table format
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tSTATUS\tSOURCE\tPATH")
	fmt.Fprintln(w, "----\t-------\t------\t------\t----")

	for _, tool := range tools {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			tool.Name,
			tool.Version,
			tool.Status,
			tool.Source,
			tool.Path,
		)
	}

	w.Flush()
	return nil
}

// runToolInstall executes the tool install command
func runToolInstall(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	repoURL := args[0]

	cmd.Printf("Installing tool from %s...\n", repoURL)

	// Create component registry
	registry := component.NewDefaultComponentRegistry()

	// Load existing registry
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	registryPath := homeDir + "/registry.yaml"
	if _, err := os.Stat(registryPath); err == nil {
		if err := registry.LoadFromConfig(registryPath); err != nil {
			return fmt.Errorf("failed to load registry: %w", err)
		}
	}

	// Create installer with dependencies
	gitOps := git.NewDefaultGitOperations()
	builder := build.NewDefaultBuildExecutor()
	installer := component.NewDefaultInstaller(gitOps, builder, registry)

	// Prepare install options
	opts := component.InstallOptions{
		Branch:       toolInstallBranch,
		Tag:          toolInstallTag,
		Force:        toolInstallForce,
		SkipBuild:    toolInstallSkipBuild,
		SkipRegister: toolInstallSkipRegister,
	}

	// Install the tool
	result, err := installer.Install(ctx, repoURL, component.ComponentKindTool, opts)
	if err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	// Save registry
	if !toolInstallSkipRegister {
		if err := registry.Save(); err != nil {
			return fmt.Errorf("failed to save registry: %w", err)
		}
	}

	cmd.Printf("Tool '%s' installed successfully (v%s) in %v\n",
		result.Component.Name,
		result.Component.Version,
		result.Duration)

	if result.BuildOutput != "" && !toolInstallSkipBuild {
		cmd.Printf("\nBuild output:\n%s\n", result.BuildOutput)
	}

	return nil
}

// runToolInstallAll executes the tool install-all command
func runToolInstallAll(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	repoURL := args[0]

	cmd.Printf("Installing all tools from %s...\n", repoURL)

	// Create component registry
	registry := component.NewDefaultComponentRegistry()

	// Load existing registry
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	registryPath := homeDir + "/registry.yaml"
	if _, err := os.Stat(registryPath); err == nil {
		if err := registry.LoadFromConfig(registryPath); err != nil {
			return fmt.Errorf("failed to load registry: %w", err)
		}
	}

	// Create installer with dependencies
	gitOps := git.NewDefaultGitOperations()
	builder := build.NewDefaultBuildExecutor()
	installer := component.NewDefaultInstaller(gitOps, builder, registry)

	// Prepare install options
	opts := component.InstallOptions{
		Branch:       toolInstallAllBranch,
		Tag:          toolInstallAllTag,
		Force:        toolInstallAllForce,
		SkipBuild:    toolInstallAllSkipBuild,
		SkipRegister: toolInstallAllSkipRegister,
	}

	// Install all tools
	result, err := installer.InstallAll(ctx, repoURL, component.ComponentKindTool, opts)
	if err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	// Save registry
	if !toolInstallAllSkipRegister {
		if err := registry.Save(); err != nil {
			return fmt.Errorf("failed to save registry: %w", err)
		}
	}

	// Print summary
	cmd.Printf("\nInstallation complete in %v\n", result.Duration)
	cmd.Printf("Components found: %d\n", result.ComponentsFound)
	cmd.Printf("Successfully installed: %d\n", len(result.Successful))
	cmd.Printf("Failed: %d\n", len(result.Failed))

	// List successful installations
	if len(result.Successful) > 0 {
		cmd.Printf("\nInstalled tools:\n")
		for _, r := range result.Successful {
			cmd.Printf("  - %s (v%s)\n", r.Component.Name, r.Component.Version)
		}
	}

	// List failures
	if len(result.Failed) > 0 {
		cmd.Printf("\nFailed installations:\n")
		for _, f := range result.Failed {
			name := f.Name
			if name == "" {
				name = f.Path
			}
			cmd.Printf("  - %s: %v\n", name, f.Error)
		}
	}

	return nil
}

// runToolUninstall executes the tool uninstall command
func runToolUninstall(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	toolName := args[0]

	// Create component registry
	registry := component.NewDefaultComponentRegistry()

	// Load existing registry
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	registryPath := homeDir + "/registry.yaml"
	if _, err := os.Stat(registryPath); err == nil {
		if err := registry.LoadFromConfig(registryPath); err != nil {
			return fmt.Errorf("failed to load registry: %w", err)
		}
	}

	// Check if tool exists
	existingTool := registry.Get(component.ComponentKindTool, toolName)
	if existingTool == nil {
		return fmt.Errorf("tool '%s' not found", toolName)
	}

	// Confirm uninstall unless --force is set
	if !toolUninstallForce {
		cmd.Printf("Are you sure you want to uninstall tool '%s'? (y/N): ", toolName)
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			cmd.Println("Uninstall cancelled.")
			return nil
		}
	}

	// Create installer
	gitOps := git.NewDefaultGitOperations()
	builder := build.NewDefaultBuildExecutor()
	installer := component.NewDefaultInstaller(gitOps, builder, registry)

	// Uninstall the tool
	result, err := installer.Uninstall(ctx, component.ComponentKindTool, toolName)
	if err != nil {
		return fmt.Errorf("uninstall failed: %w", err)
	}

	// Save registry
	if err := registry.Save(); err != nil {
		return fmt.Errorf("failed to save registry: %w", err)
	}

	cmd.Printf("Tool '%s' uninstalled successfully in %v\n", toolName, result.Duration)
	return nil
}

// runToolUpdate executes the tool update command
func runToolUpdate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	toolName := args[0]

	cmd.Printf("Updating tool '%s'...\n", toolName)

	// Create component registry
	registry := component.NewDefaultComponentRegistry()

	// Load existing registry
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	registryPath := homeDir + "/registry.yaml"
	if _, err := os.Stat(registryPath); err == nil {
		if err := registry.LoadFromConfig(registryPath); err != nil {
			return fmt.Errorf("failed to load registry: %w", err)
		}
	}

	// Check if tool exists
	existingTool := registry.Get(component.ComponentKindTool, toolName)
	if existingTool == nil {
		return fmt.Errorf("tool '%s' not found", toolName)
	}

	// Create installer
	gitOps := git.NewDefaultGitOperations()
	builder := build.NewDefaultBuildExecutor()
	installer := component.NewDefaultInstaller(gitOps, builder, registry)

	// Prepare update options
	opts := component.UpdateOptions{
		Restart:   toolUpdateRestart,
		SkipBuild: toolUpdateSkipBuild,
	}

	// Update the tool
	result, err := installer.Update(ctx, component.ComponentKindTool, toolName, opts)
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	// Save registry
	if err := registry.Save(); err != nil {
		return fmt.Errorf("failed to save registry: %w", err)
	}

	if !result.Updated {
		cmd.Printf("Tool '%s' is already up to date (v%s)\n", toolName, result.OldVersion)
		return nil
	}

	cmd.Printf("Tool '%s' updated successfully (v%s → v%s) in %v\n",
		toolName,
		result.OldVersion,
		result.NewVersion,
		result.Duration)

	if result.BuildOutput != "" && !toolUpdateSkipBuild {
		cmd.Printf("\nBuild output:\n%s\n", result.BuildOutput)
	}

	return nil
}

// runToolShow executes the tool show command
func runToolShow(cmd *cobra.Command, args []string) error {
	toolName := args[0]

	// Create component registry
	registry := component.NewDefaultComponentRegistry()

	// Load existing registry
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	registryPath := homeDir + "/registry.yaml"
	if _, err := os.Stat(registryPath); err == nil {
		if err := registry.LoadFromConfig(registryPath); err != nil {
			return fmt.Errorf("failed to load registry: %w", err)
		}
	}

	// Get tool
	toolComp := registry.Get(component.ComponentKindTool, toolName)
	if toolComp == nil {
		return fmt.Errorf("tool '%s' not found", toolName)
	}

	// Display tool details
	cmd.Printf("Tool: %s\n", toolComp.Name)
	cmd.Printf("Version: %s\n", toolComp.Version)
	cmd.Printf("Status: %s\n", toolComp.Status)
	cmd.Printf("Source: %s\n", toolComp.Source)
	cmd.Printf("Path: %s\n", toolComp.Path)

	if toolComp.Port > 0 {
		cmd.Printf("Port: %d\n", toolComp.Port)
	}

	if toolComp.PID > 0 {
		cmd.Printf("PID: %d\n", toolComp.PID)
	}

	cmd.Printf("Created: %s\n", toolComp.CreatedAt.Format(time.RFC3339))
	cmd.Printf("Updated: %s\n", toolComp.UpdatedAt.Format(time.RFC3339))

	if toolComp.StartedAt != nil {
		cmd.Printf("Started: %s\n", toolComp.StartedAt.Format(time.RFC3339))
	}

	if toolComp.StoppedAt != nil {
		cmd.Printf("Stopped: %s\n", toolComp.StoppedAt.Format(time.RFC3339))
	}

	// Display manifest information if available
	if toolComp.Manifest != nil {
		manifest := toolComp.Manifest
		cmd.Printf("\nManifest:\n")
		cmd.Printf("  Description: %s\n", manifest.Description)
		cmd.Printf("  Author: %s\n", manifest.Author)

		if manifest.License != "" {
			cmd.Printf("  License: %s\n", manifest.License)
		}

		if manifest.Repository != "" {
			cmd.Printf("  Repository: %s\n", manifest.Repository)
		}

		// Display runtime information
		cmd.Printf("\nRuntime:\n")
		cmd.Printf("  Type: %s\n", manifest.Runtime.Type)
		cmd.Printf("  Entrypoint: %s\n", manifest.Runtime.Entrypoint)

		if manifest.Runtime.Port > 0 {
			cmd.Printf("  Port: %d\n", manifest.Runtime.Port)
		}

		if manifest.Runtime.HealthURL != "" {
			cmd.Printf("  Health URL: %s\n", manifest.Runtime.HealthURL)
		}

		// Display dependencies if available
		if manifest.Dependencies != nil && manifest.Dependencies.HasDependencies() {
			cmd.Printf("\nDependencies:\n")

			if manifest.Dependencies.Gibson != "" {
				cmd.Printf("  Gibson: %s\n", manifest.Dependencies.Gibson)
			}

			systemDeps := manifest.Dependencies.GetSystem()
			if len(systemDeps) > 0 {
				cmd.Printf("  System:\n")
				for _, dep := range systemDeps {
					cmd.Printf("    - %s\n", dep)
				}
			}

			componentDeps := manifest.Dependencies.GetComponents()
			if len(componentDeps) > 0 {
				cmd.Printf("  Components:\n")
				for _, dep := range componentDeps {
					cmd.Printf("    - %s\n", dep)
				}
			}

			envDeps := manifest.Dependencies.GetEnv()
			if len(envDeps) > 0 {
				cmd.Printf("  Environment:\n")
				for key, desc := range envDeps {
					cmd.Printf("    - %s: %s\n", key, desc)
				}
			}
		}
	}

	return nil
}

// runToolBuild executes the tool build command
func runToolBuild(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	toolName := args[0]

	cmd.Printf("Building tool '%s'...\n", toolName)

	// Create component registry
	registry := component.NewDefaultComponentRegistry()

	// Load existing registry
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	registryPath := homeDir + "/registry.yaml"
	if _, err := os.Stat(registryPath); err == nil {
		if err := registry.LoadFromConfig(registryPath); err != nil {
			return fmt.Errorf("failed to load registry: %w", err)
		}
	}

	// Get tool
	toolComp := registry.Get(component.ComponentKindTool, toolName)
	if toolComp == nil {
		return fmt.Errorf("tool '%s' not found", toolName)
	}

	if toolComp.Manifest == nil || toolComp.Manifest.Build == nil {
		return fmt.Errorf("tool '%s' has no build configuration", toolName)
	}

	// Create builder
	builder := build.NewDefaultBuildExecutor()

	// Prepare build configuration
	buildCfg := toolComp.Manifest.Build
	buildConfig := build.BuildConfig{
		WorkDir:    toolComp.Path,
		Command:    "make",
		Args:       []string{"build"},
		Env:        buildCfg.GetEnv(),
	}

	// Override with manifest build command if specified
	if buildCfg.Command != "" {
		buildConfig.Command = buildCfg.Command
		buildConfig.Args = []string{}
	}

	// Build the tool
	start := time.Now()
	result, err := builder.Build(ctx, buildConfig, toolComp.Name, toolComp.Version, "dev")
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	duration := time.Since(start)

	cmd.Printf("Tool '%s' built successfully in %v\n", toolName, duration)

	if result.Stdout != "" {
		cmd.Printf("\nStdout:\n%s\n", result.Stdout)
	}

	if result.Stderr != "" {
		cmd.Printf("\nStderr:\n%s\n", result.Stderr)
	}

	return nil
}

// runToolTest executes the tool test command
func runToolTest(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	toolName := args[0]

	cmd.Printf("Running tests for tool '%s'...\n", toolName)

	// Create component registry
	registry := component.NewDefaultComponentRegistry()

	// Load existing registry
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	registryPath := homeDir + "/registry.yaml"
	if _, err := os.Stat(registryPath); err == nil {
		if err := registry.LoadFromConfig(registryPath); err != nil {
			return fmt.Errorf("failed to load registry: %w", err)
		}
	}

	// Get tool
	toolComp := registry.Get(component.ComponentKindTool, toolName)
	if toolComp == nil {
		return fmt.Errorf("tool '%s' not found", toolName)
	}

	// Create builder for running tests
	builder := build.NewDefaultBuildExecutor()

	// Prepare test configuration
	testConfig := build.BuildConfig{
		WorkDir: toolComp.Path,
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
