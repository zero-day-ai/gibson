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

func TestMissionValidateCommand(t *testing.T) {
	tests := []struct {
		name          string
		setupFixtures func(t *testing.T, tmpDir string) string
		flags         map[string]string
		wantError     bool
		errorContains string
		outputChecks  []string
	}{
		{
			name: "missing file flag",
			setupFixtures: func(t *testing.T, tmpDir string) string {
				return ""
			},
			flags:         map[string]string{},
			wantError:     true,
			errorContains: "workflow file is required",
		},
		{
			name: "file does not exist",
			setupFixtures: func(t *testing.T, tmpDir string) string {
				return filepath.Join(tmpDir, "nonexistent.yaml")
			},
			flags: map[string]string{
				"file": filepath.Join("tmpDir", "nonexistent.yaml"),
			},
			wantError:     true,
			errorContains: "workflow file not found",
		},
		{
			name: "invalid YAML format",
			setupFixtures: func(t *testing.T, tmpDir string) string {
				workflowFile := filepath.Join(tmpDir, "invalid.yaml")
				content := `
name: Invalid Workflow
nodes:
  - invalid yaml structure
    missing colon
`
				require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))
				return workflowFile
			},
			flags:         map[string]string{},
			wantError:     true,
			errorContains: "failed to parse workflow file",
		},
		{
			name: "valid workflow - minimal",
			setupFixtures: func(t *testing.T, tmpDir string) string {
				workflowFile := filepath.Join(tmpDir, "minimal.yaml")
				content := `
name: Test Workflow
description: A minimal test workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
    task:
      action: test
`
				require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))
				return workflowFile
			},
			flags: map[string]string{},
			// Note: This will fail in unit tests without daemon, but validates parsing
			wantError:     true,
			errorContains: "daemon",
		},
		{
			name: "invalid output format",
			setupFixtures: func(t *testing.T, tmpDir string) string {
				workflowFile := filepath.Join(tmpDir, "workflow.yaml")
				content := `
name: Test Workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
`
				require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))
				return workflowFile
			},
			flags: map[string]string{
				"output": "invalid",
			},
			wantError:     true,
			errorContains: "invalid output format",
		},
		{
			name: "auto-install flag not implemented",
			setupFixtures: func(t *testing.T, tmpDir string) string {
				workflowFile := filepath.Join(tmpDir, "workflow.yaml")
				content := `
name: Test Workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
`
				require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))
				return workflowFile
			},
			flags: map[string]string{
				"auto-install": "true",
			},
			wantError:     true,
			errorContains: "--auto-install is not yet implemented",
		},
		{
			name: "workflow with dependencies section",
			setupFixtures: func(t *testing.T, tmpDir string) string {
				workflowFile := filepath.Join(tmpDir, "workflow-with-deps.yaml")
				content := `
name: Test Workflow with Dependencies
description: Workflow with explicit dependencies
dependencies:
  agents:
    - recon-agent
    - scan-agent
  tools:
    - nmap
    - nikto
  plugins:
    - kubernetes-plugin
nodes:
  - id: recon
    type: agent
    agent: recon-agent
    task:
      target: example.com
  - id: scan
    type: agent
    agent: scan-agent
    depends_on: [recon]
    task:
      target: example.com
`
				require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))
				return workflowFile
			},
			flags: map[string]string{},
			// Will fail without daemon, but validates parsing
			wantError:     true,
			errorContains: "daemon",
		},
		{
			name: "workflow with multiple node types",
			setupFixtures: func(t *testing.T, tmpDir string) string {
				workflowFile := filepath.Join(tmpDir, "workflow-multi-types.yaml")
				content := `
name: Multi-Type Workflow
description: Workflow with agents, tools, and plugins
nodes:
  - id: agent-node
    type: agent
    agent: test-agent
    task:
      action: test
  - id: tool-node
    type: tool
    tool: test-tool
    task:
      action: scan
  - id: plugin-node
    type: plugin
    plugin: test-plugin
    method: query
    task:
      params: "data"
`
				require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))
				return workflowFile
			},
			flags:         map[string]string{},
			wantError:     true,
			errorContains: "daemon",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore global flags to avoid test pollution
			oldGlobalFlags := *globalFlags
			defer func() { *globalFlags = oldGlobalFlags }()

			// Setup test environment
			tmpDir := t.TempDir()
			homeDir := filepath.Join(tmpDir, ".gibson")
			require.NoError(t, os.MkdirAll(homeDir, 0755))

			// Setup fixtures and get workflow file path
			workflowFile := tt.setupFixtures(t, tmpDir)

			// Create fresh command instance to avoid flag pollution
			cmd := &cobra.Command{
				Use:   "validate",
				Short: "Validate mission dependencies",
				RunE:  runMissionValidate,
			}

			// Re-initialize flags for this test
			cmd.Flags().StringVarP(&missionValidateFile, "file", "f", "", "Path to workflow YAML file (required)")
			cmd.Flags().StringVar(&missionValidateOutput, "output", "text", "Output format: text, json, yaml")
			cmd.Flags().BoolVar(&missionValidateAutoInstall, "auto-install", false, "Attempt to install missing components")
			cmd.MarkFlagRequired("file")

			cmd.SetContext(context.Background())
			globalFlags.HomeDir = homeDir

			// Set flags
			if workflowFile != "" && tt.flags["file"] == "" {
				cmd.Flags().Set("file", workflowFile)
			}
			for k, v := range tt.flags {
				cmd.Flags().Set(k, v)
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
				assert.NoError(t, err)
				output := buf.String()
				for _, check := range tt.outputChecks {
					assert.Contains(t, output, check)
				}
			}
		})
	}
}

func TestMissionValidateOutputFormats(t *testing.T) {
	// Test that output format flag is validated correctly
	tests := []struct {
		name          string
		outputFormat  string
		wantError     bool
		errorContains string
	}{
		{
			name:         "text format",
			outputFormat: "text",
			wantError:    false,
		},
		{
			name:         "json format",
			outputFormat: "json",
			wantError:    false,
		},
		{
			name:         "yaml format",
			outputFormat: "yaml",
			wantError:    false,
		},
		{
			name:          "invalid format",
			outputFormat:  "xml",
			wantError:     true,
			errorContains: "invalid output format",
		},
		{
			name:         "empty format falls back to default",
			outputFormat: "",
			wantError:    false, // Should use default "text"
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
				Use:  "validate",
				RunE: runMissionValidate,
			}

			cmd.Flags().StringVarP(&missionValidateFile, "file", "f", "", "Path to workflow YAML file")
			cmd.Flags().StringVar(&missionValidateOutput, "output", "text", "Output format")
			cmd.Flags().BoolVar(&missionValidateAutoInstall, "auto-install", false, "Auto-install")
			cmd.MarkFlagRequired("file")

			cmd.SetContext(context.Background())
			globalFlags.HomeDir = homeDir

			// Set flags
			cmd.Flags().Set("file", workflowFile)
			if tt.outputFormat != "" {
				cmd.Flags().Set("output", tt.outputFormat)
			}

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Execute command
			err := cmd.RunE(cmd, []string{})

			// Check results - we expect daemon error unless testing invalid format
			if tt.wantError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				// Should fail with daemon error (not format error)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "daemon")
			}
		})
	}
}
