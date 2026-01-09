package toolexec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// SubprocessExecutor executes tool binaries as short-lived subprocesses,
// communicating via JSON over stdin/stdout.
type SubprocessExecutor struct{}

// NewSubprocessExecutor creates a new subprocess executor instance.
func NewSubprocessExecutor() *SubprocessExecutor {
	return &SubprocessExecutor{}
}

// Execute runs a tool binary as a subprocess with JSON input/output over stdin/stdout.
//
// The tool binary is invoked with the following characteristics:
//   - JSON input is written to stdin
//   - JSON output is read from stdout
//   - Error messages are captured from stderr
//   - Environment variable GIBSON_TOOL_MODE=subprocess is set
//   - Execution is bounded by the provided timeout via context
//   - If timeout is exceeded, the subprocess is killed with SIGKILL
//
// Returns ExecuteResult with output, duration, exit code, and stderr on success.
// Returns appropriate error on failure:
//   - ErrToolSpawnFailed: subprocess failed to start
//   - ErrToolTimeout: subprocess exceeded timeout
//   - ErrToolExecutionFailed: subprocess exited with non-zero code
//   - ErrInvalidToolOutput: subprocess output is not valid JSON
func (e *SubprocessExecutor) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResult, error) {
	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	// Create command with context for automatic timeout handling
	cmd := exec.CommandContext(execCtx, req.BinaryPath)

	// Set up environment variables
	cmd.Env = append(req.Env, "GIBSON_TOOL_MODE=subprocess")

	// Marshal input to JSON
	inputJSON, err := json.Marshal(req.Input)
	if err != nil {
		return nil, types.WrapError(
			ErrToolExecutionFailed,
			"failed to marshal tool input to JSON",
			err,
		)
	}

	// Set up stdin with JSON input
	cmd.Stdin = bytes.NewReader(inputJSON)

	// Create buffers for stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Record start time
	startTime := time.Now()

	// Run the command
	execErr := cmd.Run()
	duration := time.Since(startTime)

	// Initialize result
	result := &ExecuteResult{
		Duration: duration,
		Stderr:   stderr.String(),
	}

	// Check for timeout
	if execCtx.Err() == context.DeadlineExceeded {
		// Ensure process is killed with SIGKILL
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGKILL)
		}
		result.ExitCode = -1
		return result, types.WrapError(
			ErrToolTimeout,
			fmt.Sprintf("tool execution exceeded timeout of %v", req.Timeout),
			execCtx.Err(),
		)
	}

	// Check for other context errors (e.g., parent context cancelled)
	if execCtx.Err() != nil {
		result.ExitCode = -1
		return result, types.WrapError(
			ErrToolExecutionFailed,
			"tool execution cancelled",
			execCtx.Err(),
		)
	}

	// Get exit code
	if execErr != nil {
		// Extract exit code from error
		if exitErr, ok := execErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			// Failed to spawn the process
			return result, types.WrapError(
				ErrToolSpawnFailed,
				fmt.Sprintf("failed to spawn tool process: %s", req.BinaryPath),
				execErr,
			)
		}

		// Non-zero exit code
		return result, types.WrapError(
			ErrToolExecutionFailed,
			fmt.Sprintf("tool exited with code %d", result.ExitCode),
			execErr,
		)
	}

	// Success case - exit code 0
	result.ExitCode = 0

	// Parse JSON output from stdout
	if stdout.Len() > 0 {
		var output map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
			return result, types.WrapError(
				ErrInvalidToolOutput,
				"tool output is not valid JSON",
				err,
			)
		}
		result.Output = output
	} else {
		// Empty output is valid - return empty map
		result.Output = make(map[string]any)
	}

	return result, nil
}
