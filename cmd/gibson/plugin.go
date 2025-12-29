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
	"github.com/zero-day-ai/gibson/internal/plugin"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage Gibson plugins",
	Long:  `Install, manage, and query plugins for external data access`,
}

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all installed plugins",
	Long:  `List all installed plugins with their status and available methods`,
	RunE:  runPluginList,
}

var pluginInstallCmd = &cobra.Command{
	Use:   "install REPO_URL",
	Short: "Install a plugin from a git repository",
	Long:  `Install a plugin from a git repository URL (e.g., https://github.com/user/gibson-plugin-name)`,
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginInstall,
}

var pluginUninstallCmd = &cobra.Command{
	Use:   "uninstall NAME",
	Short: "Uninstall a plugin",
	Long:  `Stop and remove an installed plugin from the system`,
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginUninstall,
}

var pluginUpdateCmd = &cobra.Command{
	Use:   "update NAME",
	Short: "Update a plugin to the latest version",
	Long:  `Pull latest changes from git and rebuild a plugin`,
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginUpdate,
}

var pluginShowCmd = &cobra.Command{
	Use:   "show NAME",
	Short: "Show detailed plugin information",
	Long:  `Display detailed information about a plugin including available methods`,
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginShow,
}

var pluginBuildCmd = &cobra.Command{
	Use:   "build NAME",
	Short: "Build a plugin locally",
	Long:  `Build or rebuild a plugin for local development`,
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginBuild,
}

var pluginStartCmd = &cobra.Command{
	Use:   "start NAME",
	Short: "Start a plugin",
	Long:  `Start a stopped plugin and wait for it to become healthy`,
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginStart,
}

var pluginStopCmd = &cobra.Command{
	Use:   "stop NAME",
	Short: "Stop a running plugin",
	Long:  `Gracefully stop a running plugin`,
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginStop,
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

// Flags for plugin install
var (
	pluginInstallBranch       string
	pluginInstallTag          string
	pluginInstallForce        bool
	pluginInstallSkipBuild    bool
	pluginInstallSkipRegister bool
)

// Flags for plugin update
var (
	pluginUpdateRestart   bool
	pluginUpdateSkipBuild bool
)

// Flags for plugin uninstall
var (
	pluginUninstallForce bool
)

// Flags for plugin query
var (
	pluginQueryMethod string
	pluginQueryParams string
)

func init() {
	// Install command flags
	pluginInstallCmd.Flags().StringVar(&pluginInstallBranch, "branch", "", "Git branch to install")
	pluginInstallCmd.Flags().StringVar(&pluginInstallTag, "tag", "", "Git tag to install")
	pluginInstallCmd.Flags().BoolVar(&pluginInstallForce, "force", false, "Force reinstall if plugin exists")
	pluginInstallCmd.Flags().BoolVar(&pluginInstallSkipBuild, "skip-build", false, "Skip building the plugin")
	pluginInstallCmd.Flags().BoolVar(&pluginInstallSkipRegister, "skip-register", false, "Skip registering in component registry")

	// Update command flags
	pluginUpdateCmd.Flags().BoolVar(&pluginUpdateRestart, "restart", false, "Restart plugin after update if it was running")
	pluginUpdateCmd.Flags().BoolVar(&pluginUpdateSkipBuild, "skip-build", false, "Skip rebuilding the plugin")

	// Uninstall command flags
	pluginUninstallCmd.Flags().BoolVar(&pluginUninstallForce, "force", false, "Skip confirmation prompt")

	// Query command flags
	pluginQueryCmd.Flags().StringVar(&pluginQueryMethod, "method", "", "Method name to execute")
	pluginQueryCmd.Flags().StringVar(&pluginQueryParams, "params", "{}", "JSON parameters for the method")
	pluginQueryCmd.MarkFlagRequired("method")

	// Add subcommands
	pluginCmd.AddCommand(pluginListCmd)
	pluginCmd.AddCommand(pluginInstallCmd)
	pluginCmd.AddCommand(pluginUninstallCmd)
	pluginCmd.AddCommand(pluginUpdateCmd)
	pluginCmd.AddCommand(pluginShowCmd)
	pluginCmd.AddCommand(pluginBuildCmd)
	pluginCmd.AddCommand(pluginStartCmd)
	pluginCmd.AddCommand(pluginStopCmd)
	pluginCmd.AddCommand(pluginQueryCmd)
}

// runPluginList executes the plugin list command
func runPluginList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Create component registry
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	// Get all plugins
	plugins := registry.List(component.ComponentKindPlugin)

	if len(plugins) == 0 {
		cmd.Println("No plugins installed.")
		return nil
	}

	// Display results in table format
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tSTATUS\tPORT\tPID\tSOURCE")
	fmt.Fprintln(w, "----\t-------\t------\t----\t---\t------")

	for _, pluginComp := range plugins {
		port := "-"
		if pluginComp.Port > 0 {
			port = fmt.Sprintf("%d", pluginComp.Port)
		}

		pid := "-"
		if pluginComp.PID > 0 {
			pid = fmt.Sprintf("%d", pluginComp.PID)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			pluginComp.Name,
			pluginComp.Version,
			pluginComp.Status,
			port,
			pid,
			pluginComp.Source,
		)
	}

	w.Flush()

	// If context was cancelled, return error
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return nil
}

// runPluginInstall executes the plugin install command
func runPluginInstall(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	repoURL := args[0]

	cmd.Printf("Installing plugin from %s...\n", repoURL)

	// Create component registry
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	// Create installer with dependencies
	gitOps := git.NewDefaultGitOperations()
	builder := build.NewDefaultBuildExecutor()
	installer := component.NewDefaultInstaller(gitOps, builder, registry)

	// Prepare install options
	opts := component.InstallOptions{
		Branch:       pluginInstallBranch,
		Tag:          pluginInstallTag,
		Force:        pluginInstallForce,
		SkipBuild:    pluginInstallSkipBuild,
		SkipRegister: pluginInstallSkipRegister,
	}

	// Install the plugin
	result, err := installer.Install(ctx, repoURL, opts)
	if err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	// Save registry
	if !pluginInstallSkipRegister {
		if err := registry.Save(); err != nil {
			return fmt.Errorf("failed to save registry: %w", err)
		}
	}

	cmd.Printf("Plugin '%s' installed successfully (v%s) in %v\n",
		result.Component.Name,
		result.Component.Version,
		result.Duration)

	if result.BuildOutput != "" && !pluginInstallSkipBuild {
		cmd.Printf("\nBuild output:\n%s\n", result.BuildOutput)
	}

	return nil
}

// runPluginUninstall executes the plugin uninstall command
func runPluginUninstall(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	pluginName := args[0]

	// Create component registry
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	// Check if plugin exists
	existingPlugin := registry.Get(component.ComponentKindPlugin, pluginName)
	if existingPlugin == nil {
		return fmt.Errorf("plugin '%s' not found", pluginName)
	}

	// Confirm uninstall unless --force is set
	if !pluginUninstallForce {
		cmd.Printf("Are you sure you want to uninstall plugin '%s'? (y/N): ", pluginName)
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

	// Stop plugin if running
	if existingPlugin.IsRunning() {
		cmd.Printf("Stopping plugin '%s'...\n", pluginName)
		lifecycleManager := getLifecycleManager()
		if err := lifecycleManager.StopComponent(ctx, existingPlugin); err != nil {
			cmd.PrintErrf("Warning: failed to stop plugin: %v\n", err)
		}
	}

	// Create installer
	gitOps := git.NewDefaultGitOperations()
	builder := build.NewDefaultBuildExecutor()
	installer := component.NewDefaultInstaller(gitOps, builder, registry)

	// Uninstall the plugin
	result, err := installer.Uninstall(ctx, component.ComponentKindPlugin, pluginName)
	if err != nil {
		return fmt.Errorf("uninstall failed: %w", err)
	}

	// Save registry
	if err := registry.Save(); err != nil {
		return fmt.Errorf("failed to save registry: %w", err)
	}

	cmd.Printf("Plugin '%s' uninstalled successfully in %v\n", pluginName, result.Duration)
	return nil
}

// runPluginUpdate executes the plugin update command
func runPluginUpdate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	pluginName := args[0]

	cmd.Printf("Updating plugin '%s'...\n", pluginName)

	// Create component registry
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	// Check if plugin exists
	existingPlugin := registry.Get(component.ComponentKindPlugin, pluginName)
	if existingPlugin == nil {
		return fmt.Errorf("plugin '%s' not found", pluginName)
	}

	// Create installer
	gitOps := git.NewDefaultGitOperations()
	builder := build.NewDefaultBuildExecutor()
	installer := component.NewDefaultInstaller(gitOps, builder, registry)

	// Prepare update options
	opts := component.UpdateOptions{
		Restart:   pluginUpdateRestart,
		SkipBuild: pluginUpdateSkipBuild,
	}

	// Update the plugin
	result, err := installer.Update(ctx, component.ComponentKindPlugin, pluginName, opts)
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	// Save registry
	if err := registry.Save(); err != nil {
		return fmt.Errorf("failed to save registry: %w", err)
	}

	if !result.Updated {
		cmd.Printf("Plugin '%s' is already up to date (v%s)\n", pluginName, result.OldVersion)
		return nil
	}

	cmd.Printf("Plugin '%s' updated successfully (v%s → v%s) in %v\n",
		pluginName,
		result.OldVersion,
		result.NewVersion,
		result.Duration)

	if result.BuildOutput != "" && !pluginUpdateSkipBuild {
		cmd.Printf("\nBuild output:\n%s\n", result.BuildOutput)
	}

	return nil
}

// runPluginShow executes the plugin show command
func runPluginShow(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	pluginName := args[0]

	// Create component registry
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	// Get plugin from component registry
	pluginComp := registry.Get(component.ComponentKindPlugin, pluginName)
	if pluginComp == nil {
		return fmt.Errorf("plugin '%s' not found", pluginName)
	}

	// Display component details
	cmd.Printf("Plugin: %s\n", pluginComp.Name)
	cmd.Printf("Version: %s\n", pluginComp.Version)
	cmd.Printf("Status: %s\n", pluginComp.Status)
	cmd.Printf("Source: %s\n", pluginComp.Source)
	cmd.Printf("Path: %s\n", pluginComp.Path)

	if pluginComp.Port > 0 {
		cmd.Printf("Port: %d\n", pluginComp.Port)
	}

	if pluginComp.PID > 0 {
		cmd.Printf("PID: %d\n", pluginComp.PID)
	}

	cmd.Printf("Created: %s\n", pluginComp.CreatedAt.Format(time.RFC3339))
	cmd.Printf("Updated: %s\n", pluginComp.UpdatedAt.Format(time.RFC3339))

	if pluginComp.StartedAt != nil {
		cmd.Printf("Started: %s\n", pluginComp.StartedAt.Format(time.RFC3339))
	}

	if pluginComp.StoppedAt != nil {
		cmd.Printf("Stopped: %s\n", pluginComp.StoppedAt.Format(time.RFC3339))
	}

	// Display manifest information if available
	if pluginComp.Manifest != nil {
		manifest := pluginComp.Manifest
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
	}

	// If plugin is running, try to get method information from plugin registry
	if pluginComp.IsRunning() {
		cmd.Printf("\nAttempting to query plugin methods...\n")

		pluginRegistry := getPluginRegistry()
		methods, err := pluginRegistry.Methods(pluginName)
		if err != nil {
			cmd.PrintErrf("Warning: could not retrieve methods: %v\n", err)
		} else if len(methods) > 0 {
			cmd.Printf("\nAvailable Methods:\n")
			for _, method := range methods {
				cmd.Printf("  - %s\n", method.Name)
				if method.Description != "" {
					cmd.Printf("    Description: %s\n", method.Description)
				}
				// Display input schema
				inputJSON, err := json.MarshalIndent(method.InputSchema, "    ", "  ")
				if err == nil && len(inputJSON) > 0 {
					cmd.Printf("    Input Schema:\n    %s\n", string(inputJSON))
				}
				// Display output schema
				outputJSON, err := json.MarshalIndent(method.OutputSchema, "    ", "  ")
				if err == nil && len(outputJSON) > 0 {
					cmd.Printf("    Output Schema:\n    %s\n", string(outputJSON))
				}
			}
		}
	} else {
		cmd.Printf("\nNote: Plugin is not running. Start it to query available methods.\n")
	}

	// If context was cancelled, return error
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return nil
}

// runPluginBuild executes the plugin build command
func runPluginBuild(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	pluginName := args[0]

	cmd.Printf("Building plugin '%s'...\n", pluginName)

	// Create component registry
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	// Get plugin
	pluginComp := registry.Get(component.ComponentKindPlugin, pluginName)
	if pluginComp == nil {
		return fmt.Errorf("plugin '%s' not found", pluginName)
	}

	if pluginComp.Manifest == nil || pluginComp.Manifest.Build == nil {
		return fmt.Errorf("plugin '%s' has no build configuration", pluginName)
	}

	// Create builder
	builder := build.NewDefaultBuildExecutor()

	// Prepare build configuration
	buildCfg := pluginComp.Manifest.Build
	buildConfig := build.BuildConfig{
		WorkDir: pluginComp.Path,
		Command: "make",
		Args:    []string{"build"},
		Env:     buildCfg.GetEnv(),
	}

	// Override with manifest build command if specified
	if buildCfg.Command != "" {
		buildConfig.Command = buildCfg.Command
		buildConfig.Args = []string{}
	}

	// Build the plugin
	start := time.Now()
	result, err := builder.Build(ctx, buildConfig, pluginComp.Name, pluginComp.Version, "dev")
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	duration := time.Since(start)

	cmd.Printf("Plugin '%s' built successfully in %v\n", pluginName, duration)

	if result.Stdout != "" {
		cmd.Printf("\nStdout:\n%s\n", result.Stdout)
	}

	if result.Stderr != "" {
		cmd.Printf("\nStderr:\n%s\n", result.Stderr)
	}

	return nil
}

// runPluginStart executes the plugin start command
func runPluginStart(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	pluginName := args[0]

	// Get component registry
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	// Get plugin
	pluginComp := registry.Get(component.ComponentKindPlugin, pluginName)
	if pluginComp == nil {
		return fmt.Errorf("plugin '%s' not found", pluginName)
	}

	// Check if already running
	if pluginComp.IsRunning() {
		return fmt.Errorf("plugin '%s' is already running (PID: %d)", pluginName, pluginComp.PID)
	}

	cmd.Printf("Starting plugin '%s'...\n", pluginName)

	// Get lifecycle manager
	lifecycleManager := getLifecycleManager()

	// Start plugin
	port, err := lifecycleManager.StartComponent(ctx, pluginComp)
	if err != nil {
		return fmt.Errorf("failed to start plugin: %w", err)
	}

	// Registry is already updated since pluginComp is a pointer
	// Save registry
	if err := registry.Save(); err != nil {
		cmd.PrintErrf("Warning: failed to save registry: %v\n", err)
	}

	cmd.Printf("Plugin '%s' started successfully\n", pluginName)
	cmd.Printf("PID: %d\n", pluginComp.PID)
	cmd.Printf("Port: %d\n", port)

	return nil
}

// runPluginStop executes the plugin stop command
func runPluginStop(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	pluginName := args[0]

	// Get component registry
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	// Get plugin
	pluginComp := registry.Get(component.ComponentKindPlugin, pluginName)
	if pluginComp == nil {
		return fmt.Errorf("plugin '%s' not found", pluginName)
	}

	// Check if running
	if !pluginComp.IsRunning() {
		return fmt.Errorf("plugin '%s' is not running", pluginName)
	}

	cmd.Printf("Stopping plugin '%s' (PID: %d)...\n", pluginName, pluginComp.PID)

	// Get lifecycle manager
	lifecycleManager := getLifecycleManager()

	// Stop plugin
	if err := lifecycleManager.StopComponent(ctx, pluginComp); err != nil {
		return fmt.Errorf("failed to stop plugin: %w", err)
	}

	// Registry is already updated since pluginComp is a pointer
	// Save registry
	if err := registry.Save(); err != nil {
		cmd.PrintErrf("Warning: failed to save registry: %v\n", err)
	}

	cmd.Printf("Plugin '%s' stopped successfully\n", pluginName)

	return nil
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

	// Get component registry to verify plugin exists and is running
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	pluginComp := registry.Get(component.ComponentKindPlugin, pluginName)
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
