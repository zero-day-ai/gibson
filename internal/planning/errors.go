package planning

import (
	"errors"
	"fmt"
)

// ErrorType represents specific planning error types.
type ErrorType string

const (
	// ErrorTypeValidation indicates a planning validation error.
	ErrorTypeValidation ErrorType = "validation_failed"

	// ErrorTypeBudgetExhausted indicates a planning budget was exhausted.
	ErrorTypeBudgetExhausted ErrorType = "budget_exhausted"

	// ErrorTypeConstraintViolation indicates a planning constraint was violated.
	ErrorTypeConstraintViolation ErrorType = "constraint_violated"

	// ErrorTypeInternal indicates an internal planning error.
	ErrorTypeInternal ErrorType = "internal_error"

	// ErrorTypeInvalidParameter indicates an invalid parameter was provided.
	ErrorTypeInvalidParameter ErrorType = "invalid_parameter"
)

// PlanningError represents a planning-specific error with type and context.
// It implements the error interface and supports error wrapping with errors.Is/As.
type PlanningError struct {
	// Type identifies the specific error type.
	Type ErrorType

	// Message is a human-readable error message.
	Message string

	// Cause is the underlying error that caused this error (optional).
	Cause error

	// Context provides additional contextual information about the error.
	Context map[string]any
}

// Error implements the error interface.
func (e *PlanningError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

// Unwrap implements the errors.Unwrap interface for error chain traversal.
// This enables errors.Is and errors.As to work with wrapped errors.
func (e *PlanningError) Unwrap() error {
	return e.Cause
}

// Is implements the errors.Is interface for error comparison.
// Two PlanningErrors are equal if they have the same error type.
func (e *PlanningError) Is(target error) bool {
	var planningErr *PlanningError
	if errors.As(target, &planningErr) {
		return e.Type == planningErr.Type
	}
	return false
}

// WithContext adds contextual information to the error.
func (e *PlanningError) WithContext(key string, value any) *PlanningError {
	if e.Context == nil {
		e.Context = make(map[string]any)
	}
	e.Context[key] = value
	return e
}

// NewPlanningError creates a new PlanningError with the given type and message.
func NewPlanningError(errType ErrorType, message string) *PlanningError {
	return &PlanningError{
		Type:    errType,
		Message: message,
		Context: make(map[string]any),
	}
}

// WrapPlanningError wraps an existing error with planning error context.
func WrapPlanningError(errType ErrorType, message string, cause error) *PlanningError {
	return &PlanningError{
		Type:    errType,
		Message: message,
		Cause:   cause,
		Context: make(map[string]any),
	}
}

// NewInvalidParameterError creates a new error for invalid parameters.
func NewInvalidParameterError(message string) *PlanningError {
	return NewPlanningError(ErrorTypeInvalidParameter, message)
}
