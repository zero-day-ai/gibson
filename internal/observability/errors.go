package observability

import (
	"errors"
	"fmt"
)

// ObservabilityErrorCode represents error codes specific to observability operations.
type ObservabilityErrorCode string

// Observability error codes following the Gibson error pattern.
const (
	// ErrExporterConnection indicates failure to connect to an observability exporter.
	ErrExporterConnection ObservabilityErrorCode = "OBSERVABILITY_EXPORTER_CONNECTION"

	// ErrAuthenticationFailed indicates authentication failure with an observability backend.
	ErrAuthenticationFailed ObservabilityErrorCode = "OBSERVABILITY_AUTHENTICATION_FAILED"

	// ErrSpanContextMissing indicates a required span context is missing from the request.
	ErrSpanContextMissing ObservabilityErrorCode = "OBSERVABILITY_SPAN_CONTEXT_MISSING"

	// ErrMetricsRegistration indicates failure to register a metric with the metrics backend.
	ErrMetricsRegistration ObservabilityErrorCode = "OBSERVABILITY_METRICS_REGISTRATION"

	// ErrBufferOverflow indicates the observability buffer has overflowed.
	ErrBufferOverflow ObservabilityErrorCode = "OBSERVABILITY_BUFFER_OVERFLOW"

	// ErrShutdownTimeout indicates a timeout occurred during graceful shutdown.
	ErrShutdownTimeout ObservabilityErrorCode = "OBSERVABILITY_SHUTDOWN_TIMEOUT"
)

// ObservabilityError represents a structured error for observability operations.
// It follows the GibsonError pattern with code, message, retryability, and optional cause.
type ObservabilityError struct {
	Code      ObservabilityErrorCode
	Message   string
	Retryable bool
	Cause     error
}

// Error implements the error interface, returning a formatted error message.
// Format: "[CODE] message" or "[CODE] message: cause" if cause exists.
func (e *ObservabilityError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause error for error unwrapping chains.
// This enables using errors.Is() and errors.As() with wrapped errors.
func (e *ObservabilityError) Unwrap() error {
	return e.Cause
}

// Is checks if the target error matches this error by error code.
// Returns true if target is an ObservabilityError with the same Code.
func (e *ObservabilityError) Is(target error) bool {
	var obsErr *ObservabilityError
	if errors.As(target, &obsErr) {
		return e.Code == obsErr.Code
	}
	return false
}

// NewObservabilityError creates a new non-retryable ObservabilityError.
func NewObservabilityError(code ObservabilityErrorCode, message string) *ObservabilityError {
	return &ObservabilityError{
		Code:      code,
		Message:   message,
		Retryable: false,
		Cause:     nil,
	}
}

// NewRetryableObservabilityError creates a new retryable ObservabilityError.
func NewRetryableObservabilityError(code ObservabilityErrorCode, message string) *ObservabilityError {
	return &ObservabilityError{
		Code:      code,
		Message:   message,
		Retryable: true,
		Cause:     nil,
	}
}

// WrapObservabilityError creates a new ObservabilityError that wraps an existing error.
func WrapObservabilityError(code ObservabilityErrorCode, message string, cause error) *ObservabilityError {
	return &ObservabilityError{
		Code:      code,
		Message:   message,
		Retryable: false,
		Cause:     cause,
	}
}

// Helper constructors for common observability errors.

// NewExporterConnectionError creates an error for exporter connection failures.
// This error is retryable as network issues are often transient.
func NewExporterConnectionError(endpoint string, cause error) *ObservabilityError {
	return &ObservabilityError{
		Code:      ErrExporterConnection,
		Message:   fmt.Sprintf("failed to connect to exporter at %s", endpoint),
		Retryable: true,
		Cause:     cause,
	}
}

// NewAuthenticationError creates an error for authentication failures.
// This error is not retryable as credentials need to be corrected.
func NewAuthenticationError(service string, cause error) *ObservabilityError {
	return &ObservabilityError{
		Code:      ErrAuthenticationFailed,
		Message:   fmt.Sprintf("authentication failed for %s", service),
		Retryable: false,
		Cause:     cause,
	}
}

// NewSpanContextMissingError creates an error for missing span context.
// This error is not retryable as it indicates a programming error.
func NewSpanContextMissingError() *ObservabilityError {
	return &ObservabilityError{
		Code:      ErrSpanContextMissing,
		Message:   "span context is missing from request",
		Retryable: false,
		Cause:     nil,
	}
}

// NewMetricsRegistrationError creates an error for metric registration failures.
// This error is not retryable as it indicates a configuration or naming issue.
func NewMetricsRegistrationError(metricName string, cause error) *ObservabilityError {
	return &ObservabilityError{
		Code:      ErrMetricsRegistration,
		Message:   fmt.Sprintf("failed to register metric '%s'", metricName),
		Retryable: false,
		Cause:     cause,
	}
}

// NewBufferOverflowError creates an error for buffer overflow conditions.
// This error is retryable as it may resolve after buffer drains.
func NewBufferOverflowError(bufferType string) *ObservabilityError {
	return &ObservabilityError{
		Code:      ErrBufferOverflow,
		Message:   fmt.Sprintf("%s buffer overflow: too many events", bufferType),
		Retryable: true,
		Cause:     nil,
	}
}

// NewShutdownTimeoutError creates an error for shutdown timeout conditions.
// This error is not retryable as shutdown is a one-time operation.
func NewShutdownTimeoutError(component string) *ObservabilityError {
	return &ObservabilityError{
		Code:      ErrShutdownTimeout,
		Message:   fmt.Sprintf("%s shutdown timed out", component),
		Retryable: false,
		Cause:     nil,
	}
}
