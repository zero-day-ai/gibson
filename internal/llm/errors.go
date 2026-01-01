package llm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zero-day-ai/gibson/internal/types"
)

// LLM error codes follow the Gibson error pattern
const (
	// Provider errors
	ErrProviderNotFound         types.ErrorCode = "LLM_PROVIDER_NOT_FOUND"
	ErrProviderInitFailed       types.ErrorCode = "LLM_PROVIDER_INIT_FAILED"
	ErrProviderUnavailable      types.ErrorCode = "LLM_PROVIDER_UNAVAILABLE"
	ErrProviderUnauthorized     types.ErrorCode = "LLM_PROVIDER_UNAUTHORIZED"
	ErrProviderRateLimited      types.ErrorCode = "LLM_PROVIDER_RATE_LIMITED"
	ErrProviderQuotaExceeded    types.ErrorCode = "LLM_PROVIDER_QUOTA_EXCEEDED"
	ErrLLMProviderNotFound      types.ErrorCode = "LLM_PROVIDER_NOT_FOUND" // Alias for compatibility
	ErrLLMProviderInvalidInput  types.ErrorCode = "LLM_PROVIDER_INVALID_INPUT"
	ErrLLMProviderAlreadyExists types.ErrorCode = "LLM_PROVIDER_ALREADY_EXISTS"
	ErrInvalidSlotConfig        types.ErrorCode = "LLM_INVALID_SLOT_CONFIG"
	ErrNoMatchingProvider       types.ErrorCode = "LLM_NO_MATCHING_PROVIDER"

	// Model errors
	ErrModelNotFound        types.ErrorCode = "LLM_MODEL_NOT_FOUND"
	ErrModelNotSupported    types.ErrorCode = "LLM_MODEL_NOT_SUPPORTED"
	ErrModelContextExceeded types.ErrorCode = "LLM_MODEL_CONTEXT_EXCEEDED"

	// Request errors
	ErrInvalidRequest     types.ErrorCode = "LLM_INVALID_REQUEST"
	ErrInvalidMessage     types.ErrorCode = "LLM_INVALID_MESSAGE"
	ErrInvalidTemperature types.ErrorCode = "LLM_INVALID_TEMPERATURE"
	ErrInvalidMaxTokens   types.ErrorCode = "LLM_INVALID_MAX_TOKENS"
	ErrInvalidTopP        types.ErrorCode = "LLM_INVALID_TOP_P"

	// Tool errors
	ErrInvalidTool         types.ErrorCode = "LLM_INVALID_TOOL"
	ErrToolCallFailed      types.ErrorCode = "LLM_TOOL_CALL_FAILED"
	ErrToolNotFound        types.ErrorCode = "LLM_TOOL_NOT_FOUND"
	ErrInvalidToolArgs     types.ErrorCode = "LLM_INVALID_TOOL_ARGS"
	ErrToolExecutionFailed types.ErrorCode = "LLM_TOOL_EXECUTION_FAILED"

	// Completion errors
	ErrCompletionFailed    types.ErrorCode = "LLM_COMPLETION_FAILED"
	ErrStreamingFailed     types.ErrorCode = "LLM_STREAMING_FAILED"
	ErrContentFiltered     types.ErrorCode = "LLM_CONTENT_FILTERED"
	ErrResponseTruncated   types.ErrorCode = "LLM_RESPONSE_TRUNCATED"
	ErrResponseParseFailed types.ErrorCode = "LLM_RESPONSE_PARSE_FAILED"
	ErrInvalidResponse     types.ErrorCode = "LLM_INVALID_RESPONSE"
	ErrTimeoutExceeded     types.ErrorCode = "LLM_TIMEOUT_EXCEEDED"
	ErrContextCanceled     types.ErrorCode = "LLM_CONTEXT_CANCELED"

	// Network errors
	ErrNetworkFailed  types.ErrorCode = "LLM_NETWORK_FAILED"
	ErrNetworkTimeout types.ErrorCode = "LLM_NETWORK_TIMEOUT"
)

// IsRetryable determines if an error is transient and may succeed on retry.
// This helps implement intelligent retry logic for LLM operations.
func IsRetryable(err error) bool {
	var gibsonErr *types.GibsonError
	if !errors.As(err, &gibsonErr) {
		return false
	}

	// Check if error is already marked as retryable
	if gibsonErr.Retryable {
		return true
	}

	// Determine retryability based on error code
	switch gibsonErr.Code {
	// Network errors are typically retryable
	case ErrNetworkFailed, ErrNetworkTimeout:
		return true

	// Rate limiting and quota errors may succeed after waiting
	case ErrProviderRateLimited, ErrProviderQuotaExceeded:
		return true

	// Provider unavailable may be temporary
	case ErrProviderUnavailable:
		return true

	// Timeout errors may succeed with more time
	case ErrTimeoutExceeded:
		return true

	// Context cancellation is not retryable (user-initiated)
	case ErrContextCanceled:
		return false

	// Auth errors are not retryable
	case ErrProviderUnauthorized:
		return false

	// Invalid requests won't succeed on retry
	case ErrInvalidRequest, ErrInvalidMessage, ErrInvalidTemperature,
		ErrInvalidMaxTokens, ErrInvalidTopP, ErrInvalidTool, ErrInvalidToolArgs:
		return false

	// Model not found or not supported won't change
	case ErrModelNotFound, ErrModelNotSupported:
		return false

	// Content filtering won't change
	case ErrContentFiltered:
		return false

	// Context exceeded won't change
	case ErrModelContextExceeded:
		return false

	// Default to not retryable for safety
	default:
		return false
	}
}

// Helper functions for creating common LLM errors

// NewProviderNotFoundError creates an error for when a provider is not found
func NewProviderNotFoundError(providerName string) *types.GibsonError {
	return types.NewError(ErrProviderNotFound, "provider not found: "+providerName)
}

// NewProviderUnavailableError creates a retryable error for when a provider is temporarily unavailable
func NewProviderUnavailableError(providerName string, cause error) *types.GibsonError {
	return &types.GibsonError{
		Code:      ErrProviderUnavailable,
		Message:   "provider temporarily unavailable: " + providerName,
		Retryable: true,
		Cause:     cause,
	}
}

// NewRateLimitError creates a retryable error for rate limiting
func NewRateLimitError(providerName string) *types.GibsonError {
	return &types.GibsonError{
		Code:      ErrProviderRateLimited,
		Message:   "rate limit exceeded for provider: " + providerName,
		Retryable: true,
		Cause:     nil,
	}
}

// NewModelNotFoundError creates an error for when a model is not found
func NewModelNotFoundError(modelName string) *types.GibsonError {
	return types.NewError(ErrModelNotFound, "model not found: "+modelName)
}

// NewContextExceededError creates an error for when context window is exceeded
func NewContextExceededError(tokenCount, maxTokens int) *types.GibsonError {
	return types.NewError(ErrModelContextExceeded,
		fmt.Sprintf("context window exceeded: %d tokens exceeds maximum of %d", tokenCount, maxTokens))
}

// NewInvalidRequestError creates an error for invalid requests
func NewInvalidRequestError(message string) *types.GibsonError {
	return types.NewError(ErrInvalidRequest, message)
}

// NewToolCallError creates an error for tool call failures
func NewToolCallError(toolName string, cause error) *types.GibsonError {
	return types.WrapError(ErrToolCallFailed, "tool call failed: "+toolName, cause)
}

// NewCompletionError creates an error for completion failures
func NewCompletionError(message string, cause error) *types.GibsonError {
	return types.WrapError(ErrCompletionFailed, message, cause)
}

// NewNetworkError creates a retryable error for network failures
func NewNetworkError(message string, cause error) *types.GibsonError {
	return &types.GibsonError{
		Code:      ErrNetworkFailed,
		Message:   message,
		Retryable: true,
		Cause:     cause,
	}
}

// NewTimeoutError creates a retryable error for timeout failures
func NewTimeoutError(message string) *types.GibsonError {
	return &types.GibsonError{
		Code:      ErrTimeoutExceeded,
		Message:   message,
		Retryable: true,
		Cause:     nil,
	}
}

// Additional error codes for compatibility with existing code
const (
	ErrUsageNotFound  types.ErrorCode = "LLM_USAGE_NOT_FOUND"
	ErrBudgetExceeded types.ErrorCode = "LLM_BUDGET_EXCEEDED"
)

// Provider-specific error creation helpers for langchaingo integration

// NewAuthError creates an authentication error for provider integration
func NewAuthError(provider string, err error) error {
	return NewProviderUnauthorizedError(provider, err)
}

// NewProviderError creates a generic provider error
func NewProviderError(provider string, err error) error {
	if err == nil {
		return NewProviderUnavailableError(provider, fmt.Errorf("unknown error"))
	}
	return NewProviderUnavailableError(provider, err)
}

// NewInvalidInputError creates an invalid input error
func NewInvalidInputError(provider string, message string) error {
	return NewInvalidRequestError(message)
}

// TranslateError translates generic errors into Gibson errors based on error message content
func TranslateError(provider string, err error) error {
	if err == nil {
		return nil
	}

	// Check if it's already a Gibson error
	var gibsonErr *types.GibsonError
	if errors.As(err, &gibsonErr) {
		return err
	}

	errMsg := err.Error()
	lowerMsg := strings.ToLower(errMsg)

	// Detect error type from message
	switch {
	case strings.Contains(lowerMsg, "unauthorized") || strings.Contains(lowerMsg, "authentication") || strings.Contains(lowerMsg, "api key"):
		return NewProviderUnauthorizedError(provider, err)
	case strings.Contains(lowerMsg, "rate limit") || strings.Contains(lowerMsg, "too many requests"):
		return NewRateLimitError(provider)
	case strings.Contains(lowerMsg, "timeout") || strings.Contains(lowerMsg, "deadline"):
		return NewTimeoutError(errMsg)
	case strings.Contains(lowerMsg, "network") || strings.Contains(lowerMsg, "connection"):
		return NewNetworkError(errMsg, err)
	case strings.Contains(lowerMsg, "not found"):
		return NewProviderNotFoundError(provider)
	default:
		return NewProviderUnavailableError(provider, err)
	}
}

// NewProviderUnauthorizedError creates an unauthorized provider error
func NewProviderUnauthorizedError(providerName string, cause error) *types.GibsonError {
	return &types.GibsonError{
		Code:    ErrProviderUnauthorized,
		Message: fmt.Sprintf("provider '%s' authentication failed", providerName),
		Cause:   cause,
	}
}
