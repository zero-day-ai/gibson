package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestMissionPlanCommand(t *testing.T) {
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
			errorContains: "workflow file path required",
		},
		{
			name: "file does not exist",
			setupFixtures: func(t *testing.T, tmpDir string) string {
				return filepath.Join(tmpDir, "nonexistent.yaml")
			},
			flags: map[string]string{
				"file": "will-be-overridden",
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
  - invalid yaml
    no colon
`
				require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))
				return workflowFile
			},
			flags:         map[string]string{},
			wantError:     true,
			errorContains: "failed to parse workflow file",
		},
		{
			name: "valid workflow - text output",
			setupFixtures: func(t *testing.T, tmpDir string) string {
				workflowFile := filepath.Join(tmpDir, "workflow.yaml")
				content := `
name: Test Workflow
description: A test workflow for planning
nodes:
  - id: recon
    type: agent
    agent: recon-agent
    task:
      target: example.com
  - id: scan
    type: tool
    tool: nmap
    depends_on: [recon]
    task:
      ports: "1-1024"
`
				require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))
				return workflowFile
			},
			flags: map[string]string{
				"output": "text",
			},
			wantError: false,
			outputChecks: []string{
				"Mission:",
				"Resolved at:",
			},
		},
		{
			name: "valid workflow - json output",
			setupFixtures: func(t *testing.T, tmpDir string) string {
				workflowFile := filepath.Join(tmpDir, "workflow.yaml")
				content := `
name: JSON Test Workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
`
				require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))
				return workflowFile
			},
			flags: map[string]string{
				"output": "json",
			},
			wantError: false,
		},
		{
			name: "valid workflow - yaml output",
			setupFixtures: func(t *testing.T, tmpDir string) string {
				workflowFile := filepath.Join(tmpDir, "workflow.yaml")
				content := `
name: YAML Test Workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
`
				require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))
				return workflowFile
			},
			flags: map[string]string{
				"output": "yaml",
			},
			wantError: false,
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
				"output": "xml",
			},
			wantError:     true,
			errorContains: "invalid output format",
		},
		{
			name: "workflow with explicit dependencies",
			setupFixtures: func(t *testing.T, tmpDir string) string {
				workflowFile := filepath.Join(tmpDir, "workflow-deps.yaml")
				content := `
name: Workflow with Dependencies
dependencies:
  agents:
    - agent-one
    - agent-two
  tools:
    - tool-one
  plugins:
    - plugin-one
nodes:
  - id: node1
    type: agent
    agent: agent-one
`
				require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))
				return workflowFile
			},
			flags: map[string]string{
				"output": "text",
			},
			wantError: false,
		},
		{
			name: "workflow with multiple node types",
			setupFixtures: func(t *testing.T, tmpDir string) string {
				workflowFile := filepath.Join(tmpDir, "multi-type.yaml")
				content := `
name: Multi-Type Workflow
nodes:
  - id: agent-node
    type: agent
    agent: my-agent
  - id: tool-node
    type: tool
    tool: my-tool
  - id: plugin-node
    type: plugin
    plugin: my-plugin
    method: getData
`
				require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))
				return workflowFile
			},
			flags: map[string]string{
				"output": "text",
			},
			wantError: false,
		},
		{
			name: "workflow with parallel nodes",
			setupFixtures: func(t *testing.T, tmpDir string) string {
				workflowFile := filepath.Join(tmpDir, "parallel.yaml")
				content := `
name: Parallel Workflow
nodes:
  - id: root
    type: agent
    agent: coordinator
  - id: parallel-group
    type: parallel
    depends_on: [root]
    sub_nodes:
      - id: worker1
        type: agent
        agent: worker-agent
      - id: worker2
        type: agent
        agent: worker-agent
`
				require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))
				return workflowFile
			},
			flags: map[string]string{
				"output": "text",
			},
			wantError: false,
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

			// Create components directory structure (empty, but present)
			componentsDir := filepath.Join(homeDir, "components")
			require.NoError(t, os.MkdirAll(filepath.Join(componentsDir, "agent"), 0755))
			require.NoError(t, os.MkdirAll(filepath.Join(componentsDir, "tool"), 0755))
			require.NoError(t, os.MkdirAll(filepath.Join(componentsDir, "plugin"), 0755))

			// Setup fixtures and get workflow file path
			workflowFile := tt.setupFixtures(t, tmpDir)

			// Create fresh command instance
			cmd := &cobra.Command{
				Use:   "plan",
				Short: "Analyze mission dependencies",
				RunE:  runMissionPlan,
			}

			// Re-initialize flags for this test
			cmd.Flags().StringVarP(&missionPlanFile, "file", "f", "", "Workflow YAML file path")
			cmd.Flags().StringVar(&missionPlanOutput, "output", "text", "Output format")
			cmd.MarkFlagRequired("file")

			cmd.SetContext(context.Background())
			globalFlags.HomeDir = homeDir

			// Set GIBSON_HOME environment variable for the test
			oldGibsonHome := os.Getenv("GIBSON_HOME")
			os.Setenv("GIBSON_HOME", homeDir)
			defer os.Setenv("GIBSON_HOME", oldGibsonHome)

			// Set flags
			if workflowFile != "" {
				cmd.Flags().Set("file", workflowFile)
			}
			for k, v := range tt.flags {
				if k == "file" && workflowFile != "" {
					// Use actual file path instead of placeholder
					cmd.Flags().Set(k, workflowFile)
				} else {
					cmd.Flags().Set(k, v)
				}
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

func TestMissionPlanOutputFormats(t *testing.T) {
	tests := []struct {
		name           string
		outputFormat   string
		validateOutput func(t *testing.T, output string)
	}{
		{
			name:         "text format",
			outputFormat: "text",
			validateOutput: func(t *testing.T, output string) {
				// Text format should have human-readable sections
				assert.Contains(t, output, "Mission:")
				assert.Contains(t, output, "Resolved at:")
			},
		},
		{
			name:         "json format",
			outputFormat: "json",
			validateOutput: func(t *testing.T, output string) {
				// JSON format should be valid JSON
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				assert.NoError(t, err, "Output should be valid JSON")
			},
		},
		{
			name:         "yaml format",
			outputFormat: "yaml",
			validateOutput: func(t *testing.T, output string) {
				// YAML format should be valid YAML
				var result map[string]interface{}
				err := yaml.Unmarshal([]byte(output), &result)
				assert.NoError(t, err, "Output should be valid YAML")
			},
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

			// Create components directory
			componentsDir := filepath.Join(homeDir, "components")
			require.NoError(t, os.MkdirAll(filepath.Join(componentsDir, "agent"), 0755))

			// Create a valid workflow file
			workflowFile := filepath.Join(tmpDir, "workflow.yaml")
			content := `
name: Test Workflow
description: Test workflow for output format testing
nodes:
  - id: node1
    type: agent
    agent: test-agent
    task:
      action: test
`
			require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))

			// Create command
			cmd := &cobra.Command{
				Use:  "plan",
				RunE: runMissionPlan,
			}

			cmd.Flags().StringVarP(&missionPlanFile, "file", "f", "", "Workflow file")
			cmd.Flags().StringVar(&missionPlanOutput, "output", "text", "Output format")
			cmd.MarkFlagRequired("file")

			cmd.SetContext(context.Background())
			globalFlags.HomeDir = homeDir

			// Set GIBSON_HOME
			oldGibsonHome := os.Getenv("GIBSON_HOME")
			os.Setenv("GIBSON_HOME", homeDir)
			defer os.Setenv("GIBSON_HOME", oldGibsonHome)

			// Set flags
			cmd.Flags().Set("file", workflowFile)
			cmd.Flags().Set("output", tt.outputFormat)

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Execute command
			err := cmd.RunE(cmd, []string{})
			assert.NoError(t, err)

			// Validate output format
			output := buf.String()
			assert.NotEmpty(t, output)
			tt.validateOutput(t, output)
		})
	}
}

func TestMissionPlanEmptyWorkflow(t *testing.T) {
	// Save and restore global flags
	oldGlobalFlags := *globalFlags
	defer func() { *globalFlags = oldGlobalFlags }()

	// Setup test environment
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, ".gibson")
	require.NoError(t, os.MkdirAll(homeDir, 0755))

	// Create components directory
	componentsDir := filepath.Join(homeDir, "components")
	require.NoError(t, os.MkdirAll(filepath.Join(componentsDir, "agent"), 0755))

	// Create workflow with no component dependencies - just a join node
	workflowFile := filepath.Join(tmpDir, "empty-deps.yaml")
	content := `
name: Empty Dependencies Workflow
description: Workflow with minimal dependencies
nodes:
  - id: start
    type: agent
    agent: minimal-agent
`
	require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))

	// Create command
	cmd := &cobra.Command{
		Use:  "plan",
		RunE: runMissionPlan,
	}

	cmd.Flags().StringVarP(&missionPlanFile, "file", "f", "", "Workflow file")
	cmd.Flags().StringVar(&missionPlanOutput, "output", "text", "Output format")
	cmd.MarkFlagRequired("file")

	cmd.SetContext(context.Background())
	globalFlags.HomeDir = homeDir

	// Set GIBSON_HOME
	oldGibsonHome := os.Getenv("GIBSON_HOME")
	os.Setenv("GIBSON_HOME", homeDir)
	defer os.Setenv("GIBSON_HOME", oldGibsonHome)

	cmd.Flags().Set("file", workflowFile)
	cmd.Flags().Set("output", "text")

	// Capture output
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Execute command
	err := cmd.RunE(cmd, []string{})
	assert.NoError(t, err)

	// Check output - it should show agents
	output := buf.String()
	assert.True(t, strings.Contains(output, "Agents") || strings.Contains(output, "No dependencies"))
}

func TestMissionPlanRelativeVsAbsolutePath(t *testing.T) {
	// Save and restore global flags
	oldGlobalFlags := *globalFlags
	defer func() { *globalFlags = oldGlobalFlags }()

	// Setup test environment
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, ".gibson")
	require.NoError(t, os.MkdirAll(homeDir, 0755))

	// Create components directory
	componentsDir := filepath.Join(homeDir, "components")
	require.NoError(t, os.MkdirAll(filepath.Join(componentsDir, "agent"), 0755))

	// Create workflow in tmpDir
	workflowFile := filepath.Join(tmpDir, "workflow.yaml")
	content := `
name: Path Test Workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
`
	require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))

	// Test with absolute path
	t.Run("absolute path", func(t *testing.T) {
		cmd := &cobra.Command{
			Use:  "plan",
			RunE: runMissionPlan,
		}

		cmd.Flags().StringVarP(&missionPlanFile, "file", "f", "", "Workflow file")
		cmd.Flags().StringVar(&missionPlanOutput, "output", "text", "Output format")
		cmd.MarkFlagRequired("file")

		cmd.SetContext(context.Background())
		globalFlags.HomeDir = homeDir

		oldGibsonHome := os.Getenv("GIBSON_HOME")
		os.Setenv("GIBSON_HOME", homeDir)
		defer os.Setenv("GIBSON_HOME", oldGibsonHome)

		cmd.Flags().Set("file", workflowFile) // Absolute path

		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		err := cmd.RunE(cmd, []string{})
		assert.NoError(t, err)
	})

	// Test with relative path (change to tmpDir first)
	t.Run("relative path", func(t *testing.T) {
		// Save current directory
		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalDir)

		// Change to tmpDir
		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{
			Use:  "plan",
			RunE: runMissionPlan,
		}

		cmd.Flags().StringVarP(&missionPlanFile, "file", "f", "", "Workflow file")
		cmd.Flags().StringVar(&missionPlanOutput, "output", "text", "Output format")
		cmd.MarkFlagRequired("file")

		cmd.SetContext(context.Background())
		globalFlags.HomeDir = homeDir

		oldGibsonHome := os.Getenv("GIBSON_HOME")
		os.Setenv("GIBSON_HOME", homeDir)
		defer os.Setenv("GIBSON_HOME", oldGibsonHome)

		cmd.Flags().Set("file", "workflow.yaml") // Relative path

		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		err = cmd.RunE(cmd, []string{})
		assert.NoError(t, err)
	})
}

func TestMissionPlanWithComplexDependencies(t *testing.T) {
	// Save and restore global flags
	oldGlobalFlags := *globalFlags
	defer func() { *globalFlags = oldGlobalFlags }()

	// Setup test environment
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, ".gibson")
	require.NoError(t, os.MkdirAll(homeDir, 0755))

	// Create components directory
	componentsDir := filepath.Join(homeDir, "components")
	require.NoError(t, os.MkdirAll(filepath.Join(componentsDir, "agent"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(componentsDir, "tool"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(componentsDir, "plugin"), 0755))

	// Create workflow with complex dependency tree
	workflowFile := filepath.Join(tmpDir, "complex.yaml")
	content := `
name: Complex Dependencies Workflow
description: Workflow with multiple dependency types and levels
dependencies:
  agents:
    - orchestrator-agent
  tools:
    - shared-tool
nodes:
  - id: orchestrator
    type: agent
    agent: orchestrator-agent
    task:
      action: coordinate
  - id: recon
    type: agent
    agent: recon-agent
    depends_on: [orchestrator]
    task:
      target: example.com
  - id: scan-group
    type: parallel
    depends_on: [recon]
    sub_nodes:
      - id: port-scan
        type: tool
        tool: nmap
      - id: vuln-scan
        type: tool
        tool: nikto
  - id: exploit
    type: agent
    agent: exploit-agent
    depends_on: [scan-group]
  - id: report
    type: plugin
    plugin: report-generator
    method: generateReport
    depends_on: [exploit]
`
	require.NoError(t, os.WriteFile(workflowFile, []byte(content), 0644))

	// Create command
	cmd := &cobra.Command{
		Use:  "plan",
		RunE: runMissionPlan,
	}

	cmd.Flags().StringVarP(&missionPlanFile, "file", "f", "", "Workflow file")
	cmd.Flags().StringVar(&missionPlanOutput, "output", "text", "Output format")
	cmd.MarkFlagRequired("file")

	cmd.SetContext(context.Background())
	globalFlags.HomeDir = homeDir

	oldGibsonHome := os.Getenv("GIBSON_HOME")
	os.Setenv("GIBSON_HOME", homeDir)
	defer os.Setenv("GIBSON_HOME", oldGibsonHome)

	cmd.Flags().Set("file", workflowFile)
	cmd.Flags().Set("output", "text")

	// Capture output
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Execute command
	err := cmd.RunE(cmd, []string{})
	assert.NoError(t, err)

	// Verify output contains different component types
	output := buf.String()
	// Output should show the workflow structure
	assert.True(t, strings.Contains(output, "Agents") || strings.Contains(output, "Tools") || strings.Contains(output, "Plugins") || strings.Contains(output, "No dependencies"))
}
