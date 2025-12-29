package internal

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/internal/component"
)

func TestCLIError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CLIError
		expected string
	}{
		{
			name: "error without cause",
			err: &CLIError{
				Code:    ExitError,
				Message: "something went wrong",
			},
			expected: "something went wrong",
		},
		{
			name: "error with cause",
			err: &CLIError{
				Code:    ExitError,
				Message: "operation failed",
				Cause:   errors.New("underlying error"),
			},
			expected: "operation failed: underlying error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.err.Error())
			}
		})
	}
}

func TestCLIError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := &CLIError{
		Code:    ExitError,
		Message: "wrapper",
		Cause:   cause,
	}

	unwrapped := err.Unwrap()
	if unwrapped != cause {
		t.Errorf("expected unwrapped error to be %v, got %v", cause, unwrapped)
	}

	// Test error without cause
	errNoCause := &CLIError{
		Code:    ExitError,
		Message: "no cause",
	}
	if errNoCause.Unwrap() != nil {
		t.Error("expected Unwrap to return nil for error without cause")
	}
}

func TestWrapError(t *testing.T) {
	cause := errors.New("original error")
	wrapped := WrapError(ExitConfigError, "config failed", cause)

	if wrapped.Code != ExitConfigError {
		t.Errorf("expected code %d, got %d", ExitConfigError, wrapped.Code)
	}
	if wrapped.Message != "config failed" {
		t.Errorf("expected message %q, got %q", "config failed", wrapped.Message)
	}
	if wrapped.Cause != cause {
		t.Errorf("expected cause %v, got %v", cause, wrapped.Cause)
	}
}

func TestNewCLIError(t *testing.T) {
	err := NewCLIError(ExitTimeout, "operation timed out")

	if err.Code != ExitTimeout {
		t.Errorf("expected code %d, got %d", ExitTimeout, err.Code)
	}
	if err.Message != "operation timed out" {
		t.Errorf("expected message %q, got %q", "operation timed out", err.Message)
	}
	if err.Cause != nil {
		t.Errorf("expected no cause, got %v", err.Cause)
	}
}

func TestHandleError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedCode int
		checkOutput  func(t *testing.T, output string)
	}{
		{
			name:         "nil error",
			err:          nil,
			expectedCode: ExitSuccess,
			checkOutput:  func(t *testing.T, output string) {},
		},
		{
			name:         "context canceled",
			err:          context.Canceled,
			expectedCode: ExitCancelled,
			checkOutput: func(t *testing.T, output string) {
				if output != "Operation cancelled\n" {
					t.Errorf("expected cancellation message, got %q", output)
				}
			},
		},
		{
			name:         "context deadline exceeded",
			err:          context.DeadlineExceeded,
			expectedCode: ExitTimeout,
			checkOutput: func(t *testing.T, output string) {
				if output != "Operation timed out\n" {
					t.Errorf("expected timeout message, got %q", output)
				}
			},
		},
		{
			name: "CLI error",
			err: &CLIError{
				Code:    ExitConfigError,
				Message: "invalid config",
			},
			expectedCode: ExitConfigError,
			checkOutput: func(t *testing.T, output string) {
				if output != "Error: invalid config\n" {
					t.Errorf("expected error message, got %q", output)
				}
			},
		},
		{
			name: "CLI error with cause (verbose)",
			err: &CLIError{
				Code:    ExitConfigError,
				Message: "invalid config",
				Cause:   errors.New("file not found"),
			},
			expectedCode: ExitConfigError,
			checkOutput:  func(t *testing.T, output string) {},
		},
		{
			name:         "generic error",
			err:          errors.New("unknown error"),
			expectedCode: ExitError,
			checkOutput: func(t *testing.T, output string) {
				if output != "Error: unknown error\n" {
					t.Errorf("expected generic error message, got %q", output)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			cmd := &cobra.Command{}
			cmd.SetErr(buf)

			exitCode := HandleError(cmd, tt.err)
			if exitCode != tt.expectedCode {
				t.Errorf("expected exit code %d, got %d", tt.expectedCode, exitCode)
			}

			tt.checkOutput(t, buf.String())
		})
	}
}

func TestHandleError_ComponentError(t *testing.T) {
	tests := []struct {
		name         string
		err          *component.ComponentError
		expectedCode int
		checkOutput  func(t *testing.T, output string)
	}{
		{
			name: "component not found",
			err: component.NewComponentNotFoundError("test-component"),
			expectedCode: ExitComponentError,
			checkOutput: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("component not found")) {
					t.Error("expected component not found message")
				}
			},
		},
		{
			name: "component timeout",
			err: component.NewTimeoutError("test-component", "start"),
			expectedCode: ExitTimeout,
			checkOutput: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("timeout")) {
					t.Error("expected timeout message")
				}
			},
		},
		{
			name: "component already running",
			err: component.NewAlreadyRunningError("test-component", 12345),
			expectedCode: ExitComponentError,
			checkOutput: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("already running")) {
					t.Error("expected already running message")
				}
			},
		},
		{
			name: "component not running",
			err: component.NewNotRunningError("test-component"),
			expectedCode: ExitComponentError,
			checkOutput: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("not running")) {
					t.Error("expected not running message")
				}
			},
		},
		{
			name: "invalid manifest",
			err: component.NewInvalidManifestError("bad format", errors.New("parse error")),
			expectedCode: ExitComponentError,
			checkOutput: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("INVALID_MANIFEST")) {
					t.Error("expected invalid manifest error code")
				}
			},
		},
		{
			name: "permission denied",
			err: component.NewPermissionDeniedError("test-component", "start"),
			expectedCode: ExitComponentError,
			checkOutput: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("permission denied")) {
					t.Error("expected permission denied message")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			cmd := &cobra.Command{}
			cmd.SetErr(buf)

			exitCode := HandleError(cmd, tt.err)
			if exitCode != tt.expectedCode {
				t.Errorf("expected exit code %d, got %d", tt.expectedCode, exitCode)
			}

			tt.checkOutput(t, buf.String())
		})
	}
}

func TestMapComponentErrorToExitCode(t *testing.T) {
	tests := []struct {
		name         string
		errorCode    component.ComponentErrorCode
		expectedExit int
	}{
		{
			name:         "timeout error",
			errorCode:    component.ErrCodeTimeout,
			expectedExit: ExitTimeout,
		},
		{
			name:         "component not found",
			errorCode:    component.ErrCodeComponentNotFound,
			expectedExit: ExitComponentError,
		},
		{
			name:         "component exists",
			errorCode:    component.ErrCodeComponentExists,
			expectedExit: ExitComponentError,
		},
		{
			name:         "invalid manifest",
			errorCode:    component.ErrCodeInvalidManifest,
			expectedExit: ExitComponentError,
		},
		{
			name:         "load failed",
			errorCode:    component.ErrCodeLoadFailed,
			expectedExit: ExitComponentError,
		},
		{
			name:         "start failed",
			errorCode:    component.ErrCodeStartFailed,
			expectedExit: ExitComponentError,
		},
		{
			name:         "stop failed",
			errorCode:    component.ErrCodeStopFailed,
			expectedExit: ExitComponentError,
		},
		{
			name:         "validation failed",
			errorCode:    component.ErrCodeValidationFailed,
			expectedExit: ExitComponentError,
		},
		{
			name:         "connection failed",
			errorCode:    component.ErrCodeConnectionFailed,
			expectedExit: ExitComponentError,
		},
		{
			name:         "execution failed",
			errorCode:    component.ErrCodeExecutionFailed,
			expectedExit: ExitComponentError,
		},
		{
			name:         "invalid kind",
			errorCode:    component.ErrCodeInvalidKind,
			expectedExit: ExitComponentError,
		},
		{
			name:         "already running",
			errorCode:    component.ErrCodeAlreadyRunning,
			expectedExit: ExitComponentError,
		},
		{
			name:         "not running",
			errorCode:    component.ErrCodeNotRunning,
			expectedExit: ExitComponentError,
		},
		{
			name:         "permission denied",
			errorCode:    component.ErrCodePermissionDenied,
			expectedExit: ExitComponentError,
		},
		{
			name:         "unsupported operation",
			errorCode:    component.ErrCodeUnsupportedOperation,
			expectedExit: ExitComponentError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &component.ComponentError{
				Code:    tt.errorCode,
				Message: "test error",
			}

			exitCode := mapComponentErrorToExitCode(err)
			if exitCode != tt.expectedExit {
				t.Errorf("expected exit code %d for %s, got %d",
					tt.expectedExit, tt.errorCode, exitCode)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name: "retryable component error",
			err: component.NewRetryableComponentError(
				component.ErrCodeConnectionFailed,
				"connection failed",
			),
			expected: true,
		},
		{
			name: "non-retryable component error",
			err: component.NewComponentError(
				component.ErrCodeComponentNotFound,
				"component not found",
			),
			expected: false,
		},
		{
			name:     "generic error",
			err:      errors.New("generic error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("expected IsRetryable=%v, got %v", tt.expected, result)
			}
		})
	}
}

func TestExitCodeConstants(t *testing.T) {
	// Verify exit code values are as expected
	tests := []struct {
		name     string
		code     int
		expected int
	}{
		{"ExitSuccess", ExitSuccess, 0},
		{"ExitError", ExitError, 1},
		{"ExitCriticalFindings", ExitCriticalFindings, 2},
		{"ExitTimeout", ExitTimeout, 3},
		{"ExitCancelled", ExitCancelled, 4},
		{"ExitConfigError", ExitConfigError, 10},
		{"ExitComponentError", ExitComponentError, 11},
		{"ExitDatabaseError", ExitDatabaseError, 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != tt.expected {
				t.Errorf("expected %s=%d, got %d", tt.name, tt.expected, tt.code)
			}
		})
	}
}
