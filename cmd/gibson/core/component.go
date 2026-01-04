package core

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/component/build"
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

// ComponentList returns a list of all installed components of the specified kind.
func ComponentList(cc *CommandContext, kind component.ComponentKind, opts ListOptions) (*CommandResult, error) {
	// Validate flags - cannot use both --local and --remote
	if opts.Local && opts.Remote {
		return nil, fmt.Errorf("cannot use both local and remote filters")
	}

	// Get all components of the specified kind from database
	components, err := cc.DAO.List(cc.Ctx, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to list components: %w", err)
	}

	// Apply source filters if specified
	var filtered []*component.Component
	for _, comp := range components {
		if opts.Local && comp.Source != "local" {
			continue
		}
		if opts.Remote && comp.Source != "remote" {
			continue
		}
		filtered = append(filtered, comp)
	}

	return &CommandResult{
		Data: filtered,
	}, nil
}

// ComponentShow returns detailed information about a specific component.
func ComponentShow(cc *CommandContext, kind component.ComponentKind, name string) (*CommandResult, error) {
	// Get component
	comp, err := cc.DAO.GetByName(cc.Ctx, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		return nil, fmt.Errorf("component '%s' not found", name)
	}

	return &CommandResult{
		Data: comp,
	}, nil
}

// ComponentInstall installs a component from a git repository.
func ComponentInstall(cc *CommandContext, kind component.ComponentKind, source string, opts InstallOptions) (*CommandResult, error) {
	// Prepare install options
	installOpts := component.InstallOptions{
		Branch:       opts.Branch,
		Tag:          opts.Tag,
		Force:        opts.Force,
		SkipBuild:    opts.SkipBuild,
		SkipRegister: opts.SkipRegister,
	}

	// Install the component
	start := time.Now()
	result, err := cc.Installer.Install(cc.Ctx, source, kind, installOpts)
	if err != nil {
		return nil, err
	}

	return &CommandResult{
		Data:     result,
		Duration: time.Since(start),
	}, nil
}

// ComponentInstallAll installs all components from a mono-repo.
func ComponentInstallAll(cc *CommandContext, kind component.ComponentKind, source string, opts InstallOptions) (*CommandResult, error) {
	// Prepare install options
	installOpts := component.InstallOptions{
		Branch:       opts.Branch,
		Tag:          opts.Tag,
		Force:        opts.Force,
		SkipBuild:    opts.SkipBuild,
		SkipRegister: opts.SkipRegister,
	}

	// Install all components
	start := time.Now()
	result, err := cc.Installer.InstallAll(cc.Ctx, source, kind, installOpts)
	if err != nil {
		return nil, err
	}

	// Fail if no components were found
	if result.ComponentsFound == 0 {
		return nil, fmt.Errorf("no components found in repository %s", source)
	}

	return &CommandResult{
		Data:     result,
		Duration: time.Since(start),
	}, nil
}

// ComponentUninstall removes an installed component.
func ComponentUninstall(cc *CommandContext, kind component.ComponentKind, name string, opts UninstallOptions) (*CommandResult, error) {
	// Check if component exists
	existing, err := cc.DAO.GetByName(cc.Ctx, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("component '%s' not found", name)
	}

	// Uninstall the component
	start := time.Now()
	result, err := cc.Installer.Uninstall(cc.Ctx, kind, name)
	if err != nil {
		return nil, fmt.Errorf("uninstall failed: %w", err)
	}

	return &CommandResult{
		Data:     result,
		Duration: time.Since(start),
	}, nil
}

// ComponentUpdate updates a component to the latest version.
func ComponentUpdate(cc *CommandContext, kind component.ComponentKind, name string, opts UpdateOptions) (*CommandResult, error) {
	// Check if component exists
	existing, err := cc.DAO.GetByName(cc.Ctx, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("component '%s' not found", name)
	}

	// Prepare update options
	updateOpts := component.UpdateOptions{
		Restart:   opts.Restart,
		SkipBuild: opts.SkipBuild,
	}

	// Update the component
	start := time.Now()
	result, err := cc.Installer.Update(cc.Ctx, kind, name, updateOpts)
	if err != nil {
		return nil, fmt.Errorf("update failed: %w", err)
	}

	return &CommandResult{
		Data:     result,
		Duration: time.Since(start),
	}, nil
}

// ComponentBuild builds a component locally.
func ComponentBuild(cc *CommandContext, kind component.ComponentKind, name string) (*CommandResult, error) {
	// Get component
	comp, err := cc.DAO.GetByName(cc.Ctx, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		return nil, fmt.Errorf("component '%s' not found", name)
	}

	if comp.Manifest == nil || comp.Manifest.Build == nil {
		return nil, fmt.Errorf("component '%s' has no build configuration", name)
	}

	// Create builder
	builder := build.NewDefaultBuildExecutor()

	// Prepare build configuration
	buildCfg := comp.Manifest.Build
	workDir := comp.RepoPath
	if workDir == "" {
		return nil, fmt.Errorf("component '%s' has no repository path configured", name)
	}
	buildConfig := build.BuildConfig{
		WorkDir: workDir,
		Command: "make",
		Args:    []string{"build"},
		Env:     buildCfg.GetEnv(),
	}

	// Override with manifest build command if specified
	if buildCfg.Command != "" {
		// Split command into executable and args
		parts := strings.Fields(buildCfg.Command)
		if len(parts) > 0 {
			buildConfig.Command = parts[0]
			buildConfig.Args = parts[1:]
		}
	}

	// Set working directory if specified
	if buildCfg.WorkDir != "" {
		buildConfig.WorkDir = workDir + "/" + buildCfg.WorkDir
	}

	// Build the component
	start := time.Now()
	result, err := builder.Build(cc.Ctx, buildConfig, comp.Name, comp.Version, "dev")
	if err != nil {
		return nil, fmt.Errorf("build failed: %w", err)
	}

	return &CommandResult{
		Data:     result,
		Duration: time.Since(start),
	}, nil
}

// ComponentStart starts a component process.
func ComponentStart(cc *CommandContext, kind component.ComponentKind, name string) (*CommandResult, error) {
	if cc.RegManager == nil {
		return nil, fmt.Errorf("registry not available (run 'gibson init' first)")
	}

	reg := cc.RegManager.Registry()
	if reg == nil {
		return nil, fmt.Errorf("registry not started")
	}

	// Get component
	comp, err := cc.DAO.GetByName(cc.Ctx, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		return nil, fmt.Errorf("component '%s' not found", name)
	}

	// Check if already running by querying registry
	instances, err := reg.Discover(cc.Ctx, string(kind), name)
	if err != nil {
		return nil, fmt.Errorf("failed to query registry: %w", err)
	}
	if len(instances) > 0 {
		return nil, fmt.Errorf("component '%s' is already running (%d instance(s) found in registry)", name, len(instances))
	}

	// Start component with registry
	port, pid, err := startComponentWithRegistry(cc.Ctx, comp, reg, cc.RegManager, cc.HomeDir)
	if err != nil {
		return nil, fmt.Errorf("failed to start component: %w", err)
	}

	// Update component status in database
	comp.PID = pid
	comp.Port = port
	comp.UpdateStatus(component.ComponentStatusRunning)
	if err := cc.DAO.UpdateStatus(cc.Ctx, comp.ID, comp.Status, comp.PID, comp.Port); err != nil {
		// Log warning but don't fail - component is running
		// Return error in result but not as actual error
	}

	return &CommandResult{
		Data: map[string]interface{}{
			"pid":  pid,
			"port": port,
			"name": name,
		},
	}, nil
}

// ComponentStop stops a running component.
func ComponentStop(cc *CommandContext, kind component.ComponentKind, name string) (*CommandResult, error) {
	if cc.RegManager == nil {
		return nil, fmt.Errorf("registry not available (run 'gibson init' first)")
	}

	reg := cc.RegManager.Registry()
	if reg == nil {
		return nil, fmt.Errorf("registry not started")
	}

	// Get component
	comp, err := cc.DAO.GetByName(cc.Ctx, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		return nil, fmt.Errorf("component '%s' not found", name)
	}

	// Query registry for running instances
	instances, err := reg.Discover(cc.Ctx, string(kind), name)
	if err != nil {
		return nil, fmt.Errorf("failed to query registry: %w", err)
	}
	if len(instances) == 0 {
		return nil, fmt.Errorf("component '%s' is not running (no instances found in registry)", name)
	}

	// Stop all instances
	var lastErr error
	stoppedCount := 0
	for _, instance := range instances {
		if err := stopComponentInstance(cc.Ctx, instance, reg); err != nil {
			lastErr = err
		} else {
			stoppedCount++
		}
	}

	if stoppedCount == 0 && lastErr != nil {
		return nil, fmt.Errorf("failed to stop any instances: %w", lastErr)
	}

	// Update component status in database
	comp.UpdateStatus(component.ComponentStatusStopped)
	comp.PID = 0
	comp.Port = 0
	if err := cc.DAO.UpdateStatus(cc.Ctx, comp.ID, comp.Status, comp.PID, comp.Port); err != nil {
		// Log warning but don't fail - component is stopped
	}

	return &CommandResult{
		Data: map[string]interface{}{
			"name":          name,
			"stopped_count": stoppedCount,
			"total_count":   len(instances),
		},
	}, nil
}

// ComponentStatus returns the status of a component from the registry.
func ComponentStatus(cc *CommandContext, kind component.ComponentKind, name string, opts StatusOptions) (*CommandResult, error) {
	if cc.RegManager == nil {
		return nil, fmt.Errorf("registry not available (run 'gibson init' first)")
	}

	reg := cc.RegManager.Registry()
	if reg == nil {
		return nil, fmt.Errorf("registry not started")
	}

	// If name is provided, get specific component status
	if name != "" {
		// Discover instances of this specific component
		instances, err := reg.Discover(cc.Ctx, kind.String(), name)
		if err != nil {
			return nil, fmt.Errorf("failed to discover component '%s': %w", name, err)
		}

		if len(instances) == 0 {
			return nil, fmt.Errorf("component '%s' not found in registry", name)
		}

		return &CommandResult{
			Data: map[string]interface{}{
				"name":      name,
				"instances": instances,
				"json":      opts.JSON,
			},
		}, nil
	}

	// Otherwise, show all components of this kind
	instances, err := reg.DiscoverAll(cc.Ctx, kind.String())
	if err != nil {
		return nil, fmt.Errorf("failed to discover components: %w", err)
	}

	return &CommandResult{
		Data: map[string]interface{}{
			"instances": instances,
			"json":      opts.JSON,
		},
	}, nil
}

// ComponentLogs retrieves logs for a component.
func ComponentLogs(cc *CommandContext, kind component.ComponentKind, name string, opts LogsOptions) (*CommandResult, error) {
	// Build log path
	logPath := filepath.Join(cc.HomeDir, "logs", string(kind), fmt.Sprintf("%s.log", name))

	// Check if log file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("no logs found for component '%s' (expected at %s)", name, logPath)
	}

	// Open log file
	file, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	// Read logs based on options
	var lines []string
	if opts.Follow {
		// For follow mode, return the file handle and path for streaming
		return &CommandResult{
			Data: map[string]interface{}{
				"follow":   true,
				"log_path": logPath,
			},
		}, nil
	}

	// Read the last N lines
	lines, err = tailFile(file, opts.Lines)
	if err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	return &CommandResult{
		Data: map[string]interface{}{
			"lines":    lines,
			"log_path": logPath,
		},
	}, nil
}

// Helper functions

// tailFile reads the last N lines from a file.
func tailFile(file *os.File, lines int) ([]string, error) {
	scanner := bufio.NewScanner(file)
	var allLines []string

	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Get last N lines
	start := 0
	if len(allLines) > lines {
		start = len(allLines) - lines
	}

	return allLines[start:], nil
}

// startComponentWithRegistry starts a component process and waits for it to register.
func startComponentWithRegistry(
	ctx context.Context,
	comp *component.Component,
	reg sdkregistry.Registry,
	regManager *registry.Manager,
	homeDir string,
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
