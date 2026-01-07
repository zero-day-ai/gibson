package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zero-day-ai/gibson/internal/tui"
)

func TestIsInteractive(t *testing.T) {
	// In test environment, stdin is usually not a terminal
	// We're just testing that the function doesn't panic
	result := isInteractive()
	// Result will be false in test environment
	_ = result
}

func TestIsTerminalInteractive(t *testing.T) {
	// Same as isInteractive, just testing it doesn't panic
	// Note: isTerminalInteractive was renamed to isInteractive
	result := isInteractive()
	_ = result
}

func TestTUIAvailable(t *testing.T) {
	// Test that TUIAvailable returns same result as isInteractive
	assert.Equal(t, isInteractive(), TUIAvailable())
}

func TestInitializeDependencies_NoConfig(t *testing.T) {
	ctx := context.Background()

	// Test with nil config
	appConfig, cleanup, err := initializeDependencies(ctx, nil)
	defer cleanup()

	// Should not error - config is optional
	assert.NoError(t, err)

	// Registry manager and adapter may be nil without config
	// (That's OK - TUI runs without agent features in that case)

	// DB-dependent things should be nil without config
	assert.Nil(t, appConfig.DB)
	assert.Nil(t, appConfig.MissionStore)
	assert.Nil(t, appConfig.FindingStore)
}

func TestLoadTUIConfig_NoFile(t *testing.T) {
	// Set GIBSON_HOME to a non-existent directory
	oldHome := os.Getenv("GIBSON_HOME")
	os.Setenv("GIBSON_HOME", "/nonexistent/path")
	defer os.Setenv("GIBSON_HOME", oldHome)

	cfg, err := loadTUIConfig()

	// Should error because config file doesn't exist
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "config file not found")
}

func TestTuiCmdFlags(t *testing.T) {
	// Test that the command has the expected flag
	flag := tuiCmd.Flags().Lookup("force")
	assert.NotNil(t, flag)
	assert.Equal(t, "false", flag.DefValue)
}

func TestTuiCmdDescription(t *testing.T) {
	assert.Equal(t, "ui", tuiCmd.Use)
	assert.Contains(t, tuiCmd.Short, "TUI")
	assert.Contains(t, tuiCmd.Long, "Terminal User Interface")
}

func TestAppConfig(t *testing.T) {
	// Test that AppConfig can be created with all nil fields
	config := tui.AppConfig{}

	ctx := context.Background()
	app := tui.NewApp(ctx, config)

	assert.NotNil(t, app)
}

func TestModeFlags(t *testing.T) {
	// Test that mode flags are registered on rootCmd
	printFlag := rootCmd.PersistentFlags().Lookup("print")
	assert.NotNil(t, printFlag, "print flag should be registered")

	tuiFlag := rootCmd.PersistentFlags().Lookup("tui")
	assert.NotNil(t, tuiFlag, "tui flag should be registered")
}

func TestRootCmdHasRunE(t *testing.T) {
	// Test that rootCmd has RunE function (for handling no subcommand)
	assert.NotNil(t, rootCmd.RunE)
}

func TestVersionCmd(t *testing.T) {
	assert.Equal(t, "version", versionCmd.Use)
	assert.Contains(t, versionCmd.Short, "version")
}
