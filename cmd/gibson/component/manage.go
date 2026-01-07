package component

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/core"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/component/build"
	daemonclient "github.com/zero-day-ai/gibson/internal/daemon/client"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var titleCaser = cases.Title(language.English)

// newListCommand creates a list command for the specified component type.
func newListCommand(cfg Config, flags *ListFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: fmt.Sprintf("List all installed %s", cfg.DisplayPlural),
		Long:  fmt.Sprintf("List all installed %s with their status and metadata", cfg.DisplayPlural),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, args, cfg, flags)
		},
	}

	// Bind flags
	cmd.Flags().BoolVar(&flags.Local, "local", false, "Show only local components")
	cmd.Flags().BoolVar(&flags.Remote, "remote", false, "Show only remote components")

	return cmd
}

// newShowCommand creates a show command for the specified component type.
func newShowCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "show NAME",
		Short: fmt.Sprintf("Show detailed %s information", cfg.DisplayName),
		Long:  fmt.Sprintf("Display detailed information about a %s including its manifest and configuration", cfg.DisplayName),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(cmd, args, cfg)
		},
	}
}

// newUninstallCommand creates an uninstall command for the specified component type.
func newUninstallCommand(cfg Config, flags *UninstallFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall NAME",
		Short: fmt.Sprintf("Uninstall a %s", cfg.DisplayName),
		Long:  fmt.Sprintf("Remove an installed %s from the system", cfg.DisplayName),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUninstall(cmd, args, cfg, flags)
		},
	}

	// Bind flags
	cmd.Flags().BoolVar(&flags.Force, "force", false, "Skip confirmation prompt")

	return cmd
}

// newUpdateCommand creates an update command for the specified component type.
func newUpdateCommand(cfg Config, flags *UpdateFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update NAME",
		Short: fmt.Sprintf("Update a %s to the latest version", cfg.DisplayName),
		Long:  fmt.Sprintf("Pull latest changes and rebuild a %s", cfg.DisplayName),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd, args, cfg, flags)
		},
	}

	// Bind flags
	cmd.Flags().BoolVar(&flags.Restart, "restart", false, fmt.Sprintf("Restart %s after update if it was running", cfg.DisplayName))
	cmd.Flags().BoolVar(&flags.SkipBuild, "skip-build", false, fmt.Sprintf("Skip rebuilding the %s", cfg.DisplayName))
	cmd.Flags().BoolVarP(&flags.Verbose, "verbose", "v", false, "Show verbose output including build logs")

	return cmd
}

// newBuildCommand creates a build command for the specified component type.
func newBuildCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "build NAME",
		Short: fmt.Sprintf("Build a %s locally", cfg.DisplayName),
		Long:  fmt.Sprintf("Build or rebuild a %s for local development", cfg.DisplayName),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBuild(cmd, args, cfg)
		},
	}
}

// runList executes the list command.
func runList(cmd *cobra.Command, args []string, cfg Config, flags *ListFlags) error {
	ctx := cmd.Context()

	// Check for daemon client in context
	if clientIface := GetDaemonClient(ctx); clientIface != nil {
		// Daemon is available - use it for listing components
		return runListWithDaemon(cmd, clientIface, cfg)
	}

	// No daemon - fall back to local database queries
	return runListWithDatabase(cmd, cfg, flags)
}

// runListWithDaemon lists components using the daemon client
func runListWithDaemon(cmd *cobra.Command, clientIface interface{}, cfg Config) error {
	// Type assert to get the actual client
	client, ok := clientIface.(*daemonclient.Client)
	if !ok {
		return fmt.Errorf("invalid daemon client type")
	}

	ctx := cmd.Context()

	// Handle different component kinds
	switch cfg.Kind {
	case component.ComponentKindAgent:
		agents, err := client.ListAgents(ctx)
		if err != nil {
			// Check if it's a connection error
			if strings.Contains(err.Error(), "daemon not responding") {
				return fmt.Errorf("daemon not running. Start with: gibson daemon start --foreground")
			}
			return fmt.Errorf("failed to list agents from daemon: %w", err)
		}

		if len(agents) == 0 {
			cmd.Printf("No %s registered.\n", cfg.DisplayPlural)
			return nil
		}

		// Display agents in table format
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tVERSION\tSTATUS\tADDRESS")
		fmt.Fprintln(w, "----\t-------\t------\t-------")

		for _, agent := range agents {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				agent.Name,
				agent.Version,
				agent.Status,
				agent.Address,
			)
		}

		w.Flush()
		return nil

	case component.ComponentKindTool:
		tools, err := client.ListTools(ctx)
		if err != nil {
			// Check if it's a connection error
			if strings.Contains(err.Error(), "daemon not responding") {
				return fmt.Errorf("daemon not running. Start with: gibson daemon start --foreground")
			}
			return fmt.Errorf("failed to list tools from daemon: %w", err)
		}

		if len(tools) == 0 {
			cmd.Printf("No %s registered.\n", cfg.DisplayPlural)
			return nil
		}

		// Display tools in table format
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tVERSION\tSTATUS\tADDRESS")
		fmt.Fprintln(w, "----\t-------\t------\t-------")

		for _, tool := range tools {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				tool.Name,
				tool.Version,
				tool.Status,
				tool.Address,
			)
		}

		w.Flush()
		return nil

	case component.ComponentKindPlugin:
		plugins, err := client.ListPlugins(ctx)
		if err != nil {
			// Check if it's a connection error
			if strings.Contains(err.Error(), "daemon not responding") {
				return fmt.Errorf("daemon not running. Start with: gibson daemon start --foreground")
			}
			return fmt.Errorf("failed to list plugins from daemon: %w", err)
		}

		if len(plugins) == 0 {
			cmd.Printf("No %s registered.\n", cfg.DisplayPlural)
			return nil
		}

		// Display plugins in table format
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tVERSION\tSTATUS\tADDRESS")
		fmt.Fprintln(w, "----\t-------\t------\t-------")

		for _, plugin := range plugins {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				plugin.Name,
				plugin.Version,
				plugin.Status,
				plugin.Address,
			)
		}

		w.Flush()
		return nil

	default:
		return fmt.Errorf("unsupported component kind: %s", cfg.Kind)
	}
}

// runListWithDatabase lists components using local database (fallback)
func runListWithDatabase(cmd *cobra.Command, cfg Config, flags *ListFlags) error {
	// Build command context
	cc, err := buildCommandContext(cmd)
	if err != nil {
		return err
	}
	defer cc.Close()

	// Build list options
	opts := core.ListOptions{
		Local:  flags.Local,
		Remote: flags.Remote,
	}

	// Call core function
	result, err := core.ComponentList(cc, cfg.Kind, opts)
	if err != nil {
		return err
	}

	// Format output
	components, ok := result.Data.([]*component.Component)
	if !ok {
		return fmt.Errorf("unexpected result type")
	}

	if len(components) == 0 {
		cmd.Printf("No %s installed.\n", cfg.DisplayPlural)
		return nil
	}

	// Display results in table format
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)

	// Different headers based on component kind
	if cfg.Kind == component.ComponentKindAgent {
		fmt.Fprintln(w, "NAME\tVERSION\tSTATUS\tPORT\tSOURCE")
		fmt.Fprintln(w, "----\t-------\t------\t----\t------")

		for _, comp := range components {
			port := "-"
			if comp.Port > 0 {
				port = fmt.Sprintf("%d", comp.Port)
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				comp.Name,
				comp.Version,
				comp.Status,
				port,
				comp.Source,
			)
		}
	} else {
		// For tools and plugins
		fmt.Fprintln(w, "NAME\tVERSION\tSTATUS\tSOURCE\tPATH")
		fmt.Fprintln(w, "----\t-------\t------\t------\t----")

		for _, comp := range components {
			path := comp.RepoPath
			if path == "" {
				path = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				comp.Name,
				comp.Version,
				comp.Status,
				comp.Source,
				path,
			)
		}
	}

	w.Flush()
	return nil
}

// runShow executes the show command.
func runShow(cmd *cobra.Command, args []string, cfg Config) error {
	componentName := args[0]

	// Build command context
	cc, err := buildCommandContext(cmd)
	if err != nil {
		return err
	}
	defer cc.Close()

	// Call core function
	result, err := core.ComponentShow(cc, cfg.Kind, componentName)
	if err != nil {
		return err
	}

	// Extract component from result
	comp, ok := result.Data.(*component.Component)
	if !ok {
		return fmt.Errorf("unexpected result type")
	}

	// Display component details
	cmd.Printf("%s: %s\n", titleCaser.String(cfg.DisplayName), comp.Name)
	cmd.Printf("Version: %s\n", comp.Version)
	cmd.Printf("Status: %s\n", comp.Status)
	cmd.Printf("Source: %s\n", comp.Source)
	if comp.RepoPath != "" {
		cmd.Printf("Repo Path: %s\n", comp.RepoPath)
	}
	if comp.BinPath != "" {
		cmd.Printf("Binary Path: %s\n", comp.BinPath)
	}

	if comp.Port > 0 {
		cmd.Printf("Port: %d\n", comp.Port)
	}

	if comp.PID > 0 {
		cmd.Printf("PID: %d\n", comp.PID)
	}

	cmd.Printf("Created: %s\n", comp.CreatedAt.Format(time.RFC3339))
	cmd.Printf("Updated: %s\n", comp.UpdatedAt.Format(time.RFC3339))

	if comp.StartedAt != nil {
		cmd.Printf("Started: %s\n", comp.StartedAt.Format(time.RFC3339))
	}

	if comp.StoppedAt != nil {
		cmd.Printf("Stopped: %s\n", comp.StoppedAt.Format(time.RFC3339))
	}

	// Display manifest information if available
	if comp.Manifest != nil {
		manifest := comp.Manifest
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
		if manifest.Runtime != nil {
			cmd.Printf("\nRuntime:\n")
			cmd.Printf("  Type: %s\n", manifest.Runtime.Type)
			cmd.Printf("  Entrypoint: %s\n", manifest.Runtime.Entrypoint)

			if len(manifest.Runtime.Args) > 0 {
				cmd.Printf("  Args: %s\n", strings.Join(manifest.Runtime.Args, " "))
			}

			if manifest.Runtime.WorkDir != "" {
				cmd.Printf("  Working Directory: %s\n", manifest.Runtime.WorkDir)
			}

			if manifest.Runtime.Port > 0 {
				cmd.Printf("  Port: %d\n", manifest.Runtime.Port)
			}

			if manifest.Runtime.HealthURL != "" {
				cmd.Printf("  Health URL: %s\n", manifest.Runtime.HealthURL)
			}
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

// runUninstall executes the uninstall command.
func runUninstall(cmd *cobra.Command, args []string, cfg Config, flags *UninstallFlags) error {
	componentName := args[0]

	// Confirm uninstall unless --force is set
	if !flags.Force {
		cmd.Printf("Are you sure you want to uninstall %s '%s'? (y/N): ", cfg.DisplayName, componentName)
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

	// Build command context
	cc, err := buildCommandContext(cmd)
	if err != nil {
		return err
	}
	defer cc.Close()

	// Build uninstall options
	opts := core.UninstallOptions{
		Force: flags.Force,
	}

	// Call core function
	result, err := core.ComponentUninstall(cc, cfg.Kind, componentName, opts)
	if err != nil {
		return err
	}

	cmd.Printf("%s '%s' uninstalled successfully in %v\n", titleCaser.String(cfg.DisplayName), componentName, result.Duration)
	return nil
}

// runUpdate executes the update command.
func runUpdate(cmd *cobra.Command, args []string, cfg Config, flags *UpdateFlags) error {
	componentName := args[0]

	cmd.Printf("Updating %s '%s'...\n", cfg.DisplayName, componentName)

	// Build command context
	cc, err := buildCommandContext(cmd)
	if err != nil {
		return err
	}
	defer cc.Close()

	// Build update options
	opts := core.UpdateOptions{
		Restart:   flags.Restart,
		SkipBuild: flags.SkipBuild,
		Verbose:   flags.Verbose,
	}

	// Call core function
	result, err := core.ComponentUpdate(cc, cfg.Kind, componentName, opts)
	if err != nil {
		return err
	}

	// Extract update result
	updateResult, ok := result.Data.(*component.UpdateResult)
	if !ok {
		return fmt.Errorf("unexpected result type")
	}

	if !updateResult.Updated {
		cmd.Printf("%s '%s' is already up to date (v%s)\n", titleCaser.String(cfg.DisplayName), componentName, updateResult.OldVersion)
		return nil
	}

	cmd.Printf("%s '%s' updated successfully (v%s → v%s) in %v\n",
		titleCaser.String(cfg.DisplayName),
		componentName,
		updateResult.OldVersion,
		updateResult.NewVersion,
		result.Duration)

	if updateResult.BuildOutput != "" && !flags.SkipBuild && flags.Verbose {
		cmd.Printf("\nBuild output:\n%s\n", updateResult.BuildOutput)
	}

	return nil
}

// runBuild executes the build command.
func runBuild(cmd *cobra.Command, args []string, cfg Config) error {
	componentName := args[0]

	cmd.Printf("Building %s '%s'...\n", cfg.DisplayName, componentName)

	// Build command context
	cc, err := buildCommandContext(cmd)
	if err != nil {
		return err
	}
	defer cc.Close()

	// Call core function
	result, err := core.ComponentBuild(cc, cfg.Kind, componentName)
	if err != nil {
		return err
	}

	// Extract build result
	buildResult, ok := result.Data.(*build.BuildResult)
	if !ok {
		return fmt.Errorf("unexpected result type")
	}

	cmd.Printf("%s '%s' built successfully in %v\n", titleCaser.String(cfg.DisplayName), componentName, result.Duration)

	if buildResult.Stdout != "" {
		cmd.Printf("\nStdout:\n%s\n", buildResult.Stdout)
	}

	if buildResult.Stderr != "" {
		cmd.Printf("\nStderr:\n%s\n", buildResult.Stderr)
	}

	return nil
}
