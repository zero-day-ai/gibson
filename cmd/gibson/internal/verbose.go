package internal

import (
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

// VerboseLevel represents the verbosity level for output.
type VerboseLevel int

const (
	// LevelNone disables verbose output
	LevelNone VerboseLevel = iota
	// LevelBasic shows basic information
	LevelBasic
	// LevelVerbose shows detailed information
	LevelVerbose
	// LevelDebug shows all debug information
	LevelDebug
)

// SetupVerbose configures verbose logging for a CLI command.
// It adjusts the slog level based on the verbosity setting.
//
// Parameters:
//   - cmd: The Cobra command (used for error output)
//   - level: The verbosity level (LevelNone uses default logging)
//   - jsonOutput: If true, uses JSON format; otherwise uses text format
//
// Returns:
//   - func(): Cleanup function that restores original handler
func SetupVerbose(cmd *cobra.Command, level VerboseLevel, jsonOutput bool) func() {
	return SetupVerboseWithOutput(cmd, level, jsonOutput, os.Stdout)
}

// SetupVerboseWithOutput is like SetupVerbose but allows specifying a custom output writer.
func SetupVerboseWithOutput(cmd *cobra.Command, level VerboseLevel, jsonOutput bool, output io.Writer) func() {
	// If verbosity is disabled, return no-op cleanup
	if level == LevelNone {
		return func() {}
	}

	// Map verbose level to slog level
	var slogLevel slog.Level
	switch level {
	case LevelBasic:
		slogLevel = slog.LevelInfo
	case LevelVerbose:
		slogLevel = slog.LevelDebug
	case LevelDebug:
		slogLevel = slog.LevelDebug - 4 // More verbose than debug
	default:
		slogLevel = slog.LevelInfo
	}

	// Get the current default slog handler
	originalHandler := slog.Default().Handler()

	// Create new handler with appropriate format and level
	var newHandler slog.Handler
	opts := &slog.HandlerOptions{Level: slogLevel}
	if jsonOutput {
		newHandler = slog.NewJSONHandler(output, opts)
	} else {
		newHandler = slog.NewTextHandler(output, opts)
	}

	// Replace the default slog handler
	slog.SetDefault(slog.New(newHandler))

	// Return cleanup function
	return func() {
		slog.SetDefault(slog.New(originalHandler))
	}
}
