package internal

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/spf13/cobra"
)

func TestSetupVerbose_LevelNone(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	cleanup := SetupVerbose(cmd, LevelNone, false)
	defer cleanup()

	// Cleanup should be no-op (shouldn't panic)
	cleanup()
}

func TestSetupVerbose_TextFormat(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	var buf bytes.Buffer
	cleanup := SetupVerboseWithOutput(cmd, LevelVerbose, false, &buf)
	defer cleanup()
}

func TestSetupVerbose_JSONFormat(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	var buf bytes.Buffer
	cleanup := SetupVerboseWithOutput(cmd, LevelVerbose, true, &buf)
	defer cleanup()
}

func TestSetupVerbose_Cleanup(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	// Get original default handler
	originalHandler := slog.Default().Handler()

	var buf bytes.Buffer
	cleanup := SetupVerboseWithOutput(cmd, LevelVerbose, false, &buf)

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

func TestSetupVerbose_LevelFiltering(t *testing.T) {
	tests := []struct {
		name  string
		level VerboseLevel
	}{
		{
			name:  "LevelNone",
			level: LevelNone,
		},
		{
			name:  "LevelBasic",
			level: LevelBasic,
		},
		{
			name:  "LevelVerbose",
			level: LevelVerbose,
		},
		{
			name:  "LevelDebug",
			level: LevelDebug,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}

			var buf bytes.Buffer
			cleanup := SetupVerboseWithOutput(cmd, tt.level, false, &buf)
			defer cleanup()
		})
	}
}
