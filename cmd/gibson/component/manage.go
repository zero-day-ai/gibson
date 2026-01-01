package component

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/component/build"
	"github.com/zero-day-ai/gibson/internal/component/git"
	"github.com/zero-day-ai/gibson/internal/database"
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

	// Validate flags - cannot use both --local and --remote
	if flags.Local && flags.Remote {
		return fmt.Errorf("cannot use both --local and --remote flags")
	}

	// Open database connection for metadata
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	dbPath := homeDir + "/gibson.db"
	db, err := openDatabase(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create component DAO
	dao := database.NewComponentDAO(db)

	// Use database-only listing (legacy discovery removed)
	return runListFallback(cmd, ctx, cfg, flags, dao)
}

// runListFallback provides database-only listing.
func runListFallback(cmd *cobra.Command, ctx context.Context, cfg Config, flags *ListFlags, dao database.ComponentDAO) error {
	// Get all components of the specified kind from database
	components, err := dao.List(ctx, cfg.Kind)
	if err != nil {
		return fmt.Errorf("failed to list components: %w", err)
	}

	// Apply source filters if specified
	var filtered []*component.Component
	for _, comp := range components {
		if flags.Local && comp.Source != "local" {
			continue
		}
		if flags.Remote && comp.Source != "remote" {
			continue
		}
		filtered = append(filtered, comp)
	}

	if len(filtered) == 0 {
		cmd.Printf("No %s installed.\n", cfg.DisplayPlural)
		return nil
	}

	// Display results in table format
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)

	// Different headers based on component kind
	if cfg.Kind == component.ComponentKindAgent {
		fmt.Fprintln(w, "NAME\tVERSION\tSTATUS\tPORT\tSOURCE")
		fmt.Fprintln(w, "----\t-------\t------\t----\t------")

		for _, comp := range filtered {
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

		for _, comp := range filtered {
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
	ctx := cmd.Context()
	componentName := args[0]

	// Open database connection
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	dbPath := homeDir + "/gibson.db"
	db, err := openDatabase(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create component DAO
	dao := database.NewComponentDAO(db)

	// Get component
	comp, err := dao.GetByName(ctx, cfg.Kind, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		return fmt.Errorf("%s '%s' not found", cfg.DisplayName, componentName)
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
	ctx := cmd.Context()
	componentName := args[0]

	// Open database connection
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	dbPath := homeDir + "/gibson.db"
	db, err := openDatabase(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create component DAO
	dao := database.NewComponentDAO(db)

	// Check if component exists
	existing, err := dao.GetByName(ctx, cfg.Kind, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("%s '%s' not found", cfg.DisplayName, componentName)
	}

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

	// Create installer
	gitOps := git.NewDefaultGitOperations()
	builder := build.NewDefaultBuildExecutor()
	installer := component.NewDefaultInstaller(gitOps, builder, dao)

	// Uninstall the component
	result, err := installer.Uninstall(ctx, cfg.Kind, componentName)
	if err != nil {
		return fmt.Errorf("uninstall failed: %w", err)
	}

	cmd.Printf("%s '%s' uninstalled successfully in %v\n", titleCaser.String(cfg.DisplayName), componentName, result.Duration)
	return nil
}

// runUpdate executes the update command.
func runUpdate(cmd *cobra.Command, args []string, cfg Config, flags *UpdateFlags) error {
	ctx := cmd.Context()
	componentName := args[0]

	cmd.Printf("Updating %s '%s'...\n", cfg.DisplayName, componentName)

	// Open database connection
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	dbPath := homeDir + "/gibson.db"
	db, err := openDatabase(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create component DAO
	dao := database.NewComponentDAO(db)

	// Check if component exists
	existing, err := dao.GetByName(ctx, cfg.Kind, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("%s '%s' not found", cfg.DisplayName, componentName)
	}

	// Create installer
	gitOps := git.NewDefaultGitOperations()
	builder := build.NewDefaultBuildExecutor()
	installer := component.NewDefaultInstaller(gitOps, builder, dao)

	// Prepare update options
	opts := component.UpdateOptions{
		Restart:   flags.Restart,
		SkipBuild: flags.SkipBuild,
	}

	// Update the component
	result, err := installer.Update(ctx, cfg.Kind, componentName, opts)
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
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
	ctx := cmd.Context()
	componentName := args[0]

	cmd.Printf("Building %s '%s'...\n", cfg.DisplayName, componentName)

	// Open database connection
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	dbPath := homeDir + "/gibson.db"
	db, err := openDatabase(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create component DAO
	dao := database.NewComponentDAO(db)

	// Get component
	comp, err := dao.GetByName(ctx, cfg.Kind, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		return fmt.Errorf("%s '%s' not found", cfg.DisplayName, componentName)
	}

	if comp.Manifest == nil || comp.Manifest.Build == nil {
		return fmt.Errorf("%s '%s' has no build configuration", cfg.DisplayName, componentName)
	}

	// Create builder
	builder := build.NewDefaultBuildExecutor()

	// Prepare build configuration
	buildCfg := comp.Manifest.Build
	workDir := comp.RepoPath
	if workDir == "" {
		return fmt.Errorf("%s '%s' has no repository path configured", cfg.DisplayName, componentName)
	}
	buildConfig := build.BuildConfig{
		WorkDir: workDir,
		Command: "make",
		Args:    []string{"build"},
		Env:     buildCfg.GetEnv(),
	}

	// Override with manifest build command if specified
	if buildCfg.Command != "" {
		// Split command into executable and args
		parts := strings.Fields(buildCfg.Command)
		if len(parts) > 0 {
			buildConfig.Command = parts[0]
			buildConfig.Args = parts[1:]
		}
	}

	// Set working directory if specified
	if buildCfg.WorkDir != "" {
		buildConfig.WorkDir = workDir + "/" + buildCfg.WorkDir
	}

	// Build the component
	start := time.Now()
	result, err := builder.Build(ctx, buildConfig, comp.Name, comp.Version, "dev")
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	duration := time.Since(start)

	cmd.Printf("%s '%s' built successfully in %v\n", titleCaser.String(cfg.DisplayName), componentName, duration)

	if result.Stdout != "" {
		cmd.Printf("\nStdout:\n%s\n", result.Stdout)
	}

	if result.Stderr != "" {
		cmd.Printf("\nStderr:\n%s\n", result.Stderr)
	}

	return nil
}

// openDatabase opens the Gibson database at the specified path
func openDatabase(dbPath string) (*database.DB, error) {
	return database.Open(dbPath)
}

