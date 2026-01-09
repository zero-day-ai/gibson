// Package toolexec provides integration tests for the Tool Executor Service.
//
//go:build integration
// +build integration

package toolexec

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestToolExecutorService_EndToEnd tests the full tool execution flow:
// 1. Creates a test tool binary that supports --schema and subprocess mode
// 2. Starts the Tool Executor Service
// 3. Verifies tool discovery
// 4. Executes the tool
// 5. Verifies output
func TestToolExecutorService_EndToEnd(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temporary tools directory
	toolsDir := t.TempDir()

	// Create a simple test tool binary
	testToolPath := filepath.Join(toolsDir, "testtool")
	createTestToolBinary(t, testToolPath)

	// Create the service with the test tools directory
	scanner := NewDefaultBinaryScanner()
	executor := NewDefaultSubprocessExecutor()
	service := NewToolExecutorServiceImpl(scanner, executor, toolsDir, 5*time.Minute)

	// Start the service
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := service.Start(ctx)
	require.NoError(t, err, "Service should start successfully")
	defer service.Stop(ctx)

	// Verify tool was discovered
	tools := service.ListTools()
	require.Len(t, tools, 1, "Should discover exactly one tool")
	assert.Equal(t, "testtool", tools[0].Name)
	assert.Equal(t, "1.0.0", tools[0].Version)
	assert.Equal(t, "ready", tools[0].Status)

	// Execute the tool
	input := map[string]any{
		"message": "Hello, World!",
	}
	output, err := service.Execute(ctx, "testtool", input, 10*time.Second)
	require.NoError(t, err, "Tool execution should succeed")

	// Verify output
	assert.Equal(t, "echo", output["action"])
	assert.Equal(t, "Hello, World!", output["message"])
	assert.Equal(t, true, output["success"])

	// Verify metrics were recorded
	metrics := service.(*ToolExecutorServiceImpl).GetMetrics("testtool")
	require.NotNil(t, metrics)
	assert.Equal(t, int64(1), metrics.TotalExecutions)
	assert.Equal(t, int64(1), metrics.SuccessfulExecutions)
	assert.Equal(t, int64(0), metrics.FailedExecutions)
}

// TestToolExecutorService_ToolNotFound tests error handling for missing tools
func TestToolExecutorService_ToolNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	toolsDir := t.TempDir()
	scanner := NewDefaultBinaryScanner()
	executor := NewDefaultSubprocessExecutor()
	service := NewToolExecutorServiceImpl(scanner, executor, toolsDir, 5*time.Minute)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// Try to execute non-existent tool
	_, err = service.Execute(ctx, "nonexistent", map[string]any{}, 10*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestToolExecutorService_Timeout tests execution timeout handling
func TestToolExecutorService_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	toolsDir := t.TempDir()

	// Create a slow tool that sleeps
	slowToolPath := filepath.Join(toolsDir, "slowtool")
	createSlowToolBinary(t, slowToolPath)

	scanner := NewDefaultBinaryScanner()
	executor := NewDefaultSubprocessExecutor()
	service := NewToolExecutorServiceImpl(scanner, executor, toolsDir, 5*time.Minute)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// Execute with very short timeout
	_, err = service.Execute(ctx, "slowtool", map[string]any{}, 100*time.Millisecond)
	require.Error(t, err)
	// Should get a timeout error
}

// TestToolExecutorService_HotReload tests adding a new tool while running
func TestToolExecutorService_HotReload(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	toolsDir := t.TempDir()

	scanner := NewDefaultBinaryScanner()
	executor := NewDefaultSubprocessExecutor()
	service := NewToolExecutorServiceImpl(scanner, executor, toolsDir, 5*time.Minute)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// Initially no tools
	tools := service.ListTools()
	assert.Len(t, tools, 0)

	// Add a tool
	testToolPath := filepath.Join(toolsDir, "newtool")
	createTestToolBinary(t, testToolPath)

	// Refresh and verify
	err = service.RefreshTools(ctx)
	require.NoError(t, err)

	tools = service.ListTools()
	assert.Len(t, tools, 1)
	assert.Equal(t, "newtool", tools[0].Name)
}

// createTestToolBinary creates a Go-based test tool binary
func createTestToolBinary(t *testing.T, path string) {
	t.Helper()

	// Create source file
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "main.go")

	src := `package main

import (
	"encoding/json"
	"flag"
	"os"
)

type Schema struct {
	Name         string                 ` + "`json:\"name\"`" + `
	Version      string                 ` + "`json:\"version\"`" + `
	Description  string                 ` + "`json:\"description\"`" + `
	Tags         []string               ` + "`json:\"tags\"`" + `
	InputSchema  map[string]interface{} ` + "`json:\"input_schema\"`" + `
	OutputSchema map[string]interface{} ` + "`json:\"output_schema\"`" + `
}

func main() {
	schemaFlag := flag.Bool("schema", false, "Output schema")
	flag.Parse()

	if *schemaFlag {
		schema := Schema{
			Name:        "testtool",
			Version:     "1.0.0",
			Description: "A test tool for integration testing",
			Tags:        []string{"test", "integration"},
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"message": map[string]interface{}{"type": "string"},
				},
			},
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action":  map[string]interface{}{"type": "string"},
					"message": map[string]interface{}{"type": "string"},
					"success": map[string]interface{}{"type": "boolean"},
				},
			},
		}
		json.NewEncoder(os.Stdout).Encode(schema)
		return
	}

	// Read input from stdin
	var input map[string]interface{}
	json.NewDecoder(os.Stdin).Decode(&input)

	// Echo the input with action
	output := map[string]interface{}{
		"action":  "echo",
		"message": input["message"],
		"success": true,
	}
	json.NewEncoder(os.Stdout).Encode(output)
}
`
	err := os.WriteFile(srcFile, []byte(src), 0644)
	require.NoError(t, err)

	// Compile the tool
	cmd := exec.Command("go", "build", "-o", path, srcFile)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to compile test tool: %s", string(output))
}

// createSlowToolBinary creates a tool that sleeps for a long time
func createSlowToolBinary(t *testing.T, path string) {
	t.Helper()

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "main.go")

	src := `package main

import (
	"encoding/json"
	"flag"
	"os"
	"time"
)

func main() {
	schemaFlag := flag.Bool("schema", false, "Output schema")
	flag.Parse()

	if *schemaFlag {
		schema := map[string]interface{}{
			"name":        "slowtool",
			"version":     "1.0.0",
			"description": "A slow tool for timeout testing",
		}
		json.NewEncoder(os.Stdout).Encode(schema)
		return
	}

	// Sleep for a long time
	time.Sleep(10 * time.Second)

	output := map[string]interface{}{"done": true}
	json.NewEncoder(os.Stdout).Encode(output)
}
`
	err := os.WriteFile(srcFile, []byte(src), 0644)
	require.NoError(t, err)

	cmd := exec.Command("go", "build", "-o", path, srcFile)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to compile slow tool: %s", string(output))
}
