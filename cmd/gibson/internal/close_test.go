package internal

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCloser is a test double that implements io.Closer
type mockCloser struct {
	closeErr   error
	closeCalls int
}

func (m *mockCloser) Close() error {
	m.closeCalls++
	return m.closeErr
}

func TestCloseWithLog_NilCloser(t *testing.T) {
	// Test that nil closer is handled gracefully
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	CloseWithLog(nil, logger, "test resource")

	// Should not log anything or panic
	assert.Empty(t, logBuf.String(), "should not log for nil closer")
}

func TestCloseWithLog_SuccessfulClose(t *testing.T) {
	// Test successful close - should not log
	closer := &mockCloser{}
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	CloseWithLog(closer, logger, "test resource")

	assert.Equal(t, 1, closer.closeCalls, "should call Close once")
	assert.Empty(t, logBuf.String(), "should not log on successful close")
}

func TestCloseWithLog_CloseError(t *testing.T) {
	// Test close error - should log warning
	expectedErr := errors.New("close failed: resource busy")
	closer := &mockCloser{closeErr: expectedErr}
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	CloseWithLog(closer, logger, "gRPC connection")

	assert.Equal(t, 1, closer.closeCalls, "should call Close once")

	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "failed to close resource", "should log failure message")
	assert.Contains(t, logOutput, "gRPC connection", "should include resource name")
	assert.Contains(t, logOutput, "close failed", "should include error message")
	assert.Contains(t, logOutput, "level=WARN", "should log at warning level")
}

func TestCloseWithLog_NilLogger(t *testing.T) {
	// Test with nil logger - should use default logger
	closer := &mockCloser{closeErr: errors.New("test error")}

	// Should not panic with nil logger
	require.NotPanics(t, func() {
		CloseWithLog(closer, nil, "test resource")
	})

	assert.Equal(t, 1, closer.closeCalls, "should call Close once")
}

func TestCloseWithLog_DeferPattern(t *testing.T) {
	// Test typical defer usage pattern
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	closer := &mockCloser{closeErr: errors.New("cleanup error")}

	func() {
		defer CloseWithLog(closer, logger, "deferred resource")
		// Function logic here
	}()

	assert.Equal(t, 1, closer.closeCalls, "should call Close via defer")
	assert.Contains(t, logBuf.String(), "failed to close resource", "should log via defer")
}

func TestCloseWithLog_DatabaseConnection(t *testing.T) {
	// Test database connection cleanup scenario
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	dbConn := &mockCloser{closeErr: errors.New("database: connection already closed")}

	func() {
		defer CloseWithLog(dbConn, logger, "component database")
		// Simulated database operations
	}()

	assert.Equal(t, 1, dbConn.closeCalls, "should attempt to close database")
	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "component database")
	assert.Contains(t, logOutput, "database: connection already closed")
}

func TestCloseWithLog_GRPCConnection(t *testing.T) {
	// Test gRPC connection cleanup scenario
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	grpcConn := &mockCloser{closeErr: errors.New("grpc: the client connection is closing")}

	func() {
		defer CloseWithLog(grpcConn, logger, "gRPC connection")
		// Simulated gRPC operations
	}()

	assert.Equal(t, 1, grpcConn.closeCalls, "should attempt to close gRPC connection")
	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "gRPC connection")
	assert.Contains(t, logOutput, "the client connection is closing")
}

func TestCloseWithLog_MultipleResources(t *testing.T) {
	// Test multiple resource cleanup with different error states
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	closer1 := &mockCloser{}                                // success
	closer2 := &mockCloser{closeErr: errors.New("error 1")} // error
	closer3 := &mockCloser{}                                // success
	closer4 := &mockCloser{closeErr: errors.New("error 2")} // error

	func() {
		defer CloseWithLog(closer4, logger, "resource 4")
		defer CloseWithLog(closer3, logger, "resource 3")
		defer CloseWithLog(closer2, logger, "resource 2")
		defer CloseWithLog(closer1, logger, "resource 1")
		// Function logic here
	}()

	// All should be closed (reverse order due to defer)
	assert.Equal(t, 1, closer1.closeCalls)
	assert.Equal(t, 1, closer2.closeCalls)
	assert.Equal(t, 1, closer3.closeCalls)
	assert.Equal(t, 1, closer4.closeCalls)

	logOutput := logBuf.String()
	// Only errors should be logged
	assert.Contains(t, logOutput, "resource 2")
	assert.Contains(t, logOutput, "error 1")
	assert.Contains(t, logOutput, "resource 4")
	assert.Contains(t, logOutput, "error 2")

	// Successful closes should not be logged
	assert.NotContains(t, logOutput, "resource 1")
	assert.NotContains(t, logOutput, "resource 3")
}

func TestCloseWithLog_RealIOCloser(t *testing.T) {
	// Test with real io.Closer (io.Pipe)
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	r, w := io.Pipe()

	// Close writer, then try to close reader which should succeed
	w.Close()
	CloseWithLog(r, logger, "pipe reader")

	// Should not log (successful close)
	assert.Empty(t, logBuf.String())
}
