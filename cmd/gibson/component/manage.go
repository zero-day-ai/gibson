package component

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/internal/component"
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
	clientIface := GetDaemonClient(ctx)
	if clientIface == nil {
		return fmt.Errorf("daemon not running. Start with: gibson daemon start --foreground")
	}

	// Use daemon for listing components
	return runListWithDaemon(cmd, clientIface, cfg)
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

// runShow executes the show command.
func runShow(cmd *cobra.Command, args []string, cfg Config) error {
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

	// Call appropriate method based on component kind
	var comp *daemonclient.ComponentInfo
	var err error

	switch cfg.Kind.String() {
	case "agent":
		comp, err = client.ShowAgent(ctx, componentName)
	case "tool":
		comp, err = client.ShowTool(ctx, componentName)
	case "plugin":
		comp, err = client.ShowPlugin(ctx, componentName)
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
	if comp.Manifest != "" {
		cmd.Printf("\nManifest (JSON):\n%s\n", comp.Manifest)
	}

	return nil
}

// runUninstall executes the uninstall command.
func runUninstall(cmd *cobra.Command, args []string, cfg Config, flags *UninstallFlags) error {
	componentName := args[0]
	ctx := cmd.Context()

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

	// Call appropriate method based on component kind
	var err error

	switch cfg.Kind.String() {
	case "agent":
		err = client.UninstallAgent(ctx, componentName, flags.Force)
	case "tool":
		err = client.UninstallTool(ctx, componentName, flags.Force)
	case "plugin":
		err = client.UninstallPlugin(ctx, componentName, flags.Force)
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

	cmd.Printf("%s '%s' uninstalled successfully\n", titleCaser.String(cfg.DisplayName), componentName)
	return nil
}

// runUpdate executes the update command.
func runUpdate(cmd *cobra.Command, args []string, cfg Config, flags *UpdateFlags) error {
	componentName := args[0]
	ctx := cmd.Context()

	cmd.Printf("Updating %s '%s'...\n", cfg.DisplayName, componentName)

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

	// Build update options
	opts := daemonclient.UpdateOptions{
		Restart:   flags.Restart,
		SkipBuild: flags.SkipBuild,
		Verbose:   flags.Verbose,
	}

	// Call appropriate method based on component kind
	var result *daemonclient.UpdateResult
	var err error

	switch cfg.Kind.String() {
	case "agent":
		result, err = client.UpdateAgent(ctx, componentName, opts)
	case "tool":
		result, err = client.UpdateTool(ctx, componentName, opts)
	case "plugin":
		result, err = client.UpdatePlugin(ctx, componentName, opts)
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

	if !result.Updated {
		cmd.Printf("%s '%s' is already up to date (v%s)\n", titleCaser.String(cfg.DisplayName), componentName, result.OldVersion)
		return nil
	}

	cmd.Printf("%s '%s' updated successfully (v%s → v%s) in %v\n",
		titleCaser.String(cfg.DisplayName),
		componentName,
		result.OldVersion,
		result.NewVersion,
		result.Duration)

	if result.BuildOutput != "" && !flags.SkipBuild && flags.Verbose {
		cmd.Printf("\nBuild output:\n%s\n", result.BuildOutput)
	}

	return nil
}

// runBuild executes the build command.
func runBuild(cmd *cobra.Command, args []string, cfg Config) error {
	componentName := args[0]
	ctx := cmd.Context()

	cmd.Printf("Building %s '%s'...\n", cfg.DisplayName, componentName)

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

	// Call appropriate method based on component kind
	var result *daemonclient.BuildResult
	var err error

	switch cfg.Kind.String() {
	case "agent":
		result, err = client.BuildAgent(ctx, componentName)
	case "tool":
		result, err = client.BuildTool(ctx, componentName)
	case "plugin":
		result, err = client.BuildPlugin(ctx, componentName)
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

	cmd.Printf("%s '%s' built successfully in %v\n", titleCaser.String(cfg.DisplayName), componentName, result.Duration)

	if result.Stdout != "" {
		cmd.Printf("\nStdout:\n%s\n", result.Stdout)
	}

	if result.Stderr != "" {
		cmd.Printf("\nStderr:\n%s\n", result.Stderr)
	}

	return nil
}
