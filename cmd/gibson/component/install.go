package component

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	daemonclient "github.com/zero-day-ai/gibson/internal/daemon/client"
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
	repoURL := args[0]
	ctx := cmd.Context()

	cmd.Printf("Installing %s from %s...\n", cfg.DisplayName, repoURL)

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

	// Build install options
	opts := daemonclient.InstallOptions{
		Branch:    flags.Branch,
		Tag:       flags.Tag,
		Force:     flags.Force,
		SkipBuild: flags.SkipBuild,
		Verbose:   flags.Verbose,
	}

	// Call appropriate method based on component kind
	var result *daemonclient.InstallResult
	var err error

	switch cfg.Kind.String() {
	case "agent":
		result, err = client.InstallAgent(ctx, repoURL, opts)
	case "tool":
		result, err = client.InstallTool(ctx, repoURL, opts)
	case "plugin":
		result, err = client.InstallPlugin(ctx, repoURL, opts)
	default:
		return fmt.Errorf("unsupported component kind: %s", cfg.Kind)
	}

	if err != nil {
		// Check if it's a connection error
		if strings.Contains(err.Error(), "daemon not responding") {
			return fmt.Errorf("daemon not running. Start with: gibson daemon start --foreground")
		}
		return FormatInstallError(cmd, err)
	}

	cmd.Printf("%s '%s' installed successfully (v%s) in %v\n",
		titleCaser.String(cfg.DisplayName),
		result.Name,
		result.Version,
		result.Duration)

	if result.BuildOutput != "" && !flags.SkipBuild {
		cmd.Printf("\nBuild output:\n%s\n", result.BuildOutput)
	}

	return nil
}

// runInstallAll executes the install-all command logic.
func runInstallAll(cmd *cobra.Command, args []string, cfg Config, flags *InstallFlags) error {
	repoURL := args[0]
	ctx := cmd.Context()

	cmd.Printf("Installing all %s from %s...\n", cfg.DisplayPlural, repoURL)

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

	// Build install options
	opts := daemonclient.InstallOptions{
		Branch:    flags.Branch,
		Tag:       flags.Tag,
		Force:     flags.Force,
		SkipBuild: flags.SkipBuild,
		Verbose:   flags.Verbose,
	}

	// Call appropriate method based on component kind
	var result *daemonclient.InstallAllResult
	var err error

	switch cfg.Kind.String() {
	case "agent":
		result, err = client.InstallAllAgent(ctx, repoURL, opts)
	case "tool":
		result, err = client.InstallAllTool(ctx, repoURL, opts)
	case "plugin":
		result, err = client.InstallAllPlugin(ctx, repoURL, opts)
	default:
		return fmt.Errorf("unsupported component kind: %s", cfg.Kind)
	}

	if err != nil {
		// Check if it's a connection error
		if strings.Contains(err.Error(), "daemon not responding") {
			return fmt.Errorf("daemon not running. Start with: gibson daemon start --foreground")
		}
		return FormatInstallError(cmd, err)
	}

	// Print summary
	cmd.Printf("\nInstallation complete in %v\n", result.Duration)
	cmd.Printf("Components found: %d\n", result.ComponentsFound)
	cmd.Printf("Successfully installed: %d, Skipped: %d, Failed: %d\n",
		result.SuccessfulCount, result.SkippedCount, result.FailedCount)

	// List successful installations
	if len(result.Successful) > 0 {
		cmd.Printf("\nInstalled %s:\n", cfg.DisplayPlural)
		for _, r := range result.Successful {
			versionStr := ""
			if r.Version != "" {
				versionStr = fmt.Sprintf(" (v%s)", r.Version)
			}
			cmd.Printf("  - %s%s\n", r.Name, versionStr)
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
			cmd.Printf("  - %s: %s\n", name, f.Error)
		}
	}

	// List skipped components
	if len(result.Skipped) > 0 {
		cmd.Printf("\nSkipped (already installed):\n")
		for _, s := range result.Skipped {
			cmd.Printf("  - %s\n", s.Name)
		}
	}

	return nil
}
