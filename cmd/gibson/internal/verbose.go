package internal

import (
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/internal/verbose"
)

// SetupVerbose configures verbose event handling for a CLI command.
// It creates a VerboseWriter that subscribes to the global event bus and
// replaces the default slog handler with a VerboseAwareHandler.
//
// Parameters:
//   - cmd: The Cobra command (used for error output)
//   - level: The verbosity level (LevelNone disables verbose output)
//   - jsonOutput: If true, uses JSON formatter; otherwise uses text formatter
//
// Returns:
//   - *verbose.VerboseWriter: The writer instance (nil if level is LevelNone)
//   - func(): Cleanup function that restores original handler and stops the writer
//
// Usage:
//
//	writer, cleanup := SetupVerbose(cmd, verbose.LevelVerbose, false)
//	defer cleanup()
//
//	// Now all slog calls and verbose events will be captured
//	slog.Info("this will appear in verbose output")
func SetupVerbose(cmd *cobra.Command, level verbose.VerboseLevel, jsonOutput bool) (*verbose.VerboseWriter, func()) {
	// If verbosity is disabled, return no-op cleanup
	if level == verbose.LevelNone {
		return nil, func() {}
	}

	// Create VerboseWriter with stdout output
	writer := verbose.NewVerboseWriter(os.Stdout, level, jsonOutput)

	// Start the writer to begin consuming events
	ctx := context.Background()
	writer.Start(ctx)

	// Get the current default slog handler
	originalHandler := slog.Default().Handler()

	// Create VerboseAwareHandler that wraps the original handler
	verboseHandler := verbose.NewVerboseAwareHandler(originalHandler, writer.Bus(), level)

	// Replace the default slog handler
	slog.SetDefault(slog.New(verboseHandler))

	// Return cleanup function
	cleanup := func() {
		// Restore original slog handler
		slog.SetDefault(slog.New(originalHandler))

		// Stop the writer
		if writer != nil {
			writer.Stop()
		}
	}

	return writer, cleanup
}

// SetupVerboseWithOutput is like SetupVerbose but allows specifying a custom output writer.
// This is useful for testing or redirecting verbose output to a file.
//
// Parameters:
//   - cmd: The Cobra command (used for error output)
//   - level: The verbosity level (LevelNone disables verbose output)
//   - jsonOutput: If true, uses JSON formatter; otherwise uses text formatter
//   - output: The writer to send verbose output to
//
// Returns:
//   - *verbose.VerboseWriter: The writer instance (nil if level is LevelNone)
//   - func(): Cleanup function that restores original handler and stops the writer
func SetupVerboseWithOutput(cmd *cobra.Command, level verbose.VerboseLevel, jsonOutput bool, output io.Writer) (*verbose.VerboseWriter, func()) {
	// If verbosity is disabled, return no-op cleanup
	if level == verbose.LevelNone {
		return nil, func() {}
	}

	// Create VerboseWriter with custom output
	writer := verbose.NewVerboseWriter(output, level, jsonOutput)

	// Start the writer to begin consuming events
	ctx := context.Background()
	writer.Start(ctx)

	// Get the current default slog handler
	originalHandler := slog.Default().Handler()

	// Create VerboseAwareHandler that wraps the original handler
	verboseHandler := verbose.NewVerboseAwareHandler(originalHandler, writer.Bus(), level)

	// Replace the default slog handler
	slog.SetDefault(slog.New(verboseHandler))

	// Return cleanup function
	cleanup := func() {
		// Restore original slog handler
		slog.SetDefault(slog.New(originalHandler))

		// Stop the writer
		if writer != nil {
			writer.Stop()
		}
	}

	return writer, cleanup
}
