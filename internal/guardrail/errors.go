package guardrail

import (
	"fmt"

	"github.com/zero-day-ai/gibson/internal/types"
)

// Guardrail error codes
const (
	ErrGuardrailBlocked       types.ErrorCode = "GUARDRAIL_BLOCKED"
	ErrGuardrailConfigInvalid types.ErrorCode = "GUARDRAIL_CONFIG_INVALID"
	ErrGuardrailNotFound      types.ErrorCode = "GUARDRAIL_NOT_FOUND"
	ErrGuardrailExecution     types.ErrorCode = "GUARDRAIL_EXECUTION"
)

// GuardrailBlockedError represents an error when a guardrail blocks an operation
type GuardrailBlockedError struct {
	GuardrailName string
	GuardrailType GuardrailType
	Reason        string
	Metadata      map[string]any
}

// Error implements the error interface
func (e *GuardrailBlockedError) Error() string {
	return fmt.Sprintf("guardrail '%s' (%s) blocked operation: %s", 
		e.GuardrailName, e.GuardrailType, e.Reason)
}

// Unwrap returns nil as this is a terminal error
func (e *GuardrailBlockedError) Unwrap() error {
	return nil
}

// NewGuardrailBlockedError creates a new GuardrailBlockedError
func NewGuardrailBlockedError(name string, guardType GuardrailType, reason string) *GuardrailBlockedError {
	return &GuardrailBlockedError{
		GuardrailName: name,
		GuardrailType: guardType,
		Reason:        reason,
		Metadata:      make(map[string]any),
	}
}
