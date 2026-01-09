package toolexec

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
)

// createTestTool creates a test tool binary that behaves according to the specified behavior.
// The tool reads JSON from stdin and writes JSON to stdout.
func createTestTool(t *testing.T, behavior string) string {
	t.Helper()

	// Create temporary directory for test binaries
	tmpDir := t.TempDir()
	toolPath := filepath.Join(tmpDir, "test-tool")

	// Go program that implements tool behaviors
	toolSource := `package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func main() {
	behavior := os.Getenv("TEST_BEHAVIOR")

	switch behavior {
	case "success":
		// Read input from stdin
		var input map[string]interface{}
		if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
			fmt.Fprintf(os.Stderr, "failed to decode input: %v", err)
			os.Exit(1)
		}

		// Write output to stdout
		output := map[string]interface{}{
			"status": "success",
			"input": input,
		}
		if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode output: %v", err)
			os.Exit(1)
		}
		os.Exit(0)

	case "timeout":
		// Sleep longer than timeout to trigger timeout
		time.Sleep(10 * time.Second)
		os.Exit(0)

	case "exit-error":
		// Exit with non-zero code
		fmt.Fprintf(os.Stderr, "tool encountered an error")
		os.Exit(42)

	case "invalid-json":
		// Write invalid JSON to stdout
		fmt.Println("this is not valid JSON")
		os.Exit(0)

	case "empty-output":
		// Don't write anything to stdout
		os.Exit(0)

	case "stderr-only":
		// Write to stderr but exit successfully
		fmt.Fprintf(os.Stderr, "warning: something happened")
		output := map[string]interface{}{"status": "ok"}
		json.NewEncoder(os.Stdout).Encode(output)
		os.Exit(0)

	default:
		fmt.Fprintf(os.Stderr, "unknown behavior: %s", behavior)
		os.Exit(1)
	}
}
`

	// Write source file
	sourceFile := filepath.Join(tmpDir, "main.go")
	err := os.WriteFile(sourceFile, []byte(toolSource), 0644)
	require.NoError(t, err, "failed to write test tool source")

	// Compile the test tool
	cmd := exec.Command("go", "build", "-o", toolPath, sourceFile)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to compile test tool: %s", string(output))

	// Set the behavior for this test tool
	t.Setenv("TEST_BEHAVIOR", behavior)

	return toolPath
}

func TestSubprocessExecutor_Execute_Success(t *testing.T) {
	toolPath := createTestTool(t, "success")
	executor := NewSubprocessExecutor()

	req := &ExecuteRequest{
		BinaryPath: toolPath,
		Input: map[string]any{
			"message": "hello",
			"value":   42,
		},
		Timeout: 5 * time.Second,
		Env:     []string{"TEST_BEHAVIOR=success"},
	}

	result, err := executor.Execute(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.NotNil(t, result.Output)
	assert.Equal(t, "success", result.Output["status"])
	assert.Greater(t, result.Duration, time.Duration(0))
	assert.LessOrEqual(t, result.Duration, 5*time.Second)

	// Verify input was passed correctly
	inputMap, ok := result.Output["input"].(map[string]any)
	require.True(t, ok, "input should be a map")
	assert.Equal(t, "hello", inputMap["message"])
	assert.Equal(t, float64(42), inputMap["value"]) // JSON numbers are float64
}

func TestSubprocessExecutor_Execute_Timeout(t *testing.T) {
	toolPath := createTestTool(t, "timeout")
	executor := NewSubprocessExecutor()

	req := &ExecuteRequest{
		BinaryPath: toolPath,
		Input:      map[string]any{"test": "data"},
		Timeout:    100 * time.Millisecond, // Very short timeout
		Env:        []string{"TEST_BEHAVIOR=timeout"},
	}

	result, err := executor.Execute(context.Background(), req)

	require.Error(t, err)
	assert.Equal(t, -1, result.ExitCode)

	// Check error type
	var gibsonErr *types.GibsonError
	require.ErrorAs(t, err, &gibsonErr)
	assert.Equal(t, ErrToolTimeout, gibsonErr.Code)

	// Duration should be close to timeout
	assert.GreaterOrEqual(t, result.Duration, 100*time.Millisecond)
	assert.LessOrEqual(t, result.Duration, 200*time.Millisecond)
}

func TestSubprocessExecutor_Execute_NonZeroExit(t *testing.T) {
	toolPath := createTestTool(t, "exit-error")
	executor := NewSubprocessExecutor()

	req := &ExecuteRequest{
		BinaryPath: toolPath,
		Input:      map[string]any{"test": "data"},
		Timeout:    5 * time.Second,
		Env:        []string{"TEST_BEHAVIOR=exit-error"},
	}

	result, err := executor.Execute(context.Background(), req)

	require.Error(t, err)
	assert.Equal(t, 42, result.ExitCode)
	assert.Contains(t, result.Stderr, "tool encountered an error")

	// Check error type
	var gibsonErr *types.GibsonError
	require.ErrorAs(t, err, &gibsonErr)
	assert.Equal(t, ErrToolExecutionFailed, gibsonErr.Code)
}

func TestSubprocessExecutor_Execute_InvalidJSON(t *testing.T) {
	toolPath := createTestTool(t, "invalid-json")
	executor := NewSubprocessExecutor()

	req := &ExecuteRequest{
		BinaryPath: toolPath,
		Input:      map[string]any{"test": "data"},
		Timeout:    5 * time.Second,
		Env:        []string{"TEST_BEHAVIOR=invalid-json"},
	}

	result, err := executor.Execute(context.Background(), req)

	require.Error(t, err)
	assert.Equal(t, 0, result.ExitCode) // Process exited successfully

	// Check error type
	var gibsonErr *types.GibsonError
	require.ErrorAs(t, err, &gibsonErr)
	assert.Equal(t, ErrInvalidToolOutput, gibsonErr.Code)
}

func TestSubprocessExecutor_Execute_EmptyOutput(t *testing.T) {
	toolPath := createTestTool(t, "empty-output")
	executor := NewSubprocessExecutor()

	req := &ExecuteRequest{
		BinaryPath: toolPath,
		Input:      map[string]any{"test": "data"},
		Timeout:    5 * time.Second,
		Env:        []string{"TEST_BEHAVIOR=empty-output"},
	}

	result, err := executor.Execute(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.NotNil(t, result.Output)
	assert.Empty(t, result.Output) // Empty output should result in empty map
}

func TestSubprocessExecutor_Execute_StderrCapture(t *testing.T) {
	toolPath := createTestTool(t, "stderr-only")
	executor := NewSubprocessExecutor()

	req := &ExecuteRequest{
		BinaryPath: toolPath,
		Input:      map[string]any{"test": "data"},
		Timeout:    5 * time.Second,
		Env:        []string{"TEST_BEHAVIOR=stderr-only"},
	}

	result, err := executor.Execute(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.NotNil(t, result.Output)
	assert.Contains(t, result.Stderr, "warning: something happened")
	assert.Equal(t, "ok", result.Output["status"])
}

func TestSubprocessExecutor_Execute_BinaryNotFound(t *testing.T) {
	executor := NewSubprocessExecutor()

	req := &ExecuteRequest{
		BinaryPath: "/nonexistent/path/to/tool",
		Input:      map[string]any{"test": "data"},
		Timeout:    5 * time.Second,
		Env:        []string{},
	}

	result, err := executor.Execute(context.Background(), req)

	require.Error(t, err)

	// Check error type
	var gibsonErr *types.GibsonError
	require.ErrorAs(t, err, &gibsonErr)
	assert.Equal(t, ErrToolSpawnFailed, gibsonErr.Code)
	assert.NotNil(t, result) // Result should still be returned
}

func TestSubprocessExecutor_Execute_ContextCancellation(t *testing.T) {
	toolPath := createTestTool(t, "timeout")
	executor := NewSubprocessExecutor()

	ctx, cancel := context.WithCancel(context.Background())

	req := &ExecuteRequest{
		BinaryPath: toolPath,
		Input:      map[string]any{"test": "data"},
		Timeout:    10 * time.Second,
		Env:        []string{"TEST_BEHAVIOR=timeout"},
	}

	// Cancel context after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	result, err := executor.Execute(ctx, req)

	require.Error(t, err)
	assert.Equal(t, -1, result.ExitCode)

	// Check error type
	var gibsonErr *types.GibsonError
	require.ErrorAs(t, err, &gibsonErr)
	// Could be timeout or execution failed depending on timing
	assert.Contains(t, []types.ErrorCode{ErrToolTimeout, ErrToolExecutionFailed}, gibsonErr.Code)
}

func TestSubprocessExecutor_Execute_EnvironmentVariables(t *testing.T) {
	toolPath := createTestTool(t, "success")
	executor := NewSubprocessExecutor()

	customEnv := []string{
		"TEST_BEHAVIOR=success",
		"CUSTOM_VAR=custom_value",
		"ANOTHER_VAR=another_value",
	}

	req := &ExecuteRequest{
		BinaryPath: toolPath,
		Input:      map[string]any{"test": "data"},
		Timeout:    5 * time.Second,
		Env:        customEnv,
	}

	result, err := executor.Execute(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)

	// The GIBSON_TOOL_MODE env var should be set automatically
	// We can't directly verify this from the result, but the tool execution succeeding
	// with the TEST_BEHAVIOR env var proves environment passing works
}

func TestSubprocessExecutor_Execute_ComplexJSON(t *testing.T) {
	toolPath := createTestTool(t, "success")
	executor := NewSubprocessExecutor()

	complexInput := map[string]any{
		"string":  "hello world",
		"number":  123.456,
		"boolean": true,
		"null":    nil,
		"array":   []any{1, 2, 3, "four"},
		"object": map[string]any{
			"nested": "value",
			"count":  42,
		},
	}

	req := &ExecuteRequest{
		BinaryPath: toolPath,
		Input:      complexInput,
		Timeout:    5 * time.Second,
		Env:        []string{"TEST_BEHAVIOR=success"},
	}

	result, err := executor.Execute(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)

	// Verify complex input was round-tripped correctly
	inputMap, ok := result.Output["input"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "hello world", inputMap["string"])
	assert.Equal(t, 123.456, inputMap["number"])
	assert.Equal(t, true, inputMap["boolean"])
	assert.Nil(t, inputMap["null"])
}

func TestSubprocessExecutor_Execute_LargeOutput(t *testing.T) {
	// Create a test tool that outputs a large JSON response
	tmpDir := t.TempDir()
	toolPath := filepath.Join(tmpDir, "large-output-tool")

	toolSource := `package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	// Read input
	var input map[string]interface{}
	json.NewDecoder(os.Stdin).Decode(&input)

	// Generate large output
	output := map[string]interface{}{
		"status": "success",
	}

	// Add large array
	largeArray := make([]int, 10000)
	for i := range largeArray {
		largeArray[i] = i
	}
	output["data"] = largeArray

	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "encode error: %v", err)
		os.Exit(1)
	}
}
`

	sourceFile := filepath.Join(tmpDir, "main.go")
	err := os.WriteFile(sourceFile, []byte(toolSource), 0644)
	require.NoError(t, err)

	cmd := exec.Command("go", "build", "-o", toolPath, sourceFile)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to compile large output tool: %s", string(output))

	executor := NewSubprocessExecutor()

	req := &ExecuteRequest{
		BinaryPath: toolPath,
		Input:      map[string]any{"test": "data"},
		Timeout:    10 * time.Second,
		Env:        []string{},
	}

	result, err := executor.Execute(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.NotNil(t, result.Output)

	// Verify large data was received
	dataArray, ok := result.Output["data"].([]any)
	require.True(t, ok)
	assert.Len(t, dataArray, 10000)
}

func TestNewSubprocessExecutor(t *testing.T) {
	executor := NewSubprocessExecutor()
	assert.NotNil(t, executor)
}

// BenchmarkSubprocessExecutor_Execute benchmarks the executor performance
func BenchmarkSubprocessExecutor_Execute(b *testing.B) {
	// Create a simple success tool once
	tmpDir := b.TempDir()
	toolPath := filepath.Join(tmpDir, "bench-tool")

	toolSource := `package main
import (
	"encoding/json"
	"os"
)

func main() {
	var input map[string]interface{}
	json.NewDecoder(os.Stdin).Decode(&input)
	output := map[string]interface{}{"status": "ok"}
	json.NewEncoder(os.Stdout).Encode(output)
}
`

	sourceFile := filepath.Join(tmpDir, "main.go")
	os.WriteFile(sourceFile, []byte(toolSource), 0644)

	cmd := exec.Command("go", "build", "-o", toolPath, sourceFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		b.Fatalf("failed to compile bench tool: %s", string(output))
	}

	executor := NewSubprocessExecutor()
	req := &ExecuteRequest{
		BinaryPath: toolPath,
		Input:      map[string]any{"test": "data"},
		Timeout:    5 * time.Second,
		Env:        []string{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := executor.Execute(context.Background(), req)
		if err != nil {
			b.Fatalf("execution failed: %v", err)
		}
	}
}
