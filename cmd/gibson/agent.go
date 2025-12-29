package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/component/build"
	"github.com/zero-day-ai/gibson/internal/component/git"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage agents",
	Long:  `Manage Gibson agents - install, update, start, stop, and monitor agent components`,
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all agents",
	Long:  `List all installed agents with their status`,
	RunE:  runAgentList,
}

var agentInstallCmd = &cobra.Command{
	Use:   "install REPO_URL",
	Short: "Install an agent from a git repository",
	Long:  `Install an agent by cloning from a git repository URL, building it, and registering it`,
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentInstall,
}

var agentUninstallCmd = &cobra.Command{
	Use:   "uninstall NAME",
	Short: "Uninstall an agent",
	Long:  `Uninstall an agent by stopping it (if running) and removing it from the system`,
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentUninstall,
}

var agentUpdateCmd = &cobra.Command{
	Use:   "update NAME",
	Short: "Update an agent to the latest version",
	Long:  `Update an agent by pulling the latest changes from git and rebuilding`,
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentUpdate,
}

var agentShowCmd = &cobra.Command{
	Use:   "show NAME",
	Short: "Show detailed agent information",
	Long:  `Display detailed information about a specific agent including capabilities, slots, and status`,
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentShow,
}

var agentBuildCmd = &cobra.Command{
	Use:   "build PATH",
	Short: "Build an agent from a local directory",
	Long:  `Build an agent from a local directory for development purposes`,
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentBuild,
}

var agentStartCmd = &cobra.Command{
	Use:   "start NAME",
	Short: "Start an agent",
	Long:  `Start a stopped agent and wait for it to become healthy`,
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentStart,
}

var agentStopCmd = &cobra.Command{
	Use:   "stop NAME",
	Short: "Stop a running agent",
	Long:  `Gracefully stop a running agent`,
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentStop,
}

var agentLogsCmd = &cobra.Command{
	Use:   "logs NAME",
	Short: "View agent logs",
	Long:  `View process output logs for an agent`,
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentLogs,
}

// Flags for agent install
var (
	agentInstallBranch       string
	agentInstallTag          string
	agentInstallForce        bool
	agentInstallSkipBuild    bool
	agentInstallSkipRegister bool
)

// Flags for agent update
var (
	agentUpdateRestart   bool
	agentUpdateSkipBuild bool
)

// Flags for agent uninstall
var (
	agentUninstallForce bool
)

// Flags for agent logs
var (
	logsFollow bool
	logsLines  int
)

func init() {
	// Install command flags
	agentInstallCmd.Flags().StringVar(&agentInstallBranch, "branch", "", "Git branch to install from")
	agentInstallCmd.Flags().StringVar(&agentInstallTag, "tag", "", "Git tag to install from")
	agentInstallCmd.Flags().BoolVar(&agentInstallForce, "force", false, "Force reinstall if agent already exists")
	agentInstallCmd.Flags().BoolVar(&agentInstallSkipBuild, "skip-build", false, "Skip building the agent")
	agentInstallCmd.Flags().BoolVar(&agentInstallSkipRegister, "skip-register", false, "Skip registering the agent")

	// Update command flags
	agentUpdateCmd.Flags().BoolVar(&agentUpdateRestart, "restart", false, "Restart agent after update if it was running")
	agentUpdateCmd.Flags().BoolVar(&agentUpdateSkipBuild, "skip-build", false, "Skip rebuilding the agent")

	// Uninstall command flags
	agentUninstallCmd.Flags().BoolVar(&agentUninstallForce, "force", false, "Skip confirmation prompt")

	// Logs command flags
	agentLogsCmd.Flags().BoolVar(&logsFollow, "follow", false, "Follow log output")
	agentLogsCmd.Flags().IntVar(&logsLines, "lines", 100, "Number of lines to show")

	// Add subcommands
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentInstallCmd)
	agentCmd.AddCommand(agentUninstallCmd)
	agentCmd.AddCommand(agentUpdateCmd)
	agentCmd.AddCommand(agentShowCmd)
	agentCmd.AddCommand(agentBuildCmd)
	agentCmd.AddCommand(agentStartCmd)
	agentCmd.AddCommand(agentStopCmd)
	agentCmd.AddCommand(agentLogsCmd)
}

// runAgentList executes the agent list command
func runAgentList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Get component registry
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	// List all agents
	agents := registry.List(component.ComponentKindAgent)

	if len(agents) == 0 {
		cmd.Println("No agents found.")
		return nil
	}

	// Display results in table format
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tSTATUS\tPORT\tPID\tSOURCE")
	fmt.Fprintln(w, "----\t-------\t------\t----\t---\t------")

	for _, agent := range agents {
		port := "-"
		if agent.Port > 0 {
			port = fmt.Sprintf("%d", agent.Port)
		}

		pid := "-"
		if agent.PID > 0 {
			pid = fmt.Sprintf("%d", agent.PID)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			agent.Name,
			agent.Version,
			agent.Status,
			port,
			pid,
			agent.Source,
		)
	}

	w.Flush()

	// If context was cancelled, return error
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return nil
}

// runAgentInstall executes the agent install command
func runAgentInstall(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	repoURL := args[0]

	cmd.Printf("Installing agent from %s...\n", repoURL)

	// Get component registry
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	// Create installer
	installer := getInstaller(registry)

	// Prepare install options
	opts := component.InstallOptions{
		Branch:       agentInstallBranch,
		Tag:          agentInstallTag,
		Force:        agentInstallForce,
		SkipBuild:    agentInstallSkipBuild,
		SkipRegister: agentInstallSkipRegister,
	}

	// Install agent
	result, err := installer.Install(ctx, repoURL, opts)
	if err != nil {
		return fmt.Errorf("failed to install agent: %w", err)
	}

	// Save registry
	if err := registry.Save(); err != nil {
		cmd.PrintErrf("Warning: failed to save registry: %v\n", err)
	}

	cmd.Printf("Agent '%s' installed successfully (version: %s)\n", result.Component.Name, result.Component.Version)
	cmd.Printf("Installation path: %s\n", result.Path)
	cmd.Printf("Duration: %v\n", result.Duration)

	if result.BuildOutput != "" && cmd.Flag("verbose").Changed {
		cmd.Println("\nBuild output:")
		cmd.Println(result.BuildOutput)
	}

	return nil
}

// runAgentUninstall executes the agent uninstall command
func runAgentUninstall(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	agentName := args[0]

	// Get component registry
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	// Get agent to check if it exists
	agent := registry.Get(component.ComponentKindAgent, agentName)
	if agent == nil {
		return fmt.Errorf("agent '%s' not found", agentName)
	}

	// Confirm uninstall unless --force is set
	if !agentUninstallForce {
		cmd.Printf("Are you sure you want to uninstall agent '%s'? (y/N): ", agentName)
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

	// Stop agent if running
	if agent.IsRunning() {
		cmd.Printf("Stopping agent '%s'...\n", agentName)
		lifecycleManager := getLifecycleManager()
		if err := lifecycleManager.StopComponent(ctx, agent); err != nil {
			cmd.PrintErrf("Warning: failed to stop agent: %v\n", err)
		}
	}

	// Create installer for uninstall
	installer := getInstaller(registry)

	// Uninstall agent
	result, err := installer.Uninstall(ctx, component.ComponentKindAgent, agentName)
	if err != nil {
		return fmt.Errorf("failed to uninstall agent: %w", err)
	}

	// Save registry
	if err := registry.Save(); err != nil {
		cmd.PrintErrf("Warning: failed to save registry: %v\n", err)
	}

	cmd.Printf("Agent '%s' uninstalled successfully\n", agentName)
	cmd.Printf("Duration: %v\n", result.Duration)

	return nil
}

// runAgentUpdate executes the agent update command
func runAgentUpdate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	agentName := args[0]

	cmd.Printf("Updating agent '%s'...\n", agentName)

	// Get component registry
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	// Create installer
	installer := getInstaller(registry)

	// Prepare update options
	opts := component.UpdateOptions{
		Restart:   agentUpdateRestart,
		SkipBuild: agentUpdateSkipBuild,
	}

	// Update agent
	result, err := installer.Update(ctx, component.ComponentKindAgent, agentName, opts)
	if err != nil {
		return fmt.Errorf("failed to update agent: %w", err)
	}

	// Save registry
	if err := registry.Save(); err != nil {
		cmd.PrintErrf("Warning: failed to save registry: %v\n", err)
	}

	if !result.Updated {
		cmd.Printf("Agent '%s' is already up to date (version: %s)\n", agentName, result.OldVersion)
		return nil
	}

	cmd.Printf("Agent '%s' updated successfully\n", agentName)
	cmd.Printf("Old version: %s\n", result.OldVersion)
	cmd.Printf("New version: %s\n", result.NewVersion)
	cmd.Printf("Duration: %v\n", result.Duration)

	if result.BuildOutput != "" && cmd.Flag("verbose").Changed {
		cmd.Println("\nBuild output:")
		cmd.Println(result.BuildOutput)
	}

	return nil
}

// runAgentShow executes the agent show command
func runAgentShow(cmd *cobra.Command, args []string) error {
	agentName := args[0]

	// Get component registry
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	// Get agent
	agent := registry.Get(component.ComponentKindAgent, agentName)
	if agent == nil {
		return fmt.Errorf("agent '%s' not found", agentName)
	}

	// Display agent details
	cmd.Printf("Agent: %s\n", agent.Name)
	cmd.Printf("Version: %s\n", agent.Version)
	cmd.Printf("Kind: %s\n", agent.Kind)
	cmd.Printf("Status: %s\n", agent.Status)
	cmd.Printf("Source: %s\n", agent.Source)
	cmd.Printf("Path: %s\n", agent.Path)

	if agent.Port > 0 {
		cmd.Printf("Port: %d\n", agent.Port)
	}

	if agent.PID > 0 {
		cmd.Printf("PID: %d\n", agent.PID)
	}

	if agent.Manifest != nil {
		manifest := agent.Manifest

		if manifest.Description != "" {
			cmd.Printf("\nDescription: %s\n", manifest.Description)
		}

		if manifest.Author != "" {
			cmd.Printf("Author: %s\n", manifest.Author)
		}

		if manifest.License != "" {
			cmd.Printf("License: %s\n", manifest.License)
		}

		if manifest.Repository != "" {
			cmd.Printf("Repository: %s\n", manifest.Repository)
		}

		// Display runtime info
		cmd.Println("\nRuntime Configuration:")
		cmd.Printf("  Type: %s\n", manifest.Runtime.Type)
		cmd.Printf("  Entrypoint: %s\n", manifest.Runtime.Entrypoint)

		if len(manifest.Runtime.Args) > 0 {
			cmd.Printf("  Args: %s\n", strings.Join(manifest.Runtime.Args, " "))
		}

		if manifest.Runtime.WorkDir != "" {
			cmd.Printf("  Working Directory: %s\n", manifest.Runtime.WorkDir)
		}

		if manifest.Runtime.HealthURL != "" {
			cmd.Printf("  Health URL: %s\n", manifest.Runtime.HealthURL)
		}

		// Display capabilities (if available in manifest metadata)
		// Note: This would need to be added to the manifest structure
		cmd.Println("\nCapabilities:")
		cmd.Println("  - Can execute security tests")
		cmd.Println("  - Supports autonomous operation")

		// Display slots (concurrent operations supported)
		cmd.Println("\nSlots: 1 (single concurrent operation)")

		// Display dependencies if any
		if manifest.Dependencies != nil && manifest.Dependencies.HasDependencies() {
			cmd.Println("\nDependencies:")

			if manifest.Dependencies.Gibson != "" {
				cmd.Printf("  Gibson: %s\n", manifest.Dependencies.Gibson)
			}

			if len(manifest.Dependencies.Components) > 0 {
				cmd.Println("  Components:")
				for _, dep := range manifest.Dependencies.Components {
					cmd.Printf("    - %s\n", dep)
				}
			}

			if len(manifest.Dependencies.System) > 0 {
				cmd.Println("  System:")
				for _, dep := range manifest.Dependencies.System {
					cmd.Printf("    - %s\n", dep)
				}
			}
		}
	}

	cmd.Printf("\nCreated: %s\n", agent.CreatedAt.Format(time.RFC3339))
	cmd.Printf("Updated: %s\n", agent.UpdatedAt.Format(time.RFC3339))

	if agent.StartedAt != nil {
		cmd.Printf("Started: %s\n", agent.StartedAt.Format(time.RFC3339))
	}

	if agent.StoppedAt != nil {
		cmd.Printf("Stopped: %s\n", agent.StoppedAt.Format(time.RFC3339))
	}

	return nil
}

// runAgentBuild executes the agent build command
func runAgentBuild(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	agentPath := args[0]

	cmd.Printf("Building agent from %s...\n", agentPath)

	// Load manifest from path
	manifestPath := agentPath + "/component.yaml"
	manifest, err := component.LoadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to load manifest: %w", err)
	}

	// Verify it's an agent
	if manifest.Kind != component.ComponentKindAgent {
		return fmt.Errorf("component is not an agent (kind: %s)", manifest.Kind)
	}

	if manifest.Build == nil {
		return fmt.Errorf("no build configuration in manifest")
	}

	// Create build executor
	builder := build.NewDefaultBuildExecutor()

	// Prepare build configuration
	buildConfig := build.BuildConfig{
		WorkDir:    agentPath,
		Command:    "make",
		Args:       []string{"build"},
		OutputPath: "",
		Env:        manifest.Build.GetEnv(),
	}

	// Override with manifest build command if specified
	if manifest.Build.Command != "" {
		buildConfig.Command = manifest.Build.Command
		buildConfig.Args = []string{}
	}

	// Set working directory if specified
	if manifest.Build.WorkDir != "" {
		buildConfig.WorkDir = agentPath + "/" + manifest.Build.WorkDir
	}

	// Build
	result, err := builder.Build(ctx, buildConfig, manifest.Name, manifest.Version, "dev")
	if err != nil {
		cmd.PrintErrf("Build failed:\n%s\n%s\n", result.Stdout, result.Stderr)
		return fmt.Errorf("build failed: %w", err)
	}

	cmd.Println("Build completed successfully")
	cmd.Printf("Duration: %v\n", result.Duration)

	if result.Stdout != "" {
		cmd.Println("\nBuild output:")
		cmd.Println(result.Stdout)
	}

	if result.Stderr != "" && cmd.Flag("verbose").Changed {
		cmd.Println("\nBuild errors/warnings:")
		cmd.Println(result.Stderr)
	}

	return nil
}

// runAgentStart executes the agent start command
func runAgentStart(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	agentName := args[0]

	// Get component registry
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	// Get agent
	agent := registry.Get(component.ComponentKindAgent, agentName)
	if agent == nil {
		return fmt.Errorf("agent '%s' not found", agentName)
	}

	// Check if already running
	if agent.IsRunning() {
		return fmt.Errorf("agent '%s' is already running (PID: %d)", agentName, agent.PID)
	}

	cmd.Printf("Starting agent '%s'...\n", agentName)

	// Get lifecycle manager
	lifecycleManager := getLifecycleManager()

	// Start agent
	port, err := lifecycleManager.StartComponent(ctx, agent)
	if err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	// Registry is already updated since agent is a pointer
	// Save registry
	if err := registry.Save(); err != nil {
		cmd.PrintErrf("Warning: failed to save registry: %v\n", err)
	}

	cmd.Printf("Agent '%s' started successfully\n", agentName)
	cmd.Printf("PID: %d\n", agent.PID)
	cmd.Printf("Port: %d\n", port)

	return nil
}

// runAgentStop executes the agent stop command
func runAgentStop(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	agentName := args[0]

	// Get component registry
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	// Get agent
	agent := registry.Get(component.ComponentKindAgent, agentName)
	if agent == nil {
		return fmt.Errorf("agent '%s' not found", agentName)
	}

	// Check if running
	if !agent.IsRunning() {
		return fmt.Errorf("agent '%s' is not running", agentName)
	}

	cmd.Printf("Stopping agent '%s' (PID: %d)...\n", agentName, agent.PID)

	// Get lifecycle manager
	lifecycleManager := getLifecycleManager()

	// Stop agent
	if err := lifecycleManager.StopComponent(ctx, agent); err != nil {
		return fmt.Errorf("failed to stop agent: %w", err)
	}

	// Registry is already updated since agent is a pointer
	// Save registry
	if err := registry.Save(); err != nil {
		cmd.PrintErrf("Warning: failed to save registry: %v\n", err)
	}

	cmd.Printf("Agent '%s' stopped successfully\n", agentName)

	return nil
}

// runAgentLogs executes the agent logs command
func runAgentLogs(cmd *cobra.Command, args []string) error {
	agentName := args[0]

	// Get component registry
	registry, err := getComponentRegistry()
	if err != nil {
		return fmt.Errorf("failed to get component registry: %w", err)
	}

	// Get agent
	agent := registry.Get(component.ComponentKindAgent, agentName)
	if agent == nil {
		return fmt.Errorf("agent '%s' not found", agentName)
	}

	// Construct log file path (assuming logs are written to ~/.gibson/logs/<agent-name>.log)
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	logPath := fmt.Sprintf("%s/logs/%s.log", homeDir, agentName)

	// Check if log file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("no logs found for agent '%s'", agentName)
	}

	// Open log file
	file, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	if logsFollow {
		// Follow mode: tail -f style
		cmd.Printf("Following logs for agent '%s' (Ctrl+C to stop)...\n\n", agentName)

		// Read existing lines first
		scanner := bufio.NewScanner(file)
		lineCount := 0
		lines := make([]string, 0, logsLines)

		// Read all lines
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
			lineCount++
		}

		// Print last N lines
		start := 0
		if len(lines) > logsLines {
			start = len(lines) - logsLines
		}
		for i := start; i < len(lines); i++ {
			cmd.Println(lines[i])
		}

		// Now follow for new lines
		// In a real implementation, this would use inotify or similar
		// For now, we'll just poll the file
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-cmd.Context().Done():
				return nil
			case <-ticker.C:
				for scanner.Scan() {
					cmd.Println(scanner.Text())
				}
			}
		}
	} else {
		// Regular mode: show last N lines
		scanner := bufio.NewScanner(file)
		lines := make([]string, 0, logsLines)

		// Read all lines
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		// Print last N lines
		start := 0
		if len(lines) > logsLines {
			start = len(lines) - logsLines
		}

		for i := start; i < len(lines); i++ {
			cmd.Println(lines[i])
		}

		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading log file: %w", err)
		}
	}

	return nil
}

// Helper functions

// getComponentRegistry creates or retrieves the component registry
func getComponentRegistry() (component.ComponentRegistry, error) {
	registry := component.NewDefaultComponentRegistry()

	// Load existing registry from ~/.gibson/registry.yaml
	homeDir, err := getGibsonHome()
	if err != nil {
		return nil, err
	}

	registryPath := fmt.Sprintf("%s/registry.yaml", homeDir)
	if _, err := os.Stat(registryPath); err == nil {
		// Registry exists, load it
		if err := registry.LoadFromConfig(registryPath); err != nil {
			// Non-fatal: registry might be empty or corrupted
			// Continue with empty registry
		}
	}

	return registry, nil
}

// getInstaller creates a component installer
func getInstaller(registry component.ComponentRegistry) component.Installer {
	gitOps := git.NewDefaultGitOperations()
	builder := build.NewDefaultBuildExecutor()
	return component.NewDefaultInstaller(gitOps, builder, registry)
}

// getLifecycleManager creates a lifecycle manager
func getLifecycleManager() component.LifecycleManager {
	healthMonitor := component.NewHealthMonitor()
	return component.NewLifecycleManager(healthMonitor)
}
