package attack

import (
	"errors"
	"fmt"
	"testing"
)

func TestAttackError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *AttackError
		expected string
	}{
		{
			name: "error without cause",
			err: &AttackError{
				Code:    ErrAgentRequired,
				Message: "agent is required",
			},
			expected: "[agent_required] agent is required",
		},
		{
			name: "error with cause",
			err: &AttackError{
				Code:    ErrExecutionFailed,
				Message: "execution failed",
				Cause:   fmt.Errorf("connection refused"),
			},
			expected: "[execution_failed] execution failed: connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("Error() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAttackError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("underlying error")
	err := WrapAttackError(ErrExecutionFailed, "execution failed", cause)

	if unwrapped := err.Unwrap(); unwrapped != cause {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}

	errNoCause := NewAttackError(ErrAgentRequired, "agent required")
	if unwrapped := errNoCause.Unwrap(); unwrapped != nil {
		t.Errorf("Unwrap() = %v, want nil", unwrapped)
	}
}

func TestAttackError_Is(t *testing.T) {
	err1 := NewAgentRequiredError([]string{"agent1"})
	err2 := NewAgentRequiredError([]string{"agent2"})
	err3 := NewAgentNotFoundError("test-agent", []string{"agent1"})

	// Same error code should match
	if !errors.Is(err1, err2) {
		t.Error("errors.Is should return true for same error code")
	}

	// Different error code should not match
	if errors.Is(err1, err3) {
		t.Error("errors.Is should return false for different error code")
	}
}

func TestAttackError_WithContext(t *testing.T) {
	err := NewAttackError(ErrAgentNotFound, "agent not found")
	err.WithContext("agent_name", "test-agent")
	err.WithContext("available_agents", []string{"agent1", "agent2"})

	if err.Context["agent_name"] != "test-agent" {
		t.Error("Context should contain agent_name")
	}

	agents, ok := err.Context["available_agents"].([]string)
	if !ok || len(agents) != 2 {
		t.Error("Context should contain available_agents")
	}
}

func TestNewAgentRequiredError(t *testing.T) {
	agents := []string{"agent1", "agent2"}
	err := NewAgentRequiredError(agents)

	if err.Code != ErrAgentRequired {
		t.Errorf("Code = %v, want %v", err.Code, ErrAgentRequired)
	}

	if !IsAgentRequiredError(err) {
		t.Error("IsAgentRequiredError should return true")
	}

	availableAgents, ok := err.Context["available_agents"].([]string)
	if !ok || len(availableAgents) != 2 {
		t.Error("Context should contain available_agents")
	}
}

func TestNewAgentNotFoundError(t *testing.T) {
	agents := []string{"agent1", "agent2"}
	err := NewAgentNotFoundError("unknown", agents)

	if err.Code != ErrAgentNotFound {
		t.Errorf("Code = %v, want %v", err.Code, ErrAgentNotFound)
	}

	if !IsAgentNotFoundError(err) {
		t.Error("IsAgentNotFoundError should return true")
	}

	if err.Context["agent_name"] != "unknown" {
		t.Error("Context should contain agent_name")
	}
}

func TestNewInvalidTargetError(t *testing.T) {
	err := NewInvalidTargetError("invalid-url", "invalid scheme")

	if err.Code != ErrInvalidTarget {
		t.Errorf("Code = %v, want %v", err.Code, ErrInvalidTarget)
	}

	if !IsInvalidTargetError(err) {
		t.Error("IsInvalidTargetError should return true")
	}

	if err.Context["target"] != "invalid-url" {
		t.Error("Context should contain target")
	}

	if err.Context["reason"] != "invalid scheme" {
		t.Error("Context should contain reason")
	}
}

func TestNewTargetNotFoundError(t *testing.T) {
	err := NewTargetNotFoundError("my-target")

	if err.Code != ErrTargetNotFound {
		t.Errorf("Code = %v, want %v", err.Code, ErrTargetNotFound)
	}

	if !IsTargetNotFoundError(err) {
		t.Error("IsTargetNotFoundError should return true")
	}
}

func TestNewPayloadNotFoundError(t *testing.T) {
	err := NewPayloadNotFoundError("payload-123")

	if err.Code != ErrPayloadNotFound {
		t.Errorf("Code = %v, want %v", err.Code, ErrPayloadNotFound)
	}

	if !IsPayloadNotFoundError(err) {
		t.Error("IsPayloadNotFoundError should return true")
	}
}

func TestNewExecutionFailedError(t *testing.T) {
	cause := fmt.Errorf("network error")
	err := NewExecutionFailedError("connection failed", cause)

	if err.Code != ErrExecutionFailed {
		t.Errorf("Code = %v, want %v", err.Code, ErrExecutionFailed)
	}

	if !IsExecutionFailedError(err) {
		t.Error("IsExecutionFailedError should return true")
	}

	if err.Cause != cause {
		t.Error("Cause should be set")
	}
}

func TestNewTimeoutError(t *testing.T) {
	err := NewTimeoutError("30s")

	if err.Code != ErrTimeout {
		t.Errorf("Code = %v, want %v", err.Code, ErrTimeout)
	}

	if !IsTimeoutError(err) {
		t.Error("IsTimeoutError should return true")
	}

	if err.Context["timeout"] != "30s" {
		t.Error("Context should contain timeout")
	}
}

func TestNewCancelledError(t *testing.T) {
	err := NewCancelledError("user requested")

	if err.Code != ErrCancelled {
		t.Errorf("Code = %v, want %v", err.Code, ErrCancelled)
	}

	if !IsCancelledError(err) {
		t.Error("IsCancelledError should return true")
	}

	if err.Context["reason"] != "user requested" {
		t.Error("Context should contain reason")
	}
}

func TestNewValidationError(t *testing.T) {
	err := NewValidationError("invalid configuration")

	if err.Code != ErrValidation {
		t.Errorf("Code = %v, want %v", err.Code, ErrValidation)
	}

	if !IsValidationError(err) {
		t.Error("IsValidationError should return true")
	}
}

func TestNewPersistenceError(t *testing.T) {
	cause := fmt.Errorf("database error")
	err := NewPersistenceError("save", cause)

	if err.Code != ErrPersistence {
		t.Errorf("Code = %v, want %v", err.Code, ErrPersistence)
	}

	if !IsPersistenceError(err) {
		t.Error("IsPersistenceError should return true")
	}

	if err.Cause != cause {
		t.Error("Cause should be set")
	}
}

func TestNewInternalError(t *testing.T) {
	cause := fmt.Errorf("unexpected error")
	err := NewInternalError("internal failure", cause)

	if err.Code != ErrInternal {
		t.Errorf("Code = %v, want %v", err.Code, ErrInternal)
	}

	if err.Cause != cause {
		t.Error("Cause should be set")
	}
}

func TestErrorTypeCheckers(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		checker func(error) bool
		want    bool
	}{
		{
			name:    "IsAgentRequiredError with matching error",
			err:     NewAgentRequiredError([]string{}),
			checker: IsAgentRequiredError,
			want:    true,
		},
		{
			name:    "IsAgentRequiredError with non-matching error",
			err:     NewAgentNotFoundError("test", []string{}),
			checker: IsAgentRequiredError,
			want:    false,
		},
		{
			name:    "IsAgentRequiredError with non-attack error",
			err:     fmt.Errorf("generic error"),
			checker: IsAgentRequiredError,
			want:    false,
		},
		{
			name:    "IsTimeoutError with matching error",
			err:     NewTimeoutError("30s"),
			checker: IsTimeoutError,
			want:    true,
		},
		{
			name:    "IsCancelledError with matching error",
			err:     NewCancelledError("user requested"),
			checker: IsCancelledError,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.checker(tt.err); got != tt.want {
				t.Errorf("checker() = %v, want %v", got, tt.want)
			}
		})
	}
}
