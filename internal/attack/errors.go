package attack

import (
	"errors"
	"fmt"
)

// AttackErrorCode represents specific attack error types.
type AttackErrorCode string

const (
	// ErrAgentRequired indicates the agent flag was not specified.
	ErrAgentRequired AttackErrorCode = "agent_required"

	// ErrAgentNotFound indicates the specified agent was not found in the registry.
	ErrAgentNotFound AttackErrorCode = "agent_not_found"

	// ErrInvalidTarget indicates the target URL or configuration is invalid.
	ErrInvalidTarget AttackErrorCode = "invalid_target"

	// ErrTargetNotFound indicates the named target was not found.
	ErrTargetNotFound AttackErrorCode = "target_not_found"

	// ErrPayloadNotFound indicates the specified payload was not found.
	ErrPayloadNotFound AttackErrorCode = "payload_not_found"

	// ErrExecutionFailed indicates the attack execution failed.
	ErrExecutionFailed AttackErrorCode = "execution_failed"

	// ErrTimeout indicates the attack exceeded the timeout duration.
	ErrTimeout AttackErrorCode = "timeout"

	// ErrCancelled indicates the attack was cancelled by the user.
	ErrCancelled AttackErrorCode = "cancelled"

	// ErrValidation indicates attack options validation failed.
	ErrValidation AttackErrorCode = "validation_failed"

	// ErrPersistence indicates an error occurred during result persistence.
	ErrPersistence AttackErrorCode = "persistence_error"

	// ErrInternal indicates an internal attack error.
	ErrInternal AttackErrorCode = "internal_error"
)

// AttackError represents an attack-specific error with code and context.
// It implements the error interface and supports error wrapping with errors.Is/As.
type AttackError struct {
	// Code identifies the specific error type.
	Code AttackErrorCode

	// Message is a human-readable error message.
	Message string

	// Cause is the underlying error that caused this error (optional).
	Cause error

	// Context provides additional contextual information about the error.
	Context map[string]any
}

// Error implements the error interface.
func (e *AttackError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap implements the errors.Unwrap interface for error chain traversal.
// This enables errors.Is and errors.As to work with wrapped errors.
func (e *AttackError) Unwrap() error {
	return e.Cause
}

// Is implements the errors.Is interface for error comparison.
// Two AttackErrors are equal if they have the same error code.
func (e *AttackError) Is(target error) bool {
	var attackErr *AttackError
	if errors.As(target, &attackErr) {
		return e.Code == attackErr.Code
	}
	return false
}

// WithContext adds contextual information to the error.
func (e *AttackError) WithContext(key string, value any) *AttackError {
	if e.Context == nil {
		e.Context = make(map[string]any)
	}
	e.Context[key] = value
	return e
}

// NewAttackError creates a new AttackError with the given code and message.
func NewAttackError(code AttackErrorCode, message string) *AttackError {
	return &AttackError{
		Code:    code,
		Message: message,
		Context: make(map[string]any),
	}
}

// WrapAttackError wraps an existing error with attack error context.
func WrapAttackError(code AttackErrorCode, message string, cause error) *AttackError {
	return &AttackError{
		Code:    code,
		Message: message,
		Cause:   cause,
		Context: make(map[string]any),
	}
}

// Helper functions for common attack errors

// NewAgentRequiredError creates an error indicating the agent flag was not specified.
func NewAgentRequiredError(availableAgents []string) *AttackError {
	return NewAttackError(
		ErrAgentRequired,
		"agent name is required (use --agent flag)",
	).WithContext("available_agents", availableAgents)
}

// NewAgentNotFoundError creates an error indicating the agent was not found.
func NewAgentNotFoundError(agentName string, availableAgents []string) *AttackError {
	return NewAttackError(
		ErrAgentNotFound,
		fmt.Sprintf("agent not found: %s", agentName),
	).WithContext("agent_name", agentName).
		WithContext("available_agents", availableAgents)
}

// NewInvalidTargetError creates an error indicating the target is invalid.
func NewInvalidTargetError(target string, reason string) *AttackError {
	return NewAttackError(
		ErrInvalidTarget,
		fmt.Sprintf("invalid target '%s': %s", target, reason),
	).WithContext("target", target).
		WithContext("reason", reason)
}

// NewTargetNotFoundError creates an error indicating the named target was not found.
func NewTargetNotFoundError(targetName string) *AttackError {
	return NewAttackError(
		ErrTargetNotFound,
		fmt.Sprintf("target not found: %s", targetName),
	).WithContext("target_name", targetName)
}

// NewPayloadNotFoundError creates an error indicating the payload was not found.
func NewPayloadNotFoundError(payloadID string) *AttackError {
	return NewAttackError(
		ErrPayloadNotFound,
		fmt.Sprintf("payload not found: %s", payloadID),
	).WithContext("payload_id", payloadID)
}

// NewExecutionFailedError creates an error indicating attack execution failed.
func NewExecutionFailedError(reason string, cause error) *AttackError {
	return WrapAttackError(
		ErrExecutionFailed,
		fmt.Sprintf("attack execution failed: %s", reason),
		cause,
	).WithContext("reason", reason)
}

// NewTimeoutError creates an error indicating the attack timed out.
func NewTimeoutError(duration string) *AttackError {
	return NewAttackError(
		ErrTimeout,
		fmt.Sprintf("attack exceeded timeout duration: %s", duration),
	).WithContext("timeout", duration)
}

// NewCancelledError creates an error indicating the attack was cancelled.
func NewCancelledError(reason string) *AttackError {
	return NewAttackError(
		ErrCancelled,
		fmt.Sprintf("attack cancelled: %s", reason),
	).WithContext("reason", reason)
}

// NewValidationError creates an error indicating validation failed.
func NewValidationError(message string) *AttackError {
	return NewAttackError(ErrValidation, message)
}

// NewPersistenceError creates an error indicating persistence failed.
func NewPersistenceError(operation string, cause error) *AttackError {
	return WrapAttackError(
		ErrPersistence,
		fmt.Sprintf("persistence %s failed", operation),
		cause,
	).WithContext("operation", operation)
}

// NewInternalError creates an internal attack error.
func NewInternalError(message string, cause error) *AttackError {
	return WrapAttackError(ErrInternal, message, cause)
}

// IsAgentRequiredError checks if an error is an agent required error.
func IsAgentRequiredError(err error) bool {
	var attackErr *AttackError
	if errors.As(err, &attackErr) {
		return attackErr.Code == ErrAgentRequired
	}
	return false
}

// IsAgentNotFoundError checks if an error is an agent not found error.
func IsAgentNotFoundError(err error) bool {
	var attackErr *AttackError
	if errors.As(err, &attackErr) {
		return attackErr.Code == ErrAgentNotFound
	}
	return false
}

// IsInvalidTargetError checks if an error is an invalid target error.
func IsInvalidTargetError(err error) bool {
	var attackErr *AttackError
	if errors.As(err, &attackErr) {
		return attackErr.Code == ErrInvalidTarget
	}
	return false
}

// IsTargetNotFoundError checks if an error is a target not found error.
func IsTargetNotFoundError(err error) bool {
	var attackErr *AttackError
	if errors.As(err, &attackErr) {
		return attackErr.Code == ErrTargetNotFound
	}
	return false
}

// IsPayloadNotFoundError checks if an error is a payload not found error.
func IsPayloadNotFoundError(err error) bool {
	var attackErr *AttackError
	if errors.As(err, &attackErr) {
		return attackErr.Code == ErrPayloadNotFound
	}
	return false
}

// IsExecutionFailedError checks if an error is an execution failed error.
func IsExecutionFailedError(err error) bool {
	var attackErr *AttackError
	if errors.As(err, &attackErr) {
		return attackErr.Code == ErrExecutionFailed
	}
	return false
}

// IsTimeoutError checks if an error is a timeout error.
func IsTimeoutError(err error) bool {
	var attackErr *AttackError
	if errors.As(err, &attackErr) {
		return attackErr.Code == ErrTimeout
	}
	return false
}

// IsCancelledError checks if an error is a cancelled error.
func IsCancelledError(err error) bool {
	var attackErr *AttackError
	if errors.As(err, &attackErr) {
		return attackErr.Code == ErrCancelled
	}
	return false
}

// IsValidationError checks if an error is a validation error.
func IsValidationError(err error) bool {
	var attackErr *AttackError
	if errors.As(err, &attackErr) {
		return attackErr.Code == ErrValidation
	}
	return false
}

// IsPersistenceError checks if an error is a persistence error.
func IsPersistenceError(err error) bool {
	var attackErr *AttackError
	if errors.As(err, &attackErr) {
		return attackErr.Code == ErrPersistence
	}
	return false
}
