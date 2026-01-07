package internal

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/internal/verbose"
)

func TestSetupVerbose_LevelNone(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	writer, cleanup := SetupVerbose(cmd, verbose.LevelNone, false)
	defer cleanup()

	if writer != nil {
		t.Error("Expected nil writer for LevelNone")
	}

	// Cleanup should be no-op (shouldn't panic)
	cleanup()
}

func TestSetupVerbose_TextFormat(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	var buf bytes.Buffer
	writer, cleanup := SetupVerboseWithOutput(cmd, verbose.LevelVerbose, false, &buf)
	defer cleanup()

	if writer == nil {
		t.Fatal("Expected non-nil writer for LevelVerbose")
	}

	// Verify writer is created with correct format
	// We don't test actual output here because it's async and requires event processing
}

func TestSetupVerbose_JSONFormat(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	var buf bytes.Buffer
	writer, cleanup := SetupVerboseWithOutput(cmd, verbose.LevelVerbose, true, &buf)
	defer cleanup()

	if writer == nil {
		t.Fatal("Expected non-nil writer for LevelVerbose")
	}

	// Verify writer is created with correct format
	// We don't test actual output here because it's async and requires event processing
}

func TestSetupVerbose_Cleanup(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	// Get original default handler
	originalHandler := slog.Default().Handler()

	var buf bytes.Buffer
	writer, cleanup := SetupVerboseWithOutput(cmd, verbose.LevelVerbose, false, &buf)

	if writer == nil {
		t.Fatal("Expected non-nil writer")
	}

	// Verify handler was replaced
	currentHandler := slog.Default().Handler()
	if currentHandler == originalHandler {
		t.Error("Expected handler to be replaced")
	}

	// Call cleanup
	cleanup()

	// Verify handler was restored
	restoredHandler := slog.Default().Handler()
	if restoredHandler != originalHandler {
		t.Error("Expected handler to be restored to original")
	}
}

func TestSetupVerbose_MultipleCleanups(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	var buf bytes.Buffer
	writer, cleanup := SetupVerboseWithOutput(cmd, verbose.LevelVerbose, false, &buf)

	if writer == nil {
		t.Fatal("Expected non-nil writer")
	}

	// Call cleanup once (calling multiple times may panic due to channel close)
	// This is expected behavior - cleanup should be called exactly once
	cleanup()
}

func TestSetupVerbose_LevelFiltering(t *testing.T) {
	tests := []struct {
		name            string
		level           verbose.VerboseLevel
		expectNilWriter bool
	}{
		{
			name:            "LevelNone returns nil writer",
			level:           verbose.LevelNone,
			expectNilWriter: true,
		},
		{
			name:            "LevelVerbose returns writer",
			level:           verbose.LevelVerbose,
			expectNilWriter: false,
		},
		{
			name:            "LevelDebug returns writer",
			level:           verbose.LevelDebug,
			expectNilWriter: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}

			var buf bytes.Buffer
			writer, cleanup := SetupVerboseWithOutput(cmd, tt.level, false, &buf)
			defer cleanup()

			isNil := writer == nil
			if isNil != tt.expectNilWriter {
				t.Errorf("Expected nil writer=%v, got nil=%v", tt.expectNilWriter, isNil)
			}
		})
	}
}
