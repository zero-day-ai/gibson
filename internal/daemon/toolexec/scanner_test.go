package toolexec

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/schema"
)

func TestBinaryScanner_Scan(t *testing.T) {
	tests := []struct {
		name          string
		setupTools    func(t *testing.T, dir string) []string
		wantToolCount int
		wantErrors    int
		validateTools func(t *testing.T, tools []ToolBinaryInfo)
	}{
		{
			name: "successfully scans valid tools",
			setupTools: func(t *testing.T, dir string) []string {
				tool1 := createMockTool(t, dir, "tool1", validSchemaResponse("tool1", "1.0.0"))
				tool2 := createMockTool(t, dir, "tool2", validSchemaResponse("tool2", "2.0.0"))
				return []string{tool1, tool2}
			},
			wantToolCount: 2,
			wantErrors:    0,
			validateTools: func(t *testing.T, tools []ToolBinaryInfo) {
				// Find tool1 and tool2
				var tool1, tool2 *ToolBinaryInfo
				for i := range tools {
					if tools[i].Name == "tool1" {
						tool1 = &tools[i]
					} else if tools[i].Name == "tool2" {
						tool2 = &tools[i]
					}
				}

				require.NotNil(t, tool1, "tool1 should be discovered")
				require.NotNil(t, tool2, "tool2 should be discovered")

				assert.NoError(t, tool1.Error)
				assert.NoError(t, tool2.Error)
				assert.Equal(t, "object", tool1.InputSchema.Type)
				assert.Equal(t, "object", tool1.OutputSchema.Type)
			},
		},
		{
			name: "handles tools with schema fetch failures",
			setupTools: func(t *testing.T, dir string) []string {
				tool1 := createMockTool(t, dir, "valid-tool", validSchemaResponse("valid-tool", "1.0.0"))
				tool2 := createMockTool(t, dir, "invalid-tool", "invalid json")
				return []string{tool1, tool2}
			},
			wantToolCount: 2,
			wantErrors:    1,
			validateTools: func(t *testing.T, tools []ToolBinaryInfo) {
				// One tool should have succeeded, one should have failed
				var validTool, invalidTool *ToolBinaryInfo
				for i := range tools {
					if tools[i].Name == "valid-tool" {
						validTool = &tools[i]
					} else if tools[i].Name == "invalid-tool" {
						invalidTool = &tools[i]
					}
				}

				require.NotNil(t, validTool, "valid-tool should be discovered")
				require.NotNil(t, invalidTool, "invalid-tool should be discovered")

				assert.NoError(t, validTool.Error)
				assert.Error(t, invalidTool.Error, "invalid tool should have error")
			},
		},
		{
			name: "skips non-executable files",
			setupTools: func(t *testing.T, dir string) []string {
				tool1 := createMockTool(t, dir, "executable-tool", validSchemaResponse("executable-tool", "1.0.0"))

				// Create a non-executable file
				nonExecPath := filepath.Join(dir, "not-executable")
				err := os.WriteFile(nonExecPath, []byte("#!/bin/sh\necho test"), 0644)
				require.NoError(t, err)

				return []string{tool1}
			},
			wantToolCount: 1,
			wantErrors:    0,
			validateTools: func(t *testing.T, tools []ToolBinaryInfo) {
				assert.Len(t, tools, 1)
				assert.Equal(t, "executable-tool", tools[0].Name)
			},
		},
		{
			name: "handles empty directory",
			setupTools: func(t *testing.T, dir string) []string {
				return []string{}
			},
			wantToolCount: 0,
			wantErrors:    0,
			validateTools: func(t *testing.T, tools []ToolBinaryInfo) {
				assert.Empty(t, tools)
			},
		},
		{
			name: "handles nested directories",
			setupTools: func(t *testing.T, dir string) []string {
				// Create subdirectory
				subDir := filepath.Join(dir, "subdir")
				err := os.MkdirAll(subDir, 0755)
				require.NoError(t, err)

				tool1 := createMockTool(t, dir, "root-tool", validSchemaResponse("root-tool", "1.0.0"))
				tool2 := createMockTool(t, subDir, "nested-tool", validSchemaResponse("nested-tool", "1.0.0"))
				return []string{tool1, tool2}
			},
			wantToolCount: 2,
			wantErrors:    0,
			validateTools: func(t *testing.T, tools []ToolBinaryInfo) {
				assert.Len(t, tools, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary tools directory
			tmpDir := t.TempDir()

			// Setup tools in directory
			tt.setupTools(t, tmpDir)

			// Create scanner and scan
			scanner := NewBinaryScanner()
			ctx := context.Background()
			tools, err := scanner.Scan(ctx, tmpDir)

			// Verify results
			require.NoError(t, err, "Scan should not return error")
			assert.Len(t, tools, tt.wantToolCount, "tool count mismatch")

			// Count tools with errors
			errorCount := 0
			for _, tool := range tools {
				if tool.Error != nil {
					errorCount++
				}
			}
			assert.Equal(t, tt.wantErrors, errorCount, "error count mismatch")

			// Run custom validation
			if tt.validateTools != nil {
				tt.validateTools(t, tools)
			}
		})
	}
}

func TestBinaryScanner_Scan_NonExistentDirectory(t *testing.T) {
	scanner := NewBinaryScanner()
	ctx := context.Background()

	// Scan a directory that doesn't exist
	tools, err := scanner.Scan(ctx, "/nonexistent/path/to/tools")
	require.NoError(t, err, "should not error on non-existent directory")
	assert.Empty(t, tools, "should return empty slice for non-existent directory")
}

func TestBinaryScanner_Scan_NotADirectory(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "not-a-dir")
	err := os.WriteFile(filePath, []byte("test"), 0644)
	require.NoError(t, err)

	scanner := NewBinaryScanner()
	ctx := context.Background()

	// Try to scan a file instead of a directory
	_, err = scanner.Scan(ctx, filePath)
	assert.Error(t, err, "should error when path is not a directory")
	assert.Contains(t, err.Error(), "not a directory")
}

func TestBinaryScanner_GetSchema(t *testing.T) {
	tests := []struct {
		name           string
		toolOutput     string
		wantErr        bool
		errContains    string
		validateSchema func(t *testing.T, schema *ToolSchema)
	}{
		{
			name:       "successfully parses valid schema",
			toolOutput: validSchemaResponse("test-tool", "1.0.0"),
			wantErr:    false,
			validateSchema: func(t *testing.T, schema *ToolSchema) {
				assert.Equal(t, "object", schema.InputSchema.Type)
				assert.Equal(t, "object", schema.OutputSchema.Type)
				assert.NotEmpty(t, schema.InputSchema.Properties)
			},
		},
		{
			name:        "fails on invalid JSON",
			toolOutput:  "not valid json",
			wantErr:     true,
			errContains: "failed to parse schema JSON",
		},
		{
			name:        "fails on missing input schema",
			toolOutput:  `{"name":"test","version":"1.0.0","output_schema":{"type":"object"}}`,
			wantErr:     true,
			errContains: "missing input_schema",
		},
		{
			name:        "fails on missing output schema",
			toolOutput:  `{"name":"test","version":"1.0.0","input_schema":{"type":"object"}}`,
			wantErr:     true,
			errContains: "missing output_schema",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			toolPath := createMockTool(t, tmpDir, "test-tool", tt.toolOutput)

			scanner := NewBinaryScanner()
			ctx := context.Background()

			schema, err := scanner.GetSchema(ctx, toolPath)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, schema)
				if tt.validateSchema != nil {
					tt.validateSchema(t, schema)
				}
			}
		})
	}
}

func TestBinaryScanner_GetSchema_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout test in short mode")
	}

	tmpDir := t.TempDir()

	// Create a tool that sleeps longer than the timeout (5 seconds)
	// Note: CommandContext with shell scripts may not terminate immediately,
	// so this test verifies the error message rather than exact timing.
	toolPath := createSlowTool(t, tmpDir, "slow-tool", 10*time.Second)

	scanner := NewBinaryScanner()
	ctx := context.Background()

	_, err := scanner.GetSchema(ctx, toolPath)

	// Verify we got a timeout error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout", "should timeout on slow tool")
}

func TestBinaryScanner_GetSchema_NonZeroExit(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a tool that exits with error
	toolPath := createErrorTool(t, tmpDir, "error-tool")

	scanner := NewBinaryScanner()
	ctx := context.Background()

	_, err := scanner.GetSchema(ctx, toolPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema fetch failed")
}

// Helper functions

// createMockTool creates an executable shell script that outputs the given schema JSON
func createMockTool(t *testing.T, dir, name, schemaJSON string) string {
	toolPath := filepath.Join(dir, name)

	// Create shell script that outputs schema when called with --schema
	script := `#!/bin/sh
if [ "$1" = "--schema" ]; then
  cat <<'EOF'
` + schemaJSON + `
EOF
  exit 0
fi
exit 1
`
	err := os.WriteFile(toolPath, []byte(script), 0755)
	require.NoError(t, err)

	return toolPath
}

// createSlowTool creates a tool that sleeps for the specified duration
func createSlowTool(t *testing.T, dir, name string, duration time.Duration) string {
	toolPath := filepath.Join(dir, name)

	// Convert duration to seconds for shell sleep command
	seconds := int(duration.Seconds())
	script := fmt.Sprintf(`#!/bin/sh
sleep %d
echo '{"input_schema":{},"output_schema":{}}'
`, seconds)
	err := os.WriteFile(toolPath, []byte(script), 0755)
	require.NoError(t, err)

	return toolPath
}

// createErrorTool creates a tool that exits with error
func createErrorTool(t *testing.T, dir, name string) string {
	toolPath := filepath.Join(dir, name)

	script := `#!/bin/sh
echo "Error message" >&2
exit 1
`
	err := os.WriteFile(toolPath, []byte(script), 0755)
	require.NoError(t, err)

	return toolPath
}

// validSchemaResponse generates a valid schema JSON response
func validSchemaResponse(name, version string) string {
	schemaResp := map[string]interface{}{
		"name":        name,
		"version":     version,
		"description": name + " description",
		"tags":        []string{"test", "mock"},
		"input_schema": schema.JSONSchema{
			Type: "object",
			Properties: map[string]schema.SchemaField{
				"input": {
					Type:        "string",
					Description: "Input parameter",
				},
			},
			Required: []string{"input"},
		},
		"output_schema": schema.JSONSchema{
			Type: "object",
			Properties: map[string]schema.SchemaField{
				"result": {
					Type:        "string",
					Description: "Output result",
				},
			},
			Required: []string{"result"},
		},
	}

	bytes, err := json.Marshal(schemaResp)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}
