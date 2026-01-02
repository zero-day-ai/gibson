package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/component"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/attack"
	comp "github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/component/build"
	"github.com/zero-day-ai/gibson/internal/component/git"
	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/payload"
	"github.com/zero-day-ai/gibson/internal/registry"
	"github.com/zero-day-ai/gibson/internal/tui"
	"go.opentelemetry.io/otel"
	"golang.org/x/term"
)

var tuiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch the interactive TUI dashboard",
	Long: `Launch Gibson's interactive Terminal User Interface (TUI) dashboard.

The TUI provides a visual interface for:
- Monitoring mission status and progress
- Interacting with agents in real-time
- Viewing and managing security findings
- System health and metrics

Navigation:
  1-4      Switch between views (Dashboard, Console, Mission, Findings)
  Tab      Cycle focus between panels
  ?        Toggle help overlay
  q        Quit the application

The TUI automatically detects terminal capabilities and adjusts accordingly.`,
	RunE: runTUI,
}

// TUI mode flags
var (
	tuiForce bool
)

func init() {
	tuiCmd.Flags().BoolVar(&tuiForce, "force", false, "Force TUI mode even if terminal detection fails")
}

// runTUI launches the TUI application.
func runTUI(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Check if we're in an interactive terminal
	if !isInteractive() && !tuiForce {
		return fmt.Errorf("not running in an interactive terminal; use --force to override or run without arguments for headless mode")
	}

	return launchTUI(ctx)
}

// isInteractive checks if stdin is a terminal (TTY).
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// launchTUI initializes and runs the TUI application.
func launchTUI(ctx context.Context) error {
	// Load configuration
	cfg, err := loadTUIConfig()
	if err != nil {
		// Config might not exist, that's okay for TUI
		cfg = nil
	}

	// Initialize dependencies
	appConfig, cleanup, err := initializeDependencies(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize dependencies: %w", err)
	}
	defer cleanup()

	// Create the TUI application
	app := tui.NewApp(ctx, appConfig)

	// Create the Bubbletea program with options
	p := tea.NewProgram(
		app,
		tea.WithAltScreen(),       // Use alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support
		tea.WithContext(ctx),      // Pass context for cancellation
	)

	// Wire the program to the executor for agent event streaming
	app.SetProgram(p)

	// Run the program
	_, err = p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

// loadTUIConfig loads the Gibson configuration for TUI.
func loadTUIConfig() (*config.Config, error) {
	// Get home directory
	homeDir := os.Getenv("GIBSON_HOME")
	if homeDir == "" {
		homeDir = config.DefaultHomeDir()
	}

	// Get config file path
	configFile := config.DefaultConfigPath(homeDir)

	// Check if config exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", configFile)
	}

	// Load configuration using config package's loader
	loader := config.NewConfigLoader(config.NewValidator())
	cfg, err := loader.LoadWithDefaults(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return cfg, nil
}

// initializeDependencies creates and initializes all required dependencies for the TUI.
// Returns the app config, a cleanup function, and any error.
func initializeDependencies(ctx context.Context, cfg *config.Config) (tui.AppConfig, func(), error) {
	var appConfig tui.AppConfig
	var cleanupFuncs []func()

	cleanup := func() {
		for i := len(cleanupFuncs) - 1; i >= 0; i-- {
			cleanupFuncs[i]()
		}
	}

	// Set homeDir from config or use default
	if cfg != nil && cfg.Core.HomeDir != "" {
		appConfig.HomeDir = cfg.Core.HomeDir
	} else {
		appConfig.HomeDir = config.DefaultHomeDir()
	}

	// Initialize registry manager if configured
	if cfg != nil {
		regMgr := component.GetRegistryManager(ctx)
		if regMgr == nil {
			// Registry not initialized in root, create one for TUI
			regMgr = initializeRegistryForTUI(ctx, cfg)
			if regMgr != nil {
				cleanupFuncs = append(cleanupFuncs, func() {
					_ = regMgr.Stop(context.Background())
				})
			}
		}
		appConfig.RegistryManager = regMgr
	}

	// Initialize database if configured
	if cfg != nil && cfg.Database.Path != "" {
		// Expand any environment variables or ~ in the path
		dbPath := cfg.Database.Path
		if dbPath[0] == '~' {
			home, _ := os.UserHomeDir()
			dbPath = filepath.Join(home, dbPath[1:])
		}

		db, err := database.Open(dbPath)
		if err != nil {
			// Database is optional for TUI, continue without it
			fmt.Fprintf(os.Stderr, "Warning: failed to open database: %v\n", err)
		} else {
			cleanupFuncs = append(cleanupFuncs, func() { _ = db.Close() })
			appConfig.DB = db
			appConfig.MissionStore = mission.NewDBMissionStore(db)
		}
	}

	// Initialize ComponentDAO if database is available
	if appConfig.DB != nil {
		componentDAO := database.NewComponentDAO(appConfig.DB)
		appConfig.ComponentDAO = componentDAO
	}

	// Initialize finding store if database is available
	if appConfig.DB != nil {
		store := finding.NewDBFindingStore(appConfig.DB)
		appConfig.FindingStore = store
	}

	// Initialize registry adapter if registry manager is available
	if appConfig.RegistryManager != nil {
		// Create registry adapter from the registry manager
		registryAdapter := registry.NewRegistryAdapter(appConfig.RegistryManager.Registry())
		appConfig.RegistryAdapter = registryAdapter
		cleanupFuncs = append(cleanupFuncs, func() {
			_ = registryAdapter.Close()
		})
	}
	// Note: If RegistryManager is nil, TUI will run without agent features.
	// Agent registry is no longer created as a fallback (legacy registry removed).

	// Initialize SessionDAO and StreamManager if database is available
	if appConfig.DB != nil {
		sessionDAO := database.NewSessionDAO(appConfig.DB)

		// Initialize StreamManager with OpenTelemetry tracer
		tracer := otel.Tracer("gibson-tui")
		streamManager := agent.NewStreamManager(ctx, agent.StreamManagerConfig{
			SessionDAO: sessionDAO,
			Tracer:     tracer,
		})
		appConfig.StreamManager = streamManager

		// Add cleanup function for StreamManager
		cleanupFuncs = append(cleanupFuncs, func() {
			_ = streamManager.DisconnectAll()
		})
	}

	// Initialize Installer (always available if ComponentDAO exists)
	if appConfig.ComponentDAO != nil {
		gitOps := git.NewDefaultGitOperations()
		builder := build.NewDefaultBuildExecutor()
		installer := comp.NewDefaultInstaller(gitOps, builder, appConfig.ComponentDAO)
		appConfig.Installer = installer
	}

	// Initialize AttackRunner if all dependencies are available
	// Note: AttackRunner is optional - gracefully degrade if not available
	if appConfig.DB != nil && appConfig.MissionStore != nil && appConfig.FindingStore != nil && appConfig.RegistryAdapter != nil {
		// Create payload registry from database
		payloadRegistry := payload.NewPayloadRegistryWithDefaults(appConfig.DB)

		// Create mission orchestrator (simplified version for TUI)
		// Note: In production, this might be initialized differently
		tracer := otel.Tracer("gibson-attack")

		attackRunner := attack.NewAttackRunner(
			nil, // orchestrator - set to nil for now, will be enhanced
			appConfig.RegistryAdapter,
			payloadRegistry,
			appConfig.MissionStore,
			appConfig.FindingStore,
			attack.WithTracer(tracer),
		)
		appConfig.AttackRunner = attackRunner
	} else {
		// Log that AttackRunner is unavailable
		fmt.Fprintf(os.Stderr, "Warning: AttackRunner not available - missing required dependencies\n")
	}

	return appConfig, cleanup, nil
}

// initializeRegistryForTUI initializes the registry manager for TUI mode.
// This is called when the TUI is launched without going through root command.
func initializeRegistryForTUI(ctx context.Context, cfg *config.Config) *registry.Manager {
	regMgr := registry.NewManager(cfg.Registry)
	if err := regMgr.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to start registry: %v\n", err)
		return nil
	}
	return regMgr
}

// TUIAvailable returns true if the TUI can be launched.
func TUIAvailable() bool {
	return isInteractive()
}
