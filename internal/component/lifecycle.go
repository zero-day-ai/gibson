package component

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// DefaultStartupTimeout is the default timeout for component startup
	DefaultStartupTimeout = 30 * time.Second

	// DefaultShutdownTimeout is the default timeout for graceful shutdown
	DefaultShutdownTimeout = 10 * time.Second

	// DefaultPortRangeStart is the default starting port for port scanning
	DefaultPortRangeStart = 50000

	// DefaultPortRangeEnd is the default ending port for port scanning
	DefaultPortRangeEnd = 60000

	// startupHealthCheckInterval is the interval for health checks during startup
	// This is different from DefaultHealthCheckInterval in health.go which is for monitoring
	startupHealthCheckInterval = 500 * time.Millisecond
)

// StatusUpdater is a minimal interface for updating component status in the database.
// This interface avoids import cycles with the database package.
type StatusUpdater interface {
	// UpdateStatus updates status, pid, port, and timestamps
	UpdateStatus(ctx context.Context, id int64, status ComponentStatus, pid, port int) error
}

// LifecycleManager manages the lifecycle of external components.
// It handles starting, stopping, restarting, and status monitoring.
type LifecycleManager interface {
	// StartComponent starts a component and waits for it to become healthy.
	// Returns the assigned port and an error if startup fails or times out.
	StartComponent(ctx context.Context, comp *Component) (int, error)

	// StopComponent gracefully stops a running component.
	// Sends SIGTERM, waits for ShutdownTimeout, then sends SIGKILL if still running.
	StopComponent(ctx context.Context, comp *Component) error

	// RestartComponent stops and then starts a component.
	// Returns the new port assignment and an error if restart fails.
	RestartComponent(ctx context.Context, comp *Component) (int, error)

	// GetStatus returns the current status of a component.
	// Checks process status and updates component state accordingly.
	GetStatus(ctx context.Context, comp *Component) (ComponentStatus, error)
}

// DefaultLifecycleManager is the default implementation of LifecycleManager.
type DefaultLifecycleManager struct {
	mu                sync.RWMutex
	startupTimeout    time.Duration
	shutdownTimeout   time.Duration
	portRangeStart    int
	portRangeEnd      int
	healthCheckClient HealthMonitor
	dao               StatusUpdater          // optional, for persisting metadata to database
	logWriter         LogWriter              // optional, for capturing process output to log files
	processes         map[string]*os.Process // component name -> process
	localTracker      LocalTracker           // required, for local component lifecycle tracking
	tracer            trace.Tracer
}

// NewLifecycleManager creates a new DefaultLifecycleManager with default timeouts.
// The dao and logWriter parameters are optional; pass nil if not needed.
// The localTracker parameter is required for local component tracking.
func NewLifecycleManager(healthMonitor HealthMonitor, dao StatusUpdater, logWriter LogWriter, localTracker LocalTracker) *DefaultLifecycleManager {
	// If no LocalTracker is provided, create a default one
	if localTracker == nil {
		localTracker = NewDefaultLocalTracker()
	}

	return &DefaultLifecycleManager{
		startupTimeout:    DefaultStartupTimeout,
		shutdownTimeout:   DefaultShutdownTimeout,
		portRangeStart:    DefaultPortRangeStart,
		portRangeEnd:      DefaultPortRangeEnd,
		healthCheckClient: healthMonitor,
		dao:               dao,
		logWriter:         logWriter,
		processes:         make(map[string]*os.Process),
		localTracker:      localTracker,
		tracer:            otel.GetTracerProvider().Tracer("gibson.component"),
	}
}

// NewLifecycleManagerWithTimeouts creates a new DefaultLifecycleManager with custom timeouts.
// The dao and logWriter parameters are optional; pass nil if not needed.
// The localTracker parameter is required for local component tracking.
func NewLifecycleManagerWithTimeouts(
	healthMonitor HealthMonitor,
	dao StatusUpdater,
	logWriter LogWriter,
	localTracker LocalTracker,
	startupTimeout, shutdownTimeout time.Duration,
	portRangeStart, portRangeEnd int,
) *DefaultLifecycleManager {
	// If no LocalTracker is provided, create a default one
	if localTracker == nil {
		localTracker = NewDefaultLocalTracker()
	}

	return &DefaultLifecycleManager{
		startupTimeout:    startupTimeout,
		shutdownTimeout:   shutdownTimeout,
		portRangeStart:    portRangeStart,
		portRangeEnd:      portRangeEnd,
		healthCheckClient: healthMonitor,
		dao:               dao,
		logWriter:         logWriter,
		processes:         make(map[string]*os.Process),
		localTracker:      localTracker,
		tracer:            otel.GetTracerProvider().Tracer("gibson.component"),
	}
}

// StartComponent starts a component and waits for it to become healthy.
func (m *DefaultLifecycleManager) StartComponent(ctx context.Context, comp *Component) (int, error) {
	// Start tracing span
	ctx, span := m.tracer.Start(ctx, SpanComponentStart)
	defer span.End()

	start := time.Now()

	if comp == nil {
		err := NewComponentError(ErrCodeValidationFailed, "component cannot be nil")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "validate_component")...)
		return 0, err
	}

	// Add component attributes to span
	span.SetAttributes(ComponentAttributes(comp)...)

	// Check if component is already running
	if comp.IsRunning() && comp.PID > 0 {
		// Verify process still exists
		if m.isProcessAlive(comp.PID) {
			err := NewAlreadyRunningError(comp.Name, comp.PID)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.SetAttributes(ErrorAttributes(err, "check_running")...)
			return 0, err
		}
		// Process died, update status
		comp.UpdateStatus(ComponentStatusStopped)
	}

	// Validate component has required fields
	if comp.Manifest == nil {
		err := NewValidationFailedError("component manifest is required", nil)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "validate_manifest")...)
		return 0, err
	}

	if comp.BinPath == "" {
		err := NewValidationFailedError("component binary path is required", nil)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "validate_binpath")...)
		return 0, err
	}

	// Find available port
	port, err := m.findAvailablePort()
	if err != nil {
		startErr := NewStartFailedError(comp.Name, err, true)
		span.RecordError(startErr)
		span.SetStatus(codes.Error, startErr.Error())
		span.SetAttributes(ErrorAttributes(startErr, "find_port")...)
		return 0, startErr
	}

	span.SetAttributes(attribute.Int(AttrComponentPort, port))

	// Prepare command arguments
	args := append([]string{}, comp.Manifest.Runtime.GetArgs()...)
	args = append(args, "--port", strconv.Itoa(port))

	// Add health endpoint flag if specified in runtime config
	if comp.Manifest.Runtime.HealthURL != "" {
		args = append(args, "--health-endpoint", comp.Manifest.Runtime.HealthURL)
	}

	// Create command using BinPath (binary is self-contained in bin/)
	// Note: We use exec.Command instead of exec.CommandContext because we want
	// the child process to survive after Gibson exits. CommandContext kills the
	// child when the context is cancelled, which happens when Gibson exits.
	cmd := exec.Command(comp.BinPath, args...)

	// Set environment variables
	env := os.Environ()
	for k, v := range comp.Manifest.Runtime.GetEnv() {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	// Detach the child process from the parent's process group.
	// This ensures the component continues running after Gibson CLI exits.
	// Setpgid=true creates a new process group for the child.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Set up log writers if configured
	if m.logWriter != nil {
		// Create stdout writer
		stdoutWriter, err := m.logWriter.CreateWriter(comp.Name, "stdout")
		if err != nil {
			// Log warning but don't fail startup
			span.AddEvent("failed to create stdout log writer", trace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		} else {
			cmd.Stdout = stdoutWriter
		}

		// Create stderr writer
		stderrWriter, err := m.logWriter.CreateWriter(comp.Name, "stderr")
		if err != nil {
			// Log warning but don't fail startup
			span.AddEvent("failed to create stderr log writer", trace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		} else {
			cmd.Stderr = stderrWriter
		}
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		startErr := NewStartFailedError(comp.Name, err, false)
		span.RecordError(startErr)
		span.SetStatus(codes.Error, startErr.Error())
		span.SetAttributes(ErrorAttributes(startErr, "start_process")...)
		return 0, startErr
	}

	// Store process reference
	m.mu.Lock()
	m.processes[comp.Name] = cmd.Process
	m.mu.Unlock()

	// Update component with PID and port
	comp.PID = cmd.Process.Pid
	comp.Port = port

	span.SetAttributes(attribute.Int(AttrComponentPID, comp.PID))

	// Wait for health check with timeout
	healthCtx, cancel := context.WithTimeout(ctx, m.startupTimeout)
	defer cancel()

	// Create protocol-aware health checker from manifest config
	var healthChecker HealthChecker
	var healthCheckConfig *HealthCheckConfig
	if comp.Manifest != nil && comp.Manifest.Runtime != nil {
		healthCheckConfig = comp.Manifest.Runtime.HealthCheck
	}

	// Create a ProtocolAwareHealthChecker for startup checks
	healthChecker = NewProtocolAwareHealthChecker("localhost", port, healthCheckConfig)
	defer healthChecker.Close()

	healthEndpoint := m.buildHealthEndpoint(comp, port)
	if err := m.waitForHealthCheck(healthCtx, healthChecker, healthEndpoint); err != nil {
		// Health check failed, kill the process
		_ = m.killProcess(cmd.Process)
		m.mu.Lock()
		delete(m.processes, comp.Name)
		m.mu.Unlock()
		startErr := NewStartFailedError(comp.Name, err, true)
		span.RecordError(startErr)
		span.SetStatus(codes.Error, startErr.Error())
		span.SetAttributes(ErrorAttributes(startErr, "health_check")...)
		return 0, startErr
	}

	// Register with health monitor for ongoing monitoring
	if m.healthCheckClient != nil {
		// Create a new checker for ongoing monitoring (the startup one is closed)
		monitorChecker := NewProtocolAwareHealthChecker("localhost", port, healthCheckConfig)
		m.healthCheckClient.RegisterComponentWithChecker(comp.Name, monitorChecker)
	}

	// Update component status to running
	comp.UpdateStatus(ComponentStatusRunning)

	// Register with local tracker for filesystem-based lifecycle tracking
	// This creates PID file, acquires lock, and waits for socket
	if err := m.localTracker.Start(ctx, comp); err != nil {
		// Local tracking failed, kill the process
		_ = m.killProcess(cmd.Process)
		m.mu.Lock()
		delete(m.processes, comp.Name)
		m.mu.Unlock()
		startErr := NewStartFailedError(comp.Name, fmt.Errorf("local tracker start failed: %w", err), true)
		span.RecordError(startErr)
		span.SetStatus(codes.Error, startErr.Error())
		span.SetAttributes(ErrorAttributes(startErr, "local_tracker")...)
		return 0, startErr
	}

	// Record successful start
	duration := time.Since(start)
	span.SetStatus(codes.Ok, "component started successfully")
	span.SetAttributes(
		attribute.String(AttrComponentStatus, comp.Status.String()),
		attribute.Int64("gibson.component.startup_duration_ms", duration.Milliseconds()),
	)

	return port, nil
}

// StopComponent gracefully stops a running component.
func (m *DefaultLifecycleManager) StopComponent(ctx context.Context, comp *Component) error {
	// Start tracing span
	ctx, span := m.tracer.Start(ctx, SpanComponentStop)
	defer span.End()

	start := time.Now()

	if comp == nil {
		err := NewComponentError(ErrCodeValidationFailed, "component cannot be nil")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "validate_component")...)
		return err
	}

	// Add component attributes to span
	span.SetAttributes(ComponentAttributes(comp)...)

	if !comp.IsRunning() {
		err := NewNotRunningError(comp.Name)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "check_running")...)
		return err
	}

	// Get process reference
	m.mu.RLock()
	process := m.processes[comp.Name]
	m.mu.RUnlock()

	if process == nil {
		// Try to find process by PID
		if comp.PID > 0 {
			proc, err := os.FindProcess(comp.PID)
			if err != nil {
				comp.UpdateStatus(ComponentStatusStopped)
				// Unregister from local tracker
				_ = m.localTracker.Stop(ctx, comp)
				return nil
			}
			process = proc
		} else {
			comp.UpdateStatus(ComponentStatusStopped)
			// Unregister from local tracker
			_ = m.localTracker.Stop(ctx, comp)
			return nil
		}
	}

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		// Process may already be dead
		if err.Error() == "os: process already finished" {
			comp.UpdateStatus(ComponentStatusStopped)
			m.mu.Lock()
			delete(m.processes, comp.Name)
			m.mu.Unlock()
			// Unregister from local tracker
			_ = m.localTracker.Stop(ctx, comp)
			// Success - process already finished
			duration := time.Since(start)
			span.SetStatus(codes.Ok, "component stopped (already finished)")
			span.SetAttributes(
				attribute.String(AttrComponentStatus, comp.Status.String()),
				attribute.Int64("gibson.component.stop_duration_ms", duration.Milliseconds()),
			)
			return nil
		}
		stopErr := NewStopFailedError(comp.Name, err, false)
		span.RecordError(stopErr)
		span.SetStatus(codes.Error, stopErr.Error())
		span.SetAttributes(ErrorAttributes(stopErr, "signal_term")...)
		return stopErr
	}

	// Wait for process to exit with timeout
	shutdownCtx, cancel := context.WithTimeout(ctx, m.shutdownTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := process.Wait()
		done <- err
	}()

	select {
	case <-shutdownCtx.Done():
		// Timeout reached, send SIGKILL
		span.SetAttributes(attribute.Bool("gibson.component.forced_kill", true))
		if err := process.Kill(); err != nil {
			// Process may have exited between SIGTERM and SIGKILL
			if err.Error() != "os: process already finished" {
				stopErr := NewStopFailedError(comp.Name, err, false)
				span.RecordError(stopErr)
				span.SetStatus(codes.Error, stopErr.Error())
				span.SetAttributes(ErrorAttributes(stopErr, "signal_kill")...)
				return stopErr
			}
		}
		// Wait a bit for kill to complete
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	case err := <-done:
		// Process exited gracefully
		span.SetAttributes(attribute.Bool("gibson.component.graceful_shutdown", true))
		if err != nil && err.Error() != "signal: terminated" && err.Error() != "signal: killed" {
			// Unexpected error, but process is stopped
		}
	}

	// Close log writers and flush buffers
	if m.logWriter != nil {
		if err := m.logWriter.Close(comp.Name); err != nil {
			// Log warning but don't fail the stop operation
			span.AddEvent("failed to close log writer", trace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
	}

	// Update component status
	comp.UpdateStatus(ComponentStatusStopped)

	// Remove process reference
	m.mu.Lock()
	delete(m.processes, comp.Name)
	m.mu.Unlock()

	// Unregister from local tracker (releases lock, removes PID file)
	if err := m.localTracker.Stop(ctx, comp); err != nil {
		// Log warning but don't fail the stop operation
		span.AddEvent("failed to stop local tracker", trace.WithAttributes(
			attribute.String("error", err.Error()),
		))
	}

	// Record successful stop
	duration := time.Since(start)
	span.SetStatus(codes.Ok, "component stopped successfully")
	span.SetAttributes(
		attribute.String(AttrComponentStatus, comp.Status.String()),
		attribute.Int64("gibson.component.stop_duration_ms", duration.Milliseconds()),
	)

	return nil
}

// RestartComponent stops and then starts a component.
func (m *DefaultLifecycleManager) RestartComponent(ctx context.Context, comp *Component) (int, error) {
	if comp == nil {
		return 0, NewComponentError(ErrCodeValidationFailed, "component cannot be nil")
	}

	// Stop component if running
	if comp.IsRunning() {
		if err := m.StopComponent(ctx, comp); err != nil {
			return 0, fmt.Errorf("failed to stop component during restart: %w", err)
		}
	}

	// Start component
	port, err := m.StartComponent(ctx, comp)
	if err != nil {
		return 0, fmt.Errorf("failed to start component during restart: %w", err)
	}

	return port, nil
}

// GetStatus returns the current status of a component.
func (m *DefaultLifecycleManager) GetStatus(ctx context.Context, comp *Component) (ComponentStatus, error) {
	if comp == nil {
		return "", NewComponentError(ErrCodeValidationFailed, "component cannot be nil")
	}

	// Check if component is running using LocalTracker
	// This checks both the lock file and socket existence
	isRunning, err := m.localTracker.IsRunning(ctx, comp.Kind, comp.Name)
	if err != nil {
		// Error checking status - return current status
		return comp.Status, fmt.Errorf("failed to check component status: %w", err)
	}

	if !isRunning {
		// Component is not running according to LocalTracker
		if comp.IsRunning() {
			// Update component status to stopped if it thinks it's running
			comp.UpdateStatus(ComponentStatusStopped)
			m.mu.Lock()
			delete(m.processes, comp.Name)
			m.mu.Unlock()
		}
		return ComponentStatusStopped, nil
	}

	// Component is running, check health if possible
	if comp.Port > 0 {
		healthEndpoint := m.buildHealthEndpoint(comp, comp.Port)
		if m.healthCheckClient != nil {
			if err := m.healthCheckClient.CheckComponent(ctx, healthEndpoint); err != nil {
				// Health check failed, mark as error
				comp.UpdateStatus(ComponentStatusError)
				return ComponentStatusError, nil
			}
		}
	}

	// Update component status to running if it's not already
	if !comp.IsRunning() {
		comp.UpdateStatus(ComponentStatusRunning)
	}

	return ComponentStatusRunning, nil
}

// findAvailablePort scans for an available port starting from portRangeStart.
func (m *DefaultLifecycleManager) findAvailablePort() (int, error) {
	for port := m.portRangeStart; port <= m.portRangeEnd; port++ {
		if m.isPortAvailable(port) {
			return port, nil
		}
	}
	return 0, NewComponentError(
		ErrCodeExecutionFailed,
		fmt.Sprintf("no available ports in range %d-%d", m.portRangeStart, m.portRangeEnd),
	)
}

// isPortAvailable checks if a port is available for use.
func (m *DefaultLifecycleManager) isPortAvailable(port int) bool {
	addr := fmt.Sprintf("localhost:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// waitForHealthCheck waits for the component to pass its health check.
// If checker is nil, falls back to using healthCheckClient with HTTP endpoint.
func (m *DefaultLifecycleManager) waitForHealthCheck(ctx context.Context, checker HealthChecker, healthEndpoint string) error {
	// Use checker if provided
	if checker != nil {
		ticker := time.NewTicker(startupHealthCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return NewTimeoutError("component", "health check")
			case <-ticker.C:
				if err := checker.Check(ctx); err == nil {
					return nil
				}
			}
		}
	}

	// Fall back to HTTP health check client
	if m.healthCheckClient == nil {
		// No health check client, just wait a bit for process to stabilize
		select {
		case <-ctx.Done():
			return NewTimeoutError("component", "startup")
		case <-time.After(time.Second):
			return nil
		}
	}

	ticker := time.NewTicker(startupHealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return NewTimeoutError("component", "health check")
		case <-ticker.C:
			if err := m.healthCheckClient.CheckComponent(ctx, healthEndpoint); err == nil {
				// Health check passed
				return nil
			}
			// Health check failed, continue waiting
		}
	}
}

// buildHealthEndpoint constructs the health check endpoint URL.
func (m *DefaultLifecycleManager) buildHealthEndpoint(comp *Component, port int) string {
	healthURL := "/health"
	if comp.Manifest != nil && comp.Manifest.Runtime != nil && comp.Manifest.Runtime.HealthURL != "" {
		healthURL = comp.Manifest.Runtime.HealthURL
	}

	// Ensure health URL starts with /
	if healthURL[0] != '/' {
		healthURL = "/" + healthURL
	}

	return fmt.Sprintf("http://localhost:%d%s", port, healthURL)
}

// isProcessAlive checks if a process with the given PID is still running.
func (m *DefaultLifecycleManager) isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix systems, FindProcess always succeeds, so we need to send signal 0
	// However, for child processes that haven't been Wait()ed, they may be zombies
	// We also check /proc/<pid>/stat to see if the process is a zombie
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		return false
	}

	// Check if process is a zombie by reading /proc/<pid>/stat
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		// If we can't read stat, process may not exist or be accessible
		return false
	}

	// The stat file format has the process state as the third field
	// Format: pid (comm) state ...
	// State can be: R (running), S (sleeping), D (disk sleep), Z (zombie), T (stopped), etc.
	stat := string(data)
	// Find the closing paren of comm field (to handle comm with spaces)
	closeParen := -1
	for i := len(stat) - 1; i >= 0; i-- {
		if stat[i] == ')' {
			closeParen = i
			break
		}
	}
	if closeParen == -1 || closeParen+2 >= len(stat) {
		return false
	}

	// State is after ") "
	state := stat[closeParen+2]
	// Z = zombie, X = dead
	if state == 'Z' || state == 'X' {
		return false
	}

	return true
}

// killProcess forcefully kills a process.
func (m *DefaultLifecycleManager) killProcess(process *os.Process) error {
	if process == nil {
		return nil
	}
	return process.Kill()
}
