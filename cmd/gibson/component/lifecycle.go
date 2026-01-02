package component

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/registry"
	sdkregistry "github.com/zero-day-ai/sdk/registry"
)

const (
	// registryPollInterval is how often to check registry for component registration
	registryPollInterval = 500 * time.Millisecond

	// registryStartTimeout is how long to wait for component to register
	registryStartTimeout = 30 * time.Second

	// registryStopTimeout is how long to wait for graceful shutdown
	registryStopTimeout = 10 * time.Second
)

// RegistryManagerKey is the context key for storing the registry manager.
// This is exported so that the main package can use the same key.
type RegistryManagerKey struct{}

// GetRegistryManager retrieves the registry manager from the context.
// Returns nil if the manager is not present in the context.
func GetRegistryManager(ctx context.Context) *registry.Manager {
	if m, ok := ctx.Value(RegistryManagerKey{}).(*registry.Manager); ok {
		return m
	}
	return nil
}

// WithRegistryManager returns a new context with the registry manager attached.
func WithRegistryManager(ctx context.Context, m *registry.Manager) context.Context {
	return context.WithValue(ctx, RegistryManagerKey{}, m)
}

// newStartCommand creates a start command for the specified component type.
func newStartCommand(cfg Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <name>",
		Short: fmt.Sprintf("Start a %s", cfg.DisplayName),
		Long:  fmt.Sprintf("Start a %s component by name.", cfg.DisplayName),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(cmd, args, cfg)
		},
	}
	return cmd
}

// newStopCommand creates a stop command for the specified component type.
func newStopCommand(cfg Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <name>",
		Short: fmt.Sprintf("Stop a %s", cfg.DisplayName),
		Long:  fmt.Sprintf("Stop a running %s component by name.", cfg.DisplayName),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStop(cmd, args, cfg)
		},
	}
	return cmd
}

// runStart executes the start command for a component.
func runStart(cmd *cobra.Command, args []string, cfg Config) error {
	ctx := cmd.Context()
	componentName := args[0]

	// Get registry manager from context
	regManager := GetRegistryManager(ctx)
	if regManager == nil {
		return fmt.Errorf("registry not available (run 'gibson init' first)")
	}

	reg := regManager.Registry()
	if reg == nil {
		return fmt.Errorf("registry not started")
	}

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create DAO
	dao := database.NewComponentDAO(db)

	// Get component using DAO
	comp, err := dao.GetByName(ctx, cfg.Kind, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		return fmt.Errorf("%s '%s' not found", cfg.DisplayName, componentName)
	}

	// Check if already running by querying registry
	instances, err := reg.Discover(ctx, string(cfg.Kind), componentName)
	if err != nil {
		return fmt.Errorf("failed to query registry: %w", err)
	}
	if len(instances) > 0 {
		return fmt.Errorf("%s '%s' is already running (%d instance(s) found in registry)",
			cfg.DisplayName, componentName, len(instances))
	}

	cmd.Printf("Starting %s '%s'...\n", cfg.DisplayName, componentName)

	// Start component with registry
	port, pid, err := startComponentWithRegistry(ctx, comp, reg, regManager)
	if err != nil {
		return fmt.Errorf("failed to start %s: %w", cfg.DisplayName, err)
	}

	// Update component status in database
	comp.PID = pid
	comp.Port = port
	comp.UpdateStatus(component.ComponentStatusRunning)
	if err := dao.UpdateStatus(ctx, comp.ID, comp.Status, comp.PID, comp.Port); err != nil {
		// Log warning but don't fail - component is running
		cmd.PrintErrf("Warning: failed to update database: %v\n", err)
	}

	cmd.Printf("%s '%s' started successfully\n", capitalizeFirst(cfg.DisplayName), componentName)
	cmd.Printf("PID: %d\n", pid)
	cmd.Printf("Port: %d\n", port)

	// Show log path
	logPath := filepath.Join(homeDir, "logs", string(cfg.Kind), fmt.Sprintf("%s.log", componentName))
	cmd.Printf("Logs: %s\n", logPath)

	return nil
}

// runStop executes the stop command for a component.
func runStop(cmd *cobra.Command, args []string, cfg Config) error {
	ctx := cmd.Context()
	componentName := args[0]

	// Get registry manager from context
	regManager := GetRegistryManager(ctx)
	if regManager == nil {
		return fmt.Errorf("registry not available (run 'gibson init' first)")
	}

	reg := regManager.Registry()
	if reg == nil {
		return fmt.Errorf("registry not started")
	}

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create DAO
	dao := database.NewComponentDAO(db)

	// Get component using DAO
	comp, err := dao.GetByName(ctx, cfg.Kind, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		return fmt.Errorf("%s '%s' not found", cfg.DisplayName, componentName)
	}

	// Query registry for running instances
	instances, err := reg.Discover(ctx, string(cfg.Kind), componentName)
	if err != nil {
		return fmt.Errorf("failed to query registry: %w", err)
	}
	if len(instances) == 0 {
		return fmt.Errorf("%s '%s' is not running (no instances found in registry)",
			cfg.DisplayName, componentName)
	}

	cmd.Printf("Stopping %s '%s' (%d instance(s))...\n",
		cfg.DisplayName, componentName, len(instances))

	// Stop all instances
	var lastErr error
	stoppedCount := 0
	for _, instance := range instances {
		if err := stopComponentInstance(ctx, instance, reg); err != nil {
			cmd.PrintErrf("Warning: failed to stop instance %s: %v\n",
				instance.InstanceID, err)
			lastErr = err
		} else {
			stoppedCount++
		}
	}

	if stoppedCount == 0 && lastErr != nil {
		return fmt.Errorf("failed to stop any instances: %w", lastErr)
	}

	// Update component status in database
	comp.UpdateStatus(component.ComponentStatusStopped)
	comp.PID = 0
	comp.Port = 0
	if err := dao.UpdateStatus(ctx, comp.ID, comp.Status, comp.PID, comp.Port); err != nil {
		// Log warning but don't fail - component is stopped
		cmd.PrintErrf("Warning: failed to update database: %v\n", err)
	}

	cmd.Printf("%s '%s' stopped successfully (%d/%d instances)\n",
		capitalizeFirst(cfg.DisplayName), componentName, stoppedCount, len(instances))

	return nil
}

// startComponentWithRegistry starts a component process and waits for it to register.
func startComponentWithRegistry(
	ctx context.Context,
	comp *component.Component,
	reg sdkregistry.Registry,
	regManager *registry.Manager,
) (port int, pid int, error error) {
	// Validate component has required fields
	if comp.Manifest == nil {
		return 0, 0, fmt.Errorf("component manifest is required")
	}
	if comp.BinPath == "" {
		return 0, 0, fmt.Errorf("component binary path is required")
	}

	// Find available port
	port, err := findAvailablePort()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to find available port: %w", err)
	}

	// Prepare command arguments
	args := append([]string{}, comp.Manifest.Runtime.GetArgs()...)
	args = append(args, "--port", strconv.Itoa(port))

	// Add health endpoint flag if specified in runtime config
	if comp.Manifest.Runtime.HealthURL != "" {
		args = append(args, "--health-endpoint", comp.Manifest.Runtime.HealthURL)
	}

	// Create command
	cmd := exec.Command(comp.BinPath, args...)

	// Set environment variables
	env := os.Environ()
	for k, v := range comp.Manifest.Runtime.GetEnv() {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Add registry endpoints to environment
	registryEndpoint := regManager.Status().Endpoint
	env = append(env, fmt.Sprintf("GIBSON_REGISTRY_ENDPOINTS=%s", registryEndpoint))

	cmd.Env = env

	// Create log directory and file for component output
	homeDir, _ := getGibsonHome()
	logDir := filepath.Join(homeDir, "logs", string(comp.Kind))
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return 0, 0, fmt.Errorf("failed to create log directory: %w", err)
	}

	logPath := filepath.Join(logDir, fmt.Sprintf("%s.log", comp.Name))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create log file: %w", err)
	}

	// Write startup header to log
	fmt.Fprintf(logFile, "\n=== %s started at %s ===\n", comp.Name, time.Now().Format(time.RFC3339))
	fmt.Fprintf(logFile, "Port: %d\n", port)
	fmt.Fprintf(logFile, "Binary: %s\n", comp.BinPath)
	fmt.Fprintf(logFile, "Args: %v\n\n", args)

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Detach the child process from the parent's process group
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return 0, 0, fmt.Errorf("failed to start process: %w", err)
	}

	// Don't close the log file - the child process owns it now

	pid = cmd.Process.Pid

	// Wait for component to register in registry
	if err := waitForRegistration(ctx, reg, string(comp.Kind), comp.Name, registryStartTimeout); err != nil {
		// Registration failed, kill the process
		_ = cmd.Process.Kill()
		return 0, 0, fmt.Errorf("component failed to register: %w", err)
	}

	return port, pid, nil
}

// stopComponentInstance stops a single component instance.
func stopComponentInstance(
	ctx context.Context,
	instance sdkregistry.ServiceInfo,
	reg sdkregistry.Registry,
) error {
	// Extract port from endpoint (format: "host:port")
	port, err := parsePortFromEndpoint(instance.Endpoint)
	if err != nil {
		return fmt.Errorf("failed to parse endpoint: %w", err)
	}

	// Find process by port
	pid, err := findProcessByPort(port)
	if err != nil {
		return fmt.Errorf("failed to find process for port %d: %w", port, err)
	}

	// Find the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		// Process may already be dead
		if strings.Contains(err.Error(), "process already finished") {
			// Already finished, consider it success
			return nil
		}
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	// Wait for deregistration from registry with timeout
	stopCtx, cancel := context.WithTimeout(ctx, registryStopTimeout)
	defer cancel()

	if err := waitForDeregistration(stopCtx, reg, instance.Kind, instance.Name, instance.InstanceID); err != nil {
		// Timeout reached, send SIGKILL
		if err := process.Kill(); err != nil {
			if !strings.Contains(err.Error(), "process already finished") {
				return fmt.Errorf("failed to kill process: %w", err)
			}
		}
		// Wait a bit for kill to complete
		time.Sleep(time.Second)
	}

	return nil
}

// waitForRegistration polls the registry until the component appears.
func waitForRegistration(
	ctx context.Context,
	reg sdkregistry.Registry,
	kind, name string,
	timeout time.Duration,
) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(registryPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for component to register")
		case <-ticker.C:
			instances, err := reg.Discover(ctx, kind, name)
			if err != nil {
				// Continue polling on transient errors
				continue
			}
			if len(instances) > 0 {
				// Component registered successfully
				return nil
			}
		}
	}
}

// waitForDeregistration polls the registry until a specific instance disappears.
func waitForDeregistration(
	ctx context.Context,
	reg sdkregistry.Registry,
	kind, name, instanceID string,
) error {
	ticker := time.NewTicker(registryPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for deregistration")
		case <-ticker.C:
			instances, err := reg.Discover(ctx, kind, name)
			if err != nil {
				// Continue polling on transient errors
				continue
			}

			// Check if our instance is still present
			found := false
			for _, instance := range instances {
				if instance.InstanceID == instanceID {
					found = true
					break
				}
			}

			if !found {
				// Instance deregistered successfully
				return nil
			}
		}
	}
}

// parsePortFromEndpoint extracts the port number from an endpoint string.
func parsePortFromEndpoint(endpoint string) (int, error) {
	// Handle unix sockets (not supported for port extraction)
	if strings.HasPrefix(endpoint, "unix://") {
		return 0, fmt.Errorf("unix sockets not supported for port-based process lookup")
	}

	// Extract port from "host:port" format
	parts := strings.Split(endpoint, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid endpoint format: %s", endpoint)
	}

	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("invalid port number: %s", parts[1])
	}

	return port, nil
}

// findProcessByPort finds the PID of the process listening on the specified port.
func findProcessByPort(port int) (int, error) {
	// Use lsof to find the process listening on the port
	// lsof -t -i:PORT returns the PID
	cmd := exec.Command("lsof", "-t", fmt.Sprintf("-i:%d", port))
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("lsof failed: %w", err)
	}

	// Parse PID from output
	pidStr := strings.TrimSpace(string(output))
	if pidStr == "" {
		return 0, fmt.Errorf("no process found on port %d", port)
	}

	// If multiple PIDs, take the first one
	lines := strings.Split(pidStr, "\n")
	pid, err := strconv.Atoi(lines[0])
	if err != nil {
		return 0, fmt.Errorf("invalid PID: %s", lines[0])
	}

	return pid, nil
}

// findAvailablePort scans for an available port in the default range.
func findAvailablePort() (int, error) {
	// Use the same port range as the component lifecycle manager
	const portRangeStart = 50000
	const portRangeEnd = 60000

	for port := portRangeStart; port <= portRangeEnd; port++ {
		if isPortAvailable(port) {
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", portRangeStart, portRangeEnd)
}

// isPortAvailable checks if a port is available for use.
func isPortAvailable(port int) bool {
	conn, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return false
	}
	defer syscall.Close(conn)

	var sockaddr syscall.SockaddrInet4
	sockaddr.Port = port
	copy(sockaddr.Addr[:], []byte{127, 0, 0, 1})

	err = syscall.Bind(conn, &sockaddr)
	if err != nil {
		return false
	}
	return true
}

// capitalizeFirst capitalizes the first letter of a string.
func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}
