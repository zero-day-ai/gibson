package observability

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestObservabilityErrorCode_Constants verifies all error codes are defined correctly.
func TestObservabilityErrorCode_Constants(t *testing.T) {
	tests := []struct {
		name     string
		code     ObservabilityErrorCode
		expected string
	}{
		{"ErrExporterConnection", ErrExporterConnection, "OBSERVABILITY_EXPORTER_CONNECTION"},
		{"ErrAuthenticationFailed", ErrAuthenticationFailed, "OBSERVABILITY_AUTHENTICATION_FAILED"},
		{"ErrSpanContextMissing", ErrSpanContextMissing, "OBSERVABILITY_SPAN_CONTEXT_MISSING"},
		{"ErrMetricsRegistration", ErrMetricsRegistration, "OBSERVABILITY_METRICS_REGISTRATION"},
		{"ErrBufferOverflow", ErrBufferOverflow, "OBSERVABILITY_BUFFER_OVERFLOW"},
		{"ErrShutdownTimeout", ErrShutdownTimeout, "OBSERVABILITY_SHUTDOWN_TIMEOUT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.code))
		})
	}
}

// TestObservabilityError_Error tests error message formatting.
func TestObservabilityError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *ObservabilityError
		contains []string
	}{
		{
			name: "simple error without cause",
			err:  NewObservabilityError(ErrExporterConnection, "failed to connect"),
			contains: []string{
				"[OBSERVABILITY_EXPORTER_CONNECTION]",
				"failed to connect",
			},
		},
		{
			name: "error with cause",
			err: WrapObservabilityError(
				ErrAuthenticationFailed,
				"authentication failed",
				errors.New("invalid credentials"),
			),
			contains: []string{
				"[OBSERVABILITY_AUTHENTICATION_FAILED]",
				"authentication failed",
				"invalid credentials",
			},
		},
		{
			name: "retryable error",
			err:  NewRetryableObservabilityError(ErrBufferOverflow, "buffer full"),
			contains: []string{
				"[OBSERVABILITY_BUFFER_OVERFLOW]",
				"buffer full",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errMsg := tt.err.Error()
			for _, substring := range tt.contains {
				assert.Contains(t, errMsg, substring)
			}
		})
	}
}

// TestObservabilityError_Unwrap tests error unwrapping.
func TestObservabilityError_Unwrap(t *testing.T) {
	tests := []struct {
		name      string
		err       *ObservabilityError
		wantCause bool
	}{
		{
			name:      "error without cause",
			err:       NewObservabilityError(ErrSpanContextMissing, "span missing"),
			wantCause: false,
		},
		{
			name: "error with cause",
			err: WrapObservabilityError(
				ErrMetricsRegistration,
				"registration failed",
				errors.New("duplicate metric"),
			),
			wantCause: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cause := tt.err.Unwrap()
			if tt.wantCause {
				assert.NotNil(t, cause)
			} else {
				assert.Nil(t, cause)
			}
		})
	}
}

// TestObservabilityError_Is tests error comparison by code.
func TestObservabilityError_Is(t *testing.T) {
	baseErr := NewObservabilityError(ErrExporterConnection, "connection failed")
	sameCodeErr := NewObservabilityError(ErrExporterConnection, "different message")
	differentCodeErr := NewObservabilityError(ErrAuthenticationFailed, "auth failed")
	standardErr := errors.New("standard error")

	tests := []struct {
		name   string
		err    *ObservabilityError
		target error
		want   bool
	}{
		{
			name:   "same error code matches",
			err:    baseErr,
			target: sameCodeErr,
			want:   true,
		},
		{
			name:   "different error code does not match",
			err:    baseErr,
			target: differentCodeErr,
			want:   false,
		},
		{
			name:   "standard error does not match",
			err:    baseErr,
			target: standardErr,
			want:   false,
		},
		{
			name: "wrapped error with same code matches",
			err: WrapObservabilityError(
				ErrExporterConnection,
				"wrapped",
				standardErr,
			),
			target: baseErr,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Is(tt.target)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestNewObservabilityError tests basic error creation.
func TestNewObservabilityError(t *testing.T) {
	err := NewObservabilityError(ErrSpanContextMissing, "context missing")

	assert.Equal(t, ErrSpanContextMissing, err.Code)
	assert.Equal(t, "context missing", err.Message)
	assert.False(t, err.Retryable)
	assert.Nil(t, err.Cause)
}

// TestNewRetryableObservabilityError tests retryable error creation.
func TestNewRetryableObservabilityError(t *testing.T) {
	err := NewRetryableObservabilityError(ErrBufferOverflow, "buffer overflow")

	assert.Equal(t, ErrBufferOverflow, err.Code)
	assert.Equal(t, "buffer overflow", err.Message)
	assert.True(t, err.Retryable)
	assert.Nil(t, err.Cause)
}

// TestWrapObservabilityError tests error wrapping.
func TestWrapObservabilityError(t *testing.T) {
	cause := errors.New("underlying error")
	err := WrapObservabilityError(ErrMetricsRegistration, "registration failed", cause)

	assert.Equal(t, ErrMetricsRegistration, err.Code)
	assert.Equal(t, "registration failed", err.Message)
	assert.False(t, err.Retryable)
	assert.Equal(t, cause, err.Cause)
}

// TestNewExporterConnectionError tests exporter connection error helper.
func TestNewExporterConnectionError(t *testing.T) {
	cause := errors.New("connection refused")
	err := NewExporterConnectionError("http://localhost:4318", cause)

	assert.Equal(t, ErrExporterConnection, err.Code)
	assert.Contains(t, err.Message, "http://localhost:4318")
	assert.True(t, err.Retryable)
	assert.Equal(t, cause, err.Cause)
}

// TestNewAuthenticationError tests authentication error helper.
func TestNewAuthenticationError(t *testing.T) {
	cause := errors.New("invalid api key")
	err := NewAuthenticationError("langfuse", cause)

	assert.Equal(t, ErrAuthenticationFailed, err.Code)
	assert.Contains(t, err.Message, "langfuse")
	assert.False(t, err.Retryable)
	assert.Equal(t, cause, err.Cause)
}

// TestNewSpanContextMissingError tests span context missing error helper.
func TestNewSpanContextMissingError(t *testing.T) {
	err := NewSpanContextMissingError()

	assert.Equal(t, ErrSpanContextMissing, err.Code)
	assert.Contains(t, err.Message, "span context")
	assert.False(t, err.Retryable)
	assert.Nil(t, err.Cause)
}

// TestNewMetricsRegistrationError tests metrics registration error helper.
func TestNewMetricsRegistrationError(t *testing.T) {
	cause := errors.New("duplicate registration")
	err := NewMetricsRegistrationError("request_count", cause)

	assert.Equal(t, ErrMetricsRegistration, err.Code)
	assert.Contains(t, err.Message, "request_count")
	assert.False(t, err.Retryable)
	assert.Equal(t, cause, err.Cause)
}

// TestNewBufferOverflowError tests buffer overflow error helper.
func TestNewBufferOverflowError(t *testing.T) {
	err := NewBufferOverflowError("trace")

	assert.Equal(t, ErrBufferOverflow, err.Code)
	assert.Contains(t, err.Message, "trace")
	assert.True(t, err.Retryable)
	assert.Nil(t, err.Cause)
}

// TestNewShutdownTimeoutError tests shutdown timeout error helper.
func TestNewShutdownTimeoutError(t *testing.T) {
	err := NewShutdownTimeoutError("tracer")

	assert.Equal(t, ErrShutdownTimeout, err.Code)
	assert.Contains(t, err.Message, "tracer")
	assert.False(t, err.Retryable)
	assert.Nil(t, err.Cause)
}

// TestObservabilityError_ErrorsIsCompatibility tests errors.Is() compatibility.
func TestObservabilityError_ErrorsIsCompatibility(t *testing.T) {
	originalErr := errors.New("original error")
	wrappedErr := WrapObservabilityError(
		ErrExporterConnection,
		"connection failed",
		originalErr,
	)

	// Should be able to unwrap to original error
	assert.True(t, errors.Is(wrappedErr, originalErr))

	// Should match by error code
	sameCodeErr := NewObservabilityError(ErrExporterConnection, "different message")
	assert.True(t, errors.Is(wrappedErr, sameCodeErr))

	// Should not match different code
	differentCodeErr := NewObservabilityError(ErrAuthenticationFailed, "auth failed")
	assert.False(t, errors.Is(wrappedErr, differentCodeErr))
}

// TestObservabilityError_ErrorsAsCompatibility tests errors.As() compatibility.
func TestObservabilityError_ErrorsAsCompatibility(t *testing.T) {
	err := WrapObservabilityError(
		ErrShutdownTimeout,
		"timeout occurred",
		errors.New("deadline exceeded"),
	)

	var obsErr *ObservabilityError
	require.True(t, errors.As(err, &obsErr))

	assert.Equal(t, ErrShutdownTimeout, obsErr.Code)
	assert.Equal(t, "timeout occurred", obsErr.Message)
}

// TestObservabilityError_RetryableFlag tests retryable flag behavior.
func TestObservabilityError_RetryableFlag(t *testing.T) {
	tests := []struct {
		name      string
		err       *ObservabilityError
		retryable bool
	}{
		{
			name:      "exporter connection is retryable",
			err:       NewExporterConnectionError("localhost:4318", errors.New("refused")),
			retryable: true,
		},
		{
			name:      "authentication is not retryable",
			err:       NewAuthenticationError("service", errors.New("invalid key")),
			retryable: false,
		},
		{
			name:      "buffer overflow is retryable",
			err:       NewBufferOverflowError("trace"),
			retryable: true,
		},
		{
			name:      "shutdown timeout is not retryable",
			err:       NewShutdownTimeoutError("component"),
			retryable: false,
		},
		{
			name:      "span context missing is not retryable",
			err:       NewSpanContextMissingError(),
			retryable: false,
		},
		{
			name:      "metrics registration is not retryable",
			err:       NewMetricsRegistrationError("metric", errors.New("duplicate")),
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.retryable, tt.err.Retryable)
		})
	}
}

// TestObservabilityError_MessageFormatting tests message formatting for all helpers.
func TestObservabilityError_MessageFormatting(t *testing.T) {
	tests := []struct {
		name     string
		err      *ObservabilityError
		contains string
	}{
		{
			name:     "exporter connection includes endpoint",
			err:      NewExporterConnectionError("http://jaeger:14268", nil),
			contains: "http://jaeger:14268",
		},
		{
			name:     "authentication includes service name",
			err:      NewAuthenticationError("prometheus", nil),
			contains: "prometheus",
		},
		{
			name:     "metrics registration includes metric name",
			err:      NewMetricsRegistrationError("http_requests_total", nil),
			contains: "http_requests_total",
		},
		{
			name:     "buffer overflow includes buffer type",
			err:      NewBufferOverflowError("metrics"),
			contains: "metrics",
		},
		{
			name:     "shutdown timeout includes component name",
			err:      NewShutdownTimeoutError("exporter"),
			contains: "exporter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, tt.err.Error(), tt.contains)
		})
	}
}

// TestObservabilityError_ErrorChaining tests complex error chains.
func TestObservabilityError_ErrorChaining(t *testing.T) {
	// Create a chain of errors
	rootErr := errors.New("network unreachable")
	wrappedErr := WrapObservabilityError(
		ErrExporterConnection,
		"failed to connect to exporter",
		rootErr,
	)

	// Test unwrapping works correctly
	unwrapped := errors.Unwrap(wrappedErr)
	assert.Equal(t, rootErr, unwrapped)

	// Test errors.Is works through the chain
	assert.True(t, errors.Is(wrappedErr, rootErr))

	// Test error message contains both parts
	errMsg := wrappedErr.Error()
	assert.Contains(t, errMsg, "failed to connect to exporter")
	assert.Contains(t, errMsg, "network unreachable")
}

// TestObservabilityError_NilCause tests error behavior with nil cause.
func TestObservabilityError_NilCause(t *testing.T) {
	err := NewObservabilityError(ErrSpanContextMissing, "context missing")

	// Unwrap should return nil
	assert.Nil(t, err.Unwrap())

	// Error message should not contain colon separator
	errMsg := err.Error()
	assert.NotContains(t, errMsg, ": ")
	assert.Contains(t, errMsg, "[OBSERVABILITY_SPAN_CONTEXT_MISSING]")
	assert.Contains(t, errMsg, "context missing")
}

// TestObservabilityError_MultipleWrapping tests multiple levels of wrapping.
func TestObservabilityError_MultipleWrapping(t *testing.T) {
	// Create nested error chain
	level1 := errors.New("tcp connection refused")
	level2 := WrapObservabilityError(
		ErrExporterConnection,
		"failed to establish connection",
		level1,
	)

	// Verify both errors are in the chain
	assert.True(t, errors.Is(level2, level1))

	// Verify error message includes nested information
	errMsg := level2.Error()
	assert.Contains(t, errMsg, "failed to establish connection")
	assert.Contains(t, errMsg, "tcp connection refused")
}

// Benchmark error creation and formatting
func BenchmarkNewObservabilityError(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewObservabilityError(ErrExporterConnection, "connection failed")
	}
}

func BenchmarkWrapObservabilityError(b *testing.B) {
	cause := errors.New("underlying error")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = WrapObservabilityError(ErrMetricsRegistration, "registration failed", cause)
	}
}

func BenchmarkObservabilityError_Error(b *testing.B) {
	err := WrapObservabilityError(
		ErrAuthenticationFailed,
		"authentication failed",
		errors.New("invalid credentials"),
	)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = err.Error()
	}
}

func BenchmarkNewExporterConnectionError(b *testing.B) {
	cause := errors.New("connection refused")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewExporterConnectionError("http://localhost:4318", cause)
	}
}

// TestHelperConstructors_Consistency tests all helper constructors follow patterns.
func TestHelperConstructors_Consistency(t *testing.T) {
	tests := []struct {
		name      string
		createErr func() *ObservabilityError
		wantCode  ObservabilityErrorCode
	}{
		{
			name:      "exporter connection error",
			createErr: func() *ObservabilityError { return NewExporterConnectionError("endpoint", nil) },
			wantCode:  ErrExporterConnection,
		},
		{
			name:      "authentication error",
			createErr: func() *ObservabilityError { return NewAuthenticationError("service", nil) },
			wantCode:  ErrAuthenticationFailed,
		},
		{
			name:      "span context missing error",
			createErr: func() *ObservabilityError { return NewSpanContextMissingError() },
			wantCode:  ErrSpanContextMissing,
		},
		{
			name:      "metrics registration error",
			createErr: func() *ObservabilityError { return NewMetricsRegistrationError("metric", nil) },
			wantCode:  ErrMetricsRegistration,
		},
		{
			name:      "buffer overflow error",
			createErr: func() *ObservabilityError { return NewBufferOverflowError("type") },
			wantCode:  ErrBufferOverflow,
		},
		{
			name:      "shutdown timeout error",
			createErr: func() *ObservabilityError { return NewShutdownTimeoutError("component") },
			wantCode:  ErrShutdownTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.createErr()
			assert.Equal(t, tt.wantCode, err.Code)
			assert.NotEmpty(t, err.Message)
		})
	}
}

// TestErrorMessage_ContainsCode tests that all error messages include the error code.
func TestErrorMessage_ContainsCode(t *testing.T) {
	errors := []*ObservabilityError{
		NewExporterConnectionError("endpoint", nil),
		NewAuthenticationError("service", nil),
		NewSpanContextMissingError(),
		NewMetricsRegistrationError("metric", nil),
		NewBufferOverflowError("buffer"),
		NewShutdownTimeoutError("component"),
	}

	for _, err := range errors {
		t.Run(string(err.Code), func(t *testing.T) {
			errMsg := err.Error()
			assert.True(t, strings.HasPrefix(errMsg, "[OBSERVABILITY_"))
			assert.Contains(t, errMsg, string(err.Code))
		})
	}
}
