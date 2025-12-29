package internal

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/internal/component"
)

// Exit code constants for the CLI
const (
	// ExitSuccess indicates successful execution
	ExitSuccess = 0
	// ExitError indicates a general error
	ExitError = 1
	// ExitCriticalFindings indicates critical security findings were discovered
	ExitCriticalFindings = 2
	// ExitTimeout indicates the operation timed out
	ExitTimeout = 3
	// ExitCancelled indicates the operation was cancelled
	ExitCancelled = 4
	// ExitConfigError indicates a configuration error
	ExitConfigError = 10
	// ExitComponentError indicates a component error
	ExitComponentError = 11
	// ExitDatabaseError indicates a database error
	ExitDatabaseError = 12
)

// CLIError represents a CLI-specific error with an exit code
type CLIError struct {
	Code    int
	Message string
	Cause   error
}

// Error implements the error interface
func (e *CLIError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap returns the underlying cause error
func (e *CLIError) Unwrap() error {
	return e.Cause
}

// WrapError creates a new CLIError wrapping an existing error
func WrapError(code int, message string, err error) *CLIError {
	return &CLIError{
		Code:    code,
		Message: message,
		Cause:   err,
	}
}

// NewCLIError creates a new CLIError with the given code and message
func NewCLIError(code int, message string) *CLIError {
	return &CLIError{
		Code:    code,
		Message: message,
	}
}

// HandleError handles an error and returns the appropriate exit code
// It also prints the error message to the command's error output
func HandleError(cmd *cobra.Command, err error) int {
	if err == nil {
		return ExitSuccess
	}

	// Check for context cancellation
	if errors.Is(err, context.Canceled) {
		cmd.PrintErrln("Operation cancelled")
		return ExitCancelled
	}

	// Check for context deadline exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		cmd.PrintErrln("Operation timed out")
		return ExitTimeout
	}

	// Check for CLIError
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		cmd.PrintErrln("Error:", cliErr.Message)
		if cliErr.Cause != nil {
			verboseFlag := cmd.Flag("verbose")
			if verboseFlag != nil && verboseFlag.Changed {
				cmd.PrintErrln("Cause:", cliErr.Cause)
			}
		}
		return cliErr.Code
	}

	// Check for ComponentError
	var compErr *component.ComponentError
	if errors.As(err, &compErr) {
		exitCode := mapComponentErrorToExitCode(compErr)
		cmd.PrintErrln("Error:", compErr.Error())

		// Print additional context in verbose mode
		verboseFlag := cmd.Flag("verbose")
		if verboseFlag != nil && verboseFlag.Changed && len(compErr.Context) > 0 {
			cmd.PrintErrln("Context:")
			for k, v := range compErr.Context {
				cmd.PrintErrf("  %s: %v\n", k, v)
			}
		}

		return exitCode
	}

	// Generic error
	cmd.PrintErrln("Error:", err)
	return ExitError
}

// mapComponentErrorToExitCode maps ComponentError codes to CLI exit codes
func mapComponentErrorToExitCode(err *component.ComponentError) int {
	switch err.Code {
	case component.ErrCodeTimeout:
		return ExitTimeout
	case component.ErrCodeComponentNotFound,
		component.ErrCodeComponentExists,
		component.ErrCodeInvalidManifest,
		component.ErrCodeManifestNotFound,
		component.ErrCodeLoadFailed,
		component.ErrCodeStartFailed,
		component.ErrCodeStopFailed,
		component.ErrCodeValidationFailed,
		component.ErrCodeConnectionFailed,
		component.ErrCodeExecutionFailed,
		component.ErrCodeInvalidKind,
		component.ErrCodeInvalidSource,
		component.ErrCodeInvalidStatus,
		component.ErrCodeInvalidPath,
		component.ErrCodeInvalidPort,
		component.ErrCodeInvalidVersion,
		component.ErrCodeDependencyFailed,
		component.ErrCodeIncompatibleVersion,
		component.ErrCodeAlreadyRunning,
		component.ErrCodeNotRunning,
		component.ErrCodePermissionDenied,
		component.ErrCodeUnsupportedOperation:
		return ExitComponentError
	default:
		return ExitError
	}
}

// IsRetryable checks if an error is retryable
func IsRetryable(err error) bool {
	var compErr *component.ComponentError
	if errors.As(err, &compErr) {
		return compErr.Retryable
	}
	return false
}

// IsVerbose checks if verbose mode is enabled via environment variable or flag
// This is used for panic recovery to determine if stack traces should be shown
func IsVerbose() bool {
	// Check environment variable
	if os.Getenv("GIBSON_VERBOSE") != "" {
		return true
	}

	// Check common verbose flag patterns
	for _, arg := range os.Args {
		if arg == "-v" || arg == "--verbose" {
			return true
		}
	}

	return false
}
