package toolexec

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
	sdkproto "github.com/zero-day-ai/sdk/api/gen/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GRPCExecutor executes tool binaries by spawning them as gRPC servers
// and communicating via gRPC calls.
type GRPCExecutor struct {
	mu sync.Mutex
	// runningTools tracks tool processes that are currently running
	runningTools map[string]*runningTool
	// logger for debug output
	logger *slog.Logger
}

// runningTool represents a tool process that is running as a gRPC server
type runningTool struct {
	cmd    *exec.Cmd
	port   int
	conn   *grpc.ClientConn
	client sdkproto.ToolServiceClient
}


// Execute runs a tool binary by spawning it as a gRPC server and calling Execute via gRPC.
//
// The tool binary is invoked without the GIBSON_TOOL_MODE env var, causing it to start
// in gRPC server mode. We wait for it to start, connect via gRPC, execute, and then
// keep the process running for potential reuse.
func (e *GRPCExecutor) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResult, error) {
	toolName := extractToolName(req.BinaryPath)

	e.mu.Lock()
	running, exists := e.runningTools[toolName]
	e.mu.Unlock()

	// If tool is not running, start it
	if !exists || running == nil {
		var err error
		running, err = e.startTool(ctx, toolName, req.BinaryPath)
		if err != nil {
			return nil, types.WrapError(
				ErrToolSpawnFailed,
				fmt.Sprintf("failed to start tool %s as gRPC server", toolName),
				err,
			)
		}

		e.mu.Lock()
		e.runningTools[toolName] = running
		e.mu.Unlock()
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	// Marshal input to JSON
	inputJSON, err := json.Marshal(req.Input)
	if err != nil {
		return nil, types.WrapError(
			ErrToolExecutionFailed,
			"failed to marshal tool input to JSON",
			err,
		)
	}

	// Record start time
	startTime := time.Now()

	// Call the tool via gRPC
	grpcReq := &sdkproto.ToolExecuteRequest{
		InputJson: string(inputJSON),
		TimeoutMs: int64(req.Timeout.Milliseconds()),
	}

	resp, err := running.client.Execute(execCtx, grpcReq)
	duration := time.Since(startTime)

	result := &ExecuteResult{
		Duration: duration,
		ExitCode: 0,
	}

	if err != nil {
		// Check if it was a timeout
		if execCtx.Err() == context.DeadlineExceeded {
			result.ExitCode = -1
			return result, types.WrapError(
				ErrToolTimeout,
				fmt.Sprintf("tool execution exceeded timeout of %v", req.Timeout),
				execCtx.Err(),
			)
		}

		// gRPC error - tool process may have died
		result.ExitCode = 1
		result.Stderr = err.Error()

		// Remove from running tools so it gets restarted next time
		e.mu.Lock()
		delete(e.runningTools, toolName)
		e.mu.Unlock()

		return result, types.WrapError(
			ErrToolExecutionFailed,
			fmt.Sprintf("gRPC call to tool %s failed", toolName),
			err,
		)
	}

	// Check for tool-level error in response
	if resp.Error != nil {
		result.ExitCode = 1
		result.Stderr = resp.Error.Message
		return result, types.WrapError(
			ErrToolExecutionFailed,
			fmt.Sprintf("tool %s returned error: %s", toolName, resp.Error.Message),
			nil,
		)
	}

	// Parse JSON output
	if resp.OutputJson != "" {
		var output map[string]any
		if err := json.Unmarshal([]byte(resp.OutputJson), &output); err != nil {
			return result, types.WrapError(
				ErrInvalidToolOutput,
				"tool output is not valid JSON",
				err,
			)
		}
		result.Output = output
	} else {
		result.Output = make(map[string]any)
	}

	return result, nil
}

// startTool spawns a tool binary as a gRPC server and waits for it to be ready.
func (e *GRPCExecutor) startTool(ctx context.Context, toolName, binaryPath string) (*runningTool, error) {
	// Find an available port
	port, err := findAvailablePort()
	if err != nil {
		return nil, fmt.Errorf("failed to find available port: %w", err)
	}

	e.logger.Debug("starting tool as gRPC server",
		"tool", toolName,
		"binary", binaryPath,
		"port", port)

	// Start the tool process with --port flag
	// IMPORTANT: Use exec.Command (not CommandContext) so the process lifecycle
	// is NOT tied to the request context. Tools are meant to stay alive as
	// persistent gRPC servers across multiple requests.
	cmd := exec.Command(binaryPath, "--port", strconv.Itoa(port))
	cmd.Env = os.Environ() // Don't set GIBSON_TOOL_MODE - let it run as gRPC server

	// Capture stderr for debugging
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start tool process: %w", err)
	}

	// Start goroutine to log stderr
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			e.logger.Debug("tool stderr", "tool", toolName, "line", scanner.Text())
		}
	}()

	// Wait for the tool to be ready (with timeout)
	readyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	addr := fmt.Sprintf("localhost:%d", port)
	conn, err := waitForGRPCReady(readyCtx, addr)
	if err != nil {
		// Kill the process if we couldn't connect
		_ = cmd.Process.Signal(syscall.SIGKILL)
		return nil, fmt.Errorf("tool failed to become ready: %w", err)
	}

	client := sdkproto.NewToolServiceClient(conn)

	e.logger.Info("tool started as gRPC server",
		"tool", toolName,
		"port", port)

	return &runningTool{
		cmd:    cmd,
		port:   port,
		conn:   conn,
		client: client,
	}, nil
}

// Stop stops all running tool processes.
func (e *GRPCExecutor) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for name, running := range e.runningTools {
		if running.conn != nil {
			_ = running.conn.Close()
		}
		if running.cmd != nil && running.cmd.Process != nil {
			e.logger.Debug("stopping tool process", "tool", name, "pid", running.cmd.Process.Pid)
			_ = running.cmd.Process.Signal(syscall.SIGTERM)
			// Give it a moment to shutdown gracefully
			time.AfterFunc(2*time.Second, func() {
				if running.cmd.ProcessState == nil || !running.cmd.ProcessState.Exited() {
					_ = running.cmd.Process.Signal(syscall.SIGKILL)
				}
			})
		}
	}

	e.runningTools = make(map[string]*runningTool)
}

// StopTool stops a specific tool process.
func (e *GRPCExecutor) StopTool(toolName string) {
	e.mu.Lock()
	running, exists := e.runningTools[toolName]
	if exists {
		delete(e.runningTools, toolName)
	}
	e.mu.Unlock()

	if running != nil {
		if running.conn != nil {
			_ = running.conn.Close()
		}
		if running.cmd != nil && running.cmd.Process != nil {
			_ = running.cmd.Process.Signal(syscall.SIGTERM)
		}
	}
}

// findAvailablePort finds an available TCP port.
func findAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// waitForGRPCReady waits for a gRPC server to become ready.
func waitForGRPCReady(ctx context.Context, addr string) (*grpc.ClientConn, error) {
	var conn *grpc.ClientConn
	var err error

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		conn, err = grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Try a simple health check or just verify connection works
		// For now, just try to connect - the tool should be ready quickly
		healthCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		client := sdkproto.NewToolServiceClient(conn)
		_, err = client.Health(healthCtx, &sdkproto.ToolHealthRequest{})
		cancel()

		if err == nil {
			return conn, nil
		}

		// Connection failed, close and retry
		_ = conn.Close()
		time.Sleep(100 * time.Millisecond)
	}
}

// extractToolName extracts the tool name from a binary path.
func extractToolName(binaryPath string) string {
	// Get the base name
	parts := strings.Split(binaryPath, "/")
	return parts[len(parts)-1]
}

// drainReader drains a reader to prevent blocking
func drainReader(r io.Reader) {
	go func() {
		buf := make([]byte, 1024)
		for {
			_, err := r.Read(buf)
			if err != nil {
				return
			}
		}
	}()
}
