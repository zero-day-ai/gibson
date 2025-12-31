package component

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/component/build"
	"github.com/zero-day-ai/gibson/internal/component/git"
	"github.com/zero-day-ai/gibson/internal/database"
)

// newInstallCommand creates the install cobra.Command for a component type.
func newInstallCommand(cfg Config, flags *InstallFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install REPO_URL",
		Short: fmt.Sprintf("Install a %s from a git repository", cfg.DisplayName),
		Long: fmt.Sprintf(`Install a %s from a git repository URL.

For single-%s repositories:
  gibson %s install https://github.com/user/gibson-%s-example

For mono-repos with multiple %s, use the # fragment to specify the subdirectory:
  gibson %s install https://github.com/user/gibson-%s#subdirectory/name
  gibson %s install git@github.com:user/gibson-%s.git#subdirectory/name

The component.yaml manifest must be in the root of the specified directory.`,
			cfg.DisplayName, cfg.DisplayName, cfg.DisplayName, cfg.DisplayName,
			cfg.DisplayPlural, cfg.DisplayName, cfg.DisplayPlural,
			cfg.DisplayName, cfg.DisplayPlural),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstall(cmd, args, cfg, flags)
		},
	}

	// Bind flags
	cmd.Flags().StringVar(&flags.Branch, "branch", "", "Git branch to install")
	cmd.Flags().StringVar(&flags.Tag, "tag", "", "Git tag to install")
	cmd.Flags().BoolVar(&flags.Force, "force", false, fmt.Sprintf("Force reinstall if %s exists", cfg.DisplayName))
	cmd.Flags().BoolVar(&flags.SkipBuild, "skip-build", false, fmt.Sprintf("Skip building the %s", cfg.DisplayName))
	cmd.Flags().BoolVar(&flags.SkipRegister, "skip-register", false, "Skip registering in component registry")
	cmd.Flags().BoolVarP(&flags.Verbose, "verbose", "v", false, "Show verbose output including build logs")

	return cmd
}

// newInstallAllCommand creates the install-all cobra.Command for a component type.
func newInstallAllCommand(cfg Config, flags *InstallFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install-all REPO_URL",
		Short: fmt.Sprintf("Install all %s from a mono-repo", cfg.DisplayPlural),
		Long: fmt.Sprintf(`Clone a mono-repo and install all %s found within it.

This command recursively walks the repository looking for component.yaml files
and installs each component as a %s.

Examples:
  gibson %s install-all https://github.com/zero-day-ai/gibson-%s-official
  gibson %s install-all git@github.com:zero-day-ai/gibson-%s-official.git

Note: Repositories should contain only %s. Use separate repos for different
component types (tools, agents, plugins).`,
			cfg.DisplayPlural, cfg.DisplayName,
			cfg.DisplayName, cfg.DisplayPlural,
			cfg.DisplayName, cfg.DisplayPlural,
			cfg.DisplayPlural),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstallAll(cmd, args, cfg, flags)
		},
	}

	// Bind flags
	cmd.Flags().StringVar(&flags.Branch, "branch", "", "Git branch to install from")
	cmd.Flags().StringVar(&flags.Tag, "tag", "", "Git tag to install from")
	cmd.Flags().BoolVar(&flags.Force, "force", false, fmt.Sprintf("Force reinstall if %s exist", cfg.DisplayPlural))
	cmd.Flags().BoolVar(&flags.SkipBuild, "skip-build", false, fmt.Sprintf("Skip building the %s", cfg.DisplayPlural))
	cmd.Flags().BoolVar(&flags.SkipRegister, "skip-register", false, "Skip registering in component registry")
	cmd.Flags().BoolVarP(&flags.Verbose, "verbose", "v", false, "Show verbose output including build logs")

	return cmd
}

// runInstall executes the install command logic.
func runInstall(cmd *cobra.Command, args []string, cfg Config, flags *InstallFlags) error {
	ctx := cmd.Context()
	repoURL := args[0]

	cmd.Printf("Installing %s from %s...\n", cfg.DisplayName, repoURL)

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Get installer
	installer := getInstaller(db)

	// Prepare install options
	opts := component.InstallOptions{
		Branch:       flags.Branch,
		Tag:          flags.Tag,
		Force:        flags.Force,
		SkipBuild:    flags.SkipBuild,
		SkipRegister: flags.SkipRegister,
	}

	// Install the component
	result, err := installer.Install(ctx, repoURL, cfg.Kind, opts)
	if err != nil {
		return FormatInstallError(cmd, err)
	}

	cmd.Printf("%s '%s' installed successfully (v%s) in %v\n",
		strings.Title(cfg.DisplayName),
		result.Component.Name,
		result.Component.Version,
		result.Duration)

	if result.BuildOutput != "" && !flags.SkipBuild {
		cmd.Printf("\nBuild output:\n%s\n", result.BuildOutput)
	}

	return nil
}

// runInstallAll executes the install-all command logic.
func runInstallAll(cmd *cobra.Command, args []string, cfg Config, flags *InstallFlags) error {
	ctx := cmd.Context()
	repoURL := args[0]

	cmd.Printf("Installing all %s from %s...\n", cfg.DisplayPlural, repoURL)

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Get installer
	installer := getInstaller(db)

	// Prepare install options
	opts := component.InstallOptions{
		Branch:       flags.Branch,
		Tag:          flags.Tag,
		Force:        flags.Force,
		SkipBuild:    flags.SkipBuild,
		SkipRegister: flags.SkipRegister,
	}

	// Install all components
	result, err := installer.InstallAll(ctx, repoURL, cfg.Kind, opts)
	if err != nil {
		return FormatInstallError(cmd, err)
	}

	// Fail if no components were found
	if result.ComponentsFound == 0 {
		return fmt.Errorf("no %s found in repository %s", cfg.DisplayPlural, repoURL)
	}

	// Print summary
	cmd.Printf("\nInstallation complete in %v\n", result.Duration)
	cmd.Printf("Components found: %d\n", result.ComponentsFound)
	cmd.Printf("Successfully installed: %d\n", len(result.Successful))
	cmd.Printf("Failed: %d\n", len(result.Failed))

	// List successful installations
	if len(result.Successful) > 0 {
		cmd.Printf("\nInstalled %s:\n", cfg.DisplayPlural)
		for _, r := range result.Successful {
			cmd.Printf("  - %s (v%s)\n", r.Component.Name, r.Component.Version)
		}
	}

	// List failures with detailed error information
	if len(result.Failed) > 0 {
		cmd.Printf("\nFailed installations:\n")
		for _, f := range result.Failed {
			name := f.Name
			if name == "" {
				name = f.Path
			}
			cmd.Printf("  - %s: %v\n", name, f.Error)

			// Show detailed build output if available
			var compErr *component.ComponentError
			if errors.As(f.Error, &compErr) && compErr.Context != nil {
				if buildCmd, ok := compErr.Context["build_command"].(string); ok {
					cmd.Printf("    Build command: %s\n", buildCmd)
				}
				if stdout, ok := compErr.Context["stdout"].(string); ok && stdout != "" {
					cmd.Printf("\n    --- Build stdout ---\n")
					// Indent the output
					for _, line := range strings.Split(stdout, "\n") {
						cmd.Printf("    %s\n", line)
					}
				}
				if stderr, ok := compErr.Context["stderr"].(string); ok && stderr != "" {
					cmd.Printf("\n    --- Build stderr ---\n")
					for _, line := range strings.Split(stderr, "\n") {
						cmd.Printf("    %s\n", line)
					}
				}
			}
		}
	}

	return nil
}

// getInstaller creates an installer with all required dependencies.
func getInstaller(db *database.DB) component.Installer {
	gitOps := git.NewDefaultGitOperations()
	builder := build.NewDefaultBuildExecutor()
	dao := database.NewComponentDAO(db)
	return component.NewDefaultInstaller(gitOps, builder, dao)
}

// getGibsonHome returns the Gibson home directory.
func getGibsonHome() (string, error) {
	homeDir := os.Getenv("GIBSON_HOME")
	if homeDir == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		homeDir = userHome + "/.gibson"
	}
	return homeDir, nil
}
