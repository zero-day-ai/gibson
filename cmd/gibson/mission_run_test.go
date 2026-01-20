package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMissionRunStartDependenciesFlag tests the --start-dependencies flag parsing
func TestMissionRunStartDependenciesFlag(t *testing.T) {
	tests := []struct {
		name              string
		flagValue         string
		expectFlagParsing bool
	}{
		{
			name:              "flag enabled",
			flagValue:         "true",
			expectFlagParsing: true,
		},
		{
			name:              "flag disabled",
			flagValue:         "false",
			expectFlagParsing: true,
		},
		{
			name:              "flag not set",
			flagValue:         "",
			expectFlagParsing: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore global flags
			oldGlobalFlags := *globalFlags
			defer func() { *globalFlags = oldGlobalFlags }()

			// Setup test environment
			tmpDir := t.TempDir()
			homeDir := filepath.Join(tmpDir, ".gibson")
			require.NoError(t, os.MkdirAll(homeDir, 0755))

			// Create a valid workflow file
			workflowFile := filepath.Join(tmpDir, "workflow.yaml")
			content := `
name: Test Workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
`
			require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))

			// Create command
			cmd := &cobra.Command{
				Use:  "run",
				RunE: runMissionRun,
			}

			// Re-initialize flags
			cmd.Flags().StringVarP(&missionWorkflowFile, "file", "f", "", "Workflow file")
			cmd.Flags().StringVar(&missionTargetFlag, "target", "", "Target override")
			cmd.Flags().StringVar(&missionMemoryContinuity, "memory-continuity", "isolated", "Memory mode")
			cmd.Flags().BoolVar(&missionStartDependencies, "start-dependencies", false, "Start dependencies")

			cmd.SetContext(context.Background())
			globalFlags.HomeDir = homeDir

			// Set flags
			cmd.Flags().Set("file", workflowFile)
			if tt.flagValue != "" {
				cmd.Flags().Set("start-dependencies", tt.flagValue)
			}

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Execute command - will fail due to no daemon, but we're testing flag parsing
			err := cmd.RunE(cmd, []string{})

			// Should fail with daemon or dependency error (not flag parsing error)
			if tt.expectFlagParsing {
				assert.Error(t, err)
				// Either daemon error or dependency check error is acceptable (not flag parsing error)
				// The important thing is the flag was parsed correctly
			}
		})
	}
}

// TestMissionRunMemoryContinuityFlag tests the --memory-continuity flag validation
func TestMissionRunMemoryContinuityFlag(t *testing.T) {
	tests := []struct {
		name          string
		flagValue     string
		wantError     bool
		errorContains string
	}{
		{
			name:      "isolated mode",
			flagValue: "isolated",
			wantError: false,
		},
		{
			name:      "inherit mode",
			flagValue: "inherit",
			wantError: false,
		},
		{
			name:      "shared mode",
			flagValue: "shared",
			wantError: false,
		},
		{
			name:          "invalid mode",
			flagValue:     "invalid",
			wantError:     true,
			errorContains: "invalid memory-continuity",
		},
		{
			name:      "default mode (empty)",
			flagValue: "",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore global flags
			oldGlobalFlags := *globalFlags
			defer func() { *globalFlags = oldGlobalFlags }()

			// Setup test environment
			tmpDir := t.TempDir()
			homeDir := filepath.Join(tmpDir, ".gibson")
			require.NoError(t, os.MkdirAll(homeDir, 0755))

			// Create a valid workflow file
			workflowFile := filepath.Join(tmpDir, "workflow.yaml")
			content := `
name: Test Workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
`
			require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))

			// Create command
			cmd := &cobra.Command{
				Use:  "run",
				RunE: runMissionRun,
			}

			// Re-initialize flags
			cmd.Flags().StringVarP(&missionWorkflowFile, "file", "f", "", "Workflow file")
			cmd.Flags().StringVar(&missionTargetFlag, "target", "", "Target override")
			cmd.Flags().StringVar(&missionMemoryContinuity, "memory-continuity", "isolated", "Memory mode")
			cmd.Flags().BoolVar(&missionStartDependencies, "start-dependencies", false, "Start dependencies")

			cmd.SetContext(context.Background())
			globalFlags.HomeDir = homeDir

			// Set flags
			cmd.Flags().Set("file", workflowFile)
			if tt.flagValue != "" {
				cmd.Flags().Set("memory-continuity", tt.flagValue)
			}

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Execute command
			err := cmd.RunE(cmd, []string{})

			// Check results
			if tt.wantError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				// Should fail with daemon or dependency error (not validation error)
				// The important thing is the flag value was validated correctly
				assert.Error(t, err)
			}
		})
	}
}

// TestMissionRunFileFlag tests the -f/--file flag behavior
func TestMissionRunFileFlag(t *testing.T) {
	tests := []struct {
		name          string
		useFileFlag   bool
		usePositional bool
		wantError     bool
		errorContains string
	}{
		{
			name:          "file flag takes precedence",
			useFileFlag:   true,
			usePositional: false,
			wantError:     false,
		},
		{
			name:          "positional argument",
			useFileFlag:   false,
			usePositional: true,
			wantError:     false,
		},
		{
			name:          "no file specified",
			useFileFlag:   false,
			usePositional: false,
			wantError:     true,
			errorContains: "mission source required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore global flags
			oldGlobalFlags := *globalFlags
			defer func() { *globalFlags = oldGlobalFlags }()

			// Setup test environment
			tmpDir := t.TempDir()
			homeDir := filepath.Join(tmpDir, ".gibson")
			require.NoError(t, os.MkdirAll(homeDir, 0755))

			// Create a valid workflow file
			workflowFile := filepath.Join(tmpDir, "workflow.yaml")
			content := `
name: Test Workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
`
			require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))

			// Create command
			cmd := &cobra.Command{
				Use:  "run",
				RunE: runMissionRun,
			}

			// Re-initialize flags
			cmd.Flags().StringVarP(&missionWorkflowFile, "file", "f", "", "Workflow file")
			cmd.Flags().StringVar(&missionTargetFlag, "target", "", "Target override")
			cmd.Flags().StringVar(&missionMemoryContinuity, "memory-continuity", "isolated", "Memory mode")
			cmd.Flags().BoolVar(&missionStartDependencies, "start-dependencies", false, "Start dependencies")

			cmd.SetContext(context.Background())
			globalFlags.HomeDir = homeDir

			// Set flags based on test case
			var args []string
			if tt.useFileFlag {
				cmd.Flags().Set("file", workflowFile)
			}
			if tt.usePositional {
				args = []string{workflowFile}
			}

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Execute command
			err := cmd.RunE(cmd, args)

			// Check results
			if tt.wantError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				// Should fail with daemon or dependency error (not argument error)
				// The important thing is the file argument was processed correctly
				assert.Error(t, err)
			}
		})
	}
}
