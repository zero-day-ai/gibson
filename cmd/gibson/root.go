package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/internal/config"
	"golang.org/x/term"
)

// Mode flags for TUI vs headless operation
var (
	printMode bool // Force headless/print mode
	tuiMode   bool // Force TUI mode
)

var rootCmd = &cobra.Command{
	Use:   "gibson",
	Short: "Gibson - Autonomous LLM Red-Teaming Framework",
	Long: `Gibson is an autonomous AI security testing platform for
red-teaming LLM systems, RAG pipelines, and AI agents.

When run without a subcommand in an interactive terminal, Gibson
launches the TUI dashboard. Use --print to force headless mode.`,
	PersistentPreRunE: loadConfig,
	SilenceUsage:      true,
	SilenceErrors:     true,
	RunE:              runRootCmd,
}

// Execute runs the root command with signal handling
func Execute(ctx context.Context) error {
	// Create context with signal handling
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return rootCmd.ExecuteContext(ctx)
}

// loadConfig is called before any command runs to load configuration
func loadConfig(cmd *cobra.Command, args []string) error {
	// Parse global flags
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return err
	}

	// Determine home directory
	homeDir := flags.HomeDir
	if homeDir == "" {
		homeDir = os.Getenv("GIBSON_HOME")
	}
	if homeDir == "" {
		homeDir = config.DefaultHomeDir()
	}

	// Determine config file path
	configFile := flags.ConfigFile
	if configFile == "" {
		configFile = config.DefaultConfigPath(homeDir)
	}

	// For init, version, status, and help commands, skip config loading since config may not exist yet
	if cmd.Name() == "init" || cmd.Name() == "version" || cmd.Name() == "status" || cmd.Name() == "help" {
		return nil
	}

	// Check if config exists, but don't fail if it doesn't
	// Commands should handle missing config gracefully
	if _, err := os.Stat(configFile); err != nil {
		if os.IsNotExist(err) && flags.IsVerbose() {
			cmd.PrintErrf("Config file not found at %s (run 'gibson init' to create)\n", configFile)
		}
	}

	return nil
}

func init() {
	// Register global flags
	RegisterGlobalFlags(rootCmd)

	// Register TUI/print mode flags
	rootCmd.PersistentFlags().BoolVar(&printMode, "print", false, "Force headless/print mode (no TUI)")
	rootCmd.PersistentFlags().BoolVar(&tuiMode, "tui", false, "Force TUI mode even if not interactive")

	// Add subcommands
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(targetCmd)
	rootCmd.AddCommand(credentialCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(toolCmd)
	rootCmd.AddCommand(pluginCmd)
	rootCmd.AddCommand(missionCmd)
	rootCmd.AddCommand(findingCmd)
	rootCmd.AddCommand(attackCmd)
	rootCmd.AddCommand(payloadCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(tuiCmd)
}

// runRootCmd handles the root command when run without subcommands.
// By default, it launches the TUI if in an interactive terminal.
func runRootCmd(cmd *cobra.Command, args []string) error {
	// Determine mode based on flags and environment
	if printMode {
		// Force headless mode - show status summary
		return runStatusSummary(cmd)
	}

	if tuiMode || isTerminalInteractive() {
		// Launch TUI
		return launchTUI(cmd.Context())
	}

	// Non-interactive without --tui, show help
	return cmd.Help()
}

// isTerminalInteractive checks if stdin is a terminal.
func isTerminalInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// runStatusSummary prints a status summary in headless mode.
func runStatusSummary(cmd *cobra.Command) error {
	cmd.Println("Gibson - Autonomous LLM Red-Teaming Framework")
	cmd.Println("Version: v0.1.0 (Stage 15 - TUI)")
	cmd.Println("")
	cmd.Println("Run 'gibson status' for system status")
	cmd.Println("Run 'gibson --tui' to launch the interactive dashboard")
	cmd.Println("Run 'gibson help' for available commands")
	return nil
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Println("Gibson v0.1.0 (Stage 15 - TUI)")
	},
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for Gibson.

To load completions:

Bash:

  $ source <(gibson completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ gibson completion bash > /etc/bash_completion.d/gibson
  # macOS:
  $ gibson completion bash > $(brew --prefix)/etc/bash_completion.d/gibson

Zsh:

  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ gibson completion zsh > "${fpath[1]}/_gibson"

  # You will need to start a new shell for this setup to take effect.

Fish:

  $ gibson completion fish | source

  # To load completions for each session, execute once:
  $ gibson completion fish > ~/.config/fish/completions/gibson.fish

PowerShell:

  PS> gibson completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> gibson completion powershell > gibson.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			_ = cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			_ = cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			_ = cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			_ = cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		}
	},
}
